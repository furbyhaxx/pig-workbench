---
document:
  type: "backlog-item"
  id: "B-0007"
  title: "staticcheck is not part of `just check`"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "chore"
  priority: "P3"
  scope: ["Justfile"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

staticcheck was run by hand from a temporary copy because it rejects `-modfile` with `GOWORK=off`; unused code can return unnoticed. Two notes remain in test files (SA4000 in `prompt_test.go`, deliberate; S1003 in `instructions_test.go`).

# Done when

A `just lint` recipe runs staticcheck against the staged SDK and `just check` calls it.
