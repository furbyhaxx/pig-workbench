---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-03-tool-declarations"
  title: "Codex CLI model-facing tool declarations"
  description: "Registry order, built-in and extension tool families, schema sources, gates, and Responses serialization"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["documented tool registry, conditional declarations, and schema evidence"]
---

# Tool declarations

## Evidence and limits

[OBS] [tool-spec-source-dump.txt](evidence/tool-spec-source-dump.txt) contains
line-numbered source declarations, exact literal descriptions, schema
constructors, grammar and schema dependencies. The
[supplement](evidence/tool-spec-source-supplement.txt) contains selection and
dynamic-description code; [message-board declarations](evidence/tool-spec-message-board.txt)
are separate. `path:line` prefixes identify original source lines; strip them
to recover the quoted source. The source dump includes the upstream test fixture
`core/tests/suite/snapshots/all__suite__scenarios__web_search__literature_search_web_run_schema.snap:1-320`,
which is an existing **test snapshot**, not a capture made during this analysis.
Bundled model message overrides, including tool description/schema overrides, are
decoded in [catalog-nonbase-messages.json](evidence/catalog-nonbase-messages.json)
from `models-manager/models.json`; [catalog-nonbase-sources.txt](evidence/catalog-nonbase-sources.txt)
maps each decoded object's source range, and [prompt/index.txt](evidence/prompt/index.txt)
maps base-instruction positions.

[UNK] This is not an executed, serialized dump for every feature configuration.
Generated `schemars` outputs, arbitrary runtime extensions, MCP registrations,
custom catalog overrides and all combinations have not been rendered. The
source declarations provide exact constructors and strings for the cases shown,
but the complete ordered wire list and every resulting JSON schema for an
unobserved session remain unverified. No fixed global list exists independently
of provider, model, feature flags, tool policy, selected environments and external
servers (`core/src/tools/spec_plan.rs:122-182,630-729`).

## Registration and ordering

[OBS] `build_tool_router` adds core tools, MCP tools, extension executors, then
client-supplied dynamic tools; hosted specs are built separately
(`core/src/tools/spec_plan.rs:122-182`). Core registrations follow shell,
MCP-resource helpers, utilities, and collaboration
(`core/src/tools/spec_plan.rs:1187-1206`). The registry stores entries in an
`IndexMap` and iterates its values in insertion order
(`core/src/tools/registry.rs:428-430,516-565,623-625`). Code mode prepends
`exec`, then `wait` (the implementation prepends wait first and then exec);
`tool_search` is conditionally appended during finalization
(`core/src/tools/spec_plan.rs:394-466,1019-1106`). Visibility filtering occurs
before hosted specs are appended (`core/src/tools/spec_plan.rs:630-661`).

[OBS] Namespaces with the same name merge at their first occurrence, and their
members are sorted by name (`core/src/tools/spec_plan.rs:1110-1158`). Thus the
registration list below is **not** necessarily the final wire order within a
namespace. Responses Lite subsequently combines unnamespaced functions and
custom tools into its `functions` namespace (`tools/src/tool_spec.rs:95-139`).

## Core registration sequence

[OBS] The following is the order of conditional registration before namespace
merging. Source locations named in the table contain the complete descriptions,
properties, required lists and nested schemas; the table is a navigational
summary, not a replacement for them. Unless shown otherwise, schemas are function
object schemas. `*` marks a required field.

| Name | Purpose and input summary | Source and condition |
|---|---|---|
| `exec_command` | Shell: `cmd`*, `workdir`, `shell`, `login`, `tty`, `yield_time_ms`, `max_output_tokens`, approval fields; optional `environment_id`. | `core/src/tools/handlers/shell_spec.rs:22-107,225-332`; shell feature on and model shell not disabled; one-shot mode changes description and replaces yield/TTY with `timeout_ms` (`core/src/tools/handlers/unified_exec/exec_command.rs:98-118,278-307`). |
| `write_stdin` | Write/poll an existing shell session: `session_id`*, `chars`, `yield_time_ms`, `max_output_tokens`. | `core/src/tools/handlers/shell_spec.rs:110-151`; only when unified execution is enabled (`core/src/tools/spec_plan.rs:1244-1275`). |
| `list_mcp_resources`, `list_mcp_resource_templates` | List resources/templates: `server`, `cursor`. | `core/src/tools/handlers/mcp_resource_spec.rs:8-63`; an MCP server exists or CodeModeOnly is active (`core/src/tools/spec_plan.rs:1278-1294`). |
| `read_mcp_resource` | Read resource: `server`*, `uri`*. | `core/src/tools/handlers/mcp_resource_spec.rs:65-96`; same gate as the preceding row. |
| `update_plan` | Update checklist: `plan`* array of `step`* and `status`* (`pending`, `in_progress`, `completed`), optional `explanation`. | `core/src/tools/handlers/plan_spec.rs:7-57`; configuration enables it (`core/src/tools/spec_plan.rs:1302-1304`). |
| `wait_for_environment` | Wait for environment: `environment_id`*. | `core/src/tools/handlers/wait_for_environment.rs:19-23,84-104`; deferred-executor feature (`core/src/tools/spec_plan.rs:1306-1315`). |
| `request_user_input` | Blocking questions: `questions`*, each with `id`, `header`, `question`, `options`; all four required. | `core/src/tools/handlers/request_user_input_spec.rs:16-93,119-142`; experimental setting (`core/src/tools/spec_plan.rs:1318-1325`). |
| `request_user_input_async` | Nonblocking questions: `questions`* of `title`* and optional `options`; catalog can override description and schema. | `core/src/tools/handlers/request_user_input_async.rs:22-89`; additionally requires supported model tool and unrestricted root (`core/src/tools/spec_plan.rs:1328-1353`). |
| `send_message_to_user_async` | Nonblocking message: `message`*. | `core/src/tools/handlers/send_message_to_user_async.rs:21-56`; feature or model advertisement and unrestricted root (`core/src/tools/spec_plan.rs:1355-1363`). |
| `request_permissions` | Request `permissions`*, optionally `reason`, `environment_id`; nested network/filesystem schema. | `core/src/tools/handlers/shell_spec.rs:154-188,273-332`; feature gate (`core/src/tools/spec_plan.rs:1365-1367`). |
| `new_context`, `get_context_remaining` | Start a new context window; get remaining tokens; both default to empty object schemas, subject to catalog override. | `core/src/tools/handlers/new_context_window_spec.rs:8-19`; `core/src/tools/handlers/get_context_remaining_spec.rs:10-22`; token-budget feature (`core/src/tools/spec_plan.rs:1369-1381`). |
| `clock.curr_time`, `clock.sleep` | Read UTC time with no parameters; wait using `duration_ms`*. | `core/src/tools/handlers/current_time.rs:24-77`; `core/src/tools/handlers/sleep.rs:26-63`; feature/model gates (`core/src/tools/spec_plan.rs:1385-1409`). |
| `list_available_plugins_to_install`, `request_plugin_install` | List install candidates with empty parameters; request install with presentation-dependent required fields. | `core/src/tools/handlers/list_available_plugins_to_install_spec.rs:7-21`; `core/src/tools/handlers/request_plugin_install_spec.rs:10-83`; plugin/app/tool-suggest gates and nonempty candidates (`core/src/tools/spec_plan.rs:1411-1425`). |
| `apply_patch` | Custom freeform Lark grammar, not JSON arguments; multi-environment mode adds an environment directive. | `core/src/tools/handlers/apply_patch_spec.rs:5-27`; `core/assets/tools/apply_patch.lark:1-19`; model patch type must be present (`core/src/tools/spec_plan.rs:1427-1431`). |
| `test_sync_tool` | Test synchronization, optional sleep, barrier and Git-enrichment parameters. | `core/src/tools/handlers/test_sync_spec.rs:6-69`; requires explicit model advertisement (`core/src/tools/spec_plan.rs:1433-1439`). |
| `view_image` | View local image: `path`*, optional model/budget-dependent `detail` and multi-environment `environment_id`. | `core/src/tools/handlers/view_image_spec.rs:16-51`; feature gate (`core/src/tools/spec_plan.rs:1442-1453`). |

[OBS] The shell model enum in this tag has `UnifiedExec` and `Disabled`, with
legacy aliases parsed into `UnifiedExec` (`protocol/src/openai_models.rs:329-333`).
The registry shown here emits `exec_command` and possibly `write_stdin`; it does
not establish a separate model-facing `local_shell` tool
(`core/src/tools/spec_plan.rs:1244-1275`).

## Collaboration tools

[OBS] With V1 enabled, registration is `multi_agent_v1.spawn_agent`,
`send_input`, `resume_agent`, `wait_agent`, `close_agent`; depth and deferred
exposure affect visibility (`core/src/tools/spec_plan.rs:735-748,1572-1599`).
Their complete defaults and schema constructors are in
`core/src/tools/handlers/multi_agents_spec.rs:70-107,166-202,265-301,336-359`.

[OBS] V2 registration is `spawn_agent`, optionally `send_message` and
`followup_task`, optionally `wait_agent`, then `interrupt_agent` and
`list_agents`, under a configurable namespace. Descriptions and parameter
schemas may be overridden by model messages; spawn also depends on available
models, role configuration and override settings
(`core/src/tools/spec_plan.rs:1457-1571`;
`core/src/tools/handlers/multi_agents_spec.rs:108-164,204-263,303-334,361-385`).
Within a merged namespace the members are alphabetically sorted, not kept in
this registration order.

## Code mode, hosted search, and extension tools

[OBS] `exec` is a freeform Lark-constrained code tool. Its generated description
includes selected nested definitions; `wait` takes `cell_id`* and optional
`yield_time_ms`, `max_tokens`, `terminate`, with catalog overrides possible
(`core/src/tools/code_mode/execute_spec.rs:5-26`;
`core/src/tools/code_mode/wait_spec.rs:6-64`;
`core/src/tools/spec_plan.rs:1019-1106`). `tool_search` is a special
`type: tool_search` client-executed spec with `query`* and optional `limit`,
registered when the model supports search and deferred searchable tools exist
(`core/src/tools/handlers/tool_search_spec.rs:16-103`;
`core/src/tools/spec_plan.rs:397-466`). CodeModeOnly can hide direct tools while
making eligible tools available inside `exec` (`core/src/tools/spec_plan.rs:834-868`).

[OBS] Hosted `web_search` is gated by provider capability, search mode and
absence of standalone search. Cached, indexed and live modes set different
external/indexed access flags; Responses Lite omits hosted specs
(`core/src/tools/hosted_spec.rs:14-45`; `core/src/tools/spec_plan.rs:690-725`).
Standalone `web.run` uses a generated schema and bundled description
(`ext/web-search/src/tool.rs:50-91`; `ext/web-search/src/schema.rs:8-42`).

[OBS] The extension interfaces can additionally expose:

- `get_goal`, `create_goal`, `update_goal` when goal tools are visible
  (`ext/goal/src/spec.rs:9-94`; `ext/goal/src/extension.rs:555-607`).
- `history.list_windows`, `history.list_items`, `history.read_item`,
  `history.search_contents`, and `notes.list_files_by_prefix`, `notes.read_file`,
  `notes.search_contents`, `notes.append_to_file`, `notes.write_file` when the
  extension has thread configuration and identity
  (`ext/history-notes/src/tools.rs:34-79,122-292`;
  `ext/history-notes/src/extension.rs:189-218`).
- `memories.add_ad_hoc_note`, `memories.list`, `memories.read`, `memories.search`
  when dedicated memories tools are enabled (`ext/memories/src/lib.rs:18-22`;
  `ext/memories/src/tools/mod.rs:24-76`; `ext/memories/src/extension.rs:138-161`).
- `skills.list`, `skills.read`, and optionally `skills.installations` with
  enabled skill tools and V2 discovery (`ext/skills/src/tools/mod.rs:53-93`).
- `multi_env_copy_file` and `image_gen.imagegen`, when their respective
  extensions expose them (`ext/multi-env-copy-file/src/tool.rs:21-89`;
  `ext/image-generation/src/lib.rs:8-9`; `ext/image-generation/src/extension.rs:86-114`).
- Discussion-board `create_channel`, `get_channels`, `list_threads`,
  `search_posts`, `read_thread`, `read_post`, `subscribe`, `unsubscribe`, `post`
  in a configured namespace when a board binding exists
  (`message-board/src/tools/selection.rs:25-58`;
  `message-board/src/tools/spec.rs:10-151`;
  `ext/agent-message-board/src/extension.rs:101-137`).

[OBS] MCP and client-supplied dynamic function/namespace definitions are converted
from externally supplied names, descriptions and schemas, so their complete
contents cannot be enumerated solely from the bundled source
(`core/src/tools/spec_plan.rs:153-173,1604-1635`;
`tools/src/responses_api.rs:82-89,119-180`).

## Wire conversion

[OBS] `ToolSpec` serializes with a `type` discriminator: `function`,
`namespace`, `tool_search`, `web_search`, or `custom`
(`tools/src/tool_spec.rs:18-59`). Function fields include `name`, `description`,
`strict`, optional `defer_loading`, and `parameters`; `output_schema` is skipped
in this serializer (`tools/src/responses_api.rs:33-45`). Object, array, scalar,
enum, `required`, and `additionalProperties` schema construction/serialization
are defined in `tools/src/json_schema/types.rs:40-172`. The client sends raw
serialized tools for ordinary Responses; Responses Lite converts them to
`AdditionalTools` developer input (`core/src/client.rs:1053-1077`;
`tools/src/tool_spec.rs:82-146`).
