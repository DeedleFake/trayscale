# This machine

This machine is the self peer page. It shows the hostname, Tailscale IPs with copy buttons, option switches, incoming files, advertised routes, and a network check group.

## Sub-features

- `self-identity` shows the hostname and MagicDNS name on the page.
- `self-ips` lists each Tailscale address under `Tailscale IPs`.
- `self-options` shows `Advertise exit node`, `Allow LAN access`, `Accept routes`, and `Accept DNS`.
- `self-files` shows `No incoming files.` when none are waiting.
- `self-routes` shows `No advertised routes.` when none are advertised.
- `self-netcheck` shows `Last run` with `Never` until a check runs.

## How to get to it (user POV)

- After connect, the first sidebar row under `This machine` is selected and its page is visible.
- Choose that row in the sidebar if another page is visible.

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Tailscale is connected. If the snapshot has `Not Connected` instead of `grouping 'Tailscale IPs'`, stop and report `verified-unreachable` with that tree.

- **Open self page.** After launch the self page is already visible. Capture it before any search moves the content page to a peer. `select` cannot bring it back: it highlights the row without changing the page. If a search moved it, close search after a search that matched nothing, or run `cleanup` and `launch` again.
- **Identity.** Run `control-trayscale snapshot --path .cursor/skills/verify-trayscale/artifacts/this-machine/page.tree.txt`. The `panel` named after this machine's sidebar label has a hostname label and a MagicDNS label. On Tailscale's coordination server the MagicDNS label ends in `.ts.net.`; other control servers can use another name.
- **IPs.** That snapshot contains `grouping 'Tailscale IPs'` and at least one `list item` that is not `No addresses.`. Each address has a `button` with the same name as the address.
- **Options.** The snapshot contains `switch 'Advertise exit node'`, `switch 'Allow LAN access'`, `switch 'Accept routes'`, and `switch 'Accept DNS'`. Do not toggle them.
- **Empty files and routes.** The snapshot contains `No incoming files.` and `No advertised routes.` unless the live node actually has files or routes.
- **Netcheck.** The snapshot contains `grouping 'Network Check'`, `label 'Last run'`, and `label 'Never'` if no check has run in this session.
- **Proof.** Keep `page.tree.txt`. It must include `frame 'Trayscale'`, `grouping 'Tailscale IPs'`, and the four option switches.

## Gotchas

- IP strings and hostnames come from the live node. Assert grouping names and the presence of address rows, not a particular IP.
- Copy buttons are named with the address. Their action copies; that is not proof the page loaded.
- `select` on a sidebar label marks `states=selected` on the parent list item but leaves the content page where it was. A selected self row is not proof that the self page is showing; confirm `grouping 'Options'` and the four option switches.
- Checked state on `Allow LAN access` or `Accept DNS` is tailscaled prefs, not a GSettings default.
