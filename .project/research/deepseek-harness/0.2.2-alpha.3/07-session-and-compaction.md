---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-07-session-and-compaction"
  title: "DeepSeek Harness 0.2.2-alpha.3 session and compaction"
  description: "Source-derived session log, surface, persistence, compaction, spill and orchestration"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:02+01:00"
  updated_at: "2026-11-03T11:54:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document source-observed session and orchestration mechanics"]
---

# Session and compaction

[OBS] The source snapshot is `/tmp/re-recon/dsh` at commit `8bd073392eef6f54e921a093b95076248eb08cbb`; citations are relative to that checkout. No target runtime was installed or executed.

## Log and model surface

[OBS] A Session has an immutable header and an append-only event log; persistence is a plugin subscribing to `session/event` and `session/flush`. The current logical `SESSION_FORMAT_VERSION` is 4. Header fields include `id`, `createdAt`, optional `cwd`, `parentSession`, `origin`, `delegationDepth`, `agentPreset`, and `isSeeded`. A fork carries a cut marked by `session/end-seed`; parent identity alone does not establish subagent origin (`packages/core/session/src/index.ts:1`, `:44`; `packages/core/session/src/types.ts:62`, `:91`, `:435`).

[OBS] A separate derived surface retains ordered event sequence numbers; eligible `system/message`, `developer/message`, `user/message`, `assistant/message` and `tool/result` events can append or replace a surface span while the original log remains intact. Empty system/developer/assistant content produces no provider message; ordinary user content retains its producer-owned framing. `request/header` carries route configuration and tool schemas rather than system text (`packages/core/session/src/surface.ts:1`, `:50`, `:106`; `packages/core/session/src/types.ts:241`, `:439`).

[OBS] `SystemPromptProjection` writes the rendered system prompt as a system message, replacing old nodes or appending under its series/capability rules. `RuntimeContextProjection` emits a user message only when the retained snapshot changes, including a clearing notice when the current context becomes empty (`packages/core/agent-loop/src/runtime-context.ts:68`, `:91`, `:119`, `:155`). See `04-instructions-and-injections.md` for the message templates.

## Persistence

[OBS] The JSONL backend requires a `root`; it groups by project and encoded session id. The first record is a `type: 'session'` header followed by contiguous events. Physical compression options are `zstd` and `none`, with source default `zstd`; the extension is `.jsonl.zstd` or `.jsonl`. The versioned generation filename comes from the session-format helper. IDs are injectively encoded as safe path components (`packages/session/session-persistence-jsonl/src/index.ts:1`, `:68`, `:90`; `packages/session/session-persistence-jsonl/src/format.ts:35`, `:58`, `:80`, `:190`, `:254`, `:267`, `:313`).

[OBS] Historical formats are prepared through an adjacent migration chain; source headers and generations remain separate from the published current generation. Header-only versus event-body reads have different admission responsibilities. Source code distinguishes a prepared historical log from an already durable current one (`packages/core/session/src/types.ts:62`; `packages/session/session-persistence-jsonl/src/index.ts:126`; `packages/session/session-persistence-jsonl/src/generation.ts:1`). [UNK] Exact crash-tail recovery, locking and all generation publication steps were not traced; read `session-persistence-jsonl/src/{lease,generation,storage}.ts` before reproducing physical write semantics.

[OBS] Separate from Session persistence, the storage hub exposes versioned KV units. The JSON backend requires `root` and writes either one document per unit or per-record documents; the SQLite backend requires `path`, uses JSON documents per row, and defaults `journalMode` to `wal`. Unit callers own write serialization; resolved writes are durable, but a per-record batch is not crash-atomic before resolution (`packages/storage/storage/src/backend.ts:28`, `:77`; `packages/storage/storage-json/src/index.ts:1`, `:22`; `packages/storage/storage-sqlite/src/index.ts:1`, `:23`).

## Compaction

[OBS] `compaction-basic` defaults `thresholdRatio` to 0.8, `retainRatio` to 0.16, `headroomTokens` to 65,536, `maxTokens` to headroom, `compactionRetries` and `maxOverflowRetries` to 1, and `auto` to true. Target-specific `modelPolicies` may override these values. Given context window `W` and reserved completion tokens `R`, `messageBudget = W - R`, `threshold = floor(min(W * thresholdRatio, messageBudget - headroomTokens))`, and default retained tail is `floor(messageBudget * retainRatio)`; invalid budgets fail rather than silently selecting a target (`packages/compaction/compaction-basic/src/config.ts:19`, `:67`, `:108`, `:145`).

[OBS] Automatic compaction can run at pre-step pressure, at a model switch, and on provider context-window overflow. A model-switch compaction failure preserves the prior route and throws. Overflow retry requires durable replacement-surface progress and is bounded by the configured retry count (`packages/compaction/compaction-basic/src/index.ts:133`, `:157`, `:182`, `:215`).

[OBS] Range selection retains the token-budgeted tail, excludes the system head, and adjusts the cutoff to preserve tool-call/result pairing. A compaction appends `compaction/start`, runs summarization, commits a summary and a user-role checkpoint replacing the selected surface range, and appends `compaction/end`; failure attempts an error-bearing close. The original events remain in the log. The summarizer receives the conversation prefix plus a final user instruction asking for a structured Markdown checkpoint; the landed text is wrapped in `<compacted-summary>` with a fixed preamble (`packages/compaction/compaction-basic/src/region.ts:117`, `:173`, `:210`, `:237`, `:491`, `:506`; `packages/compaction/compaction-basic/src/summarizer.ts:21`, `:31`, `:70`, `:188`). Its verbatim builder is in `evidence/state-templates.txt`.

[OBS] An optional model-free pruner replaces the middle of over-budget tool-result text while preserving head, tail and rich-block order; its character counts use Unicode code points. Separately, on `IMAGE_OFFLOAD_REQUIRED`, image-offload records a durable projection that substitutes placeholder text for selected oldest input images and retries (`packages/compaction/compaction-tool-result-pruner/src/index.ts:40`, `:64`; `packages/compaction/compaction-image-offload/src/index.ts:1`, `:25`).

## Spill

[OBS] `spill-policy.maxInlineTokens` is optional; omission disables retention. When an accepted supported tool result exceeds the budget, the policy saves full text to `spillStore`, keeps a priced head/tail and omission notice, and retains the canonical program value. It skips `read`; image prices include route-specific costs. If storage, image pricing, or the notice budget fails, it logs a warning and keeps the original inline content. A PTC image dispatch can also generate a `ptc-mode` user context (`packages/spill/spill-policy/src/index.ts:22`, `:46`, `:94`, `:133`).

[OBS] Local spill's default root is a lazily created private process temp directory with prefix `dsh-spill-`. Session subdirectories use the first 12 hex characters of a SHA-256 session-id hash; files have randomized names, exclusive creation, mode `0600`, and their directory is created with mode `0700` (`packages/spill/spill-local/src/store.ts:15`, `:33`, `:75`, `:108`). [UNK] Retention cleanup policy and every spill backend are not characterized (`packages/spill/spill-local/src/cleanup.ts`).

## Todo, goal and plan state

[OBS] `todo_write` appends a whole `todo/write` list, validates unique nonempty trimmed content and statuses `pending`, `in_progress`, `completed`, and rejects non-agent callers. `allowParallelInProgress` is a required deployment choice; false rejects more than one active item. The projection retains the most recent write through turn end but clears at the next `turn/start` (`packages/todo/tool-todo/src/index.ts:1`, `:25`, `:70`, `:119`, `:189`). Thus a persisted todo event is not an indefinitely active cross-turn checklist.

[OBS] Goals use full `goal/change` snapshots or a clear tombstone; revisions advance on mutation. Goal phases are `active`, `paused`, `blocked`, `complete`, and live activation is `armed` or `disarmed`. On resume/fork an active goal is disarmed; the goal policy instructs resuming it explicitly. The round driver follows up only an active, armed goal, records goal id/revision/round in the message source, and blocks at the configured maximum round count (`packages/goal/goal/src/domain.ts:16`; `packages/goal/goal/src/types.ts:19`, `:46`, `:71`; `packages/goal/tool-goal/src/index.ts:110`; `packages/goal/goal-round-driver/src/index.ts:102`, `:172`, `:192`). Plan mode is a separate `plan/mode` fold described in `05-modes-and-approvals.md` (`packages/plan/plan-mode/src/index.ts:135`).

## Subagents and teams

[OBS] A subagent activation names a provider, child identity, delivery policy `parent` or `caller`, task content and cancellation signal. Providers declare capabilities for options, preset, output schema and depth limit. In-process children can inherit cwd, lineage and delegation depth; fresh and fork-seeded continuable children differ in whether a completed-turn prefix is provided (`packages/subagent/subagent/src/types.ts:28`, `:116`, `:128`, `:183`). Parent-delivered completion notices and peer relay text are constructed by `continuation-messages.ts` (`packages/subagent/subagent/src/continuation-messages.ts:62`, `:99`, `:137`).

[OBS] A Team is rooted at a top-level Session. Its task snapshot has id, revision, subject, description, status, optional owner, blockers, and advisory write scopes. Mutations require `expectedRevision`, increment revisions, and validate the dependency graph. Readiness means all blockers are completed; completion requires `in_progress`. Team task revisions are appended to the root session and flushed (`packages/team/agent-team/src/types.ts:7`, `:60`, `:156`; `packages/team/agent-team/src/task-board.ts:128`, `:175`; `packages/team/agent-team/src/task-graph.ts:21`; `packages/team/agent-team/src/task-view.ts:17`; `packages/team/agent-team/src/journal.ts:57`). Historical `team-message` metadata is declared, but this alone does not establish a current producer (`packages/team/agent-team/src/types.ts:108`).

## Jobs and schedule

[OBS] A settled job may send a user-role `tool-jobs` notice. Default `completionDelivery` is `wakeup`: an idle owner can receive a follow-up, while a busy owner gets queued input. `inlineOutputBytes` defaults to 2,048; larger or not-fitting output remains accessible via `job_output`. The model guidance says to avoid busy-polling and collect relevant jobs before the final answer (`packages/jobs/tool-jobs/src/index.ts:230`, `:316`, `:365`, `:384`).

[OBS] Schedule targets distinguish an existing session, `new_once`, and `new` per delivery. Reminders are queued as `schedule` follow-ups and awaited through Session persistence flush before the task is retired or advanced; a `new` target records `pendingDelivery` before creating its target. A reminder receipt confirms durable inbox delivery, not model completion (`packages/schedule/schedule/src/types.ts:350`; `packages/schedule/schedule/src/runtime.ts:228`, `:349`, `:407`, `:427`; `packages/schedule/schedule/src/delivery-history.ts:1`). [INF] Because the session flush and task advance are separate writes, a crash between them can cause duplicate delivery; the implementation is not an atomic exactly-once transaction (`packages/schedule/schedule/src/runtime.ts:259`, `:267`, `:353`).
