# Peer page

Each peer in the sidebar has its own page. It shows the peer's hostname and MagicDNS name, its Tailscale IPs with copy buttons, a Misc. group with connection details, and the routes the peer advertises.

## Sub-features

- `peer-open` shows a peer's page when the user picks that peer.
- `peer-identity` shows the hostname as the title and the MagicDNS name below it.
- `peer-ips` lists each address under `Tailscale IPs` with a copy button.
- `peer-misc` shows `Online`, `Created at`, `Last handshake`, `Bytes received`, and `Bytes sent` under `Misc.`, plus `Last seen` for an offline peer and `Use as exit node` for a peer that offers an exit node.
- `peer-routes` shows `No advertised routes.` when the peer advertises none.

## How to get to it (user POV)

- Choose the peer's row in the sidebar.
- Search for the peer and choose the result.

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Tailscale is connected. For the full set, the tailnet has an online peer, an offline peer (`show-offline-peers` is on by default), and a peer that offers an exit node.
- `select` cannot open a peer page: it highlights the row but the content page stays put. Open the page through search instead.

- **Open.** Start search and type a distinctive part of the peer's name. Run `control-trayscale action search-peers`, then `control-trayscale fill --role entry --value "<token>"`. When the visible page is not a match, the first match becomes the visible page.
- **Identity.** Snapshot `.cursor/skills/verify-trayscale/artifacts/peer-page/<peer>.tree.txt`. The tree has a `panel` named after the peer's sidebar label, and under it the hostname label and the MagicDNS label (the full name with a trailing dot).
- **IPs.** That snapshot contains `grouping 'Tailscale IPs'` and a `list item` per address, each with a `button` of the same name.
- **Misc.** It contains `grouping 'Misc.'` with list items `Online`, `Created at`, `Last handshake`, `Bytes received`, and `Bytes sent`. For an offline peer it also has `Last seen`. For an exit-node peer it has `switch 'Use as exit node'`. Do not toggle it.
- **Routes.** It contains `grouping 'Advertised Routes'` and `No advertised routes.` unless the peer advertises subnets.
- **Proof.** Keep one snapshot per peer kind. Each must include `frame 'Trayscale'`, the peer's `panel`, `grouping 'Tailscale IPs'`, and `grouping 'Misc.'`. Close search afterwards with `control-trayscale click --role toggle --name "Search peers" --exact`.

## Gotchas

- Sidebar row icons (online, offline, exit node, active exit node) are not in the AT-SPI tree. Check them in a screenshot when the compositor allows one.
- The `Online` row shows its value as an icon, so the snapshot only proves the row exists.
- `Created at`, `Last handshake`, and the byte counts depend on the live peer and can be empty or `0`.
- The page menu (Copy FQDN, Send file, Send directory) sits behind an unnamed header button. Sending opens a file chooser; leave it out of a routine proof.
- `Use as exit node` changes the live Tailscale state. Never toggle it in a routine proof.
