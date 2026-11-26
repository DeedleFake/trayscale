---
name: verify-profiles-and-files
description: Model-check that Trayscale's profile list and waiting Taildrop files list load, refresh, and switch correctly under every interleaving of tailscaled events, the Bus, Profiles, and Files goroutines, and the GTK main thread, using a TLA+ model whose guards are read from the code. Use when a change touches Bus, Profiles, fetchLoop, await, or kick in internal/tsutil, the Files poller in internal/tsutil/files.go or its profile checks in Files.Open and Files.Delete, the FileStatus, ProfileStatus, or IPNStatus cases of App.update, showIPN, switchHold, setSwitching or the switchTimeout fallback, syncWindow, onAppActivate's refreshes, onWindowActive, the tray's profile switch, Switch account submenu, or SetSwitching, the profile drop-down, the self page's file list, save, or delete button, saveFile, deleteSaved, maybeAutoSaveFiles, retryPendingDeletes, the pending deletes in internal/ui/pending.go, or the tailscale.com version, or when running /verify-profiles-and-files.
---

# Verify profiles and files

The model in `model/ProfilesAndFiles.tla` checks what Trayscale shows about login profiles and waiting Taildrop files, what it does when you switch profiles or delete a file, and how it holds the old profile on screen while a switch is in progress. TLC, the TLA+ model checker, tries every order of the steps below within the bounds: tailscaled events, the `Bus` and `Profiles` goroutines, the `Files` poller, `gutil.IdleAdd` callbacks on the main thread, and what you click. Tests only cover the orders someone wrote down.

Profiles follow the IPN bus: `Profiles` fetches when the bus signals a change. The bus doesn't report waiting files with fields that aren't deprecated, so `Files` polls for them: a long poll while none wait and a check every `Interval` while some do. Each check reads the current profile before and after the list and tags the list with it. Its pushes go through `New` and `App.update` like everything else. Saving and deleting a file check that the list's profile is still current, and a save whose delete found another profile adds the file to the pending deletes, which run when that profile's list comes back. One model covers both lists because they share the Bus and the main thread, and because a profile switch changes which files are waiting. The `Part` constant runs the profile side and the file side separately, so neither pays for the other's states.

## Run it

From the repo root:

```bash
.agents/skills/verify-profiles-and-files/scripts/check-profiles-and-files
```

It needs Java 11 or newer, `perl`, and `curl`. Without a system Java, `mise exec java@temurin-21 -- <command>` works. The first run downloads `tla2tools.jar` 1.7.4 to `~/.cache/trayscale/` and checks its sha256. Stay on a stable release: the 1.8.0 prerelease jar is rebuilt in place, so its sha256 changes. Set `TLA2TOOLS_JAR` to use another copy. A run takes about twelve minutes on 8 cores, most of it in `autosave`. `TLC_WORKERS=2` limits the cores it uses.

To check one property alone in the main runs, ignoring the known findings list:

```bash
.agents/skills/verify-profiles-and-files/scripts/check-profiles-and-files --only ProfilesSettle
```

Output and counterexamples go to `.agents/skills/verify-profiles-and-files/out/`.

## What it checks

| Property | Plain meaning | Harm when broken |
|---|---|---|
| `ProfilesSettle` | Once nothing is in flight, the tray and the window's drop-down show the current profile and every profile that exists, unless another client changed a profile that isn't current and you haven't opened a profile list since. | You see the wrong account as active, or switch to an account that no longer exists. |
| `FailedTraySwitchNotifies` | Every failed switch from the tray shows a `Switch account` notification. | A tray switch fails silently and you think you're on the other account. |
| `MainThreadNeverWaits` | No profile switch or file delete runs a LocalAPI call on the GTK main thread. | The window freezes until tailscaled answers, up to 30 seconds. |
| `FilesSettle` | Once `Files` has read the current list, nothing is in flight, and Tailscale is running, an open window's self page lists exactly the files waiting for the current profile, including after another client took one. | Files you could save are missing from the list, or files that are gone are still in it. |
| `FilesWatched` | Whenever `Files` is blocked while online, it either waits for a file to arrive, with none waiting, or has its next check armed. So it reads the list again within `Interval` of any change. | The list stops updating until you delete a file or restart Trayscale. |
| `NoStrayFileNotice` | A `New Incoming File` notification only names a file that arrived in that profile, including the first list after coming back to a profile. | You're told about a file that has been waiting all along, or one from the other account. |
| `NoDuplicateFileNotice` | No arrival gets a second `New Incoming File` notification. | You're told about the same file twice. |
| `NoMissedFileNotice` | Once `Files` has read the current list, nothing is in flight, and Tailscale is running, every file still waiting that arrived after `Files` passed its first list for the profile got a `New Incoming File` notification. | A file arrives and you never hear about it. |
| `NoWrongDelete` | Deleting or saving a file from the list, auto-saving one from `a.waiting`, or retrying a pending delete deletes it from the profile that list came from. | You delete another account's file, which can't be undone. |
| `NoUncheckedWrongDelete` | Every wrong delete passed `Files.Delete`'s profile check: the switch landed between the check and the name-only delete, the one window LocalAPI leaves open. | A delete acts on a stale list for longer than a LocalAPI round trip, such as a whole auto-save copy. |
| `NoWrongSave` | A save, by hand or automatic, only writes a file read from the profile of the list it was picked from. | You save another account's file under this account's name. |
| `SavedFilesCleared` | Once the current profile's list is applied and everything has landed, no file saved from that profile is still waiting in it. | A file you saved stays in the incoming list after a switch, and auto-save saves it again. |
| `NoStaleFilesShown` | When the window is enabled and the self page shows a list taken from a profile, that profile is the one currently logged in. | After a switch you still see the old account's files, and saving or deleting one can hit the new account. |
| `NoStaleFilesAfterHold` | Once a switch has reached `Running`, or stalled and the 5-second fallback has fired, and the bus has been applied, an enabled self page does not still show the previous profile's files. | The hold ends, the window is usable, and the old account's files stay on the page until the next file list arrives. |
| `SwitchStaysConnected` | A profile switch that doesn't stall never shows Tailscale as offline, so it sends neither `Tailscale Status` notification. | Every switch flashes `Not Connected`, tears down the peer pages, and sends a disconnected and a connected notification. |
| `HoldEnds` | Once a switch has reached `Running`, or stalled and the 5-second fallback has fired, nothing is held, the tray and the window are enabled, and they show tailscaled's state. | The window stays greyed out, or keeps showing the old profile, after the switch is over. |
| `HeldStateDisabled` | While a switch is held, the tray's Tailscale items are disabled and an open window's content is insensitive. | You click a peer, a file, or the status switch of the profile you just left. |

`ProfilesSettle`, `FailedTraySwitchNotifies`, `FilesSettle`, `NoMissedFileNotice`, and `SavedFilesCleared` are checked only once nothing is in flight: the bus queue is read, `Profiles` is idle and `Files` is blocked with nothing pending (its `Interval` timer may be armed), every `IdleAdd` callback has run, no fallback timer is armed, and tailscaled isn't mid-switch. `HoldEnds` uses a looser condition that also counts a switch stalled in `Starting` once the fallback has fired.

The three switch properties run in every run. They only have something to check where a switch can happen: `profiles`, `switches`, `autosave`, and `returns`.

The script makes five runs:

- `profiles`: the profile side. One switch, one profile added or deleted by another client, two looks at a profile list.
- `files`: the file side with no switch. Two arrivals, one file taken by another client, one delete, one connect or disconnect. Auto-save is off.
- `switches`: the file side across one switch by another client, with one arrival, one delete, and one connect or disconnect. Auto-save is off.
- `autosave`: the file side with auto-save on, one switch, and one save from the self page. No new arrivals, so the start files are the ones auto-saved or saved.
- `returns`: the file side with two switches by another client and one save from the self page, auto-save off. It covers a save that defers its delete and the retry when the profile comes back, and switches there and back within one LocalAPI sequence.

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
| `FilesResetOnState` | `filesResets` in `files.go` including `changeState` | `FilesWatched`, in `switches` |
| `ResetOnPublished` | `drain` resetting when the latest published state isn't `r.state` | `FilesWatched`: `Files` stays offline after the bus comes online |
| `FilesGatedOnline` | `run` starting each loop with `if !r.online() { r.reset()`, and `online` reading `Bus.latest` | none in `files`; in `switches` the state space grows past 46 million states, as `Files` polls through the switch |
| `DiscardStaleFetch` | `run` calling `drain` after `check` returns and dropping the list on a reset | none in these bounds: `ProfileTagged` and `ResetOnProfile` also drop a list read across a switch. `TestFilesDiscardsCheckAcrossReset` covers it |
| `PollWhileWaiting` | `run` arming `time.After(r.interval)` after a list with files, and going straight to a long poll after an empty one | `FilesWatched` |
| `BaselineQuiet` | `push` passing the first list after a reset to `New` with no `Arrived` when `r.seen` has no list for its profile | `NoStrayFileNotice` in `files`, `NoDuplicateFileNotice` in `switches` |
| `SeenAcrossReset` | `push` giving the first list after a reset `Arrived` from `arrivedFiles(seen[profile], files)`, `remember` keeping each profile's last list, and `reset` keeping `r.seen` | `NoMissedFileNotice` in `files` with `Resends=FALSE,MaxToggles=2`: a file that arrives across a disconnect is never announced |
| `ProfileTagged` | `profileWaitingFiles` reading `currentProfile` before and after `waitingFiles` and failing with `ErrProfileChanged` when they differ, `run` resetting on it, and `push` tagging `FileStatus.Profile` | `NoStrayFileNotice` in `switches` |
| `ResetOnProfile` | `run` resetting when a list's profile isn't `r.profile` before `push` | `NoStrayFileNotice` in `switches` |
| `OpenChecksProfile` | `Files.Open` calling `checkProfile` before and after `getWaitingFile`, and `saveFile` opening through it | `NoWrongSave` in `autosave` |
| `DeleteChecksProfile` | `Files.Delete` calling `checkProfile` before `deleteWaitingFile`, and `deleteSaved` and the delete button passing the list's profile | `NoUncheckedWrongDelete` in `autosave` |
| `SaveDefersDelete` | `deleteSaved` adding the file to `a.pending` on `ErrProfileChanged` | `SavedFilesCleared` in `returns` |
| `RetriesPending` | `App.update`'s `FileStatus` case calling `retryPendingDeletes`, which deletes the pending files of the list's profile that are still waiting | `SavedFilesCleared` in `returns` |
| `RefreshRepeats` | `check` setting `again` for a `Refresh` while a check that doesn't wait is in flight | none: the next `Interval` check fixes the list, so this only makes it up to 5 seconds late, which the untimed model doesn't measure |
| `RefreshBuffered` | `Files` and `Profiles` making `refresh` with `make(chan struct{}, 1)`, and `kick` sending without blocking | `ProfilesSettle`; for files, none: the next `Interval` check fixes the list, so this only makes it up to 5 seconds late, which the untimed model doesn't measure |
| `SwitchRefreshes` | `Profiles.Switch` starting with `defer p.Refresh()` | none in these bounds: the state change after a switch already fetches, and a failed switch changes nothing to fetch |
| `DeleteRefreshes` | `Files.Delete` starting with `defer f.Refresh()` | none: the next `Interval` check fixes the list, so this only makes it up to 5 seconds late, which the untimed model doesn't measure |
| `WindowOpenRefreshes` | `onAppActivate` calling `a.files.Refresh()` and then `a.profiles.Refresh()` | `ProfilesSettle`; for files, none: the next `Interval` check fixes the list, so this only makes it up to 5 seconds late, which the untimed model doesn't measure |
| `DropDownRefreshes` | `NewMainWindow` calling `onDropDownPopup(win.ProfileDropDown, app.profiles.Refresh)` | `ProfilesSettle` |
| `FocusRefreshes` | `NewMainWindow` calling `onWindowActive(&win.MainWindow.Window, app.profiles.Refresh)`, and `onWindowActive` calling it only when the window is active | `ProfilesSettle` |
| `TrayOpenRefreshes` | the tray's `OnAccountsOpen` calling `a.profiles.Refresh()` | `ProfilesSettle` |
| `TraySwitchOffMain` | `OnProfileSwitch` calling `a.profiles.Switch` inside `go func()` | `MainThreadNeverWaits` |
| `TraySwitchFailureNotifies` | a failed tray switch calling `a.notify(locale.Get("Switch account"), ...)` | `FailedTraySwitchNotifies` |
| `DropDownSwitchOffMain` | the drop-down's `selected-item` handler calling `app.profiles.Switch` inside `go func()` | `MainThreadNeverWaits` |
| `DeleteOffMain` | the self page's delete confirmation calling `a.files.Delete` inside `go func()` | `MainThreadNeverWaits` |
| `SelfPageFillsFiles` | `SelfPage.init` calling `page.fileModel.Set(*a.waiting)` | `FilesSettle` |
| `NoticesFromArrivals` | `App.update`'s `FileStatus` case notifying only for a file whose name is in `status.Arrived`. FALSE gives the list diff against `a.waiting` that came before it | none in these bounds: `push` already compares with the last list |
| `HoldGatesFiles` | `App.update`'s `FileStatus` case updating the window only `if a.win != nil && !a.hold.active()` | none: the settle properties don't look during a hold |
| `HoldReplaysFiles` | `setSwitching(false)` calling `syncWindow`, which shows `*a.waiting` when it is not nil | none on its own: the stale-row findings are `NoStaleFilesShown` and `NoStaleFilesAfterHold`, from `syncWindow` skipping a nil `a.waiting` |
| `SwitchHold` | `App.update` starting with `a.hold.offer(a.online, status)` and returning while it holds, and `switchHold.offer` holding only `NoState` and `Starting` with `WantRunning` while online | `SwitchStaysConnected` |
| `HoldDisables` | a hold's start calling `a.setSwitching(true)`, `setSwitching` reaching the tray and the window and calling `syncActions`, `syncActions` enabling `use_exit_node` only `a.online && !a.hold.active()`, and `onAppActivate` calling `a.win.SetSwitching(a.hold.active())` | `HeldStateDisabled` |
| `HoldReenables` | `a.setSwitching(false)` after `showIPN` both when a hold ends on a new status and when the fallback fires | `HoldEnds` |
| `HoldFallback` | a hold's start arming `gutil.TimeoutAdd(switchTimeout, ...)` | `HoldEnds`, after a stall |
| `SaveOffMain` | the self page's save button calling `a.saveFile` inside `go func()` | `MainThreadNeverWaits` |
| `AutoSaveOffMain` | `maybeAutoSaveFiles` starting `a.saveFile` inside `go func()` | `MainThreadNeverWaits`, in `autosave` |
| `SyncShowsEmptyWaiting` | `syncWindow` updating the self page with an empty file list when `a.waiting` is nil | `NoStaleFilesAfterHold` holds in `autosave` when this is TRUE. In `switches`, opening the window from a stale `a.waiting` after the new profile is already `Running` still fails, so that run uses `MaxSwitches=0` |

A regex checks the shape of one guard. It does not see a new code path that changes what the guard means, such as a second place that sets `a.waiting`. Model such a path as a new action before trusting a pass.

When a check fails, compare the facts with a run on `master`:

1. A fact flipped, and the change meant to remove or move that guard. If the guard is gone, the failure is real. Report the counterexample. If the guard moved or was renamed, update its regex and rerun.
2. A fact flipped by accident, for example after a reformat. Fix the regex so it matches the same guard.
3. No fact changed, but a property fails. The change altered behavior that the model describes as fixed. Find the action in the table below, compare it with the new code, and update the model to match the code. Never loosen a property to make it pass.

## Where the model comes from

Each LocalAPI call is one step, because tailscaled serves it under `b.mu`. Each `gutil.IdleAdd` callback is one step, and they run in the order they were added, so the model keeps them in one queue, `mainQ`. Comments in the model give the file and line of each step: `bus`, `src`, `files`, `app`, `win`, `self`, and `io` are `internal/tsutil/bus.go`, `sources.go`, and `files.go`, and `internal/ui/app.go`, `mainwindow.go`, `selfpage.go`, and `io.go`. `ll` and `td` are tailscale.com's `ipn/ipnlocal/local.go` and `feature/taildrop/ext.go` at `MODELED_TAILSCALE` (find the module with `go list -m -f '{{.Dir}}' tailscale.com`).

| Model action | Code |
|---|---|
| `BusReads` | `Bus.Run` taking one event from `watchOnce`: `watchState.apply` decides the changes, `Run` sets `pending` and fires the signals before it publishes |
| `BusPublishes` | `Bus.publish`: it swaps `b.latest`, its `New` does `IdleAdd(a.update)`, and it fires `changePublished`. It waits while `SetRunning` holds publishing |
| `SetRunningHolds`, `SetRunningCalls`, `SetRunningReleases` | `Bus.SetRunning` from the window's status switch or the tray's connection toggle: hold publishing (not signals), run `tailscale up` or `down`, release |
| `ProfilesFetch`, `ProfilesPublish` | `fetchLoop` in `Profiles.Run`: `await` returns, `getProfileStatus`, then `New` does `IdleAdd(a.update)` |
| `FilesResets` | `filesRun.drain` finding a signal in `filesResets` or a published state other than `r.state`: forget the last list, drop the timer and any check in flight, and go back to the top of `run`, offline or online |
| `FilesRefreshWakes`, `FilesRefreshDuringCheck`, `FilesTimer` | `filesRun.next` returning for a `Refresh` or the `Interval` timer, or `check` taking a `Refresh`, which it repeats only for a check that doesn't wait |
| `FilesFetch`, `FilesProfileAfter` | the check goroutine's `profileWaitingFiles`: `currentProfile` (in `Top`), `waitingFiles` for the current login, which waits for a file while the last list was empty, then `currentProfile` again, failing with `ErrProfileChanged` on a change |
| `FilesPush` | `drain` finding nothing, a reset if the list's profile isn't `r.profile`, then `push`: `New` with the first list after a reset and the files not in `r.seen` for its profile (none if it has no list), or with a changed list and the files that weren't in the last one, then a long poll for an empty list or the `Interval` timer |
| `MainRuns` | one `IdleAdd` callback: `App.update` for an `IPNStatus` (`switchHold.offer`, then `showIPN` or a hold), `FileStatus` (notices, `a.waiting`, `retryPendingDeletes`, `maybeAutoSaveFiles`, and the window unless a hold is active), `deleteSaved`'s `a.pending.add`, or `ProfileStatus` (and `MainWindow.Update`), a tray click, or a notification |
| `HoldExpires` | the `switchTimeout` callback in `App.update`: `switchHold.expire`, then `showIPN` and `setSwitching(false)` |
| `UserOpensWindow` | `onAppActivate` with no window: refresh both lists, build the pages from `a.ipn` |
| `UserOpensProfileList` | the drop-down's popover showing (`onDropDownPopup`), the tray's `OnAccountsOpen`, or the window becoming active (`onWindowActive`) |
| `UserPicksInDropDown` | the drop-down's `selected-item` handler calling `Profiles.Switch` |
| `UserPicksInTray` | `OnProfileSwitch`, which does `IdleAdd` and then switches unless the profile is already active |
| `UserDeletes` | the self page's delete confirmation calling `Files.Delete` with the profile the list had at the click |
| `UserSaves` | the self page's save button calling `saveFile` with the list's profile |
| `BackgroundFileStep` | one LocalAPI call of a save, auto-save, delete, or pending-delete retry: `Files.Open`'s first check and `getWaitingFile`, its second check (then the write), `Files.Delete`'s check, and `deleteWaitingFile` with its deferred `Refresh` |
| `BackgroundFinishes`, `MainThreadFinishes` | a `Switch` returning on a goroutine, or a switch or file op on the main thread as one step |
| `OtherClientSwitches` | `tailscale switch` or another client |
| `TailscaledStarts`, `TailscaledComesBackUp` | `startLocked` after a switch: `Starting`, then `Running`. `TailscaledStarts(TRUE)` is a switch that stalls in `Starting`, as when the new profile's control server doesn't answer |
| `FileArrives` | a Taildrop transfer finishing: `PutFile` in `feature/taildrop/send.go` renames it into place, which wakes a waiting `AwaitWaitingFiles` |
| `OtherClientTakesFile` | `tailscale file get` or another client taking a waiting file |
| `OtherClientChangesProfiles` | another client adding or deleting a profile that isn't current |

Tailscale's side, at `MODELED_TAILSCALE`:

- `SwitchProfile` runs under `b.mu`. `resetForProfileChangeLocked` moves the Taildrop manager to the new login (`onChangeProfile`) and enters `NoState` before `startLocked`. So the switch, the new file directory, and the `NoState` notify are one step.
- After a switch the bus reports `NoState`, `Starting`, and `Running`, about 250ms apart, with `WantRunning` set throughout (seen on tsharness with tailscaled v1.104.4).
- Taildrop keeps one directory per login (`fileRoot`, `td:204`), so each profile has its own waiting files. `DeleteWaitingFile` sends `DELETE /localapi/v0/files/<name>` (`client/local/local.go:844-847`), which `serveFiles` passes to `ext.DeleteFile(name)` (`feature/taildrop/localapi.go:454`), which deletes in the current manager's directory (`td:330-332`). There is no delete for a given profile, so a check before it can't close the window.
- `PutFile` renames the file into place and then wakes `AwaitWaitingFiles`, which answers at once whenever any file is waiting. A profile switch doesn't wake it, and its deadline passing answers an empty list. `Rename` keeps the sent name unless a different file already has it. The bus's `IncomingFiles` and `FilesWaiting` are deprecated, so `Files` doesn't follow them.
- Nothing reports a waiting file taken by another client, or a profile that isn't current being added or deleted. `waitingFiles` returns names and sizes only (`apitype.WaitingFile`, `client/tailscale/apitype/apitype.go:54-57`), and doesn't say which profile the list came from. `ipn.Notify` carries no profile ID either, so `Files` reads `profiles/current` around the list.

When the warning says go.mod moved past `MODELED_TAILSCALE`, read each `ll:` and `td:` line in the model at the new version. Update the model where tailscaled's behavior changed, and the line numbers where only they moved. Then set `MODELED_TAILSCALE` and rerun.

`busLatest` is `b.latest`'s state. The `f` variables are `filesRun`: `fPhase` is where `run` is blocked (`off`, `wait`, or a check in flight: `list` before `waitingFiles`, `after` before the second `currentProfile`) or that it holds a list (`got`), `fLast`, `fBase`, and `fProf` are `r.last`, `r.baseline`, and `r.profile`, `fPB` the profile read before the list, `fSeen` and `fSeenP` are `r.seen` and the profiles it has, `fState` is `r.state`, `fSig` a fired reset signal `drain` hasn't taken, `fTimer` the `Interval` timer, `fLong` a check that waits for a file, and `fAgain` check's `again`. `pend` is `a.pending`. `heldSt`, `timerArmed`, `trayOff`, and `winOff` are App and UI state: the held status, the armed fallback, and the tray and window disabled by `SetSwitching`.

Ghost variables exist only to state properties. `gen` counts arrivals per profile and file. `noticed` is the latest arrival a notification or a quiet first list accounted for. `stray` records a notification for a file that never arrived in that profile, and `dup` one for an arrival already accounted for. `owed` is the latest arrival after `Files` passed its first list for the profile, and `told` the latest one a notification named. A file list carries its profile and each file's arrival as ghosts, but the code compares only names and sizes. A file list's `rp` ghost is the profile it was really read from, next to its `p` tag; they differ only after a switch there and back within one check. `saved` holds the files a save wrote and hasn't deleted, `wrongSave` a save that wrote another profile's file, and `unchecked` a wrong delete that didn't pass its own check. `fDirty` records a change to the current list since `Files` last read it. `unseenProf` records a profile change nothing reports since you last looked at a profile list. `wrongDelete`, `failedTray`, and `notices` count what their names say. `stuck` marks a switch that stalled in `Starting`, which is a real failure. `flap` records a switch that didn't stall showing offline.

## Bounds and assumptions

- Two profiles, A and B. A starts with `f1` waiting, B with `f1` and `f2`. Run bounds are in `RUNS` in the script.
- `Resends` is a bound, not a code fact. With it FALSE, a file only arrives under a name that never waited in that profile. The `files` and `switches` runs have it TRUE.
- `AutoSaveEnabled` is a bound, not a code fact. It stands for the `taildrop-auto-save` setting being on and a usable folder being set. The `profiles`, `files`, and `switches` runs have it FALSE. The `autosave` run has it TRUE, and assumes `autosave.DirOK` succeeds, so every waiting file is started exactly once (`autoSaving` skips a name already in flight; failed auto-saves are not modeled).
- A file sent again under the name of one still waiting arrives under a new name in the code, because `Rename` picks one. The model has `FileArrives` only for a name that isn't waiting.
- A switch goes through `NoState` and `Starting` into `Running`, unless the profile no longer exists. It can stall in `Starting` and recover later. The model has no `NeedsLogin` or stopped profile after a switch, so every `NoState` and `Starting` status has `WantRunning` set.
- A switch that doesn't stall reaches `Running` on the main thread well within `switchTimeout`, so the fallback only fires after a stall or after the hold has ended. Without this, an untimed model lets the 5-second timer fire after any step.
- The bounds allow one switch, so one hold. The generation check that keeps an old hold's timer from ending a newer hold is covered by `TestSwitchHold`, not by the model.
- The connect toggle, the drop-down, the tray's account items, and the delete button only act while enabled. The model doesn't start a tray icon in the middle of a hold.
- Bus connects once, at the start. No reconnects or tailscaled restarts. A transfer that finishes while Bus is reconnecting, for example after tailscaled closes a lagging watcher, gets no notification in the code. The model doesn't cover that.
- The self page exists exactly while Tailscale is `Running` and the window is open. In the code it needs a self node too.
- Adding and removing pages is part of the `IPNStatus` step that does it. `updatePeers` and `updatePages` detach the sidebar with `holdSidebar` while they change the stack's pages and reattach it before they return, on the main thread, and touch nothing the model tracks.
- `changeIdentity` is not modeled. It only adds profile fetches, which can't make `ProfilesSettle` fail, and `Files` resets on it, which a switch's state change already does.
- In the `files` and `switches` runs, auto-save is off and notifications are on, so every new file would get a notification. In the `autosave` run, auto-save is on, so `App.update` sends no `New Incoming File` notification and starts `saveFile` for each waiting name instead.
- No LocalAPI call fails or times out, other than a switch to a deleted profile. `Files`' retry backoff and its `ErrNoTaildrop` case are covered by `TestFilesRetry` and `TestFilesNoTaildrop`, not by the model.
- A long poll whose deadline passes with no file waiting answers an empty list that `push` drops as unchanged, so the model leaves it out.
- `arrivedFiles` and the pending-delete match compare names and sizes. The model compares names, as a file sent again can have the same size.
- `Files.Open`'s first check and `getWaitingFile` are one step. Its second check catches a switch between them, so splitting them adds only states.
- The pending deletes' state file, pruning entries for deleted profiles (`pendingDeletes.prune`), and a delete retried after a restart are not modeled. `internal/ui/pending_test.go` covers loading, saving, and the in-memory fallback.
- The delete button adds nothing to the pending deletes when its check fails, so a delete you asked for never runs later without you.
- The window opens at most once. Closing it drops all its state, so a second open goes the same way as the first.

## Not covered

- What the window shows while something is in flight, other than the switch hold. The settle properties only look once everything has landed.
- The peer list during a switch. Trayscale shows whatever peer list tailscaled sends with `Running`.
- Changing the auto-save setting or folder, a missing auto-save directory, and `autoSaveFailed` retries. `maybeAutoSaveFiles` is modeled when a `FileStatus` arrives, not when the setting changes.
- Picking a unique destination name (`autosave.UniquePath`). The model only tracks the waiting-file name that `saveFile` later deletes.
- Two profiles that share a login. Taildrop keys its directory by login, so they share waiting files.
- Logins, logouts, and profile deletion of the current profile.
- The exit node across a switch. `/verify-exit-node-selection` covers exit node prefs without switches.
- Engine updates and the peer list.

## Findings

Each finding lists the shortest path TLC found, in code terms. All of them come from LocalAPI: the waiting-file list and its delete are keyed by name in the current login's directory, with no profile in the request or the answer.

**A switch between `Files.Delete`'s check and the delete still deletes the other profile's file** (`NoWrongDelete` in `switches`, `autosave`, and `returns`). The check reads A, another client switches to B, and `deleteWaitingFile` deletes B's file of the same name. The window is one LocalAPI round trip, milliseconds, for a delete by hand, a save, an auto-save, and a pending-delete retry alike. `NoUncheckedWrongDelete` holds, so no delete acts on a stale list for longer. The same path breaks `SavedFilesCleared` in `returns`: a save's delete that passed its check and then hit B leaves the saved file waiting in A, and nothing tells the code the delete went to B. With `MaxSwitches=1` the run never comes back to A, so that override passes trivially. The break tests of `SaveDefersDelete` and `RetriesPending` show the property catches a missing deferral or retry. LocalAPI has no delete for a given profile (Tailscale's side above), so this can't be closed.

**The self page can show the previous profile's files after a switch** (`NoStaleFilesShown` in `switches`, `autosave`, and `returns`; `NoStaleFilesAfterHold` in `switches`). `Files` pushes A's list, another client switches to B, and the window opens or is open before the bus or `Files` catches up. The rows stay until `Files`' next list, a few milliseconds when the switch's state change resets it. Saving or deleting one of those rows now fails its profile check and shows a toast, so only the display window is left. `NoStaleFilesAfterHold` holds in `autosave` and in `switches` with `MaxSwitches=0`.

**Switching there and back within one check can tag the other profile's list** (`NoStrayFileNotice` and `NoWrongSave` in `returns`). `Files` reads A, another client switches to B, `waitingFiles` reads B's list, the client switches back to A, and the second read sees A again. The list is tagged A, so a new file in it is announced and a save writes B's file. Two switches inside one LocalAPI sequence take more than a few milliseconds of user action. Both hold with `MaxSwitches=1`.

**A file sent again under the same name within `Interval` gets no notification** (`NoMissedFileNotice` in `files` and `switches`). `f2` is taken and sent again between two checks, so `push` sees the same name and size. LocalAPI gives only names and sizes, so the code can't tell it apart. The property holds with `Resends=FALSE`, including across a disconnect in `files` with `MaxToggles=2`.

## Coverage

Sync commit: `379339c`

Move the sync commit forward only after you confirm, at the new commit, that the model and the code facts still describe the code below. The script lists covered files changed since the sync commit. The repo squash merges, so after a PR that moved the sync commit merges, set it to the merge commit on `master`.

- `internal/tsutil/sources.go`: `Profiles`, `profilesTriggers`, `kick`, `fetchLoop`, `await`
- `internal/tsutil/files.go`: `Files` (`Open`, `Delete`, `checkProfile`), `filesResets`, `filesRun` (`run`, `check`, `next`, `drain`, `reset`, `online`, `push`, `remember`), `profileWaitingFiles`, `arrivedFiles`
- `internal/tsutil/bus.go`: `Bus.Run`, `publish`, `latest`, `watchOnce`, `SetRunning`, `watchState.apply`, `fire`
- `internal/tsutil/client.go`: `waitingFiles`, `getWaitingFile`, `deleteWaitingFile`, `currentProfile`, `getProfileStatus`, `switchProfile`
- `internal/ui/app.go`: `App.update`, `showIPN`, `setSwitching`, `syncWindow`, `syncActions`, `switchTimeout`, `onAppActivate`, `startTray`, `OnProfileSwitch`, `OnAccountsOpen`, and the source wiring in `Run`
- `internal/ui/io.go`: `saveFile`, `deleteSaved`, `retryPendingDeletes`, `autoSaveSettings`, `maybeAutoSaveFiles`
- `internal/ui/pending.go`: `pendingDeletes` (`add`, `take`, `prune`)
- `internal/autosave/autosave.go`: `Enabled`, `DirOK`, `Files`, `UniquePath`
- `internal/ui/switchhold.go`: `switchHold`, `switching`
- `internal/tray/tray.go`: `SetSwitching`, `update`
- `internal/ui/mainwindow.go`: the drop-down's `selected-item` handler, `onDropDownPopup`, `onWindowActive`, `SetSwitching`, `Update`, `updateProfiles`, `updatePeers`, `ensureSectionPages`, `updatePages`, `holdSidebar`
- `internal/ui/selfpage.go`: `init`, the save and delete buttons in `fileRow`, `UpdateIPN`, `UpdateFiles`
- `go.mod`: the `tailscale.com` version
