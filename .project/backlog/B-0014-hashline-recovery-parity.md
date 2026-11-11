---
document:
  type: "backlog-item"
  id: "B-0014"
  title: "SPEC-0005 G5.2 lists donor recovery cases that are refused, not recovered"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "drift"
  priority: "P3"
  scope: [".project/specs/0005-hashline-editing.md", "internal/hashline/relocate.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

The G5.2 paragraph still describes body remapping, parse and sibling checks and context-matched duplicate ranges as behavior; a note below it says the implementation refuses those cases. The donor fixtures are not vendored, so parity is not checked mechanically.

# Done when

G5.2 is one statement of the behavior as built, or the donor cases are implemented with the donor fixtures pinned at a named revision.
