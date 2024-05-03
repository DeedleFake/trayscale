#!/usr/bin/bash

# This script is intended to help with correct packaging of Trayscale.
# It is not intended for general usage. If you'd like to install
# Trayscale manually, you can read through the installation section of
# this script to see where various files are put.

_usage() {
	echo "Usage: $0 <build | install> [options]"
	echo
	echo "Modes:"
	echo "  build: build the binary with embedded version information"
	echo "    Usage: build [version]"
	echo "  install: install the binary and other files into a directory tree"
	echo "    Usage: install <destination directory>"
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

case "$1" in
	build)
		_build "$2"
		;;
	install)
		_install "$2"
		;;
	--help)
		_usage
		;;
	*)
		echo "Error: Unknown mode $1"
		_usage
		;;
esac
