---
document:
  type: "backlog-item"
  id: "B-0011"
  title: "Referenced env files are missed when the shell builds the path"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "drift"
  priority: "P3"
  scope: ["internal/host/secrets_env.go"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

An env file is learned before a call only when the call names it statically. A path built at run time (variable, glob, `cd` then a relative name) is not seen; a recursive search learns only the `.env*` files directly inside the named directory; a value under a name outside the `[env] names` globs still shows unless it has a token shape.

# Done when

SPEC-0013 §2 states these limits, or bash output is checked against env files found in directories the command touched.
