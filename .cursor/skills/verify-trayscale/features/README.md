# Trayscale verification map

This directory is the maintained source for verifying the user-facing behavior of Trayscale. Read the index before driving the app, then use the matching feature file as the recipe.

## Baseline preconditions

- Launch with `.cursor/skills/verify-trayscale/scripts/control-trayscale launch` from the repo root.
- Run `control-trayscale doctor` and require `frame 'Trayscale'`, `bus_name_owned` true, and `gsettings_backend` equal to `memory`.
- Never drive a Trayscale that this run did not start. The user may already have `trayscale --hide-window` on the session bus.
- `tailscaled` is shared. Stay on read-only paths unless a feature file names a mutating action.
- Put proof under `.cursor/skills/verify-trayscale/artifacts/<feature>/`.

When Tailscale is connected, the sidebar shows `This machine`, `Exit Nodes`, `Online`, and `Offline` (offline peers depend on `show-offline-peers`, default on). When it is not connected, the content page is `Not Connected` and those sections are absent. Feature files say which of those states they need.

## Driving conventions

- Start every recipe from the baseline window unless its preconditions say otherwise.
- Prefer AT-SPI names and GApplication actions over coordinates.
- Treat every command as literal. Keep quoted names and flags unchanged.
- Run UI actions through `control-trayscale`.
- Do not click the unnamed header `switch`. That is connect/disconnect.

## Proof and skip reporting

- Capture the user action and the resulting state, not only the final tree.
- UI proof is an AT-SPI snapshot that names Trayscale and the end state. Add a screenshot when the compositor allows it.
- Record the feature ID and entry point used with every artifact.
- Report an unreachable path with the attempted command and the unmet precondition.
- Do not report a skipped entry point as verified through a different path.

## Feature entry contract

Each feature file starts with an H1 title and one paragraph describing the user-visible behavior. It then uses exactly four H2 sections in this order.

1. `Sub-features` lists short IDs with one line for each behavior.
2. `How to get to it (user POV)` lists every user entry point.
3. `Driving it with control-trayscale` starts with `Preconditions:` and uses labeled bullets that pair each user action with an exact command and observable result.
4. `Gotchas` lists traps that can waste or invalidate a verification run.

Keep implementation details out of the map. Name only user paths, stable handles, required state, commands, and observable proof.

## Features

- [Main window](./main-window.md) covers the window title, sidebar sections, and connected vs not-connected chrome.
- [This machine](./this-machine.md) covers the self page: hostname, Tailscale IPs, options, files, routes, and network check.
- [About](./about.md) covers the About dialog from the app menu action.
- [Preferences](./preferences.md) covers the Preferences dialog and its General and Taildrop groups.
- [Peer search](./peer-search.md) covers opening search from the toolbar button and from Ctrl+F.
