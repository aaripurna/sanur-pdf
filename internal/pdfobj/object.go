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

// SetString sets a literal-string entry.
func (d *Dict) SetString(key, s string) *Dict { return d.Set(key, String(s)) }

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
