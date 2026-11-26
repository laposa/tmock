package components

import (
	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// HeaderRow is one header of a HeaderRows.
type HeaderRow struct {
	Name  string
	Value string
}

// HeaderRowsProps configures a HeaderRows.
type HeaderRowsProps struct {
	Label      string // names the group for screen readers, such as "Response headers"
	NameField  string // the form name of every name input
	ValueField string // the form name of every value input
	Rows       []HeaderRow
	Error      string
}

// HeaderRows edits a list of headers as name/value rows (legacy ResponseHeadersEdit).
// The rows are submitted as parallel lists: NameField and ValueField once per row, in
// order. The Alpine component "headerRows" in app.js adds and removes rows; without
// JavaScript the rows shown can still be edited. With no rows, one empty row is shown.
func HeaderRows(p HeaderRowsProps) g.Node {
	rows := p.Rows
	if len(rows) == 0 {
		rows = []HeaderRow{{}}
	}
	return h.Div(h.Class("header-rows"), g.Attr("x-data", "headerRows"), h.Role("group"), h.Aria("label", p.Label),
		h.Span(h.Class("header-rows__label"), h.Aria("hidden", "true"), g.Text(p.Label)),
		h.Div(h.Class("header-rows__list"), g.Attr("data-header-rows"),
			g.Map(rows, func(r HeaderRow) g.Node { return headerRow(p, r) })),
		// A blank row for "Add Header" to copy.
		h.Template(g.Attr("data-header-row-template"), headerRow(p, HeaderRow{})),
		h.Div(h.Class("header-rows__actions"),
			Button(ButtonProps{Label: "Add Header", Variant: ButtonText, Attrs: []g.Node{g.Attr("x-on:click", "add")}})),
		g.If(p.Error != "", h.P(h.Class("header-rows__error"), h.Role("alert"), g.Text(p.Error))),
	)
}

func headerRow(p HeaderRowsProps, r HeaderRow) g.Node {
	return h.Div(h.Class("header-rows__row"), g.Attr("data-header-row"),
		h.Input(h.Class("header-rows__input"), h.Name(p.NameField), h.Aria("label", "Header name"),
			h.Placeholder("Header"), h.AutoComplete("off"), g.If(r.Name != "", h.Value(r.Name))),
		h.Input(h.Class("header-rows__input"), h.Name(p.ValueField), h.Aria("label", "Header value"),
			h.Placeholder("Value (empty removes the header)"), h.AutoComplete("off"), g.If(r.Value != "", h.Value(r.Value))),
		h.Button(h.Type("button"), h.Class("header-rows__remove"), h.Aria("label", "Remove header"),
			g.Attr("x-on:click", "remove"), g.Text("✕")),
	)
}
