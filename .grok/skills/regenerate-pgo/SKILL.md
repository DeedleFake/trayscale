---
name: regenerate-pgo
description: >
  Regenerate cmd/trayscale/default.pgo by launching an isolated Trayscale,
  driving representative UI paths under PPROF, then replacing the profile.
  Use when updating PGO, collecting a CPU profile for default.pgo,
  refreshing profile-guided optimization, or when the user runs /regenerate-pgo.
---

# Regenerate PGO

Replace `cmd/trayscale/default.pgo` with a CPU profile from this checkout's binary.

Read `.cursor/skills/verify-trayscale/SKILL.md` first. Use `control-trayscale` only as the launch/drive/cleanup mechanism. Do not follow its verification recipes and do not write proof artifacts.

Run every command from the repo root:

```bash
CTRL=.cursor/skills/verify-trayscale/scripts/control-trayscale
PROFILE=$(mktemp /tmp/trayscale-pgo.XXXXXX.pprof)
```

Keep that `$PROFILE` path for launch, validation, and the final copy.

## Launch

`control-trayscale launch` copies the process environment into the isolated session. `cmd/trayscale` writes a CPU profile to `PPROF` when `main` returns.

```bash
PPROF="$PROFILE" $CTRL launch
$CTRL doctor
```

Doctor requirements are the same as verify-trayscale. If doctor fails, read the run logs, run `cleanup`, and stop.

The live `tailscaled` is shared. Memory GSettings do not isolate Tailscale prefs or Taildrop.

## Do not

Follow verify-trayscale forbidden actions (`use_exit_node`, `login`, `change_control_server`, `admin_dashboard`, the header connect switch, option switches). `quit` is required at the end and is the exception.

Also never:

- activate `peer.sendFile`, open a send/receive file chooser, or drop files on a peer page
- click `Use as exit node`
- advertise or remove routes
- switch profiles
- click About `Website`, `Report an Issue`, or `about.save-debug-info`

Skip a coverage step whose AT-SPI handle is missing. Do not substitute a forbidden action.

## Drive

Keep the process up long enough for the 5s poller to run several times.

1. Snapshot the tree to stdout. Section titles are `This machine`, `Exit Nodes`, `Online`, and `Offline`.
2. If the tree has `Not Connected`, skip peer pages. Still open About and Preferences.
3. This machine: select the hostname row if the self page is not showing (`grouping 'Tailscale IPs'`). Click one Tailscale IP copy button if a button is named with an address.
4. If `Mullvad Exit Nodes` is in the tree, `select --role label --name "Mullvad Exit Nodes" --exact`. Do not enable an exit node. Do not expand every country.
5. Select up to five `Online` peer labels and one `Offline` peer label, if those rows exist.
6. `$CTRL action search-peers`. If `fill` on the search entry fails, skip typing.
7. `$CTRL action about`, then `$CTRL wait --role dialog --name About --exact --timeout 10`. Do not click links.
8. `$CTRL action preferences`, then wait for `dialog 'Preferences'`. Do not toggle rows.
9. `sleep 15` so the poller samples the open UI.

Tray code is unreachable on the isolated bus. Leave it uncovered.

## Stop and install

The profile is complete only after `pprof.StopCPUProfile` runs, which happens when `main` returns. `cleanup` sends `SIGTERM` and skips that defer.

```bash
$CTRL action quit --i-mean-it
```

Wait until doctor no longer reports a live window or `bus_name_owned`. If the process is still up after about 10s, send `SIGINT` to the isolated `trayscale` PID (the binary under the verify run dir). Do not signal by binary name.

Then:

```bash
$CTRL cleanup
go tool pprof -top -nodecount=20 "$PROFILE"
cp "$PROFILE" cmd/trayscale/default.pgo
rm -f "$PROFILE"
go test -vet=all ./...
```

`go tool pprof` must print sampled functions. If it errors, the profile is truncated; leave `cmd/trayscale/default.pgo` unchanged.

Replace `cmd/trayscale/default.pgo`. Do not merge with `go tool pprof -proto`. Do not leave `$PROFILE` in the repo.

Report which coverage steps ran, which were skipped and why, and that `go test -vet=all ./...` passed.
