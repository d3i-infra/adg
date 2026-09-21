# ADG — Architectural Decision Guidance

A command-line tool for managing **Architectural Decision Records (ADRs)** and compiling them into
architecture-context **briefs** that a coding agent reads before it edits. ADRs live in a *model* —
a directory of `NNNN-slug.md` files — as **lean** records: one-screen Decision/Guidance rules with
routing frontmatter (`applies_to`, `excludes`, `forbids`, `companions`) that answer the active
question — *what rule governs my next edit, and how do I know if I've violated it?* `adg` authors
and validates the records, and compiles the ones that match a change into a brief injected at edit
time via a Claude Code hook. The human reasoning lives in record-only sections (`## Why`) that never
inflate the injected brief.

Lean is the sole user-facing format
([ADR-0016](./docs/decisions/0016-lean-is-the-sole-user-facing-adg-adr-format-madr-is-shared-parsing-plumbing.md));
the MADR authoring lifecycle was retired in v2.0.0.

`adg` began as a derivative of the [adr/ad-guidance-tool](https://github.com/adr/ad-guidance-tool) CLI
and has since been rebuilt around lean records and compiled briefs — see
[Acknowledgements](#acknowledgements).

## Install

**Package manager (recommended).** One command, then `adg` is on your `PATH` and upgrades with
everything else:

| Platform | Install | Upgrade |
|---|---|---|
| macOS, Linux, Windows (Node + pnpm) | `pnpm add -g @d3i-infra/adg` (or `npm install -g @d3i-infra/adg`) | `pnpm add -g @d3i-infra/adg@latest` |
| Arch Linux (AUR) | `yay -S adg-bin` (or `paru -S adg-bin`) | with your usual `-Syu` |
| Any Go toolchain | `go install github.com/d3i-infra/adg@latest` | same command |

A pnpm project can also pin `adg` for the whole team with `pnpm add -D -E @d3i-infra/adg` and call
it as `pnpm exec adg …` (this is how the governed repos' CI runs it). The Claude Code hooks call
`adg` bare, outside `node_modules/.bin`, so the global install above is still needed on each
developer machine.

**Prebuilt binary (no package manager).** Installs the latest release into `~/.local/bin`:

```sh
curl -fsSL https://raw.githubusercontent.com/d3i-infra/adg/main/install.sh | sh
```

Pin a version with `ADG_VERSION=v4.0.0` or change the location with `ADG_INSTALL_DIR`. Binaries for
macOS/Linux/Windows (amd64/arm64) are on the
[Releases](https://github.com/d3i-infra/adg/releases) page.

**From source** (needs Go 1.24+):

```sh
git clone https://github.com/d3i-infra/adg.git
cd adg
go build           # produces ./adg
# or:
go install ./...   # installs to $GOBIN
```

## Lean format and governance

A lean record is one screen, optimized for the compiled brief. Required sections are `Decision`,
`Guidance`, and `Why` (the reasoning — required on every accepted record, but **record-only**: never
rendered into a brief); `Checks` is optional. Routing lives in frontmatter:

```markdown
---
status: accepted          # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN | amended by ADR-NNNN
category: Extraction      # groups the generated index (not the directory layout)
priority: invariant       # invariant | default — force in the brief
applies_to:
    - port/**/*.py
excludes:
    - "**/port_helpers.py"
---

# Reject unsafe uploads before validation and extraction

## Decision

One to three sentences: what was decided.

## Guidance

- What new code must do, what review rejects, the fix path.

## Why            # required on an accepted record; record-only (never in a brief)
## Checks         # optional; grep targets rolled up into the brief
```

- A path is **governed** iff some `applies_to` glob matches it and no `excludes` glob does.
- `forbids` is negative-space scope — paths that should *not* exist; it routes the brief like
  `applies_to` but warns when it *does* match instead of when it's stale.
- `companions` are expected partner edits (e.g. the TS side of a prop) that the ADR does **not**
  govern; they surface in the brief as "related files," never routed on.
- IDs are a flat global `NNNN` across the model; `category` (not a subfolder) groups the index.
- Globs are forward-slash, repo-root-relative, doublestar (`**`). Brace globs `{a,b}` are rejected —
  write one glob per alternative.
- **Migrating an older ADR** (MADR-format or otherwise): author it as a lean record —
  `adg lean new --from-stdin` with a hidden `--date YYYY-MM-DD` flag to preserve the original decision
  date. There is no auto-converter
  ([ADR-0016](./docs/decisions/0016-lean-is-the-sole-user-facing-adg-adr-format-madr-is-shared-parsing-plumbing.md)).

### Command reference

| Command | Purpose |
|---|---|
| `lean new --title <t> [--status …] [--priority …] [--category …] [--applies-to <glob>] [--excludes <glob>] [--from-stdin]` | Author a lean ADR. Validates the candidate and **refuses to write on a hard failure**, so an invalid record never lands on disk. Prints the new ID. |
| `lean index [--write] [--root <tree>] [--overlaps]` | Validate the model and print/write the grouped README. `--root` scope-lints globs against the source tree (wire this into CI — the hook only routes, the index gates). `--overlaps` adds the opt-in default-vs-default hub diagnostic. |
| `lean brief [--hook] <changed-path…>` | Compile the architecture brief for the changed paths: the ADRs that govern them, grouped by force, each with Decision + Guidance + consolidated checks. `--hook` is the PreToolUse entry point. |
| `lean verify [--hook] [<changed-path…>]` | Re-validate the model and re-show the brief + its "Before you finish" footer for files changed this session. `--hook` is a Stop-hook entry point (advisory, non-blocking). |
| `lean check [<changed-path…>]` | Run the executable grep-assertion checks declared in matched ADRs' `## Checks`. |
| `lean review [<adr-file…>] [--since <ref>]` | Emit a deterministic review packet (target ADRs + lint findings) for a reviewer to judge against the rubric. `adg` makes no LLM call — review runs in a Claude Code subagent ([ADR-0011](./docs/decisions/0011-adg-makes-no-llm-calls-review-runs-in-a-subagent.md)). |

### The brief, the hooks, and CI

The same compiled-brief renderer drives the CLI, the hooks, and CI
([ADR-0002](./docs/decisions/0002-one-canonical-compiled-lean-renderer-shared-by-every-consumer.md)):

The `write-adr` plugin bundles a suite of **fail-open** hooks that route the brief across the change
lifecycle — the 2 KB-capped session digest (a grouped titles-only tripwire index) at `SessionStart`, the
whole-corpus brief at `Plan`-subagent dispatch, the per-context-deduped
file-scoped brief before an edit (each fresh subagent gets its own first injection), a staged-file brief
before a commit — plus a **guard** that blocks hand-creating an ADR record and two **agent** reviewers
(code-vs-ADR compliance at commit, ADR-quality after a record edit). The hard stops: a commit that stages a
`forbids` violation, hand-creating a record, and a commit the compliance judge denies with a cited
violation; everything else advises.

- **CI** runs `adg lean index --root .` for real enforcement (stale globs, duplicate IDs, brace globs,
  leanness lints). The hooks route and advise; the index gates.

The full suite, the exact hook JSON, and a worked example model live in
[`docs/lean-example/hook-setup.md`](./docs/lean-example/hook-setup.md). Because the hooks fire only on
Claude's tool calls and are fail-open, **"no brief appeared" never means "no rule applies"** —
comprehensive enforcement is CI / review / executable checks.

---

## Claude Code plugin (ADR skills)

The [`write-adr`](./tools/adr-plugin/) plugin ships *with* `adg` so its guidance tracks the CLI in
lockstep. It provides three skills — one for *authoring*, one for
*obeying* lean briefs while changing code, and a *gateway* that routes any ADR task to the right one:

- **using-write-adr** — the gateway: broadly discoverable ("Use when ADRs come up in any way"), it routes
  ADR work to `adg` + the specific skill instead of letting the agent hand-roll it.
- **write-lean-adr** — author/migrate/rewrite/review lean records with routing frontmatter.
- **follow-adr-governance** — a behavior primer for obeying an injected lean brief while editing code
  (the hook and the brief do the real work).

```
/plugin marketplace add d3i-infra/adg
```

The skills and hooks call `adg` as a bare command, so **install it first** (see [Install](#install)); the
plugin does not bundle or download it. The `d3i-claude-skills` marketplace **lists** this plugin via a
`git-subdir` source pinned to `main` — a reference to this repo, which stays the canonical source. At
session start the plugin checks that the installed `adg` is not older than the version it ships for
and prints the install or upgrade command for your platform when it is missing or older.

---

## Scripting (stdout / stderr / exit codes)

`adg` follows the usual Unix conventions, so it's safe to pipe and script:

- **stdout** carries machine-readable values: `lean new` writes the new ID; `lean brief` / `lean index`
  write their rendered output.
- **stderr** carries human-readable status and all errors.
- **`--quiet`** (global) suppresses stderr status; machine values on stdout and errors on stderr still
  print.
- **Exit codes:** `0` on success; `1` on any failure including validation issues.

```sh
ID=$(adg lean new --title "Bounded subprocess output")   # captures 0007
adg --quiet lean new --title "X"                         # only the ID prints
adg lean index || echo "model has problems"              # exit 1 when issues exist
```

This split is an invariant
([ADR-0008](./docs/decisions/0008-route-machine-output-to-stdout-status-to-stderr.md)).

## Per-project settings (no global config)

`adg` keeps no global user state: an invocation's behavior is determined by the repo and the
flags alone. The model directory is `docs/decisions` by convention; pass `--model <dir>` per
invocation if a repo keeps its records elsewhere. A repo may tune authoring discipline with one
version-controlled file beside its records:

```yaml
# docs/decisions/.adg.yaml
body_budget: narrative   # lean (default) | narrative — relaxes only the one-screen nudge
```

Settings are advisory and fail-open, and they never change what a compiled brief injects
([ADR-0015](./docs/decisions/0015-per-project-config-lives-in-adg-yaml-read-at-the-command-edge-and-never-changes-the-brief.md)).

## The tool's own decisions

`adg` governs itself. Its current architectural decisions are lean records in
[`docs/decisions/`](./docs/decisions/) (the routing kernel, the canonical renderer, single-format
consolidation, enforcement tiers, round-trip stability, relationship types, stdout/stderr, no-index, …).
A worked lean example model is in [`docs/lean-example/`](./docs/lean-example/).

## Contributing

Business logic lives in the domain (`internal/domain/`); commands are thin cobra adapters
([ADR-0017](./docs/decisions/0017-commands-are-thin-cobra-adapters-over-the-domain.md)). Tests use
[testify](https://github.com/stretchr/testify). For changes:

1. Start with the domain logic (`internal/domain/decision/lean` for anything lean).
2. Add or extend the thin cobra command at the adapter layer; route machine output to stdout, status
   to stderr.
3. Cover with unit tests, and run `go test ./...` before pushing.

## Acknowledgements

`adg` is derived from [adr/ad-guidance-tool](https://github.com/adr/ad-guidance-tool) by Raphael
Schellander and collaborators at the [Eastern Switzerland University of Applied Sciences](https://www.ost.ch/en/),
released under the Apache License 2.0. Their CLI skeleton (cobra root command, the
`internal/adapter/command` layout) and the idea of a curated *model* of recurring architectural
decisions formed the basis of this tool; the record format, routing, brief compilation, hooks, and
plugin were built here. The original theses:

- [Concept Alternatives for the Management of Architectural Decisions in Clean Architectures](https://eprints.ost.ch/id/eprint/1280/1/MSECS-FS24-CleanArchitectureDecisionsConceptsRS.pdf)
- [A Command-Line Tool for Managing Recurring Architectural Decisions](https://eprints.ost.ch/id/eprint/1287/1/PA2-Raphael-Schellander.pdf)

The lean record's frontmatter and file conventions descend from [MADR](https://adr.github.io/madr/).

## License

Apache License 2.0 — see [LICENSE](./LICENSE).
