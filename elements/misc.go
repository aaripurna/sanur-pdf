package elements

import (
	"math"
	"strconv"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
)

// PageBreak forces everything after it onto a new page.
//
// It works by reporting a partial render of zero size: the enclosing column sees
// an item with content still outstanding, ends the page, and resumes at the page
// break on the next one — where the break has been marked done and reports a full
// render so the column carries on past it.
type PageBreak struct {
	consumed bool
}

func (p *PageBreak) Measure(core.Size) core.SpacePlan {
	if p.consumed {
		return core.EmptyRender()
	}
	return core.PartialRender(core.Size{})
}

// Draw records that the break has been honoured. The state change belongs here
// rather than in Measure because Measure is called speculatively and must stay
// repeatable.
func (p *PageBreak) Draw(core.Canvas, core.Size) {
	p.consumed = true
}

func (p *PageBreak) ResetState(hard bool) {
	if hard {
		p.consumed = false
	}
}

// Spacer occupies fixed empty space. A zero size makes it a no-op, which is
// occasionally useful as a placeholder.
type Spacer struct {
	Width  float64
	Height float64
}

func (s *Spacer) Measure(available core.Size) core.SpacePlan {
	size := core.Size{Width: s.Width, Height: s.Height}
	if !size.FitsWithin(available) {
		return core.Wrap("spacer of %.1fx%.1f exceeds the available %.1fx%.1f",
			size.Width, size.Height, available.Width, available.Height)
	}
	return core.FullRender(size)
}

func (s *Spacer) Draw(core.Canvas, core.Size) {}

// Line draws a rule across the box it is given.
//
// It reports only its own thickness on the axis it spans, and takes the full
// extent on the other, so a horizontal rule in a column is as wide as the column
// and as tall as its width setting.
type Line struct {
	Vertical bool
	Width    float64
	Color    core.Color

	// Dash alternates on and off lengths in points; nil draws a solid rule. A
	// single value means equal dashes and gaps.
	Dash []float64

	// Cap finishes the ends of each dash. Round caps turn a short-on pattern into
	// a row of dots, which is the usual intent behind a dotted rule.
	Cap core.LineCap
}

func (l *Line) Measure(available core.Size) core.SpacePlan {
	if l.Vertical {
		if l.Width > available.Width+core.Epsilon {
			return core.Wrap("vertical line of width %.1f exceeds the available %.1f",
				l.Width, available.Width)
		}
		return core.FullRender(core.Size{Width: l.Width, Height: available.Height})
	}
	if l.Width > available.Height+core.Epsilon {
		return core.Wrap("horizontal line of width %.1f exceeds the available height %.1f",
			l.Width, available.Height)
	}
	return core.FullRender(core.Size{Width: available.Width, Height: l.Width})
}

func (l *Line) Draw(canvas core.Canvas, available core.Size) {
	if l.Width <= 0 || !l.Color.Visible() {
		return
	}
	// The line is centred on its own thickness so it sits inside the box it
	// reported rather than straddling the edge.
	half := l.Width / 2

	from := core.Position{X: 0, Y: half}
	to := core.Position{X: available.Width, Y: half}
	if l.Vertical {
		from = core.Position{X: half, Y: 0}
		to = core.Position{X: half, Y: available.Height}
	}

	style := core.PathStyle{Stroke: l.Color, Width: l.Width, Dash: l.Dash, Cap: l.Cap}
	if !style.Dashed() && l.Cap == core.CapButt {
		// A plain solid rule goes through the dedicated line call, which emits the
		// whole stroke as a single operator instead of a path plus a paint.
		canvas.DrawLine(from, to, l.Color, l.Width)
		return
	}
	canvas.DrawPath(core.Polyline(from, to), style)
}

// ImageFit selects how an image is scaled into its box.
type ImageFit int

const (
	// FitWidth scales to the available width and derives the height from the
	// aspect ratio.
	FitWidth ImageFit = iota

	// FitArea scales to fit entirely inside the box, preserving aspect ratio and
	// leaving empty space on one axis.
	FitArea

	// FitStretch fills the box exactly, distorting the image if it has to.
	FitStretch

	// FitUnscaled draws the image at its pixel size treated as points.
	FitUnscaled
)

// Image draws a raster image.
type Image struct {
	Source core.Image
	Fit    ImageFit
}

// resolveSize computes the drawn size for the available box.
func (i *Image) resolveSize(available core.Size) core.Size {
	ratio := i.Source.AspectRatio()
	if ratio <= 0 {
		return core.Size{}
	}

	switch i.Fit {
	case FitStretch:
		return available

	case FitUnscaled:
		return core.Size{
			Width:  float64(i.Source.PixelWidth),
			Height: float64(i.Source.PixelHeight),
		}

	case FitArea:
		// Take the more restrictive of the two axes so the whole image lands
		// inside the box.
		scale := math.Min(
			available.Width/float64(i.Source.PixelWidth),
			available.Height/float64(i.Source.PixelHeight),
		)
		return core.Size{
			Width:  float64(i.Source.PixelWidth) * scale,
			Height: float64(i.Source.PixelHeight) * scale,
		}

	default: // FitWidth
		return core.Size{Width: available.Width, Height: available.Width / ratio}
	}
}

func (i *Image) Measure(available core.Size) core.SpacePlan {
	if len(i.Source.Data) == 0 {
		return core.EmptyRender()
	}

	size := i.resolveSize(available)
	if size.IsEmpty() {
		return core.EmptyRender()
	}
	if !size.FitsWithin(available) {
		// An image is atomic: there is no way to render the top half now and the
		// bottom half on the next page, so it defers whole.
		return core.Wrap("image %q needs %.1fx%.1f but only %.1fx%.1f is available",
			i.Source.Key, size.Width, size.Height, available.Width, available.Height)
	}
	return core.FullRender(size)
}

func (i *Image) Draw(canvas core.Canvas, available core.Size) {
	if len(i.Source.Data) == 0 {
		return
	}
	size := i.resolveSize(available)
	if size.IsEmpty() {
		return
	}
	canvas.DrawImage(i.Source, core.Position{}, size)
}

// PageNumber renders text derived from the page it lands on.
//
// The format string is expanded with the current page and, where known, the
// total. Totals are only available on the second generation pass; on the first
// the placeholder expands to "?" so the text still measures to a believable width
// and the page count it feeds into stays stable.
type PageNumber struct {
	// Format may contain {page} and {total}.
	Format string
	Style  core.TextStyle
	Align  core.HorizontalAlign

	ctx  core.PageContext
	text *Text
}

// NewPageNumber builds a page-number label.
func NewPageNumber(format string, style core.TextStyle) *PageNumber {
	return &PageNumber{Format: format, Style: style}
}

func (p *PageNumber) SetPageContext(ctx core.PageContext) {
	p.ctx = ctx
	// The rendered string changes, so the cached line layout is no longer valid.
	p.text = nil
}

func (p *PageNumber) resolve() *Text {
	if p.text != nil {
		return p.text
	}

	total := "?"
	if p.ctx.TotalPages > 0 {
		total = strconv.Itoa(p.ctx.TotalPages)
	}

	s := strings.ReplaceAll(p.Format, "{page}", strconv.Itoa(p.ctx.PageNumber))
	s = strings.ReplaceAll(s, "{total}", total)

	p.text = NewText(s, p.Style)
	p.text.Align = p.Align
	return p.text
}

func (p *PageNumber) Measure(available core.Size) core.SpacePlan {
	return p.resolve().Measure(available)
}

func (p *PageNumber) Draw(canvas core.Canvas, available core.Size) {
	p.resolve().Draw(canvas, available)
}

func (p *PageNumber) ResetState(hard bool) {
	if hard {
		p.text = nil
	}
}

var (
	_ core.Element         = (*PageBreak)(nil)
	_ core.StateResettable = (*PageBreak)(nil)
	_ core.Element         = (*Spacer)(nil)
	_ core.Element         = (*Line)(nil)
	_ core.Element         = (*Image)(nil)
	_ core.Element         = (*PageNumber)(nil)
	_ core.ContextAware    = (*PageNumber)(nil)
)

// PathShape draws a prepared path.
//
// It claims the space it is offered rather than measuring its own extent. A path
// has a bounding box, but using it would make the element's size depend on where
// its points happen to sit, so a shape centred at (100, 100) in a small box would
// silently resize its parent. Taking the offered space instead keeps the caller in
// control: constrain the box, then draw within it.
type PathShape struct {
	Path  *core.Path
	Style core.PathStyle
}

func (p *PathShape) Measure(available core.Size) core.SpacePlan {
	if p.Path.Empty() || !p.Style.Visible() {
		return core.EmptyRender()
	}
	return core.FullRender(available)
}

func (p *PathShape) Draw(canvas core.Canvas, available core.Size) {
	if p.Path.Empty() || !p.Style.Visible() || available.IsEmpty() {
		return
	}
	canvas.DrawPath(p.Path, p.Style)
}

var _ core.Element = (*PathShape)(nil)
