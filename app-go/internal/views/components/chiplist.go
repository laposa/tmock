package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ChipListProps configures a ChipList.
type ChipListProps struct {
	Chips []g.Node // Chip components
	End   g.Node   // after the chips, such as an IconButton that adds one
}

// ChipList lays out chips in a wrapping row, followed by End.
func ChipList(p ChipListProps) g.Node {
	return h.Div(h.Class("chip-list"), g.Group(p.Chips), p.End)
}
