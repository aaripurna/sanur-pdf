package sanur

import (
	"codeberg.org/aaripurna/sanur/core"
	"codeberg.org/aaripurna/sanur/elements"
)

// columnSpec is one column's width rule.
type columnSpec struct {
	sizing elements.RowSizing
	size   float64
}

// TableBuilder lays out a grid of rows that share one set of column widths.
//
// A table is a column of rows whose cells are all sized from the same
// specification, which is what keeps cells vertically aligned: each row resolves
// its widths from the identical rule set, so column three starts at the same x on
// every row without any cross-row measurement.
//
// Because it is a column underneath, a long table paginates for free — it breaks
// between rows, and a header row can be repeated by declaring it on each page.
type TableBuilder struct {
	style   core.TextStyle
	columns []columnSpec
	rows    []*elements.Row

	rowSpacing    float64
	columnSpacing float64
}

// ColumnsRelative declares columns that share the width in proportion to their
// weights. Passing 1, 1, 2 gives a quarter, a quarter and a half.
func (t *TableBuilder) ColumnsRelative(weights ...float64) *TableBuilder {
	for _, w := range weights {
		t.columns = append(t.columns, columnSpec{sizing: elements.RowRelative, size: w})
	}
	return t
}

// ColumnsConstant declares columns of fixed width in points.
func (t *TableBuilder) ColumnsConstant(widths ...float64) *TableBuilder {
	for _, w := range widths {
		t.columns = append(t.columns, columnSpec{sizing: elements.RowConstant, size: w})
	}
	return t
}

// ColumnAuto declares one column sized to its content.
func (t *TableBuilder) ColumnAuto() *TableBuilder {
	t.columns = append(t.columns, columnSpec{sizing: elements.RowAuto})
	return t
}

// ColumnRelative declares one proportional column, for mixing with constants.
func (t *TableBuilder) ColumnRelative(weight float64) *TableBuilder {
	t.columns = append(t.columns, columnSpec{sizing: elements.RowRelative, size: weight})
	return t
}

// ColumnConstant declares one fixed-width column.
func (t *TableBuilder) ColumnConstant(width float64) *TableBuilder {
	t.columns = append(t.columns, columnSpec{sizing: elements.RowConstant, size: width})
	return t
}

// RowSpacing sets the vertical gap between rows.
func (t *TableBuilder) RowSpacing(v float64) *TableBuilder {
	t.rowSpacing = v
	return t
}

// ColumnSpacing sets the horizontal gap between cells.
func (t *TableBuilder) ColumnSpacing(v float64) *TableBuilder {
	t.columnSpacing = v
	return t
}

// Row appends a row and hands back a builder for its cells.
func (t *TableBuilder) Row(build func(*TableRowBuilder)) *TableBuilder {
	row := &elements.Row{Spacing: t.columnSpacing}
	t.rows = append(t.rows, row)
	build(&TableRowBuilder{table: t, row: row})
	return t
}

// build assembles the table into a column of rows.
func (t *TableBuilder) build() core.Element {
	col := &elements.Column{Spacing: t.rowSpacing}
	for _, row := range t.rows {
		// Column spacing is applied here rather than at Row construction so that
		// ColumnSpacing can be called after the rows are declared.
		row.Spacing = t.columnSpacing
		col.Items = append(col.Items, row)
	}
	return col
}

// TableRowBuilder fills in the cells of one row.
type TableRowBuilder struct {
	table *TableBuilder
	row   *elements.Row
}

// Cell appends the next cell, taking its width from the matching column.
//
// Extra cells beyond the declared columns fall back to auto sizing rather than
// panicking: a stray cell then shows up as a visibly misaligned row, which is far
// easier to find in the output than a crash halfway through a report.
func (r *TableRowBuilder) Cell() *Container {
	index := len(r.row.Items)

	spec := columnSpec{sizing: elements.RowAuto}
	if index < len(r.table.columns) {
		spec = r.table.columns[index]
	}

	r.row.Items = append(r.row.Items, elements.RowItem{
		Sizing: spec.sizing,
		Size:   spec.size,
	})

	return newContainer(func(e core.Element) {
		r.row.Items[index].Element = e
	}, r.table.style)
}

// Cells is shorthand for filling a row with plain text.
func (r *TableRowBuilder) Cells(values ...string) *TableRowBuilder {
	for _, v := range values {
		r.Cell().Text(v)
	}
	return r
}
