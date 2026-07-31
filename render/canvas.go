package render

import (
	"bytes"
	"fmt"
	"math"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// PDFCanvas writes drawing operations as PDF content stream operators.
//
// Layout works in a top-left origin space with Y increasing downwards, which is
// how nested boxes are naturally expressed, while PDF user space has its origin
// at the bottom-left with Y increasing upwards. Rather than making every element
// convert coordinates, the canvas installs one flipping transform at the top of
// the page and works in layout space throughout. The only place the flip has to
// be undone is text, which would otherwise render mirrored.
type PDFCanvas struct {
	builder *Builder
	size    core.Size
	buf     bytes.Buffer

	// page is the index this canvas will occupy once closed, needed so that
	// annotations can be attached to the right page.
	page int

	// depth tracks unbalanced Save calls so Close can detect a container that
	// forgot to Restore, which would otherwise corrupt the graphics stack
	// silently and misplace everything after it.
	depth int

	// marks is the stack of open structure elements, and whether each currently has a
	// marked-content sequence open in the stream.
	marks []markFrame

	// implicit records that the sequence currently open was opened for ink that declared
	// no role, and so has to be closed as soon as that ink is drawn.
	implicit bool

	// ctm mirrors the transform built up by Translate and Rotate, excluding the
	// page-level Y flip. Drawing does not need it — the reader applies the cm
	// operators — but annotations are positioned in absolute page coordinates and
	// have no operator stream to live in, so their geometry has to be resolved
	// here.
	ctm   matrix
	stack []matrix

	err error

	closed bool
}

func newPDFCanvas(b *Builder, size core.Size) *PDFCanvas {
	c := &PDFCanvas{
		builder: b,
		size:    size,
		page:    b.PageCount(),
		ctm:     identity(),
	}

	// Flip the Y axis so layout coordinates can be emitted verbatim. After this
	// transform, (0,0) is the top-left corner and positive Y runs down the page.
	//
	// The flip is deliberately left out of ctm: that tracks layout space, and
	// converting to PDF space happens once, in pageRect.
	c.op("%s 0 0 %s 0 %s cm", pdfobj.Num(1), pdfobj.Num(-1), pdfobj.Num(size.Height))
	return c
}

// op appends one content stream operator.
func (c *PDFCanvas) op(format string, args ...any) {
	fmt.Fprintf(&c.buf, format, args...)
	c.buf.WriteByte('\n')
}

func (c *PDFCanvas) Save() {
	c.depth++
	c.stack = append(c.stack, c.ctm)
	c.op("q")
}

func (c *PDFCanvas) Restore() {
	if c.depth == 0 {
		c.Fail(fmt.Errorf("sanur/render: Restore called without a matching Save"))
		return
	}
	c.depth--
	c.ctm = c.stack[len(c.stack)-1]
	c.stack = c.stack[:len(c.stack)-1]
	c.op("Q")
}

func (c *PDFCanvas) Translate(p core.Position) {
	if p.X == 0 && p.Y == 0 {
		return
	}
	c.ctm = translation(p.X, p.Y).mul(c.ctm)
	c.op("1 0 0 1 %s %s cm", pdfobj.Num(p.X), pdfobj.Num(p.Y))
}

func (c *PDFCanvas) Rotate(degrees float64) {
	if math.Mod(degrees, 360) == 0 {
		return
	}
	c.ctm = rotation(degrees).mul(c.ctm)

	rad := degrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)

	// In the Y-down space this canvas works in, the standard rotation matrix
	// turns clockwise on screen, which is what callers asking for a positive
	// rotation expect.
	c.op("%s %s %s %s 0 0 cm",
		pdfobj.Num(cos), pdfobj.Num(sin), pdfobj.Num(-sin), pdfobj.Num(cos))
}

// pageRect converts a local rectangle into the absolute PDF-space rectangle an
// annotation needs.
//
// Two conversions happen here. The tracked transform maps local coordinates into
// layout space, and then the Y axis is flipped: layout space grows downwards from
// the top of the page while PDF space grows upwards from the bottom, so the top
// and bottom edges swap as well as move.
func (c *PDFCanvas) pageRect(pos core.Position, size core.Size) [4]float64 {
	x0, top, x1, bottom := c.ctm.bounds(pos, size)

	return [4]float64{
		x0, c.size.Height - bottom,
		x1, c.size.Height - top,
	}
}

// Link records a clickable rectangle.
func (c *PDFCanvas) Link(pos core.Position, size core.Size, target core.LinkTarget) {
	if !target.Valid() || size.Width <= 0 || size.Height <= 0 {
		return
	}
	c.builder.addLink(c.page, c.pageRect(pos, size), target)
}

// Destination registers a named anchor.
//
// The point is nudged up by a little so that a reader scrolling to it shows the
// content rather than clipping its top edge against the window.
func (c *PDFCanvas) Destination(name string, pos core.Position) {
	if name == "" {
		return
	}
	at := c.ctm.apply(pos)
	c.builder.addDestination(name, c.page, c.size.Height-at.Y+destinationInset)
}

// destinationInset is how far above an anchor a reader is scrolled, so the
// content sits inside the window instead of flush against its top edge.
const destinationInset = 8

// Bookmark records an outline entry.
func (c *PDFCanvas) Bookmark(title string, level int, destination string) {
	if title == "" || destination == "" {
		return
	}
	c.builder.addBookmark(title, level, destination)
}

func (c *PDFCanvas) ClipRect(pos core.Position, size core.Size) {
	if size.IsEmpty() {
		// An empty clip must still be applied, otherwise content that should be
		// entirely hidden would draw at full size.
		c.op("0 0 0 0 re W n")
		return
	}
	c.op("%s %s %s %s re W n",
		pdfobj.Num(pos.X), pdfobj.Num(pos.Y),
		pdfobj.Num(size.Width), pdfobj.Num(size.Height))
}

func (c *PDFCanvas) DrawRect(pos core.Position, size core.Size, fill core.Color) {
	if !fill.Visible() || size.Width <= 0 || size.Height <= 0 {
		return
	}
	c.markContent()
	defer c.endContent()

	c.withAlpha(fill, func() {
		c.setFillColor(fill)
		c.op("%s %s %s %s re f",
			pdfobj.Num(pos.X), pdfobj.Num(pos.Y),
			pdfobj.Num(size.Width), pdfobj.Num(size.Height))
	})
}

// DrawRoundedRect fills a rectangle with rounded corners.
//
// The corner geometry lives in core.RoundedRect so that the layout engine and the
// canvas cannot disagree about where a corner sits; this is a filled path like any
// other. A zero radius still takes the rectangle operator, which is shorter than
// four straight path segments.
func (c *PDFCanvas) DrawRoundedRect(pos core.Position, size core.Size, radius float64, fill core.Color) {
	if !fill.Visible() || size.Width <= 0 || size.Height <= 0 {
		return
	}
	if math.Min(radius, math.Min(size.Width, size.Height)/2) <= 0 {
		c.DrawRect(pos, size, fill)
		return
	}
	c.DrawPath(core.RoundedRect(pos, size, radius), core.Filled(fill))
}

// DrawPath emits an arbitrary outline.
//
// Fill and stroke are combined into one painting operator where both apply, which
// is both fewer bytes and correct: filling and stroking as two operations would
// composite the stroke over the fill twice along the shared boundary, showing as
// a darker edge wherever either colour is translucent.
func (c *PDFCanvas) DrawPath(path *core.Path, style core.PathStyle) {
	if path.Empty() || !style.Visible() {
		return
	}
	c.markContent()
	defer c.endContent()

	c.withPathAlpha(style, func() {
		if style.Fills() {
			c.setFillColor(style.Fill)
		}
		if style.Strokes() {
			c.setStrokeColor(style.Stroke)
			c.applyStrokeState(style)
		}

		c.emitPath(path)

		// The starred operators fill by the even-odd rule; the plain ones use
		// nonzero winding, which is the PDF default.
		star := ""
		if style.EvenOdd {
			star = "*"
		}

		switch {
		case style.Fills() && style.Strokes():
			// B fills and strokes in one pass. Filling implicitly closes each
			// subpath, so an unclosed outline still fills sensibly.
			c.op("B%s", star)
		case style.Fills():
			c.op("f%s", star)
		default:
			// A stroke traces the outline, so no fill rule applies.
			c.op("S")
		}
	})
}

// applyStrokeState emits the stroke parameters.
func (c *PDFCanvas) applyStrokeState(style core.PathStyle) {
	c.op("%s w", pdfobj.Num(style.Width))

	if style.Cap != core.CapButt {
		c.op("%d J", int(style.Cap))
	}
	if style.Join != core.JoinMiter {
		c.op("%d j", int(style.Join))
	}
	// The limit only matters for mitred joins, and PDF already defaults to 10.
	if style.Join == core.JoinMiter && style.MiterLimit > 0 &&
		style.MiterLimit != core.DefaultMiterLimit {
		c.op("%s M", pdfobj.Num(style.MiterLimit))
	}

	if style.Dashed() {
		lengths := make([]float64, len(style.Dash))
		copy(lengths, style.Dash)
		c.op("%s %s d", pdfobj.NumArray(lengths...), pdfobj.Num(style.DashPhase))
	}
}

// emitPath writes the path construction operators.
func (c *PDFCanvas) emitPath(path *core.Path) {
	for _, segment := range path.Segments() {
		switch segment.Op {
		case core.PathMoveTo:
			c.op("%s %s m",
				pdfobj.Num(segment.Points[0].X), pdfobj.Num(segment.Points[0].Y))

		case core.PathLineTo:
			c.op("%s %s l",
				pdfobj.Num(segment.Points[0].X), pdfobj.Num(segment.Points[0].Y))

		case core.PathCurveTo:
			c.curve(
				segment.Points[0].X, segment.Points[0].Y,
				segment.Points[1].X, segment.Points[1].Y,
				segment.Points[2].X, segment.Points[2].Y)

		case core.PathClose:
			c.op("h")
		}
	}
}

// withPathAlpha selects a transparency state when either half of the style needs
// one, wrapped in q/Q so it cannot leak into later drawing.
func (c *PDFCanvas) withPathAlpha(style core.PathStyle, draw func()) {
	fillAlpha, strokeAlpha := uint8(255), uint8(255)
	if style.Fills() {
		fillAlpha = style.Fill.Opacity()
	}
	if style.Strokes() {
		strokeAlpha = style.Stroke.Opacity()
	}

	// A stroke's width, cap, join and dash are graphics state too, so the save is
	// needed whenever the path is stroked at all — not only when it is
	// translucent — or a dash pattern would persist into unrelated drawing.
	if fillAlpha == 255 && strokeAlpha == 255 && !style.Strokes() {
		draw()
		return
	}

	c.Save()
	if fillAlpha != 255 || strokeAlpha != 255 {
		c.op("%s gs", pdfobj.Name(c.builder.alphaResource(fillAlpha, strokeAlpha)))
	}
	draw()
	c.Restore()
}

func (c *PDFCanvas) curve(x1, y1, x2, y2, x3, y3 float64) {
	c.op("%s %s %s %s %s %s c",
		pdfobj.Num(x1), pdfobj.Num(y1),
		pdfobj.Num(x2), pdfobj.Num(y2),
		pdfobj.Num(x3), pdfobj.Num(y3))
}

func (c *PDFCanvas) DrawLine(from, to core.Position, stroke core.Color, width float64) {
	if !stroke.Visible() || width <= 0 {
		return
	}
	c.markContent()
	defer c.endContent()

	c.withAlpha(stroke, func() {
		c.setStrokeColor(stroke)
		c.op("%s w", pdfobj.Num(width))
		c.op("%s %s m %s %s l S",
			pdfobj.Num(from.X), pdfobj.Num(from.Y),
			pdfobj.Num(to.X), pdfobj.Num(to.Y))
	})
}

func (c *PDFCanvas) DrawText(text string, pos core.Position, style core.TextStyle) {
	if text == "" || !style.Color.Visible() || style.Size <= 0 {
		return
	}

	usage, err := c.builder.fontResource(style.Font)
	if err != nil {
		c.Fail(err)
		return
	}

	// The operand is built before the operators are emitted, because encoding is
	// what registers the glyphs this font needs — and the font dictionary that
	// declares them is written only once every page has been drawn.
	operand := c.builder.encodeText(usage, text)

	c.markContent()
	defer c.endContent()

	c.withAlpha(style.Color, func() {
		c.setFillColor(style.Color)
		c.op("BT")
		c.op("%s %s Tf", pdfobj.Name(usage.name), pdfobj.Num(style.Size))

		if style.LetterSpacing != 0 {
			c.op("%s Tc", pdfobj.Num(style.LetterSpacing))
		}
		if style.WordSpacing != 0 {
			c.op("%s Tw", pdfobj.Num(style.WordSpacing))
		}

		// The text matrix undoes the page-level Y flip. Composed with the
		// flipping CTM it places the baseline origin at pos while leaving glyphs
		// the right way up.
		c.op("1 0 0 -1 %s %s Tm", pdfobj.Num(pos.X), pdfobj.Num(pos.Y))
		c.op("%s Tj", operand)
		c.op("ET")

		c.drawTextDecorations(text, pos, style)
	})
}

// drawTextDecorations strokes underlines and strikethroughs.
//
// PDF has no decoration property on text, so these are drawn as lines measured
// from the font's own metrics: the thickness scales with size, and the positions
// are placed relative to the baseline and the x-height so they stay visually
// correct across font sizes.
func (c *PDFCanvas) drawTextDecorations(text string, pos core.Position, style core.TextStyle) {
	if !style.Underline && !style.Strikeout {
		return
	}

	width := style.MeasureText(text)
	if width <= 0 {
		return
	}
	thickness := math.Max(0.5, style.Size*0.06)

	if style.Underline {
		// Just below the baseline, clear of most descenders.
		y := pos.Y + style.Font.Descent(style.Size)*0.45
		c.DrawLine(
			core.Position{X: pos.X, Y: y},
			core.Position{X: pos.X + width, Y: y},
			style.Color, thickness)
	}
	if style.Strikeout {
		// Roughly a quarter of the ascent above the baseline puts the line
		// through the middle of lowercase letters.
		y := pos.Y - style.Font.Ascent(style.Size)*0.28
		c.DrawLine(
			core.Position{X: pos.X, Y: y},
			core.Position{X: pos.X + width, Y: y},
			style.Color, thickness)
	}
}

func (c *PDFCanvas) DrawImage(img core.Image, pos core.Position, size core.Size) {
	if len(img.Data) == 0 || size.Width <= 0 || size.Height <= 0 {
		return
	}

	resource, err := c.builder.imageResource(img)
	if err != nil {
		c.Fail(err)
		return
	}
	c.markContent()
	defer c.endContent()

	// An image XObject draws into the unit square, so it is positioned and
	// sized entirely by the transform. The negative Y scale and the offset by
	// the full height together account for the image's own bottom-up origin
	// inside this canvas's top-down space.
	c.Save()
	c.op("%s 0 0 %s %s %s cm",
		pdfobj.Num(size.Width), pdfobj.Num(-size.Height),
		pdfobj.Num(pos.X), pdfobj.Num(pos.Y+size.Height))
	c.op("%s Do", pdfobj.Name(resource))
	c.Restore()
}

// setFillColor selects a non-stroking colour in the colour's own space.
//
// PDF has one operator per space, so the space a colour was specified in decides
// which is emitted — and a CMYK colour reaches the printer as the plates it was
// written as, with no conversion in between.
func (c *PDFCanvas) setFillColor(col core.Color) {
	if col.Space() == core.SpaceCMYK {
		cy, m, y, k := col.CMYKComponents()
		c.op("%s %s %s %s k",
			pdfobj.Num(cy), pdfobj.Num(m), pdfobj.Num(y), pdfobj.Num(k))
		return
	}

	r, g, b := col.RGBComponents()
	c.op("%s %s %s rg", pdfobj.Num(r), pdfobj.Num(g), pdfobj.Num(b))
}

// setStrokeColor is setFillColor for the stroking operators, which PDF spells in
// upper case: RG against rg, K against k.
func (c *PDFCanvas) setStrokeColor(col core.Color) {
	if col.Space() == core.SpaceCMYK {
		cy, m, y, k := col.CMYKComponents()
		c.op("%s %s %s %s K",
			pdfobj.Num(cy), pdfobj.Num(m), pdfobj.Num(y), pdfobj.Num(k))
		return
	}

	r, g, b := col.RGBComponents()
	c.op("%s %s %s RG", pdfobj.Num(r), pdfobj.Num(g), pdfobj.Num(b))
}

// withAlpha runs draw with a transparency state selected when the colour is not
// fully opaque, wrapping it in q/Q so the state does not leak into later
// drawing.
func (c *PDFCanvas) withAlpha(col core.Color, draw func()) {
	if col.Opaque() {
		draw()
		return
	}
	c.Save()
	c.op("%s gs", pdfobj.Name(c.builder.alphaResource(col.Opacity(), col.Opacity())))
	draw()
	c.Restore()
}

// BeginMarked opens a structure element.
//
// The marked-content sequence in the stream is not opened here. Whether this element owns
// any ink is not yet known — a table cell holding a paragraph owns none — and a sequence
// nested inside another leaves a reader unable to tell which of the two the content
// belongs to. So the sequence opens on the first drawing operation that needs it, which
// means the innermost element always owns the ink and nothing ever nests.
func (c *PDFCanvas) BeginMarked(mark core.Mark) {
	if !c.builder.tags.enabled {
		return
	}

	// Content inside a running header is decoration whatever the element thinks it is, so
	// the artifact wins over what is nested in it — except for a link. A conforming
	// document requires every link annotation to sit inside a Link element, so a link in a
	// footer stays a link, and the words it is on are content again rather than decoration.
	if c.builder.tags.insideArtifact() &&
		mark.Role != core.RoleArtifact && mark.Role != core.RoleLink {
		mark = core.Mark{Role: core.RoleArtifact}
	}

	elem := c.builder.tags.push(mark)
	c.marks = append(c.marks, markFrame{elem: elem, artifact: elem == nil})
}

// EndMarked closes the element opened by the matching BeginMarked, and its sequence if one
// was opened.
func (c *PDFCanvas) EndMarked() {
	if !c.builder.tags.enabled || len(c.marks) == 0 {
		return
	}

	frame := c.marks[len(c.marks)-1]
	c.marks = c.marks[:len(c.marks)-1]

	if frame.open {
		c.op("EMC")
	}
	c.builder.tags.pop()
}

// markContent opens a sequence for the innermost element, if one is not open already.
//
// Every drawing operation calls it, which is what makes the ink belong to the element that
// is actually innermost at the moment it is drawn. An ancestor holding an open sequence is
// closed first: sequences may not nest, and the ancestor will open a fresh one if it draws
// again later.
func (c *PDFCanvas) markContent() {
	if !c.builder.tags.enabled {
		return
	}

	// Ink drawn with nothing open would be content a conforming document is not allowed to
	// have: it is neither tagged nor artifact, and software reports it as missing from the
	// structure. Decoration reaches here — a background installed by a decorating method
	// rather than as content — and so would a custom element that declares no role. Naming
	// it an artifact is the safe reading: a reader skips it, rather than the file being
	// invalid.
	if len(c.marks) == 0 {
		c.op("/Artifact BMC")
		c.implicit = true
		return
	}

	top := &c.marks[len(c.marks)-1]
	if top.open {
		return
	}

	for i := range c.marks[:len(c.marks)-1] {
		if c.marks[i].open {
			c.op("EMC")
			c.marks[i].open = false
		}
	}

	if top.artifact {
		// An artifact has no identity to record, so the shorter operator applies.
		c.op("/Artifact BMC")
	} else {
		mcid := c.builder.tags.allocate(top.elem, c.page)
		c.op("%s << /MCID %d >> BDC",
			pdfobj.Name(string(top.elem.mark.Role)), mcid)
	}
	top.open = true
}

// endContent closes a sequence markContent opened implicitly.
//
// Only the implicit one: a sequence belonging to a structure element stays open until that
// element ends, so that a paragraph drawn as several runs is one content item rather than
// one per run.
func (c *PDFCanvas) endContent() {
	if c.implicit {
		c.op("EMC")
		c.implicit = false
	}
}

func (c *PDFCanvas) Fail(err error) {
	if c.err == nil && err != nil {
		c.err = err
	}
}

func (c *PDFCanvas) Err() error { return c.err }

// Close finishes the page and appends it to the document.
func (c *PDFCanvas) Close() error {
	if c.closed {
		return c.err
	}
	c.closed = true

	if c.err != nil {
		return c.err
	}
	if c.depth != 0 {
		return fmt.Errorf(
			"sanur/render: page finished with %d unbalanced Save call(s); "+
				"an element drew without restoring the canvas state", c.depth)
	}

	c.builder.addPage(c.size, c.buf.Bytes())
	return nil
}

// Size returns the page dimensions.
func (c *PDFCanvas) Size() core.Size { return c.size }

var _ core.Canvas = (*PDFCanvas)(nil)

// markFrame is one open structure element on the canvas's stack.
type markFrame struct {
	// elem is the structure element, or nil for an artifact.
	elem *structElem

	// artifact distinguishes a nil elem meaning "artifact" from a nil elem meaning
	// nothing, since an artifact still opens a sequence in the stream.
	artifact bool

	// open records whether a sequence is currently open for this frame, so EndMarked
	// knows whether to close one and markContent knows whether to open one.
	open bool
}
