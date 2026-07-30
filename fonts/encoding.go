package fonts

// WinAnsiEncoding is the single-byte encoding the built-in fonts use.
//
// PDF's simple fonts address glyphs by byte, so text drawn in one has to be
// transcoded from Go's UTF-8 on the way out. WinAnsi is the only sensible choice for
// the standard-14 faces: it is a superset of Latin-1 with the typographic punctuation
// real documents use, and every one of those faces ships metrics for it.
//
// It is also the ceiling on what a built-in font can say. WinAnsi is Windows code
// page 1252, so it stops at Western Europe — no Polish, Czech, Turkish, Romanian or
// Vietnamese, let alone Greek or Cyrillic. Anything beyond it needs a registered
// TrueType or OpenType font, which sanur embeds as a composite font addressed by
// glyph identifier; see composite.go.
var winAnsiToRune [256]rune

// runeToWinAnsi is the reverse lookup, built from winAnsiToRune so the two can
// never disagree.
var runeToWinAnsi map[rune]byte

// Undefined marks the WinAnsi code points with no assigned glyph.
const undefinedRune = rune(0)

// substituteByte is emitted for runes outside the encoding. A question mark is
// visible in the output, which is the point: silently dropping characters hides
// the fact that a document is missing content.
const substituteByte = byte('?')

func init() {
	// Codes 0x20..0x7E are plain ASCII, and 0xA0..0xFF match Latin-1.
	for c := 0x20; c <= 0x7E; c++ {
		winAnsiToRune[c] = rune(c)
	}
	for c := 0xA0; c <= 0xFF; c++ {
		winAnsiToRune[c] = rune(c)
	}

	// 0x80..0x9F is where WinAnsi diverges from Latin-1, holding the
	// punctuation and accented capitals that Latin-1 leaves as control codes.
	for code, r := range map[byte]rune{
		0x80: '€', // euro
		0x82: '‚', // single low-9 quote
		0x83: 'ƒ', // florin
		0x84: '„', // double low-9 quote
		0x85: '…', // ellipsis
		0x86: '†', // dagger
		0x87: '‡', // double dagger
		0x88: 'ˆ', // circumflex
		0x89: '‰', // per mille
		0x8A: 'Š', // S caron
		0x8B: '‹', // single left angle quote
		0x8C: 'Œ', // OE ligature
		0x8E: 'Ž', // Z caron
		0x91: '‘', // left single quote
		0x92: '’', // right single quote
		0x93: '“', // left double quote
		0x94: '”', // right double quote
		0x95: '•', // bullet
		0x96: '–', // en dash
		0x97: '—', // em dash
		0x98: '˜', // small tilde
		0x99: '™', // trademark
		0x9A: 'š', // s caron
		0x9B: '›', // single right angle quote
		0x9C: 'œ', // oe ligature
		0x9E: 'ž', // z caron
		0x9F: 'Ÿ', // Y dieresis
	} {
		winAnsiToRune[code] = r
	}

	runeToWinAnsi = make(map[rune]byte, 256)
	for c, r := range winAnsiToRune {
		if r != undefinedRune {
			runeToWinAnsi[r] = byte(c)
		}
	}
}

// EncodeWinAnsi transcodes a UTF-8 string to WinAnsi bytes, substituting '?'
// for runes the encoding cannot represent.
func EncodeWinAnsi(s string) []byte {
	out := make([]byte, 0, len(s))
	for _, r := range s {
		if b, ok := runeToWinAnsi[r]; ok {
			out = append(out, b)
			continue
		}
		out = append(out, substituteByte)
	}
	return out
}

// WinAnsiCode returns the byte for r and whether the encoding covers it.
func WinAnsiCode(r rune) (byte, bool) {
	b, ok := runeToWinAnsi[r]
	return b, ok
}

// RuneForWinAnsiCode returns the rune a WinAnsi byte maps to, or 0 if the code
// is unassigned.
//
// It is the inverse of WinAnsiCode, and exists so that the two directions of the
// table are derived from one source and cannot drift apart. Nothing in sanur reads it
// any more — the widths of the built-in faces are indexed by code directly — but it
// is the only way for a caller to find out what a byte in a simple font's string
// means.
func RuneForWinAnsiCode(code byte) rune {
	return winAnsiToRune[code]
}
