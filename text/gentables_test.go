package text

import (
	"bytes"
	"os"
	"os/exec"
	"testing"
)

// TestArabicTableMatchesItsGenerator regenerates the shaping table and compares it with
// the committed file.
//
// The table is 76 letters with four forms each, derived from the Unicode character
// database. Generated code that has drifted from its generator is worse than
// hand-written code, because the next person to regenerate it silently reverts whatever
// was changed by hand — and a single wrong codepoint here shows up as one Arabic letter
// that draws in the wrong shape, which nobody reviewing 300 hex literals would catch.
func TestArabicTableMatchesItsGenerator(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping generator execution in short mode")
	}

	generated, err := exec.Command("go", "run", "./internal/gentables").Output()
	if err != nil {
		t.Fatalf("running the generator: %v", err)
	}

	formatted, err := gofmt(generated)
	if err != nil {
		t.Fatalf("formatting the generated table: %v", err)
	}

	committed, err := os.ReadFile("arabic_table.go")
	if err != nil {
		t.Fatal(err)
	}

	if !bytes.Equal(formatted, committed) {
		t.Error("arabic_table.go differs from what the generator produces; " +
			"regenerate it with: go run ./text/internal/gentables > text/arabic_table.go")
	}
}

// gofmt formats source the way the committed file is stored, so the comparison is of
// content rather than of whitespace.
func gofmt(source []byte) ([]byte, error) {
	cmd := exec.Command("gofmt")
	cmd.Stdin = bytes.NewReader(source)
	return cmd.Output()
}
