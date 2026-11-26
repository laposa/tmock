// Package logx sets up logging (spec §2.4): the console and fan-out handlers, the names of
// log events and attributes, W3C trace context, and the request-log middleware.
//
// Event names and attribute keys are constants here; other packages MUST use them instead
// of string literals, so dashboards and alerts can rely on them (spec §2.4.2).
package logx

// Event names (the event.name attribute), from the table in spec §2.4.2.
const (
	EventServerStarted      = "tmock.server.started"
	EventServerStopped      = "tmock.server.stopped"
	EventHTTPRequest        = "tmock.http.request"
	EventHTTPPanic          = "tmock.http.panic"
	EventMigrationApplied   = "tmock.db.migration.applied"
	EventUserSeeded         = "tmock.user.seeded"
	EventLoginSucceeded     = "tmock.auth.login.succeeded"
	EventLoginFailed        = "tmock.auth.login.failed"
	EventLogout             = "tmock.auth.logout"
	EventSessionsSwept      = "tmock.session.swept"
	EventEntityChanged      = "tmock.entity.changed"
	EventCatalogRebuilt     = "tmock.catalog.rebuilt"
	EventCatalogFailed      = "tmock.catalog.rebuild_failed"
	EventProxyRequest       = "tmock.proxy.request"
	EventUpstreamFailed     = "tmock.proxy.upstream_failed"
	EventTemplateFailed     = "tmock.template.render_failed"
	EventImportIssue        = "tmock.import.issue"
	EventImportFinished     = "tmock.import.finished"
	EventOTLPExportFailed   = "tmock.otlp.export_failed" // console only (spec §2.4.5)
	EventOTLPDropped        = "tmock.otlp.dropped"       // console only (spec §2.4.5)
	EventOTLPPartialSuccess = "tmock.otlp.partial_success"
)

// Attribute keys. OpenTelemetry semantic-convention names where one exists, tmock.* otherwise.
const (
	AttrEventName = "event.name"

	AttrHTTPMethod     = "http.request.method"
	AttrHTTPRoute      = "http.route"
	AttrHTTPStatusCode = "http.response.status_code"
	AttrHTTPBodySize   = "http.response.body.size"
	AttrURLPath        = "url.path"
	AttrURLQuery       = "url.query"
	AttrClientAddress  = "client.address"
	AttrServerAddress  = "server.address"
	AttrServerPort     = "server.port"
	AttrUserID         = "user.id"

	AttrErrorType           = "error.type"
	AttrExceptionType       = "exception.type"
	AttrExceptionMessage    = "exception.message"
	AttrExceptionStacktrace = "exception.stacktrace"

	AttrDurationMS     = "tmock.duration_ms"
	AttrEnv            = "tmock.env"
	AttrShutdownForced = "tmock.shutdown.forced"

	AttrSessionsDeleted = "tmock.sessions.deleted"

	AttrEntityType   = "tmock.entity.type"
	AttrEntityID     = "tmock.entity.id"
	AttrEntityAction = "tmock.entity.action"
	AttrActor        = "tmock.actor"
	AttrService      = "tmock.service"  // a service's path
	AttrScenario     = "tmock.scenario" // a scenario's name

	AttrMigrationVersion = "tmock.migration.version"
	AttrMigrationName    = "tmock.migration.name"

	AttrOTLPRecords         = "tmock.otlp.records"
	AttrOTLPEndpoint        = "tmock.otlp.endpoint"
	AttrOTLPDroppedQueue    = "tmock.otlp.dropped.queue_full"
	AttrOTLPDroppedFailed   = "tmock.otlp.dropped.export_failed"
	AttrOTLPRejectedRecords = "tmock.otlp.rejected_records"
)

// Values of the tmock.entity.* and tmock.actor attributes of tmock.entity.changed
// (spec §2.4.2).
const (
	EntityService  = "service"
	EntityScenario = "scenario"

	ActionCreate = "create"
	ActionUpdate = "update"
	ActionDelete = "delete"

	ActorUser = "user"
)
