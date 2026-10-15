# Automatic exit node

The this-machine page lets Tailscale choose the exit node automatically and shows the current node. Turning automatic selection off keeps the current node selected. Choosing a node manually cancels automatic selection.

## Sub-features

- `automatic-enable` turns on automatic selection by Tailscale.
- `automatic-current` shows the selected node, or `Waiting for an exit node` while none is selected.
- `automatic-disable` stops automatic selection and keeps the current node.
- `automatic-manual` switches to a manually chosen node.
- `automatic-off` disables exit-node use from the existing app action.

## How to get to it (user POV)

- Open the self page under `This machine` and use `Automatic exit node` in `Options`.
- Select an exit node's page and turn on `Use as exit node` for a manual choice.
- Use the main menu's exit-node toggle to stop using an exit node.

## Driving it with control-trayscale

Preconditions:

- `doctor` reports `frame 'Trayscale'` and `gsettings_backend` equal to `memory`.
- Before every preference change, `check-fake-tailnet` must report `FAKE`. This recipe permits `Automatic exit node`, `Use as exit node`, and `use_exit_node` only against tsharness. Never use these steps against the user's real tailnet.
- The test node is connected and does not advertise itself as an exit node. Start on its self page.
- Run tsharness with `-suggest-exit-nodes` for a resolved automatic choice, or without it to exercise waiting for a recommendation.

- **Enable.** Save `before.tree.txt`, then `click --role switch --name "Automatic exit node" --exact --index 1`. Save `enabled.tree.txt`; the switch is checked and `Current exit node` names the chosen node or says `Waiting for an exit node`. Check `tailscale debug prefs` against the harness socket: `AutoExitNode` is `any`.
- **Disable.** Click the same switch and save `manual-current.tree.txt`. `AutoExitNode` is empty and the selected `ExitNodeID` remains unchanged when a node was selected.
- **Manual choice.** Enable Automatic again. Search for a different exit node (for example `echo-down`) and open its page, then `click --role switch --name "Use as exit node" --exact --index 1`. Check that `AutoExitNode` is empty and `ExitNodeID` names that peer. Restart the isolated app to return to the self page and capture the unchecked Automatic switch and current node.
- **Off.** While an exit node is in use, run `action use_exit_node --i-mean-it` and capture `off.tree.txt`. Both `AutoExitNode` and `ExitNodeID` are empty and `Current exit node` says `None`.
- **Persistence.** Enable Automatic, clean up the isolated app, and launch it again. Its Automatic switch remains checked because the setting belongs to Tailscale.
- **Resume.** With Automatic enabled, toggle `use_exit_node` off and on. Tailscale restores automatic selection; `AutoExitNode` is `any` again.

## Gotchas

- A selected server may change while Automatic is enabled. Assert the mode and displayed current node, rather than a fixed server name.
- The switch is unavailable while this machine advertises itself as an exit node.
- Turning Automatic off preserves a resolved node. If no node has been selected yet, there is none to preserve.
- A private D-Bus session isolates the app, not the Tailscale socket. Use a separate test machine or a mount namespace exposing only the harness socket at Trayscale's default path.
- AT-SPI exposes the row and its child switch with the same name. Index 1 is the child with the `toggle` action.
- A toggle action starts an animation. Wait for the expected daemon preferences and checked state before navigating away; the AT-SPI action returning is not confirmation that the setting changed.
