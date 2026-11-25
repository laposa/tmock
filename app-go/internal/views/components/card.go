package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// CardProps configures a Card.
type CardProps struct {
	Title string // optional
	Body  []g.Node
	Attrs []g.Node
}

// Card is a raised surface with an optional title (legacy v-card).
func Card(p CardProps) g.Node {
	return h.Section(h.Class("card"), attrs(p.Attrs),
		g.If(p.Title != "", h.H2(h.Class("card__title"), g.Text(p.Title))),
		h.Div(h.Class("card__body"), g.Group(p.Body)),
	)
}
