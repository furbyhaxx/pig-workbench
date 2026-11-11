---
document:
  type: "backlog-item"
  id: "B-0008"
  title: "Instruction views miss withdrawals, non-project rules and the silent baseline"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "bug"
  priority: "P3"
  scope: ["internal/host/instruction*.go", "internal/instructions/**"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

Three gaps: a withdrawn source stays listed as delivered in `session://self/instructions/` (a silent withdrawal leaves no ledger record); `/workbench instructions rules` shows only the project-boundary selection; with `instructions.updates.self-edits = silent` the baseline advances only in memory, so after a resume the first change is diffed against the older stored view.

# Done when

Each of the three has a test: a withdrawn file is no longer listed, rules lists every class, and a silent self-edit followed by resume and an external change yields a diff against the self-edited text.
