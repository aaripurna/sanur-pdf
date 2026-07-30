// Command themed renders one document under two themes loaded from JSON.
//
// The point of the example is that main.go below contains no colours, no font
// names, no sizes and no margins. Structure — the loops, the conditionals, the
// order of sections — stays in Go, where the language provides those things.
// Appearance lives in themes/*.json, where it can be changed without a rebuild.
//
//	go run ./examples/themed themed-light.pdf themes/light.json
//
// Run it against both theme files and diff the two PDFs to see how much a theme
// controls.
package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
	"github.com/aaripurna/sanur-pdf/theme"
)

type region struct {
	Name    string
	Revenue float64
	Growth  float64
}

func main() {
	out := "themed.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	themePath := filepath.Join(themeDir(), "light.json")
	if len(os.Args) > 2 {
		themePath = os.Args[2]
	}

	// Every name in the theme is resolved and checked here. A misspelled colour or
	// an unregistered font fails now, with the alternatives listed, rather than
	// rendering as invisible text somewhere in the middle of the document.
	th, err := theme.Load(themePath)
	if err != nil {
		log.Fatalf("loading theme: %v", err)
	}

	regions := []region{
		{"South-East Asia", 1940, 18.3},
		{"Oceania", 1120, 9.7},
		{"South Asia", 830, 22.1},
		{"East Asia", 610, 4.4},
		{"Rest of world", 320, -2.8},
	}

	doc := sanur.New().
		Title("Themed Report").
		Author("sanur").
		Creator("sanur/examples/themed")

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(th.PageSize()).
			MarginEach(th.Margins()).
			Background(th.Background())

		// StyleFrom is the bridge from a resolved core.TextStyle back into the
		// builder the fluent API takes, carrying the theme's own face across.
		p.DefaultTextStyle(sanur.StyleFrom(th.Style("body")))

		p.Header().PaddingBottom(16).Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).StyledText("Themed Report",
					sanur.StyleFrom(th.Style("label")))
				r.AutoItem().StyledText(filepath.Base(themePath),
					sanur.StyleFrom(th.Style("code")))
			})
			c.Item().LineHorizontal(1, th.Color("hairline"))
		})

		p.Footer().PaddingTop(12).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("Appearance from JSON, structure from Go",
				sanur.StyleFrom(th.Style("caption")))
			r.ConstantItem(100).AlignRight().
				DefaultTextStyle(sanur.StyleFrom(th.Style("caption"))).
				PageNumber("Page {page} of {total}")
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(22)

			c.Item().Column(func(inner *sanur.ColumnBuilder) {
				inner.Spacing(5)
				inner.Item().StyledText("Regional performance",
					sanur.StyleFrom(th.Style("title")))
				inner.Item().StyledText("Q2 2026",
					sanur.StyleFrom(th.Style("caption")))
			})

			c.Item().Element(section(th, "Revenue by region"))

			// The chart takes its palette, gridlines and label styles from the same
			// theme, so it stays consistent with the prose around it.
			c.Item().Height(165).Element(&chart.Bar{
				Categories:   names(regions),
				Series:       []chart.Series{{Values: revenues(regions)}},
				CornerRadius: 3,
				Style:        th.ChartStyle(),
			})

			c.Item().Element(section(th, "Detail"))

			c.Item().Table(func(tb *sanur.TableBuilder) {
				tb.ColumnsRelative(3, 1.4, 1.2).ColumnSpacing(10)

				tb.Row(func(tr *sanur.TableRowBuilder) {
					for _, heading := range []string{"REGION", "REVENUE", "GROWTH"} {
						align := sanur.AlignRight
						if heading == "REGION" {
							align = sanur.AlignLeft
						}
						tr.Cell().PaddingXY(0, 7).
							BorderBottom(1, th.Color("hairline")).
							Element(alignedText(th, "label", heading, align))
					}
				})

				for i, r := range regions {
					r := r
					band := core.Transparent
					if i%2 == 1 {
						band = th.Color("band")
					}

					tb.Row(func(tr *sanur.TableRowBuilder) {
						tr.Cell().Background(band).PaddingXY(6, 7).
							Element(alignedText(th, "body", r.Name, sanur.AlignLeft))

						tr.Cell().Background(band).PaddingXY(6, 7).
							Element(alignedText(th, "figure",
								fmt.Sprintf("%.0fk", r.Revenue), sanur.AlignRight))

						tr.Cell().Background(band).PaddingXY(6, 7).
							Element(alignedText(th, "body",
								fmt.Sprintf("%+.1f%%", r.Growth), sanur.AlignRight))
					})
				}
			})

			c.Item().Background(th.Color("band")).
				BorderLeft(3, th.Color("accent")).Padding(13).
				StyledText(
					"Nothing in this program names a colour, a font, a size or a margin. "+
						"Swap the theme file and the same code produces a different-looking "+
						"document — but the loops that build these five rows stay in Go, "+
						"where a language that has loops can express them.",
					sanur.StyleFrom(th.Style("body")))
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("generating document: %v", err)
	}
	fmt.Printf("wrote %s using %s\n", out, filepath.Base(themePath))
}

// section is a heading over a rule, styled from the theme.
func section(th *theme.Theme, title string) core.Element {
	col := &elements.Column{Spacing: 6}
	col.Add(elements.NewText(title, th.Style("heading")))
	col.Add(&elements.Line{Width: 0.75, Color: th.Color("hairline")})
	return col
}

// alignedText places themed text within its cell.
//
// A themed style is a core.TextStyle, which elements take directly — the builder
// bridge is only needed by the fluent methods.
func alignedText(th *theme.Theme, style, content string, align core.HorizontalAlign) core.Element {
	text := elements.NewText(content, th.Style(style))
	text.Align = align
	return text
}

func names(regions []region) []string {
	out := make([]string, len(regions))
	for i, r := range regions {
		out[i] = r.Name
	}
	return out
}

func revenues(regions []region) []float64 {
	out := make([]float64, len(regions))
	for i, r := range regions {
		out[i] = r.Revenue
	}
	return out
}

// themeDir locates the bundled themes next to this program, so it runs from any
// working directory.
func themeDir() string {
	if wd, err := os.Getwd(); err == nil {
		candidate := filepath.Join(wd, "examples", "themed", "themes")
		if _, err := os.Stat(candidate); err == nil {
			return candidate
		}
	}
	return "themes"
}
