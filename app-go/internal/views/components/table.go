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
	Empty   string // shown in place of the rows when there are none
	// Widths, one per column, fix the column widths (table-layout: fixed), so tables
	// shown one under another line up whatever their content. Without them, columns are
	// sized by their content.
	Widths []ColumnWidth
	Attrs  []g.Node // on the outer element: id (a swap target), hx-* …
}

// ColumnWidth is the width of a table column (TableProps.Widths).
type ColumnWidth int

const (
	ColumnFill   ColumnWidth = iota // shares the width the other columns leave
	ColumnSmall                     // a short value: a method, a status code, an icon
	ColumnMedium                    // a button or a short label
)

var columnClasses = map[ColumnWidth]string{
	ColumnFill:   "table__col",
	ColumnSmall:  "table__col table__col--small",
	ColumnMedium: "table__col table__col--medium",
}

// TableRow is one row: a cell per column.
type TableRow struct {
	Cells []g.Node
	Attrs []g.Node // on the <tr>
}

// Table is a data table with the dark look of the legacy v-table theme="dark". The outer
// element scrolls sideways when the table is wider than the page.
func Table(p TableProps) g.Node {
	if len(p.Widths) != 0 && len(p.Widths) != len(p.Columns) {
		panic("components.Table: one width per column")
	}
	class := "table"
	var cols g.Node
	if len(p.Widths) != 0 {
		class += " table--fixed"
		cols = h.ColGroup(g.Map(p.Widths, func(w ColumnWidth) g.Node { return h.Col(h.Class(columnClasses[w])) }))
	}
	return h.Div(h.Class(class), attrs(p.Attrs),
		h.Table(h.Class("table__table"),
			cols,
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
