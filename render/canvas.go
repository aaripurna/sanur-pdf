package render

import (
	"bytes"
	"fmt"
	"math"

	"codeberg.org/aaripurna/sanur/core"
	"codeberg.org/aaripurna/sanur/fonts"
	"codeberg.org/aaripurna/sanur/internal/pdfobj"
)

// kappa is the control-point ratio that makes a cubic Bézier approximate a
// quarter circle. PDF has no arc operator, so every rounded corner is drawn as a
// curve, and 0.5523 is the constant that minimises the error of that fit.
const kappa = 0.55228475

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

	// depth tracks unbalanced Save calls so Close can detect a container that
	// forgot to Restore, which would otherwise corrupt the graphics stack
	// silently and misplace everything after it.
	depth int

	err error

	closed bool
}

func newPDFCanvas(b *Builder, size core.Size) *PDFCanvas {
	c := &PDFCanvas{builder: b, size: size}

	// Flip the Y axis so layout coordinates can be emitted verbatim. After this
	// transform, (0,0) is the top-left corner and positive Y runs down the page.
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
	c.op("q")
}

func (c *PDFCanvas) Restore() {
	if c.depth == 0 {
		c.Fail(fmt.Errorf("sanur/render: Restore called without a matching Save"))
		return
	}
	c.depth--
	c.op("Q")
}

func (c *PDFCanvas) Translate(p core.Position) {
	if p.X == 0 && p.Y == 0 {
		return
	}
	c.op("1 0 0 1 %s %s cm", pdfobj.Num(p.X), pdfobj.Num(p.Y))
}

func (c *PDFCanvas) Rotate(degrees float64) {
	if math.Mod(degrees, 360) == 0 {
		return
	}
	rad := degrees * math.Pi / 180
	cos, sin := math.Cos(rad), math.Sin(rad)

	// In the Y-down space this canvas works in, the standard rotation matrix
	// turns clockwise on screen, which is what callers asking for a positive
	// rotation expect.
	c.op("%s %s %s %s 0 0 cm",
		pdfobj.Num(cos), pdfobj.Num(sin), pdfobj.Num(-sin), pdfobj.Num(cos))
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
	c.withAlpha(fill, func() {
		c.setFillColor(fill)
		c.op("%s %s %s %s re f",
			pdfobj.Num(pos.X), pdfobj.Num(pos.Y),
			pdfobj.Num(size.Width), pdfobj.Num(size.Height))
	})
}

func (c *PDFCanvas) DrawRoundedRect(pos core.Position, size core.Size, radius float64, fill core.Color) {
	if !fill.Visible() || size.Width <= 0 || size.Height <= 0 {
		return
	}
	// A radius larger than half the shorter side would make opposite corners
	// overlap and self-intersect, so it is clamped to the largest value that
	// still produces a valid outline (a stadium, or a circle for a square).
	radius = math.Min(radius, math.Min(size.Width, size.Height)/2)
	if radius <= 0 {
		c.DrawRect(pos, size, fill)
		return
	}

	x, y := pos.X, pos.Y
	w, h := size.Width, size.Height
	ctl := radius * kappa

	c.withAlpha(fill, func() {
		c.setFillColor(fill)
		c.op("%s %s m", pdfobj.Num(x+radius), pdfobj.Num(y))
		c.op("%s %s l", pdfobj.Num(x+w-radius), pdfobj.Num(y))
		c.curve(x+w-radius+ctl, y, x+w, y+radius-ctl, x+w, y+radius)
		c.op("%s %s l", pdfobj.Num(x+w), pdfobj.Num(y+h-radius))
		c.curve(x+w, y+h-radius+ctl, x+w-radius+ctl, y+h, x+w-radius, y+h)
		c.op("%s %s l", pdfobj.Num(x+radius), pdfobj.Num(y+h))
		c.curve(x+radius-ctl, y+h, x, y+h-radius+ctl, x, y+h-radius)
		c.op("%s %s l", pdfobj.Num(x), pdfobj.Num(y+radius))
		c.curve(x, y+radius-ctl, x+radius-ctl, y, x+radius, y)
		c.op("h f")
	})
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
	c.withAlpha(stroke, func() {
		r, g, b := stroke.Components()
		c.op("%s %s %s RG", pdfobj.Num(r), pdfobj.Num(g), pdfobj.Num(b))
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

	resource, err := c.builder.fontResource(style.Font)
	if err != nil {
		c.Fail(err)
		return
	}

	c.withAlpha(style.Color, func() {
		c.setFillColor(style.Color)
		c.op("BT")
		c.op("%s %s Tf", pdfobj.Name(resource), pdfobj.Num(style.Size))

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
		c.op("%s Tj", pdfobj.StringBytes(fonts.EncodeWinAnsi(text)))
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

func (c *PDFCanvas) setFillColor(col core.Color) {
	r, g, b := col.Components()
	c.op("%s %s %s rg", pdfobj.Num(r), pdfobj.Num(g), pdfobj.Num(b))
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
	c.op("%s gs", pdfobj.Name(c.builder.alphaResource(col.A)))
	draw()
	c.Restore()
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
