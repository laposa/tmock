# Tmock (Go)

The Go rewrite of Tmock: REST API, proxy/mock engine and server-rendered dashboard in one
module and one binary. The legacy Node app lives in `../api` and `../app` until cutover.

- **Why and what:** [../intent.md](../intent.md) and [../spec.md](../spec.md).
- **Status:** iteration **I00** (scaffolding). The binary serves the health check and a
  "coming soon" page. Progress is tracked in spec §16.

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

What to check after I00:

| Check | Expected |
|---|---|
| `make check` | passes |
| `curl localhost:3000/api/status` and `curl localhost:3000/status` | `{"status":"ok"}` |
| Open <http://localhost:3000/> | the "Tmock — coming soon" page; the grey line reads "htmx 4.0.2 and Alpine 3.17.5 are running."; "Check API status" shows `{"status":"ok"}` |
| Browser console | `tmock: htmx 4.0.2, Alpine 3.17.5`, no errors and no CSP violations |
| `make db-up`, then `PGPASSWORD=tmock psql -h localhost -U tmock -d tmock -c 'select version()'` | PostgreSQL 17 |
| `make otel-up`, then send a record (below) and `make otel-logs` | the collector prints `Body: Str(hello)` |

```bash
curl -s -X POST localhost:4318/v1/logs -H 'Content-Type: application/json' -d \
  '{"resourceLogs":[{"scopeLogs":[{"logRecords":[{"severityText":"INFO","body":{"stringValue":"hello"}}]}]}]}'
```

Ports already taken? `TMOCK_DB_PORT=5433 make db-up`, `TMOCK_OTEL_PORT=14318 make otel-up`.

## Layout

See spec §2.3 for the full layout. What exists so far:

```
cmd/tmock/            the binary; the only package that wires the UI into the server
dev/vendorjs/         `make vendor-js`: re-downloads and verifies vendored JavaScript
dev/otelcol.yaml      collector config for `make otel-up`
internal/api/         REST handlers (health check)
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
