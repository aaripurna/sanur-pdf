// Package pdfobj serialises the PDF object model: indirect objects, streams,
// the cross-reference table and the trailer.
//
// It knows nothing about layout, pages or fonts — it only turns dictionaries and
// byte streams into a valid PDF file. Keeping that boundary sharp means the
// layout engine can be tested without producing files, and the file format can
// be verified without running a layout.
package pdfobj

import (
	"fmt"
	"strconv"
	"strings"
	"unicode/utf16"
)

// Ref is a reference to an indirect object, numbered from 1.
type Ref int

// String renders the reference as it appears inside a dictionary.
func (r Ref) String() string { return strconv.Itoa(int(r)) + " 0 R" }

// Valid reports whether the reference points at a real object.
func (r Ref) Valid() bool { return r > 0 }

// Dict is an ordered PDF dictionary.
//
// Order is preserved rather than using a Go map because map iteration is
// randomised: identical documents would otherwise serialise to different bytes
// on every run, which breaks golden-file tests and reproducible builds for no
// benefit.
type Dict struct {
	keys   []string
	values map[string]string
}

// NewDict creates an empty dictionary.
func NewDict() *Dict {
	return &Dict{values: map[string]string{}}
}

// Set adds or replaces an entry. The key is given without its leading slash;
// the value must already be formatted as a PDF object.
func (d *Dict) Set(key, value string) *Dict {
	if _, exists := d.values[key]; !exists {
		d.keys = append(d.keys, key)
	}
	d.values[key] = value
	return d
}

// SetName sets a name-valued entry.
func (d *Dict) SetName(key, name string) *Dict { return d.Set(key, Name(name)) }

// SetInt sets an integer-valued entry.
func (d *Dict) SetInt(key string, v int) *Dict { return d.Set(key, strconv.Itoa(v)) }

// SetNum sets a real-valued entry.
func (d *Dict) SetNum(key string, v float64) *Dict { return d.Set(key, Num(v)) }

// SetRef sets an indirect-reference entry.
func (d *Dict) SetRef(key string, r Ref) *Dict { return d.Set(key, r.String()) }

// SetString sets a literal-string entry, for values that are ASCII by
// specification such as URIs and dates.
func (d *Dict) SetString(key, s string) *Dict { return d.Set(key, String(s)) }

// SetTextString sets an entry holding human-readable text, encoded so that
// non-ASCII characters survive.
func (d *Dict) SetTextString(key, s string) *Dict { return d.Set(key, TextString(s)) }

// Has reports whether the key is present.
func (d *Dict) Has(key string) bool {
	_, ok := d.values[key]
	return ok
}

// Len returns the number of entries.
func (d *Dict) Len() int { return len(d.keys) }

// String serialises the dictionary.
func (d *Dict) String() string {
	if d == nil || len(d.keys) == 0 {
		return "<< >>"
	}
	var b strings.Builder
	b.WriteString("<<")
	for _, k := range d.keys {
		b.WriteString(" /")
		b.WriteString(k)
		b.WriteByte(' ')
		b.WriteString(d.values[k])
	}
	b.WriteString(" >>")
	return b.String()
}

// Name formats a PDF name, escaping the characters that would otherwise
// terminate it.
func Name(s string) string {
	var b strings.Builder
	b.WriteByte('/')
	for i := 0; i < len(s); i++ {
		c := s[i]
		// Regular characters pass through; delimiters, whitespace and '#'
		// itself must use the #xx hex form.
		if c > 0x20 && c < 0x7F && !strings.ContainsRune("()<>[]{}/%#", rune(c)) {
			b.WriteByte(c)
			continue
		}
		fmt.Fprintf(&b, "#%02X", c)
	}
	return b.String()
}

// Num formats a real number.
//
// PDF has no exponent notation, so strconv's 'g' and 'e' forms are unusable and
// a fixed three decimal places is used instead — well beyond the precision of
// anything measured in points. Trailing zeros are trimmed to keep content
// streams compact, since coordinates dominate the size of a drawing-heavy file.
func Num(v float64) string {
	// Avoid emitting "-0", which some older parsers mishandle.
	if v == 0 {
		return "0"
	}
	s := strconv.FormatFloat(v, 'f', 3, 64)
	s = strings.TrimRight(s, "0")
	s = strings.TrimSuffix(s, ".")
	if s == "" || s == "-" {
		return "0"
	}
	return s
}

// String formats a literal string, escaping the delimiters and backslash.
func String(s string) string {
	var b strings.Builder
	b.WriteByte('(')
	for i := 0; i < len(s); i++ {
		switch c := s[i]; c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		case '\t':
			b.WriteString(`\t`)
		default:
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

// TextString formats human-readable text: a document title, an author, an
// outline entry.
//
// These are not the same as the strings inside a content stream. A PDF text
// string is either PDFDocEncoding — near enough Latin-1 — or UTF-16BE introduced
// by a byte-order mark, and nothing else. Writing Go's UTF-8 through verbatim
// works only for ASCII and turns every accented character into mojibake: a title
// of "Café" arrives as "CafÃ©".
//
// ASCII is emitted as a literal string because it is readable in the output and
// costs half the bytes; anything else becomes a UTF-16BE hex string, which covers
// the whole of Unicode. Latin-1 would cover more than ASCII for fewer bytes, but
// choosing between three encodings to save a few bytes on a title is not worth
// the extra path.
func TextString(s string) string {
	if isASCII(s) {
		return String(s)
	}

	var b strings.Builder
	b.WriteByte('<')
	// The byte-order mark is what tells a reader this is UTF-16 rather than
	// PDFDocEncoding.
	b.WriteString("FEFF")

	for _, r := range s {
		// Runes outside the basic multilingual plane need a surrogate pair, which
		// is what utf16.Encode produces.
		for _, unit := range utf16.Encode([]rune{r}) {
			fmt.Fprintf(&b, "%04X", unit)
		}
	}

	b.WriteByte('>')
	return b.String()
}

func isASCII(s string) bool {
	for i := 0; i < len(s); i++ {
		if s[i] > 0x7E || s[i] < 0x20 {
			return false
		}
	}
	return true
}

// StringBytes formats already-encoded bytes as a literal string. Text is
// transcoded to a single-byte encoding before it reaches here, so the bytes are
// written through unchanged apart from escaping.
func StringBytes(data []byte) string {
	var b strings.Builder
	b.WriteByte('(')
	for _, c := range data {
		switch c {
		case '(', ')', '\\':
			b.WriteByte('\\')
			b.WriteByte(c)
		case '\r':
			b.WriteString(`\r`)
		case '\n':
			b.WriteString(`\n`)
		default:
			if c < 0x20 || c > 0x7E {
				// Non-printable and high bytes use octal so the content stream
				// stays 7-bit clean and survives any transport.
				fmt.Fprintf(&b, `\%03o`, c)
				continue
			}
			b.WriteByte(c)
		}
	}
	b.WriteByte(')')
	return b.String()
}

// HexString formats bytes as a hexadecimal string.
//
// This is the form composite text uses. Its operands are two-byte glyph numbers
// rather than characters, so most of them fall outside the printable range and a
// literal string would spend four bytes on an octal escape for nearly every one.
// Hex is exactly two characters per byte regardless, and it needs no escaping at
// all, which means no way to get the escaping wrong.
func HexString(data []byte) string {
	const digits = "0123456789ABCDEF"

	out := make([]byte, 0, len(data)*2+2)
	out = append(out, '<')
	for _, c := range data {
		out = append(out, digits[c>>4], digits[c&0x0F])
	}
	return string(append(out, '>'))
}

// UTF16BEHex formats runes as the hex string a CMap uses to name a character,
// including the angle brackets.
//
// The encoding is UTF-16BE, so anything outside the basic multilingual plane occupies
// two units as a surrogate pair — an emoji or a rarer CJK ideograph is four bytes here,
// not two.
//
// Several runes are allowed because one glyph can stand for more than one character: an
// Arabic lam-alef ligature is a single mark on the page and two letters in the text.
func UTF16BEHex(runes ...rune) string {
	var b strings.Builder
	b.WriteByte('<')
	for _, unit := range utf16.Encode(runes) {
		fmt.Fprintf(&b, "%04X", unit)
	}
	b.WriteByte('>')
	return b.String()
}

// Array formats a PDF array from pre-formatted elements.
func Array(items ...string) string {
	return "[" + strings.Join(items, " ") + "]"
}

// IntArray formats an array of integers.
func IntArray(items []int) string {
	parts := make([]string, len(items))
	for i, v := range items {
		parts[i] = strconv.Itoa(v)
	}
	return Array(parts...)
}

// NumArray formats an array of reals.
func NumArray(items ...float64) string {
	parts := make([]string, len(items))
	for i, v := range items {
		parts[i] = Num(v)
	}
	return Array(parts...)
}
