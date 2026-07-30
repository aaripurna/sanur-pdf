package sanur_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
)

// printDocument mixes both colour spaces on one page, which is the realistic case:
// a press-bound document specifies its inks, and a chart or a logo brought in from
// elsewhere is still RGB.
func printDocument(t *testing.T) []byte {
	t.Helper()

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)
			c.Item().StyledText("Single-plate black",
				sanur.TextStyle().Size(18).Color(sanur.CMYK(0, 0, 0, 100)))
			c.Item().Size(200, 40).Background(sanur.CMYK(0, 100, 100, 0)).Empty()
			c.Item().Size(200, 40).Background(sanur.Red).Empty()
			c.Item().LineHorizontal(2, sanur.CMYK(100, 0, 0, 0))
			c.Item().Size(200, 40).Background(sanur.CMYKA(0, 0, 0, 100, 40)).Empty()
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	return data
}

func TestPrintColorsReachTheFileAsPlates(t *testing.T) {
	stream := string(printDocument(t))

	// Both spaces, side by side, unconverted.
	wants(t, stream,
		"0 0 0 1 k",   // the text
		"0 1 1 0 k",   // the CMYK swatch
		"1 0 0 0 K",   // the rule
		"0.898 0.224", // the RGB swatch, sanur.Red
	)
}

// TestPrintDocumentIsAcceptedByGhostscript parses the mixed-space document with a
// real interpreter, which is the only way to catch a file that sanur considers
// well-formed and no reader will open.
func TestPrintDocumentIsAcceptedByGhostscript(t *testing.T) {
	checkWithGhostscript(t, printDocument(t))
}

// TestPlatesAreNotRoutedThroughRGB catches the failure that matters most, and that
// no operator assertion can see: a CMYK colour converted to RGB on the way out.
//
// cmyk(100, 0, 0, 100) is cyan over full black. In RGB it collapses to #000000,
// which Ghostscript then converts back into a four-plate black — so the tell is not
// the cyan plate, which survives either way, but magenta and yellow arriving inked.
// The page still looks black in a viewer, which is exactly why this needs measuring
// rather than looking at.
func TestPlatesAreNotRoutedThroughRGB(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(0)
		p.Content().Extend().Background(sanur.CMYK(100, 0, 0, 100)).Empty()
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	plates := inkCoverage(t, data)

	// The page is one full-bleed rectangle, so the two specified plates are total and
	// the two unspecified ones must be untouched.
	for i, want := range []float64{1, 0, 0, 1} {
		if plates[i] < want-0.01 || plates[i] > want+0.01 {
			t.Errorf("%s coverage = %.4f, want %.0f (all four plates were %v)",
				plateNames[i], plates[i], want, plates)
		}
	}
}

var plateNames = [4]string{"cyan", "magenta", "yellow", "black"}

var coveragePattern = regexp.MustCompile(
	`([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s+([0-9.]+)\s+CMYK`)

// inkCoverage returns the fraction of the first page covered by each plate, as
// measured by Ghostscript's inkcov device.
//
// This is the only way to check print colour. Reading the content stream proves
// only that sanur emitted what it meant to, and rendering to RGB collapses the
// distinctions that matter — cmyk(0, 0, 0, 100) and a four-plate black are the same
// pixels on screen and quite different objects on a press. The separations are where
// the difference lives.
func inkCoverage(t *testing.T, data []byte) [4]float64 {
	t.Helper()

	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}

	path := filepath.Join(t.TempDir(), "coverage.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(gs,
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-sDEVICE=inkcov", "-o", "-", path,
	).CombinedOutput()
	if err != nil {
		t.Fatalf("ghostscript rejected the document: %v\n%s", err, out)
	}

	fields := coveragePattern.FindStringSubmatch(string(out))
	if fields == nil {
		t.Fatalf("no ink coverage reported:\n%s", out)
	}

	var plates [4]float64
	for i := range plates {
		if plates[i], err = strconv.ParseFloat(fields[i+1], 64); err != nil {
			t.Fatalf("parsing %s coverage %q: %v", plateNames[i], fields[i+1], err)
		}
	}
	return plates
}

func TestColorHelperAcceptsBothNotations(t *testing.T) {
	if got := sanur.Color("#1E88E5"); got != sanur.RGB(0x1E, 0x88, 0xE5) {
		t.Errorf("hex = %v", got)
	}
	if got := sanur.Color("cmyk(0, 0, 0, 100)"); got != sanur.CMYK(0, 0, 0, 100) {
		t.Errorf("cmyk = %v", got)
	}
	if got := sanur.Color("cmyk(0, 0, 0, 100)").Space(); got != sanur.SpaceCMYK {
		t.Errorf("space = %v, want CMYK", got)
	}
}

func TestColorHelperPanicsOnMalformedInput(t *testing.T) {
	// Same reasoning as Hex: colours are literals in layout code, so a bad one is a
	// programming error that should surface on the first run.
	defer func() {
		if recover() == nil {
			t.Error("Color did not panic on malformed input")
		}
	}()
	sanur.Color("cmyk(nope)")
}

func TestRegistrationPrintsOnEveryPlate(t *testing.T) {
	// Crop marks have to appear on all four plates, or misregistration is invisible
	// on the proof and shows up only once the job is on press.
	cy, m, y, k := sanur.Registration.CMYKComponents()

	for name, plate := range map[string]float64{"cyan": cy, "magenta": m, "yellow": y, "black": k} {
		if plate < 1 {
			t.Errorf("%s plate = %.2f, want 1", name, plate)
		}
	}
}

func TestCMYKAlphaIsAPercentage(t *testing.T) {
	if got := sanur.CMYKA(0, 0, 0, 100, 40).Opacity(); got != 102 {
		t.Errorf("opacity = %d, want 102 (40%% of 255)", got)
	}
	if strings.Contains(sanur.CMYK(0, 0, 0, 100).String(), ", 100)") == false {
		t.Errorf("String = %q, want the black plate at 100", sanur.CMYK(0, 0, 0, 100).String())
	}
}
