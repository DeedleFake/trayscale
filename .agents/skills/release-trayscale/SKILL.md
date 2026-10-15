---
name: release-trayscale
description: Prepare a Trayscale release PR that updates the AppStream metainfo releases entry and regenerates PGO when code changes warrant it. Use when cutting a release, adding a version to metainfo, or the user asks to prepare a release.
---

# Release Trayscale

Prepare a release on a branch and open a PR for review. Do not tag, push tags, or create a GitHub Release (the user tags in the GitHub release UI).

## In scope

1. Create or update the top `<release>` in `dev.deedles.Trayscale.metainfo.xml`.
2. If code changes since the previous release warrant it, regenerate `cmd/trayscale/default.pgo` by following the `regenerate-pgo` skill (do not reinvent its drive steps).

## Out of scope

Do **not** do any of these as part of this skill:

- Git tags or GitHub Releases
- Vendor tarball (`go mod vendor`, `bsdtar`, zstd)
- Dependency updates (`go get`, `go.mod` bumps)
- Flathub packaging or bots

## Metainfo entry

File: `dev.deedles.Trayscale.metainfo.xml` (repo root). The About dialog reads the newest `<release>` from this file for release notes.

Insert a new block at the top of `<releases>` (newest first), matching existing style (tabs, `v`-prefixed version, ISO date):

```xml
		<release version="v0.21.0" date="2026-10-15">
			<description>
				<ul>
					<li>User-facing change in plain English.</li>
					<li>Another change.</li>
				</ul>
			</description>
		</release>
```

Rules:

- Ask the user for the version if they did not give one. Keep the `v` prefix (e.g. `v0.21.0`).
- Use today's date as `YYYY-MM-DD` unless the user gives another date.
- If an entry for that version already exists, update its `<description>` and/or `date` instead of duplicating it.
- Build the `<li>` list from user-facing changes since the previous release version (git log / merged PRs). Prefer concrete UI and behavior notes over commit subjects. Skip pure chore/meta noise unless it matters to users (e.g. a required Tailscale version bump).
- Keep older `<release>` entries intact.

Commit the metainfo change alone as `meta: add vX.Y.Z to metainfo` (or `meta: update vX.Y.Z metainfo` if editing).

## When to regenerate PGO

Follow `regenerate-pgo` (and thus `drive-trayscale`) when, since the previous release, there were meaningful changes under paths that affect the hot profile: `internal/ui/`, `internal/tray/`, `internal/tsutil/`, `cmd/trayscale/`, or other code that runs on the poller / peer-update / tray path.

Skip PGO when the release is metainfo-only, docs/skills only, or otherwise does not change those paths. If unsure, ask once; default to regenerating when UI, tray, or poller code changed.

When regenerating: run the `regenerate-pgo` skill as written, then commit as `cmd/trayscale: regenerate default.pgo`. Do not invent alternate harness flags or drive sequences in this skill.

## PR

- Branch from current `master`.
- Include the metainfo commit; include the PGO commit only when you regenerated.
- Open the PR against `master` and send the link.
