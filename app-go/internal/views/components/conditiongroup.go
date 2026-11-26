package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ConditionGroupProps configures a ConditionGroup.
type ConditionGroupProps struct {
	Not      bool     // the group is negated: NOT ( … )
	NotAttrs []g.Node // on the NOT chip, which toggles it
	Or       bool     // OR between the children; AND otherwise
	// OperatorAttrs are on each AND/OR chip between the children, which toggles it.
	OperatorAttrs []g.Node
	Children      []g.Node // ConditionGroups and ConditionItems
	Add           g.Node   // the "Add Expression" menu
	RemoveLabel   string   // the ✕ chip's name for screen readers; no ✕ when empty (the top-level group)
	RemoveAttrs   []g.Node
	Hidden        []g.Node // the group's form state (Hidden inputs)
	Error         string   // a problem with the group itself
}

// ConditionGroup is a group of the condition builder (spec §10.5), after the legacy
// ClientConditionGroup.vue: a NOT chip, "(", the children with an AND/OR chip between
// each two, the add menu, ")" and a ✕. The chips are submit buttons, so the builder
// also works without JavaScript.
func ConditionGroup(p ConditionGroupProps) g.Node {
	op, opClass := "AND", "condition-group__operator"
	if p.Or {
		op, opClass = "OR", "condition-group__operator condition-group__operator--or"
	}
	notPressed := "false"
	if p.Not {
		notPressed = "true"
	}
	var body []g.Node
	for i, ch := range p.Children {
		body = append(body, h.Div(h.Class("condition-group__child"), ch))
		if i < len(p.Children)-1 {
			body = append(body, h.Button(h.Type("submit"), h.Class(opClass),
				h.Title("Switch between AND and OR"), attrs(p.OperatorAttrs), g.Text(op)))
		}
	}
	return h.Div(h.Class("condition-group"), g.Attr("role", "group"),
		g.Group(p.Hidden),
		h.Div(h.Class("condition-group__head"),
			h.Button(h.Type("submit"), h.Class("condition-group__not"), h.Aria("pressed", notPressed),
				h.Title("Negate this group"), attrs(p.NotAttrs), g.Text("NOT")),
			h.Span(h.Class("condition-group__paren"), h.Aria("hidden", "true"), g.Text("(")),
		),
		h.Div(h.Class("condition-group__body"),
			g.Group(body),
			h.Div(h.Class("condition-group__actions"), p.Add),
		),
		h.Div(h.Class("condition-group__foot"),
			h.Span(h.Class("condition-group__paren"), h.Aria("hidden", "true"), g.Text(")")),
			g.If(p.RemoveLabel != "", h.Button(h.Type("submit"), h.Class("condition-group__remove"),
				h.Aria("label", p.RemoveLabel), attrs(p.RemoveAttrs), g.Text("✕"))),
		),
		g.If(p.Error != "", h.P(h.Class("condition-group__error"), g.Text(p.Error))),
	)
}
