package elements

import (
	"math"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
)

// Span is a run of text sharing one style. A Text element holds several so that
// a bold word or a coloured figure can sit inside an ordinary sentence without
// breaking the line-breaking across the whole paragraph.
type Span struct {
	Text  string
	Style core.TextStyle
}

// Text lays out styled spans as wrapped lines.
//
// Line breaking is greedy and runs across span boundaries, so styling has no
// effect on where lines break. Lines are the unit of pagination: a paragraph too
// long for the page renders as many lines as fit and resumes at the next line on
// the following page.
type Text struct {
	Spans []Span
	Align core.HorizontalAlign

	// lines is the cached result of breaking the spans, valid only for
	// layoutWidth. Re-breaking on every Measure would be wasteful given the
	// engine measures speculatively and often repeatedly at the same width.
	lines       []textLine
	layoutWidth float64
	laidOut     bool

	// rendered is the index of the first line not yet drawn, the element's
	// pagination state.
	rendered int
}

// NewText builds a single-style text element.
func NewText(content string, style core.TextStyle) *Text {
	return &Text{Spans: []Span{{Text: content, Style: style}}}
}

// AddSpan appends a styled run.
func (t *Text) AddSpan(content string, style core.TextStyle) {
	t.Spans = append(t.Spans, Span{Text: content, Style: style})
	t.laidOut = false
}

// textSegment is a piece of one line drawn with a single style.
type textSegment struct {
	text  string
	style core.TextStyle
	width float64
}

// textLine is one laid-out line.
type textLine struct {
	segments []textSegment

	// width is the ink width, excluding any trailing space that was dropped at
	// the break.
	width float64

	// spaces counts the space characters available to stretch when justifying.
	spaces int

	ascent  float64
	descent float64
	height  float64
}

type atomKind int

const (
	atomWord atomKind = iota
	atomSpace
	atomBreak
)

// atom is the smallest unit line breaking moves around.
type atom struct {
	kind  atomKind
	text  string
	style core.TextStyle
	width float64
}

// tokenize flattens the spans into atoms.
//
// Spaces become their own atoms rather than staying attached to words, because a
// line break has to be able to discard the space it broke at: keeping it would
// leave a ragged extra gap at the end of every wrapped line and throw off
// centred and right-aligned text by that width.
func (t *Text) tokenize() []atom {
	var atoms []atom

	for _, span := range t.Spans {
		if span.Style.Font == nil || span.Style.Size <= 0 {
			continue
		}

		// Normalise line endings so a break is always a single \n atom.
		text := strings.ReplaceAll(span.Text, "\r\n", "\n")
		text = strings.ReplaceAll(text, "\r", "\n")

		var current strings.Builder
		flush := func(kind atomKind) {
			if current.Len() == 0 {
				return
			}
			s := current.String()
			atoms = append(atoms, atom{
				kind:  kind,
				text:  s,
				style: span.Style,
				width: span.Style.MeasureText(s),
			})
			current.Reset()
		}

		for _, r := range text {
			switch {
			case r == '\n':
				flush(atomWord)
				atoms = append(atoms, atom{kind: atomBreak, style: span.Style})
			case r == ' ' || r == '\t':
				flush(atomWord)
				// Tabs are treated as single spaces; PDF has no tab stops, and
				// aligning columns is what Row is for.
				current.WriteByte(' ')
				flush(atomSpace)
			default:
				current.WriteRune(r)
			}
		}
		flush(atomWord)
	}

	return atoms
}

// layout breaks the spans into lines that fit maxWidth.
func (t *Text) layout(maxWidth float64) {
	if t.laidOut && math.Abs(t.layoutWidth-maxWidth) < core.Epsilon {
		return
	}
	t.laidOut = true
	t.layoutWidth = maxWidth
	t.lines = nil

	var (
		current  []textSegment
		width    float64
		spaces   int
		pending  []atom // spaces held back until the next word commits them
		pendingW float64
	)

	commit := func() {
		t.lines = append(t.lines, buildLine(current, width, spaces))
		current = nil
		width = 0
		spaces = 0
		pending = nil
		pendingW = 0
	}

	push := func(a atom) {
		current = append(current, textSegment{text: a.text, style: a.style, width: a.width})
		width += a.width
		if a.kind == atomSpace {
			spaces += strings.Count(a.text, " ")
		}
	}

	for _, a := range t.tokenize() {
		switch a.kind {
		case atomBreak:
			// An explicit break ends the line even when nothing was placed on
			// it. Such a line still needs the height of the style that produced
			// it, so an empty segment carries those metrics through — otherwise a
			// blank line between paragraphs would occupy no space at all and the
			// paragraphs would run together.
			if len(current) == 0 {
				current = append(current, textSegment{style: a.style})
			}
			commit()

		case atomSpace:
			// Held back: if the next word wraps, this space disappears with the
			// break instead of dangling at the end of the line.
			if len(current) > 0 {
				pending = append(pending, a)
				pendingW += a.width
			}

		case atomWord:
			fits := width+pendingW+a.width <= maxWidth+core.Epsilon

			if !fits && len(current) > 0 {
				commit()
			}

			// A single word wider than the line has to be broken mid-word;
			// otherwise it would overflow forever and no width would ever fit.
			if a.width > maxWidth+core.Epsilon {
				for _, chunk := range splitAtom(a, maxWidth) {
					if width > 0 {
						commit()
					}
					push(chunk)
				}
				continue
			}

			for _, p := range pending {
				push(p)
			}
			pending = nil
			pendingW = 0
			push(a)
		}
	}

	if len(current) > 0 {
		commit()
	}
}

// buildLine merges adjacent same-style segments and computes vertical metrics.
func buildLine(segments []textSegment, width float64, spaces int) textLine {
	line := textLine{width: width, spaces: spaces}

	for _, seg := range segments {
		n := len(line.segments)
		// Merging runs that share a style keeps the content stream compact: a
		// sentence becomes one Tj rather than one per word.
		if n > 0 && sameStyle(line.segments[n-1].style, seg.style) {
			line.segments[n-1].text += seg.text
			line.segments[n-1].width += seg.width
		} else {
			line.segments = append(line.segments, seg)
		}

		line.ascent = math.Max(line.ascent, seg.style.Font.Ascent(seg.style.Size))
		line.descent = math.Max(line.descent, seg.style.Font.Descent(seg.style.Size))
		line.height = math.Max(line.height, seg.style.LineSpacing())
	}

	return line
}

// sameStyle reports whether two styles produce identical output, so their runs
// can be merged.
func sameStyle(a, b core.TextStyle) bool {
	return a.Font == b.Font &&
		a.Size == b.Size &&
		a.Color == b.Color &&
		a.LetterSpacing == b.LetterSpacing &&
		a.WordSpacing == b.WordSpacing &&
		a.Underline == b.Underline &&
		a.Strikeout == b.Strikeout
}

// splitAtom breaks an over-long word into chunks that each fit maxWidth.
func splitAtom(a atom, maxWidth float64) []atom {
	runes := []rune(a.text)
	var out []atom

	start := 0
	for start < len(runes) {
		width := 0.0
		end := start

		for end < len(runes) {
			w := a.style.Font.AdvanceOf(runes[end], a.style.Size) + a.style.LetterSpacing
			if end > start && width+w > maxWidth+core.Epsilon {
				break
			}
			width += w
			end++
		}

		// Always take at least one rune: a glyph wider than the whole line would
		// otherwise make no progress and loop forever.
		if end == start {
			end = start + 1
			width = a.style.Font.AdvanceOf(runes[start], a.style.Size)
		}

		text := string(runes[start:end])
		out = append(out, atom{
			kind:  atomWord,
			text:  text,
			style: a.style,
			width: a.style.MeasureText(text),
		})
		start = end
	}

	return out
}

// visibleLines returns how many of the remaining lines fit in height, and the
// total they occupy. Measure and Draw both call it so they cannot disagree.
func (t *Text) visibleLines(height float64) (count int, used float64) {
	for i := t.rendered; i < len(t.lines); i++ {
		lh := t.lines[i].height
		if used+lh > height+core.Epsilon {
			break
		}
		used += lh
		count++
	}
	return count, used
}

func (t *Text) Measure(available core.Size) core.SpacePlan {
	t.layout(available.Width)

	if t.rendered >= len(t.lines) {
		return core.EmptyRender()
	}

	count, used := t.visibleLines(available.Height)
	if count == 0 {
		return core.Wrap("not enough height for a line of text (%.1f available, %.1f needed)",
			available.Height, t.lines[t.rendered].height)
	}

	width := 0.0
	for i := t.rendered; i < t.rendered+count; i++ {
		width = math.Max(width, t.lines[i].width)
	}

	// Justified text is stretched to the full column, so it reports that width
	// rather than its natural one. A block of a single line is left alone, since
	// the last line of justified text is never stretched.
	if t.Align == core.AlignJustify && len(t.lines) > 1 {
		width = available.Width
	}

	size := core.Size{Width: width, Height: used}
	if t.rendered+count < len(t.lines) {
		return core.PartialRender(size)
	}
	return core.FullRender(size)
}

func (t *Text) Draw(canvas core.Canvas, available core.Size) {
	t.layout(available.Width)

	if t.rendered >= len(t.lines) {
		return
	}

	count, _ := t.visibleLines(available.Height)
	if count == 0 {
		return
	}

	last := t.rendered + count
	var y float64

	for i := t.rendered; i < last; i++ {
		line := t.lines[i]

		// Centre the ink within the line box so that the extra leading is split
		// above and below rather than all falling below the text.
		leading := line.height - (line.ascent + line.descent)
		baseline := y + leading/2 + line.ascent

		x := t.Align.OffsetX(available.Width, line.width)
		justify := 0.0

		if t.Align == core.AlignJustify {
			// The final line of a justified block stays flush left; stretching it
			// is what produces the notorious rivers of whitespace.
			isLastOfParagraph := i == len(t.lines)-1
			if !isLastOfParagraph && line.spaces > 0 {
				justify = (available.Width - line.width) / float64(line.spaces)
			}
			x = 0
		}

		for _, seg := range line.segments {
			style := seg.style
			style.WordSpacing += justify

			canvas.DrawText(seg.text, core.Position{X: x, Y: baseline}, style)
			x += style.MeasureText(seg.text)
		}

		y += line.height
	}

	t.rendered = last
}

func (t *Text) ResetState(hard bool) {
	if hard {
		t.rendered = 0
	}
}

var (
	_ core.Element         = (*Text)(nil)
	_ core.StateResettable = (*Text)(nil)
)
