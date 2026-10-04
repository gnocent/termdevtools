#!/usr/bin/env bash
# Cross-compiles the release binaries: one self-sufficient file per platform,
# with nothing to ship next to it (endpoints, _cat columns and recipes are
# built into the binary), and their SHA-256 checksums.
#
#   ./build-release.sh [version]
#
# The version stamped into the binaries ("termdevtools --version") is the
# argument if there is one, otherwise what "git describe" says of the
# checkout: the tag itself on a tagged commit, "v0.5-3-gabc1234" three commits
# later, with "-dirty" if files are modified.
set -euo pipefail
cd "$(dirname "$0")"

version="${1:-$(git describe --tags --always --dirty 2>/dev/null || echo dev)}"

rm -rf dist
mkdir -p dist

build() {
  goos="$1"
  goarch="$2"
  out="dist/termdevtools-$goos-$goarch$3"
  echo "building $out ($version)..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" \
    go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$out" .
}

build linux   amd64 ""
build windows amd64 .exe
build darwin  arm64 ""

# What lets a binary carried to a machine without Internet access be checked
# there: "sha256sum -c SHA256SUMS" (Linux), "shasum -a 256 -c SHA256SUMS"
# (macOS), "Get-FileHash" (Windows).
if command -v sha256sum >/dev/null 2>&1; then
  (cd dist && sha256sum termdevtools-* > SHA256SUMS)
else # macOS has no sha256sum
  (cd dist && shasum -a 256 termdevtools-* > SHA256SUMS)
fi

echo "---sizes---"
du -h dist/termdevtools-*
echo "---checksums---"
cat dist/SHA256SUMS
