package fonts_test

import (
	"math"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

func TestWinAnsiRoundTrip(t *testing.T) {
	// Each of these lives in the 0x80..0x9F block where WinAnsi diverges from
	// Latin-1, which is exactly where an encoding bug would hide.
	for _, r := range []rune{'€', '‚', 'ƒ', '…', '†', 'Š', 'Œ', '‘', '’', '“', '”', '•', '–', '—', '™', 'Ÿ'} {
		code, ok := fonts.WinAnsiCode(r)
		if !ok {
			t.Errorf("WinAnsi has no code for %q", r)
			continue
		}
		if got := fonts.RuneForWinAnsiCode(code); got != r {
			t.Errorf("code 0x%02X maps back to %q, want %q", code, got, r)
		}
	}
}

func TestEncodeWinAnsiSubstitutesUnsupportedRunes(t *testing.T) {
	got := fonts.EncodeWinAnsi("café 中")

	// The accented e is in the encoding; the CJK character is not and must
	// become a visible substitute rather than disappearing.
	want := []byte{'c', 'a', 'f', 0xE9, ' ', '?'}
	if string(got) != string(want) {
		t.Errorf("EncodeWinAnsi = % X, want % X", got, want)
	}
}

func TestHelveticaWidthsMatchAdobeMetrics(t *testing.T) {
	f := fonts.MustStandard(fonts.Helvetica)

	// Spot checks against Helvetica.afm at 1000pt, where the advance in points
	// equals the published value in 1/1000 em.
	for _, tc := range []struct {
		r    rune
		want float64
	}{
		{' ', 278}, {'A', 667}, {'M', 833}, {'W', 944}, {'i', 222},
		{'m', 833}, {'0', 556}, {'.', 278}, {'@', 1015},
	} {
		if got := f.AdvanceOf(tc.r, 1000); math.Abs(got-tc.want) > 0.01 {
			t.Errorf("advance of %q = %.2f, want %.2f", tc.r, got, tc.want)
		}
	}
}

func TestHelveticaBoldIsWiderThanRegular(t *testing.T) {
	regular := fonts.MustStandard(fonts.Helvetica)
	bold := fonts.MustStandard(fonts.HelveticaBold)

	const sample = "Handgloves"
	if bold.Measure(sample, 12) <= regular.Measure(sample, 12) {
		t.Error("bold Helvetica should measure wider than regular")
	}
}

func TestCourierIsMonospaced(t *testing.T) {
	f := fonts.MustStandard(fonts.Courier)

	want := f.AdvanceOf('M', 12)
	for _, r := range []rune{'i', 'W', '.', 'm', '0'} {
		if got := f.AdvanceOf(r, 12); math.Abs(got-want) > 0.01 {
			t.Errorf("advance of %q = %.4f, want %.4f (Courier is monospaced)", r, got, want)
		}
	}
}

func TestObliqueSharesUprightWidths(t *testing.T) {
	upright := fonts.MustStandard(fonts.Helvetica)
	oblique := fonts.MustStandard(fonts.HelveticaOblique)

	const sample = "The quick brown fox"
	// An oblique is a shear of the upright, so advances are identical. If these
	// diverged, italic text would measure wrongly and wrap in the wrong places.
	if math.Abs(upright.Measure(sample, 11)-oblique.Measure(sample, 11)) > 0.01 {
		t.Error("Helvetica-Oblique should share Helvetica's advance widths")
	}
}

func TestMeasureIsAdditive(t *testing.T) {
	f := fonts.MustStandard(fonts.Helvetica)

	whole := f.Measure("abcdef", 11)
	parts := f.Measure("abc", 11) + f.Measure("def", 11)

	// Line breaking sums per-word widths to decide where to break, so this has to
	// hold or the reported line width would drift from what is drawn.
	if math.Abs(whole-parts) > 0.001 {
		t.Errorf("Measure(\"abcdef\") = %.4f but the parts sum to %.4f", whole, parts)
	}
}

func TestStandardRejectsUnknownFont(t *testing.T) {
	if _, err := fonts.Standard("Times-Roman"); err == nil {
		t.Error("expected an error for a font with no built-in metrics")
	}
}

func TestStandardFamilyResolvesWeightAndSlant(t *testing.T) {
	for _, tc := range []struct {
		family string
		weight core.FontWeight
		italic bool
		want   string
	}{
		{"Helvetica", core.FontNormal, false, fonts.Helvetica},
		{"Helvetica", core.FontBold, false, fonts.HelveticaBold},
		{"sans-serif", core.FontNormal, true, fonts.HelveticaOblique},
		{"arial", core.FontBold, true, fonts.HelveticaBoldOblique},
		{"mono", core.FontNormal, false, fonts.Courier},
	} {
		got, err := fonts.StandardFamily(tc.family, tc.weight, tc.italic)
		if err != nil {
			t.Errorf("StandardFamily(%q, %d, %v): %v", tc.family, tc.weight, tc.italic, err)
			continue
		}
		if got.Name() != tc.want {
			t.Errorf("StandardFamily(%q, %d, %v) = %q, want %q",
				tc.family, tc.weight, tc.italic, got.Name(), tc.want)
		}
	}
}

func TestStandard14ProgramNeedsNoEmbeddedData(t *testing.T) {
	program, ok := fonts.ProgramOf(fonts.MustStandard(fonts.Helvetica))
	if !ok {
		t.Fatal("built-in font cannot describe itself to the PDF writer")
	}
	if !program.Standard14 {
		t.Error("built-in font is not marked standard-14")
	}
	if len(program.Data) != 0 {
		t.Error("built-in font should carry no embedded font program")
	}
}

// systemFont finds a TrueType font to exercise the embedding path.
func systemFont(t *testing.T) string {
	t.Helper()

	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Verdana.ttf",
		"/System/Library/Fonts/Supplemental/Andale Mono.ttf",
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

func TestRegisterTrueTypeReadsRealMetrics(t *testing.T) {
	path := systemFont(t)

	f, err := fonts.LoadTrueTypeFile("TestFace", path)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}

	if f.Name() != "TestFace" {
		t.Errorf("Name = %q, want %q", f.Name(), "TestFace")
	}
	if f.Ascent(12) <= 0 {
		t.Errorf("ascent at 12pt = %.4f, want positive", f.Ascent(12))
	}
	if f.Descent(12) <= 0 {
		t.Errorf("descent at 12pt = %.4f, want positive", f.Descent(12))
	}
	if f.LineHeight(12) <= f.Ascent(12) {
		t.Error("line height should exceed the ascent")
	}

	// Widths must be proportional to size, since layout scales one measurement
	// across every size the document uses.
	at10 := f.Measure("Handgloves", 10)
	at20 := f.Measure("Handgloves", 20)
	if math.Abs(at20-at10*2) > 0.01 {
		t.Errorf("measurement is not linear in size: %.4f at 10pt, %.4f at 20pt", at10, at20)
	}

	if f.AdvanceOf('W', 12) <= f.AdvanceOf('i', 12) {
		t.Error("expected a proportional font to advance W further than i")
	}
}

func TestTrueTypeProgramCarriesEmbeddableData(t *testing.T) {
	path := systemFont(t)

	f, err := fonts.LoadTrueTypeFile("Embedded", path)
	if err != nil {
		t.Fatal(err)
	}

	program, ok := fonts.ProgramOf(f)
	if !ok {
		t.Fatal("TrueType font cannot describe itself to the PDF writer")
	}
	if program.Standard14 {
		t.Error("a loaded font must not be marked standard-14")
	}
	if len(program.Data) == 0 {
		t.Error("no font program to embed")
	}
	if program.Ascent <= 0 || program.Descent >= 0 {
		t.Errorf("descriptor ascent/descent = %d/%d, want positive/negative",
			program.Ascent, program.Descent)
	}
	// Widths are indexed by WinAnsi code, so 'A' must land at 0x41.
	if program.Widths['A'] <= 0 {
		t.Error("no width recorded for 'A'")
	}
}

func TestRegisterTrueTypeRejectsBadInput(t *testing.T) {
	if _, err := fonts.RegisterTrueType("", []byte("x")); err == nil {
		t.Error("expected an error for an empty font name")
	}
	if _, err := fonts.RegisterTrueType("Helvetica", []byte("x")); err == nil {
		t.Error("expected an error for a name colliding with a built-in font")
	}
	if _, err := fonts.RegisterTrueType("Garbage", []byte("not a font")); err == nil {
		t.Error("expected an error for data that is not a font")
	}
}

func TestMustStandardPanicsOnUnknownFont(t *testing.T) {
	// The constants in this package are the intended argument, so a miss is a
	// programming error rather than a runtime condition.
	defer func() {
		if recover() == nil {
			t.Error("MustStandard did not panic on an unknown font")
		}
	}()
	fonts.MustStandard("No-Such-Font")
}

func TestStandardFamilyRejectsUnknownFamily(t *testing.T) {
	if _, err := fonts.StandardFamily("Comic Sans", core.FontNormal, false); err == nil {
		t.Error("expected an error for a family with no built-in metrics")
	}
}

func TestStandardNamesListsEveryBuiltInFace(t *testing.T) {
	names := fonts.StandardNames()

	if len(names) != 8 {
		t.Errorf("got %d built-in names, want 8", len(names))
	}
	// Every advertised name must actually resolve, or the error message that
	// lists them would be lying.
	for _, name := range names {
		if _, err := fonts.Standard(name); err != nil {
			t.Errorf("advertised font %q does not resolve: %v", name, err)
		}
	}
}

// foreignFont satisfies core.Font but not fonts.Programmable.
type foreignFont struct{}

func (foreignFont) Name() string                    { return "Foreign" }
func (foreignFont) AdvanceOf(rune, float64) float64 { return 1 }
func (foreignFont) Measure(string, float64) float64 { return 1 }
func (foreignFont) Ascent(float64) float64          { return 1 }
func (foreignFont) Descent(float64) float64         { return 1 }
func (foreignFont) LineHeight(float64) float64      { return 1 }

func TestProgramOfRejectsAForeignFont(t *testing.T) {
	// A font implementing only core.Font cannot be embedded, and the writer has
	// to detect that rather than emitting a broken resource.
	if _, ok := fonts.ProgramOf(foreignFont{}); ok {
		t.Error("ProgramOf accepted a font that cannot describe itself")
	}
}

func TestLoadTrueTypeFileReportsAMissingFile(t *testing.T) {
	if _, err := fonts.LoadTrueTypeFile("Absent", "/no/such/font.ttf"); err == nil {
		t.Error("expected an error for a missing font file")
	}
}

func TestLoadTrueTypeFileDerivesNameFromPath(t *testing.T) {
	path := systemFont(t)

	f, err := fonts.LoadTrueTypeFile("", path)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}

	// An empty name falls back to the file's base name without its extension.
	base := filepath.Base(path)
	want := strings.TrimSuffix(strings.TrimSuffix(base, ".ttf"), ".otf")
	if f.Name() != want {
		t.Errorf("derived name = %q, want %q", f.Name(), want)
	}
}

func TestUnsupportedRunesFallBackToTheMissingWidth(t *testing.T) {
	f := fonts.MustStandard(fonts.Helvetica)

	// A rune outside WinAnsi has no glyph, but must still occupy believable space
	// so that a line containing it measures sensibly instead of collapsing.
	if got := f.AdvanceOf('中', 12); got <= 0 {
		t.Errorf("advance of an unsupported rune = %v, want positive", got)
	}
}

func TestTrueTypeCachesBothHitsAndMisses(t *testing.T) {
	f, err := fonts.LoadTrueTypeFile("Fallback", systemFont(t))
	if err != nil {
		t.Fatal(err)
	}

	// Two passes exercise the advance cache on both the miss and the hit path.
	// A private-use code point is chosen because no real font maps it.
	for i := 0; i < 2; i++ {
		if got := f.AdvanceOf('\ue000', 12); got <= 0 {
			t.Errorf("advance of an unmapped rune = %v, want the fallback width", got)
		}
		if got := f.AdvanceOf('A', 12); got <= 0 {
			t.Errorf("advance of 'A' = %v, want positive", got)
		}
	}
}

// --- the name registry ------------------------------------------------------

func TestNewRegistryHoldsTheBuiltInFaces(t *testing.T) {
	r := fonts.NewRegistry()

	// The standard-14 are always present, so configuration naming one works with no
	// setup at all. This also guards the initialisation order: the map they come
	// from is built by a function precisely so a registry created in a variable
	// initialiser does not find it empty.
	for _, name := range fonts.StandardNames() {
		if _, ok := r.Lookup(name); !ok {
			t.Errorf("built-in face %q is missing from a fresh registry", name)
		}
	}
	if got := len(r.Names()); got != 8 {
		t.Errorf("a fresh registry holds %d fonts, want 8", got)
	}
}

func TestDefaultRegistryIsSeeded(t *testing.T) {
	// The shared registry is a package-level variable, so this is where an
	// initialisation-order regression would show up first.
	if _, ok := fonts.Lookup(fonts.Helvetica); !ok {
		t.Error("the shared registry has no Helvetica")
	}
	if len(fonts.RegisteredNames()) < 8 {
		t.Errorf("the shared registry holds %d fonts, want at least the built-in 8",
			len(fonts.RegisteredNames()))
	}
}

func TestRegistryRegisterAndResolve(t *testing.T) {
	r := fonts.NewRegistry()
	face := fonts.MustStandard(fonts.CourierBold)

	if err := r.Register("Brand", face); err != nil {
		t.Fatal(err)
	}

	resolved, err := r.Resolve("Brand")
	if err != nil {
		t.Fatalf("resolving a registered font: %v", err)
	}
	if resolved.Name() != fonts.CourierBold {
		t.Errorf("resolved %q, want %q", resolved.Name(), fonts.CourierBold)
	}
}

func TestRegistryReplacesOnCollision(t *testing.T) {
	// Replacing is deliberate: overriding a built-in with a real licensed face is a
	// reasonable thing to want, unlike the accidental collisions that destination
	// names guard against.
	r := fonts.NewRegistry()
	if err := r.Register(fonts.Helvetica, fonts.MustStandard(fonts.Courier)); err != nil {
		t.Fatal(err)
	}

	got, _ := r.Lookup(fonts.Helvetica)
	if got.Name() != fonts.Courier {
		t.Errorf("registering over a name gave %q, want the replacement", got.Name())
	}
}

func TestRegistryRejectsBadRegistrations(t *testing.T) {
	r := fonts.NewRegistry()

	if err := r.Register("", fonts.MustStandard(fonts.Helvetica)); err == nil {
		t.Error("an empty name was accepted")
	}
	if err := r.Register("Nil", nil); err == nil {
		t.Error("a nil font was accepted")
	}
}

func TestRegistryResolveListsAlternatives(t *testing.T) {
	r := fonts.NewRegistry()

	_, err := r.Resolve("Comic Sans")
	if err == nil {
		t.Fatal("expected an error for an unregistered name")
	}
	// A bad font name is nearly always a typo or a missing registration, and both
	// are quicker to fix when the message says what was available.
	for _, want := range []string{"Comic Sans", fonts.Helvetica} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}
}

func TestRegistryNamesAreSorted(t *testing.T) {
	r := fonts.NewRegistry()
	if err := r.Register("Aardvark", fonts.MustStandard(fonts.Helvetica)); err != nil {
		t.Fatal(err)
	}

	names := r.Names()
	for i := 1; i < len(names); i++ {
		if names[i-1] > names[i] {
			t.Fatalf("names are not sorted: %v", names)
		}
	}
	if names[0] != "Aardvark" {
		t.Errorf("first name = %q, want the alphabetically first", names[0])
	}
}

func TestRegistryLoadsTrueTypeFromDisk(t *testing.T) {
	path := systemFont(t)
	r := fonts.NewRegistry()

	face, err := r.LoadTrueType("Loaded", path)
	if err != nil {
		t.Fatalf("loading %s: %v", path, err)
	}

	// Loading registers it, which is the whole point of the method.
	resolved, err := r.Resolve("Loaded")
	if err != nil {
		t.Fatalf("a loaded font should be registered: %v", err)
	}
	if resolved != face {
		t.Error("the resolved font is not the one that was loaded")
	}
}

func TestRegistryRegistersTrueTypeFromBytes(t *testing.T) {
	data, err := os.ReadFile(systemFont(t))
	if err != nil {
		t.Fatal(err)
	}

	r := fonts.NewRegistry()
	if _, err := r.RegisterTrueTypeBytes("Embedded", data); err != nil {
		t.Fatal(err)
	}
	if _, err := r.Resolve("Embedded"); err != nil {
		t.Errorf("a registered font should resolve: %v", err)
	}
}

func TestRegistryReportsBadTrueTypeInput(t *testing.T) {
	r := fonts.NewRegistry()

	if _, err := r.LoadTrueType("Absent", "/no/such/font.ttf"); err == nil {
		t.Error("expected an error for a missing file")
	}
	if _, err := r.RegisterTrueTypeBytes("Garbage", []byte("not a font")); err == nil {
		t.Error("expected an error for data that is not a font")
	}
}

func TestSharedRegistryFunctions(t *testing.T) {
	// Registering into the shared registry is process-wide, so this uses a name no
	// other test looks up.
	face := fonts.MustStandard(fonts.CourierOblique)
	if err := fonts.Register("test-shared-face", face); err != nil {
		t.Fatal(err)
	}

	if got, ok := fonts.Lookup("test-shared-face"); !ok || got != face {
		t.Error("the shared registry did not return the registered face")
	}
	if _, err := fonts.Resolve("test-shared-face"); err != nil {
		t.Errorf("resolving from the shared registry: %v", err)
	}
	if _, err := fonts.Resolve("test-absent-face"); err == nil {
		t.Error("expected an error from the shared registry")
	}
}
