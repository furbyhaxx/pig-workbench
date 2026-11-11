---
document:
  type: "backlog-item"
  id: "B-0010"
  title: "`jobs` list arguments, row bounds and notice title"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "drift"
  priority: "P3"
  scope: ["internal/host/jobs*.go", "internal/jobs/**"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

Left from the typed-envelope change: `jobs list` has no `maxFinished` or `cursor` argument; a row echoes the full command text however long; the completion notice names the command, not the `title`; an id-less `wait` does not watch jobs started after it began and has no default timeout (it blocks until exit or user cancel); the `unknown` state is tested at manager level only.

# Done when

Rows cut the command at a stated length; the notice shows the title when one was given; the wait default is decided and written in SPEC-0008 §6; list paging is added or removed from the spec table.
