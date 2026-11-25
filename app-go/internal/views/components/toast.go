package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// ToastRegion is where toasts appear, top-right (spec §10.2). Handlers raise them with an
// HX-Trigger header; the Alpine component "toasts" in app.js renders them from its store
// and removes them after 5 seconds. The type is a data attribute, so app.js sets no
// classes. Texts are set with x-text, which escapes them.
func ToastRegion() g.Node {
	return h.Div(h.Class("toasts"), h.ID("toasts"), g.Attr("x-data", "toasts"), h.Aria("live", "polite"),
		h.Template(g.Attr("x-for", "toast in items"), g.Attr("x-bind:key", "toast.id"),
			h.Div(h.Class("toast"), g.Attr("x-bind:data-type", "toast.type"), h.Role("status"),
				h.Div(h.Class("toast__text"),
					h.Strong(h.Class("toast__title"), g.Attr("x-text", "toast.title")),
					h.P(h.Class("toast__message"), g.Attr("x-show", "toast.message"), g.Attr("x-text", "toast.message")),
				),
				h.Button(h.Class("toast__close"), h.Type("button"), h.Aria("label", "Dismiss"),
					g.Attr("x-on:click", "toast.dismiss"), g.Text("✕")),
			),
		),
	)
}
