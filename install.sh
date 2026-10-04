#!/usr/bin/env bash
# Builds TermDevTools for the current platform (native build, no
# cross-compilation), installs the binary — all there is to install: its
# reference data and recipes are built in — then symlinks it onto your PATH.
#
# Run from anywhere; it locates the repository from its own path. Override
# the install locations with TERMDEVTOOLS_INSTALL_DIR / TERMDEVTOOLS_BIN_DIR
# if the defaults don't suit you (e.g. a shared /opt install for a team).
set -euo pipefail
cd "$(dirname "$0")"

install_dir="${TERMDEVTOOLS_INSTALL_DIR:-$HOME/.local/share/termdevtools}"
bin_dir="${TERMDEVTOOLS_BIN_DIR:-$HOME/.local/bin}"

mkdir -p "$install_dir" "$bin_dir"

# What "termdevtools --version" will answer: the tag of the checkout, or how
# far past it this build is.
version="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"

echo "Building termdevtools $version into $install_dir ..."
CGO_ENABLED=0 go build -trimpath -ldflags "-X main.version=$version" -o "$install_dir/termdevtools" .

ln -sf "$install_dir/termdevtools" "$bin_dir/termdevtools"

echo
echo "Installed to:  $install_dir"
echo "Symlinked as:  $bin_dir/termdevtools"

# Versions up to 0.5 installed companion files next to the binary. Nothing is
# deleted here: endpoints.txt and cheatsheet.txt are still read, as the
# team's own additions (SPEC.md §9.1), and may have been customized.
for legacy in cat_columns.txt endpoints.txt cheatsheet.txt; do
	if [ -f "$install_dir/$legacy" ]; then
		case "$legacy" in
		cat_columns.txt) note="no longer read, can be deleted" ;;
		endpoints.txt) note="now built in; keep it only if you added endpoints of your own to it" ;;
		cheatsheet.txt) note="still the editor's starting content; delete it to get the built-in one" ;;
		esac
		echo "Left over from an earlier version: $install_dir/$legacy ($note)"
	fi
done

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
