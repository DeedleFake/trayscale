# Peer search

Peer search filters the sidebar to matching peers. The user opens it from the search button or Ctrl+F, types a query, and either sees matching rows or No matching peers.

## Sub-features

- `search-button` opens search from the `Search peers` toggle button.
- `search-shortcut` opens search from Ctrl+F (`app.search-peers`).
- `search-match` keeps only matching peer rows in the sidebar, without section titles.
- `search-empty` shows `No matching peers` when nothing matches.
- `search-close` restores the sectioned sidebar when search closes.

## How to get to it (user POV)

- Choose the search button in the sidebar header (tooltip Search peers).
- Press Ctrl+F while the window is focused.

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Tailscale is connected and the sidebar lists at least one peer label besides this machine.
- No dialog is open, so the search entry is the only `entry` in the tree.

- **Button entry.** Choose Search peers. Run `control-trayscale click --role toggle --name "Search peers" --exact`. Output names `toggle button 'Search peers'`.
- **Entry present.** Run `control-trayscale find --role entry`. Output is `entry ''`.
- **Match.** Type a token from a visible sidebar peer label. Run `control-trayscale fill --role entry --value "<token>"`, wait a second, then snapshot. The sidebar list keeps that peer's label, drops peers that do not match, and the tree no longer contains `label 'This machine'`.
- **Empty.** Type a token that no peer has. Run `control-trayscale fill --role entry --value "zzzxnotapeer"`. Then run `control-trayscale find --name "No matching peers" --exact`. Output is `grouping 'No matching peers'`.
- **Close.** Choose Search peers again. Run `control-trayscale click --role toggle --name "Search peers" --exact`. The next snapshot has `label 'This machine'` and the other section titles again, and `control-trayscale find --role entry` fails.
- **Shortcut entry.** Open search through the same action as Ctrl+F. Run `control-trayscale action search-peers`. Exit is 0, and `control-trayscale find --role entry` prints `entry ''`.
- **Proof.** Snapshot `.cursor/skills/verify-trayscale/artifacts/peer-search/<step>.tree.txt` after each step. Each file contains `frame 'Trayscale'` and `toggle button 'Search peers'`.

## Gotchas

- Matching is fuzzy: a peer matches when the token appears in its name, hostname, MagicDNS name, or an address, either as a substring or as letters in order. Use a long, distinctive token so unrelated peers drop out.
- The search entry has no accessible name. `find --role entry --name "Search peers"` fails even while search is open; drop `--name`.
- The entry exists only while search is open. A missing entry before you open search is expected.
- `press --key Control+f` is not a reliable Ctrl+F on GNOME Wayland. Use `action search-peers`.
- `click` on a peer `label` defaults away from `clipboard.copy`. `select --name` highlights a row but does not change the content page.
- Self and Mullvad pages are omitted from search results. A query that only matches this machine shows `No matching peers`.
- A search that matches peers also changes the content page to the first match when the current page is not a match. Snapshot the content page before you assume it is still this machine.
