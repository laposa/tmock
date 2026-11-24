// Package layout renders the page shell shared by every full page (spec §10).
package layout

import (
	g "maragu.dev/gomponents"
	c "maragu.dev/gomponents/components"
	h "maragu.dev/gomponents/html"

	"github.com/laposa/tmock/app-go/static"
)

// HTMXConfig is the htmx 4 configuration (spec §10.1). It goes in a <meta> tag because
// htmx reads it while its script loads, before any other script can run; a meta tag is not
// a script, so the strict CSP is unaffected.
const HTMXConfig = `{"includeIndicatorCSS":false,"history":false}`

// Props configures a page.
type Props struct {
	Title string   // shown as "<Title> · Tmock"; empty means just "Tmock"
	Body  []g.Node // content of <main>
}

// Page renders a complete HTML document with the layout shell.
func Page(p Props) g.Node {
	title := "Tmock"
	if p.Title != "" {
		title = p.Title + " · Tmock"
	}
	return c.HTML5(c.HTML5Props{
		Title:    title,
		Language: "en",
		Head: g.Group{
			h.Meta(h.Name("htmx-config"), h.Content(HTMXConfig)),
			h.Link(h.Rel("icon"), h.Href(asset("favicon.ico"))),
			h.Link(h.Rel("stylesheet"), h.Href(asset("app.css"))),
			// Order matters (spec §10.1). All are deferred, which keeps document order:
			// htmx, its Alpine extension, our components, then Alpine itself.
			h.Script(h.Src(asset(static.HTMX)), h.Defer()),
			h.Script(h.Src(asset(static.HTMXAlpineCompat)), h.Defer()),
			h.Script(h.Src(asset("app.js")), h.Defer()),
			h.Script(h.Src(asset(static.Alpine)), h.Defer()),
		},
		Body: g.Group{
			header(),
			h.Main(h.Class("app-main"), g.Group(p.Body)),
		},
	})
}

func header() g.Node {
	return h.Header(h.Class("app-header"),
		h.Div(h.Class("app-header__inner"),
			h.A(h.Class("app-brand"), h.Href("/"),
				h.Img(h.Src(asset("logo.svg")), h.Alt("")),
				g.Text("Tmock"),
			),
			// Navigation arrives with the pages it links to (from I04 on).
			h.Nav(h.Class("app-nav"), h.Aria("label", "Main")),
		),
	)
}

func asset(name string) string { return "/static/" + name }
