# Tray account

The tray menu shows Switch account when Tailscale has more than one login profile. Each profile is a radio item. The active profile is selected. Choosing another profile switches to it.

## Sub-features

- `tray-switch-account` shows a Switch account submenu only when more than one profile exists.
- `tray-account-radios` lists each profile as a radio item under that submenu.
- `tray-account-private` shows each profile name as `profile@example.com` under `TRAYSCALE_PRIVATE=1`.

## How to get to it (user POV)

- Open the tray icon menu while the tray watcher is running.
- Choose Switch account, then choose a profile.

## Driving it with control-trayscale

Preconditions:

- A StatusNotifierWatcher is on the session bus. Isolated `launch` (without `--tray`) has no watcher, so the tray icon never registers. `launch --tray` starts `sni-host` so the tray code runs, but the tray menu is still not in the window AT-SPI tree; `find --name "Switch account"` still fails.
- Tailscale reports at least two login profiles. One profile removes the submenu.
- Do not choose a profile in a routine proof. That switches the live account.

- **Attempt.** After `launch` and `doctor`, run `control-trayscale find --name "Switch account" --exact`. On the isolated bus this fails, including after `launch --tray`. Record `verified-unreachable` with the preconditions above: a StatusNotifierWatcher, at least two login profiles, and a way to read the tray menu other than the window AT-SPI tree.
- **Proof.** A reachable run's tree contains `Switch account` and one radio item per profile. Under `TRAYSCALE_PRIVATE=1` those items are named `profile@example.com`. Opening Switch account also refreshes the profile list.

## Gotchas

- Isolated launch has no tray watcher, so the icon never registers. `launch --tray` does not put `Switch account` into the window AT-SPI tree either.
- A single login profile looks the same as a build with no account submenu. Check the profile count before calling the submenu missing.
- The header profile combo is a different control. It is covered by `profile-private` in `main-window.md`.
- During a switch the Connect or Disconnect, exit node, This machine, and Switch account items are disabled. They come back when tailscaled reports `Running`, or after 5 seconds if the switch stalls.
- Toggling exit node from the tray posts a desktop notification titled `Exit node`. The body is `Enabled: <peer>` when the peer name is known, `Enabled` when it is not, or `Disabled` when turning it off.
