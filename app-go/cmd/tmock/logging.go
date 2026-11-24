package main

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"runtime/debug"

	"github.com/laposa/tmock/app-go/internal/config"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/logx/otlp"
)

// module is the instrumentation scope of exported log records (spec §2.4.5).
const module = "github.com/laposa/tmock/app-go"

type logging struct {
	logger   *slog.Logger
	exporter *otlp.Exporter // nil when export is off
}

// setupLogging builds the pipeline of spec §2.4.1: a console handler on stdout and, when
// configured, the OTLP exporter. The exporter reports its own problems to the console
// handler only, never to itself.
func setupLogging(cfg config.Config, stdout io.Writer) logging {
	console := logx.NewConsoleHandler(stdout, cfg.LogFormat, cfg.LogLevel)
	var l logging
	var export slog.Handler
	if cfg.OTLP.Enabled {
		o := cfg.OTLP
		l.exporter = otlp.New(otlp.Options{
			Endpoint:           o.Endpoint,
			Headers:            o.Headers,
			Timeout:            o.Timeout,
			Gzip:               o.Gzip,
			Resource:           resourceAttributes(cfg),
			ScopeName:          module,
			ScopeVersion:       version(),
			ScheduleDelay:      o.ScheduleDelay,
			ExportTimeout:      o.ExportTimeout,
			MaxQueueSize:       o.MaxQueueSize,
			MaxExportBatchSize: o.MaxExportBatchSize,
			Diagnostics:        slog.New(console),
		})
		export = l.exporter.Handler()
	}
	l.logger = slog.New(logx.NewHandler(cfg.LogLevel, console, export))

	if cfg.OTLP.Enabled {
		l.logger.Info("exporting logs over OTLP to "+cfg.OTLP.Endpoint, slog.String(logx.AttrOTLPEndpoint, cfg.OTLP.Endpoint))
	}
	if cfg.SessionSecretGenerated {
		l.logger.Info("SESSION_SECRET is not set; using a random secret, so sessions will not survive a restart")
	}
	return l
}

// flush sends the exporter's remaining records, waiting at most flushTimeout.
func (l logging) flush() {
	if l.exporter == nil {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), flushTimeout)
	defer cancel()
	if err := l.exporter.Shutdown(ctx); err != nil {
		// The exporter has stopped, so this reaches the console only.
		l.logger.Warn("OTLP flush did not finish in time; some log records were lost")
	}
}

// resourceAttributes describes this process (spec §2.4.5). OTEL_RESOURCE_ATTRIBUTES can
// override the defaults; OTEL_SERVICE_NAME, when set, wins for service.name, as the
// OpenTelemetry specification requires.
func resourceAttributes(cfg config.Config) map[string]string {
	attrs := map[string]string{
		"service.name":                "tmock",
		"service.version":             version(),
		"service.instance.id":         newUUID(),
		"deployment.environment.name": cfg.Env,
	}
	maps.Copy(attrs, cfg.OTLP.ResourceAttributes)
	if cfg.OTLP.ServiceName != "" {
		attrs["service.name"] = cfg.OTLP.ServiceName
	}
	return attrs
}

// version is the module version from the build information: the tag when built from a
// tagged module, otherwise the VCS revision (with "-dirty" for local changes), or "devel".
func version() string {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return "devel"
	}
	if v := info.Main.Version; v != "" && v != "(devel)" {
		return v
	}
	var rev string
	var dirty bool
	for _, s := range info.Settings {
		switch s.Key {
		case "vcs.revision":
			rev = s.Value
		case "vcs.modified":
			dirty = s.Value == "true"
		}
	}
	if rev == "" {
		return "devel"
	}
	if len(rev) > 12 {
		rev = rev[:12]
	}
	if dirty {
		rev += "-dirty"
	}
	return rev
}

// newUUID returns a random (version 4) UUID.
func newUUID() string {
	var b [16]byte
	_, _ = rand.Read(b[:]) // never fails (crypto/rand)
	b[6] = b[6]&0x0f | 0x40
	b[8] = b[8]&0x3f | 0x80
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
