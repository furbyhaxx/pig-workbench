---
document:
  type: "handoff"
  id: "HO-pig-web-m0-4-answers"
  title: "Workbench answers to pig-web's M0.4 inputs (M1–M6)"
  description: "Frozen-contract answers for extension inference, contributed model fields, descriptor summaries, !command credentials, trailing annotations and activity items, with the probe evidence behind each"
  status: "open"
  revision: 1
  created_at: "2026-11-05T17:25:00+01:00"
  updated_at: "2026-11-05T17:25:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Answer pig-web's M1–M6 before it corrects its specs against the freeze"
      changes: ["Answered M1–M6 with probe evidence", "Named the three contracts pig-web must not assume", "Recorded the one open coverage gap (native custom resolver)"]
---

# Answers

Answered against pig-workbench `5b4ebe9` (PLAN-0002 rev 13, probe analysis
[ANA-2026-11-05-slice-1-probes](../../analyses/2026-11-05-slice-1-probes/README.md)).
The specification freeze itself is awaiting the user's approval; these answers are the contracts as they
stand and will not change shape at approval. Re-read the specs at the commit named in the freeze handoff.

| Ask | Answer |
|---|---|
| **M1** extension inference and secrets | **Option A survives as the only implemented seam**: opt-in `secrets.redact`, refused today because the `secrets` capability is not advertised. Requests carry placeholders only; plaintext never travels on the bus. Your preference for a fork hook (B) is unchanged as a *fork* feature and is not part of Slice 1. Research child sessions started with `NoExtensions:true` get **no** automatic boundary: your owner must redact each request or run them with Workbench selected. Do not rely on C (sharing the detector) — it is not published. |
| **M2** `knowledge_cutoff` in `session.model` | **Accepted as implemented.** Register a `frameProducer` with `contextFields: [{source:"session.model", field:"knowledge_cutoff"}]` and set it through `context.fields.set`. The value is bound to the current model: a model change makes it stale and hides it until you set it again. Shape: `{date:"YYYY-MM-DD", basis:"explicit" or "estimated", source:"..."}`. Verified end to end in `TestLiveReferenceProducerSeams`. |
| **M3** descriptor summary | **Accepted as specified, still pending user sign-off at the freeze**: `resourceProvider` descriptors carry a required one-line `summary` of at most 120 Unicode characters, rendered in the `session.capabilities` frame line for registered schemes. Nothing else in the descriptor may be used for that line. |
| **M4** `!command` credentials | **Qualified exactly as you asked.** Proven for `models.json` provider config: a literal, `$ENV` and a leading `!command` all resolve, and the wire carries the resolved value (`TestProbeNativeCredentialExpansion`). **`RegisterNativeProvider` with a custom resolver is not reachable from an extension**, so Workbench cannot observe it; SPEC-0013 §2 records it as a coverage gap, not as detected. If you register that way, treat literal `!command` strings as outside the inventory. |
| **M5** trailing annotations | **Tolerated, and order does not matter.** An intact snapshot block grants seen whether your annotation block is appended before or after it; a block altered in place grants nothing (`TestAnnotationOrderCannotChangeSeenVisibility`). Only a verbatim block counts, so an annotation can never add seen lines. Your block must be redacted by you before you return it (SPEC-0013 §4.2); do not expect Workbench to patch another extension's appended block in Slice 1 — the redaction order contract for appended blocks is Phase I and is **not** yet implemented. |
| **M6** activity items | **Confirmed.** `statusSegment` with `placement:"activity"` or `"status"`, `glyph` (one cell), `priority`, `minColumns`/`maxColumns`, and `open` accepting **either** a slash command or a resource URI. Updates are coalesced to four per second and the segment is fitted to the terminal width; you must not call `SetStatus` yourself. Click behaviour is a fork feature (PLAN-0004 C4), not Slice 1. |

# Three assumptions you must not make

1. `secrets.redact` is **not** advertised yet. `discover` with `capabilities:["secrets"]` fails `unsupported`.
2. `snapshots` is **not** advertised yet, so `snapshots.view` will fail `unsupported` in this slice even though
   the edit-tracking contract is frozen.
3. Another extension's appended tool-result block is **not** redacted by Workbench in Slice 1. Redact your own
   annotation text before returning it.

# What to do next

Correct your specs against these six answers, then build on the Workbench commit named in the slice-exit
handoff. The real two-extension co-load run belongs to your own gate; Workbench hands off its seam evidence
instead of claiming that run.
