package sanur_test

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"

	sanur "github.com/aaripurna/sanur-pdf"
	"github.com/aaripurna/sanur-pdf/render"
)

// Tagging is verified by reading the object graph back, not by searching for substrings.
//
// The reason is the nature of the failure. A structure tree that is present but wrong —
// an MCID pointing at the neighbouring paragraph, a parent tree with a hole in it — looks
// identical to a correct one from the outside, renders identically, and is believed by
// the software that consumes it. Asserting that "/StructTreeRoot" appears would pass for
// any of those. So these tests walk the tree and check the numbers line up.

// pdfObjects parses an uncompressed PDF into its indirect objects, by number.
type pdfObjects map[int]string

var objectPattern = regexp.MustCompile(`(?s)(\d+) 0 obj\s*(.*?)\s*endobj`)

func parseObjects(t *testing.T, data []byte) pdfObjects {
	t.Helper()

	objs := pdfObjects{}
	for _, m := range objectPattern.FindAllSubmatch(data, -1) {
		number, err := strconv.Atoi(string(m[1]))
		if err != nil {
			t.Fatalf("object number %q: %v", m[1], err)
		}
		objs[number] = string(m[2])
	}
	if len(objs) == 0 {
		t.Fatal("no objects found; the document may be compressed")
	}
	return objs
}

// find returns the one object containing marker, failing if there is not exactly one.
func (o pdfObjects) find(t *testing.T, marker string) (int, string) {
	t.Helper()

	var (
		number int
		body   string
		count  int
	)
	for n, b := range o {
		if strings.Contains(b, marker) {
			number, body, count = n, b, count+1
		}
	}
	if count != 1 {
		t.Fatalf("found %d objects containing %q, want exactly 1", count, marker)
	}
	return number, body
}

var refPattern = regexp.MustCompile(`(\d+) 0 R`)

// ref returns the object number of the first reference in a dictionary entry.
func (o pdfObjects) ref(t *testing.T, body, key string) int {
	t.Helper()

	m := regexp.MustCompile(regexp.QuoteMeta(key) + `\s*(\[[^\]]*\]|\d+ 0 R)`).FindStringSubmatch(body)
	if m == nil {
		t.Fatalf("no %s in %s", key, body)
	}
	inner := refPattern.FindStringSubmatch(m[1])
	if inner == nil {
		t.Fatalf("%s is not a reference: %s", key, m[1])
	}
	n, _ := strconv.Atoi(inner[1])
	return n
}

// node is one structure element, read back from the file.
type node struct {
	number   int
	role     string
	alt      string
	mcids    []int
	children []*node
	objrs    int
}

// tree reads the structure tree from a document.
func (o pdfObjects) tree(t *testing.T, root int) *node {
	t.Helper()

	body := o[root]

	n := &node{number: root}
	if m := regexp.MustCompile(`/S /(\w+)`).FindStringSubmatch(body); m != nil {
		n.role = m[1]
	}
	if m := regexp.MustCompile(`/Alt \(([^)]*)\)`).FindStringSubmatch(body); m != nil {
		n.alt = m[1]
	}
	n.objrs = strings.Count(body, "/Type /OBJR")

	kids := regexp.MustCompile(`(?s)/K (\[.*?\]|\d+ 0 R|\d+)`).FindStringSubmatch(body)
	if kids == nil {
		return n
	}

	// A bare integer among the kids is an MCID; a reference is a child element or an
	// object reference.
	for _, field := range regexp.MustCompile(`\d+ 0 R|<<[^>]*>>|\d+`).FindAllString(kids[1], -1) {
		switch {
		case strings.HasPrefix(field, "<<"):
			// An OBJR or MCR dictionary, counted above.
		case strings.HasSuffix(field, " 0 R"):
			child, _ := strconv.Atoi(strings.TrimSuffix(field, " 0 R"))
			if strings.Contains(o[child], "/Type /StructElem") {
				n.children = append(n.children, o.tree(t, child))
			}
		default:
			mcid, _ := strconv.Atoi(field)
			n.mcids = append(n.mcids, mcid)
		}
	}
	return n
}

// outline renders a tree as indented role names, for comparing shapes.
func (n *node) outline() string {
	var b strings.Builder
	n.write(&b, 0)
	return b.String()
}

func (n *node) write(b *strings.Builder, depth int) {
	fmt.Fprintf(b, "%s%s\n", strings.Repeat("  ", depth), n.role)
	for _, child := range n.children {
		child.write(b, depth+1)
	}
}

// structureOf returns the root structure element of a tagged document.
func structureOf(t *testing.T, data []byte) (pdfObjects, *node) {
	t.Helper()

	objs := parseObjects(t, data)
	_, root := objs.find(t, "/Type /StructTreeRoot")
	return objs, objs.tree(t, objs.ref(t, root, "/K"))
}

// taggedDocument is a document exercising every kind of content that gets a role.
func taggedDocument(t *testing.T) []byte {
	t.Helper()

	img, err := render.DecodeImage("logo", makePNG(t, 20, 10, false))
	if err != nil {
		t.Fatal(err)
	}

	doc := sanur.New().Title("Accessible report").Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Header().Text("running header")
		p.Footer().AlignRight().PageNumber("Page {page} of {total}")

		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Spacing(6)
			c.Item().Tag(sanur.Heading1).Text("Title")
			c.Item().Text("A paragraph.")
			c.Item().Link("https://example.com").Text("A link")
			c.Item().Width(60).Describe("The company logo").Image(img)
			c.Item().LineHorizontal(1, sanur.Grey300)
			c.Item().Tag(sanur.Heading2).Text("Subsection")
			c.Item().Table(func(tb *sanur.TableBuilder) {
				tb.ColumnRelative(1)
				tb.ColumnConstant(60)
				tb.HeaderRow(func(r *sanur.TableRowBuilder) { r.Cells("Item", "Amount") })
				tb.Row(func(r *sanur.TableRowBuilder) { r.Cells("Design", "250.00") })
			})
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatalf("generating: %v", err)
	}
	return data
}

func TestTaggingIsOptIn(t *testing.T) {
	// Tagging changes every content stream and adds a tree of objects. A document that
	// did not ask for it must come out exactly as before, or the feature is a silent
	// change to everyone's output.
	build := func(tagged bool) []byte {
		doc := sanur.New().Title("Same document")
		if tagged {
			doc = doc.Tagged("en-GB")
		}
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			p.Content().Column(func(c *sanur.ColumnBuilder) {
				c.Item().Tag(sanur.Heading1).Text("Heading")
				c.Item().Text("Body.")
			})
		})

		data, err := doc.Bytes()
		if err != nil {
			t.Fatalf("generating: %v", err)
		}
		return data
	}

	plain := build(false)

	if bytes.Contains(plain, []byte("BDC")) || bytes.Contains(plain, []byte("StructTreeRoot")) {
		t.Error("an untagged document carries marked content")
	}
	// The Tag call above is present in both, so it has to be inert without Tagged.
	if bytes.Contains(plain, []byte("/MarkInfo")) {
		t.Error("an untagged document declares itself marked")
	}
	if len(build(true)) <= len(plain) {
		t.Error("tagging added nothing")
	}
}

func TestTaggedDocumentDeclaresItself(t *testing.T) {
	// Without these a reader has no reason to look for the tree, so the structure might
	// as well not be there.
	data := taggedDocument(t)

	for _, want := range []string{
		"/StructTreeRoot",
		"/MarkInfo << /Marked true >>",
		"/Lang (en-GB)",
		// PDF/UA requires a reader to show the title rather than the filename.
		"/ViewerPreferences << /DisplayDocTitle true >>",
	} {
		if !bytes.Contains(data, []byte(want)) {
			t.Errorf("the catalog is missing %q", want)
		}
	}
}

func TestReadersSeeADocumentAsTagged(t *testing.T) {
	// The one external check available: poppler reports whether it found the apparatus a
	// tagged document needs.
	pdfinfo, err := exec.LookPath("pdfinfo")
	if err != nil {
		t.Skip("pdfinfo not installed")
	}

	path := filepath.Join(t.TempDir(), "tagged.pdf")
	if err := os.WriteFile(path, taggedDocument(t), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(pdfinfo, path).Output()
	if err != nil {
		t.Fatalf("pdfinfo failed: %v", err)
	}
	if !regexp.MustCompile(`(?m)^Tagged:\s+yes`).Match(out) {
		t.Errorf("poppler does not consider the document tagged:\n%s", out)
	}
}

func TestStructureTreeMirrorsTheDocument(t *testing.T) {
	_, root := structureOf(t, taggedDocument(t))

	// Reading order, and the nesting a reader navigates by. The rule and the running
	// furniture are absent because they are artifacts, which is the point.
	want := strings.TrimSpace(`
Document
  H1
  P
  Link
    P
  Figure
  H2
  Table
    TR
      TH
        P
      TH
        P
    TR
      TD
        P
      TD
        P
`)

	if got := strings.TrimSpace(root.outline()); got != want {
		t.Errorf("structure tree:\n%s\nwant:\n%s", got, want)
	}
}

func TestEveryMarkedSequenceIsReachableFromTheParentTree(t *testing.T) {
	// This is the direction nothing else exercises and everything depends on. The
	// structure tree lets software walk from meaning to content; the parent tree lets it
	// walk from content to meaning, which is what a screen reader does when the user
	// clicks a word. A document can have a flawless tree and be unusable without it.
	data := taggedDocument(t)
	objs := parseObjects(t, data)

	_, root := objs.find(t, "/Type /StructTreeRoot")
	_, nums := objs.find(t, "/Nums")

	// Every sequence the content stream declares.
	declared := map[int]string{}
	for _, m := range regexp.MustCompile(`/(\w+) << /MCID (\d+) >> BDC`).FindAllSubmatch(data, -1) {
		mcid, _ := strconv.Atoi(string(m[2]))
		declared[mcid] = string(m[1])
	}
	if len(declared) == 0 {
		t.Fatal("no marked content in the stream")
	}

	// The parent tree's array for page zero, in order.
	entry := regexp.MustCompile(`(?s)/Nums \[\s*0\s*\[(.*?)\]`).FindStringSubmatch(nums)
	if entry == nil {
		t.Fatalf("no page-zero entry in the parent tree: %s", nums)
	}
	owners := refPattern.FindAllStringSubmatch(entry[1], -1)

	if len(owners) != len(declared) {
		t.Errorf("the parent tree maps %d MCIDs, but the stream declares %d",
			len(owners), len(declared))
	}

	// Position i in the array is the element owning MCID i, and its role has to be the
	// one the stream used. An off-by-one here is exactly the failure that renders
	// perfectly and reads as nonsense.
	for mcid, owner := range owners {
		number, _ := strconv.Atoi(owner[1])

		role := regexp.MustCompile(`/S /(\w+)`).FindStringSubmatch(objs[number])
		if role == nil {
			t.Errorf("MCID %d maps to object %d, which is not a structure element", mcid, number)
			continue
		}
		if role[1] != declared[mcid] {
			t.Errorf("MCID %d is /%s in the stream but /%s in the tree",
				mcid, declared[mcid], role[1])
		}
	}

	_ = root
}

func TestMarkedContentOperatorsAreBalanced(t *testing.T) {
	// An unbalanced sequence corrupts everything after it in the page, and a reader will
	// either give up on the page or attribute the rest of it to the wrong element.
	data := string(taggedDocument(t))

	opens := strings.Count(data, "BDC") + strings.Count(data, "BMC")
	closes := strings.Count(data, "EMC")

	if opens != closes {
		t.Errorf("%d sequences opened, %d closed", opens, closes)
	}
	if opens == 0 {
		t.Error("no marked content at all")
	}
}

func TestFurnitureAndRulesAreArtifacts(t *testing.T) {
	// A tagged document has no third category: anything not marked as an artifact is
	// content a reader announces. A running header read out on every sheet, or a rule
	// announced between paragraphs, makes a document worse than an untagged one.
	data := taggedDocument(t)
	_, root := structureOf(t, data)

	if !bytes.Contains(data, []byte("/Artifact BMC")) {
		t.Error("nothing was marked as an artifact")
	}

	// The header's text, the page number and the rule must not appear in the tree, which
	// for the text means no structure element owns its marked content.
	var roles []string
	var collect func(*node)
	collect = func(n *node) {
		roles = append(roles, n.role)
		for _, child := range n.children {
			collect(child)
		}
	}
	collect(root)

	// Five paragraphs of real content: the body paragraph, the link's text and the four
	// table cells. The header and the page number would make it more.
	if got := strings.Count(strings.Join(roles, " "), "P"); got != 6 {
		t.Errorf("found %d paragraph-ish roles in %v; the furniture may have leaked in",
			got, roles)
	}
}

func TestFigureWithoutADescriptionIsRefused(t *testing.T) {
	// A figure with nothing to read out is the gap tagging exists to close. Generating a
	// document that passes for accessible and is not would be the worse outcome, since
	// nobody checks twice.
	img, err := render.DecodeImage("logo", makePNG(t, 20, 10, false))
	if err != nil {
		t.Fatal(err)
	}

	build := func(prepare func(*sanur.Container) *sanur.Container) error {
		doc := sanur.New().Tagged("en-GB")
		doc.Page(func(p *sanur.Page) {
			p.Size(sanur.A4).Margin(40)
			prepare(p.Content().Width(60)).Image(img)
		})
		_, err := doc.Bytes()
		return err
	}

	err = build(func(c *sanur.Container) *sanur.Container { return c })
	if err == nil {
		t.Fatal("an undescribed image was accepted")
	}
	for _, want := range []string{"description", "Describe", "Decoration"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// Either describing it or declaring it decorative resolves the problem, and both are
	// legitimate answers: a logo repeated on every page is decoration.
	if err := build(func(c *sanur.Container) *sanur.Container {
		return c.Describe("The company logo")
	}); err != nil {
		t.Errorf("a described image was refused: %v", err)
	}
	if err := build(func(c *sanur.Container) *sanur.Container {
		return c.Decoration()
	}); err != nil {
		t.Errorf("a decorative image was refused: %v", err)
	}
}

func TestSkippedHeadingLevelIsReported(t *testing.T) {
	// A document going from a first-level heading straight to a third gives a reader an
	// outline with a hole in it, and no way to tell whether a level was forgotten or one
	// was mislabelled. It is invisible in the rendered page, which is why it is worth
	// refusing rather than warning about.
	doc := sanur.New().Tagged("en-GB")
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Tag(sanur.Heading1).Text("One")
			c.Item().Tag(sanur.Heading3).Text("Three")
		})
	})

	_, err := doc.Bytes()
	if err == nil {
		t.Fatal("a skipped heading level was accepted")
	}
	for _, want := range []string{"level 3", "level 1", "skipping"} {
		if !strings.Contains(err.Error(), want) {
			t.Errorf("error %q does not mention %q", err, want)
		}
	}

	// Descending by one is fine, and so is going back up several at once: a new
	// first-level section after a third-level one skips nothing.
	fine := sanur.New().Tagged("en-GB")
	fine.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Tag(sanur.Heading1).Text("One")
			c.Item().Tag(sanur.Heading2).Text("Two")
			c.Item().Tag(sanur.Heading3).Text("Three")
			c.Item().Tag(sanur.Heading1).Text("Back to one")
		})
	})
	if _, err := fine.Bytes(); err != nil {
		t.Errorf("a well-formed heading sequence was refused: %v", err)
	}
}

func TestLinkIsReachableFromTheStructure(t *testing.T) {
	// A link annotation has to be findable from the tree, not merely present on the page.
	// A reader announcing "link" needs to know which words it is on, and an annotation
	// floating free says nothing.
	data := taggedDocument(t)
	objs, root := structureOf(t, data)

	var link *node
	var find func(*node)
	find = func(n *node) {
		if n.role == "Link" {
			link = n
		}
		for _, child := range n.children {
			find(child)
		}
	}
	find(root)

	if link == nil {
		t.Fatal("no Link element in the structure tree")
	}
	if link.objrs != 1 {
		t.Errorf("the Link element holds %d object references, want 1", link.objrs)
	}

	// And the annotation itself carries a description, so a reader has something to say
	// beyond "link".
	if !bytes.Contains(data, []byte("/Contents (https://example.com)")) {
		t.Error("the link annotation carries no description")
	}
	_ = objs
}

func TestFooterLinksStayOutOfTheStructure(t *testing.T) {
	// A link in running furniture should still be clickable on every sheet, and should
	// still not be part of the document's structure — the footer is decoration.
	doc := sanur.New().Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Footer().Link("https://example.com/terms").Text("Terms")
		p.Content().Text("Body.")
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Contains(data, []byte("/Subtype /Link")) {
		t.Error("the footer link is not clickable")
	}
	if bytes.Contains(data, []byte("/Type /OBJR")) {
		t.Error("a link in running furniture was added to the structure tree")
	}

	_, root := structureOf(t, data)
	if strings.Contains(root.outline(), "Link") {
		t.Errorf("the footer link entered the structure:\n%s", root.outline())
	}
}

func TestTaggedTextStillExtracts(t *testing.T) {
	// Marked content wraps the text-showing operators, and a mistake there would break
	// extraction — the one thing that would make a document less accessible, not more.
	pdftotext, err := exec.LookPath("pdftotext")
	if err != nil {
		t.Skip("pdftotext not installed")
	}

	path := filepath.Join(t.TempDir(), "tagged.pdf")
	if err := os.WriteFile(path, taggedDocument(t), 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(pdftotext, path, "-").Output()
	if err != nil {
		t.Fatalf("pdftotext failed: %v", err)
	}

	for _, want := range []string{"Title", "A paragraph.", "Subsection", "Design", "250.00"} {
		if !bytes.Contains(out, []byte(want)) {
			t.Errorf("extraction lost %q; got:\n%s", want, out)
		}
	}
}

func TestTaggedDocumentIsDeterministic(t *testing.T) {
	// The tree is built by walking maps and stacks, either of which could leak an order.
	if !bytes.Equal(taggedDocument(t), taggedDocument(t)) {
		t.Error("two runs produced different bytes")
	}
}

func TestTaggedDocumentPassesGhostscript(t *testing.T) {
	checkWithGhostscript(t, taggedDocument(t))
}

func TestMarkedContentIsNumberedPerPage(t *testing.T) {
	// MCIDs are unique within a page, not within a document, and the parent tree is keyed
	// by page. A counter shared across pages would produce numbers that look plausible
	// and a tree whose arrays are the wrong length — which no single-page test can see.
	doc := sanur.New().Tagged("en-GB").Uncompressed()
	doc.Page(func(p *sanur.Page) {
		p.Size(sanur.A4).Margin(40)
		p.Content().Column(func(c *sanur.ColumnBuilder) {
			c.Item().Tag(sanur.Heading1).Text("First sheet")
			c.Item().Text("Body on the first sheet.")
			c.Item().PageBreak()
			c.Item().Tag(sanur.Heading2).Text("Second sheet")
			c.Item().Text("Body on the second sheet.")
			c.Item().Text("More body on the second sheet.")
		})
	})

	data, err := doc.Bytes()
	if err != nil {
		t.Fatal(err)
	}

	if pages := countPages(data); pages != 2 {
		t.Fatalf("expected 2 sheets, got %d", pages)
	}

	objs := parseObjects(t, data)

	// Each page declares its own key into the parent tree, and the keys are the page
	// indices.
	structParents := regexp.MustCompile(`/StructParents (\d+)`).FindAllSubmatch(data, -1)
	if len(structParents) != 2 {
		t.Fatalf("%d pages carry a parent-tree key, want 2", len(structParents))
	}
	for i, m := range structParents {
		if got := string(m[1]); got != strconv.Itoa(i) {
			t.Errorf("page %d has key %s, want %d", i, got, i)
		}
	}

	// Both pages number their sequences from zero.
	_, nums := objs.find(t, "/Nums")
	entries := regexp.MustCompile(`(?s)(\d+)\s*\[([^\]]*)\]`).FindAllStringSubmatch(nums, -1)
	if len(entries) != 2 {
		t.Fatalf("the parent tree has %d page entries, want 2: %s", len(entries), nums)
	}

	total := 0
	for _, entry := range entries {
		owners := refPattern.FindAllString(entry[2], -1)
		if len(owners) == 0 {
			t.Errorf("page %s owns no marked content", entry[1])
		}
		total += len(owners)
	}

	declared := len(regexp.MustCompile(`<< /MCID \d+ >> BDC`).FindAll(data, -1))
	if total != declared {
		t.Errorf("the parent tree maps %d sequences across both pages, "+
			"but the streams declare %d", total, declared)
	}

	// The second page's own numbering starts again at zero, which is what makes the key
	// per page meaningful.
	if !bytes.Contains(data, []byte("/H2 << /MCID 0 >> BDC")) {
		t.Error("the second sheet does not number its content from zero")
	}
}
