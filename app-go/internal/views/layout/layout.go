// Package layout renders the page shells (spec §10.3, §10.7). It places the shell
// components and the page's own content, and so decides the order of the page regions;
// it sets no classes of its own.
package layout

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
	"github.com/laposa/tmock/app-go/static"
)

// HTMXConfig is the htmx 4 configuration (spec §10.1). It goes in a <meta> tag because
// htmx reads it while its script loads, before any other script can run; a meta tag is not
// a script, so the strict CSP is unaffected.
const HTMXConfig = `{"includeIndicatorCSS":false,"history":false}`

// Section identifies a page in the navigation.
type Section string

// The sections of the dashboard, in the order of the legacy menu.
const (
	Services  Section = "/services"
	Clients   Section = "/"
	Scenarios Section = "/scenarios"
	Accounts  Section = "/accounts"
	MyAccount Section = "/my-account"
)

// Props configures a logged-in page.
type Props struct {
	Title   string  // shown as "<Title> · Tmock"
	Current Section // highlighted in the navigation
	Admin   bool    // shows the Accounts entry
	Body    []g.Node
}

// Page renders a logged-in page: the header with the navigation, the page content, and
// the toast region and dialogs that every page shares.
func Page(p Props) g.Node {
	return document(p.Title,
		components.Frame(
			components.AppHeader(components.AppHeaderProps{
				Brand: components.Brand(components.BrandProps{Href: "/", LogoSrc: Asset("logo.svg")}),
				Nav:   Nav(p.Current, p.Admin),
			}),
			components.Main(p.Body...),
		),
		components.ToastRegion(),
		components.ModalHost(),
		components.ConfirmDialog(),
	)
}

// LogoutQuestion is what the confirmation dialog asks before logging out.
const LogoutQuestion = "Are you sure you want to log out?"

// Nav is the main navigation (spec §10.3 Layout): Services, Clients, Scenarios, Accounts
// (admins only), My Account, Logout.
func Nav(current Section, admin bool) g.Node {
	item := func(label string, s Section) components.NavItem {
		return components.NavItem{Label: label, Href: string(s), Current: s == current}
	}
	items := []components.NavItem{item("Services", Services), item("Clients", Clients), item("Scenarios", Scenarios)}
	if admin {
		items = append(items, item("Accounts", Accounts))
	}
	items = append(items, item("My Account", MyAccount))
	return components.Nav(components.NavProps{
		Label: "Main",
		Items: items,
		Forms: []components.NavForm{{
			Label: "Logout", Action: "/logout",
			// Asks first, in the confirmation dialog; without JavaScript the form logs out
			// at once.
			Attrs: []g.Node{hx.Post("/logout"), hx.Confirm(LogoutQuestion)},
		}},
	})
}

// BareProps configures a page without navigation.
type BareProps struct {
	Title string
	Body  []g.Node
}

// Bare renders a page without navigation, centred in the window: the login page.
func Bare(p BareProps) g.Node {
	return document(p.Title, components.Centered(p.Body...), components.ToastRegion())
}

// document is the HTML document around a page's body.
func document(title string, body ...g.Node) g.Node {
	full := "Tmock"
	if title != "" {
		full = title + " · Tmock"
	}
	return c.HTML5(c.HTML5Props{
		Title:    full,
		Language: "en",
		Head: g.Group{
			h.Meta(h.Name("htmx-config"), h.Content(HTMXConfig)),
			h.Link(h.Rel("icon"), h.Href(Asset("favicon.ico"))),
			h.Link(h.Rel("stylesheet"), h.Href(Asset("app.css"))),
			// Order matters (spec §10.1). All are deferred, which keeps document order:
			// htmx, its Alpine extension, our components, then Alpine itself.
			h.Script(h.Src(Asset(static.HTMX)), h.Defer()),
			h.Script(h.Src(Asset(static.HTMXAlpineCompat)), h.Defer()),
			h.Script(h.Src(Asset("app.js")), h.Defer()),
			h.Script(h.Src(Asset(static.Alpine)), h.Defer()),
		},
		Body: g.Group(body),
	})
}

// Asset is the URL of an embedded static file.
func Asset(name string) string { return "/static/" + name }
