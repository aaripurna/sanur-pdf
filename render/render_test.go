package render_test

import (
	"bytes"
	"errors"
	"image"
	"image/color"
	"image/jpeg"
	"image/png"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"testing/fstest"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/render"
)

var a4 = core.Size{Width: 595.28, Height: 841.89}

func helvetica() core.Font { return fonts.MustStandard(fonts.Helvetica) }

// drawOn runs draw on a single-page canvas and returns that page's content
// stream as text.
//
// Compression is disabled so the operators can be read back directly; asserting
// on the real emitted stream is the only way to catch a coordinate or operator
// mistake that still produces a structurally valid file.
func drawOn(t *testing.T, draw func(c *render.PDFCanvas)) string {
	t.Helper()

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	draw(canvas)

	if err := canvas.Close(); err != nil {
		t.Fatalf("closing page: %v", err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatalf("serialising document: %v", err)
	}
	return contentStream(t, data)
}

var streamPattern = regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`)

// contentStream returns the first stream that looks like page content rather
// than an embedded resource.
func contentStream(t *testing.T, data []byte) string {
	t.Helper()

	for _, m := range streamPattern.FindAllSubmatch(data, -1) {
		body := string(m[1])
		// Resource streams (fonts, images) are binary; a content stream is
		// operators, and every page begins with the axis-flipping transform.
		if strings.Contains(body, "cm") || strings.Contains(body, "re") {
			return body
		}
	}
	t.Fatalf("no content stream found in:\n%s", data)
	return ""
}

func requireContains(t *testing.T, got string, wants ...string) {
	t.Helper()
	for _, want := range wants {
		if !strings.Contains(got, want) {
			t.Errorf("content stream is missing %q; got:\n%s", want, got)
		}
	}
}

// --- coordinate system -----------------------------------------------------

func TestPageStartsWithAxisFlip(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {})

	// Layout works top-left origin with Y down; PDF user space is bottom-left
	// with Y up. One flip at the top of the page lets every element emit its own
	// coordinates verbatim.
	if !strings.HasPrefix(strings.TrimSpace(stream), "1 0 0 -1 0 841.89 cm") {
		t.Errorf("page does not begin with the Y-axis flip; got:\n%s", stream)
	}
}

func TestTranslateEmitsMatrixAndSkipsNoOps(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.Translate(core.Position{X: 10, Y: 20})
		c.Translate(core.Position{}) // no-op, must emit nothing
	})

	requireContains(t, stream, "1 0 0 1 10 20 cm")

	if got := strings.Count(stream, "1 0 0 1"); got != 1 {
		t.Errorf("emitted %d translation matrices, want 1 (the zero translate should be skipped)", got)
	}
}

func TestRotateEmitsRotationAndSkipsFullTurns(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.Rotate(90)
		c.Rotate(360) // a full turn is a no-op
		c.Rotate(0)
	})

	// cos 90 is 0 and sin 90 is 1, so a quarter turn is "0 1 -1 0 0 0 cm".
	requireContains(t, stream, "0 1 -1 0 0 0 cm")

	if got := strings.Count(stream, "cm"); got != 2 {
		t.Errorf("emitted %d transforms, want 2 (the page flip and one rotation)", got)
	}
}

func TestSaveRestoreEmitBalancedOperators(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.Save()
		c.Translate(core.Position{X: 5})
		c.Restore()
	})

	if strings.Count(stream, "q") != 1 || strings.Count(stream, "Q") != 1 {
		t.Errorf("unbalanced graphics state operators in:\n%s", stream)
	}
}

// --- shapes ----------------------------------------------------------------

func TestDrawRectEmitsFillInLayoutCoordinates(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{X: 10, Y: 20}, core.Size{Width: 100, Height: 50}, core.RGB(255, 0, 0))
	})

	// The rectangle is emitted exactly as laid out; the page-level flip handles
	// the conversion into PDF space.
	requireContains(t, stream, "1 0 0 rg", "10 20 100 50 re f")
}

func TestDrawRectSkipsInvisibleAndDegenerateShapes(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10}, core.Transparent)
		c.DrawRect(core.Position{}, core.Size{Width: 0, Height: 10}, core.RGB(0, 0, 0))
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: -5}, core.RGB(0, 0, 0))
	})

	if strings.Contains(stream, "re f") {
		t.Errorf("an invisible or degenerate rectangle produced a fill:\n%s", stream)
	}
}

func TestDrawRoundedRectEmitsCurves(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRoundedRect(core.Position{}, core.Size{Width: 100, Height: 60}, 8, core.RGB(0, 0, 255))
	})

	// PDF has no arc operator, so each of the four corners is a cubic Bézier.
	if got := strings.Count(stream, " c\n"); got != 4 {
		t.Errorf("emitted %d curves, want 4 (one per corner); got:\n%s", got, stream)
	}
	// The outline is closed and then filled. These are separate operators because
	// a rounded rectangle is a filled path like any other, sharing its corner
	// geometry with core.RoundedRect rather than duplicating it here.
	requireContains(t, stream, "\nh\nf\n")
}

func TestRoundedRectWithZeroRadiusFallsBackToPlainFill(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRoundedRect(core.Position{}, core.Size{Width: 50, Height: 50}, 0, core.RGB(0, 0, 0))
	})

	requireContains(t, stream, "re f")
	if strings.Contains(stream, " c\n") {
		t.Error("a zero radius should not produce curves")
	}
}

func TestRoundedRectClampsOversizedRadius(t *testing.T) {
	// A radius beyond half the shorter side would make opposite corners overlap
	// and self-intersect. It must be clamped, not rejected.
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRoundedRect(core.Position{}, core.Size{Width: 40, Height: 40}, 500, core.RGB(0, 0, 0))
	})

	if got := strings.Count(stream, " c\n"); got != 4 {
		t.Errorf("emitted %d curves, want 4; got:\n%s", got, stream)
	}
	// Clamped to 20, the outline is a circle: no straight run should exceed it.
	requireContains(t, stream, "20 0 m")
}

func TestDrawLineEmitsStroke(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawLine(core.Position{X: 0, Y: 5}, core.Position{X: 100, Y: 5}, core.RGB(0, 255, 0), 2)
	})

	requireContains(t, stream, "0 1 0 RG", "2 w", "0 5 m 100 5 l S")
}

func TestDrawLineSkipsInvisibleStrokes(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawLine(core.Position{}, core.Position{X: 10}, core.Transparent, 1)
		c.DrawLine(core.Position{}, core.Position{X: 10}, core.RGB(0, 0, 0), 0)
	})

	if strings.Contains(stream, " S") {
		t.Errorf("an invisible or zero-width line produced a stroke:\n%s", stream)
	}
}

// --- colour spaces ----------------------------------------------------------

func TestCMYKFillsUseThePlateOperator(t *testing.T) {
	// A CMYK colour has to reach the file as the plates it was written as. Emitting
	// rg would silently convert it, and a press cannot recover 100% K from a
	// four-plate approximation of it.
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10},
			core.CMYKPercent(0, 0, 0, 100))
	})

	requireContains(t, stream, "0 0 0 1 k")
	if strings.Contains(stream, "rg") {
		t.Errorf("a CMYK fill emitted an RGB operator:\n%s", stream)
	}
}

func TestCMYKStrokesUseTheUpperCasePlateOperator(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawLine(core.Position{}, core.Position{X: 10},
			core.CMYKPercent(100, 0, 0, 0), 1)
	})

	requireContains(t, stream, "1 0 0 0 K")
	if strings.Contains(stream, "RG") {
		t.Errorf("a CMYK stroke emitted an RGB operator:\n%s", stream)
	}
}

func TestCMYKTextUsesThePlateOperator(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawText("print", core.Position{X: 10, Y: 20}, core.TextStyle{
			Font:  fonts.MustStandard(fonts.Helvetica),
			Size:  12,
			Color: core.CMYKPercent(0, 0, 0, 100),
		})
	})

	requireContains(t, stream, "0 0 0 1 k")
}

func TestBothSpacesCoexistInOneStream(t *testing.T) {
	// PDF selects a colour space per operation, so an RGB chart and a CMYK logo can
	// share a page without either being converted.
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10}, core.RGB(255, 0, 0))
		c.DrawRect(core.Position{X: 20}, core.Size{Width: 10, Height: 10},
			core.CMYKPercent(0, 100, 100, 0))
	})

	requireContains(t, stream, "1 0 0 rg", "0 1 1 0 k")
}

func TestTranslucentCMYKStillSelectsAGraphicsState(t *testing.T) {
	// Opacity lives in a graphics state dictionary rather than in the colour
	// operands, so it has to work the same way in either space.
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10},
			core.CMYKPercent(0, 0, 0, 100).WithAlpha(128))
	})

	requireContains(t, stream, "0 0 0 1 k", "gs")
}

func TestClipRectEmitsClipPath(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.ClipRect(core.Position{X: 5, Y: 5}, core.Size{Width: 50, Height: 50})
	})

	requireContains(t, stream, "5 5 50 50 re W n")
}

func TestEmptyClipStillRestrictsDrawing(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.ClipRect(core.Position{}, core.Size{})
	})

	// Skipping an empty clip would let content that should be fully hidden draw
	// at full size, which is the opposite of what was asked.
	requireContains(t, stream, "0 0 0 0 re W n")
}

// --- text ------------------------------------------------------------------

func TestDrawTextCounterFlipsTheTextMatrix(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawText("Hi", core.Position{X: 12, Y: 34}, core.TextStyle{
			Font:  helvetica(),
			Size:  11,
			Color: core.RGB(0, 0, 0),
		})
	})

	// Composed with the page flip, a text matrix of "1 0 0 -1 x y" lands the
	// baseline at (x, y) with the glyphs upright rather than mirrored.
	requireContains(t, stream, "BT", "/F0 11 Tf", "1 0 0 -1 12 34 Tm", "(Hi) Tj", "ET")
}

func TestDrawTextEmitsSpacingOperators(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawText("a b", core.Position{}, core.TextStyle{
			Font:          helvetica(),
			Size:          10,
			Color:         core.RGB(0, 0, 0),
			LetterSpacing: 1.5,
			WordSpacing:   3,
		})
	})

	// Justification relies on Tw, so it must reach the stream.
	requireContains(t, stream, "1.5 Tc", "3 Tw")
}

func TestDrawTextSkipsNothingToDraw(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		style := core.TextStyle{Font: helvetica(), Size: 11, Color: core.RGB(0, 0, 0)}

		c.DrawText("", core.Position{}, style)

		invisible := style
		invisible.Color = core.Transparent
		c.DrawText("hidden", core.Position{}, invisible)

		zero := style
		zero.Size = 0
		c.DrawText("tiny", core.Position{}, zero)
	})

	if strings.Contains(stream, "BT") {
		t.Errorf("text with nothing to draw still opened a text object:\n%s", stream)
	}
}

func TestDrawTextWithoutFontFails(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	canvas.DrawText("no font", core.Position{}, core.TextStyle{Size: 11, Color: core.RGB(0, 0, 0)})

	if canvas.Err() == nil {
		t.Fatal("expected an error when drawing text with no font set")
	}
	// Draw has no error return, so failures surface through the canvas and are
	// reported once the page closes.
	if err := canvas.Close(); err == nil {
		t.Error("Close should surface the recorded failure")
	}
}

func TestTextDecorationsAreStroked(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawText("underlined", core.Position{X: 0, Y: 50}, core.TextStyle{
			Font:      helvetica(),
			Size:      12,
			Color:     core.RGB(0, 0, 0),
			Underline: true,
			Strikeout: true,
		})
	})

	// PDF has no decoration property, so each rule is a stroked line.
	if got := strings.Count(stream, " l S"); got != 2 {
		t.Errorf("got %d decoration strokes, want 2 (underline and strikeout); stream:\n%s", got, stream)
	}
}

// --- resources -------------------------------------------------------------

func TestFontsAreDeduplicatedAcrossPages(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	style := core.TextStyle{Font: helvetica(), Size: 11, Color: core.RGB(0, 0, 0)}

	for i := 0; i < 3; i++ {
		canvas := b.NewPage(a4)
		canvas.DrawText("repeated", core.Position{X: 0, Y: 20}, style)
		if err := canvas.Close(); err != nil {
			t.Fatal(err)
		}
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if got := bytes.Count(data, []byte("/BaseFont /Helvetica")); got != 1 {
		t.Errorf("font emitted %d times across 3 pages, want 1", got)
	}
	if got := bytes.Count(data, []byte("/Type /Page\n")) + bytes.Count(data, []byte("/Type /Page ")); got != 3 {
		t.Errorf("page count = %d, want 3", got)
	}
}

func TestDistinctFontsGetDistinctResourceNames(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		regular := core.TextStyle{Font: helvetica(), Size: 10, Color: core.RGB(0, 0, 0)}
		bold := regular
		bold.Font = fonts.MustStandard(fonts.HelveticaBold)

		c.DrawText("regular", core.Position{Y: 10}, regular)
		c.DrawText("bold", core.Position{Y: 30}, bold)
	})

	requireContains(t, stream, "/F0 10 Tf", "/F1 10 Tf")
}

func TestStandard14FontsCarryNoDescriptor(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawText("built in", core.Position{Y: 20}, core.TextStyle{
		Font: helvetica(), Size: 11, Color: core.RGB(0, 0, 0),
	})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// A reader resolves a standard-14 font from its name alone. Supplying a
	// Widths array is what makes readers demand a full descriptor too.
	requireContains(t, string(data), "/Subtype /Type1", "/Encoding /WinAnsiEncoding")
	if bytes.Contains(data, []byte("/FontDescriptor")) {
		t.Error("a built-in font should not emit a font descriptor")
	}
	if bytes.Contains(data, []byte("/FontFile2")) {
		t.Error("a built-in font should not embed a font program")
	}
}

func TestTranslucentFillsSelectAGraphicsState(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10}, core.RGBA(255, 0, 0, 128))
	})

	// Colour operators carry no alpha, so transparency arrives via ExtGState,
	// wrapped in q/Q so it cannot leak into later drawing.
	requireContains(t, stream, "/GS0 gs", "q", "Q")
}

func TestOpaqueFillsSkipTheGraphicsState(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10}, core.RGB(255, 0, 0))
	})

	if strings.Contains(stream, "gs") {
		t.Errorf("an opaque fill should need no graphics state:\n%s", stream)
	}
}

func TestAlphaStatesArePooledAndOrdered(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	// Two distinct alphas used twice each should yield exactly two states.
	for _, a := range []uint8{128, 64, 128, 64} {
		canvas.DrawRect(core.Position{}, core.Size{Width: 5, Height: 5}, core.RGBA(0, 0, 0, a))
	}
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if got := bytes.Count(data, []byte("/Type /ExtGState")); got != 2 {
		t.Errorf("emitted %d graphics states, want 2", got)
	}
}

// --- images ----------------------------------------------------------------

func encodePNG(t *testing.T, w, h int, alpha uint8) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: 200, G: 100, B: 50, A: alpha})
		}
	}

	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func encodeJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	img := image.NewRGBA(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			img.Set(x, y, color.RGBA{R: uint8(x), G: uint8(y), B: 90, A: 255})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, img, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestDecodeImageReadsDimensionsAndFormat(t *testing.T) {
	for _, tc := range []struct {
		name   string
		data   []byte
		format string
	}{
		{"png", encodePNG(t, 40, 25, 255), "png"},
		{"jpeg", encodeJPEG(t, 32, 16), "jpeg"},
	} {
		img, err := render.DecodeImage(tc.name, tc.data)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if img.Format != tc.format {
			t.Errorf("%s: format = %q, want %q", tc.name, img.Format, tc.format)
		}
		if img.PixelWidth == 0 || img.PixelHeight == 0 {
			t.Errorf("%s: dimensions not read (%dx%d)", tc.name, img.PixelWidth, img.PixelHeight)
		}
		// The encoded bytes must be kept as supplied so a JPEG can be embedded
		// without requantisation.
		if !bytes.Equal(img.Data, tc.data) {
			t.Errorf("%s: image data was altered during decode", tc.name)
		}
	}
}

func TestDecodeImageRejectsBadInput(t *testing.T) {
	for _, tc := range []struct {
		name string
		data []byte
	}{
		{"empty", nil},
		{"garbage", []byte("definitely not an image")},
	} {
		if _, err := render.DecodeImage(tc.name, tc.data); err == nil {
			t.Errorf("%s: expected an error", tc.name)
		}
	}
}

func TestJPEGIsEmbeddedVerbatim(t *testing.T) {
	raw := encodeJPEG(t, 32, 16)
	img, err := render.DecodeImage("photo", raw)
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{X: 10, Y: 10}, core.Size{Width: 64, Height: 32})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// DCTDecode is PDF's built-in JPEG codec, so the original file passes
	// straight through.
	requireContains(t, string(data), "/Filter /DCTDecode", "/Subtype /Image")
	if !bytes.Contains(data, raw) {
		t.Error("the original JPEG bytes are not present in the output")
	}
}

func TestOpaquePNGNeedsNoSoftMask(t *testing.T) {
	img, err := render.DecodeImage("opaque", encodePNG(t, 20, 20, 255))
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{}, core.Size{Width: 20, Height: 20})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(data, []byte("/SMask")) {
		t.Error("a fully opaque image should not carry a soft mask")
	}
}

func TestTranslucentPNGGetsSoftMask(t *testing.T) {
	img, err := render.DecodeImage("translucent", encodePNG(t, 20, 20, 90))
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{}, core.Size{Width: 20, Height: 20})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// A PDF image's colour samples have no alpha channel, so transparency has to
	// travel as a separate greyscale mask.
	requireContains(t, string(data), "/SMask", "/ColorSpace /DeviceGray")
}

func TestDrawImagePositionsViaTransform(t *testing.T) {
	img, err := render.DecodeImage("placed", encodePNG(t, 10, 10, 255))
	if err != nil {
		t.Fatal(err)
	}

	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawImage(img, core.Position{X: 10, Y: 20}, core.Size{Width: 100, Height: 50})
	})

	// An image XObject draws into the unit square, so placement and scale are
	// entirely the transform's job. The negative Y scale and the offset by the
	// full height account for the image's bottom-up origin.
	requireContains(t, stream, "100 0 0 -50 10 70 cm", "/Im0 Do")
}

func TestDrawImageSkipsEmptyGeometry(t *testing.T) {
	img, err := render.DecodeImage("skipped", encodePNG(t, 10, 10, 255))
	if err != nil {
		t.Fatal(err)
	}

	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawImage(img, core.Position{}, core.Size{Width: 0, Height: 10})
		c.DrawImage(core.Image{Key: "no data"}, core.Position{}, core.Size{Width: 10, Height: 10})
		_ = img
	})

	if strings.Contains(stream, "Do") {
		t.Errorf("an image with no data or no size was still drawn:\n%s", stream)
	}
}

func TestEncodeJPEGProducesEmbeddableImage(t *testing.T) {
	src := image.NewRGBA(image.Rect(0, 0, 24, 12))
	for y := 0; y < 12; y++ {
		for x := 0; x < 24; x++ {
			src.Set(x, y, color.RGBA{R: 30, G: 60, B: 90, A: 255})
		}
	}

	img, err := render.EncodeJPEG("encoded", src, 80)
	if err != nil {
		t.Fatalf("EncodeJPEG: %v", err)
	}
	if img.Format != "jpeg" {
		t.Errorf("format = %q, want jpeg", img.Format)
	}
	if img.PixelWidth != 24 || img.PixelHeight != 12 {
		t.Errorf("dimensions = %dx%d, want 24x12", img.PixelWidth, img.PixelHeight)
	}
}

// --- builder and error handling --------------------------------------------

func TestBuilderRejectsDocumentWithNoPages(t *testing.T) {
	if _, err := render.NewBuilder(render.Metadata{}, true).Bytes(); err == nil {
		t.Error("expected an error for a document with no pages")
	}
}

func TestPageCountTracksAddedPages(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)

	if b.PageCount() != 0 {
		t.Errorf("PageCount = %d on a fresh builder, want 0", b.PageCount())
	}
	for i := 1; i <= 2; i++ {
		canvas := b.NewPage(a4)
		if err := canvas.Close(); err != nil {
			t.Fatal(err)
		}
		if b.PageCount() != i {
			t.Errorf("PageCount = %d after %d pages", b.PageCount(), i)
		}
	}
}

func TestUnbalancedSaveIsReportedOnClose(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.Save() // never restored

	err := canvas.Close()
	if err == nil {
		t.Fatal("expected an error for an unbalanced Save")
	}
	// A leaked graphics state would silently misplace everything drawn after it,
	// so it has to be caught rather than tolerated.
	if !strings.Contains(err.Error(), "unbalanced") {
		t.Errorf("error %q does not explain the imbalance", err)
	}
}

func TestRestoreWithoutSaveIsRecorded(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.Restore()

	if canvas.Err() == nil {
		t.Error("expected Restore without Save to record a failure")
	}
}

func TestCanvasKeepsTheFirstError(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	first := errorf("first")
	canvas.Fail(first)
	canvas.Fail(errorf("second"))
	canvas.Fail(nil)

	if canvas.Err() != first {
		t.Errorf("Err = %v, want the first failure", canvas.Err())
	}
}

func TestCloseIsIdempotent(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}
	if err := canvas.Close(); err != nil {
		t.Errorf("second Close returned %v, want nil", err)
	}
	if b.PageCount() != 1 {
		t.Errorf("PageCount = %d, want 1: closing twice must not add the page twice", b.PageCount())
	}
}

func TestCanvasSizeIsReported(t *testing.T) {
	canvas := render.NewBuilder(render.Metadata{}, false).NewPage(a4)

	if canvas.Size() != a4 {
		t.Errorf("Size = %v, want %v", canvas.Size(), a4)
	}
}

func TestMetadataReachesTheInfoDictionary(t *testing.T) {
	meta := render.Metadata{
		Title:        "The Title",
		Author:       "The Author",
		Subject:      "The Subject",
		Keywords:     "a, b",
		Creator:      "The Creator",
		Producer:     "sanur",
		CreationDate: "D:20260730120000Z",
	}

	b := render.NewBuilder(meta, false)
	if err := b.NewPage(a4).Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	requireContains(t, string(data),
		"/Title (The Title)", "/Author (The Author)", "/Subject (The Subject)",
		"/Keywords (a, b)", "/Creator (The Creator)", "/Producer (sanur)",
		"/CreationDate (D:20260730120000Z)", "/Info")
}

func TestNoMetadataOmitsTheInfoDictionary(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	if err := b.NewPage(a4).Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if bytes.Contains(data, []byte("/Info")) {
		t.Error("an empty metadata set should emit no info dictionary")
	}
}

func TestCompressionShrinksRepetitiveContent(t *testing.T) {
	build := func(compress bool) []byte {
		t.Helper()

		b := render.NewBuilder(render.Metadata{}, compress)
		canvas := b.NewPage(a4)
		for i := 0; i < 400; i++ {
			canvas.DrawRect(
				core.Position{X: float64(i), Y: float64(i)},
				core.Size{Width: 10, Height: 10},
				core.RGB(1, 2, 3))
		}
		if err := canvas.Close(); err != nil {
			t.Fatal(err)
		}
		data, err := b.Bytes()
		if err != nil {
			t.Fatal(err)
		}
		return data
	}

	compressed, plain := build(true), build(false)

	if !bytes.Contains(compressed, []byte("/FlateDecode")) {
		t.Error("a large content stream was not compressed")
	}
	if len(compressed) >= len(plain) {
		t.Errorf("compressed output (%d bytes) is not smaller than plain (%d)",
			len(compressed), len(plain))
	}
}

// --- discard canvas --------------------------------------------------------

func TestDiscardCanvasAcceptsEveryOperation(t *testing.T) {
	c := render.NewDiscardCanvas()

	// The counting pass drives a full draw through this canvas, so every method
	// has to be safe to call and none may panic.
	c.Save()
	c.Translate(core.Position{X: 1, Y: 2})
	c.Rotate(45)
	c.ClipRect(core.Position{}, core.Size{Width: 10, Height: 10})
	c.DrawRect(core.Position{}, core.Size{Width: 10, Height: 10}, core.RGB(0, 0, 0))
	c.DrawRoundedRect(core.Position{}, core.Size{Width: 10, Height: 10}, 2, core.RGB(0, 0, 0))
	c.DrawLine(core.Position{}, core.Position{X: 5}, core.RGB(0, 0, 0), 1)
	c.DrawText("text", core.Position{}, core.TextStyle{Font: helvetica(), Size: 10})
	c.DrawImage(core.Image{}, core.Position{}, core.Size{Width: 1, Height: 1})
	c.Restore()

	if c.Err() != nil {
		t.Errorf("Err = %v, want nil", c.Err())
	}
}

func TestDiscardCanvasStillCollectsFailures(t *testing.T) {
	c := render.NewDiscardCanvas()

	first := errorf("bad font")
	c.Fail(first)
	c.Fail(errorf("later problem"))

	// Failures matter even on a discarded pass: an unembeddable font is worth
	// reporting before the second pass repeats the same work.
	if c.Err() != first {
		t.Errorf("Err = %v, want the first failure", c.Err())
	}
}

type stringError string

func (e stringError) Error() string { return string(e) }

func errorf(s string) error { return stringError(s) }

func TestLoadImageFileReadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(path, encodePNG(t, 30, 15, 255), 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := render.LoadImageFile("explicit", path)
	if err != nil {
		t.Fatalf("LoadImageFile: %v", err)
	}
	if img.Key != "explicit" {
		t.Errorf("key = %q, want %q", img.Key, "explicit")
	}
	if img.PixelWidth != 30 || img.PixelHeight != 15 {
		t.Errorf("dimensions = %dx%d, want 30x15", img.PixelWidth, img.PixelHeight)
	}
}

func TestLoadImageFileDefaultsKeyToPath(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sample.png")
	if err := os.WriteFile(path, encodePNG(t, 10, 10, 255), 0o644); err != nil {
		t.Fatal(err)
	}

	img, err := render.LoadImageFile("", path)
	if err != nil {
		t.Fatal(err)
	}
	// The key is what pools the image across the document, so defaulting it to
	// the path means loading the same file twice costs its bytes once.
	if img.Key != path {
		t.Errorf("key = %q, want the path %q", img.Key, path)
	}
}

func TestLoadImageFileReportsAMissingFile(t *testing.T) {
	if _, err := render.LoadImageFile("absent", "/no/such/image.png"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestLoadImageFSReadsFromAFilesystem(t *testing.T) {
	fsys := fstest.MapFS{
		"art/logo.png": &fstest.MapFile{Data: encodePNG(t, 24, 12, 255)},
	}

	img, err := render.LoadImageFS(fsys, "logo", "art/logo.png")
	if err != nil {
		t.Fatalf("LoadImageFS: %v", err)
	}
	if img.PixelWidth != 24 || img.PixelHeight != 12 {
		t.Errorf("dimensions = %dx%d, want 24x12", img.PixelWidth, img.PixelHeight)
	}

	// An empty key falls back to the name within the filesystem.
	named, err := render.LoadImageFS(fsys, "", "art/logo.png")
	if err != nil {
		t.Fatal(err)
	}
	if named.Key != "art/logo.png" {
		t.Errorf("key = %q, want %q", named.Key, "art/logo.png")
	}

	if _, err := render.LoadImageFS(fsys, "gone", "art/missing.png"); err == nil {
		t.Error("expected an error for a name not in the filesystem")
	}
}

// --- paths ------------------------------------------------------------------

func TestDrawPathEmitsConstructionOperators(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		path := core.NewPath().
			MoveTo(core.Position{X: 10, Y: 20}).
			LineTo(core.Position{X: 30, Y: 20}).
			CurveTo(
				core.Position{X: 40, Y: 20},
				core.Position{X: 50, Y: 30},
				core.Position{X: 50, Y: 40}).
			Close()

		c.DrawPath(path, core.Filled(core.RGB(255, 0, 0)))
	})

	requireContains(t, stream,
		"10 20 m",
		"30 20 l",
		"40 20 50 30 50 40 c",
		"\nh\nf\n")
}

func TestDrawPathSelectsThePaintingOperator(t *testing.T) {
	black := core.RGB(0, 0, 0)

	for _, tc := range []struct {
		name   string
		style  core.PathStyle
		want   string
		absent string
	}{
		{"fill only", core.Filled(black), "\nf\n", "\nS\n"},
		{"stroke only", core.Stroked(black, 2), "\nS\n", "\nf\n"},
		// One operator does both, so the shared boundary is composited once
		// rather than twice.
		{"both", core.PathStyle{Fill: black, Stroke: black, Width: 1}, "\nB\n", "\nf\n"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := drawOn(t, func(c *render.PDFCanvas) {
				c.DrawPath(core.Polygon(
					core.Position{X: 0, Y: 0},
					core.Position{X: 10, Y: 0},
					core.Position{X: 10, Y: 10},
				), tc.style)
			})

			requireContains(t, stream, tc.want)
			if strings.Contains(stream, tc.absent) {
				t.Errorf("stream should not contain %q:\n%s", tc.absent, stream)
			}
		})
	}
}

func TestDrawPathSkipsNothingToPaint(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		square := core.Polygon(
			core.Position{X: 0, Y: 0},
			core.Position{X: 5, Y: 0},
			core.Position{X: 5, Y: 5})

		// An empty path, an invisible style, and a stroke with no width all put no
		// ink on the page.
		c.DrawPath(core.NewPath(), core.Filled(core.RGB(0, 0, 0)))
		c.DrawPath(square, core.PathStyle{})
		c.DrawPath(square, core.PathStyle{Stroke: core.RGB(0, 0, 0)})
		c.DrawPath(nil, core.Filled(core.RGB(0, 0, 0)))
	})

	for _, op := range []string{"\nf\n", "\nS\n", "\nB\n", " m\n"} {
		if strings.Contains(stream, op) {
			t.Errorf("expected no painting, but found %q:\n%s", op, stream)
		}
	}
}

func TestStrokeStateOperators(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(
			core.Polyline(core.Position{}, core.Position{X: 50, Y: 50}),
			core.PathStyle{
				Stroke:    core.RGB(0, 0, 255),
				Width:     3,
				Cap:       core.CapRound,
				Join:      core.JoinBevel,
				Dash:      []float64{4, 2},
				DashPhase: 1,
			})
	})

	requireContains(t, stream,
		"0 0 1 RG",
		"3 w",
		"1 J", // round cap
		"2 j", // bevel join
		"[4 2] 1 d")
}

func TestStrokeStateOmitsDefaults(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(
			core.Polyline(core.Position{}, core.Position{X: 10, Y: 0}),
			core.Stroked(core.RGB(0, 0, 0), 1))
	})

	// Butt caps, mitre joins and a solid line are the PDF defaults, so emitting
	// them would be noise.
	for _, op := range []string{" J", " j", " d", " M"} {
		if strings.Contains(stream, op) {
			t.Errorf("default stroke state should not be emitted, found %q:\n%s", op, stream)
		}
	}
	requireContains(t, stream, "1 w")
}

func TestAllZeroDashIsTreatedAsSolid(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(
			core.Polyline(core.Position{}, core.Position{X: 10, Y: 0}),
			core.PathStyle{Stroke: core.RGB(0, 0, 0), Width: 1, Dash: []float64{0, 0}})
	})

	// A pattern of zeros would draw nothing at all and is rejected by readers.
	if strings.Contains(stream, " d\n") {
		t.Errorf("an all-zero dash pattern should not be emitted:\n%s", stream)
	}
}

func TestMiterLimitOnlyWhenItDiffersFromTheDefault(t *testing.T) {
	custom := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(core.Polyline(core.Position{}, core.Position{X: 10, Y: 10}),
			core.PathStyle{Stroke: core.RGB(0, 0, 0), Width: 2, MiterLimit: 4})
	})
	requireContains(t, custom, "4 M")

	def := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(core.Polyline(core.Position{}, core.Position{X: 10, Y: 10}),
			core.PathStyle{Stroke: core.RGB(0, 0, 0), Width: 2,
				MiterLimit: core.DefaultMiterLimit})
	})
	if strings.Contains(def, " M\n") {
		t.Errorf("the default miter limit should not be emitted:\n%s", def)
	}
}

func TestStrokeStateIsScopedToItsPath(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(core.Polyline(core.Position{}, core.Position{X: 10, Y: 0}),
			core.PathStyle{Stroke: core.RGB(0, 0, 0), Width: 6, Dash: []float64{3}})
		c.DrawRect(core.Position{}, core.Size{Width: 5, Height: 5}, core.RGB(0, 0, 0))
	})

	// Width, cap, join and dash are graphics state. Without a q/Q around the
	// stroke, the dash pattern would persist into everything drawn afterwards.
	dashAt := strings.Index(stream, " d\n")
	restoreAt := strings.Index(stream[dashAt:], "\nQ\n")
	rectAt := strings.Index(stream, "re f")

	if dashAt < 0 || restoreAt < 0 || rectAt < 0 {
		t.Fatalf("expected a dashed stroke, a restore and a rectangle:\n%s", stream)
	}
	if dashAt+restoreAt > rectAt {
		t.Errorf("the stroke state was not restored before the next shape:\n%s", stream)
	}
}

func TestPathAlphaUsesSeparateFillAndStrokeOpacity(t *testing.T) {
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	canvas.DrawPath(
		core.Polygon(core.Position{}, core.Position{X: 10}, core.Position{X: 10, Y: 10}),
		core.PathStyle{
			Fill:   core.RGBA(255, 0, 0, 128),
			Stroke: core.RGBA(0, 0, 255, 64),
			Width:  1,
		})

	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}
	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// PDF carries fill and stroke opacity separately, so a translucent fill under
	// a differently translucent outline needs both set on one state.
	requireContains(t, string(data), "/ca 0.502", "/CA 0.251")
}

func TestArcRendersAsCurves(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		// A pie slice: centre, out to the rim, sweep, close.
		slice := core.NewPath().
			MoveTo(core.Position{X: 100, Y: 100}).
			Arc(core.Position{X: 100, Y: 100}, 50, 0, 90).
			Close()

		c.DrawPath(slice, core.Filled(core.RGB(0, 128, 255)))
	})

	requireContains(t, stream, "100 100 m", "150 100 l", "\nh\nf\n")
	if got := strings.Count(stream, " c\n"); got != 1 {
		t.Errorf("a quarter turn should emit 1 curve, got %d:\n%s", got, stream)
	}
}

func TestRoundedRectDelegatesToThePathAPI(t *testing.T) {
	// The corner geometry lives in core.RoundedRect, so both routes must agree
	// exactly — otherwise a background and a chart panel would round differently.
	viaHelper := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawRoundedRect(core.Position{X: 5, Y: 5},
			core.Size{Width: 80, Height: 40}, 6, core.RGB(10, 20, 30))
	})

	viaPath := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(
			core.RoundedRect(core.Position{X: 5, Y: 5},
				core.Size{Width: 80, Height: 40}, 6),
			core.Filled(core.RGB(10, 20, 30)))
	})

	if viaHelper != viaPath {
		t.Errorf("DrawRoundedRect and the path API disagree:\n--- helper ---\n%s\n--- path ---\n%s",
			viaHelper, viaPath)
	}
}

func TestEvenOddFillUsesStarredOperators(t *testing.T) {
	// A ring is two circles wound the same way. Under nonzero winding the middle
	// counts as inside and fills solid, so the hole only appears with even-odd.
	ring := func() *core.Path {
		centre := core.Position{X: 50, Y: 50}
		return core.NewPath().Circle(centre, 40).Circle(centre, 20)
	}

	nonzero := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(ring(), core.Filled(core.RGB(0, 128, 128)))
	})
	requireContains(t, nonzero, "\nf\n")
	if strings.Contains(nonzero, "f*") {
		t.Errorf("nonzero winding should use the plain operator:\n%s", nonzero)
	}

	evenOdd := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(ring(), core.PathStyle{Fill: core.RGB(0, 128, 128), EvenOdd: true})
	})
	requireContains(t, evenOdd, "\nf*\n")
}

func TestEvenOddAppliesToFillAndStrokeToo(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		centre := core.Position{X: 40, Y: 40}
		c.DrawPath(
			core.NewPath().Circle(centre, 30).Circle(centre, 15),
			core.PathStyle{
				Fill:    core.RGB(0, 0, 0),
				Stroke:  core.RGB(255, 255, 255),
				Width:   1,
				EvenOdd: true,
			})
	})

	requireContains(t, stream, "\nB*\n")
}

func TestStrokeOnlyIgnoresTheFillRule(t *testing.T) {
	stream := drawOn(t, func(c *render.PDFCanvas) {
		c.DrawPath(
			core.Polygon(core.Position{}, core.Position{X: 10}, core.Position{X: 10, Y: 10}),
			core.PathStyle{Stroke: core.RGB(0, 0, 0), Width: 1, EvenOdd: true})
	})

	// A stroke traces the outline, so there is no interior for a fill rule to
	// classify; the operator must not be starred.
	requireContains(t, stream, "\nS\n")
	if strings.Contains(stream, "S*") {
		t.Errorf("a stroke has no fill rule:\n%s", stream)
	}
}

// --- JPEG colour spaces -----------------------------------------------------

// encodeGrayJPEG produces a genuine single-channel JPEG. Go's encoder takes this
// path for *image.Gray, which is what makes the greyscale case testable
// end to end; it has no four-channel path, so CMYK is covered by unit tests
// against synthesised headers instead.
func encodeGrayJPEG(t *testing.T, w, h int) []byte {
	t.Helper()

	g := image.NewGray(image.Rect(0, 0, w, h))
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			g.SetGray(x, y, color.Gray{Y: uint8(x * 255 / w)})
		}
	}

	var buf bytes.Buffer
	if err := jpeg.Encode(&buf, g, nil); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestGrayscaleJPEGDeclaresDeviceGray(t *testing.T) {
	img, err := render.DecodeImage("gray", encodeGrayJPEG(t, 40, 20))
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{}, core.Size{Width: 80, Height: 40})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// A single-channel JPEG labelled DeviceRGB is what readers reject: three
	// components are promised and one arrives.
	requireContains(t, string(data), "/ColorSpace /DeviceGray", "/Filter /DCTDecode")
	if bytes.Contains(data, []byte("/ColorSpace /DeviceRGB")) {
		t.Error("a greyscale JPEG was labelled DeviceRGB")
	}
}

func TestColourJPEGStillDeclaresDeviceRGB(t *testing.T) {
	img, err := render.DecodeImage("colour", encodeJPEG(t, 32, 16))
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{}, core.Size{Width: 64, Height: 32})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	requireContains(t, string(data), "/ColorSpace /DeviceRGB")
}

func TestGrayscaleJPEGRendersCleanly(t *testing.T) {
	// The regression this guards: Ghostscript reported a recoverable image error
	// for every greyscale JPEG, because the colour space was hardcoded.
	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}

	img, err := render.DecodeImage("gray", encodeGrayJPEG(t, 60, 40))
	if err != nil {
		t.Fatal(err)
	}

	b := render.NewBuilder(render.Metadata{}, true)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{X: 40, Y: 40}, core.Size{Width: 120, Height: 80})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "gray.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(gs,
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-sDEVICE=nullpage", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ghostscript rejected the document: %v\n%s", err, out)
	}
	if bytes.Contains(bytes.ToLower(out), []byte("error")) {
		t.Errorf("ghostscript complained about a greyscale JPEG:\n%s", out)
	}
}

func TestMalformedJPEGIsReported(t *testing.T) {
	// DecodeImage accepts anything image.DecodeConfig can read, so a file that
	// parses as an image but has no readable frame header has to fail later, with
	// the image named.
	img := core.Image{
		Key:        "broken",
		Format:     "jpeg",
		Data:       []byte{0xFF, 0xD8, 0xFF, 0xD9},
		PixelWidth: 10, PixelHeight: 10,
	}

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)
	canvas.DrawImage(img, core.Position{}, core.Size{Width: 10, Height: 10})

	if canvas.Err() == nil {
		t.Fatal("expected a malformed JPEG to be reported")
	}
	if !strings.Contains(canvas.Err().Error(), "broken") {
		t.Errorf("error %q does not name the image", canvas.Err())
	}
}

// --- font emission failures -------------------------------------------------

// stubFace is a core.Font with predictable metrics, used as the base for the
// deliberately broken fonts below.
type stubFace struct{ name string }

func (f stubFace) Name() string                    { return f.name }
func (f stubFace) AdvanceOf(rune, float64) float64 { return 1 }
func (f stubFace) Measure(string, float64) float64 { return 1 }
func (f stubFace) Ascent(float64) float64          { return 8 }
func (f stubFace) Descent(float64) float64         { return 2 }
func (f stubFace) LineHeight(float64) float64      { return 12 }

// neitherKind describes itself as a font the writer has no way to emit: not a
// standard-14 name the reader can resolve, and not a composite font with a program
// to embed. Nothing sanur ships does this, but a caller's own core.Font can.
type neitherKind struct{ stubFace }

func (neitherKind) Program() fonts.FontProgram {
	return fonts.FontProgram{BaseName: "Neither"}
}

// compositeWithoutGlyphs claims to be composite but cannot map a rune to a glyph,
// which would otherwise crash on the first character drawn.
type compositeWithoutGlyphs struct{ stubFace }

func (compositeWithoutGlyphs) Program() fonts.FontProgram {
	return fonts.FontProgram{BaseName: "Incomplete", Composite: true}
}

// brokenSubset is composite and maps glyphs, but cannot produce a program.
type brokenSubset struct {
	stubFace
	empty bool
}

func (brokenSubset) Program() fonts.FontProgram {
	return fonts.FontProgram{BaseName: "Broken", Composite: true}
}

func (brokenSubset) GlyphID(r rune) (uint16, bool) { return uint16(r), true }
func (brokenSubset) SubstituteGlyph() uint16       { return 0 }
func (brokenSubset) GlyphWidth(uint16) int         { return 500 }

func (f brokenSubset) Subset(map[uint16]bool) (fonts.Subset, error) {
	if f.empty {
		return fonts.Subset{}, nil
	}
	return fonts.Subset{}, errors.New("no outlines available")
}

// drawWith renders one string with a font and returns whatever went wrong.
//
// The failure has to surface as an error from generation rather than as a panic or a
// file with a broken font dictionary in it, which is the only reason these types
// exist: every one of them is a mistake a caller can make in its own core.Font.
func drawWith(t *testing.T, face core.Font) error {
	t.Helper()

	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	canvas.DrawText("text", core.Position{X: 10, Y: 20}, core.TextStyle{
		Font: face, Size: 11, Color: core.RGB(0, 0, 0),
	})

	if err := canvas.Close(); err != nil {
		return err
	}

	_, err := b.Bytes()
	return err
}

func TestFontThatIsNeitherStandard14NorCompositeIsRejected(t *testing.T) {
	err := drawWith(t, neitherKind{stubFace{"Neither"}})
	if err == nil {
		t.Fatal("a font with no way to be embedded was accepted")
	}
	for _, want := range []string{"Neither", "composite"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestCompositeFontWithoutAGlyphSourceIsRejected(t *testing.T) {
	// Caught at registration rather than at the first draw call, so the message names
	// the interface to implement instead of arriving as a nil dereference.
	err := drawWith(t, compositeWithoutGlyphs{stubFace{"Incomplete"}})
	if err == nil {
		t.Fatal("a composite font with no glyph source was accepted")
	}
	for _, want := range []string{"Incomplete", "GlyphSource"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestSubsettingFailureIsReported(t *testing.T) {
	err := drawWith(t, brokenSubset{stubFace: stubFace{"Broken"}})
	if err == nil {
		t.Fatal("a font that cannot produce a program was accepted")
	}
	if !strings.Contains(err.Error(), "no outlines available") {
		t.Errorf("error %q does not carry the subsetter's own message", err)
	}
}

func TestEmptyFontProgramIsReported(t *testing.T) {
	// An empty stream would produce a font dictionary pointing at nothing, which a
	// reader reports as a damaged file rather than as a missing font.
	err := drawWith(t, brokenSubset{stubFace: stubFace{"Empty"}, empty: true})
	if err == nil {
		t.Fatal("an empty font program was accepted")
	}
	if !strings.Contains(err.Error(), "empty program") {
		t.Errorf("error %q does not say the program was empty", err)
	}
}

func TestCompositeFontWithoutADeclaredWidthFallsBackToThePDFDefault(t *testing.T) {
	// /DW is what a reader uses for any glyph absent from /W. A zero would collapse
	// those glyphs to no width at all, so the PDF default stands in.
	b := render.NewBuilder(render.Metadata{}, false)
	canvas := b.NewPage(a4)

	canvas.DrawText("A", core.Position{X: 10, Y: 20}, core.TextStyle{
		Font:  brokenSubsetThatWorks{stubFace{"NoDefault"}},
		Size:  11,
		Color: core.RGB(0, 0, 0),
	})
	if err := canvas.Close(); err != nil {
		t.Fatal(err)
	}

	data, err := b.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(data, []byte("/DW 1000")) {
		t.Errorf("no default width emitted:\n%s", data)
	}
}

// brokenSubsetThatWorks is composite, maps glyphs and produces a program, but
// declares no default width.
type brokenSubsetThatWorks struct{ stubFace }

func (brokenSubsetThatWorks) Program() fonts.FontProgram {
	return fonts.FontProgram{BaseName: "NoDefault", Composite: true}
}

func (brokenSubsetThatWorks) GlyphID(r rune) (uint16, bool) { return uint16(r), true }
func (brokenSubsetThatWorks) SubstituteGlyph() uint16       { return 0 }
func (brokenSubsetThatWorks) GlyphWidth(uint16) int         { return 500 }

func (brokenSubsetThatWorks) Subset(map[uint16]bool) (fonts.Subset, error) {
	return fonts.Subset{Data: []byte("not really a font, but not empty")}, nil
}
