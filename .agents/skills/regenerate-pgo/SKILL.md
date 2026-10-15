---
name: regenerate-pgo
description: Regenerate cmd/trayscale/default.pgo from a CPU profile of this checkout's Trayscale under realistic load. Use when updating PGO, refreshing profile-guided optimization, or before a release when UI, tray, or poller code changed.
---

# Regenerate PGO

Replace `cmd/trayscale/default.pgo` with a CPU profile that covers what Trayscale spends its time on in normal use. Uses `drive-trayscale` for launch, driving, tray, fake peers, and cleanup; read it first.

## What to cover

Trayscale mostly sits idle and reacts to IPN bus notifications. The hot path is:

1. `tsutil.Poller` applies a notification (`applyNotify`, `applyPeersChanged`).
2. `App.update` notifies on connection changes and calls `Tray.Update` and `MainWindow.Update`.
3. `MainWindow.updatePeers` creates pages for new peers, calls `Update` on **every** page (self, peers, Mullvad), removes stale ones, and restacks the sidebar.

So the profile is driven by **state churn across many peers**, not by clicking. Clicks add page construction, search filtering, and dialogs on top.

## Setup

This needs tsharness, so only run it where no real `tailscaled` owns `/run/tailscale/tailscaled.sock`. If one does, stop and tell the user; do not profile against their tailnet.

From the repo root:

```bash
CTRL=.agents/skills/drive-trayscale/scripts/control-trayscale
TS=/tmp/tsharness/tailscale
PROFILE=$(mktemp /tmp/trayscale-pgo.XXXXXX.pprof)
.agents/skills/drive-trayscale/scripts/tsharness/run.sh -extra 30 -mullvad 150 -churn 1s &   # wait for READY
PPROF="$PROFILE" $CTRL launch --tray
$CTRL doctor
```

That gives about 190 peers, 150 of them Mullvad exit nodes, with one peer flipping online state every second. Doctor must pass, and `runs/<id>/inner.log` (under `drive-trayscale`) must show no `failed to start tray icon` error. Everything here talks to the fake node, so `drive-trayscale`'s forbidden list is relaxed only for the `tailscale` CLI calls below. Still never touch files, links, profiles, or the user's real daemon.

## Drive

Let churn run the whole time (aim for about two minutes total). In this order:

1. Search and peer pages: `$CTRL action search-peers`, then `fill --role entry` with `peer-1`, `zzz` (no match), `bravo-exit`, `delta-router`, and empty, about 3s apart. Each match shows that peer's page while churn updates it. Close search with `action search-peers`.
2. Offline toggle: `$CTRL action show-offline-peers` twice.
3. Exit node: `$TS set --exit-node=bravo-exit`, wait 10s, `$TS set --exit-node=`. This changes prefs, the exit-node section, the tray icon, and the menu.
4. Connection: `$TS down`, wait 10s, `$TS up`, wait 15s. This covers the offline page, the connection notifications, and rebuilding every page.
5. Dialogs: `$CTRL action about`, wait for `dialog 'About'`; `$CTRL action preferences`, wait for `dialog 'Preferences'`. Do not click inside them.
6. Wait 30s more with the window open.

Skip a step whose handle is missing, and say so in the report. The Mullvad page is updated by churn, but its location search is not reachable: AT-SPI `select` does not switch the visible page.

## Stop and install

The profile is written only when `main` returns. `cleanup` sends `SIGTERM`, which skips that, so quit first:

```bash
$CTRL action quit --i-mean-it
```

Wait until `inner.log` shows `profiling stopped` (doctor errors once the app is gone). If it does not appear within about 10s, send `SIGINT` to the `trayscale` PID running from the current run directory, never by name. Then `$CTRL cleanup`, stop tsharness with `SIGINT`, and check:

```bash
go tool pprof -top -nodecount=40 "$PROFILE"
go tool pprof -top -nodecount=40 cmd/trayscale/default.pgo
```

The new profile must load and show nonzero cumulative time for `MainWindow.updatePeers`, `PeerPage.Update`, `MullvadPage.Update`, and `tray.(*Tray).Update` (use `-nodefraction=0`). If it fails to load or any of those are missing, leave `default.pgo` alone and report why. Otherwise:

```bash
cp "$PROFILE" cmd/trayscale/default.pgo && rm -f "$PROFILE"
go test -vet=all ./...
```

Do not merge with the old profile. Commit as `cmd/trayscale: regenerate default.pgo`.

Report which steps ran or were skipped, and compare the two `-top` listings for `internal/ui`, `internal/tray`, and `internal/tsutil`.
