---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-01-architecture"
  title: "DeepSeek Harness 0.2.2-alpha.3: architecture"
  description: "Package composition, agent loop, session history and model request path"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:59+01:00"
  updated_at: "2026-11-03T11:52:59+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Trace the source architecture and request lifecycle"]
---

# Architecture

## Composition and package map

[OBS] The root manifest declares `packages/*/*` as workspace packages (`package.json:10-17`). The shared base bundle inserts service and plugin rows; its file states that later patches target row IDs and replace the entire row config (`packages/bundle/base/cordis.patch.yml:1-15`). Web layers its own patch and four preset patches as declared in `packages/bundle/web-app/package.json:45-55`; headless is a separate overlay (`packages/bundle/headless/cordis.patch.yml:1-12`). The base-backed compositions need not yield identical tools or prompts: overlays and per-agent presets change them, and executor selection is scoped (`packages/bundle/web-app/cordis.patch.yml:580-601`; `packages/core/tools/src/index.ts:321-375`).

| Group | Relevant responsibility and source |
|---|---|
| `packages/core/agent`, `agent-loop` | Agent registry and factory interface; loop factory and concrete driver (`packages/core/agent/src/index.ts:1-5,165-198`; `packages/core/agent-loop/src/index.ts:290-335`). |
| `packages/core/session` | Event-sourced session log and derived message surface (`packages/core/session/src/index.ts:1-5,519-544,980-1018`). |
| `packages/core/system-prompt`, `tools`, executor packages | Prompt/input assembly, scoped tool definitions, native and code executors (`packages/core/system-prompt/src/index.ts:620-695`; `packages/core/tools/src/index.ts:102-117,321-375`; `packages/core/tool-executor-native/src/index.ts:8-22`; `packages/core/tool-executor-ptc/src/index.ts:22-55`). |
| `packages/core/scope`, `packages/preset` | Nested context keys, inherited registrations and preset composition (`packages/core/scope/src/index.ts:35-39,91-99,130-145`; `packages/preset/agent-preset-registry/src/index.ts:108-145`). |
| `packages/llm/llm`, `llm-deepseek` | Request and schema types and provider-specific serialization (`packages/llm/llm/src/types.ts:517-529,557-588`; `packages/llm/llm-deepseek/src/serialize.ts:151-167`). |
| `packages/fs`, `shell`, `web`, `skill`, `subagent`, `jobs` | Model-facing capabilities and their providers are composed as separate rows (`packages/bundle/base/cordis.patch.yml:281-320,363-413,463-503`). |
| `packages/session` | Persistence, projections and durable context services; JSONL receives session events (`packages/session/session-persistence-jsonl/src/storage.ts:534-560`). |
| `packages/context` | Workspace instruction files, time and references; the instruction plugin injects user messages (`packages/context/agent-instructions/src/index.ts:194-195,233-234`; `packages/bundle/sdk-app/cordis.patch.yml:57-75`). |
| `packages/host`, `client`, `api` | HTTP host, browser connection and session business API (`packages/host/webserver/src/index.ts:1-9`; `packages/client/connection/src/index.ts:118-158`; `packages/api/session-controller/src/index.ts:133-150`). |

## Agent, session and turn

[OBS] An `Agent` is a live interface registered by `AgentRegistry`; creation and driving are provided by an `AgentFactory`, installed by `AgentLoop` (`packages/core/agent/src/index.ts:165-198`; `packages/core/agent-loop/src/index.ts:290-335`). Agent creation and its session share an ID and can carry cwd, fork lineage, preset and a seeded history (`packages/core/agent/src/index.ts:58-100`). Scope identities and parent links determine inherited registrations and event routing (`packages/core/scope/src/index.ts:35-39,91-99,130-145`).

[OBS] `Session` holds an append-only event log, immutable header and ordered surface (`packages/core/session/src/index.ts:519-544`); `append` snapshots JSON data, assigns sequence and time and commits it (`packages/core/session/src/index.ts:806-837`). `deriveMessages()` builds the model history from surface-producing events and excludes log-only entries (`packages/core/session/src/index.ts:980-1018`). The JSONL persistence backend queues `session/event` and flushes on `session/flush` (`packages/session/session-persistence-jsonl/src/storage.ts:534-560`).

[OBS] In a proposed step the driver claims inbox input, resolves the model selection and admitted tool set, assembles prompt inputs and runs `agent/pre-step` (`packages/core/agent-loop/src/agent.ts:283-314`). The turn records its boundaries (`packages/core/agent-loop/src/agent.ts:327-361,388-422`). On an admitted step, it renders the prompt, prepares the route, refreshes runtime context, logs system and user messages, builds the request and streams the model response (`packages/core/agent-loop/src/agent.ts:435-490`). If the final assistant message has no tool calls the step completes; otherwise the loop sends the calls through `ctx.tools.execute`, using the admitted tool set, and can continue (`packages/core/agent-loop/src/agent.ts:584-603`). The tool pipeline has an ordered pre-execution policy and guard stage (`packages/core/tools/src/pipeline.ts:190-240`).

## Request shape and client boundary

[OBS] `GenerateOptions` includes the provider and model, ordered `messages`, optional separate `system` for one-shot callers, optional `tools`, tool history and generation controls (`packages/llm/llm/src/types.ts:557-595`). The loop-generated request instead derives session messages, includes the canonical header's schemas only when nonempty, adds session ID and abort signal, and freezes the request (`packages/core/agent-loop/src/agent.ts:656-672,726-744`). The loop projects the system prompt into session system-message history (`packages/core/agent-loop/src/agent.ts:442-461`); it does not set the optional standalone `system` property in this object. The DeepSeek Messages adapter then maps schemas to the wire fields `name`, `description`, `input_schema` and optionally `defer_loading` (`packages/llm/llm-deepseek/src/serialize.ts:151-167`).

[OBS] The Host webserver provides HTTP/HTTPS route registration but does not itself serve application files (`packages/host/webserver/src/index.ts:1-9`). The connection plugin supplies an authenticated host-side `/api` bridge and a connect route (`packages/client/connection/src/index.ts:118-158`); its browser implementation supports HTTP plus WebSocket (`packages/client/connection/src/client/index.ts:80-93,116-117`). The Host session controller exposes session operations and depends on the agent, session and workspace services (`packages/api/session-controller/src/index.ts:133-150`). [INF] These browser/Host surfaces present and control the same session/agent services rather than constituting a second model loop. [UNK] Transport details and every adapter-specific wire encoding are outside this source pass.
