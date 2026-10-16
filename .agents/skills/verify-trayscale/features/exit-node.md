# Exit node

The `Exit Node` preference picks how Trayscale chooses an exit node when the user turns exit node use on from Trayscale's menu: `Previous exit node` (the default) or `Recommended exit node`. The self page shows the exit node in use, and its row opens that node's page.

## Sub-features

- `exit-node-preference` shows the `Exit Node` combo row in the Preferences `General` group with `Previous exit node` selected by default.
- `exit-node-previous` turns exit node use on from the menu and gets the last node back, with automatic selection off.
- `exit-node-recommended` turns exit node use on from the menu and lets Tailscale pick a node automatically.
- `exit-node-row` shows the self page `Exit node` row: `None`, the peer name, the Mullvad location, or a bare IP.
- `exit-node-open-peer` opens the exit node's peer page from that row.
- `exit-node-open-mullvad` opens the Mullvad page with the node's country expanded and its row in view.

## How to get to it (user POV)

- Open Preferences from the main menu and use `Exit Node` in `General`.
- Turn exit node use on or off from the main menu or the tray menu.
- Open the self page under `This machine`. `Exit node` is the first row in `Options`.

## Driving it with control-trayscale

Preconditions:

- `doctor` reports `frame 'Trayscale'` and `gsettings_backend` equal to `memory`.
- Before every step that changes Tailscale state, `check-fake-tailnet` must report `FAKE`. This recipe runs `use_exit_node` and `Use as exit node` only against tsharness. Never run these steps against the user's real tailnet.
- The test node is connected and does not advertise itself as an exit node.
- Run tsharness with `-suggest-exit-nodes` so Recommended can resolve a node. Add `-mullvad N` for the Mullvad checks.

- **Preference.** Run `action preferences`, then snapshot `prefs.tree.txt`. `grouping 'General'` contains `Exit Node` with `Previous exit node` selected.
- **Row, none in use.** On the self page, snapshot `none.tree.txt`. The `Exit node` row says `None` and has no activate action.
- **Previous.** Use a peer page's `Use as exit node` switch to select `bravo-exit`, then run `action use_exit_node --i-mean-it` twice (off, then on). Check `tailscale debug prefs` against the harness socket: `ExitNodeID` names `bravo-exit` and `AutoExitNode` is empty.
- **Recommended.** Select `Recommended exit node` in the preference, turn exit node use off and on with `use_exit_node`, and check that `AutoExitNode` is `any`. The `Exit node` row names whatever node Tailscale picked.
- **Previous after Recommended.** Switch the preference back to `Previous exit node`, then toggle `use_exit_node` off and on. `AutoExitNode` is empty and `ExitNodeID` is the node automatic mode had chosen.
- **Open peer.** With `bravo-exit` in use, activate the `Exit node` row and snapshot `peer.tree.txt`. The visible page is `bravo-exit`'s peer page.
- **Open Mullvad.** Select a Mullvad node with `$TS set --exit-node=<ip>`, activate the `Exit node` row, wait at least 300 ms, and snapshot `mullvad.tree.txt`. The Mullvad page is visible, search is empty, the node's country is expanded, and the node's row has focus.
- **Proof.** Keep every snapshot above with the `tailscale debug prefs` output next to each state change.

## Gotchas

- The preference applies only when Trayscale turns exit node use on. `tailscale set` and other clients ignore it.
- If exit node use is turned off outside Trayscale while automatic mode is on, Tailscale remembers automatic mode instead of a node, and Previous can restore only what Tailscale remembered. Turn it off from Trayscale when testing Previous.
- Recommended can pick a different node on each run. Assert `AutoExitNode` and that the row names a node, not a specific server.
- Sharee nodes and nodes known only by IP have no page, so their row is not activatable.
- The Mullvad page expands the country first and focuses the node row after the expand animation. A snapshot taken right away may show only the country expanded.
- A private D-Bus session isolates the app, not the Tailscale socket. Use a separate test machine or a mount namespace exposing only the harness socket at Trayscale's default path.
