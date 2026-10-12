#!/bin/sh
# Builds tailscale and tailscaled at the version in trayscale's go.mod
# and runs the harness. Extra arguments go to the harness.
set -e
cd "$(dirname "$0")"
DIR=${DIR:-/tmp/tsharness}
mkdir -p "$DIR"
V=$(cd ../../../../.. && go list -m -f '{{.Version}}' tailscale.com)
[ -x "$DIR/tailscaled" ] || GOBIN=$DIR go install "tailscale.com/cmd/tailscaled@$V"
[ -x "$DIR/tailscale" ] || GOBIN=$DIR go install "tailscale.com/cmd/tailscale@$V"
exec go run . -dir "$DIR" "$@"
