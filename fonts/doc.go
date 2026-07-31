// Package fonts supplies the metrics and the encodings text layout needs, and describes
// a font to the PDF writer.
//
// There are two kinds of font here, and which one a document uses decides what that
// document is able to say.
//
// # Built-in fonts
//
// Helvetica and Courier are built in with exact Adobe metrics, in all four weight and
// slant combinations. They need no font file: a PDF names them and the reader supplies
// the outlines. They are addressed one byte at a time through [EncodeWinAnsi], which is
// Windows code page 1252 — so what they cannot say is not only the exotic. It is Polish,
// Czech, Turkish, Romanian, Vietnamese, all of Greek and all of Cyrillic, plus the arrows
// and mathematical operators ordinary documents want. Every one of those becomes a
// question mark.
//
// # Registered fonts
//
// [RegisterTrueType] and [LoadTrueTypeFile] parse a TrueType or OpenType file, reading
// metrics from the font's own tables. Such a font is embedded as a PDF composite font:
// two bytes per glyph, addressing the font's glyphs directly instead of through a
// 256-entry encoding table, which reaches everything the font has.
//
// Only the glyphs a document actually drew are embedded, with their identifiers left
// where the font put them. Arial is 773 kB on disk; a page of mixed Latin, Cyrillic and
// Greek from it comes to about 14 kB. OpenType fonts with PostScript outlines are
// embedded whole, since subsetting those means rewriting charstrings.
//
// Group faces into a [Family] so that asking for bold picks the real bold font rather
// than letting a reader synthesise one by smearing the regular outlines.
//
// # The registry
//
// A [Registry] maps names to faces, pre-populated with the built-in ones, so a font can
// be named from a theme file rather than threaded through the code. It is safe for
// concurrent use: register at startup, read from as many goroutines as you like.
//
// # Talking to the writer
//
// The render package never learns where a face came from. Both kinds produce a
// [FontProgram] describing themselves, and a registered font additionally implements
// [GlyphSource] so the writer can map runes to glyphs and ask for a subset once it knows
// which ones were used.
package fonts
