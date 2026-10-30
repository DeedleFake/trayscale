---
name: verify-profiles-and-files
description: Model-check that Trayscale's profile list and waiting Taildrop files list load, refresh, and switch correctly under every interleaving of tailscaled events, the Bus, Profiles, and Files goroutines, and the GTK main thread, using a TLA+ model whose guards are read from the code. Use when a change touches Bus, Files, Profiles, fetchLoop, await, or kick in internal/tsutil, the FileStatus, ProfileStatus, or IPNStatus cases of App.update, showIPN, switchHold, setSwitching or the switchTimeout fallback, onAppActivate's refreshes, the tray's profile switch, Switch account submenu, or SetSwitching, the profile drop-down, the self page's file list or delete button, or the tailscale.com version, or when running /verify-profiles-and-files.
---

# Verify profiles and files

The model in `model/ProfilesAndFiles.tla` checks what Trayscale shows about login profiles and waiting Taildrop files, what it does when you switch profiles or delete a file, and how it holds the old profile on screen while a switch is in progress. TLC, the TLA+ model checker, tries every order of the steps below within the bounds: tailscaled events, the `Bus`, `Profiles`, and `Files` goroutines, `gutil.IdleAdd` callbacks on the main thread, and what you click. Tests only cover the orders someone wrote down.

One model covers both lists because they share the Bus, `fetchLoop`, and the main thread, and because a profile switch changes which files are waiting. The `Part` constant runs the profile side and the file side separately, so neither pays for the other's states.

## Run it

From the repo root:

```bash
.agents/skills/verify-profiles-and-files/scripts/check-profiles-and-files
```

It needs Java 11 or newer, `perl`, and `curl`. Without a system Java, `mise exec java@temurin-21 -- <command>` works. The first run downloads `tla2tools.jar` 1.7.4 to `~/.cache/trayscale/` and checks its sha256. Stay on a stable release: the 1.8.0 prerelease jar is rebuilt in place, so its sha256 changes. Set `TLA2TOOLS_JAR` to use another copy. A run takes about 4 minutes on 8 cores. `TLC_WORKERS=2` limits the cores it uses.

To check one property alone in the main runs, ignoring the known findings list:

```bash
.agents/skills/verify-profiles-and-files/scripts/check-profiles-and-files --only ProfilesSettle
```

Output and counterexamples go to `.agents/skills/verify-profiles-and-files/out/`. CI runs the same command in the `profiles and files` job.

## What it checks

| Property | Plain meaning | Harm when broken |
|---|---|---|
| `ProfilesSettle` | Once nothing is in flight, the tray and the window's drop-down show the current profile and every profile that exists, unless another client changed a profile that isn't current and you haven't opened a profile list since. | You see the wrong account as active, or switch to an account that no longer exists. |
| `FailedTraySwitchNotifies` | Every failed switch from the tray shows a `Switch account` notification. | A tray switch fails silently and you think you're on the other account. |
| `MainThreadNeverWaits` | No profile switch or file delete runs a LocalAPI call on the GTK main thread. | The window freezes until tailscaled answers, up to 30 seconds. |
| `FilesSettle` | Once nothing is in flight and Tailscale is running, an open window's self page lists exactly the files waiting for the current profile, unless another client took a file and you haven't reopened the window since. | Files you could save are missing from the list, or files that are gone are still in it. |
| `NoStrayFileNotice` | A `New Incoming File` notification only names a file that just arrived and wasn't announced before. | You're told about a file that has been waiting all along, or about the same file twice. |
| `NoMissedFileNotice` | Once nothing is in flight, every file that arrived and is still waiting got a `New Incoming File` notification. | A file arrives and you never hear about it. |
| `NoWrongDelete` | Deleting a file from the list deletes it from the profile the list came from. | You delete another account's file, which can't be undone. |
| `SwitchStaysConnected` | A profile switch that doesn't stall never shows Tailscale as offline, so it sends neither `Tailscale Status` notification. | Every switch flashes `Not Connected`, tears down the peer pages, and sends a disconnected and a connected notification. |
| `HoldEnds` | Once a switch has reached `Running`, or stalled and the 5-second fallback has fired, nothing is held, the tray and the window are enabled, and they show tailscaled's state. | The window stays greyed out, or keeps showing the old profile, after the switch is over. |
| `HeldStateDisabled` | While a switch is held, the tray's Tailscale items are disabled and an open window's content is insensitive. | You click a peer, a file, or the status switch of the profile you just left. |

`ProfilesSettle`, `FailedTraySwitchNotifies`, `FilesSettle`, and `NoMissedFileNotice` are checked only once nothing is in flight: the bus queue is read, both loops are idle with nothing pending, every `IdleAdd` callback has run, no fallback timer is armed, and tailscaled isn't mid-switch. `HoldEnds` uses a looser condition that also counts a switch stalled in `Starting` once the fallback has fired.

The three switch properties run in every run. They only have something to check where a switch can happen: `profiles` and `switches`.

The script makes three runs:

- `profiles`: the profile side. One switch, one profile added or deleted by another client, two looks at a profile list.
- `files`: the file side with no switch. Two arrivals, one file taken by another client, one delete, one connect or disconnect.
- `switches`: the file side across one switch by another client, with one arrival, one delete, and one connect or disconnect.

## Read the result

- `PASS` lists every property that held in every reachable state of that run.
- `FAIL <property>` means a new break. Open the `.out` file it names. Its `State N:` lines are the steps, and the variables under each step show what changed.
- `KNOWN FINDING still present` is expected for the findings below. The check exits 0 when only those fail.
- `PASS ... with <overrides>` is the other half of each finding: with the finding's cause removed, the property holds. A `FAIL ... with <overrides>` is a new break hiding behind a known finding.
- `FIXED? <property> now passes` means the model no longer finds that break. Confirm the fix in the code with a test, since a code fact can flip without the bug being gone. Then remove the entry from `KNOWN_FINDINGS` in the script and from Findings below. The check exits 1 until you do.
- `TOOL ERROR` means TLC did not finish. Read the `.out` file. It is usually a model syntax or type error, or an action that leaves a variable unassigned.
- A `warning: go.mod has tailscale.com ...` line means the daemon side of the model may be stale. See Tailscale's side below.

Read every result with the `/statechart` skill. It turns a violation into its shortest path in plain words and shows which violations a change fixes. The runs are larger than its chart budget, so it draws only the bug paths. Give it the `.cfg` that the script wrote next to the `.out` you are reading.

## Real bug or modeling error

The script prints `code facts` first. Each is a model constant read from the code by a regex in `scripts/check-profiles-and-files`. The last column is the first property TLC reports with that fact alone FALSE and the known findings' causes removed.

| Constant | TRUE when the code has | Fails without it |
|---|---|---|
| `ProfilesOnState` | `profilesTriggers` in `sources.go` including `changeState` | `ProfilesSettle` |
| `FilesOnState` | `filesTriggers` including `changeState` | `FilesSettle`, in `switches` |
| `FilesOnFiles` | `filesTriggers` including `changeFiles` | `NoMissedFileNotice` |
| `RefreshBuffered` | `Files` and `Profiles` making `refresh` with `make(chan struct{}, 1)`, and `kick` sending without blocking | `ProfilesSettle`, `FilesSettle` |
| `SwitchRefreshes` | `Profiles.Switch` starting with `defer p.Refresh()` | none in these bounds: the state change after a switch already fetches, and a failed switch changes nothing to fetch |
| `DeleteRefreshes` | `Files.Delete` starting with `defer f.Refresh()` | `FilesSettle` |
| `WindowOpenRefreshes` | `onAppActivate` calling `a.files.Refresh()` and then `a.profiles.Refresh()` | `ProfilesSettle`, `FilesSettle` |
| `DropDownRefreshes` | `NewMainWindow` calling `onDropDownPopup(win.ProfileDropDown, app.profiles.Refresh)` | `ProfilesSettle` |
| `TrayOpenRefreshes` | the tray's `OnAccountsOpen` calling `a.profiles.Refresh()` | `ProfilesSettle` |
| `TraySwitchOffMain` | `OnProfileSwitch` calling `a.profiles.Switch` inside `go func()` | `MainThreadNeverWaits` |
| `TraySwitchFailureNotifies` | a failed tray switch calling `a.notify(locale.Get("Switch account"), ...)` | `FailedTraySwitchNotifies` |
| `DropDownSwitchOffMain` | the drop-down's `selected-item` handler calling `app.profiles.Switch` inside `go func()` | `MainThreadNeverWaits` |
| `DeleteOffMain` | the self page's delete confirmation calling `a.files.Delete` inside `go func()` | `MainThreadNeverWaits` |
| `SelfPageFillsFiles` | `SelfPage.init` calling `page.fileModel.Set(*a.waiting)` | `FilesSettle` |
| `NoticesFromArrivals` | `App.update`'s `FileStatus` case notifying only for a file whose name is in `status.Arrived`. FALSE gives the list diff against `a.waiting` that came before it | `NoMissedFileNotice` |
| `ArrivalsOnce` | `applyFiles` adding a finished transfer to `w.arrived` only if `w.received` doesn't have it | `NoStrayFileNotice`, in `files` |
| `ArrivedBeforeList` | `Files.Run` calling `f.Bus.takeArrived()` right before `waitingFiles` | `NoMissedFileNotice` |
| `SwitchHold` | `App.update` starting with `a.hold.offer(a.online, status)` and returning while it holds, and `switchHold.offer` holding only `NoState` and `Starting` with `WantRunning` while online | `SwitchStaysConnected` |
| `HoldDisables` | a hold's start calling `a.setSwitching(true)`, `setSwitching` reaching the tray and the window, and `onAppActivate` calling `a.win.SetSwitching(a.hold.active())` | `HeldStateDisabled` |
| `HoldReenables` | `a.setSwitching(false)` after `showIPN` both when a hold ends on a new status and when the fallback fires | `HoldEnds` |
| `HoldFallback` | a hold's start arming `gutil.TimeoutAdd(switchTimeout, ...)` | `HoldEnds`, after a stall |

A regex checks the shape of one guard. It does not see a new code path that changes what the guard means, such as a second place that sets `a.waiting`. Model such a path as a new action before trusting a pass.

When a check fails, compare the facts with a run on `master`:

1. A fact flipped, and the change meant to remove or move that guard. If the guard is gone, the failure is real. Report the counterexample. If the guard moved or was renamed, update its regex and rerun.
2. A fact flipped by accident, for example after a reformat. Fix the regex so it matches the same guard.
3. No fact changed, but a property fails. The change altered behavior that the model describes as fixed. Find the action in the table below, compare it with the new code, and update the model to match the code. Never loosen a property to make it pass.

## Where the model comes from

Each LocalAPI call is one step, because tailscaled serves it under `b.mu`. Each `gutil.IdleAdd` callback is one step, and they run in the order they were added, so the model keeps them in one queue, `mainQ`. Comments in the model give the file and line of each step: `bus`, `src`, `app`, `win`, `self`, and `io` are `internal/tsutil/bus.go`, `internal/tsutil/sources.go`, and `internal/ui/app.go`, `mainwindow.go`, `selfpage.go`, and `io.go`. `ll` and `td` are tailscale.com's `ipn/ipnlocal/local.go` and `feature/taildrop/ext.go` at `MODELED_TAILSCALE` (find the module with `go list -m -f '{{.Dir}}' tailscale.com`).

| Model action | Code |
|---|---|
| `BusReads` | `Bus.Run` taking one event from `watchOnce`: `watchState.apply` decides the changes and `applyFiles` the arrivals, `Run` sets `pending`, adds the arrivals to `b.arrived`, and fires the signals before it publishes |
| `BusPublishes` | `Bus.publish`, whose `New` does `IdleAdd(a.update)`. It waits while `SetRunning` holds publishing |
| `SetRunningHolds`, `SetRunningCalls`, `SetRunningReleases` | `Bus.SetRunning` from the window's status switch or the tray's connection toggle: hold publishing (not signals), run `tailscale up` or `down`, release |
| `ProfilesFetch`, `ProfilesPublish` | `fetchLoop` in `Profiles.Run`: `await` returns, `getProfileStatus`, then `New` does `IdleAdd(a.update)` |
| `FilesFetch`, `FilesPublish` | `fetchLoop` in `Files.Run`: `await` returns, `takeArrived`, `waitingFiles` for the current login, then `New` with the list and the arrivals |
| `MainRuns` | one `IdleAdd` callback: `App.update` for an `IPNStatus` (`switchHold.offer`, then `showIPN` or a hold), `FileStatus`, or `ProfileStatus` (and `MainWindow.Update`), a tray click, or a notification |
| `HoldExpires` | the `switchTimeout` callback in `App.update`: `switchHold.expire`, then `showIPN` and `setSwitching(false)` |
| `UserOpensWindow` | `onAppActivate` with no window: refresh both lists, build the pages from `a.ipn` |
| `UserOpensProfileList` | the drop-down's popover showing (`onDropDownPopup`) or the tray's `OnAccountsOpen` |
| `UserPicksInDropDown` | the drop-down's `selected-item` handler calling `Profiles.Switch` |
| `UserPicksInTray` | `OnProfileSwitch`, which does `IdleAdd` and then switches unless the profile is already active |
| `UserDeletes` | the self page's delete confirmation calling `Files.Delete` |
| `BackgroundFinishes`, `MainThreadFinishes` | a `Switch` or `Delete` returning, with its deferred `Refresh`, on a goroutine or on the main thread |
| `OtherClientSwitches` | `tailscale switch` or another client |
| `TailscaledStarts`, `TailscaledComesBackUp` | `startLocked` after a switch: `Starting`, then `Running`. `TailscaledStarts(TRUE)` is a switch that stalls in `Starting`, as when the new profile's control server doesn't answer |
| `FileArrives` | a Taildrop transfer finishing: `PutFile` in `feature/taildrop/send.go` renames it into place and sends `IncomingFiles` with `Done` and `FinalPath`, which `applyFiles` reports as `changeFiles` and an arrival |
| `OtherClientTakesFile` | `tailscale file get` or another client taking a waiting file |
| `OtherClientChangesProfiles` | another client adding or deleting a profile that isn't current |

Tailscale's side, at `MODELED_TAILSCALE`:

- `SwitchProfile` runs under `b.mu`. `resetForProfileChangeLocked` moves the Taildrop manager to the new login (`onChangeProfile`) and enters `NoState` before `startLocked`. So the switch, the new file directory, and the `NoState` notify are one step.
- After a switch the bus reports `NoState`, `Starting`, and `Running`, about 250ms apart, with `WantRunning` set throughout (seen on tsharness with tailscaled v1.104.4).
- Taildrop keeps one directory per login (`fileRoot`), so each profile has its own waiting files, and `DeleteWaitingFile` deletes by name in the current profile's directory.
- `PutFile` sets `Done`, renames the file into place, sets `FinalPath`, and then sends `IncomingFiles`. A finished transfer stays in the list until `PutFile` returns, so another transfer's progress notify can list it again. `Rename` keeps the sent name unless a different file already has it. `IncomingFiles` is deprecated but still sent at v1.100 and v1.104.4.
- Nothing on the bus reports a waiting file taken by another client unless it was the last one, when `FilesWaiting` goes away. Nothing reports a profile that isn't current being added or deleted.

When the warning says go.mod moved past `MODELED_TAILSCALE`, read each `ll:` and `td:` line in the model at the new version. Update the model where tailscaled's behavior changed, and the line numbers where only they moved. Then set `MODELED_TAILSCALE` and rerun.

`arrived` is `b.arrived`, the arrivals no fetch has taken yet. `heldSt`, `timerArmed`, `trayOff`, and `winOff` are App and UI state: the held status, the armed fallback, and the tray and window disabled by `SetSwitching`.

Ghost variables exist only to state properties. `gen` counts arrivals per profile and file. `noticed` is the latest arrival a notification or a quiet first list accounted for, and `stray` records a notification for anything else. `owed` is the latest arrival, and `told` the latest one a notification named. Each arrival in `arrived` and in a file list's `arr` carries its profile and arrival as ghosts, but the code compares only the name. `unseenProf` and `unseenFiles` record a change nothing reports since you last looked. `wrongDelete`, `failedTray`, and `notices` count what their names say. `stuck` marks a switch that stalled in `Starting`, which is a real failure. `flap` records a switch that didn't stall showing offline.

## Bounds and assumptions

- Two profiles, A and B. A starts with `f1` waiting, B with `f1` and `f2`. Run bounds are in `RUNS` in the script.
- `Resends` is a bound, not a code fact. With it FALSE, a file only arrives under a name that never waited in that profile. The `files` and `switches` runs have it TRUE.
- A file sent again under the name of one still waiting arrives under a new name in the code, because `Rename` picks one. The model has `FileArrives` only for a name that isn't waiting.
- A switch goes through `NoState` and `Starting` into `Running`, unless the profile no longer exists. It can stall in `Starting` and recover later. The model has no `NeedsLogin` or stopped profile after a switch, so every `NoState` and `Starting` status has `WantRunning` set.
- A switch that doesn't stall reaches `Running` on the main thread well within `switchTimeout`, so the fallback only fires after a stall or after the hold has ended. Without this, an untimed model lets the 5-second timer fire after any step.
- The bounds allow one switch, so one hold. The generation check that keeps an old hold's timer from ending a newer hold is covered by `TestSwitchHold`, not by the model.
- The connect toggle, the drop-down, the tray's account items, and the delete button only act while enabled. The model doesn't start a tray icon in the middle of a hold.
- Bus connects once, at the start. No reconnects or tailscaled restarts. A transfer that finishes while Bus is reconnecting, for example after tailscaled closes a lagging watcher, gets no notification in the code. The model doesn't cover that.
- The self page exists exactly while Tailscale is `Running` and the window is open. In the code it needs a self node too.
- `changeIdentity` is not modeled. It only adds profile fetches, which can't make `ProfilesSettle` fail.
- Auto-save is off and notifications are on, so every new file would get a notification.
- No LocalAPI call fails or times out, other than a switch to a deleted profile.
- The window opens at most once. Closing it drops all its state, so a second open goes the same way as the first.

## Not covered

- What the window shows while something is in flight, other than the switch hold. The settle properties only look once everything has landed.
- The peer list during a switch. Trayscale shows whatever peer list tailscaled sends with `Running`.
- `saveFile`'s delete after saving, and auto-save. Both pick files by name from `a.waiting` like the delete button does, so the `NoWrongDelete` finding likely applies to them too (inferred, not modeled).
- Two profiles that share a login. Taildrop keys its directory by login, so they share waiting files.
- Logins, logouts, and profile deletion of the current profile.
- The exit node across a switch. `/verify-exit-node-selection` covers exit node prefs without switches.
- Engine updates and the peer list.

## Findings

Each finding lists the shortest path TLC found, in code terms.

**An arrival just before a switch can announce the new profile's old file** (`NoStrayFileNotice` in `switches`). `f2` arrives in A, and `Bus.Run` adds it to `b.arrived` and fires `changeFiles`. Before `Files` fetches, another client switches to B. `Files` takes the arrival and fetches B's list, which has had an `f2` waiting all along. `App.update` finds `f2` in both and sends a `New Incoming File` notification for B's `f2`. The window is the time between the arrival notify and the `waitingFiles` call, a few milliseconds. The bus can tell which profile an arrival landed in, but tailscaled doesn't say which profile a waiting files list came from, so the code can't tell the two `f2`s apart.

**A delete right after a switch can delete the other profile's file** (`NoWrongDelete` in `switches`). The window shows A's list with `f1`. Another client switches to B, and B also has a file named `f1`. Before the `NoState` status reaches the main thread, you delete `f1` from the list. `DeleteWaitingFile` deletes by name in the current login's directory, so B's `f1` is gone and A's stays. The LocalAPI has no delete for a given profile, at v1.100 or v1.104.4.

## Coverage

Sync commit: `fa9adf4`

Move the sync commit forward only after you confirm, at the new commit, that the model and the code facts still describe the code below. The script lists covered files changed since the sync commit. The repo squash merges, so after a PR that moved the sync commit merges, set it to the merge commit on `master`.

- `internal/tsutil/sources.go`: `Files`, `Profiles`, `filesTriggers`, `profilesTriggers`, `kick`, `fetchLoop`, `await`
- `internal/tsutil/bus.go`: `Bus.Run`, `publish`, `watchOnce`, `SetRunning`, `takeArrived`, `watchState.apply`, `applyFiles`, `fire`
- `internal/tsutil/client.go`: `waitingFiles`, `deleteWaitingFile`, `getProfileStatus`, `switchProfile`
- `internal/ui/app.go`: `App.update`, `showIPN`, `setSwitching`, `switchTimeout`, `onAppActivate`, `startTray`, `OnProfileSwitch`, `OnAccountsOpen`, and the source wiring in `Run`
- `internal/ui/switchhold.go`: `switchHold`, `switching`
- `internal/tray/tray.go`: `SetSwitching`, `update`
- `internal/ui/mainwindow.go`: the drop-down's `selected-item` handler, `onDropDownPopup`, `SetSwitching`, `Update`, `updateProfiles`, `ensureSectionPages`, `updatePages`
- `internal/ui/selfpage.go`: `init`, the delete button in `fileRow`, `UpdateIPN`, `UpdateFiles`
- `go.mod`: the `tailscale.com` version
