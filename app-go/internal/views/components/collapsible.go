package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// CollapsibleProps configures a Collapsible.
type CollapsibleProps struct {
	Title string
	Meta  string // secondary text after the title, such as a service's path
	Open  bool
	Body  []g.Node
	Attrs []g.Node // on the <details>
}

// Collapsible is a group that opens and closes (<details>), such as one service's
// scenarios (legacy group-by rows). It needs no JavaScript.
func Collapsible(p CollapsibleProps) g.Node {
	return h.Details(h.Class("collapsible"), g.If(p.Open, h.Open()), attrs(p.Attrs),
		h.Summary(h.Class("collapsible__summary"),
			h.Span(h.Class("collapsible__title"), g.Text(p.Title)),
			g.If(p.Meta != "", h.Span(h.Class("collapsible__meta"), g.Text(p.Meta))),
		),
		h.Div(h.Class("collapsible__body"), g.Group(p.Body)),
	)
}
