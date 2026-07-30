package sanur_test

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/render"
)

// systemFont finds a TrueType font to exercise the embedding path.
func systemFont(t *testing.T) string {
	t.Helper()

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
	t.Skip("no system TrueType font available")
	return ""
}

// checkWithGhostscript parses a document with a real interpreter, which is the
// only way to catch a structurally plausible file that no reader will accept.
func checkWithGhostscript(t *testing.T, data []byte) {
	t.Helper()

	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}

	path := filepath.Join(t.TempDir(), "check.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(gs,
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-sDEVICE=nullpage", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ghostscript rejected the document: %v\n%s", err, out)
	}
	if bytes.Contains(bytes.ToLower(out), []byte("error")) {
		t.Errorf("ghostscript reported errors:\n%s", out)
	}
}

func TestEmbeddedTrueTypeFontRenders(t *testing.T) {
	path := systemFont(t)

	face, err := fonts.LoadTrueTypeFile("SanurEmbedded", path)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			c.Item().Text("Embedded TrueType, measured from the real font tables.")
			c.Item().Text("Accented text: café, naïve, Zürich, façade.")
			// Enough text to force wrapping, which depends entirely on the
			// embedded font's own advance widths being read correctly.
			c.Item().Text("The quick brown fox jumps over the lazy dog, and keeps " +
				"jumping until this sentence has to wrap onto several lines.")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	// An embedded font is emitted as a composite font: a Type0 wrapper with a CID
	// descendant, addressed by glyph ID rather than through a single-byte encoding.
	for _, want := range []string{
		"/Subtype /Type0",
		"/Encoding /Identity-H",
		"/Subtype /CIDFontType2",
		"/CIDToGIDMap /Identity",
		"/FontFile2",
		"/FontDescriptor",
		"/ToUnicode",
		"/W [",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q, so the font was not embedded as a composite font", want)
		}
	}

	// The single-byte apparatus must be gone. Leaving a /Widths array or a
	// WinAnsi encoding on a Type0 font is the kind of leftover a reader may act on,
	// and it would mean the two code paths had been mixed.
	if bytes.Contains(data, []byte("/Subtype /TrueType")) {
		t.Error("the simple-font path was used for an embedded font")
	}

	checkWithGhostscript(t, data)
}

func TestEmbeddedFontTextIsExtractable(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	face, err := fonts.LoadTrueTypeFile("SanurExtract", systemFont(t))
	if err != nil {
		t.Fatal(err)
	}

	const phrase = "Embedded glyphs remain real text"

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(12))
		p.Content().Text(phrase)
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "embedded.pdf")
	txtPath := filepath.Join(dir, "embedded.txt")
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
	if !bytes.Contains(extracted, []byte("Embedded")) {
		t.Errorf("embedded-font text did not survive extraction; got:\n%s", extracted)
	}
}

// makePNG builds a small image with a transparent region, so the soft-mask path
// is exercised rather than just opaque RGB.
func makePNG(t *testing.T, w, h int, withAlpha bool) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			a := uint8(255)
			if withAlpha && x < w/2 {
				a = 90
			}
			img.Set(x, y, color.RGBA{
				R: uint8(x * 255 / w),
				G: uint8(y * 255 / h),
				B: 128,
				A: a,
			})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestPNGWithTransparencyEmbedsSoftMask(t *testing.T) {
	img, err := render.DecodeImage("logo", makePNG(t, 40, 20, true))
	if err != nil {
		t.Fatalf("decoding image: %v", err)
	}
	if img.PixelWidth != 40 || img.PixelHeight != 20 {
		t.Errorf("decoded size = %dx%d, want 40x20", img.PixelWidth, img.PixelHeight)
	}

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)
			c.Item().Width(120).Image(img)
			c.Item().Text("Below the image.")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	for _, want := range []string{"/Subtype /Image", "/SMask", "/ColorSpace /DeviceRGB"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}

	checkWithGhostscript(t, data)
}

func TestRepeatedImageIsEmbeddedOnce(t *testing.T) {
	img, err := render.DecodeImage("shared", makePNG(t, 30, 30, false))
	if err != nil {
		t.Fatal(err)
	}

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)
			// The same image many times over must not multiply the file size:
			// resources are pooled by key across the whole document.
			for i := 0; i < 12; i++ {
				c.Item().Width(60).Image(img)
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if got := bytes.Count(data, []byte("/Subtype /Image")); got != 1 {
		t.Errorf("image embedded %d times, want 1", got)
	}

	checkWithGhostscript(t, data)
}

func TestDecodeImageRejectsUnsupportedData(t *testing.T) {
	if _, err := render.DecodeImage("empty", nil); err == nil {
		t.Error("expected an error for empty image data")
	}
	if _, err := render.DecodeImage("garbage", []byte("not an image")); err == nil {
		t.Error("expected an error for data that is not an image")
	}
}

func TestTransparentColorsUseGraphicsState(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().Background(sanur.RGBA(255, 0, 0, 128)).Padding(20).Text("Half-opaque panel")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// PDF colour operators carry no alpha, so transparency has to arrive through
	// an ExtGState selected before the fill.
	for _, want := range []string{"/ExtGState", "/ca 0.502", "gs"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}

	checkWithGhostscript(t, data)
}
