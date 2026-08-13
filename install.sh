#!/usr/bin/env bash
# Builds TermDevTools for the current platform (native build, no
# cross-compilation) and installs it together with its companion files
# (cat_columns.txt, endpoints.txt, cheatsheet.txt) into a self-contained
# directory, then symlinks the binary onto your PATH.
#
# Run from anywhere; it locates the repository from its own path. Override
# the install locations with TERMDEVTOOLS_INSTALL_DIR / TERMDEVTOOLS_BIN_DIR
# if the defaults don't suit you (e.g. a shared /opt install for a team).
set -euo pipefail
cd "$(dirname "$0")"

install_dir="${TERMDEVTOOLS_INSTALL_DIR:-$HOME/.local/share/termdevtools}"
bin_dir="${TERMDEVTOOLS_BIN_DIR:-$HOME/.local/bin}"

mkdir -p "$install_dir" "$bin_dir"

echo "Building termdevtools into $install_dir ..."
CGO_ENABLED=0 go build -trimpath -o "$install_dir/termdevtools" .

# cat_columns.txt/endpoints.txt are team-shared reference data (SPEC.md
# §9.1): always refreshed from the source tree. cheatsheet.txt is a personal
# starting point instead — only seeded once, never overwritten, so a
# previous customization survives re-running this script.
cp -f cat_columns.txt endpoints.txt "$install_dir/"
if [ ! -f "$install_dir/cheatsheet.txt" ]; then
	cp cheatsheet.txt.example "$install_dir/cheatsheet.txt"
fi

ln -sf "$install_dir/termdevtools" "$bin_dir/termdevtools"

echo
echo "Installed to:  $install_dir"
echo "Symlinked as:  $bin_dir/termdevtools"

case ":$PATH:" in
*":$bin_dir:"*) echo ; echo "Run: termdevtools" ;;
*)
	echo
	echo "$bin_dir is not on your PATH yet. Add this to your shell profile (~/.bashrc, ~/.zshrc...):"
	echo "  export PATH=\"$bin_dir:\$PATH\""
	echo
	echo "Then run: termdevtools"
	;;
esac
