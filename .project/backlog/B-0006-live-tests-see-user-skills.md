---
document:
  type: "backlog-item"
  id: "B-0006"
  title: "Live tests load the machine's own skills and flake on start order"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "debt"
  priority: "P2"
  scope: ["internal/host/live_*_test.go", "internal/host/testharness*.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

Live fixtures do not isolate `HOME`, so PiG discovers `~/.agents/skills` in every live test and listings, panel row order and frame counts depend on the machine. Two flakes were seen once each and not reproduced: `TestLiveSelfEditedInstructionsFollowTheSelfEditsSetting` (candidate: the window between commit and `recordAuthored` in `write.go` and `edit.go`) and `TestLiveMessageBeyondMaxHopsIsQueuedWithoutWaking` (a peer socket dialled before it existed).

# Done when

Live sessions run with an isolated home; both tests pass 200 consecutive runs under `-race` or their cause is fixed.
