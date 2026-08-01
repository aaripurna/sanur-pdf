# A Linux environment for running the suite on a machine that is not Linux.
#
# The image holds the environment, not the project: no source is copied in, and the
# working tree is bind-mounted at run time instead. So it is built once and stays built
# through every edit, each run tests exactly what is on disk including uncommitted
# changes, and there is nothing for a .dockerignore to exclude. `make debian` has the
# invocation, with volumes for the Go caches — those must not live on the bind mount,
# because on macOS the working tree arrives over virtiofs and the build cache writing
# thousands of small files across it is the slow part.
#
# Debian rather than the ubuntu-latest of CI. What actually differs between platforms
# here is font metrics, and both take these fonts from the same upstream packages.
FROM docker.io/golang:1.26.5-trixie

# The same list the workflow installs, for the same reason: the suite verifies its output
# by handing it to real implementations rather than only inspecting bytes it wrote itself,
# and every one of those checks skips cleanly when the tool is absent. A container without
# them would look green while checking a great deal less.
#
#   ghostscript    parses the whole document and rejects a structurally plausible file
#                  that no reader would open
#   poppler-utils  pdftotext proves the text is text; pdffonts proves an embedded font is
#                  embedded, subsetted and mapped back to Unicode
#   fribidi        the reference implementation the bidirectional algorithm is compared
#                  against, character for character
#
# The two font packages carrying PostScript outlines are there because the CFF checks
# search for a font rather than naming one. veraPDF is not in this list because Debian does
# not package it; it is installed separately below.
RUN apt-get update && \
    apt-get install -y --no-install-recommends \
        ghostscript \
        poppler-utils \
        libfribidi-bin \
        libfribidi-dev \
        fonts-dejavu \
        fonts-stix \
        fonts-texgyre && \
    rm -rf /var/lib/apt/lists/*

# veraPDF, the PDF/UA reference validator, pinned to the version Homebrew installs on
# macOS so both platforms validate against the same rules.
#
# It is not packaged for Debian, and the workflow leaves it out for a reason: its IzPack
# installer takes an answer file naming the installer's own panel identifiers, which change
# between releases, so CI would break on an upgrade for reasons unrelated to this project.
# That argument does not apply here. The version is pinned, so nothing moves underneath the
# answers below, and the console installer is driven directly instead of through an XML
# file — the prompts are the interface, and they are the same every time for a given build.
#
# Worth the trouble: veraPDF found six real defects that the hand-written structural tests
# passed straight over, including one that made every tagged sequence read as untagged.
# Without it, the two PDF/UA conformance tests are the only checks that skip on Linux.
#
# Their server sends the 33 MB installer at about 50 KB/s, so the download dominates this
# layer. It is kept in a cache mount rather than the layer, which is what makes editing
# anything above this line survivable: the cache outlives layer invalidation, so the
# download happens once on a machine and not once per rebuild.
ARG VERAPDF_VERSION=1.30.2
RUN --mount=type=cache,target=/var/cache/verapdf \
    apt-get update && \
    apt-get install -y --no-install-recommends default-jre-headless unzip && \
    rm -rf /var/lib/apt/lists/* && \
    zip="/var/cache/verapdf/verapdf-${VERAPDF_VERSION}.zip" && \
    if [ ! -s "$zip" ]; then \
        curl -sSL -o "$zip" \
            "https://software.verapdf.org/releases/1.30/verapdf-greenfield-${VERAPDF_VERSION}-installer.zip"; \
    fi && \
    unzip -q "$zip" -d /tmp/verapdf && \
    cd "/tmp/verapdf/verapdf-greenfield-${VERAPDF_VERSION}" && \
    # In order: dismiss the welcome, set the install path, confirm creating it, continue,
    # then one answer per optional pack — GUI no, CLI yes, documentation no, sample
    # plugins no — continue, decline the offer to write an install script, and finish.
    printf '1\n/opt/verapdf\nO\n1\nN\nY\nN\nN\n1\nN\n1\n' | \
        java -jar "verapdf-izpack-installer-${VERAPDF_VERSION}.jar" -console && \
    ln -s /opt/verapdf/verapdf /usr/local/bin/verapdf && \
    # Leave the directory before deleting it. A process whose working directory has been
    # removed cannot start a JVM at all — it fails in Properties init, long before main —
    # and the check below is a JVM.
    cd / && \
    rm -rf /tmp/verapdf && \
    # A silent failure here would look exactly like the skip it is meant to remove.
    verapdf --version

# The mounted tree belongs to the host user, not to root, so git calls it dubious and
# refuses to read it. That matters more than it sounds: Go stamps VCS information into
# every build, so a git that exits non-zero makes `go list` emit nothing at all — and an
# empty package list is what silently turned a coverage report over nine packages into a
# confident, wrong figure over one.
RUN git config --global --add safe.directory /app

WORKDIR /app

ENTRYPOINT [ "/app/scripts/runner/test-runner.sh" ]
