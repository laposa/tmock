# T-Mock Go Rewrite — Intent

This document explains **why** T-Mock is being rewritten, **what we want to achieve**, and the
**principles** that guide every decision. The detailed behaviour, architecture and iteration plan
live in [spec.md](spec.md). When the two documents disagree, fix the disagreement — `intent.md`
states the goals, `spec.md` must serve them.

---

## 1. What T-Mock is

T-Mock is an HTTP mock & proxy for testing. Requests sent to `/api/proxy/<service>/...` are either
forwarded to the real upstream service or answered (fully or partially) from a **scenario** — a
predefined status code, headers and/or body. Which scenario applies is decided by **clients**: a
client is a named, switchable set of scenarios plus a condition (IP, CIDR, header, header regex,
nested AND/OR/NOT) that decides which incoming requests belong to it. A web dashboard manages
services, scenarios, clients and users; automated tests flip scenarios on and off through a REST
API using per-client access tokens.

## 2. Why rewrite

| Problem today | What the rewrite gives us |
|---|---|
| Two large npm dependency trees (NestJS + Vue/Vuetify/Vite/CodeMirror, ~60 direct packages) that need constant upgrades and security patching. | A Go codebase with two dependencies: the **pgx** PostgreSQL driver for the backend and **Gomponents** for the UI (plus vendored HTMX/AlpineJS files). Everything else is the standard library. |
| A build pipeline with two Node builds and runtime patching of the compiled JS bundle (`${API_ENDPOINT}` injection). | A single static binary, built with `go build`, embedding all assets. |
| Request conditions on scenarios are evaluated with JavaScript `eval()`. Any logged-in user can run arbitrary code on the server. | A small, safe, purpose-built expression language. |
| Client tokens are stored in plaintext. Sessions live in process memory and are lost on every restart. | Hashed tokens, passwords hashed with stdlib crypto, sessions in PostgreSQL. |
| Several latent bugs (template output cached by accident, cache not invalidated after service changes, a boolean setting that `"false"` turns on, undefined scenario order, …). | A clean implementation that doesn't carry them over (spec §12). |
| A client-heavy SPA for what is essentially a set of CRUD tables and forms. | Server-rendered HTML with HTMX: less code, no client build, and the server holds the state. |

## 3. Goals

1. **Feature parity, not behaviour parity.** Everything users can *do* today, they can do in the
   Go app: proxy and mock, manage services, scenarios, clients and users in the dashboard, and
   toggle clients from automation. How exactly it behaves is designed fresh; the legacy code is a
   feature reference, not a specification.
2. **A standalone application.** The Go app has its own database schema, its own REST API
   conventions and its own UI. It does not share a database with the legacy app, and the legacy
   Vue app does not have to work against it. Compatibility is kept only where it is free (URL
   paths, JSON field names, the template marker, environment variable names).
3. **Keep the users' data.** A one-time `import-legacy` command brings services, scenarios and
   clients (including working client tokens) over from the legacy database at cutover. User
   accounts are recreated.
4. **Minimal dependencies.**
   - One Go module, `app-go`, with two direct requirements: `maragu.dev/gomponents` for the UI and
     `github.com/jackc/pgx/v5` as the PostgreSQL driver.
   - Only the UI packages may import gomponents. The backend (API, proxy, database, auth) uses the
     Go standard library only, except for the PostgreSQL driver (`pgx`), which only the database
     package may import. A test enforces both rules.
   - HTMX 4 and AlpineJS are vendored JavaScript files embedded in the binary, not Go dependencies.
   - Any further dependency needs an explicit, recorded decision (spec §3.2). pgx is the one
     approved exception: the standard library has no PostgreSQL driver, and writing one in-house
     would be a large, risky project that adds nothing to the product.
5. **Small, safe iterations.** The rewrite is done in small steps. Each one is logged as a GitHub
   issue, and each one **builds, runs and can be tested locally** on its own. From the first
   feature onwards, each iteration delivers a slice you can click through: use case, REST API and
   page together.
6. **The legacy app stays untouched until it is deleted.** `api/`, `app/`, the root `Dockerfile`
   and the existing CI workflow are not modified. All Go code, backend and frontend, lives in one
   new top-level folder, `app-go/`. Deleting the legacy code is the last step after cutover.
7. **Simple operations.** One container image, one process, one port, one PostgreSQL database.
8. **Observable.** What the app does (requests, proxy decisions, logins, data changes, failures)
   is logged as named, structured events through `log/slog`. When an OpenTelemetry collector is
   configured with the standard `OTEL_*` variables, the events are exported to it over OTLP, so
   tmock's logs sit next to the logs and traces of the system under test. A collector is optional;
   without one, the console output is complete on its own. See §7.

## 4. Non-goals

- New product features. Ideas go to spec §14 ("Later").
- Compatibility with the legacy app beyond what is free: no shared database, no identical error
  messages or status codes, no support for the Vue app.
- A pixel-perfect copy of the Vuetify UI. The new UI keeps the same pages, the same workflows and a
  similar dark/indigo look, but it is built with plain CSS.
- Full LiquidJS compatibility. We implement the subset that mock bodies need (spec §8).
- Full JavaScript-expression compatibility for request conditions. We support a comparison and
  boolean subset (spec §7.4); the legacy import reports conditions that don't fit.
- Changing the deployment model (Kubernetes manifests, Docker Hub repository). We supply a new image
  and a new optional workflow. Switching production over is the team's call.

## 5. Guiding principles

1. **Simplest correct design wins.** When the legacy app does something odd, we don't copy it. When
   keeping a legacy convention costs nothing, we keep it so people have less to relearn. When it
   costs effort, we drop it and list the difference (spec §12).
2. **One set of rules, two front doors.** Business rules live in one use-case layer
   (`internal/app`). The REST API (for test automation) and the server-rendered UI are both thin
   layers over it, so the UI can never allow something the API forbids, or the other way round.
3. **Standard library first, own code second, dependency last.** Before adding a dependency, check
   the standard library. If it isn't there, write a small, well-tested package. A dependency is
   added only when that package would be too large or too risky, and only after an explicit
   decision.
4. **Server owns state; the browser owns ephemera.** HTMX handles everything that reads or writes
   data. AlpineJS is used only for state that lives and dies in the browser (see §6).
5. **Boring Go.** `net/http`, `database/sql`, `log/slog`, `embed`, plain structs, explicit SQL, and
   small packages with clear names. No frameworks, no reflection-heavy magic, no code generation.
6. **Every iteration is shippable to a laptop.** Each one has a working `go build`, `go vet`,
   `go test`, a documented "how to verify" checklist, and a running binary to click through.

## 6. Decision: AlpineJS — yes, in a narrow role

AlpineJS is **viable and worth including**, under strict rules. HTMX alone covers about 80% of the
UI: tables, toggles, CRUD forms, modals loaded from the server, and confirmations. Alpine pays for
itself in the few places where round-tripping to the server for purely visual state would be
clumsy:

| Use | Why Alpine and not HTMX |
|---|---|
| Scenario editor: the switches that enable or disable an optional field (method, path, condition, code, headers, body), adding and removing response-header rows, the "Skip proxy" switch that only appears when a body is set. | This is pure form state, with no server data involved until Save. |
| Unsaved-changes guard when closing the scenario editor. | Needs a snapshot of the form compared on close: a few lines in Alpine. |
| Toast notifications fed by the `HX-Trigger` response header. | A small client-side store with auto-dismiss timers. |
| Opening and closing the native `<dialog>` modals after HTMX swaps their content. | Glue code; Alpine keeps it declarative and CSP-safe. |
| Copying a client token to the clipboard. | Browser-only API. |

Rules for Alpine use:

- Use the **CSP build** of Alpine (`@alpinejs/csp`). All components are registered with
  `Alpine.data()` in a single embedded `app.js`, so there are no inline expressions, no
  `unsafe-eval`, and the Content-Security-Policy stays strict.
- Alpine never calls `fetch` and never holds business data. If something must persist, it goes
  through an HTMX request.
- The client-condition builder (a recursive AND/OR tree) is rendered **on the server** with HTMX.
  Recursion is natural in Go and awkward in Alpine, and keeping it on the server lets the logic be
  unit-tested in Go.
- Alpine is optional at runtime: with JavaScript disabled, the pages still render and the forms
  still submit. Only the conveniences listed above are lost.

## 7. Decision: OpenTelemetry for log events — yes, without the SDK

Teams running tmock already collect telemetry with OpenTelemetry, and when a test fails they want
to see what tmock did (which client matched, which scenario mocked the response) next to the logs
of the service under test. We therefore export tmock's log events as OpenTelemetry log records.

How we do it follows principle 3 (standard library first, own code second, dependency last):

- `log/slog` stays the only logging API in the code. Every log line is a slog record; notable ones
  are **events** with a stable name and attributes that follow the OpenTelemetry semantic
  conventions (spec §2.4.2).
- Export is a small exporter of our own, OTLP over HTTP with JSON, built on `net/http` and
  `encoding/json` (spec §2.4.5). The OpenTelemetry Go SDK would bring about ten modules plus gRPC
  and protobuf for one feature, and its logs SDK is still pre-1.0. It is listed as considered and
  rejected in spec §3.2.
- Configuration uses the standard `OTEL_*` environment variables, so existing collector set-ups and
  the OpenTelemetry Operator work as they are. Export is off until an endpoint is set.
- An incoming W3C `traceparent` is attached to the records of that request, so tmock's events can
  be found from a trace of the system under test. tmock does not create spans itself. Traces and
  metrics are "Later" (spec §14).
- Telemetry never gets in the way of testing: logging never blocks or fails a request, and secrets
  and bodies are never logged (spec §2.4.3).

## 8. Success criteria

The rewrite is done when all of the following hold:

1. The Go UI covers every dashboard workflow listed in spec §10.3.
2. The proxy, matching, request conditions and templates behave as described in spec §7–§8, shown
   by the test suite and the M2 manual check.
3. `tmock import-legacy` imports a copy of the production legacy database, and its report has been
   reviewed.
4. A single image built from `app-go/Dockerfile` runs in a staging environment, test automation
   runs green against it, and the team signs off.
5. With `OTEL_EXPORTER_OTLP_ENDPOINT` set in staging, the spec §2.4.2 events arrive in the
   team's log backend, and a proxy request can be found by the `traceparent` of the test that sent
   it.
6. `app-go/go.mod` directly requires only `maragu.dev/gomponents` and `github.com/jackc/pgx/v5`;
   only the UI packages import gomponents and only `internal/db` imports pgx, apart from any
   further exception recorded in spec §3.2.

## 9. Risks and how we handle them

| Risk | Mitigation |
|---|---|
| Automation scripts depend on legacy REST details (response envelopes, status codes, removed endpoints). | Paths and field names are kept; every change is listed in spec §12 and in the migration guide (I17). The client-token toggle, the main automation use, works with the same request. |
| Existing request conditions use JavaScript features beyond the new expression language. | The import reports every condition that doesn't parse. The legacy `eval` path appears to throw on every call, so real-world use is probably close to zero. |
| Liquid edge cases differ from LiquidJS. | Template syntax is checked on save and at import; hand-checked test cases cover the filters people use. |
| Users must recreate their accounts. | There are only a few dashboard users; admins recreate them at cutover (spec §13.3). |
| Several replicas have separate caches. | Every change invalidates the local cache immediately; other replicas catch up within `CACHE_TTL`, which should be short when running more than one replica (spec §7.2). |
| The in-house OTLP exporter drifts from what collectors expect. | It writes only the stable OTLP/HTTP JSON fields, is tested against golden JSON and a real collector (spec I01), and sits behind slog in one package, so it can be swapped for the SDK through a §3.2 decision without touching any caller. |
| A dependency (pgx) brings its own updates and transitive modules. | It is confined to `internal/db` behind `database/sql`, so it can be upgraded or replaced without touching the rest of the code. Upgrades are reviewed like any other change (spec §3.2). |
