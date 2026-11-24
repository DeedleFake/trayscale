---
name: verify-connection
description: Model-check that Trayscale's connection and login lifecycle keeps App.online, the status switch, and the offline page in step with tailscaled, clears engine and waiting state when offline, offers login instead of SetRunning when the node needs auth, always releases a SetRunning hold, resyncs after a tailscaled drop, and observes the BrowseToURL that StartLogin publishes, using a TLA+ model whose guards are read from the code. Use when a change touches Bus.Run, watch, watchOnce, publish, Next, Latest, or SetRunning, App.online, update, showIPN, setRunning, toggleConn, or login, MainWindow.setStatus or updateStatusSwitch, OfflinePage.NeedsAuthGroup, the tray's OnConnToggle, IPNStatus.Online or NeedsAuth, Start, Stop, or StartLogin, or the tailscale.com version, or when running /verify-connection.
---

# Verify connection

The model in `model/Connection.tla` checks that turning Tailscale on or off from the status switch or the tray leaves the window on the state tailscaled settled on, that disconnecting clears engine and waiting state, that a node in `NeedsLogin` is offered a login instead of `SetRunning(true)`, that a `SetRunning` hold always releases, that the UI resyncs after tailscaled drops and comes back, and that `App.login` opens the `BrowseToURL` tailscaled published for that attempt. TLC, the TLA+ model checker, tries every order of the steps below within the bounds. Tests only cover the orders someone wrote down.

## Run it

From the repo root:

```bash
.agents/skills/verify-connection/scripts/check-connection
```

It needs Java 11 or newer, `perl`, and `curl`. Without a system Java, `mise exec java@temurin-21 -- <command>` works. The first run downloads `tla2tools.jar` 1.7.4, the latest stable release, to `~/.cache/trayscale/` and checks its sha256. Set `TLA2TOOLS_JAR` to use another copy. A run takes about 30 seconds on 4 cores. `TLC_WORKERS=2` limits the cores it uses.

To check one property alone, ignoring the known findings list:

```bash
.agents/skills/verify-connection/scripts/check-connection --only OfflineIdleAndCleared
```

Output and counterexamples go to `.agents/skills/verify-connection/out/`.

Read every TLC result with the pstack `/statechart` skill. It shows each invariant violation as its shortest path in plain words, and when the check passes it draws how the system moves between states.

## What it checks

Each property is an invariant. `OfflineIdleAndCleared` is checked whenever the main thread has no updates left to apply. `DaemonReconnectResyncs` is checked once nothing is in flight and the watcher has caught up with whether tailscaled is reachable. The others are checked in every reachable state, or once a toggle or login attempt has finished, as noted.

| Property | Plain meaning | Harm when broken |
|---|---|---|
| `OfflineIdleAndCleared` | When the UI is offline and the main thread has caught up, engine and waiting state are cleared, stale engine updates that arrived while offline were ignored, and the offline page's login group is shown only in `NeedsLogin`. | Reconnecting shows leftover peer stats or a waiting-file list from the previous session, or hides the Sign in control. |
| `NeedsAuthPromptsLogin` | Toggling connect when `Latest` is `NeedsLogin` asks for auth rather than calling `SetRunning(true)`. | The switch pretends to connect, then nothing happens, or `tailscale up` runs with no way to log in. |
| `LoginSeesBrowseURL` | Starting `App.login` reliably observes the `BrowseToURL` that tailscaled published for that attempt. | The login URL is sitting on the bus, the 10s wait times out, and the user sees "Failed to start login" instead of a browser. |
| `SetRunningHoldReleases` | A `SetRunning` hold always releases, and after a failed call has settled the status switch matches the shown state. | The UI stays on a suppressed in-between status, or the switch stays flipped after a failed start or stop. |
| `DaemonReconnectResyncs` | When tailscaled drops and comes back, `Bus` reconnects and `App.online` converges to the daemon's real state. | The window keeps showing "connected" after tailscaled is gone, or stays offline after it is back. |
| `NoDeadlock` | A finished `SetRunning` call never leaves publishing held. | Status updates stop reaching the UI. |

## Read the result

- `PASS` lists every property that held in every reachable state.
- `FAIL <property>` means a new break. Open the `.out` file it names. Its `State N:` lines are the steps, and the variables under each step show what changed.
- `KNOWN FINDING still present` is expected for the finding below. The check exits 0 when only that fails.
- `FIXED? <property> now passes` means the model no longer finds that break. Confirm the fix in the code with a test, since a code fact can flip without the bug being gone. Then remove the entry from `KNOWN_FINDINGS` in the script and from Findings below. The check exits 1 until you do. A broken guard can also remove the finding's path, so a `FIXED?` next to a `FAIL` means the `FAIL` comes first.
- `TOOL ERROR` means TLC did not finish. Read the `.out` file. It is usually a model syntax or type error.

The script prints `code facts` first. Each is a model constant read from the code by a regex in `scripts/check-connection`. The last column is the first property TLC reports with that fact alone FALSE and the known findings' causes removed.

| Constant | TRUE when the code has | Fails without it |
|---|---|---|
| `OfflineClearsEngine` | `showIPN` sets `a.engine = nil` when `!online` | `OfflineIdleAndCleared` |
| `OfflineClearsWaiting` | `showIPN` sets `a.waiting = nil` when `!online` | `OfflineIdleAndCleared` |
| `OfflineIgnoresEngine` | the `EngineStatus` case of `App.update` returns if `!a.online` | `OfflineIdleAndCleared` |
| `WindowClearsEngine` | `MainWindow.Update` sets `win.engine = nil` when the status is not online | `OfflineIdleAndCleared` |
| `NeedsAuthPrompts` | `setRunning` offers a login confirmation instead of `SetRunning` when `NeedsAuth()`, and `NeedsAuth` is `State == NeedsLogin` | `NeedsAuthPromptsLogin` |
| `LoginWaitsAfterStart` | `login` calls `StartLogin`, then `a.bus.Next`, or `WaitBrowseURL` runs `fn` before taking a cursor. FALSE is the subscribe-first order in `WaitBrowseURL` | none: `LoginSeesBrowseURL` holds when this is FALSE |
| `SetRunningHolds` | `SetRunning` sends `holds <- 1` before `run` and `holds <- -1` after, and `Run` publishes only while `holds == 0` | none in these bounds: publishing the steps on the way still settles on the same state |
| `SetRunningResendsOnFail` | `SetRunning` queues `resend` when `run` returns an error | `SetRunningHoldReleases`: the switch stays flipped after a failed start that published nothing |
| `WatchSendsNoState` | `watch` sends a synthetic `NoState` when the bus connection drops | `DaemonReconnectResyncs` |
| `FilesGatedOnline` | `Files` only pushes a list when `Bus.latest` is online | `OfflineIdleAndCleared` |

A regex checks the shape of one guard. It does not see a new code path that changes what the guard means. Model such a path as a new action before trusting a pass.

The `AtomicUI` constant is not a code fact. The check sets it FALSE, so each `IdleAdd` is its own step. Set it TRUE only when drawing a chart of the same model: the window then updates in the same step as the publish, which is what the user sees, and the state space shrinks.

When a check fails, compare the facts with a run on `master`:

1. A fact flipped, and the change meant to remove or move that guard. If the guard is gone, the failure is real. Report the counterexample. If the guard moved or was renamed, update its regex and rerun.
2. A fact flipped by accident, for example after a reformat. Fix the regex so it matches the same guard.
3. No fact changed, but a property fails. The change altered behavior that the model describes as fixed. Find the action in the table below, compare it with the new code, and update the model to match the code. Never loosen a property to make it pass.

## Where the model comes from

Each LocalAPI or CLI call is one step, because tailscaled serves it under `b.mu`. Each `gutil.IdleAdd` callback is one step, and they run in the order they were added, so the model keeps them in one queue, `mainQ`. Comments in the model give the file and line of each step: `bus`, `app`, `win`, `off`, `tray`, `poll`, `tc`, `files`, and `gutil` are `internal/tsutil/bus.go`, `internal/ui/app.go`, `mainwindow.go`, `offlinepage.go`, `internal/tray/tray.go`, `internal/tsutil/poller.go`, `client.go`, `files.go`, and `internal/gutil/data.go`. `ll` and `lapi` are tailscale.com's `ipn/ipnlocal/local.go` and `client/local/local.go` at `MODELED_TAILSCALE` (find the module with `go list -m -f '{{.Dir}}' tailscale.com`).

| Model action | Code |
|---|---|
| `UserToggles` | The window's status switch (`setStatus` → `setRunning`) or the tray's `OnConnToggle` (`toggleConn` → `setRunning`). `setRunning` reads `Latest`. If that is `NeedsLogin`, it offers a confirmation and does not call `SetRunning`. The switch is turned back when it didn't try (`!sent`). |
| `SetRunningFinishes` | `Bus.SetRunning` running `tailscale up` or `down`, then the unbuffered `holds <- -1`, which `Run` applies before a resend is queued. A successful call settles on `Running` or `Stopped`; `Starting` notifies during `up` are overwritten in `pending`. |
| `SetRunningQueuesResend` | After the hold is released, a failed call queues `resend` so `New` is passed the last published status again. |
| `BusPublishes` | `Bus.publish`: it swaps `b.latest`, its `New` does `IdleAdd(a.update)`, and it fires `changePublished`. It waits while `SetRunning` holds publishing. Engine updates are not held. |
| `BusResends` | The `resend` arm of `Run`: `New(latest)` again, without `changePublished`. |
| `AppUpdates` | One `IdleAdd` callback: `showIPN`, an ignored or kept engine update, or a waiting-file list. |
| `LoginStarts`, `UserConfirmsLogin` | `App.login`, including from the confirmation's Accept. It calls `Bus.WaitBrowseURL`, which takes a cursor and then `StartLogin`. `StartLoginInteractive` either pops an existing auth URL under `b.mu` before the RPC returns, or asks control for one. |
| `LoginSubscribes` | `a.bus.Next` after `StartLogin` returns, only when `LoginWaitsAfterStart`. The current code subscribes first, so this action is disabled. |
| `LoginSeesStatus` | `Next` returning the latest published status; a non-empty `BrowseToURL` opens it. |
| `LoginTimesOut` | The 10s context on `login` running out while nothing still in flight would unblock `Next`. |
| `DaemonOffersURL` | Control sending an auth URL after `cc.Login`. |
| `DaemonDrops`, `WatchReportsDown` | `watchOnce` failing, then `watch` sending a synthetic `NoState` with empty prefs, so a profile `switchHold` does not hold it. |
| `DaemonReturns`, `WatchReconnects` | tailscaled is reachable again, `watchOnce` remakes the connection, and the initial state notify is published. |
| `EngineArrives` | `NewEngine` → `IdleAdd`, not held. |
| `FileArrives` | `Files.push`, only while `Bus.latest` is online. |

The check uses `MaxToggles = 2`, `MaxLogins = 2`, `MaxDrops = 1`, `MaxEngine = 1`, and `MaxFiles = 1`, and at most one failed `SetRunning`. A drop is modeled only while `SetRunning` and `login` are idle, so the reconnect invariant is checked without mixing it into an in-flight CLI call. `login` requires a live bus; if tailscaled is already down, `StartLogin` fails at the LocalAPI and is not the `BrowseToURL` race. Profile switches, the operator dialog, `NeedsMachineAuth`, and wall-clock delays other than the login timeout and the 5s reconnect wait (which is one step) are left out.

## Coverage

Sync commit: `51b50aa`

Move the sync commit forward only after you confirm, at the new commit, that the model and the code facts still describe the code below. The script lists covered files changed since the sync commit. The repo squash merges, so after a PR that moved the sync commit merges, set it to the merge commit on `master`.

- `internal/tsutil/bus.go`: `Bus.Run`, `watch`, `watchOnce`, `publish`, `Next`, `WaitBrowseURL`, `cursor`, `Latest`, `SetRunning`, `fire`
- `internal/tsutil/poller.go`: `IPNStatus.Online`, `IPNStatus.NeedsAuth`, `IPNStatus.copy`
- `internal/tsutil/client.go`: `Start`, `Stop`, `StartLogin`
- `internal/tsutil/files.go`: `filesRun.online`, `filesRun.push`
- `internal/ui/app.go`: `App.update`, `showIPN`, `setRunning`, `toggleConn`, `login`, `App.Run` (`New` and `NewEngine` via `IdleAdd`)
- `internal/ui/mainwindow.go`: `setStatus`, `MainWindow.Update`, `updateStatusSwitch`
- `internal/ui/offlinepage.go`: `OfflinePage.Update`, `NeedsAuthGroup`
- `internal/tray/tray.go`: `OnConnToggle`
- `go.mod`: the `tailscale.com` version
