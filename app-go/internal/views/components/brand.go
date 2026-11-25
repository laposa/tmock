package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// BrandProps configures a Brand.
type BrandProps struct {
	Href    string // link target; empty renders the brand without a link (login page)
	LogoSrc string // URL of the logo image
	Heading bool   // render the name as the page's <h1> (login page)
}

// Brand is the logo and the name "Tmock".
func Brand(p BrandProps) g.Node {
	logo := h.Img(h.Class("brand__logo"), h.Src(p.LogoSrc), h.Alt(""))
	var name g.Node = h.Span(h.Class("brand__name"), g.Text("Tmock"))
	if p.Heading {
		name = h.H1(h.Class("brand__name"), g.Text("Tmock"))
	}
	if p.Href == "" {
		return h.Div(h.Class("brand"), logo, name)
	}
	return h.A(h.Class("brand brand--link"), h.Href(p.Href), logo, name)
}
