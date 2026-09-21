---
status: accepted
date: "2026-07-01"
category: Release
applies_to:
    - tools/adr-plugin/.claude-plugin/plugin.json
priority: invariant
---

# Ship every plugin.json version bump with its tag and release

## Decision

The d3i-claude-skills marketplace pins the `write-adr` plugin at `ref: main`, so merging to `main` rolls the release out. A commit that changes `plugin.json` `version` must land with its matching tag `vX.Y.Z` and a published Release — the plugin's session-start check tells every consumer to upgrade to that version, so an untagged bump sends every governed session to a release that does not exist.

## Guidance

- Bump `plugin.json` `version` only as a release: merge to `main`, then immediately `git tag vX.Y.Z && git push origin vX.Y.Z` so the workflow publishes the Release assets, the AUR package, and the npm packages.
- Never leave a `version` on `main` without a pushed tag and a live Release at that version — every consumer tracking `main` is told to upgrade to a version no package manager can serve.
- The tag minus its `v` must equal `plugin.json` `version` exactly; `adg --version` comes from the tag via `-ldflags`, never a hardcoded string.

## Why

With `ref: main` a version bump is instantly live to every consumer, so there is no window to fix a missing release: an untagged or mismatched `version` breaks first-call installs org-wide until the tag lands. Tying the invariant to the version bump keeps "bump == release" atomic.
