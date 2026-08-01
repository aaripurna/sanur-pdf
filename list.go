package sanur

import (
	"fmt"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

// List places items with a marker in a gutter and the body beside it.
//
//	c.Item().List(func(l *sanur.ListBuilder) {
//		l.Numbered()
//		l.Item().Text("Measure the space available.")
//		l.Item().Text("Draw into whatever the parent allotted.")
//	})
//
// A list built this way paginates like any other column, and in a tagged document it
// carries the structure a reader needs: the list, each item, and within each item the
// marker and the body as separate things. That distinction is the reason this exists
// rather than leaving callers to compose a column of rows — the markers are drawn as
// ordinary text, so without it nothing says whether "1." is a list marker or the start of
// a sentence, and a reader announces the digits instead of "item one of five".
func (c *Container) List(build func(*ListBuilder)) {
	column := &elements.Column{Spacing: defaultListSpacing}

	b := &ListBuilder{
		column:      column,
		style:       c.style,
		numbering:   core.NumberingDisc,
		marker:      discMarker,
		gutter:      defaultListGutter,
		markerSpace: defaultMarkerSpace,
	}
	build(b)

	// The mark is applied after the build so that Numbered and its relatives can be
	// called in any order relative to the items.
	c.install(&elements.Tagged{
		Mark:  core.Mark{Role: core.RoleList, Numbering: b.numbering},
		Child: column,
	})
}

// Defaults chosen to look right at ordinary body sizes rather than to be significant:
// a gutter wide enough for two digits and a full stop, and a gap that separates the
// marker from the text without detaching it.
const (
	defaultListSpacing = 4
	defaultListGutter  = 16
	defaultMarkerSpace = 6
)

// discMarker is the unordered marker. U+2022 is in WinAnsi, so it needs no registered
// font — unlike the arrows and geometric shapes, which do.
func discMarker(int) string { return "•" }

// ListBuilder collects the items of a list.
type ListBuilder struct {
	column *elements.Column
	style  core.TextStyle

	numbering core.Numbering
	marker    func(index int) string

	gutter      float64
	markerSpace float64

	// markerStyle overrides the item style for the markers alone, for a list whose
	// bullets are a different colour from its text.
	markerStyle *core.TextStyle
}

// Bulleted labels items with a disc, which is the default.
func (b *ListBuilder) Bulleted() *ListBuilder {
	b.numbering = core.NumberingDisc
	b.marker = discMarker
	return b
}

// Numbered labels items 1., 2., 3.
func (b *ListBuilder) Numbered() *ListBuilder {
	b.numbering = core.NumberingDecimal
	b.marker = func(i int) string { return fmt.Sprintf("%d.", i+1) }
	return b
}

// Lettered labels items a., b., c., continuing aa., ab. past the twenty-sixth.
func (b *ListBuilder) Lettered() *ListBuilder {
	b.numbering = core.NumberingLowerAlpha
	b.marker = func(i int) string { return letterMarker(i) + "." }
	return b
}

// Unmarked leaves the gutter empty, for a list whose items are distinguished some other
// way. It is still a list: the structure says so even though nothing is drawn.
func (b *ListBuilder) Unmarked() *ListBuilder {
	b.numbering = core.NumberingNone
	b.marker = func(int) string { return "" }
	return b
}

// Marker sets the label for each item from its zero-based index, for a scheme none of the
// helpers covers.
//
// numbering tells a reader what the scheme is. Pass core.NumberingNone when it is
// something a reader has no name for, rather than claiming a scheme the labels do not
// follow.
func (b *ListBuilder) Marker(numbering core.Numbering, marker func(index int) string) *ListBuilder {
	b.numbering = numbering
	b.marker = marker
	return b
}

// Spacing sets the gap between items.
func (b *ListBuilder) Spacing(v float64) *ListBuilder {
	b.column.Spacing = v
	return b
}

// Gutter sets the width reserved for markers. Markers are set flush right within it, so
// that "9." and "10." line up on the full stop.
func (b *ListBuilder) Gutter(v float64) *ListBuilder {
	b.gutter = v
	return b
}

// MarkerSpace sets the gap between the gutter and the body.
func (b *ListBuilder) MarkerSpace(v float64) *ListBuilder {
	b.markerSpace = v
	return b
}

// MarkerStyle styles the markers independently of the item bodies.
//
// A marker is text, so its line height is what places its baseline. Give it the same line
// height as the bodies, or the markers sit above the lines they belong to.
func (b *ListBuilder) MarkerStyle(style *StyleBuilder) *ListBuilder {
	built := style.Build()
	b.markerStyle = &built
	return b
}

// Item appends a list item and returns the slot for its body.
//
// The marker is generated, so the caller supplies only the content:
//
//	l.Item().Text("Ordinary prose.")
//	l.Item().List(func(inner *sanur.ListBuilder) { ... })  // nested
func (b *ListBuilder) Item() *Container {
	index := len(b.column.Items)

	// Appended immediately so that ordering follows the order of the calls, and filled in
	// when the caller's chain resolves.
	b.column.Items = append(b.column.Items, nil)

	row := &elements.Row{Spacing: b.markerSpace}

	markerStyle := b.style
	if b.markerStyle != nil {
		markerStyle = *b.markerStyle
	}

	// The marker is a label rather than a paragraph, and is right-aligned in the gutter so
	// that the numbers of a long list line up on their full stops instead of their digits.
	label := elements.NewText(b.marker(index), markerStyle)
	label.Align = core.AlignRight

	row.Items = append(row.Items, elements.RowItem{
		Sizing: elements.RowConstant,
		Size:   b.gutter,
		Element: &elements.Tagged{
			Mark:  core.Mark{Role: core.RoleListLabel},
			Child: label,
		},
	})

	body := &elements.Container{}
	row.Items = append(row.Items, elements.RowItem{
		Sizing: elements.RowRelative,
		Size:   1,
		Element: &elements.Tagged{
			Mark:  core.Mark{Role: core.RoleListBody},
			Child: body,
		},
	})

	b.column.Items[index] = &elements.Tagged{
		Mark:  core.Mark{Role: core.RoleListItem},
		Child: row,
	}

	return newContainer(body.Set, b.style)
}

// Items is shorthand for a list of plain paragraphs.
func (b *ListBuilder) Items(values ...string) *ListBuilder {
	for _, v := range values {
		b.Item().Text(v)
	}
	return b
}

// letterMarker renders an index as a, b, ... z, aa, ab, in the spreadsheet-column scheme
// rather than as base 26.
//
// The difference shows at the twenty-seventh item: this gives aa, where a plain base-26
// conversion gives ba. Lists rarely run that long, but the one that does should not look
// like a bug.
func letterMarker(index int) string {
	var b strings.Builder

	for index >= 0 {
		b.WriteByte(byte('a' + index%26))
		index = index/26 - 1
	}

	// Built least-significant first, so it comes out backwards.
	runes := []rune(b.String())
	for i, j := 0, len(runes)-1; i < j; i, j = i+1, j-1 {
		runes[i], runes[j] = runes[j], runes[i]
	}
	return string(runes)
}
