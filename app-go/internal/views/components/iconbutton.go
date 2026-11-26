package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// Icons an IconButton can show. They are text, so they need no image files.
const (
	IconAdd  = "⊕"
	IconInfo = "ⓘ"
)

// IconButtonProps configures an IconButton.
type IconButtonProps struct {
	Label string // the button's name for screen readers (aria-label) and its tooltip
	Icon  string // IconAdd, IconInfo, …
	Attrs []g.Node
}

// IconButton is a round button that shows only an icon (legacy v-icon buttons).
func IconButton(p IconButtonProps) g.Node {
	return h.Button(h.Type("button"), h.Class("icon-button"), h.Aria("label", p.Label), h.Title(p.Label),
		attrs(p.Attrs), h.Span(h.Aria("hidden", "true"), g.Text(p.Icon)))
}
