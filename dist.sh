#!/bin/bash

# This script is intended to help with correct packaging of Trayscale.
# It is not intended for general usage. If you'd like to install
# Trayscale manually, you can read through the installation section of
# this script to see where various files are put.

_usage() {
	echo "Usage: $0 <build | install | install-macos | uninstall-macos> [options]"
	echo
	echo "Modes:"
	echo "  build: build the binary with embedded version information"
	echo "    Usage: build [version]"
	echo "  install: install the binary and other files into a directory tree"
	echo "    Usage: install <destination directory>"
	echo "  install-macos: install Trayscale for macOS (Apple Silicon)"
	echo "    Usage: install-macos"
	echo "  uninstall-macos: uninstall Trayscale from macOS"
	echo "    Usage: uninstall-macos"
	exit 2
}

_build() {
	version="$1"
	if [ -z "$version" ]; then
		version="$(git describe --tags --always)"
	fi

	echo "Building $version"
	go build -v -trimpath -ldflags="-w -s -X 'deedles.dev/trayscale/internal/version.version=$version'" -o trayscale ./cmd/trayscale
}

_install() {
	dstdir="$1"
	if [ -z "$dstdir" ]; then
		_usage
	fi

	echo "Installing to $dstdir"
	install -D -t "$dstdir/bin" trayscale
	install -Dm644 -t "$dstdir/share/icons/hicolor/256x256/apps" dev.deedles.Trayscale.png
	install -Dm644 -t "$dstdir/share/applications" dev.deedles.Trayscale.desktop
	install -Dm644 -t "$dstdir/share/metainfo" dev.deedles.Trayscale.metainfo.xml
	install -Dm644 -t "$dstdir/share/glib-2.0/schemas" dev.deedles.Trayscale.gschema.xml
}

_install_macos() {
	if [ -z "$HOMEBREW_PREFIX" ]; then
		echo "Error: HOMEBREW_PREFIX not set. Is Homebrew installed?"
		exit 1
	fi
	
	echo "Installing for macOS"
	# Create wrapper script to set up environment
	cat > trayscale.wrapper << EOF
#!/bin/bash
export XDG_DATA_DIRS="$HOMEBREW_PREFIX/share:\${XDG_DATA_DIRS:-/usr/local/share:/usr/share}"
exec "$HOMEBREW_PREFIX/libexec/trayscale"
EOF
	chmod +x trayscale.wrapper
	
	# Install binary to libexec (actual executable)
	install -d "$HOMEBREW_PREFIX/libexec"
	install trayscale "$HOMEBREW_PREFIX/libexec/"
	
	# Install wrapper to bin (what users will run)
	install trayscale.wrapper "$HOMEBREW_PREFIX/bin/trayscale"
	rm trayscale.wrapper
	
	schema_dir="$HOMEBREW_PREFIX/share/glib-2.0/schemas"
	install -d "$schema_dir"
	install -m644 dev.deedles.Trayscale.gschema.xml "$schema_dir/"
	glib-compile-schemas "$schema_dir"
	
	share_dir="$HOMEBREW_PREFIX/share"
	install -d "$share_dir/applications"
	install -m644 dev.deedles.Trayscale.desktop "$share_dir/applications/"
	
	install -d "$share_dir/icons/hicolor/256x256/apps"
	install -m644 dev.deedles.Trayscale.png "$share_dir/icons/hicolor/256x256/apps/"
	
	echo "Installation complete. You can now run 'trayscale' from anywhere"
}

_uninstall_macos() {
	if [ -z "$HOMEBREW_PREFIX" ]; then
		echo "Error: HOMEBREW_PREFIX not set. Is Homebrew installed?"
		exit 1
	fi

	echo "Uninstalling from macOS"
	rm -f "$HOMEBREW_PREFIX/bin/trayscale"
	rm -f "$HOMEBREW_PREFIX/libexec/trayscale"
	
	schema_dir="$HOMEBREW_PREFIX/share/glib-2.0/schemas"
	rm -f "$schema_dir/dev.deedles.Trayscale.gschema.xml"
	glib-compile-schemas "$schema_dir"
	
	share_dir="$HOMEBREW_PREFIX/share"
	rm -f "$share_dir/applications/dev.deedles.Trayscale.desktop"
	rm -f "$share_dir/icons/hicolor/256x256/apps/dev.deedles.Trayscale.png"
	
	echo "Uninstallation complete"
}

case "$1" in
	build)
		_build "$2"
		;;
	install)
		_install "$2"
		;;
	install-macos)
		_install_macos
		;;
	uninstall-macos)
		_uninstall_macos
		;;
	--help)
		_usage
		;;
	*)
		echo "Error: Unknown mode $1"
		_usage
		;;
esac
