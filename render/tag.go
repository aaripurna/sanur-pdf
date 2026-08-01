package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// This file builds the parallel structure a tagged PDF carries alongside its ink.
//
// A page stream says where marks go. The structure tree says what those marks mean, and
// the two are joined by a number: a marked-content sequence in the stream carries an
// MCID, and a structure element points back at the page and that MCID. Getting the
// correspondence wrong is the whole difficulty — a tree that looks right but whose
// numbers point at the wrong content is worse than no tree, because software believes it.
//
// Three things have to line up:
//
//   - The content stream wraps each piece of content in "/P <</MCID n>> BDC ... EMC",
//     with n unique within the page.
//   - A structure element records /Pg (the page) and /K (the MCID), so it can be found.
//   - The parent tree maps each page's MCIDs back to their structure elements, so a
//     reader that starts from a point on the page can find its place in the structure.
//
// The last of those is the one nothing checks and everything depends on.

// structElem is one node of the structure tree.
type structElem struct {
	mark core.Mark

	// ref is reserved when the element is created, because a child records its parent
	// and the parent's own dictionary is not written until its children are known.
	ref pdfobj.Ref

	parent   *structElem
	children []*structElem

	// content is the marked-content this element owns directly: the page it was drawn
	// on and the MCID within it. A grouping element has none.
	content []contentRef

	// annotations are the link annotations this element owns, referenced from the tree
	// so that a reader can find the link from the words it sits on.
	annotations []pdfobj.Ref
}

// contentRef is one marked-content sequence: which page, and which sequence on it.
type contentRef struct {
	page int
	mcid int
}

// tagState is a document's structure, accumulated as pages are drawn.
type tagState struct {
	// enabled is false unless the document asked to be tagged, in which case every
	// call here is a no-op and the output is byte-identical to an untagged run.
	enabled bool

	// language is the document's natural language, as a BCP 47 tag.
	language string

	root  *structElem
	stack []*structElem

	// mcidByPage counts sequences per page, since MCIDs are numbered within a page.
	mcidByPage map[int]int

	// parents maps a page to the structure element owning each of its MCIDs, indexed
	// by MCID. This becomes the parent tree.
	parents map[int][]*structElem

	// annotationOwners maps a parent-tree key to the element owning an annotation.
	//
	// An annotation needs the same two-way link a piece of content does. The structure
	// reaches the annotation through an object reference; the annotation reaches the
	// structure through a key into this tree, and software validates the second
	// direction — a link with only the first is reported as untagged.
	annotationOwners map[int]*structElem

	// headings records the heading levels used, in order, so a document that skips
	// from H1 to H3 can be reported rather than shipped.
	headings []int

	// missingAlt counts figures with no description.
	missingAlt int
}

func newTagState() *tagState {
	return &tagState{
		mcidByPage:       map[int]int{},
		parents:          map[int][]*structElem{},
		annotationOwners: map[int]*structElem{},
	}
}

// push opens a structure element, returning it — or nil for an artifact, which carries no
// structure and never enters the tree.
//
// No marked-content identifier is allocated here. Whether an element owns any ink is not
// known when it opens: a table cell holding a paragraph owns none, and the paragraph owns
// it all. Allocating eagerly produced a sequence nested inside another, and a reader
// cannot tell which of the two owns the content — veraPDF reports such content as neither
// tagged nor artifact, which is to say invisible. So the identifier is allocated on the
// first drawing operation that actually needs one, and an element that draws nothing
// directly simply becomes a grouping element.
func (t *tagState) push(mark core.Mark) *structElem {
	if !t.enabled {
		return nil
	}

	if mark.Role == core.RoleArtifact || mark.Role == core.RoleNone {
		t.stack = append(t.stack, nil)
		return nil
	}

	if level := mark.Role.HeadingLevel(); level > 0 {
		t.headings = append(t.headings, level)
	}

	// A figure with nothing to read out is the gap tagging exists to close, so this is
	// reported rather than allowed through. Producing a document that passes for
	// accessible and is not would be the worse outcome: nobody checks twice.
	if mark.Role == core.RoleFigure && mark.Alt == "" && mark.ActualText == "" {
		t.missingAlt++
	}

	parent := t.root
	for i := len(t.stack) - 1; i >= 0; i-- {
		if t.stack[i] != nil {
			parent = t.stack[i]
			break
		}
	}

	elem := &structElem{mark: mark, parent: parent}
	parent.children = append(parent.children, elem)
	t.stack = append(t.stack, elem)

	return elem
}

// allocate reserves a marked-content identifier for an element on a page.
//
// An element may allocate more than once: content, then a nested child, then more content
// leaves it owning two sequences, which is what /K holding an array is for.
func (t *tagState) allocate(elem *structElem, page int) int {
	mcid := t.mcidByPage[page]
	t.mcidByPage[page] = mcid + 1

	elem.content = append(elem.content, contentRef{page: page, mcid: mcid})

	// The parent tree is indexed by identifier, so the slot has to exist even before the
	// element that fills it is known.
	for len(t.parents[page]) <= mcid {
		t.parents[page] = append(t.parents[page], nil)
	}
	t.parents[page][mcid] = elem

	return mcid
}

// pop closes the most recently opened element.
func (t *tagState) pop() {
	if !t.enabled || len(t.stack) == 0 {
		return
	}
	t.stack = t.stack[:len(t.stack)-1]
}

// current returns the innermost open structure element, or nil when nothing is open or
// the innermost thing is an artifact.
func (t *tagState) current() *structElem {
	if !t.enabled {
		return nil
	}
	for i := len(t.stack) - 1; i >= 0; i-- {
		if t.stack[i] == nil {
			// An artifact: a link inside a running header is decoration, and hanging an
			// object reference off the tree for it would put the header back into the
			// structure by the back door.
			return nil
		}
		return t.stack[i]
	}
	return nil
}

// insideArtifact reports whether the innermost open element is an artifact, so that
// content drawn inside a running header is decoration even if the element asks otherwise.
//
// The innermost decides, rather than anything in the stack. That is what lets a link
// inside a running footer be a link: a conforming document requires every link annotation
// to sit inside a Link element, so the link escapes the artifact around it, and the text
// inside the link is then content again rather than decoration.
func (t *tagState) insideArtifact() bool {
	if len(t.stack) == 0 {
		return false
	}
	return t.stack[len(t.stack)-1] == nil
}

// headingProblems reports heading levels that skip.
//
// A document going straight from a first-level heading to a third gives a reader an
// outline with a hole in it, and the reader has no way to tell whether the second level
// was forgotten or the third was mislabelled. This is the one structural mistake worth
// reporting, because it is both common and invisible in the rendered page.
func (t *tagState) headingProblems() []string {
	var problems []string

	previous := 0
	for _, level := range t.headings {
		if previous > 0 && level > previous+1 {
			problems = append(problems, fmt.Sprintf(
				"a level %d heading follows a level %d one, skipping level %d",
				level, previous, previous+1))
		}
		previous = level
	}
	return problems
}

// emitStructure writes the structure tree, the parent tree and the root, and returns
// the root's reference.
func (b *Builder) emitStructure(pageRefs []pdfobj.Ref) (pdfobj.Ref, error) {
	tags := b.tags
	if !tags.enabled {
		return 0, nil
	}

	if problems := tags.headingProblems(); len(problems) > 0 {
		return 0, fmt.Errorf("sanur/render: heading structure: %s",
			strings.Join(problems, "; "))
	}

	if n := tags.missingAlt; n > 0 {
		subject, advice := "an image in a tagged document has", "it"
		if n > 1 {
			subject = fmt.Sprintf("%d images in a tagged document have", n)
			advice = "them"
		}
		return 0, fmt.Errorf(
			"sanur/render: %s no description; describe %s with Describe, or mark %s "+
				"as decoration with Decoration", subject, advice, advice)
	}

	// Every element's number is reserved before any is written, because a child records
	// its parent and a parent records its children — and the tree's own root has to be
	// reserved first, since the topmost element names it as its parent.
	//
	// That /P is not optional. Every structure element requires one, and software walks
	// it upwards to establish that a piece of content sits in the tree at all: with the
	// topmost element's parent missing, veraPDF reported every tagged sequence in the
	// document as untagged, because the walk from the content ran out before reaching
	// the root.
	rootRef := b.writer.Reserve()

	tags.root.ref = b.writer.Reserve()
	reserveRefs(b.writer, tags.root)

	parentTreeRef := b.emitParentTree(tags, pageRefs)

	if err := b.writeStructElem(tags.root, pageRefs, rootRef); err != nil {
		return 0, err
	}

	b.writer.Put(rootRef, pdfobj.NewDict().
		SetName("Type", "StructTreeRoot").
		Set("K", pdfobj.Array(tags.root.ref.String())).
		SetRef("ParentTree", parentTreeRef).
		String())

	return rootRef, nil
}

// reserveRefs walks the tree reserving an object number for every node.
func reserveRefs(w *pdfobj.Writer, elem *structElem) {
	for _, child := range elem.children {
		child.ref = w.Reserve()
		reserveRefs(w, child)
	}
}

// writeStructElem writes one node and everything beneath it.
func (b *Builder) writeStructElem(elem *structElem, pageRefs []pdfobj.Ref, rootRef pdfobj.Ref) error {
	dict := pdfobj.NewDict().
		SetName("Type", "StructElem").
		SetName("S", string(elem.mark.Role))

	// The topmost element's parent is the tree root itself.
	if elem.parent != nil {
		dict.SetRef("P", elem.parent.ref)
	} else {
		dict.SetRef("P", rootRef)
	}

	// The kids of an element are its own content sequences followed by its child
	// elements, in the order they were drawn — which is reading order.
	var kids []string

	// /Pg names the page an element's bare MCIDs belong to, so it can only be one page.
	// Content on any other page has to carry its own, as a marked-content reference.
	//
	// In practice an element belongs to a single sheet: the wrapper opens and closes
	// once per sheet, so a paragraph split across a page break becomes two structure
	// elements rather than one spanning both. Handling the general case anyway costs a
	// few lines and closes a failure that would otherwise be silent — MCIDs quietly
	// attributed to the wrong page, which no reader would report and no rendering would
	// reveal.
	home := -1
	if len(elem.content) > 0 {
		home = elem.content[0].page
	}

	for _, ref := range elem.content {
		if ref.page < 0 || ref.page >= len(pageRefs) {
			return fmt.Errorf("sanur/render: structure element on page %d, "+
				"which does not exist", ref.page+1)
		}

		if ref.page == home {
			kids = append(kids, fmt.Sprint(ref.mcid))
			continue
		}

		kids = append(kids, pdfobj.NewDict().
			SetName("Type", "MCR").
			SetRef("Pg", pageRefs[ref.page]).
			SetInt("MCID", ref.mcid).
			String())
	}

	if home >= 0 {
		dict.SetRef("Pg", pageRefs[home])
	}

	for _, child := range elem.children {
		kids = append(kids, child.ref.String())
		if err := b.writeStructElem(child, pageRefs, rootRef); err != nil {
			return err
		}
	}

	// An object reference is how the structure reaches an annotation: the link is on the
	// page, and this is what says which element's content it belongs to.
	for _, annot := range elem.annotations {
		kids = append(kids, pdfobj.NewDict().
			SetName("Type", "OBJR").
			SetRef("Obj", annot).
			String())
	}

	if len(kids) == 1 {
		dict.Set("K", kids[0])
	} else if len(kids) > 1 {
		dict.Set("K", pdfobj.Array(kids...))
	}

	// A header cell has to say which way it heads: down a column or across a row. Without
	// it a reader knows the cell is a heading and not what it heads, which is most of the
	// value of marking it.
	if elem.mark.Role == core.RoleTableHeader {
		scope := elem.mark.Scope
		if scope == "" {
			scope = core.ScopeColumn
		}
		dict.Set("A", pdfobj.NewDict().
			SetName("O", "Table").
			SetName("Scope", string(scope)).
			String())
	}

	// A list says how it labels its items, because the labels themselves are drawn as
	// ordinary text: nothing else in the file distinguishes "1." as a marker from "1." as
	// part of a sentence.
	if elem.mark.Role == core.RoleList {
		numbering := elem.mark.Numbering
		if numbering == "" {
			numbering = core.NumberingNone
		}
		dict.Set("A", pdfobj.NewDict().
			SetName("O", "List").
			SetName("ListNumbering", string(numbering)).
			String())
	}

	if elem.mark.Alt != "" {
		dict.SetTextString("Alt", elem.mark.Alt)
	}
	if elem.mark.ActualText != "" {
		dict.SetTextString("ActualText", elem.mark.ActualText)
	}
	if elem.mark.Lang != "" {
		dict.SetString("Lang", elem.mark.Lang)
	}
	if elem.mark.Title != "" {
		dict.SetTextString("T", elem.mark.Title)
	}

	b.writer.Put(elem.ref, dict.String())
	return nil
}

// emitParentTree writes the number tree mapping each page's MCIDs to the structure
// elements that own them.
//
// This is the direction nothing else exercises. The structure tree lets software walk
// from meaning to content; the parent tree lets it walk from content to meaning, which
// is what a screen reader does when the user clicks a word, and what a "read from here"
// command needs. A document can have a perfect structure tree and be unusable without it.
func (b *Builder) emitParentTree(tags *tagState, pageRefs []pdfobj.Ref) pdfobj.Ref {
	pages := make([]int, 0, len(tags.parents))
	for page := range tags.parents {
		pages = append(pages, page)
	}
	sort.Ints(pages)

	var numbers []string

	for _, page := range pages {
		owners := tags.parents[page]

		refs := make([]string, 0, len(owners))
		for _, owner := range owners {
			if owner == nil {
				// An MCID with no owner cannot happen for content that was tagged,
				// but the array has to stay dense: a reader indexes into it.
				refs = append(refs, "null")
				continue
			}
			refs = append(refs, owner.ref.String())
		}

		// The key is the page's /StructParents value, which is its index.
		numbers = append(numbers, fmt.Sprint(page), pdfobj.Array(refs...))
	}

	// Annotation keys follow the page keys, so the number tree stays in ascending order.
	keys := make([]int, 0, len(tags.annotationOwners))
	for key := range tags.annotationOwners {
		keys = append(keys, key)
	}
	sort.Ints(keys)

	for _, key := range keys {
		// An annotation's entry is the element itself rather than an array: it is one
		// object, not a numbered sequence of them.
		numbers = append(numbers, fmt.Sprint(key), tags.annotationOwners[key].ref.String())
	}

	tree := pdfobj.NewDict().Set("Nums", pdfobj.Array(numbers...))
	return b.writer.AddDict(tree)
}

// structParents returns the /StructParents value for a page, and whether it has one.
func (b *Builder) structParents(page int) (int, bool) {
	if !b.tags.enabled {
		return 0, false
	}
	if _, ok := b.tags.parents[page]; !ok {
		return 0, false
	}
	return page, true
}

// xmpMetadata builds the XMP packet a conforming document has to carry.
//
// Tagging the content is not enough on its own: a document also has to say that it claims
// conformance, and that claim lives in XMP rather than in the document information
// dictionary. Without it a file can be perfectly tagged and still fail validation, which
// is the least useful way to fail — everything works and nothing certifies it.
//
// The title is included because a reader is asked to show it in place of the filename, and
// asking for that while leaving it empty would show nothing.
func xmpMetadata(title string) []byte {
	var b strings.Builder

	// The packet identifier is fixed by the XMP specification, and the leading marker is a
	// byte-order mark, which is what tells a tool scanning raw bytes that the packet is
	// UTF-8. Go will not accept one as a literal in source, so it is escaped here and
	// written as the single character it is.
	b.WriteString("<?xpacket begin=\"\ufeff\" id=\"W5M0MpCehiHzreSzNTczkc9d\"?>\n")
	b.WriteString(`<x:xmpmeta xmlns:x="adobe:ns:meta/">` + "\n")
	b.WriteString(` <rdf:RDF xmlns:rdf="http://www.w3.org/1999/02/22-rdf-syntax-ns#">` + "\n")

	if title != "" {
		b.WriteString(`  <rdf:Description rdf:about="" xmlns:dc="http://purl.org/dc/elements/1.1/">` + "\n")
		b.WriteString("   <dc:title><rdf:Alt><rdf:li xml:lang=\"x-default\">")
		b.WriteString(escapeXML(title))
		b.WriteString("</rdf:li></rdf:Alt></dc:title>\n")
		b.WriteString("  </rdf:Description>\n")
	}

	b.WriteString(`  <rdf:Description rdf:about="" xmlns:pdfuaid="http://www.aiim.org/pdfua/ns/id/">` + "\n")
	b.WriteString("   <pdfuaid:part>1</pdfuaid:part>\n")
	b.WriteString("  </rdf:Description>\n")
	b.WriteString(" </rdf:RDF>\n</x:xmpmeta>\n")
	b.WriteString("<?xpacket end=\"w\"?>")

	return []byte(b.String())
}

func escapeXML(s string) string {
	replacements := []struct{ from, to string }{
		{"&", "&amp;"},
		{"<", "&lt;"},
		{">", "&gt;"},
		{`"`, "&quot;"},
	}
	for _, r := range replacements {
		s = strings.ReplaceAll(s, r.from, r.to)
	}
	return s
}
