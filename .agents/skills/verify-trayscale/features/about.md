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
- **Identity.** Capture the tree. Run `control-trayscale snapshot --path .agents/skills/verify-trayscale/artifacts/about/open.tree.txt`. Under `dialog 'About'` the tree contains `label 'Trayscale'` and `label 'DeedleFake'`.
- **Version.** That snapshot contains a version `button` or `label` inside the About dialog. A `control-trayscale launch` build (`go build` of the checkout) shows `(devel)`. A module or release build shows a `v...` version string.
- **Links.** That snapshot contains `What’s New`, `Website`, `Report an Issue`, and `Legal`.
- **Proof.** Keep `open.tree.txt`. It must contain `frame 'Trayscale'`, `dialog 'About'`, `label 'Trayscale'`, and `label 'DeedleFake'`.

Optional: `control-trayscale screenshot --path .agents/skills/verify-trayscale/artifacts/about/open.png --allow-fail`.

## Gotchas

- AdwAboutDialog children can take a moment to appear. Wait for `dialog 'About'`, then snapshot. If `DeedleFake` is missing, wait one second and snapshot again.
- The version string is the Go module version stamped at build time. `control-trayscale launch` shows `button '(devel)'` (and `label '(devel)'`). A module or release build shows a `v...` tag or pseudo-version, with `+dirty` when the checkout has local changes. Assert that a version button or label is present, not an exact build id, and do not require a `v` prefix on a launch-built binary.
- `press --key Escape` often does not close the dialog on GNOME Wayland. Cleanup tears the process down; do not require a close path.
- Do not activate `about.save-debug-info` unless you intend to write a debug dump.
- Choosing Website or Report an Issue can open a browser. Do not click those links in a routine proof.
