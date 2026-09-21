# adr-plugin — ADR skills for Claude Code

This is a [Claude Code](https://code.claude.com) plugin that ships *with* `adg` so its
guidance tracks the CLI in lockstep — the references in `skills/*/references/` are updated
in the same change that updates the CLI, which is why this repo is the plugin's canonical
home. The `d3i-claude-skills` marketplace **lists** this plugin via a `git-subdir` source pinned to
`main` — a reference to this repo, not a copy — so there is one source of truth and nothing
to sync.

It ships three skills — a *gateway* that routes any ADR task, one for *authoring*, and one for
*obeying* lean briefs while changing code:

- **using-write-adr** — the gateway: broadly discoverable ("Use when ADRs / decisions / adg come up
  in any way"), it routes ADR work to `adg` + the specific skill below rather than hand-rolling it. It
  is the auto-discovery front door; the UserPromptSubmit router hook is its deterministic backstop.
- **write-lean-adr** — author/migrate/rewrite/review compact lean Decision/Guidance records
  with routing frontmatter (`applies_to` / `excludes` / `forbids` / `companions`).
- **follow-adr-governance** — a behavior primer for obeying an injected lean ADR brief while
  editing code. The real work is done by the brief and the PreToolUse hook; this skill is the
  fallback/primer (treat the brief as authoritative, invariants are hard constraints, stop on
  forbidden scope, run the footer's checks/tests).

The split is deliberate: authoring is rare and deliberate (rich skills with a rubric);
consuming is frequent and carried by the hook + the self-contained brief (a tiny skill).

## Layout

```
tools/adr-plugin/
  .claude-plugin/plugin.json     plugin manifest
  skills/write-lean-adr/         lean authoring skill (SKILL.md, references, evals)
  skills/follow-adr-governance/  obey-the-brief behavior primer (SKILL.md, evals)
```

## Install

Either add this repo as a marketplace directly:

```
/plugin marketplace add d3i-infra/adg
```

…or install via a marketplace that references it with a `git-subdir` source pointing at
`tools/adr-plugin` (this is how the `d3i-claude-skills` marketplace lists it, pinned to `main`).

The skills and the bundled hooks call the `adg` CLI as a bare command. `adg` is a **system
dependency**: install it once with your package manager (`pnpm add -g @d3i-infra/adg`, or
`yay -S adg-bin` on Arch (not yet available), or `go install github.com/d3i-infra/adg/v4@latest`; the repo README has the
full table). When it is missing, the
bundled hooks **fail loudly** (an `adg: command not found` hook error at session start and on tool
calls) — deliberate, so the governance never degrades silently — and the SessionStart hook turns that
noise into instructions: it shows the user the install commands directly (as a `systemMessage`) and
briefs the agent to relay them. When the installed `adg` is older than the version the plugin ships
for, the same hook prints the upgrade command.

## Bundled hooks

As of **v1.3.0** the plugin ships `hooks/hooks.json` with a suite of fail-open governance hooks, so
enabling the plugin registers them automatically — a repo no longer needs the manual
`.claude/settings.json` snippets from `docs/lean-example/hook-setup.md` (they remain for non-plugin
users). Every hook routes off the same compiled brief and needs system `adg` on `PATH` plus a
`docs/decisions` lean model:

- **SessionStart** (all sources) → `bin/adg-session-start.sh`. In a governed repo (has `docs/decisions/`)
  it **greets** every session — announcing that the write-adr governance is active and its entry points,
  *even when the lean model is empty* (a read-only or mid-migration session meets no other hook) — and
  **version-checks**: when the system `adg` is missing or older than the plugin, it shows the user the
  package-manager install commands directly (a `systemMessage`) and tells the agent the hooks are idle. Silent in
  ungoverned repos.
- **UserPromptSubmit** (fires on every prompt; the script keyword-filters) → `bin/adr-router.sh`. When a
  prompt mentions ADRs / `docs/decisions` / `adg`, injects a pointer telling the agent to do ADR work
  through the write-adr skills + `adg` (author/migrate with `adg lean new`, review, or obey a brief via
  follow-adr-governance) rather than reinventing it. This is the **deterministic** backstop for skill
  auto-discovery, which is unreliable — a grep fires it, not the model's discretion. Silent otherwise.
- **SessionStart** (matcher `^(startup|clear|compact)$`) → `adg lean brief --hook --digest`. Injects the
  **session digest** — a grouped titles-only tripwire index of every in-force ADR (`!` marks invariants),
  hard-capped at 2 KB so it always lands inline instead of being spooled to a file by the hook-output
  limit. Recall, not instruction: the titles are the payload; full guidance stays on demand
  (`adg lean brief <paths>`, the edit-time hooks). On a large corpus the digest degrades — flat (group
  headers dropped), then invariants only, then a counts-and-pointers floor — but it always arrives.
  (Not on `resume`: the earlier injection is already restored.)
- **SubagentStart** (matcher `^Plan$`) → `adg lean brief --hook --whole`. Injects the **whole-corpus
  brief** into a `Plan` subagent as it starts designing a change — the planner tier of the two-tier
  subagent design. Implementer subagents get no start-time brief (the dispatch payload carries no paths);
  they are served the targeted file-scoped brief at their first edit, below.
- **PreToolUse** (matcher `^(Edit|Write|MultiEdit|NotebookEdit)$`) → `adg lean brief --hook`. Injects the
  governing ADRs for the file *about to be edited*, **deduped per context** — the session, or the
  subagent within it (`session_id` + `agent_id`), so a fresh-context subagent always gets its own first
  injection (each ADR at most once per context; a forbids violation always re-surfaces).
- **PreToolUse** (matcher `^Bash$`) → `adg lean brief --hook --staged`. The *deterministic* commit layer:
  on a `git commit` call, briefs the **staged** files and **blocks the commit** (`permissionDecision:
  deny`) when a staged path hits a `forbids` glob — a deliberate block
  ([ADR-0005](../../docs/decisions/0005-validation-has-enforcement-tiers.md)).
- **PreToolUse** (matcher `^Bash$`, `if: "Bash(git commit *)"` and a twin entry `if: "Bash(git -c *)"`
  for flagged commits) → a **`type: agent`** hook: the *code-compliance judge* (distinct from the
  ADR-quality reviewer below). Before a commit lands, it assesses whether the **staged diff obeys** the
  ADRs governing the touched files (`git diff --cached` + `adg lean brief`) and concludes via its
  structured verdict — ok=true (fail open) or ok=false with a cited violation, which **denies the
  commit** with the reason fed back so the agent can fix and retry. Pinned to Sonnet, 120s timeout
  (fail-open: a timeout silently allows, so the prompt orders invariants judged first); the
  commit pauses while it judges (`if` keeps it off every other Bash call). **Prereq:** the repo must
  allow `Bash(adg:*)` in `permissions.allow` — the judge's subagent Bash calls consult the project
  allowlist, and a denied call deadlocks the hook until its timeout (the SessionStart greeter warns when
  the rule is missing).
- **PreToolUse** (matcher `^(Edit|Write|MultiEdit)$`) → `adg lean brief --hook --guard`. Guards the ADR
  model itself: **blocks** a hand-authored *new* record (a `Write` to a not-yet-existing `NNNN-*.md`) so
  records go through `adg lean new`, and **warns** (advisory, no block) on an *edit* to an existing record
  so the write-lean-adr revise/review flow isn't deadlocked. Not deduped.
- **PostToolUse** (matchers `^Edit$` / `^Write$`, `if: "Edit(docs/decisions/*.md)"` /
  `if: "Write(docs/decisions/*.md)"`) → a **`type: agent`** hook: the *ADR-quality reviewer*. After a
  record under the model changes, it runs `adg lean review` on it, judges the record against the lean
  rubric, and concludes ok=true (pass) or ok=false with rubric-anchored fixes fed back to the editing
  agent. (This replaced a `FileChanged` hook: FileChanged matchers are literal filenames — no globs, and
  relative matchers watch but never match — so they cannot express `NNNN-*.md`.) Same `Bash(adg:*)`
  allowlist prereq as the judge.

The injection hooks are **fail-open** with respect to the edit: no system `adg`, no lean model, or any
error means nothing is injected and the edit proceeds — so "no brief appeared" never means "no rule
applies." Fail-open is not fail-*silent*, though: a missing `adg` errors visibly on every hook so the
gap gets fixed rather than ignored (the SessionStart `systemMessage` carries the fix). The two blocking
hooks (commit `forbids`, ADR-record creation) are the deliberate exceptions to fail-open — and note
neither can fire while `adg` is missing.

Together these cover the lifecycle: **SessionStart/SubagentStart** put the rules in view at *planning*
time, **PreToolUse** is the *edit-time* safety net, and the **commit advisor** is the last gate before a
change lands. The always-loaded `CLAUDE.md` convention (the "Golden path" in
`docs/lean-example/hook-setup.md`) is now complementary rather than the only planning-time channel.
**Comprehensive enforcement** still belongs at **commit time / CI**: install the lean `pre-commit` gate
(`skills/write-lean-adr/assets/githooks/pre-commit`) to run `adg lean index` + `check` once per commit.
The authoring-side review phase is *not* a hook either — it is driven by the `write-lean-adr` skill,
which has a subagent judge `adg lean review` output against the rubric.
