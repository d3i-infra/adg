---
status: superseded by ADR-0016
date: "2026-06-29"
category: ADR formats
source: the MADR-era design records, removed 2026-09 (git history before v4.0.0, docs/fork-design/)
priority: default
applies_to:
    - internal/domain/decision/madr/**/*.go
    - internal/domain/decision/lean/**/*.go
    - tools/adr-plugin/skills/**/SKILL.md
---

# MADR and lean are separate user-facing formats, not implementation islands

## Decision

MADR and lean are two distinct user-facing ADR formats — different body shapes, workflows, and skills
(write-madr-adr vs write-lean-adr). A repo uses one or the other. They are NOT separate implementation
islands: the lean package deliberately reuses MADR's low-level primitives (`madr.Frontmatter`,
`SplitFile`, `ParseFrontmatter`). The split is at the user-facing layer, not the parsing layer.

## Guidance

- Sharing low-level parsing/frontmatter primitives across the two formats is fine and encouraged — do
  not fork the YAML/file-splitting code.
- Do **not** leak format-specific *semantics* across the boundary without an explicit compatibility
  decision: no lean-only frontmatter or routing behavior wired into the MADR authoring/validation
  workflow, and no MADR-only body rules imposed on lean records.
- Keep the two skills and their references format-specific; a change that blends the workflows needs its
  own ADR.

## Why

The two formats differ in how a person authors and reads them, but their YAML/frontmatter parsing is
mechanical and identical — forking it would double maintenance and let the two silently drift on plumbing
that should never differ. Drawing the split at the user-facing layer keeps one parsing core while letting
the authoring experiences evolve independently.
