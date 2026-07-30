// Package elements holds the layout primitives every document is built from.
//
// # The measure/draw contract
//
// Each element implements core.Element: Measure reports what it would do with a
// given amount of space, Draw commits to it. Containers must honour two rules
// for the tree to lay out predictably.
//
// The first rule governs the space handed down. When a container draws a child,
// it passes the child's negotiated main-axis extent but the full cross-axis
// extent it has itself. A column gives each child the measured height of that
// child and the column's own full width; a row gives each child its allotted
// width and the row's full height.
//
// This asymmetry is what makes backgrounds and borders behave the way people
// expect. If children received only their measured width, a background inside a
// column would shrink to hug its text instead of spanning the column, and every
// full-width rule or banded table row would need an explicit stretch. Because
// the cross axis is passed through in full, an element that wants to fill it
// simply does, and one that wants to hug its content wraps itself in a
// constraint.
//
// The second rule is that Measure must be repeatable. Draw is only ever called
// after a Measure the parent accepted, and containers re-measure their children
// during Draw to recover the sizes they were promised. An element whose Measure
// answer changed between those two calls would draw itself into a box of the
// wrong size. Elements that track progress across pages — text, columns, page
// breaks — therefore advance that progress in Draw and never in Measure.
//
// # Pagination
//
// An element that cannot fit everything it holds returns a partial render: it
// draws what fits, and the document loop calls it again on a fresh page without
// resetting it. Wrapping instead means nothing useful fits at all and the
// element wants to start over on the next page. Containers propagate both: a
// column whose third child partially rendered is itself partially rendered, and
// resumes at that same child next page.
package elements
