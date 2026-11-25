package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// OutputProps configures an Output.
type OutputProps struct {
	ID    string
	Attrs []g.Node
}

// Output is a place for a result that htmx loads, announced politely to screen readers.
func Output(p OutputProps) g.Node {
	return h.Output(h.Class("output"), h.ID(p.ID), h.Aria("live", "polite"), attrs(p.Attrs))
}
