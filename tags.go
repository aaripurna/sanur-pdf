package sanur

import "github.com/aaripurna/sanur-pdf/core"

// Structure roles, re-exported so a caller need not import core to tag a document.
//
// Most roles are inferred — text is a paragraph, an image a figure, a rule decoration —
// so these are for the ones that cannot be. Headings are the important case: no amount
// of looking at a font size reveals whether text is a first- or a third-level heading,
// and an outline that is confidently wrong is worse for a screen reader than none at all.
//
// See Document.Tagged and Container.Tag.
const (
	// The heading levels.
	Heading1 = core.RoleHeading1
	Heading2 = core.RoleHeading2
	Heading3 = core.RoleHeading3
	Heading4 = core.RoleHeading4
	Heading5 = core.RoleHeading5
	Heading6 = core.RoleHeading6

	// Paragraph is ordinary prose, and what text is unless told otherwise.
	Paragraph = core.RoleParagraph

	// Section groups related content into a unit a reader can navigate.
	Section = core.RoleSection

	// Figure is a picture or a chart, and requires a description.
	Figure = core.RoleFigure

	// Caption labels a figure or a table.
	Caption = core.RoleCaption

	// Quote is a block quotation; Code is preformatted text whose line breaks matter.
	Quote = core.RoleQuote
	Code  = core.RoleCode

	// Table and its parts. A header cell is what makes a table navigable: it is how a
	// reader can say which column a figure sits under.
	Table       = core.RoleTable
	TableRow    = core.RoleTableRow
	TableHeader = core.RoleTableHeader
	TableCell   = core.RoleTableCell

	// List and its parts.
	List      = core.RoleList
	ListItem  = core.RoleListItem
	ListLabel = core.RoleListLabel
	ListBody  = core.RoleListBody

	// Artifact is content carrying no meaning, which a reader skips. Container's
	// Decoration method is the readable way to ask for it.
	Artifact = core.RoleArtifact
)

// HeadingLevel returns the role for a heading at the given depth, clamped to the six
// levels PDF defines.
//
// Useful when the depth is computed rather than written: a section renderer that nests
// need not spell out six cases.
func HeadingLevel(level int) core.Role { return core.Heading(level) }
