---
document:
  type: "backlog-item"
  id: "B-0003"
  title: "Frame ledger metadata grows without bound and is marshalled whole per publish"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "debt"
  priority: "P2"
  scope: ["internal/frames/**", "internal/host/frame_context.go", "internal/host/contributed_frames.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

Bodies of delivered frames are dropped past the history cap, but records, events and the id index are kept for the whole session. `Admit` and `SelectPending` scan all records, several host paths marshal the full snapshot on each publish or command, and restore parses every frame entry at session start. None of it is measured. `frames.get` on a record whose body was dropped has no test, and SPEC-0002 §4 does not state the history bound. No real session file above 2 MiB was resumed end to end.

# Done when

A benchmark over a 10 000-frame session shows bounded cost per publish and per restore, or the metadata is bounded; SPEC-0002 §4 states the bound; a resume test loads an over-limit session file.
