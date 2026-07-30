// Package chart draws static charts as ordinary layout elements.
//
// Every chart here implements core.Element, so it composes with the rest of the
// layout vocabulary: put one in a row beside a table, inside a bordered panel, or
// in a column that paginates. Nothing about a chart is special to the engine.
//
// # Sizing
//
// A plot has no natural size. Asked how tall it wants to be, a chart can only
// answer "as tall as you like", so every chart type fills the box it is offered
// and the caller decides:
//
//	c.Item().Height(190).Element(&chart.Line{
//		Categories: []string{"Jan", "Feb", "Mar"},
//		Series:     []chart.Series{{Name: "Revenue", Values: []float64{31, 34, 32}}},
//	})
//
// # Why charts are not built from boxes
//
// The rest of the library composes nested containers, and a bar chart could
// almost be expressed that way — a bar is a rectangle. Axes cannot. Gridlines
// need to land on computed values, tick labels need to sit beside them, and a
// line series is a polyline through arbitrary points. So each chart type draws
// itself onto the canvas directly rather than delegating to child elements, which
// is exactly what core.Element exists to allow.
//
// # Layers
//
// The package separates what is computed from what is drawn, because the
// computation is where the errors live and it is testable without rendering
// anything:
//
//   - Ticks and Scale are pure arithmetic: choosing round axis values and mapping
//     data onto coordinates.
//   - Style holds the theme. A zero Style is valid and resolves to sensible
//     defaults, so a chart needs no configuration to look finished.
//   - Line, Bar and Pie assemble those into a drawing.
//
// # Not included
//
// Stacked series, dual axes, time-based category axes, scatter and radar plots,
// and axis titles are all absent. Each is a reasonable addition, and the shared
// pieces — Series, Style, Ticks, the plot frame — are shaped to accommodate them.
package chart
