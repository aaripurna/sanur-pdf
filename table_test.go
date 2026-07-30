package sanur_test

import (
	"strings"
	"testing"

	"codeberg.org/aaripurna/sanur"
)

func TestTableColumnsConstantGivesFixedWidths(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsConstant(60, 40)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Background(sanur.Red).Size(10, 20).Empty()
				tr.Cell().Background(sanur.Blue).Size(10, 20).Empty()
			})
		})
	})

	// Each cell is filled at its declared column width and the row's height.
	wants(t, stream, "0 0 60 20 re f", "0 0 40 20 re f")
	// The second cell starts where the first ends.
	wants(t, stream, "1 0 0 1 60 0 cm")
}

func TestTableMixedColumnKindsResolveInOrder(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(0)
		p.Content().Width(300).Table(func(tb *sanur.TableBuilder) {
			// Constants are reserved first, then auto measures against the
			// remainder, and relative shares what is left after both.
			tb.ColumnConstant(100).ColumnAuto().ColumnRelative(1)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Background(sanur.Red).Size(10, 15).Empty()
				tr.Cell().Background(sanur.Green).Size(50, 15).Empty()
				tr.Cell().Background(sanur.Blue).Size(10, 15).Empty()
			})
		})
	})

	wants(t, stream,
		"0 0 100 15 re f", // constant
		"0 0 50 15 re f",  // auto, sized to its content
		"0 0 150 15 re f", // relative takes 300 - 100 - 50
	)
}

func TestTableColumnAndRowSpacingAreApplied(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsConstant(50, 50).ColumnSpacing(12).RowSpacing(9)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Size(10, 20).Empty()
				tr.Cell().Size(10, 20).Empty()
			})
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Size(10, 20).Empty()
				tr.Cell().Size(10, 20).Empty()
			})
		})
	})

	// Column spacing offsets the second cell by width plus gap.
	wants(t, stream, "1 0 0 1 62 0 cm")
	// Row spacing offsets the second row by height plus gap.
	wants(t, stream, "1 0 0 1 0 29 cm")
}

func TestColumnSpacingCanBeSetAfterRowsAreDeclared(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsConstant(50, 50)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Size(10, 20).Empty()
				tr.Cell().Size(10, 20).Empty()
			})
			// Declared last, but it must still reach the rows above.
			tb.ColumnSpacing(20)
		})
	})

	wants(t, stream, "1 0 0 1 70 0 cm")
}

func TestExtraCellsFallBackToAutoSizing(t *testing.T) {
	// A stray cell beyond the declared columns must not panic; it shows up as a
	// visibly misaligned row, which is far easier to find than a crash halfway
	// through a report.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsConstant(40)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cell().Background(sanur.Red).Size(10, 12).Empty()
				tr.Cell().Background(sanur.Blue).Size(25, 12).Empty()
			})
		})
	})

	wants(t, stream, "0 0 40 12 re f", "0 0 25 12 re f")
}

func TestTableCellsShorthandFillsARow(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsRelative(1, 1, 1)
			tb.Row(func(tr *sanur.TableRowBuilder) {
				tr.Cells("alpha", "beta", "gamma")
			})
		})
	})

	wants(t, stream, "(alpha) Tj", "(beta) Tj", "(gamma) Tj")
}

func TestLongTablePaginatesBetweenRows(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(30)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsRelative(3, 1)
			for i := 0; i < 120; i++ {
				tb.Row(func(tr *sanur.TableRowBuilder) {
					tr.Cell().PaddingXY(4, 6).Text("description")
					tr.Cell().PaddingXY(4, 6).AlignRight().Text("42")
				})
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	// A table is a column of rows underneath, so it paginates for free.
	if got := countPages(data); got < 2 {
		t.Errorf("120 rows produced %d page(s), want at least 2", got)
	}
}

func TestEmptyTableProducesAValidPage(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(20)
		p.Content().Table(func(tb *sanur.TableBuilder) {
			tb.ColumnsRelative(1, 1)
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("a table with no rows should still produce a page: %v", err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Error("output is not a PDF")
	}
}
