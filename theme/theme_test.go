package theme_test

import (
	"math"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/chart"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/theme"
)

func closeTo(t *testing.T, label string, got, want float64) {
	t.Helper()
	if math.Abs(got-want) > 0.01 {
		t.Errorf("%s = %.4f, want %.4f", label, got, want)
	}
}

// parse loads a theme and fails the test on error.
func parse(t *testing.T, source string, opts ...theme.Option) *theme.Theme {
	t.Helper()

	th, err := theme.Parse([]byte(source), opts...)
	if err != nil {
		t.Fatalf("parsing theme: %v", err)
	}
	return th
}

// mustFail loads a theme expecting failure, returning the message.
func mustFail(t *testing.T, source string, opts ...theme.Option) string {
	t.Helper()

	_, err := theme.Parse([]byte(source), opts...)
	if err == nil {
		t.Fatal("expected the theme to be rejected")
	}
	return err.Error()
}

// --- page geometry ----------------------------------------------------------

func TestPageSizeByName(t *testing.T) {
	for _, tc := range []struct {
		name string
		want core.Size
	}{
		{"A4", core.A4},
		{"a4", core.A4},
		{"Letter", core.Letter},
		{"A4 landscape", core.Landscape(core.A4)},
		{"a5 portrait", core.A5},
	} {
		th := parse(t, `{"page": {"size": "`+tc.name+`"}}`)
		if got := th.PageSize(); got != tc.want {
			t.Errorf("size %q = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestExplicitDimensionsOverrideTheName(t *testing.T) {
	th := parse(t, `{"page": {"size": "A4", "width": 300, "height": 500}}`)

	closeTo(t, "width", th.PageSize().Width, 300)
	closeTo(t, "height", th.PageSize().Height, 500)
}

func TestMissingPageDefaultsToA4(t *testing.T) {
	// A theme that says nothing about the page still has to produce something
	// usable.
	if got := parse(t, `{}`).PageSize(); got != core.A4 {
		t.Errorf("size = %v, want A4", got)
	}
}

func TestUnknownPageSizeIsRejected(t *testing.T) {
	message := mustFail(t, `{"page": {"size": "Foolscap"}}`)

	for _, want := range []string{"Foolscap", "A4", "landscape"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %q", message, want)
		}
	}
}

// --- margins ----------------------------------------------------------------

func TestMarginShorthandForms(t *testing.T) {
	// Margins are the field most often edited by hand, so the long form cannot be
	// the only option.
	for _, tc := range []struct {
		name                     string
		json                     string
		top, right, bottom, left float64
	}{
		{"scalar", `40`, 40, 40, 40, 40},
		{"single element", `[40]`, 40, 40, 40, 40},
		{"vertical horizontal", `[24, 40]`, 24, 40, 24, 40},
		{"css order", `[10, 20, 30, 40]`, 10, 20, 30, 40},
		{"object", `{"top": 10, "left": 20}`, 10, 0, 0, 20},
		{"absent", `null`, 0, 0, 0, 0},
	} {
		th := parse(t, `{"page": {"margin": `+tc.json+`}}`)
		top, right, bottom, left := th.Margins()

		closeTo(t, tc.name+" top", top, tc.top)
		closeTo(t, tc.name+" right", right, tc.right)
		closeTo(t, tc.name+" bottom", bottom, tc.bottom)
		closeTo(t, tc.name+" left", left, tc.left)
	}
}

func TestMarginRejectsBadShapes(t *testing.T) {
	for _, bad := range []string{`[1, 2, 3]`, `[1, 2, 3, 4, 5]`, `"forty"`, `true`} {
		if _, err := theme.Parse([]byte(`{"page": {"margin": ` + bad + `}}`)); err == nil {
			t.Errorf("margin %s was accepted", bad)
		}
	}
}

// --- colours ----------------------------------------------------------------

func TestColorsAreReadAsHex(t *testing.T) {
	th := parse(t, `{"colors": {"ink": "#1A1D29", "faded": "#ffffff80", "short": "#abc"}}`)

	if got := th.Color("ink"); got != core.RGB(0x1A, 0x1D, 0x29) {
		t.Errorf("ink = %v", got)
	}
	if got := th.Color("faded"); got.Opacity() != 0x80 {
		t.Errorf("faded alpha = %d, want 0x80", got.Opacity())
	}
	// The shorthand form repeats each digit.
	if got := th.Color("short"); got != core.RGB(0xAA, 0xBB, 0xCC) {
		t.Errorf("short = %v", got)
	}
}

func TestMalformedColorIsRejected(t *testing.T) {
	if _, err := theme.Parse([]byte(`{"colors": {"bad": "not a colour"}}`)); err == nil {
		t.Error("a malformed colour was accepted")
	}
}

func TestColorsCanBeSpecifiedForPrint(t *testing.T) {
	// The point of naming colours in a file is that a print build and a screen build
	// can differ by one line, so the file has to be able to say CMYK.
	th := parse(t, `{
	  "page":   {"background": "cmyk(0, 0, 0, 0)"},
	  "colors": {"registration": "cmyk(0, 0, 0, 100)", "faded": "cmyk(0, 0, 0, 100, 50)"},
	  "text":   {"body": {"font": "Helvetica", "size": 10, "color": "cmyk(100, 0, 0, 0)"}}
	}`)

	if got := th.Color("registration"); got != core.CMYKPercent(0, 0, 0, 100) {
		t.Errorf("registration = %v (%v space)", got, got.Space())
	}
	if got := th.Color("faded"); got.Space() != core.SpaceCMYK || got.Opacity() != 128 {
		t.Errorf("faded = %v at alpha %d", got, got.Opacity())
	}
	// A literal in either notation is accepted wherever a name is.
	if got := th.Style("body").Color; got != core.CMYKPercent(100, 0, 0, 0) {
		t.Errorf("body colour = %v", got)
	}
	if got := th.Background(); got.Space() != core.SpaceCMYK {
		t.Errorf("background = %v (%v space), want CMYK", got, got.Space())
	}
}

func TestUnknownColorNameIsNotMistakenForALiteral(t *testing.T) {
	// A typo has to be reported against the declared colours rather than handed to
	// the parser and coming back as a syntax complaint about the name.
	message := mustFail(t, `{"colors": {"ink": "#000"}, "text": {"body": {"font": "Helvetica", "size": 10, "color": "inkk"}}}`)

	for _, want := range []string{"inkk", "ink", "cmyk("} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %q", message, want)
		}
	}
}

func TestMalformedCMYKLiteralIsRejected(t *testing.T) {
	if _, err := theme.Parse([]byte(`{"colors": {"bad": "cmyk(0, 0, 0)"}}`)); err == nil {
		t.Error("a three-plate colour was accepted")
	}
}

func TestColorNamesAreSorted(t *testing.T) {
	th := parse(t, `{"colors": {"zebra": "#000", "apple": "#fff", "mango": "#f00"}}`)

	got := strings.Join(th.ColorNames(), ",")
	if got != "apple,mango,zebra" {
		t.Errorf("names = %q, want them sorted", got)
	}
}

func TestUnknownColorPanicsWithTheAlternatives(t *testing.T) {
	th := parse(t, `{"colors": {"ink": "#000"}}`)

	defer func() {
		message, ok := recover().(string)
		if !ok {
			t.Fatal("expected a panic for an unknown colour")
		}
		// Returning a zero colour instead would render as invisible text somewhere
		// in the middle of a document, which is far harder to trace.
		for _, want := range []string{"typo", "ink"} {
			if !strings.Contains(message, want) {
				t.Errorf("panic %q does not mention %q", message, want)
			}
		}
	}()

	th.Color("typo")
}

// --- text styles ------------------------------------------------------------

const styledTheme = `{
  "colors": {"ink": "#1A1D29", "muted": "#6B7280"},
  "fonts":  {"body": "Helvetica", "bold": "Helvetica-Bold", "mono": "Courier"},
  "text": {
    "body":    {"font": "body", "size": 9.5, "color": "ink"},
    "heading": {"font": "bold", "size": 19, "color": "ink"},
    "caption": {"font": "body", "size": 8, "color": "muted",
                "lineHeight": 1.3, "letterSpacing": 0.4,
                "underline": true, "strikeout": true},
    "code":    {"font": "mono", "size": 8.5, "color": "#0F766E"}
  }
}`

func TestTextStylesResolveNames(t *testing.T) {
	th := parse(t, styledTheme)

	body := th.Style("body")
	closeTo(t, "body size", body.Size, 9.5)
	if body.Color != core.RGB(0x1A, 0x1D, 0x29) {
		t.Errorf("body colour = %v", body.Color)
	}
	if body.Font == nil || body.Font.Name() != fonts.Helvetica {
		t.Errorf("body font = %v, want Helvetica", body.Font)
	}

	if got := th.Style("heading").Font.Name(); got != fonts.HelveticaBold {
		t.Errorf("heading font = %q, want %q", got, fonts.HelveticaBold)
	}
	if got := th.Style("code").Font.Name(); got != fonts.Courier {
		t.Errorf("code font = %q, want %q", got, fonts.Courier)
	}
}

func TestTextStyleCarriesTypography(t *testing.T) {
	caption := parse(t, styledTheme).Style("caption")

	closeTo(t, "line height", caption.LineHeightFactor, 1.3)
	closeTo(t, "letter spacing", caption.LetterSpacing, 0.4)
	if !caption.Underline || !caption.Strikeout {
		t.Error("decorations were not carried through")
	}
}

func TestColorFieldAcceptsAHexLiteral(t *testing.T) {
	// Allowing both means a shared value can live in Colors without having to, and
	// a one-off shade needs no invented name.
	code := parse(t, styledTheme).Style("code")

	if code.Color != core.Hex("#0F766E") {
		t.Errorf("code colour = %v, want the literal", code.Color)
	}
}

func TestFontFieldAcceptsARegisteredNameDirectly(t *testing.T) {
	// No alias declared: the name goes straight to the registry.
	th := parse(t, `{"text": {"body": {"font": "Courier-Bold", "size": 10}}}`)

	if got := th.Style("body").Font.Name(); got != fonts.CourierBold {
		t.Errorf("font = %q, want %q", got, fonts.CourierBold)
	}
}

func TestStyleWithoutASizeGetsADefault(t *testing.T) {
	// A zero size would render nothing at all.
	th := parse(t, `{"text": {"body": {"font": "Helvetica"}}}`)

	if got := th.Style("body").Size; got <= 0 {
		t.Errorf("size = %v, want a positive default", got)
	}
}

func TestStyleWithoutAFontIsRejected(t *testing.T) {
	message := mustFail(t, `{"text": {"body": {"size": 10}}}`)

	if !strings.Contains(message, "body") || !strings.Contains(message, "font") {
		t.Errorf("error %q should name the style and the missing font", message)
	}
}

func TestUnknownFontIsRejectedWithAlternatives(t *testing.T) {
	message := mustFail(t, `{"text": {"body": {"font": "Comic Sans", "size": 10}}}`)

	for _, want := range []string{"Comic Sans", "Helvetica"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q does not mention %q", message, want)
		}
	}
}

func TestUnknownColorInAStyleIsRejected(t *testing.T) {
	message := mustFail(t, `{
	  "colors": {"ink": "#000"},
	  "text": {"body": {"font": "Helvetica", "size": 10, "color": "inkk"}}
	}`)

	if !strings.Contains(message, "inkk") || !strings.Contains(message, "ink") {
		t.Errorf("error %q should name the typo and the alternatives", message)
	}
}

func TestEveryProblemIsReportedAtOnce(t *testing.T) {
	// A theme with three mistakes should take one edit to fix, not three rounds of
	// load-and-retry.
	message := mustFail(t, `{
	  "page": {"size": "Nonesuch"},
	  "text": {
	    "a": {"font": "Missing", "size": 10},
	    "b": {"font": "Helvetica", "size": 10, "color": "ghost"}
	  }
	}`)

	for _, want := range []string{"Nonesuch", "Missing", "ghost"} {
		if !strings.Contains(message, want) {
			t.Errorf("error %q omits %q; every problem should be reported", message, want)
		}
	}
}

func TestLookupStyleReportsAbsence(t *testing.T) {
	th := parse(t, styledTheme)

	if _, ok := th.LookupStyle("body"); !ok {
		t.Error("body should be found")
	}
	if _, ok := th.LookupStyle("nope"); ok {
		t.Error("an undeclared style should not be found")
	}
}

func TestStyleNamesAreSorted(t *testing.T) {
	got := strings.Join(parse(t, styledTheme).StyleNames(), ",")
	if got != "body,caption,code,heading" {
		t.Errorf("names = %q, want them sorted", got)
	}
}

// --- an isolated registry ---------------------------------------------------

func TestWithFontsUsesTheGivenRegistry(t *testing.T) {
	registry := fonts.NewRegistry()
	if err := registry.Register("Brand", fonts.MustStandard(fonts.CourierBold)); err != nil {
		t.Fatal(err)
	}

	th := parse(t, `{"text": {"body": {"font": "Brand", "size": 10}}}`,
		theme.WithFonts(registry))

	if got := th.Style("body").Font.Name(); got != fonts.CourierBold {
		t.Errorf("font = %q, want the registered face", got)
	}

	// The shared registry must not have been touched, or tests and per-tenant
	// rendering would leak into each other.
	if _, ok := fonts.Lookup("Brand"); ok {
		t.Error("an isolated registry should not write to the shared one")
	}
}

// --- background -------------------------------------------------------------

func TestBackgroundDefaultsToWhite(t *testing.T) {
	if got := parse(t, `{}`).Background(); got != core.RGB(255, 255, 255) {
		t.Errorf("background = %v, want white", got)
	}
}

func TestBackgroundResolvesNamesAndLiterals(t *testing.T) {
	named := parse(t, `{"colors": {"paper": "#FAFAF8"}, "page": {"background": "paper"}}`)
	if got := named.Background(); got != core.Hex("#FAFAF8") {
		t.Errorf("named background = %v", got)
	}

	literal := parse(t, `{"page": {"background": "#EEEEEE"}}`)
	if got := literal.Background(); got != core.Hex("#EEEEEE") {
		t.Errorf("literal background = %v", got)
	}
}

func TestUnknownBackgroundIsRejected(t *testing.T) {
	message := mustFail(t, `{"page": {"background": "ghost"}}`)
	if !strings.Contains(message, "ghost") {
		t.Errorf("error %q does not name the colour", message)
	}
}

// --- chart styling ----------------------------------------------------------

const chartTheme = `{
  "colors": {"accent": "#4F46E5", "hairline": "#E5E7EB"},
  "fonts":  {"body": "Helvetica"},
  "text":   {"tiny": {"font": "body", "size": 7, "color": "#6B7280"}},
  "chart": {
    "palette":   ["accent", "#0891B2", "#059669"],
    "grid":      "hairline",
    "gridWidth": 0.5,
    "gridDash":  [2, 2],
    "legend":    "right",
    "tickCount": 4,
    "labelStyle": "tiny",
    "hideValueLabels": true
  }
}`

func TestChartStyleResolves(t *testing.T) {
	style := parse(t, chartTheme).ChartStyle()

	if len(style.Palette) != 3 {
		t.Fatalf("palette has %d entries, want 3", len(style.Palette))
	}
	if style.Palette[0] != core.Hex("#4F46E5") {
		t.Errorf("first palette entry = %v, want the named colour", style.Palette[0])
	}
	if style.Grid != core.Hex("#E5E7EB") {
		t.Errorf("grid = %v", style.Grid)
	}
	closeTo(t, "grid width", style.GridWidth, 0.5)
	if style.Legend != chart.LegendRight {
		t.Errorf("legend = %v, want right", style.Legend)
	}
	if style.TickCount != 4 {
		t.Errorf("tick count = %d, want 4", style.TickCount)
	}
	if !style.HideValueLabels {
		t.Error("hideValueLabels was not carried through")
	}
	if style.Label.Size != 7 {
		t.Errorf("label style size = %v, want the referenced style", style.Label.Size)
	}
}

func TestChartStyleLeavesFormatUnset(t *testing.T) {
	// Format is a function, so no JSON file can supply one; charts fall back to
	// their own number formatting.
	if parse(t, chartTheme).ChartStyle().Format != nil {
		t.Error("Format should be left for the chart package to default")
	}
}

func TestChartStyleOmitsWhatTheThemeDidNotMention(t *testing.T) {
	// Unset fields must stay zero so chart.Style.resolve fills them, rather than
	// being pinned to a theme's idea of a default.
	style := parse(t, `{}`).ChartStyle()

	if len(style.Palette) != 0 {
		t.Error("an unmentioned palette should stay empty")
	}
	if style.TickCount != 0 {
		t.Error("an unmentioned tick count should stay zero")
	}
	if style.Label.Font != nil {
		t.Error("an unmentioned label style should stay zero")
	}
}

func TestChartLegendNames(t *testing.T) {
	for _, tc := range []struct {
		name string
		want chart.LegendPosition
	}{
		{"top", chart.LegendTop},
		{"bottom", chart.LegendBottom},
		{"right", chart.LegendRight},
		{"none", chart.LegendNone},
		{"RIGHT", chart.LegendRight},
	} {
		th := parse(t, `{"chart": {"legend": "`+tc.name+`"}}`)
		if got := th.ChartStyle().Legend; got != tc.want {
			t.Errorf("legend %q = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestUnknownChartReferencesAreRejected(t *testing.T) {
	for _, tc := range []struct {
		name   string
		source string
		expect string
	}{
		{"legend", `{"chart": {"legend": "sideways"}}`, "sideways"},
		{"palette", `{"chart": {"palette": ["ghost"]}}`, "ghost"},
		{"grid", `{"chart": {"grid": "ghost"}}`, "ghost"},
		{"label style", `{"chart": {"labelStyle": "ghost"}}`, "ghost"},
	} {
		message := mustFail(t, tc.source)
		if !strings.Contains(message, tc.expect) {
			t.Errorf("%s: error %q does not mention %q", tc.name, message, tc.expect)
		}
	}
}

// --- malformed input --------------------------------------------------------

func TestMalformedJSONIsRejected(t *testing.T) {
	if _, err := theme.Parse([]byte(`{not json`)); err == nil {
		t.Error("malformed JSON was accepted")
	}
}

func TestLoadReportsAMissingFile(t *testing.T) {
	if _, err := theme.Load("/no/such/theme.json"); err == nil {
		t.Error("expected an error for a missing file")
	}
}

func TestDecodeReadsAStream(t *testing.T) {
	th, err := theme.Decode(strings.NewReader(styledTheme))
	if err != nil {
		t.Fatal(err)
	}
	if got := th.Style("heading").Size; got != 19 {
		t.Errorf("heading size = %v, want 19", got)
	}
}
