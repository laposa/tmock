package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// InlineErrorProps configures an InlineError.
type InlineErrorProps struct {
	Message string
	Shown   bool     // shown from the start; otherwise hidden until a component shows it
	Attrs   []g.Node // x-bind:hidden …
}

// InlineError is an error message inside a form that the page shows and hides as the user
// types, such as "The new passwords do not match" (spec §10.3 M1). Errors that come from
// the server are shown under their fields instead.
func InlineError(p InlineErrorProps) g.Node {
	return h.P(h.Class("inline-error"), g.Attr("role", "alert"), g.If(!p.Shown, g.Attr("hidden")),
		attrs(p.Attrs), g.Text(p.Message))
}
