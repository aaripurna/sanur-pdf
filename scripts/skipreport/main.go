// Command skipreport lists the tests that skipped, with the reason each gave.
//
// A skipped check is the failure mode this project keeps running into: an assertion that
// reads like it works and cannot fire is worse than no assertion. Skips here are
// legitimate — Ghostscript, poppler, fribidi and a font with Arabic presentation forms
// are not installed everywhere — but which ones skipped decides how much a green run
// actually verified, and buried in a few thousand lines of verbose output nobody sees it.
//
// Usage:
//
//	go test -v ./... | go run ./scripts/skipreport
//	go run ./scripts/skipreport test.log
//
// Exits zero whether anything skipped or not: this reports, it does not judge.
package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strings"
)

// The three lines that matter in `go test -v` output.
//
// Pairing a skip with its reason cannot be done by adjacency, which is the obvious first
// attempt: a subtest logs its reason as it runs and its result is printed later, batched
// under the parent, so the two can be hundreds of lines apart. The reason is therefore
// attributed to whichever test was running when it was logged.
var (
	runLine    = regexp.MustCompile(`^=== (?:RUN|CONT|PAUSE)\s+(\S+)`)
	skipLine   = regexp.MustCompile(`^\s*--- SKIP:\s+(\S+)`)
	messageLog = regexp.MustCompile(`^\s*[^\s]+\.go:\d+:\s+(.*)$`)
)

func main() {
	input := io.Reader(os.Stdin)

	if len(os.Args) > 1 {
		file, err := os.Open(os.Args[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "skipreport: %v\n", err)
			os.Exit(1)
		}
		defer file.Close()
		input = file
	}

	skipped, err := parse(input)
	if err != nil {
		fmt.Fprintf(os.Stderr, "skipreport: %v\n", err)
		os.Exit(1)
	}

	if len(skipped) == 0 {
		fmt.Println("Nothing skipped: every check ran.")
		return
	}

	fmt.Printf("%d skipped:\n", len(skipped))
	for _, s := range skipped {
		fmt.Printf("  %s\n      %s\n", s.name, s.reason)
	}
}

type skip struct {
	name   string
	reason string
}

// parse reads verbose test output and returns the skipped tests, sorted by name.
func parse(r io.Reader) ([]skip, error) {
	var (
		current string
		reasons = map[string]string{}
		names   []string
	)

	scanner := bufio.NewScanner(r)
	// Verbose output can carry a long line from a failing assertion that dumps a whole
	// document, and the default limit would stop the scan partway through.
	scanner.Buffer(make([]byte, 0, 64*1024), 8*1024*1024)

	for scanner.Scan() {
		line := scanner.Text()

		if m := runLine.FindStringSubmatch(line); m != nil {
			current = m[1]
			continue
		}

		if m := skipLine.FindStringSubmatch(line); m != nil {
			names = append(names, m[1])
			continue
		}

		// The last message a test logs before skipping is its reason; a test that logs
		// several has the most recent one attributed to it.
		if m := messageLog.FindStringSubmatch(line); m != nil && current != "" {
			reasons[current] = strings.TrimSpace(m[1])
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := make([]skip, 0, len(names))
	seen := map[string]bool{}

	for _, name := range names {
		if seen[name] {
			continue
		}
		seen[name] = true

		reason, ok := reasons[name]
		if !ok {
			reason = "(no reason given)"
		}
		out = append(out, skip{name: name, reason: reason})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].name < out[j].name })
	return out, nil
}
