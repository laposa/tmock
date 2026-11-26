// Command echo-upstream is a development tool: a fake upstream that answers every request
// with a JSON description of what it received (method, path, query, headers and body), so
// the proxy can be tried and tested by hand (spec §15.3, I09).
//
//	go run ./cmd/echo-upstream                       # listens on localhost:8081
//	go run ./cmd/echo-upstream -addr :9000 -status 503 -delay 2s -gzip
//
// Each request is also printed to stdout as one line.
package main

import (
	"compress/gzip"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"
)

// Echo is the JSON the server answers with.
type Echo struct {
	Method  string              `json:"method"`
	URL     string              `json:"url"`   // path and query as received, escaped
	Path    string              `json:"path"`  // the escaped path
	Query   string              `json:"query"` // the raw query, without '?'
	Host    string              `json:"host"`
	Headers map[string][]string `json:"headers"`
	Body    string              `json:"body"`                 // as text, when it is valid UTF-8
	BodyB64 []byte              `json:"bodyBase64,omitempty"` // otherwise base64
	Size    int64               `json:"bodySize"`
}

// Options are the command-line flags.
type Options struct {
	Status int           // response status
	Delay  time.Duration // wait before answering
	Gzip   bool          // gzip the response when the client accepts it
}

func main() {
	addr := flag.String("addr", "localhost:8081", "listen address")
	var opts Options
	flag.IntVar(&opts.Status, "status", http.StatusOK, "response status code")
	flag.DurationVar(&opts.Delay, "delay", 0, "delay before each response, e.g. 500ms")
	flag.BoolVar(&opts.Gzip, "gzip", false, "gzip-compress responses for clients that send Accept-Encoding: gzip")
	flag.Parse()
	if opts.Status < 100 || opts.Status > 599 {
		fmt.Fprintln(os.Stderr, "echo-upstream: -status must be 100–599")
		os.Exit(2)
	}

	fmt.Printf("echo-upstream listening on http://%s (status %d, delay %s, gzip %v)\n", *addr, opts.Status, opts.Delay, opts.Gzip)
	srv := &http.Server{Addr: *addr, Handler: Handler(opts, os.Stdout), ReadHeaderTimeout: 10 * time.Second}
	log.Fatal(srv.ListenAndServe())
}

// Handler echoes every request as JSON and writes one line per request to logw.
func Handler(opts Options, logw io.Writer) http.Handler {
	if opts.Status == 0 {
		opts.Status = http.StatusOK
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, "reading the body: "+err.Error(), http.StatusBadRequest)
			return
		}
		if opts.Delay > 0 {
			select {
			case <-time.After(opts.Delay):
			case <-r.Context().Done():
				return
			}
		}
		e := Echo{
			Method:  r.Method,
			URL:     r.URL.RequestURI(),
			Path:    r.URL.EscapedPath(),
			Query:   r.URL.RawQuery,
			Host:    r.Host,
			Headers: r.Header,
			Size:    int64(len(body)),
		}
		if utf8.Valid(body) {
			e.Body = string(body)
		} else {
			e.BodyB64 = body
		}
		if logw != nil {
			fmt.Fprintf(logw, "%s %s %s %d bytes\n", time.Now().Format(time.TimeOnly), r.Method, e.URL, e.Size)
		}

		out, _ := json.MarshalIndent(e, "", "  ") // cannot fail for these types
		out = append(out, '\n')
		w.Header().Set("Content-Type", "application/json")
		if opts.Gzip && acceptsGzip(r) {
			w.Header().Set("Content-Encoding", "gzip")
			w.Header().Set("Vary", "Accept-Encoding")
			w.WriteHeader(opts.Status)
			zw := gzip.NewWriter(w)
			_, _ = zw.Write(out)
			_ = zw.Close()
			return
		}
		w.WriteHeader(opts.Status)
		_, _ = w.Write(out)
	})
}

// acceptsGzip reports whether the request's Accept-Encoding lists gzip.
func acceptsGzip(r *http.Request) bool {
	for _, v := range r.Header.Values("Accept-Encoding") {
		for part := range strings.SplitSeq(v, ",") {
			name, _, _ := strings.Cut(part, ";")
			if strings.EqualFold(strings.TrimSpace(name), "gzip") {
				return true
			}
		}
	}
	return false
}
