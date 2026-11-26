package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// PopoverProps configures a Popover.
type PopoverProps struct {
	ID           string // the panel's id; point the trigger at it with aria-describedby
	Trigger      g.Node // shows the panel while hovered or focused
	Text         string
	Preformatted bool // keep white space and use a monospace font (JSON, bodies)
}

// Popover shows Text beside its trigger while the trigger is hovered or has focus
// (legacy v-tooltip location="end"). The panel is a native popover, so it is drawn
// above the page and is not clipped by a scrolling table; the Alpine component
// "popover" in app.js opens it and places it. Without JavaScript it stays hidden.
func Popover(p PopoverProps) g.Node {
	text := g.Node(h.P(h.Class("popover__text"), g.Text(p.Text)))
	if p.Preformatted {
		text = h.Pre(h.Class("popover__text popover__text--pre"), g.Text(p.Text))
	}
	return h.Div(h.Class("popover"), g.Attr("x-data", "popover"),
		g.Attr("x-on:mouseenter", "show"), g.Attr("x-on:mouseleave", "hide"),
		g.Attr("x-on:focusin", "show"), g.Attr("x-on:focusout", "hide"),
		p.Trigger,
		h.Div(h.Class("popover__panel"), h.ID(p.ID), g.Attr("popover", "manual"), h.Role("tooltip"), text),
	)
}
