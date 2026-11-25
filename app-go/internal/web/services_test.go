package web

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"
)

// uiSession logs in through the form of a handler from newDBHandler.
func uiSession(t *testing.T, h http.Handler) *http.Cookie {
	t.Helper()
	rec := postLogin(h, "ann@example.com", "s3cret-pass", false)
	if rec.Code != 303 {
		t.Fatalf("login: %d", rec.Code)
	}
	return rec.Result().Cookies()[0]
}

func send(h http.Handler, method, path string, form url.Values, c *http.Cookie, htmx bool) *httptest.ResponseRecorder {
	r := httptest.NewRequest(method, path, strings.NewReader(form.Encode()))
	r.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	if htmx {
		r.Header.Set("HX-Request", "true")
	}
	if c != nil {
		r.AddCookie(c)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, r)
	return rec
}

func serviceForm(path, name, upstream string) url.Values {
	return url.Values{"path": {path}, "name": {name}, "upstreamUrl": {upstream}}
}

// triggers decodes HX-Trigger.
func triggers(t *testing.T, rec *httptest.ResponseRecorder) map[string]map[string]string {
	t.Helper()
	var m map[string]map[string]string
	if h := rec.Header().Get("HX-Trigger"); h != "" {
		if err := json.Unmarshal([]byte(h), &m); err != nil {
			t.Fatalf("HX-Trigger %q: %v", h, err)
		}
	}
	return m
}

func wantContains(t *testing.T, what, body string, wants ...string) {
	t.Helper()
	for _, w := range wants {
		if !strings.Contains(body, w) {
			t.Errorf("%s lacks %q:\n%s", what, w, body)
		}
	}
}

// S1–S3: list, add, edit (with a rename), delete.
func TestServicesPage(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	c := uiSession(t, h)

	rec := serve(h, "GET", "/services", c, false)
	body := rec.Body.String()
	if rec.Code != 200 {
		t.Fatalf("page: %d", rec.Code)
	}
	wantContains(t, "empty page", body,
		"<title>Services · Tmock</title>",
		`aria-current="page">Services</a>`,
		`hx-get="/ui/services/new" hx-target="#modal-body">Add Service</button>`,
		`<th class="table__head" scope="col">Name</th><th class="table__head" scope="col">Upstream URL</th><th class="table__head" scope="col">Path</th>`,
		"No services yet.")
	assertNoEval(t, body)

	// S2: the add modal.
	rec = serve(h, "GET", "/ui/services/new", c, true)
	body = rec.Body.String()
	if rec.Code != 200 || strings.Contains(body, "<html") {
		t.Fatalf("add modal: %d %s", rec.Code, body)
	}
	wantContains(t, "add modal", body, `id="modal-title">Add New Service</h2>`, `hx-post="/ui/services"`,
		`name="name"`, `name="upstreamUrl"`, `name="path"`, `>Create</button>`, `x-on:click="close">Close</button>`)
	if strings.Contains(body, "Delete") {
		t.Error("the add modal offers Delete")
	}
	assertNoEval(t, body)

	// A successful create replaces the table, closes the modal and shows a toast.
	rec = send(h, "POST", "/ui/services", serviceForm("svc", "My <Svc>", "http://up.test"), c, true)
	body = rec.Body.String()
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#services-table" || rec.Header().Get("HX-Reswap") != "outerHTML" {
		t.Fatalf("create: %d %v", rec.Code, rec.Header())
	}
	tr := triggers(t, rec)
	if tr["toast"]["type"] != "success" || tr["toast"]["title"] != `Service "My <Svc>" created` || tr["toast"]["target"] != "body" || tr["modal:close"]["target"] != "body" {
		t.Errorf("create triggers: %v", tr)
	}
	wantContains(t, "table after create", body, `<div class="table" id="services-table">`,
		`hx-get="/ui/services/svc/edit" hx-target="#modal-body">My &lt;Svc&gt;</button>`,
		`<td class="table__cell">http://up.test</td><td class="table__cell">svc</td>`)

	// S3: the edit modal, with Delete and its confirmation that mentions the scenarios.
	rec = serve(h, "GET", "/ui/services/svc/edit", c, true)
	body = rec.Body.String()
	wantContains(t, "edit modal", body, `id="modal-title">Edit Service</h2>`, `hx-post="/ui/services/svc"`,
		`value="My &lt;Svc&gt;"`, `value="http://up.test"`, `value="svc"`,
		`hx-delete="/ui/services/svc"`, `hx-confirm="Are you sure you want to delete the service My &lt;Svc&gt;? Its scenarios will be deleted too."`,
		`>Save</button>`)
	assertNoEval(t, body)

	// Saving with a new path renames the service.
	rec = send(h, "POST", "/ui/services/svc", serviceForm("renamed", "Svc", "https://up.test/v2"), c, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `Service "Svc" updated` {
		t.Fatalf("update: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	wantContains(t, "table after rename", rec.Body.String(), `hx-get="/ui/services/renamed/edit"`, "https://up.test/v2")
	if rec := serve(h, "GET", "/ui/services/svc/edit", c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" ||
		triggers(t, rec)["toast"]["message"] != `Service "svc" not found` {
		t.Errorf("edit old path: %d %v", rec.Code, rec.Header())
	}

	// Delete.
	rec = serve(h, "DELETE", "/ui/services/renamed", c, true)
	tr = triggers(t, rec)
	if rec.Code != 200 || tr["toast"]["title"] != `Service "Svc" deleted` || tr["modal:close"] == nil {
		t.Errorf("delete: %d %v", rec.Code, rec.Header())
	}
	wantContains(t, "table after delete", rec.Body.String(), "No services yet.")
	if rec := serve(h, "DELETE", "/ui/services/renamed", c, true); rec.Code != 404 || rec.Header().Get("HX-Reswap") != "none" {
		t.Errorf("second delete: %d %v", rec.Code, rec.Header())
	}
}

// S2: errors appear next to the fields, as 422 with an error toast (spec §10.2).
func TestServiceFormErrors(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	c := uiSession(t, h)

	rec := send(h, "POST", "/ui/services", serviceForm("a/b", "", "not-a-url"), c, true)
	body := rec.Body.String()
	if rec.Code != 422 || rec.Header().Get("HX-Retarget") != "" {
		t.Fatalf("invalid: %d %v", rec.Code, rec.Header())
	}
	if tr := triggers(t, rec); tr["toast"]["type"] != "error" || tr["modal:close"] != nil {
		t.Errorf("invalid triggers: %v", tr)
	}
	wantContains(t, "invalid form", body,
		`id="modal-title">Add New Service</h2>`,
		`value="a/b"`, `value="not-a-url"`,
		`<p class="field__error" id="service-name-message">is required</p>`,
		`<p class="field__error" id="service-upstreamUrl-message">must be an absolute http or https URL</p>`,
		`<p class="field__error" id="service-path-message">may contain only letters, digits, &#39;.&#39;, &#39;_&#39; and &#39;-&#39;</p>`)

	// A duplicate path is shown above the fields.
	if rec := send(h, "POST", "/ui/services", serviceForm("svc", "Svc", "http://up.test"), c, true); rec.Code != 200 {
		t.Fatalf("create: %d", rec.Code)
	}
	if rec := send(h, "POST", "/ui/services", serviceForm("other", "Other", "http://up.test"), c, true); rec.Code != 200 {
		t.Fatalf("create: %d", rec.Code)
	}
	rec = send(h, "POST", "/ui/services", serviceForm("svc", "Svc 2", "http://up.test"), c, true)
	wantContains(t, "duplicate", rec.Body.String(), `role="alert">A service with path &#34;svc&#34; already exists</div>`)
	if rec.Code != 422 {
		t.Errorf("duplicate: %d", rec.Code)
	}

	// A failed rename keeps posting to the stored path.
	rec = send(h, "POST", "/ui/services/other", serviceForm("svc", "Other", "http://up.test"), c, true)
	if rec.Code != 422 {
		t.Errorf("rename onto a taken path: %d", rec.Code)
	}
	wantContains(t, "failed rename", rec.Body.String(), `hx-post="/ui/services/other"`, `value="svc"`, "already exists")
}

// Without JavaScript the forms still work: a page of their own, and a 303 after saving.
func TestServiceFormsWithoutJS(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	c := uiSession(t, h)

	if rec := serve(h, "GET", "/ui/services/new", c, false); rec.Code != 200 || !strings.Contains(rec.Body.String(), "<title>Services · Tmock</title>") {
		t.Errorf("add page: %d", rec.Code)
	}
	rec := send(h, "POST", "/ui/services", serviceForm("svc", "", "http://up.test"), c, false)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "<title>Services · Tmock</title>") || !strings.Contains(rec.Body.String(), "is required") {
		t.Errorf("invalid: %d", rec.Code)
	}
	rec = send(h, "POST", "/ui/services", serviceForm("svc", "Svc", "http://up.test"), c, false)
	if rec.Code != 303 || rec.Header().Get("Location") != "/services" {
		t.Errorf("create: %d %v", rec.Code, rec.Header())
	}
	if body := serve(h, "GET", "/services", c, false).Body.String(); !strings.Contains(body, `<td class="table__cell">svc</td>`) {
		t.Errorf("page after create: %s", body)
	}
}

// Every services route needs a session; htmx requests are sent to /login with HX-Redirect.
func TestServiceRoutesNeedSession(t *testing.T) {
	t.Parallel()
	h := newDBHandler(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/services"},
		{"GET", "/ui/services/new"},
		{"POST", "/ui/services"},
		{"GET", "/ui/services/svc/edit"},
		{"POST", "/ui/services/svc"},
		{"DELETE", "/ui/services/svc"},
	} {
		rec := send(h, r.method, r.path, serviceForm("svc", "Svc", "http://up.test"), nil, true)
		if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/login" {
			t.Errorf("%s %s: %d %v", r.method, r.path, rec.Code, rec.Header())
		}
	}
}

func TestTriggerMerges(t *testing.T) {
	rec := httptest.NewRecorder()
	toast(rec, toastSuccess, "Saved", "")
	closeModal(rec)
	tr := triggers(t, rec)
	if len(tr) != 2 || tr["toast"]["title"] != "Saved" || tr["modal:close"]["target"] != "body" {
		t.Errorf("triggers %v", tr)
	}
}
