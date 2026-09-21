---
status: accepted
date: "2026-06-29"
category: ADR formats
source: the MADR-era design records, removed 2026-09 (git history before v4.0.0, docs/fork-design/)
priority: invariant
applies_to:
    - internal/domain/decision/madr/parser.go
    - internal/domain/decision/madr/renderer.go
    - internal/domain/decision/lean/parse.go
---

# Parser/renderer round-trip stability is an invariant

## Decision

The format packages must preserve intentional content through a parse → render round-trip and must not
destructively rewrite a record. Unknown H2 sections and unknown frontmatter keys survive verbatim; only
explicitly generated sections (the `## Comments` block, regenerated from frontmatter) are rewritten, and
only deliberately.

## Guidance

- A parser/renderer change must ship with a fixture or round-trip test — especially for unknown sections
  and frontmatter and for generated sections. Extend `internal/domain/decision/madr/roundtrip_test.go`
  and its `testdata/fixtures/` (the load-bearing property: parse → render equals the original).
- Preserve `CustomSections` passthrough and `omitempty` frontmatter; do not drop keys or sections the
  format does not recognize.
- A change that intentionally rewrites content (like the `## Comments` regeneration) must be scoped and
  tested so it touches only that content.

## Why

These tools edit users' decision records in place. A round-trip that silently drops an unknown section
or reorders frontmatter corrupts records the tool was trusted to manage. The round-trip test is the
guard; this ADR makes the guard a rule.
