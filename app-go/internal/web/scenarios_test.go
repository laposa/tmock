package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// scenarioUI is a handler with a session and the services "svc" (named "My Svc") and
// "other".
func scenarioUI(t *testing.T) (http.Handler, *http.Cookie) {
	t.Helper()
	h := newDBHandler(t)
	c := uiSession(t, h)
	for _, f := range []url.Values{serviceForm("svc", "My Svc", "http://up.test"), serviceForm("other", "Other", "http://up.test")} {
		if rec := send(h, "POST", "/ui/services", f, c, true); rec.Code != 200 {
			t.Fatalf("create service: %d %s", rec.Code, rec.Body.String())
		}
	}
	return h, c
}

// fullScenario is the editor form with every field switched on.
func fullScenario(name string) url.Values {
	return url.Values{
		"name": {name}, "service": {"svc"},
		"requestMethodEnabled": {"on"}, "requestMethod": {"POST"},
		"requestPathEnabled": {"on"}, "requestPath": {"/orders/42/"},
		"requestConditionEnabled": {"on"}, "requestCondition": {"$request.query.page == '2'"},
		"responseCodeEnabled": {"on"}, "responseCode": {"503"},
		"responseHeadersEnabled": {"on"}, "headerName": {"Content-Type", "ETag", ""}, "headerValue": {"application/json", "", ""},
		"responseBodyEnabled": {"on"}, "responseBody": {"{\r\n  \"down\": true\r\n}"},
		"skipProxy": {"on"},
	}
}

var scenarioIDRe = regexp.MustCompile(`hx-get="/ui/scenarios/(\d+)/edit"`)

// SC1–SC4: list, add with every field, edit, switch fields off, delete.
func TestScenariosPage(t *testing.T) {
	t.Parallel()
	h, c := scenarioUI(t)

	rec := serve(h, "GET", "/scenarios", c, false)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("page: %d", rec.Code)
	}
	wantContains(t, "empty page", body,
		"<title>Scenarios · Tmock</title>",
		`aria-current="page">Scenarios</a>`,
		`hx-get="/ui/scenarios/new" hx-target="#modal-body">Add Scenario</button>`,
		`<div id="scenarios-list">`,
		"No scenarios yet.")
	assertNoEval(t, body)

	// SC2: the editor with empty fields; it does not close on a click on the backdrop.
	rec = serve(h, "GET", "/ui/scenarios/new", c, true)
	body = rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<html") {
		t.Fatalf("add modal: %d %s", rec.Code, body)
	}
	wantContains(t, "add modal", body,
		`<div class="modal-content" data-persistent><h2 class="modal-content__title" id="modal-title">Add Scenario</h2>`,
		`hx-post="/ui/scenarios" hx-target="#modal-body" x-data="scenarioEditor" x-on:input="update" x-on:change="changed"`,
		`<option value="" selected>Select a service</option><option value="svc">My Svc</option><option value="other">Other</option>`,
		`<legend class="form-section__title">Request Conditions</legend>`,
		`<legend class="form-section__title">Response Modifications</legend>`,
		`name="requestMethodEnabled">`, `name="requestPathEnabled">`, `name="requestConditionEnabled">`,
		`name="responseCodeEnabled">`, `name="responseHeadersEnabled">`, `name="responseBodyEnabled">`,
		`<option value="OPTIONS">OPTIONS</option>`,
		`id="scenario-headers"`, `id="scenario-body"`, `data-language="Plain text"`,
		`name="skipProxy"`, `>Create</button>`, `x-on:click="close">Close</button>`)
	if strings.Contains(body, "Delete") || strings.Contains(body, "checked") || strings.Contains(body, "data-unsaved") {
		t.Error("the add modal offers Delete, has a switch on, or starts unsaved")
	}
	assertNoEval(t, body)

	// Create with every field.
	rec = send(h, "POST", "/ui/scenarios", fullScenario("Down <1>"), c, true)
	body = rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#scenarios-list" || rec.Header().Get("HX-Reswap") != "outerHTML" {
		t.Fatalf("create: %d %v %s", rec.Code, rec.Header(), body)
	}
	tr := triggers(t, rec)
	if tr["toast"]["type"] != "success" || tr["toast"]["title"] != `Scenario "Down <1>" created` || tr["modal:close"]["target"] != "body" {
		t.Errorf("create triggers: %v", tr)
	}
	m := scenarioIDRe.FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no scenario link in %s", body)
	}
	id := m[1]
	edit := "/ui/scenarios/" + id + "/edit"
	// SC1: grouped by service; the row's columns; previews that open the editor scrolled.
	wantContains(t, "list after create", body,
		`<div id="scenarios-list"><details class="collapsible" open><summary class="collapsible__summary"><span class="collapsible__title">My Svc</span><span class="collapsible__meta">svc</span>`,
		`<th class="table__head" scope="col">Name</th><th class="table__head" scope="col">Method</th><th class="table__head" scope="col">Path</th>`+
			`<th class="table__head" scope="col">Condition</th><th class="table__head" scope="col">Code</th><th class="table__head" scope="col">Headers</th><th class="table__head" scope="col">Body</th>`,
		`hx-get="`+edit+`" hx-target="#modal-body">Down &lt;1&gt;</button>`,
		`<td class="table__cell">POST</td><td class="table__cell">orders/42</td><td class="table__cell">$request.query.page == &#39;2&#39;</td><td class="table__cell">503</td>`,
		`hx-get="`+edit+`?focus=headers" hx-target="#modal-body" aria-describedby="scenario-`+id+`-headers">Show Headers</button>`,
		`<div class="popover__panel" id="scenario-`+id+`-headers" popover="manual" role="tooltip"><pre class="popover__text popover__text--pre">{
  &#34;Content-Type&#34;: &#34;application/json&#34;,
  &#34;ETag&#34;: null
}</pre>`,
		`hx-get="`+edit+`?focus=body" hx-target="#modal-body" aria-describedby="scenario-`+id+`-body">Show Body</button>`,
		// CRLF from the browser is stored as LF.
		`<pre class="popover__text popover__text--pre">{
  &#34;down&#34;: true
}</pre>`)

	// The editor of the stored scenario: every switch on, the values, Delete.
	rec = serve(h, "GET", edit, c, true)
	body = rec.Body.String()
	wantContains(t, "edit modal", body,
		`id="modal-title">Edit Scenario</h2>`, `hx-post="/ui/scenarios/`+id+`"`,
		`value="Down &lt;1&gt;"`, `<option value="svc" selected>My Svc</option>`,
		`name="requestMethodEnabled" checked>`, `<option value="POST" selected>POST</option>`,
		`name="requestPathEnabled" checked>`, `value="orders/42"`,
		`name="requestConditionEnabled" checked>`, `value="$request.query.page == &#39;2&#39;"`,
		`name="responseCodeEnabled" checked>`, `value="503"`,
		`name="responseHeadersEnabled" checked>`, `name="headerName" aria-label="Header name" placeholder="Header" autocomplete="off" value="Content-Type">`,
		`name="headerName" aria-label="Header name" placeholder="Header" autocomplete="off" value="ETag"><input class="header-rows__input" name="headerValue" aria-label="Header value" placeholder="Value (empty removes the header)" autocomplete="off"><button`,
		`name="responseBodyEnabled" checked>`, `data-language="JSON"`, ">\n{\n  &#34;down&#34;: true\n}</textarea>",
		`id="scenario-skipProxy" name="skipProxy" checked>`,
		`hx-delete="/ui/scenarios/`+id+`"`, `hx-confirm="Are you sure you want to delete the scenario Down &lt;1&gt;?"`, `>Save</button>`)
	if strings.Contains(body, "data-scroll-to") {
		t.Error("the editor scrolls without a focus")
	}
	assertNoEval(t, body)
	// "Show Headers" / "Show Body" open it scrolled there.
	wantContains(t, "focus headers", serve(h, "GET", edit+"?focus=headers", c, true).Body.String(), `data-scroll-to="scenario-headers"`)
	wantContains(t, "focus body", serve(h, "GET", edit+"?focus=body", c, true).Body.String(), `data-scroll-to="scenario-body"`)
	if body := serve(h, "GET", edit+"?focus=nope", c, true).Body.String(); strings.Contains(body, "data-scroll-to") {
		t.Error("an unknown focus scrolls")
	}

	// SC3: a field that is switched off is saved as unset, whatever its text; Skip Proxy
	// only counts while Body is on.
	off := url.Values{"name": {"Down"}, "service": {"svc"},
		"requestMethod": {"POST"}, "requestPath": {"x"}, "requestCondition": {"y"}, "responseCode": {"201"},
		"headerName": {"X-A"}, "headerValue": {"1"}, "responseBody": {"z"}, "skipProxy": {"on"}}
	rec = send(h, "POST", "/ui/scenarios/"+id, off, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Scenario "Down" updated` {
		t.Fatalf("update: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	if body := rec.Body.String(); strings.Contains(body, "Show Headers") || strings.Contains(body, "Show Body") || strings.Contains(body, "POST") {
		t.Errorf("switched-off fields were kept: %s", body)
	}
	body = serve(h, "GET", edit, c, true).Body.String()
	if strings.Contains(body, " checked") {
		t.Errorf("a switch is still on after switching everything off: %s", body)
	}
	// A switch that is on with an empty value is unset too.
	empty := url.Values{"name": {"Down"}, "service": {"other"}, "requestPathEnabled": {"on"}, "requestPath": {"/"},
		"responseHeadersEnabled": {"on"}, "headerName": {""}, "headerValue": {""}}
	if rec := send(h, "POST", "/ui/scenarios/"+id, empty, c, true); rec.Code != 200 {
		t.Fatalf("update: %d %s", rec.Code, rec.Body.String())
	}
	body = serve(h, "GET", edit, c, true).Body.String()
	wantContains(t, "moved", body, `<option value="other" selected>Other</option>`)
	if strings.Contains(body, " checked") {
		t.Errorf("empty values were stored: %s", body)
	}

	// SC4: delete.
	rec = serve(h, "DELETE", "/ui/scenarios/"+id, c, true)
	tr = triggers(t, rec)
	if rec.Code != 200 || tr["toast"]["title"] != `Scenario "Down" deleted` || tr["modal:close"] == nil {
		t.Errorf("delete: %d %v", rec.Code, rec.Header())
	}
	wantContains(t, "list after delete", rec.Body.String(), "No scenarios yet.")
	if rec := serve(h, "DELETE", "/ui/scenarios/"+id, c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" ||
		triggers(t, rec)["toast"]["message"] != "Scenario "+id+" not found" {
		t.Errorf("second delete: %d %v", rec.Code, rec.Header())
	}
	if rec := serve(h, "GET", "/ui/scenarios/abc/edit", c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("bad id: %d %v", rec.Code, rec.Header())
	}
}

// SC3, SC5: errors appear next to the fields, as 422 with an error toast; the editor
// stays unsaved, so closing it still asks.
func TestScenarioFormErrors(t *testing.T) {
	t.Parallel()
	h, c := scenarioUI(t)

	bad := url.Values{"name": {""}, "service": {""},
		"responseCodeEnabled": {"on"}, "responseCode": {"abc"},
		"responseHeadersEnabled": {"on"}, "headerName": {"X-A", "X-A"}, "headerValue": {"1", "2"}}
	rec := send(h, "POST", "/ui/scenarios", bad, c, true)
	body := rec.Body.String()
	if rec.Code != 422 || rec.Header().Get("HX-Retarget") != "" {
		t.Fatalf("form errors: %d %v", rec.Code, rec.Header())
	}
	if tr := triggers(t, rec); tr["toast"]["type"] != "error" || tr["modal:close"] != nil {
		t.Errorf("triggers: %v", tr)
	}
	wantContains(t, "form errors", body, `data-unsaved`,
		`<p class="field__error" id="scenario-responseCode-message">must be a whole number</p>`,
		`<p class="header-rows__error" role="alert">&#34;X-A&#34; appears twice</p>`,
		`value="abc"`, `value="2"`)

	// The use case's checks.
	bad = url.Values{"name": {""}, "service": {""}, "requestMethodEnabled": {"on"}, "requestMethod": {"FETCH"},
		"requestConditionEnabled": {"on"}, "requestCondition": {"$request.method == GET"},
		"responseCodeEnabled": {"on"}, "responseCode": {"42"},
		"responseHeadersEnabled": {"on"}, "headerName": {"Bad Name"}, "headerValue": {"1"},
		"responseBodyEnabled": {"on"}, "responseBody": {""}, "skipProxy": {"on"}}
	rec = send(h, "POST", "/ui/scenarios", bad, c, true)
	wantContains(t, "validation", rec.Body.String(),
		`<p class="field__error" id="scenario-name-message">is required</p>`,
		`<p class="field__error" id="scenario-service-message">is required</p>`,
		`<p class="field__error" id="scenario-requestMethod-message">must be one of GET, POST, PUT, PATCH, DELETE, HEAD, OPTIONS</p>`,
		`<p class="field__error" id="scenario-requestCondition-message">column 20: unknown name &#34;GET&#34;: values come from $request, strings need quotes</p>`,
		`<p class="field__error" id="scenario-responseCode-message">must be between 100 and 599</p>`,
		`<p class="header-rows__error" role="alert">&#34;Bad Name&#34; is not a valid header name</p>`,
		`<p class="switch__error" id="scenario-skipProxy-message">requires a response body</p>`)
	if rec.Code != 422 {
		t.Errorf("validation: %d", rec.Code)
	}

	// A duplicate name is shown next to the name; an unknown service next to the service.
	if rec := send(h, "POST", "/ui/scenarios", url.Values{"name": {"s"}, "service": {"svc"}}, c, true); rec.Code != 200 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	rec = send(h, "POST", "/ui/scenarios", url.Values{"name": {"s"}, "service": {"svc"}}, c, true)
	wantContains(t, "duplicate", rec.Body.String(),
		`<p class="field__error" id="scenario-name-message">A scenario named &#34;s&#34; already exists in service &#34;svc&#34;</p>`)
	if rec.Code != 422 || triggers(t, rec)["toast"]["message"] != `A scenario named "s" already exists in service "svc"` {
		t.Errorf("duplicate: %d %v", rec.Code, rec.Header())
	}
	rec = send(h, "POST", "/ui/scenarios", url.Values{"name": {"t"}, "service": {"gone"}}, c, true)
	wantContains(t, "unknown service", rec.Body.String(), `<p class="field__error" id="scenario-service-message">Service &#34;gone&#34; not found</p>`)
	if rec.Code != 422 {
		t.Errorf("unknown service: %d", rec.Code)
	}

	// Saving a scenario that was deleted meanwhile is a toast, not a form error.
	if rec := send(h, "POST", "/ui/scenarios/999999", url.Values{"name": {"s"}, "service": {"svc"}}, c, true); rec.Code != 404 ||
		rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("update deleted: %d %v", rec.Code, rec.Header())
	}
}

// Without JavaScript the editor still works: a page of its own, and a 303 after saving.
func TestScenarioFormsWithoutJS(t *testing.T) {
	t.Parallel()
	h, c := scenarioUI(t)

	if rec := serve(h, "GET", "/ui/scenarios/new", c, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Scenarios · Tmock</title>") {
		t.Errorf("add page: %d", rec.Code)
	}
	rec := send(h, "POST", "/ui/scenarios", url.Values{"name": {""}, "service": {"svc"}}, c, false)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "<title>Scenarios · Tmock</title>") || !strings.Contains(rec.Body.String(), "is required") {
		t.Errorf("invalid: %d", rec.Code)
	}
	rec = send(h, "POST", "/ui/scenarios", fullScenario("full"), c, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/scenarios" {
		t.Errorf("create: %d %v", rec.Code, rec.Header())
	}
	if body := serve(h, "GET", "/scenarios", c, false).Body.String(); !strings.Contains(body, `>full</button>`) {
		t.Errorf("page after create: %s", body)
	}
}

// Every scenarios route needs a session; htmx requests are sent to /login.
func TestScenarioRoutesNeedSession(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/scenarios"},
		{"GET", "/ui/scenarios/new"},
		{"POST", "/ui/scenarios"},
		{"GET", "/ui/scenarios/1/edit"},
		{"POST", "/ui/scenarios/1"},
		{"DELETE", "/ui/scenarios/1"},
	} {
		rec := send(h, r.method, r.path, fullScenario("s"), nil, true)
		if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/login" {
			t.Errorf("%s %s: %d %v", r.method, r.path, rec.Code, rec.Header())
		}
	}
}
