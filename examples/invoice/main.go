// Command invoice generates a multi-page invoice, exercising most of sanur's
// layout vocabulary: headers and footers, rows, tables, page numbering and
// pagination across sheets.
package main

import (
	"fmt"
	"log"
	"os"
	"strings"

	sanur "github.com/aaripurna/sanur-pdf"
)

type lineItem struct {
	Description string
	Quantity    int
	UnitPrice   float64
}

func (l lineItem) Total() float64 { return float64(l.Quantity) * l.UnitPrice }

func main() {
	items := sampleItems(45)

	var subtotal float64
	for _, item := range items {
		subtotal += item.Total()
	}
	tax := subtotal * 0.11
	total := subtotal + tax

	doc := sanur.New().
		Title("Invoice INV-2026-0142").
		Author("Nuanu").
		Creator("sanur")

	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(18))
		p.DefaultTextStyle(sanur.TextStyle().Size(9.5).Color(sanur.Grey900))

		p.Header().PaddingBottom(12).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).Column(func(inner *sanur.ColumnBuilder) {
					inner.Spacing(2)
					inner.Item().StyledText("INVOICE",
						sanur.TextStyle().Size(22).Bold().Color(sanur.Indigo))
					inner.Item().StyledText("INV-2026-0142",
						sanur.TextStyle().Size(10).Color(sanur.Grey600))
				})

				r.ConstantItem(180).AlignRight().Column(func(inner *sanur.ColumnBuilder) {
					inner.Spacing(2)
					inner.Item().StyledText("Nuanu Creative City",
						sanur.TextStyle().Size(10).Bold())
					inner.Item().Text("Jl. Pantai Saba, Gianyar")
					inner.Item().Text("Bali 80581, Indonesia")
				})
			})

			c.Item().LineHorizontal(1, sanur.Grey300)
		})

		p.Footer().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)
			c.Item().LineHorizontal(0.5, sanur.Grey300)
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).StyledText("Thank you for your business.",
					sanur.TextStyle().Size(8).Color(sanur.Grey600))
				r.ConstantItem(120).AlignRight().
					DefaultTextStyle(sanur.TextStyle().Size(8).Color(sanur.Grey600)).
					PageNumber("Page {page} of {total}")
			})
		})

		// An overlay spans the whole sheet and reserves no space, so it paints over
		// the content rather than displacing it — which is why it is translucent and
		// turned out of the way of the text.
		p.Overlay().Rotate(-38).
			StyledText("DRAFT", sanur.TextStyle().
				Size(96).Bold().Color(sanur.RGBA(0x1A, 0x1D, 0x29, 22)))

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(20)
				r.RelativeItem(1).Column(func(inner *sanur.ColumnBuilder) {
					inner.Spacing(3)
					inner.Item().StyledText("BILL TO",
						sanur.TextStyle().Size(8).Bold().Color(sanur.Grey600).LetterSpacing(0.8))
					inner.Item().StyledText("Sanur Studio", sanur.TextStyle().Size(11).Bold())
					inner.Item().Text("Jl. Danau Tamblingan 88")
					inner.Item().Text("Denpasar, Bali 80228")
				})

				r.ConstantItem(170).Background(sanur.Grey100).Padding(12).
					Column(func(inner *sanur.ColumnBuilder) {
						inner.Spacing(4)
						for _, pair := range [][2]string{
							{"Issue date", "30 Jul 2026"},
							{"Due date", "29 Aug 2026"},
							{"Terms", "Net 30"},
						} {
							inner.Item().Row(func(row *sanur.RowBuilder) {
								row.RelativeItem(1).StyledText(pair[0],
									sanur.TextStyle().Size(9).Color(sanur.Grey600))
								row.AutoItem().StyledText(pair[1],
									sanur.TextStyle().Size(9).Bold())
							})
						}
					})
			})

			c.Item().Table(func(tb *sanur.TableBuilder) {
				tb.ColumnsRelative(5, 1, 2, 2).RowSpacing(0).ColumnSpacing(8)

				// HeaderRow rather than Row: this invoice runs to two pages, and a
				// column that splits resumes at the row it reached, so a heading
				// declared as an ordinary row would label only the first sheet.
				tb.HeaderRow(func(tr *sanur.TableRowBuilder) {
					header := sanur.TextStyle().Size(8).Bold().Color(sanur.White).LetterSpacing(0.6)
					tr.Cell().Background(sanur.Indigo).PaddingXY(8, 6).
						StyledText("DESCRIPTION", header)
					tr.Cell().Background(sanur.Indigo).PaddingXY(8, 6).AlignRight().
						StyledText("QTY", header)
					tr.Cell().Background(sanur.Indigo).PaddingXY(8, 6).AlignRight().
						StyledText("UNIT", header)
					tr.Cell().Background(sanur.Indigo).PaddingXY(8, 6).AlignRight().
						StyledText("AMOUNT", header)
				})

				for i, item := range items {
					item := item
					band := sanur.White
					if i%2 == 1 {
						band = sanur.Grey100
					}

					tb.Row(func(tr *sanur.TableRowBuilder) {
						tr.Cell().Background(band).PaddingXY(8, 5).Text(item.Description)
						tr.Cell().Background(band).PaddingXY(8, 5).AlignRight().
							Text(fmt.Sprintf("%d", item.Quantity))
						tr.Cell().Background(band).PaddingXY(8, 5).AlignRight().
							Text(money(item.UnitPrice))
						tr.Cell().Background(band).PaddingXY(8, 5).AlignRight().
							Text(money(item.Total()))
					})
				}
			})

			c.Item().AlignRight().Width(240).Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(5)
				inner.Item().LineHorizontal(1, sanur.Grey300)
				totalRow(inner, "Subtotal", money(subtotal), false)
				totalRow(inner, "Tax (11%)", money(tax), false)
				inner.Item().LineHorizontal(1, sanur.Grey400)
				totalRow(inner, "Total due", money(total), true)
			})
		})
	})

	out := "invoice.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	if err := doc.Write(out); err != nil {
		log.Fatalf("generating invoice: %v", err)
	}
	fmt.Printf("wrote %s\n", out)
}

func totalRow(c *sanur.ColumnBuilder, label, amount string, emphasise bool) {
	labelStyle := sanur.TextStyle().Size(9.5).Color(sanur.Grey700)
	valueStyle := sanur.TextStyle().Size(9.5)
	if emphasise {
		labelStyle = sanur.TextStyle().Size(11).Bold()
		valueStyle = sanur.TextStyle().Size(11).Bold().Color(sanur.Indigo)
	}

	c.Item().Row(func(r *sanur.RowBuilder) {
		r.RelativeItem(1).StyledText(label, labelStyle)
		r.ConstantItem(90).AlignRight().StyledText(amount, valueStyle)
	})
}

// money formats an amount with thousands separators, which fmt has no verb for.
func money(v float64) string {
	s := fmt.Sprintf("%.2f", v)

	whole, frac, _ := strings.Cut(s, ".")

	var grouped strings.Builder
	for i, digit := range whole {
		// A separator goes before every digit whose distance from the end is a
		// multiple of three, except at the very start of the number.
		if i > 0 && (len(whole)-i)%3 == 0 {
			grouped.WriteByte(',')
		}
		grouped.WriteRune(digit)
	}

	return grouped.String() + "." + frac
}

func sampleItems(n int) []lineItem {
	descriptions := []string{
		"Brand identity design",
		"Wayfinding signage system",
		"Interior visualisation, ground floor",
		"Landscape masterplan revision",
		"Structural review and coordination",
		"Lighting design, public areas",
		"Site survey and documentation",
	}

	items := make([]lineItem, 0, n)
	for i := 0; i < n; i++ {
		items = append(items, lineItem{
			Description: fmt.Sprintf("%s (phase %d)", descriptions[i%len(descriptions)], i/len(descriptions)+1),
			Quantity:    1 + i%4,
			UnitPrice:   250 + float64(i%9)*125,
		})
	}
	return items
}
