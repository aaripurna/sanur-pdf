# Packages under test. The examples and scripts directories hold standalone
# binaries rather than library code, so including them would dilute the coverage
# figure without saying anything about the engine.
PKGS := $(shell go list ./... | grep -Ev '/(examples|scripts)/')
COVERPKG := $(shell go list ./... | grep -Ev '/(examples|scripts)/' | paste -sd, -)

.PHONY: test
test:
	go test $(PKGS)

.PHONY: vet
vet:
	gofmt -l .
	go vet ./...

# cover measures every package against every test, not just each package's own
# tests. Without -coverpkg, code in render exercised by the root package's
# end-to-end tests would be reported as untested.
.PHONY: cover
cover:
	@go test -count=1 -covermode=atomic -coverpkg=$(COVERPKG) \
		-coverprofile=coverage.out $(PKGS) \
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
examples: invoice images report charts themed

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

# Rendered twice on purpose: the same program, two theme files.
.PHONY: themed
themed:
	go run ./examples/themed themed-light.pdf examples/themed/themes/light.json
	go run ./examples/themed themed-dark.pdf examples/themed/themes/dark.json

.PHONY: clean
clean:
	rm -f coverage.out coverage.html *.pdf
