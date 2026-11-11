---
document:
  type: "backlog-item"
  id: "B-0005"
  title: "`!` commands run in PiG's host cwd, not the Workbench location"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "bug"
  priority: "P2"
  scope: ["internal/host/secrets_hooks.go", "internal/host/schemecommand.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

After `cd`, a `!` command typed in the editor still runs in the directory PiG was started in, while the `bash` tool runs in the Workbench location. The behavior was never reviewed against SPEC-0008 §2.

# Done when

Decided and tested: either `!` follows the Workbench location, or SPEC-0008 says it does not and the activity line's away item makes the difference visible.
