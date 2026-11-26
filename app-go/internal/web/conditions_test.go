package web

import (
	"net/url"
	"strings"
	"testing"
)

// specExample is NOT (ip = 10.0.0.1 OR (header X-Env = qa AND cidr 192.168.0.0/16)) as
// the builder posts it (spec §15.3 I08).
func specExample() url.Values {
	return url.Values{
		"n.0.kind": {"group"}, "n.0.not": {"on"}, "n.0.op": {"or"},
		"n.0.0.kind": {"ip"}, "n.0.0.value": {"10.0.0.1"},
		"n.0.1.kind": {"group"}, "n.0.1.op": {"and"},
		"n.0.1.0.kind": {"header"}, "n.0.1.0.header": {"X-Env"}, "n.0.1.0.value": {"qa"},
		"n.0.1.1.kind": {"cidr"}, "n.0.1.1.value": {"192.168.0.0/16"},
	}
}

// C7: the condition builder (spec §10.5). Edits re-render it and save nothing; Save
// stores the condition, and reopening shows the same tree.
func TestConditionBuilder(t *testing.T) {
	t.Parallel()
	h, c, _, _ := clientUI(t)
	id := newClient(t, h, c, "Phone")
	u := "/ui/clients/" + id

	// No condition: an empty AND group, which cannot be removed.
	body := serve(h, "GET", u+"/condition", c, true).Body.String()
	wantContains(t, "empty builder", body,
		`id="modal-title">Edit Client Conditions: Phone</h2>`,
		`<form class="form" method="post" action="`+u+`/condition" hx-post="`+u+`/condition" hx-target="#modal-body" hx-sync="this:drop" data-persistent x-data="unsavedGuard">`,
		`<button type="submit" class="button" hidden tabindex="-1" aria-hidden="true">Save</button>`,
		`<input type="hidden" name="n.0.kind" value="group"><input type="hidden" name="n.0.op" value="and">`,
		`aria-pressed="false" title="Negate this group" name="edit" value="not:n.0" formaction="`+u+`/condition/edit" formnovalidate hx-post="`+u+`/condition/edit" hx-target="#modal-body" hx-sync="closest form:drop">NOT</button>`,
		`<summary class="menu__button">Add Expression</summary>`,
		`name="edit" value="add-headerRegex:n.0"`, `>Header Regex</button>`)
	if strings.Contains(body, "condition-group__remove") {
		t.Error("the top-level group can be removed")
	}
	// The stored tree: closing does not ask.
	if strings.Contains(body, "data-unsaved") {
		t.Error("the stored condition is marked unsaved")
	}
	// Every edit button drops its click while a request of the form is in flight, so
	// a quick second edit cannot undo the first.
	if edits, synced := strings.Count(body, `name="edit"`), strings.Count(body, `hx-sync="closest form:drop"`); edits == 0 || edits != synced {
		t.Errorf("%d edit buttons, %d with hx-sync", edits, synced)
	}
	assertNoEval(t, body)

	// An edit: the whole form plus the button's edit; the answer is the builder.
	form := specExample()
	form.Set("edit", "add-ip:n.0.1")
	rec := send(h, "POST", u+"/condition/edit", form, c, true)
	wantContains(t, "after add", rec.Body.String(),
		`name="n.0.1.2.kind" value="ip"`, `name="n.0.1.0.header" value="X-Env"`,
		`aria-pressed="true" title="Negate this group" name="edit" value="not:n.0"`,
		`class="condition-group__operator condition-group__operator--or"`, `>Equals</button>`,
		`aria-label="Remove this group" name="edit" value="remove:n.0.1"`)
	if rec.Code != 200 || rec.Header().Get("HX-Trigger") != "" {
		t.Errorf("edit: %d %v", rec.Code, rec.Header())
	}
	// An edited tree is not what is stored: closing asks.
	wantContains(t, "edited is unsaved", rec.Body.String(), `x-data="unsavedGuard" data-unsaved>`)
	form.Set("edit", "not:n.0.0")
	wantContains(t, "NOT on an item", send(h, "POST", u+"/condition/edit", form, c, true).Body.String(),
		`<button type="submit" class="condition-item__operator" aria-pressed="true" title="Negate this expression" name="edit" value="not:n.0.0"`, `>Not Equals</button>`)
	for _, bad := range []string{"remove:n.0", "explode:n.0", "add-ip:n.0.9", ""} {
		form.Set("edit", bad)
		if rec := send(h, "POST", u+"/condition/edit", form, c, true); rec.Code != 400 || rec.Header().Get("HX-Reswap") != "none" {
			t.Errorf("edit %q: %d", bad, rec.Code)
		}
	}
	if got := serve(h, "GET", u+"/condition", c, true).Body.String(); strings.Contains(got, "X-Env") {
		t.Error("an edit was saved")
	}

	// Save shows problems next to their expressions.
	bad := specExample()
	bad.Set("n.0.0.value", "10.0.0")
	bad.Set("n.0.1.0.header", "")
	rec = send(h, "POST", u+"/condition", bad, c, true)
	wantContains(t, "invalid", rec.Body.String(),
		`<p class="condition-item__error" id="n.0.0.value-message">IP address: must be an IPv4 or IPv6 address</p>`,
		`<p class="condition-item__error" id="n.0.1.0.header-message">Header name: must be 1–255 characters</p>`,
		`value="10.0.0"`, `data-unsaved>`)
	if rec.Code != 422 || triggers(t, rec)["toast"]["title"] != "The condition was not saved" {
		t.Errorf("invalid save: %d %v", rec.Code, rec.Header())
	}

	// Save, and reopen: the same tree.
	rec = send(h, "POST", u+"/condition", specExample(), c, true)
	tr := triggers(t, rec)
	if rec.Code != 200 || tr["toast"]["title"] != `Condition of "Phone" saved` || tr["modal:close"] == nil || rec.Header().Get("HX-Retarget") != "#clients-table" {
		t.Fatalf("save: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	reopened := serve(h, "GET", u+"/condition", c, true).Body.String()
	for name, values := range specExample() {
		want := `name="` + name + `" value="` + values[0] + `"`
		if !strings.Contains(reopened, want) {
			t.Errorf("reopened builder lacks %s", want)
		}
	}

	// Enabled, the condition cannot be emptied.
	send(h, "POST", u+"/enabled", url.Values{"enabled": {"on"}}, c, true)
	rec = send(h, "POST", u+"/condition", url.Values{"n.0.kind": {"group"}, "n.0.op": {"and"}}, c, true)
	wantContains(t, "empty while enabled", rec.Body.String(), "This client is enabled, and an enabled client needs a condition.")
	if rec.Code != 422 {
		t.Errorf("empty while enabled: %d", rec.Code)
	}
	// Disabled, it can: an empty builder saves no condition.
	send(h, "POST", u+"/enabled", url.Values{}, c, true)
	if rec := send(h, "POST", u+"/condition", url.Values{"n.0.kind": {"group"}}, c, true); rec.Code != 200 {
		t.Errorf("empty: %d %s", rec.Code, rec.Body.String())
	}
	if got := serve(h, "GET", u+"/condition", c, true).Body.String(); strings.Contains(got, "n.0.0.kind") {
		t.Error("the condition was not cleared")
	}
	// A form that is not a tree is a 400.
	if rec := send(h, "POST", u+"/condition", url.Values{"n.0.kind": {"ip"}}, c, true); rec.Code != 400 {
		t.Errorf("not a tree: %d", rec.Code)
	}
}

// Without JavaScript the builder is a page, and its buttons post to the edit URL
// (formaction); the answer is the page again.
func TestConditionBuilderWithoutJS(t *testing.T) {
	t.Parallel()
	h, c, _, _ := clientUI(t)
	u := "/ui/clients/" + newClient(t, h, c, "Plain")
	if rec := serve(h, "GET", u+"/condition", c, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Clients · Tmock</title>") {
		t.Errorf("page: %d", rec.Code)
	}
	rec := send(h, "POST", u+"/condition/edit", url.Values{"n.0.kind": {"group"}, "edit": {"add-cidr:n.0"}}, c, false)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), `name="n.0.0.kind" value="cidr"`) || !strings.Contains(rec.Body.String(), "<title>Clients · Tmock</title>") {
		t.Errorf("edit page: %d", rec.Code)
	}
	rec = send(h, "POST", u+"/condition", url.Values{"n.0.kind": {"group"}, "n.0.0.kind": {"cidr"}, "n.0.0.value": {"10.0.0.0/8"}}, c, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/" {
		t.Errorf("save: %d %v", rec.Code, rec.Header())
	}
}
