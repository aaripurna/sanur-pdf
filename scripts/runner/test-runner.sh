#!/usr/bin/env bash
#
# Runs the CI suite the way the workflow runs it, inside a Linux container. See
# debian.Dockerfile and `make debian`.
#
# The point is not that the tests pass on Linux too. It is that which fonts and tools a
# machine has decides how much a green run actually verified, and macOS and Linux differ:
# the system font the suite picks up is Arial on one and DejaVu on the other, and they are
# not the same width. A test that depended on that passes locally and fails in CI, which
# is a slow way to find out.

set -u
set -o pipefail

cd "$(dirname "$0")/../.."

make vet
go build ./...

# Verbose, and kept, so the next step can report what did not run.
#
# Deliberately not under `set -e`. A failing suite is when the skip report matters most,
# and aborting here would throw it away; the workflow marks that step `if: always()` for
# the same reason. The status is carried past it instead.
go test -count=1 -v ./... 2>&1 | tee test.log
status=$?

# A skipped check is the failure mode this project keeps running into: an assertion that
# reads like it works and cannot fire is worse than no assertion. Skips are legitimate —
# veraPDF is not packaged for Debian — but which ones skipped decides how much a green
# run verified.
echo
go run ./scripts/skipreport test.log | tee skipped.txt

if [ "$status" -ne 0 ]; then
	exit "$status"
fi

# Fonts and themes are meant to be shared between documents generated concurrently.
# That promise is only worth making if it is checked.
make race

make cover | tee coverage.txt
