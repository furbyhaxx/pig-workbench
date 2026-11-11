---
document:
  type: "backlog-item"
  id: "B-0012"
  title: "Tool-row snapshots: duplicated bash tail, unflagged cut diffs, write row"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "debt"
  priority: "P3"
  scope: ["internal/host/presentation_builtin.go", "internal/host/presentation_render.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

The bash snapshot repeats up to 4 KiB of output inside Details of every result (session size). A cut diff is not flagged as truncated. A `write` row shows `diff create` without hunk lines. Call rows carry no glyph, so call and result labels are offset by two columns. Running and cancelled rows were never captured live. Density is PLAN-0004.

# Done when

The tail is referenced, not copied; a cut diff shows the incomplete marker; the user reviewed the write and call rows in captures.
