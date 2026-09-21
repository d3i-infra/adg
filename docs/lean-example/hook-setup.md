# Hooks: inject the brief, guard the model

The `write-adr` plugin bundles a suite of Claude Code hooks that route the compiled lean brief into the
model across the change lifecycle, and guard the ADR model itself. Every hook is **fail-open** (any error,
or no governing ADR, injects nothing and the action proceeds) except two deliberate blocks, noted below.

| When | Event · matcher | What it does |
|---|---|---|
| Session start | `SessionStart` · all sources | **greet + version check**: announce the governance (even on an empty model), and prompt to update `adg` if it lags |
| Any ADR prompt | `UserPromptSubmit` (script filters) | **ADR router**: point the agent at the write-adr skills + `adg` when a prompt mentions ADRs (else silent) |
| Session start | `SessionStart` · startup/clear/compact | inject the **session digest** (grouped titles-only tripwire index, `!` = invariant, capped at 2 KB so it always lands inline) once |
| Plan dispatch | `SubagentStart` · `Plan` | inject the **whole-corpus brief** into the planning subagent (implementers get the file-scoped brief at first edit instead) |
| Before an edit | `PreToolUse` · Edit/Write/MultiEdit | inject the **file-scoped brief** (deduped per context: session, or subagent within it) |
| Before a commit | `PreToolUse` · Bash | brief the **staged** files; **block** on a `forbids` hit (deterministic) |
| ADR write/edit | `PreToolUse` · Edit/Write/MultiEdit | **guard**: **block** hand-creating a record, warn on editing one |
| Before a commit | `PreToolUse` · Bash + `if: git commit *` / `if: git -c *` | **agent judge**: deny the commit with a cited violation when the staged code breaks a governing ADR; fail open otherwise |
| After an ADR edit | `PostToolUse` · Edit/Write + `if: docs/decisions/*.md` | **agent reviewer**: judge the changed **record** against the lean rubric, feeding fixes back |

The blocks are the commit `forbids` gate, the ADR-creation guard, and a judge-denied commit ([ADR-0005](../decisions/0005-validation-has-enforcement-tiers.md)); everything else advises. The agent hooks need `Bash(adg:*)` in the repo's `permissions.allow` — a denied subagent command deadlocks the hook until its timeout.

## 1. Install adg

The hooks invoke `adg` as a bare command, so it must be on your `PATH` (a project-local
`node_modules/.bin/adg` is not enough). Install it with your package manager:
`pnpm add -g @d3i-infra/adg` (macOS, Linux, Windows), `yay -S adg-bin` (Arch), or
`go install github.com/d3i-infra/adg@latest`. The full table is in the
[repo README](../../README.md#install).

From source: `go install .`. The hook entry point is `adg lean brief --hook` (with `--whole` / `--invariants`
/ `--staged` / `--guard` selecting the mode); the two agent hooks need no `adg` change — they orchestrate
`git diff` + `adg lean brief`.

## 2. Enable

**With the plugin (recommended):** enabling `write-adr` registers all of the above from
`tools/adr-plugin/hooks/hooks.json` — you only need step 1. Skip to Verify.

**Without the plugin:** copy that `hooks.json` into the target repo's `.claude/settings.json` and point each
`--model` at the repo's lean ADR directory. The hooks run from the project root, so a relative `--model`
resolves, and the edited path is relativized before matching (write `applies_to` globs repo-root-relative,
e.g. `port/**/*.py`). The two agent hooks (`type: agent`) and `FileChanged` are **experimental** — confirm
they fire in a live session before relying on them.

## 3. Verify

Run `/hooks` in Claude Code to confirm registration. Test a `command` hook directly with the JSON Claude
Code sends:

```
printf '{"cwd":"%s","tool_name":"Edit","tool_input":{"file_path":"%s/port/helpers/flow_builder.py"}}' "$PWD" "$PWD" \
  | adg lean brief --hook --model docs/lean-example
```

A governed file prints a `hookSpecificOutput.additionalContext` object; an ungoverned file prints nothing.
`--whole`/`--invariants` take a `SessionStart`/`SubagentStart` payload; `--guard`/`--staged` take the
Edit/Bash payload. The `agent`/`FileChanged` hooks can only be exercised in a live session.

## Notes

- **Advisory routing, not comprehensive enforcement.** The hooks fire on Claude's tool calls only — not on
  shell edits, formatters, or other tools — and are fail-open, so **"no brief appeared" never means "no rule
  applies."** Real enforcement is `adg lean index --root` in CI, the commit gate below, and the executable
  `## Checks`.
- **Token cost is the point:** only matching ADRs are injected; an edit to an ungoverned file costs zero.
  The file-scoped brief dedupes per context — the session, or the subagent within it, so each fresh
  subagent gets its own first injection (each ADR at most once per context; a `forbids` hit always
  re-surfaces); the session digest fires once per session and is ≤ 2 KB by construction; the whole-corpus
  brief fires once per Plan subagent; the guard never dedupes.
- **Prereq:** the model must be lean records with routing frontmatter (`applies_to`, optional
  `excludes`/`forbids`/`companions`/`priority`). See
  [`lean-format.md`](../../tools/adr-plugin/skills/write-lean-adr/references/lean-format.md).
- **Where the logic lives:** `lean.HookContext` / `SessionDigest` / `SessionBrief` / `SubagentBrief` /
  `CommitAdvisory` / `ModelGuard` in `internal/domain/decision/lean/`; `adg lean brief --hook …` is a thin
  stdin/stdout shell.

## Commit-time enforcement gate (recommended)

The bundled commit hook advises; hard enforcement belongs in a git `pre-commit`. The lean template
(`tools/adr-plugin/skills/write-lean-adr/assets/githooks/pre-commit`) runs once per commit: `adg lean index
--root .` (validates the model) and `adg lean check` on staged files (the executable grep rules). It is
**graceful** — if `adg` is absent it warns and lets the commit through — so CI is the comprehensive gate.
Install with `cp …/pre-commit .git/hooks/pre-commit` (or the same logic in `.husky/pre-commit`); override the
model dir with `ADR_MODEL` (default `docs/decisions`).

A **`Stop`** hook (`adg lean verify --hook`) that re-shows the brief for changed files at turn-end is
available for manual wiring but **not bundled** — it fires every turn, too often; prefer the commit gate.
