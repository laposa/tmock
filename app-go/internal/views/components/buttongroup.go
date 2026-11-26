package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ButtonGroup lays out buttons side by side, in the order given, such as a table cell's
// "Regenerate" and "Revoke".
func ButtonGroup(buttons ...g.Node) g.Node {
	return h.Div(h.Class("button-group"), g.Group(buttons))
}
