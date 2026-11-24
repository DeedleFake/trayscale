---
name: update-deps
description: Update Trayscale Go module dependencies. Use when bumping go.mod, running go get -u, or updating indirect deps such as gvisor.dev/gvisor.
---

# Update dependencies

Run `scripts/update-deps` from the repo root. Do not run `go get -u` by itself.

That script updates the root module and `.agents/skills/drive-trayscale/scripts/tsharness`, then re-pins `gvisor.dev/gvisor` to the `go` branch (`go get gvisor.dev/gvisor@go` and `go mod tidy`). gvisor's `master` branch is not usable with the normal Go toolchain; `go get -u` follows `master` and the build fails with duplicate packages in `pkg/tcpip/stack` and `pkg/refs`.

Do not skip the gvisor bump or leave it at an older pin to "avoid breakage." If a change still breaks the build after the script runs, stop and report it.
