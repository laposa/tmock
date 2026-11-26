package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// MenuItem is one choice of a Menu.
type MenuItem struct {
	Label string
	Attrs []g.Node // on the item's <button>: name, value, formaction, hx-* …
	// Submit makes the item a submit button, so it also works in a form without
	// JavaScript; otherwise it is type="button".
	Submit bool
}

// MenuProps configures a Menu.
type MenuProps struct {
	Label string // the button that opens the menu
	Items []MenuItem
}

// Menu is a button that opens a list of choices (legacy v-menu), built on a native
// <details> so it needs no JavaScript.
func Menu(p MenuProps) g.Node {
	return h.Details(h.Class("menu"),
		h.Summary(h.Class("menu__button"), g.Text(p.Label)),
		h.Div(h.Class("menu__list"), h.Role("menu"),
			g.Map(p.Items, func(it MenuItem) g.Node {
				typ := "button"
				if it.Submit {
					typ = "submit"
				}
				return h.Button(h.Type(typ), h.Class("menu__item"), h.Role("menuitem"), attrs(it.Attrs), g.Text(it.Label))
			}),
		),
	)
}
