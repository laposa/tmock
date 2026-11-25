package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ConfirmDialog is the smaller <dialog> that asks before a request with hx-confirm is
// sent (spec §10.2). The Alpine component "confirm" in app.js fills in the question and
// lets the request go ahead on Confirm. The buttons are, in order, Confirm and Cancel,
// as in the legacy dialog.
func ConfirmDialog() g.Node {
	return h.Dialog(h.Class("confirm"), h.ID("confirm"), g.Attr("x-data", "confirm"), h.Aria("describedby", "confirm-message"),
		h.P(h.Class("confirm__message"), h.ID("confirm-message"), g.Attr("x-text", "message")),
		h.Div(h.Class("confirm__actions"),
			Button(ButtonProps{Label: "Confirm", Variant: ButtonDanger, Attrs: []g.Node{g.Attr("x-on:click", "accept")}}),
			Button(ButtonProps{Label: "Cancel", Attrs: []g.Node{g.Attr("x-on:click", "cancel")}}),
		),
	)
}
