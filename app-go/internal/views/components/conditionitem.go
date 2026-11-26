package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ConditionKind is the kind of a ConditionItem, which picks its colour.
type ConditionKind int

// The kinds, as in the legacy builder.
const (
	ConditionIP ConditionKind = iota
	ConditionCIDR
	ConditionHeader
	ConditionHeaderRegex
)

// ConditionInput is one of an item's text inputs.
type ConditionInput struct {
	Label       string // for screen readers ("Header name")
	Name        string
	Value       string
	Placeholder string
	Error       string
}

// ConditionItemProps configures a ConditionItem.
type ConditionItemProps struct {
	Kind      ConditionKind
	KindLabel string          // "IP Address", "Header Regex" …
	Header    *ConditionInput // the header name; nil for IP and CIDR
	// Operator is the chip between the header and the value ("Equals", "Not Matches"),
	// which toggles NOT.
	Operator      string
	Not           bool
	OperatorAttrs []g.Node
	Value         ConditionInput
	RemoveLabel   string
	RemoveAttrs   []g.Node
	Hidden        []g.Node // the item's form state (Hidden inputs)
}

// ConditionItem is one expression of the condition builder (spec §10.5), after the
// legacy ClientConditionItem.vue: [Kind] [header] [Equals|Matches|Not …] [value] [✕],
// with the problems of its inputs underneath.
func ConditionItem(p ConditionItemProps) g.Node {
	kind := "condition-item__kind"
	switch p.Kind {
	case ConditionCIDR:
		kind += " condition-item__kind--cidr"
	case ConditionHeader:
		kind += " condition-item__kind--header"
	case ConditionHeaderRegex:
		kind += " condition-item__kind--header-regex"
	default:
		kind += " condition-item__kind--ip"
	}
	pressed := "false"
	if p.Not {
		pressed = "true"
	}
	var errs []g.Node
	input := func(in ConditionInput) g.Node {
		id := in.Name
		if in.Error != "" {
			errs = append(errs, h.P(h.Class("condition-item__error"), h.ID(id+"-message"), g.Text(in.Label+": "+in.Error)))
		}
		return h.Input(h.Class("condition-item__input"), h.Type("text"), h.ID(id), h.Name(in.Name),
			g.If(in.Value != "", h.Value(in.Value)),
			g.If(in.Placeholder != "", h.Placeholder(in.Placeholder)),
			h.Aria("label", in.Label),
			g.If(in.Error != "", g.Group{h.Aria("invalid", "true"), h.Aria("describedby", id+"-message")}),
		)
	}
	var header g.Node
	if p.Header != nil {
		header = input(*p.Header)
	}
	value := input(p.Value)
	return h.Div(h.Class("condition-item"),
		g.Group(p.Hidden),
		h.Div(h.Class("condition-item__row"),
			h.Span(h.Class(kind), g.Text(p.KindLabel)),
			header,
			h.Button(h.Type("submit"), h.Class("condition-item__operator"), h.Aria("pressed", pressed),
				h.Title("Negate this expression"), attrs(p.OperatorAttrs), g.Text(p.Operator)),
			value,
			g.If(p.RemoveLabel != "", h.Button(h.Type("submit"), h.Class("condition-item__remove"),
				h.Aria("label", p.RemoveLabel), attrs(p.RemoveAttrs), g.Text("✕"))),
		),
		g.Group(errs),
	)
}
