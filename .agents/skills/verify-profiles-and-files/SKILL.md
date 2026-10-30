---
name: verify-profiles-and-files
description: Model-check that Trayscale's profile list and waiting Taildrop files list load, refresh, and switch correctly under every interleaving of tailscaled events, the Bus, Profiles, and Files goroutines, and the GTK main thread, using a TLA+ model whose guards are read from the code. Use when a change touches Bus, Files, Profiles, fetchLoop, await, or kick in internal/tsutil, the FileStatus, ProfileStatus, or IPNStatus cases of App.update, onAppActivate's refreshes, the tray's profile switch or Switch account submenu, the profile drop-down, the self page's file list or delete button, or the tailscale.com version, or when running /verify-profiles-and-files.
---

# Verify profiles and files

The model in `model/ProfilesAndFiles.tla` checks what Trayscale shows about login profiles and waiting Taildrop files, and what it does when you switch profiles or delete a file. TLC, the TLA+ model checker, tries every order of the steps below within the bounds: tailscaled events, the `Bus`, `Profiles`, and `Files` goroutines, `gutil.IdleAdd` callbacks on the main thread, and what you click. Tests only cover the orders someone wrote down.

One model covers both lists because they share the Bus, `fetchLoop`, and the main thread, and because a profile switch changes which files are waiting. The `Part` constant runs the profile side and the file side separately, so neither pays for the other's states.

## Run it

From the repo root:

```bash
.agents/skills/verify-profiles-and-files/scripts/check-profiles-and-files
```

It needs Java 11 or newer, `perl`, and `curl`. Without a system Java, `mise exec java@temurin-21 -- <command>` works. The first run downloads `tla2tools.jar` 1.7.4 to `~/.cache/trayscale/` and checks its sha256. Stay on a stable release: the 1.8.0 prerelease jar is rebuilt in place, so its sha256 changes. Set `TLA2TOOLS_JAR` to use another copy. A run takes about 2.5 minutes on 4 cores. `TLC_WORKERS=2` limits the cores it uses.

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
| `NoMissedFileNotice` | Once nothing is in flight, every file that arrived while Trayscale already held that profile's list got a `New Incoming File` notification. | A file arrives and you never hear about it. |
| `NoWrongDelete` | Deleting a file from the list deletes it from the profile the list came from. | You delete another account's file, which can't be undone. |

`ProfilesSettle`, `FailedTraySwitchNotifies`, `FilesSettle`, and `NoMissedFileNotice` are checked only once nothing is in flight: the bus queue is read, both loops are idle with nothing pending, every `IdleAdd` callback has run, and tailscaled isn't mid-switch.

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
| `DropDownSwitchOffMain` | the drop-down's `selected-item` handler calling `app.profiles.Switch` inside `go func()` | `MainThreadNeverWaits` (FALSE today, see Findings) |
| `DeleteOffMain` | the self page's delete confirmation calling `a.files.Delete` inside `go func()` | `MainThreadNeverWaits` (FALSE today, see Findings) |
| `SelfPageFillsFiles` | `SelfPage.init` calling `page.fileModel.Set(*a.waiting)` | `FilesSettle` (FALSE today, see Findings) |
| `FirstListQuiet` | `App.update` comparing a new file list against `*a.waiting` only inside `if a.waiting != nil` | `NoStrayFileNotice` |

A regex checks the shape of one guard. It does not see a new code path that changes what the guard means, such as a second place that sets `a.waiting`. Model such a path as a new action before trusting a pass.

When a check fails, compare the facts with a run on `master`:

1. A fact flipped, and the change meant to remove or move that guard. If the guard is gone, the failure is real. Report the counterexample. If the guard moved or was renamed, update its regex and rerun.
2. A fact flipped by accident, for example after a reformat. Fix the regex so it matches the same guard.
3. No fact changed, but a property fails. The change altered behavior that the model describes as fixed. Find the action in the table below, compare it with the new code, and update the model to match the code. Never loosen a property to make it pass.

## Where the model comes from

Each LocalAPI call is one step, because tailscaled serves it under `b.mu`. Each `gutil.IdleAdd` callback is one step, and they run in the order they were added, so the model keeps them in one queue, `mainQ`. Comments in the model give the file and line of each step: `bus`, `src`, `app`, `win`, `self`, and `io` are `internal/tsutil/bus.go`, `internal/tsutil/sources.go`, and `internal/ui/app.go`, `mainwindow.go`, `selfpage.go`, and `io.go`. `ll` and `td` are tailscale.com's `ipn/ipnlocal/local.go` and `feature/taildrop/ext.go` at `MODELED_TAILSCALE` (find the module with `go list -m -f '{{.Dir}}' tailscale.com`).

| Model action | Code |
|---|---|
| `BusReads` | `Bus.Run` taking one event from `watchOnce`: `watchState.apply` decides the changes, `Run` sets `pending` and fires the signals before it publishes |
| `BusPublishes` | `Bus.publish`, whose `New` does `IdleAdd(a.update)`. It waits while `SetRunning` holds publishing |
| `SetRunningHolds`, `SetRunningCalls`, `SetRunningReleases` | `Bus.SetRunning` from the window's status switch or the tray's connection toggle: hold publishing (not signals), run `tailscale up` or `down`, release |
| `ProfilesFetch`, `ProfilesPublish` | `fetchLoop` in `Profiles.Run`: `await` returns, `getProfileStatus`, then `New` does `IdleAdd(a.update)` |
| `FilesFetch`, `FilesPublish` | `fetchLoop` in `Files.Run`: `await` returns, `waitingFiles` for the current login, then `New` |
| `MainRuns` | one `IdleAdd` callback: `App.update` for an `IPNStatus`, `FileStatus`, or `ProfileStatus` (and `MainWindow.Update`), a tray click, or a notification |
| `UserOpensWindow` | `onAppActivate` with no window: refresh both lists, build the pages from `a.ipn` |
| `UserOpensProfileList` | the drop-down's popover showing (`onDropDownPopup`) or the tray's `OnAccountsOpen` |
| `UserPicksInDropDown` | the drop-down's `selected-item` handler calling `Profiles.Switch` |
| `UserPicksInTray` | `OnProfileSwitch`, which does `IdleAdd` and then switches unless the profile is already active |
| `UserDeletes` | the self page's delete confirmation calling `Files.Delete` |
| `BackgroundFinishes`, `MainThreadFinishes` | a `Switch` or `Delete` returning, with its deferred `Refresh`, on a goroutine or on the main thread |
| `OtherClientSwitches` | `tailscale switch` or another client |
| `TailscaledComesBackUp` | `startLocked` after a switch, back to `Running` |
| `FileArrives` | a Taildrop transfer finishing, which `applyFiles` reports as `changeFiles` |
| `OtherClientTakesFile` | `tailscale file get` or another client taking a waiting file |
| `OtherClientChangesProfiles` | another client adding or deleting a profile that isn't current |

Tailscale's side, at `MODELED_TAILSCALE`:

- `SwitchProfile` runs under `b.mu`. `resetForProfileChangeLocked` moves the Taildrop manager to the new login (`onChangeProfile`) and enters `NoState` before `startLocked`. So the switch, the new file directory, and the `NoState` notify are one step.
- Taildrop keeps one directory per login (`fileRoot`), so each profile has its own waiting files, and `DeleteWaitingFile` deletes by name in the current profile's directory.
- Nothing on the bus reports a waiting file taken by another client unless it was the last one, when `FilesWaiting` goes away. Nothing reports a profile that isn't current being added or deleted.

When the warning says go.mod moved past `MODELED_TAILSCALE`, read each `ll:` and `td:` line in the model at the new version. Update the model where tailscaled's behavior changed, and the line numbers where only they moved. Then set `MODELED_TAILSCALE` and rerun.

Ghost variables exist only to state properties. `gen` counts arrivals per profile and file. `noticed` is the latest arrival a notification or a quiet first list accounted for, and `stray` records a notification for anything else. `owed` is the latest arrival that landed while Trayscale held that profile's list, and `told` the latest one a notification named. `unseenProf` and `unseenFiles` record a change nothing reports since you last looked. `wrongDelete`, `failedTray`, and `notices` count what their names say.

## Bounds and assumptions

- Two profiles, A and B. A starts with `f1` waiting, B with `f1` and `f2`. Run bounds are in `RUNS` in the script.
- `Resends` is a bound, not a code fact. With it FALSE, a file only arrives under a name that never waited in that profile. The `files` and `switches` runs have it TRUE. One known finding needs it, so `NoMissedFileNotice` is also checked with it FALSE.
- A switch always succeeds into `Running`, unless the profile no longer exists. The model has no `NeedsLogin`, `Starting`, or stopped profile after a switch.
- Bus connects once, at the start. No reconnects or tailscaled restarts.
- The self page exists exactly while Tailscale is `Running` and the window is open. In the code it needs a self node too.
- `changeIdentity` is not modeled. It only adds profile fetches, which can't make `ProfilesSettle` fail.
- Auto-save is off and notifications are on, so every new file would get a notification.
- No LocalAPI call fails or times out, other than a switch to a deleted profile.
- The window opens at most once. Closing it drops all its state, so a second open goes the same way as the first.

## Not covered

- What the window shows while something is in flight. The settle properties only look once everything has landed.
- `saveFile`'s delete after saving, and auto-save. Both pick files by name from `a.waiting` like the delete button does, so the `NoWrongDelete` finding likely applies to them too (inferred, not modeled).
- Two profiles that share a login. Taildrop keys its directory by login, so they share waiting files.
- Logins, logouts, and profile deletion of the current profile.
- The exit node across a switch. `/verify-exit-node-selection` covers exit node prefs without switches.
- Engine updates and the peer list.

## Findings

Each finding lists the shortest path TLC found, in code terms.

**The drop-down switches profiles on the main thread** (`MainThreadNeverWaits` in `profiles`). Open the window, pick another profile in the drop-down. The `selected-item` handler in `mainwindow.go` calls `app.profiles.Switch` directly, so the window is frozen until tailscaled answers, up to 30 seconds. The tray already switches in a goroutine.

**The delete button deletes on the main thread** (`MainThreadNeverWaits` in `files` and `switches`). Open the window, delete a waiting file, confirm. The confirmation callback in `selfpage.go` calls `a.files.Delete` directly, with `context.TODO()` and no timeout.

**The self page starts with an empty file list** (`FilesSettle` in `files` and `switches`). At startup the window opens before the first status arrives. `Files` fetches the list and `App.update` stores it in `a.waiting`. Then the first `Running` status arrives and `ensureSectionPages` creates the self page. `SelfPage.init` copies only the count from `a.waiting`, for the badge, and not the files. Nothing fetches the list again, so the page says `No incoming files.` while the badge counts them. The same happens whenever the self page is rebuilt after Tailscale comes back online and the list was fetched before the status landed.

**A switch can announce the new profile's old files** (`NoStrayFileNotice` in `switches`). Trayscale fetches A's list. Another client switches to B. `Bus.Run` reads the `NoState` event and fires `changeState` before it queues the status, so `Files` fetches and queues B's list. `App.update` applies A's list and then B's list, while `a.waiting` still holds A's list because the offline status hasn't run yet. `f2`, which had been waiting in B all along, gets a `New Incoming File` notification. A connect or disconnect in progress widens the window, because `SetRunning` holds statuses but not signals (inferred from the code).

**A delete right after a switch can delete the other profile's file** (`NoWrongDelete` in `switches`). The window shows A's list with `f1`. Another client switches to B, and B also has a file named `f1`. Before the `NoState` status reaches the main thread, you delete `f1` from the list. `DeleteWaitingFile` deletes by name in the current login's directory, so B's `f1` is gone and A's stays.

**Some arrivals get no notification** (`NoMissedFileNotice` in `files` and `switches`). Two causes:

1. A stop during an arrival. Trayscale holds A's list. You turn Tailscale off. While `SetRunning` holds statuses, `f2` arrives and `Files` fetches a list with it. The `Stopped` status then sets `a.waiting` to nil, and the next list, which has `f2`, is the quiet first list, so `f2` never gets a notification.
2. A resend under the same name. Trayscale shows `f1`. You delete it, or another client takes it, and the same file is sent again before the next fetch. `await` folds both changes into one fetch, the new list matches `a.waiting` by name and size, and `App.update` sees nothing new.

With `Resends` FALSE and no connect or disconnect, the property holds.

## Coverage

Sync commit: `f8dc492`

Move the sync commit forward only after you confirm, at the new commit, that the model and the code facts still describe the code below. The script lists covered files changed since the sync commit. The repo squash merges, so after a PR that moved the sync commit merges, set it to the merge commit on `master`.

- `internal/tsutil/sources.go`: `Files`, `Profiles`, `filesTriggers`, `profilesTriggers`, `kick`, `fetchLoop`, `await`
- `internal/tsutil/bus.go`: `Bus.Run`, `publish`, `watchOnce`, `SetRunning`, `watchState.apply`, `applyFiles`, `fire`
- `internal/tsutil/client.go`: `waitingFiles`, `deleteWaitingFile`, `getProfileStatus`, `switchProfile`
- `internal/ui/app.go`: `App.update`, `onAppActivate`, `OnProfileSwitch`, `OnAccountsOpen`, and the source wiring in `Run`
- `internal/ui/mainwindow.go`: the drop-down's `selected-item` handler, `onDropDownPopup`, `Update`, `updateProfiles`, `ensureSectionPages`, `updatePages`
- `internal/ui/selfpage.go`: `init`, the delete button in `fileRow`, `UpdateIPN`, `UpdateFiles`
- `go.mod`: the `tailscale.com` version
