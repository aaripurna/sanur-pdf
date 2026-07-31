// Command scripts demonstrates text outside WinAnsi: Latin Extended, Cyrillic,
// Greek and the typographic symbols a single-byte encoding cannot reach.
//
// Everything here rests on one thing — registering a TrueType or OpenType font. The
// built-in Helvetica and Courier have no glyphs for these scripts and are addressed
// one byte at a time, so a document that needs them has to bring its own font. What
// sanur does with that font is embed a subset of it as a composite font, which makes
// every script the font covers available and keeps the file small.
//
// No font is shipped with sanur: system fonts are licensed, and vendoring one into a
// repository is a decision each project has to make for itself. Pass a path as the
// second argument, or let the usual system locations be searched.
package main

import (
	"fmt"
	"log"
	"os"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

// candidates are the faces commonly installed on a development machine that cover
// Cyrillic and Greek as well as Latin.
var candidates = []struct{ regular, bold string }{
	{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial Bold.ttf",
	},
	{
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans-Bold.ttf",
	},
	{
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans-Bold.ttf",
	},
	{"C:/Windows/Fonts/arial.ttf", "C:/Windows/Fonts/arialbd.ttf"},
}

var (
	ink      = sanur.Hex("#1A1D29")
	muted    = sanur.Hex("#6B7280")
	accent   = sanur.Hex("#4F46E5")
	hairline = sanur.Hex("#E5E7EB")
	panel    = sanur.Hex("#F6F5FA")
	negative = sanur.Hex("#B91C1C")
)

// family is resolved in main and read by the section builders. A package variable
// rather than a parameter threaded through nine functions: every line of text in the
// document uses the same family, and there is nothing to decide per section.
var family sanur.Family

// sample is one line of text in one language, with a note on what it needs from the
// encoding — which is the entire point of the table.
type sample struct {
	language, text, needs string
}

var samples = []sample{
	{"English", "The quick brown fox jumps over the lazy dog", "Latin-1, which WinAnsi covers"},
	{"French", "Portez ce vieux whisky au juge blond qui fume", "still Latin-1"},
	{"Polish", "Zażółć gęślą jaźń", "ł ż ó ń ę ś — Latin Extended-A"},
	{"Czech", "Příšerně žluťoučký kůň úpěl ďábelské ódy", "ř ě ť ů ď"},
	{"Slovak", "Kŕdeľ ďatľov učí koňa žrať kôru", "ŕ ľ ď ň ô"},
	{"Hungarian", "Egy hűtlen vízi tükörfúrógép", "ű ő with double acute"},
	{"Turkish", "Pijamalı hasta yağız şoföre çabucak güvendi", "ı ş ğ — and a dotless i"},
	{"Romanian", "Bătrânul șofer așteaptă țăranul", "ș ț with comma below"},
	{"Vietnamese", "Tôi có thể ăn thủy tinh mà không hại gì", "stacked tone marks"},
	{"Russian", "Съешь же ещё этих мягких французских булок", "Cyrillic"},
	{"Ukrainian", "Чуєш їх, доцю, га?", "Cyrillic with ї є"},
	{"Greek", "Ξεσκεπάζω την ψυχοφθόρα βδελυγμία", "Greek with accents"},
}

// rightToLeft needs more than a wider encoding: the characters have to be reordered for
// display, and Arabic letters have to change shape according to their neighbours.
var rightToLeft = []sample{
	{"Hebrew", "דג סקרן שט בים", "reordered; Hebrew letters do not join"},
	{"Hebrew", "עמוד 12 מתוך 34", "the numbers still read left to right"},
	{"Arabic", "السلام عليكم ورحمة الله", "reordered and joined"},
	{"Arabic", "الصفحة 12 من 34", "a number inside a right-to-left clause"},
	{"Arabic", "لا إله إلا الله", "the lam-alef ligature"},
	{"Arabic", "مرحبا Go بالعالم", "a Latin word inside Arabic"},
	{"Persian", "سلام دنیا", "Persian letters, joined"},
	{"Urdu", "ہیلو دنیا", "Urdu letters, joined"},
}

// symbols are characters WinAnsi lacks outright.
var symbols = []struct{ glyph, name string }{
	{"→", "rightwards arrow"},
	{"←", "leftwards arrow"},
	{"↑", "upwards arrow"},
	{"↓", "downwards arrow"},
	{"≈", "almost equal to"},
	{"≠", "not equal to"},
	{"≤", "less than or equal"},
	{"≥", "greater than or equal"},
	{"∞", "infinity"},
	{"√", "square root"},
	{"∑", "summation"},
	{"∆", "increment"},
	{"°", "degree"},
	{"±", "plus-minus"},
	{"×", "multiplication"},
	{"÷", "division"},
}

func main() {
	out := "scripts.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	loaded, err := loadFamily()
	if err != nil {
		log.Fatal(err)
	}
	family = sanur.NewFamily(loaded.regular, loaded.bold, nil, nil)

	doc := sanur.New().
		Title("Scripts beyond WinAnsi").
		Author("sanur").
		Creator("sanur/examples/scripts").
		Subject("Composite fonts, glyph subsetting and multilingual text")

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(sanur.Mm(18))
		p.DefaultTextStyle(style(9.5).Color(ink))

		p.Footer().PaddingTop(10).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("One registered font, subsetted to what this document says",
				style(7.5).Color(muted))
			r.ConstantItem(110).AlignRight().
				DefaultTextStyle(style(7.5).Color(muted)).
				PageNumber("Page {page} of {total}")
		})
	})

	// Three pages, each holding one idea. The sections are sized so none of them
	// straddles a break: a table cut in half by a page boundary paginates correctly and
	// still reads as a mistake.
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)
			titleBlock(c)
			sampleTable(c)
			symbolGrid(c)
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)
			rtlTable(c)
			limitations(c)
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)
			comparison(c)
		})
	})

	if err := doc.Write(out); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}

	report(out, loaded.paths)
}

// style is shorthand for a text style in the registered family.
func style(size float64) *sanur.StyleBuilder {
	return sanur.TextStyle().Family(family).Size(size)
}

func titleBlock(c *sanur.ColumnBuilder) {
	c.Item().Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(6)
		col.Item().StyledText("Scripts beyond WinAnsi", style(22).Bold().Color(ink))
		col.Item().StyledText(
			"Every line below is drawn from one registered font, addressed by glyph "+
				"identifier rather than by a byte in a code page. Selecting any of it "+
				"copies the original characters back out, and a search finds them.",
			style(9.5).Color(muted).LineHeight(1.4))
		col.Item().PaddingTop(2).LineHorizontal(1, hairline)
	})
}

func sampleTable(c *sanur.ColumnBuilder) {
	c.Item().Table(func(t *sanur.TableBuilder) {
		t.ColumnConstant(72)
		t.ColumnRelative(1)
		t.ColumnConstant(148)

		t.HeaderRow(func(r *sanur.TableRowBuilder) {
			for _, heading := range []string{"Language", "Text", "What it needs"} {
				r.Cell().Background(accent).PaddingXY(8, 5).
					StyledText(heading, style(7.5).Bold().Color(sanur.White))
			}
		})

		for i, s := range samples {
			// Banding is what makes a table this dense readable, and it costs nothing
			// but a background on the cells: the row's own height comes from whichever
			// cell needs the most.
			background := sanur.White
			if i%2 == 1 {
				background = panel
			}

			t.Row(func(r *sanur.TableRowBuilder) {
				r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
					StyledText(s.language, style(8.5).Bold().Color(ink))
				r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
					StyledText(s.text, style(10).Color(ink))
				r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
					StyledText(s.needs, style(7.5).Color(muted))
			})
		}
	})
}

func rtlTable(c *sanur.ColumnBuilder) {
	c.Item().Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(8)
		col.Item().StyledText("Right to left", style(13).Bold().Color(ink))
		col.Item().StyledText(
			"These need two things a wider encoding does not give: the characters are "+
				"reordered for display, and Arabic letters take one of four shapes "+
				"according to what sits either side of them. The text column below is "+
				"right-aligned, which is what a right-to-left paragraph wants.",
			style(8.5).Color(muted).LineHeight(1.4))

		col.Item().PaddingTop(2).Table(func(t *sanur.TableBuilder) {
			t.ColumnConstant(58)
			t.ColumnRelative(1)
			t.ColumnConstant(196)

			t.HeaderRow(func(r *sanur.TableRowBuilder) {
				for _, heading := range []string{"Language", "Text", "What it needs"} {
					r.Cell().Background(accent).PaddingXY(8, 5).
						StyledText(heading, style(7.5).Bold().Color(sanur.White))
				}
			})

			for i, s := range rightToLeft {
				background := sanur.White
				if i%2 == 1 {
					background = panel
				}

				t.Row(func(r *sanur.TableRowBuilder) {
					r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
						StyledText(s.language, style(8.5).Bold().Color(ink))
					// Right-aligned: the reordering makes the text read correctly at
					// either margin, but flush left is not where a right-to-left
					// paragraph belongs.
					r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
						AlignRight().StyledText(s.text, style(11).Color(ink))
					r.Cell().Background(background).PaddingXY(8, 6).AlignMiddle().
						StyledText(s.needs, style(7.5).Color(muted))
				})
			}
		})
	})
}

// symbolsPerRow keeps the grid aligned; the last row is padded to match.
const symbolsPerRow = 6

func symbolGrid(c *sanur.ColumnBuilder) {
	c.Item().Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(8)
		col.Item().StyledText("Symbols", style(13).Bold().Color(ink))
		col.Item().StyledText(
			"Arrows and mathematical operators are the everyday casualties of a "+
				"single-byte encoding: a table of contents that wanted a → had to settle "+
				"for the » that WinAnsi happens to include.",
			style(8.5).Color(muted).LineHeight(1.4))

		for start := 0; start < len(symbols); start += symbolsPerRow {
			end := start + symbolsPerRow
			if end > len(symbols) {
				end = len(symbols)
			}

			col.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(8)
				for _, symbol := range symbols[start:end] {
					r.RelativeItem(1).Background(panel).PaddingXY(8, 6).
						Row(func(cell *sanur.RowBuilder) {
							cell.Spacing(6)
							cell.ConstantItem(14).AlignMiddle().
								StyledText(symbol.glyph, style(12).Color(accent))
							cell.RelativeItem(1).AlignMiddle().
								StyledText(symbol.name, style(7).Color(muted))
						})
				}
				for i := end; i < start+symbolsPerRow; i++ {
					r.RelativeItem(1).Empty()
				}
			})
		}
	})
}

// comparison sets the same text twice: once in a built-in font and once in the
// registered one.
//
// This is the whole feature in one picture. The built-in faces are standard-14 —
// resolved by the reader from a name, addressed one byte at a time through WinAnsi —
// so every character outside that code page arrives as a question mark. Nothing is
// wrong with the layout; the encoding simply cannot say what the text says.
func comparison(c *sanur.ColumnBuilder) {
	c.Item().Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(6)
		col.Item().StyledText("The same text, two encodings", style(16).Bold().Color(ink))
		col.Item().StyledText(
			"On the left, Helvetica: a built-in face with no glyphs for these scripts, "+
				"addressed by byte. On the right, the registered font, addressed by glyph. "+
				"The left column is what every sanur document looked like before composite "+
				"fonts existed.",
			style(8.5).Color(muted).LineHeight(1.4))

		col.Item().PaddingTop(6).Table(func(t *sanur.TableBuilder) {
			t.ColumnRelative(1)
			t.ColumnRelative(1)

			t.HeaderRow(func(r *sanur.TableRowBuilder) {
				for _, heading := range []string{
					"Helvetica, WinAnsi bytes",
					"Registered font, glyph identifiers",
				} {
					r.Cell().Background(accent).PaddingXY(8, 5).
						StyledText(heading, style(7.5).Bold().Color(sanur.White))
				}
			})

			for i, s := range samples {
				background := sanur.White
				if i%2 == 1 {
					background = panel
				}

				t.Row(func(r *sanur.TableRowBuilder) {
					// No Family call: this cell falls back to the page default, which
					// the built-in Helvetica supplies.
					r.Cell().Background(background).PaddingXY(8, 6).StyledText(s.text,
						sanur.TextStyle().Size(9).Color(negative))
					r.Cell().Background(background).PaddingXY(8, 6).
						StyledText(s.text, style(9).Color(ink))
				})
			}
		})
	})
}

func limitations(c *sanur.ColumnBuilder) {
	c.Item().Background(panel).Padding(12).Column(func(col *sanur.ColumnBuilder) {
		col.Spacing(7)
		col.Item().StyledText("What this does not do", style(11).Bold().Color(ink))

		for _, note := range []string{
			"Indic scripts are not shaped: Devanagari and its relatives need glyph " +
				"reordering and conjunct formation, which no codepoint substitution can " +
				"express.",
			"Arabic vowel marks sit at the font's default offset rather than centred " +
				"over the letter they belong to, which needs the font's positioning " +
				"table.",
			"No ligatures, small capitals, alternates or kerning pairs. Advance widths " +
				"come from the font; its layout tables are ignored.",
			"A character the font lacks becomes a question mark — visible on purpose, " +
				"since dropping it silently would hide missing content.",
		} {
			col.Item().Row(func(r *sanur.RowBuilder) {
				r.Spacing(6)
				r.ConstantItem(9).StyledText("•", style(8.5).Color(muted))
				r.RelativeItem(1).StyledText(note, style(8.5).Color(muted).LineHeight(1.4))
			})
		}
	})
}

// loadedFamily is the resolved family plus the files it came from, kept so the size
// report can compare the output against them.
type loadedFamily struct {
	regular, bold core.Font
	paths         []string
}

// loadFamily resolves a font from the command line or the usual system locations.
//
// The bold face is optional: an incomplete family degrades to the nearest available
// face rather than failing, so a machine carrying only a regular weight still
// renders the document.
func loadFamily() (loadedFamily, error) {
	if len(os.Args) > 2 {
		path := os.Args[2]

		regular, err := fonts.LoadTrueTypeFile("", path)
		if err != nil {
			return loadedFamily{}, err
		}
		return loadedFamily{regular: regular, paths: []string{path}}, nil
	}

	for _, candidate := range candidates {
		if _, err := os.Stat(candidate.regular); err != nil {
			continue
		}

		regular, err := fonts.LoadTrueTypeFile("", candidate.regular)
		if err != nil {
			return loadedFamily{}, err
		}
		// A face that cannot draw Cyrillic would turn this document into a page of
		// question marks, which is worse than saying so and stopping.
		if regular.AdvanceOf('Ж', 10) <= 0 {
			continue
		}

		loaded := loadedFamily{regular: regular, paths: []string{candidate.regular}}

		if _, err := os.Stat(candidate.bold); err == nil {
			// A second face registered under a different name is a second composite
			// font in the output, each subsetted to the glyphs drawn in that weight.
			if bold, err := fonts.LoadTrueTypeFile("", candidate.bold); err == nil {
				loaded.bold = bold
				loaded.paths = append(loaded.paths, candidate.bold)
			}
		}
		return loaded, nil
	}

	return loadedFamily{}, fmt.Errorf(
		"no system font with Cyrillic coverage was found; pass one as the second "+
			"argument, for example:\n\n  go run ./examples/scripts scripts.pdf %s",
		candidates[0].regular)
}

// report prints the size of the output next to the size of the fonts it drew from,
// because the ratio is the whole argument for subsetting.
func report(out string, fontPaths []string) {
	written, err := os.Stat(out)
	if err != nil {
		log.Fatal(err)
	}

	var embedded int64
	for _, path := range fontPaths {
		info, err := os.Stat(path)
		if err != nil {
			continue
		}
		embedded += info.Size()
	}

	if embedded == 0 {
		fmt.Printf("wrote %s (%.0f kB)\n", out, float64(written.Size())/1000)
		return
	}

	fmt.Printf("wrote %s (%.0f kB, from %.0f kB of font files: %.0f times smaller)\n",
		out,
		float64(written.Size())/1000,
		float64(embedded)/1000,
		float64(embedded)/float64(written.Size()))
}
