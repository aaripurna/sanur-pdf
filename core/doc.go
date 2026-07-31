// Package core defines the layout contract every element implements, and nothing else.
//
// It knows nothing about PDF. That boundary is deliberate and load-bearing: layout can
// be tested without producing a file, and the file format can be verified without
// running a layout. Nothing in this package imports anything else of sanur's.
//
// # The contract
//
// An [Element] answers two questions. Measure reports what it would do with a given
// amount of space, without drawing anything or changing anything. Draw paints it into
// the box its parent settled on.
//
// Measure returns a [SpacePlan], which is one of three answers:
//
//   - FullRender — everything fits, and one Draw finishes it.
//   - PartialRender — this much fits; call me again on the next page.
//   - Wrap — nothing useful fits here; retry me on a fresh page.
//
// PartialRender is the whole design. An element that could only report "I need N points"
// would be unable to take part in pagination, so page breaking would have to be
// special-cased somewhere central and would never compose. Because every element can
// report partial progress instead, a paragraph splits between pages, a column splits
// because its children can, and a table splits because it is a column of rows — one
// mechanism, and no element knows anything about pages.
//
// # Rules a custom element must honour
//
// Measure is repeatable and free of side effects. Containers re-measure their children
// during Draw to recover the sizes they were promised, so an element whose answer changed
// in between would draw into a box of the wrong size. Elements that track progress across
// pages advance it in Draw, never in Measure.
//
// Cross-axis space passes through in full. When a column draws a child it gives that
// child the child's own measured height but the column's full width. This is what makes
// a background span its parent instead of hugging its text, and why full-width rules and
// banded table rows need no extra stretching.
//
// An element that fills whatever it is offered reports its natural size separately, by
// implementing [CrossAxisNatural]. A row has to know its height before it can align
// anything inside it, but a vertically centred child answers Measure with the whole
// height on offer, because that is what centring means. [NaturalSizeOf] breaks the
// circularity, and every pass-through decorator forwards the query.
//
// # Optional interfaces
//
// Beyond Element, an element may implement any of [Composite] to expose its children,
// [StateResettable] to rewind pagination progress, [ContextAware] to receive the page
// number and total, and [CrossAxisNatural] as described above. Each is asserted where it
// is needed, so a simple element implements none of them.
package core
