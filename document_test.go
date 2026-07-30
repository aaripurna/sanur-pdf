package sanur_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
)

// generate builds a document and fails the test on error.
func generate(t *testing.T, build func(*sanur.Document)) []byte {
	t.Helper()

	doc := sanur.New()
	build(doc)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	return data
}

// countPages counts page objects in the output, an independent check on what the
// engine believes it produced.
func countPages(data []byte) int {
	return bytes.Count(data, []byte("/Type /Page\n")) +
		bytes.Count(data, []byte("/Type /Page "))
}

func TestGeneratesValidPDFStructure(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Title("Structure").Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Content().Text("Hello, Sanur.")
		})
	})

	if !bytes.HasPrefix(data, []byte("%PDF-1.7")) {
		t.Errorf("output does not start with a PDF header: %q", data[:min(16, len(data))])
	}
	if !bytes.HasSuffix(data, []byte("%%EOF\n")) {
		t.Errorf("output does not end with %%%%EOF")
	}
	for _, want := range []string{"/Type /Catalog", "/Type /Pages", "/Type /Page ", "trailer", "startxref"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}
	if got := countPages(data); got != 1 {
		t.Errorf("page count = %d, want 1", got)
	}
}

func TestOutputIsDeterministic(t *testing.T) {
	build := func(d *sanur.Document) {
		d.Title("Repeatable").Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(6)
				c.Item().Text("one")
				c.Item().Text("two")
			})
		})
	}

	// Generating the same document twice must produce identical bytes. Sanur
	// never reads the clock or iterates a map when writing, precisely so that
	// output can be diffed and cached.
	first := generate(t, build)
	second := generate(t, build)

	if !bytes.Equal(first, second) {
		t.Errorf("two runs produced different output (%d vs %d bytes)", len(first), len(second))
	}
}

func TestLongContentPaginates(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(4)
				// Far more rows than one A4 sheet can hold.
				for i := 0; i < 200; i++ {
					c.Item().Height(20).Background(sanur.Grey200).Text("row")
				}
			})
		})
	})

	pages := countPages(data)
	if pages < 5 {
		t.Errorf("200 rows of 24pt produced %d pages, want at least 5", pages)
	}
}

func TestPageBreakStartsNewSheet(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Item().Text("first sheet")
				c.Item().PageBreak()
				c.Item().Text("second sheet")
			})
		})
	})

	if got := countPages(data); got != 2 {
		t.Errorf("page count = %d, want 2", got)
	}
}

func TestHeaderAndFooterRepeatOnEverySheet(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Header().Text("REPEATING HEADER")
			p.Footer().AlignCenter().PageNumber("Page {page} of {total}")
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 120; i++ {
					c.Item().Height(20).Text("row")
				}
			})
		})
	})

	pages := countPages(data)
	if pages < 3 {
		t.Fatalf("expected several pages, got %d", pages)
	}

	// A header that failed to reset between sheets would render once and then
	// vanish, so its occurrence count is the real check.
	uncompressed := generate(t, func(d *sanur.Document) {
		d.Uncompressed().Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Header().Text("REPEATING HEADER")
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 120; i++ {
					c.Item().Height(20).Text("row")
				}
			})
		})
	})

	headers := bytes.Count(uncompressed, []byte("REPEATING HEADER"))
	if headers != countPages(uncompressed) {
		t.Errorf("header appears %d times across %d pages, want one per page",
			headers, countPages(uncompressed))
	}
}

func TestPageNumberResolvesTotal(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Uncompressed().Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Footer().PageNumber("Page {page} of {total}")
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				for i := 0; i < 120; i++ {
					c.Item().Height(20).Text("row")
				}
			})
		})
	})

	total := countPages(data)

	// The total must be the real page count, not the "?" placeholder the first
	// counting pass uses.
	if bytes.Contains(data, []byte("?")) {
		t.Errorf("output still contains the unresolved total placeholder")
	}

	want := []byte("of " + itoa(total))
	if !bytes.Contains(data, want) {
		t.Errorf("footer does not mention the resolved total %d", total)
	}
}

func TestRowsAndTableLayOut(t *testing.T) {
	data := generate(t, func(d *sanur.Document) {
		d.Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(10)

				c.Item().Row(func(r *sanur.RowBuilder) {
					r.Spacing(8)
					r.ConstantItem(80).Background(sanur.Blue).Text("fixed")
					r.AutoItem().Text("auto")
					r.RelativeItem(1).Background(sanur.Grey200).Text("rest")
				})

				c.Item().Table(func(tb *sanur.TableBuilder) {
					tb.ColumnsRelative(2, 1, 1).RowSpacing(4).ColumnSpacing(6)
					tb.Row(func(tr *sanur.TableRowBuilder) {
						tr.Cells("Item", "Qty", "Price")
					})
					tb.Row(func(tr *sanur.TableRowBuilder) {
						tr.Cells("Widget", "2", "9.99")
					})
				})
			})
		})
	})

	if got := countPages(data); got != 1 {
		t.Errorf("page count = %d, want 1", got)
	}
}

func TestEmptyDocumentIsAnError(t *testing.T) {
	if _, err := sanur.New().Bytes(); err == nil {
		t.Error("expected an error for a document with no pages")
	}
}

func TestOversizedContentReportsAUsefulError(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		// A single atomic item taller than the page can never be placed, and no
		// number of extra pages will help.
		p.Content().Height(5000).Text("too tall")
	})

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected an error for content taller than the page")
	}
	if !strings.Contains(err.Error(), "does not fit") {
		t.Errorf("error %q does not explain that the content did not fit", err)
	}
}

func TestMarginsLargerThanPageReportAnError(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A5).Margin(400)
		p.Content().Text("nowhere to go")
	})

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected an error when margins consume the whole page")
	}
	if !strings.Contains(err.Error(), "margins") {
		t.Errorf("error %q does not mention margins", err)
	}
}

// TestRendersWithGhostscript checks the output against a real PDF interpreter.
//
// Every other test here inspects bytes sanur wrote itself, which cannot catch a
// structurally valid file that no reader accepts — a wrong xref offset or a
// malformed font dictionary. Ghostscript parses the whole document and fails on
// exactly those.
func TestRendersWithGhostscript(t *testing.T) {
	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}

	data := generate(t, func(d *sanur.Document) {
		d.Title("Ghostscript check").Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Header().Text("Header")
			p.Footer().AlignRight().PageNumber("{page} / {total}")
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Spacing(8)
				c.Item().StyledText("Title", sanur.TextStyle().Size(24).Bold())
				c.Item().RichText(func(tb *sanur.TextBuilder) {
					tb.Span("Mixed ").Bold("bold").Span(" and ").Italic("italic").Span(" text.")
				})
				c.Item().RoundedBackground(sanur.Grey200, 6).Padding(10).Text("Rounded panel")
				c.Item().LineHorizontal(1, sanur.Grey500)
				for i := 0; i < 80; i++ {
					c.Item().Text("Body row with some length to it, to force wrapping and pagination.")
				}
			})
		})
	})

	path := filepath.Join(t.TempDir(), "out.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := exec.Command(gs,
		"-dNOPAUSE", "-dBATCH", "-dSAFER",
		"-sDEVICE=nullpage", // parse and render, but write no image
		path)

	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("ghostscript rejected the document: %v\n%s", err, out)
	}
	if bytes.Contains(bytes.ToLower(out), []byte("error")) {
		t.Errorf("ghostscript reported errors:\n%s", out)
	}
}

// TestTextIsExtractable confirms the text really is text, not shapes.
func TestTextIsExtractable(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	const phrase = "Extractable sentence for the round trip."

	data := generate(t, func(d *sanur.Document) {
		d.Page(func(p *sanur.Page) {
			p.Margin(40)
			p.Content().Text(phrase)
		})
	})

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "text.pdf")
	txtPath := filepath.Join(dir, "text.txt")

	if err := os.WriteFile(pdfPath, data, 0o644); err != nil {
		t.Fatal(err)
	}

	if out, err := exec.Command(pdftotext, pdfPath, txtPath).CombinedOutput(); err != nil {
		t.Fatalf("pdftotext failed: %v\n%s", err, out)
	}

	extracted, err := os.ReadFile(txtPath)
	if err != nil {
		t.Fatal(err)
	}

	// Extraction may re-wrap lines, so the words are checked rather than the
	// exact string.
	for _, word := range strings.Fields(phrase) {
		if !strings.Contains(string(extracted), word) {
			t.Errorf("extracted text is missing %q; got:\n%s", word, extracted)
		}
	}
}

func itoa(v int) string {
	if v == 0 {
		return "0"
	}
	var digits []byte
	for v > 0 {
		digits = append([]byte{byte('0' + v%10)}, digits...)
		v /= 10
	}
	return string(digits)
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
