# Releasing `adg`

`adg` is the CLI; `write-adr` (in `tools/adr-plugin/`) is the Claude Code plugin that drives it.
They ship as **one version-locked bundle**, and a single git tag drives everything.

## The version-lock invariant

One tag `vX.Y.Z` simultaneously fixes:

1. the GitHub Release assets — `adg_<os>_<arch>[.exe]` + `checksums.txt` (built by `.goreleaser.yaml`);
2. the CLI's `adg --version` (the tag, minus the `v`, injected via `-ldflags`);
3. `tools/adr-plugin/.claude-plugin/plugin.json` `version` (**must** equal the tag minus `v`);
4. what consumers receive — the d3i-skills marketplace pins the plugin at `ref: main`, so
   **merging to `main` is the rollout** (there is no tag-pinned ref to bump separately).

The plugin's `bin/adg` wrapper reads its version out of `plugin.json` and downloads
`adg vX.Y.Z` from the Release — so **`plugin.json` version == release tag minus `v`** is a hard
requirement, not a convention. Mismatch ⇒ the wrapper's first download 404s — and because the
marketplace tracks `main`, that mismatch is live to everyone the instant it merges, so the tag and
Release must land immediately after the bump
([ADR-0013](docs/decisions/0013-the-marketplace-tracks-main-so-a-plugin-json-version-bump-must-ship-with-its-tag-and-release.md)).

## Cutting a release

1. In your change PR, bump `tools/adr-plugin/.claude-plugin/plugin.json` `version` → `X.Y.Z`, then
   merge to `main`. Because the marketplace tracks `main`, the merge is the rollout.
2. **Immediately** tag the merge commit and push: `git tag vX.Y.Z && git push origin vX.Y.Z`. Don't
   leave an unreleased version on `main` — it 404s installs
   ([ADR-0013](docs/decisions/0013-the-marketplace-tracks-main-so-a-plugin-json-version-bump-must-ship-with-its-tag-and-release.md)).
3. The `release` workflow runs goreleaser and publishes the GitHub Release with the six
   `adg_<os>_<arch>` assets + `checksums.txt`.
4. Verify: the Release page lists all assets, and a downloaded asset prints `X.Y.Z`:
   `./adg_linux_amd64 --version`.

There is no ref-bump step: the marketplace's `write-adr` entry pins `ref: main`, so consumers get
`X.Y.Z` the moment step 1 merges — which is exactly why step 2 must follow immediately.

## How consumers receive it

- **Plugin skills:** the plugin's `bin/adg` is auto-added to PATH; on first call it lazily downloads
  the matching `adg vX.Y.Z` into `${CLAUDE_PLUGIN_DATA}` (cached, persists across updates).
- **Governed-repo hooks** (PreToolUse/Stop, the git pre-commit hook, the `adr` wrapper): these run
  outside the plugin's PATH and need a system `adg` — install with `install.sh` (see README).

## d3i-skills marketplace entry

```json
{ "name": "write-adr", "source": "git-subdir",
  "url": "daniellemccool/ad-guidance-tool", "path": "tools/adr-plugin", "ref": "main" }
```

The `ref` is pinned to `main` (not a version tag) on purpose: the plugin tracks the latest release
automatically, so there is no per-release adoption PR. The cost is the invariant above — a version
bump on `main` without an immediate tag + Release breaks installs
([ADR-0013](docs/decisions/0013-the-marketplace-tracks-main-so-a-plugin-json-version-bump-must-ship-with-its-tag-and-release.md)).

## Local verification (no real release)

- Version wiring: `go build -ldflags "-X github.com/d3i-infra/adg/cmd.version=v9.9.9-test" -o /tmp/adg . && /tmp/adg --version`
- Release config: `goreleaser check` then `goreleaser release --snapshot --clean` (inspect `dist/`).
