# Agent instructions

## Rules

| Rule | Enforced by |
|------|-------------|
| Update Go dependencies with `scripts/update-deps`. Do not run `go get -u` alone. After any module upgrade, pin `gvisor.dev/gvisor` to the `go` branch (`go get gvisor.dev/gvisor@go` then `go mod tidy`) in the root module and in `.agents/skills/drive-trayscale/scripts/tsharness`. gvisor's `master` branch is not usable with the normal Go toolchain. | `scripts/update-deps` performs the pin. `go build ./...` fails if `gvisor.dev/gvisor` is taken from `master` (duplicate packages in `pkg/tcpip/stack` and `pkg/refs`). |
| Import `encoding/json/v2`, not `encoding/json`. | `depguard` in `.golangci.yml` fails the import with "use encoding/json/v2". |
