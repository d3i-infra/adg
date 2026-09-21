---
status: accepted
date: "2026-06-29"
category: Decision model
source: the MADR-era design records, removed 2026-09 (git history before v4.0.0, docs/fork-design/)
priority: invariant
applies_to:
    - internal/domain/decision/lean/load.go
    - internal/domain/decision/lean/index.go
    - internal/adapter/command/lean/index.go
---

# ADR files are the only source of truth — no index or cache

## Decision

The ADR markdown files are the single source of truth. The model is read by scanning the files on every
read; there is no `index.yaml`, sidecar database, or persistent cache that could drift from the files.
The generated README index is a pure projection of the files — derived output, never an input.

## Guidance

- Read the model by scanning files (`LoadDir`); never introduce a cache or index file that the tool
  then trusts as authoritative.
- The generated README (`RenderIndex` / `adg lean index --write`) is regenerated from frontmatter and is
  never read back as state — treat it as build output, safe to delete and recreate.
- An optimization that caches parsed records must be in-memory and per-invocation, not persisted as a
  source of truth.

## Why

Any second store the tool trusts as authoritative — an index, a sidecar DB, a persisted cache — can drift
from the markdown, and then the guidance an agent sees stops matching what is on disk. Reading the files
on every read is what keeps advisory routing and enforcement honest about the same bytes; the generated
README is a projection, not an input, precisely so nothing downstream can depend on stale state.
