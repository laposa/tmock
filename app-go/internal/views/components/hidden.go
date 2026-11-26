package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// HiddenProps configures a Hidden input.
type HiddenProps struct {
	Name  string
	Value string
}

// Hidden is an <input type="hidden">: state a form posts back, such as the kind of a node
// of the condition builder.
func Hidden(p HiddenProps) g.Node {
	return h.Input(h.Type("hidden"), h.Name(p.Name), h.Value(p.Value))
}
