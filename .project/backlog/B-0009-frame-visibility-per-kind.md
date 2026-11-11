---
document:
  type: "backlog-item"
  id: "B-0009"
  title: "Per-kind frame visibility overrides are not implemented"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "drift"
  priority: "P3"
  scope: ["internal/host/frame_rows.go", "internal/config/catalog.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

SPEC-0002 §7 describes visibility overrides per frame kind and per source. Only per-source visibility exists; the catalog has no per-kind keys. SPEC-0014 §5 and §10 also name `context.frames.<source>` switches that the catalog does not have.

# Done when

Either the keys exist with a renderer test, or both spec sections are rewritten to the per-source setting as built.
