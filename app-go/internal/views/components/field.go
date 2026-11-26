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
	id := fieldID(p.ID, p.Name)
	typ := p.Type
	if typ == "" {
		typ = "text"
	}
	msg, describe := fieldMessage(id, p.Hint, p.Error)
	return fieldFrame(id, p.Label,
		h.Input(
			h.Class("field__input"),
			h.ID(id), h.Name(p.Name), h.Type(typ),
			g.If(p.Value != "", h.Value(p.Value)),
			g.If(p.Required, h.Required()),
			g.If(p.Autofocus, h.AutoFocus()),
			g.If(p.Autocomplete != "", h.AutoComplete(p.Autocomplete)),
			describe,
			attrs(p.Attrs),
		),
		msg,
	)
}

// fieldFrame is the frame that every labelled form control shares (TextField, Select,
// CodeTextarea): the label, then the control, then its message.
func fieldFrame(id, label string, control, msg g.Node) g.Node {
	return h.Div(h.Class("field"),
		h.Label(h.Class("field__label"), h.For(id), g.Text(label)),
		control,
		msg,
	)
}

// fieldMessage is the line under a control: its error or, without one, its hint. It
// also returns the attributes that mark the control invalid and point it at the line.
func fieldMessage(id, hint, errMsg string) (msg, controlAttrs g.Node) {
	msgID := id + "-message"
	text, class := hint, "field__hint"
	if errMsg != "" {
		text, class = errMsg, "field__error"
	}
	return g.If(text != "", h.P(h.Class(class), h.ID(msgID), g.Text(text))),
		g.Group{
			g.If(errMsg != "", h.Aria("invalid", "true")),
			g.If(text != "", h.Aria("describedby", msgID)),
		}
}

// fieldID is the id of a control: id if set, otherwise its name.
func fieldID(id, name string) string {
	if id != "" {
		return id
	}
	return name
}
