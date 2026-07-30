// Package render turns laid-out elements into PDF bytes.
//
// It owns the two Canvas implementations and the assembly of the PDF page tree.
// The layout engine in core and elements never touches this package directly:
// it draws onto a Canvas, and render decides whether those calls become content
// stream operators or are thrown away.
package render

import (
	"fmt"
	"sort"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/fonts"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// Metadata is the document information dictionary.
type Metadata struct {
	Title    string
	Author   string
	Subject  string
	Keywords string
	Creator  string
	Producer string

	// CreationDate is formatted as a PDF date string by the caller, or left
	// empty. Sanur never reads the clock itself, so that generating the same
	// document twice produces identical bytes.
	CreationDate string
}

// Builder assembles a PDF document page by page.
//
// Fonts, images and alpha states are pooled across the whole document and
// emitted into one shared resource dictionary that every page inherits. A logo
// repeated in a footer therefore costs its bytes once, not once per page.
type Builder struct {
	writer *pdfobj.Writer

	pages    []pageEntry
	pagesRef pdfobj.Ref

	fontNames map[string]string // font identity -> resource name
	fontRefs  map[string]pdfobj.Ref
	fontOrder []string

	imageNames map[string]string
	imageRefs  map[string]pdfobj.Ref
	imageOrder []string

	// alphaNames is keyed by the fill and stroke opacity pair, since PDF carries
	// the two separately and a stroked shape may differ from its fill.
	alphaNames map[[2]uint8]string

	meta Metadata
}

type pageEntry struct {
	size    core.Size
	content []byte
}

// NewBuilder creates a builder. Setting compress to false leaves content
// streams as plain text, which makes generated files readable during debugging.
func NewBuilder(meta Metadata, compress bool) *Builder {
	b := &Builder{
		writer:     pdfobj.NewWriter(compress),
		fontNames:  map[string]string{},
		fontRefs:   map[string]pdfobj.Ref{},
		imageNames: map[string]string{},
		imageRefs:  map[string]pdfobj.Ref{},
		alphaNames: map[[2]uint8]string{},
		meta:       meta,
	}
	// The page tree node is referenced by every page it contains, so its number
	// has to exist before any page object is written.
	b.pagesRef = b.writer.Reserve()
	return b
}

// NewPage returns a canvas for a fresh page of the given size. The page is
// added to the document when the canvas is closed.
func (b *Builder) NewPage(size core.Size) *PDFCanvas {
	return newPDFCanvas(b, size)
}

// fontResource registers a font and returns its resource name, e.g. "F0".
func (b *Builder) fontResource(f core.Font) (string, error) {
	if f == nil {
		return "", fmt.Errorf("sanur/render: text drawn with no font set")
	}
	id := f.Name()
	if name, ok := b.fontNames[id]; ok {
		return name, nil
	}

	program, ok := fonts.ProgramOf(f)
	if !ok {
		return "", fmt.Errorf(
			"sanur/render: font %q cannot describe itself to the PDF writer "+
				"(it must implement fonts.Programmable)", id)
	}

	name := fmt.Sprintf("F%d", len(b.fontOrder))
	ref, err := b.emitFont(program)
	if err != nil {
		return "", err
	}

	b.fontNames[id] = name
	b.fontRefs[name] = ref
	b.fontOrder = append(b.fontOrder, name)
	return name, nil
}

// emitFont writes the font dictionary and, for embedded fonts, the descriptor
// and font program.
func (b *Builder) emitFont(p fonts.FontProgram) (pdfobj.Ref, error) {
	dict := pdfobj.NewDict().
		SetName("Type", "Font").
		SetName("BaseFont", p.BaseName).
		SetName("Encoding", "WinAnsiEncoding")

	if p.Standard14 {
		// A standard-14 font is resolved by the reader from its name alone. It
		// takes no descriptor and no Widths array, and supplying one is what
		// causes readers to demand a full descriptor as well.
		dict.SetName("Subtype", "Type1")
		return b.writer.AddDict(dict), nil
	}

	if len(p.Data) == 0 {
		return 0, fmt.Errorf("sanur/render: font %q is not standard-14 but carries no data", p.BaseName)
	}

	fileDict := pdfobj.NewDict().SetInt("Length1", len(p.Data))
	fileRef := b.writer.AddStream(fileDict, p.Data)

	descriptor := pdfobj.NewDict().
		SetName("Type", "FontDescriptor").
		SetName("FontName", p.BaseName).
		SetInt("Flags", p.Flags).
		Set("FontBBox", pdfobj.IntArray(p.BBox[:])).
		SetInt("ItalicAngle", p.ItalicAngle).
		SetInt("Ascent", p.Ascent).
		SetInt("Descent", p.Descent).
		SetInt("CapHeight", p.CapHeight).
		SetInt("StemV", p.StemV).
		SetRef("FontFile2", fileRef)
	descriptorRef := b.writer.AddDict(descriptor)

	widths := make([]int, 0, p.LastChar-p.FirstChar+1)
	for c := p.FirstChar; c <= p.LastChar; c++ {
		widths = append(widths, p.Widths[c])
	}

	dict.SetName("Subtype", "TrueType").
		SetInt("FirstChar", p.FirstChar).
		SetInt("LastChar", p.LastChar).
		Set("Widths", pdfobj.IntArray(widths)).
		SetRef("FontDescriptor", descriptorRef)

	return b.writer.AddDict(dict), nil
}

// alphaResource registers a graphics state for a fill and stroke opacity pair.
//
// PDF colour operators carry no alpha; transparency lives in a separate
// ExtGState dictionary that has to be selected before drawing. One state per
// distinct pair is pooled, since documents typically use a handful of opacities
// across thousands of draw calls.
func (b *Builder) alphaResource(fill, stroke uint8) string {
	key := [2]uint8{fill, stroke}
	if name, ok := b.alphaNames[key]; ok {
		return name
	}
	name := fmt.Sprintf("GS%d", len(b.alphaNames))
	b.alphaNames[key] = name
	return name
}

// addPage records a finished page.
func (b *Builder) addPage(size core.Size, content []byte) {
	b.pages = append(b.pages, pageEntry{size: size, content: content})
}

// PageCount returns the number of pages added so far.
func (b *Builder) PageCount() int { return len(b.pages) }

// Bytes finalises the document.
func (b *Builder) Bytes() ([]byte, error) {
	if len(b.pages) == 0 {
		return nil, fmt.Errorf("sanur: document has no pages")
	}

	resourcesRef := b.emitResources()

	pageRefs := make([]string, 0, len(b.pages))
	for _, p := range b.pages {
		contentRef := b.writer.AddStream(pdfobj.NewDict(), p.content)

		page := pdfobj.NewDict().
			SetName("Type", "Page").
			SetRef("Parent", b.pagesRef).
			Set("MediaBox", pdfobj.NumArray(0, 0, p.size.Width, p.size.Height)).
			SetRef("Resources", resourcesRef).
			SetRef("Contents", contentRef)

		pageRefs = append(pageRefs, b.writer.AddDict(page).String())
	}

	b.writer.Put(b.pagesRef, pdfobj.NewDict().
		SetName("Type", "Pages").
		Set("Kids", pdfobj.Array(pageRefs...)).
		SetInt("Count", len(b.pages)).
		String())

	catalog := b.writer.AddDict(pdfobj.NewDict().
		SetName("Type", "Catalog").
		SetRef("Pages", b.pagesRef))

	return b.writer.Serialize(catalog, b.infoDict())
}

// emitResources writes the single resource dictionary shared by every page.
func (b *Builder) emitResources() pdfobj.Ref {
	resources := pdfobj.NewDict().
		Set("ProcSet", pdfobj.Array(pdfobj.Name("PDF"), pdfobj.Name("Text"), pdfobj.Name("ImageC")))

	if len(b.fontOrder) > 0 {
		fontDict := pdfobj.NewDict()
		for _, name := range b.fontOrder {
			fontDict.SetRef(name, b.fontRefs[name])
		}
		resources.SetRef("Font", b.writer.AddDict(fontDict))
	}

	if len(b.imageOrder) > 0 {
		xobjects := pdfobj.NewDict()
		for _, name := range b.imageOrder {
			xobjects.SetRef(name, b.imageRefs[name])
		}
		resources.SetRef("XObject", b.writer.AddDict(xobjects))
	}

	if len(b.alphaNames) > 0 {
		// Map iteration is randomised, so the states are emitted in a fixed order
		// to keep output byte-identical between runs.
		keys := make([][2]uint8, 0, len(b.alphaNames))
		for key := range b.alphaNames {
			keys = append(keys, key)
		}
		sort.Slice(keys, func(i, j int) bool {
			if keys[i][0] != keys[j][0] {
				return keys[i][0] < keys[j][0]
			}
			return keys[i][1] < keys[j][1]
		})

		states := pdfobj.NewDict()
		for _, key := range keys {
			state := pdfobj.NewDict().
				SetName("Type", "ExtGState").
				SetNum("ca", float64(key[0])/255).
				SetNum("CA", float64(key[1])/255)
			states.SetRef(b.alphaNames[key], b.writer.AddDict(state))
		}
		resources.SetRef("ExtGState", b.writer.AddDict(states))
	}

	return b.writer.AddDict(resources)
}

// infoDict builds the document information dictionary, or nil when no metadata
// was supplied. Entries are written in a fixed order so output stays
// byte-identical across runs.
func (b *Builder) infoDict() *pdfobj.Dict {
	d := pdfobj.NewDict()
	for _, entry := range []struct{ key, value string }{
		{"Title", b.meta.Title},
		{"Author", b.meta.Author},
		{"Subject", b.meta.Subject},
		{"Keywords", b.meta.Keywords},
		{"Creator", b.meta.Creator},
		{"Producer", b.meta.Producer},
		{"CreationDate", b.meta.CreationDate},
	} {
		if entry.value != "" {
			d.SetString(entry.key, entry.value)
		}
	}
	if d.Len() == 0 {
		return nil
	}
	return d
}
