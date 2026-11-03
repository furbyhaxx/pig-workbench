---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-03-tool-declarations"
  title: "DeepSeek Harness 0.2.2-alpha.3: model-facing tools"
  description: "Source-backed tool declarations, executor projection, ordering and scenario schemas"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:59+01:00"
  updated_at: "2026-11-03T11:52:59+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Record tool schema evidence and executor behavior"]
---

# Tool declarations

## Exact scenario inventory and limitations

[OBS] [evidence/sdk-desktop-schemas.json](evidence/sdk-desktop-schemas.json) is a verbatim copy of `snapshots/sdk/desktop-context/tool-schemas.expected.json:1-935`: the complete `initial` array preserves the expected names, descriptions, nested JSON schemas and their order for that SDK scenario. The corresponding complete expected prompt is [evidence/sdk-desktop-system-prompt.txt](evidence/sdk-desktop-system-prompt.txt). `snapshots/sdk/desktop-context/snapshot.yml:1-6` names an authored SDK-profile scenario; `snapshots/sdk/desktop-context/cordis.yml:1-41` changes the skill roots, model configuration, subagent configuration, clock and `applyPatch` setting. This is a checked-in expectation, not a request captured in this investigation.

[OBS] In particular, the fixture explicitly turns `apply_patch` on (`snapshots/sdk/desktop-context/cordis.yml:38-41`); the shared base and Web standard preset configure it off (`packages/bundle/base/cordis.patch.yml:296-300`; `packages/bundle/web-app/presets/standard.patch.yml:26-31`). Consequently, do not copy the snapshot's first entry as an unconditional current default. The base also chooses `bash` on non-Windows and `pwsh` on Windows (`packages/bundle/base/cordis.patch.yml:281-287`). [UNK] The full exact unpatched SDK, Web-standard and headless schema arrays, including dynamic provider and optional-service effects, have not been reconstructed. There is no assertion that the snapshot proves a live invocation.

[OBS] The following 33 names are the **snapshot order**, not a universal installation's tool list. The source column identifies the owner of the declaration. The linked schema evidence contains the full verbatim description and parameter schema for every row; its upstream line points at the particular name. `subagent` and `subagent_fork` share a parameterized tool definition, configured as distinct names (`packages/subagent/tool-subagent/src/index.ts:162,207-209`; `packages/bundle/base/cordis.patch.yml:389-402`).

| Name | Declaration owner | Snapshot name line |
|---|---|---:|
| `apply_patch` | `packages/fs/tool-fs/src/apply-patch.ts:27` | `snapshots/sdk/desktop-context/tool-schemas.expected.json:4` |
| `ask_user_question` | `packages/interaction/tool-ask-user/src/index.ts:40` | same file `:32` |
| `bash` | `packages/shell/tool-bash/src/index.ts:369` | `:95` |
| `create_goal` | `packages/goal/tool-goal/src/index.ts:208` | `:140` |
| `edit` | `packages/fs/tool-fs/src/edit.ts:86` | `:160` |
| `get_goal` | `packages/goal/tool-goal/src/index.ts:196` | `:202` |
| `glob` | `packages/fs/tool-fs-search/src/glob.ts:322` | `:210` |
| `grep` | `packages/fs/tool-fs-search/src/grep.ts:287` | `:230` |
| `interrupt_agent` | `packages/subagent/tool-subagent-control/src/index.ts:126` | `:254` |
| `job_kill` | `packages/jobs/tool-jobs/src/index.ts:561` | `:270` |
| `job_list` | `packages/jobs/tool-jobs/src/index.ts:541` | `:290` |
| `job_output` | `packages/jobs/tool-jobs/src/index.ts:433` | `:298` |
| `list_agents` | `packages/subagent/tool-subagent-control/src/list-agents.ts:66` | `:322` |
| `plugin_manager` | `packages/boot/plugin-manager/src/tools.ts:87` | `:330` |
| `present` | `packages/deliverables/tool-present/src/index.ts:39` | `:391` |
| `read` | `packages/fs/tool-fs/src/read.ts:78` | `:424` |
| `read_image` | `packages/fs/tool-fs/src/read-image.ts:211` | `:448` |
| `schedule_create` | `packages/schedule/tool-schedule/src/index.ts:372` | `:464` |
| `schedule_edit` | `packages/schedule/tool-schedule/src/index.ts:431` | `:500` |
| `schedule_list` | `packages/schedule/tool-schedule/src/index.ts:394` | `:554` |
| `schedule_view` | `packages/schedule/tool-schedule/src/index.ts:417` | `:575` |
| `send_message` | `packages/subagent/tool-subagent-control/src/index.ts:46` | `:591` |
| `skill` | `packages/skill/tool-skill/src/index.ts:85` | `:612` |
| `subagent` | `packages/subagent/tool-subagent/src/index.ts:207-209` | `:628` |
| `subagent_fork` | `packages/subagent/tool-subagent/src/index.ts:207-209` | `:653` |
| `todo_write` | `packages/todo/tool-todo/src/index.ts:137` | `:678` |
| `update_goal` | `packages/goal/tool-goal/src/index.ts:235` | `:717` |
| `wait_agent` | `packages/subagent/tool-subagent-control/src/wait-agent.ts:73` | `:762` |
| `web_fetch` | `packages/web/tool-web/src/fetch.ts:459` | `:775` |
| `web_search` | `packages/web/tool-web/src/search.ts:363` | `:791` |
| `workflow` | `packages/workflow/tool-workflow/src/index.ts:331-333` | `:810` |
| `working_directory` | `packages/session/tool-working-directory/src/index.ts:21` | `:888` |
| `write` | `packages/fs/tool-fs/src/write.ts:74` | `:901` |

[OBS] The older SDK snapshot and its exact ordered schemas are separately preserved as [evidence/sdk-bash-tool-schemas.json](evidence/sdk-bash-tool-schemas.json) (`snapshots/sdk/bash-tool/tool-schemas.expected.json:1-651`). Its composition applies a legacy overlay disabling `present`, questions, scheduling and other desktop features while enabling `apply_patch` (`snapshots/sdk/legacy-context.patch.yml:1-28`; selection `snapshots/sdk/sdk.snapshot.ts:704-726,775-779`).

## Definition, presentation and dispatch

[OBS] `defineTool` compiles author parameter maps into object-rooted JSON Schema, validates arguments, and couples each definition with a canonical output schema, renderer and optional presentation metadata (`packages/core/tools/src/schema.ts:444-458,479-503,563-602`). Tool definitions can also provide display callbacks and a concurrency classifier; these callbacks are not model-facing declarations (`packages/core/tools/src/contracts.ts:17-108`). `projectToolSchema` projects `name`, `description`, `parameters` and optional `deferLoading` (`packages/core/tools/src/schema.ts:641-646`). The `ToolSchema` type itself is defined at `packages/llm/llm/src/types.ts:517-529`.

[OBS] `ToolsService` registers a prompt-schema provider that selects the active scoped executor and describes the step-visible definitions (`packages/core/tools/src/index.ts:102-117`). Native execution projects each visible tool directly and dispatches the batch through the shared pipeline (`packages/core/tool-executor-native/src/index.ts:8-22`). PTC instead presents one `run_code` schema, adds a PTC-only instruction and generated language-specific SDK to system sections, and rejects direct calls to nested tool names (`packages/core/tool-executor-ptc/src/index.ts:12-55`). `run_code` has required string arguments `code` and `description`; its language-specific text and optional timeout/sandbox controls depend on the runtime (`packages/core/tool-executor-ptc/src/ptc.ts:43-63,105-125,298-319`). [OBS] One complete expected PTC `run_code` description and parameter schema is copied verbatim in [evidence/sdk-desktop-ptc-schemas.json](evidence/sdk-desktop-ptc-schemas.json) (`snapshots/sdk/desktop-ptc-context/tool-schemas.expected.json:1-42`), with its corresponding full prompt and generated SDK in [evidence/sdk-desktop-ptc-system-prompt.txt](evidence/sdk-desktop-ptc-system-prompt.txt) (`snapshots/sdk/desktop-ptc-context/system-prompt.expected.md:1-1637`). The scenario explicitly enables PTC and `apply_patch` (`snapshots/sdk/desktop-ptc-context/cordis.yml:38-47`). [UNK] This fixture does not establish all runtime- or language-dependent variants.

[OBS] `agent-tool-presentation` is a deprecated selector that mounts either executor; it is not a third executor (`packages/core/agent-tool-presentation/src/index.ts:1-54`). The loop builds the request with session-derived `messages`, optional `tools`, session ID and cancellation signal (`packages/core/agent-loop/src/agent.ts:656-744`). The DeepSeek Messages serializer maps each tool to `name`, `description`, `input_schema` and optional `defer_loading` (`packages/llm/llm-deepseek/src/serialize.ts:151-167`). Other adapters can project this interface differently; no wire traffic was observed.

## Order and conditional surfaces

[OBS] In the absence of configuration, tools sort by code-unit name. A configured `toolOrder` must have exactly one `<unlisted-tools>` marker; unlisted tools go there in lexicographic order, duplicates fail at config validation, and names absent from the known pre-restriction universe fail assembly (`packages/core/system-prompt/src/index.ts:209-253`). The tests explicitly check alphabetical order, marker placement, stable equal-name order, missing-name failures, and that the waterfall cannot override tool schemas (`packages/core/system-prompt/tests/tool-order.spec.ts:20-108`). A tool registered but restricted from a scope is a known, permissible absence (`packages/core/system-prompt/tests/scoped.spec.ts:190-204`).

[OBS] Optional and conditional registrations change the set. Plan-mode registers `exit_plan_mode` with a step-time admission condition (`packages/plan/plan-mode/src/index.ts:280-281,375`); MCP can supply external tools (`packages/mcp/mcp-resources/src/tools.ts`), and experimental terminal support is a separate plugin (`packages/experimental/tool-terminal/src/index.ts:159`). Test-only definitions also exist (`packages/test-support/tool-testkit/src/index.ts`). [UNK] Optional, external and test-only declarations have not been inventoried exhaustively; names and schemas may be configuration-dependent.
