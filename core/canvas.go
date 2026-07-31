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

	// Link makes a rectangle clickable.
	//
	// The rectangle is given in the element's own coordinates; the canvas converts
	// it to the absolute page position a PDF annotation requires. Rotated content
	// gets the enclosing axis-aligned box, since annotation rectangles cannot be
	// rotated.
	Link(pos Position, size Size, target LinkTarget)

	// Destination registers a named anchor at a point, for internal links and
	// outline entries to aim at.
	//
	// Names are document-wide and resolved after every page has been drawn, so a
	// link may point forwards to a destination that has not been reached yet.
	Destination(name string, pos Position)

	// Bookmark adds an entry to the document outline, nested by level, pointing at
	// a destination registered separately.
	Bookmark(title string, level int, destination string)

	// Tagger records the document's logical structure. Elements declare what their
	// content means; a canvas with tagging switched off ignores it.
	Tagger

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

// WithoutAnchors returns a canvas that draws normally but registers no
// destinations or outline entries.
//
// Headers and footers are redrawn on every sheet, so an anchor inside one would
// register the same name once per page. A PDF destination is single-valued by
// name, so the only coherent reading is that the first occurrence wins — but
// registering the rest anyway trips the duplicate-name guard, and the caller gets
// told they reused a name when they wrote exactly one.
//
// Links are deliberately left alone: a URL in a footer should be clickable on
// every page, not just the first.
func WithoutAnchors(c Canvas) Canvas { return withoutAnchors{Canvas: c} }

// withoutAnchors embeds the wrapped canvas, so every method except the two it
// overrides passes straight through and no future addition to Canvas can be
// silently dropped here.
type withoutAnchors struct {
	Canvas
}

func (withoutAnchors) Destination(string, Position) {}
func (withoutAnchors) Bookmark(string, int, string) {}

// WithoutTags returns a canvas that draws normally but records no structure, marking
// everything it draws as an artifact instead.
//
// This is what running furniture wants. A header repeated on forty sheets is not forty
// paragraphs a reader should announce; it is decoration, and the structure tree should
// have nothing to say about it. Page numbers are the clearest case — "Page 12 of 40"
// read out between every two paragraphs is worse than silence.
func WithoutTags(c Canvas) Canvas { return withoutTags{Canvas: c} }

type withoutTags struct {
	Canvas
}

func (w withoutTags) BeginMarked(Mark) { w.Canvas.BeginMarked(Mark{Role: RoleArtifact}) }

// LinkTarget is where a link leads.
//
// A link is either external or internal, never both, so this is one struct with
// two fields rather than an interface: the constructors below make the choice
// explicit, and URL wins if somebody sets both.
type LinkTarget struct {
	// URL is an external address, opened by the reader.
	URL string

	// Name is an internal destination registered by Canvas.Destination.
	Name string
}

// ExternalLink targets a URL.
func ExternalLink(url string) LinkTarget { return LinkTarget{URL: url} }

// InternalLink targets a named destination within the document.
func InternalLink(name string) LinkTarget { return LinkTarget{Name: name} }

// External reports whether the target is a URL.
func (t LinkTarget) External() bool { return t.URL != "" }

// Valid reports whether the target leads anywhere.
func (t LinkTarget) Valid() bool { return t.URL != "" || t.Name != "" }

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
