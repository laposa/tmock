package components

import (
	"strconv"

	g "maragu.dev/gomponents"
	h "maragu.dev/gomponents/html"
)

// TableProps configures a Table.
type TableProps struct {
	Columns []string // the column headings, in order
	Rows    []TableRow
	Empty   string   // shown in place of the rows when there are none
	Attrs   []g.Node // on the outer element: id (a swap target), hx-* …
}

// TableRow is one row: a cell per column.
type TableRow struct {
	Cells []g.Node
	Attrs []g.Node // on the <tr>
}

// Table is a data table with the dark look of the legacy v-table theme="dark". The outer
// element scrolls sideways when the table is wider than the page.
func Table(p TableProps) g.Node {
	return h.Div(h.Class("table"), attrs(p.Attrs),
		h.Table(h.Class("table__table"),
			h.THead(h.Tr(g.Map(p.Columns, func(c string) g.Node {
				return h.Th(h.Class("table__head"), h.Scope("col"), g.Text(c))
			}))),
			h.TBody(
				g.Map(p.Rows, func(r TableRow) g.Node {
					return h.Tr(h.Class("table__row"), attrs(r.Attrs), g.Map(r.Cells, func(c g.Node) g.Node {
						return h.Td(h.Class("table__cell"), c)
					}))
				}),
				g.If(len(p.Rows) == 0 && p.Empty != "",
					h.Tr(h.Td(h.Class("table__empty"), h.ColSpan(strconv.Itoa(len(p.Columns))), g.Text(p.Empty)))),
			),
		),
	)
}
