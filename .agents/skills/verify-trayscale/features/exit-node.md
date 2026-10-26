# Exit node

The `Exit Node` preference picks how Trayscale chooses an exit node when the user turns exit node use on from Trayscale's menu: `Previous exit node` (the default) or `Recommended exit node`. The self page shows the exit node in use, and its row opens that node's page.

## Sub-features

- `exit-node-preference` shows the `Exit Node` combo row in the Preferences `General` group with `Previous exit node` selected by default.
- `exit-node-previous` turns exit node use on from the menu and gets the last node back, with automatic selection off.
- `exit-node-recommended` turns exit node use on from the menu and lets Tailscale pick a node automatically.
- `exit-node-row` shows the self page `Exit node` row: `None`, the peer name, the Mullvad location, a bare IP, `Waiting for an exit node` while Recommended has not picked a node yet, or `Unavailable` when a manually chosen node has left the netmap.
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
- The `Exit Node` combo box and the `Exit node` row expose no AT-SPI action. Changing the preference and activating the row need a pointer click (for example `xdotool` on X11). Without one, run only the steps that use actions and the CLI.
- `tailscale debug prefs` omits `AutoExitNode` when it is empty. Treat a missing key as empty.

- **Preference.** Run `action preferences`, then snapshot `prefs.tree.txt`. `grouping 'General'` contains `combo box 'Exit Node'`, and its `list item ''` holds `label 'Previous exit node'`.
- **Row, none in use.** On the self page, snapshot `none.tree.txt`. `list item 'Exit node'` has `label 'None'` and no `image ''` child.
- **Previous.** Use a peer page's `Use as exit node` switch to select `bravo-exit`, then run `action use_exit_node --i-mean-it` twice (off, then on). Check `tailscale debug prefs` against the harness socket: `ExitNodeID` names `bravo-exit` and `AutoExitNode` is empty.
- **Recommended.** Click `combo box 'Exit Node'` and choose `Recommended exit node` from the popover, turn exit node use off and on with `use_exit_node`, and check that `AutoExitNode` is `any`. The `Exit node` row names whatever node Tailscale picked. Without `-suggest-exit-nodes`, `ExitNodeID` stays `auto:any` and the row says `Waiting for an exit node`.
- **Previous after a page switch.** Select `bravo-exit` on its peer page, then turn the page's `Use as exit node` switch off. Run `action use_exit_node --i-mean-it` once. `ExitNodeID` names `bravo-exit` again. Against a harness without `-suggest-exit-nodes`, a turn on with nothing to restore shows the `Failed to toggle exit node` toast and leaves `ExitNodeID` empty.
- **Previous after Recommended.** Switch the preference back to `Previous exit node`, then toggle `use_exit_node` off and on. `AutoExitNode` is empty and `ExitNodeID` is the node automatic mode had chosen.
- **Node leaves the netmap.** Run tsharness with `-remove-exit-after 60s` and select `bravo-exit` with `$TS set --exit-node=bravo-exit` before the timer fires. After the harness logs `removed bravo-exit from netmap`, `ExitNodeID` still names it and `use_exit_node` is still on. The row says `Unavailable` and has no `image ''` child. With `-suggest-exit-nodes` and `$TS set --exit-node=auto:any` instead, the row says `Waiting for an exit node`. Restart tsharness afterwards, because testcontrol stops updating the netmap.
- **Open peer.** With `bravo-exit` in use, the row has `label 'bravo-exit'` and an `image ''` child. Click the row and snapshot `peer.tree.txt`. The visible page is `bravo-exit`'s peer page, with `switch 'Use as exit node' states=checked`.
- **Open Mullvad.** Select a Mullvad node with `$TS set --exit-node=<ip>`, activate the `Exit node` row, wait at least 300 ms, and snapshot `mullvad.tree.txt`. The Mullvad page is visible (its sidebar label becomes the location, for example `🇸🇪 Stockholm, Sweden`), the search `entry ''` is empty, the country is `list item '<flag> <country>' states=expanded`, and the node is `switch '<city>' states=checked,focused`.
- **Proof.** Keep every snapshot above with the `tailscale debug prefs` output next to each state change.

## Gotchas

- The preference applies only when Trayscale turns exit node use on. `tailscale set` and other clients ignore it.
- If exit node use is turned off outside Trayscale while automatic mode is on, Tailscale remembers automatic mode instead of a node, and Previous can restore only what Tailscale remembered. Turn it off from Trayscale when testing Previous.
- To get back to the self page, click `This machine`'s sidebar row. `select` marks it selected but does not change the page.
- Recommended can pick a different node on each run. Assert `AutoExitNode` and that the row names a node, not a specific server.
- Sharee nodes and nodes known only by IP have no page, so their row is not activatable.
- The Mullvad page expands the country first and focuses the node row after the expand animation. A snapshot taken right away may show only the country expanded.
- A private D-Bus session isolates the app, not the Tailscale socket. Use a separate test machine or a mount namespace exposing only the harness socket at Trayscale's default path.
