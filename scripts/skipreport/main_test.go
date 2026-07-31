package main

import (
	"strings"
	"testing"
)

// The layouts below are copied from real `go test -v` output. The subtest case is the
// reason this is parsed rather than grepped: Go logs a subtest's reason as it runs and
// prints its result later, batched under the parent, so the two are not adjacent and an
// adjacency-based reader reports "(no reason given)" for exactly the skips that matter.
const output = `=== RUN   TestPlain
--- PASS: TestPlain (0.01s)
=== RUN   TestWithReason
    embed_test.go:41: ghostscript not installed
--- SKIP: TestWithReason (0.00s)
=== RUN   TestNoReason
--- SKIP: TestNoReason (0.00s)
=== RUN   TestParent
=== RUN   TestParent/first
    composite_test.go:62: no system font with Cyrillic and Greek coverage
=== RUN   TestParent/second
--- PASS: TestParent (0.00s)
    --- SKIP: TestParent/first (0.00s)
    --- PASS: TestParent/second (0.00s)
=== RUN   TestSeveralMessages
    a_test.go:10: measuring something
    a_test.go:12: fribidi not installed
--- SKIP: TestSeveralMessages (0.00s)
PASS
ok  	github.com/aaripurna/sanur-pdf	1.234s
`

func TestParseAttributesReasonsToTheRightTests(t *testing.T) {
	got, err := parse(strings.NewReader(output))
	if err != nil {
		t.Fatal(err)
	}

	want := []skip{
		{"TestNoReason", "(no reason given)"},
		{"TestParent/first", "no system font with Cyrillic and Greek coverage"},
		{"TestSeveralMessages", "fribidi not installed"},
		{"TestWithReason", "ghostscript not installed"},
	}

	if len(got) != len(want) {
		t.Fatalf("found %d skips, want %d: %+v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("skip %d = %+v, want %+v", i, got[i], want[i])
		}
	}
}

func TestParseIgnoresPassingTests(t *testing.T) {
	// A run with nothing skipped has to come back empty rather than listing every test
	// that logged something.
	clean := `=== RUN   TestOne
    a_test.go:3: a note
--- PASS: TestOne (0.00s)
PASS
`
	got, err := parse(strings.NewReader(clean))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 0 {
		t.Errorf("found %+v, want nothing", got)
	}
}

func TestParseDeduplicatesAcrossPackages(t *testing.T) {
	// The same helper skips in several packages, and the report is about what did not run
	// rather than how many times it did not.
	repeated := `=== RUN   TestSame
    a_test.go:1: gs missing
--- SKIP: TestSame (0.00s)
=== RUN   TestSame
    a_test.go:1: gs missing
--- SKIP: TestSame (0.00s)
`
	got, err := parse(strings.NewReader(repeated))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Errorf("found %d entries, want 1: %+v", len(got), got)
	}
}
