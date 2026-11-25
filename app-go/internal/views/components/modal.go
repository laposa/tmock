package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ModalBodyID is the element modal content is loaded into (hx-target="#modal-body").
const ModalBodyID = "modal-body"

// ModalTitleID is the id of the title of the modal content, which names the dialog.
const ModalTitleID = "modal-title"

// ModalHost is the single <dialog> that every modal is shown in (spec §10.2). htmx loads
// the content into #modal-body; the Alpine component "modal" in app.js opens the dialog
// when content arrives and empties it when it closes.
func ModalHost() g.Node {
	return h.Dialog(h.Class("modal"), h.ID("modal"), g.Attr("x-data", "modal"), h.Aria("labelledby", ModalTitleID),
		h.Div(h.Class("modal__body"), h.ID(ModalBodyID)),
	)
}

// ModalContentProps configures a ModalContent.
type ModalContentProps struct {
	Title      string
	Persistent bool // a click on the backdrop does not close the modal
	Body       []g.Node
}

// ModalContent is what a handler loads into the modal host: the title, then the body
// (usually a Form with a ModalFooter).
func ModalContent(p ModalContentProps) g.Node {
	return h.Div(h.Class("modal-content"), g.If(p.Persistent, g.Attr("data-persistent")),
		h.H2(h.Class("modal-content__title"), h.ID(ModalTitleID), g.Text(p.Title)),
		g.Group(p.Body),
	)
}

// ModalFooterProps configures a ModalFooter.
type ModalFooterProps struct {
	Start []g.Node // on the left: destructive actions, such as Delete
	End   []g.Node // on the right: Close, then the main action
}

// ModalFooter holds a modal's buttons, laid out as in the legacy dialogs: Delete on the
// left, Close and Save on the right.
func ModalFooter(p ModalFooterProps) g.Node {
	return h.Div(h.Class("modal-footer"),
		h.Div(h.Class("modal-footer__start"), g.Group(p.Start)),
		h.Div(h.Class("modal-footer__end"), g.Group(p.End)),
	)
}

// CloseModalButton closes the modal it is in (the "close" method of the Alpine component
// "modal"), without saving.
func CloseModalButton() g.Node {
	return Button(ButtonProps{Label: "Close", Attrs: []g.Node{g.Attr("x-on:click", "close")}})
}
