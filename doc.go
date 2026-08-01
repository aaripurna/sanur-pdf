// Package sanur builds PDF documents from a tree of nested containers.
//
// You describe what a document contains and sanur works out where everything goes,
// breaking content across as many pages as it needs. There are no template files, no
// cursor arithmetic and no MoveTo(x, y).
//
// The package name is sanur while the module path ends in sanur-pdf, so an explicit
// import alias keeps the identifier obvious:
//
//	import sanur "github.com/aaripurna/sanur-pdf"
//
// There is no cgo, and no dependency beyond golang.org/x/image for TrueType metrics and
// golang.org/x/text for bidirectional reordering. The PDF bytes, the fonts and the page
// structure are all written here.
//
// # A complete document
//
//	doc := sanur.New().Title("Report")
//
//	doc.Page(func(p *sanur.Page) {
//		p.Size(sanur.A4).Margin(40)
//		p.Header().Text("Quarterly Report")
//		p.Footer().AlignCenter().PageNumber("Page {page} of {total}")
//
//		p.Content().Column(func(c *sanur.ColumnBuilder) {
//			c.Spacing(10)
//			c.Item().StyledText("Summary", sanur.TextStyle().Size(18).Bold())
//			c.Item().Text("Body text that wraps and paginates on its own.")
//			c.Item().Background(sanur.Grey100).Padding(12).Text("Callout")
//		})
//	})
//
//	err := doc.Write("report.pdf")
//
// # Page definitions, not pages
//
// Page declares a definition rather than a sheet. One definition produces as many
// sheets as its content needs, redrawing its header and footer on each — so a definition
// holding a five-hundred-row table becomes a dozen sheets with the header on every one,
// with no configuration. EveryPage sets furniture shared across several definitions.
//
// # Columns
//
// [Page.Columns] divides a sheet into tracks the content flows through: it fills the
// first, carries on into the second, and only when the last is full does a new sheet
// begin. A paragraph breaking between columns is the same event as one breaking between
// sheets, handled by the same mechanism, so no element has to know columns exist.
// [Page.Spanning] is the full-width lead-in above them, for the headline an article
// opens with.
//
// # How layout works
//
// Everything rests on core.Element, which has two methods: Measure reports what an
// element would do with a given amount of space, and Draw paints it. Measure answers
// with one of three things — everything fits, this much fits and call me again on the
// next page, or nothing useful fits here. That middle answer is what makes pagination
// compose: a paragraph splits between pages, a column splits because its children can,
// and a table splits because it is a column of rows, all through one mechanism with no
// element knowing anything about pages.
//
// Two rules keep the tree predictable, and custom elements have to honour them: space
// on the cross axis passes through in full, so a background spans its parent rather
// than hugging its text; and Measure is repeatable and free of side effects, because
// containers re-measure children while drawing to recover the sizes they promised.
//
// # Fluent API and elements
//
// The types here — Document, Page, Container and the builders — are a fluent façade
// over the elements package. Anything the façade cannot express can be built as a
// core.Element and installed with Container.Element, and the two mix freely.
//
// # Text
//
// The built-in Helvetica and Courier need no font files, and are addressed one byte at
// a time through WinAnsi — which stops at Western Europe. Register a TrueType or
// OpenType font and sanur embeds a subset of it as a composite font, which reaches
// every glyph the font has: Latin Extended, Greek, Cyrillic, and Hebrew, Arabic,
// Persian and Urdu complete with bidirectional reordering and Arabic letter shaping.
//
// # Accessible output
//
// [Document.Tagged] records the document's logical structure alongside its ink, so that
// software can tell a heading from a caption and a table's figures from its labels. Most
// of it is inferred; a heading's level and a picture's description have to be declared,
// and generation fails without them rather than producing a document that passes for
// accessible.
//
// # Concurrency
//
// A Document is not safe for concurrent use, and neither is anything reachable from
// one. Build and generate each document from a single goroutine. Generating several at
// once is fine, and fonts, themes and decoded images may be shared between them; see
// the Document documentation for why.
//
// # The other packages
//
//   - [github.com/aaripurna/sanur-pdf/core] — Element, SpacePlan, Size, Canvas, Color,
//     TextStyle. Knows nothing about PDF, so layout is testable without producing a file.
//   - [github.com/aaripurna/sanur-pdf/elements] — the layout primitives.
//   - [github.com/aaripurna/sanur-pdf/chart] — line, area, bar, pie and donut charts,
//     which are ordinary elements.
//   - [github.com/aaripurna/sanur-pdf/fonts] — standard-14 metrics, TrueType loading
//     and subsetting, the name registry.
//   - [github.com/aaripurna/sanur-pdf/text] — bidirectional reordering and Arabic shaping.
//   - [github.com/aaripurna/sanur-pdf/theme] — document styling loaded from JSON.
//   - [github.com/aaripurna/sanur-pdf/render] — the PDF canvas and image embedding.
//
// Output is deterministic: sanur never reads the clock and never iterates a map while
// writing, so the same document always produces identical bytes.
package sanur
