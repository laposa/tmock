package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// SelectOption is one choice of a Select.
type SelectOption struct {
	Value string
	Label string // shown; the value when empty
}

// SelectProps configures a Select.
type SelectProps struct {
	Label       string
	Name        string // also the id, unless ID is set
	ID          string
	Options     []SelectOption
	Value       string // the selected option's value
	Placeholder string // an empty first option, such as "Select a service"
	Error       string
	Hint        string
	Required    bool
	Attrs       []g.Node // on the <select>
}

// Select is a labelled <select> with its error or hint underneath, framed like a
// TextField.
func Select(p SelectProps) g.Node {
	id := fieldID(p.ID, p.Name)
	msg, describe := fieldMessage(id, p.Hint, p.Error)
	return fieldFrame(id, p.Label,
		h.Select(h.Class("select"), h.ID(id), h.Name(p.Name),
			g.If(p.Required, h.Required()),
			describe,
			attrs(p.Attrs),
			g.If(p.Placeholder != "", h.Option(h.Value(""), g.If(p.Value == "", h.Selected()), g.Text(p.Placeholder))),
			g.Map(p.Options, func(o SelectOption) g.Node {
				label := o.Label
				if label == "" {
					label = o.Value
				}
				return h.Option(h.Value(o.Value), g.If(o.Value == p.Value, h.Selected()), g.Text(label))
			}),
		),
		msg,
	)
}
