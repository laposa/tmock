package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ChipProps configures a Chip.
type ChipProps struct {
	Strong      string   // shown in bold first, such as a service path
	Text        string   // shown after a separator, such as a scenario name
	RemoveLabel string   // the ✕ button's name for screen readers; no button when empty
	RemoveAttrs []g.Node // hx-* of the ✕ button
}

// Chip is a small rounded label, "**Strong** | Text", with an optional ✕ that removes it
// (legacy v-chip closable).
func Chip(p ChipProps) g.Node {
	return h.Span(h.Class("chip"),
		h.Span(h.Class("chip__text"),
			g.If(p.Strong != "", h.Strong(h.Class("chip__strong"), g.Text(p.Strong))),
			g.If(p.Strong != "" && p.Text != "", g.Text(" | ")),
			g.Text(p.Text),
		),
		g.If(p.RemoveLabel != "", h.Button(h.Type("button"), h.Class("chip__remove"), h.Aria("label", p.RemoveLabel),
			attrs(p.RemoveAttrs), g.Text("✕"))),
	)
}
