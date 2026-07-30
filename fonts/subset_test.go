package fonts

// These tests are in the package rather than beside it, because subsetting is
// verified by reading the font back with the same table reader that wrote it.
// Comparing decoded outlines through an external parser was the first plan and it
// is not possible: golang.org/x/image/font/sfnt refuses a font with no cmap table,
// and the subset deliberately has none — a PDF composite font addresses glyphs by
// ID, so nothing consults a character map. Comparing the raw glyph bytes is also
// the stricter check, since it cannot be fooled by two different outlines that
// happen to decode to the same segments.

import (
	"encoding/binary"
	"os"
	"strings"
	"testing"
)

// testFace loads a real font, since nothing about subsetting can be exercised with
// a synthetic one: the interesting cases are composite glyphs, a loca table in
// either format, and glyph IDs scattered across a large repertoire.
func testFace(t *testing.T) *trueTypeFont {
	t.Helper()

	for _, path := range []string{
		"/System/Library/Fonts/Supplemental/Arial.ttf",
		"/System/Library/Fonts/Supplemental/Verdana.ttf",
		"/usr/share/fonts/truetype/dejavu/DejaVuSans.ttf",
		"/usr/share/fonts/TTF/DejaVuSans.ttf",
	} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		face, err := RegisterTrueType("SubsetTest", data)
		if err != nil {
			t.Fatalf("loading %s: %v", path, err)
		}
		return face.(*trueTypeFont)
	}

	t.Skip("no system TrueType font available")
	return nil
}

// glyphsFor returns the glyph IDs a string needs.
func glyphsFor(t *testing.T, face *trueTypeFont, text string) map[uint16]bool {
	t.Helper()

	set := map[uint16]bool{}
	for _, r := range text {
		gid, ok := face.glyphIndex(r)
		if !ok {
			t.Fatalf("the test font has no glyph for %q", r)
		}
		set[gid] = true
	}
	return set
}

// outlines opens a font and returns a function reading the raw bytes of a glyph.
func outlines(t *testing.T, data []byte) (func(gid int) []byte, int) {
	t.Helper()

	container, err := parseSfnt(data)
	if err != nil {
		t.Fatalf("parsing font: %v", err)
	}

	head, err := container.table("head")
	if err != nil {
		t.Fatal(err)
	}
	maxp, err := container.table("maxp")
	if err != nil {
		t.Fatal(err)
	}
	glyf, err := container.table("glyf")
	if err != nil {
		t.Fatal(err)
	}

	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))

	offsets, err := readLoca(container, numGlyphs, int16(binary.BigEndian.Uint16(head[50:])))
	if err != nil {
		t.Fatal(err)
	}

	return func(gid int) []byte { return glyphAt(glyf, offsets, gid) }, numGlyphs
}

func TestSubsetPreservesEveryKeptOutlineExactly(t *testing.T) {
	// This is the property the whole feature rests on. A subsetter that shifts an
	// outline by one byte produces a font that loads, reports plausible metrics and
	// draws the wrong shapes — which is far harder to notice than one that fails.
	face := testFace(t)

	// A mixture on purpose: Latin, accented Latin that is usually composite,
	// Cyrillic and Greek, so the glyph IDs are scattered rather than contiguous.
	wanted := glyphsFor(t, face, "Handgloves café Zürich Съешь Ξεσκ 0123")

	subset, err := face.Subset(wanted)
	if err != nil {
		t.Fatalf("subsetting: %v", err)
	}

	original, _ := outlines(t, face.data)
	reduced, numGlyphs := outlines(t, subset.Data)

	for gid := range wanted {
		if int(gid) >= numGlyphs {
			t.Errorf("glyph %d was requested but the subset holds only %d glyphs", gid, numGlyphs)
			continue
		}

		before, after := original(int(gid)), reduced(int(gid))
		if len(before) == 0 {
			// A space has no outline in the first place, so there is nothing to
			// compare — an empty entry is the correct representation of it.
			if len(after) != 0 {
				t.Errorf("glyph %d has no outline in the font but %d bytes in the subset",
					gid, len(after))
			}
			continue
		}
		if len(after) == 0 {
			t.Errorf("glyph %d is empty in the subset", gid)
			continue
		}
		// The subset pads each entry to a four-byte boundary, so the retained
		// outline is a prefix of what is stored rather than the whole entry.
		if len(after) < len(before) || string(after[:len(before)]) != string(before) {
			t.Errorf("glyph %d: outline changed (%d bytes before, %d after)",
				gid, len(before), len(after))
		}
	}
}

func TestSubsetDropsEverythingElse(t *testing.T) {
	face := testFace(t)
	wanted := glyphsFor(t, face, "abc")

	subset, err := face.Subset(wanted)
	if err != nil {
		t.Fatal(err)
	}

	reduced, numGlyphs := outlines(t, subset.Data)

	// Composite glyphs pull in their components, so the retained set is the closure
	// rather than the request. Anything outside it has to be gone, otherwise there is
	// no subsetting happening at all.
	kept, err := closeOverComponents(mustTable(t, face.data, "glyf"), mustLoca(t, face.data), wanted, numGlyphs)
	if err != nil {
		t.Fatal(err)
	}

	for gid := 0; gid < numGlyphs; gid++ {
		if kept[uint16(gid)] {
			continue
		}
		if len(reduced(gid)) != 0 {
			t.Errorf("glyph %d was retained but nothing needs it", gid)
		}
	}
}

func TestSubsetKeepsCompositeComponents(t *testing.T) {
	// An accented letter is normally stored as a reference to the base letter plus a
	// reference to the accent. Keeping the composite and dropping its parts leaves a
	// glyph that draws nothing — a bug that appears in one language and not another.
	face := testFace(t)

	glyf := mustTable(t, face.data, "glyf")
	loca := mustLoca(t, face.data)

	var (
		composite  uint16
		components []uint16
	)
	for _, r := range "áàâäãåçéèêëíìîïñóòôöõúùûüýÿ" {
		gid, ok := face.glyphIndex(r)
		if !ok {
			continue
		}
		found, err := componentsOf(glyphAt(glyf, loca, int(gid)))
		if err != nil {
			t.Fatalf("reading glyph %d: %v", gid, err)
		}
		if len(found) > 0 {
			composite, components = gid, found
			break
		}
	}
	if composite == 0 {
		t.Skip("the test font stores no accented letter as a composite glyph")
	}

	subset, err := face.Subset(map[uint16]bool{composite: true})
	if err != nil {
		t.Fatal(err)
	}

	reduced, numGlyphs := outlines(t, subset.Data)

	for _, component := range components {
		if int(component) >= numGlyphs {
			t.Errorf("component %d of composite glyph %d is outside the subset", component, composite)
			continue
		}
		if len(reduced(int(component))) == 0 {
			t.Errorf("component %d of composite glyph %d was dropped", component, composite)
		}
	}
}

func TestSubsetAlwaysKeepsNotdef(t *testing.T) {
	// Glyph zero is what a reader draws for anything it cannot resolve. Dropping it
	// turns a missing character into nothing at all instead of a visible box.
	face := testFace(t)

	subset, err := face.Subset(glyphsFor(t, face, "x"))
	if err != nil {
		t.Fatal(err)
	}

	reduced, _ := outlines(t, subset.Data)
	if len(reduced(0)) == 0 {
		original, _ := outlines(t, face.data)
		if len(original(0)) > 0 {
			t.Error(".notdef was dropped from the subset")
		}
	}
}

func TestSubsetPreservesAdvanceWidths(t *testing.T) {
	// The widths in the PDF come from the original font, but the widths a reader uses
	// for glyph positioning inside the program come from the subset's own hmtx. If
	// the two disagree, text drawn with kerning or justification drifts.
	face := testFace(t)
	wanted := glyphsFor(t, face, "Handgloves Съешь")

	subset, err := face.Subset(wanted)
	if err != nil {
		t.Fatal(err)
	}

	container, err := parseSfnt(subset.Data)
	if err != nil {
		t.Fatal(err)
	}
	hmtx, err := container.table("hmtx")
	if err != nil {
		t.Fatal(err)
	}
	hhea, err := container.table("hhea")
	if err != nil {
		t.Fatal(err)
	}
	maxp, err := container.table("maxp")
	if err != nil {
		t.Fatal(err)
	}

	// hmtx has no length of its own: a reader works out where it ends from hhea and
	// maxp. A table that disagrees with them is read at the wrong offsets from the
	// first glyph past the boundary, so the size is the invariant to assert — and
	// checking a few advances cannot see it, since the entries it reads may well be
	// correct while everything after them is not.
	metrics := int(binary.BigEndian.Uint16(hhea[34:]))
	numGlyphs := int(binary.BigEndian.Uint16(maxp[4:]))
	if want := 4*metrics + 2*(numGlyphs-metrics); len(hmtx) != want {
		t.Errorf("hmtx is %d bytes, but hhea declares %d metrics and maxp %d glyphs, "+
			"which needs %d", len(hmtx), metrics, numGlyphs, want)
	}
	if metrics > numGlyphs {
		t.Errorf("hhea declares %d metrics for %d glyphs", metrics, numGlyphs)
	}

	for gid := range wanted {
		offset := int(gid) * 4
		if offset+2 > len(hmtx) {
			t.Errorf("glyph %d has no metric entry in the subset", gid)
			continue
		}

		got := int(binary.BigEndian.Uint16(hmtx[offset:]))
		want := int(face.glyphAdvanceUnits(gid))
		if got != want {
			t.Errorf("glyph %d advance = %d units in the subset, want %d", gid, got, want)
		}
	}
}

func TestRebuildHmtxExpandsTheCompressedTail(t *testing.T) {
	// The format lets a run of trailing glyphs share the final advance and record
	// only their own left side bearing. Fonts in the wild do use it, and a subsetter
	// that reads those glyphs as if they had full entries takes their advance from
	// whatever bytes follow. Building the table by hand is the only reliable way to
	// exercise it, since whether a given font compresses its tail is not up to us.
	//
	// Two full entries then two bearing-only ones: advances 500 and 600, then two
	// glyphs that both advance 600.
	hmtx := []byte{
		0x01, 0xF4, 0x00, 0x0A, // glyph 0: advance 500, bearing 10
		0x02, 0x58, 0x00, 0x14, // glyph 1: advance 600, bearing 20
		0x00, 0x1E, // glyph 2: bearing 30
		0x00, 0x28, // glyph 3: bearing 40
	}

	got := rebuildHmtx(hmtx, 2, 4, 4)

	if len(got) != 16 {
		t.Fatalf("rebuilt table is %d bytes, want 16 (four full entries)", len(got))
	}

	for gid, want := range []struct{ advance, bearing uint16 }{
		{500, 10},
		{600, 20},
		{600, 30},
		{600, 40},
	} {
		advance := binary.BigEndian.Uint16(got[gid*4:])
		bearing := binary.BigEndian.Uint16(got[gid*4+2:])

		if advance != want.advance || bearing != want.bearing {
			t.Errorf("glyph %d = advance %d bearing %d, want %d and %d",
				gid, advance, bearing, want.advance, want.bearing)
		}
	}
}

func TestSubsetIsMuchSmallerThanTheWholeFont(t *testing.T) {
	// The size is the entire point of subsetting. A document setting one line of
	// text should not ship a megabyte of outlines for scripts it never mentions.
	face := testFace(t)

	subset, err := face.Subset(glyphsFor(t, face, "Hello, world"))
	if err != nil {
		t.Fatal(err)
	}

	if len(subset.Data) >= len(face.data)/8 {
		t.Errorf("subset is %d bytes against an original of %d; expected at least an eightfold reduction",
			len(subset.Data), len(face.data))
	}
	if len(subset.Data) == 0 {
		t.Error("the subset is empty")
	}
}

func TestSubsetTagIsStableAndGlyphDependent(t *testing.T) {
	// Output has to be byte-identical between runs, so the tag cannot come from a
	// map iteration or a clock. It also has to change with the glyph set, since
	// telling two subsets of one typeface apart is the only thing it is for.
	face := testFace(t)

	first, err := face.Subset(glyphsFor(t, face, "abc"))
	if err != nil {
		t.Fatal(err)
	}
	again, err := face.Subset(glyphsFor(t, face, "abc"))
	if err != nil {
		t.Fatal(err)
	}
	other, err := face.Subset(glyphsFor(t, face, "xyz"))
	if err != nil {
		t.Fatal(err)
	}

	if first.Tag != again.Tag {
		t.Errorf("the same glyph set produced tags %q and %q", first.Tag, again.Tag)
	}
	if first.Tag == other.Tag {
		t.Errorf("different glyph sets share the tag %q", first.Tag)
	}

	// PDF requires exactly six uppercase letters followed by a plus sign.
	if len(first.Tag) != 6 {
		t.Errorf("tag %q is %d characters, want 6", first.Tag, len(first.Tag))
	}
	for _, r := range first.Tag {
		if r < 'A' || r > 'Z' {
			t.Errorf("tag %q contains %q, which is not an uppercase letter", first.Tag, r)
		}
	}
	if got := first.BaseName("Arial"); got != first.Tag+"+Arial" {
		t.Errorf("BaseName = %q", got)
	}
}

func TestSubsetBytesAreDeterministic(t *testing.T) {
	face := testFace(t)
	wanted := glyphsFor(t, face, "Determinism")

	first, err := face.Subset(wanted)
	if err != nil {
		t.Fatal(err)
	}
	second, err := face.Subset(wanted)
	if err != nil {
		t.Fatal(err)
	}

	if string(first.Data) != string(second.Data) {
		t.Error("subsetting the same glyphs twice produced different bytes")
	}
}

func TestUnsubsettedNameHasNoTag(t *testing.T) {
	if got := (Subset{}).BaseName("Arial"); got != "Arial" {
		t.Errorf("BaseName = %q, want the bare name when nothing was subsetted", got)
	}
}

// --- helpers ----------------------------------------------------------------

func mustTable(t *testing.T, data []byte, tag string) []byte {
	t.Helper()

	container, err := parseSfnt(data)
	if err != nil {
		t.Fatal(err)
	}
	table, err := container.table(tag)
	if err != nil {
		t.Fatal(err)
	}
	return table
}

func mustLoca(t *testing.T, data []byte) []uint32 {
	t.Helper()

	container, err := parseSfnt(data)
	if err != nil {
		t.Fatal(err)
	}
	head, err := container.table("head")
	if err != nil {
		t.Fatal(err)
	}
	maxp, err := container.table("maxp")
	if err != nil {
		t.Fatal(err)
	}

	offsets, err := readLoca(container,
		int(binary.BigEndian.Uint16(maxp[4:])),
		int16(binary.BigEndian.Uint16(head[50:])))
	if err != nil {
		t.Fatal(err)
	}
	return offsets
}

func TestSubsetUsesTheLongLocaFormatWhenItHasTo(t *testing.T) {
	// The short format stores halved offsets in sixteen bits, so it runs out at 128 kB
	// of outlines. Every font on this machine uses the long format, which means the
	// short read path is exercised by reading a small subset back — but the long
	// *write* path only happens once a subset gets big, and a document with a page of
	// CJK gets there easily. Verifying it needs a deliberately large subset.
	face := testFace(t)

	wide := map[uint16]bool{}
	for r := rune(0x20); r < 0x2500; r++ {
		if gid, ok := face.glyphIndex(r); ok {
			wide[gid] = true
		}
	}
	if len(wide) < 200 {
		t.Skip("the test font is too small to produce an oversized subset")
	}

	subset, err := face.Subset(wide)
	if err != nil {
		t.Fatal(err)
	}

	container, err := parseSfnt(subset.Data)
	if err != nil {
		t.Fatal(err)
	}
	head, err := container.table("head")
	if err != nil {
		t.Fatal(err)
	}

	format := int16(binary.BigEndian.Uint16(head[50:]))
	glyf := container.tables["glyf"]

	// The choice has to follow the size rather than being fixed either way: a short
	// table that cannot address the outlines truncates them silently.
	if len(glyf) > 0x1FFFF && format != 1 {
		t.Errorf("glyf is %d bytes but indexToLocFormat is %d, which cannot address it",
			len(glyf), format)
	}
	if len(glyf) <= 0x1FFFF {
		t.Skipf("the subset came to %d bytes, which still fits the short format", len(glyf))
	}

	// And the offsets have to survive the wider format, which is the part that would
	// break: reading them back must still land on the original outlines.
	original, _ := outlines(t, face.data)
	reduced, numGlyphs := outlines(t, subset.Data)

	for gid := range wide {
		if int(gid) >= numGlyphs {
			t.Errorf("glyph %d is outside a subset of %d glyphs", gid, numGlyphs)
			continue
		}

		before, after := original(int(gid)), reduced(int(gid))
		if len(before) == 0 {
			continue
		}
		if len(after) < len(before) || string(after[:len(before)]) != string(before) {
			t.Errorf("glyph %d: outline changed under the long loca format", gid)
		}
	}
}

func TestReadLocaRejectsATruncatedOrUnknownTable(t *testing.T) {
	// A loca table shorter than the glyph count means every offset past the end reads
	// as zero, which turns real glyphs into empty ones. Failing is the only safe
	// answer, and the message has to say which format was too small.
	for _, tc := range []struct {
		name      string
		loca      []byte
		numGlyphs int
		format    int16
	}{
		{"short too small", make([]byte, 4), 8, 0},
		{"long too small", make([]byte, 8), 8, 1},
		{"unknown format", make([]byte, 64), 8, 7},
	} {
		font := &sfntFile{tables: map[string][]byte{"loca": tc.loca}}

		if _, err := readLoca(font, tc.numGlyphs, tc.format); err == nil {
			t.Errorf("%s was accepted", tc.name)
		}
	}

	// A missing table is reported by name rather than as a size problem.
	if _, err := readLoca(&sfntFile{tables: map[string][]byte{}}, 1, 0); err == nil {
		t.Error("a missing loca table was accepted")
	}
}

func TestSubsetRejectsAFontMissingWhatItNeeds(t *testing.T) {
	// Every table named here is one the subsetter reads a specific field out of, so
	// there is no useful fallback: producing a font from a guess would be worse than
	// saying which table is absent.
	full := func() map[string][]byte {
		return map[string][]byte{
			"head": make([]byte, 54),
			"maxp": {0, 1, 0, 0, 0, 2},
			"hhea": make([]byte, 36),
			"hmtx": make([]byte, 8),
			"loca": make([]byte, 12),
			"glyf": make([]byte, 4),
		}
	}

	for _, missing := range []string{"head", "maxp", "hhea", "hmtx", "loca", "glyf"} {
		tables := full()
		delete(tables, missing)

		_, err := subsetGlyf(&sfntFile{version: versionTrueType, tables: tables}, map[uint16]bool{1: true})
		if err == nil {
			t.Errorf("a font with no %q table was accepted", missing)
			continue
		}
		if !strings.Contains(err.Error(), missing) {
			t.Errorf("missing %q reported as %q", missing, err)
		}
	}

	// A table that is present but too short to hold the fields read out of it would
	// otherwise be indexed past its end.
	for _, short := range []string{"head", "maxp", "hhea"} {
		tables := full()
		tables[short] = tables[short][:1]

		if _, err := subsetGlyf(&sfntFile{version: versionTrueType, tables: tables}, map[uint16]bool{1: true}); err == nil {
			t.Errorf("a truncated %q table was accepted", short)
		}
	}
}

func TestComponentsOfReadsEveryArgumentForm(t *testing.T) {
	// The argument and transform flags change how many bytes each component occupies.
	// Misreading one desynchronises the walk and the remaining component IDs come out
	// as noise, so a font using two-by-two transforms would lose glyphs — silently,
	// since the closure would simply not include them.
	composite := func(components ...[]byte) []byte {
		// numberOfContours of -1 marks a composite; the bounding box is unread here.
		glyph := []byte{0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0}
		for _, component := range components {
			glyph = append(glyph, component...)
		}
		return glyph
	}
	// flags, glyphIndex, then arguments and any transform.
	entry := func(flags uint16, gid uint16, extra int) []byte {
		out := make([]byte, 4+extra)
		binary.BigEndian.PutUint16(out, flags)
		binary.BigEndian.PutUint16(out[2:], gid)
		return out
	}

	for _, tc := range []struct {
		name  string
		glyph []byte
		want  []uint16
	}{
		{
			"byte arguments",
			composite(entry(0, 11, 2)),
			[]uint16{11},
		},
		{
			"word arguments",
			composite(entry(compositeArgsAreWords, 12, 4)),
			[]uint16{12},
		},
		{
			"single scale",
			composite(entry(compositeHaveScale, 13, 2+2)),
			[]uint16{13},
		},
		{
			"x and y scale",
			composite(entry(compositeHaveXYScale, 14, 2+4)),
			[]uint16{14},
		},
		{
			"two by two",
			composite(entry(compositeHaveTwoByTwo, 15, 2+8)),
			[]uint16{15},
		},
		{
			"several components",
			composite(
				entry(compositeMoreComponents, 21, 2),
				entry(compositeMoreComponents|compositeArgsAreWords, 22, 4),
				entry(compositeHaveTwoByTwo, 23, 2+8),
			),
			[]uint16{21, 22, 23},
		},
	} {
		got, err := componentsOf(tc.glyph)
		if err != nil {
			t.Errorf("%s: %v", tc.name, err)
			continue
		}
		if len(got) != len(tc.want) {
			t.Errorf("%s: read %v, want %v", tc.name, got, tc.want)
			continue
		}
		for i := range got {
			if got[i] != tc.want[i] {
				t.Errorf("%s: read %v, want %v", tc.name, got, tc.want)
				break
			}
		}
	}
}

func TestComponentsOfHandlesSimpleAndBrokenGlyphs(t *testing.T) {
	// A simple glyph has no components, and neither does an entry too short to hold a
	// header — both must come back empty rather than as an error, since a font full of
	// simple glyphs is the normal case.
	for _, name := range []string{"empty", "too short", "simple"} {
		var glyph []byte
		switch name {
		case "too short":
			glyph = []byte{0xFF, 0xFF}
		case "simple":
			glyph = []byte{0, 2, 0, 0, 0, 0, 0, 0, 0, 0}
		}

		got, err := componentsOf(glyph)
		if err != nil {
			t.Errorf("%s: %v", name, err)
		}
		if len(got) != 0 {
			t.Errorf("%s: read %v, want no components", name, got)
		}
	}

	// A composite that ends in the middle of a component is a real error: continuing
	// would read whatever follows in the table as a glyph ID.
	truncated := []byte{0xFF, 0xFF, 0, 0, 0, 0, 0, 0, 0, 0, 0x00}
	if _, err := componentsOf(truncated); err == nil {
		t.Error("a composite glyph ending mid-component was accepted")
	}
}
