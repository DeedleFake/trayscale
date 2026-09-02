---
name: verify-trayscale
description: Drive the Trayscale GTK 4 / Libadwaita desktop UI the way a user does. Use when proving a Trayscale UI change, checking the main window, About, Preferences, peer search, or this-machine page against a live tailscaled.
---

# Verify Trayscale

Trayscale is an unofficial Tailscale GUI. The user-facing surface is a Libadwaita window titled `Trayscale` plus an optional tray icon. There is no web UI and no first-party CLI besides `tailscale` itself.

This skill drives **this checkout's binary** in an isolated D-Bus session. It does not drive a Trayscale the user already started.

Set `CTRL` to the helper and run every command from the repo root:

```bash
CTRL=.cursor/skills/verify-trayscale/scripts/control-trayscale
```

Read `features/README.md` before a proof. Drive every entry point the matching feature file lists, not one convenient path.

## Isolation

GApplication owns the session-bus name `dev.deedles.Trayscale`. A second process on the user bus only activates the first. `launch` starts `dbus-run-session` so the user instance can stay running.

Shared with the user and **not** isolated:

- `tailscaled` and the live tailnet
- the Wayland display (the verify window appears on the current session)

Do not activate `use_exit_node`, `login`, `change_control_server`, `admin_dashboard`, or `quit` unless a feature file says to and passes `--i-mean-it`. Do not click the unnamed header `switch` (connect/disconnect). Do not toggle `Advertise exit node`, `Allow LAN access`, `Accept routes`, `Accept DNS`, or `Use as exit node`.

Two verify launches at once are refused. Two user-plus-verify instances are allowed.

## Launch

```bash
$CTRL launch
```

This compiles `dev.deedles.Trayscale.gschema.xml` into the run dir, builds `./cmd/trayscale` into that run dir, then starts the binary under `dbus-run-session` with:

- `GSETTINGS_BACKEND=memory` and `GSETTINGS_SCHEMA_DIR` pointing at the run schema
- `TRAYSCALE_PRIVATE=1` (profile names render as `profile@example.com`)
- `GTK_A11Y=atspi` and a private AT-SPI registry
- isolated `XDG_*` directories under the run dir

Ready when `doctor` prints `"window": "frame 'Trayscale'"` and `"bus_name_owned": true`. First build can take more than a minute. `--timeout` default is 180 seconds. A `dbind-WARNING` about `/org/a11y/atspi/cache` during launch is ignorable if doctor then succeeds.

Tray registration on the private bus is expected to fail (`StatusNotifierWatcher` is absent). That is isolation working, not a product bug.

Teardown is `cleanup`. There is no long-lived server besides the process `launch` started.

## Doctor

```bash
$CTRL doctor
```

Require all of:

- `pgid_alive` true
- `binary_exists` true
- `bus_name_owned` true
- `window` equal to `frame 'Trayscale'`
- `gsettings_backend` equal to `memory`

Run doctor before the first drive, after any failed drive, and before treating a surprising tree as product behavior. If doctor fails, read `runs/<id>/inner.log` and `runs/<id>/session.log`, then `cleanup` before another `launch`.

## Drive

GApplication actions are the same handlers the menus and shortcuts call. Object path `/dev/deedles/Trayscale` on the isolated bus.

```bash
$CTRL actions
$CTRL action about
$CTRL action preferences
$CTRL action search-peers
```

AT-SPI handles that exist in this app:

| Handle | Role | Notes |
| --- | --- | --- |
| `Trayscale` | `frame` | Main window |
| `Search peers` | `toggle button` | Sidebar search control |
| `This machine`, `Exit Nodes`, `Online`, `Offline` | `label` | Sidebar section titles when connected |
| `Tailscale IPs`, `Options`, `Files`, `Advertised Routes`, `Network Check` | `grouping` | This-machine page |
| `Advertise exit node` and the other option rows | `switch` | Read only |
| `About` | `dialog` | After `action about` |
| `Trayscale`, `DeedleFake` | `label` | Inside the About dialog |
| `Preferences` | `dialog` | After `action preferences` |
| `General`, `Taildrop` | `grouping` | Inside Preferences |

```bash
$CTRL snapshot --path artifacts/example.tree.txt
$CTRL find --role dialog --name About --exact
$CTRL wait --role dialog --name About --exact --timeout 10
$CTRL click --role toggle --name "Search peers" --exact
$CTRL select --role label --name "<peer sidebar label>" --exact
$CTRL fill --role entry --name "Search peers" --value "query"
```

`click` uses a `click`/`press`/`activate`/`toggle` action. It refuses to default to `clipboard.copy` (GTK exposes that on selectable labels). Sidebar rows have empty `list item` names; `select --name` selects the list item that contains that label. Selection proves the row is selected (`states=selected`). It does not always realize the peer page widget tree.

`fill` needs an AT-SPI text interface. GtkSearchEntry currently has none, so peer-search typing is not automatable. Do not treat a missing entry as a product regression unless the feature file says the entry became exposed.

`press` synthesizes AT-SPI key events. On GNOME Wayland those events often do nothing. Prefer `action search-peers` over `press --key Control+f`.

## Evidence

Store proof under `.cursor/skills/verify-trayscale/artifacts/<feature>/`. Cleanup deletes `runs/` and does not delete `artifacts/`. Do not commit artifacts; they can contain peer names and Tailscale IPs.

Required for a UI proof:

1. Drive the user path (window, menu action, or listed control), not an internal setter.
2. Capture the tree **before** the action and **after** it when the action changes state.
3. The after-tree must identify Trayscale (`frame 'Trayscale'` or `application 'trayscale'`) and the feature's end state.

```bash
$CTRL snapshot --path .cursor/skills/verify-trayscale/artifacts/<feature>/after.tree.txt
$CTRL screenshot --path .cursor/skills/verify-trayscale/artifacts/<feature>/after.png --allow-fail
```

GNOME Shell on this kind of session returns `AccessDenied` for `org.gnome.Shell.Screenshot`. `--allow-fail` writes `after.png.skipped.txt` and continues. An AT-SPI snapshot is enough when the screenshot is denied.

Mocks: none. The window talks to the real `tailscaled`. If Tailscale is down, the offline page (`Not Connected`) is the correct UI. Do not fake IPN status.

## Cleanup

```bash
$CTRL cleanup
```

This signals only the process group recorded at launch (the `dbus-run-session` pgid). It never `pkill trayscale`. Confirm the user's instance, if any, is still alive after cleanup.

`--keep-run` leaves the run directory for log inspection. Artifacts stay either way.

## Helpers

`scripts/control-trayscale` is executable. Commands: `launch`, `doctor`, `cleanup`, `actions`, `action`, `snapshot`, `screenshot`, `find`, `click`, `select`, `fill`, `press`, `wait`.
