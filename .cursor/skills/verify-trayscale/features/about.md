# About

About shows the application name, developer, version, What’s New, Website, Report an Issue, and Legal inside a dialog named About.

## Sub-features

- `about-open` opens the About dialog from each listed entry point.
- `about-identity` shows `Trayscale` and `DeedleFake`.
- `about-version` shows a version button or label.
- `about-links` shows `What’s New`, `Website`, `Report an Issue`, and `Legal`.

## How to get to it (user POV)

- Choose About in the primary window menu (the menu button in the sidebar header).
- Activate the `about` application action (what that menu item calls).

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- No About dialog is required before the first open. If `dialog 'About'` is already present, skip `about-open` and capture identity from the open dialog.

- **Menu action.** Open About. Run `control-trayscale action about`. Exit is 0.
- **Dialog.** Wait for the dialog. Run `control-trayscale wait --role dialog --name About --exact --timeout 10`. Output is `dialog 'About'`.
- **Identity.** Capture the tree. Run `control-trayscale snapshot --path .cursor/skills/verify-trayscale/artifacts/about/open.tree.txt`. Under `dialog 'About'` the tree contains `label 'Trayscale'` and `label 'DeedleFake'`.
- **Version.** That snapshot contains a `button` or `label` whose name starts with `v` inside the About dialog.
- **Links.** That snapshot contains `What’s New`, `Website`, `Report an Issue`, and `Legal`.
- **Proof.** Keep `open.tree.txt`. It must contain `frame 'Trayscale'`, `dialog 'About'`, `label 'Trayscale'`, and `label 'DeedleFake'`.

Optional: `control-trayscale screenshot --path .cursor/skills/verify-trayscale/artifacts/about/open.png --allow-fail`.

## Gotchas

- AdwAboutDialog children can take a moment to appear. Wait for `dialog 'About'`, then snapshot. If `DeedleFake` is missing, wait one second and snapshot again.
- The version string is a git describe and can include `+dirty`. Assert a `v` prefix, not an exact build id.
- `press --key Escape` often does not close the dialog on GNOME Wayland. Cleanup tears the process down; do not require a close path.
- Do not activate `about.save-debug-info` unless you intend to write a debug dump.
- Choosing Website or Report an Issue can open a browser. Do not click those links in a routine proof.
