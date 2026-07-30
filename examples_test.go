package sanur_test

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
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
	{"report", "./examples/report", 3},
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

			// A glyph outside WinAnsi is replaced with a question mark, which is
			// deliberate but never what an example should ship. Catching it here
			// keeps the rendered output honest.
			if bytes.Contains(data, []byte("(?)")) {
				t.Errorf("%s contains a substituted glyph; use a WinAnsi character "+
					"or register a TrueType font", example.name)
			}

			assertRendersCleanly(t, example.name, data)
		})
	}
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
