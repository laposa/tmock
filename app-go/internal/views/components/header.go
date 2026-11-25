package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// AppHeaderProps configures an AppHeader.
type AppHeaderProps struct {
	Brand g.Node // usually a Brand
	Nav   g.Node // usually a Nav
}

// AppHeader is the bar at the top of every logged-in page: the brand, then the
// navigation (legacy MainNavigation).
func AppHeader(p AppHeaderProps) g.Node {
	return h.Header(h.Class("app-header"), p.Brand, p.Nav)
}
