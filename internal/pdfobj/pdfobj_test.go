package pdfobj_test

import (
	"bytes"
	"regexp"
	"strconv"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

func TestNumAvoidsExponentNotation(t *testing.T) {
	// PDF has no exponent form, so a coordinate that strconv would render as
	// "1e-07" must come out as a plain decimal or a zero.
	for _, tc := range []struct {
		in   float64
		want string
	}{
		{0, "0"},
		{1, "1"},
		{-0.5, "-0.5"},
		{595.276, "595.276"},
		{0.0000001, "0"},
		{100.5000, "100.5"},
	} {
		if got := pdfobj.Num(tc.in); got != tc.want {
			t.Errorf("Num(%g) = %q, want %q", tc.in, got, tc.want)
		}
		if strings.ContainsAny(pdfobj.Num(tc.in), "eE") {
			t.Errorf("Num(%g) = %q contains exponent notation", tc.in, pdfobj.Num(tc.in))
		}
	}
}

func TestStringEscapesDelimiters(t *testing.T) {
	got := pdfobj.String(`a(b)c\d`)
	want := `(a\(b\)c\\d)`
	if got != want {
		t.Errorf("String = %q, want %q", got, want)
	}
}

func TestNameEscapesSpecialCharacters(t *testing.T) {
	if got := pdfobj.Name("Simple"); got != "/Simple" {
		t.Errorf("Name(Simple) = %q", got)
	}
	// A space or a delimiter inside a name would terminate it early.
	if got := pdfobj.Name("has space"); got != "/has#20space" {
		t.Errorf("Name(has space) = %q, want /has#20space", got)
	}
}

func TestDictPreservesInsertionOrder(t *testing.T) {
	d := pdfobj.NewDict().
		SetName("Type", "Page").
		SetInt("Count", 3).
		SetNum("Width", 1.5)

	want := "<< /Type /Page /Count 3 /Width 1.5 >>"
	if got := d.String(); got != want {
		t.Errorf("Dict = %q, want %q", got, want)
	}

	// Replacing a key must not move it, or output would stop being reproducible.
	d.SetInt("Count", 9)
	if got := d.String(); !strings.Contains(got, "/Type /Page /Count 9") {
		t.Errorf("after replacement Dict = %q, want Count to stay in place", got)
	}
}

func TestSerializeProducesConsistentXref(t *testing.T) {
	w := pdfobj.NewWriter(false)

	pages := w.Reserve()
	page := w.AddDict(pdfobj.NewDict().SetName("Type", "Page").SetRef("Parent", pages))
	w.Put(pages, pdfobj.NewDict().
		SetName("Type", "Pages").
		Set("Kids", pdfobj.Array(page.String())).
		SetInt("Count", 1).
		String())
	catalog := w.AddDict(pdfobj.NewDict().SetName("Type", "Catalog").SetRef("Pages", pages))

	data, err := w.Serialize(catalog, pdfobj.NewDict().SetString("Title", "T"))
	if err != nil {
		t.Fatal(err)
	}

	// Every xref entry must point at the "N 0 obj" line for that object;
	// a stale offset is the single most common way to produce a file that looks
	// fine but no reader will open.
	offsets := extractXrefOffsets(t, data)
	for i, off := range offsets {
		objNum := i + 1
		prefix := []byte(strconv.Itoa(objNum) + " 0 obj")
		if off < 0 || off+len(prefix) > len(data) {
			t.Fatalf("object %d offset %d is out of range", objNum, off)
		}
		if !bytes.HasPrefix(data[off:], prefix) {
			t.Errorf("xref offset for object %d points at %q, want %q",
				objNum, data[off:off+min(len(prefix), len(data)-off)], prefix)
		}
	}

	// The info dictionary is object 4, appended during serialisation.
	if len(offsets) != 4 {
		t.Errorf("got %d xref entries, want 4 (3 objects plus info)", len(offsets))
	}
	if !bytes.Contains(data, []byte("/Info 4 0 R")) {
		t.Error("trailer does not reference the info object")
	}
}

func TestSerializeIsRepeatable(t *testing.T) {
	build := func() *pdfobj.Writer {
		w := pdfobj.NewWriter(false)
		w.AddDict(pdfobj.NewDict().SetName("Type", "Catalog"))
		return w
	}

	w := build()
	info := pdfobj.NewDict().SetString("Title", "Same")

	first, err := w.Serialize(1, info)
	if err != nil {
		t.Fatal(err)
	}
	// Serialising twice must not append a second info object or shift offsets.
	second, err := w.Serialize(1, info)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(first, second) {
		t.Error("serialising the same writer twice produced different output")
	}
}

func TestSerializeRejectsUnfilledObject(t *testing.T) {
	w := pdfobj.NewWriter(false)
	catalog := w.AddDict(pdfobj.NewDict().SetName("Type", "Catalog"))
	w.Reserve() // reserved and never filled

	if _, err := w.Serialize(catalog, nil); err == nil {
		t.Error("expected an error for an object that was reserved but never filled")
	}
}

func TestStreamsCompressAboveThreshold(t *testing.T) {
	large := bytes.Repeat([]byte("content stream data "), 200)

	compressed := pdfobj.NewWriter(true)
	compressed.AddStream(pdfobj.NewDict(), large)
	withFlate, err := compressed.Serialize(compressed.Add(pdfobj.NewDict().SetName("Type", "Catalog").String()), nil)
	if err != nil {
		t.Fatal(err)
	}

	plain := pdfobj.NewWriter(false)
	plain.AddStream(pdfobj.NewDict(), large)
	without, err := plain.Serialize(plain.Add(pdfobj.NewDict().SetName("Type", "Catalog").String()), nil)
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(withFlate, []byte("/Filter /FlateDecode")) {
		t.Error("a large stream was not compressed")
	}
	if len(withFlate) >= len(without) {
		t.Errorf("compressed output (%d bytes) is not smaller than plain (%d bytes)",
			len(withFlate), len(without))
	}
}

func TestSmallStreamsStayUncompressed(t *testing.T) {
	w := pdfobj.NewWriter(true)
	w.AddStream(pdfobj.NewDict(), []byte("tiny"))
	catalog := w.AddDict(pdfobj.NewDict().SetName("Type", "Catalog"))

	data, err := w.Serialize(catalog, nil)
	if err != nil {
		t.Fatal(err)
	}

	// Below the threshold the zlib header costs more than it saves, and leaving
	// the stream readable makes generated files debuggable.
	if bytes.Contains(data, []byte("/Filter /FlateDecode")) {
		t.Error("a tiny stream was compressed")
	}
	if !bytes.Contains(data, []byte("tiny")) {
		t.Error("stream content is not present verbatim")
	}
}

var xrefLine = regexp.MustCompile(`(?m)^(\d{10}) 00000 n $`)

func extractXrefOffsets(t *testing.T, data []byte) []int {
	t.Helper()

	matches := xrefLine.FindAllSubmatch(data, -1)
	offsets := make([]int, 0, len(matches))
	for _, m := range matches {
		v, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatalf("bad xref offset %q", m[1])
		}
		offsets = append(offsets, v)
	}
	return offsets
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func TestStringBytesEscapesHighAndControlBytes(t *testing.T) {
	// Text reaches here already transcoded to WinAnsi, so accented characters
	// arrive as high bytes. They are written as octal escapes to keep the content
	// stream 7-bit clean and safe to move through any transport.
	got := pdfobj.StringBytes([]byte{'c', 'a', 'f', 0xE9})

	if want := `(caf\351)`; got != want {
		t.Errorf("StringBytes = %q, want %q", got, want)
	}
}

func TestStringBytesEscapesDelimitersAndNewlines(t *testing.T) {
	got := pdfobj.StringBytes([]byte("a(b)c\\d\ne\rf\x01"))

	want := `(a\(b\)c\\d\ne\rf\001)`
	if got != want {
		t.Errorf("StringBytes = %q, want %q", got, want)
	}
}

func TestPutBytesPanicsOnUnreservedObject(t *testing.T) {
	// Filling an object that was never reserved means the caller has lost track
	// of its own object numbers, which would silently corrupt the xref table.
	defer func() {
		if recover() == nil {
			t.Error("expected a panic when filling an unreserved object")
		}
	}()

	pdfobj.NewWriter(false).Put(pdfobj.Ref(42), "<< >>")
}

func TestEmptyDictSerialisesAsEmpty(t *testing.T) {
	if got := pdfobj.NewDict().String(); got != "<< >>" {
		t.Errorf("empty Dict = %q, want %q", got, "<< >>")
	}

	var nilDict *pdfobj.Dict
	if got := nilDict.String(); got != "<< >>" {
		t.Errorf("nil Dict = %q, want %q", got, "<< >>")
	}
}

func TestRefFormatting(t *testing.T) {
	if got := pdfobj.Ref(7).String(); got != "7 0 R" {
		t.Errorf("Ref(7) = %q, want %q", got, "7 0 R")
	}
	if pdfobj.Ref(0).Valid() {
		t.Error("Ref(0) must be invalid: object numbering starts at 1")
	}
	if !pdfobj.Ref(1).Valid() {
		t.Error("Ref(1) should be valid")
	}
}

func TestArrayHelpers(t *testing.T) {
	if got := pdfobj.IntArray([]int{1, 2, 3}); got != "[1 2 3]" {
		t.Errorf("IntArray = %q", got)
	}
	if got := pdfobj.NumArray(0, 1.5, 595.276); got != "[0 1.5 595.276]" {
		t.Errorf("NumArray = %q", got)
	}
	if got := pdfobj.Array(); got != "[]" {
		t.Errorf("empty Array = %q, want []", got)
	}
}

func TestSerializeRejectsMissingCatalog(t *testing.T) {
	if _, err := pdfobj.NewWriter(false).Serialize(0, nil); err == nil {
		t.Error("expected an error when no catalog reference is given")
	}
}

func TestTextStringKeepsASCIIReadable(t *testing.T) {
	// ASCII stays a literal string: readable in the output and half the bytes of
	// the hex form.
	if got := pdfobj.TextString("Quarterly Report"); got != "(Quarterly Report)" {
		t.Errorf("TextString = %q, want a literal string", got)
	}
	// Escaping still applies.
	if got := pdfobj.TextString("a(b)"); got != `(a\(b\))` {
		t.Errorf("TextString = %q, want the delimiters escaped", got)
	}
}

func TestTextStringEncodesNonASCIIAsUTF16(t *testing.T) {
	// A PDF text string is PDFDocEncoding or UTF-16BE with a byte-order mark, and
	// nothing else. Passing Go's UTF-8 through verbatim turns "Café" into "CafÃ©"
	// in every reader.
	got := pdfobj.TextString("Café")

	want := "<FEFF00430061006600E9>"
	if got != want {
		t.Errorf("TextString = %q, want %q", got, want)
	}
}

func TestTextStringHandlesAstralPlanes(t *testing.T) {
	// A rune above the basic multilingual plane needs a surrogate pair; emitting
	// its code point directly would produce an invalid UTF-16 sequence.
	got := pdfobj.TextString("\U0001F600")

	if got != "<FEFFD83DDE00>" {
		t.Errorf("TextString = %q, want a surrogate pair", got)
	}
}

func TestTextStringTreatsControlCharactersAsNonASCII(t *testing.T) {
	// A literal string would have to escape these; routing them through the hex
	// form avoids needing a second escaping rule.
	got := pdfobj.TextString("line\nbreak")

	if !strings.HasPrefix(got, "<FEFF") {
		t.Errorf("TextString = %q, want the hex form", got)
	}
}
