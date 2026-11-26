package web

import (
	"context"
	"log/slog"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"testing"

	"github.com/laposa/tmock/app-go/internal/app"
	"github.com/laposa/tmock/app-go/internal/auth"
	"github.com/laposa/tmock/app-go/internal/db/dbtest"
	"github.com/laposa/tmock/app-go/internal/password"
	"github.com/laposa/tmock/app-go/internal/store"
)

// accountsUI is a handler on a database with the admin Ann (id 1) and the user Bob (id 2),
// both with password "s3cret-pass", and their sessions.
func accountsUI(t *testing.T) (h http.Handler, ann, bob *http.Cookie) {
	t.Helper()
	pool := dbtest.Migrated(t)
	hasher := password.Hasher{Iterations: 1000}
	hash, _ := hasher.Hash("s3cret-pass")
	users := store.NewUsers(pool)
	for _, u := range []store.NewUser{
		{Name: "Ann", Email: "ann@example.com", PasswordHash: hash, Admin: true},
		{Name: "Bob", Email: "bob@example.com", PasswordHash: hash},
	} {
		if _, err := users.Create(context.Background(), u); err != nil {
			t.Fatal(err)
		}
	}
	logger := slog.New(slog.DiscardHandler)
	h = Handler(Deps{
		App:      app.New(pool, app.Options{Logger: logger, Hasher: hasher}),
		Sessions: auth.New(store.NewSessions(pool), auth.Options{Secret: []byte("0123456789abcdef0123456789abcdef"), Logger: logger}),
		Logger:   logger,
	})
	login := func(email, pw string) *http.Cookie {
		rec := postLogin(h, email, pw, false)
		if rec.Code != 303 {
			t.Fatalf("login %s: %d", email, rec.Code)
		}
		return rec.Result().Cookies()[0]
	}
	return h, login("ann@example.com", "s3cret-pass"), login("bob@example.com", "s3cret-pass")
}

var accountIDRe = regexp.MustCompile(`hx-get="/ui/accounts/(\d+)/edit"`)

// A1–A4.
func TestAccountsPage(t *testing.T) {
	t.Parallel()
	h, ann, _ := accountsUI(t)

	rec := serve(h, "GET", "/accounts", ann, false)
	body := rec.Body.String()
	wantContains(t, "page", body, "<title>Accounts · Tmock</title>", `aria-current="page">Accounts</a>`,
		`hx-get="/ui/accounts/new" hx-target="#modal-body">Add User</button>`,
		`<th class="table__head" scope="col">Name</th><th class="table__head" scope="col">Email</th><th class="table__head" scope="col">Admin</th>`,
		`<td class="table__cell">Ann</td><td class="table__cell">ann@example.com</td><td class="table__cell">✓</td>`,
		`<td class="table__cell">Bob</td><td class="table__cell">bob@example.com</td><td class="table__cell">✕</td>`,
		`hx-delete="/ui/accounts/2" hx-confirm="Are you sure you want to delete the user Bob?">Delete</button>`)
	assertNoEval(t, body)

	// A2: name, email, password, admin; Create waits for the required fields.
	body = serve(h, "GET", "/ui/accounts/new", ann, true).Body.String()
	wantContains(t, "add modal", body, `id="modal-title">Add User</h2>`, `x-data="requiredFields" x-on:input="update"`,
		`name="name" type="text" required`, `type="email" required`, `name="password" type="password" required`, `name="admin"`,
		`x-bind:disabled="incomplete">Create</button>`)
	rec = send(h, "POST", "/ui/accounts", url.Values{"name": {"Cy"}, "email": {"cy@example.com"}, "password": {"password1"}, "admin": {"on"}}, ann, true)
	tr := triggers(t, rec)
	if rec.Code != 200 || rec.Header().Get("HX-Retarget") != "#accounts-table" || tr["toast"]["title"] != `User "Cy" created` || tr["modal:close"] == nil {
		t.Fatalf("create: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	cy := accountIDRe.FindAllStringSubmatch(rec.Body.String(), -1)[2][1]

	// A2 errors next to the fields; a duplicate email (any case) too.
	rec = send(h, "POST", "/ui/accounts", url.Values{"name": {""}, "email": {"nope"}, "password": {"short"}}, ann, true)
	wantContains(t, "add errors", rec.Body.String(),
		`id="account-name-message">is required</p>`, `id="account-email-message">must be an email address</p>`,
		`id="account-password-message">must be at least 8 characters</p>`)
	rec = send(h, "POST", "/ui/accounts", url.Values{"name": {"B"}, "email": {"BOB@example.com"}, "password": {"password1"}}, ann, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="account-email-message">A user with email &#34;BOB@example.com&#34; already exists</p>`) {
		t.Errorf("duplicate: %d %s", rec.Code, rec.Body.String())
	}

	// A3: the edit modal; only changed fields are saved, a blank password keeps it.
	body = serve(h, "GET", "/ui/accounts/"+cy+"/edit", ann, true).Body.String()
	wantContains(t, "edit modal", body, `id="modal-title">Edit User</h2>`, `value="Cy"`, `value="cy@example.com"`,
		`<input type="hidden" name="orig.name" value="Cy">`, `<input type="hidden" name="orig.admin" value="on">`,
		`Leave blank to keep the current password.`, `name="admin" checked`)
	if strings.Contains(body, `name="password" type="password" required`) {
		t.Error("the new password is required")
	}
	edit := url.Values{"orig.name": {"Cy"}, "orig.email": {"cy@example.com"}, "orig.admin": {"on"},
		"name": {"Cyril"}, "email": {"cy@example.com"}, "admin": {"on"}, "password": {""}}
	rec = send(h, "POST", "/ui/accounts/"+cy, edit, ann, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `User "Cyril" updated` || !strings.Contains(rec.Body.String(), "<td class=\"table__cell\">Cyril</td>") {
		t.Errorf("rename: %d %v", rec.Code, rec.Header())
	}
	if rec := postLogin(h, "cy@example.com", "password1", false); rec.Code != 303 {
		t.Errorf("a blank password changed it: %d", rec.Code)
	}
	// Ann edits a stale modal (the name was Cy): her unchanged fields do not set the
	// name back; the new password and the admin flag are saved.
	stale := url.Values{"orig.name": {"Cy"}, "orig.email": {"cy@example.com"}, "orig.admin": {"on"},
		"name": {"Cy"}, "email": {"cy@example.com"}, "password": {"password2"}}
	rec = send(h, "POST", "/ui/accounts/"+cy, stale, ann, true)
	if rec.Code != 200 || !strings.Contains(rec.Body.String(), "<td class=\"table__cell\">Cyril</td><td class=\"table__cell\">cy@example.com</td><td class=\"table__cell\">✕</td>") {
		t.Errorf("only changes: %d %s", rec.Code, rec.Body.String())
	}
	if rec := postLogin(h, "cy@example.com", "password2", false); rec.Code != 303 {
		t.Errorf("new password: %d", rec.Code)
	}

	// A3: Ann is the last admin and cannot take her own flag.
	rec = send(h, "POST", "/ui/accounts/1", url.Values{"orig.name": {"Ann"}, "orig.email": {"ann@example.com"}, "orig.admin": {"on"},
		"name": {"Ann"}, "email": {"ann@example.com"}}, ann, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), "the last admin must stay an admin") {
		t.Errorf("demote last admin: %d %s", rec.Code, rec.Body.String())
	}

	// A4: Delete asks; refusals are toasts.
	rec = serve(h, "DELETE", "/ui/accounts/1", ann, true)
	tr = triggers(t, rec)
	if rec.Code != 422 || rec.Header().Get("HX-Reswap") != "none" || tr["toast"]["title"] != "The user was not deleted" || tr["toast"]["message"] != "You cannot delete your own account" {
		t.Errorf("own account: %d %v", rec.Code, rec.Header())
	}
	rec = serve(h, "DELETE", "/ui/accounts/"+cy, ann, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != `User "Cyril" deleted` || strings.Contains(rec.Body.String(), "Cyril") {
		t.Errorf("delete: %d %v", rec.Code, rec.Header())
	}
	if rec := serve(h, "DELETE", "/ui/accounts/"+cy, ann, true); rec.Code != 404 {
		t.Errorf("delete again: %d", rec.Code)
	}
}

// Admins only: a user who is not an admin is sent to /, for every route.
func TestAccountsRoutesNeedAdmin(t *testing.T) {
	t.Parallel()
	h, _, bob := accountsUI(t)
	for _, r := range []struct{ method, path string }{
		{"GET", "/accounts"}, {"GET", "/ui/accounts/new"}, {"POST", "/ui/accounts"},
		{"GET", "/ui/accounts/1/edit"}, {"POST", "/ui/accounts/1"}, {"DELETE", "/ui/accounts/1"},
	} {
		rec := send(h, r.method, r.path, url.Values{"name": {"x"}}, bob, true)
		if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/" {
			t.Errorf("%s %s as Bob: %d %v", r.method, r.path, rec.Code, rec.Header())
		}
		rec = send(h, r.method, r.path, url.Values{"name": {"x"}}, nil, true)
		if rec.Code != 200 || rec.Header().Get("HX-Redirect") != "/login" {
			t.Errorf("%s %s without a session: %d %v", r.method, r.path, rec.Code, rec.Header())
		}
	}
}

// M1.
func TestMyAccount(t *testing.T) {
	t.Parallel()
	h, _, bob := accountsUI(t)

	body := serve(h, "GET", "/my-account", bob, false).Body.String()
	wantContains(t, "page", body, "<title>My Account · Tmock</title>",
		`<form class="form" method="post" action="/my-account" id="my-account-form" hx-post="/my-account" hx-target="this" hx-swap="outerHTML" x-data="passwordConfirm" x-on:input="check">`,
		`value="Bob"`, `value="bob@example.com"`, `name="currentPassword" type="password"`, `name="newPassword" type="password"`,
		`name="confirmPassword" type="password"`,
		`<p class="inline-error" role="alert" hidden x-bind:hidden="matches">The new passwords do not match.</p>`,
		`x-bind:disabled="mismatch">Save</button>`)
	assertNoEval(t, body)

	// The server checks the confirmation too.
	form := url.Values{"name": {"Bob"}, "email": {"bob@example.com"}, "currentPassword": {"s3cret-pass"}, "newPassword": {"new-password"}, "confirmPassword": {"other-password"}}
	rec := send(h, "POST", "/my-account", form, bob, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="my-confirmPassword-message">does not match the new password</p>`) {
		t.Errorf("mismatch: %d %s", rec.Code, rec.Body.String())
	}
	// A wrong current password.
	form.Set("confirmPassword", "new-password")
	form.Set("currentPassword", "wrong")
	rec = send(h, "POST", "/my-account", form, bob, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="my-currentPassword-message">is not correct</p>`) {
		t.Errorf("wrong current: %d %s", rec.Code, rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `value="new-password"`) || strings.Contains(rec.Body.String(), `value="wrong"`) {
		t.Error("a password came back in the form")
	}
	// Success: the name and the password change; the password fields come back empty.
	form.Set("currentPassword", "s3cret-pass")
	form.Set("name", "Robert")
	rec = send(h, "POST", "/my-account", form, bob, true)
	if rec.Code != 200 || triggers(t, rec)["toast"]["title"] != "Your account was saved" || !strings.Contains(rec.Body.String(), `value="Robert"`) {
		t.Fatalf("save: %d %v %s", rec.Code, rec.Header(), rec.Body.String())
	}
	if strings.Contains(rec.Body.String(), `value="new-password"`) || strings.Contains(rec.Body.String(), "s3cret-pass") {
		t.Error("the password fields were not cleared")
	}
	if rec := postLogin(h, "bob@example.com", "new-password", false); rec.Code != 303 {
		t.Errorf("login with the new password: %d", rec.Code)
	}
	// A duplicate email.
	rec = send(h, "POST", "/my-account", url.Values{"name": {"Robert"}, "email": {"Ann@example.com"}}, bob, true)
	if rec.Code != 422 || !strings.Contains(rec.Body.String(), `id="my-email-message">A user with email &#34;Ann@example.com&#34; already exists</p>`) {
		t.Errorf("duplicate email: %d %s", rec.Code, rec.Body.String())
	}
	// Without JavaScript a save redirects back.
	if rec := send(h, "POST", "/my-account", url.Values{"name": {"Rob"}, "email": {"bob@example.com"}}, bob, false); rec.Code != 303 || rec.Header().Get("Location") != "/my-account" {
		t.Errorf("no-JS save: %d %v", rec.Code, rec.Header())
	}
}
