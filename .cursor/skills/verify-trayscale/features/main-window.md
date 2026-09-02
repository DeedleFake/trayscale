# Main window

The main window is titled Trayscale. When Tailscale is connected it shows a sidebar of this machine, exit nodes, online peers, and offline peers. When Tailscale is not connected it shows a Not Connected page instead of that sidebar.

## Sub-features

- `window-title` shows a frame named `Trayscale`.
- `sidebar-sections` shows `This machine`, `Exit Nodes`, `Online`, and `Offline` while connected.
- `offline-chrome` shows `Not Connected` when Tailscale is down.
- `profile-private` shows `profile@example.com` in the profile dropdown under `TRAYSCALE_PRIVATE=1`.

## How to get to it (user POV)

- Start Trayscale without `--hide-window`.
- Choose Show on the tray menu (not available on the isolated bus).
- Activate the already-running application so the window presents.

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'` and `gsettings_backend` equal to `memory`.
- For `sidebar-sections` and `profile-private`, `tailscaled` is running and the node is connected.
- For `offline-chrome`, Tailscale is not connected. Do not toggle the header switch to force that state.

- **Window.** The frame exists after launch. Run `control-trayscale find --role window --name Trayscale --exact`. Output is `frame 'Trayscale'`.
- **Connected sidebar.** Capture the tree. Run `control-trayscale snapshot --path .cursor/skills/verify-trayscale/artifacts/main-window/connected.tree.txt`. The file contains `label 'This machine'`, `label 'Exit Nodes'`, `label 'Online'`, and `label 'Offline'`.
- **Private profile.** In that same snapshot, `combo box 'profile@example.com'` is present.
- **Offline chrome.** Only when Tailscale is already down: the snapshot contains `Not Connected` and does not contain `label 'This machine'`.
- **Proof.** Keep the snapshot. The frame name `Trayscale` and the section labels (or `Not Connected`) are the end state.

## Gotchas

- The unnamed header `switch` with `states=checked` is the Tailscale connect control. Do not click it.
- Isolated launch has no tray. Do not treat a missing tray icon as a product failure.
- A user-owned `trayscale --hide-window` can sit on the session bus at the same time. Doctor the isolated run, not that process.
- Offline peers disappear from the sidebar if `show-offline-peers` is off. Launch uses in-memory defaults, so the key is on.
