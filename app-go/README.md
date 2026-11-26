# Tmock (Go)

The Go rewrite of Tmock: REST API, proxy/mock engine and server-rendered dashboard in one
module and one binary. The legacy Node app lives in `../api` and `../app` until cutover.

- **Why and what:** [../intent.md](../intent.md) and [../spec.md](../spec.md).
- **Status:** iteration **I09** (proxy core). You can log in and manage services, scenarios,
  clients (with the condition builder, scenario assignments and client tokens), and, as an
  admin, user accounts; every user can edit their own account. The REST API has login,
  logout, `me` and CRUD on `/api/service`, `/api/scenario`, `/api/client` (with client tokens)
  and `/api/user` (with `/api/user/profile`). `/api/proxy/{service}/…` forwards every request
  to the service's upstream; client matching and mocking follow in I10–I13. Progress is
  tracked in spec §16.

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

Trying the proxy with the fake upstream `cmd/echo-upstream`, which answers every request with
a JSON description of what it received:

```bash
go run ./cmd/echo-upstream          # localhost:8081; -status 503, -delay 2s, -gzip, -addr :9000
make run                            # in another terminal
# in the dashboard (Services → Add Service): path "echo", upstream URL http://localhost:8081/base
curl -i 'localhost:3000/api/proxy/echo/a/b?x=1'   # echo shows "url": "/base/a/b?x=1"
curl -i localhost:3000/api/proxy/nope              # 404 Service not found
```

Each proxy request is logged once, as `tmock.proxy.request` (`<ip> <METHOD> <url> <status>
<ms>ms <size>`); a failing upstream answers `502 Bad Gateway` and logs
`tmock.proxy.upstream_failed`. The proxy reads a cached catalog of services, scenarios and
clients (`CACHE_TTL`, default `10m`; `DISABLE_CACHE=true` reads the database on every
request). Every change made in the dashboard or the REST API invalidates it, so the next proxy
request sees the change; at `LOG_LEVEL=debug` each rebuild logs `tmock.catalog.rebuilt`. With
several replicas, each has its own cache: changes reach the others within `CACHE_TTL`.

Without `SESSION_SECRET`, sessions end when the server restarts (spec §9.2); set one in `.env` to
keep them.

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
  dropped afterwards, so tests run in parallel against one database. At most 4 DB tests per
  package hold a schema at once, which keeps a full run under PostgreSQL's 100 connections.
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
func (a *API) updateService(w http.ResponseWriter, r *http.Request) {
	var p app.ServicePatch // PATCH fields are httpx.Optional[T]: absent, null or a value
	if err := httpx.DecodeJSON(w, r, &p); err != nil { // 400 / 413, §6.2 shape
		httpx.WriteErr(w, r, a.logger, err)
		return
	}
	s, err := a.app.UpdateService(r.Context(), actor(r), r.PathValue("path"), p)
	if err != nil {
		httpx.WriteErr(w, r, a.logger, err) // *app.Error → its status; anything else → logged 500
		return
	}
	httpx.WriteJSON(w, http.StatusOK, s)
}
```

- Use cases return `*app.Error` for expected failures (`app.Invalid`, `NotFound`, `Conflict`, …)
  and translate store errors with `app.StoreError`. Validation collects every problem with
  `httpx.Validator` and field paths built by `httpx.Field("condition", "or", 0, "cidr")`.
- Numeric ids in paths are parsed with `httpx.PathID` (`400` for anything else).
- `internal/server` adds the security headers and cross-origin protection to everything except
  `/api/proxy/*`, and answers unknown API routes with JSON `404`/`405`.
- Routes that need a login are wrapped in `sessions.RequireUser` or `sessions.RequireAdmin`
  (`internal/auth`); the handler reads the user with `auth.UserFrom(r.Context())`.
- Use cases that change something take an `app.Actor` (the logged-in user) and log
  `tmock.entity.changed` themselves (spec §2.4.2).

## Writing UI pages

Every UI element is a component in `internal/views/components`, one file each, with its own
section in `static/app.css` (spec §10.6, §10.7). Pages and layouts only compose components:

```go
components.Form(components.FormProps{
	Action: "/services",
	Attrs:  []g.Node{hx.Post("/services"), hx.Swap("outerHTML")}, // behaviour, never classes
	Fields: []g.Node{components.TextField(components.TextFieldProps{Label: "Name", Name: "name", Error: errs["name"]})},
	Actions: []g.Node{components.Button(components.ButtonProps{Label: "Save", Variant: components.ButtonPrimary, Submit: true})},
})
```

- Only `internal/views/components` sets `class` attributes; a test fails otherwise. A new
  element means a new component, its render test and its `app.css` section.
- Components that hold others (`Form`, `AppHeader`, `PageHeader`, `ConfirmDialog`) decide the
  order of their parts, so a layout change is one edit in one place.
- `internal/views/hx` builds the `hx-*` attributes; nothing that needs eval (`hx-on`, `js:`,
  trigger filters) is allowed.
- Handlers in `internal/web` answer with `redirect` (303, or `HX-Redirect` for htmx), re-render
  a form with `422` on validation errors, and use `fail` for other errors: the status, an error
  toast and `HX-Reswap: none` (spec §10.2).
- `static/app.js` holds the Alpine components (`toasts`, `modal`, `confirm`, `requiredFields`,
  `popover`, `headerRows`, `codeEditor`, `scenarioEditor`, `copyField`, `passwordConfirm`);
  HTML refers to them by name only.
- A list that other actions change can reload itself: the clients table has
  `hx-trigger="clients:changed from:body"`, and a handler raises the event with
  `trigger(w, pages.ClientsChangedEvent, …)` (the assignment and token modals do).
  Another component can ask a question in the confirmation dialog with a `confirm:ask` event
  on `document` (detail `{question, accept}`). A toast is raised from Go with `toast(w, type, title, msg)`,
  the modal closed with `closeModal(w)`; both add to the same `HX-Trigger` header.
- **Modals** (the Services page is the example): a button with `hx-get` and
  `hx-target="#modal-body"` loads a `components.ModalContent` holding a `Form` whose `Footer`
  is a `ModalFooter`. The form posts with `hx-target="#modal-body"`, so a `422` replaces the
  modal content with the re-rendered form. A successful save answers with the refreshed list
  and `HX-Retarget` / `HX-Reswap: outerHTML` pointing at it, plus a toast and `closeModal`.
- **Server-side editing** (the condition builder is the example): every button is a submit
  button with `name`, `value` and a `formaction`, plus `hx-post` to the same URL, so htmx posts
  the whole form with the clicked button's value, and a browser without JavaScript does the
  same. The handler parses the form, applies the edit and renders the form again; only Save
  stores anything. A hidden first submit button makes Enter in a text field mean Save.
- **Optional fields** (the scenario editor is the example): `components.ToggleField` pairs a
  switch with its control and shows one or the other with CSS (`:has`), so it works without
  JavaScript; the handler ignores a control whose switch is off.
- An error about one field (`app.Error.Field`, such as a taken name) is shown next to that
  field; other conflicts above the form.

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
cmd/echo-upstream/    dev tool: a fake upstream that echoes each request as JSON
dev/vendorjs/         `make vendor-js`: re-downloads and verifies vendored JavaScript
dev/otelcol.yaml      collector config for `make otel-up`
internal/api/         REST handlers (health check, login, logout, me, services, scenarios,
                      clients and the token-or-user guard)
internal/app/         use cases shared by the REST API and the UI; app.Error (spec §2.1, §6.2)
internal/auth/        sessions: cookie, lookup, logout, hourly sweep; the user and admin guards
internal/auth/authtest/  in-memory session store for handler tests
internal/catalog/     the proxy's cached snapshot: TTL, single rebuild, invalidation (spec §7.2)
internal/clientip/    the client-IP rule (spec §7.3)
internal/config/      configuration and .env loading (spec §4)
internal/db/          connection pool, typed errors, migration runner and migrations (spec §3.3, §5)
internal/db/dbtest/   per-test schemas for DB integration tests
internal/password/    PBKDF2 password hashing (spec §9.1)
internal/proxy/       /api/proxy/: service lookup, upstream forwarding, proxy log (spec §7)
internal/store/       repositories over database/sql (users, sessions, services, scenarios,
                      clients, and the catalog snapshot read)
internal/condition/   client conditions: JSON model, parsing, validation, the empty rule
internal/condition/builder/  the condition builder's tree: form ↔ tree ↔ condition, and its edits
internal/httpx/       HTTP plumbing: error format, JSON decode/encode, Optional[T], Validator,
                      panic recovery, security headers, cross-origin protection, proxy CORS
internal/logx/        log handlers, event names, trace context, request log (spec §2.4)
internal/logx/logxtest/  records log events in tests
internal/logx/otlp/   OTLP/HTTP JSON log exporter
internal/server/      assembles the http.Handler (backend; receives the UI from cmd/tmock)
internal/web/         UI routes, page guards, login/logout, redirects, toasts, error mapping,
                      and the static file server
internal/views/components/  every UI element, one file each; the only package that sets classes
internal/views/layout/      page shells: the logged-in page and the bare login page
internal/views/pages/       pages composed from components
internal/views/hx/          htmx attribute helpers
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
