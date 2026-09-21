---
name: using-write-adr
description: >
  Use when ADRs, architecture decisions, docs/decisions, or the adg tool come up in any
  way — planning a change with architectural implications, authoring / migrating / rewriting
  / reviewing a decision record, editing docs/decisions/, or just discussing decisions.
  Routes ADR work to the right write-adr skill and the adg CLI instead of hand-rolling it.
---

# using-write-adr — do ADR work through `adg`, not by hand

If a task touches Architecture Decision Records (ADRs) — authoring, migrating, rewriting,
reviewing, or obeying them — it goes through the `adg` CLI and the write-adr skills. **Do not
hand-roll ADR work**: don't hand-write record files, reinvent the lean format, guess IDs, or
do a manual migration without the tooling. If you think a task might involve ADRs, load the
matching skill below before acting.

## Which skill to load

- **Authoring / migrating / rewriting / reviewing a lean record** → load **write-lean-adr**.
  Records are created with `adg lean new` (add `--date <original>` to preserve a migrated
  record's decision date), validated with `adg lean index`, and reviewed against the rubric
  with `adg lean review`. Never hand-write a record file — the ADR guard hook blocks creating
  one by hand.
- **Obeying an injected architecture brief while changing code** → load
  **follow-adr-governance**; pull the brief yourself with
  `adg lean brief --model docs/decisions <paths you expect to touch>`.

## `adg` is the tool

`adg` is on `PATH`. Core commands: `adg lean brief` (route paths → governing ADRs), `adg lean
new` (author), `adg lean index` (validate + regenerate the README), `adg lean review` (rubric
review), `adg lean check` (executable checks). If an `adg` command errors or `adg` is missing,
surface it and prompt the user to install `adg` with their package manager (the session-start hook prints the command) — do not fall back to editing records by
hand.

## When the model looks empty

If `adg lean brief` returns nothing and the routing hooks are silent, the lean model may not be
populated yet — records need `applies_to` frontmatter, e.g. in a repo still mid-migration from
MADR. That is expected: **bootstrap the lean records first** (that bootstrap is itself
write-lean-adr work), then the brief and the edit/commit hooks start engaging.
