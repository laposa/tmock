# Tmock (Go)

The Go rewrite of Tmock: REST API, proxy/mock engine and server-rendered dashboard in one
module and one binary. The legacy Node app lives in `../api` and `../app` until cutover.

- **Why and what:** [../intent.md](../intent.md) and [../spec.md](../spec.md).
- **Status:** iteration **I06** (scenarios). You can log in to the dashboard and manage
  services and scenarios; Clients, Accounts and My Account are placeholder pages until later
  iterations build them. The REST API has login, logout, `me` and CRUD on `/api/service` and
  `/api/scenario`.
  Progress is tracked in spec §16.

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

What to check after I06 (the earlier checks still apply; start the server with `make run`, log
in with `ADMIN_EMAIL` / `ADMIN_PASSWORD` from `.env`, and add a service first):

| Check | Expected |
|---|---|
| Open <http://localhost:3000/scenarios> | "Add Scenario" and "No scenarios yet." |
| Add Scenario | the editor: Name, Service, then **Request Conditions** (Method, Path, Condition) and **Response Modifications** (Code, Headers, Body), each with a switch that is off |
| Switch each field on and fill it in: Content-Type `application/json`, a body, Skip Proxy | each field appears when its switch goes on; Skip Proxy only while Body is on; the body's hint reads JSON and "Format JSON" appears |
| "Format JSON" on `{"a":1,` and on valid JSON | an inline "Not valid JSON: …"; valid JSON is indented. Tab in the body inserts two spaces |
| Add Header, ✕ on a row; switch Headers off | rows are added and removed; switching off clears them |
| Close (or Escape) after a change | "You will lose unsaved changes. Are you sure you want to close?"; Cancel keeps the editor. A click on the backdrop never closes it |
| Create | the editor closes; the scenario is listed in a group named after its service, with Method, Path, Condition and Code; toast `Scenario "…" created` |
| Hover "Show Headers" / "Show Body" | a popover with the headers as JSON / the first 255 characters of the body |
| Click "Show Body" | the editor opens scrolled to the body |
| Edit: switch fields off, Save | the list shows those columns empty; reopening shows the switches off |
| Code `42`, or the same name twice in a service | the message next to the field, an error toast; the editor stays open and Close still asks |
| Edit, Delete | "Are you sure you want to delete the scenario …?"; Confirm removes it from the list |
| Browser console | no errors and no CSP violations (a failed save shows as a `422` network line, which is expected) |
| curl (below) | `201`, `200`, `204`; a duplicate name `409`; `skipProxy` without a body `400`; without the cookie `401` |
| Console log | one `tmock.entity.changed` line per change with `tmock.entity.type=scenario`, `tmock.service` and `tmock.scenario` |
| `make test-db` | the scenarios store, use cases, endpoints and UI handlers |

```bash
curl -b /tmp/cj -H 'Content-Type: application/json' \
  -d '{"service":"svc","name":"down","responseCode":503,"responseHeaders":{"Content-Type":"application/json"},"responseBody":"{}","skipProxy":true}' \
  localhost:3000/api/scenario
curl -b /tmp/cj 'localhost:3000/api/scenario?service=svc'
curl -b /tmp/cj -X PATCH -H 'Content-Type: application/json' -d '{"responseBody":null,"skipProxy":false}' localhost:3000/api/scenario/1
curl -b /tmp/cj -X DELETE localhost:3000/api/scenario/1
```

In the editor, a header row with an empty value is saved as `null`: the proxy removes that
header from the upstream response (spec §7.8).

What to check after I05:

| Check | Expected |
|---|---|
| Open <http://localhost:3000/services> | the Services page: "Add Service" and a dark table Name \| Upstream URL \| Path ("No services yet." when empty) |
| Add Service, Create with empty fields | the modal stays open; "is required" under each field; an error toast |
| Path `a/b`, upstream URL `ftp://x` | "may contain only letters, digits, '.', '_' and '-'" under Path; "must be an absolute http or https URL" under Upstream URL |
| Valid values, Create | the modal closes, the table shows the service, toast `Service "…" created` |
| The same path again | "A service with path "…" already exists" above the fields |
| Click a service's name, change Name, URL and Path, Save | the modal closes; the table shows the new values; toast `Service "…" updated` |
| Edit, Delete | the confirmation names the service and says its scenarios go too; Cancel keeps it, Confirm removes it from the table with a toast |
| Close, or a click on the backdrop | the modal closes without saving |
| Logout in the menu | the confirmation dialog asks "Are you sure you want to log out?"; Cancel (or Escape) stays logged in, Confirm goes to `/login` |
| Browser console | no errors and no CSP violations (a failed save shows as a `422` network line, which is expected) |
| curl CRUD (below) | `201`, `200`, `200`, `204`; a duplicate path `409`; `a/b` or a bad URL `400` with `details`; without the cookie `401` |
| Console log | one `tmock.entity.changed` line per create, update and delete, with `tmock.entity.type=service`, `tmock.service=<path>` and `user.id` |
| `make test-db` | the services store, use cases, endpoints and UI handlers |

```bash
curl -c /tmp/cj -H 'Content-Type: application/json' -d '{"email":"…","password":"…"}' localhost:3000/api/auth/login
curl -b /tmp/cj -H 'Content-Type: application/json' -d '{"path":"svc","name":"Svc","upstreamUrl":"http://localhost:9999"}' localhost:3000/api/service
curl -b /tmp/cj localhost:3000/api/service
curl -b /tmp/cj -X PATCH -H 'Content-Type: application/json' -d '{"path":"svc2"}' localhost:3000/api/service/svc
curl -b /tmp/cj -X DELETE localhost:3000/api/service/svc2
```

What to check after I04:

| Check | Expected |
|---|---|
| Open <http://localhost:3000/> | redirected to `/login`; the Login button is disabled until both fields are filled |
| Log in with a wrong password | "Invalid email or password" above the fields; the email is kept |
| Log in with the seeded admin | the Clients page with the menu Services, Clients, Scenarios, Accounts, My Account, Logout; each entry except Services opens a placeholder naming its iteration |
| Reload | still logged in |
| Logout | back on `/login`; `/services` redirects to `/login` again |
| A non-admin user (see below) | no Accounts entry; `/accounts` redirects to `/` |
| Browser console | no errors and no CSP violations (a failed login shows as a `422` network line, which is expected) |
| System dark mode | dark page as in the legacy dashboard |
| `curl -c /tmp/cj -X POST localhost:3000/api/auth/login -d '{"email":"…","password":"…"}'` | `200` with the user, and a `tmock_session` cookie (`HttpOnly`, `SameSite=Lax`) |
| `curl -b /tmp/cj localhost:3000/api/auth/me` | `200` with the user; without `-b`: `401 {"status":401,"message":"Not logged in"}` |
| `curl -b /tmp/cj -X POST localhost:3000/api/auth/logout`, then `me` again | `204`, then `401` |
| `make test-db` | login, expiry (`synctest`), logout, sweep, a deleted user's session, guards and the UI pages |

A non-admin user for the check above (there is no Accounts page until I08):

```bash
make db-psql
INSERT INTO users (name, email, password_hash)
  SELECT 'Bob', 'bob@example.com', password_hash FROM users WHERE id = 1;  -- same password as the admin
```

Without `SESSION_SECRET`, sessions end when the server restarts (spec §9.2); set one in `.env` to
keep them.

What to check after I03:

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
| Open <http://localhost:3000/> and log in (from I04) | a placeholder page; the grey line reads "htmx 4.0.2 and Alpine 3.17.5 are running."; "Check API status" shows `{"status":"ok"}` |
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
  `popover`, `headerRows`, `codeEditor`, `scenarioEditor`); HTML refers to them by name only.
  Another component can ask a question in the confirmation dialog with a `confirm:ask` event
  on `document` (detail `{question, accept}`). A toast is raised from Go with `toast(w, type, title, msg)`,
  the modal closed with `closeModal(w)`; both add to the same `HX-Trigger` header.
- **Modals** (the Services page is the example): a button with `hx-get` and
  `hx-target="#modal-body"` loads a `components.ModalContent` holding a `Form` whose `Footer`
  is a `ModalFooter`. The form posts with `hx-target="#modal-body"`, so a `422` replaces the
  modal content with the re-rendered form. A successful save answers with the refreshed list
  and `HX-Retarget` / `HX-Reswap: outerHTML` pointing at it, plus a toast and `closeModal`.
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
dev/vendorjs/         `make vendor-js`: re-downloads and verifies vendored JavaScript
dev/otelcol.yaml      collector config for `make otel-up`
internal/api/         REST handlers (health check, login, logout, me, services, scenarios)
internal/app/         use cases shared by the REST API and the UI; app.Error (spec §2.1, §6.2)
internal/auth/        sessions: cookie, lookup, logout, hourly sweep; the user and admin guards
internal/auth/authtest/  in-memory session store for handler tests
internal/clientip/    the client-IP rule (spec §7.3)
internal/config/      configuration and .env loading (spec §4)
internal/db/          connection pool, typed errors, migration runner and migrations (spec §3.3, §5)
internal/db/dbtest/   per-test schemas for DB integration tests
internal/password/    PBKDF2 password hashing (spec §9.1)
internal/store/       repositories over database/sql (users, sessions, services, scenarios)
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
