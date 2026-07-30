package render

import (
	"fmt"
	"sort"
	"strings"

	"github.com/aaripurna/sanur-pdf/core"
	"github.com/aaripurna/sanur-pdf/internal/pdfobj"
)

// linkRequest is one clickable rectangle awaiting emission.
type linkRequest struct {
	page   int
	rect   [4]float64
	target core.LinkTarget
}

// destination is a resolved anchor: which page, and how far up it.
type destination struct {
	page int

	// y is in PDF space, measured up from the bottom of the page.
	y float64
}

// bookmark is one outline entry, in the order it was drawn.
type bookmark struct {
	title string
	level int
	dest  string
}

// addLink records a link for the given page.
func (b *Builder) addLink(page int, rect [4]float64, target core.LinkTarget) {
	b.links = append(b.links, linkRequest{page: page, rect: rect, target: target})
}

// addDestination records a named anchor.
//
// The first registration of a name wins. A duplicate is far more likely to be an
// accident — the same section helper called twice — than a deliberate
// redefinition, and silently moving every link to the later one would be the
// harder failure to notice.
func (b *Builder) addDestination(name string, page int, y float64) {
	if _, exists := b.destinations[name]; exists {
		b.duplicateDests = append(b.duplicateDests, name)
		return
	}
	b.destinations[name] = destination{page: page, y: y}
}

// addBookmark records an outline entry.
func (b *Builder) addBookmark(title string, level int, dest string) {
	if level < 0 {
		level = 0
	}
	b.bookmarks = append(b.bookmarks, bookmark{title: title, level: level, dest: dest})
}

// emitAnnotations writes the link annotations for one page and returns the array
// to attach to it, or an empty string when the page has none.
func (b *Builder) emitAnnotations(page int, pageRefs []pdfobj.Ref) (string, error) {
	var refs []string

	for _, link := range b.links {
		if link.page != page {
			continue
		}

		action, err := b.linkAction(link.target, pageRefs)
		if err != nil {
			return "", err
		}

		annot := pdfobj.NewDict().
			SetName("Type", "Annot").
			SetName("Subtype", "Link").
			Set("Rect", pdfobj.NumArray(link.rect[0], link.rect[1], link.rect[2], link.rect[3])).
			// A zero-width border is what stops readers drawing the black
			// rectangle that older PDF tools are notorious for.
			Set("Border", pdfobj.IntArray([]int{0, 0, 0})).
			Set("A", action)

		refs = append(refs, b.writer.AddDict(annot).String())
	}

	if len(refs) == 0 {
		return "", nil
	}
	return pdfobj.Array(refs...), nil
}

// linkAction builds the action dictionary for a target.
func (b *Builder) linkAction(target core.LinkTarget, pageRefs []pdfobj.Ref) (string, error) {
	if target.External() {
		return pdfobj.NewDict().
			SetName("Type", "Action").
			SetName("S", "URI").
			SetString("URI", target.URL).
			String(), nil
	}

	dest, ok := b.destinations[target.Name]
	if !ok {
		return "", fmt.Errorf(
			"sanur/render: link to unknown destination %q; "+
				"register it with an Anchor or Bookmark somewhere in the document",
			target.Name)
	}
	if dest.page < 0 || dest.page >= len(pageRefs) {
		return "", fmt.Errorf(
			"sanur/render: destination %q refers to page %d, which does not exist",
			target.Name, dest.page+1)
	}

	// XYZ with a null zoom scrolls to the point without changing magnification,
	// which is what a cross-reference should do; Fit would zoom the whole page.
	return pdfobj.NewDict().
		SetName("Type", "Action").
		SetName("S", "GoTo").
		Set("D", pdfobj.Array(
			pageRefs[dest.page].String(),
			pdfobj.Name("XYZ"),
			"null",
			pdfobj.Num(dest.y),
			"null",
		)).
		String(), nil
}

// checkDestinations reports names registered more than once.
func (b *Builder) checkDestinations() error {
	if len(b.duplicateDests) == 0 {
		return nil
	}

	unique := map[string]bool{}
	var names []string
	for _, n := range b.duplicateDests {
		if !unique[n] {
			unique[n] = true
			names = append(names, n)
		}
	}
	sort.Strings(names)

	return fmt.Errorf(
		"sanur/render: destination name used more than once: %s; "+
			"names are document-wide, so each anchor needs its own",
		strings.Join(names, ", "))
}

// outlineNode is a bookmark placed in the tree.
type outlineNode struct {
	entry    bookmark
	children []*outlineNode

	ref pdfobj.Ref
}

// buildOutlineTree turns the flat, draw-ordered bookmark list into a tree.
//
// Levels are interpreted the way headings in a document are: an entry becomes a
// child of the nearest preceding entry with a lower level. A level that jumps by
// more than one — a level 3 following a level 1 — attaches to that level 1 rather
// than being discarded, since dropping the entry would lose a bookmark over a
// formatting slip.
func buildOutlineTree(entries []bookmark) []*outlineNode {
	var roots []*outlineNode

	// ancestors[i] is the deepest open node at level i.
	var ancestors []*outlineNode

	for _, entry := range entries {
		node := &outlineNode{entry: entry}

		// Discard any open ancestors at or below this entry's level.
		if entry.level < len(ancestors) {
			ancestors = ancestors[:entry.level]
		}

		if len(ancestors) == 0 {
			roots = append(roots, node)
		} else {
			parent := ancestors[len(ancestors)-1]
			parent.children = append(parent.children, node)
		}

		ancestors = append(ancestors, node)
	}

	return roots
}

// emitOutline writes the outline tree and returns the root reference.
func (b *Builder) emitOutline(pageRefs []pdfobj.Ref) (pdfobj.Ref, error) {
	if len(b.bookmarks) == 0 {
		return 0, nil
	}

	roots := buildOutlineTree(b.bookmarks)
	if len(roots) == 0 {
		return 0, nil
	}

	// The root has to exist before its children can name it as their parent, so
	// its number is reserved and filled in once they are written.
	rootRef := b.writer.Reserve()

	first, last, total, err := b.emitOutlineLevel(roots, rootRef, pageRefs)
	if err != nil {
		return 0, err
	}

	root := pdfobj.NewDict().SetName("Type", "Outlines").SetInt("Count", total)
	if first.Valid() {
		root.SetRef("First", first).SetRef("Last", last)
	}
	b.writer.Put(rootRef, root.String())

	return rootRef, nil
}

// emitOutlineLevel writes one level of siblings, returning the first and last
// references and the number of entries visible when the level is open.
func (b *Builder) emitOutlineLevel(
	nodes []*outlineNode,
	parent pdfobj.Ref,
	pageRefs []pdfobj.Ref,
) (first, last pdfobj.Ref, visible int, err error) {
	// Every sibling needs a number before any of them can be written, because each
	// names its neighbours.
	for _, node := range nodes {
		node.ref = b.writer.Reserve()
	}

	for i, node := range nodes {
		childFirst, childLast, childVisible, err := b.emitOutlineLevel(node.children, node.ref, pageRefs)
		if err != nil {
			return 0, 0, 0, err
		}

		dict := pdfobj.NewDict().
			SetTextString("Title", node.entry.title).
			SetRef("Parent", parent)

		if i > 0 {
			dict.SetRef("Prev", nodes[i-1].ref)
		}
		if i < len(nodes)-1 {
			dict.SetRef("Next", nodes[i+1].ref)
		}

		if childFirst.Valid() {
			dict.SetRef("First", childFirst).SetRef("Last", childLast)
			// A positive count means the branch opens by default; negative would
			// collapse it. Showing the structure is the more useful default.
			dict.SetInt("Count", childVisible)
		}

		dest, ok := b.destinations[node.entry.dest]
		if !ok {
			return 0, 0, 0, fmt.Errorf(
				"sanur/render: bookmark %q points at unknown destination %q",
				node.entry.title, node.entry.dest)
		}
		if dest.page < 0 || dest.page >= len(pageRefs) {
			return 0, 0, 0, fmt.Errorf(
				"sanur/render: bookmark %q refers to page %d, which does not exist",
				node.entry.title, dest.page+1)
		}

		dict.Set("Dest", pdfobj.Array(
			pageRefs[dest.page].String(),
			pdfobj.Name("XYZ"),
			"null",
			pdfobj.Num(dest.y),
			"null",
		))

		b.writer.Put(node.ref, dict.String())
		visible += 1 + childVisible
	}

	if len(nodes) == 0 {
		return 0, 0, 0, nil
	}
	return nodes[0].ref, nodes[len(nodes)-1].ref, visible, nil
}
