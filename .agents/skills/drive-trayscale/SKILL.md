---
name: drive-trayscale
description: Launch this checkout's Trayscale in an isolated D-Bus session and drive its GTK 4 / Libadwaita window through GApplication actions and AT-SPI. Use when another skill or task needs to start, inspect, click through, or stop a private Trayscale instance, or needs fake peers from tsharness.
---

# Drive Trayscale

Trayscale is an unofficial Tailscale GUI: a Libadwaita window titled `Trayscale` plus an optional tray icon. This skill drives **this checkout's binary** in an isolated D-Bus session. It does not drive a Trayscale the user already started.

Run every command from the repo root:

```bash
CTRL=.agents/skills/drive-trayscale/scripts/control-trayscale
```

## Isolation

GApplication owns the session-bus name `dev.deedles.Trayscale`. A second process on the user bus only activates the first. `launch` starts `dbus-run-session` so the user instance can stay running.

Shared with the user and **not** isolated:

- `tailscaled` and the live tailnet
- the Wayland display (the window appears on the current session)

Two isolated launches at once are refused. One user instance plus one isolated instance is fine.

## Forbidden actions

Against a real `tailscaled`, never do these unless the calling skill explicitly allows a specific one:

- activate `use_exit_node`, `login`, `change_control_server`, `admin_dashboard`, or `quit` (all need `--i-mean-it`)
- click the unnamed header `switch` (connect/disconnect)
- toggle `Advertise exit node`, `Allow LAN access`, `Accept routes`, `Accept DNS`, or `Use as exit node`
- advertise or remove routes, or switch profiles
- activate `peer.sendFile` (`Send file...`, `Send directory...`), open a file chooser, or drop files on a peer page. Allowed only when `check-fake-tailnet` says `FAKE` and the target is tsharness's `receiver`
- click About `Website`, `Report an Issue`, or `about.save-debug-info`

If a step's AT-SPI handle is missing, skip the step. Do not substitute a forbidden action.

## Real or fake tailnet

Before any step that changes Tailscale state or sends anything (exit node, up/down, `tailscale set`, file or directory sends), run:

```bash
.agents/skills/drive-trayscale/scripts/check-fake-tailnet
```

It asks the daemon on `/run/tailscale/tailscaled.sock` (override with `SOCKET=`) for its control URL and tailnet:

- `FAKE: control http://127.0.0.1:<port>, tailnet tail-test.ts.net`, exit 0: this is tsharness. Steps a calling skill allows against tsharness may run.
- `REAL: ...` or `UNKNOWN: ...`, exit 1: this is, or might be, the user's tailnet. **Stop** and tell the user what it printed. Continue only if the user explicitly said in this conversation to run against their real tailnet.

`doctor` reports the same line as `tailnet`. `action` refuses `use_exit_node`, `login`, `change_control_server`, and `admin_dashboard` unless it says `FAKE`, even with `--i-mean-it`; `--real-tailnet` overrides that and needs the same explicit permission. `quit` is exempt because it only stops the isolated instance.

## Launch

Needs `go`, `glib-compile-schemas`, `dbus-run-session`, `gdbus`, at-spi2-core, and PyGObject with the `Atspi` 2.0 typelib (`python3 -c 'import gi; gi.require_version("Atspi", "2.0")'` must succeed).

```bash
$CTRL launch
```

This compiles `dev.deedles.Trayscale.gschema.xml` and builds `./cmd/trayscale` into `runs/<id>/` under this skill, then starts the binary under `dbus-run-session` with:

- `GSETTINGS_BACKEND=memory` and `GSETTINGS_SCHEMA_DIR` pointing at the run schema
- `TRAYSCALE_PRIVATE=1` (profile names render as `profile@example.com`)
- `GTK_A11Y=atspi` and a private AT-SPI registry
- isolated `XDG_*` directories under the run dir
- `LANGUAGE=en` and `LANG`/`LC_ALL` set to `C.UTF-8`, so labels match the table below
- the rest of the caller's environment (for example `PPROF`)

First build can take more than a minute; `--timeout` defaults to 180 seconds. A `dbind-WARNING` about `/org/a11y/atspi/cache` is ignorable if doctor then succeeds.

Without `--tray`, tray registration fails on the private bus (`failed to start tray icon` in `inner.log`); that is isolation working. `launch --tray` first starts `scripts/sni-host`, a minimal StatusNotifierWatcher and host that re-reads the item's properties and menu on every change signal, so the tray code runs as it does under a panel. Nothing shows the tray visually.

If the build fails, `launch` removes its run directory and exits nonzero. If the D-Bus name never appears, it stops the session and keeps `runs/<id>/` for its logs; delete it once read.

## Doctor

```bash
$CTRL doctor
```

Require `pgid_alive`, `binary_exists`, and `bus_name_owned` true, `window` equal to `frame 'Trayscale'`, and `gsettings_backend` equal to `memory`.

Run doctor before the first drive, after any failed drive, and before treating a surprising tree as product behavior. If it fails, read `runs/<id>/inner.log` and `runs/<id>/session.log`, then `cleanup` before another `launch`.

With `tailscaled` running but not connected, the `Not Connected` page is correct. With no `tailscaled` at all, builds without #322 block the GTK main loop after the first draw, so `doctor` reports `atspi_error`; that is the app bug, not a harness failure.

## Drive

GApplication actions are the same handlers the menus and shortcuts call (object path `/dev/deedles/Trayscale` on the isolated bus):

```bash
$CTRL actions
$CTRL action about
$CTRL action preferences
$CTRL action search-peers
$CTRL action show-offline-peers
```

AT-SPI handles:

| Handle | Role | Notes |
| --- | --- | --- |
| `Trayscale` | `frame` | Main window |
| `Search peers` | `toggle button` | Sidebar search control |
| `profile@example.com` | `combo box` | Profile dropdown under `TRAYSCALE_PRIVATE=1` |
| (empty) | `entry` | Peer search entry, present only while search is open |
| `No matching peers` | `grouping` | Sidebar placeholder when a search matches nothing |
| `This machine`, `Exit Nodes`, `Online`, `Offline` | `label` | Sidebar section titles when connected |
| `Tailscale IPs`, `Options`, `Files`, `Advertised Routes`, `Network Check` | `grouping` | This-machine page |
| `Advertise exit node` and the other option rows | `switch` | Read only |
| `Tailscale IPs`, `Misc.`, `Advertised Routes` | `grouping` | Peer page |
| `Use as exit node` | `switch` | Peer page of an exit node. Read only |
| `About` | `dialog` | After `action about` |
| `Trayscale`, `DeedleFake` | `label` | Inside the About dialog |
| `Preferences` | `dialog` | After `action preferences` |
| `General`, `Taildrop` | `grouping` | Inside Preferences |
| (empty) | `toggle button` | Page menu button in the content header: `click --role "toggle button" --name "" --exact --index 1` (index 0 is the main menu). Insensitive when no peer page is visible |
| (empty) | `menu item` | Page menu items, in order: index 0 `Copy FQDN`, 1 `Send file...`, 2 `Send directory...`. Need `snapshot --max-depth 60` to see |
| `Select dir(s) to send to <host>` / `Select file(s) ...` | `dialog` | File chooser from the send items. `press --key /` opens its location field (`text ''`); `fill --role text --name "" --exact --value <path>`; then `click --role button --name Open` (file) or `Select` (directory) |

```bash
$CTRL snapshot                      # tree to stdout; --path FILE to save
$CTRL find --role dialog --name About --exact
$CTRL wait --role dialog --name About --exact --timeout 10
$CTRL click --role toggle --name "Search peers" --exact
$CTRL select --role label --name "<peer sidebar label>" --exact
$CTRL fill --role entry --value "query"
$CTRL screenshot --path FILE --allow-fail
```

`click` uses a `click`/`press`/`activate`/`toggle` action and refuses to default to `clipboard.copy`. Sidebar rows have empty `list item` names and no actions; `select --name` selects the list item containing that label, which marks it `selected` but does not change the content page. To show a peer page, search for the peer: when the visible page is not among the matches, the first match becomes the visible page.

The search entry has no accessible name; address it as `--role entry` without `--name` while no dialog is open. `fill` replaces its text and the sidebar filters as if typed.

`press` synthesizes AT-SPI key events, which often do nothing on GNOME Wayland but work on X11 (including Xvfb). Prefer actions.

Without a GNOME session (for example Xvfb plus a private `dbus-daemon --session` as the caller's bus), everything above works except `screenshot`, which needs GNOME Shell; use `--allow-fail`. The app needs a libadwaita new enough for `AdwViewSwitcherSidebar` (Debian trixie's 1.7 fails at startup with `Invalid object type 'AdwViewSwitcherSidebar'`).

## Fake peers (tsharness)

`scripts/tsharness` runs a real `tailscaled` (userspace networking) against Tailscale's in-process test control server with a local DERP/STUN, and registers five peers with `100.64.1.x` addresses: `alpha` (online), `bravo-exit` (online, exit node), `charlie` (offline), `delta-router` (online, advertises `192.168.50.0/24`), and `echo-down` (offline, exit node). It sets the operator to `$USER`. The daemon and IPN bus traffic are real; only the control server is a test one.

```bash
.agents/skills/drive-trayscale/scripts/tsharness/run.sh   # prints READY, runs until interrupted
```

Flags: `-extra N` adds `peer-NN` peers (every third offline); `-mullvad N` adds Mullvad exit nodes in a few cities and grants this machine the `mullvad` capability, so the `Mullvad Exit Nodes` page appears; `-suggest-exit-nodes` makes exit nodes eligible for recommendation, so the `Recommended exit node` preference can resolve a node; `-churn 20s` flips one peer's online state (and toggles an extra route on routers) per tick; `-receiver` also starts a second real `tailscaled`, hostname `receiver`, and grants both nodes Taildrop, so sends from Trayscale to `receiver` complete and land in `/tmp/tsharness/receiver/state/files/*/` (a directory arrives as `<name>.tar.zst`). Clear that directory before a run that checks it. Use `/tmp/tsharness/tailscale` for CLI changes against the fake node.

It serves the LocalAPI on `/run/tailscale/tailscaled.sock`, Trayscale's default socket, so use it only where no system `tailscaled` runs (create `/run/tailscale` writable by you first). Its `go.mod` pins `tailscale.com` separately from the app's; `run.sh` builds `tailscale`/`tailscaled` at the app's version into `$DIR` (default `/tmp/tsharness`). Go skips dot-directories, so CI does not build it.

Against tsharness the forbidden actions only touch the fake node, so a calling skill may allow them, including sending files to `receiver`. Confirm with `check-fake-tailnet` first.

## Cleanup

```bash
$CTRL cleanup
```

This signals only the process group recorded at launch (the `dbus-run-session` pgid), never `pkill trayscale`. If the group leader died but members still run from the recorded run directory, it still signals the group; `doctor` reports `pgid_alive` the same way. `--keep-run` leaves the run directory for logs. Confirm the user's own instance, if any, is still alive afterward.
