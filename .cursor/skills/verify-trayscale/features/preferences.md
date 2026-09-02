# Preferences

Preferences is a dialog with a General group (tray icon and polling interval) and a Taildrop group (auto-save received files).

## Sub-features

- `prefs-open` opens the Preferences dialog from the app menu action.
- `prefs-general` shows a `General` group.
- `prefs-taildrop` shows a `Taildrop` group.

## How to get to it (user POV)

- Choose Preferences in the primary window menu.
- Activate the `preferences` application action (what that menu item calls).

## Driving it with control-trayscale

Preconditions:

- `control-trayscale doctor` reports `frame 'Trayscale'`.
- Launch compiled the schema into the run dir so the dialog can bind GSettings. If settings were missing, the app would toast `Settings schema not found` instead of opening the dialog.

- **Menu action.** Open Preferences. Run `control-trayscale action preferences`. Exit is 0.
- **Dialog.** Wait for the dialog. Run `control-trayscale wait --role dialog --name Preferences --exact --timeout 10`. Output is `dialog 'Preferences'`.
- **Groups.** Capture the tree. Run `control-trayscale snapshot --path .cursor/skills/verify-trayscale/artifacts/preferences/open.tree.txt`. Under `dialog 'Preferences'` the tree contains `grouping 'General'` and `grouping 'Taildrop'`.
- **Proof.** Keep `open.tree.txt`. It must contain `frame 'Trayscale'`, `dialog 'Preferences'`, `grouping 'General'`, and `grouping 'Taildrop'`.

## Gotchas

- Switch row titles such as `Use Tray Icon` are often unnamed in AT-SPI. Prove the groups, not those titles, until a snapshot actually contains them.
- Launch uses `GSETTINGS_BACKEND=memory`. Preference changes in this instance do not write the user dconf database. Still avoid toggling tray or auto-save unless the feature under test is that binding.
- Enabling Auto-save Received Files with an empty folder opens a file chooser. Do not turn that switch on in a routine proof.
- The isolated bus has no tray watcher. A tray-icon switch does not place an icon on the user panel.
