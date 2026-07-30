package core

// Canvas is the drawing surface elements paint onto.
//
// Two implementations exist: one writes PDF content-stream operators, and one
// discards everything. The discarding canvas lets the engine run a full Draw
// pass purely to see where things land — that is how page counting works
// without producing throwaway output.
//
// The coordinate space is top-left origin with Y increasing downwards, and each
// element draws as though it sits at (0, 0). Containers use Translate to move
// the origin for their children, which keeps every element's own arithmetic
// local and relative.
type Canvas interface {
	// Save pushes the current transform and clip.
	Save()

	// Restore pops the state pushed by the matching Save.
	Restore()

	// Translate shifts the origin by p.
	Translate(p Position)

	// Rotate turns the coordinate system clockwise by the given degrees.
	Rotate(degrees float64)

	// ClipRect restricts drawing to the given rectangle until the next Restore.
	ClipRect(pos Position, size Size)

	// DrawRect fills a rectangle.
	DrawRect(pos Position, size Size, fill Color)

	// DrawRoundedRect fills a rectangle with a uniform corner radius.
	DrawRoundedRect(pos Position, size Size, radius float64, fill Color)

	// DrawPath paints an arbitrary outline, filled, stroked, or both according to
	// the style.
	//
	// This is the primitive the rectangle and line calls above cannot express:
	// arcs, polygons, dashed rules and stroke joins. Anything that is not an
	// axis-aligned box goes through here.
	DrawPath(path *Path, style PathStyle)

	// DrawLine strokes a straight line.
	DrawLine(from, to Position, stroke Color, width float64)

	// DrawText draws a single line of text with its left end on the baseline at
	// pos. Callers are responsible for having placed the baseline; the canvas
	// applies no ascent offset of its own.
	DrawText(text string, pos Position, style TextStyle)

	// DrawImage draws a decoded image scaled into the given box.
	DrawImage(img Image, pos Position, size Size)

	// Fail records an error encountered while drawing.
	//
	// Draw has no error return by design: threading one through every container
	// would add a check to each child loop for a case that essentially never
	// happens mid-page. Instead the canvas collects failures and the document
	// loop reports them once, after the page is complete.
	Fail(err error)

	// Err returns the first error recorded via Fail, if any.
	Err() error
}

// Image is an encoded raster image ready to embed. Sanur stores the original
// compressed bytes and hands them to the PDF writer untouched, so a JPEG in the
// input is a JPEG in the output with no requantization.
type Image struct {
	// Key deduplicates identical images across pages.
	Key string

	// Format is "jpeg" or "png".
	Format string

	// Data is the encoded file content.
	Data []byte

	// PixelWidth and PixelHeight are the intrinsic dimensions.
	PixelWidth  int
	PixelHeight int
}

// AspectRatio returns width divided by height, or zero for a degenerate image.
func (i Image) AspectRatio() float64 {
	if i.PixelHeight == 0 {
		return 0
	}
	return float64(i.PixelWidth) / float64(i.PixelHeight)
}
