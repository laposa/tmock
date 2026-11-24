// Command tmock is the whole Tmock application: REST API, proxy and dashboard (spec §2.2).
//
// It is the composition root: the only package that wires the UI into the server, which
// keeps backend packages free of UI imports (spec §1.3).
package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/laposa/tmock/app-go/internal/config"
	"github.com/laposa/tmock/app-go/internal/logx"
	"github.com/laposa/tmock/app-go/internal/server"
	"github.com/laposa/tmock/app-go/internal/web"
)

const usage = `Usage: tmock [command]

Commands:
  serve   Run the server (default). Configuration comes from the environment and an
          optional .env file in the working directory; see app-go/README.md.
  help    Show this help.
`

const (
	// drainTimeout is how long a shutdown waits for open requests before closing them.
	// Together with the log flush it stays under Kubernetes' default 30s grace period.
	drainTimeout = 20 * time.Second
	// flushTimeout bounds the final OTLP export (spec §2.4.5).
	flushTimeout = 5 * time.Second
)

func main() {
	os.Exit(run(context.Background(), os.Args[1:], os.LookupEnv, os.Stdout, os.Stderr))
}

// run executes a command and returns the process exit code: 0 on success, 1 when the
// command fails, 2 for a usage error.
func run(ctx context.Context, args []string, env config.Lookup, stdout, stderr io.Writer) int {
	cmd := "serve"
	if len(args) > 0 {
		cmd = args[0]
	}
	switch cmd {
	case "serve":
		if len(args) > 1 {
			fmt.Fprintf(stderr, "serve takes no arguments\n\n%s", usage)
			return 2
		}
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "tmock: unknown command %q\n\n%s", cmd, usage)
		return 2
	}

	cfg, err := loadConfig(env)
	if err != nil {
		fmt.Fprintf(stderr, "tmock: %v\n", err)
		return 1
	}
	if cfg.TZ != nil {
		time.Local = cfg.TZ // TZ may come from .env, which the Go runtime does not read
	}

	logs := setupLogging(cfg, stdout)
	slog.SetDefault(logs.logger)
	defer logs.flush()

	// The first SIGINT/SIGTERM starts a graceful shutdown. Once it has, the signals get
	// their default behaviour back, so a second Ctrl-C stops the process at once.
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()
	context.AfterFunc(ctx, stop)

	ln, err := net.Listen("tcp", net.JoinHostPort("", strconv.Itoa(cfg.Port)))
	if err != nil {
		logs.logger.Error("cannot listen: " + err.Error())
		return 1
	}
	handler := server.New(server.Options{UI: web.Handler(), Logger: logs.logger})
	if err := serve(ctx, logs.logger, ln, handler, cfg.Env, drainTimeout); err != nil {
		logs.logger.Error("server failed: " + err.Error())
		return 1
	}
	return 0
}

// loadConfig reads the configuration from env, with .env in the working directory as
// fallback (spec §4.1).
func loadConfig(env config.Lookup) (config.Config, error) {
	dotenv := map[string]string{}
	f, err := os.Open(".env")
	switch {
	case errors.Is(err, fs.ErrNotExist):
	case err != nil:
		return config.Config{}, err
	default:
		defer f.Close()
		if dotenv, err = config.ParseDotenv(f); err != nil {
			return config.Config{}, err
		}
	}
	return config.Load(config.Chain(env, config.MapLookup(dotenv)))
}

// serve runs the HTTP server on ln until ctx is cancelled, then shuts it down: it stops
// accepting connections and waits up to drain for open requests, after which the
// remaining connections are closed.
func serve(ctx context.Context, logger *slog.Logger, ln net.Listener, h http.Handler, env string, drain time.Duration) error {
	srv := &http.Server{
		Handler:           h,
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
		ErrorLog:          slog.NewLogLogger(logger.Handler(), slog.LevelWarn),
		// No WriteTimeout: proxied responses may stream for a long time.
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()

	port := ln.Addr().(*net.TCPAddr).Port
	logger.LogAttrs(ctx, slog.LevelInfo, fmt.Sprintf("tmock listening on port %d", port),
		slog.String(logx.AttrEventName, logx.EventServerStarted),
		slog.Int(logx.AttrServerPort, port),
		slog.String(logx.AttrEnv, env),
	)

	select {
	case err := <-served:
		return err // Serve failed on its own
	case <-ctx.Done():
	}

	logger.Info(fmt.Sprintf("shutting down; waiting up to %s for open requests", drain))
	start := time.Now()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), drain)
	defer cancel()
	forced := false
	if err := srv.Shutdown(shutdownCtx); err != nil {
		forced = true
		_ = srv.Close()
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	msg := "tmock stopped"
	if forced {
		msg = "tmock stopped; open requests were cut off after " + drain.String()
	}
	logger.LogAttrs(context.Background(), slog.LevelInfo, msg,
		slog.String(logx.AttrEventName, logx.EventServerStopped),
		slog.Int64(logx.AttrDurationMS, time.Since(start).Milliseconds()),
		slog.Bool(logx.AttrShutdownForced, forced),
	)
	return nil
}
