---
document:
  type: "backlog-item"
  id: "B-0016"
  title: "Skill catalog re-reads every SKILL.md per list and read; replay order"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "debt"
  priority: "P3"
  scope: ["internal/skills/**", "internal/host/skill*.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

The catalog reads every `SKILL.md` on each list or read with no cache. The frontmatter parser is a hand-written subset, not YAML. Replay after compaction does not order explicit loads before auto-loads (matters above 20 loaded skills). A description-only edit of a listed skill emits no frame. `/tools` completions are empty until a tools view ran once.

# Done when

The catalog is cached and invalidated on file change; replay order follows SPEC-0014 §8.5 with a test.
