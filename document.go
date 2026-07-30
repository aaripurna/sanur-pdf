package sanur

import (
	"fmt"
	"os"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
	"github.com/aaripurna/sanur-pdf/render"
)

// MaxPagesPerSection bounds how many sheets one page definition may produce.
//
// A layout bug that makes no progress would otherwise generate pages until the
// process ran out of memory. The engine also detects stalls directly, but this
// cap is the backstop for a stall it cannot recognise, and 5000 sheets is far
// past any document someone meant to produce.
const MaxPagesPerSection = 5000

// Document is a PDF under construction.
type Document struct {
	pages []*Page
	meta  render.Metadata

	// compress controls Flate encoding of content streams. It is on by default;
	// turning it off makes the output readable for debugging.
	compress bool

	// template is applied to each page definition before its own build function
	// runs. See EveryPage.
	template func(*Page)
}

// New creates an empty document.
func New() *Document {
	return &Document{
		compress: true,
		meta:     render.Metadata{Producer: "sanur"},
	}
}

// Title, Author, Subject, Keywords and Creator set document metadata.
func (d *Document) Title(v string) *Document    { d.meta.Title = v; return d }
func (d *Document) Author(v string) *Document   { d.meta.Author = v; return d }
func (d *Document) Subject(v string) *Document  { d.meta.Subject = v; return d }
func (d *Document) Keywords(v string) *Document { d.meta.Keywords = v; return d }
func (d *Document) Creator(v string) *Document  { d.meta.Creator = v; return d }

// CreationDate sets the creation timestamp, which must already be formatted as a
// PDF date string such as "D:20260730120000Z".
//
// Sanur never reads the clock itself, so that generating the same document twice
// yields byte-identical output. A caller who wants a timestamp supplies it.
func (d *Document) CreationDate(pdfDate string) *Document {
	d.meta.CreationDate = pdfDate
	return d
}

// Uncompressed leaves content streams as plain text, for inspecting output.
func (d *Document) Uncompressed() *Document {
	d.compress = false
	return d
}

// EveryPage sets defaults applied to every page definition added afterwards:
// page size, margins, background, default text style, and the header and footer.
//
// A single page definition already repeats its own header and footer on every
// sheet it produces. EveryPage covers the other case — a document built from
// several definitions that should nevertheless look like one document — without
// each definition having to remember to install the same furniture.
//
// It takes a function rather than a prepared element tree, and that is the whole
// point. Elements carry pagination state: a column remembers which item it
// reached, a text block which line. Sharing one header instance across
// definitions would share that state too, and the second definition would
// inherit a header that believed it had already been drawn. Running the function
// afresh per definition gives each one its own instances.
//
// A definition can still override anything the template set, since its own build
// function runs second:
//
//	doc.EveryPage(func(p *sanur.Page) {
//		p.Size(sanur.A4).Margin(40)
//		p.Header().Text("Annual Report")
//		p.Footer().AlignCenter().PageNumber("Page {page} of {total}")
//	})
//
//	doc.Page(func(p *sanur.Page) {
//		p.Content().Text("Inherits the size, margins and furniture.")
//	})
//
//	doc.Page(func(p *sanur.Page) {
//		p.Size(sanur.Landscape(sanur.A4))  // overrides just the size
//		p.Content().Text("Keeps the same header and footer.")
//	})
func (d *Document) EveryPage(build func(*Page)) *Document {
	d.template = build
	return d
}

// Page appends a page definition and configures it through build.
//
// One definition can produce many sheets: its content is laid out repeatedly
// until exhausted, with the header and footer redrawn on each. That is why this
// is a "page definition" rather than a page — a definition holding a long table
// becomes as many sheets as the table needs.
//
// Any defaults set by EveryPage are applied first, so build can override them.
func (d *Document) Page(build func(*Page)) *Document {
	p := newPage()
	d.pages = append(d.pages, p)

	if d.template != nil {
		d.template(p)
	}
	build(p)

	return d
}

// Bytes lays out the document and returns the PDF file.
func (d *Document) Bytes() ([]byte, error) {
	if len(d.pages) == 0 {
		return nil, fmt.Errorf("sanur: document has no pages; call Page first")
	}

	total, err := d.countPages()
	if err != nil {
		return nil, err
	}

	builder := render.NewBuilder(d.meta, d.compress)
	if _, err := d.layout(builder, total); err != nil {
		return nil, err
	}
	return builder.Bytes()
}

// Write lays out the document and writes it to path.
func (d *Document) Write(path string) error {
	data, err := d.Bytes()
	if err != nil {
		return err
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		return fmt.Errorf("sanur: writing %s: %w", path, err)
	}
	return nil
}

// countPages determines the page total by laying the document out onto a canvas
// that discards everything.
//
// A "page N of M" label cannot be rendered until M is known, and M cannot be
// known until the document has been laid out — but the label's own width affects
// the layout. The loop below resolves that by laying out repeatedly until the
// count stops changing. In practice it settles on the second pass, since only a
// digit count changes; the iteration limit exists for the pathological case where
// a document oscillates between two totals, in which case the last count is used
// and the document is still valid, just possibly with an off-by-one label.
func (d *Document) countPages() (int, error) {
	const maxAttempts = 3

	total := 0
	for attempt := 0; attempt < maxAttempts; attempt++ {
		count, err := d.layout(nil, total)
		if err != nil {
			return 0, err
		}
		if count == total {
			break
		}
		total = count
	}
	return total, nil
}

// layout runs the whole document through the engine, drawing onto builder or, if
// builder is nil, onto a discard canvas. It returns the number of sheets.
func (d *Document) layout(builder *render.Builder, totalPages int) (int, error) {
	pageNumber := 0

	for index, page := range d.pages {
		// Every pass starts from scratch, so any progress recorded by a previous
		// pass is discarded. Without this the counting pass would leave content
		// half-consumed and the real pass would render only the remainder.
		page.reset()

		produced, err := d.layoutPage(builder, page, &pageNumber, totalPages)
		if err != nil {
			return 0, fmt.Errorf("sanur: page definition %d: %w", index+1, err)
		}
		if produced == 0 {
			return 0, fmt.Errorf("sanur: page definition %d produced no pages", index+1)
		}
	}

	return pageNumber, nil
}

// layoutPage renders one page definition across as many sheets as it needs.
func (d *Document) layoutPage(
	builder *render.Builder,
	page *Page,
	pageNumber *int,
	totalPages int,
) (int, error) {
	inner := page.contentArea()
	if inner.Width <= 0 || inner.Height <= 0 {
		return 0, fmt.Errorf(
			"margins of %.1f/%.1f/%.1f/%.1f leave no room on a %.1fx%.1f page",
			page.margin.top, page.margin.right, page.margin.bottom, page.margin.left,
			page.size.Width, page.size.Height)
	}

	sheets := 0
	stalled := 0

	for {
		if sheets >= MaxPagesPerSection {
			return 0, fmt.Errorf(
				"exceeded %d pages; the content is most likely not shrinking between pages",
				MaxPagesPerSection)
		}

		*pageNumber++
		sheets++

		ctx := core.PageContext{PageNumber: *pageNumber, TotalPages: totalPages}

		// The header and footer are redrawn in full on every sheet, so any
		// progress they recorded on the previous one has to be rewound. Skipping
		// this would make a text header render its first line on page one and
		// nothing thereafter.
		page.resetFurniture()
		page.applyContext(ctx)

		furniture, err := page.measureFurniture(inner)
		if err != nil {
			return 0, err
		}

		contentSpace := core.Size{
			Width:  inner.Width,
			Height: inner.Height - furniture.headerHeight - furniture.footerHeight,
		}
		if contentSpace.Height <= 0 {
			return 0, fmt.Errorf(
				"header (%.1f) and footer (%.1f) leave no room for content in %.1f points",
				furniture.headerHeight, furniture.footerHeight, inner.Height)
		}

		contentPlan := page.content.Measure(contentSpace)
		if contentPlan.Wrapped() {
			// The content did not fit even though this sheet is empty of content,
			// so no future sheet will be roomier either.
			return 0, fmt.Errorf(
				"content does not fit in the available %.1fx%.1f: %s",
				contentSpace.Width, contentSpace.Height, contentPlan.WrapReason)
		}

		canvas, closer, err := d.newCanvas(builder, page.size)
		if err != nil {
			return 0, err
		}

		d.drawSheet(canvas, page, inner, furniture, contentPlan.Size)

		if err := canvas.Err(); err != nil {
			return 0, err
		}
		if err := closer(); err != nil {
			return 0, err
		}

		if contentPlan.Full() {
			return sheets, nil
		}

		// A partial render of zero height means the sheet carried none of the
		// content — legitimate exactly once, for a page break. Twice running
		// means the layout is not advancing and would loop until the cap.
		if contentPlan.Size.Height < core.Epsilon {
			stalled++
			if stalled > 1 {
				return 0, fmt.Errorf(
					"layout stalled: two consecutive pages rendered no content in %.1fx%.1f",
					contentSpace.Width, contentSpace.Height)
			}
		} else {
			stalled = 0
		}
	}
}

// newCanvas returns a canvas for one sheet plus the function that finalises it.
func (d *Document) newCanvas(
	builder *render.Builder,
	size core.Size,
) (core.Canvas, func() error, error) {
	if builder == nil {
		c := render.NewDiscardCanvas()
		return c, func() error { return c.Err() }, nil
	}
	c := builder.NewPage(size)
	return c, c.Close, nil
}

// drawSheet paints one sheet: background, header, content, footer.
func (d *Document) drawSheet(
	canvas core.Canvas,
	page *Page,
	inner core.Size,
	furniture furnitureSizes,
	contentSize core.Size,
) {
	if page.background.Visible() {
		canvas.DrawRect(core.Position{}, page.size, page.background)
	}

	origin := core.Position{X: page.margin.left, Y: page.margin.top}

	if furniture.headerHeight > 0 {
		canvas.Save()
		canvas.Translate(origin)
		page.header.Draw(canvas, core.Size{Width: inner.Width, Height: furniture.headerHeight})
		canvas.Restore()
	}

	canvas.Save()
	canvas.Translate(origin.Add(0, furniture.headerHeight))
	page.content.Draw(canvas, core.Size{Width: inner.Width, Height: contentSize.Height})
	canvas.Restore()

	if furniture.footerHeight > 0 {
		// The footer is pinned to the bottom margin rather than following the
		// content, so it lands in the same place whether the sheet is full or
		// nearly empty.
		footerTop := page.size.Height - page.margin.bottom - furniture.footerHeight
		canvas.Save()
		canvas.Translate(core.Position{X: page.margin.left, Y: footerTop})
		page.footer.Draw(canvas, core.Size{Width: inner.Width, Height: furniture.footerHeight})
		canvas.Restore()
	}
}

// furnitureSizes holds the resolved header and footer heights for one sheet.
type furnitureSizes struct {
	headerHeight float64
	footerHeight float64
}

// Page is one page definition: its geometry, its furniture and its content.
type Page struct {
	size       core.Size
	margin     margins
	background core.Color
	style      core.TextStyle

	header  *elements.Container
	content *elements.Container
	footer  *elements.Container
}

type margins struct {
	top, right, bottom, left float64
}

func newPage() *Page {
	return &Page{
		size:       A4,
		background: White,
		style:      TextStyle().Build(),
		header:     elements.NewContainer(),
		content:    elements.NewContainer(),
		footer:     elements.NewContainer(),
	}
}

// Size sets the sheet dimensions.
func (p *Page) Size(s core.Size) *Page {
	p.size = s
	return p
}

// Margin sets all four margins.
func (p *Page) Margin(v float64) *Page {
	p.margin = margins{top: v, right: v, bottom: v, left: v}
	return p
}

// MarginXY sets horizontal and vertical margins.
func (p *Page) MarginXY(x, y float64) *Page {
	p.margin = margins{top: y, bottom: y, left: x, right: x}
	return p
}

// MarginEach sets each margin independently.
func (p *Page) MarginEach(top, right, bottom, left float64) *Page {
	p.margin = margins{top: top, right: right, bottom: bottom, left: left}
	return p
}

// Background fills the whole sheet, behind the margins as well as inside them.
func (p *Page) Background(c core.Color) *Page {
	p.background = c
	return p
}

// DefaultTextStyle sets the style inherited by the page's content.
func (p *Page) DefaultTextStyle(style *StyleBuilder) *Page {
	p.style = style.Build()
	return p
}

// Header returns the slot drawn at the top of every sheet.
func (p *Page) Header() *Container {
	return newContainer(p.header.Set, p.style)
}

// Content returns the slot for the page body, the only part that paginates.
func (p *Page) Content() *Container {
	return newContainer(p.content.Set, p.style)
}

// Footer returns the slot pinned to the bottom of every sheet.
func (p *Page) Footer() *Container {
	return newContainer(p.footer.Set, p.style)
}

// contentArea is the sheet minus its margins.
func (p *Page) contentArea() core.Size {
	return core.Size{
		Width:  p.size.Width - p.margin.left - p.margin.right,
		Height: p.size.Height - p.margin.top - p.margin.bottom,
	}
}

// reset rewinds the whole page, including content progress.
func (p *Page) reset() {
	core.ResetTree(p.header, true)
	core.ResetTree(p.content, true)
	core.ResetTree(p.footer, true)
}

// resetFurniture rewinds only the header and footer, which are redrawn whole on
// every sheet while the content carries on where it left off.
func (p *Page) resetFurniture() {
	core.ResetTree(p.header, true)
	core.ResetTree(p.footer, true)
}

// applyContext pushes the page context into context-aware elements.
func (p *Page) applyContext(ctx core.PageContext) {
	core.ApplyPageContext(p.header, ctx)
	core.ApplyPageContext(p.content, ctx)
	core.ApplyPageContext(p.footer, ctx)
}

// measureFurniture resolves the header and footer heights for one sheet.
//
// Both are measured against the full inner area rather than a share of it, so a
// header is free to be as tall as it needs. What it takes is then subtracted from
// what the content may use.
func (p *Page) measureFurniture(inner core.Size) (furnitureSizes, error) {
	var f furnitureSizes

	headerPlan := p.header.Measure(inner)
	if headerPlan.Wrapped() {
		return f, fmt.Errorf("header does not fit: %s", headerPlan.WrapReason)
	}
	f.headerHeight = headerPlan.Size.Height

	footerPlan := p.footer.Measure(inner)
	if footerPlan.Wrapped() {
		return f, fmt.Errorf("footer does not fit: %s", footerPlan.WrapReason)
	}
	f.footerHeight = footerPlan.Size.Height

	return f, nil
}
