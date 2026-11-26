# T-Mock Go Rewrite — Specification

Status: **Draft for review** · Companion document: [intent.md](intent.md) (why, goals, principles)

This specification describes:

- **what the Go application does**: features, data, REST API, proxy and UI;
- **how it is structured**: the module, its packages, dependencies and runtime;
- **the iteration plan** (§15). Each iteration is written so it can be pasted into a GitHub issue,
  and each one leaves the repository in a buildable, locally testable state.

The Go application is a **standalone product**. The legacy Node application (`api/`, `app/`) is the
reference for *which features exist*, not for exact behaviour. The Go app does not share a database
with it, does not have to serve the Vue app, and does not copy its quirks. Where keeping a legacy
convention costs nothing (URL paths, JSON field names, the template marker), it is kept so that
users and test automation have less to relearn. Where it would add work or complexity, the Go app
does the simpler, cleaner thing. §12 lists the differences users will notice.

Conventions:

- **MUST**, **SHOULD** and **MAY** have their RFC 2119 meaning.
- "**Kept:**" marks a legacy convention that is deliberately kept because it is free.
- File references such as `api/src/proxy/proxy.service.ts` point to the legacy code, for background.

---

## Table of contents

1. [Scope and repository layout](#1-scope-and-repository-layout)
2. [Architecture](#2-architecture)
3. [Technology and dependency policy](#3-technology-and-dependency-policy)
4. [Configuration](#4-configuration)
5. [Data model and migrations](#5-data-model-and-migrations)
6. [REST API](#6-rest-api)
7. [Proxy and mock engine](#7-proxy-and-mock-engine)
8. [Response templating (Liquid subset)](#8-response-templating-liquid-subset)
9. [Authentication, sessions and HTTP security](#9-authentication-sessions-and-http-security)
10. [Web UI](#10-web-ui)
11. [Testing strategy](#11-testing-strategy)
12. [Differences from the legacy app](#12-differences-from-the-legacy-app)
13. [Legacy import, packaging and cutover](#13-legacy-import-packaging-and-cutover)
14. [Later (explicitly out of scope)](#14-later-explicitly-out-of-scope)
15. [Iteration plan](#15-iteration-plan)
16. [Iteration log](#16-iteration-log)

---

## 1. Scope and repository layout

### 1.1 What stays untouched

The rewrite MUST NOT modify `api/`, `app/`, the root `Dockerfile`, `README.md` (apart from an
optional pointer to these documents) or `.github/workflows/build.yaml`. The legacy application stays
buildable and deployable until it is deleted, which is a separate step after cutover (§13.3).

### 1.2 What is added

```
tmock/
├── api/                  # legacy NestJS backend (unchanged, deleted after cutover)
├── app/                  # legacy Vue frontend  (unchanged, deleted after cutover)
├── app-go/               # NEW — the whole Go application in one module and one binary:
│                         #       REST API, proxy/mock engine and server-rendered UI
│                         #       (Gomponents + HTMX + Alpine)
├── intent.md             # NEW — why
├── spec.md               # NEW — what / how (this file)
├── Dockerfile            # legacy (unchanged)
└── README.md             # existing
```

An optional new workflow file `.github/workflows/build-go.yaml` MAY be added in I17, subject to
team approval (§13.2).

### 1.3 Go module

| Folder | Module path | Allowed `require`s |
|---|---|---|
| `app-go/` | `github.com/laposa/tmock/app-go` | `maragu.dev/gomponents` and `github.com/jackc/pgx/v5` (plus their indirect requirements) |

There is exactly one `go.mod`, in `app-go/`. No `go.work` file is needed.

**Import boundary.** Third-party code is confined to the packages that need it:

| Third-party module | May be imported only by |
|---|---|
| `maragu.dev/gomponents` | the UI packages (`internal/web`, `internal/views/...`) |
| `github.com/jackc/pgx/v5/stdlib` | `internal/db` (as a blank import that registers the `database/sql` driver) |

Every other package uses the standard library only, and all SQL is written against `database/sql`.
A test in `internal/archtest` runs `go list -deps` over the module and enforces two rules:

1. **Direct imports:** a package may import a third-party module only if the table above grants
   it. Indirect reach is allowed (for example `internal/store` → `internal/db` → pgx).
2. **UI isolation:** no package except the UI packages and the composition root `cmd/tmock` may
   depend on a UI package or on gomponents, directly or indirectly. So `internal/server` does not
   import the UI; `cmd/tmock` builds the UI handler and passes it in.

The test also reads every source file, so `go test` re-runs it whenever the code changes (its
result would otherwise be cached, because the files `go list` reads are invisible to the cache).

---

## 2. Architecture

### 2.1 Components

```
        Browser (dashboard)                     Test automation / services under test
               │ HTML (HTMX)                                    │ JSON / proxied HTTP
               ▼                                                ▼
┌─────────────────────────────────────────────────────────────────────────────────────┐
│ app-go — one process, one port                                                      │
│                                                                                     │
│  UI  (internal/web, internal/views)          REST API (internal/api)  Proxy         │
│   • /            Gomponents pages            • /api/*                 (internal/    │
│   • /ui/*        HTMX fragments              • /api/status, /status    proxy)       │
│   • /static/*    css, js (embedded)          • /swagger               • /api/proxy/*│
│          │                                          │                      │        │
│          └──────────────┬───────────────────────────┘                      │        │
│                         ▼                                                  ▼        │
│          internal/app — use cases: validation, business rules,    internal/catalog  │
│                         cache invalidation, permission checks              │        │
│                         │                                                  │        │
│                         └──────────────► internal/store ◄──────────────────┘        │
└──────────────────────────────────────────────────┬──────────────────────────────────┘
                                                   │ database/sql
                                                   ▼
                                       PostgreSQL (own schema, §5)
```

- **`internal/app`** owns every business rule: validation and its messages, the enable rule, admin
  rules, password checks, token handling and cache invalidation. Both the REST handlers and the UI
  handlers call it. Neither of them talks to `internal/store` directly.
- **The REST API** (`internal/api`) is a thin JSON layer over `internal/app`, for test automation
  and scripts.
- **The UI** (`internal/web`, `internal/views`) is a thin HTML layer over the same `internal/app`.
  There is no REST client and no extra network hop. Any rule the UI relies on is, by
  construction, the same rule the REST API enforces.
- **The proxy** (`internal/proxy`) reads the cached catalog and forwards or mocks requests.

### 2.2 Runtime

There is one binary, `app-go/cmd/tmock`, listening on `PORT` (default 3000). Subcommands:

| Subcommand | Purpose |
|---|---|
| `serve` (default) | Run the server |
| `migrate` | Apply database migrations |
| `seed` | Create the first admin user if there are no users |
| `import-legacy` | One-time import of services, scenarios and clients from the legacy database (§13.1) |

The mux mounts the REST API and the proxy at `/api/`, the health check at `/status`, API docs at
`/swagger`, and the UI at `/` (including `/ui/*` fragments and `/static/*`).

**Shutdown.** On the first SIGINT or SIGTERM, `serve` stops accepting connections and waits up
to **20 s** for open requests (`tmock.server.stopped` reports whether any had to be cut off),
then flushes the log exporter for up to **5 s** (§2.4.5), so the whole sequence fits
Kubernetes' default 30-second grace period. A second signal during the drain ends the process
at once.

### 2.3 Package layout

```
app-go/
├── go.mod                    # module github.com/laposa/tmock/app-go; requires gomponents and pgx
├── Makefile                  # run, test, test-db, vet, fmt, db-up, db-down, migrate, seed,
│                             # vendor-js, docker
├── compose.yaml              # local PostgreSQL for development and tests
├── Dockerfile                # production image (build context: repository root)
├── README.md
├── cmd/
│   ├── tmock/                # main: serve | migrate | seed | import-legacy; composition root
│   └── echo-upstream/        # dev tool: fake upstream that echoes requests (for proxy testing)
├── dev/
│   ├── vendorjs/             # `make vendor-js`: re-downloads and verifies static/vendor
│   └── otelcol.yaml          # OpenTelemetry Collector config for `make otel-up`
├── openapi/                  # openapi.json (embedded) + swagger page
├── static/                   # embedded via go:embed (static.go: FS, vendor paths, VERSIONS parser)
│   ├── app.css               # hand-written: design tokens, then one section per component (§10.6)
│   ├── app.js                # Alpine components (Alpine.data) and htmx event listeners
│   ├── logo.svg, favicon.ico
│   └── vendor/
│       ├── htmx-4.x.y.min.js
│       ├── hx-alpine-compat-4.x.y.js     # htmx extension, same htmx release
│       ├── alpinejs-csp-3.x.y.min.js
│       └── VERSIONS          # exact versions, source URLs, SHA-256 checksums
├── e2e/                      # end-to-end smoke test of the binary
└── internal/
    │   # ── backend (standard library only, except pgx in internal/db; enforced by archtest) ──
    ├── config/               # env + .env loading, validation
    ├── logx/                 # slog setup, fan-out handler, event names and attribute keys,
    │   │                     # request-log middleware, W3C trace-context parsing (§2.4)
    │   └── otlp/             # OTLP/HTTP+JSON log exporter: mapping, batching, retry (§2.4.5)
    ├── password/             # PBKDF2 password hashing (§9.1)
    ├── db/                   # *sql.DB setup via pgx stdlib (§3.3), migration runner,
    │                         # migrations/*.sql (embedded), SQLSTATE → typed errors
    ├── store/                # repositories: services, scenarios, clients, users, sessions
    ├── app/                  # use cases shared by the REST API and the UI (§2.1)
    ├── httpx/                # error format, panic recovery; JSON I/O, validation, Optional[T],
    │                         # CORS, security headers (from I03)
    ├── clientip/             # the client-IP rule (§7.3), shared by logging, conditions and auth
    ├── auth/                 # sessions, guards, client-token guard (used by API and UI)
    ├── api/                  # REST handlers
    ├── catalog/              # cached snapshot of services/scenarios/clients for the proxy
    ├── condition/            # client condition model, validation, evaluation
    ├── expr/                 # request-condition expression language
    ├── liquid/               # Liquid template subset
    ├── proxy/                # proxy handler, scenario matching, response modification
    ├── legacyimport/         # reads the legacy mp_* tables (§13.1)
    ├── server/               # wires everything into one http.Handler; lifecycle
    ├── archtest/             # import-boundary test (§1.3)
    │   # ── UI (may import gomponents) ──
    ├── web/                  # UI routes, handlers, page guards, redirects, errors → toasts
    └── views/
        ├── components/       # every UI element, one file each: button, field, switch, table,
        │                     # chip, modal, confirm, toast region, nav, … (§10.7); the only
        │                     # package that sets `class` attributes
        ├── layout/           # page shell: places the shell components and page regions (§10.7)
        ├── pages/            # login, clients, services, scenarios, accounts, myaccount;
        │                     # composed from components only
        └── hx/               # hx-* attribute helpers (here, not in web/, so that pages and
                              # components can use them: web imports the views)
```

### 2.4 Logging, log events and OpenTelemetry

All logging goes through `log/slog`. Every record is written to the console and, when configured,
exported as an **OpenTelemetry log record** over OTLP to a collector (OpenTelemetry Collector,
Grafana Alloy, Datadog Agent, …). The exporter is a small package of our own
(`internal/logx/otlp`), written against the standard library. The OpenTelemetry Go SDK is **not**
used (§3.2, "Considered and rejected").

#### 2.4.1 Pipeline

```
 slog.Logger ──► logx fan-out handler ──┬──► console handler (text | json, stdout)  — always
   (ctx-aware calls)    │ LOG_LEVEL     └──► otlp handler ──► bounded queue ──► batch exporter
                        │                                       (drops when full)     │ POST /v1/logs
                        └── adds trace context from ctx                                ▼
                                                                         OTLP/HTTP collector (JSON)
```

- `internal/logx` builds one `slog.Handler` that fans every record out to the console handler and,
  if export is enabled (§2.4.6), to the OTLP handler. `LOG_LEVEL` applies to both sinks.
- `slog.SetDefault` is called once at start-up, so third-party and stdlib logs (for example
  `http.Server.ErrorLog`, bridged with `slog.NewLogLogger`) take the same path.
- Code that logs inside a request MUST use the context-aware calls (`slog.InfoContext`,
  `logger.LogAttrs(ctx, …)`) so the trace context reaches the record (§2.4.4).
- Logging never blocks a request and never fails one. If the collector is slow or down, records
  are queued and, once the queue is full, dropped (§2.4.5).
- The exporter's own diagnostics (export failures, dropped records) are written to the **console
  handler only**, never back into the OTLP pipeline, so a broken collector cannot cause a loop.

#### 2.4.2 Log events

Notable things the application does are logged as **events**: records with a stable name in the
`event.name` attribute (which the exporter also writes to the OTLP `eventName` field), a fixed
level and a fixed set of attributes. The message stays human-readable, so the console output is
still useful on its own; dashboards and alerts are built on the event name and attributes, never
on the message text.

Event names and attribute keys are constants in `internal/logx` (for example
`logx.EventProxyRequest`, `logx.AttrClientAddress`); packages MUST NOT spell them as string
literals. Attribute keys follow the OpenTelemetry semantic conventions where one exists
(`http.request.method`, `url.path`, `http.response.status_code`, `client.address`,
`server.address`, `user.id`, `error.type`, `exception.*`), and use the `tmock.` prefix otherwise.
Durations are logged in milliseconds as integers (`tmock.duration_ms`).

| Event (`event.name`) | Level | When | Attributes (besides the common ones) | Added in |
|---|---|---|---|---|
| `tmock.server.started` | info | the listener is open | `server.port`, `tmock.env` | I01 |
| `tmock.server.stopped` | info | graceful shutdown finished | `tmock.duration_ms` (drain time), `tmock.shutdown.forced` | I01 |
| `tmock.http.request` | info; debug for `/status` and `/api/status` | each API and UI request (§9.6) | `http.request.method`, `http.route`, `url.path`, `url.query`, `http.response.status_code`, `client.address`, `tmock.duration_ms`, `user.id` (if a session is present) | I01 |
| `tmock.http.panic` | error | a handler panicked; `500` returned | `http.request.method`, `url.path`, `exception.type`, `exception.message`, `exception.stacktrace` | I01 |
| `tmock.db.migration.applied` | info | each applied migration | `tmock.migration.version`, `tmock.migration.name`, `tmock.duration_ms` | I02 |
| `tmock.user.seeded` | info | `tmock seed` created the admin | `user.id` | I02 |
| `tmock.auth.login.succeeded` | info | login | `user.id`, `client.address` | I04 |
| `tmock.auth.login.failed` | warn | wrong email or password | `client.address` (never the email or password) | I04 |
| `tmock.auth.logout` | info | logout | `user.id` | I04 |
| `tmock.session.swept` | debug | hourly sweep (§9.2) | `tmock.sessions.deleted` | I04 |
| `tmock.entity.changed` | info | a service, scenario, client or user was created, updated or deleted, or a client token was generated or revoked | `tmock.entity.type`, `tmock.entity.id`, `tmock.entity.action` (`create` \| `update` \| `delete` \| `token-generate` \| `token-revoke` \| `disable-all`), `tmock.actor` (`user` \| `client-token`), `user.id` or `tmock.client.id`; for a service also `tmock.service` (its path after the change); for a scenario `tmock.service` and `tmock.scenario` (its name); for a client `tmock.client` (its name). `disable-all` has no `tmock.entity.id` and carries `tmock.clients.disabled` (how many) | I05–I08 |
| `tmock.catalog.rebuilt` | debug | a new snapshot is in place (§7.2) | `tmock.duration_ms`, `tmock.catalog.services`, `tmock.catalog.scenarios`, `tmock.catalog.clients` | I09 |
| `tmock.catalog.rebuild_failed` | error | the rebuild failed; the old snapshot stays | `error.type`, `exception.message` | I09 |
| `tmock.proxy.request` | info | each proxy request (§7.9) | `http.request.method`, `url.path`, `url.query`, `http.response.status_code`, `client.address`, `tmock.service`, `server.address` (upstream host), `tmock.duration_ms`, `http.response.body.size` (when known); from I10 also `tmock.client`, `tmock.scenario`, `tmock.mocked` (array of `status`, `headers`, `body`), `tmock.proxy_skipped` | I09, I10 |
| `tmock.proxy.upstream_failed` | error | the upstream call failed; `502` returned | `tmock.service`, `server.address`, `error.type`, `exception.message` | I09 |
| `tmock.template.render_failed` | warn | a template failed at runtime and the fallback was used (§8.1) | `tmock.scenario`, `exception.message` | I12 |
| `tmock.import.issue` | warn | one item reported by the legacy import (§13.1) | `tmock.entity.type`, `tmock.legacy.id`, `tmock.import.reason` | I15 |
| `tmock.import.finished` | info | `import-legacy` finished | `tmock.import.dry_run`, `tmock.import.services`, `tmock.import.scenarios`, `tmock.import.clients`, `tmock.import.issues` | I15 |

`tmock.entity.changed` makes changes traceable in the log backend. It is **not** an audit log: it is
not stored by tmock and carries no before/after values (an audit log stays in §14).

#### 2.4.3 What is never logged

Passwords and password hashes, session cookie values and session ids, client tokens, the
`X-Client-Token`, `Authorization`, `Cookie`, `Set-Cookie` and `Apikey` headers, `SERVICES_API_KEY`,
`SESSION_SECRET`, `OTEL_EXPORTER_OTLP_HEADERS`, database passwords (the DSN is logged with the
password masked), and request or response bodies. Header values in general are not logged. Login
failures do not record the email that was tried.

A test in `internal/logx` runs a login, a token `PATCH` and a proxied request through a capturing
handler and fails if any of these values appears in a record.

#### 2.4.4 Trace context

tmock does not create spans (tracing is in §14), but it keeps log records correlatable with the
traces of the system under test:

- The request-log middleware parses an incoming W3C `traceparent` header (version `00`, non-zero
  trace and parent ids; anything else is ignored) and stores it in the request context.
- Every record logged with that context carries it: the OTLP handler sets `traceId`, `spanId` and
  `flags`, and the console handler adds `trace_id` and `span_id` attributes.
- `traceparent` and `tracestate` are forwarded to the upstream unchanged, like any other
  end-to-end header (§7.7).

#### 2.4.5 OTLP exporter (`internal/logx/otlp`)

- **Protocol:** OTLP/HTTP with the JSON encoding (`Content-Type: application/json`), one
  `POST` per batch of an `ExportLogsServiceRequest`. gRPC and `http/protobuf` are not supported;
  every common collector accepts OTLP/HTTP JSON on port 4318. Only the stable OTLP JSON fields are
  written.
- **Resource:** `service.name` (from `OTEL_SERVICE_NAME`, default `tmock`), `service.version`
  (from `runtime/debug.ReadBuildInfo`), `service.instance.id` (a random UUID per process),
  `deployment.environment.name` (from `TMOCK_ENV`), plus `OTEL_RESOURCE_ATTRIBUTES`. The
  instrumentation scope is `github.com/laposa/tmock/app-go`.
- **Mapping** of a `slog.Record` to a `LogRecord`:

  | slog | OTLP |
  |---|---|
  | `Time` | `timeUnixNano`; `observedTimeUnixNano` = time the handler saw it |
  | `Level` | `severityNumber` = level + 9, clamped to 1–24 (DEBUG → 5, INFO → 9, WARN → 13, ERROR → 17); `severityText` = `slog.Level.String()` |
  | `Message` | `body.stringValue` |
  | `event.name` attribute | `eventName`, and kept as an attribute for collectors that predate the field |
  | string / bool / float64 | `stringValue` / `boolValue` / `doubleValue` |
  | int / int64 / uint64 | `intValue` (as a decimal string, per the OTLP JSON mapping) |
  | `time.Time` | `stringValue`, RFC 3339 with nanoseconds |
  | `time.Duration` | `intValue` in milliseconds (but prefer `tmock.duration_ms`) |
  | `[]string`, `[]int64` | `arrayValue` |
  | `error`, `fmt.Stringer`, anything else | `stringValue` via `fmt.Sprint` |
  | `slog.Group` / `WithGroup` | keys flattened with `.` (`db.pool` + `open` → `db.pool.open`) |
  | trace context (§2.4.4) | `traceId`, `spanId` (lower-case hex), `flags` |

  String values longer than 8 KiB are truncated and get the suffix `…[truncated]`.
- **Batching:** a bounded queue (`OTEL_BLRP_MAX_QUEUE_SIZE`) feeds one export goroutine. A batch
  is sent when it reaches `OTEL_BLRP_MAX_EXPORT_BATCH_SIZE` records or `OTEL_BLRP_SCHEDULE_DELAY`
  after its first record, whichever comes first. Only one export is in flight at a time. When the
  queue is full, new records are dropped and counted; the count is reported on the console at
  most once a minute (`tmock.otlp.dropped`, warn, with `tmock.otlp.dropped.queue_full` and
  `tmock.otlp.dropped.export_failed`), and once more at shutdown.
- **Retry:** network errors and `429`, `502`, `503`, `504` are retried with exponential backoff
  (1 s, 2 s, 4 s, … with jitter, honouring `Retry-After`) until `OTEL_BLRP_EXPORT_TIMEOUT` has
  passed, then the batch is dropped and counted. Other `4xx`/`5xx` responses drop the batch
  immediately. Each dropped batch is reported on the console at once (`tmock.otlp.export_failed`,
  warn). A `partialSuccess` with `rejectedLogRecords > 0` is reported on the console too
  (`tmock.otlp.partial_success`, warn). These three events go to the console only.
  Each HTTP attempt is limited by `OTEL_EXPORTER_OTLP_TIMEOUT`.
- **Transport:** its own `http.Client` (not the proxy's transport), HTTPS with the system roots,
  optional gzip (`OTEL_EXPORTER_OTLP_COMPRESSION=gzip`, using `compress/gzip`), and the headers
  from `OTEL_EXPORTER_OTLP_HEADERS` (for example an API key for a hosted backend).
- **Shutdown:** after the HTTP server has drained, the exporter stops accepting records and
  flushes the queue with a 5-second deadline. The short-lived subcommands (`migrate`, `seed`,
  `import-legacy`) flush the same way before exiting, so their events are not lost.

#### 2.4.6 Configuration

Export uses the **standard OpenTelemetry environment variables**, so the usual collector set-up
(and the Kubernetes OpenTelemetry Operator, which injects them) works without tmock-specific
configuration. Only the subset in §4.2 is read; others are ignored. Export is **off** unless an
endpoint is set; with export off, the app behaves exactly as with console logging alone.

---

## 3. Technology and dependency policy

### 3.1 Choices

| Concern | Backend packages | UI packages (`internal/web`, `internal/views`) |
|---|---|---|
| Go version | `go 1.25` minimum in `go.mod`; build with the latest stable release | same module |
| HTTP server and routing | `net/http`, `http.ServeMux` method and wildcard patterns | same mux |
| Reverse proxy | `net/http/httputil.ReverseProxy` with a `Rewrite` func | — |
| JSON | `encoding/json` | `encoding/json` (condition builder, popovers) |
| PostgreSQL | `database/sql` + `github.com/jackc/pgx/v5/stdlib` driver (§3.3) | — (goes through `internal/app`) |
| Password hashing | `crypto/pbkdf2` with SHA-256 (§9.1) | — |
| Sessions | PostgreSQL table `sessions` (§9.2) | the same session cookie and the same `internal/auth` |
| CSRF | `http.CrossOriginProtection` (§9.3) | same |
| Logging | `log/slog` | `log/slog` |
| Telemetry export | own OTLP/HTTP+JSON log exporter in `internal/logx/otlp` (`net/http`, `encoding/json`, `compress/gzip`); §2.4 | same pipeline |
| Config | `os.Getenv` + small `.env` loader | same |
| Time zones | `time` + `import _ "time/tzdata"` so that `TZ` works in minimal images | — |
| HTML | — | `maragu.dev/gomponents` |
| Styling | — | one hand-written `static/app.css`, values copied from the legacy app; no CSS framework or build step (§10.6) |
| Interactivity | — | HTMX **4.x** (4.0.2 at the time of writing) with its `hx-alpine-compat` extension, and AlpineJS 3.x **CSP build**, vendored and embedded (§10.1) |
| API docs | hand-written `openapi.json` (embedded); `/swagger` page loads Swagger UI from a CDN | — |
| Tests | `testing`, `net/http/httptest`, `testing/synctest` where timing matters | same |

The **gomponents-htmx** helper module is NOT used. `internal/views/hx` provides the handful of
`hx-*` attribute helpers needed, so the UI has a single third-party dependency.

### 3.2 Exception register

Adding any direct requirement to `go.mod`, or letting a package import a non-stdlib package outside
the §1.3 table (which also means updating the allow-list in `internal/archtest`), requires an entry
here, agreed in review. Indirect requirements pulled in by an approved module need no entry of
their own, but are reviewed when the approved module is upgraded.

| # | Module | Scope | Reason | Decided in | Status |
|---|---|---|---|---|---|
| — | `maragu.dev/gomponents` | UI packages only | Pre-approved by project intent | intent.md | Approved |
| E1 | `github.com/jackc/pgx/v5` (used via `pgx/v5/stdlib`) | `internal/db` only | The standard library has no PostgreSQL driver; writing one in-house was judged too large and risky for the value it adds | spec review | Approved |

Dev-only tools (`gofmt`, `go vet`, `staticcheck` installed by the developer) are not dependencies
and MUST NOT appear in `go.mod`.

**Considered and rejected**

| Module | Reason |
|---|---|
| OpenTelemetry Go SDK (`go.opentelemetry.io/otel`, `otel/sdk/log`, `otel/exporters/otlp/otlplog/...`, `contrib/bridges/otelslog`) | Exporting logs needs about ten modules plus, for the OTLP exporters, `google.golang.org/grpc` and `google.golang.org/protobuf`, and the logs SDK is still pre-1.0. tmock only needs to ship slog records to a collector; an OTLP/HTTP JSON exporter is a few hundred lines of standard-library code that we can test fully (§2.4.5). Revisit if tracing or metrics (§14) are taken up. |

### 3.3 PostgreSQL driver: pgx via `database/sql`

The standard library has `database/sql` but no PostgreSQL driver. The app uses
**`github.com/jackc/pgx/v5`** (v5.11.0 at the time of writing; requires Go 1.25+ and supports
PostgreSQL 14+; tmock targets PostgreSQL 17 only) through its `database/sql` adapter:

- `internal/db` blank-imports `github.com/jackc/pgx/v5/stdlib` and opens the pool with
  `sql.Open("pgx", dsn)`. No other package imports pgx (§1.3).
- All SQL in `store` is plain `database/sql` with `$n` parameters. pgx-native APIs (`pgxpool`,
  `pgx.Conn`, `CopyFrom`, …) are **not** used, so the rest of the backend stays standard-library
  only and the driver remains replaceable.
- The DSN is passed through to pgx, which understands URLs and the libpq `sslmode` values:
  `disable`, `require` (TLS without certificate verification, which is what the legacy deployment
  does today), `verify-ca` and `verify-full`. pgx also handles SCRAM-SHA-256 and MD5
  authentication, `.pgpass`, connect timeouts and query cancellation on context expiry.
- Pool settings are set on `*sql.DB`: `SetMaxOpenConns(10)`, `SetMaxIdleConns(10)`,
  `SetConnMaxIdleTime(5m)`.
- Errors: `internal/db` unwraps `*pgconn.PgError` and maps SQLSTATE `23505` (unique_violation) and
  `23503` (foreign_key_violation) to the typed errors that `store` returns (§5.3). Only
  `internal/db` sees the pgx error type.
- `jsonb` columns are scanned into `[]byte` and decoded with `encoding/json`; `bytea` into
  `[]byte`; `timestamptz` into `time.Time`. A nullable `jsonb` column cannot be scanned into
  `json.RawMessage` directly: `database/sql` does not store `NULL` in a named byte-slice type
  (found in I02; `json.RawMessage` works for `NOT NULL` columns).

---

## 4. Configuration

### 4.1 Loading

- Values come from environment variables. A `.env` file in the working directory, if present,
  supplies defaults (real environment variables win). The loader supports `KEY=value`, quoted
  values, `#` comments and blank lines. Nothing more.
- Invalid values (for example a non-numeric `PORT`, or `DISABLE_CACHE=maybe`) MUST stop the
  process at start-up with a clear message. Booleans accept `true`/`false`/`1`/`0` only.

### 4.2 Variables

Names are **kept** from the legacy app where the meaning is the same, so existing deployment
configuration can be reused with few edits.

| Variable | Default | Meaning |
|---|---|---|
| `PORT` | `3000` | Listen port |
| `TZ` | system | Time zone for templates (`date` filter) |
| `TMOCK_ENV` | `development` | `production` → secure cookies, `upgrade-insecure-requests`, `SESSION_SECRET` required |
| `DATABASE_URL` | — | Full DSN, passed to pgx (§3.3). If unset, it is built from the variables below. Required by `serve`, `migrate` and `seed`, which stop with a clear message when neither is set (`help` needs no database); setting only some of `DATABASE_HOST`, `_NAME`, `_USER` is an error |
| `DATABASE_HOST`, `DATABASE_PORT`, `DATABASE_NAME`, `DATABASE_USER`, `DATABASE_PASSWORD` | —, `5432`, … | PostgreSQL connection |
| `DATABASE_ENABLE_SSL` | `false` | `true` → `sslmode=require` |
| `SESSION_SECRET` | random per process outside production | Secret for hashing session IDs; at least 32 characters (for example `openssl rand -base64 48`) |
| `DISABLE_CACHE` | `false` | `true` → the proxy reads the DB on every request |
| `CACHE_TTL` | `10m` | Catalog cache TTL (Go duration) |
| `SERVICES_API_KEY` | — | If set, the `Apikey` header is added to every upstream request |
| `SERVICES_UPSTREAM_OVERRIDES` | — | `path::url,path::url` overrides of upstream URLs |
| `PROXY_REMOVE_FORWARDED_HEADERS` | `false` | `true` → strip `x-forwarded-host`, `-port`, `-proto`, `-scheme`, `x-scheme`, `x-forwarded-server` from upstream requests |
| `PROXY_UPSTREAM_TIMEOUT` | `30s` | Timeout for upstream requests (Go duration) |
| `PROXY_CORS_ALLOWED_ORIGINS` | *(empty = reflect any origin)* | Comma-separated allow-list for proxy CORS (§9.5) |
| `LOG_LEVEL` | `info` | `debug` \| `info` \| `warn` \| `error` |
| `LOG_FORMAT` | `text` | `text` \| `json` (console output; OTLP export is independent of it) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | — | Base URL of an OTLP/HTTP collector (e.g. `http://otel-collector:4318`); `/v1/logs` is appended. Setting it (or the next variable) turns log export on (§2.4) |
| `OTEL_EXPORTER_OTLP_LOGS_ENDPOINT` | — | Full logs URL, used as is; wins over the base endpoint |
| `OTEL_EXPORTER_OTLP_PROTOCOL`, `OTEL_EXPORTER_OTLP_LOGS_PROTOCOL` | `http/json` | Only `http/json` is supported; any other value stops the process at start-up |
| `OTEL_EXPORTER_OTLP_HEADERS`, `OTEL_EXPORTER_OTLP_LOGS_HEADERS` | — | `key=value,key=value` (values URL-encoded) sent with every export; never logged |
| `OTEL_EXPORTER_OTLP_TIMEOUT`, `OTEL_EXPORTER_OTLP_LOGS_TIMEOUT` | `10000` | Per-request timeout in milliseconds |
| `OTEL_EXPORTER_OTLP_COMPRESSION`, `OTEL_EXPORTER_OTLP_LOGS_COMPRESSION` | `none` | `none` \| `gzip` |
| `OTEL_SERVICE_NAME` | `tmock` | `service.name` resource attribute |
| `OTEL_RESOURCE_ATTRIBUTES` | — | Extra resource attributes, `key=value,…` |
| `OTEL_LOGS_EXPORTER` | `otlp` | `otlp` \| `none` (`none` turns export off even when an endpoint is set) |
| `OTEL_SDK_DISABLED` | `false` | `true` turns export off |
| `OTEL_BLRP_SCHEDULE_DELAY` | `1000` | Max delay in ms before a batch is sent |
| `OTEL_BLRP_MAX_QUEUE_SIZE` | `2048` | Records buffered before new ones are dropped |
| `OTEL_BLRP_MAX_EXPORT_BATCH_SIZE` | `512` | Records per export request (must be ≤ the queue size) |
| `OTEL_BLRP_EXPORT_TIMEOUT` | `30000` | Total time in ms for one batch, including retries |
| `MIGRATE_ON_START` | `false` | Run migrations before serving. When `false`, `serve` refuses to start while migrations are pending |
| `ADMIN_NAME`, `ADMIN_EMAIL`, `ADMIN_PASSWORD` | `Admin`, —, — | Used by `tmock seed` only; email and password required |

---

## 5. Data model and migrations

### 5.1 Schema

The Go app has its **own schema**, designed for it. No table prefix. Unset optional values are
stored as `NULL`, never as `""` or `0`; `internal/app` normalises empty input to `NULL` on save, so
"empty" has one meaning everywhere.

```sql
CREATE TABLE services (
  id           bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  path         text NOT NULL UNIQUE CHECK (path ~ '^[A-Za-z0-9._-]{1,255}$'),
  name         text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  upstream_url text NOT NULL CHECK (length(upstream_url) BETWEEN 1 AND 2048)
);

CREATE TABLE scenarios (
  id                bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  service_id        bigint NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  name              text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  request_method    text,                     -- upper-case, NULL = any
  request_path      text,                     -- trimmed of '/', NULL = any
  request_condition text,                     -- expr source (§7.4), NULL = none
  response_code     integer CHECK (response_code BETWEEN 100 AND 599),
  response_headers  jsonb,                    -- {"Name": "value" | null}
  response_body     text,
  skip_proxy        boolean NOT NULL DEFAULT false,
  UNIQUE (service_id, name)
);

CREATE TABLE clients (
  id         bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name       text NOT NULL UNIQUE CHECK (length(name) BETWEEN 1 AND 255),
  enabled    boolean NOT NULL DEFAULT false,
  condition  jsonb,
  token_hash bytea UNIQUE                     -- SHA-256 of the token; NULL = no token
);

CREATE TABLE client_scenarios (
  client_id   bigint NOT NULL REFERENCES clients(id)   ON DELETE CASCADE,
  scenario_id bigint NOT NULL REFERENCES scenarios(id) ON DELETE CASCADE,
  PRIMARY KEY (client_id, scenario_id)
);

CREATE TABLE users (
  id            bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
  name          text NOT NULL CHECK (length(name) BETWEEN 1 AND 255),
  email         text NOT NULL,
  password_hash text NOT NULL,                -- §9.1
  admin         boolean NOT NULL DEFAULT false,
  created_at    timestamptz NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX users_email_key ON users (lower(email));

CREATE TABLE sessions (
  id         bytea PRIMARY KEY,               -- HMAC-SHA256 of the cookie value (§9.2)
  user_id    bigint NOT NULL REFERENCES users(id) ON DELETE CASCADE,
  expires_at timestamptz NOT NULL
);
CREATE INDEX sessions_expires_at_idx ON sessions (expires_at);
```

Notes:

- Services have a surrogate `id`; the API and the proxy address them by `path` (**Kept**).
  Renaming a path is a plain `UPDATE`, and scenarios follow automatically.
- Users are **hard-deleted**. Their sessions go with them through the cascade.
- Scenario and client names are unique in the database, not just by convention in code.

### 5.2 Migration runner

- Migrations are plain `.sql` files embedded from `internal/db/migrations/`, applied in lexical
  order and tracked in `schema_migrations (version text PRIMARY KEY, applied_at timestamptz)`.
  Files are named `NNNN_name.sql`; `NNNN` is the version stored, `name` is logged
  (`tmock.db.migration.applied`). Any other file name stops the run.
- Each migration runs in a transaction, under `pg_advisory_lock`, so concurrent starts are safe:
  a run that waited for the lock finds the work done and applies nothing.
- `serve` and `seed` check that no migration is pending and stop with a message pointing at
  `tmock migrate` or `MIGRATE_ON_START=true` otherwise.
- Migrations are forward-only. A mistake is fixed with a new migration.
- `0001_init.sql` creates the schema in §5.1.

### 5.3 Store behaviour

| Repository | Operation | Behaviour |
|---|---|---|
| services | list | `ORDER BY name` |
| services | snapshot (catalog) | every service, with its scenarios `ORDER BY name` (the same order as the scenario list, §7.5) |
| scenarios | list | `ORDER BY service path, name` |
| clients | list | `ORDER BY id`, each with its assigned scenario ids and their service path and name |
| clients | snapshot (catalog) | `enabled = true ORDER BY id`, each with its set of scenario ids |
| clients | set scenarios | delete all assignments, then insert the new set, in one transaction |
| clients | disable all | `UPDATE clients SET enabled = false` |
| clients | find by token | `WHERE token_hash = sha256(token)` |
| users | list | `ORDER BY id` |
| users | count admins | used by the delete and demote rules |
| sessions | create / get (join user, not expired) / delete / sweep expired | — |

Unique and foreign-key violations (SQLSTATE `23505`, `23503`) are mapped to typed errors that
`internal/app` turns into `409` and `400` respectively.

---

## 6. REST API

The REST API is for test automation and scripts. The dashboard does not use it (§2.1).

All paths are under `/api`. JSON in and out, UTF-8. The request body limit is **10 MB**. Unknown
JSON fields are rejected with `400` (`DisallowUnknownFields`), so typos surface immediately.

**Kept:** the resource paths (`/api/service`, `/api/scenario`, `/api/client`, `/api/user`) and the
camelCase JSON field names, so legacy scripts need only small changes (§12). Response envelopes,
status codes and error messages follow the conventions below, not the legacy ones.

### 6.1 Object shapes

Optional fields are always present, as `null` when unset.

```jsonc
// Service
{ "path": "svc", "name": "Svc", "upstreamUrl": "http://…" }

// Scenario
{ "id": 1, "service": "svc", "name": "…",
  "requestMethod": "GET" | null, "requestPath": "content" | null, "requestCondition": "…" | null,
  "responseCode": 500 | null, "responseHeaders": { "Content-Type": "…" } | null,
  "responseBody": "…" | null, "skipProxy": false }

// Client
{ "id": 1, "name": "…", "enabled": true, "condition": Condition | null,
  "scenarios": [1, 4], "hasToken": true }

// Condition (recursive; exactly one predicate per node, §7.3)
{ "and"?: Condition[], "or"?: Condition[], "not"?: boolean,
  "headerMatch"?: { "header": "…", "value": "…" }, "headerRegex"?: { "header": "…", "value": "…" },
  "ip"?: "…", "cidr"?: "…" }

// User (never contains the password)
{ "id": 1, "name": "…", "email": "…", "admin": true }
```

- Create and update responses return the **resource itself**, not a `{"message": …}` wrapper.
- `PATCH` semantics: an *absent* field is left unchanged; `null` clears an optional field; any
  other value sets it. `null` on a required field (for example `name`) is `400`. Implemented with
  `httpx.Optional[T]`, which records whether the field was present during decoding.

### 6.2 Status codes and errors

| Situation | Status |
|---|---|
| Read, update | `200` |
| Create, token generation | `201` |
| Delete, logout, disable-all | `204`, no body |
| Validation error, malformed JSON, unknown field, non-integer id | `400` |
| Not logged in, bad credentials, unknown client token | `401` |
| Logged in but not allowed (not admin, token for another client, disallowed field) | `403` |
| Not found, unknown route | `404` |
| Known route, wrong method | `405`, with an `Allow` header |
| Duplicate (path, name, email) | `409` |
| Body over the limit | `413` |
| Unexpected error or panic | `500` (details logged, never returned) |
| Upstream unreachable (proxy only) | `502` |

Every error response has one shape:

```json
{ "status": 400, "message": "Validation failed", "details": { "email": "must be an email address" } }
{ "status": 409, "message": "A service with path \"svc\" already exists" }
```

`details` is present only for validation errors and maps the JSON field path (for example
`condition.and[1].ip`) to a short message. The same messages are shown in the UI forms.

Request bodies are decoded by `httpx.DecodeJSON`: an empty body, malformed JSON, a body that is
not an object (`null` included), anything after the object, and an unknown field
(`details: {"nmae": "unknown field"}`) are `400`; a value of the wrong type is `400` with
`details` keyed by its path (`items[1].label: "must be a string"`). `encoding/json` names an
unknown field without its path, so one inside a nested object is reported by its bare name. With
Go 1.27's `encoding/json`, a wrong type inside an `Optional[T]` field is reported without the
field name (the message says which type was expected); Go 1.25 names the field (found in I03).
A path id must be written canonically (`/api/service/7`); `0`, `+7` and `007` are `400`.

Use cases in `internal/app` return `*app.Error` (kinds `Invalid`, `Unauthorized`, `Forbidden`,
`NotFound`, `Conflict`) for expected failures; its message and details are safe to show. A
`NotFound` or `Conflict` about one input field (a taken scenario name, an unknown service) names
it in `Error.Field`, which the UI uses to show the message next to that field; the REST API does
not write it (found in I06). The REST
API writes it with `httpx.WriteErr` as the body above, the UI shows it as a toast and next to the
form fields (§10.2). Any other error is unexpected: `500`, logged, never shown.

### 6.3 Authentication on the API

| Guard | Rule | Failure |
|---|---|---|
| `public` | — | — |
| `user` | valid session cookie (§9.2) | `401` |
| `admin` | `user`, and the user is an admin | `401` / `403` |
| `token-or-user` | If `X-Client-Token` is present: it must belong to a client (else `401`), the path `{id}` must be that client's id (else `403`), and the body may contain only `enabled` and `scenarios` (else `403`, naming the fields). Otherwise the `user` rule applies. | as listed |

Browsers are protected from cross-site requests by `http.CrossOriginProtection` (§9.3). Non-browser
clients (curl, test automation) are unaffected and need no CSRF token.

### 6.4 Endpoints

**Health and session**

| Method & path | Guard | Body | Success |
|---|---|---|---|
| `GET /api/status`, `GET /status` | public | — | `200 {"status":"ok"}` |
| `POST /api/auth/login` | public | `{email, password}` | `200 User`; sets the session cookie. Wrong email or password → `401` with one generic message |
| `POST /api/auth/logout` | public | — | `204`; deletes the session; clears the cookie |
| `GET /api/auth/me` | user | — | `200 User` |

**Services** (guard: `user`)

| Method & path | Body | Success | Errors |
|---|---|---|---|
| `GET /api/service` | — | `200 Service[]` | — |
| `POST /api/service` | `{path, name, upstreamUrl}` | `201 Service` | `400`; `409` duplicate path |
| `GET /api/service/{path}` | — | `200 Service` | `404` |
| `PATCH /api/service/{path}` | any subset of the create fields | `200 Service` | `400`; `404`; `409` |
| `DELETE /api/service/{path}` | — | `204`; cascades to scenarios and assignments | `404` |

Validation: `path` matches `^[A-Za-z0-9._-]{1,255}$` and is not `.` or `..` (the proxy URL
`/api/proxy/../x` is cleaned to another path, so such a service could never be reached; found in
I05); `upstreamUrl` is an absolute `http`/`https` URL with a host, at most 2048 characters; `name`
is 1–255 characters. Leading and trailing white space is trimmed from all three.

**Scenarios** (guard: `user`)

| Method & path | Body | Success | Errors |
|---|---|---|---|
| `GET /api/scenario` | — | `200 Scenario[]` (full bodies); optional `?service=<path>` filter | — |
| `POST /api/scenario` | `{service, name, requestMethod?, requestPath?, requestCondition?, responseCode?, responseHeaders?, responseBody?, skipProxy?}` | `201 Scenario` | `400` (including a request condition that does not parse, with the error position); `404` unknown service; `409` duplicate name within the service |
| `GET /api/scenario/{id}` | — | `200 Scenario` | `404` |
| `PATCH /api/scenario/{id}` | any subset | `200 Scenario` | `400`; `404`; `409` |
| `DELETE /api/scenario/{id}` | — | `204` | `404` |

Validation: `requestMethod` ∈ `GET POST PUT PATCH DELETE HEAD OPTIONS` (stored upper-case);
`responseCode` 100–599; `responseHeaders` is an object whose values are strings or `null`;
`skipProxy: true` requires a non-null `responseBody`.

Found in I06:

- Values are normalised before they are checked: `service`, `name`, `requestMethod`,
  `requestCondition` are trimmed, `requestPath` is also trimmed of `/`; an empty string (and an
  empty `responseHeaders` object) is stored as `NULL` (§5.1). `responseBody` is stored as
  written, but `""` is unset too, so `skipProxy` with an empty body is `400`.
- Header names must be HTTP tokens and must not differ only in case (`X-A` and `x-a` are the
  same header); values must not contain line breaks. All header problems are reported under
  `details.responseHeaders`.
- The unknown-service `404` comes after validation. A `PATCH` with `service` moves the scenario
  to that service. `?service=` with an unknown path returns `[]`.
- The `409` message is `A scenario named "x" already exists in service "svc"`; the scenario's
  `404` is `Scenario 7 not found`.

**Clients** (guard: `token-or-user` on `GET`/`PATCH /api/client/{id}`, `user` elsewhere)

| Method & path | Body | Success | Errors |
|---|---|---|---|
| `GET /api/client` | — | `200 Client[]` | — |
| `POST /api/client` | `{name, enabled?, condition?, scenarios?}` | `201 Client` | `400`; `409` duplicate name |
| `GET /api/client/{id}` | — | `200 Client` | `404` |
| `PATCH /api/client/{id}` | `{name?, enabled?, condition?, scenarios?}` | `200 Client` | `400`; `404`; `409` |
| `DELETE /api/client/{id}` | — | `204` | `404` |
| `POST /api/client/disable-all` | — | `204` | — |
| `POST /api/client/{id}/token` | — | `201 {"token":"tmck_<base64url(32 random bytes)>"}`; replaces any existing token. The token is shown **only in this response**. | `404` |
| `DELETE /api/client/{id}/token` | — | `204` | `404` |

Rules:

- **Enable rule.** A client can only be enabled when its resulting condition is non-empty (§7.3).
  Checked on create and on every patch.
- **Condition validation** at every level: exactly one predicate per node; `and`/`or` are arrays of
  conditions; header names and values are 1–255 characters; `ip` is a valid IPv4 or IPv6 address;
  `cidr` parses as a prefix (`net/netip`); `headerRegex.value` compiles as RE2. Errors name the
  path, for example `condition.or[0].cidr`.
- `scenarios` is the full list of assigned scenario ids; unknown ids → `400`.
- Found in I07: duplicate ids in `scenarios` are dropped, and the list is returned sorted.
  `null` for `name`, `enabled` or `scenarios` is `400`; `null` for `condition` clears it (and is
  refused while the client is enabled). Unknown ids are listed:
  `details.scenarios: "unknown scenario ids: 7, 9"`. The enable-rule message is
  `details.enabled: "a client needs a condition before it can be enabled"`. Condition errors
  name the path under `condition` (`condition.or[1].cidr`); a node with no or several predicates
  is reported at its own path (`condition.and[0]`). `ip` and `cidr` must not carry an IPv6 zone.
  Revoking a token when there is none is `204`.
- Found in I07 (concurrency): an update is a read-modify-write, so it locks the client's row
  (`SELECT … FOR UPDATE`) from the read to the write; overlapping updates of one client run
  one after the other and none is lost (for example a rename in the UI and an enable by token).
  The UI's assignment switches and chip ✕ change the one `client_scenarios` row, not the whole
  list, so toggles sent at once all stay. A toggle that changes nothing logs nothing.
- Found in I07 (token-or-user): with a token, `{id}` is checked before the body; the field
  check reads only the top-level keys, so a body that is not a JSON object still reaches the
  handler's `400`. The `403` messages are `This token belongs to another client` and
  `A client token may only change enabled and scenarios, not: condition, name`. Token changes
  are logged with `tmock.actor=client-token` and `tmock.client.id`.
- Every client, scenario and service mutation invalidates the catalog (§7.2). There are no manual
  "clear cache" endpoints.

**Users** (guard: `admin`, except the profile endpoints)

| Method & path | Guard | Body | Success | Errors |
|---|---|---|---|---|
| `GET /api/user/profile` | user | — | `200 User` | — |
| `PATCH /api/user/profile` | user | `{name?, email?, currentPassword?, newPassword?}` | `200 User` | `400` (a new password needs the correct current password); `409` duplicate email |
| `GET /api/user` | admin | — | `200 User[]` | — |
| `POST /api/user` | admin | `{name, email, password, admin?}` | `201 User` | `400`; `409` |
| `PATCH /api/user/{id}` | admin | `{name?, email?, password?, admin?}` | `200 User` | `400`; `404`; `409` |
| `DELETE /api/user/{id}` | admin | — | `204` | `400` own account or last admin; `404` |

Passwords are at least 8 characters. Demoting the last admin is also rejected with `400`.

### 6.5 OpenAPI

`openapi/openapi.json` (OpenAPI 3.1) is written by hand and embedded. `GET /swagger` serves a page
that loads `swagger-ui-dist` from a CDN, with a CSP exception for that page only.
`GET /swagger/openapi.json` serves the spec. A unit test MUST check that every registered API route
appears in the spec, and vice versa.

---

## 7. Proxy and mock engine

Feature reference: `api/src/proxy/proxy.service.ts` and `api/src/common/utils/helpers.ts`.

### 7.1 Routing

- `ANY /api/proxy/{service}` and `ANY /api/proxy/{service}/{rest...}` (**Kept**, so services under
  test need no reconfiguration beyond the host).
- `service` is matched exactly (case-sensitive) against `services.path`. If there is no match, the
  response is `404 text/plain "Service not found"`, with no upstream call.
- The proxy needs no session and no CSRF check. Proxy CORS applies (§9.5); security headers do not.

### 7.2 Catalog (cache)

- The `catalog` package holds an immutable snapshot behind an `atomic.Pointer`:
  - all services, with `SERVICES_UPSTREAM_OVERRIDES` already applied, each with its scenarios
    ordered by name, in the order the database returns them (§5.3; the catalog does not re-sort
    them in Go, so the proxy and the scenario list always agree);
  - the enabled clients, ordered by id, each with its set of scenario ids, with conditions and
    regexes pre-compiled.
- The snapshot is rebuilt lazily when it is older than `CACHE_TTL`, or after an invalidation. Only
  one rebuild runs at a time; concurrent requests use the previous snapshot until the new one is
  ready.
- `internal/app` invalidates the snapshot after **every** mutation of services, scenarios or
  clients. `DISABLE_CACHE=true` builds a fresh snapshot for every proxy request.
- The snapshot is **never mutated** after it is built. Per-request values (the rendered body)
  live in request-scoped structs.
- With several replicas, each one has its own cache. Changes made on one replica reach the others
  within `CACHE_TTL`; set a short TTL (for example `5s`) when running more than one replica.

### 7.3 Client conditions

Matching clients are the enabled clients whose condition evaluates to true, in id order.

Each condition node has exactly one predicate, plus an optional `not` that negates it:

| Predicate | True when |
|---|---|
| `and: [...]` | every child is true (empty list → true) |
| `or: [...]` | at least one child is true (empty list → false) |
| `ip` | the client IP equals the value |
| `cidr` | the client IP is in the prefix (IPv4 and IPv6) |
| `headerMatch` | the header value equals the value (a missing header never matches) |
| `headerRegex` | the RE2 regex matches the header value, unanchored (a missing header is `""`) |

- A condition is **empty** when it is `null`, or when it contains no `ip`, `cidr`, `headerMatch` or
  `headerRegex` predicate anywhere in its tree. `{"and": []}` is empty. Empty conditions cannot be
  enabled (§6.4).
- Header names are case-insensitive. Multiple values of one header are joined with `", "`.
- **Client IP:** the first value that is a valid IP address of `cf-connecting-ip`, then `x-real-ip`,
  then the first entry of `x-forwarded-for`, then the socket's remote address (without the port),
  trimmed. Header values that are not IP addresses are skipped, so they never reach logs or conditions.
  IPv4-mapped IPv6 addresses (`::ffff:a.b.c.d`) are normalised to IPv4 on both sides.

### 7.4 Request conditions (scenario `requestCondition`)

The legacy app runs these through JavaScript `eval`, which lets any logged-in user execute code on
the server. The Go app uses a small, safe expression language (package `expr`):

```
expr     := or
or       := and ( "||" and )*
and      := unary ( "&&" unary )*
unary    := "!" unary | cmp
cmp      := primary ( op primary )?          op ∈ == != < <= > >=   (=== and !== are aliases)
primary  := literal | ref | "(" expr ")"
ref      := "$request" ( "." ident | "[" ( string | integer ) "]" )*
literal  := 'single' | "double" | number | true | false | null
```

- **Namespace:** only `$request`, with `method`, `path`, `query` (map of first values), `headers`
  (map with lower-case keys), `body` (parsed JSON, or the form map for
  `application/x-www-form-urlencoded`, otherwise `null`) and `ip` (§7.3).
- **Semantics** are deliberately simple and strict:
  - `==` and `!=` compare JSON values by type and value; numbers compare numerically;
  - `<`, `<=`, `>`, `>=` compare two numbers or two strings; anything else is false;
  - a missing path yields `null`, never an error;
  - truthiness (for bare references and `!`): `null`, `false`, `0` and `""` are false.
- A condition that does not parse is rejected when the scenario is saved (`400` with the error
  position). Evaluation itself cannot fail.

### 7.5 Scenario selection

Given the service `S`, the matching clients `C` (in id order) and `rest`, the part of the path after
`/api/proxy/{service}/`:

1. If `C` is empty, there is no scenario: proxy the request normally (§7.7).
2. Let `reqPath` = `rest` without the query string and without leading or trailing `/`. For
   example, `/api/proxy/users/orders/42/?page=2` gives `orders/42`, and so does
   `/api/proxy/users/orders/42`; a scenario with `requestPath` `orders/42` matches both.
3. For each scenario `s` of `S` in name order (as the scenario list shows them; names are unique
   within a service, so the order is fixed), skip it when:
   - no client in `C` has `s` assigned;
   - `s.requestPath` is set and does not equal `reqPath` case-insensitively;
   - `s.requestMethod` is set and does not equal the request method;
   - `s.requestCondition` is set and evaluates to false.

   The first scenario that is not skipped wins.
4. The **reported client** is the first client in `C` that has the winning scenario assigned. When
   no scenario wins, it is the first client in `C`.

### 7.6 Short-circuit (skip proxy)

If the winning scenario has `skipProxy = true` (which implies a body, §6.4), the upstream is never
contacted:

- status: `responseCode`, or 200 when unset;
- headers: the scenario's `responseHeaders` (a `null` value is ignored here), then the
  content-type fallback (§7.8 step 4), then the `x-tmock-*` headers;
- body: the rendered template (§8).

### 7.7 Upstream forwarding

- **Target:** the service's `upstreamUrl` (after overrides), joined with `/` + `rest` and the
  original raw query. When `upstreamUrl` has a path, `rest` is appended to it
  (`http://h/base` + `content` → `http://h/base/content`). An empty `rest` becomes the base path,
  or `/`.
- **Host:** set to the upstream host.
- **Request headers:** the client's headers minus hop-by-hop headers. `X-Forwarded-For` is not
  added (use `ReverseProxy.Rewrite`, never `Director`). In addition:
  - `Apikey: $SERVICES_API_KEY` when set;
  - the six forwarded headers are removed when `PROXY_REMOVE_FORWARDED_HEADERS=true`;
  - `x-tmock-status: mocked|proxied`, `x-tmock-client: <name>` (if a client matched) and
    `x-tmock-scenario: <name>` (if a scenario matched). `mocked` means "a scenario applies", even
    when the request still goes upstream.
- **Request body:** forwarded unchanged and streamed. It is buffered (up to 10 MB, otherwise `413`)
  only when a candidate scenario has a `requestCondition` that needs it.
- **Upstream failure** (connection refused, DNS, timeout): `502 text/plain "Bad Gateway"`, logged.

### 7.8 Upstream response modification

- **No scenario:** status, headers and body pass through unchanged and streamed. The response gets
  `x-tmock-status: proxied`, plus `x-tmock-client` if a client matched.
- **With a scenario:**
  1. Start from the upstream headers, then apply the scenario's `responseHeaders` (`null` deletes
     the header).
  2. If `responseCode` is set, override the status.
  3. If `responseBody` is set: remove `ETag`, `Content-Encoding` and `Content-Length`; discard the
     upstream body; write the rendered template.
  4. If the response has no `Content-Type` after step 1, set `application/json` when the body is
     valid JSON, otherwise `text/plain; charset=utf-8`.
  5. Set the `x-tmock-*` headers.
- Without a replaced body, the upstream body is streamed untouched (compressed bodies stay
  compressed).

### 7.9 Proxy log line

One line per proxy request at level `info`:

```
<ip> <METHOD> <url> <status>[ [client <c>; scenario <s>; mocked: status, headers, body[; proxy skipped]]] <ms>ms[ <size>]
```

`mocked:` lists `status`, `headers` and `body`, but only those the scenario defines. Sizes are
human-readable (`0B`, `512B`, `1.5KB`, `2MB`).

This line is the message of the `tmock.proxy.request` event; the same data is attached as
structured attributes (§2.4.2). An upstream failure additionally logs
`tmock.proxy.upstream_failed` with the error.

---

## 8. Response templating (Liquid subset)

### 8.1 Activation

- A `responseBody` that **starts with** the marker `<% TEMPLATE: LiquidJS %>` is a template
  (**Kept**, so imported scenarios work). The marker is removed and the rest is rendered with an
  **empty context**: only literals and `assign` are available.
- Template syntax is checked when the scenario is saved (`400` with the line and column). If
  rendering still fails at request time, the error is logged and the body is returned unrendered.
- Rendering happens **on every request**, so `'now'` is always current.

### 8.2 Supported language

- **Output:** `{{ expr | filter: arg, arg }}`, with whitespace control (`{{-`, `-}}`, `{%-`, `-%}`).
- **Tags:** `assign`, `capture`/`endcapture`, `if`/`elsif`/`else`/`endif`,
  `unless`/`else`/`endunless`, `case`/`when`/`else`/`endcase`, `for … in …` (with `limit:`,
  `offset:`, `reversed`, ranges `(1..n)`, `else`, and `forloop.index`, `.index0`, `.first`,
  `.last`, `.length`), `break`, `continue`, `comment`/`endcomment`, `raw`/`endraw`.
- **Expressions:** string, integer and float literals; `true`, `false`, `nil`, `empty`, `blank`;
  variables with `.` and `[ ]` access; `==`, `!=`, `<>`, `<`, `>`, `<=`, `>=`, `contains`; `and`
  and `or` (evaluated right to left, as in Liquid).
- **Filters:**
  - math: `plus`, `minus`, `times`, `divided_by` (integer division when both operands are
    integers), `modulo`, `abs`, `ceil`, `floor`, `round`, `at_least`, `at_most`;
  - string: `append`, `prepend`, `upcase`, `downcase`, `capitalize`, `strip`, `lstrip`, `rstrip`,
    `replace`, `replace_first`, `remove`, `remove_first`, `split`, `truncate`, `truncatewords`,
    `slice`, `escape`, `url_encode`, `url_decode`, `newline_to_br`, `strip_newlines`;
  - array: `join`, `first`, `last`, `size`, `reverse`, `sort`, `uniq`, `map`, `compact`, `concat`;
  - other: `default`, `json`, `date`.
- **`date` filter:**
  - inputs: `'now'` and `'today'`; numbers or numeric strings (Unix seconds); RFC 3339 strings;
    `YYYY-MM-DD`;
  - strftime directives: `%a %A %b %B %d %e %H %I %j %k %l %L %m %M %p %P %s %S %u %w %y %Y %z %Z %%`,
    plus the `-` flag (no padding);
  - time zone: the process local zone (`TZ`).
- **Undefined** variables and filters render as empty strings.
- **Number formatting:** integers have no decimal point; floats use the shortest round-trip form.

### 8.3 Examples that MUST work (from the legacy README and test seed)

```liquid
{% assign seconds = 5 | times: 24 | times: 60 | times: 60 %}{"validFrom": "{{ 'now' | date: "%s" | plus: seconds | date: "%Y-%m-%d" }}"}
{{ 'now' | date: "%Y-%m-%dT%H:%M:%SZ" }}
```

### 8.4 Tests

Table-driven cases with hand-checked expected output live in `internal/liquid/testdata/cases.txt`.
Cases involving `now` are checked structurally (format, and within ±2 seconds). When a case is in
doubt, the expected output MAY be cross-checked by hand with LiquidJS; no generator script is kept.

---

## 9. Authentication, sessions and HTTP security

### 9.1 Passwords

- Hashing uses `crypto/pbkdf2` with HMAC-SHA-256, 600,000 iterations, a 16-byte random salt and a
  32-byte key, following the OWASP recommendation for PBKDF2.
- Stored as `pbkdf2-sha256$<iterations>$<base64 salt>$<base64 key>`, so the parameters can be
  raised later. A hash with fewer iterations than the current setting is re-hashed on successful
  login.
- Comparison is constant-time. Login looks the user up by `lower(email)`. "Unknown email" and
  "wrong password" return the same `401` message, and both paths take similar time (a dummy hash
  is computed for unknown emails).

### 9.2 Sessions

- Cookie `tmock_session` holds 32 random bytes, base64url-encoded. Attributes: `HttpOnly`,
  `SameSite=Lax`, `Path=/`, `Max-Age=86400`, and `Secure` in production.
- The database stores only `HMAC-SHA256(SESSION_SECRET, cookie value)` as `sessions.id`. Sessions
  expire 24 hours after login (absolute, no sliding window).
- Every authenticated request loads the session joined with its user, so deleting a user or
  changing their admin flag takes effect immediately.
- Login always creates a new session ID. Logout deletes the row and expires the cookie.
- Expired rows are swept hourly.
- Outside production, a missing `SESSION_SECRET` is replaced by a random one per process, which
  means sessions do not survive a restart in development.

### 9.3 CSRF

- All routes except `/api/proxy/*` are wrapped in Go's `http.CrossOriginProtection`. It rejects
  unsafe cross-origin browser requests using the `Sec-Fetch-Site` and `Origin` headers, and lets
  non-browser clients through.
- Together with `SameSite=Lax` cookies, this replaces CSRF tokens entirely: no token endpoint, no
  hidden form fields, no `X-CSRF-Token` header.
- Rejected requests get `403`.

### 9.4 Security headers

Applied to API, UI and swagger responses. **Not** applied to proxy responses, which must stay
transparent.

`Content-Security-Policy: default-src 'self'; base-uri 'self'; font-src 'self' data:; form-action 'self'; frame-ancestors 'self'; img-src 'self' data:; object-src 'none'; script-src 'self'; script-src-attr 'none'; style-src 'self'`
(plus `upgrade-insecure-requests` in production only), `Cross-Origin-Opener-Policy: same-origin`,
`Cross-Origin-Resource-Policy: same-origin`, `Referrer-Policy: no-referrer`,
`Strict-Transport-Security: max-age=31536000; includeSubDomains` (production only),
`X-Content-Type-Options: nosniff`, `X-Frame-Options: SAMEORIGIN`. The swagger page gets a relaxed
CSP that allows the CDN.

### 9.5 CORS

- **API and UI:** no CORS headers. The dashboard is same-origin, and automation is not a browser.
- **Proxy:** browser apps under test call the proxy cross-origin, so the proxy reflects the request
  `Origin` with `Access-Control-Allow-Credentials: true` and `Vary: Origin`. When
  `PROXY_CORS_ALLOWED_ORIGINS` is set, only listed origins are reflected. Preflight requests
  (`OPTIONS` with `Access-Control-Request-Method`) get `204`, the method list
  `GET,HEAD,PUT,PATCH,POST,DELETE` and the requested headers, and are not forwarded upstream. CORS
  headers sent by the upstream take precedence over tmock's.

### 9.6 Request log

One line per API and UI request: `<ip> <METHOD> <url> <status> <ms>ms`, through `slog`.
`/api/status` and `/status` are logged at level `debug`, so health checks don't flood the log.
The line is the message of the `tmock.http.request` event (§2.4.2), with the same data as
attributes. Proxy requests are logged by the proxy instead (§7.9), not twice.

---

## 10. Web UI

The pages and workflows follow the legacy dashboard (`app/src`), because users know them. The
implementation is new.

### 10.1 Principles

- **Gomponents** renders every page and fragment. Views are pure functions from data to `g.Node`,
  and have no I/O.
- **HTMX** handles every interaction that reads or writes data. A full page is rendered when
  `HX-Request` is absent. Fragments are swapped in when it is present. Forms are real `<form>`s
  that also work without JavaScript, so a plain submit triggers a full-page redirect.
- **Alpine** (CSP build) is used only for the ephemeral UI state listed in intent.md §6. All
  components live in `static/app.js`, registered via `Alpine.data`. No inline `x-data` expressions
  beyond component names and simple property references.
- **htmx version: 4.x.** It is pinned to an exact release (4.0.2 at the time of writing) with its
  SHA-256 in `static/vendor/VERSIONS`. npm still publishes 4.x under the `next` tag and keeps 2.x as
  `latest`; that doesn't matter here, because the file is vendored, not installed. Upgrades within
  4.x are a `make vendor-js` change plus a click-through.
- **No eval, so no extra CSP work.** In htmx 4, only `hx-on:*`, trigger filters (`[...]`) and
  `js:`/`javascript:` attribute values use `new Function()`. The UI uses none of them; a render test
  fails if any appears in the output. CSP therefore stays as in §9.4, with `script-src 'self'` and no
  `unsafe-eval`, and the `hx-csp` extension (nonces on every element) is **not** needed. No inline
  `<script>` anywhere.
- **Configuration** is set with `<meta name="htmx-config">` in the layout, not in `app.js`:
  htmx 4 reads the meta tag, and creates its indicator stylesheet, while its own script loads,
  before any later script runs. A meta tag is not a script, so the CSP is unaffected.
  - `includeIndicatorCSS: false` (indicator styles live in `app.css`);
  - `history: false`, because navigation between pages is plain links and modals don't push
    URLs; back/forward behave like a normal multi-page site;
  - `implicitInheritance` stays at its default `false`.
- **Explicit inheritance.** htmx 4 attributes don't flow to children unless the parent uses the
  `:inherited` modifier, for example `hx-target:inherited="#modal-body"` on a table whose rows open
  the modal. The `internal/views/hx` helpers emit the modifier explicitly. Request verbs and
  `hx-trigger` are never inherited.
- **Error responses are swapped.** htmx 4 swaps `4xx` and `5xx` responses by default (2.x didn't).
  The UI relies on that:
  - validation and conflict errors return **`422`** with the re-rendered form, which htmx swaps in
    place, plus an error toast;
  - unexpected errors return `500` with `HX-Reswap: none` and an error toast, so the page is left
    as it was.
- **Events** follow htmx 4's `htmx:phase:action` names. `app.js` listens to `htmx:after:settle` to
  open dialogs whose content was just swapped, and to `htmx:error` (network failure or timeout) to
  show a "Connection problem" toast.
- **Alpine integration** uses htmx's `hx-alpine-compat` extension, a separate file from the same htmx
  release. It makes Alpine initialise components against the final DOM after a swap. Script order
  in the layout: `htmx-4.x.y.min.js`, then `hx-alpine-compat-4.x.y.js`, then `app.js`, then Alpine with `defer`.

### 10.2 UI mechanics

- **Use cases, not REST calls.** UI handlers call `internal/app` directly (§2.1). Validation
  errors come back as the same typed error (`message` + `details`) that the REST API serialises.
- **Sessions.** The UI uses the same `tmock_session` cookie and `internal/auth` as the REST API.
- **CSRF.** Handled by `http.CrossOriginProtection` (§9.3). No tokens in pages or forms.
- **Error mapping:**
  - no session → redirect to `/login`; a non-admin on an admin page → redirect to `/`
    (`HX-Redirect` for HTMX requests);
  - validation and conflict errors → `422` with the form re-rendered, the messages next to the
    fields, plus an error toast (§10.1);
  - not found → `404` with `HX-Reswap: none` and an error toast;
  - unexpected errors → `500` with `HX-Reswap: none` and a generic error toast; the details are
    logged.
  - Redirects use `HX-Redirect` on a `200` response, because htmx 4 ignores response headers on
    `3xx` responses. Plain (non-HTMX) requests get an ordinary `303`.
- **Toasts:** handlers set `HX-Trigger: {"toast":{"target":"body","type":"success|error|info|warning","title":"…","message":"…"}}`.
  htmx 4 fires the event after the swap completes, with the object's fields on `event.detail`.
  It fires on the element that sent the request unless `target` names another, and once a swap
  has removed that element the event never reaches `document`; hence `"target":"body"` (found in
  I04). An Alpine store displays toasts top-right and auto-dismisses them after 5 seconds.
  Messages are always escaped.
- **Modals:** a single `<dialog id="modal">` host in the layout. HTMX loads the modal content into
  `#modal-body`, and an Alpine component calls `showModal()` when content arrives
  (`htmx:after:settle`) and empties it on `close`. A click on the backdrop closes it unless the
  content carries `data-persistent`; a response closes it with
  `HX-Trigger: {"modal:close":{"target":"body"}}`.
- **Saving in a modal** (found in I05): the form posts with `hx-target="#modal-body"`, so a `422`
  replaces the modal content with the re-rendered form. A successful save or delete answers `200`
  with the refreshed list, `HX-Retarget` naming the list's id and `HX-Reswap: outerHTML`, and one
  `HX-Trigger` with both the success toast and `modal:close`. A Close button in the content calls
  the `modal` component's `close()`.
- **Confirmations** use a second, smaller `<dialog id="confirm">`. Buttons carry `hx-confirm`;
  `app.js` cancels htmx 4's `htmx:confirm` event, shows the question, and calls
  `issueRequest()` on Confirm or `dropRequest()` on Cancel or Escape, instead of
  `window.confirm`. Alpine components that need to ask (the scenario editor's unsaved-changes
  guard) dispatch `confirm:ask` on `document` with `{question, accept}` (found in I06).
- **Login:** a failed login re-renders the form with `422` and the message above the fields
  (empty fields: the messages next to them), like any form error. A logged-in user who opens
  `/login` is redirected to `/`. Logout is a `POST /logout` form in the navigation, not a link. It asks "Are you sure you want to
  log out?" in the confirmation dialog first (`hx-post` with `hx-confirm`); without JavaScript the
  form logs out at once.

### 10.3 Pages and workflows (acceptance checklist)

Every bullet is an acceptance item for its iteration.

**Layout and navigation**
- Logo plus "Tmock" links to `/`. Menu: Services, Clients, Scenarios, Accounts (admins only),
  My Account, Logout (asks for confirmation). Toast region top-right.
- Route guard: anonymous users are redirected to `/login`; `/login` redirects to `/` when already
  logged in; `/accounts` redirects non-admins to `/`.

**Login** (`/login`)
- Email and password; the button is disabled until both are filled; the error message is shown
  inline.

**Clients** (`/`)
- C1 Table with columns: enabled switch | name (opens the edit modal) | "Conditions" (opens the
  builder) | scenario chips "**service** \| name", each with ✕, and a ⊕ that opens the assignment
  modal | Token: "Generate" when there is none, otherwise "Regenerate" and "Revoke".
- C2 The enabled switch saves `enabled`. On error (for example "a client needs a condition before
  it can be enabled") an error toast appears and the switch reverts. On success, a toast "Client
  **x** enabled/disabled".
- C3 "Add Client" modal: name only; the client starts disabled.
- C4 Edit modal: rename (Save) and Delete, with confirmation "Are you sure you want to delete the
  client **x**?".
- C5 A chip's ✕ removes that scenario. Toast "Scenario removed from client".
- C6 Assignment modal: one collapsible `<details>` per service. Each scenario has a switch that
  saves the assignment **immediately** when toggled, and an info icon showing the scenario's
  settings (popover).
- C7 Condition builder (§10.5).
- Found in I07: the table reloads itself when a modal changes a client (the assignment and token
  modals raise `clients:changed`, the table has `hx-trigger="clients:changed from:body"`).
  Regenerate also asks first, because the current token stops working. The token modal does not
  close on a backdrop click, so the token is not lost by accident. Until I08 the condition is
  edited as JSON in the edit modal ("Conditions" opens it at that field).
- C8 Token modal, shown right after Generate or Regenerate: the token in a read-only field, a Copy
  button (clipboard, toast "Token copied to clipboard") and the warning "This token is shown only
  once". Revoke asks for confirmation.

**Services** (`/services`)
- S1 Table: name (opens the edit modal) | upstream URL | path.
- S2 "Add Service" modal: name, upstream URL, path. Errors appear next to the fields.
- S3 Edit modal: name, upstream URL, path; Save; Delete with confirmation (it also deletes the
  service's scenarios, and the dialog says so).

**Scenarios** (`/scenarios`)
- SC1 Scenarios grouped by service in collapsible groups (`<details>`). Columns: Name (opens the
  editor) | Method | Path | Condition | Code | Headers ("Show Headers" with a JSON popover;
  clicking opens the editor scrolled to the headers) | Body ("Show Body" with a popover of the
  first 255 characters; clicking opens the editor scrolled to the body).
- SC2 "Add Scenario" → the editor with empty fields.
- SC3 Editor fields:
  - Name; Service (a select of services by name).
  - **Request Conditions**, each with an enable switch: Method (select), Path, Condition.
  - **Response Modifications**, each with an enable switch: Code (number); Headers (key/value rows,
    add and remove; switching off clears them); Body (a code textarea with a language hint
    JSON/HTML/Plain text, derived from the `Content-Type` header row); Skip Proxy (shown only while
    Body is enabled).
  - A field that is switched off is saved as unset, and so is one that is on but empty.
  - Found in I06: the switches show and hide their fields with CSS (`:has`), so the editor also
    works without JavaScript; the server ignores a field whose switch is off, and Skip Proxy
    while Body is off. A header row with an empty value is saved as `null` (the proxy removes
    that header, §7.8), and empty rows are ignored. A taken name or an unknown service is shown
    next to its field. After a failed save the editor counts as unsaved, so closing it still
    asks.
- SC4 Delete asks for confirmation. Closing with unsaved changes asks "You will lose unsaved
  changes. Are you sure you want to close?". The editor does not close on backdrop click.
- SC5 Errors in the request condition or the template are shown next to the field, with the
  position.

**Accounts** (`/accounts`, admins only)
- A1 Table: name | email | admin (✓/✕) | Edit, Delete.
- A2 Add modal: name, email, password, admin switch. Create is disabled until name, email and
  password are filled.
- A3 Edit modal: name, email, new password ("leave blank to keep"), admin. Only changed fields are
  saved.
- A4 Delete asks for confirmation; errors (own account, last admin) appear as toasts.

**My Account** (`/my-account`)
- M1 Name, email, current password, new password, confirm new password. An inline error appears
  when the new and confirm passwords differ, and Save is disabled. Only changed fields are saved;
  the password fields are cleared after success.

### 10.4 Body editor

A `<textarea>` with a monospace font and no wrapping. Alpine adds: Tab inserts two spaces; a
"Format JSON" button (shown when the language is JSON) pretty-prints the content and shows a parse
error inline if the JSON is invalid. There is no syntax highlighting.

### 10.5 Client condition builder (server-side tree)

- The condition is rendered recursively by Go. Each node is a set of form inputs named by its path
  in the tree, for example `n.0.1.kind=ip`, `n.0.1.value=10.0.0.1`, `n.0.not=on`, `n.0.op=and`.
- Every edit (add an expression of type Group / IP Address / CIDR / Header / Header Regex; remove a
  node; toggle NOT; toggle AND↔OR by clicking the operator chip) is an `hx-post` of the **whole
  form**, with `op` and `path` parameters, to `/ui/clients/{id}/condition/edit`. The handler parses
  the tree, applies the operation, and re-renders the builder. Nothing is saved until **Save**.
- Visuals: groups shown as `NOT ( … )`; items as `[Kind] [header] [Equals|Matches|Not …] [value] [✕]`;
  an AND/OR chip between siblings; an "Add Expression" menu (native `<details>` menu); the
  top-level group cannot be deleted; an empty condition starts as `{"and": []}`.
- The parse → operation → serialise logic lives in a pure Go package and is unit-tested.

### 10.6 Styling

A single hand-written `static/app.css`. No CSS framework, no preprocessor, no build step. It uses
native `<dialog>`, `<details>`, `popover` and `:has()`.

- **Copied from the legacy app.** The look follows the legacy dashboard, and the values come
  from it rather than being invented: the page colours, link colour (`#596dde`), font and the
  light/dark switch on `prefers-color-scheme` from `app/src/assets/base.css`; the page width and
  spacing from `main.css`; the plain header with a 50 px logo and the menu links from
  `MainNavigation.vue`; the indigo of buttons and switches (Vuetify's `indigo`, `#3f51b5`), red
  for destructive actions (`#f44336`), the `v-alert` colours, and the dark tables
  (`v-table theme="dark"`). Where Vuetify sets a value (field height, radius, elevation), we copy
  the value that the legacy page renders. The result should look similar, not identical
  (intent.md §4).
- **Tokens first.** `app.css` starts with CSS custom properties in `:root`: colours, spacing,
  radius, fonts, widths. Component rules use only these tokens, never literal colours or sizes,
  so another style is a change to the token block (or a second block, for example
  `[data-theme="light"]`), not a change to every rule.
- **One section per component.** After the tokens and a small base (reset, `body`, links), the
  file has one section per component in §10.7, headed with its Go file name
  (`/* ── button.go ── */`). A section styles only its own component's classes.
- **CSP is unchanged** (§9.4): the stylesheet is a static file under `style-src 'self'`, and no
  component uses inline `style` attributes or `<style>` elements.

Switches are styled checkboxes. The layout is centred with a maximum width. It MUST remain usable
at laptop widths; phone layout is a nice-to-have.

### 10.7 Components

Every UI element is its own component, so that its look, or the order of elements on a page, can
change later in one place without editing the pages.

- **One component per element.** Each element the pages show is a Go function in
  `internal/views/components`, in its own file: button, link button, icon button, text field,
  select, textarea and code textarea, switch, checkbox, form and form actions, table, chip,
  inline error, popover, collapsible group (`<details>`), menu, card, page header, modal,
  confirmation dialog, toast region, brand, nav and nav item; from I06 also the toggle field
  (an optional field with its enable switch) and the form section (`<fieldset>`); from I07 the
  chip list, button group and copy field. Composites built from these (the
  condition-group row, the header key/value rows) are components too, even when only one page
  uses them.
- **Semantic props, no classes from callers.** A component takes a `…Props` struct with
  what it shows and what state it is in (label, name, value, error, disabled), and a typed
  variant where there is a choice (`ButtonPrimary`, `ButtonDanger`, `ButtonText`). Callers never
  pass class names or styles. Behaviour attributes (`id`, `hx-*`, `x-*`, `aria-*`) go in an
  `Attrs []g.Node` field, which the component puts on its main element.
- **Components own their markup and classes.** Class names are BEM-style and named after the
  component (`button`, `button--danger`, `field__error`). Only `internal/views/components` sets
  `class` attributes. A test parses the other UI packages (`internal/views/layout`,
  `internal/views/pages`, `internal/views/hx`, `internal/web`) and fails if any of them sets a
  `class` attribute (`html.Class`, or `class`, `:class` or `x-bind:class` through `Attr`).
  `static/app.js` sets no classes either: state that CSS needs is a data attribute that the
  component's template binds (a toast's `data-type`).
- **Order is decided by layout components, not by pages.** A page passes its parts as separate
  props (title, actions, toolbar, content), and a layout component places them: `layout.Page`
  for the shell (header, nav, main, toast region, modal host), `components.PageHeader` for title
  and actions, `components.Form` and `components.ModalFooter` for fields and buttons. So moving
  the actions, or putting Delete left of Save everywhere, is one change in one component.
  Reordering changes the DOM order, not CSS `order`, so keyboard and screen-reader order stay
  the same as the visual order.
- **Tested alone.** Each component has a render test for each variant and state. Pages are
  tested for content and behaviour, not for markup details.

---

## 11. Testing strategy

### 11.1 Levels

| Level | Where | What | Runs |
|---|---|---|---|
| Unit | every package | conditions, expr, liquid, password hashing, CIDR/IP, catalog, Optional[T], validation messages, `internal/app` use cases, condition-builder tree ops, view rendering, import boundary (`internal/archtest`), slog → OTLP mapping, batching, retry and the never-logged list (§2.4) | `go test ./...` (always) |
| Handler | `internal/api`, `internal/proxy`, `internal/web` | `httptest` against handlers, with a fake store and a fake upstream (`httptest.Server`) | `go test ./...` (always) |
| DB integration | `internal/db`, `internal/store`, `internal/legacyimport` | real PostgreSQL from `compose.yaml` | when `TMOCK_TEST_DATABASE_URL` is set (`make test-db`); otherwise skipped with a message |
| End-to-end smoke | `app-go/e2e` | the binary + test DB: log in through the UI endpoints, create a service, a scenario and a client, assign them, call the proxy, check the `x-tmock-*` headers; with `OTEL_EXPORTER_OTLP_ENDPOINT` pointing at an in-test fake collector, check that the `tmock.proxy.request` event arrives with the request's `traceparent` trace id | when `TMOCK_TEST_DATABASE_URL` is set |
| Fuzz | `internal/expr`, `internal/liquid`, `internal/condition` | parsers never panic | `go test -fuzz` (manual, per iteration that touches them) |
| Manual | issue checklist | click-through of the §10.3 items for the iteration | per iteration |

### 11.2 Conventions

- Table-driven tests; `t.Parallel()` where safe; no sleeps (use `testing/synctest` for TTL and
  timer logic).
- Golden files live under `testdata/`. Update them with `go test ./... -update`, a flag defined in a
  small test helper.
- Each DB test creates its own schema (`CREATE SCHEMA t_<random>` + `search_path`) and drops it
  afterwards, so the tests can run in parallel against one database. At most 4 DB tests of one
  package hold a schema at a time, because the parallel tests of all packages otherwise exceed
  PostgreSQL's default of 100 connections (found in I07).

### 11.3 Local loop (every iteration)

```bash
cd app-go
make db-up            # PostgreSQL in Docker
make test             # unit + handler tests
make test-db          # DB integration + e2e
make run              # migrate, seed (from .env) and serve on :3000 (API + UI)
make otel-up          # optional: local OpenTelemetry Collector on :4318 that prints received logs
```

---

## 12. Differences from the legacy app

This list is for the migration guide (I17): what users and automation owners will notice. It is not
a compatibility commitment.

| Area | Legacy | Go app |
|---|---|---|
| Accounts | Existing users and passwords | Users are not imported; admins are re-seeded and recreate accounts (§13.1) |
| Client tokens | Stored in plaintext; "View" shows them any time | Stored as SHA-256; shown once on generation. Imported tokens keep working |
| REST responses | `{"message": …, "service": …}` envelopes; `201` on login; `404` for duplicates | The resource itself; conventional codes (§6.2), `409` for duplicates, `204` on delete |
| REST auth | Session + CSRF token from `/api/auth/csrf-token` | Session only; no CSRF token (§9.3) |
| Removed endpoints | `PUT` upserts, `GET /api/client/active`, both `clear-cache` endpoints, `/api/auth/csrf-token` | Gone: cache invalidation is automatic, and upserts are replaced by `GET` + `POST`/`PATCH` |
| Renamed endpoint | `POST /api/client/deactivate/all` | `POST /api/client/disable-all` |
| Request conditions | JavaScript `eval` (and apparently broken) | Safe expression language (§7.4); conditions that don't parse are reported by the import |
| Templates | Output cached for up to 10 minutes by accident | Rendered on every request |
| Cache | Not invalidated after service changes | Invalidated after every change |
| Scenario order | Undefined | By name; the first matching scenario in the list wins (§7.5) |
| Path matching | Compared the whole rest of the URL, query string included: a scenario with path `orders` did not match `/api/proxy/users/orders?page=2` or `…/orders/` | Compares only the path, without the query string and without leading or trailing `/`; case-insensitive in both (§7.5) |
| Headers | `x-tmock-*` missing on skip-proxy responses | Always present |
| `PROXY_REMOVE_FORWARDED_HEADERS` | Any value, even `false`, enabled it | Parsed as a boolean |
| Conditions | IPv4-only CIDR; JS regex syntax | IPv4 and IPv6; RE2 regex syntax |
| Config | `NODE_ENV` | `TMOCK_ENV` |
| Look | Vuetify, dark/indigo | Similar dark/indigo look in plain CSS, with values copied from the legacy app (§10.6); built from separate components (§10.7) |
| Body editor | CodeMirror with highlighting | Plain textarea with "Format JSON" |
| Proxy CORS | `CORS_*` reflection on all routes | Proxy only, with an optional allow-list |
| Logs | Unstructured console lines | Console lines plus named, structured events (§2.4.2); optional OTLP export to an OpenTelemetry collector via the standard `OTEL_*` variables |

---

## 13. Legacy import, packaging and cutover

### 13.1 Legacy import (`tmock import-legacy`)

```
tmock import-legacy --legacy-dsn postgres://user:pass@host/db [--dry-run]
```

- Reads `mp_services`, `mp_scenarios`, `mp_clients` and `mp_clients_scenarios` from the legacy
  database (read-only) and writes them into the Go database in **one transaction**.
- Refuses to run unless the target has no services, scenarios or clients, so it is safe to retry
  after a failed run and cannot duplicate data.
- Mapping:
  - empty strings and `0` response codes become `NULL`; methods are upper-cased; request paths are
    trimmed of `/`; a `skipProxy` scenario without a body is imported with `skipProxy = false`;
  - client tokens are hashed with SHA-256, so existing automation tokens keep working;
  - condition nodes with more than one predicate are split into an `and` group; nodes with none are
    dropped;
  - assignments are carried over by mapping old ids to new ones.
- Anything that cannot be imported as-is is **reported, not fatal**, and the import continues:
  - request conditions that don't parse are imported as `NULL` and listed;
  - templates that don't parse are imported unchanged and listed;
  - header regexes that don't compile as RE2, and invalid IPs or CIDRs, are dropped from the
    condition and listed; the client is imported disabled.
- `--dry-run` prints the report without writing anything.
- Users (`mp_users`) and `mp_options` are **not** imported.

### 13.2 Image and CI

`app-go/Dockerfile`, build context = repository root:

```dockerfile
FROM golang:1.25-trixie AS build
WORKDIR /src/app-go
COPY app-go/ .
RUN CGO_ENABLED=0 go build -trimpath -ldflags="-s -w" -o /out/tmock ./cmd/tmock

FROM gcr.io/distroless/static-debian12:nonroot   # CA certs + tzdata included; no shell
COPY --from=build /out/tmock /tmock
EXPOSE 3000
ENTRYPOINT ["/tmock"]
CMD ["serve"]
```

- Build with: `docker build -f app-go/Dockerfile -t tmock-go .`
- The image runs as non-root.
- Migrations: `docker run … tmock-go migrate` as an init container or job, or set
  `MIGRATE_ON_START=true`.
- The base image tags are illustrative. Pin exact tags (or digests) when implementing.

**CI (optional, needs approval).** A new workflow, `.github/workflows/build-go.yaml`, triggered
manually and on pushes to `master`, runs `go vet`, `go test` (with a PostgreSQL service container)
and pushes `laposa/tmock:go-<yymmdd>.<sha>`. The existing `build.yaml` and its tags are untouched
until the legacy app is deleted.

### 13.3 Cutover

1. Create a new, empty database for the Go app. Deploy the image; run `migrate` and `seed`.
2. Run `tmock import-legacy --dry-run` against the production legacy database, review the report,
   fix anything important in the legacy app, then run it for real.
3. Admins recreate user accounts in the new dashboard.
4. Point services under test and automation at the Go instance. Proxy URLs are unchanged apart
   from the host; automation that calls the REST API is updated per §12.
5. After sign-off, retire the legacy deployment and database.
6. Delete `api/`, `app/`, the root `Dockerfile` and `build.yaml` in a separate PR, and move the Go
   documentation into `README.md`.

---

## 14. Later (explicitly out of scope)

- Request data in templates (`{{ request.body.x }}`).
- Upsert endpoints for scenarios and clients.
- Scenario import/export (JSON files), audit log, metrics endpoint.
- OpenTelemetry traces and metrics (spans for proxy requests with `traceparent` injection upstream;
  request and upstream latency histograms). Logs are exported from I01 (§2.4).
- Syntax highlighting in the body editor.
- Shared cache invalidation between replicas (for example `LISTEN/NOTIFY`).
- API keys for automation that needs more than the client-token endpoints.

---

## 15. Iteration plan

### 15.1 Definition of Done (applies to every iteration)

- [ ] `go build ./...`, `go vet ./...` and `gofmt -l .` (empty output) pass in `app-go/`.
- [ ] `go test ./...` passes. DB tests pass with `make test-db`.
- [ ] No new direct `require` in `go.mod`, and no change to the `internal/archtest` allow-list,
      unless it is recorded in §3.2.
- [ ] `api/`, `app/`, the root `Dockerfile` and `build.yaml` are unchanged.
- [ ] The "run & verify" section of `app-go/README.md` is updated.
- [ ] If views changed: every new UI element is a component in `internal/views/components` with its
      own render test and its own section in `app.css`, and the class-attribute test passes (§10.7).
- [ ] The log events §2.4.2 lists for this iteration are emitted, with a test per event that
      checks its name, level and attributes.
- [ ] The issue's **Verify locally** checklist has been performed, and its results noted in the
      issue.
- [ ] Any finding that contradicts this spec has been fixed in `spec.md`, in the same PR.

### 15.2 Milestones

- **M1 — Dashboard** (end of I08): log in and manage services, scenarios, clients and users in the
  Go UI, with the matching REST API.
- **M2 — Proxy and mocking** (end of I13): forwarding, matching, mocking, request conditions and
  templates work end to end.
- **M3 — Shippable** (end of I17): OpenAPI, legacy import, image, e2e smoke test and cutover guide.

From I04 on, each iteration delivers a feature **vertically**: the use case in `internal/app`, the
REST endpoints and the UI page together, so every iteration ends with something to click through.

### 15.3 Iterations

Each block below is ready to paste as a GitHub issue. The title is the heading.

---

#### I00 — Go scaffolding: the `app-go` module

**Goal:** One empty but runnable Go module with the agreed layout and tooling.

**Scope**
- `app-go/go.mod` (`go 1.25`, requires `maragu.dev/gomponents`; pgx is added in I02).
- `cmd/tmock` serving `GET /api/status` and `GET /status` → `{"status":"ok"}`, a gomponents page
  "Tmock — coming soon" with the layout shell at `/`, and `/static/*` from `go:embed`.
- Vendored `htmx` 4.x (`dist/htmx.min.js` and `dist/ext/hx-alpine-compat.js` from the same
  release) and `alpinejs` CSP 3.x into `app-go/static/vendor/`, with `VERSIONS`
  (version, URL, SHA-256); a `make vendor-js` target that re-downloads and verifies them.
- `internal/archtest` with the import-boundary rule and allow-list (§1.3).
- `app-go/compose.yaml` (PostgreSQL 17, persistent volume, health check, and an
  `otel-collector` service under the `otel` profile: `otel/opentelemetry-collector` with a pinned
  tag, an OTLP/HTTP receiver on `:4318` and the `debug` exporter, configured in
  `app-go/dev/otelcol.yaml`); Makefile (including `otel-up`); README; `.gitignore` entries.

**Verify locally**
- [ ] `make test` passes, including the import-boundary test (temporarily importing gomponents
      from a backend package makes it fail).
- [ ] `curl localhost:3000/api/status` and `curl localhost:3000/status` both return `{"status":"ok"}`.
- [ ] `localhost:3000/` renders the page; the browser console confirms that htmx 4 and Alpine
      loaded, with no CSP violations.
- [ ] `make db-up` starts PostgreSQL, and `psql` connects.

**Depends on:** —

---

#### I01 — Configuration, logging and server lifecycle

**Goal:** Production-grade process plumbing.

**Scope**
- `internal/config`: every variable in §4.2, the `.env` loader, strict boolean parsing,
  validation with clear start-up errors, `SESSION_SECRET` required in production,
  `SERVICES_UPSTREAM_OVERRIDES` parsing, building the DSN.
- `internal/logx`: slog in text or JSON; the fan-out handler, event-name and attribute-key
  constants, `traceparent` parsing and the request-log middleware (§2.4.1–§2.4.4, §9.6); the
  I01 events of §2.4.2.
- `internal/logx/otlp`: the OTLP/HTTP JSON exporter (§2.4.5) and the `OTEL_*` variables (§4.2),
  flushed on shutdown.
- Graceful shutdown on SIGINT/SIGTERM (drain with a timeout); panic recovery → `500` JSON (§6.2).

**Verify locally**
- [ ] Unit tests cover parsing (including bad values) and the override format.
- [ ] Starting with `TMOCK_ENV=production` and no `SESSION_SECRET` exits with a clear error.
- [ ] Health checks appear only at `LOG_LEVEL=debug`.
- [ ] Ctrl-C shuts down cleanly while a slow request completes.
- [ ] Exporter tests against an `httptest` collector: golden JSON for the mapping (every attribute
      type, groups, severities, trace context, resource); batching by size and by delay and queue
      overflow (with `synctest`); retry on `503` and `Retry-After`, no retry on `400`; gzip;
      `partialSuccess`; flush on shutdown.
- [ ] `OTEL_EXPORTER_OTLP_PROTOCOL=grpc` or a malformed `OTEL_EXPORTER_OTLP_HEADERS` stops start-up
      with a clear error.
- [ ] `make otel-up` and `OTEL_EXPORTER_OTLP_ENDPOINT=http://localhost:4318 make run`:
      `curl -H 'traceparent: 00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01' localhost:3000/api/status`
      at `LOG_LEVEL=debug` shows a `tmock.http.request` record with that trace id in the collector
      output. Stopping the collector doesn't slow requests; the console reports the failed exports.

**Depends on:** I00

---

#### I02 — Database, schema, migrations, password hashing and seed

**Goal:** The Go app connects to PostgreSQL, creates its own schema and its first admin.

**Scope**
- `internal/db` (§3.3): pgx through `database/sql`, the pool settings, the SQLSTATE → typed error
  mapping, and a start-up ping with a clear error message when the database is unreachable.
- The migration runner (§5.2) and `0001_init.sql` (§5.1); `tmock migrate`; `MIGRATE_ON_START`.
- `internal/password` (§9.1).
- The users store and `tmock seed` (creates the admin from `ADMIN_*` if there are no users).

**Verify locally**
- [ ] `make test-db` passes against PostgreSQL 17.
- [ ] DB tests cover NULLs, `bytea` and `jsonb` round-trips, unique and foreign-key violations
      mapped to typed errors, and a context timeout cancelling `pg_sleep`.
- [ ] A fresh DB plus `tmock migrate` creates the schema; running it again is a no-op; two
      concurrent runs don't conflict.
- [ ] `DATABASE_ENABLE_SSL=true` against a non-TLS server fails with a clear error.
- [ ] Password tests: round-trip, wrong password, a tampered hash, re-hashing on a lower iteration
      count.
- [ ] `tmock seed` creates the admin once; a second run does nothing.

**Depends on:** I01

---

#### I03 — HTTP foundation

**Goal:** Shared plumbing that every endpoint and page will use.

**Scope**
- `ServeMux` routing; JSON decode (10 MB limit, `413`, malformed JSON and unknown fields → `400`)
  and encode; the error type and the §6.2 response format; `404` for unknown routes.
- `httpx.Optional[T]` and a validation helper that produces `details` keyed by field path.
- `http.CrossOriginProtection` (§9.3), security headers (§9.4), proxy CORS middleware (§9.5,
  wired in I10).
- The `internal/app` skeleton and the convention for typed errors shared by API and UI.

**Verify locally**
- [ ] Handler tests cover the error shapes, validation output, `Optional[T]` (absent / null /
      value), the headers, and a cross-origin browser `POST` being rejected while curl passes.

**Depends on:** I01 (and I02 for wiring the server)

---

#### I04 — Login, sessions and the UI shell

**Goal:** Log in to the Go dashboard and see the empty shell.

**Scope**
- DB-backed sessions (§9.2); the `user` and `admin` guards (§6.3).
- `POST /api/auth/login`, `POST /api/auth/logout`, `GET /api/auth/me`.
- UI: `internal/views/components` with the components these pages need (§10.7). The existing
  shell markup (header, brand, nav) and the placeholder page move into components, and their
  `app.css` rules into per-component sections. Also the design tokens copied from the legacy app
  (§10.6), the class-attribute test, layout and navigation, the Alpine toast store and the
  `HX-Trigger` convention, the `<dialog>` modal host and confirmation dialog, the `hx-*` helpers,
  the login page, logout, and the route guards (§10.3 Layout and Login).

**Verify locally**
- [ ] Handler and DB tests cover login (including a wrong password and an unknown email), session
      expiry (with `synctest`), logout, and a deleted user's session stopping working.
- [ ] Render tests cover the navigation for admins and non-admins.
- [ ] `make run`: log in with the seeded admin, reload (still logged in), log out; a wrong password
      shows the error. `curl -c` / `-b` login works against the API.
- [ ] The browser console shows no CSP violations.
- [ ] Each new component has a render test; no package outside `internal/views/components` sets a
      `class` attribute. The pages have the legacy dark/indigo look (§10.6).

**Depends on:** I02, I03

---

#### I05 — Services: API and page

**Scope:** the services use cases, the §6.4 service endpoints, and §10.3 S1–S3.

**Verify locally**
- [ ] Use-case, handler and render tests, including duplicate path (`409`), a path with `/`, and
      a bad URL.
- [ ] Manual: list, add, edit (including renaming the path), delete with confirmation, and the
      field errors.
- [ ] `curl` CRUD against `/api/service` works with a session cookie.

**Depends on:** I04

---

#### I06 — Scenarios: API, list and editor

**Scope:** the scenario use cases, the §6.4 scenario endpoints, §10.3 SC1–SC5 and §10.4. Request
conditions and templates are stored as text; their syntax checks arrive in I11 and I12.

**Verify locally**
- [ ] Use-case, handler and render tests (unset vs value for every toggled field; `skipProxy`
      requires a body; duplicate names within a service).
- [ ] Manual: create a scenario with every field; switch fields off, which clears them; the
      unsaved-changes prompt appears; deletion asks for confirmation; "Format JSON" works;
      clicking "Show Body" scrolls the editor to the body.

**Depends on:** I05

---

#### I07 — Clients: API, page, assignments and tokens

**Scope:** the client use cases and the §6.4 client endpoints, including `disable-all`, hashed
tokens and the `token-or-user` guard; §10.3 C1–C6 and C8. Conditions can be edited as raw JSON in
the edit modal until I08 adds the builder.

**Verify locally**
- [ ] Tests cover the enable rule, nested condition validation with field paths, unknown scenario
      ids, and the token cases: wrong token → `401`, other client → `403`, disallowed field →
      `403`, valid token → `200` without a session.
- [ ] Manual: toggle (enabling without a condition shows the error and reverts), add, rename,
      delete, chips, the assignment modal (each toggle persists; a reload confirms it), generate,
      copy, regenerate and revoke a token.
- [ ] `curl -X PATCH localhost:3000/api/client/1 -H "X-Client-Token: …" -d '{"enabled":true,"scenarios":[1]}'` works.
- [ ] The DB contains no plaintext tokens.

**Depends on:** I06

---

#### I08 — Client condition builder, Accounts and My Account (milestone M1)

**Scope:** §10.5 and §10.3 C7; the user use cases and the §6.4 user endpoints; §10.3 A1–A4 and M1.

**Verify locally**
- [ ] Unit tests on the tree operations (add of each kind, remove, toggle NOT, AND↔OR, and parsing
      from and serialising to condition JSON, round-trip on random trees).
- [ ] Tests cover deleting your own account, deleting or demoting the last admin, a wrong current
      password, and a duplicate email (case-insensitive).
- [ ] Manual: build `NOT (ip = 10.0.0.1 OR (header X-Env = qa AND cidr 192.168.0.0/16))`, save,
      reopen, and get the same tree. Every admin flow; the password mismatch blocks Save; revoking
      a user's admin flag takes effect on their next request.
- [ ] **M1 check:** a full pass of the §10.3 checklist, with results recorded in the issue.

**Depends on:** I07

---

#### I09 — Proxy core and catalog cache

**Goal:** `/api/proxy/*` forwards requests when no scenario applies.

**Scope**
- Routing and service lookup (§7.1); the catalog snapshot, TTL, single-flight rebuild,
  invalidation from `internal/app`, and `DISABLE_CACHE` (§7.2).
- Upstream forwarding (§7.7) and passthrough responses (§7.8, no-scenario branch), streaming, `502`
  on failure, and the log line and events (§7.9, §2.4.2).
- `cmd/echo-upstream`: a dev server that echoes the method, path, headers and body as JSON,
  optionally gzip-compressed, with configurable delay and status.

**Verify locally**
- [ ] Handler tests with an `httptest` upstream cover path joining (with and without a base path),
      query preservation, headers (`Apikey`, forwarded-header removal), a large streamed body, a
      gzip passthrough, an unknown service → `404`, and upstream down → `502`.
- [ ] Cache tests use `testing/synctest` for the TTL; a mutation is visible on the next request.
- [ ] Manual: `go run ./cmd/echo-upstream`, create a service pointing at it in the UI, and
      `curl localhost:3000/api/proxy/<svc>/a/b?x=1` shows `/a/b?x=1` upstream.

**Depends on:** I08 (only needs I05–I07; can start in parallel with I08)

---

#### I10 — Client matching, scenario selection and response mocking

**Goal:** The core mocking behaviour.

**Scope**
- Condition evaluation and client IP (§7.3); scenario selection (§7.5); skip-proxy (§7.6);
  response modification (§7.8); proxy CORS (§9.5); the `mocked:` log details. Templates are not
  yet rendered; the body is used verbatim.

**Verify locally**
- [ ] Table-driven tests cover every predicate, `not`, empty `and`/`or`, header joining,
      IPv4-mapped addresses and IPv6 CIDRs.
- [ ] Selection tests cover the order, the method and path filters, case-insensitivity and the
      reported client.
- [ ] Manual with `echo-upstream` and the UI: a client with condition `ip = 127.0.0.1` and a
      500-body scenario → mocked; with skip-proxy, the upstream sees no request; the `x-tmock-*`
      headers are present; a browser preflight from another origin gets `204`.

**Depends on:** I09

---

#### I11 — Safe request-condition expressions

**Goal:** Request conditions without `eval`.

**Scope**
- `internal/expr` (§7.4), the `$request` namespace, request-body buffering when needed (§7.7), and
  its use in selection.
- Syntax check on scenario save, with the error position shown in the editor (SC5).

**Verify locally**
- [ ] Parser and evaluator tests, including `$request.body.userId == "4a30d84d-…"` (the legacy seed
      example), missing paths, `!`, parentheses, number vs string comparison, and error positions.
- [ ] Fuzz test on the parser, with no panics.
- [ ] Manual: a POST with a JSON body matches or doesn't match as expected; an invalid condition
      shows the error in the editor.

**Depends on:** I10

---

#### I12 — Liquid template subset

**Goal:** Dynamic mock bodies.

**Scope**
- `internal/liquid` (§8.2), with activation, save-time syntax check and runtime fallback (§8.1),
  and per-request rendering.
- `import _ "time/tzdata"`, and a test with `TZ=Europe/Dublin`.

**Verify locally**
- [ ] The `cases.txt` tests pass; both §8.3 examples work.
- [ ] A fuzz test on the parser runs with no panics.
- [ ] Manual: a skip-proxy scenario with the `validFrom` template returns today + 5 days, and the
      value changes immediately when the scenario is edited.

**Depends on:** I10 (can run in parallel with I11)

---

#### I13 — Proxy hardening (milestone M2)

**Goal:** Confidence that the proxy behaves well under realistic use.

**Scope**
- End-to-end proxy tests that combine clients, conditions, expressions and templates.
- Timeouts on the upstream transport (configurable via `PROXY_UPSTREAM_TIMEOUT`, default `30s`);
  limits on header and body sizes; a benchmark of the no-scenario path.
- Any proxy bug found during I09–I12 that was deferred.

**Verify locally**
- [ ] The benchmark shows no allocation-heavy hot spots on the passthrough path (results in the
      issue).
- [ ] **M2 check:** with `echo-upstream`, every proxy row in §12 behaves as described.

**Depends on:** I11, I12

---

#### I14 — OpenAPI and swagger

**Scope:** `openapi/openapi.json` covering §6.4; `/swagger` and `/swagger/openapi.json`; the
route-coverage test (§6.5).

**Verify locally**
- [ ] `/swagger` renders and lists every endpoint; "Try it out" works with a session cookie.
- [ ] The route-coverage test fails when a route is added without documentation.

**Depends on:** I08

---

#### I15 — Legacy import

**Scope:** `tmock import-legacy` (§13.1) and `internal/legacyimport`.

**Verify locally**
- [ ] DB tests load `api/database/test-seed.sql` (with its table prefix adapted to `mp_`) and a
      hand-made legacy fixture with every awkward case (empty strings, `0` codes, multi-predicate
      conditions, bad regexes, tokens), then import and check the result and the report.
- [ ] `--dry-run` writes nothing; a second run refuses because the target is not empty.
- [ ] Manual: import a copy of a real legacy database, check a few scenarios in the UI, and call
      the client-token `PATCH` with a token from the legacy database.

**Depends on:** I13

---

#### I16 — Hardening and polish

**Scope:** accessibility pass (labels, focus handling in dialogs, keyboard use of switches and the
condition builder); empty states and loading indicators; a review of every error message shown in
the UI; README "run & verify" and configuration reference.

**Verify locally**
- [ ] Every §10.3 page is usable with the keyboard only.
- [ ] The browser console shows no errors or CSP violations on any page.

**Depends on:** I08

---

#### I17 — Packaging, e2e smoke test and cutover guide (milestone M3)

**Goal:** A deployable image and a documented cutover path.

**Scope**
- `app-go/Dockerfile` (§13.2); the `app-go/e2e` smoke test (§11.1); an optional
  `.github/workflows/build-go.yaml` (§13.2, needs approval).
- Deployment and migration guide in `app-go/README.md`: environment variables, migrations, the
  cutover procedure (§13.3), the multi-replica cache note (§7.2), log export to an
  OpenTelemetry collector (§2.4, with an example collector config), and the user-facing changes
  (§12).

**Verify locally**
- [ ] `docker build -f app-go/Dockerfile -t tmock-go .` builds; the image size is noted in the
      issue.
- [ ] `docker run` with the compose DB: migrate, seed, import, log in, and proxy a request.
- [ ] `TZ=Europe/Dublin` affects template dates inside the container.
- [ ] The e2e smoke test passes.

**Depends on:** I14, I15, I16

---

## 16. Iteration log

Update this table as issues are created and closed.

| ID | Title | Issue | Status | Notes |
|---|---|---|---|---|
| I00 | Go scaffolding | #31 | In review | htmx 4.0.2, Alpine CSP 3.17.5, gomponents v1.4.0 |
| I01 | Configuration, logging, lifecycle | #34 | Done | own OTLP exporter; `internal/clientip` added |
| I02 | Database, migrations, passwords, seed | #36 | Done | pgx v5.11.0; `internal/db/dbtest` helper; PostgreSQL 17 only |
| I03 | HTTP foundation | #40 | In review | JSON 405 with `Allow`; `app.Error` kinds; Optional field name caveat on Go 1.27 |
| I04 | Login, sessions, UI shell | #43 | Done | `internal/views/hx` instead of helpers in `web`; toasts need `"target":"body"`; placeholder pages until I05–I08 |
| I05 | Services | #45 | In review | path `.`/`..` rejected; modal saves answer with the list via `HX-Retarget`; `tmock.service` on the entity event; table, link button, modal content/footer components |
| I06 | Scenarios | #47 | In review | empty values unset, headers checked as HTTP tokens; `app.Error.Field` puts conflicts next to their field; `confirm:ask` for the unsaved-changes guard; switches toggle fields with CSS `:has`; select, switch, toggle field, form section, collapsible, popover, header rows and code textarea components; a test that every component has an `app.css` section |
| I07 | Clients, assignments, tokens | #49 | In review | `internal/condition` (model, parse, validate, empty rule; evaluation in I10); tokens are SHA-256 only; `tmock.client`, `tmock.client.id`, `tmock.clients.disabled`; the table reloads on `clients:changed`; Regenerate asks; chip, chip list, icon button, button group and copy field components; `dbtest` caps parallel DB tests per package |
| I08 | Condition builder, accounts (M1) | | Planned | |
| I09 | Proxy core and catalog cache | | Planned | |
| I10 | Matching and mocking | | Planned | |
| I11 | Request-condition expressions | | Planned | |
| I12 | Liquid template subset | | Planned | |
| I13 | Proxy hardening (M2) | | Planned | |
| I14 | OpenAPI and swagger | | Planned | |
| I15 | Legacy import | | Planned | |
| I16 | Hardening and polish | | Planned | |
| I17 | Packaging and cutover (M3) | | Planned | |
