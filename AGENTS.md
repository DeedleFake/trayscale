# AGENTS.md

Instructions for AI coding agents working in this repository.

## Project overview

**Trayscale** is an unofficial GUI for the Tailscale daemon, aimed primarily at Linux (no official Linux GUI client exists). It provides a system tray icon and a Libadwaita-based window for managing Tailscale features such as peers, exit nodes, Taildrop, profiles, and Mullvad exit nodes.

The app talks to the local Tailscale daemon (`tailscaled`) via the Tailscale local API and CLI helpers from the `tailscale.com` module. Full functionality requires the current user to be configured as the Tailscale operator (`sudo tailscale set --operator=$USER`).

## Technology stack

| Layer | Choice |
|-------|--------|
| Language | Go — see `go.mod` for the required toolchain |
| GUI | GTK 4 + Libadwaita via `github.com/diamondburned/gotk4` and `gotk4-adwaita` |
| Tray | `deedles.dev/tray` (StatusNotifierItem); not maintained on macOS |
| Tailscale | `tailscale.com` local client / IPN APIs |
| UI design | Cambalache (`.cmb` project + per-window `.ui` XML) |
| Settings | GSettings schema `dev.deedles.Trayscale` |
| App ID | `dev.deedles.Trayscale` |

This file should not pin toolchain or dependency versions (they go stale). Prefer “as specified in `go.mod`” (or the README for system libraries). Other project docs, such as the README, may name specific versions when useful.

## Directory structure

```
.
├── cmd/trayscale/           # Main binary entrypoint
│   ├── trayscale.go        # main(), optional PPROF CPU profiling
│   └── default.pgo         # Profile-guided optimization profile
├── internal/
│   ├── ui/                 # Application UI (gotk4/adwaita)
│   │   ├── app.go          # App lifecycle, notifications, updates
│   │   ├── mainwindow.go   # Primary window and peer stack
│   │   ├── *page.go        # Per-peer / offline / Mullvad pages
│   │   ├── *.ui            # GtkBuilder XML (edit via Cambalache)
│   │   ├── trayscale.cmb   # Cambalache project for the .ui files
│   │   ├── app.css         # Application CSS
│   │   └── ...
│   ├── tsutil/             # Tailscale client helpers and status poller
│   ├── tray/               # System tray icon and menu
│   ├── gutil/              # GTK helpers (FillFromBuilder/UI, CSS, widgets)
│   ├── listmodels/         # gio.ListModel iterators and binding helpers
│   ├── giofs/              # gio.File → io/fs bridge (file pickers / Taildrop)
│   └── metadata/           # App ID, version, license, release notes
├── assets.go               # Embedded LICENSE + metainfo (package trayscale)
├── dist.sh                 # Packaging helper (build / install / macOS)
├── dev.deedles.Trayscale.* # Desktop entry, icon, GSettings schema, AppStream metainfo
├── gschemas.compiled       # Local compiled schemas (gitignored; regenerate as needed)
└── go.mod
```

- Application code lives under `internal/`. The only public package is the module root (`assets.go`).
- UI logic is Go; layouts are GtkBuilder XML loaded with `//go:embed` and `gutil.FillFromUI`.

## Development commands

System packages needed for development (names vary by distro): GTK 4, Libadwaita, and GObject introspection development headers. CI installs the equivalent of `libgtk-4-dev`, `libadwaita-1-dev`, and `gobject-introspection` on Ubuntu.

```bash
# Download modules
go mod download

# Run (compiles as needed)
go run ./cmd/trayscale
# Optional: start with the main window hidden
go run ./cmd/trayscale --hide-window

# Tests, vet, format — go test already compiles packages; a separate
# go build is unnecessary for verification
go test ./...
go vet ./...
go fmt ./...

# Produce a binary only when you need one (not for routine checks)
go build -o trayscale ./cmd/trayscale
# Or with version injection (uses git describe if version omitted)
./dist.sh build [version]

# Staticcheck (declared as a tool in go.mod)
go tool staticcheck ./...

# Validate AppStream metainfo
appstreamcli validate --pedantic --no-net --explain dev.deedles.Trayscale.metainfo.xml
```

For local GSettings during development, compile the schema into the repo root (or install it system-wide / under `$XDG_DATA_DIRS`):

```bash
glib-compile-schemas .
# gschemas.compiled is gitignored
```

### Environment variables

| Variable | Purpose |
|----------|---------|
| `PPROF` | If set to a file path, write a CPU profile on exit |
| `TRAYSCALE_PRIVATE` | If `1`, enables private-mode behavior in `internal/metadata` |

## Packaging and distribution

- **`dist.sh build [version]`** — builds `./trayscale` with `-trimpath` and injects version via `-ldflags` into `internal/metadata.version`.
- **`dist.sh install <destdir>`** — installs binary, icon, desktop file, metainfo, and GSettings schema into a FHS-like tree.
- **`dist.sh install-macos` / `uninstall-macos`** — Homebrew-prefix install helpers (community/unofficial path).
- Flathub and AUR packages are maintained outside this repo; keep `dev.deedles.Trayscale.metainfo.xml` release notes accurate when shipping versions.

## Architecture notes

```
tailscaled (local API)
        ▲
        │
   internal/tsutil
   (Client, Poller, status types)
        │
        ▼
   internal/ui.App  ──►  MainWindow + Page stack
        │                     ├── SelfPage
        │                     ├── PeerPage
        │                     ├── MullvadPage
        │                     └── OfflinePage
        └── internal/tray (optional, GSettings tray-icon)
```

- **`tsutil.Poller`** polls / watches IPN status, waiting files, and profiles; delivers `tsutil.Status` values to `App.update`.
- **GTK main thread**: use `glib.IdleAdd` (or existing helpers) when updating UI from poller/background work.
- **Pages** implement `ui.Page` (`Widget`, `Actions`, `Init`, `Update`). Prefer extending that pattern for new peer-related UI.
- **Widgets from XML**: declare exported fields matching builder object names (or `gtk:"Name"` tags) and call `gutil.FillFromUI` / `FillFromBuilder`.
- **App ID** `dev.deedles.Trayscale` is used for the Adwaita application, notifications, GSettings, and metainfo — keep these consistent.

## UI files

- Edit layouts with [Cambalache](https://gitlab.gnome.org/jpu/cambalache) using `internal/ui/trayscale.cmb` when possible; keep `.ui` files and the `.cmb` project in sync.
- Each screen typically pairs `name.go` + `name.ui` with `//go:embed name.ui`.
- Prefer Libadwaita patterns already used in the tree (`ApplicationWindow`, `NavigationSplitView`, `ViewStack`, `ToastOverlay`, `Spinner`, etc.).

## Code style and conventions

- **Logging** — `log/slog` with structured key-value fields.
- **Context** — pass `context.Context` as the first argument for cancelable / long-running work.
- **Errors** — handle explicitly; wrap with `fmt.Errorf("...: %w", err)` when adding context. Avoid panics except in true programmer-error / must-succeed paths (e.g. embedded asset load).
- **Modern Go** — match existing use of generics, `iter`, `slices`, `maps`, `cmp`, and related stdlib helpers as used in this codebase and `go.mod`.
- **Imports** — goimports-style groups: standard library, third-party, then `deedles.dev/...`.
- **Comments** — full sentences for exported symbols; stay consistent with nearby code.
- **Scope** — prefer small, focused changes. Do not reformat unrelated files or drive-by refactors.

## Testing and CI

CI (`.github/workflows/test.yml`) runs:

1. `go vet ./...`
2. `staticcheck`
3. `go test ./...`
4. AppStream metainfo validation

Tests live next to the code they cover (`*_test.go`). Coverage is currently sparse; add tests when introducing non-trivial pure logic (helpers in `tsutil`, `metadata`, list-model utilities, etc.). GUI-heavy code need not be unit-tested unless practical.

## Agent guidelines

1. **Git is read-only under all circumstances.** Never create commits, amend, rebase, merge, cherry-pick, stash, checkout branches, reset, clean, tag, push, or otherwise mutate the git repository or index. Read-only commands (`status`, `diff`, `log`, `show`, `blame`, etc.) are fine. Leave all commits and branch management to the user.
2. **Read before writing** — match patterns in `internal/ui`, `internal/tsutil`, and existing gotk4 usage.
3. **Do not pin versions in this file** (`AGENTS.md`) — refer to `go.mod` or unversioned dependency names so agent instructions stay valid as versions change. Pinning versions elsewhere (README, comments, code) is fine when appropriate.
4. **Verify** with `go test ./...` and `go vet ./...` (and `go tool staticcheck ./...` when practical) before considering work done. Do not run `go build` solely to check that the project compiles — `go test` already builds packages.
5. **UI changes** — update both Go and `.ui` (and Cambalache project when relevant). Do not hand-edit generated or compiled schema blobs; edit `dev.deedles.Trayscale.gschema.xml` and recompile schemas if needed.
6. **Secrets / environment** — do not commit tokens or machine-specific paths. This app does not ship API keys; keep it that way.
7. **Tailscale behavior** — prefer the local API / existing `tsutil` helpers over shelling out, except where the code already uses `cli.Run` for up/down-style operations.

## PR checklist

- [ ] `go test ./...` and `go vet ./...` pass (no separate `go build` needed)
- [ ] `go fmt ./...` applied
- [ ] staticcheck clean when feasible
- [ ] Metainfo still validates if `dev.deedles.Trayscale.metainfo.xml` changed
- [ ] GSettings schema and desktop/metainfo App ID remain consistent
- [ ] No secrets in the diff
- [ ] No agent-created git commits or other git writes
