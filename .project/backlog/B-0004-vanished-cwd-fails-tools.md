---
document:
  type: "backlog-item"
  id: "B-0004"
  title: "A deleted working directory fails every tool in secrets detection"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "bug"
  priority: "P2"
  scope: ["internal/host/secrets_env.go", "internal/host/secrets_hooks.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

When the session's working directory no longer exists, every tool call, `jobs list` included, fails before it runs with `resolve detection directory: lstat ...`.

# Done when

Detection skips an unreadable directory and the call proceeds; a test removes the directory and runs `jobs list` and `cd`.
