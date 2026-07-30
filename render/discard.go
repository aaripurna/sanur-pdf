package render

import "github.com/aaripurna/sanur-pdf/core"

// DiscardCanvas satisfies core.Canvas and throws every operation away.
//
// Its purpose is to let the engine run a complete Draw pass purely for its side
// effects on element state. Counting pages needs exactly that: elements only
// advance their rendering progress in Draw, so the number of pages a document
// occupies cannot be known without drawing it — but the output of that first
// pass is worthless, because page-number labels do not yet know the total. So
// the document is drawn once onto a discard canvas to learn the count, then
// again for real.
//
// Using the same code path for both passes is what keeps the count honest: a
// counting pass that approximated layout differently would produce a total that
// disagreed with the document it describes.
type DiscardCanvas struct {
	err error
}

// NewDiscardCanvas creates a canvas that draws nothing.
func NewDiscardCanvas() *DiscardCanvas { return &DiscardCanvas{} }

func (c *DiscardCanvas) Save()                             {}
func (c *DiscardCanvas) Restore()                          {}
func (c *DiscardCanvas) Translate(core.Position)           {}
func (c *DiscardCanvas) Rotate(float64)                    {}
func (c *DiscardCanvas) ClipRect(core.Position, core.Size) {}

func (c *DiscardCanvas) DrawRect(core.Position, core.Size, core.Color) {}
func (c *DiscardCanvas) DrawRoundedRect(core.Position, core.Size, float64, core.Color) {
}
func (c *DiscardCanvas) DrawPath(*core.Path, core.PathStyle)                        {}
func (c *DiscardCanvas) DrawLine(core.Position, core.Position, core.Color, float64) {}
func (c *DiscardCanvas) DrawText(string, core.Position, core.TextStyle)             {}
func (c *DiscardCanvas) DrawImage(core.Image, core.Position, core.Size)             {}

// Links, anchors and bookmarks are discarded along with everything else. The
// counting pass exists only to learn how many pages there are; collecting
// annotations here would register every one twice, since the real pass draws the
// same document again.
func (c *DiscardCanvas) Link(core.Position, core.Size, core.LinkTarget) {}
func (c *DiscardCanvas) Destination(string, core.Position)              {}
func (c *DiscardCanvas) Bookmark(string, int, string)                   {}

// Fail records an error. Failures are still collected during a discarded pass:
// a font that cannot be embedded or a malformed image is worth reporting before
// the second pass repeats the same work.
func (c *DiscardCanvas) Fail(err error) {
	if c.err == nil && err != nil {
		c.err = err
	}
}

func (c *DiscardCanvas) Err() error { return c.err }

var _ core.Canvas = (*DiscardCanvas)(nil)
