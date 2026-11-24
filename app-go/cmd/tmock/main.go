// Command tmock is the whole Tmock application: REST API, proxy and dashboard (spec §2.2).
//
// It is the composition root: the only package that wires the UI into the server, which
// keeps backend packages free of UI imports (spec §1.3).
package main

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/laposa/tmock/app-go/internal/server"
	"github.com/laposa/tmock/app-go/internal/web"
)

const usage = `Usage: tmock [command]

Commands:
  serve   Run the server (default). Listens on $PORT (default 3000).
  help    Show this help.
`

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, stdout, stderr io.Writer) int {
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
		if err := serve(getenv); err != nil {
			fmt.Fprintf(stderr, "tmock: %v\n", err)
			return 1
		}
		return 0
	case "help", "-h", "-help", "--help":
		fmt.Fprint(stdout, usage)
		return 0
	default:
		fmt.Fprintf(stderr, "tmock: unknown command %q\n\n%s", cmd, usage)
		return 2
	}
}

// serve runs the HTTP server. Full configuration, structured logging and graceful
// shutdown arrive in I01.
func serve(getenv func(string) string) error {
	port, err := parsePort(getenv("PORT"))
	if err != nil {
		return err
	}
	srv := &http.Server{
		Addr:              net.JoinHostPort("", strconv.Itoa(port)),
		Handler:           server.New(server.Options{UI: web.Handler()}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	slog.Info("tmock listening", "server.port", port)
	if err := srv.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}

func parsePort(s string) (int, error) {
	if s == "" {
		return 3000, nil
	}
	p, err := strconv.Atoi(s)
	if err != nil || p < 1 || p > 65535 {
		return 0, fmt.Errorf("PORT must be a number between 1 and 65535, got %q", s)
	}
	return p, nil
}
