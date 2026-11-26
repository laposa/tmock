package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// CopyFieldProps configures a CopyField.
type CopyFieldProps struct {
	Label   string
	ID      string
	Value   string
	Copied  string // the toast after copying, such as "Token copied to clipboard"
	Warning string // shown under the field, such as "This token is shown only once"
}

// CopyField shows a value read-only, with a Copy button (legacy ClientToken). The Alpine
// component "copyField" in app.js copies the value to the clipboard and shows Copied as a
// toast. Without JavaScript the value can still be selected and copied by hand.
func CopyField(p CopyFieldProps) g.Node {
	msg, describe := fieldMessage(p.ID, p.Warning, "")
	return fieldFrame(p.ID, p.Label,
		h.Div(h.Class("copy-field"), g.Attr("x-data", "copyField"), g.Attr("data-copied", p.Copied),
			h.Input(h.Class("copy-field__input"), h.ID(p.ID), h.Type("text"), h.ReadOnly(), h.Value(p.Value),
				h.SpellCheck("false"), h.AutoComplete("off"), describe, g.Attr("x-on:focus", "select")),
			Button(ButtonProps{Label: "Copy", Variant: ButtonPrimary, Attrs: []g.Node{g.Attr("x-on:click", "copy")}}),
		),
		msg,
	)
}
