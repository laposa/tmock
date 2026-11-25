package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// TextFieldProps configures a TextField.
type TextFieldProps struct {
	Label        string
	Name         string // also the id, unless ID is set
	ID           string
	Type         string // "text" (default), "email", "password", "number", …
	Value        string
	Error        string // shown under the input; marks it invalid
	Hint         string // shown under the input when there is no error
	Required     bool
	Autofocus    bool
	Autocomplete string
	Attrs        []g.Node // on the <input>
}

// TextField is a labelled <input> with its error or hint underneath.
func TextField(p TextFieldProps) g.Node {
	id := p.ID
	if id == "" {
		id = p.Name
	}
	typ := p.Type
	if typ == "" {
		typ = "text"
	}
	msgID := id + "-message"
	msg, msgClass := p.Hint, "field__hint"
	if p.Error != "" {
		msg, msgClass = p.Error, "field__error"
	}
	return h.Div(h.Class("field"),
		h.Label(h.Class("field__label"), h.For(id), g.Text(p.Label)),
		h.Input(
			h.Class("field__input"),
			h.ID(id), h.Name(p.Name), h.Type(typ),
			g.If(p.Value != "", h.Value(p.Value)),
			g.If(p.Required, h.Required()),
			g.If(p.Autofocus, h.AutoFocus()),
			g.If(p.Autocomplete != "", h.AutoComplete(p.Autocomplete)),
			g.If(p.Error != "", h.Aria("invalid", "true")),
			g.If(msg != "", h.Aria("describedby", msgID)),
			attrs(p.Attrs),
		),
		g.If(msg != "", h.P(h.Class(msgClass), h.ID(msgID), g.Text(msg))),
	)
}
