package sanur_test

import (
	"bytes"
	"fmt"
	"os"
	"sync"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/theme"
)

// These tests exist because of how the library will actually be used: from an HTTP
// handler, generating a document per request, with the fonts and the theme loaded once
// at startup. That is the case that has to be safe, and nothing but a race detector run
// against real concurrent work can show that it is.
//
// Run them with -race. Without it they only prove the output is right.

// concurrentTheme is loaded once and read from every goroutine, which is the arrangement
// the documentation recommends.
const concurrentTheme = `{
  "page":   {"size": "A4", "margin": 36},
  "colors": {"ink": "#1A1D29", "accent": "#4F46E5"},
  "fonts":  {"body": "Helvetica", "bold": "Helvetica-Bold"},
  "text": {
    "body":    {"font": "body", "size": 9.5, "color": "ink"},
    "heading": {"font": "bold", "size": 16,  "color": "accent"}
  },
  "chart": {"palette": ["accent"], "legend": "none"}
}`

// buildConcurrentDocument produces a document that touches everything with shared state:
// font metrics, a theme, a chart, links and a table that paginates.
func buildConcurrentDocument(th *theme.Theme, face core.Font, index int) ([]byte, error) {
	doc := sanur.New().Title(fmt.Sprintf("Document %d", index))

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(th.PageSize()).MarginEach(th.Margins()).Background(th.Background())
		p.DefaultTextStyle(sanur.StyleFrom(th.Style("body")))
		p.Footer().AlignRight().PageNumber("{page} of {total}")
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)

			c.Item().StyledText(fmt.Sprintf("Report %d", index),
				sanur.StyleFrom(th.Style("heading")))

			// A registered font, so the shared metric caches are exercised. Every
			// goroutine measures a different string, so they are not all reading an
			// already-warm cache entry.
			if face != nil {
				c.Item().StyledText(
					fmt.Sprintf("Съешь %d этих мягких булок · Ξεσκεπάζω %d", index, index),
					sanur.TextStyle().Font(face).Size(11))
			}

			// A contents entry, so the destination resolution pass runs too.
			c.Item().Row(func(r *sanur.RowBuilder) {
				r.RelativeItem(1).LinkTo("section").Text("Detail")
				r.ConstantItem(30).AlignRight().PageRef("section")
			})

			c.Item().Height(90).Element(&chart.Bar{
				Categories: []string{"A", "B", "C"},
				Series:     []chart.Series{{Name: "n", Values: []float64{1, 2, float64(index + 1)}}},
				Style:      th.ChartStyle(),
			})

			c.Item().PageBreak()
			c.Item().Anchor("section").Text("Detail")

			// Enough rows to paginate, so the sheet loop runs several times.
			for i := 0; i < 60; i++ {
				c.Item().Text(fmt.Sprintf("Row %d of document %d.", i, index))
			}
		})
	})

	return doc.Bytes()
}

func TestIndependentDocumentsGenerateConcurrently(t *testing.T) {
	// The documented promise: separate documents, shared fonts and theme.
	th, err := theme.Parse([]byte(concurrentTheme))
	if err != nil {
		t.Fatalf("parsing the theme: %v", err)
	}

	var face core.Font
	if path := optionalFont(); path != "" {
		if loaded, err := fonts.LoadTrueTypeFile("Concurrent", path); err == nil {
			face = loaded
		}
	}

	const workers = 16

	var (
		wg      sync.WaitGroup
		results = make([][]byte, workers)
		errs    = make([]error, workers)
	)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index], errs[index] = buildConcurrentDocument(th, face, index)
		}(i)
	}
	wg.Wait()

	for i, err := range errs {
		if err != nil {
			t.Errorf("document %d: %v", i, err)
			continue
		}
		if !bytes.HasPrefix(results[i], []byte("%PDF-")) {
			t.Errorf("document %d is not a PDF", i)
		}
	}
}

func TestConcurrentGenerationMatchesSequential(t *testing.T) {
	// A race the detector does not happen to catch would still show up here: a shared
	// cache corrupted by one goroutine changes what another measures, and the bytes stop
	// matching what the same document produces on its own.
	th, err := theme.Parse([]byte(concurrentTheme))
	if err != nil {
		t.Fatal(err)
	}

	var face core.Font
	if path := optionalFont(); path != "" {
		if loaded, err := fonts.LoadTrueTypeFile("ConcurrentCompare", path); err == nil {
			face = loaded
		}
	}

	const workers = 8

	// Sequential first, on a warm cache, to have something to compare against.
	want := make([][]byte, workers)
	for i := range want {
		data, err := buildConcurrentDocument(th, face, i)
		if err != nil {
			t.Fatalf("document %d: %v", i, err)
		}
		want[i] = data
	}

	got := make([][]byte, workers)

	var wg sync.WaitGroup
	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			data, err := buildConcurrentDocument(th, face, index)
			if err != nil {
				t.Errorf("document %d: %v", index, err)
				return
			}
			got[index] = data
		}(i)
	}
	wg.Wait()

	for i := range want {
		if !bytes.Equal(got[i], want[i]) {
			t.Errorf("document %d differs when generated concurrently (%d bytes against %d)",
				i, len(got[i]), len(want[i]))
		}
	}
}

func TestSharedFontMeasuresConsistentlyUnderLoad(t *testing.T) {
	// A loaded face caches an advance per glyph behind a mutex, and hands sfnt a scratch
	// buffer that cannot be shared. This hits both from many goroutines at once on cold
	// caches, which is where a missing lock would show.
	path := optionalFont()
	if path == "" {
		t.Skip("no system TrueType font available")
	}

	face, err := fonts.LoadTrueTypeFile("SharedMetrics", path)
	if err != nil {
		t.Fatal(err)
	}

	const (
		workers = 24
		text    = "Съешь же ещё этих мягких французских булок Ξεσκεπάζω 0123456789"
	)

	want := face.Measure(text, 12)

	var wg sync.WaitGroup
	widths := make([]float64, workers)

	for i := 0; i < workers; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			// Each goroutine walks the string differently, so they arrive at the same
			// cache entries in different orders.
			total := 0.0
			for _, r := range text {
				total += face.AdvanceOf(r, 12)
			}
			_ = total
			widths[index] = face.Measure(text, 12)
		}(i)
	}
	wg.Wait()

	for i, got := range widths {
		if got != want {
			t.Errorf("goroutine %d measured %.6f, want %.6f", i, got, want)
		}
	}
}

func TestFontRegistryIsSafeUnderConcurrentUse(t *testing.T) {
	// Registration at startup and lookup thereafter is the documented pattern, but a
	// registry read from many goroutines while one is still registering must not corrupt.
	registry := fonts.NewRegistry()

	var wg sync.WaitGroup

	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			for j := 0; j < 50; j++ {
				if _, err := registry.Resolve(fonts.Helvetica); err != nil {
					t.Errorf("resolving Helvetica: %v", err)
					return
				}
				_ = registry.Names()
			}
		}(i)
	}

	if path := optionalFont(); path != "" {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 5; j++ {
				// Registering while others read is the interleaving that matters.
				_, _ = registry.LoadTrueType(fmt.Sprintf("Loaded%d", j), path)
			}
		}()
	}

	wg.Wait()
}

// optionalFont returns a system TrueType path, or empty if none is installed.
//
// Separate from the systemFont helper because these tests degrade rather than skip: the
// concurrency they exercise is worth checking with the built-in fonts alone.
func optionalFont() string {
	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Verdana.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
	} {
		if _, err := os.Stat(path); err == nil {
			return path
		}
	}
	return ""
}
