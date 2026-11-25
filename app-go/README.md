# Tmock (Go)

The Go rewrite of Tmock: REST API, proxy/mock engine and server-rendered dashboard in one
module and one binary. The legacy Node app lives in `../api` and `../app` until cutover.

- **Why and what:** [../intent.md](../intent.md) and [../spec.md](../spec.md).
- **Status:** iteration **I03** (HTTP foundation). The binary connects to PostgreSQL,
  creates its schema (`tmock migrate`) and its first admin (`tmock seed`), and serves the
  health check and a "coming soon" page behind the shared HTTP plumbing: JSON errors,
  security headers and cross-origin protection. Progress is tracked in spec §16.

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
cp .env.example .env   # once; it points at the database below and sets ADMIN_*
make db-up     # PostgreSQL 17 on localhost:5432, user/password/database "tmock"
make check     # gofmt check + go vet + go test (DB tests are skipped)
make test-db   # all tests, including the DB integration tests, against make db-up
make run       # migrate, seed and serve on http://localhost:3000 (set PORT to change)
make otel-up   # optional: OpenTelemetry Collector on localhost:4318 (make otel-logs to watch)
make db-down   # stop the containers; make db-reset also deletes the database volume
```

The commands of the binary. `make build` writes it to `bin/tmock`; it is not installed on
your `PATH`, so run it as `bin/tmock <command>` from `app-go/` (or without building:
`go run ./cmd/tmock <command>`). `make migrate`, `make seed` and `make run` build it first.

| Command | What it does |
|---|---|
| `bin/tmock serve` (default) | Runs the server. Refuses to start while migrations are pending, unless `MIGRATE_ON_START=true`, which applies them first |
| `bin/tmock migrate` | Applies pending migrations from `internal/db/migrations/` and exits. Safe to run concurrently |
| `bin/tmock seed` | Creates the first admin from `ADMIN_NAME`, `ADMIN_EMAIL` and `ADMIN_PASSWORD` if there are no users; otherwise does nothing |

What to check after I03 (the earlier checks still apply; start the server with `make run`):

| Check | Expected |
|---|---|
| `curl -i localhost:3000/api/status` | `200`, with `Content-Security-Policy`, `X-Frame-Options: SAMEORIGIN`, `Referrer-Policy: no-referrer` and the other headers of spec §9.4 |
| `TMOCK_ENV=production` (with a `SESSION_SECRET`), same request | also `Strict-Transport-Security` and `upgrade-insecure-requests` in the CSP |
| `curl localhost:3000/api/nope` | `404 {"status":404,"message":"Not found"}` |
| `curl -i -X DELETE localhost:3000/api/status` | `405 {"status":405,"message":"Method not allowed"}`, `Allow: GET, HEAD` |
| `curl -X POST -H 'Sec-Fetch-Site: cross-site' -H 'Origin: https://evil.example' localhost:3000/api/status` | `403 {"status":403,"message":"Cross-origin request rejected"}` |
| `go test ./internal/httpx ./internal/app ./internal/server` | error shapes, validation details, `Optional[T]` (absent / null / value), headers, cross-origin and proxy CORS |

What to check after I02:

| Check | Expected |
|---|---|
| `make test-db` | passes |
| `make db-reset && make db-up && make migrate` | `applied migration 0001_init`; `make db-psql` then `\dt` lists the six tables and `schema_migrations` |
| `make migrate` again | `the database schema is up to date` |
| `bin/tmock migrate & bin/tmock migrate; wait` | one of them applies `0001_init`, the other reports the schema is up to date |
| `DATABASE_ENABLE_SSL=true` with `DATABASE_HOST`/`_NAME`/`_USER`/`_PASSWORD` instead of `DATABASE_URL`, then `bin/tmock migrate` | exits: `the server does not accept TLS connections, but TLS is required` |
| `make db-down`, then `bin/tmock migrate` | exits: `cannot connect to the database at postgres://tmock:xxxxx@localhost:5432/tmock…` (password masked) |
| `make seed` | `created the first admin user (id 1)` (`tmock.user.seeded`) |
| `make seed` again | `users already exist; nothing to seed` |
| `make db-reset && make db-up && bin/tmock serve` | exits: `the database schema is not up to date (pending migrations: 0001)` |
| `go test ./internal/password` | round-trip, wrong password, tampered hash, re-hash on lower iteration count |

What to check after I01:

| Check | Expected |
|---|---|
| `make check` | passes |
| `curl localhost:3000/api/status` and `curl localhost:3000/status` | `{"status":"ok"}` |
| Open <http://localhost:3000/> | the "Tmock — coming soon" page; the grey line reads "htmx 4.0.2 and Alpine 3.17.5 are running."; "Check API status" shows `{"status":"ok"}` |
| Browser console | `tmock: htmx 4.0.2, Alpine 3.17.5`, no errors and no CSP violations |
| `TMOCK_ENV=production bin/tmock serve` | exits at once: `SESSION_SECRET: is required when TMOCK_ENV=production` |
| `OTEL_EXPORTER_OTLP_PROTOCOL=grpc bin/tmock serve` | exits at once: `only http/json is supported` |
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

Ports already taken? `TMOCK_DB_PORT=5433 make db-up`, `TMOCK_OTEL_PORT=14318 make otel-up`
(and `TMOCK_DB_PORT=5433 make test-db`; point `DATABASE_URL` in `.env` at the same port).

## Database

PostgreSQL 17, through pgx's `database/sql` driver (spec §3.3). `internal/db` opens
the pool and checks the connection at start-up, maps unique and foreign-key violations to
`db.ErrUnique` and `db.ErrForeignKey` (re-exported by `internal/store`), and runs the
migrations.

- **Migrations** are `internal/db/migrations/NNNN_name.sql`, embedded in the binary, applied in
  order, each in a transaction, under an advisory lock, and recorded in `schema_migrations`.
  They are forward-only: never edit a released file; add a new one.
- **DB tests** run only when `TMOCK_TEST_DATABASE_URL` is set (`make test-db` sets it).
  `internal/db/dbtest` gives each test its own schema (`t_<random>` via `search_path`),
  dropped afterwards, so tests run in parallel against one database.
- **Passwords** are hashed with PBKDF2-HMAC-SHA-256, 600,000 iterations (`internal/password`,
  spec §9.1).

## Configuration

All settings come from environment variables; a `.env` file in the working directory fills
in anything not set in the environment. Start from the example:

```bash
cp .env.example .env
```

The full list with defaults is in spec §4.2. Invalid values stop start-up with a list of
every problem found. Booleans accept only `true`, `false`, `1` and `0`; empty values count
as unset.

## Writing handlers

REST handlers are thin: decode, call a use case in `internal/app`, write the result (spec §2.1).

```go
func (h *Handlers) patchService(w http.ResponseWriter, r *http.Request) {
	var in app.ServicePatch // PATCH fields are httpx.Optional[T]: absent, null or a value
	if err := httpx.DecodeJSON(w, r, &in); err != nil { // 400 / 413, §6.2 shape
		httpx.WriteErr(w, r, h.logger, err)
		return
	}
	svc, err := h.app.PatchService(r.Context(), r.PathValue("path"), in)
	if err != nil {
		httpx.WriteErr(w, r, h.logger, err) // *app.Error → its status; anything else → logged 500
		return
	}
	httpx.WriteJSON(w, http.StatusOK, svc)
}
```

- Use cases return `*app.Error` for expected failures (`app.Invalid`, `NotFound`, `Conflict`, …)
  and translate store errors with `app.StoreError`. Validation collects every problem with
  `httpx.Validator` and field paths built by `httpx.Field("condition", "or", 0, "cidr")`.
- Numeric ids in paths are parsed with `httpx.PathID` (`400` for anything else).
- `internal/server` adds the security headers and cross-origin protection to everything except
  `/api/proxy/*`, and answers unknown API routes with JSON `404`/`405`.

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
internal/app/         use cases shared by the REST API and the UI; app.Error (spec §2.1, §6.2)
internal/clientip/    the client-IP rule (spec §7.3)
internal/config/      configuration and .env loading (spec §4)
internal/db/          connection pool, typed errors, migration runner and migrations (spec §3.3, §5)
internal/db/dbtest/   per-test schemas for DB integration tests
internal/password/    PBKDF2 password hashing (spec §9.1)
internal/store/       repositories over database/sql (users so far)
internal/httpx/       HTTP plumbing: error format, JSON decode/encode, Optional[T], Validator,
                      panic recovery, security headers, cross-origin protection, proxy CORS
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
`internal/web` and `internal/views/...`, and pgx in `internal/db`. Backend
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
