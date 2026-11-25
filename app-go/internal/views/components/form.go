package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// FormProps configures a Form.
type FormProps struct {
	Method  string // "post" (default) or "get"
	Action  string // where a plain (non-htmx) submit goes
	Error   string // a message about the whole form, shown above the fields
	Fields  []g.Node
	Actions []g.Node // buttons, in reading order
	Attrs   []g.Node // hx-*, x-* …
}

// Form is a <form> that also works without JavaScript (spec §10.1). It places, in this
// order: the form error, the fields, the actions.
func Form(p FormProps) g.Node {
	method := p.Method
	if method == "" {
		method = "post"
	}
	return h.Form(h.Class("form"),
		h.Method(method),
		g.If(p.Action != "", h.Action(p.Action)),
		attrs(p.Attrs),
		g.If(p.Error != "", Alert(AlertProps{Type: AlertError, Message: p.Error})),
		g.Group(p.Fields),
		g.If(len(p.Actions) > 0, h.Div(h.Class("form__actions"), g.Group(p.Actions))),
	)
}
