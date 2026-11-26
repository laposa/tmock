package pages

import (
	g "maragu.dev/gomponents"

	"github.com/laposa/tmock/app-go/internal/condition/builder"
	"github.com/laposa/tmock/app-go/internal/views/components"
	"github.com/laposa/tmock/app-go/internal/views/hx"
)

// EditParam is the form parameter of a builder edit: the clicked button's name, with the
// edit and its path as the value ("remove:n.0.1", builder.Edit).
const EditParam = "edit"

// ConditionBuilderProps is what the condition builder shows.
type ConditionBuilderProps struct {
	ClientID   int64
	ClientName string
	Tree       builder.Node
	// Errors are the problems of a failed save, keyed by node path ("n.0.1") for the
	// node itself, or by path and input ("n.0.1.value").
	Errors map[string]string
	Error  string // a problem with the whole condition, such as the enable rule
}

// ConditionBuilder is the condition builder modal (spec §10.3 C7, §10.5). Every edit
// (add, remove, NOT, AND↔OR) posts the whole form to ConditionEditURL and gets the
// builder back; Save posts it to ConditionURL. The edit buttons are submit buttons
// with a formaction, so the builder also works without JavaScript.
func ConditionBuilder(p ConditionBuilderProps) g.Node {
	b := conditionBuilder{p: p}
	save := ConditionURL(p.ClientID)
	return components.ModalContent(components.ModalContentProps{
		Title: "Edit Client Conditions: " + p.ClientName,
		Body: []g.Node{components.Form(components.FormProps{
			Action: save,
			Error:  p.Error,
			Attrs:  []g.Node{hx.Post(save), hx.Target("#" + components.ModalBodyID), g.Attr("data-persistent")},
			Fields: []g.Node{
				// Enter in a text input submits with the form's first submit button: make
				// that Save, not the first NOT chip.
				components.Button(components.ButtonProps{Label: "Save", Submit: true,
					Attrs: []g.Node{g.Attr("hidden"), g.Attr("tabindex", "-1"), g.Attr("aria-hidden", "true")}}),
				components.Text(components.TextProps{Muted: true, Children: []g.Node{g.Text(
					"Requests match when the condition is true. Nothing is saved until Save; an empty condition is saved as none.")}}),
				b.node(builder.Root, p.Tree, true),
			},
			Footer: components.ModalFooter(components.ModalFooterProps{
				End: []g.Node{
					components.CloseModalButton(),
					components.Button(components.ButtonProps{Label: "Save", Variant: components.ButtonPrimary, Submit: true}),
				},
			}),
		})},
	})
}

type conditionBuilder struct{ p ConditionBuilderProps }

// edit is the attributes of a button that makes e.
func (b conditionBuilder) edit(e builder.Edit) []g.Node {
	url := ConditionEditURL(b.p.ClientID)
	return []g.Node{
		g.Attr("name", EditParam), g.Attr("value", e.String()),
		g.Attr("formaction", url), g.Attr("formnovalidate"),
		hx.Post(url), hx.Target("#" + components.ModalBodyID),
	}
}

func (b conditionBuilder) node(path string, n builder.Node, root bool) g.Node {
	hidden := []g.Node{components.Hidden(components.HiddenProps{Name: path + ".kind", Value: string(n.Kind)})}
	if n.Not {
		hidden = append(hidden, components.Hidden(components.HiddenProps{Name: path + ".not", Value: "on"}))
	}
	if n.Kind == builder.Group {
		op := "and"
		if n.Or {
			op = "or"
		}
		hidden = append(hidden, components.Hidden(components.HiddenProps{Name: path + ".op", Value: op}))
		children := make([]g.Node, 0, len(n.Children))
		for i, ch := range n.Children {
			children = append(children, b.node(builder.Child(path, i), ch, false))
		}
		items := make([]components.MenuItem, 0, len(builder.Kinds))
		for _, k := range builder.Kinds {
			items = append(items, components.MenuItem{Label: k.Label(), Submit: true,
				Attrs: b.edit(builder.Edit{Op: builder.AddOp(k), Path: path})})
		}
		p := components.ConditionGroupProps{
			Not: n.Not, NotAttrs: b.edit(builder.Edit{Op: builder.ToggleNot, Path: path}),
			Or: n.Or, OperatorAttrs: b.edit(builder.Edit{Op: builder.ToggleAndOr, Path: path}),
			Children: children,
			Add:      components.Menu(components.MenuProps{Label: "Add Expression", Items: items}),
			Hidden:   hidden,
			Error:    b.p.Errors[path],
		}
		if !root {
			p.RemoveLabel, p.RemoveAttrs = "Remove this group", b.edit(builder.Edit{Op: builder.Remove, Path: path})
		}
		return components.ConditionGroup(p)
	}

	p := components.ConditionItemProps{
		KindLabel: n.Kind.Label(), Not: n.Not,
		OperatorAttrs: b.edit(builder.Edit{Op: builder.ToggleNot, Path: path}),
		RemoveLabel:   "Remove this " + n.Kind.Label() + " expression",
		RemoveAttrs:   b.edit(builder.Edit{Op: builder.Remove, Path: path}),
		Hidden:        hidden,
	}
	value := components.ConditionInput{Label: "Value", Name: path + ".value", Value: n.Value, Error: b.itemError(path, "value")}
	operator := "Equals"
	switch n.Kind {
	case builder.IP:
		p.Kind, value.Label, value.Placeholder = components.ConditionIP, "IP address", "10.0.0.1"
	case builder.CIDR:
		p.Kind, value.Label, value.Placeholder = components.ConditionCIDR, "CIDR prefix", "192.168.0.0/16"
	case builder.Header:
		p.Kind, value.Label, value.Placeholder = components.ConditionHeader, "Header value", "value"
	case builder.HeaderRegex:
		p.Kind, value.Label, value.Placeholder = components.ConditionHeaderRegex, "Pattern", "^qa-"
		operator = "Matches"
	}
	if n.Kind.HasHeader() {
		p.Header = &components.ConditionInput{Label: "Header name", Name: path + ".header", Value: n.Header,
			Placeholder: "X-Header", Error: b.itemError(path, "header")}
	}
	if n.Not {
		operator = "Not " + operator
	}
	p.Operator, p.Value = operator, value
	return components.ConditionItem(p)
}

// itemError is the problem of an item's input, or of the item itself.
func (b conditionBuilder) itemError(path, field string) string {
	if msg := b.p.Errors[path+"."+field]; msg != "" {
		return msg
	}
	if field == "value" {
		return b.p.Errors[path]
	}
	return ""
}

// ConditionBuilderPage shows the builder as a page of its own, for a browser without
// JavaScript.
func ConditionBuilderPage(admin bool, p ConditionBuilderProps) g.Node {
	return clientPage(admin, ConditionBuilder(p))
}

// ConditionURL is where the builder of the client with id is loaded and saved.
func ConditionURL(id int64) string { return ClientURL(id) + "/condition" }

// ConditionEditURL is where the builder's edits are posted.
func ConditionEditURL(id int64) string { return ConditionURL(id) + "/edit" }
