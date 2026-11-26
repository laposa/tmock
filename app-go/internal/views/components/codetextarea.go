package components

import (
	"strconv"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// Code languages a CodeTextarea shows as its hint. Only JSON changes behaviour: it
// offers "Format JSON".
const (
	LanguageJSON  = "JSON"
	LanguageHTML  = "HTML"
	LanguagePlain = "Plain text"
)

// CodeTextareaProps configures a CodeTextarea.
type CodeTextareaProps struct {
	Label    string
	Name     string // also the id, unless ID is set
	ID       string
	Value    string
	Language string // LanguageJSON, LanguageHTML or LanguagePlain (the default)
	Error    string
	Hint     string
	Rows     int      // visible lines; 12 when 0
	Attrs    []g.Node // on the <textarea>
}

// CodeTextarea is a labelled monospace <textarea> without wrapping, for response bodies
// (spec §10.4), framed like a TextField. The Alpine component "codeEditor" in app.js
// makes Tab insert two spaces and, for JSON, offers "Format JSON", which pretty-prints
// the content or shows why it is not valid JSON. There is no syntax highlighting.
//
// The language can change while the editor is open: dispatch a "code-editor:language"
// event with detail {language} on the element with data-language.
func CodeTextarea(p CodeTextareaProps) g.Node {
	id := fieldID(p.ID, p.Name)
	lang := p.Language
	if lang == "" {
		lang = LanguagePlain
	}
	rows := p.Rows
	if rows == 0 {
		rows = 12
	}
	msg, describe := fieldMessage(id, p.Hint, p.Error)
	return fieldFrame(id, p.Label,
		h.Div(h.Class("code-field"), g.Attr("x-data", "codeEditor"), g.Attr("data-language", lang),
			h.Div(h.Class("code-field__toolbar"),
				h.Span(h.Class("code-field__language"), g.Attr("x-text", "language"), g.Text(lang)),
				Button(ButtonProps{Label: "Format JSON", Variant: ButtonText, Attrs: []g.Node{
					g.Attr("hidden"), g.Attr("x-bind:hidden", "notJSON"), g.Attr("x-on:click", "format"),
				}}),
			),
			h.Textarea(h.Class("code-field__input"), h.ID(id), h.Name(p.Name), h.Rows(strconv.Itoa(rows)),
				h.SpellCheck("false"), g.Attr("wrap", "off"), g.Attr("autocapitalize", "off"),
				g.Attr("x-on:keydown.tab", "indent"),
				describe,
				attrs(p.Attrs),
				// The parser drops one newline right after <textarea>; this one, so that a
				// value that starts with a newline keeps it.
				g.Text("\n"+p.Value),
			),
			h.P(h.Class("code-field__error"), h.Role("alert"), g.Attr("x-show", "formatError"), g.Attr("x-text", "formatError")),
		),
		msg,
	)
}
