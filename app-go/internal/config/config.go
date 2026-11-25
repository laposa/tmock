// Package config loads and validates the application's configuration from environment
// variables and an optional .env file (spec §4).
package config

import (
	"crypto/rand"
	"fmt"
	"log/slog"
	"net"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Config is the validated configuration. It holds secrets: never log it as a whole.
type Config struct {
	Env  string         // TMOCK_ENV
	Port int            // PORT
	TZ   *time.Location // TZ; nil means the system zone

	DatabaseURL string // DATABASE_URL or built from DATABASE_*; "" if not configured (§3.3)

	SessionSecret          []byte // SESSION_SECRET
	SessionSecretGenerated bool   // true if SESSION_SECRET was unset and a random one is used

	DisableCache bool          // DISABLE_CACHE
	CacheTTL     time.Duration // CACHE_TTL

	ServicesAPIKey              string            // SERVICES_API_KEY
	UpstreamOverrides           map[string]string // SERVICES_UPSTREAM_OVERRIDES: service path → URL
	ProxyRemoveForwardedHeaders bool              // PROXY_REMOVE_FORWARDED_HEADERS
	ProxyUpstreamTimeout        time.Duration     // PROXY_UPSTREAM_TIMEOUT
	ProxyCORSAllowedOrigins     []string          // PROXY_CORS_ALLOWED_ORIGINS; empty = any

	LogLevel  slog.Level // LOG_LEVEL
	LogFormat string     // LOG_FORMAT: "text" or "json"
	OTLP      OTLP

	MigrateOnStart bool // MIGRATE_ON_START
	Admin          Admin
}

// OTLP is the log-export configuration (spec §2.4.6), from the standard OTEL_* variables.
type OTLP struct {
	Enabled            bool
	Endpoint           string            // full logs URL, e.g. http://localhost:4318/v1/logs
	Headers            map[string]string // sent with every export; secret
	Timeout            time.Duration     // per HTTP attempt
	Gzip               bool
	ServiceName        string            // OTEL_SERVICE_NAME; "" if unset
	ResourceAttributes map[string]string // OTEL_RESOURCE_ATTRIBUTES
	ScheduleDelay      time.Duration
	ExportTimeout      time.Duration // per batch, including retries
	MaxQueueSize       int
	MaxExportBatchSize int
}

// Admin is the first admin user created by `tmock seed`.
type Admin struct {
	Name, Email, Password string
}

// Production reports whether TMOCK_ENV is "production".
func (c Config) Production() bool { return c.Env == "production" }

// Error lists every configuration problem found, one per line.
type Error struct{ Problems []string }

func (e *Error) Error() string {
	return "invalid configuration:\n  - " + strings.Join(e.Problems, "\n  - ")
}

// MinSessionSecretLength is the shortest SESSION_SECRET accepted.
const MinSessionSecretLength = 32

// Lookup returns the value of a variable and whether it is set.
type Lookup func(key string) (string, bool)

// Chain looks a variable up in each source in turn. Use it to let real environment
// variables win over the .env file: Chain(os.LookupEnv, MapLookup(dotenv)).
func Chain(sources ...Lookup) Lookup {
	return func(key string) (string, bool) {
		for _, src := range sources {
			if v, ok := src(key); ok {
				return v, true
			}
		}
		return "", false
	}
}

// MapLookup adapts a map, such as the result of ParseDotenv.
func MapLookup(m map[string]string) Lookup {
	return func(key string) (string, bool) { v, ok := m[key]; return v, ok }
}

// Load reads and validates every variable of spec §4.2. An empty value counts as unset.
// On failure it returns an *Error that lists all problems, not just the first.
func Load(lookup Lookup) (Config, error) {
	p := &parser{lookup: lookup}
	c := Config{
		Env:  p.pattern("TMOCK_ENV", "development", envName, "letters, digits, '.', '_' or '-'"),
		Port: p.int("PORT", 3000, 1, 65535),
	}

	if tz, ok := p.get("TZ"); ok {
		loc, err := time.LoadLocation(tz)
		if err != nil {
			p.problem("TZ", fmt.Sprintf("unknown time zone %q", tz))
		}
		c.TZ = loc
	}

	c.DatabaseURL = p.databaseURL()

	if secret, ok := p.get("SESSION_SECRET"); ok {
		if len(secret) < MinSessionSecretLength {
			p.problem("SESSION_SECRET", fmt.Sprintf("must be at least %d characters (generate one with: openssl rand -base64 48)", MinSessionSecretLength))
		}
		c.SessionSecret = []byte(secret)
	} else if c.Production() {
		p.problem("SESSION_SECRET", "is required when TMOCK_ENV=production (generate one with: openssl rand -base64 48)")
	} else {
		c.SessionSecret = make([]byte, 32)
		_, _ = rand.Read(c.SessionSecret) // never fails (crypto/rand)
		c.SessionSecretGenerated = true
	}

	c.DisableCache = p.bool("DISABLE_CACHE", false)
	c.CacheTTL = p.duration("CACHE_TTL", 10*time.Minute)

	c.ServicesAPIKey, _ = p.get("SERVICES_API_KEY")
	c.UpstreamOverrides = p.overrides()
	c.ProxyRemoveForwardedHeaders = p.bool("PROXY_REMOVE_FORWARDED_HEADERS", false)
	c.ProxyUpstreamTimeout = p.duration("PROXY_UPSTREAM_TIMEOUT", 30*time.Second)
	c.ProxyCORSAllowedOrigins = p.origins("PROXY_CORS_ALLOWED_ORIGINS")

	c.LogLevel = p.logLevel()
	c.LogFormat = p.oneOf("LOG_FORMAT", "text", "text", "json")
	c.OTLP = p.otlp()

	c.MigrateOnStart = p.bool("MIGRATE_ON_START", false)
	c.Admin.Name = p.string("ADMIN_NAME", "Admin")
	c.Admin.Email, _ = p.get("ADMIN_EMAIL")
	c.Admin.Password, _ = p.get("ADMIN_PASSWORD")

	if len(p.problems) > 0 {
		return Config{}, &Error{Problems: p.problems}
	}
	return c, nil
}

var (
	envName     = regexp.MustCompile(`^[A-Za-z0-9._-]{1,64}$`)
	servicePath = regexp.MustCompile(`^[A-Za-z0-9._-]{1,255}$`) // spec §6.4
	headerName  = regexp.MustCompile("^[!#$%&'*+.^_`|~0-9A-Za-z-]+$")
)

type parser struct {
	lookup   Lookup
	problems []string
}

func (p *parser) problem(key, msg string) {
	p.problems = append(p.problems, key+": "+msg)
}

func (p *parser) get(key string) (string, bool) {
	v, ok := p.lookup(key)
	if !ok || v == "" {
		return "", false
	}
	return v, true
}

func (p *parser) string(key, def string) string {
	if v, ok := p.get(key); ok {
		return v
	}
	return def
}

func (p *parser) pattern(key, def string, re *regexp.Regexp, want string) string {
	v, ok := p.get(key)
	if !ok {
		return def
	}
	if !re.MatchString(v) {
		p.problem(key, fmt.Sprintf("must contain only %s, got %q", want, v))
	}
	return v
}

func (p *parser) oneOf(key, def string, allowed ...string) string {
	v, ok := p.get(key)
	if !ok {
		return def
	}
	for _, a := range allowed {
		if v == a {
			return v
		}
	}
	p.problem(key, fmt.Sprintf("must be one of %s, got %q", strings.Join(allowed, ", "), v))
	return def
}

// bool accepts exactly true, false, 1 and 0 (spec §4.1).
func (p *parser) bool(key string, def bool) bool {
	v, ok := p.get(key)
	if !ok {
		return def
	}
	switch v {
	case "true", "1":
		return true
	case "false", "0":
		return false
	}
	p.problem(key, fmt.Sprintf("must be true, false, 1 or 0, got %q", v))
	return def
}

func (p *parser) int(key string, def, lo, hi int) int {
	v, ok := p.get(key)
	if !ok {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < lo || n > hi {
		p.problem(key, fmt.Sprintf("must be a whole number from %d to %d, got %q", lo, hi, v))
		return def
	}
	return n
}

// duration parses a Go duration such as 10m or 30s. It must be positive.
func (p *parser) duration(key string, def time.Duration) time.Duration {
	v, ok := p.get(key)
	if !ok {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil || d <= 0 {
		p.problem(key, fmt.Sprintf("must be a positive duration such as 30s or 10m, got %q", v))
		return def
	}
	return d
}

// millis parses the OpenTelemetry style of durations: a whole number of milliseconds.
func (p *parser) millis(key string, def time.Duration) time.Duration {
	return time.Duration(p.int(key, int(def/time.Millisecond), 1, 24*60*60*1000)) * time.Millisecond
}

func (p *parser) logLevel() slog.Level {
	v, ok := p.get("LOG_LEVEL")
	if !ok {
		return slog.LevelInfo
	}
	switch strings.ToLower(v) {
	case "debug":
		return slog.LevelDebug
	case "info":
		return slog.LevelInfo
	case "warn":
		return slog.LevelWarn
	case "error":
		return slog.LevelError
	}
	p.problem("LOG_LEVEL", fmt.Sprintf("must be debug, info, warn or error, got %q", v))
	return slog.LevelInfo
}

// databaseURL returns DATABASE_URL, or builds the DSN from DATABASE_* (spec §3.3, §4.2).
// The database is optional until I02, so having neither is not an error; a partial set is.
func (p *parser) databaseURL() string {
	if raw, ok := p.get("DATABASE_URL"); ok {
		u, err := url.Parse(raw)
		if err != nil || (u.Scheme != "postgres" && u.Scheme != "postgresql") || u.Host == "" {
			p.problem("DATABASE_URL", "must be a URL like postgres://user:password@host:5432/dbname")
			return ""
		}
		return raw
	}

	host, hasHost := p.get("DATABASE_HOST")
	name, hasName := p.get("DATABASE_NAME")
	user, hasUser := p.get("DATABASE_USER")
	password, hasPassword := p.get("DATABASE_PASSWORD")
	port := p.int("DATABASE_PORT", 5432, 1, 65535)
	ssl := p.bool("DATABASE_ENABLE_SSL", false)
	if !hasHost && !hasName && !hasUser && !hasPassword {
		return ""
	}
	for _, req := range []struct {
		key string
		set bool
	}{{"DATABASE_HOST", hasHost}, {"DATABASE_NAME", hasName}, {"DATABASE_USER", hasUser}} {
		if !req.set {
			p.problem(req.key, "is required when the other DATABASE_* variables are set (or use DATABASE_URL)")
		}
	}
	u := url.URL{
		Scheme:   "postgres",
		User:     url.User(user),
		Host:     net.JoinHostPort(host, strconv.Itoa(port)),
		Path:     "/" + name,
		RawQuery: "sslmode=disable",
	}
	if hasPassword {
		u.User = url.UserPassword(user, password)
	}
	if ssl {
		u.RawQuery = "sslmode=require"
	}
	return u.String()
}

// overrides parses SERVICES_UPSTREAM_OVERRIDES: path::url,path::url.
func (p *parser) overrides() map[string]string {
	const key = "SERVICES_UPSTREAM_OVERRIDES"
	raw, ok := p.get(key)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for i, entry := range strings.Split(raw, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		path, target, ok := strings.Cut(entry, "::")
		path, target = strings.TrimSpace(path), strings.TrimSpace(target)
		switch {
		case !ok:
			p.problem(key, fmt.Sprintf("entry %d (%q) must look like <service-path>::<upstream-url>", i+1, entry))
		case !servicePath.MatchString(path):
			p.problem(key, fmt.Sprintf("entry %d: service path %q may contain only letters, digits, '.', '_' and '-'", i+1, path))
		case !isHTTPURL(target):
			p.problem(key, fmt.Sprintf("entry %d: %q is not an absolute http or https URL", i+1, target))
		case out[path] != "":
			p.problem(key, fmt.Sprintf("service path %q is listed twice", path))
		default:
			out[path] = target
		}
	}
	return out
}

// origins parses a comma-separated list of origins such as https://app.example.com:8443.
func (p *parser) origins(key string) []string {
	raw, ok := p.get(key)
	if !ok {
		return nil
	}
	var out []string
	for _, o := range strings.Split(raw, ",") {
		o = strings.TrimSpace(o)
		if o == "" {
			continue
		}
		u, err := url.Parse(o)
		if o == "*" {
			p.problem(key, `"*" is not accepted; leave the variable empty to allow any origin`)
			continue
		}
		if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" ||
			u.User != nil || (u.Path != "" && u.Path != "/") || u.RawQuery != "" || u.Fragment != "" {
			p.problem(key, fmt.Sprintf("%q is not an origin like https://app.example.com", o))
			continue
		}
		out = append(out, strings.ToLower(u.Scheme+"://"+u.Host))
	}
	return out
}

// otlp reads the OpenTelemetry exporter variables (spec §2.4.6, §4.2). The
// OTEL_EXPORTER_OTLP_LOGS_* form of a variable wins over the generic one. Values are
// validated even when export is off, so a typo does not go unnoticed.
func (p *parser) otlp() OTLP {
	signal := func(name string) string {
		if _, ok := p.get("OTEL_EXPORTER_OTLP_LOGS_" + name); ok {
			return "OTEL_EXPORTER_OTLP_LOGS_" + name
		}
		return "OTEL_EXPORTER_OTLP_" + name
	}

	o := OTLP{
		Headers:            p.keyValues(signal("HEADERS"), true),
		Timeout:            p.millis(signal("TIMEOUT"), 10*time.Second),
		Gzip:               p.oneOf(signal("COMPRESSION"), "none", "none", "gzip") == "gzip",
		ServiceName:        p.string("OTEL_SERVICE_NAME", ""),
		ResourceAttributes: p.keyValues("OTEL_RESOURCE_ATTRIBUTES", false),
		ScheduleDelay:      p.millis("OTEL_BLRP_SCHEDULE_DELAY", time.Second),
		ExportTimeout:      p.millis("OTEL_BLRP_EXPORT_TIMEOUT", 30*time.Second),
		MaxQueueSize:       p.int("OTEL_BLRP_MAX_QUEUE_SIZE", 2048, 1, 1<<20),
		MaxExportBatchSize: p.int("OTEL_BLRP_MAX_EXPORT_BATCH_SIZE", 512, 1, 1<<20),
	}
	if o.MaxExportBatchSize > o.MaxQueueSize {
		p.problem("OTEL_BLRP_MAX_EXPORT_BATCH_SIZE", fmt.Sprintf("must not be larger than OTEL_BLRP_MAX_QUEUE_SIZE (%d)", o.MaxQueueSize))
	}
	protocol := signal("PROTOCOL")
	if v, ok := p.get(protocol); ok && v != "http/json" {
		p.problem(protocol, fmt.Sprintf("only http/json is supported, got %q (gRPC and http/protobuf are not)", v))
	}

	if v, ok := p.get("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT"); ok {
		if !isHTTPURL(v) {
			p.problem("OTEL_EXPORTER_OTLP_LOGS_ENDPOINT", fmt.Sprintf("%q is not an absolute http or https URL", v))
		}
		o.Endpoint = v
	} else if v, ok := p.get("OTEL_EXPORTER_OTLP_ENDPOINT"); ok {
		if !isHTTPURL(v) {
			p.problem("OTEL_EXPORTER_OTLP_ENDPOINT", fmt.Sprintf("%q is not an absolute http or https URL", v))
		}
		o.Endpoint = strings.TrimSuffix(v, "/") + "/v1/logs"
	}
	exporter := p.oneOf("OTEL_LOGS_EXPORTER", "otlp", "otlp", "none")
	disabled := p.bool("OTEL_SDK_DISABLED", false)
	o.Enabled = o.Endpoint != "" && exporter == "otlp" && !disabled
	return o
}

// keyValues parses key=value,key=value with percent-encoded values (the format of
// OTEL_EXPORTER_OTLP_HEADERS and OTEL_RESOURCE_ATTRIBUTES). Errors never quote the values,
// because headers may hold API keys.
func (p *parser) keyValues(key string, headers bool) map[string]string {
	raw, ok := p.get(key)
	if !ok {
		return nil
	}
	out := map[string]string{}
	for i, entry := range strings.Split(raw, ",") {
		if strings.TrimSpace(entry) == "" {
			continue
		}
		k, v, ok := strings.Cut(entry, "=")
		k = strings.TrimSpace(k)
		val, err := url.PathUnescape(strings.TrimSpace(v))
		switch {
		case !ok || k == "":
			p.problem(key, fmt.Sprintf("entry %d is not key=value", i+1))
		case headers && !headerName.MatchString(k):
			p.problem(key, fmt.Sprintf("entry %d: %q is not a valid header name", i+1, k))
		case err != nil:
			p.problem(key, fmt.Sprintf("entry %d (%s): the value is not correctly percent-encoded", i+1, k))
		default:
			out[k] = val
		}
	}
	return out
}

func isHTTPURL(s string) bool {
	u, err := url.Parse(s)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}
