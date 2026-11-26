package web

import (
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"
)

// clientUI is a handler with a session, the service svc and its scenarios "slow" and
// "down"; it returns their ids as strings.
func clientUI(t *testing.T) (http.Handler, *http.Cookie, string, string) {
	t.Helper()
	h, c := scenarioUI(t)
	var ids []string
	for _, n := range []string{"slow", "down"} {
		rec := send(h, "POST", "/ui/scenarios", url.Values{"name": {n}, "service": {"svc"}}, c, true)
		if rec.Code != 200 {
			t.Fatalf("scenario: %d %s", rec.Code, rec.Body.String())
		}
	}
	body := serve(h, "GET", "/scenarios", c, false).Body.String()
	for _, n := range []string{"slow", "down"} {
		m := regexp.MustCompile(`hx-get="/ui/scenarios/(\d+)/edit" hx-target="#modal-body">` + n + `<`).FindStringSubmatch(body)
		if m == nil {
			t.Fatalf("no scenario %s", n)
		}
		ids = append(ids, m[1])
	}
	return h, c, ids[0], ids[1]
}

var clientIDRe = regexp.MustCompile(`hx-get="/ui/clients/(\d+)/edit" hx-target="#modal-body">`)

func newClient(t *testing.T, h http.Handler, c *http.Cookie, name string) string {
	t.Helper()
	rec := send(h, "POST", "/ui/clients", url.Values{"name": {name}}, c, true)
	if rec.Code != 200 {
		t.Fatalf("create: %d %s", rec.Code, rec.Body.String())
	}
	m := clientIDRe.FindAllStringSubmatch(rec.Body.String(), -1)
	return m[len(m)-1][1]
}

// C1–C6: list, add, edit (name), the enabled switch, chips, assignments, delete. The
// condition builder (C7) is in TestConditionBuilder.
func TestClientsPage(t *testing.T) {
	t.Parallel()
	h, c, slow, down := clientUI(t)

	rec := serve(h, "GET", "/", c, false)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("page: %d", rec.Code)
	}
	wantContains(t, "empty page", body, "<title>Clients · Tmock</title>", `aria-current="page">Clients</a>`,
		`hx-get="/ui/clients/new" hx-target="#modal-body">Add Client</button>`,
		`<div class="table" id="clients-table" hx-get="/ui/clients/table" hx-trigger="clients:changed from:body" hx-target="this" hx-swap="outerHTML">`,
		"No clients yet.")
	assertNoEval(t, body)

	// C3: the add modal has a name only.
	body = serve(h, "GET", "/ui/clients/new", c, true).Body.String()
	wantContains(t, "add modal", body, `id="modal-title">Add Client</h2>`, `hx-post="/ui/clients"`, `name="name"`, `>Create</button>`)
	if strings.Contains(body, `name="condition"`) || strings.Contains(body, "Delete") {
		t.Error("the add modal has more than a name")
	}

	rec = send(h, "POST", "/ui/clients", url.Values{"name": {"Phone <1>"}}, c, true)
	tr := triggers(t, rec)
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#clients-table" || tr["toast"]["title"] != `Client "Phone <1>" created` || tr["modal:close"] == nil {
		t.Fatalf("create: %d %v", rec.Code, rec.Header())
	}
	id := clientIDRe.FindStringSubmatch(rec.Body.String())[1]
	u := "/ui/clients/" + id
	wantContains(t, "row", rec.Body.String(),
		`<input class="switch__input" type="checkbox" role="switch" name="enabled" hx-post="`+u+`/enabled" hx-trigger="change">`,
		`hx-get="`+u+`/edit" hx-target="#modal-body">Phone &lt;1&gt;</button>`,
		`hx-get="`+u+`/condition" hx-target="#modal-body">Conditions</button>`,
		`aria-label="Assign scenarios to Phone &lt;1&gt;" title="Assign scenarios to Phone &lt;1&gt;" hx-get="`+u+`/scenarios" hx-target="#modal-body">`,
		`hx-post="`+u+`/token" hx-target="#modal-body">Generate</button>`)

	// C2: enabling without a condition is refused; the table comes back with the switch
	// off and an error toast.
	rec = send(h, "POST", u+"/enabled", url.Values{"enabled": {"on"}}, c, true)
	tr = triggers(t, rec)
	if rec.Code != 422 || rec.Header().Get("HX-Retarget") != "#clients-table" || tr["toast"]["type"] != "error" ||
		tr["toast"]["title"] != "The client was not enabled" || tr["toast"]["message"] != "A client needs a condition before it can be enabled" {
		t.Errorf("enable without condition: %d %v", rec.Code, rec.Header())
	}
	if strings.Contains(rec.Body.String(), `name="enabled" checked`) {
		t.Error("the switch stayed on")
	}

	// C4: the edit modal: rename and Delete, and no condition (that is the builder's).
	body = serve(h, "GET", u+"/edit", c, true).Body.String()
	wantContains(t, "edit modal", body, `id="modal-title">Edit Client</h2>`, `hx-post="`+u+`"`, `value="Phone &lt;1&gt;"`,
		`hx-delete="`+u+`"`, `hx-confirm="Are you sure you want to delete the client Phone &lt;1&gt;?"`)
	if strings.Contains(body, "condition") {
		t.Error("the edit modal still edits the condition")
	}
	assertNoEval(t, body)
	rec = send(h, "POST", u, url.Values{"name": {"Phone"}}, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Client "Phone" updated` {
		t.Fatalf("rename: %d %s", rec.Code, rec.Body.String())
	}
	// A failed rename shows the typed name, but Delete names the client as stored.
	rec = send(h, "POST", u, url.Values{"name": {"   "}}, c, true)
	wantContains(t, "failed rename", rec.Body.String(), `hx-confirm="Are you sure you want to delete the client Phone?"`)
	if rec.Code != 422 {
		t.Errorf("failed rename: %d", rec.Code)
	}
	rec = send(h, "POST", u, url.Values{"name": {strings.Repeat("x", 300)}}, c, true)
	wantContains(t, "too long rename", rec.Body.String(), `value="`+strings.Repeat("x", 300)+`"`,
		`hx-confirm="Are you sure you want to delete the client Phone?"`)

	// C2: with a condition, enabling works; disabling too.
	rec = send(h, "POST", u+"/condition", url.Values{"n.0.kind": {"group"}, "n.0.0.kind": {"header"}, "n.0.0.header": {"x-test"}, "n.0.0.value": {"1"}}, c, true)
	if rec.Code != 200 {
		t.Fatalf("condition: %d %s", rec.Code, rec.Body.String())
	}
	rec = send(h, "POST", u+"/enabled", url.Values{"enabled": {"on"}}, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Client "Phone" enabled` || !strings.Contains(rec.Body.String(), `name="enabled" checked`) {
		t.Errorf("enable: %d %v", rec.Code, rec.Header())
	}
	rec = send(h, "POST", u+"/enabled", url.Values{}, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Client "Phone" disabled` {
		t.Errorf("disable: %d %v", rec.Code, rec.Header())
	}

	// C6 then C5: assign through the modal's switches, then remove with a chip's ✕.
	body = serve(h, "GET", u+"/scenarios", c, true).Body.String()
	wantContains(t, "assignment modal", body, `id="modal-title">Scenarios of Phone</h2>`,
		`<span class="collapsible__title">My Svc</span><span class="collapsible__meta">svc</span>`,
		`<div id="assignment-`+id+`-`+slow+`"><div class="switch"><label class="switch__control"><input class="switch__input" type="checkbox" role="switch" name="assigned" hx-post="`+u+`/scenarios/`+slow+`" hx-trigger="change" hx-target="#assignment-`+id+`-`+slow+`" hx-swap="outerHTML">`,
		`id="scenario-`+slow+`-info" popover="manual"`, "Method:    any")
	assertNoEval(t, body)
	rec = send(h, "POST", u+"/scenarios/"+slow, url.Values{"assigned": {"on"}}, c, true)
	tr = triggers(t, rec)
	if rec.Code != 200 || tr["toast"]["title"] != `Scenario "slow" assigned to "Phone"` || tr["clients:changed"]["target"] != "body" || tr["modal:close"] != nil {
		t.Errorf("assign: %d %v", rec.Code, rec.Header())
	}
	wantContains(t, "assigned switch", rec.Body.String(), `<div id="assignment-`+id+`-`+slow+`">`, `name="assigned" checked`)
	send(h, "POST", u+"/scenarios/"+down, url.Values{"assigned": {"on"}}, c, true)
	// A reload shows both, as chips in the table and switches on in the modal.
	table := serve(h, "GET", "/ui/clients/table", c, true).Body.String()
	wantContains(t, "chips", table,
		`<strong class="chip__strong">svc</strong> | down</span><button type="button" class="chip__remove" aria-label="Remove svc | down" hx-delete="`+u+`/scenarios/`+down+`">✕</button>`,
		`<strong class="chip__strong">svc</strong> | slow</span>`)
	if n := strings.Count(serve(h, "GET", u+"/scenarios", c, true).Body.String(), `name="assigned" checked`); n != 2 {
		t.Errorf("%d switches on after reload, want 2", n)
	}
	rec = serve(h, "DELETE", u+"/scenarios/"+down, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != "Scenario removed from client" || strings.Contains(rec.Body.String(), "| down") {
		t.Errorf("remove chip: %d %v", rec.Code, rec.Header())
	}
	// Unassigning through the modal.
	rec = send(h, "POST", u+"/scenarios/"+slow, url.Values{}, c, true)
	if rec.Code != 200 || strings.Contains(rec.Body.String(), "checked") || triggers(t, rec)["toast"]["title"] != "Scenario removed from client" {
		t.Errorf("unassign: %d %s", rec.Code, rec.Body.String())
	}
	// An unknown scenario is a toast.
	if rec := send(h, "POST", u+"/scenarios/999999", url.Values{"assigned": {"on"}}, c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("unknown scenario: %d %v", rec.Code, rec.Header())
	}

	// C4: delete.
	rec = serve(h, "DELETE", u, c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Client "Phone" deleted` || !strings.Contains(rec.Body.String(), "No clients yet.") {
		t.Errorf("delete: %d %v", rec.Code, rec.Header())
	}
	if rec := serve(h, "DELETE", u, c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("second delete: %d", rec.Code)
	}
}

// C8: Generate shows the token once, in a persistent modal; Regenerate and Revoke ask.
func TestClientTokenUI(t *testing.T) {
	t.Parallel()
	h, c, _, _ := clientUI(t)
	id := newClient(t, h, c, "Bot")
	u := "/ui/clients/" + id

	rec := send(h, "POST", u+"/token", nil, c, true)
	body := rec.Body.String()
	m := regexp.MustCompile(`value="(tmck_[A-Za-z0-9_-]{43})"`).FindStringSubmatch(body)
	if rec.Code != 200 || m == nil || triggers(t, rec)["clients:changed"]["target"] != "body" {
		t.Fatalf("generate: %d %v %s", rec.Code, rec.Header(), body)
	}
	wantContains(t, "token modal", body, `<div class="modal-content" data-persistent>`, `id="modal-title">Access Token of Bot</h2>`,
		`role="alert">This token is shown only once. Copy it now; it cannot be shown again.</div>`,
		`x-data="copyField" data-copied="Token copied to clipboard"`, `readonly value="`+m[1]+`"`, `x-on:click="copy">Copy</button>`)
	assertNoEval(t, body)

	// The table now offers Regenerate (asks) and Revoke (asks), and never the token.
	table := serve(h, "GET", "/ui/clients/table", c, true).Body.String()
	wantContains(t, "table with token", table,
		`hx-post="`+u+`/token" hx-target="#modal-body" hx-confirm="Regenerate the token of Bot? The current token stops working.">Regenerate</button>`,
		`hx-delete="`+u+`/token" hx-confirm="Are you sure you want to revoke the token of the client Bot?">Revoke</button>`)
	if strings.Contains(table, m[1]) || strings.Contains(table, ">Generate<") {
		t.Error("the table shows the token, or still offers Generate")
	}
	if strings.Contains(serve(h, "GET", u+"/edit", c, true).Body.String(), m[1]) {
		t.Error("the edit modal shows the token")
	}

	rec = serve(h, "DELETE", u+"/token", c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Token of "Bot" revoked` || !strings.Contains(rec.Body.String(), ">Generate</button>") {
		t.Errorf("revoke: %d %v", rec.Code, rec.Header())
	}
	if rec := send(h, "POST", "/ui/clients/999999/token", nil, c, true); rec.Code != 404 {
		t.Errorf("token for missing client: %d", rec.Code)
	}
}

func TestClientFormErrors(t *testing.T) {
	t.Parallel()
	h, c, _, _ := clientUI(t)
	newClient(t, h, c, "taken")
	other := newClient(t, h, c, "other")

	rec := send(h, "POST", "/ui/clients", url.Values{"name": {""}}, c, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `<p class="field__error" id="client-name-message">is required</p>`) {
		t.Errorf("empty name: %d %s", rec.Code, rec.Body.String())
	}
	rec = send(h, "POST", "/ui/clients", url.Values{"name": {"taken"}}, c, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="client-name-message">A client named &#34;taken&#34; already exists</p>`) {
		t.Errorf("duplicate: %d %s", rec.Code, rec.Body.String())
	}
	rec = send(h, "POST", "/ui/clients/"+other, url.Values{"name": {"taken"}}, c, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="client-name-message">A client named &#34;taken&#34; already exists</p>`) || !strings.Contains(rec.Body.String(), `value="taken"`) {
		t.Errorf("rename to a taken name: %d %s", rec.Code, rec.Body.String())
	}
}

// Without JavaScript: the forms are pages of their own and saves redirect to /.
func TestClientFormsWithoutJS(t *testing.T) {
	t.Parallel()
	h, c, _, _ := clientUI(t)
	if rec := serve(h, "GET", "/ui/clients/new", c, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Clients · Tmock</title>") {
		t.Errorf("add page: %d", rec.Code)
	}
	rec := send(h, "POST", "/ui/clients", url.Values{"name": {"plain"}}, c, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/" {
		t.Errorf("create: %d %v", rec.Code, rec.Header())
	}
	id := clientIDRe.FindStringSubmatch(serve(h, "GET", "/", c, false).Body.String())[1]
	rec = send(h, "POST", "/ui/clients/"+id+"/token", nil, c, false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Clients · Tmock</title>") || !strings.Contains(rec.Body.String(), `readonly value="tmck_`) {
		t.Errorf("token page: %d", rec.Code)
	}
}

func TestClientRoutesNeedSession(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/"}, {"GET", "/ui/clients/table"}, {"GET", "/ui/clients/new"}, {"POST", "/ui/clients"},
		{"GET", "/ui/clients/1/edit"}, {"POST", "/ui/clients/1"}, {"DELETE", "/ui/clients/1"},
		{"POST", "/ui/clients/1/enabled"}, {"GET", "/ui/clients/1/scenarios"},
		{"POST", "/ui/clients/1/scenarios/1"}, {"DELETE", "/ui/clients/1/scenarios/1"},
		{"POST", "/ui/clients/1/token"}, {"DELETE", "/ui/clients/1/token"},
		{"GET", "/ui/clients/1/condition"}, {"POST", "/ui/clients/1/condition"}, {"POST", "/ui/clients/1/condition/edit"},
	} {
		rec := send(h, r.method, r.path, url.Values{"name": {"x"}}, nil, true)
		if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/login" {
			t.Errorf("%s %s: %d %v", r.method, r.path, rec.Code, rec.Header())
		}
	}
}
