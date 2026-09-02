# Peer search

Peer search filters the sidebar to matching peers. The user opens it from the search button or Ctrl+F, types a query, and either sees matching rows or No matching peers.

## Sub-features

- `search-button` opens search from the `Search peers` toggle button.
- `search-shortcut` opens search from Ctrl+F (`app.search-peers`).
- `search-match` keeps matching peer rows in the sidebar.
- `search-empty` shows `No matching peers` when nothing matches.

## How to get to it (user POV)

- Choose the search button in the sidebar header (tooltip Search peers).
- Press Ctrl+F while the window is focused.

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Tailscale is connected and the sidebar lists at least one peer label besides this machine.
- GtkSearchEntry does not currently expose an AT-SPI text interface. `search-match` and `search-empty` are `verified-unreachable` until `control-trayscale find --role entry --name "Search peers"` succeeds.

- **Button entry.** Choose Search peers. Run `control-trayscale click --role toggle --name "Search peers" --exact`. Output names `toggle button 'Search peers'`.
- **Shortcut entry.** Open search through the same action as Ctrl+F. Run `control-trayscale action search-peers`. Exit is 0.
- **Entry present?** Run `control-trayscale find --role entry --name "Search peers"`. If that fails, record `search-match` and `search-empty` as `verified-unreachable` with that command, and stop.
- **Match.** If the entry exists, fill a token from a visible sidebar peer label. Run `control-trayscale fill --role entry --name "Search peers" --value "<token>"`. The snapshot still contains that peer label and does not contain unrelated peers that fail the token.
- **Empty.** Fill a token that no peer has, for example `zzzxnotapeer`. Run `control-trayscale fill --role entry --name "Search peers" --value "zzzxnotapeer"`. The tree contains `No matching peers`.
- **Proof.** Snapshot `.cursor/skills/verify-trayscale/artifacts/peer-search/after.tree.txt`. For the open-search paths, the file contains `toggle button 'Search peers'` and `frame 'Trayscale'`.

## Gotchas

- `press --key Control+f` is not a reliable Ctrl+F on GNOME Wayland. Use `action search-peers`.
- `click` on a peer `label` defaults away from `clipboard.copy`. Use `select --name` to change the selected sidebar row.
- Self and Mullvad pages are omitted from search results. A query that only matches this machine can look empty.
- Do not report `search-match` as verified because the shortcut action returned 0. That only proves search was asked to open.
