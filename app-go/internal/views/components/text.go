package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// TextProps configures a Text.
type TextProps struct {
	Muted    bool // secondary information
	Children []g.Node
	Attrs    []g.Node
}

// Text is a paragraph.
func Text(p TextProps) g.Node {
	c := "text"
	if p.Muted {
		c = "text text--muted"
	}
	return h.P(h.Class(c), attrs(p.Attrs), g.Group(p.Children))
}
