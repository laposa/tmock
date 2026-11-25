package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// NavItem is a link in the main navigation.
type NavItem struct {
	Label   string
	Href    string
	Current bool // the page being shown (aria-current="page")
}

// NavForm is a navigation entry that submits a POST, such as Logout: changing state
// with a plain link would let any page log the user out.
type NavForm struct {
	Label  string
	Action string
	Attrs  []g.Node // on the <form>: hx-post, hx-confirm …
}

// NavProps configures a Nav.
type NavProps struct {
	Label string // accessible name, for example "Main"
	Items []NavItem
	Forms []NavForm // placed after the links
}

// Nav is the main navigation: the links, then the form entries.
func Nav(p NavProps) g.Node {
	return h.Nav(h.Class("nav"), h.Aria("label", p.Label),
		g.Map(p.Items, func(it NavItem) g.Node {
			return h.A(h.Class("nav__link"), h.Href(it.Href),
				g.If(it.Current, h.Aria("current", "page")),
				g.Text(it.Label))
		}),
		g.Map(p.Forms, func(f NavForm) g.Node {
			return h.Form(h.Class("nav__form"), h.Method("post"), h.Action(f.Action), attrs(f.Attrs),
				h.Button(h.Class("nav__link nav__link--button"), h.Type("submit"), g.Text(f.Label)))
		}),
	)
}
