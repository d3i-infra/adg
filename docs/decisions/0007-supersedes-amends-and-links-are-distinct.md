---
status: accepted
date: "2026-06-29"
category: Decision model
source: the MADR-era design records, removed 2026-09 (git history before v4.0.0, docs/fork-design/)
priority: default
applies_to:
    - internal/domain/decision/lean/validate.go
---

# Supersedes, amends, and links are distinct relationships

## Decision

`supersedes`, `amends`, and `links` are three distinct relationships with different precedence, status,
and force semantics — not interchangeable labels. A relationship-bearing change must use the one that
matches the author's actual intent.

## Guidance

- Pick by intent and never overload one for another: **supersede** when a new record replaces and
  retires an old one (bidirectional — the old record's status flips to "superseded by ADR-NNNN");
  **amend** for an additive correction that leaves the base record in force; **link** for a loose
  cross-reference with no precedence, status, or force.
- Keep both ends consistent: `lean/validate.go` verifies forward + reverse integrity for supersedes and
  amends, so a relationship-bearing change must keep both ends in sync.
- `links` must not carry supersede/amend semantics — a cross-reference that should flip status or
  force is one of the other two relationships, not a link.

## Why

The three relationships carry different status, precedence, and force: a supersede flips the old record's
status and retires it from routing; an amend leaves the base in force; a link changes nothing. Overloading
one for another silently loses those semantics — the file still looks valid while the model's integrity
quietly breaks — so the distinction has to be enforced, not left to the author's memory.
