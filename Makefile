# LIBPKGS is everything that is not a standalone command: the examples, the coverage
# reporter, the skip reporter and the table generator are all package main.
#
# It bounds what coverage is *measured over*, not what is tested. Including a binary would
# dilute the figure without saying anything about the engine, but its tests still have to
# run — the skip reporter has its own, and a tool that lies about what was checked is
# worse than no tool. Filtering on the package name rather than the directory means a new
# command needs no change here.
LIBPKGS := $(shell go list -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' ./...)
COVERPKG := $(shell go list -f '{{if ne .Name "main"}}{{.ImportPath}}{{end}}' ./... | paste -sd, -)

.PHONY: test
test:
	go test ./...

# skipped lists the checks that did not run and why.
#
# Ghostscript, poppler, fribidi and a font carrying Arabic presentation forms are not
# installed everywhere, and every check that needs one skips cleanly without it. Which
# ones skipped decides how much a green run actually verified, so it is worth saying out
# loud rather than leaving in a few thousand lines of verbose output.
.PHONY: skipped
skipped:
	@go test -count=1 -v ./... 2>&1 | go run ./scripts/skipreport

# race matters more here than in most libraries: fonts and themes are meant to be shared
# between documents generated concurrently, and that promise is only worth making if it is
# checked.
.PHONY: race
race:
	go test -race ./...

# vet fails on unformatted files rather than merely listing them: gofmt -l reports by
# printing, and exits zero either way, so a plain invocation here would let badly
# formatted code through a CI gate that looks like it is checking.
.PHONY: vet
vet:
	@bad=$$(gofmt -l .); test -z "$$bad" || { echo "not gofmt'd:"; echo "$$bad" | sed 's/^/  /'; exit 1; }
	go vet ./...

# cover measures every package against every test, not just each package's own
# tests. Without -coverpkg, code in render exercised by the root package's
# end-to-end tests would be reported as untested.
.PHONY: cover
cover:
	@go test -count=1 -covermode=atomic -coverpkg=$(COVERPKG) \
		-coverprofile=coverage.out $(LIBPKGS) \
		| sed -e 's/ in github.com.*$$//' -e 's|github.com/aaripurna/||'
	@echo
	@go run ./scripts/coverreport coverage.out

.PHONY: cover-html
cover-html: cover
	go tool cover -html=coverage.out -o coverage.html
	@echo "wrote coverage.html"

.PHONY: cover-func
cover-func: cover
	go tool cover -func=coverage.out

.PHONY: examples
examples: invoice images report charts themed print scripts concurrent

.PHONY: invoice
invoice:
	go run ./examples/invoice invoice.pdf

.PHONY: images
images:
	go run ./examples/images images.pdf

.PHONY: report
report:
	go run ./examples/report report.pdf

.PHONY: charts
charts:
	go run ./examples/charts charts.pdf

.PHONY: print
print:
	go run ./examples/print print.pdf

# Rendered twice on purpose: the same program, two theme files.
.PHONY: themed
themed:
	go run ./examples/themed themed-light.pdf examples/themed/themes/light.json
	go run ./examples/themed themed-dark.pdf examples/themed/themes/dark.json

# Needs a system font with Cyrillic coverage; pass one as a second argument if the
# usual locations come up empty.
.PHONY: scripts
scripts:
	go run ./examples/scripts scripts.pdf

# Generates 64 invoices in parallel and the same 64 one at a time, then compares them.
.PHONY: concurrent
concurrent:
	go run ./examples/concurrent concurrent.pdf

.PHONY: clean
clean:
	rm -f coverage.out coverage.html *.pdf
