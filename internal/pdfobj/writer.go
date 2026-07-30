package pdfobj

import (
	"bytes"
	"compress/zlib"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strconv"
)

// Version is the PDF specification level sanur targets. 1.7 is ISO 32000-1 and
// is universally supported; nothing sanur emits needs a later version.
const Version = "1.7"

// CompressionThreshold is the stream size above which Flate compression is
// applied. Below it, the zlib header and dictionary cost more bytes than the
// compression saves, and leaving small streams readable makes generated files
// far easier to debug by eye.
const CompressionThreshold = 256

// Writer accumulates indirect objects and serialises them into a PDF file.
//
// Object numbers can be reserved before their contents are known, which is what
// makes the page tree expressible: a page needs to reference its parent, and the
// parent needs to list its children, so one of the two must be forward-declared.
type Writer struct {
	// bodies is indexed by object number minus one. A nil entry is a reserved
	// but unfilled object, which Serialize rejects.
	bodies [][]byte

	// compress controls Flate encoding of streams.
	compress bool
}

// NewWriter creates a writer. When compress is false, streams are stored
// uncompressed, which is useful when inspecting output during development.
func NewWriter(compress bool) *Writer {
	return &Writer{compress: compress}
}

// Reserve allocates an object number without contents.
func (w *Writer) Reserve() Ref {
	w.bodies = append(w.bodies, nil)
	return Ref(len(w.bodies))
}

// Put fills a previously reserved object.
func (w *Writer) Put(ref Ref, body string) {
	w.PutBytes(ref, []byte(body))
}

// PutBytes fills a previously reserved object with raw bytes.
func (w *Writer) PutBytes(ref Ref, body []byte) {
	idx := int(ref) - 1
	if idx < 0 || idx >= len(w.bodies) {
		panic(fmt.Sprintf("pdfobj: object %d was never reserved", ref))
	}
	w.bodies[idx] = body
}

// Add appends a new object and returns its reference.
func (w *Writer) Add(body string) Ref {
	ref := w.Reserve()
	w.Put(ref, body)
	return ref
}

// AddDict appends a dictionary object.
func (w *Writer) AddDict(d *Dict) Ref {
	return w.Add(d.String())
}

// AddStream appends a stream object, compressing the data when worthwhile and
// filling in /Length and /Filter itself.
//
// Passing a dictionary that already carries a /Filter (an embedded JPEG, say)
// suppresses compression: re-deflating an already-compressed image wastes time
// and usually grows the file.
func (w *Writer) AddStream(d *Dict, data []byte) Ref {
	ref := w.Reserve()
	w.PutStream(ref, d, data)
	return ref
}

// PutStream fills a reserved object with a stream.
func (w *Writer) PutStream(ref Ref, d *Dict, data []byte) {
	if d == nil {
		d = NewDict()
	}

	payload := data
	if w.compress && !d.Has("Filter") && len(data) >= CompressionThreshold {
		if deflated, err := deflate(data); err == nil && len(deflated) < len(data) {
			payload = deflated
			d.SetName("Filter", "FlateDecode")
		}
	}
	d.SetInt("Length", len(payload))

	var buf bytes.Buffer
	buf.WriteString(d.String())
	buf.WriteString("\nstream\n")
	buf.Write(payload)
	buf.WriteString("\nendstream")
	w.PutBytes(ref, buf.Bytes())
}

func deflate(data []byte) ([]byte, error) {
	var buf bytes.Buffer
	zw, err := zlib.NewWriterLevel(&buf, zlib.BestCompression)
	if err != nil {
		return nil, err
	}
	if _, err := zw.Write(data); err != nil {
		return nil, err
	}
	if err := zw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// Serialize produces the finished PDF file.
//
// root must reference the document catalog and info may be nil. Serialize does
// not mutate the writer, so it can be called more than once and always yields
// identical bytes for identical input.
func (w *Writer) Serialize(root Ref, info *Dict) ([]byte, error) {
	if !root.Valid() {
		return nil, fmt.Errorf("pdfobj: no document catalog")
	}
	for i, body := range w.bodies {
		if body == nil {
			return nil, fmt.Errorf("pdfobj: object %d was reserved but never filled", i+1)
		}
	}

	// The info dictionary becomes one more indirect object. It is appended to a
	// local copy of the object list rather than to the writer itself, so that
	// serialising twice cannot accumulate duplicate info objects.
	bodies := w.bodies
	var infoRef Ref
	if info != nil && info.Len() > 0 {
		bodies = append(append([][]byte(nil), w.bodies...), []byte(info.String()))
		infoRef = Ref(len(bodies))
	}

	var buf bytes.Buffer

	// The header's second line must contain bytes above 127 so that tools
	// transferring the file treat it as binary rather than text and refrain from
	// translating line endings, which would corrupt every stream.
	buf.WriteString("%PDF-" + Version + "\n")
	buf.Write([]byte{'%', 0xE2, 0xE3, 0xCF, 0xD3, '\n'})

	// Offsets are byte positions of each object from the start of the file, and
	// must be recorded as the file is written since they depend on every
	// preceding object's serialised length.
	offsets := make([]int, len(bodies))
	for i, body := range bodies {
		offsets[i] = buf.Len()
		buf.WriteString(strconv.Itoa(i + 1))
		buf.WriteString(" 0 obj\n")
		buf.Write(body)
		buf.WriteString("\nendobj\n")
	}

	xrefOffset := buf.Len()
	buf.WriteString("xref\n0 ")
	buf.WriteString(strconv.Itoa(len(bodies) + 1))
	buf.WriteByte('\n')

	// Entry zero heads the free list and is fixed by the specification. Every
	// xref line is exactly 20 bytes including its trailing space and newline.
	buf.WriteString("0000000000 65535 f \n")
	for _, off := range offsets {
		fmt.Fprintf(&buf, "%010d 00000 n \n", off)
	}

	fileID := documentID(bodies)
	trailer := NewDict().
		SetInt("Size", len(bodies)+1).
		SetRef("Root", root).
		Set("ID", Array("<"+fileID+">", "<"+fileID+">"))
	if infoRef.Valid() {
		trailer.SetRef("Info", infoRef)
	}

	buf.WriteString("trailer\n")
	buf.WriteString(trailer.String())
	buf.WriteString("\nstartxref\n")
	buf.WriteString(strconv.Itoa(xrefOffset))
	buf.WriteString("\n%%EOF\n")

	return buf.Bytes(), nil
}

// documentID derives the file identifier from the object contents.
//
// The specification wants an ID that distinguishes revisions of a document.
// Hashing the content gives that while keeping generation deterministic: the
// same document always produces the same bytes, which a random or
// timestamp-based ID would prevent.
func documentID(bodies [][]byte) string {
	h := sha256.New()
	for _, b := range bodies {
		h.Write(b)
	}
	return hex.EncodeToString(h.Sum(nil)[:16])
}
