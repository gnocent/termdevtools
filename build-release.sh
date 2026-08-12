#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")"
rm -rf dist

build() {
  name="$1"
  goos="$2"
  goarch="$3"
  outbin="$4"
  outdir="dist/$name"
  mkdir -p "$outdir"
  echo "building $name..."
  CGO_ENABLED=0 GOOS="$goos" GOARCH="$goarch" go build -trimpath -o "$outdir/$outbin" .
  cp cat_columns.txt endpoints.txt cheatsheet.txt.example config.yaml.example "$outdir/"
}

build linux-amd64   linux   amd64 termdevtools
build windows-amd64 windows amd64 termdevtools.exe
build darwin-arm64  darwin  arm64 termdevtools

find dist -type f | sort
echo "---sizes---"
du -h dist/*/*
