# sanur

A declarative PDF layout engine for Go, in the spirit of
[QuestPDF](https://github.com/QuestPDF/QuestPDF).

You describe a document as a tree of nested containers and sanur works out where
everything goes, breaking content across as many pages as it needs. No template
files, no manual cursor arithmetic, no `MoveTo(x, y)`.

```
go get github.com/aaripurna/sanur-pdf
```

The package is named `sanur` while the module path ends in `sanur-pdf`, so an
explicit alias keeps the identifier obvious at a glance:

```go
import (
	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/elements"
	"github.com/aaripurna/sanur-pdf/render"
)
```

```go
doc := sanur.New().Title("Report")

doc.Page(func(p *sanur.Page) {
	p.Size(sanur.A4).Margin(40)
	p.Header().Text("Quarterly Report")
	p.Footer().AlignCenter().PageNumber("Page {page} of {total}")

	p.Content().Column(func(c *sanur.ColumnBuilder) {
		c.Spacing(10)
		c.Item().StyledText("Summary", sanur.TextStyle().Size(18).Bold())
		c.Item().Text("Body text that wraps and paginates on its own.")
		c.Item().Background(sanur.Grey100).Padding(12).Text("Callout")
	})
})

err := doc.Write("report.pdf")
```

No cgo, and no dependencies beyond `golang.org/x/image` for reading TrueType
metrics. PDF bytes, fonts and page structure are all written here.

## Why the layout engine looks the way it does

Everything rests on one interface:

```go
type Element interface {
	Measure(available Size) SpacePlan
	Draw(canvas Canvas, available Size)
}
```

`Measure` reports what an element would do with a given amount of space, and
returns one of three answers:

| Plan | Meaning |
| --- | --- |
| `FullRender(size)` | Everything fits; one `Draw` finishes it. |
| `PartialRender(size)` | Rendered what fit; call me again on the next page. |
| `Wrap(reason)` | Nothing useful fits here; retry me on a fresh page. |

`PartialRender` is the whole point. An element that can only say "I need N
points" cannot take part in pagination, so page breaking has to be special-cased
somewhere central and never composes properly. Because every element can report
partial progress instead, a paragraph splits between pages, a column splits
because its children can, and a table splits because it is a column of rows —
all through the same mechanism, with no element knowing anything about pages.

Two rules keep the tree predictable, and custom elements need to honour them:

- **Cross-axis space passes through in full.** When a column draws a child it
  gives it the child's measured *height* but the column's full *width*. This is
  what makes `Background` span its parent instead of hugging its text, and why
  full-width rules and banded table rows need no extra stretching. An element
  that wants to fill the other axis uses `Extend`; one that wants to hug its
  content uses `Width`/`Height`.
- **`Measure` is repeatable and side-effect free.** Containers re-measure
  children during `Draw` to recover the sizes they were promised, so an element
  whose answer changed in between would draw into a box of the wrong size.
  Elements that track progress across pages advance it in `Draw`, never in
  `Measure`.
- **Elements that fill report a natural size separately.** A row must know its
  height before it can align anything inside it, but a vertically centred child
  answers `Measure` with the whole height on offer — that is what centring means.
  Such elements implement `core.CrossAxisNatural` to report what their content
  needs, and a row resolves its height from those answers first. Every
  pass-through decorator forwards the query, since a row cell is usually a
  background wrapping padding wrapping the alignment rather than a bare aligned
  element — and it forwards it *as* a natural-size query, via
  `core.NaturalSizeOf`, never as a `Measure`. Measuring would let the child expand
  again, which is what `AlignRight().AlignMiddle()` does: two of these nest, and
  the outer one has to ask the inner for its content size, not its aligned size.

## Packages

All paths below are relative to `github.com/aaripurna/sanur-pdf`.

| Package | Contents |
| --- | --- |
| `.` (`sanur`) | Fluent API: `Document`, `Page`, `Container`, styles, sizes, colours. |
| `core` | `Element`, `SpacePlan`, `Size`, `Canvas`, `Color`, `TextStyle`. No PDF knowledge. |
| `elements` | Layout primitives implementing `core.Element`. |
| `chart` | Static line, area, bar, pie and donut charts. Depends only on `core` and `fonts`. |
| `fonts` | Standard-14 metrics, WinAnsi encoding, TrueType loading, the name registry. |
| `theme` | Document styling loaded from JSON. Depends on `core`, `fonts` and `chart`. |
| `render` | The PDF canvas, the discard canvas, image embedding. |
| `internal/pdfobj` | PDF objects, streams, xref, trailer. |

The dependency direction is strict: `core` knows nothing about PDF, so layout is
testable without producing a file, and the file format is testable without
running a layout.

## Pages, sheets and shared furniture

`doc.Page(...)` declares a page *definition*, not a page. One definition produces
as many sheets as its content needs, redrawing its header and footer on each — so
a definition holding a 500-row table becomes a dozen sheets with the header on
every one. That much needs no configuration.

What needs configuring is furniture shared across *several* definitions.
`EveryPage` sets defaults — size, margins, background, text style, header and
footer — for every definition declared after it:

```go
doc.EveryPage(func(p *sanur.Page) {
	p.Size(sanur.A4).Margin(40)
	p.Header().Text("Annual Report")
	p.Footer().AlignCenter().PageNumber("Page {page} of {total}")
})

doc.Page(func(p *sanur.Page) {
	p.Content().Text("Inherits everything above.")
})

doc.Page(func(p *sanur.Page) {
	p.Size(sanur.Landscape(sanur.A4))  // overrides only the size
	p.Content().Text("Same header and footer.")
})
```

A definition's own build runs second, so it overrides whatever it names and
inherits the rest.

`EveryPage` takes a function rather than a prepared element tree, and that is
deliberate. Elements carry pagination state — a column remembers which item it
reached, a text block which line — so one shared header instance would arrive at
the second definition believing it had already been drawn. Running the function
afresh per definition gives each one its own instances.

## Links and bookmarks

Links are elements, so anything can be made clickable — a word, a table row, a
whole panel. The clickable area is whatever box the child occupies.

```go
c.Item().Link("https://example.com").Text("External link")

c.Item().LinkTo("methods").Text("Jump to Methods")   // internal
c.Item().Anchor("methods").Text("Methods")           // the destination

c.Item().Bookmark("Introduction").Text("Introduction")
c.Item().BookmarkAt(1, "Background").Text("Background")
```

`Bookmark` does two things: it adds an entry to the reader's outline panel, and it
registers a destination named after its title — so `LinkTo("bookmark:Introduction")`
targets the same spot without a separate anchor. `BookmarkNamed` takes an explicit
name for when two sections legitimately share a title.

Outline entries nest by level, the way document headings do: an entry becomes a
child of the nearest preceding entry with a lower level, in the order they were
drawn. A level that jumps by more than one attaches to the nearest open ancestor
rather than being dropped.

Nothing is resolved until every page has been drawn, so **a link may point
forwards** — which is what a table of contents needs. A name that never gets
registered, or one registered twice, is reported as an error rather than becoming
a dead or ambiguous link.

Two things to know:

- **A link draws nothing.** Colour or underline the text yourself if it should
  look like a link; a document may reasonably want neither.
- **A table of contents cannot show page numbers yet.** Destinations resolve after
  layout, so the page a section landed on is not available to the text that
  describes it. The entries are clickable; they just cannot say "page 7".

## Theming from JSON

`theme` loads the static half of a document's appearance — page geometry, named
colours, named text styles, chart styling — from JSON, so a designer can change it
without a rebuild.

```json
{
  "page":   {"size": "A4", "margin": [34, 42], "background": "paper"},
  "colors": {"paper": "#FFFFFF", "ink": "#1A1D29", "accent": "#4F46E5"},
  "fonts":  {"body": "Helvetica", "bold": "Helvetica-Bold"},
  "text": {
    "body":    {"font": "body", "size": 9.5, "color": "ink", "lineHeight": 1.35},
    "heading": {"font": "bold", "size": 12,  "color": "accent"}
  },
  "chart": {"palette": ["accent", "#0891B2"], "legend": "top", "tickCount": 4}
}
```

```go
th, err := theme.Load("brand.json")

doc.EveryPage(func(p *sanur.Page) {
	p.Size(th.PageSize()).MarginEach(th.Margins()).Background(th.Background())
	p.DefaultTextStyle(sanur.StyleFrom(th.Style("body")))
})

c.Item().StyledText("Heading", sanur.StyleFrom(th.Style("heading")))
c.Item().Height(165).Element(&chart.Bar{ /* ... */ Style: th.ChartStyle()})
```

`sanur.StyleFrom` is the bridge from a resolved `core.TextStyle` back into the
builder the fluent API takes; elements accept a `core.TextStyle` directly.

**Structure stays in Go, and that is the whole design.** JSON has no loops or
conditionals, so putting document structure in it means inventing a template
language inside string values — a DSL with no type checking, no editor support, and
errors that point at a config file rather than at code. The first time a report
needs one table row per invoice line, a `for` loop wins outright. What JSON is
genuinely good at is flat, static configuration.

Details worth knowing:

- **Every reference is resolved at load**, and *all* problems are reported at once.
  A misspelled colour fails at startup with the available names listed, not as
  invisible text on page forty.
- **Colour and font fields take a name or a literal.** `"color": "ink"`,
  `"color": "#FF0000"` or `"color": "cmyk(0, 0, 0, 100)"`; `"font": "body"` or
  `"font": "Helvetica-Bold"`.
- **Margins accept shorthand:** `40`, `[24, 40]`, `[10, 20, 30, 40]` in CSS order,
  or `{"top": 10, "left": 20}`.
- **`Style` and `Color` panic on an unknown name**, listing what exists. A zero
  style renders as invisible text in the middle of a document, which is far harder
  to trace than a stack trace on the first run. `LookupStyle` and `LookupColor` are
  the non-panicking forms.
- **`chart.Style.Format` is never set from a theme.** It is a function, so no JSON
  file can supply one; assign it on the value `ChartStyle` returns.

Fonts are named through `fonts.Registry`, pre-populated with the standard-14. A
registered TrueType face becomes nameable from a theme:

```go
fonts.Default().LoadTrueType("Inter", "Inter-Regular.ttf")
```

Use `theme.WithFonts(registry)` to resolve against a specific registry rather than
the shared one — which matters for tests, and for a server rendering with
per-tenant fonts.

## Layout vocabulary

Chain these off any container. Decorating methods return the container for the
child they just wrapped, so calls nest rather than overwrite.

**Spacing** — `Padding`, `PaddingXY`, `PaddingEach`, `PaddingTop`/`Right`/`Bottom`/`Left`

**Sizing** — `Width`, `Height`, `Size`, `MinWidth`, `MaxWidth`, `MinHeight`,
`MaxHeight`, `Extend`, `ExtendHorizontal`, `ExtendVertical`

**Alignment** — `AlignLeft`, `AlignCenter`, `AlignRight`, `AlignTop`,
`AlignMiddle`, `AlignBottom`

**Decoration** — `Background`, `RoundedBackground`, `Border`, `BorderEach`,
`BorderTop`/`Right`/`Bottom`/`Left`, `Clip`, `Rotate`, `ShowIf`

**Navigation** — `Link`, `LinkTo`, `Anchor`, `Bookmark`, `BookmarkAt`, `BookmarkNamed`

**Containers** — `Column`, `Row`, `Table`, `Repeat`, `Layers`

**Leaves** — `Text`, `StyledText`, `RichText`, `PageNumber`, `Image`, `ImageFit`,
`LineHorizontal`, `LineVertical`, `DashedLineHorizontal`, `DashedLineVertical`,
`Path`, `PageBreak`, `Spacer`, `Empty`, `Element`

### Paths

Anything that is not an axis-aligned box goes through a path: arcs, polygons,
dashed rules, stroke joins. A `core.Path` is a value built independently of any
canvas, so shapes are reusable and their geometry is testable without rendering.

```go
centre := core.Position{X: 60, Y: 60}

slice := core.NewPath().
	MoveTo(centre).                  // pie slices start at the centre,
	Arc(centre, 50, -90, 120).       // sweep the rim,
	Close()                          // and close back along the second radius

c.Item().Size(120, 120).Path(slice, core.PathStyle{
	Fill:   sanur.Indigo,
	Stroke: sanur.White,
	Width:  1,
})
```

`PathStyle` covers fill, stroke, width, caps, joins, dash pattern and fill rule in
one struct. An invisible fill or a zero width simply omits that half, so the same
type expresses fill-only, stroke-only and both — and where both apply they are
painted in a single operator, so a translucent edge is not composited twice.

Angles are degrees from the positive X axis, and because layout space has Y
growing downwards, **a positive sweep turns clockwise** — the same direction as
`Rotate`.

Two things worth knowing:

- **Rings need `EvenOdd: true`.** PDF fills by nonzero winding, so two circles
  traced the same way both count the region between them as inside and it fills
  solid. Even-odd counts crossings instead, so the inner circle punches a hole.
  (Reversing the inner subpath with a negative sweep also works, but requires
  reasoning about direction.)
- **A path claims the box it is offered**, rather than measuring its own extent.
  Using the bounding box would let a shape whose points sit far from the origin
  silently resize its parent, so the caller constrains the box and draws within
  it.

Arcs are Bézier-approximated — PDF has no arc operator — subdivided at quarter
turns, which keeps the deviation from a true circle under a thousandth of the
radius.

### Rows

Row widths resolve in a fixed order — constants, then auto items measured against
what is left, then relative items sharing the remainder:

```go
c.Item().Row(func(r *sanur.RowBuilder) {
	r.Spacing(8)
	r.ConstantItem(80).Text("fixed 80pt")
	r.AutoItem().Text("as wide as its content")
	r.RelativeItem(1).Text("half the rest")
	r.RelativeItem(1).Text("the other half")
})
```

A row is atomic across its width: if one cell cannot fit, the whole row moves to
the next page rather than leaving a hole.

### Repeating headers

A column that splits across pages resumes at the item it reached, so a heading
declared as row zero labels only the first sheet. `HeaderRow` declares it
separately, and it reappears at the top of every continuation:

```go
c.Item().Table(func(t *sanur.TableBuilder) {
	t.ColumnsRelative(5, 1, 2)
	t.HeaderRow(func(r *sanur.TableRowBuilder) {
		r.Cells("Description", "Qty", "Amount")
	})
	for _, item := range items {
		t.Row(func(r *sanur.TableRowBuilder) { /* ... */ })
	}
})
```

`Repeat` is the general form, for anything that should reappear above paginating
content:

```go
c.Item().Repeat(func(r *sanur.RepeatBuilder) {
	r.Header().PaddingBottom(6).Text("Schedule 1 (continued)")
	r.Body().Column(func(c *sanur.ColumnBuilder) { /* ... */ })
})
```

The header is measured and its height subtracted from what the body may use, so the
two never overlap, and its state is rewound after each sheet so it draws in full
every time. A header that would itself paginate is reported as not fitting — a
heading is only useful whole.

### Layers, watermarks and overlays

PDF has no z-index: things appear in the order they are painted. `Layers` is how
overlap is expressed, and `Content` alone decides the size so an oversized
decoration cannot stretch the layout around it:

```go
c.Item().Layers(func(l *sanur.LayersBuilder) {
	l.Below().Background(sanur.Grey100).Empty()
	l.Content().Padding(12).Text("Above the tint, under the badge")
	l.Above().AlignRight().AlignTop().Text("NEW")
})
```

Page-level equivalents span the whole sheet, margins included, reserve no space, and
repeat on every sheet:

```go
p.Watermark().Rotate(-38).StyledText("CONFIDENTIAL", faint)  // behind everything
p.Overlay().Rotate(-38).StyledText("DRAFT", faint)           // over everything
```

`Rotate` turns its child about the centre of its box, which is what a stamp or a
sideways label wants — about a corner, anything turned more than a few degrees
swings off the page. Text inside a `Rotate` does not wrap, since a rotated
element is measured against unbounded space.

### Tables

A table is a column of rows sharing one column specification, which is what keeps
cells aligned vertically. Being a column underneath, it paginates for free.

```go
c.Item().Table(func(t *sanur.TableBuilder) {
	t.ColumnsRelative(5, 1, 2).ColumnSpacing(8)
	t.Row(func(r *sanur.TableRowBuilder) {
		r.Cells("Description", "Qty", "Amount")
	})
	for _, item := range items {
		t.Row(func(r *sanur.TableRowBuilder) {
			r.Cell().PaddingXY(8, 5).Text(item.Name)
			r.Cell().AlignRight().Text(item.Qty)
			r.Cell().AlignRight().Text(item.Amount)
		})
	}
})
```

### Rich text

Line breaking runs across span boundaries, so styling never affects where lines
break:

```go
c.Item().RichText(func(t *sanur.TextBuilder) {
	t.Align(sanur.AlignJustify)
	t.Span("Ordinary text with ").Bold("bold").Span(" and ").Italic("italic").Span(" runs.")
})
```

Justification uses PDF's word-spacing operator, so a justified line costs one
number rather than a repositioned run per word. The last line of a block is left
flush.

## Charts

`chart` draws static charts as ordinary elements, so one goes wherever any other
element goes — in a row beside a table, inside a bordered panel, in a column that
paginates. The only thing a chart needs from its caller is a height, because a
plot has no natural size of its own.

```go
c.Item().Height(190).Element(&chart.Line{
	Categories: []string{"Jan", "Feb", "Mar", "Apr"},
	Series: []chart.Series{
		{Name: "Revenue", Values: []float64{31, 34, 32, 39}},
		{Name: "Costs", Values: []float64{22, 23, 24, 24}},
	},
})
```

| Type | Notes |
| --- | --- |
| `Line` | One or more series; set `Area` to fill beneath them |
| `Bar` | Grouped series, vertical or `Horizontal`, optional `CornerRadius` |
| `Pie` | Set `InnerRadius` for a donut, with optional centre text |

A zero `Style` resolves to the defaults, so a chart needs no configuration to
look finished. Setting one field overrides only that field:

```go
Style: chart.Style{
	Palette:   []core.Color{sanur.Hex("#0F766E"), sanur.Hex("#B45309")},
	TickCount: 4,
	Legend:    chart.LegendRight,
	Format:    func(v float64) string { return fmt.Sprintf("%.0fms", v) },
}
```

Three behaviours worth knowing:

- **Axis ticks land on round numbers**, not on the data extremes. `Ticks` and
  `NiceStep` are exported, along with `Scale` and `FormatValue`, since they are
  pure arithmetic and useful when building a chart type of your own.
- **Gutters are measured, not fixed.** The left gutter comes from the widest tick
  label and a horizontal chart's from the longest category name, so a jump to
  seven figures cannot silently overlap the plot.
- **Bars and areas always include zero** in their axis. An axis starting at the
  smallest value exaggerates every difference.

Negative values work throughout `Line`, `Bar` and areas: the axis is drawn at zero
whenever the data crosses it, negative bars hang the other side, and their labels
are placed clear of the category names. `Pie` is the exception — a wedge cannot
sweep backwards and still tile a circle, so a negative slice is **reported as an
error** rather than dropped. Dropping it would rescale the remaining slices to
100% and produce a chart that looks entirely plausible with data missing.

`chart` depends only on `core` and `fonts` — never the root package — so the
dependency runs one way and a future `c.Item().Chart(...)` wrapper stays possible.

## Fonts

Helvetica and Courier are built in with exact Adobe metrics, in all four
weight/slant combinations. A document needs no font files:

```go
p.DefaultTextStyle(sanur.TextStyle().Size(11))                 // Helvetica
c.Item().StyledText("code", sanur.TextStyle().Mono().Size(9))  // Courier
```

For anything else, register a TrueType or OpenType file. Metrics come from the
font's own tables and the program is embedded in the output:

```go
face, err := fonts.LoadTrueTypeFile("Inter", "Inter-Regular.ttf")
bold, err := fonts.LoadTrueTypeFile("Inter-Bold", "Inter-Bold.ttf")

family := sanur.NewFamily(face, bold, nil, nil)
p.DefaultTextStyle(sanur.TextStyle().Family(family).Size(11))
```

Grouping faces into a `Family` means `.Bold()` picks the real bold font rather
than letting a reader synthesise one by smearing the regular outlines. An
incomplete family degrades to the nearest available face.

Text is encoded as WinAnsi, which covers Latin-1 plus the typographic
punctuation real documents use. Runes outside it become `?` — visibly wrong
rather than silently missing. Other scripts need a TrueType font.

## Colour, for screen and for print

Colours come in two spaces, and a document may mix them freely — PDF selects a
colour space per drawing operation, so an RGB chart and a CMYK logo can share a
page.

```go
sanur.RGB(30, 136, 229)          // additive, for a screen
sanur.Hex("#1E88E5")             // the same thing, written the usual way
sanur.RGBA(30, 136, 229, 128)    // half opaque

sanur.CMYK(0, 0, 0, 100)         // plate percentages, for a press
sanur.CMYKA(0, 0, 0, 100, 40)    // the fifth value is opacity, also a percentage
sanur.Color("cmyk(0, 0, 0, 100)")// either notation, parsed
sanur.Registration               // all four plates, for crop marks
```

**A CMYK colour is written to the file as CMYK.** Nothing in the output path
converts it: the canvas emits `k` and `K` rather than `rg` and `RG`, so the plates
specified here are the plates the press lays down.

That distinction is the whole reason the space is tracked rather than everything
being normalised to RGB. Black text wants 100% K on its own — one plate, so
misregistration cannot fringe the letters. A photographic black wants all four.
Both are `#000000` in RGB, so a conversion has to pick one, and the choice belongs
where the colour is specified rather than in a conversion nobody sees.

`Color` values are comparable and convert on demand:

```go
c.Space()             // SpaceRGB or SpaceCMYK
c.RGBComponents()     // 0..1, converting if needed
c.CMYKComponents()    // 0..1, converting if needed
c.WithAlpha(128)      // same colour, same space, different opacity
c.String()            // "#1E88E5" or "cmyk(0, 0, 0, 100)" — parses back exactly
```

Conversion between the spaces is the naive formula, adequate for a preview and not
colour management: a faithful conversion needs the source and destination ICC
profiles and depends on the press. It only runs when something asks for the other
space — an RGB colour reaching `CMYKComponents` maps pure black to 100% K rather
than to four plates, again the safer default for text.

Theme files take either notation wherever a colour is expected, which is what makes
a print build and a screen build differ by one line:

```json
{"colors": {"ink": "cmyk(0, 0, 0, 100)", "accent": "cmyk(78, 68, 0, 0)"}}
```

Opacity is not a colour-space matter: it lives in a PDF graphics state dictionary
either way, so translucent CMYK works exactly as translucent RGB does.

## Images

Loading is separate from layout. The fluent API cannot fail, so reading a file
happens first, where the error can be handled; the resulting `core.Image` is then
handed to as many `.Image()` calls as you like.

```go
// From a path
img, err := render.LoadImageFile("photo", "assets/photo.jpg")

// From bytes you already hold — //go:embed, an HTTP body, a database blob
img, err := render.DecodeImage("logo", logoBytes)

// From any fs.FS, including an embed.FS
img, err := render.LoadImageFS(assets, "logo", "assets/logo.png")

// From an image.Image you generated
img, err := render.EncodeJPEG("chart", rendered, 85)

c.Item().Width(120).Image(img)
```

JPEGs are embedded byte-for-byte via `DCTDecode`, so no quality is lost. Because
the data is never decoded, its colour space has to be declared from outside it —
so the channel count is read from the frame header and mapped to `DeviceGray`,
`DeviceRGB` or `DeviceCMYK`. Adobe writes CMYK JPEGs inverted and records the fact
only by the presence of an APP14 segment, so that case gets a `/Decode` array to
undo it.

PNGs are decoded to RGB, with any transparency emitted as a separate soft mask,
because PDF image samples carry no alpha channel. Images are pooled by key, so a
logo repeated in a footer costs its bytes once — which makes the key worth setting
deliberately when the same picture is loaded twice.

Fit modes: `FitWidth` (default), `FitArea`, `FitStretch`, `FitUnscaled`.

An image refuses a box it cannot fit rather than overflowing, since rendering two
thirds of a photograph is not meaningful. To crop instead, wrap it in `Clip`,
which measures its child against unbounded space and hides the excess:

```go
c.Item().Size(160, 60).Clip().Image(img)   // cropped, not squashed
```

## Examples

```
make examples     # or: make invoice / images / report / charts / themed / print
```

| Example | What it covers |
| --- | --- |
| `examples/invoice` | A table that paginates with a repeating header row, a DRAFT overlay, repeated header and footer, page numbering, right-aligned currency |
| `examples/images` | All four loading routes, the four fit modes side by side, cropping, pooled logos in a table |
| `examples/report` | `EveryPage` furniture, a clickable table of contents with bookmarks, stat tiles, sparklines, justified two-column prose, mixed portrait and landscape sheets |
| `examples/charts` | Every chart type, negative values across all of them, styling overrides, and charts nested in other layout |
| `examples/themed` | One document, two JSON themes. Contains no colour, font, size or margin literals at all |
| `examples/print` | Press-ready CMYK: process inks, tint ramps, the two blacks, a duotone, both spaces on one page, and crop marks on a bleed sheet |

The report example is the one to read for complex layout. There is no chart
element, no stat-tile element and no sidebar element in sanur, and it shows why
none are needed: each is a short composition of rows, columns, backgrounds and
lines. It also includes a `sparkline` implementing `core.Element` directly, for the
case where composition genuinely runs out — a polyline through arbitrary points
cannot be expressed as nested boxes.

Every example is executed by the test suite and checked with Ghostscript, so they
cannot silently rot.

## Page numbering

`{page}` and `{total}` expand in `PageNumber`. Knowing the total requires laying
the document out first, so generation runs the layout onto a canvas that discards
everything, then repeats it for real. The count is iterated until it stabilises,
since the label's own width can affect where content falls.

Output is deterministic: sanur never reads the clock and never iterates a map
while writing, so the same document always produces identical bytes. Supply
`CreationDate` yourself if you want a timestamp.

## Errors

The fluent API cannot fail — no method returns an error — and layout problems
surface once, from `Bytes()` or `Write()`, with the reason:

```
sanur: page definition 1: content does not fit in the available 515.3x761.9:
column item 3 does not fit: minimum height 900.0 exceeds the available 761.9
```

A layout that makes no progress across two consecutive pages is reported as a
stall rather than filling a disk.

## Testing

```
make test         # run the suite
make cover        # per-package statement coverage
make cover-html   # write coverage.html
make example      # generate invoice.pdf
```

534 tests across eight packages, at 94.9% statement coverage:

| Package | Statements | Covered | |
| --- | --- | --- | --- |
| `core` | 272 | 269 | 98.9% |
| `sanur` (root) | 452 | 434 | 96.0% |
| `elements` | 660 | 630 | 95.5% |
| `internal/pdfobj` | 172 | 164 | 95.3% |
| `theme` | 181 | 170 | 93.9% |
| `render` | 512 | 480 | 93.8% |
| `chart` | 479 | 448 | 93.5% |
| `fonts` | 219 | 202 | 92.2% |
| **Total** | **2947** | **2797** | **94.9%** |

Coverage is measured with `-coverpkg` across the whole module rather than
per-package, because much of `render` is exercised by the root package's
end-to-end tests and would otherwise be reported as untested. `make cover`
summarises by statement count; averaging `go tool cover -func`'s per-function
percentages would weight a one-line accessor the same as the layout engine.

What the suite checks, and why in that particular way:

- **Layout semantics** run against stub elements of known size, so container
  arithmetic is verified without depending on font metrics.
- **Argument order** in `PaddingEach`, `BorderEach` and `MarginEach` is asserted
  against the emitted content stream. A measured size cannot distinguish left
  padding from right — they sum identically — so only the translation reveals a
  swapped argument.
- **Content stream operators** are read back from uncompressed output: the axis
  flip, the counter-flipped text matrix, clip paths, Bézier corners and the
  transparency graphics state.
- **Font metrics** are spot-checked against the published Adobe values, and the
  WinAnsi encoding is round-tripped over the 0x80–0x9F block where it diverges
  from Latin-1.
- **Chart arithmetic** — tick selection, scaling, value formatting — is tested as
  pure functions, with no canvas involved. Drawing is asserted through a recording
  canvas that captures labels and their positions, so a collision between a
  negative bar's label and its category label is caught without pinning
  coordinates that any cosmetic change would break.
- **Annotation geometry** is checked against an exact rectangle, because a link in
  the wrong place is invisible until somebody clicks empty space. The transform
  maths and the layout-to-PDF coordinate flip are tested separately from the
  documents that use them.
- **Cross-axis resolution** is pinned per decorator. A row whose height came from
  a vertically centred cell would silently become page-tall — the layout still
  succeeds, it just looks wrong — so each `NaturalSize` forward is tested on its
  own.
- **JPEG colour spaces** are covered two ways. Greyscale runs end to end through
  Ghostscript, since Go can encode a single-channel JPEG. Go has no four-channel
  encoder, so CMYK is tested against synthesised marker headers instead — enough
  to pin the colour-space decision and the Adobe inversion, but the CMYK path has
  not been rendered against a real file.
- **Print colour is measured, not inspected.** Ghostscript's `inkcov` device
  reports coverage per plate, so a CMYK colour that was quietly routed through RGB
  is caught by the plates that come back inked. `cmyk(100, 0, 0, 100)` and
  `cmyk(0, 0, 0, 100)` are the same `#000000` in RGB and look identical in a
  viewer; only the separations tell them apart.
- **Real interpreters** parse the output. Ghostscript and `pdftotext` run where
  installed, which is the only way to catch a structurally plausible file that no
  reader will actually open, and to confirm the text is text rather than shapes.
- **The examples themselves** are compiled, run and Ghostscript-checked, including
  an assertion that no glyph was substituted with a question mark.
- **Determinism** is asserted by generating the same document twice and diffing
  the bytes.

Tests that need a system font or an external tool skip cleanly when it is
missing, so the suite passes on a bare machine.

## Not yet implemented

- Standard-14 Times (its metrics are not reproduced here; register a TrueType
  font instead)
- Font subsetting — embedded fonts are included whole
- Multi-column text flow
- Stacked chart series, dual axes, time-based category axes, scatter and radar plots
- Page numbers in a table of contents (destinations resolve after layout)
- Annotations beyond links: notes, highlights, form fields
- Tagged and accessible output (PDF/UA)
- Encryption, and PDF/A conformance
- Gradients and blend modes (dash patterns, arcs and paths are implemented)
- Spot colours (`Separation` and `DeviceN`), overprint control, and ICC-based colour
  management — RGB and CMYK are written as `DeviceRGB` and `DeviceCMYK`, so a
  conversion between them is arithmetic rather than a managed transform
- Clipping to an arbitrary path — `Clip` takes a rectangle only

## License

MIT — see [LICENSE](LICENSE).

The only runtime dependency, `golang.org/x/image` (and `golang.org/x/text` beneath
it), is BSD-3-Clause, which imposes nothing MIT does not. Attribution for the
standard-14 font metrics is noted in `fonts/standard14.go`: the advance widths come
from Adobe's published AFM files, which Adobe released for unrestricted use.
