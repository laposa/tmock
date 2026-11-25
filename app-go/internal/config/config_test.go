package config

import (
	"errors"
	"log/slog"
	"maps"
	"slices"
	"strings"
	"testing"
	"time"
)

func load(t *testing.T, env map[string]string) (Config, error) {
	t.Helper()
	return Load(MapLookup(env))
}

func mustLoad(t *testing.T, env map[string]string) Config {
	t.Helper()
	c, err := load(t, env)
	if err != nil {
		t.Fatal(err)
	}
	return c
}

func problems(t *testing.T, env map[string]string) []string {
	t.Helper()
	_, err := load(t, env)
	var cfgErr *Error
	if !errors.As(err, &cfgErr) {
		t.Fatalf("expected a *config.Error, got %v", err)
	}
	return cfgErr.Problems
}

const secret = "0123456789abcdef0123456789abcdef" // 32 characters

func TestDefaults(t *testing.T) {
	c := mustLoad(t, nil)
	switch {
	case c.Env != "development" || c.Production():
		t.Errorf("Env %q", c.Env)
	case c.Port != 3000:
		t.Errorf("Port %d", c.Port)
	case c.TZ != nil:
		t.Errorf("TZ %v", c.TZ)
	case c.DatabaseURL != "":
		t.Errorf("DatabaseURL %q", c.DatabaseURL)
	case !c.SessionSecretGenerated || len(c.SessionSecret) != 32:
		t.Errorf("session secret: generated=%v len=%d", c.SessionSecretGenerated, len(c.SessionSecret))
	case c.DisableCache || c.CacheTTL != 10*time.Minute:
		t.Errorf("cache %v %v", c.DisableCache, c.CacheTTL)
	case c.ProxyRemoveForwardedHeaders || c.ProxyUpstreamTimeout != 30*time.Second:
		t.Errorf("proxy %v %v", c.ProxyRemoveForwardedHeaders, c.ProxyUpstreamTimeout)
	case c.UpstreamOverrides != nil || c.ProxyCORSAllowedOrigins != nil:
		t.Errorf("lists %v %v", c.UpstreamOverrides, c.ProxyCORSAllowedOrigins)
	case c.LogLevel != slog.LevelInfo || c.LogFormat != "text":
		t.Errorf("log %v %q", c.LogLevel, c.LogFormat)
	case c.MigrateOnStart || c.Admin.Name != "Admin":
		t.Errorf("misc %v %q", c.MigrateOnStart, c.Admin.Name)
	}

	o := c.OTLP
	if o.Enabled || o.Endpoint != "" || o.Timeout != 10*time.Second || o.Gzip ||
		o.ScheduleDelay != time.Second || o.ExportTimeout != 30*time.Second ||
		o.MaxQueueSize != 2048 || o.MaxExportBatchSize != 512 {
		t.Errorf("OTLP defaults %+v", o)
	}
}

func TestEmptyMeansUnset(t *testing.T) {
	c := mustLoad(t, map[string]string{"PORT": "", "DATABASE_HOST": "", "LOG_LEVEL": ""})
	if c.Port != 3000 || c.DatabaseURL != "" || c.LogLevel != slog.LevelInfo {
		t.Errorf("empty values not treated as unset: %+v", c)
	}
}

func TestValues(t *testing.T) {
	c := mustLoad(t, map[string]string{
		"TMOCK_ENV":                      "production",
		"PORT":                           "8080",
		"TZ":                             "Europe/Dublin",
		"SESSION_SECRET":                 secret,
		"DISABLE_CACHE":                  "1",
		"CACHE_TTL":                      "5s",
		"SERVICES_API_KEY":               "key",
		"SERVICES_UPSTREAM_OVERRIDES":    " users::https://staging.example.com/users , products::http://localhost:9000 ",
		"PROXY_REMOVE_FORWARDED_HEADERS": "true",
		"PROXY_UPSTREAM_TIMEOUT":         "2m",
		"PROXY_CORS_ALLOWED_ORIGINS":     "https://App.Example.com, http://localhost:5173/",
		"LOG_LEVEL":                      "DEBUG",
		"LOG_FORMAT":                     "json",
		"MIGRATE_ON_START":               "true",
		"ADMIN_EMAIL":                    "admin@example.com",
	})
	if !c.Production() || c.Port != 8080 || c.TZ.String() != "Europe/Dublin" || string(c.SessionSecret) != secret || c.SessionSecretGenerated {
		t.Errorf("basics %+v", c)
	}
	if !c.DisableCache || c.CacheTTL != 5*time.Second || c.ServicesAPIKey != "key" ||
		!c.ProxyRemoveForwardedHeaders || c.ProxyUpstreamTimeout != 2*time.Minute ||
		c.LogLevel != slog.LevelDebug || c.LogFormat != "json" || !c.MigrateOnStart || c.Admin.Email != "admin@example.com" {
		t.Errorf("values %+v", c)
	}
	wantOverrides := map[string]string{"users": "https://staging.example.com/users", "products": "http://localhost:9000"}
	if !maps.Equal(c.UpstreamOverrides, wantOverrides) {
		t.Errorf("overrides %v", c.UpstreamOverrides)
	}
	if want := []string{"https://app.example.com", "http://localhost:5173"}; !slices.Equal(c.ProxyCORSAllowedOrigins, want) {
		t.Errorf("origins %v", c.ProxyCORSAllowedOrigins)
	}
}

func TestBadValues(t *testing.T) {
	cases := []struct {
		key, value, want string
	}{
		{"PORT", "abc", "PORT: must be a whole number from 1 to 65535"},
		{"PORT", "70000", "PORT: must be a whole number"},
		{"TMOCK_ENV", "prod uction", "TMOCK_ENV: must contain only"},
		{"TZ", "Mars/Olympus", `TZ: unknown time zone "Mars/Olympus"`},
		{"DISABLE_CACHE", "maybe", `DISABLE_CACHE: must be true, false, 1 or 0, got "maybe"`},
		{"DISABLE_CACHE", "TRUE", "DISABLE_CACHE: must be true, false, 1 or 0"},
		{"PROXY_REMOVE_FORWARDED_HEADERS", "true/false", "PROXY_REMOVE_FORWARDED_HEADERS: must be true, false, 1 or 0"},
		{"CACHE_TTL", "10", "CACHE_TTL: must be a positive duration"},
		{"CACHE_TTL", "-5s", "CACHE_TTL: must be a positive duration"},
		{"PROXY_UPSTREAM_TIMEOUT", "0s", "PROXY_UPSTREAM_TIMEOUT: must be a positive duration"},
		{"LOG_LEVEL", "verbose", "LOG_LEVEL: must be debug, info, warn or error"},
		{"LOG_FORMAT", "xml", "LOG_FORMAT: must be one of text, json"},
		{"SESSION_SECRET", "short", "SESSION_SECRET: must be at least 32 characters"},
		{"DATABASE_URL", "mysql://h/db", "DATABASE_URL: must be a URL like postgres://"},
		{"PROXY_CORS_ALLOWED_ORIGINS", "*", `PROXY_CORS_ALLOWED_ORIGINS: "*" is not accepted`},
		{"PROXY_CORS_ALLOWED_ORIGINS", "https://a.example.com/path", "is not an origin"},
		{"PROXY_CORS_ALLOWED_ORIGINS", "a.example.com", "is not an origin"},
		{"MIGRATE_ON_START", "yes", "MIGRATE_ON_START: must be true, false, 1 or 0"},
	}
	for _, tc := range cases {
		got := problems(t, map[string]string{tc.key: tc.value})
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			t.Errorf("%s=%q: problems %q, want one containing %q", tc.key, tc.value, got, tc.want)
		}
	}
}

func TestAllProblemsReported(t *testing.T) {
	_, err := load(t, map[string]string{"PORT": "x", "LOG_LEVEL": "y", "DISABLE_CACHE": "z"})
	msg := err.Error()
	if !strings.HasPrefix(msg, "invalid configuration:\n  - ") {
		t.Errorf("message %q", msg)
	}
	for _, key := range []string{"PORT", "LOG_LEVEL", "DISABLE_CACHE"} {
		if !strings.Contains(msg, "  - "+key+":") {
			t.Errorf("%s missing from %q", key, msg)
		}
	}
}

func TestProductionRequiresSessionSecret(t *testing.T) {
	got := problems(t, map[string]string{"TMOCK_ENV": "production"})
	if len(got) != 1 || !strings.Contains(got[0], "SESSION_SECRET: is required when TMOCK_ENV=production") {
		t.Errorf("problems %q", got)
	}
}

// Secret values must never appear in error messages (spec §2.4.3).
func TestErrorsDoNotLeakSecrets(t *testing.T) {
	_, err := load(t, map[string]string{
		"SESSION_SECRET":             "s3cr3t-value",
		"OTEL_EXPORTER_OTLP_HEADERS": "api-key=s3cr3t-header,broken",
		"OTEL_RESOURCE_ATTRIBUTES":   "team=%zz",
	})
	if err == nil {
		t.Fatal("expected an error")
	}
	for _, s := range []string{"s3cr3t-value", "s3cr3t-header"} {
		if strings.Contains(err.Error(), s) {
			t.Errorf("error leaks %q: %v", s, err)
		}
	}
}

func TestOverrideErrors(t *testing.T) {
	cases := map[string]string{
		"users":                           "must look like <service-path>::<upstream-url>",
		"us/ers::http://x":                "may contain only letters",
		"users::/relative":                "is not an absolute http or https URL",
		"users::ftp://x":                  "is not an absolute http or https URL",
		"users::http://a,users::http://b": `"users" is listed twice`,
	}
	for in, want := range cases {
		got := problems(t, map[string]string{"SERVICES_UPSTREAM_OVERRIDES": in})
		if len(got) != 1 || !strings.Contains(got[0], want) {
			t.Errorf("%q: problems %q, want %q", in, got, want)
		}
	}
}

func TestDatabaseURL(t *testing.T) {
	cases := []struct {
		name string
		env  map[string]string
		want string
	}{
		{"none", map[string]string{"DATABASE_PORT": "5433", "DATABASE_ENABLE_SSL": "true"}, ""},
		{"built", map[string]string{"DATABASE_HOST": "db", "DATABASE_NAME": "tmock", "DATABASE_USER": "u", "DATABASE_PASSWORD": "p@ss/w:rd"},
			"postgres://u:p%40ss%2Fw%3Ard@db:5432/tmock?sslmode=disable"},
		{"built with SSL and port", map[string]string{"DATABASE_HOST": "db", "DATABASE_PORT": "6432", "DATABASE_NAME": "t", "DATABASE_USER": "u", "DATABASE_ENABLE_SSL": "1"},
			"postgres://u@db:6432/t?sslmode=require"},
		{"IPv6 host", map[string]string{"DATABASE_HOST": "::1", "DATABASE_NAME": "t", "DATABASE_USER": "u"},
			"postgres://u@[::1]:5432/t?sslmode=disable"},
		{"DATABASE_URL wins", map[string]string{"DATABASE_URL": "postgresql://x@h/d", "DATABASE_HOST": "ignored"}, "postgresql://x@h/d"},
	}
	for _, tc := range cases {
		if got := mustLoad(t, tc.env).DatabaseURL; got != tc.want {
			t.Errorf("%s: %q, want %q", tc.name, got, tc.want)
		}
	}

	got := problems(t, map[string]string{"DATABASE_HOST": "db"})
	if want := []string{
		"DATABASE_NAME: is required when the other DATABASE_* variables are set (or use DATABASE_URL)",
		"DATABASE_USER: is required when the other DATABASE_* variables are set (or use DATABASE_URL)",
	}; !slices.Equal(got, want) {
		t.Errorf("partial set: %q", got)
	}
}

func TestOTLP(t *testing.T) {
	c := mustLoad(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":         "http://collector:4318/",
		"OTEL_EXPORTER_OTLP_HEADERS":          "x-generic=ignored",
		"OTEL_EXPORTER_OTLP_LOGS_HEADERS":     "api-key=a%2Cb%3Dc, X-Team = core ,",
		"OTEL_EXPORTER_OTLP_TIMEOUT":          "5000",
		"OTEL_EXPORTER_OTLP_LOGS_COMPRESSION": "gzip",
		"OTEL_EXPORTER_OTLP_PROTOCOL":         "http/json",
		"OTEL_SERVICE_NAME":                   "tmock-qa",
		"OTEL_RESOURCE_ATTRIBUTES":            "deployment.region=eu-west-1,team=platform%20core",
		"OTEL_BLRP_SCHEDULE_DELAY":            "200",
		"OTEL_BLRP_MAX_QUEUE_SIZE":            "100",
		"OTEL_BLRP_MAX_EXPORT_BATCH_SIZE":     "10",
		"OTEL_BLRP_EXPORT_TIMEOUT":            "1000",
	})
	o := c.OTLP
	if !o.Enabled || o.Endpoint != "http://collector:4318/v1/logs" {
		t.Errorf("endpoint %v %q", o.Enabled, o.Endpoint)
	}
	if want := map[string]string{"api-key": "a,b=c", "X-Team": "core"}; !maps.Equal(o.Headers, want) {
		t.Errorf("headers %q", o.Headers)
	}
	if want := map[string]string{"deployment.region": "eu-west-1", "team": "platform core"}; !maps.Equal(o.ResourceAttributes, want) {
		t.Errorf("resource %q", o.ResourceAttributes)
	}
	if o.Timeout != 5*time.Second || !o.Gzip || o.ServiceName != "tmock-qa" || o.ScheduleDelay != 200*time.Millisecond ||
		o.MaxQueueSize != 100 || o.MaxExportBatchSize != 10 || o.ExportTimeout != time.Second {
		t.Errorf("OTLP %+v", o)
	}

	logs := mustLoad(t, map[string]string{
		"OTEL_EXPORTER_OTLP_ENDPOINT":      "http://ignored:4318",
		"OTEL_EXPORTER_OTLP_LOGS_ENDPOINT": "https://otlp.example.com/custom/logs",
	}).OTLP
	if logs.Endpoint != "https://otlp.example.com/custom/logs" {
		t.Errorf("logs endpoint %q", logs.Endpoint)
	}

	for name, env := range map[string]map[string]string{
		"exporter none": {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_LOGS_EXPORTER": "none"},
		"sdk disabled":  {"OTEL_EXPORTER_OTLP_ENDPOINT": "http://c:4318", "OTEL_SDK_DISABLED": "true"},
		"no endpoint":   {"OTEL_EXPORTER_OTLP_HEADERS": "a=b"},
	} {
		if mustLoad(t, env).OTLP.Enabled {
			t.Errorf("%s: export enabled", name)
		}
	}
}

func TestOTLPErrors(t *testing.T) {
	cases := []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{"OTEL_EXPORTER_OTLP_PROTOCOL": "grpc"}, "OTEL_EXPORTER_OTLP_PROTOCOL: only http/json is supported"},
		{map[string]string{"OTEL_EXPORTER_OTLP_LOGS_PROTOCOL": "http/protobuf"}, "OTEL_EXPORTER_OTLP_LOGS_PROTOCOL: only http/json is supported"},
		{map[string]string{"OTEL_EXPORTER_OTLP_HEADERS": "novalue"}, "OTEL_EXPORTER_OTLP_HEADERS: entry 1 is not key=value"},
		{map[string]string{"OTEL_EXPORTER_OTLP_HEADERS": "a=b,=c"}, "entry 2 is not key=value"},
		{map[string]string{"OTEL_EXPORTER_OTLP_HEADERS": "bad header=x"}, "is not a valid header name"},
		{map[string]string{"OTEL_EXPORTER_OTLP_HEADERS": "a=%zz"}, "not correctly percent-encoded"},
		{map[string]string{"OTEL_EXPORTER_OTLP_ENDPOINT": "collector:4318"}, "OTEL_EXPORTER_OTLP_ENDPOINT: \"collector:4318\" is not an absolute"},
		{map[string]string{"OTEL_EXPORTER_OTLP_COMPRESSION": "zstd"}, "OTEL_EXPORTER_OTLP_COMPRESSION: must be one of none, gzip"},
		{map[string]string{"OTEL_EXPORTER_OTLP_TIMEOUT": "10s"}, "OTEL_EXPORTER_OTLP_TIMEOUT: must be a whole number"},
		{map[string]string{"OTEL_LOGS_EXPORTER": "console"}, "OTEL_LOGS_EXPORTER: must be one of otlp, none"},
		{map[string]string{"OTEL_BLRP_MAX_QUEUE_SIZE": "10", "OTEL_BLRP_MAX_EXPORT_BATCH_SIZE": "20"}, "must not be larger than OTEL_BLRP_MAX_QUEUE_SIZE (10)"},
	}
	for _, tc := range cases {
		got := problems(t, tc.env)
		if len(got) != 1 || !strings.Contains(got[0], tc.want) {
			t.Errorf("%v: problems %q, want one containing %q", tc.env, got, tc.want)
		}
	}
}

func TestChain(t *testing.T) {
	env := MapLookup(map[string]string{"PORT": "1", "ONLY_ENV": "e"})
	dotenv := MapLookup(map[string]string{"PORT": "2", "ONLY_FILE": "f"})
	l := Chain(env, dotenv)
	for key, want := range map[string]string{"PORT": "1", "ONLY_ENV": "e", "ONLY_FILE": "f"} {
		if v, ok := l(key); !ok || v != want {
			t.Errorf("%s = %q, %v", key, v, ok)
		}
	}
	if _, ok := l("MISSING"); ok {
		t.Error("MISSING found")
	}
}
