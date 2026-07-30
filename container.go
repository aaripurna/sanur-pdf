package sanur

import (
	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/elements"
)

// Container is the fluent building surface.
//
// Every container is an empty slot plus the styling context inherited from its
// ancestors. A decorating method — Padding, Background, Border — installs an
// element into this slot and returns a new container pointing at that element's
// own slot, so chained calls nest rather than overwrite. Terminal methods like
// Text install a leaf and return nothing to chain.
//
// Go has no extension methods, so unlike QuestPDF's split across many static
// classes, every method lives on this one type. The upside is that a caller sees
// the complete vocabulary through one value's method set.
type Container struct {
	// install places an element into the parent's slot.
	install func(core.Element)

	// style is the inherited text style, applied by Text and its relatives.
	style core.TextStyle
}

func newContainer(install func(core.Element), style core.TextStyle) *Container {
	return &Container{install: install, style: style}
}

// wrap installs a decorating element and returns the container for its child.
func (c *Container) wrap(element core.Element, slot func(core.Element)) *Container {
	c.install(element)
	return newContainer(slot, c.style)
}

// Element installs an arbitrary element, the escape hatch for custom layout
// primitives that implement core.Element.
func (c *Container) Element(e core.Element) { c.install(e) }

// DefaultTextStyle overrides the inherited style for this subtree.
func (c *Container) DefaultTextStyle(style *StyleBuilder) *Container {
	return &Container{install: c.install, style: style.Build()}
}

// --- Spacing ---------------------------------------------------------------

// Padding insets the child on all four edges.
func (c *Container) Padding(value float64) *Container {
	p := &elements.Padding{Top: value, Right: value, Bottom: value, Left: value}
	return c.wrap(p, func(e core.Element) { p.Child = e })
}

// PaddingXY insets the child horizontally and vertically.
func (c *Container) PaddingXY(x, y float64) *Container {
	p := &elements.Padding{Top: y, Bottom: y, Left: x, Right: x}
	return c.wrap(p, func(e core.Element) { p.Child = e })
}

// PaddingEach insets each edge independently.
func (c *Container) PaddingEach(top, right, bottom, left float64) *Container {
	p := &elements.Padding{Top: top, Right: right, Bottom: bottom, Left: left}
	return c.wrap(p, func(e core.Element) { p.Child = e })
}

// PaddingTop, PaddingRight, PaddingBottom and PaddingLeft inset a single edge.
func (c *Container) PaddingTop(v float64) *Container    { return c.PaddingEach(v, 0, 0, 0) }
func (c *Container) PaddingRight(v float64) *Container  { return c.PaddingEach(0, v, 0, 0) }
func (c *Container) PaddingBottom(v float64) *Container { return c.PaddingEach(0, 0, v, 0) }
func (c *Container) PaddingLeft(v float64) *Container   { return c.PaddingEach(0, 0, 0, v) }

// --- Sizing ----------------------------------------------------------------

// Width fixes the child's width.
func (c *Container) Width(v float64) *Container {
	k := &elements.Constrained{MinWidth: v, MaxWidth: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

// Height fixes the child's height.
func (c *Container) Height(v float64) *Container {
	k := &elements.Constrained{MinHeight: v, MaxHeight: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

// Size fixes both dimensions.
func (c *Container) Size(w, h float64) *Container {
	k := &elements.Constrained{MinWidth: w, MaxWidth: w, MinHeight: h, MaxHeight: h}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

// MinWidth, MaxWidth, MinHeight and MaxHeight bound one dimension, leaving the
// other and the opposite bound free.
func (c *Container) MinWidth(v float64) *Container {
	k := &elements.Constrained{MinWidth: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

func (c *Container) MaxWidth(v float64) *Container {
	k := &elements.Constrained{MaxWidth: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

func (c *Container) MinHeight(v float64) *Container {
	k := &elements.Constrained{MinHeight: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

func (c *Container) MaxHeight(v float64) *Container {
	k := &elements.Constrained{MaxHeight: v}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

// Extend claims all the space available on both axes.
func (c *Container) Extend() *Container {
	e := &elements.Extend{Horizontal: true, Vertical: true}
	return c.wrap(e, func(child core.Element) { e.Child = child })
}

// ExtendHorizontal claims the full available width, the usual way to make a
// background or border span its parent.
func (c *Container) ExtendHorizontal() *Container {
	e := &elements.Extend{Horizontal: true}
	return c.wrap(e, func(child core.Element) { e.Child = child })
}

// ExtendVertical claims the full available height.
func (c *Container) ExtendVertical() *Container {
	e := &elements.Extend{Vertical: true}
	return c.wrap(e, func(child core.Element) { e.Child = child })
}

// --- Alignment -------------------------------------------------------------

// AlignLeft, AlignCenter and AlignRight position a narrower child horizontally.
func (c *Container) AlignLeft() *Container   { return c.aligned(core.AlignLeft, -1) }
func (c *Container) AlignCenter() *Container { return c.aligned(core.AlignCenter, -1) }
func (c *Container) AlignRight() *Container  { return c.aligned(core.AlignRight, -1) }

// AlignTop, AlignMiddle and AlignBottom position a shorter child vertically.
func (c *Container) AlignTop() *Container    { return c.aligned(-1, core.AlignTop) }
func (c *Container) AlignMiddle() *Container { return c.aligned(-1, core.AlignMiddle) }
func (c *Container) AlignBottom() *Container { return c.aligned(-1, core.AlignBottom) }

// aligned builds an Aligned element, where -1 means "leave this axis alone".
func (c *Container) aligned(h core.HorizontalAlign, v core.VerticalAlign) *Container {
	a := &elements.Aligned{}
	if h >= 0 {
		a.Horizontal = h
	}
	if v >= 0 {
		a.Vertical = v
	}
	return c.wrap(a, func(e core.Element) { a.Child = e })
}

// --- Decoration ------------------------------------------------------------

// Background paints a filled rectangle behind the child, covering the box the
// parent allocated.
func (c *Container) Background(color core.Color) *Container {
	b := &elements.Background{Color: color}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// RoundedBackground paints a background with rounded corners.
func (c *Container) RoundedBackground(color core.Color, radius float64) *Container {
	b := &elements.Background{Color: color, Radius: radius}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// Border strokes all four edges.
func (c *Container) Border(width float64, color core.Color) *Container {
	side := elements.BorderSide{Width: width, Color: color}
	b := &elements.Border{Top: side, Right: side, Bottom: side, Left: side}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// BorderEach strokes each edge with its own width.
func (c *Container) BorderEach(top, right, bottom, left float64, color core.Color) *Container {
	b := &elements.Border{
		Top:    elements.BorderSide{Width: top, Color: color},
		Right:  elements.BorderSide{Width: right, Color: color},
		Bottom: elements.BorderSide{Width: bottom, Color: color},
		Left:   elements.BorderSide{Width: left, Color: color},
	}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// BorderTop, BorderRight, BorderBottom and BorderLeft stroke a single edge.
func (c *Container) BorderTop(w float64, color core.Color) *Container {
	return c.BorderEach(w, 0, 0, 0, color)
}

func (c *Container) BorderRight(w float64, color core.Color) *Container {
	return c.BorderEach(0, w, 0, 0, color)
}

func (c *Container) BorderBottom(w float64, color core.Color) *Container {
	return c.BorderEach(0, 0, w, 0, color)
}

func (c *Container) BorderLeft(w float64, color core.Color) *Container {
	return c.BorderEach(0, 0, 0, w, color)
}

// Clip confines the child's drawing to its box without affecting measurement.
func (c *Container) Clip() *Container {
	k := &elements.Clip{}
	return c.wrap(k, func(e core.Element) { k.Child = e })
}

// Rotate turns the child clockwise by the given degrees.
func (c *Container) Rotate(degrees float64) *Container {
	r := &elements.Rotate{Degrees: degrees}
	return c.wrap(r, func(e core.Element) { r.Child = e })
}

// --- Links and navigation --------------------------------------------------

// Link makes the subtree clickable, opening a URL.
//
// The clickable area is the box the child occupies, so wrapping a row makes the
// whole row clickable and wrapping the text makes only the words clickable.
// Nothing is drawn: colour or underline the text if the link should look like one.
func (c *Container) Link(url string) *Container {
	l := &elements.Link{Target: core.ExternalLink(url)}
	return c.wrap(l, func(e core.Element) { l.Child = e })
}

// LinkTo makes the subtree clickable, jumping to a named destination registered
// by Anchor or Bookmark.
//
// The destination may be anywhere in the document, including a later page:
// nothing is resolved until every page has been drawn. A name that never gets
// registered is reported as an error rather than becoming a dead link.
func (c *Container) LinkTo(destination string) *Container {
	l := &elements.Link{Target: core.InternalLink(destination)}
	return c.wrap(l, func(e core.Element) { l.Child = e })
}

// Anchor registers a named destination for LinkTo to aim at.
func (c *Container) Anchor(name string) *Container {
	a := &elements.Anchor{Name: name}
	return c.wrap(a, func(e core.Element) { a.Child = e })
}

// Bookmark adds a top-level entry to the document outline, the panel a reader
// shows alongside the page. It also registers a destination, so LinkTo can target
// the same spot.
func (c *Container) Bookmark(title string) *Container {
	return c.BookmarkAt(0, title)
}

// BookmarkAt adds an outline entry at a nesting level, zero being top level.
//
// An entry becomes a child of the nearest preceding entry with a lower level, the
// way headings nest in a document, and entries appear in the order they were
// drawn.
func (c *Container) BookmarkAt(level int, title string) *Container {
	b := &elements.Bookmark{Title: title, Level: level}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// BookmarkNamed adds an outline entry with an explicit destination name, for when
// two bookmarks would otherwise share a title.
func (c *Container) BookmarkNamed(level int, title, name string) *Container {
	b := &elements.Bookmark{Title: title, Level: level, Name: name}
	return c.wrap(b, func(e core.Element) { b.Child = e })
}

// ShowIf renders the subtree only when condition holds. When it does not, the
// chained content is still built but never measured or drawn.
func (c *Container) ShowIf(condition bool) *Container {
	s := &elements.ShowIf{Condition: condition}
	return c.wrap(s, func(e core.Element) { s.Child = e })
}

// --- Layout containers -----------------------------------------------------

// Column stacks children vertically and is where page breaking happens.
func (c *Container) Column(build func(*ColumnBuilder)) {
	col := &elements.Column{}
	c.install(col)
	build(&ColumnBuilder{column: col, style: c.style})
}

// Row places children side by side.
func (c *Container) Row(build func(*RowBuilder)) {
	row := &elements.Row{}
	c.install(row)
	build(&RowBuilder{row: row, style: c.style})
}

// Table lays out a grid with shared column widths.
func (c *Container) Table(build func(*TableBuilder)) {
	t := &TableBuilder{style: c.style}
	build(t)
	c.install(t.build())
}

// --- Leaves ----------------------------------------------------------------

// Text draws wrapped text in the inherited style.
func (c *Container) Text(content string) {
	c.install(elements.NewText(content, c.style))
}

// StyledText draws text in an explicit style.
func (c *Container) StyledText(content string, style *StyleBuilder) {
	c.install(elements.NewText(content, style.Build()))
}

// RichText composes text from several styled spans, breaking lines across span
// boundaries so styling does not affect where lines wrap.
func (c *Container) RichText(build func(*TextBuilder)) {
	t := &elements.Text{}
	c.install(t)
	build(&TextBuilder{text: t, style: c.style})
}

// PageNumber draws a label derived from the page it lands on. The format may
// contain {page} and {total}.
func (c *Container) PageNumber(format string) {
	c.install(elements.NewPageNumber(format, c.style))
}

// Image draws a decoded image, scaled to the available width by default.
func (c *Container) Image(img core.Image) {
	c.install(&elements.Image{Source: img, Fit: elements.FitWidth})
}

// ImageFit draws an image with an explicit fitting mode.
func (c *Container) ImageFit(img core.Image, fit elements.ImageFit) {
	c.install(&elements.Image{Source: img, Fit: fit})
}

// LineHorizontal draws a horizontal rule spanning the available width.
func (c *Container) LineHorizontal(width float64, color core.Color) {
	c.install(&elements.Line{Width: width, Color: color})
}

// LineVertical draws a vertical rule spanning the available height.
func (c *Container) LineVertical(width float64, color core.Color) {
	c.install(&elements.Line{Vertical: true, Width: width, Color: color})
}

// DashedLineHorizontal draws a dashed horizontal rule. The dash lengths alternate
// on and off in points; a single value means equal dashes and gaps.
func (c *Container) DashedLineHorizontal(width float64, color core.Color, dash ...float64) {
	c.install(&elements.Line{Width: width, Color: color, Dash: dash})
}

// DashedLineVertical draws a dashed vertical rule.
func (c *Container) DashedLineVertical(width float64, color core.Color, dash ...float64) {
	c.install(&elements.Line{Vertical: true, Width: width, Color: color, Dash: dash})
}

// Path draws an arbitrary outline, filled, stroked or both.
//
// This is the escape hatch for shapes the box vocabulary cannot express — arcs,
// polygons, connecting lines. The path is drawn in the element's own coordinate
// space with the origin at its top-left corner, and it claims whatever space the
// parent offers, so wrap it in a constraint to give it a definite size.
func (c *Container) Path(path *core.Path, style core.PathStyle) {
	c.install(&elements.PathShape{Path: path, Style: style})
}

// PageBreak pushes everything after it in the enclosing column onto a new page.
func (c *Container) PageBreak() {
	c.install(&elements.PageBreak{})
}

// Spacer occupies fixed empty space.
func (c *Container) Spacer(width, height float64) {
	c.install(&elements.Spacer{Width: width, Height: height})
}

// Empty draws nothing and takes no space.
func (c *Container) Empty() {
	c.install(elements.Empty{})
}

// --- Builders --------------------------------------------------------------

// ColumnBuilder collects the items of a column.
type ColumnBuilder struct {
	column *elements.Column
	style  core.TextStyle
}

// Spacing sets the gap between items.
func (b *ColumnBuilder) Spacing(v float64) *ColumnBuilder {
	b.column.Spacing = v
	return b
}

// Item appends a slot for the next item.
func (b *ColumnBuilder) Item() *Container {
	// The item is appended immediately so that ordering follows the order the
	// calls were made, and filled in later when the caller's chain resolves.
	index := len(b.column.Items)
	b.column.Items = append(b.column.Items, nil)

	return newContainer(func(e core.Element) {
		b.column.Items[index] = e
	}, b.style)
}

// RowBuilder collects the items of a row.
type RowBuilder struct {
	row   *elements.Row
	style core.TextStyle
}

// Spacing sets the gap between items.
func (b *RowBuilder) Spacing(v float64) *RowBuilder {
	b.row.Spacing = v
	return b
}

func (b *RowBuilder) item(sizing elements.RowSizing, size float64) *Container {
	index := len(b.row.Items)
	b.row.Items = append(b.row.Items, elements.RowItem{Sizing: sizing, Size: size})

	return newContainer(func(e core.Element) {
		b.row.Items[index].Element = e
	}, b.style)
}

// ConstantItem takes a fixed width in points.
func (b *RowBuilder) ConstantItem(width float64) *Container {
	return b.item(elements.RowConstant, width)
}

// RelativeItem takes a share of the width left over after constant and auto
// items, in proportion to weight.
func (b *RowBuilder) RelativeItem(weight float64) *Container {
	return b.item(elements.RowRelative, weight)
}

// AutoItem takes its content's natural width.
func (b *RowBuilder) AutoItem() *Container {
	return b.item(elements.RowAuto, 0)
}

// TextBuilder composes text from styled spans.
type TextBuilder struct {
	text  *elements.Text
	style core.TextStyle
}

// Align sets the paragraph alignment.
func (b *TextBuilder) Align(a core.HorizontalAlign) *TextBuilder {
	b.text.Align = a
	return b
}

// Span appends a run in the inherited style.
func (b *TextBuilder) Span(content string) *TextBuilder {
	b.text.AddSpan(content, b.style)
	return b
}

// StyledSpan appends a run in an explicit style.
func (b *TextBuilder) StyledSpan(content string, style *StyleBuilder) *TextBuilder {
	b.text.AddSpan(content, style.Build())
	return b
}

// Bold appends a bold run, the common case that would otherwise need a full
// style expression.
func (b *TextBuilder) Bold(content string) *TextBuilder {
	return b.StyledSpan(content, StyleFrom(b.style).Bold())
}

// Italic appends an italic run.
func (b *TextBuilder) Italic(content string) *TextBuilder {
	return b.StyledSpan(content, StyleFrom(b.style).Italic())
}

// Line appends a run followed by a line break.
func (b *TextBuilder) Line(content string) *TextBuilder {
	return b.Span(content + "\n")
}
