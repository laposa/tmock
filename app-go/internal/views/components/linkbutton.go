package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// LinkButtonProps configures a LinkButton.
type LinkButtonProps struct {
	Label string
	Attrs []g.Node // hx-* that say what the click does
}

// LinkButton is a button that looks like a link, such as a name in a table that opens
// its edit modal (legacy span.edit). It is a <button>, because it does something on the
// page rather than go to another one.
func LinkButton(p LinkButtonProps) g.Node {
	return h.Button(h.Type("button"), h.Class("link-button"), attrs(p.Attrs), g.Text(p.Label))
}
