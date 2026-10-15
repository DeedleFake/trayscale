---
name: verify-trayscale
description: Prove a Trayscale UI change by driving the GTK 4 / Libadwaita window the way a user does and capturing AT-SPI evidence. Use when proving a Trayscale UI change, checking the main window, About, Preferences, peer search, the this-machine page, or a peer page.
---

# Verify Trayscale

Launching, driving, the AT-SPI handle table, forbidden actions, tsharness, and cleanup live in the `drive-trayscale` skill. Read it first and use its `CTRL`:

```bash
CTRL=.agents/skills/drive-trayscale/scripts/control-trayscale
```

This skill adds only what makes a run count as proof.

Read `features/README.md` before a proof. Drive every entry point the matching feature file lists, not one convenient path. Do not use any action `drive-trayscale` forbids unless a feature file says to and passes `--i-mean-it`.

## Evidence

Store proof under `.agents/skills/verify-trayscale/artifacts/<feature>/`. `cleanup` does not touch it. Do not commit artifacts; they can contain peer names and Tailscale IPs.

Required for a UI proof:

1. Drive the user path (window, menu action, or listed control), not an internal setter.
2. Capture the tree **before** the action and **after** it when the action changes state.
3. The after-tree must identify Trayscale (`frame 'Trayscale'` or `application 'trayscale'`) and the feature's end state.
4. When the action has a side effect outside the window (a sent file, a changed pref), check it too: for sends, `inner.log` and the files under `/tmp/tsharness/receiver/state/files/`; for prefs, `/tmp/tsharness/tailscale debug prefs`.

```bash
$CTRL snapshot --path .agents/skills/verify-trayscale/artifacts/<feature>/after.tree.txt
$CTRL screenshot --path .agents/skills/verify-trayscale/artifacts/<feature>/after.png --allow-fail
```

If the compositor denies the screenshot, `--allow-fail` writes `after.png.skipped.txt` and continues; the AT-SPI snapshot is then the proof.

## Mocks

None. The window talks to a real `tailscaled`: the user's, or tsharness's on a machine without one. Do not fake IPN status.

- `tailscaled` running but not connected: the `Not Connected` page is the correct UI.
- A feature that needs peers in a given state (exit node, offline, routes) uses tsharness where no system `tailscaled` runs.
