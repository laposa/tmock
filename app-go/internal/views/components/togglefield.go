package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ToggleFieldProps configures a ToggleField.
type ToggleFieldProps struct {
	ID          string // on the outer element, for example to scroll to it
	Label       string // the switch's name, shown while the field is off
	Name        string // the switch's form name: sent as "on" while the field is on
	On          bool
	Control     []g.Node // the field itself, shown while the switch is on
	SwitchAttrs []g.Node
}

// ToggleField is an optional field with an enable switch, as in the legacy scenario
// editor: while the switch is off, only its label shows; while it is on, the control
// does. CSS does the showing and hiding (:has), so it also works without JavaScript. A
// control that is hidden is still submitted; the server ignores it while the switch is
// off.
func ToggleField(p ToggleFieldProps) g.Node {
	return h.Div(h.Class("toggle-field"), g.If(p.ID != "", h.ID(p.ID)),
		h.Div(h.Class("toggle-field__switch"),
			Switch(SwitchProps{Label: p.Label, HideLabel: true, Name: p.Name, Checked: p.On, Attrs: p.SwitchAttrs})),
		h.Span(h.Class("toggle-field__label"), h.Aria("hidden", "true"), g.Text(p.Label)),
		h.Div(h.Class("toggle-field__control"), g.Group(p.Control)),
	)
}
