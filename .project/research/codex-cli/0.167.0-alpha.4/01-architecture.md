---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-01-architecture"
  title: "Codex CLI source architecture and model request flow"
  description: "Thread/session ownership, turn sampling, tool dispatch, prompt construction and Responses request shape"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["traced the model-facing execution path from source"]
---

# Architecture

## Major seams

[OBS] `cli/` owns the `codex` binary entry point (`cli/Cargo.toml:1-10`);
`core/` owns thread/session orchestration; `protocol/` supplies shared response
items and instruction types; `models-manager/` selects model metadata and
prompts; `prompts/` resolves bundled/model text; `tools/` defines host tool
and Responses serialization types; `codex-api/` contains the Responses request
and transport (`core/src/codex_thread.rs:194-210`;
`protocol/src/models.rs:1694-1724`; `models-manager/src/lib.rs:18-21`;
`prompts/src/lib.rs:1-36`; `tools/src/tool_spec.rs:18-59`;
`codex-api/src/common.rs:281-307`). Extension-owned tool families are documented
in [03-tool-declarations.md](03-tool-declarations.md).

## Thread, session, and turn

[OBS] `ThreadManager` creates and retains threads in memory; its shared state
holds a map of `ThreadId` to `CodexThread` and shared services
(`core/src/thread_manager.rs:260-266,518-551`). A `CodexThread` owns an
`Arc<Session>` and `SessionIo`; it exposes submission and event channels
(`core/src/codex_thread.rs:194-249,873-875`). Session state owns thread identity,
configuration, conversation history services, input queue and active turn;
the source states at most one running task per session
(`core/src/session/session.rs:60-108`). Existing history can be resumed or forked
(`core/src/thread_manager.rs:1391,1637`); this does not imply all session state is
identical across those operations.

[OBS] `submission_loop` receives operations and dispatches turn input, interrupts
and other operations (`core/src/session/handlers.rs:459-521,595-609`). A
`RegularTask` signals turn start, resolves prewarm/lifecycle work, then calls
`run_turn`; pending input can cause another pass in the same regular task
(`core/src/tasks/regular.rs:38-118,124-165`).

## The sampling/tool loop

1. [OBS] `run_turn` optionally compacts before sampling, captures a first
   `StepContext` including required MCP state, records context updates, resolves
   skill/plugin injections, and records user input
   (`core/src/session/turn.rs:178-213,216-253,269-339,395-414`).
2. [OBS] Its loop accepts pending input, captures a step snapshot, records
   changed world state, clones history, and calls `run_sampling_request`
   (`core/src/session/turn.rs:485-605`). A step snapshot ensures the request's
   context, advertised tools and calls use the same view
   (`core/src/session/turn.rs:532-575`).
3. [OBS] Sampling formats history and builds `Prompt` with the selected base,
   tool specs and output schema; incremental-tool mode instead supplies empty
   top-level tools/base because they are managed in context history
   (`core/src/session/turn.rs:1751-1778,1792-1868`). The client opens a stream
   (`core/src/session/turn.rs:2732-2744,2834-2850`).
4. [OBS] On a completed output item, core builds a tool call if appropriate,
   records it and schedules execution; otherwise it finalizes the message item.
   Tool calls set the follow-up flag (`core/src/session/turn.rs:3014-3145`;
   `core/src/stream_events_utils.rs:316-380`). The router converts function,
   client tool-search and custom-call variants and dispatches via its registry
   (`core/src/tools/router.rs:271-340,390-427`). Tool parallelism uses a shared
   or exclusive admission lock (`core/src/tools/parallel.rs:269-334`); collected
   results are recorded in history in `FuturesOrdered` order
   (`core/src/session/turn.rs:2679-2704`).
5. [OBS] On completion, token usage and any `end_turn` continuation are recorded;
   a tool result or pending input can cause another sample, while a no-follow-up
   path runs stop hooks and can finish or continue. Retry and compaction branches
   also exist (`core/src/session/turn.rs:611-637,687-715,751-784,1792-1944,3329-3383`).

This is source control flow, not a traced running conversation. [UNK] Actual
stream timing and provider-specific behavior were not measured.

## Prompt and wire request

[OBS] The session's base is selected from configuration, restored history, then
model instructions; the separate world-state and prompt assembly rules are
traced in [02-system-prompt.md](02-system-prompt.md)
(`core/src/session/mod.rs:834-837,871-875`;
`core/src/session/world_state.rs:178-446`). `Prompt` contains input items, tool
specs, a parallel-tools flag, base instructions, and optional structured output
schema (`core/src/client_common.rs:21-42`).

[OBS] `build_responses_request` prepends nonempty base instructions as a
developer input item. Ordinary Responses puts serialized declarations in
`tools`; Responses Lite instead prepends an `AdditionalTools` developer item
and omits the top-level `tools` field (`core/src/client.rs:1027-1092`). The
constructed request sets model from selected slug, `tool_choice: "auto"`,
`store: false`, `stream: true`; it includes reasoning, encrypted-reasoning
request, optional service tier, prompt-cache key, text/output controls and client
metadata. Parallel tool calls are disabled for Lite
(`core/src/client.rs:1119-1183`). The typed wire structure and optional fields
are in `codex-api/src/common.rs:281-307`.

[OBS] The HTTP Responses client sends a POST to `/responses`, requests
`text/event-stream`, and wraps the resulting event stream
(`codex-api/src/endpoint/responses.rs:110-154`). WebSocket is also conditional
on provider support and session fallback state (`core/src/client.rs:1195-1205`;
`codex-api/src/common.rs:310-360`). No actual request was sent in this analysis.
