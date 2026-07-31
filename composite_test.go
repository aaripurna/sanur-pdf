package sanur_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
)

// The scripts below are what the feature is for. WinAnsi is Windows code page 1252,
// so what a single-byte encoding shuts out is not only the exotic — it is Polish,
// Czech, Turkish, Romanian, Vietnamese, Greek and all of Cyrillic. Every line here
// used to come out as a row of question marks.
var scripts = []struct {
	name, text string
}{
	{"latin", "The quick brown fox"},
	{"polish", "Zażółć gęślą jaźń"},
	{"czech", "Příšerně žluťoučký kůň"},
	{"turkish", "Pijamalı hasta yağız şoföre"},
	{"vietnamese", "Tôi có thể ăn thủy tinh"},
	{"cyrillic", "Съешь же ещё этих мягких булок"},
	{"greek", "Ξεσκεπάζω την ψυχοφθόρα βδελυγμία"},
	{"arrows", "→ ← ↑ ↓ ½ € £ ¥ ™"},
}

// embeddedFont loads a system face broad enough to cover the scripts above.
//
// The metrics tests elsewhere take any TrueType file; this one needs Cyrillic and
// Greek, so the candidate list is narrower and skipping is the honest outcome when
// none is installed.
func embeddedFont(t *testing.T, name string) core.Font {
	t.Helper()

	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Arial Unicode.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
	} {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		face, err := fonts.LoadTrueTypeFile(name, path)
		if err != nil {
			t.Fatalf("loading %s: %v", path, err)
		}
		// A face with no Cyrillic would make every assertion below vacuous.
		if face.AdvanceOf('Ж', 12) > 0 && face.AdvanceOf('Ξ', 12) > 0 {
			return face
		}
	}

	t.Skip("no system font with Cyrillic and Greek coverage")
	return nil
}

// scriptDocument renders every script above with an embedded font.
func scriptDocument(t *testing.T, name string) []byte {
	t.Helper()

	face := embeddedFont(t, name)

	doc := sanur.New().Title("Scripts")
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(50)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(15))
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(12)
			for _, script := range scripts {
				c.Item().Text(script.text)
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	return data
}

func TestNonLatinTextIsExtractable(t *testing.T) {
	// Extraction is the only check that proves the whole chain end to end. It fails if
	// the glyph IDs are wrong, if the ToUnicode map is missing or malformed, or if the
	// text was encoded as single bytes — and it is what a reader, a search index and a
	// screen reader all depend on.
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	dir := t.TempDir()
	pdfPath := filepath.Join(dir, "scripts.pdf")
	if err := os.WriteFile(pdfPath, scriptDocument(t, "ScriptsExtract"), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(pdftotext, pdfPath, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext failed: %v", err)
	}

	// Whitespace is compared out. Extraction tools reconstruct spaces from glyph
	// positions rather than trusting the space glyph, and for a run of wide symbols
	// poppler decides there is no gap — which says nothing about whether the
	// characters came back right, and that is what this test is about.
	extracted := withoutSpaces(string(out))

	for _, script := range scripts {
		// Hebrew and Arabic are deliberately absent from the list: extraction tools
		// apply bidirectional reordering, so the text comes back in visual order and
		// would not match. Sanur does not reorder either, which is recorded as a
		// limitation rather than tested as a feature.
		if !strings.Contains(extracted, withoutSpaces(script.text)) {
			t.Errorf("%s text did not survive extraction; got:\n%s", script.name, out)
		}
	}
}

func withoutSpaces(s string) string {
	return strings.Map(func(r rune) rune {
		if r == ' ' || r == '\n' || r == '\t' || r == '\r' {
			return -1
		}
		return r
	}, s)
}

func TestNonLatinTextDrawsNoSubstituteGlyphs(t *testing.T) {
	// A rune the font cannot represent becomes a question mark, which is deliberate.
	// None of the scripts above contains one, so any question mark in a drawn string
	// means a glyph was not found.
	data := scriptDocument(t, "ScriptsGlyphs")

	for _, run := range textRuns(t, data) {
		if strings.Contains(run, "?") {
			t.Errorf("a drawn string contains a substituted glyph: %q", run)
		}
	}
}

func TestCompositeTextIsEncodedAsGlyphIDs(t *testing.T) {
	face := embeddedFont(t, "ScriptsEncoding")

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(12))
		p.Content().Text("Aa")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	// Composite text is two bytes per glyph, written as a hex string. A literal
	// string would spend an octal escape on nearly every byte, and there would be no
	// way to tell a glyph number from an accidental delimiter.
	operand := regexp.MustCompile(`<([0-9A-F]+)> Tj`).FindSubmatch(data)
	if operand == nil {
		t.Fatalf("no hex text operand found; composite text was not encoded as glyph IDs:\n%s", data)
	}
	if len(operand[1]) != 8 {
		t.Errorf("operand %q is %d hex digits, want 8 (two glyphs at two bytes each)",
			operand[1], len(operand[1]))
	}

	source, ok := fonts.GlyphSourceOf(face)
	if !ok {
		t.Fatal("an embedded font must expose its glyphs")
	}
	for i, r := range []rune{'A', 'a'} {
		gid, ok := source.GlyphID(r)
		if !ok {
			t.Fatalf("the font has no glyph for %q", r)
		}
		if got := string(operand[1][i*4 : i*4+4]); got != hex4(gid) {
			t.Errorf("%q encoded as %q, want %q (glyph %d)", r, got, hex4(gid), gid)
		}
	}
}

func hex4(v uint16) string {
	const digits = "0123456789ABCDEF"
	return string([]byte{
		digits[v>>12&0xF], digits[v>>8&0xF], digits[v>>4&0xF], digits[v&0xF],
	})
}

func TestCompositeFontCarriesWidthsForEveryGlyphDrawn(t *testing.T) {
	// A glyph missing from /W falls back to /DW, so text drawn with it is positioned
	// by a guess. The result is a line that looks slightly wrong and measures
	// correctly in sanur's own arithmetic, which makes it hard to trace.
	face := embeddedFont(t, "ScriptsWidths")

	const text = "Съешь Ξεσκ Wij"

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(12))
		p.Content().Text(text)
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	widths := widthArrayOf(t, data)

	// Every run is "<first glyph> [w w w ...]", so the glyphs a group covers are the
	// leading number and the ones following it.
	covered := map[uint16]bool{}
	for _, group := range regexp.MustCompile(`(\d+) \[([\d ]+)\]`).FindAllStringSubmatch(widths, -1) {
		first := parseUint16(t, group[1])
		for i := range strings.Fields(group[2]) {
			covered[first+uint16(i)] = true
		}
	}

	source, _ := fonts.GlyphSourceOf(face)
	for _, r := range text {
		gid, ok := source.GlyphID(r)
		if !ok {
			continue
		}
		if !covered[gid] {
			t.Errorf("glyph %d (%q) has no width in %s", gid, r, widths)
		}
	}
}

// widthArrayOf returns the /W array as text.
//
// A regular expression cannot do this: the array holds nested arrays, and any
// non-greedy match stops at the first inner bracket. That is not a hypothetical —
// the first version of this test matched "[3 [277]" and reported every glyph as
// missing. Counting brackets is the only way to read a nested structure.
func widthArrayOf(t *testing.T, data []byte) string {
	t.Helper()

	start := bytes.Index(data, []byte("/W ["))
	if start < 0 {
		t.Fatalf("no width array found:\n%s", data)
	}
	start += len("/W ")

	depth := 0
	for i := start; i < len(data); i++ {
		switch data[i] {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				return string(data[start : i+1])
			}
		}
	}

	t.Fatalf("the width array starting at %d is never closed", start)
	return ""
}

func parseUint16(t *testing.T, s string) uint16 {
	t.Helper()

	var v uint32
	for _, c := range s {
		if c < '0' || c > '9' {
			t.Fatalf("%q is not a number", s)
		}
		v = v*10 + uint32(c-'0')
	}
	if v > 0xFFFF {
		t.Fatalf("%q does not fit in a glyph ID", s)
	}
	return uint16(v)
}

func TestEmbeddedFontIsSubsetted(t *testing.T) {
	// Without subsetting, a document setting one line of text ships every outline the
	// typeface has — hundreds of kilobytes for a Latin font and megabytes for one
	// covering CJK.
	face := embeddedFont(t, "ScriptsSubset")

	source, ok := fonts.GlyphSourceOf(face)
	if !ok {
		t.Fatal("an embedded font must expose its glyphs")
	}

	whole, err := source.Subset(everyGlyph(t, face))
	if err != nil {
		t.Fatal(err)
	}

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(12))
		p.Content().Text("One short line")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if len(data) >= len(whole.Data)/4 {
		t.Errorf("a one-line document is %d bytes against a full subset of %d; "+
			"the font does not appear to be subsetted", len(data), len(whole.Data))
	}

	// The subset tag is not decoration: it is what stops two documents carrying
	// different subsets of one typeface from being merged into a font that draws
	// blanks for half of them.
	if !regexp.MustCompile(`/BaseFont /[A-Z]{6}\+`).Match(data) {
		t.Error("the embedded font name carries no subset tag")
	}
}

// everyGlyph returns a glyph set covering a wide sweep of the font, to stand in for
// "the whole thing" without assuming how many glyphs it has.
func everyGlyph(t *testing.T, face core.Font) map[uint16]bool {
	t.Helper()

	source, ok := fonts.GlyphSourceOf(face)
	if !ok {
		t.Fatal("an embedded font must expose its glyphs")
	}

	set := map[uint16]bool{}
	for r := rune(0x20); r < 0x2500; r++ {
		if gid, ok := source.GlyphID(r); ok {
			set[gid] = true
		}
	}
	return set
}

func TestCompositeOutputIsDeterministic(t *testing.T) {
	// Glyph sets, width runs and the subset tag all come from maps. Sorting them is
	// what keeps the guarantee that the same document produces the same bytes, and
	// nothing but a byte comparison can confirm it.
	first := scriptDocument(t, "ScriptsDeterminism")
	second := scriptDocument(t, "ScriptsDeterminism")

	if !bytes.Equal(first, second) {
		t.Error("two runs of the same document produced different bytes")
	}
}

func TestStandard14FontsStayASimpleFont(t *testing.T) {
	// The built-in faces have no program to embed and no glyphs to index, so they must
	// keep going out as a bare Type1 dictionary the reader resolves by name. Turning
	// them into composite fonts would mean embedding something, which is the one thing
	// standard-14 exists to avoid.
	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.Content().Text("Helvetica")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{"/Subtype /Type1", "/BaseFont /Helvetica", "/Encoding /WinAnsiEncoding"} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}
	for _, unwanted := range []string{"/Type0", "/Identity-H", "/FontFile", "/ToUnicode"} {
		if bytes.Contains(data, []byte(unwanted)) {
			t.Errorf("a standard-14 document contains %q", unwanted)
		}
	}
	// A standard-14 font is still addressed by byte, so the operand stays a literal.
	if !bytes.Contains(data, []byte("(Helvetica) Tj")) {
		t.Errorf("standard-14 text was not written as a literal string:\n%s", data)
	}
}

func TestRunesOutsideTheFontBecomeVisibleSubstitutes(t *testing.T) {
	// Silently dropping a character hides missing content. A question mark is wrong
	// in a way somebody notices.
	face := embeddedFont(t, "ScriptsSubstitute")
	if face.AdvanceOf('漢', 12) != face.AdvanceOf('?', 12) {
		t.Skip("the test font covers CJK, so nothing here would be substituted")
	}

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(12))
		p.Content().Text("漢字")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	source, _ := fonts.GlyphSourceOf(face)
	question, ok := source.GlyphID('?')
	if !ok {
		t.Skip("the test font has no question mark")
	}

	operand := regexp.MustCompile(`<([0-9A-F]+)> Tj`).FindSubmatch(data)
	if operand == nil {
		t.Fatalf("no text operand found:\n%s", data)
	}
	if got, want := string(operand[1]), hex4(question)+hex4(question); got != want {
		t.Errorf("operand = %q, want %q (two substitute glyphs)", got, want)
	}

	// The ToUnicode map has to say question mark too. Mapping the substitute back to
	// the character it replaced would make a copy-paste disagree with the page, and
	// would be arbitrary as soon as two missing runes shared one glyph.
	if !bytes.Contains(data, []byte("<003F>")) {
		t.Error("the substitute glyph is not mapped to a question mark in ToUnicode")
	}
}

func TestCompositeDocumentPassesGhostscript(t *testing.T) {
	checkWithGhostscript(t, scriptDocument(t, "ScriptsGhostscript"))
}

// --- CFF (PostScript) outlines ----------------------------------------------

// cffFont loads an OpenType font with CFF outlines rather than glyf ones.
func cffFont(t *testing.T, name string) core.Font {
	t.Helper()

	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/STIXGeneral.otf",
		"/usr/share/fonts/opentype/stix/STIXGeneral.otf",
	} {
		if _, err := os.Stat(path); err != nil {
			continue
		}

		face, err := fonts.LoadTrueTypeFile(name, path)
		if err != nil {
			t.Fatalf("loading %s: %v", path, err)
		}
		return face
	}

	t.Skip("no system OpenType/CFF font available")
	return nil
}

func TestCFFFontIsEmbeddedAsAPostScriptCIDFont(t *testing.T) {
	// A CFF program is not a TrueType program. Writing it to /FontFile2 produces a
	// file that Ghostscript sniffs its way through and poppler flags outright as a
	// mismatch between the font type and the embedded file — which is what sanur did
	// before this path existed.
	face := cffFont(t, "CFFEmbedded")

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Text("CFF outlines")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	for _, want := range []string{
		"/Subtype /Type0",
		"/Encoding /Identity-H",
		"/Subtype /CIDFontType0",
		"/FontFile3",
		"/Subtype /OpenType",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("output is missing %q", want)
		}
	}

	if bytes.Contains(data, []byte("/FontFile2")) {
		t.Error("CFF outlines were written to /FontFile2, which is defined as a TrueType program")
	}
	// CIDToGIDMap belongs to the TrueType flavour only, and a reader may act on it.
	if bytes.Contains(data, []byte("/CIDToGIDMap")) {
		t.Error("a PostScript CID font carries no CIDToGIDMap")
	}

	checkWithGhostscript(t, data)
}

func TestCFFFontReportsNoMismatchToPoppler(t *testing.T) {
	// pdffonts is the check that matters here: it reports the font type it inferred
	// from the dictionary alongside what it found in the stream, and warns on stderr
	// when the two disagree. Ghostscript accepts the mismatched file silently, so
	// nothing else in the suite would notice a regression.
	pdffonts, err := exec.LookPath("pdffonts")
	if err != nil {
		t.Skip("pdffonts not installed")
	}

	face := cffFont(t, "CFFPoppler")

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Text("CFF outlines through poppler")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "cff.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(pdffonts, path).CombinedOutput()
	if err != nil {
		t.Fatalf("pdffonts failed: %v\n%s", err, out)
	}

	if bytes.Contains(bytes.ToLower(out), []byte("mismatch")) {
		t.Errorf("poppler reports a font mismatch:\n%s", out)
	}
	// "emb yes" and "uni yes" are the two columns that matter: the program is in the
	// file, and there is a map from its glyphs back to characters.
	if !bytes.Contains(out, []byte("Identity-H")) {
		t.Errorf("poppler does not see an Identity-H encoding:\n%s", out)
	}
}

func TestCFFFontTextIsExtractable(t *testing.T) {
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	face := cffFont(t, "CFFExtract")

	const greek = "Ξεσκεπάζω"

	doc := sanur.New()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Text(greek)
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	path := filepath.Join(t.TempDir(), "cff.pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(pdftotext, path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext failed: %v", err)
	}
	if !bytes.Contains(out, []byte(greek)) {
		t.Errorf("Greek text from a CFF font did not survive extraction; got:\n%s", out)
	}
}

// --- right-to-left scripts --------------------------------------------------

// rtlScripts are the languages that need reordering, and Arabic additionally needs
// contextual letter forms. Every one of these used to render as a row of question
// marks; before reordering they rendered as correct letters in the wrong order, which
// is worse, because it looks like text.
var rtlScripts = []struct{ name, text string }{
	{"hebrew", "דג סקרן שט בים"},
	{"hebrew with a number", "עמוד 12 מתוך 34"},
	{"arabic", "السلام عليكم ورحمة الله"},
	{"arabic with a number", "الصفحة 12 من 34"},
	{"arabic with latin", "مرحبا Go بالعالم"},
	{"arabic ligature", "لا إله إلا الله"},
	{"persian", "سلام دنیا"},
}

// rtlDocument renders the given lines with an embedded font.
func rtlDocument(t *testing.T, name string, lines ...string) []byte {
	return rtlDocumentWith(t, name, true, lines...)
}

// rtlDocumentWith is rtlDocument with control over compression, so a test that needs to
// read the ToUnicode stream back can have it in plain text.
func rtlDocumentWith(t *testing.T, name string, compress bool, lines ...string) []byte {
	t.Helper()

	face := embeddedFont(t, name)
	if face.AdvanceOf('م', 12) <= 0 || face.AdvanceOf('ﻣ', 12) <= 0 {
		t.Skip("the test font has no Arabic presentation forms")
	}

	doc := sanur.New().Title("Right to left")
	if !compress {
		doc = doc.Uncompressed()
	}
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(45)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(12)
			for _, line := range lines {
				c.Item().Text(line)
			}
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating document: %v", err)
	}
	return data
}

// allRTLText is every line above, for the checks that do not care which document a
// line came from.
func allRTLText() []string {
	lines := make([]string, 0, len(rtlScripts))
	for _, script := range rtlScripts {
		lines = append(lines, script.text)
	}
	return lines
}

func TestRightToLeftTextIsExtractableAsWritten(t *testing.T) {
	// This is the check that proves the whole chain. Extraction fails if the text was
	// not reordered, if it was reordered twice, if the Arabic was left unshaped, or — the
	// subtle one — if the shaped presentation forms were reported as the document's text
	// instead of the letters they stand for, which would leave it unsearchable.
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	dir := t.TempDir()

	// One document per script, which is both realistic and necessary. Arabic yeh and
	// Farsi yeh are drawn with the same glyph in medial position, and Identity-H
	// addresses glyphs, so a document containing both can only declare one of them —
	// see TestGlyphSharedByTwoCharactersKeepsTheFirstMeaning.
	for i, script := range rtlScripts {
		path := filepath.Join(dir, fmt.Sprintf("rtl-%d.pdf", i))

		data := rtlDocument(t, fmt.Sprintf("RTLExtract%d", i), script.text)
		if err := os.WriteFile(path, data, 0o644); err != nil {
			t.Fatal(err)
		}

		out, err := exec.Command(pdftotext, path, "-").Output()
		if err != nil {
			t.Fatalf("pdftotext failed: %v", err)
		}

		// Extraction tools wrap each run in bidirectional control characters, and
		// reconstruct spaces from glyph positions rather than from the space glyph, so
		// neither survives a literal comparison. The characters do.
		extracted := withoutSpaces(stripBidiControls(string(out)))

		if !strings.Contains(extracted, withoutSpaces(script.text)) {
			t.Errorf("%s did not survive extraction as written; got:\n%s", script.name, out)
		}
	}
}

func TestGlyphSharedByTwoCharactersKeepsTheFirstMeaning(t *testing.T) {
	// Arabic yeh and Farsi yeh are different characters that most fonts draw with one
	// glyph in medial position, because there they look the same. Identity-H addresses
	// glyphs, so the two characters share a code and only one can be named in
	// ToUnicode. That is a limit of the encoding, not something to fix — what can be
	// fixed is the answer depending on which page was drawn last.
	face := embeddedFont(t, "RTLSharedGlyph")

	source, ok := fonts.GlyphSourceOf(face)
	if !ok {
		t.Fatal("an embedded font must expose its glyphs")
	}

	arabic, hasArabic := source.GlyphID('\ufef4') // yeh medial
	farsi, hasFarsi := source.GlyphID('\ufbff')   // Farsi yeh medial
	if !hasArabic || !hasFarsi || arabic != farsi {
		t.Skip("the test font draws the two yehs with different glyphs")
	}

	// Both orders must name the character from the line drawn first.
	for _, tc := range []struct {
		name  string
		lines []string
		want  string
	}{
		{"arabic first", []string{"عليكم", "دنیا"}, "<064A>"},
		{"farsi first", []string{"دنیا", "عليكم"}, "<06CC>"},
	} {
		data := rtlDocumentWith(t, "RTLShared"+tc.name, false, tc.lines...)

		entry := regexp.MustCompile(`<` + hex4(arabic) + `> (<[0-9A-F]+>)`).FindSubmatch(data)
		if entry == nil {
			t.Errorf("%s: glyph %d has no ToUnicode entry", tc.name, arabic)
			continue
		}
		if got := string(entry[1]); got != tc.want {
			t.Errorf("%s: glyph %d maps to %s, want %s", tc.name, arabic, got, tc.want)
		}
	}
}

// stripBidiControls removes the directional formatting characters an extraction tool
// inserts to record which runs were right-to-left.
func stripBidiControls(s string) string {
	return strings.Map(func(r rune) rune {
		if r >= 0x2000 && r <= 0x206F {
			return -1
		}
		return r
	}, s)
}

func TestArabicIsDrawnWithContextualForms(t *testing.T) {
	// Arabic set without its contextual forms reads as a row of disconnected letters:
	// legible to nobody, and correct according to every check that only looks at
	// characters. The forms have their own codepoints, so the drawn glyphs can be
	// compared against the ones the letters should have taken.
	face := embeddedFont(t, "RTLShaping")

	source, ok := fonts.GlyphSourceOf(face)
	if !ok {
		t.Fatal("an embedded font must expose its glyphs")
	}
	if _, has := source.GlyphID('ﻣ'); !has {
		t.Skip("the test font has no Arabic presentation forms")
	}

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		// Meem-reh-hah-beh-alef. Meem opens a cluster, reh closes it because nothing
		// joins forward out of a reh, then hah opens the next one.
		p.Content().Text("مرحبا")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	operand := regexp.MustCompile(`<([0-9A-F]+)> Tj`).FindSubmatch(data)
	if operand == nil {
		t.Fatalf("no text operand found:\n%s", data)
	}

	// Reordered for display, so the last letter is drawn first.
	var want string
	for _, r := range []rune{'ﺎ', 'ﺒ', 'ﺣ', 'ﺮ', 'ﻣ'} {
		gid, ok := source.GlyphID(r)
		if !ok {
			t.Skipf("the test font has no glyph for %q", r)
		}
		want += hex4(gid)
	}

	if got := string(operand[1]); got != want {
		t.Errorf("drawn glyphs = %s, want %s (the shaped forms in visual order)", got, want)
	}
}

func TestHebrewIsDrawnInVisualOrder(t *testing.T) {
	// Hebrew needs no shaping at all — its letters do not join — so reordering is the
	// whole of what makes it correct, and the glyphs are the letters themselves.
	face := embeddedFont(t, "RTLHebrew")

	source, _ := fonts.GlyphSourceOf(face)

	doc := sanur.New().Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Margin(40)
		p.DefaultTextStyle(sanur.TextStyle().Font(face).Size(14))
		p.Content().Text("שלום")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	operand := regexp.MustCompile(`<([0-9A-F]+)> Tj`).FindSubmatch(data)
	if operand == nil {
		t.Fatalf("no text operand found:\n%s", data)
	}

	var want string
	for _, r := range []rune{'ם', 'ו', 'ל', 'ש'} {
		gid, ok := source.GlyphID(r)
		if !ok {
			t.Skip("the test font has no Hebrew")
		}
		want += hex4(gid)
	}

	if got := string(operand[1]); got != want {
		t.Errorf("drawn glyphs = %s, want %s (the letters reversed)", got, want)
	}
}

func TestRightToLeftDocumentPassesGhostscript(t *testing.T) {
	checkWithGhostscript(t, rtlDocument(t, "RTLGhostscript", allRTLText()...))
}
