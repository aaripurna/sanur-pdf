package sanur

import (
	"fmt"
	"math"
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
//
// # Concurrency
//
// A Document is not safe for concurrent use, and neither is anything reachable from one.
// Build and generate each document from a single goroutine.
//
// The reason is not just unguarded maps in the writer: elements carry pagination state.
// A column remembers which item it reached and a text block which line, because that is
// what lets content resume on the next sheet. Two goroutines drawing the same element
// tree would interleave that progress and produce two wrong documents rather than one
// error. It is also why EveryPage takes a function instead of a prepared tree.
//
// Generating several documents at once is fine, and is what a server does. Fonts, themes
// and decoded images may be shared freely between them: a fonts.Registry is safe for
// concurrent use, a loaded face guards its own metric caches, and a theme is read-only
// once parsed. Load those once at startup and hand them to as many documents as you like.
type Document struct {
	pages []*Page
	meta  render.Metadata

	// compress controls Flate encoding of content streams. It is on by default;
	// turning it off makes the output readable for debugging.
	compress bool

	// template is applied to each page definition before its own build function
	// runs. See EveryPage.
	template func(*Page)

	// language is the document's natural language, and switching tagging on. Empty
	// means untagged, which is the default.
	language string
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

// Tagged records the document's logical structure alongside its ink, and sets the
// natural language — a BCP 47 tag such as "en-GB".
//
// An ordinary PDF says where marks go and nothing more. A heading is text that happens
// to be large; a table is lines that happen to form a grid. Nothing in the file says so,
// which is why a PDF is opaque to a screen reader, cannot reflow onto a small display,
// and resists conversion to anything structured. Tagging is the parallel structure that
// carries the meaning, and it is a legal requirement for public-sector documents in much
// of the world.
//
// Most of it is inferred: text is a paragraph, an image a figure, a rule and a running
// header decoration a reader should skip. Two things are not, and both fail generation
// rather than producing a document that merely passes for accessible:
//
//   - A heading has to be declared with Container.Tag, since a font size cannot reveal
//     whether text is a first- or a third-level heading, and an outline that is
//     confidently wrong is worse than none.
//   - An image has to be described with Container.Describe, since a figure with nothing
//     to read out is exactly the gap tagging exists to close.
//
// The language is required because a reader that does not know what language a document
// is in cannot pronounce it.
func (d *Document) Tagged(language string) *Document {
	d.language = language
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

	resolved, err := d.resolve()
	if err != nil {
		return nil, err
	}

	builder := render.NewBuilder(d.meta, d.compress)
	if d.language != "" {
		builder.Tag(d.language)
	}

	if _, err := d.layout(builder, resolved); err != nil {
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

// pageFacts is what a page needs to know about the document as a whole, and cannot
// know while it is being laid out.
type pageFacts struct {
	// total is the sheet count, zero until it has been established.
	total int

	// destinations maps a named destination to the sheet it landed on.
	destinations map[string]int
}

// resolve establishes the page total and the page of every named destination, by
// laying the document out onto a canvas that discards everything.
//
// Neither can be known before the document has been laid out, and printing either
// changes the widths involved, which can change the layout — a table of contents that
// grows a column of page numbers may itself push a section onto the next sheet. So the
// answers are found by repetition: lay out, see what the facts are, lay out again, and
// stop when nothing moves.
//
// In practice it settles on the second or third pass, since only a digit count changes.
// The limit exists for a document that oscillates, which then renders with the last
// answer — still a valid file, possibly with a label off by one. That is a far better
// outcome than refusing to generate.
func (d *Document) resolve() (pageFacts, error) {
	const maxAttempts = 5

	facts := pageFacts{destinations: map[string]int{}}

	for attempt := 0; attempt < maxAttempts; attempt++ {
		found := map[string]int{}

		count, err := d.layoutWith(nil, facts, found)
		if err != nil {
			return facts, err
		}

		settled := count == facts.total && samePages(facts.destinations, found)
		facts = pageFacts{total: count, destinations: found}

		if settled {
			break
		}
	}

	return facts, nil
}

// samePages reports whether two destination maps agree.
func samePages(a, b map[string]int) bool {
	if len(a) != len(b) {
		return false
	}
	for name, page := range a {
		if b[name] != page {
			return false
		}
	}
	return true
}

// layout runs the whole document through the engine, drawing onto builder or, if
// builder is nil, onto a discard canvas. It returns the number of sheets.
func (d *Document) layout(builder *render.Builder, facts pageFacts) (int, error) {
	return d.layoutWith(builder, facts, nil)
}

// layoutWith is layout, additionally recording where each destination landed when
// record is non-nil.
func (d *Document) layoutWith(
	builder *render.Builder,
	facts pageFacts,
	record map[string]int,
) (int, error) {
	pageNumber := 0

	for index, page := range d.pages {
		// Every pass starts from scratch, so any progress recorded by a previous
		// pass is discarded. Without this the counting pass would leave content
		// half-consumed and the real pass would render only the remainder.
		page.reset()

		produced, err := d.layoutPage(builder, page, &pageNumber, facts, record)
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
	facts pageFacts,
	record map[string]int,
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

		ctx := core.PageContext{
			PageNumber:   *pageNumber,
			TotalPages:   facts.total,
			Destinations: facts.destinations,
		}

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

		contentHeight := inner.Height - furniture.headerHeight - furniture.footerHeight
		if contentHeight <= 0 {
			return 0, fmt.Errorf(
				"header (%.1f) and footer (%.1f) leave no room for content in %.1f points",
				furniture.headerHeight, furniture.footerHeight, inner.Height)
		}

		// The lead-in belongs to the first sheet only, and takes its space out of the
		// columns before they are divided up.
		spanning := core.EmptyRender()
		if sheets == 1 {
			spanning = page.spanning.Measure(core.Size{
				Width:  inner.Width,
				Height: contentHeight,
			})
			if !spanning.Full() {
				return 0, fmt.Errorf(
					"content spanning the columns does not fit the %.1fx%.1f above them: %s",
					inner.Width, contentHeight, spanning.WrapReason)
			}
			contentHeight -= spanning.Size.Height
			if contentHeight <= 0 {
				return 0, fmt.Errorf(
					"content spanning the columns takes all %.1f points, leaving them nothing",
					spanning.Size.Height)
			}
		}

		columns, err := page.columnLayout(inner.Width, contentHeight)
		if err != nil {
			return 0, err
		}

		// The first column is measured before a sheet is committed to, so content
		// that can never fit is reported without emitting a page for it.
		contentPlan := page.content.Measure(columns.box)
		if contentPlan.Wrapped() {
			// The content did not fit even though this column is empty, so no
			// future column will be roomier either.
			return 0, fmt.Errorf(
				"content does not fit in the available %.1fx%.1f: %s",
				columns.box.Width, columns.box.Height, contentPlan.WrapReason)
		}

		canvas, closer, err := d.newCanvas(builder, page.size)
		if err != nil {
			return 0, err
		}
		if record != nil {
			canvas = recordDestinations(canvas, *pageNumber, record)
		}

		filled, finished, err := d.drawSheet(
			canvas, page, inner, furniture, spanning, columns, contentPlan, sheets == 1)
		if err != nil {
			return 0, err
		}

		if err := canvas.Err(); err != nil {
			return 0, err
		}
		if err := closer(); err != nil {
			return 0, err
		}

		if finished {
			return sheets, nil
		}

		// A sheet that carried none of the content is legitimate exactly once, for
		// a page break. Twice running means the layout is not advancing and would
		// loop until the cap.
		if filled < core.Epsilon {
			stalled++
			if stalled > 1 {
				return 0, fmt.Errorf(
					"layout stalled: two consecutive pages rendered no content in %.1fx%.1f",
					columns.box.Width, columns.box.Height)
			}
		} else {
			stalled = 0
		}
	}
}

// recordDestinations wraps a canvas so that every named destination drawn on it is
// noted against the sheet it landed on.
//
// The document is the only thing that knows the sheet number, which is why this sits
// here rather than in the canvas.
//
// The first registration of a name wins. In a valid document that never comes up, since
// a name may be registered only once and the writer reports a duplicate as an error —
// furniture redrawn on later sheets has its anchors suppressed before reaching here, so
// a header anchor arrives exactly once. Keeping the first is simply the stable choice
// for a document that is not valid yet.
func recordDestinations(canvas core.Canvas, page int, into map[string]int) core.Canvas {
	return destinationRecorder{Canvas: canvas, page: page, pages: into}
}

type destinationRecorder struct {
	core.Canvas

	page  int
	pages map[string]int
}

func (r destinationRecorder) Destination(name string, pos core.Position) {
	if name != "" {
		if _, seen := r.pages[name]; !seen {
			r.pages[name] = r.page
		}
	}
	r.Canvas.Destination(name, pos)
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
// drawSheet paints one sheet and reports how deep its columns were filled and whether
// the page definition is finished with them.
func (d *Document) drawSheet(
	canvas core.Canvas,
	page *Page,
	inner core.Size,
	furniture furnitureSizes,
	spanning core.SpacePlan,
	columns columnLayout,
	firstPlan core.SpacePlan,
	firstSheet bool,
) (float64, bool, error) {
	if page.background.Visible() {
		canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
		canvas.DrawRect(core.Position{}, page.size, page.background)
		canvas.EndMarked()
	}

	origin := core.Position{X: page.margin.left, Y: page.margin.top}

	// Furniture is redrawn on every sheet, so anything it registers by name has to
	// be registered once. Links still fire every time — a footer URL should be
	// clickable on every page — but destinations and outline entries do not.
	furnitureCanvas := canvas
	if !firstSheet {
		furnitureCanvas = core.WithoutAnchors(canvas)
	}

	// The watermark spans the whole sheet, margins included, and reserves no space:
	// it sits behind the content rather than beside it.
	canvas.Save()
	drawFurniture(canvas, furnitureCanvas, page.watermark, page.size)
	canvas.Restore()

	if furniture.headerHeight > 0 {
		canvas.Save()
		canvas.Translate(origin)
		drawFurniture(canvas, furnitureCanvas, page.header, core.Size{Width: inner.Width, Height: furniture.headerHeight})
		canvas.Restore()
	}

	contentTop := origin.Add(0, furniture.headerHeight)

	if firstSheet && spanning.Size.Height > 0 {
		canvas.Save()
		canvas.Translate(contentTop)
		page.spanning.Draw(canvas, core.Size{
			Width:  inner.Width,
			Height: spanning.Size.Height,
		})
		canvas.Restore()

		contentTop = contentTop.Add(0, spanning.Size.Height)
	}

	filled, finished, err := drawColumns(canvas, page, contentTop, columns, firstPlan)
	if err != nil {
		return 0, false, err
	}

	if furniture.footerHeight > 0 {
		// The footer is pinned to the bottom margin rather than following the
		// content, so it lands in the same place whether the sheet is full or
		// nearly empty.
		footerTop := page.size.Height - page.margin.bottom - furniture.footerHeight
		canvas.Save()
		canvas.Translate(core.Position{X: page.margin.left, Y: footerTop})
		drawFurniture(canvas, furnitureCanvas, page.footer, core.Size{Width: inner.Width, Height: furniture.footerHeight})
		canvas.Restore()
	}

	// Last, so it paints over everything.
	canvas.Save()
	drawFurniture(canvas, furnitureCanvas, page.overlay, page.size)
	canvas.Restore()

	return filled, finished, nil
}

// drawColumns flows the content through one sheet's columns.
//
// Each column is measured immediately before it is drawn, because drawing is what
// advances the content's progress: the plan for column two only exists once column one
// has been committed. That is the same sequence the sheet loop runs, one level down,
// which is why a paragraph splits between columns without knowing columns exist.
//
// It returns the depth the deepest column reached, and whether the content is spent.
func drawColumns(
	canvas core.Canvas,
	page *Page,
	origin core.Position,
	columns columnLayout,
	firstPlan core.SpacePlan,
) (float64, bool, error) {
	plan := firstPlan
	filled := 0.0
	finished := false

	for i, x := range columns.offsets {
		if i > 0 {
			plan = page.content.Measure(columns.box)
			if plan.Wrapped() {
				// Every column on every sheet is this size, so content that will not
				// start in an empty one will not start anywhere.
				return 0, false, fmt.Errorf(
					"content does not fit in a %.1fx%.1f column: %s",
					columns.box.Width, columns.box.Height, plan.WrapReason)
			}
		}

		canvas.Save()
		canvas.Translate(origin.Add(x, 0))
		page.content.Draw(canvas, core.Size{
			Width:  columns.box.Width,
			Height: plan.Size.Height,
		})
		canvas.Restore()

		filled = math.Max(filled, plan.Size.Height)

		if plan.Full() {
			// Everything left fitted in this column, so the remaining ones stay empty
			// and the definition is done.
			finished = true
			break
		}
	}

	drawColumnRule(canvas, page, origin, columns, filled)

	return filled, finished, nil
}

// drawColumnRule draws the hairline between columns, if one was asked for.
//
// It spans the depth the columns were filled to rather than the whole content area, so
// a final sheet holding two lines does not get a rule down the rest of the page.
func drawColumnRule(
	canvas core.Canvas,
	page *Page,
	origin core.Position,
	columns columnLayout,
	filled float64,
) {
	if !page.rule.visible() || columns.count() < 2 || filled <= 0 {
		return
	}

	// A rule carries no meaning, so a reader should not be told it is there.
	canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
	defer canvas.EndMarked()

	canvas.Save()
	defer canvas.Restore()
	canvas.Translate(origin)

	for _, x := range columns.offsets[1:] {
		// Centred in the gap, which starts where the previous column ended.
		center := x - page.columnSpacing/2

		canvas.DrawLine(
			core.Position{X: center},
			core.Position{X: center, Y: filled},
			page.rule.color,
			page.rule.width,
		)
	}
}

// drawFurniture draws a running element inside an artifact scope.
//
// Running furniture is decoration whatever it contains: a header repeated on forty sheets
// is not forty paragraphs to announce, and "Page 12 of 40" read out between every two
// paragraphs is worse than silence. A tagged document has no third category between
// content and artifact, so this is what keeps the furniture out of the structure.
//
// A link inside it is the exception, and escapes: every link annotation has to sit inside a
// Link element, and a "Terms" link in a footer is genuinely worth reaching.
func drawFurniture(canvas core.Canvas, inner core.Canvas, part core.Element, size core.Size) {
	canvas.BeginMarked(core.Mark{Role: core.RoleArtifact})
	defer canvas.EndMarked()

	part.Draw(inner, size)
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

	// watermark and overlay cover the whole sheet, behind and over everything
	// else, and are redrawn on every sheet like the header and footer.
	watermark *elements.Container
	overlay   *elements.Container

	// spanning sits above the columns on the first sheet only, across their whole
	// width. It is content, not furniture: it is drawn once.
	spanning *elements.Container

	// columns divides the content area into tracks the content flows through, in
	// reading order, before it moves to the next sheet.
	columns       int
	columnSpacing float64
	rule          columnRule
}

// columnRule is the optional hairline drawn down the middle of each gap.
type columnRule struct {
	width float64
	color core.Color
}

// visible reports whether the rule would draw anything.
func (r columnRule) visible() bool { return r.width > 0 && r.color.Visible() }

type margins struct {
	top, right, bottom, left float64
}

func newPage() *Page {
	return &Page{
		size:          A4,
		background:    White,
		style:         TextStyle().Build(),
		columns:       1,
		columnSpacing: defaultColumnSpacing,
		header:        elements.NewContainer(),
		content:       elements.NewContainer(),
		footer:        elements.NewContainer(),
		watermark:     elements.NewContainer(),
		overlay:       elements.NewContainer(),
		spanning:      elements.NewContainer(),
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

// Watermark returns a slot covering the whole sheet, drawn behind everything else.
//
// Unlike the header and footer it ignores the margins and takes the full page, since
// that is what a watermark or a letterhead background wants. It is drawn on every
// sheet, and takes no space away from the content.
func (p *Page) Watermark() *Container {
	return newContainer(p.watermark.Set, p.style)
}

// Overlay returns a slot covering the whole sheet, drawn over everything else.
//
// This is where a "DRAFT" stamp goes. Like the watermark it spans the full page and
// reserves no space, so it will paint over content rather than displace it —
// translucency or rotation is usually wanted.
func (p *Page) Overlay() *Container {
	return newContainer(p.overlay.Set, p.style)
}

// contentArea is the sheet minus its margins.
// Spanning returns the container for content above the columns, across all of them.
//
//	p.Columns(3)
//	p.Spanning().Text("A headline over three columns")
//
// This is the full-width lead-in an article or a newsletter opens with: a headline and a
// standfirst over the columns, with the body flowing beneath. It is drawn on the first
// sheet of the page definition and nowhere else, which is what distinguishes it from a
// header — a headline repeated on every sheet would be furniture, and this is content. It
// is tagged as content too, so it lands in the structure in reading order ahead of the
// columns.
//
// It has to fit on that first sheet: content that spans columns cannot itself be split
// between them, so generation fails rather than paginating it somewhere surprising.
func (p *Page) Spanning() *Container {
	return newContainer(p.spanning.Set, p.style)
}

// Columns divides the content area into n tracks that the content flows through.
//
//	p.Columns(2)
//
// The content fills the first track, carries on into the second, and only then moves
// to the next sheet — so a paragraph breaks between columns exactly as it breaks
// between pages, through the same mechanism. The header, the footer and the watermark
// are properties of the sheet and stay full width.
//
// This is a property of the page rather than of an element, because a column is a
// region that content flows into, and the engine already knows how to fill a region
// and continue in the next one. A two-column block sitting inside a one-column page —
// a full-width headline above the columns — is not this, and is not supported: see
// Columns in the README for the shape to use instead.
func (p *Page) Columns(n int) *Page {
	p.columns = n
	return p
}

// ColumnSpacing sets the gap between columns, which defaults to 18 points.
func (p *Page) ColumnSpacing(v float64) *Page {
	p.columnSpacing = v
	return p
}

// ColumnRule draws a vertical hairline down the middle of each gap.
//
// It spans the depth the columns are filled to on that sheet rather than the whole
// content area, so a half-empty final sheet has no rule dangling below its text. The
// rule is decoration, and is marked as an artifact in a tagged document.
func (p *Page) ColumnRule(width float64, c core.Color) *Page {
	p.rule = columnRule{width: width, color: c}
	return p
}

// defaultColumnSpacing is a gap wide enough to read two columns of body text as
// separate at ordinary sizes, and is what CSS would call 1em at 18 points.
const defaultColumnSpacing = 18

// columnLayout is the geometry of one sheet's columns: the box each gets, and where
// each starts.
type columnLayout struct {
	box     core.Size
	offsets []float64
}

// count returns the number of columns.
func (c columnLayout) count() int { return len(c.offsets) }

// columnLayout divides width into the page's columns.
//
// height is the space left after the furniture, and is the same for every column: a
// column is a full-depth region, which is what makes the wrap check meaningful. If
// content will not start in an empty column it will not start in any of them.
func (p *Page) columnLayout(width, height float64) (columnLayout, error) {
	if p.columns < 1 {
		return columnLayout{}, fmt.Errorf(
			"a page needs at least one column, not %d", p.columns)
	}
	if p.columnSpacing < 0 {
		return columnLayout{}, fmt.Errorf(
			"column spacing cannot be negative, got %.1f", p.columnSpacing)
	}

	n := float64(p.columns)
	track := (width - (n-1)*p.columnSpacing) / n
	if track <= 0 {
		return columnLayout{}, fmt.Errorf(
			"%d columns with %.1f between them leave no room in %.1f points",
			p.columns, p.columnSpacing, width)
	}

	offsets := make([]float64, p.columns)
	for i := range offsets {
		offsets[i] = float64(i) * (track + p.columnSpacing)
	}

	return columnLayout{
		box:     core.Size{Width: track, Height: height},
		offsets: offsets,
	}, nil
}

func (p *Page) contentArea() core.Size {
	return core.Size{
		Width:  p.size.Width - p.margin.left - p.margin.right,
		Height: p.size.Height - p.margin.top - p.margin.bottom,
	}
}

// reset rewinds the whole page, including content progress.
func (p *Page) reset() {
	for _, part := range p.parts() {
		core.ResetTree(part, true)
	}
	core.ResetTree(p.spanning, true)
	core.ResetTree(p.content, true)
}

// resetFurniture rewinds everything drawn afresh on each sheet, leaving the
// content to carry on where it left off.
func (p *Page) resetFurniture() {
	for _, part := range p.parts() {
		core.ResetTree(part, true)
	}
}

// parts lists the furniture: everything redrawn on every sheet.
func (p *Page) parts() []core.Element {
	return []core.Element{p.header, p.footer, p.watermark, p.overlay}
}

// applyContext pushes the page context into context-aware elements.
func (p *Page) applyContext(ctx core.PageContext) {
	for _, part := range p.parts() {
		core.ApplyPageContext(part, ctx)
	}
	core.ApplyPageContext(p.spanning, ctx)
	core.ApplyPageContext(p.content, ctx)
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
