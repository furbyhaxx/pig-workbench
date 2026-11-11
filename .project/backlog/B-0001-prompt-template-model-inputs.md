---
document:
  type: "backlog-item"
  id: "B-0001"
  title: "Prompt templates get no model family, api or thinking inputs"
  description: "Left open by the Slice 1 remediation (PLAN-0005)"
  status: "open"
  revision: 1
  created_at: "2026-11-11T01:35:00+01:00"
  updated_at: "2026-11-11T01:35:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  kind: "feature"
  priority: "P2"
  scope: ["internal/host/prompt.go", "internal/prompt/**"]
  changelog:
    - revision: 1
      reason: "Recorded at the end of PLAN-0005 area M"
      changes: ["Created"]
---

# Current

`promptData` (`internal/host/prompt.go`) does not set `model.family`, `api`, `thinking`, `mid_convo_system` or `prompt.families`, so a template test such as `model is family(...)` never matches and `/workbench prompt` cannot show which family entry matched. SPEC-0014 §3.4 and §3.5 assign these inputs to Slice 1 (PLAN-0002 J); no plan step carries them.

# Done when

The inputs are set from the active model and the `prompt.families` table; a template conditional on a family renders differently for two models in a test; `/workbench prompt` names the matched family.
