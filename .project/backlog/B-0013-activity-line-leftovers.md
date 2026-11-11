---
document:
  type: "backlog-item"
  id: "B-0013"
  title: "Activity line still reads the endpoint registry every 5 s"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "debt"
  priority: "P3"
  scope: ["internal/host/activity.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

Publishing is event-driven except the peer count: other processes write the endpoint registry and nothing watches it, so a 5 s tick reads it. `Retrying n/m` is not shown because the SDK has no retry event. Working, Compacting, held instruction files and the away location were not seen in a live capture.

# Done when

The registry directory is watched or the tick is justified in SPEC-0009 §9; the four states are captured in a real TUI.
