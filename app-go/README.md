# Tmock (Go)

The Go rewrite of Tmock: REST API, proxy/mock engine and server-rendered dashboard in one
module and one binary. The legacy Node app lives in `../api` and `../app` until cutover.

- **Why and what:** [../intent.md](../intent.md) and [../spec.md](../spec.md).
- **Status:** iteration **I01** (configuration, logging, lifecycle). The binary serves the
  health check and a "coming soon" page, logs every request, can export its logs over OTLP,
  and shuts down gracefully. Progress is tracked in spec §16.

## Prerequisites

- Go 1.25 or newer (see [Installing Go](#installing-go) below).
- Docker with Compose v2, for PostgreSQL and the optional OpenTelemetry Collector.
- `psql` (optional), to look at the database from the host.

## Installing Go

Use the official release from <https://go.dev/dl/>. Distribution packages (`apt install golang`
and similar) are often older than the 1.25 this module needs.

### Linux and WSL

Pick the latest stable version from <https://go.dev/dl/> (use `linux-arm64` on ARM machines):

```bash
GO_VERSION=1.27.4   # replace with the current release
curl -fLO "https://go.dev/dl/go${GO_VERSION}.linux-amd64.tar.gz"
sudo rm -rf /usr/local/go                      # remove any previous install first
sudo tar -C /usr/local -xzf "go${GO_VERSION}.linux-amd64.tar.gz"
rm "go${GO_VERSION}.linux-amd64.tar.gz"
```

Then put Go on your `PATH`. Add this line to `~/.profile` (or `~/.bashrc` / `~/.zshrc`) and
open a new terminal:

```bash
export PATH="$PATH:/usr/local/go/bin:$HOME/go/bin"
```

`/usr/local/go/bin` holds `go` and `gofmt`; `$HOME/go/bin` is where `go install` puts tools.

### macOS

Either run the `.pkg` installer from <https://go.dev/dl/>, which installs to `/usr/local/go`
and adds it to your `PATH`, or use Homebrew: `brew install go`.

### Windows

Develop inside WSL and follow the Linux steps. Native Windows works with the `.msi` installer
from <https://go.dev/dl/>, but the Makefile expects a Unix shell.

### Check the install

```bash
go version   # go version go1.27.4 linux/amd64 (or newer)
go env GOPATH GOROOT
```

**Upgrading** is the same as installing: remove `/usr/local/go` and unpack the new release.
If your Go is at least 1.21 but older than the module needs, the `go` command downloads the
required toolchain automatically the first time you build (this is controlled by
`GOTOOLCHAIN`, and `auto` is the default).

**Go installed but not on your `PATH`?** Either add the `export` line above, or pass the
binaries to make: `make GO=/usr/local/go/bin/go GOFMT=/usr/local/go/bin/gofmt <target>`.

## Run and verify

All commands run in `app-go/`. `make help` lists every target.

```bash
make check     # gofmt check + go vet + go test
make run       # serve on http://localhost:3000 (set PORT to change)
make db-up     # PostgreSQL 17 on localhost:5432, user/password/database "tmock"
make otel-up   # optional: OpenTelemetry Collector on localhost:4318 (make otel-logs to watch)
make db-down   # stop the containers; make db-reset also deletes the database volume
```

What to check after I01 (the I00 checks still apply):

| Check | Expected |
|---|---|
| `make check` | passes |
| `curl localhost:3000/api/status` and `curl localhost:3000/status` | `{"status":"ok"}` |
| Open <http://localhost:3000/> | the "Tmock — coming soon" page; the grey line reads "htmx 4.0.2 and Alpine 3.17.5 are running."; "Check API status" shows `{"status":"ok"}` |
| Browser console | `tmock: htmx 4.0.2, Alpine 3.17.5`, no errors and no CSP violations |
| `TMOCK_ENV=production make run` | exits at once: `SESSION_SECRET: is required when TMOCK_ENV=production` |
| `OTEL_EXPORTER_OTLP_PROTOCOL=grpc make run` | exits at once: `only http/json is supported` |
| `make run`, then `curl localhost:3000/status` | no log line; with `LOG_LEVEL=debug make run`, a DEBUG `tmock.http.request` line |
| Ctrl-C while running | `shutting down…` then `tmock stopped` (`tmock.shutdown.forced=false`); a second Ctrl-C exits at once |
| `make db-up`, then `PGPASSWORD=tmock psql -h localhost -U tmock -d tmock -c 'select version()'` | PostgreSQL 17 |
| OTLP export (below) | the collector prints the `tmock.http.request` record with `Trace ID: 4bf92f3577b34da6a3ce929d0e0e4736` |

Exporting logs to the local collector:

```bash
make otel-up
OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 LOG_LEVEL=debug make run
# in another terminal:
curl -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' localhost:3000/api/status
make otel-logs
```

Stop the collector (`docker compose --profile otel stop otel-collector`) and requests stay
fast; after about 30 s of retries the console reports `OTLP log export failed`.

Ports already taken? `TMOCK_DB_PORT=5433 make db-up`, `TMOCK_OTEL_PORT=14318 make otel-up`.

## Configuration

All settings come from environment variables; a `.env` file in the working directory fills
in anything not set in the environment. Start from the example:

```bash
cp .env.example .env
```

The full list with defaults is in spec §4.2. Invalid values stop start-up with a list of
every problem found. Booleans accept only `true`, `false`, `1` and `0`; empty values count
as unset.

## Logging

Logs go to stdout as text (`LOG_FORMAT=json` for JSON) at `LOG_LEVEL` (default `info`).
Notable things are logged as **events** with a stable `event.name` attribute, such as
`tmock.http.request` or `tmock.server.started` (spec §2.4.2); build dashboards on those, not on
the message text. Health checks are logged at `debug`. An incoming W3C `traceparent` is
attached to every record of that request (`trace_id`/`span_id` on the console).

Setting `OTEL_EXPORTER_OTLP_ENDPOINT` also exports every record as an OpenTelemetry log
record over OTLP/HTTP (JSON), using the standard `OTEL_*` variables. Logging never blocks a
request: if the collector is slow or down, records queue up and are then dropped, and the
exporter reports that on the console. On shutdown, open requests get 20 s to finish and the
remaining logs 5 s to be sent.

## Layout

See spec §2.3 for the full layout. What exists so far:

```
cmd/tmock/            the binary: config, logging set-up, lifecycle; the only package that
                      wires the UI into the server
dev/vendorjs/         `make vendor-js`: re-downloads and verifies vendored JavaScript
dev/otelcol.yaml      collector config for `make otel-up`
internal/api/         REST handlers (health check)
internal/clientip/    the client-IP rule (spec §7.3)
internal/config/      configuration and .env loading (spec §4)
internal/httpx/       error responses and panic recovery
internal/logx/        log handlers, event names, trace context, request log (spec §2.4)
internal/logx/otlp/   OTLP/HTTP JSON log exporter
internal/server/      assembles the http.Handler (backend; receives the UI from cmd/tmock)
internal/web/         UI routes and the static file server
internal/views/       gomponents views: layout shell and pages
internal/archtest/    import-boundary test (spec §1.3)
static/               CSS, JS, images and vendored libraries, embedded with go:embed
```

### Import boundary

Third-party code may only be imported where spec §1.3 allows it: gomponents in
`internal/web` and `internal/views/...`, and (from I02) pgx in `internal/db`. Backend
packages must not depend on the UI at all, even indirectly; only `cmd/tmock` joins the two.
`internal/archtest` enforces this as part of `make test`, so a stray import fails the build.

## Vendored JavaScript

htmx 4, its `hx-alpine-compat` extension and the Alpine CSP build are committed under
`static/vendor/` and embedded in the binary. `static/vendor/VERSIONS` pins each file's version,
source URL and SHA-256; the `static` package test checks the committed files against it.

- **Verify / restore:** `make vendor-js` downloads every file and fails without writing
  anything unless all checksums match.
- **Upgrade:**
  1. In `VERSIONS`, change the file name, version and URL, and set the checksum to `-`.
  2. Run `make vendor-js`. It downloads the file and writes its checksum into `VERSIONS`.
     Compare that checksum with the publisher's (for example jsdelivr's file hashes).
  3. Update the path constants in `static/static.go`, delete the old file, run `make check`
     and click through the UI.

htmx is configured with `<meta name="htmx-config">` in `internal/views/layout`, not in
`app.js`: htmx 4 reads its configuration while its own script loads.
