# sanur

A declarative PDF layout engine for Go, in the spirit of
[QuestPDF](https://github.com/QuestPDF/QuestPDF).

You describe a document as a tree of nested containers and sanur works out where
everything goes, breaking content across as many pages as it needs. No template
files, no manual cursor arithmetic, no `MoveTo(x, y)`.

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
  element.

## Packages

| Package | Contents |
| --- | --- |
| `sanur` | Fluent API: `Document`, `Page`, `Container`, styles, sizes, colours. |
| `core` | `Element`, `SpacePlan`, `Size`, `Canvas`, `Color`, `TextStyle`. No PDF knowledge. |
| `elements` | Layout primitives implementing `core.Element`. |
| `fonts` | Standard-14 metrics, WinAnsi encoding, TrueType loading. |
| `render` | The PDF canvas, the discard canvas, image embedding. |
| `internal/pdfobj` | PDF objects, streams, xref, trailer. |

The dependency direction is strict: `core` knows nothing about PDF, so layout is
testable without producing a file, and the file format is testable without
running a layout.

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

**Containers** — `Column`, `Row`, `Table`

**Leaves** — `Text`, `StyledText`, `RichText`, `PageNumber`, `Image`, `ImageFit`,
`LineHorizontal`, `LineVertical`, `PageBreak`, `Spacer`, `Empty`, `Element`

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

JPEGs are embedded byte-for-byte via `DCTDecode`, so no quality is lost. PNGs are
decoded to RGB, with any transparency emitted as a separate soft mask, because PDF
image samples carry no alpha channel. Images are pooled by key, so a logo repeated
in a footer costs its bytes once — which makes the key worth setting deliberately
when the same picture is loaded twice.

Fit modes: `FitWidth` (default), `FitArea`, `FitStretch`, `FitUnscaled`.

An image refuses a box it cannot fit rather than overflowing, since rendering two
thirds of a photograph is not meaningful. To crop instead, wrap it in `Clip`,
which measures its child against unbounded space and hides the excess:

```go
c.Item().Size(160, 60).Clip().Image(img)   // cropped, not squashed
```

## Examples

```
make examples     # or: make invoice / make images / make report
```

| Example | What it covers |
| --- | --- |
| `examples/invoice` | Tables that paginate, repeated header and footer, page numbering, right-aligned currency |
| `examples/images` | All four loading routes, the four fit modes side by side, cropping, pooled logos in a table |
| `examples/report` | Stat tiles, bar charts and sparklines built from primitives, justified two-column prose, mixed portrait and landscape sheets |

The report example is the one to read for complex layout. There is no chart
element, no stat-tile element and no sidebar element in sanur, and it shows why
none are needed: each is a short composition of rows, columns, backgrounds and
lines. It also includes a `sparkline` implementing `core.Element` directly, for the
case where composition genuinely runs out — a polyline through arbitrary points
cannot be expressed as nested boxes.

All three are executed by the test suite and checked with Ghostscript, so they
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

255 tests across six packages, at 95.6% statement coverage:

| Package | Statements | Covered | |
| --- | --- | --- | --- |
| `core` | 102 | 102 | 100.0% |
| `elements` | 554 | 535 | 96.6% |
| `sanur` (root) | 373 | 359 | 96.2% |
| `internal/pdfobj` | 157 | 149 | 94.9% |
| `render` | 272 | 257 | 94.5% |
| `fonts` | 174 | 159 | 91.4% |
| **Total** | **1632** | **1561** | **95.6%** |

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
- **Cross-axis resolution** is pinned per decorator. A row whose height came from
  a vertically centred cell would silently become page-tall — the layout still
  succeeds, it just looks wrong — so each `NaturalSize` forward is tested on its
  own.
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
- Multi-column text flow, and repeating a table header on every page
- Links, outlines, annotations, form fields, tagged/accessible output
- Encryption, and PDF/A conformance
- Gradients, dash patterns, and blend modes
