package sanur_test

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
	"github.com/aaripurna/sanur-pdf/render"
)

// streamOf renders a one-page document and returns its content stream as text.
//
// Several of the methods below differ only in which edge they affect, and a
// measured size cannot tell those apart — a padding of 4 on the left and 4 on the
// right sum identically. Reading the emitted translation is what actually pins
// the argument order down.
func streamOf(t *testing.T, build func(*sanur.Page)) string {
	t.Helper()

	doc := sanur.New().Uncompressed()
	doc.Page(build)

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	for _, m := range regexp.MustCompile(`(?s)stream\n(.*?)\nendstream`).FindAllSubmatch(data, -1) {
		body := string(m[1])
		if strings.Contains(body, "cm") || strings.Contains(body, "re") {
			return body
		}
	}
	t.Fatalf("no content stream found in:\n%s", data)
	return ""
}

func wants(t *testing.T, got string, needles ...string) {
	t.Helper()
	for _, want := range needles {
		if !strings.Contains(got, want) {
			t.Errorf("content stream is missing %q; got:\n%s", want, got)
		}
	}
}

// --- padding edge order ----------------------------------------------------

func TestPaddingEachAppliesEdgesInOrder(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(0)
		// top 1, right 2, bottom 3, left 4
		p.Content().PaddingEach(1, 2, 3, 4).Size(10, 10).Background(sanur.Red).Empty()
	})

	// The child is offset by the left and top insets specifically.
	wants(t, stream, "1 0 0 1 4 1 cm")
}

func TestSingleEdgePaddingHelpers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*sanur.Container) *sanur.Container
		want  string
	}{
		{"top", func(c *sanur.Container) *sanur.Container { return c.PaddingTop(7) }, "1 0 0 1 0 7 cm"},
		{"left", func(c *sanur.Container) *sanur.Container { return c.PaddingLeft(7) }, "1 0 0 1 7 0 cm"},
		// Right and bottom padding shift nothing; they only enlarge the box, so
		// the child stays at the origin.
		{"right", func(c *sanur.Container) *sanur.Container { return c.PaddingRight(7) }, "re"},
		{"bottom", func(c *sanur.Container) *sanur.Container { return c.PaddingBottom(7) }, "re"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				tc.apply(p.Content()).Size(10, 10).Background(sanur.Red).Empty()
			})
			wants(t, stream, tc.want)
		})
	}
}

func TestPaddingXYSplitsHorizontalAndVertical(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().PaddingXY(5, 9).Size(10, 10).Background(sanur.Red).Empty()
	})

	// x becomes left, y becomes top.
	wants(t, stream, "1 0 0 1 5 9 cm")
}

func TestPaddingEnlargesTheReportedBox(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		// AlignLeft draws its child at exactly the measured size, which is the
		// only way to observe a tight box: the page itself hands content its full
		// inner width, so an unwrapped background would span the sheet.
		//
		// Padding is tight on both axes, so the background is the child plus the
		// insets: 10 + 4 + 2 wide, 10 + 1 + 3 tall.
		p.Content().AlignLeft().Background(sanur.Blue).
			PaddingEach(1, 2, 3, 4).Size(10, 10).Empty()
	})

	wants(t, stream, "0 0 16 14 re f")
}

// --- margins ---------------------------------------------------------------

func TestMarginEachOffsetsContentOrigin(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).MarginEach(11, 22, 33, 44)
		p.Content().Text("x")
	})

	// Content begins at the left and top margins.
	wants(t, stream, "1 0 0 1 44 11 cm")
}

func TestMarginXYOffsetsContentOrigin(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A4).MarginXY(15, 25)
		p.Content().Text("x")
	})

	wants(t, stream, "1 0 0 1 15 25 cm")
}

func TestPageBackgroundCoversTheWholeSheet(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Size(sanur.A5).Margin(20).Background(sanur.Grey200)
		p.Content().Text("x")
	})

	// The page background sits behind the margins as well as inside them.
	wants(t, stream, "0 0 419.53 595.28 re f")
}

// --- borders ---------------------------------------------------------------

func TestBorderStrokesAllFourEdges(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Size(100, 50).Border(2, sanur.Red).Empty()
	})

	if got := strings.Count(stream, " l S"); got != 4 {
		t.Errorf("got %d strokes, want 4; stream:\n%s", got, stream)
	}
}

func TestBorderEachAppliesWidthsInOrder(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		// Distinct widths make each edge identifiable in the output.
		p.Content().Size(100, 50).BorderEach(1, 2, 3, 4, sanur.Black).Empty()
	})

	// Each stroke is inset by half its width so it sits fully inside the box.
	wants(t, stream,
		"1 w\n0 0.5 m 100 0.5 l S",   // top, width 1
		"3 w\n0 48.5 m 100 48.5 l S", // bottom, width 3
		"4 w\n2 0 m 2 50 l S",        // left, width 4
		"2 w\n99 0 m 99 50 l S",      // right, width 2
	)
}

func TestSingleEdgeBorderHelpers(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*sanur.Container) *sanur.Container
		want  string
	}{
		{"top", func(c *sanur.Container) *sanur.Container { return c.BorderTop(2, sanur.Black) }, "0 1 m 100 1 l S"},
		{"bottom", func(c *sanur.Container) *sanur.Container { return c.BorderBottom(2, sanur.Black) }, "0 49 m 100 49 l S"},
		{"left", func(c *sanur.Container) *sanur.Container { return c.BorderLeft(2, sanur.Black) }, "1 0 m 1 50 l S"},
		{"right", func(c *sanur.Container) *sanur.Container { return c.BorderRight(2, sanur.Black) }, "99 0 m 99 50 l S"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				tc.apply(p.Content().Size(100, 50)).Empty()
			})
			wants(t, stream, tc.want)
			if got := strings.Count(stream, " l S"); got != 1 {
				t.Errorf("got %d strokes, want 1", got)
			}
		})
	}
}

// --- sizing ----------------------------------------------------------------

func TestSizeConstrainsBothDimensions(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Size(120, 60).Background(sanur.Red).Empty()
	})

	wants(t, stream, "0 0 120 60 re f")
}

func TestMinAndMaxBoundsAreHonoured(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*sanur.Container) *sanur.Container
		want  string
	}{
		// A minimum pads a smaller child out to the bound.
		{"MinWidth", func(c *sanur.Container) *sanur.Container { return c.MinWidth(200) }, "0 0 200 10 re f"},
		{"MinHeight", func(c *sanur.Container) *sanur.Container { return c.MinHeight(80) }, "0 0 10 80 re f"},
		// A maximum narrows the space offered to the child.
		{"MaxWidth", func(c *sanur.Container) *sanur.Container { return c.MaxWidth(30) }, "0 0 10 10 re f"},
		{"MaxHeight", func(c *sanur.Container) *sanur.Container { return c.MaxHeight(30) }, "0 0 10 10 re f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				// AlignLeft keeps the box tight so the bound itself is visible.
				tc.apply(p.Content().AlignLeft()).Background(sanur.Red).Size(10, 10).Empty()
			})
			wants(t, stream, tc.want)
		})
	}
}

func TestExtendVariantsClaimTheOfferedSpace(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*sanur.Container) *sanur.Container
		want  string
	}{
		{"Extend", func(c *sanur.Container) *sanur.Container { return c.Extend() }, "0 0 200 100 re f"},
		{"ExtendHorizontal", func(c *sanur.Container) *sanur.Container { return c.ExtendHorizontal() }, "0 0 200 10 re f"},
		{"ExtendVertical", func(c *sanur.Container) *sanur.Container { return c.ExtendVertical() }, "0 0 10 100 re f"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				// A fixed outer box gives Extend something definite to fill, and
				// AlignLeft then draws it at its measured size — a fixed-size
				// container hands its child that size outright, which would
				// otherwise mask what Extend itself claimed.
				inner := p.Content().Size(200, 100).AlignLeft()
				tc.apply(inner).Background(sanur.Red).Size(10, 10).Empty()
			})
			wants(t, stream, tc.want)
		})
	}
}

// --- alignment -------------------------------------------------------------

func TestAlignmentPositionsASmallerChild(t *testing.T) {
	for _, tc := range []struct {
		name  string
		apply func(*sanur.Container) *sanur.Container
		want  string
	}{
		{"AlignLeft", func(c *sanur.Container) *sanur.Container { return c.AlignLeft() }, "0 0 20 20 re f"},
		{"AlignCenter", func(c *sanur.Container) *sanur.Container { return c.AlignCenter() }, "1 0 0 1 90 0 cm"},
		{"AlignRight", func(c *sanur.Container) *sanur.Container { return c.AlignRight() }, "1 0 0 1 180 0 cm"},
		{"AlignTop", func(c *sanur.Container) *sanur.Container { return c.AlignTop() }, "0 0 20 20 re f"},
		{"AlignMiddle", func(c *sanur.Container) *sanur.Container { return c.AlignMiddle() }, "1 0 0 1 0 40 cm"},
		{"AlignBottom", func(c *sanur.Container) *sanur.Container { return c.AlignBottom() }, "1 0 0 1 0 80 cm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				outer := p.Content().Size(200, 100)
				tc.apply(outer).Size(20, 20).Background(sanur.Red).Empty()
			})
			wants(t, stream, tc.want)
		})
	}
}

// --- decoration ------------------------------------------------------------

func TestRoundedBackgroundEmitsCurves(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Size(80, 40).RoundedBackground(sanur.Blue, 6).Empty()
	})

	if got := strings.Count(stream, " c\n"); got != 4 {
		t.Errorf("got %d curves, want 4; stream:\n%s", got, stream)
	}
}

func TestClipEmitsAClipPathWithoutChangingLayout(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		// A deliberately oversized custom element is the clean way to show that
		// clipping affects drawing and not measurement. Chaining Size(200, 200)
		// instead would wrap, because a minimum larger than the box is a genuine
		// layout failure rather than an overflow.
		p.Content().Size(50, 50).Clip().Element(oversize{width: 200, height: 200})
	})

	wants(t, stream, "0 0 50 50 re W n", "0 0 200 200 re f")
}

// oversize reports a size larger than whatever it is offered, without wrapping,
// standing in for content that legitimately overflows its box.
type oversize struct {
	width, height float64
}

func (o oversize) Measure(core.Size) core.SpacePlan {
	return core.FullRender(core.Size{Width: o.width, Height: o.height})
}

func (o oversize) Draw(canvas core.Canvas, _ core.Size) {
	canvas.DrawRect(core.Position{}, core.Size{Width: o.width, Height: o.height}, sanur.Red)
}

func TestRotateEmitsARotation(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Rotate(90).Text("sideways")
	})

	wants(t, stream, "0 1 -1 0 0 0 cm")
}

func TestShowIfSuppressesHiddenContent(t *testing.T) {
	visible := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().ShowIf(true).Size(30, 30).Background(sanur.Red).Empty()
	})
	wants(t, visible, "0 0 30 30 re f")

	hidden := streamOf(t, func(p *sanur.Page) {
		// The sheet's own background is a fill too, so it is turned off to leave
		// the stream empty of anything but the element under test.
		p.Margin(0).Background(sanur.Transparent)
		p.Content().ShowIf(false).Size(30, 30).Background(sanur.Red).Empty()
	})
	if strings.Contains(hidden, "re f") {
		t.Errorf("hidden content was still drawn:\n%s", hidden)
	}
}

// --- leaves ----------------------------------------------------------------

func TestSpacerOccupiesSpaceWithoutDrawing(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Spacer(0, 40)
			c.Item().Size(10, 10).Background(sanur.Red).Empty()
		})
	})

	// The spacer draws nothing but pushes the following item down by its height.
	wants(t, stream, "1 0 0 1 0 40 cm")
}

func TestEmptyDrawsNothing(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0).Background(sanur.Transparent)
		p.Content().Empty()
	})

	if strings.Contains(stream, "re f") || strings.Contains(stream, "BT") {
		t.Errorf("Empty produced output:\n%s", stream)
	}
}

func TestLineVerticalSpansTheAvailableHeight(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Size(100, 60).LineVertical(2, sanur.Black)
	})

	// Centred on its own thickness so it stays inside the reported box.
	wants(t, stream, "1 0 m 1 60 l S")
}

func TestElementInstallsACustomPrimitive(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		// The escape hatch for anything implementing core.Element.
		p.Content().Element(&elements.Spacer{Width: 25, Height: 35})
	})

	// A spacer draws nothing, so a valid page with only the flip proves it was
	// measured and drawn without error.
	if !strings.Contains(stream, "cm") {
		t.Errorf("expected a valid page; got:\n%s", stream)
	}
}

func TestImageFitModesResolveDifferentBoxes(t *testing.T) {
	img, err := render.DecodeImage("fit", makePNG(t, 40, 20, false))
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range []struct {
		name string
		fit  elements.ImageFit
		want string
	}{
		// A 2:1 image in a 100x100 box.
		{"FitWidth", elements.FitWidth, "100 0 0 -50 0 50 cm"},
		{"FitArea", elements.FitArea, "100 0 0 -50 0 50 cm"},
		{"FitStretch", elements.FitStretch, "100 0 0 -100 0 100 cm"},
		{"FitUnscaled", elements.FitUnscaled, "40 0 0 -20 0 20 cm"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stream := streamOf(t, func(p *sanur.Page) {
				p.Margin(0)
				p.Content().Size(100, 100).ImageFit(img, tc.fit)
			})
			wants(t, stream, tc.want)
		})
	}
}

// --- text builders ---------------------------------------------------------

func TestRichTextAlignAndLine(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Width(400).RichText(func(tb *sanur.TextBuilder) {
			tb.Align(sanur.AlignRight)
			tb.Line("first line")
			tb.Span("second line")
		})
	})

	// Two lines means the explicit break took effect.
	if got := strings.Count(stream, " Tj"); got != 2 {
		t.Errorf("got %d text runs, want 2; stream:\n%s", got, stream)
	}
	wants(t, stream, "(first line) Tj", "(second line) Tj")
}

func TestRichTextStyledSpanUsesItsOwnFont(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().RichText(func(tb *sanur.TextBuilder) {
			tb.Span("plain ")
			tb.StyledSpan("mono", sanur.TextStyle().Mono().Size(9))
		})
	})

	// Two distinct faces mean two font resources.
	wants(t, stream, "/F0", "/F1")
}

func TestJustifiedTextEmitsWordSpacing(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Width(120).RichText(func(tb *sanur.TextBuilder) {
			tb.Align(sanur.AlignJustify)
			tb.Span("several words here that will certainly need to wrap onto more than one line")
		})
	})

	// Justification stretches the gaps via Tw rather than repositioning words.
	if !strings.Contains(stream, "Tw") {
		t.Errorf("justified text emitted no word spacing:\n%s", stream)
	}
}

// --- styles ----------------------------------------------------------------

func TestStyleBuilderResolvesFaces(t *testing.T) {
	for _, tc := range []struct {
		name  string
		style *sanur.StyleBuilder
		want  string
	}{
		{"default", sanur.TextStyle(), "Helvetica"},
		{"bold", sanur.TextStyle().Bold(), "Helvetica-Bold"},
		{"italic", sanur.TextStyle().Italic(), "Helvetica-Oblique"},
		{"bold italic", sanur.TextStyle().Bold().Italic(), "Helvetica-BoldOblique"},
		{"mono", sanur.TextStyle().Mono(), "Courier"},
		{"mono bold", sanur.TextStyle().Mono().Bold(), "Courier-Bold"},
		// Semibold and above resolve to the family's bold face, since the
		// built-in families carry only two weights.
		{"semibold weight", sanur.TextStyle().Weight(600), "Helvetica-Bold"},
		{"light weight", sanur.TextStyle().Weight(300), "Helvetica"},
	} {
		if got := tc.style.Build().Font.Name(); got != tc.want {
			t.Errorf("%s: font = %q, want %q", tc.name, got, tc.want)
		}
	}
}

func TestStyleBuilderCarriesTypography(t *testing.T) {
	style := sanur.TextStyle().
		Size(13).
		Color(sanur.Red).
		LineHeight(1.5).
		LetterSpacing(0.4).
		Underline().
		Strikeout().
		Build()

	if style.Size != 13 {
		t.Errorf("size = %v, want 13", style.Size)
	}
	if style.Color != sanur.Red {
		t.Errorf("colour = %v, want %v", style.Color, sanur.Red)
	}
	if style.LineHeightFactor != 1.5 {
		t.Errorf("line height factor = %v, want 1.5", style.LineHeightFactor)
	}
	if style.LetterSpacing != 0.4 {
		t.Errorf("letter spacing = %v, want 0.4", style.LetterSpacing)
	}
	if !style.Underline || !style.Strikeout {
		t.Error("decorations were not carried through")
	}
}

func TestStyleBuilderDefaultsSizeWhenUnset(t *testing.T) {
	// A zero size would make text invisible, so it falls back to the default.
	if got := sanur.TextStyle().Size(0).Build().Size; got != sanur.DefaultFontSize {
		t.Errorf("size = %v, want %v", got, sanur.DefaultFontSize)
	}
}

func TestStyleFromInheritsAndOverrides(t *testing.T) {
	base := sanur.TextStyle().Size(20).Bold().Build()

	derived := sanur.StyleFrom(base).Size(9).Build()

	if derived.Size != 9 {
		t.Errorf("size = %v, want 9", derived.Size)
	}
	// StyleFrom recovers bold from the base's weight, so the face survives.
	if derived.Font.Name() != "Helvetica-Bold" {
		t.Errorf("font = %q, want Helvetica-Bold", derived.Font.Name())
	}
}

func TestExplicitFontOverridesTheFamily(t *testing.T) {
	courier := sanur.CourierFamily.Regular

	style := sanur.TextStyle().Bold().Font(courier).Build()

	// Font replaces the family outright and clears the bold/italic selection,
	// since a single face has no siblings to resolve against.
	if style.Font.Name() != "Courier" {
		t.Errorf("font = %q, want Courier", style.Font.Name())
	}
}

func TestFamilyPickDegradesGracefully(t *testing.T) {
	regular := sanur.HelveticaFamily.Regular
	bold := sanur.HelveticaFamily.Bold

	partial := sanur.NewFamily(regular, bold, nil, nil)

	// An incomplete family falls back to the nearest available face rather than
	// erroring or producing a nil font.
	if got := partial.Pick(false, true).Name(); got != "Helvetica" {
		t.Errorf("italic fallback = %q, want Helvetica", got)
	}
	if got := partial.Pick(true, true).Name(); got != "Helvetica-Bold" {
		t.Errorf("bold-italic fallback = %q, want Helvetica-Bold", got)
	}

	empty := sanur.NewFamily(nil, bold, nil, nil)
	if got := empty.Pick(false, false).Name(); got != "Helvetica-Bold" {
		t.Errorf("regular fallback = %q, want Helvetica-Bold", got)
	}
}

func TestDefaultTextStyleAppliesToASubtree(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.DefaultTextStyle(sanur.TextStyle().Size(20))
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Text("inherits twenty")
			c.Item().DefaultTextStyle(sanur.TextStyle().Size(8)).Text("overridden to eight")
		})
	})

	wants(t, stream, "20 Tf", "8 Tf")
}

// --- units and sizes -------------------------------------------------------

func TestUnitConversions(t *testing.T) {
	for _, tc := range []struct {
		name string
		got  float64
		want float64
	}{
		{"one inch", sanur.In(1), 72},
		{"one centimetre", sanur.Cm(1), 28.3464566929},
		{"25.4 millimetres", sanur.Mm(25.4), 72},
	} {
		if diff := tc.got - tc.want; diff > 0.0001 || diff < -0.0001 {
			t.Errorf("%s = %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestLandscapeSwapsDimensions(t *testing.T) {
	got := sanur.Landscape(sanur.A4)

	if got.Width != sanur.A4.Height || got.Height != sanur.A4.Width {
		t.Errorf("Landscape(A4) = %v, want dimensions swapped", got)
	}
}

func TestColorHelpersMatchCore(t *testing.T) {
	if sanur.RGB(1, 2, 3) != sanur.RGBA(1, 2, 3, 255) {
		t.Error("RGB should be RGBA with full alpha")
	}
	if sanur.Hex("#FF0000") != sanur.RGB(255, 0, 0) {
		t.Error("Hex(#FF0000) should equal RGB(255, 0, 0)")
	}
}

// --- document metadata and output ------------------------------------------

func TestAllMetadataFieldsReachTheOutput(t *testing.T) {
	doc := sanur.New().Uncompressed().
		Title("T").
		Author("A").
		Subject("S").
		Keywords("K").
		Creator("C").
		CreationDate("D:20260730120000Z")

	doc.Page(func(p *sanur.Page) {
		p.Content().Text("x")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	wants(t, string(data),
		"/Title (T)", "/Author (A)", "/Subject (S)",
		"/Keywords (K)", "/Creator (C)", "/CreationDate (D:20260730120000Z)")
}

func TestWriteCreatesTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "out.pdf")

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Content().Text("written to disk")
	})

	if err := doc.Write(path); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(string(data), "%PDF-") {
		t.Error("the written file is not a PDF")
	}
}

func TestWriteReportsAnUnwritablePath(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Content().Text("x")
	})

	err := doc.Write(filepath.Join(t.TempDir(), "no-such-dir", "out.pdf"))
	if err == nil {
		t.Fatal("expected an error writing into a missing directory")
	}
}

func TestWritePropagatesLayoutErrors(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A5).Margin(400)
		p.Content().Text("x")
	})

	// A layout failure must surface from Write, not produce a half-written file.
	if err := doc.Write(filepath.Join(t.TempDir(), "never.pdf")); err == nil {
		t.Error("expected the layout error to propagate")
	}
}

func TestMultiplePageDefinitionsAreConcatenated(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(20)
		p.Content().Text("portrait sheet")
	})
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.Landscape(sanur.A4)).Margin(20)
		p.Footer().PageNumber("{page} of {total}")
		p.Content().Text("landscape sheet")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if got := countPages(data); got != 2 {
		t.Errorf("page count = %d, want 2", got)
	}
	// Page numbering runs across definitions, so the total covers both.
	wants(t, string(data), "595.28 841.89", "841.89 595.28")
}

// --- document-wide page defaults -------------------------------------------

func TestEveryPageAppliesFurnitureToAllDefinitions(t *testing.T) {
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(30)
		p.Header().Text("SHARED HEADER")
		p.Footer().Text("SHARED FOOTER")
	})

	doc.Page(func(p *sanur.Page) { p.Content().Text("first definition") })
	doc.Page(func(p *sanur.Page) { p.Content().Text("second definition") })
	doc.Page(func(p *sanur.Page) { p.Content().Text("third definition") })

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}

	if got := countPages(data); got != 3 {
		t.Fatalf("page count = %d, want 3", got)
	}
	// The furniture has to appear once per sheet, not once for the whole run.
	if got := strings.Count(string(data), "SHARED HEADER"); got != 3 {
		t.Errorf("header drawn %d times across 3 pages, want 3", got)
	}
	if got := strings.Count(string(data), "SHARED FOOTER"); got != 3 {
		t.Errorf("footer drawn %d times across 3 pages, want 3", got)
	}
}

func TestEveryPageSuppliesGeometryDefaults(t *testing.T) {
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A5).MarginEach(11, 22, 33, 44)
	})
	doc.Page(func(p *sanur.Page) { p.Content().Text("x") })

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	wants(t, string(data), "419.53 595.28")    // A5 media box
	wants(t, string(data), "1 0 0 1 44 11 cm") // left and top margins
}

func TestPageOverridesTemplateDefaults(t *testing.T) {
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(20)
		p.Header().Text("TEMPLATE HEADER")
	})

	doc.Page(func(p *sanur.Page) {
		p.Content().Text("inherits")
	})
	doc.Page(func(p *sanur.Page) {
		// The definition's own build runs second, so it wins.
		p.Size(sanur.Landscape(sanur.A4))
		p.Header().Text("OVERRIDDEN")
		p.Content().Text("overrides")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	if got := strings.Count(text, "TEMPLATE HEADER"); got != 1 {
		t.Errorf("template header appears %d times, want 1", got)
	}
	if got := strings.Count(text, "OVERRIDDEN"); got != 1 {
		t.Errorf("overriding header appears %d times, want 1", got)
	}
	// Both orientations should be present: portrait then landscape.
	wants(t, text, "595.28 841.89", "841.89 595.28")
}

func TestEveryPageGivesEachDefinitionFreshElements(t *testing.T) {
	// This is why the template is a function rather than a prepared element tree.
	// Elements carry pagination state, so a shared header instance would arrive at
	// the second definition believing it had already been drawn.
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Margin(30)
		// A multi-line header exercises the state that would be shared: a text
		// block tracks which line it has rendered.
		p.Header().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(2)
			c.Item().Text("HEADER LINE ONE")
			c.Item().Text("HEADER LINE TWO")
		})
	})

	for i := 0; i < 3; i++ {
		doc.Page(func(p *sanur.Page) { p.Content().Text("body") })
	}

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	for _, line := range []string{"HEADER LINE ONE", "HEADER LINE TWO"} {
		if got := strings.Count(string(data), line); got != 3 {
			t.Errorf("%q drawn %d times across 3 pages, want 3", line, got)
		}
	}
}

func TestEveryPageStillRepeatsAcrossSheetsOfOneDefinition(t *testing.T) {
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(30)
		p.Header().Text("PER SHEET")
	})

	// One definition long enough to spill over several sheets.
	doc.Page(func(p *sanur.Page) {
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			for i := 0; i < 120; i++ {
				c.Item().Height(20).Text("row")
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	pages := countPages(data)
	if pages < 3 {
		t.Fatalf("expected several sheets, got %d", pages)
	}
	if got := strings.Count(string(data), "PER SHEET"); got != pages {
		t.Errorf("header drawn %d times across %d sheets, want one per sheet", got, pages)
	}
}

func TestEveryPageAppliesOnlyToLaterDefinitions(t *testing.T) {
	doc := sanur.New().Uncompressed()

	// Declared before the template, so it gets none of it.
	doc.Page(func(p *sanur.Page) {
		p.Margin(30)
		p.Content().Text("early")
	})

	doc.EveryPage(func(p *sanur.Page) {
		p.Margin(30)
		p.Header().Text("LATE TEMPLATE")
	})

	doc.Page(func(p *sanur.Page) { p.Content().Text("late") })

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// A builder reads top to bottom, so the template affects what follows it.
	if got := strings.Count(string(data), "LATE TEMPLATE"); got != 1 {
		t.Errorf("template header appears %d times, want 1", got)
	}
}

func TestEveryPageCanBeReplaced(t *testing.T) {
	doc := sanur.New().Uncompressed()

	doc.EveryPage(func(p *sanur.Page) {
		p.Margin(30)
		p.Header().Text("FIRST TEMPLATE")
	})
	doc.Page(func(p *sanur.Page) { p.Content().Text("a") })

	doc.EveryPage(func(p *sanur.Page) {
		p.Margin(30)
		p.Header().Text("SECOND TEMPLATE")
	})
	doc.Page(func(p *sanur.Page) { p.Content().Text("b") })

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	if got := strings.Count(text, "FIRST TEMPLATE"); got != 1 {
		t.Errorf("first template appears %d times, want 1", got)
	}
	if got := strings.Count(text, "SECOND TEMPLATE"); got != 1 {
		t.Errorf("second template appears %d times, want 1", got)
	}
}

// --- path primitives -------------------------------------------------------

func TestDashedRulesReachTheContentStream(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().DashedLineHorizontal(1, sanur.Grey500, 4, 2)
			c.Item().Height(30).DashedLineVertical(0.5, sanur.Grey500, 2)
		})
	})

	wants(t, stream, "[4 2] 0 d", "[2] 0 d")
}

func TestPieSliceDrawsAsOneClosedPath(t *testing.T) {
	// A pie slice is the case a DrawArc canvas primitive could not express: the
	// arc has to join two radii inside a single closed path.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)

		centre := core.Position{X: 60, Y: 60}
		slice := core.NewPath().
			MoveTo(centre).
			Arc(centre, 50, -90, 120).
			Close()

		p.Content().Size(120, 120).Path(slice, core.PathStyle{
			Fill:   sanur.Indigo,
			Stroke: sanur.White,
			Width:  1,
		})
	})

	// Fill and stroke in one operator, closed, with the arc subdivided into
	// quarter turns.
	wants(t, stream, "60 60 m", "\nh\nB\n")
	if got := strings.Count(stream, " c\n"); got != 2 {
		t.Errorf("a 120 degree sweep should emit 2 curves, got %d:\n%s", got, stream)
	}
}

func TestDonutRingUsesTwoSubpaths(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)

		centre := core.Position{X: 50, Y: 50}
		ring := core.NewPath().
			Circle(centre, 40).
			Circle(centre, 20)

		p.Content().Size(100, 100).Path(ring, core.Filled(sanur.Teal))
	})

	// Two closed subpaths, eight quarter turns between them. The even-odd versus
	// nonzero winding question is the reader's to answer; what matters here is
	// that both circles reach the stream as one path.
	if got := strings.Count(stream, " c\n"); got != 8 {
		t.Errorf("two circles should emit 8 curves, got %d:\n%s", got, stream)
	}
	if got := strings.Count(stream, "\nh\n"); got != 2 {
		t.Errorf("expected 2 closed subpaths, got %d:\n%s", got, stream)
	}
}

func TestPolygonAreaFill(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)

		// The shape an area chart needs: a baseline, the series, and back down.
		area := core.Polygon(
			core.Position{X: 0, Y: 50},
			core.Position{X: 20, Y: 20},
			core.Position{X: 40, Y: 35},
			core.Position{X: 60, Y: 10},
			core.Position{X: 60, Y: 50},
		)

		p.Content().Size(60, 50).Path(area, core.Filled(sanur.RGBA(30, 100, 200, 90)))
	})

	wants(t, stream, "0 50 m", "20 20 l", "60 10 l", "\nh\nf\n")
	// A translucent fill needs a graphics state, since colour operators carry no
	// alpha.
	wants(t, stream, "gs")
}

func TestPathShapeTakesTheOfferedBox(t *testing.T) {
	// A path claims the space it is given rather than measuring its own extent, so
	// a shape whose points sit far from the origin cannot silently resize its
	// parent.
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0)
		p.Content().AlignLeft().Size(40, 25).Background(sanur.Grey200).
			Path(core.Polyline(
				core.Position{X: 0, Y: 0},
				core.Position{X: 500, Y: 500},
			), core.Stroked(sanur.Black, 1))
	})

	wants(t, stream, "0 0 40 25 re f")
}

func TestEmptyPathDrawsNothing(t *testing.T) {
	stream := streamOf(t, func(p *sanur.Page) {
		p.Margin(0).Background(sanur.Transparent)
		p.Content().Size(50, 50).Path(core.NewPath(), core.Filled(sanur.Red))
	})

	for _, op := range []string{"\nf\n", "\nS\n", "\nB\n"} {
		if strings.Contains(stream, op) {
			t.Errorf("an empty path painted %q:\n%s", op, stream)
		}
	}
}

// --- links and bookmarks ----------------------------------------------------

func TestExternalLinkEmitsAURIAnnotation(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Link("https://example.com/docs").Text("Documentation")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	wants(t, string(data),
		"/Type /Annot", "/Subtype /Link", "/S /URI",
		"/URI (https://example.com/docs)",
		"/Annots [",
		// A zero border stops readers drawing the black rectangle older tools are
		// notorious for.
		"/Border [0 0 0]")
}

func TestLinkRectangleIsTheAllocatedBoxInPDFSpace(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(core.Size{Width: 600, Height: 800}).Margin(0)
		// A fixed box at a known offset, so the rectangle can be checked exactly.
		p.Content().PaddingEach(100, 0, 0, 50).Size(200, 30).
			Link("https://example.com").Empty()
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// Layout puts the box at x 50..250, y 100..130 measured down from the top.
	// PDF measures up from the bottom of an 800-point page, so y becomes 670..700.
	wants(t, string(data), "/Rect [50 670 250 700]")
}

func TestInternalLinkResolvesToItsDestination(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)
			// The link is declared before the anchor it points at, which is the
			// common case for a table of contents.
			c.Item().LinkTo("methods").Text("Jump to Methods")
			c.Item().PageBreak()
			c.Item().Anchor("methods").Text("Methods")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	wants(t, string(data), "/S /GoTo", "/D [")
	if strings.Contains(string(data), "/S /URI") {
		t.Error("an internal link should not become a URI action")
	}
	if got := countPages(data); got != 2 {
		t.Fatalf("page count = %d, want 2", got)
	}
}

func TestDanglingInternalLinkIsReported(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().LinkTo("nowhere").Text("broken")
	})

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected an error for a link to an unregistered destination")
	}
	// A dead link is invisible in the output, so the name has to be in the error.
	for _, want := range []string{"nowhere", "destination"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestDuplicateDestinationIsReported(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Anchor("intro").Text("first")
			c.Item().Anchor("intro").Text("second")
		})
	})

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("expected an error for a reused destination name")
	}
	if !strings.Contains(err.Error(), "intro") {
		t.Errorf("error %q does not name the duplicate", err)
	}
}

func TestBookmarksBuildAnOutline(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			c.Item().Bookmark("Introduction").Text("Introduction")
			c.Item().BookmarkAt(1, "Background").Text("Background")
			c.Item().BookmarkAt(1, "Scope").Text("Scope")
			c.Item().Bookmark("Results").Text("Results")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	text := string(data)
	wants(t, text,
		"/Type /Outlines",
		"/Outlines ",
		// The reader is asked to open with the outline panel showing, which is the
		// point of having one.
		"/PageMode /UseOutlines",
		"/Title (Introduction)", "/Title (Background)",
		"/Title (Scope)", "/Title (Results)")

	// Two top-level entries, each pointing at the other.
	if got := strings.Count(text, "/Parent "); got < 4 {
		t.Errorf("got %d parent links, want one per entry", got)
	}
	// Background and Scope nest under Introduction, so it carries a child count.
	if !strings.Contains(text, "/First ") || !strings.Contains(text, "/Last ") {
		t.Error("expected the nested entries to be linked from their parent")
	}
}

func TestBookmarkDoublesAsADestination(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			c.Item().LinkTo("bookmark:Results").Text("see Results")
			c.Item().Bookmark("Results").Text("Results")
		})
	})

	// A bookmark registers a destination named after its title, so a link can aim
	// at the same spot without a separate anchor.
	if _, err := doc.Bytes(); err != nil {
		t.Fatalf("linking to a bookmark's derived name failed: %v", err)
	}
}

func TestBookmarkNamedAvoidsTitleCollisions(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(8)
			// Two sections legitimately share a title; explicit names keep their
			// destinations distinct.
			c.Item().BookmarkNamed(1, "Overview", "a-overview").Text("A overview")
			c.Item().BookmarkNamed(1, "Overview", "b-overview").Text("B overview")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("explicitly named bookmarks should not collide: %v", err)
	}
	if got := strings.Count(string(data), "/Title (Overview)"); got != 2 {
		t.Errorf("got %d entries titled Overview, want 2", got)
	}
}

func TestLinksAttachToTheCorrectPage(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Link("https://first.example").Text("first page link")
			c.Item().PageBreak()
			c.Item().Link("https://second.example").Text("second page link")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if got := countPages(data); got != 2 {
		t.Fatalf("page count = %d, want 2", got)
	}
	// Each page carries exactly one annotation array; a link landing on the wrong
	// page is invisible until somebody clicks the empty space where it should be.
	if got := strings.Count(string(data), "/Annots ["); got != 2 {
		t.Errorf("got %d annotation arrays, want one per page", got)
	}
	wants(t, string(data), "(https://first.example)", "(https://second.example)")
}

func TestPagesWithoutLinksCarryNoAnnots(t *testing.T) {
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Text("no links here")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "/Annots") {
		t.Error("a page with no links should carry no annotation array")
	}
	if strings.Contains(string(data), "/Outlines") {
		t.Error("a document with no bookmarks should carry no outline")
	}
}

func TestLinkedDocumentRendersCleanly(t *testing.T) {
	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(10)
			c.Item().Bookmark("Contents").Text("Contents")
			c.Item().LinkTo("bookmark:Detail").Text("Go to detail")
			c.Item().Link("https://example.com").Text("External")
			c.Item().PageBreak()
			c.Item().Bookmark("Detail").Text("Detail")
			c.Item().BookmarkAt(1, "Sub-detail").Text("Sub-detail")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}
	assertRendersCleanly(t, "links", data)
}
