---
document:
  type: "backlog-item"
  id: "B-0002"
  title: "First-touch guides reach the model only for contributed schemes on read"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "drift"
  priority: "P2"
  scope: ["internal/host/tools.go", "internal/host/guides*.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

SPEC-0012 §5.5 wants a scheme's guide delivered as a frame on the first `ls`, `read`, `find`, `grep` or `write` of that scheme, builtin schemes included. Today only contributed schemes have a guide, only `read` triggers it, and it is appended to the tool result. Needs a user decision first: builtin guides add model context to every session that touches a scheme.

# Done when

The user decided; either the spec section is rewritten to the behavior as built, or guides arrive as one frame per scheme on first touch through any of the five tools, with a live test.
