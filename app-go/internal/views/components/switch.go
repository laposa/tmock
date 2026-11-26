package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// SwitchProps configures a Switch.
type SwitchProps struct {
	Label     string
	HideLabel bool // the label is only for screen readers (a ToggleField shows its own)
	Name      string
	ID        string
	Value     string // sent when on; the browser sends "on" when empty
	Checked   bool
	Disabled  bool
	Error     string   // shown under the switch
	Attrs     []g.Node // on the <input>
}

// Switch is an on/off control: a checkbox styled as the legacy indigo v-switch, with
// role="switch". It submits like a checkbox: Name=Value when on, nothing when off.
func Switch(p SwitchProps) g.Node {
	labelClass := "switch__label"
	if p.HideLabel {
		labelClass = "switch__label switch__label--hidden"
	}
	errID := ""
	if p.Error != "" && p.ID != "" {
		errID = p.ID + "-message"
	}
	return h.Div(h.Class("switch"),
		h.Label(h.Class("switch__control"),
			h.Input(h.Class("switch__input"), h.Type("checkbox"), h.Role("switch"),
				g.If(p.ID != "", h.ID(p.ID)),
				g.If(p.Name != "", h.Name(p.Name)),
				g.If(p.Value != "", h.Value(p.Value)),
				g.If(p.Checked, h.Checked()),
				g.If(p.Disabled, h.Disabled()),
				g.If(p.Error != "", h.Aria("invalid", "true")),
				g.If(errID != "", h.Aria("describedby", errID)),
				attrs(p.Attrs),
			),
			h.Span(h.Class("switch__track"), h.Aria("hidden", "true")),
			h.Span(h.Class(labelClass), g.Text(p.Label)),
		),
		g.If(p.Error != "", h.P(h.Class("switch__error"), g.If(errID != "", h.ID(errID)), g.Text(p.Error))),
	)
}
