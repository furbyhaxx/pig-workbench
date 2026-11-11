---
document:
  type: "backlog-item"
  id: "B-0015"
  title: "Tool schemas: nested combinators unaudited, xAI not verified live"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "chore"
  priority: "P3"
  scope: ["internal/host/*.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

The root of every tool schema is a plain object and a live test enforces it. Nested property schemas were not audited for `oneOf`/`anyOf`, and the fix was not run against the live xAI API.

# Done when

The test walks nested schemas; one real request to an xAI model with all ten tools succeeds.
