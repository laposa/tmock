package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ModalBodyID is the element modal content is loaded into (hx-target="#modal-body").
const ModalBodyID = "modal-body"

// ModalHost is the single <dialog> that every modal is shown in (spec §10.2). htmx loads
// the content into #modal-body; the Alpine component "modal" in app.js opens the dialog
// when content arrives and empties it when it closes.
func ModalHost() g.Node {
	return h.Dialog(h.Class("modal"), h.ID("modal"), g.Attr("x-data", "modal"), h.Aria("labelledby", "modal-title"),
		h.Div(h.Class("modal__body"), h.ID(ModalBodyID)),
	)
}
