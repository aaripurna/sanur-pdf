package core

// Role is what a piece of content means, as opposed to what it looks like.
//
// A PDF says where ink goes and nothing else. A heading is text that happens to be
// large and bold; a table is lines that happen to form a grid. Nothing in the file
// says so, which is why an ordinary PDF is unreadable to a screen reader, unusable
// for reflow on a small display, and impossible to convert to anything structured.
// Tagging is the parallel structure that carries the meaning, and a Role is one node
// in it.
//
// The names are PDF's own standard structure types, so they need no role map. A role
// this package does not name can still be used — the writer maps an unknown role onto
// the nearest standard type — but the standard ones are what every reader understands.
type Role string

// The structure types a document is built from.
//
// This is deliberately not the whole of PDF's vocabulary. It is the set a report, an
// invoice or an article actually needs, and each entry earns its place by changing how
// assistive technology behaves.
const (
	// RoleNone leaves content untagged, which the writer treats as an artifact.
	RoleNone Role = ""

	// RoleDocument is the root of a document's structure.
	RoleDocument Role = "Document"

	// RoleSection groups related content. A reader offers it as a navigable unit.
	RoleSection Role = "Sect"

	// RoleParagraph is ordinary prose, and the default for text.
	RoleParagraph Role = "P"

	// The heading levels. These cannot be inferred: a heading is text that happens to
	// be large, and guessing from a font size would produce a document whose outline
	// is confidently wrong — worse for a screen reader than no outline at all. So a
	// caller declares them, and the writer checks the nesting.
	RoleHeading1 Role = "H1"
	RoleHeading2 Role = "H2"
	RoleHeading3 Role = "H3"
	RoleHeading4 Role = "H4"
	RoleHeading5 Role = "H5"
	RoleHeading6 Role = "H6"

	// RoleFigure is a picture or a chart. It requires alternative text, since a
	// figure with nothing to read out is exactly the gap tagging exists to close.
	RoleFigure Role = "Figure"

	// RoleTable and its parts. A table's meaning is entirely in which cell heads
	// which column, and none of that survives in the drawn rules.
	RoleTable       Role = "Table"
	RoleTableRow    Role = "TR"
	RoleTableHeader Role = "TH"
	RoleTableCell   Role = "TD"

	// RoleList and its parts. LI holds a Lbl for the bullet and a LBody for the text.
	RoleList      Role = "L"
	RoleListItem  Role = "LI"
	RoleListLabel Role = "Lbl"
	RoleListBody  Role = "LBody"

	// RoleLink wraps the content of a hyperlink, so a reader can offer the link by
	// the words it is on rather than by its address, and holds a reference to the
	// annotation a click follows.
	RoleLink Role = "Link"

	// RoleCaption labels a figure or a table.
	RoleCaption Role = "Caption"

	// RoleQuote is a block quotation.
	RoleQuote Role = "BlockQuote"

	// RoleCode is preformatted text, where the line breaks are meaningful.
	RoleCode Role = "Code"

	// RoleSpan marks a run inside a paragraph that differs in some way the structure
	// should record — a change of language, most usefully.
	RoleSpan Role = "Span"

	// RoleArtifact marks content that carries no meaning: a rule, a background, a
	// running header, a watermark. It is not part of the structure at all.
	//
	// Marking these matters as much as tagging the content. A conforming document has
	// no third category, so anything left unmarked is content a reader must announce —
	// and a decorative rule announced between every paragraph makes a document worse
	// than an untagged one.
	RoleArtifact Role = "Artifact"
)

// HeadingLevel returns the heading level of a role, or zero if it is not a heading.
func (r Role) HeadingLevel() int {
	switch r {
	case RoleHeading1:
		return 1
	case RoleHeading2:
		return 2
	case RoleHeading3:
		return 3
	case RoleHeading4:
		return 4
	case RoleHeading5:
		return 5
	case RoleHeading6:
		return 6
	}
	return 0
}

// Heading returns the role for a heading level, clamped to the six PDF defines.
//
// Clamping rather than failing, because a document nested seven deep is a document
// with a structure problem, not a reason to refuse to generate — and H6 is the honest
// approximation of "deeper than the format can say".
func Heading(level int) Role {
	switch {
	case level <= 1:
		return RoleHeading1
	case level >= 6:
		return RoleHeading6
	}
	return Role("H" + string(rune('0'+level)))
}

// Mark describes one structure element: what it means, and what a reader should say
// about it beyond the text it contains.
type Mark struct {
	// Role is the structure type.
	Role Role

	// Alt is alternative text, read out in place of the content. It is required on a
	// figure and ignored elsewhere unless the content genuinely cannot be read.
	Alt string

	// ActualText is what the content really says, for when the glyphs do not spell
	// it: a decorative capital, a logo standing in for a word, text drawn as paths.
	ActualText string

	// Lang overrides the document language for this element, so a quotation in
	// another language is spoken with the right voice.
	Lang string

	// Title is a human-readable label for the element, shown in a reader's structure
	// panel and useful on a section.
	Title string

	// Scope says which way a table header applies: down its column or across its row.
	// It is required on a header cell — a reader that knows a cell is a heading but not
	// what it heads has gained very little — and defaults to the column.
	Scope Scope
}

// Scope is the direction a table header cell applies in.
type Scope string

const (
	// ScopeColumn heads the cells below, which is what a table's top row usually does.
	ScopeColumn Scope = "Column"

	// ScopeRow heads the cells beside it, for a table whose labels run down the left.
	ScopeRow Scope = "Row"

	// ScopeBoth heads both, for the corner cell of a table labelled on two edges.
	ScopeBoth Scope = "Both"
)

// Tagger is the part of a Canvas that records structure.
//
// It is separate from Canvas so that the two implementations can share it and a
// wrapper cannot silently drop it, and because a canvas that discards its output still
// has to accept the calls: the counting pass draws the whole document.
type Tagger interface {
	// BeginMarked opens a structure element and the marked-content sequence that
	// belongs to it. Calls nest, and each must be matched by EndMarked.
	//
	// Elements call this unconditionally; a canvas with tagging switched off does
	// nothing. That is what keeps the decision in one place instead of making every
	// element ask whether it should bother.
	BeginMarked(mark Mark)

	// EndMarked closes the most recently opened element.
	EndMarked()
}
