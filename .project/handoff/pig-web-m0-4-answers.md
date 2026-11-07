---
document:
  type: "handoff"
  id: "HO-pig-web-m0-4-answers"
  title: "Workbench answers to pig-web's M0.4 inputs (M1–M6)"
  description: "Current service and safety answers for pig-web, with seam evidence and open Slice 1 gates"
  status: "open"
  revision: 4
  created_at: "2026-11-05T17:25:00+01:00"
  updated_at: "2026-11-07T16:32:35+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Answer pig-web's M1–M6 before it corrects its specs against the freeze"
      changes: ["Answered M1–M6 with probe evidence", "Named the three contracts pig-web must not assume", "Recorded the one open coverage gap (native custom resolver)"]
    - revision: 2
      reason: "Replace obsolete pre-implementation answers with current observed behavior"
      changes: ["Documented conditional secrets and snapshots services", "Distinguished the reference seam gate from the downstream co-load", "Named outstanding freeze and acceptance blockers"]
    - revision: 3
      reason: "User approved the five remaining specifications"
      changes: ["Recorded contract approval without claiming implementation or slice exit"]
    - revision: 4
      reason: "Wire declared provider mutations without new permission prompts"
      changes: ["Updated search and mutation support for consumers", "Linked the current full baseline evidence and retained the live-provider gate"]
---

# Status

Workbench's reference-extension seam gate has passed with a real PiG host and a local scripted provider; see `TestLiveReferenceProducerSeams` and the [acceptance evidence](../analyses/2026-11-05-slice-1-inventory/evidence/acceptance-subset.txt). This is not a pig-web co-load test. The actual two-extension co-load belongs to pig-web's own gate.

**There is no Slice 1 exit commit yet.** All fourteen specifications are approved, but several acceptance behaviors are incomplete and the live-provider evaluation awaits user authorization. See the [current inventory](../analyses/2026-11-05-slice-1-inventory/README.md). Use the eventual slice-exit handoff and commit when they exist; do not interpret this checkpoint as that handoff.

# Answers

| Ask | Current answer |
|---|---|
| **M1: extension inference and secrets** | `secrets.redact` is advertised by `discover` only to an owner explicitly listed in `secrets.redact-owners`; it returns redacted text or fails without echoing input. The owner name is a claim on a shared bus, not authenticated extension identity. An extension making its own inference must redact every model-visible field and withhold on failure. Child sessions run without extensions have no automatic Workbench boundary. See [extension services](../../docs/extension-services.md). |
| **M2: contributed model fields** | Register a `frameProducer` with `session.model` / `knowledge_cutoff` and set it through `context.fields.set`. The field binds to the current model and becomes stale on model change. `TestLiveReferenceProducerSeams` exercises the path. |
| **M3: descriptor summary** | The user approved SPEC-0004, including the required one-line `resourceProvider.summary` of 1–120 characters. It is enforced at registration. |
| **M4: `!command` credentials** | `TestProbeNativeCredentialExpansion` observed literal, `$ENV` and `!command` expansion for `models.json` provider configuration. A custom `RegisterNativeProvider` resolver is host-internal and not observable through this extension; do not assume that path is covered. |
| **M5: trailing annotations** | `TestAnnotationOrderCannotChangeSeenVisibility` verifies that an intact snapshot block grants seen regardless of annotation order; altering the block grants nothing. The shared tool-result hook redacts known secrets, including appended text, but extensions should protect their own model requests and must not rely on unrecognized secrets being detected. |
| **M6: activity items** | Contributed `statusSegment` items can target activity or status placement; updates are coalesced and fitted. Activity click needs a fork-host change and is not available. The fixed `alt+w` shortcut inspects activity; the configurable shortcut is not applied. |

# Consumer limits

- `snapshots.view`, `snapshots.status` and `snapshots.touched` are advertised; editable snapshot views currently support local files only.
- Contributed providers support bounded `read`, `ls`, `grep` and `find`. Providers can declare write/mkdir/remove/copy/move with stable revisions, compare-and-swap, a retention/durability declaration and typed receipts (SPEC-0004 rev 8). Workbench adds no new confirmation or permission prompts; providers enforce backing rules and commit-time preconditions. Provider `edit` stays refused until its snapshot backend exists. See [mutation authoring](../../docs/extension-services.md#provider-mutation-outcomes).
- A shared-bus timeout cannot preempt an uncooperative listener. A mutating call with unknown outcome must be reconciled, not blindly retried.
- Windows messaging, session storage and registry have compile-level coverage only. Activity click and the rich questionnaire panel are later host/UI work.

At implementation `eb55bf7`, fresh full race-enabled unit and actual PiG integration runs passed. The exact `just check`, `just race` and `just integration` gates passed at `733298a`; [current evidence](../analyses/2026-11-05-slice-1-inventory/README.md) includes all ownership rows and explicit remaining limits. No billed inference was used.

# Next handoff

Once implementation and acceptance are complete, produce a slice-exit handoff naming its actual commit and reference-gate evidence. pig-web can then run its own real co-load gate against that commit.
