---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-07-notable-mechanisms"
  title: "Codex CLI notable mechanisms"
  description: "Compaction, rollout, budgets, delegation, tool catalogs and review"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:06+01:00"
  updated_at: "2026-11-03T11:54:06+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["documented source-observed execution mechanisms"]
---

# Notable mechanisms

Source root: `host:/tmp/re-recon/codex-tag/codex-rs`. [OBS] is source-observed; [INF] is inference; [UNK] is unverified. Nothing was executed against Codex. Exact compaction, realtime and review templates are copied in [mechanism templates](evidence/07-mechanism-templates.md).

## Compaction and history

[OBS] Automatic compaction synthesizes a user input from configured `compact_prompt` or bundled `SUMMARIZATION_PROMPT`; manual compaction has a distinct trigger and standalone-turn phase (`core/src/compact.rs:117-175`; `prompts/src/compact.rs:1-2`). The bundled prompt and summary prefix are `prompts/templates/compact/prompt.md` and `prompts/templates/compact/summary_prefix.md`, copied verbatim in evidence.

[OBS] Replacement history is built by rendering a fresh world state, splitting prefix from ordinary context, and inserting ordinary context before the last real user message or summary; prefix items go first (`core/src/compact.rs:84-114,690-699`). In the shown history builder, recent user messages are selected in reverse order under an approximate 20,000-token budget, then restored to chronological order. If a selected message does not fit or has non-text content, annotated text is truncated and discarded media are not cloned; source completeness is marked when truncated. A user-role compaction summary is appended, with an explicit fallback if summary text is empty (`core/src/compact.rs:63,702-818`). [UNK] The remote compaction algorithm and exact automatic trigger selection are not traced.

[OBS] `RolloutRecorder` describes canonical session rollout items as JSONL, with queued add, persist, flush, shutdown and discard commands and a background writer (`rollout/src/recorder.rs:73-147`). New rollout paths are under `$CODEX_HOME/sessions/YYYY/MM/DD/`, using local date and a filename with timestamp and thread identifier; revert can add a distinct rollout ID (`rollout/src/recorder.rs:91-103,1718-1741`). The archive directory is separately named `archived_sessions` (`rollout/src/lib.rs:86-87`). [UNK] Detailed durability semantics, rollout item filtering, compaction compression and all resume/fork reconstruction rules are not established here.

## Budgets and state

[OBS] Token-budget reminders require the feature, a known remaining-window value and configuration. If remaining tokens meet the configured threshold, a state claim gates recording the configured reminder; the fallback prompt is only eligible when explicitly allowed, remaining equals zero, a fallback exists and its claim succeeds (`core/src/session/token_budget.rs:160-220`). The context fragment contains agent path and first/current/optional previous window identifiers; its body is a developer fragment (`core/src/context/token_budget_context.rs:19-88`). Shared rollout budget accounting is separate: controller records usage; host runtimes can return `Exhausted`, producing `SessionBudgetExceeded` (`core/src/session/rollout_budget.rs:12-38,40-73`). [UNK] Threshold defaults and complete claim-reset cadence depend on configuration/model and are not asserted.

[OBS] World-state sections have stable IDs, snapshots and change rendering; restored snapshots can use retained-fragment matching or fall back to legacy matching. `host_skills` is inserted before permissions (`core/src/context/world_state/mod.rs:408-419,519-605`). [INF] A reimplementation that emits every unchanged instruction on every step would produce a different conversation and cache behavior even if the text were the same.

## Agents and realtime

[OBS] Inter-agent message/new-task and final-answer fragments are assistant role; legacy-style subagent status is a user fragment. Builders and delivery sites are listed in [chapter 04](04-instructions-and-injections.md#inter-agent-and-realtime) (`core/src/context/inter_agent_message.rs:8-20,47-72`; `core/src/context/inter_agent_completion_message.rs:25-47`; `core/src/context/subagent_notification.rs:23-48`). [UNK] Spawn limits, inheritance, delivery acknowledgements and all multi-agent tool semantics were not traced.

[OBS] A realtime handoff selects nonempty input transcript or a role-labelled transcript delta, wraps it as a user-role delegation fragment, and can include a transcript delta (`core/src/realtime_conversation.rs:1957-1999`; `core/src/context/realtime_delegation.rs:40-77`). Input is XML-escaped and bounded to 4,096 bytes preserving its start; transcript normally preserves its end under the same limit, but explicit transcript preservation bypasses that cap (`core/src/context/realtime_delegation.rs:22-24,57-76,86-115`). Realtime world-state start/end fragments follow active-state transitions (`core/src/context/world_state/realtime.rs:34-92`). [UNK] End-to-end scheduling and response routing are not fully traced.

## Code mode and incremental tools

[OBS] Code mode exposes a freeform tool with a Lark grammar permitting a `// @exec:` first line followed by source, or plain source (`core/src/tools/code_mode/execute_spec.rs:5-25`). The execute handler parses source, constructs sorted/deduplicated enabled nested tool definitions, starts a cell and may return a yielded response while it continues running (`core/src/tools/code_mode/execute_handler.rs:36-88,119-152`). Nested calls are sent through a cancellation-aware broker and carry both cell and runtime tool-call IDs (`core/src/tools/code_mode/delegate.rs:304-357`). When unavailable, direct tool mode falls back to direct tools, while code-mode modes fail closed (`core/src/tools/code_mode/mod.rs:104-122`). [UNK] Runtime language restrictions, complete callback behavior and sandbox isolation were not traced.

[OBS] For incremental Responses Lite declarations, tool and namespace definitions are hashed, changed definitions are emitted as developer `AdditionalTools` prefix items, and removed namespaces/tools become a standalone developer notice; a changed namespace description can instead emit developer instructions (`core/src/context/world_state/top_level_tools.rs:1-5,33-79,124-250`). The composed multi-agent role prompt says collaboration tools are absent from `functions.exec` and directs the model to call them directly (`prompts/src/multi_agent_instructions.rs:10-22`). This is prompt text; its complete enforcement was not independently traced. [UNK] Whether all other code-mode nested tools participate in the same incremental top-level catalog path remains unresolved.

## Review mode

[OBS] A review request can target uncommitted changes, a base branch, a commit, or custom instructions; request-specific text is generated by `prompts/src/review_request.rs:18-21,31-32,42-99`. A one-shot review subagent receives the review rubric as custom base instructions, web search and collaboration features are disabled, and its approval policy is constrained to `Never`; configured review model otherwise falls back to the parent model (`core/src/tasks/review.rs:99-142`).

[OBS] Review event processing parses the final assistant message as structured JSON, then attempts an embedded JSON object, then falls back to plain text. It records an exit result back into conversation history; interruption has a separate exit path (`core/src/tasks/review.rs:145-204,210-243`; `prompts/src/review_exit.rs:6-23`). Exact rubric and exit templates are in the linked evidence file. [UNK] The reviewer's full runtime permission profile and every exposed tool are not traced.
