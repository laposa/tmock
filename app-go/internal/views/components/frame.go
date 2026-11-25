package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// Frame is the centred, width-limited column that holds a logged-in page (legacy #app).
func Frame(children ...g.Node) g.Node {
	return h.Div(h.Class("frame"), g.Group(children))
}

// Main is the page's main content (<main>).
func Main(children ...g.Node) g.Node {
	return h.Main(h.Class("main"), g.Group(children))
}

// Centered is a narrow panel in the middle of the viewport, for the login page.
func Centered(children ...g.Node) g.Node {
	return h.Main(h.Class("centered"), h.Div(h.Class("centered__panel"), g.Group(children)))
}
