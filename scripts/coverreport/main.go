// Command coverreport summarises a Go coverage profile per package, weighted by
// statement count.
//
// `go tool cover -func` reports one line per function, which is too granular to
// read at a glance, and averaging those percentages is misleading because it
// weights a one-line accessor the same as a hundred-statement layout routine.
// Summing statements instead produces per-package figures that reconcile exactly
// with the total `go tool cover` prints.
package main

import (
	"bufio"
	"fmt"
	"os"
	"path"
	"sort"
	"strconv"
	"strings"
)

// modulePrefix is trimmed from package paths so the output stays readable.
const modulePrefix = "github.com/aaripurna/sanur-pdf"

type stats struct {
	total   int
	covered int
}

func main() {
	profile := "coverage.out"
	if len(os.Args) > 1 {
		profile = os.Args[1]
	}

	blocks, err := parseProfile(profile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "coverreport:", err)
		os.Exit(1)
	}
	if len(blocks) == 0 {
		fmt.Fprintf(os.Stderr, "coverreport: %s contains no coverage blocks\n", profile)
		os.Exit(1)
	}

	byPackage := map[string]*stats{}
	var overall stats

	for id, b := range blocks {
		pkg := packageOf(id.file)

		s := byPackage[pkg]
		if s == nil {
			s = &stats{}
			byPackage[pkg] = s
		}

		s.total += b.statements
		overall.total += b.statements
		if b.hits > 0 {
			s.covered += b.statements
			overall.covered += b.statements
		}
	}

	names := make([]string, 0, len(byPackage))
	for name := range byPackage {
		names = append(names, name)
	}
	sort.Strings(names)

	const rule = "-------------------------------------------------"

	fmt.Printf("%-22s %8s %9s %8s\n", "PACKAGE", "STMTS", "COVERED", "PCT")
	fmt.Println(rule)
	for _, name := range names {
		s := byPackage[name]
		fmt.Printf("%-22s %8d %9d %7.1f%%\n", name, s.total, s.covered, percent(s.covered, s.total))
	}
	fmt.Println(rule)
	fmt.Printf("%-22s %8d %9d %7.1f%%\n", "TOTAL", overall.total, overall.covered,
		percent(overall.covered, overall.total))
}

func percent(covered, total int) float64 {
	if total == 0 {
		return 0
	}
	return 100 * float64(covered) / float64(total)
}

// blockID identifies one basic block: its file and its source range.
type blockID struct {
	file string
	span string
}

type block struct {
	statements int
	hits       int
}

// parseProfile reads a coverage profile, merging the hit counts of blocks that
// appear more than once.
//
// Under -coverpkg every test binary emits the complete block set, so a given
// block shows up once per binary with its own count. Merging by block before
// totalling is essential: otherwise a block covered by one binary and missed by
// another would be counted on both sides of the ratio.
func parseProfile(name string) (map[blockID]*block, error) {
	f, err := os.Open(name)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w (run 'make cover' first)", name, err)
	}
	defer f.Close()

	blocks := map[blockID]*block{}
	scanner := bufio.NewScanner(f)

	for lineNo := 0; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(scanner.Text())

		// The first line is the mode declaration, e.g. "mode: atomic".
		if line == "" || strings.HasPrefix(line, "mode:") {
			continue
		}

		// Each remaining line is "file:startLine.col,endLine.col numStmt count".
		fields := strings.Fields(line)
		if len(fields) != 3 {
			return nil, fmt.Errorf("%s:%d: expected 3 fields, got %d", name, lineNo+1, len(fields))
		}

		colon := strings.LastIndex(fields[0], ":")
		if colon < 0 {
			return nil, fmt.Errorf("%s:%d: no file/range separator in %q", name, lineNo+1, fields[0])
		}

		statements, err := strconv.Atoi(fields[1])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad statement count %q", name, lineNo+1, fields[1])
		}
		hits, err := strconv.Atoi(fields[2])
		if err != nil {
			return nil, fmt.Errorf("%s:%d: bad hit count %q", name, lineNo+1, fields[2])
		}

		id := blockID{file: fields[0][:colon], span: fields[0][colon+1:]}

		if existing, ok := blocks[id]; ok {
			existing.hits += hits
			continue
		}
		blocks[id] = &block{statements: statements, hits: hits}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("reading %s: %w", name, err)
	}
	return blocks, nil
}

// packageOf reduces a file path to the package label shown in the report.
func packageOf(file string) string {
	dir := path.Dir(file)

	trimmed := strings.TrimPrefix(dir, modulePrefix)
	trimmed = strings.TrimPrefix(trimmed, "/")
	if trimmed == "" || trimmed == "." {
		return "sanur (root)"
	}
	return trimmed
}
