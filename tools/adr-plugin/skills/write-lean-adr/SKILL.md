---
name: write-lean-adr
description: >
  Use when creating, migrating, rewriting, or reviewing an architecture decision record
  (ADR) in a repo using the adg lean model — including bringing older or MADR-format ADRs
  into the lean format, editing files under docs/decisions/, or any task where an ADR is
  authored, changed, or evaluated. To obey an injected architecture brief while changing
  code use follow-adr-governance.
---

# write-lean-adr — author lean decisions with `adg`

A lean ADR is a one-screen record optimized for the compiled brief: it answers, before an
edit, *what rule governs this file, and how do I know if I've violated it?* This skill is for
**authoring** — creating, migrating, rewriting, and reviewing the records. Obeying a brief
while changing code is the **follow-adr-governance** skill; the PreToolUse hook and the brief
do that at edit time.

**Read `references/lean-rubric.md` before writing** — it is the standard a good lean record
meets (Decision = one rule, the first Guidance bullet load-bearing, a prohibition expressed as
`forbids`, scope narrowed to the enforcement points). Full format spec (frontmatter, glob
rules, what each command enforces): `references/lean-format.md`.

**Which model:** operate on the repo's *active* lean model — `docs/decisions/` by convention
(pass `--model <dir>` per invocation if a repo keeps its records elsewhere). Other ADR-like files — a historical MADR record under
`docs/lean-example/`, seed or template models — are **not** your model; don't route, validate, or
author against them. Identify the active model by its format (lean Decision/Guidance + routing
frontmatter), not by every `NNNN-*.md` in the tree.

## Author: a lean record

`adg lean new` authors the record — new lean records are created with it, not by hand. It
assigns the next flat-global `NNNN` (or `--id`), builds the frontmatter from flags, scaffolds
the body (or reads it from stdin with `--from-stdin`, taking the H1 from `--title`), validates
the candidate against the model, and **refuses to write on a hard failure** so an invalid
record never lands on disk. On success it writes `docs/decisions/NNNN-slug.md`, regenerates the
README, and prints the new ID to stdout (status and warnings go to stderr).

```bash
adg lean new --model docs/decisions \
    --title "Reject unsafe uploads before validation and extraction" \
    --status accepted --priority invariant --category Extraction \
    --applies-to 'port/**/*.py' --excludes '**/port_helpers.py'
# → prints the new ID; scaffolds Decision / Guidance / Why
```

Pass `--from-stdin` to supply the body yourself; otherwise fill in the scaffolded
Decision / Guidance / Why after it is written.

**Migrating an existing ADR?** Pass `--date YYYY-MM-DD` to preserve its original decision
date — otherwise the record is stamped with today's. The flag is intentionally hidden from
`--help` (it backs migration and deterministic tests), but it is supported.

The record it produces:

```markdown
---
status: accepted          # proposed | accepted | rejected | deprecated | superseded by ADR-NNNN | amended by ADR-NNNN
category: Architecture    # groups the generated index (not the directory layout)
priority: invariant       # invariant | default — force in the brief
applies_to:
    - port/**/*.py
excludes:
    - "**/port_helpers.py"
---

# <the rule, as a short imperative instruction>

## Decision

One to three sentences: what was decided.

## Guidance

- What new code must do, what review rejects, the fix path.

## Why            # required on an accepted record — why the rule exists / what it protects, so a reader can generalize (record-only; never in a brief)
## Checks         # optional; grep targets / invariants, rolled up into the brief
```

- **IDs are a flat global `NNNN`** across the whole model — `adg lean new` assigns the next
  free one (or `--id NNNN`, which fails if already taken); `category` (not a subfolder)
  groups the index. Duplicate IDs hard-fail.
- **Globs** are forward-slash, repo-root-relative, doublestar (`**`). **Brace globs
  `{a,b}` are rejected** — write separate globs, one per alternative. A single-star
  segment under a nestable directory (`platforms/*.py`) is flagged; prefer
  `platforms/**/*.py`.
- Keep it to one screen. If it runs longer, it is probably two ADRs.

### Before you finish (lean self-check)

Compact, high-leverage subset of `references/lean-rubric.md` — apply it before accepting:

- [ ] **Decision** is the rule in 1–3 sentences of prose — no list, no per-case enumeration
  (that belongs in Guidance).
- [ ] **Guidance** leads with reviewable bullets, and the *first* bullet would still steer
  the edit alone (it's what a compact brief renders).
- [ ] **`applies_to`** names the few files that enforce the rule, not the whole neighborhood;
  a pure prohibition is `forbids`-only (no `applies_to` re-describing the mechanism).
- [ ] **`Why`** states why the rule exists — required on every accepted record (record-only,
  never injected into a brief); an invariant's `Why` makes explicit what breaks if it is weakened.
- [ ] Body names the **mechanism/file**, not the **ADR number** (numbers churn on renumber).
- [ ] Behavioral rule? List its **test(s)** — as `companions` if they're partner edits, or in
  `applies_to` if a test *is* the rule's enforcement. Add `## Checks` only for concrete
  grep/verification targets not already implied by Guidance.

`adg lean index` flags the mechanical leanness subset of this (Decision-as-list / over-length,
Guidance-without-a-bullet) as advisory warnings; a missing required section (Decision, Guidance,
or `## Why`) on an accepted record is a hard failure.

### Preview how it routes

A record is consumed as a brief, so check it that way before accepting — especially for a
likely **hub** file (one many ADRs already govern), where an over-broad `applies_to` or a
weak first Guidance bullet shows up immediately:

```bash
adg lean brief --model docs/decisions <a file your ADR governs>   # see the compact rendering
adg lean index --model docs/decisions --root . --overlaps         # is this ADR inflating a hub?
```

If the compact line doesn't steer the edit, tighten the first Guidance bullet or narrow the
scope. (Obeying briefs at edit time is the follow-adr-governance skill — this is authoring QA.)

For a judgment-level review against the full rubric, run `adg lean review <adr-file>` (or
`--since <ref>`) — it emits a deterministic packet (the target ADRs + their lint findings).
Then judge each against `references/lean-rubric.md` and report a **pass/revise** verdict with
rubric-anchored fixes; prefer a fresh-context **subagent** per ADR. No API key — `adg` makes no
LLM call, the review uses this session's model access (ADR-0011).

### Retiring or superseding a record

A record that merely *evolved* is usually best **edited in place** — a lean record is the rule
*now*, and git carries what it used to say. Reach for supersession only when the change is a real
replacement whose history matters: author the new record (it carries the routing), set the old
one's `status: superseded by ADR-NNNN`, and add `supersedes: ["NNNN"]` to the new one — `adg lean
index` checks both ends agree. A terminal status (`superseded`/`deprecated`/`rejected`) **retires
the record from routing automatically** — it drops out of briefs and the hook, so you do not strip
its globs; its body stays as history in the index.

## Validate and index

```bash
adg lean index --model docs/decisions                      # validate + print the grouped README
adg lean index --model docs/decisions --write              # write docs/decisions/README.md
adg lean index --model docs/decisions --root .             # also scope-lint globs against the tree
adg lean index --model docs/decisions --root . --overlaps  # + opt-in scope-hub overlap diagnostic
```

`adg lean index` hard-fails on a duplicate ID, a brace glob, or an accepted record missing a
required section (Decision, Guidance, or `## Why`); it warns on glob hygiene, over-length
bodies, and leanness nudges (Decision as a list or over-length, Guidance with no bullet — see
`references/lean-rubric.md`), and (with `--root`) stale `applies_to`/`excludes` and `forbids`
that now match a file. The leanness warnings are
advisory and skip terminal records and unfilled scaffold placeholders. Wire `adg lean index
--root` into CI for real enforcement — the hook only routes; the index gates.

**Default-vs-default overlap is opt-in**, not part of `--root`: overlap between defaults is
usually benign, so it floods CI on a hub-heavy model. Pass `--overlaps` (grouped per-hub
summary — *N files: M defaults apply: ADR-…*) or `--overlaps=pairs` (unaggregated per-pair
detail) when auditing the model; both require `--root` and print an advisory `[info]` block,
never a failure.

## Reference files

- `references/lean-format.md` — the lean format spec: body, frontmatter, glob rules,
  and the "what enforces what" matrix.
- `references/lean-rubric.md` — the authoring rubric: how to keep a record lean (Decision =
  one rule, first-bullet-load-bearing, prohibition-as-`forbids`, scope-to-enforcement, …),
  what the index warns on, and the standard `adg lean review` judges against.
- `assets/githooks/pre-commit` — the commit-time enforcement gate: `adg lean index` +
  `check` on staged files, once per commit (graceful when `adg` is absent). Install per-repo
  (`.git/hooks/` or `.husky/pre-commit`); the edit-time hook is the safety net, this is the gate.
