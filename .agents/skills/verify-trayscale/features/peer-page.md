# Peer page

Each peer in the sidebar has its own page. It shows the peer's hostname and MagicDNS name, its Tailscale IPs with copy buttons, a Misc. group with connection details, and the routes the peer advertises.

## Sub-features

- `peer-open` shows a peer's page when the user picks that peer.
- `peer-identity` shows the hostname as the title and the MagicDNS name below it.
- `peer-ips` lists each address under `Tailscale IPs` with a copy button.
- `peer-misc` shows `Online`, `Created at`, `Last handshake`, `Bytes received`, and `Bytes sent` under `Misc.`, plus `Last seen` for an offline peer and `Use as exit node` for a peer that offers an exit node.
- `peer-send-file` sends one or more files to the peer through Taildrop from the page menu's `Send file...`.
- `peer-send-dir` sends a directory from `Send directory...`; it arrives as `<name>.tar.zst`.
- `peer-routes` shows one row per advertised prefix. The row title is the prefix, and the row has no Remove button. The group shows `No advertised routes.` when the peer advertises none. An exit node's default route is not a row.

## How to get to it (user POV)

- Choose the peer's row in the sidebar.
- Search for the peer and choose the result.
- Sends: the page's menu button in the header, then `Send file...` or `Send directory...`, then pick in the file chooser. Dropping files on the page also sends (not drivable through AT-SPI).

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Tailscale is connected. For the full set, the tailnet has an online peer, an offline peer (`show-offline-peers` is on by default), a peer that offers an exit node, and a subnet router.
- `select` cannot open a peer page: it highlights the row but the content page stays put. Open the page through search instead.

- **Open.** Start search and type a distinctive part of the peer's name. Run `control-trayscale action search-peers`, then `control-trayscale fill --role entry --value "<token>"`. When the visible page is not a match, the first match becomes the visible page.
- **Identity.** Snapshot `.agents/skills/verify-trayscale/artifacts/peer-page/<peer>.tree.txt`. The tree has a `panel` named after the peer's sidebar label, and under it the hostname label and the MagicDNS label (the full name with a trailing dot).
- **IPs.** That snapshot contains `grouping 'Tailscale IPs'` and a `list item` per address, each with a `button` of the same name.
- **Misc.** It contains `grouping 'Misc.'` with list items `Online`, `Created at`, `Last handshake`, `Bytes received`, and `Bytes sent`. For an offline peer it also has `Last seen`. For an exit-node peer it has `switch 'Use as exit node'`. Do not toggle it.
- **Routes.** It contains `grouping 'Advertised Routes'`. A peer with no subnet routes has `No advertised routes.` A subnet router has a `list item` titled with the prefix and no `button 'Remove'` on that row.
- **Send file / directory.** Only under tsharness started with `-receiver`, and only after `check-fake-tailnet` prints `FAKE`. Open `receiver`'s page by search, then `click --role "toggle button" --name "" --exact --index 1`, `click --role "menu item" --name "" --exact --index 1` (file) or `--index 2` (directory). Require `dialog 'Select file(s) to send to receiver'` (or `dir(s)`). `press --key /`, `fill --role text --name "" --exact --value <path>`, `click --role button --name Open --exact` (or `Select`). Proof: `inner.log` has `starting file push` and `done pushing file` with the path, and `/tmp/tsharness/receiver/state/files/*/` holds the file (or `<dir>.tar.zst`).
- **Proof.** Keep one snapshot per peer kind. Each must include `frame 'Trayscale'`, the peer's `panel`, `grouping 'Tailscale IPs'`, and `grouping 'Misc.'`. Close search afterwards with `control-trayscale click --role toggle --name "Search peers" --exact`.

## Gotchas

- Sidebar row icons (online, offline, exit node, active exit node) are not in the AT-SPI tree. Check them in a screenshot when the compositor allows one.
- The `Online` row shows its value as an icon, so the snapshot only proves the row exists.
- `Created at`, `Last handshake`, and the byte counts depend on the live peer and can be empty or `0`.
- The page menu (Copy FQDN, Send file, Send directory) sits behind an unnamed header button, and its items have no accessible names; address them by index. Never send against a real tailnet: it delivers files to the user's devices.
- The send items are disabled for peers that are not Taildrop targets (every fake peer except `receiver`).
- `press --key /` only works on X11; on GNOME Wayland the location field may not open, so the send is unreachable there.
- `Use as exit node` changes the live Tailscale state. Never toggle it in a routine proof.
