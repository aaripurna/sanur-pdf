// Command concurrent demonstrates the shape a server wants: expensive things loaded
// once, a document per request, generated in parallel.
//
// The rule is one document per goroutine. A Document is not safe for concurrent use and
// neither is anything reachable from one — elements carry pagination state, so two
// goroutines drawing the same tree would interleave that progress and produce two wrong
// documents rather than one error.
//
// What is safe is sharing the expensive things. A fonts.Registry guards itself, a loaded
// face guards its own metric caches, and a theme is read-only once parsed. So they are
// loaded here at startup and read from every goroutine, which is what makes generating
// a hundred invoices at once worth doing rather than a hundred times the work.
package main

import (
	"fmt"
	"log"
	"os"
	"runtime"
	"sync"
	"time"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/theme"
)

// The theme is the sort of thing that would come from a file at startup. Inline here so
// the example needs no assets.
const brand = `{
  "page":   {"size": "A4", "margin": 40, "background": "#FFFFFF"},
  "colors": {"ink": "#1A1D29", "muted": "#6B7280", "accent": "#4F46E5", "rule": "#E5E7EB"},
  "fonts":  {"body": "Helvetica", "bold": "Helvetica-Bold"},
  "text": {
    "body":    {"font": "body", "size": 9.5,  "color": "ink"},
    "muted":   {"font": "body", "size": 8,    "color": "muted"},
    "heading": {"font": "bold", "size": 20,   "color": "ink"},
    "total":   {"font": "bold", "size": 11,   "color": "accent"}
  }
}`

// invoice is one unit of work: what a request would carry.
type invoice struct {
	number string
	client string
	lines  []line
}

type line struct {
	description string
	quantity    int
	unitPrice   float64
}

func (i invoice) total() float64 {
	var sum float64
	for _, l := range i.lines {
		sum += float64(l.quantity) * l.unitPrice
	}
	return sum
}

func main() {
	out := "concurrent.pdf"
	if len(os.Args) > 1 {
		out = os.Args[1]
	}

	// Loaded once. Every goroutine below reads these and none of them writes.
	brandTheme, err := theme.Parse([]byte(brand))
	if err != nil {
		log.Fatalf("parsing the theme: %v", err)
	}

	work := buildWorkload(64)

	// One goroutine per document, capped at the number of cores: this is CPU-bound
	// work, so more goroutines than cores buys nothing.
	documents, elapsed := generateAll(brandTheme, work, runtime.NumCPU())

	// Generating the same workload one at a time, to show the results are identical.
	// A shared cache corrupted by one goroutine would change what another measured,
	// and the bytes would stop matching.
	sequential, sequentialElapsed := generateAll(brandTheme, work, 1)

	identical := 0
	for i := range documents {
		if string(documents[i]) == string(sequential[i]) {
			identical++
		}
	}

	if err := os.WriteFile(out, documents[0], 0o644); err != nil {
		log.Fatalf("writing %s: %v", out, err)
	}

	fmt.Printf("generated %d documents on %d goroutines in %s\n",
		len(documents), runtime.NumCPU(), elapsed.Round(time.Millisecond))
	fmt.Printf("            the same %d one at a time in %s\n",
		len(sequential), sequentialElapsed.Round(time.Millisecond))
	fmt.Printf("%d of %d byte-identical either way\n", identical, len(documents))
	fmt.Printf("wrote %s (the first of them)\n", out)
}

// generateAll renders every invoice, with at most workers running at once.
func generateAll(th *theme.Theme, work []invoice, workers int) ([][]byte, time.Duration) {
	started := time.Now()

	out := make([][]byte, len(work))

	// A buffered channel as a semaphore, rather than a worker pool: the work is a fixed
	// list, so there is nothing to distribute dynamically and this is fewer moving parts.
	slots := make(chan struct{}, workers)

	var wg sync.WaitGroup
	for i, item := range work {
		wg.Add(1)
		go func(index int, item invoice) {
			defer wg.Done()

			slots <- struct{}{}
			defer func() { <-slots }()

			// Each goroutine builds its own Document. Nothing about it is shared;
			// only the theme is, and it is read-only.
			data, err := render(th, item)
			if err != nil {
				log.Fatalf("invoice %s: %v", item.number, err)
			}
			out[index] = data
		}(i, item)
	}
	wg.Wait()

	return out, time.Since(started)
}

// render builds one invoice. This is the whole of what a request handler would do.
func render(th *theme.Theme, in invoice) ([]byte, error) {
	doc := sanur.New().
		Title("Invoice " + in.number).
		Author("sanur").
		Creator("sanur/examples/concurrent")

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(th.PageSize()).MarginEach(th.Margins()).Background(th.Background())
		p.DefaultTextStyle(sanur.StyleFrom(th.Style("body")))

		p.Footer().PaddingTop(12).Row(func(r *sanur.RowBuilder) {
			r.RelativeItem(1).StyledText("Invoice "+in.number,
				sanur.StyleFrom(th.Style("muted")))
			r.ConstantItem(100).AlignRight().
				DefaultTextStyle(sanur.StyleFrom(th.Style("muted"))).
				PageNumber("Page {page} of {total}")
		})
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(14)

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).Column(func(col *sanur.ColumnBuilder) {
					col.Spacing(4)
					col.Item().StyledText("INVOICE", sanur.StyleFrom(th.Style("heading")))
					col.Item().StyledText(in.number, sanur.StyleFrom(th.Style("muted")))
				})
				r.ConstantItem(180).AlignRight().Column(func(col *sanur.ColumnBuilder) {
					col.Spacing(2)
					col.Item().AlignRight().StyledText(in.client,
						sanur.StyleFrom(th.Style("body")))
					col.Item().AlignRight().StyledText("Net 30",
						sanur.StyleFrom(th.Style("muted")))
				})
			})

			c.Item().LineHorizontal(1, th.Color("rule"))

			c.Item().Table(func(t *sanur.TableBuilder) {
				t.ColumnRelative(1)
				t.ColumnConstant(50)
				t.ColumnConstant(80)
				t.ColumnConstant(90)

				t.HeaderRow(func(r *sanur.TableRowBuilder) {
					for i, heading := range []string{"Description", "Qty", "Unit", "Amount"} {
						cell := r.Cell().Background(th.Color("accent")).PaddingXY(8, 5)
						if i > 0 {
							cell = cell.AlignRight()
						}
						cell.StyledText(heading,
							sanur.TextStyle().Size(7.5).Bold().Color(sanur.White))
					}
				})

				for i, l := range in.lines {
					background := sanur.White
					if i%2 == 1 {
						background = sanur.Hex("#F8F8FB")
					}

					t.Row(func(r *sanur.TableRowBuilder) {
						r.Cell().Background(background).PaddingXY(8, 5).
							StyledText(l.description, sanur.StyleFrom(th.Style("body")))
						r.Cell().Background(background).PaddingXY(8, 5).AlignRight().
							StyledText(fmt.Sprint(l.quantity), sanur.StyleFrom(th.Style("body")))
						r.Cell().Background(background).PaddingXY(8, 5).AlignRight().
							StyledText(money(l.unitPrice), sanur.StyleFrom(th.Style("body")))
						r.Cell().Background(background).PaddingXY(8, 5).AlignRight().
							StyledText(money(float64(l.quantity)*l.unitPrice),
								sanur.StyleFrom(th.Style("body")))
					})
				}
			})

			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).Empty()
				r.ConstantItem(170).PaddingTop(6).BorderTop(1, th.Color("rule")).
					PaddingTop(6).Row(func(row *sanur.RowBuilder) {
					row.RelativeItem(1).StyledText("Total", sanur.StyleFrom(th.Style("total")))
					row.AutoItem().AlignRight().StyledText(money(in.total()),
						sanur.StyleFrom(th.Style("total")))
				})
			})
		})
	})

	return doc.Bytes()
}

// buildWorkload invents n invoices of varying length, so the documents differ in page
// count and the goroutines do unequal amounts of work.
func buildWorkload(n int) []invoice {
	descriptions := []string{
		"Brand identity design",
		"Wayfinding signage system",
		"Interior visualisation",
		"Landscape masterplan revision",
		"Structural review and coordination",
		"Lighting design, public areas",
		"Site survey and documentation",
	}

	work := make([]invoice, 0, n)

	for i := 0; i < n; i++ {
		// Between 5 and 40 lines, so some invoices paginate and some do not.
		count := 5 + (i*7)%36

		lines := make([]line, 0, count)
		for j := 0; j < count; j++ {
			lines = append(lines, line{
				description: fmt.Sprintf("%s (phase %d)", descriptions[j%len(descriptions)], j/7+1),
				quantity:    1 + j%4,
				unitPrice:   125 + float64((j%8)*125),
			})
		}

		work = append(work, invoice{
			number: fmt.Sprintf("INV-2026-%04d", 1000+i),
			client: fmt.Sprintf("Client %02d Pty Ltd", i%17),
			lines:  lines,
		})
	}

	return work
}

// money formats an amount with thousands separators, since fmt has no verb for it.
func money(v float64) string {
	whole := int64(v)
	cents := int64((v-float64(whole))*100 + 0.5)

	digits := fmt.Sprint(whole)

	var grouped []byte
	for i, d := range []byte(digits) {
		if i > 0 && (len(digits)-i)%3 == 0 {
			grouped = append(grouped, ',')
		}
		grouped = append(grouped, d)
	}

	return fmt.Sprintf("%s.%02d", grouped, cents)
}
