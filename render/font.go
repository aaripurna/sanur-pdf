package render

import (
	"fmt"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
	sanurtext "github.com/aaripurna/sanur-pdf/text"
)

// fontUsage is one font as this document uses it.
//
// The glyph set is the reason this type exists. A composite font is emitted with a
// width array and a subsetted program, and both depend on which glyphs were
// actually drawn — which is not known until the last page is finished. So the font
// object's number is reserved on first use, the glyphs accumulate as text is drawn,
// and the dictionaries are written at the end.
type fontUsage struct {
	font    core.Font
	program fonts.FontProgram
	source  fonts.GlyphSource

	name string            // resource name, e.g. "F0"
	ref  pdfobj.Ref        // reserved on first use, filled in at the end
	used map[uint16][]rune // glyph -> the characters it stands for, for extraction
}

// fontResource registers a font and returns its resource name, e.g. "F0".
func (b *Builder) fontResource(f core.Font) (*fontUsage, error) {
	if f == nil {
		return nil, fmt.Errorf("sanur/render: text drawn with no font set")
	}

	id := f.Name()
	if usage, ok := b.fontsByID[id]; ok {
		return usage, nil
	}

	program, ok := fonts.ProgramOf(f)
	if !ok {
		return nil, fmt.Errorf(
			"sanur/render: font %q cannot describe itself to the PDF writer "+
				"(it must implement fonts.Programmable)", id)
	}

	usage := &fontUsage{
		font:    f,
		program: program,
		name:    fmt.Sprintf("F%d", len(b.fontOrder)),
		ref:     b.writer.Reserve(),
		used:    map[uint16][]rune{},
	}

	if program.Composite {
		source, ok := fonts.GlyphSourceOf(f)
		if !ok {
			return nil, fmt.Errorf(
				"sanur/render: font %q is composite but cannot map runes to glyphs "+
					"(it must implement fonts.GlyphSource)", id)
		}
		usage.source = source
	}

	b.fontsByID[id] = usage
	b.fontOrder = append(b.fontOrder, id)
	return usage, nil
}

// encodeText turns a Go string into the operand of a text-showing operator.
//
// The two encodings are genuinely different languages. A simple font is addressed
// by WinAnsi byte, so the string is transcoded and anything outside the repertoire
// becomes a question mark. A composite font is addressed by glyph ID, so the string
// becomes two bytes per glyph and the whole of the font's repertoire is reachable.
func (b *Builder) encodeText(usage *fontUsage, text string) string {
	if !usage.program.Composite {
		return pdfobj.StringBytes(fonts.EncodeWinAnsi(text))
	}

	codes := make([]byte, 0, len(text)*2)

	for _, r := range text {
		gid, ok := usage.source.GlyphID(r)

		meaning := sanurtext.BaseRunes(r)
		if !ok {
			// The substitute stands in, and is recorded as a question mark rather
			// than as the rune it replaced. Mapping it back to the original would
			// make a copy-paste disagree with what is on the page, and would be
			// arbitrary anyway once two different missing runes share it.
			gid = usage.source.SubstituteGlyph()
			meaning = []rune{'?'}
		}

		// What is recorded is what the glyph means, not the codepoint that produced
		// it. A shaped Arabic letter is drawn from a presentation form, and a document
		// reporting those as its text can be neither searched for the word as anyone
		// would type it nor usefully copied out of.
		//
		// The first meaning wins where a glyph has more than one. That happens for real:
		// Arabic yeh and Farsi yeh are different characters that most fonts draw with
		// the same glyph in medial position, because in that position they look the
		// same. Identity-H addresses glyphs, so the two characters share one code and
		// only one of them can be named — a limit of the encoding rather than of this
		// code. Keeping the first keeps the answer stable instead of depending on which
		// page happened to be drawn last.
		if _, seen := usage.used[gid]; !seen {
			usage.used[gid] = meaning
		}
		codes = append(codes, byte(gid>>8), byte(gid))
	}

	return pdfobj.HexString(codes)
}

// emitFonts writes every font dictionary. Called once, after all pages are drawn.
func (b *Builder) emitFonts() error {
	for _, id := range b.fontOrder {
		usage := b.fontsByID[id]

		var (
			dict *pdfobj.Dict
			err  error
		)
		if usage.program.Composite {
			dict, err = b.compositeFont(usage)
		} else {
			dict, err = b.simpleFont(usage.program)
		}
		if err != nil {
			return err
		}

		b.writer.Put(usage.ref, dict.String())
	}
	return nil
}

// simpleFont emits a standard-14 font: a name the reader resolves for itself.
//
// It takes no descriptor and no width array, and supplying one is what causes
// readers to demand a full descriptor as well.
func (b *Builder) simpleFont(p fonts.FontProgram) (*pdfobj.Dict, error) {
	// A conforming tagged document has to embed every font it uses, and a built-in face is
	// by definition not embedded: the file names it and the reader supplies the outlines,
	// which means the document depends on what the reader happens to have. There is
	// nothing to embed, so this cannot be fixed here — only reported, before a document
	// ships that is tagged and not conformant.
	if b.tags.enabled {
		return nil, fmt.Errorf(
			"sanur/render: %q is a built-in font with no program to embed, and a tagged "+
				"document must embed every font it uses; register a TrueType or OpenType "+
				"font instead", p.BaseName)
	}

	if !p.Standard14 {
		return nil, fmt.Errorf(
			"sanur/render: font %q is neither standard-14 nor composite, "+
				"so there is no way to embed it", p.BaseName)
	}

	return pdfobj.NewDict().
		SetName("Type", "Font").
		SetName("Subtype", "Type1").
		SetName("BaseFont", p.BaseName).
		SetName("Encoding", "WinAnsiEncoding"), nil
}

// compositeFont emits a Type0 font and everything hanging off it: the descendant
// CID font, the descriptor, the subsetted program and the ToUnicode map.
func (b *Builder) compositeFont(usage *fontUsage) (*pdfobj.Dict, error) {
	set := make(map[uint16]bool, len(usage.used))
	for gid := range usage.used {
		set[gid] = true
	}

	subset, err := usage.source.Subset(set)
	if err != nil {
		return nil, err
	}

	baseName := subset.BaseName(usage.program.BaseName)

	descriptorRef, err := b.emitDescriptor(usage.program, baseName, subset)
	if err != nil {
		return nil, err
	}

	descendant := pdfobj.NewDict().
		SetName("Type", "Font").
		SetName("BaseFont", baseName).
		Set("CIDSystemInfo", pdfobj.NewDict().
			SetString("Registry", "Adobe").
			SetString("Ordering", "Identity").
			SetInt("Supplement", 0).
			String()).
		SetRef("FontDescriptor", descriptorRef).
		SetInt("DW", defaultWidth(usage.program)).
		Set("W", widthArray(usage))

	if subset.CFF {
		// A CFF program has no glyf table to index, so the CID font is the
		// PostScript flavour. With a program that is not CID-keyed — which a plain
		// OpenType font is not — a reader uses the CIDs as glyph indices directly,
		// which is exactly what Identity-H gives it.
		descendant.SetName("Subtype", "CIDFontType0")
	} else {
		descendant.SetName("Subtype", "CIDFontType2").
			// The subsetter keeps glyph IDs where they were, so no mapping stream
			// is needed and the identity relation is declared instead.
			SetName("CIDToGIDMap", "Identity")
	}

	dict := pdfobj.NewDict().
		SetName("Type", "Font").
		SetName("Subtype", "Type0").
		SetName("BaseFont", baseName).
		SetName("Encoding", "Identity-H").
		Set("DescendantFonts", pdfobj.Array(b.writer.AddDict(descendant).String()))

	// Without this the glyphs draw correctly and the text cannot be searched,
	// selected or copied, because nothing in the file says which characters those
	// glyph numbers stand for.
	dict.SetRef("ToUnicode", b.writer.AddStream(pdfobj.NewDict(), toUnicodeCMap(usage)))

	return dict, nil
}

// emitDescriptor writes the font descriptor and the embedded program.
func (b *Builder) emitDescriptor(p fonts.FontProgram, baseName string, subset fonts.Subset) (pdfobj.Ref, error) {
	if len(subset.Data) == 0 {
		return 0, fmt.Errorf("sanur/render: font %q produced an empty program", p.BaseName)
	}

	descriptor := pdfobj.NewDict().
		SetName("Type", "FontDescriptor").
		SetName("FontName", baseName).
		SetInt("Flags", p.Flags).
		Set("FontBBox", pdfobj.IntArray(p.BBox[:])).
		SetInt("ItalicAngle", p.ItalicAngle).
		SetInt("Ascent", p.Ascent).
		SetInt("Descent", p.Descent).
		SetInt("CapHeight", p.CapHeight).
		SetInt("StemV", p.StemV)

	if subset.CFF {
		// FontFile2 is defined as a TrueType program. Putting CFF bytes there
		// produces a file that some readers sniff their way through and others
		// reject outright with a font-type mismatch.
		fileDict := pdfobj.NewDict().SetName("Subtype", "OpenType")
		descriptor.SetRef("FontFile3", b.writer.AddStream(fileDict, subset.Data))
	} else {
		fileDict := pdfobj.NewDict().SetInt("Length1", len(subset.Data))
		descriptor.SetRef("FontFile2", b.writer.AddStream(fileDict, subset.Data))
	}

	return b.writer.AddDict(descriptor), nil
}

func defaultWidth(p fonts.FontProgram) int {
	if p.DefaultWidth > 0 {
		return p.DefaultWidth
	}
	// PDF's own default, and a reasonable width for an unknown glyph.
	return 1000
}

// widthArray builds the /W entry: advance widths keyed by glyph ID.
//
// The format allows either one width per glyph or a run of glyphs sharing one, and
// consecutive glyphs are grouped into a single bracketed list. Glyph IDs from a
// subset of a text font come in dense runs — a Latin alphabet is contiguous in
// nearly every font — so grouping turns hundreds of separate entries into a handful.
func widthArray(usage *fontUsage) string {
	gids := sortedUsedGIDs(usage.used)
	if len(gids) == 0 {
		return pdfobj.Array()
	}

	var groups []string

	for i := 0; i < len(gids); {
		start := i
		for i+1 < len(gids) && gids[i+1] == gids[i]+1 {
			i++
		}
		i++

		widths := make([]int, 0, i-start)
		for _, gid := range gids[start:i] {
			widths = append(widths, usage.source.GlyphWidth(gid))
		}

		groups = append(groups,
			fmt.Sprintf("%d %s", gids[start], pdfobj.IntArray(widths)))
	}

	return pdfobj.Array(groups...)
}

// cmapEntriesPerBlock is the limit the CMap format puts on one bfchar section.
const cmapEntriesPerBlock = 100

// toUnicodeCMap builds the stream that maps glyph IDs back to characters.
//
// This is what makes the text searchable and selectable. A composite font addresses
// glyphs by a number that means nothing outside the font, so without this map a
// reader has a page it can draw and cannot read: copying a paragraph yields
// gibberish, and a full-text index sees nothing at all. It costs a few hundred bytes
// and it is the difference between a PDF and a picture of one.
func toUnicodeCMap(usage *fontUsage) []byte {
	gids := sortedUsedGIDs(usage.used)

	var b strings.Builder

	b.WriteString(`/CIDInit /ProcSet findresource begin
12 dict begin
begincmap
/CIDSystemInfo << /Registry (Adobe) /Ordering (UCS) /Supplement 0 >> def
/CMapName /Adobe-Identity-UCS def
/CMapType 2 def
1 begincodespacerange
<0000> <FFFF>
endcodespacerange
`)

	for start := 0; start < len(gids); start += cmapEntriesPerBlock {
		end := start + cmapEntriesPerBlock
		if end > len(gids) {
			end = len(gids)
		}

		fmt.Fprintf(&b, "%d beginbfchar\n", end-start)
		for _, gid := range gids[start:end] {
			fmt.Fprintf(&b, "<%04X> %s\n", gid, pdfobj.UTF16BEHex(usage.used[gid]...))
		}
		b.WriteString("endbfchar\n")
	}

	b.WriteString(`endcmap
CMapName currentdict /CMap defineresource pop
end
end
`)

	return []byte(b.String())
}

// sortedUsedGIDs returns the glyphs a font used, in ascending order.
//
// Map iteration is randomised, and everything downstream — the width runs, the
// ToUnicode blocks, the subset tag — depends on the order. Sorting here is what
// keeps two runs of the same document byte-identical.
func sortedUsedGIDs(used map[uint16][]rune) []uint16 {
	set := make(map[uint16]bool, len(used))
	for gid := range used {
		set[gid] = true
	}
	return fonts.SortedGlyphIDs(set)
}
