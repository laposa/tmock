package layout

import (
	"regexp"
	"strings"
	"testing"

	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/static"
)

func render(t *testing.T, n g.Node) string {
	t.Helper()
	var b strings.Builder
	if err := n.Render(&b); err != nil {
		t.Fatal(err)
	}
	return b.String()
}

var navLink = regexp.MustCompile(`<a class="nav__link" href="([^"]+)"( aria-current="page")?>([^<]+)</a>`)

// navEntries lists the navigation as "label href" pairs, with a * on the current one.
func navEntries(html string) []string {
	var out []string
	for _, m := range navLink.FindAllStringSubmatch(html, -1) {
		e := m[3] + " " + m[1]
		if m[2] != "" {
			e += "*"
		}
		out = append(out, e)
	}
	return out
}

// Verify item: the navigation for admins and non-admins (spec §10.3 Layout).
func TestNav(t *testing.T) {
	admin := render(t, Nav(Scenarios, true))
	want := "Services /services|Clients /|Scenarios /scenarios*|Accounts /accounts|My Account /my-account"
	if got := strings.Join(navEntries(admin), "|"); got != want {
		t.Errorf("admin nav\n got %s\nwant %s", got, want)
	}
	user := render(t, Nav(Clients, false))
	want = "Services /services|Clients /*|Scenarios /scenarios|My Account /my-account"
	if got := strings.Join(navEntries(user), "|"); got != want {
		t.Errorf("user nav\n got %s\nwant %s", got, want)
	}
	for name, html := range map[string]string{"admin": admin, "user": user} {
		if !strings.HasSuffix(html, `<form class="nav__form" method="post" action="/logout" hx-post="/logout" hx-confirm="Are you sure you want to log out?"><button class="nav__link nav__link--button" type="submit">Logout</button></form></nav>`) {
			t.Errorf("%s nav does not end with the logout form: %s", name, html)
		}
	}
}

// The page regions come in this order: header (brand, nav), main, toasts, modal host,
// confirmation dialog.
func TestPageRegions(t *testing.T) {
	html := render(t, Page(Props{Title: "Services", Current: Services, Body: []g.Node{g.Text("[content]")}}))
	order := []string{`<title>Services · Tmock</title>`, `class="app-header"`, `class="brand brand--link"`, `class="nav"`,
		`<main class="main">[content]</main>`, `id="toasts"`, `id="modal"`, `id="confirm"`}
	pos := 0
	for _, s := range order {
		i := strings.Index(html[pos:], s)
		if i < 0 {
			t.Fatalf("%q missing or out of order in %s", s, html)
		}
		pos += i + len(s)
	}
}

func TestBare(t *testing.T) {
	html := render(t, Bare(BareProps{Title: "Login", Body: []g.Node{g.Text("[form]")}}))
	for _, want := range []string{`<title>Login · Tmock</title>`, `<main class="centered"><div class="centered__panel">[form]</div></main>`, `id="toasts"`} {
		if !strings.Contains(html, want) {
			t.Errorf("missing %q", want)
		}
	}
	if strings.Contains(html, `class="nav"`) {
		t.Error("bare page has navigation")
	}
}

func TestHead(t *testing.T) {
	html := render(t, Bare(BareProps{}))
	if !strings.Contains(html, "<title>Tmock</title>") {
		t.Error("empty title is not just Tmock")
	}
	if !strings.Contains(html, `<meta name="htmx-config" content="{&#34;includeIndicatorCSS&#34;:false,&#34;history&#34;:false}">`) {
		t.Error("htmx config missing")
	}
	// Scripts load in the order spec §10.1 requires, all deferred.
	re := regexp.MustCompile(`<script src="([^"]+)" defer></script>`)
	var got []string
	for _, m := range re.FindAllStringSubmatch(html, -1) {
		got = append(got, m[1])
	}
	want := []string{"/static/" + static.HTMX, "/static/" + static.HTMXAlpineCompat, "/static/app.js", "/static/" + static.Alpine}
	if strings.Join(got, " ") != strings.Join(want, " ") {
		t.Errorf("scripts\n got %v\nwant %v", got, want)
	}
}
