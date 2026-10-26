---
name: verify-exit-node-selection
description: Model-check that Trayscale's Exit Node preference (Previous or Recommended) is what takes effect when exit node use is turned on from the menu or tray, using a TLA+ model of Trayscale's calls and tailscaled's exit node prefs whose guards are read from the code. Use when a change touches SetUseExitNode, DisableExitNode, ExitNode, the keep, pin, or suggest helpers in internal/tsutil/client.go, the use_exit_node action or tray exit toggle, the exit-node-selection setting, the exit node switches on the peer, Mullvad, or self page, or the tailscale.com version, or when running /verify-exit-node-selection.
---

# Verify exit node selection

The model in `model/ExitNodeSelection.tla` checks that turning exit node use on from Trayscale uses the selection the Exit Node preference names, gets back the last node under Previous, and either turns an exit node on or reports that it failed. TLC, the TLA+ model checker, tries every order of the steps below within the bounds, including netmaps that change the recommended node between two LocalAPI calls. Tests only cover the orders someone wrote down.

## Run it

From the repo root:

```bash
.agents/skills/verify-exit-node-selection/scripts/check-exit-node-selection
```

It needs Java 11 or newer, `perl`, and `curl`. Without a system Java, `mise exec java@temurin-21 -- <command>` works. The first run downloads `tla2tools.jar` 1.8.0 to `~/.cache/trayscale/` and checks its sha256. Set `TLA2TOOLS_JAR` to use another copy. A run takes about 5 seconds. `TLC_WORKERS=2` limits the cores it uses.

To check one property alone, ignoring the known findings list:

```bash
.agents/skills/verify-exit-node-selection/scripts/check-exit-node-selection --only TurningOnTurnsOn
```

Output and counterexamples go to `.agents/skills/verify-exit-node-selection/out/`. CI runs the same command, then `scripts/run-harness-tests`, in the `exit node selection` job.

## What it checks

Each property is an invariant. It is checked once the last toggle has finished and nothing but that toggle has changed exit node prefs since.

| Property | Plain meaning | Harm when broken |
|---|---|---|
| `SelectionTakesEffect` | The selection type chosen in the setting is always the one that takes effect: Recommended leaves Tailscale's automatic selection on, Previous leaves it off. | The preference does nothing, or does the opposite. |
| `PreviousIsLastUsed` | Previous gets back the node this device last used as its exit node, even if Recommended picked it. | Traffic goes out through a node the user didn't choose. |
| `TurningOnTurnsOn` | A turn on that reports success leaves an exit node in use. | The tray and menu say the exit node is on while traffic goes out directly. |
| `FailedTurnOnLeavesItOff` | A turn on that reports failure leaves exit node use off, with automatic selection off. | Trayscale shows an error but leaves Recommended running. |

## Read the result

- `PASS` lists every property that held in every reachable state.
- `FAIL <property>` means a new break. Open the `.out` file it names. Its `State N:` lines are the steps, and the variables under each step show what changed.
- `KNOWN FINDING still present` is expected for the finding below. The check exits 0 when only that fails.
- `FIXED? <property> now passes` means the model no longer finds that break. Confirm the fix in the code with a test, since a code fact can flip without the bug being gone. Then remove the entry from `KNOWN_FINDINGS` in the script and from Findings below. The check exits 1 until you do. A broken guard can also remove the finding's path, for example when Recommended never turns on, so a `FIXED?` next to a `FAIL` means the `FAIL` comes first.
- `TOOL ERROR` means TLC did not finish. Read the `.out` file. It is usually a model syntax or type error.
- A `warning: go.mod has tailscale.com ...` line means the daemon side of the model may be stale. See Tailscale's side below.

Read every result with the `/statechart` skill. It turns a violation into its shortest path in plain words and shows which violations a change fixes. On a pass it draws how exit node use moves between states.

## Real bug or modeling error

The script prints `code facts` first. Each is a model constant read from the code by regexes in `scripts/check-exit-node-selection`:

| Constant | TRUE when the code has | Property that fails without it |
|---|---|---|
| `SettingReadBack` | `settings.go` writing `sel.String()` to `exit-node-selection` and `exitNodeSelection()` reading it with `ParseExitNodeSelection`, both mapping `"recommended"`, `SelectPreviousExitNode` before `SelectRecommendedExitNode`, and the combo row's items in that order in `preferences.ui` | `SelectionTakesEffect` |
| `MenuUsesSetting` | the `use_exit_node` action calling `tsutil.SetUseExitNode(ctx, s, a.exitNodeSelection())` | `SelectionTakesEffect` |
| `TrayUsesSetting` | `OnExitToggle` reading `sel := a.exitNodeSelection()` and passing it to `SetUseExitNode` | `SelectionTakesEffect` |
| `RecommendedUsesAuto` | `SetUseExitNode` calling `useAutoExitNode` for `SelectRecommendedExitNode` | `SelectionTakesEffect` |
| `PreviousPinsRestoredAuto` | `SetUseExitNode` ending the Previous path with `keepPreviousExitNode` | `SelectionTakesEffect` |
| `OffPinsAutoNode` | `SetUseExitNode` turning off through `DisableExitNode`, which calls `pinAutoExitNode` before `localClient.SetUseExitNode(ctx, false)` | `PreviousIsLastUsed` |
| `SuggestRejectsEmpty` | `suggestExitNode` returning `errNoRecommendedExitNode` for an empty suggestion ID, and both `useSuggestedExitNode` and `keepPreviousExitNode` using it | `TurningOnTurnsOn` |
| `KeepUndoesFailedPin` | `keepPreviousExitNode` calling `localClient.SetUseExitNode(ctx, false)` when it has no node to pin | `FailedTurnOnLeavesItOff` |
| `PeerPageOffKeepsPrior` | the peer page's `Use as exit node` switch turning off with `tsutil.DisableExitNode` | `PreviousIsLastUsed` |
| `MullvadPageOffKeepsPrior` | a Mullvad node's switch turning off with `tsutil.DisableExitNode` | `PreviousIsLastUsed` |
| `AdvertiseKeepsPrior` | turning `Advertise exit node` on calling `tsutil.DisableExitNode` first | `PreviousIsLastUsed` |

`KeepUndoesFailedPin` matters only while `SuggestRejectsEmpty` is TRUE. Without `SuggestRejectsEmpty`, `keepPreviousExitNode` pins an empty ID instead of failing, and `TurningOnTurnsOn` fails first.

When a check fails, compare the facts with a run on `master`:

1. A fact flipped, and the change meant to remove or move that guard. If the guard is gone, the failure is real. Report the counterexample. If the guard moved or was renamed, update its regex and rerun.
2. A fact flipped by accident, for example after a reformat. Fix the regex so it matches the same guard.
3. No fact changed, but the property fails. The change altered behavior that the model describes as fixed. Find the action in the table below, compare it with the new code, and update the model to match the code. Never loosen a property to make it pass.

A new caller of `tsutil.ExitNode` with an empty ID, or a new path that turns exit node use on, has no fact. Model it as a new action before trusting a pass.

## Where the model comes from

Each LocalAPI call is one step, because tailscaled serves it under its backend lock. Comments in the model give the file and line of each step. `tc:` lines are in `internal/tsutil/client.go`. `ll:` and `api:` lines are in tailscale.com's `ipn/ipnlocal/local.go` and `ipn/localapi/localapi.go` at `MODELED_TAILSCALE` (find the module with `go list -m -f '{{.Dir}}' tailscale.com`).

| Model action | Code |
|---|---|
| `Click` | the `use_exit_node` action (`app.go`) or the tray's `OnExitToggle`, asking for the opposite of `ExitNodeActive` |
| `ChangeSetting` | the Preferences `Exit Node` combo row |
| `UseAutoExitNode` | `useAutoExitNode`, then tailscaled resolving automatic selection |
| `EnableExitNode` | `localClient.SetUseExitNode(ctx, true)`: tailscaled's `SetUseExitNodeEnabled` restoring `InternalExitNodePrior`, or failing when it is empty |
| `FallbackSuggest`, `FallbackPin` | `useSuggestedExitNode`: `suggestExitNode`, then `ExitNode` |
| `KeepReadPrefs`, `KeepSuggest`, `KeepPin`, `KeepUndo` | `keepPreviousExitNode` |
| `OffReadPrefs`, `OffPin`, `OffDisable` | `DisableExitNode`: `pinAutoExitNode`, then `localClient.SetUseExitNode(ctx, false)` |
| `PickOnPage` | a peer or Mullvad page switch turned on: `ExitNode(node)` |
| `OffOnPage` | a peer or Mullvad page switch turned off, or `Advertise exit node` turned on |
| `NewSuggestion` | a netmap changing the recommended node, which also moves automatic selection |

Tailscale's side, at `MODELED_TAILSCALE`:

- `SetUseExitNodeEnabled(false)` remembers `auto:any`, not the node, while automatic selection is on. That is why `pinAutoExitNode` exists.
- `SetUseExitNodeEnabled(true)` with nothing remembered is an error, and Trayscale falls back to the recommended node.
- `suggest-exit-node` answers an empty ID, not an error, when there is nothing to recommend.
- An edit that sets `ExitNodeID` to empty also zeroes `InternalExitNodePrior`. That is why the pages use `DisableExitNode` instead of `ExitNode("")`.
- An edit that turns automatic selection off while `ExitNodeID` is still `auto:any` also empties `ExitNodeID`.

When the warning says go.mod moved past `MODELED_TAILSCALE`, read each `ll:` and `api:` line in the model at the new version. Update the model where tailscaled's behavior changed, and the line numbers where only they moved. Then set `MODELED_TAILSCALE` and rerun.

Two ghost variables exist only to state properties. `lastUsed` is the last node this device used as its exit node. `otherSince` records that something other than a toggle changed exit node prefs since the last toggle.

## Bounds and assumptions

- Two exit nodes. Tailscale's recommendation starts empty and changes at most 3 times, always to a node.
- At most 4 toggles, 3 page actions, and 2 setting changes.
- `OffWhileWaiting` is a bound, not a code fact. With it FALSE, nothing turns exit node use off while Recommended has no node yet, and no netmap lands in the middle of a toggle that is turning it off with automatic selection still on. The main run has it TRUE. The known finding below needs it, so `PreviousIsLastUsed` is checked with it FALSE and expected to fail with it TRUE.
- A page action is one step. Its pin and disable run on the main thread with automatic selection already off in between, so splitting them changes no outcome the properties see.
- No LocalAPI call fails or times out. `ExitNodeIP` is never set.

## Not covered

- Two toggles at once, for example the tray and the menu.
- Other Tailscale clients, the CLI, and admin exit node policy. Turning exit node use off elsewhere while Recommended is on makes Tailscale remember Recommended, and Previous can't undo that.
- Profile switches, logins, tailscaled restarts, and a node leaving the netmap.
- What the UI shows about the exit node. The model stops at tailscaled's prefs.
- Picking a node on a page while Recommended has no node. tailscaled drops the pick and turns exit node use off. The model includes that step, but no property covers it.

## Regression tests

`scripts/run-harness-tests` starts two tsharness daemons from the `drive-trayscale` skill, one with `-suggest-exit-nodes` and one without, and runs the `TestHarness` tests in `internal/tsutil/exitnode_harness_test.go` against them. They need `go` and take about 20 seconds. Without the script they skip.

## Findings

`PreviousIsLastUsed` fails when exit node use is turned off from Trayscale while Recommended has no node yet. Shortest path:

1. With Recommended selected, turn exit node use on. Tailscale has nothing to recommend, so it waits on `auto:any`.
2. Turn exit node use off. `pinAutoExitNode` reads `auto:any` and has no node to pin.
3. A netmap arrives, and Tailscale starts using its recommended node X.
4. `SetUseExitNodeEnabled(false)` runs and remembers `auto:any`, not X.
5. Switch the preference to Previous and turn exit node use on. Tailscale turns Recommended back on.
6. Another netmap moves the recommendation to Y, and `keepPreviousExitNode` pins Y instead of X.

Turning off while waiting through the self page's `Advertise exit node` switch has the same effect. Fixing it needs Trayscale to record the last node it saw in use instead of relying on Tailscale's memory.

## Coverage

Sync commit: `fe68ed678f14b29df73d1ea98935e2e3972ef263`

Move the sync commit forward only after you confirm, at the new commit, that the model and the code facts still describe the code below. The script lists covered files changed since the sync commit. The repo squash merges, so after a PR that moved the sync commit merges, set it to the merge commit on `master`.

- `internal/tsutil/client.go`: `ExitNode`, `ParseExitNodeSelection`, `ExitNodeSelection.String`, `SetUseExitNode`, `DisableExitNode`, `pinAutoExitNode`, `useAutoExitNode`, `suggestExitNode`, `useSuggestedExitNode`, `keepPreviousExitNode`
- `internal/tsutil/poller.go`: `IPNStatus.ExitNodeActive`
- `internal/ui/app.go`: the `use_exit_node` action and `OnExitToggle`
- `internal/ui/settings.go`: the `ExitNodeSelectionRow` handler and `exitNodeSelection`
- `internal/ui/preferences.ui`: `ExitNodeSelectionRow`
- `internal/ui/peerpage.go`: the `Use as exit node` switch
- `internal/ui/mullvadpage.go`: each node's switch
- `internal/ui/selfpage.go`: the `Advertise exit node` switch
- `dev.deedles.Trayscale.gschema.xml`: the `exit-node-selection` key
- `go.mod`: the `tailscale.com` version
