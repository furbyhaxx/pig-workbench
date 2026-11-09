---
document:
  type: "specification"
  id: "SPEC-0002"
  title: "Message frames and delivery API"
  description: "External frame producers, provenance, delivery, replay and presentation"
  status: "approved"
  revision: 10
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-09T15:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Define the public Workbench extension APIs before implementation"
      changes: ["Initial contract, lifecycle, examples and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes measured host delivery modes"
      changes: ["Mapped delivery values to host mechanisms with observed ordering", "Specified idle behaviour, nextTurn branch guard and compaction re-projection", "Replaced host gates with resolved findings and remaining upstream issues"]
    - revision: 3
      reason: "M0.1 donor parity audit"
      changes: ["Added donor parity rules for escaping, timestamps and sequences"]
    - revision: 4
      reason: "pig-web harmonisation and user decisions"
      changes: ["Added the message frame kind for inter-session messages"]
    - revision: 5
      reason: "SPEC-0012 project model and URI namespace"
      changes: ["Scheme guide frames"]
    - revision: 6
      reason: "Context frames and prompt diffs (SPEC-0014)"
      changes: ["Delivery beforePrompt: steer from before_agent_start, lands before the user prompt (P30)", "Context frames with refresh policies are a Workbench producer on this pipeline"]
    - revision: 7
      reason: "User approved owner-scoped fields on Workbench context frames"
      changes: ["Linked typed contextFields registration and update contract", "Kept internal source ownership distinct from external data contributions"]
    - revision: 8
      reason: "User approved the contract freeze on 05.11.2026"
      changes: ["approved for Slice 1 implementation; ledger, delivery and trust semantics frozen"]
    - revision: 9
      reason: "User directive 08.11.2026: frames must be able to wake an idle model, as an explicit flag"
      changes: ["Added publish field wake (default false), valid with nextBoundary only", "Removed the delivery value followUp: wake replaces it", "allowWake plus a per-producer hourly budget gate external wakes; Workbench's own sources wake by user setting", "Receipt reports wake outcome and duplicate publications", "Migration: a producer that sent delivery followUp now sends delivery nextBoundary with wake true; it was always denied before, so no working producer changes"]
    - revision: 10
      reason: "User report 09.11.2026: the nonce changed with every frame; it must be stable inside a session, its forks and branches"
      changes: ["Nonce is one 8-digit uppercase hexadecimal value per session lineage, persisted as the custom entry pig-workbench.nonce and shared by every frame of every producer and branch", "Forks and tree branches copy entries, so they inherit it", "Migration: records stored before this revision keep the per-frame nonce they were written with and still restore; frames admitted after upgrade use the lineage nonce"]

---

# 1. Purpose and dependency

Implement one attributed frame pipeline for Workbench and cooperating extensions. External producers use [SPEC-0001](0001-extension-services.md); they never insert custom system-role text directly. This draft specifies intended v1 behavior; nothing here is implemented yet. Donor references: `host:/mnt/Projects/furbyhaxx/pi-workbench/src/message-framing/`, its `src/instructions/`, and `host:/mnt/Projects/nukular/nukular-ai/crates/nukular-agent-sdk/src/frame/instruction_ledger.rs`.

Frames separate **facts**, **model delivery**, and **transcript presentation**. Hiding a row never removes instructions from context. Delivery to a model request does not prove the model read, obeyed or acted on it.

# 2. Producer registration and API

Register kind `frameProducer`:
| Field | Contract |
|---|---|
| key | Stable dotted source prefix owned by producer, e.g. build-tools.build |
| label, description | Plain text for inspection/settings |
| kinds | Allowed subset: instruction, reminder, warning, error, notice, info, message (`message` is reserved for Workbench's own `workbench.sessions` source, SPEC-0011) |
| defaultVisibility | shown, collapsed or hidden; user override wins |
| requestedAuthority | data or instruction; host/user policy grants actual authority |
| allowWake | Declares that the producer may publish with `wake: true`, default false. Declaration is the permission; the hourly budget (`frames.wake-budget`) bounds it. |
| contextFields | Optional typed field claims on Workbench context sources; initially `session.model.knowledge_cutoff` only (SPEC-0014 §5.1) |

External sources default to data authority. Requesting instruction authority is not self-authorization. Trusted project data and arbitrary command output are not inherently harness instructions. Producer prefixes cannot impersonate Workbench internal sources. Declared `contextFields` are a separate, typed data contribution consumed by Workbench's producer (SPEC-0014 §5.1); they do not grant permission to publish with an internal source name.

Service methods:
- `frames.publish`: registrationId + publish request → durable admission receipt.
- `frames.status`: frameId → admitted/pending/delivered/cancelled/failed with reason.
- `frames.cancel`: frameId → cancel only an undelivered frame owned by caller.
- `frames.list`: branch scope + filters/cursor → bounded records.
- `frames.get`: frameId → stored record, bounded body page and delivery state.
- `context.fields.set`: registrationId + model-bound typed contribution → acceptance (SPEC-0014 §5.1); it does not itself acknowledge frame delivery.

Workbench's own source `workbench.guide.<scheme>` delivers first-touch scheme guides (SPEC-0012 §5.5) as kind `instruction`. A guide contributed by another extension keeps data authority unless the user trusts that provider.

A producer cannot edit an already delivered frame. Updates, corrections and withdrawals are new records linked to the earlier subject/version.

# 3. Publish contract

Required: `eventKey,kind,source,body`. Optional fields:
| Field | Meaning |
|---|---|
| title, summary | Display labels; never substitutes for model body |
| resource | Canonical resource URI concerned |
| subjectKey | Stable semantic entity under source, e.g. build/412 |
| scope | session, or explicit branch binding; default current branch |
| delivery | nextBoundary (default), nextPrompt, beforePrompt |
| wake | Boolean, default false. `true` asks Workbench to start a model turn for this frame when the session is idle. Valid only with `nextBoundary`: a frame held for the next prompt cannot also start a turn now. |
| coalesceKey | Only pending replaceable notices; same source/subject/scope |
| dataTrust | trusted or untrusted; actual trust is policy-constrained |
| expiresAt | Optional pending-notice expiry; never silently expires instruction changes |
| instruction | Structured replacement/diff/withdrawal metadata for granted instruction producers |

EventKey deduplicates within (session, producer key, eventKey). Same key/same payload returns the existing receipt; same key/different payload is conflict. Dedup survives ordinary reload via persisted records. Fork scope is explicit: replayed facts are not re-published as new events.

## Host delivery mapping (observed, [probes](../analyses/2026-11-02-host-integration-probes/README.md) P5, P19–P21, P30)

| Delivery | Host mechanism | Running | Idle |
|---|---|---|---|
| nextBoundary | `SendMessage(customType, body, display, {DeliverAs: "steer", TriggerTurn: false})` | Lands after the current tool batch, before the next assistant call; persisted as `role: custom` | Persisted immediately; seen with the next prompt; no turn |
| nextPrompt | Workbench-held queue, returned as `{"message": {customType, content, display}}` from the next `before_agent_start` | Same as idle | Lands directly after the next user prompt and is persisted (P30). A `steer` sent from that hook would land *before* the prompt instead |
| beforePrompt | Workbench-held queue, sent as `SendMessage(steer)` from the next `before_agent_start` | Same as idle | Lands directly before the next user prompt and is persisted (P30); used by context frames (SPEC-0014 §5) |
| nextBoundary + `wake: true` | Idle: `SendMessage(..., {DeliverAs: "followUp", TriggerTurn: true})`. Running: as nextBoundary | Lands at the next boundary; the running turn already consumes it | Starts a turn |

Workbench does not use host `nextTurn`: its in-memory queue survives tree navigation and is delivered on the new branch (P20), which violates the branch guard below. Workbench always sets `TriggerTurn` explicitly and validates `DeliverAs` itself; the host accepts unknown values silently (P19). Custom messages reach the model as user-role content; stored history keeps `role: custom` and `customType` for replay.

Body is plain text, maximum 32 KiB per frame by default inside the 64 KiB control envelope; oversized publications fail limitExceeded. Producers may provide a resource continuation URI and an explicitly partial body; Workbench must not silently cut an instruction or present partial input as a full baseline.

```json
{"v":1,"id":"publish-1","sessionId":"s-1","generation":"g-1","from":{"owner":"build-tools","instance":"i-1"},"method":"frames.publish","registrationId":"reg-1","params":{"eventKey":"build-412-finished","kind":"warning","source":"build-tools.build","subjectKey":"build/412","title":"Build 412 failed","body":"Build exited with code 1. Inspect build://412/log for compiler diagnostics.","resource":"build://412/log","dataTrust":"untrusted","delivery":"nextBoundary"}}
```

```json
{"v":1,"id":"publish-1","sessionId":"s-1","generation":"g-1","to":{"owner":"build-tools","instance":"i-1"},"ok":true,"result":{"frameId":"f-12","sequence":12,"state":"pending","delivery":"nextBoundary","effectiveAuthority":"data"}}
```

Ordinary publish (`wake` absent or false) is a fact-delivery request: it never starts a turn. `wake: true` is honored when the session is idle; a running session already receives the frame at its next boundary, so no second turn starts. A producer without `allowWake` fails `permissionDenied` (outcome `notApplied`) and may retry with `wake: false`. A producer over its hourly `frames.wake-budget` is not refused: the frame is delivered as an ordinary `nextBoundary` frame and the receipt says `wake: "suppressed"`. The receipt's `wake` is `woken` (turn started), `steered` (session was running) or `suppressed`; it is absent when `wake` was not requested. A duplicate publication returns the stored receipt with `duplicate: true` and performs no delivery and no wake. Workbench's own sources wake by user policy, not by `allowWake`: finished background jobs wake the model when `jobs.wake-on-completion` is true (default). Waking is never an escalation of authority: a woken frame keeps its data authority and trust.

# 4. Stored frame and delivery ledger

Versioned stored record:
- schemaVersion, frameId, sequence, sessionId, original branch leaf/generation, createdAt.
- producer owner/key and source; kind, body, title/summary, subjectKey/resource.
- requested/effective authority and trust, eventKey, content digest.
- requested delivery, scope, coalescing/expiry policy.
- instruction metadata if relevant.
- display defaults and retained provenance sufficient for replay without producer.

Use canonical custom type `pig-workbench.frame` for new records; no donor-session migration is required. Sequence is allocated monotonically within the session and persisted; branch replay can contain gaps. frameId is globally unique enough to avoid sibling-branch collision. New forks retain provenance of replayed frames and allocate new IDs for new facts.

Delivery records separately identify frameId, branch/context epoch, state and request association when the host exposes it. Do not conflate appendEntry success with actual context inclusion.

```text
validated → persisted pending → selected for context → delivered
                      │                 │
                      ├→ cancelled      └→ failed/retry pending
                      └→ superseded (only eligible pending notices)
```

Admission acknowledgement is returned only after persistence succeeds (or in-memory session store accepts it in no-session mode, labeled non-durable). Delivery acknowledgement means included by Workbench in a specific outgoing context, not provider success. If the host cannot expose that boundary, report `queuedToHost` instead of claiming delivered; capability negotiation must disclose this limitation. Observed boundary: the `context` hook runs for every outgoing provider request (not for compaction summary requests, P21), so Workbench records `delivered` when a frame is present in the message list it returns from that hook. Provider receipt is never confirmed.

When idle, an ordinary nextBoundary frame waits for the next prompt; only `wake: true` starts a turn (§3). During a run it uses the next safe context boundary after the current tool batch. Branch-scoped publications are cancelled/not delivered to a different branch after navigation. Session-scoped notices remain pending but must be re-evaluated for applicability. A stale producer binding gets staleGeneration.

Coalesce only undelivered info/notice updates explicitly sharing a key. Preserve all errors and instruction transitions or compute a verifiably equivalent latest baseline. Queue overflow returns busy; do not silently drop frames. Default 256 pending frames/session and 2 MiB aggregate body memory, backed by bounded durable records.

# 5. Instruction changes, caps and compaction

Instruction metadata:
`classId,canonicalUri,operation,baseVersion?,version,contentDigest,viewDigest,complete,continuation?`.
Operation is baseline, changed, withdrawn, removed, relocated or selfEdit. Changed records retain the full replacement delivered view for reconstruction; model projection may use a unified diff with explicit base/new version. Relocation references an existing same-epoch/equal-trust content frame. It is not a path-only assertion.

The instruction service is responsible for discovery, file classes, ancestor/root+nearest selection and pinning; the frame API transports its decisions. Other producers may supply instructions only under granted source policy and the same base-version checks. No arbitrary external “remove all instructions” operation.

For capped BACKLOG.md or custom views, diff **previous delivered view → new delivered view**. Source version and view version are distinct. Changes outside the cap update hidden-count/version facts without manufacturing unseen hunks. Expanding a UI row is not the same as injecting the full source.

Ephemeral projections (instruction views, pinned ambient context) are appended by the `context` hook on every request; the host does not persist them, and after compaction the hook sees `compactionSummary` plus kept messages (P21). On compaction, the epoch advances and active instruction producers/retained baselines reconstruct full current views. Do not replay every historic diff or rely on a baseline removed from context. Source unregister does not erase history: pending instructions are cancelled, active contributions are withdrawn through a new frame before being removed from the active set. If the host cannot deliver that withdrawal immediately, it remains a pending consistency action.

# 6. Projection and trust

Model envelope retains donor-readable shape:
```text
<system-warning src="build-tools.build" seq="12" n="A1B2C3D4" ts="2026-11-02T16:00:47+01:00" trust="untrusted">
Build exited with code 1. Inspect build://412/log for compiler diagnostics.
</system-warning>
```

Escape attributes and matching closing delimiters in body. Persist nonce/createdAt, never regenerate timestamps on replay. The nonce is one value per session lineage (session, forks and branches), shared by all frames and producers, persisted as the custom entry `pig-workbench.nonce`; it is never derived per frame. Nonce is a provenance cue, not authentication; all copied markup in user/tool content remains data.

Default external projection is framed data via the host-supported custom-message route. Native system/developer projection requires a verified capability and granted instruction authority; unsupported providers retain explicit envelope semantics without invented role support. Never change stored history roles to fit one provider. Preserve provider message-order constraints, including trailing assistant/tool sequences. Prompt wording must not tell the model that external data has higher authority than the user's instructions.

# 7. Presentation and extension settings

Registered producers appear under `/workbench frames` and `/workbench settings frames`, grouped by owner/source. Settings: density/style (row/rail/card), default visibility, per-kind/per-source overrides, source wake policy where user-authorized. Rendering uses stable stored data; source absence is an inspection detail, not a replay failure.

Each row shows kind, subject/title, primary change/result and expand hint if hidden. Inspector shows authority, source, resource, sequence/time, delivery state, base/new versions and full bounded body. Visibility changes are explicitly transcript-only; disabling delivery is a distinct policy action requiring explanation.

Wireframes and action/state behavior: [SKETCH-0002](../sketches/extension-apis/README.md). No UI callback may call the producer during rendering.

# 8. Acceptance scenarios

| Scenario | Required result |
|---|---|
| Producer loads before/after Workbench | Discovers and registers without order dependence |
| Duplicate publish / changed duplicate | Existing receipt / conflict, never two deliveries |
| Publish error then retry | No acknowledged-but-lost frame |
| Idle ordinary frame / wake without `allowWake` | Waits for prompt / explicit permission error |
| Idle frame with `wake: true` | A turn starts and the model sees the frame once |
| Duplicate publication with `wake: true` | `duplicate: true`, no second delivery, no second turn |
| `wake: true` with nextPrompt or beforePrompt | `invalidRequest` |
| Wake over budget | Delivered without a turn, `wake: "suppressed"` |
| Branch switch during pending publication | No delivery to wrong branch |
| Observer misses delivery event | frames.status reconciles state |
| Compaction after diff | Current baseline reconstructed, not orphan diff |
| Capped backlog reorder/change below cap | Reconstructible view diff and accurate omission notice |
| External untrusted text includes tags/escapes | No authority escalation or terminal escape execution |
| Source unload/reload | Retained history renderable; active instructions reconciled |
| Host lacks outgoing-request acknowledgement | queuedToHost visible; no false delivered state |
| Multiple simultaneous producers | Stable sequence, bounded queue, no starvation by progress spam |

Host findings: inclusion acknowledgement via the `context` hook, custom-role persistence and branch behaviour are probed (see mapping above). Native system/developer role projection remains unverified and stays unadvertised. Upstream issue: queued `nextTurn` crosses branches (P20).

# 9. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), nukular frame-ledger tests:
- G2.1 Body and attribute escaping is injective: every hostile body (delimiters, nonce-like strings, whitespace runs) round-trips byte-exact through render → parse. Timestamps carry an explicit offset and may carry fractional seconds; invalid calendar dates and offsets are rejected. Concurrent sequence reservations share one strictly positive counter, and an exhausted counter fails; it never wraps or reuses a number. Untrusted frames always project as user-role content and are marked. Each wire kind has a golden fixture.
