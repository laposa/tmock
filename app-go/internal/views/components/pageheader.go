package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// PageHeaderProps configures a PageHeader.
type PageHeaderProps struct {
	Title   string
	Actions []g.Node // for example an "Add Service" button
}

// PageHeader is a page's title with its actions beside it (legacy .heading): the title
// first, then the actions.
func PageHeader(p PageHeaderProps) g.Node {
	return h.Div(h.Class("page-header"),
		h.H1(h.Class("page-header__title"), g.Text(p.Title)),
		g.If(len(p.Actions) > 0, h.Div(h.Class("page-header__actions"), g.Group(p.Actions))),
	)
}
