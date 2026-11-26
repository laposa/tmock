package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// FormSectionProps configures a FormSection.
type FormSectionProps struct {
	Title  string
	Fields []g.Node
}

// FormSection is a titled group of fields in a form, such as "Request Conditions" in the
// scenario editor (a <fieldset> with its <legend>).
func FormSection(p FormSectionProps) g.Node {
	return h.FieldSet(h.Class("form-section"),
		h.Legend(h.Class("form-section__title"), g.Text(p.Title)),
		g.Group(p.Fields),
	)
}
