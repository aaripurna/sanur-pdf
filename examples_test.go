package sanur_test

import (
	"bytes"
	"compress/zlib"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/aaripurna/sanur-pdf/theme"
)

// The examples are the documentation people actually run, so they are compiled
// and executed here rather than left to rot. Each one is a full exercise of the
// engine — the report builds charts and a landscape sheet, the images example
// loads from four different sources — which makes this the broadest end-to-end
// coverage in the suite.
var examples = []struct {
	name     string
	pkg      string
	minPages int
}{
	{"invoice", "./examples/invoice", 2},
	{"images", "./examples/images", 2},
	{"report", "./examples/report", 4},
	{"charts", "./examples/charts", 5},
	{"themed", "./examples/themed", 1},
}

func TestExamplesProduceValidDocuments(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping example execution in short mode")
	}

	for _, example := range examples {
		t.Run(example.name, func(t *testing.T) {
			out := filepath.Join(t.TempDir(), example.name+".pdf")

			cmd := exec.Command("go", "run", example.pkg, out)
			// The examples resolve their assets relative to the module root, which
			// is where `go test` runs.
			if combined, err := cmd.CombinedOutput(); err != nil {
				t.Fatalf("running %s: %v\n%s", example.pkg, err, combined)
			}

			data, err := os.ReadFile(out)
			if err != nil {
				t.Fatalf("reading output: %v", err)
			}

			if !bytes.HasPrefix(data, []byte("%PDF-")) {
				t.Errorf("%s did not produce a PDF", example.name)
			}
			if !bytes.HasSuffix(data, []byte("%%EOF\n")) {
				t.Errorf("%s output is truncated", example.name)
			}
			if pages := countPages(data); pages < example.minPages {
				t.Errorf("%s produced %d pages, want at least %d",
					example.name, pages, example.minPages)
			}

			assertNoSubstitutedGlyphs(t, example.name, data)

			assertRendersCleanly(t, example.name, data)
		})
	}
}

// assertNoSubstitutedGlyphs checks that no drawn text contains a question mark.
//
// A rune outside WinAnsi is replaced with '?', which is deliberate — silently
// dropping characters would hide missing content — but never what an example
// should ship. None of the examples use a question mark in prose, so any '?' in a
// text-showing operator is a substitution.
//
// The streams have to be inflated to see it. An earlier version of this check
// searched the raw file for "(?)" and therefore found nothing at all, since
// content streams are compressed by default: the assertion passed for months
// while an arrow glyph in the report rendered as a question mark.
func assertNoSubstitutedGlyphs(t *testing.T, name string, data []byte) {
	t.Helper()

	for _, run := range textRuns(t, data) {
		if strings.Contains(run, "?") {
			t.Errorf("%s draws %q, which contains a substituted glyph; "+
				"use a WinAnsi character or register a TrueType font", name, run)
		}
	}
}

var (
	streamPattern  = regexp.MustCompile(`(?s)stream\r?\n(.*?)\r?\nendstream`)
	textRunPattern = regexp.MustCompile(`\(((?:[^()\\]|\\.)*)\)\s*Tj`)
)

// textRuns returns every string drawn by a text-showing operator, inflating
// compressed streams on the way.
func textRuns(t *testing.T, data []byte) []string {
	t.Helper()

	var runs []string

	for _, match := range streamPattern.FindAllSubmatch(data, -1) {
		body := match[1]

		if inflated, err := inflate(body); err == nil {
			body = inflated
		}

		// Font programs and image samples are binary and would yield nonsense
		// matches, so only streams that look like operators are scanned.
		if !bytes.Contains(body, []byte("BT")) {
			continue
		}

		for _, run := range textRunPattern.FindAllSubmatch(body, -1) {
			runs = append(runs, string(run[1]))
		}
	}

	return runs
}

func inflate(data []byte) ([]byte, error) {
	r, err := zlib.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return io.ReadAll(r)
}

// assertRendersCleanly parses the document with Ghostscript, which is the only
// way to catch a structurally plausible file that no reader will open.
func assertRendersCleanly(t *testing.T, name string, data []byte) {
	t.Helper()

	gs, err := exec.LookPath("gs")
	if err != nil {
		t.Skip("ghostscript not installed")
	}

	path := filepath.Join(t.TempDir(), name+".pdf")
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}

	out, err := exec.Command(gs,
		"-dNOPAUSE", "-dBATCH", "-dSAFER", "-sDEVICE=nullpage", path).CombinedOutput()
	if err != nil {
		t.Fatalf("ghostscript rejected %s: %v\n%s", name, err, out)
	}

	// Ghostscript reports a malformed image or font as a recoverable warning
	// rather than a failure, so the output has to be inspected as well.
	for _, complaint := range []string{"error", "Error", "ERROR"} {
		if bytes.Contains(out, []byte(complaint)) {
			t.Errorf("ghostscript complained about %s:\n%s", name, out)
			return
		}
	}
}

// TestThemesChangeTheOutput checks the claim the themed example makes: the same
// program under two theme files produces different documents.
//
// Comparing whole files is the point. Asserting on individual colours would only
// prove the theme was read; the interesting property is that appearance is
// externalised well enough for a swap to be visible throughout.
func TestThemesChangeTheOutput(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping example execution in short mode")
	}

	dir := t.TempDir()
	rendered := map[string][]byte{}

	for _, name := range []string{"light", "dark"} {
		out := filepath.Join(dir, name+".pdf")

		cmd := exec.Command("go", "run", "./examples/themed", out,
			filepath.Join("examples", "themed", "themes", name+".json"))
		if combined, err := cmd.CombinedOutput(); err != nil {
			t.Fatalf("rendering the %s theme: %v\n%s", name, err, combined)
		}

		data, err := os.ReadFile(out)
		if err != nil {
			t.Fatal(err)
		}
		rendered[name] = data

		assertNoSubstitutedGlyphs(t, name, data)
		assertRendersCleanly(t, name, data)
	}

	if bytes.Equal(rendered["light"], rendered["dark"]) {
		t.Error("the two themes produced identical output; the theme is not being applied")
	}

	// The text is the same in both: a theme changes appearance, not content.
	lightText := textRuns(t, rendered["light"])
	darkText := textRuns(t, rendered["dark"])

	if len(lightText) != len(darkText) {
		t.Errorf("light draws %d text runs and dark draws %d; a theme should not "+
			"change what the document says", len(lightText), len(darkText))
	}
}

func TestThemeErrorsNameTheFile(t *testing.T) {
	// A program loading several themes needs to know which one was wrong.
	broken := filepath.Join(t.TempDir(), "broken.json")
	if err := os.WriteFile(broken, []byte(`{"page": {"size": "Nonesuch"}}`), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := theme.Load(broken)
	if err == nil {
		t.Fatal("expected a malformed theme to be rejected")
	}
	if !strings.Contains(err.Error(), "broken.json") {
		t.Errorf("error %q does not name the file", err)
	}
}
