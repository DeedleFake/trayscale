#!/usr/bin/bash

# This script is intended to help with correct packaging of Trayscale.
# It is not intended for general usage. If you'd like to install
# Trayscale manually, you can read through the installation section of
# this script to see where various files are put.

usage() {
	echo "Usage: $0 <destination directory>"
	exit 2
}

version="$(git describe --tags)"
dstdir="$1"

if [ -z "$dstdir" ]; then
	usage
fi

echo "Building $version"
go build -v -trimpath -ldflags="-X 'deedles.dev/trayscale/internal/version.version=$version'" -o trayscale ./cmd/trayscale
echo

echo "Installing to $dstdir"
install -D trayscale "$dstdir/bin/trayscale"
install -Dm644 dev.deedles.Trayscale.png "$dstdir/share/icons/hicolor/256x256/apps/dev.deedles.Trayscale.png"
install -Dm644 dev.deedles.Trayscale.desktop "$dstdir/share/applications/dev.deedles.Trayscale.desktop"
install -Dm644 dev.deedles.Trayscale.metainfo.xml "$dstdir/share/metainfo/dev.deedles.Trayscale.metainfo.xml"
install -Dm644 dev.deedles.Trayscale.gschema.xml "$dstdir/share/glib-2.0/schemas/dev.deedles.Trayscale.gschema.xml"
echo

echo "Done."
