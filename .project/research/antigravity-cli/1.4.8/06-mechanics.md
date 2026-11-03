---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-06-mechanics"
  title: "agy 1.4.8 agent execution mechanics"
  description: "Turn loop, tool inventory, permission presets and grants, sandboxing, subagents, extensions and persistence"
  status: "published"
  revision: 2
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/06-mechanics.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:59:34+01:00"
      note: "Initial analysis of agy 1.4.8."
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/06-mechanics.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# agy 1.4.8 — agent execution mechanics

Internal mechanics only: the turn loop, tools, permissions, sandboxing, subagents,
extension surfaces, persistence. Network and auth are covered elsewhere.

**Method note.** Everything below is either (a) observed — recovered bytes, a decoded
protobuf descriptor embedded in `.rodata`, a klog line, or a file on this machine — or
(b) derived and labelled as such. The highest-yield tool turned out to be the ~409
serialized `FileDescriptorProto` blobs embedded in `.rodata` (`protoimpl` rawDesc): they
give exact message layouts and enum numbers for every JetSki proto without disassembly.
Descriptor roots found by scanning for `\n<len><path>.proto` where `<len>` equals the
path length; the largest ones are

| proto | file offset |
|---|---|
| `third_party/jetski/cortex_pb/cortex.proto` | `0x61f8af9` |
| `third_party/jetski/language_server_pb/language_server.proto` | `0x6177fe5` |
| `third_party/jetski/jetski_cortex_pb/jetski_cortex.proto` | `0x5ecfb64` |
| `third_party/jetski/cortex/core/tools_pb/tools.proto` | `0x5c6969b` |
| `third_party/gemini_coder/proto/trajectory.proto` | `0x5e4c5af` |
| `third_party/gemini_coder/proto/service.proto` | `0x5c59a19` |
| `third_party/jetski/cortex/policyguardian/state.proto` | `0x5c1fdc0` |
| `google/gca/aicode/v1main/remote_agent_proxy.proto` | `0x6054dab` |

---

## 1. The turn loop

### 1.1 Three nested containers

| concept | type | what it is |
|---|---|---|
| **cascade** | `exa.jetski_cortex_pb.CascadeState` | the whole live conversation: run status, queued steps, the trajectory, background commands/tasks, child agents, cost and context accounting. One per conversation. |
| **trajectory** | `exa.cortex_pb.Trajectory` (+ `Step.subtrajectory`, field 6) | an ordered append-only list of `Step`s with parallel metadata arrays. A cascade has one main trajectory; subagents, browser agents, replays and battle forks each get their own. |
| **step** | `gemini_coder.Step` | one typed record: a model response, a tool call+result, a user input, a system message, a checkpoint, a permission request. |

`CascadeState` (jetski_cortex.proto) is the object the UI subscribes to:

```
cascade_id, status(CascadeRunStatus), executable_status, executor_loop_status,
executor_metadata, queued_steps[], artifact_snapshots[], trajectory,
trajectory_file_diffs[], background_commands[], background_tasks[],
pending_agent_messages[], accumulated_sections[], active_hook, step_hooks{},
execution_status, credit_usage_summary, context_window_usage, fully_idle,
has_active_children
```

Two statuses side by side — `executable_status` and `executor_loop_status` — plus
`execution_status{initialization | model_request{attempt,max_attempts,retry_at}}` show the
layering: an **executor loop** drives a **generator** and each iteration is a *model
request*. `ExecutorMetadata` is the loop's ledger:

```
termination_reason(ExecutorTerminationReason), num_generator_invocations,
last_step_idx, proceeded_with_auto_continue, num_forced_invocations,
execution_id, cascade_config, request_override_config, mendel_experiment_ids,
execution_error, agent_config_resolution, loaded_customizations,
before_snapshot, after_snapshot, segment_records[], trajectory_records[]
```

`ExecutorTerminationReason` (`cortex.proto`) enumerates every way a turn ends, and is the
single best description of the loop's exit conditions:

```
0 UNSPECIFIED 1 ERROR 2 USER_CANCELED 3 MAX_INVOCATIONS 4 NO_TOOL_CALL
6 MAX_FORCED_INVOCATIONS 7 EARLY_CONTINUE 8 TERMINAL_STEP_TYPE
9 TERMINAL_CUSTOM_HOOK 10 INJECTED_RESPONSE 11 MAX_TOKEN_BUDGET_EXCEEDED
```

So: the loop stops when the model emits no tool call, when a terminal step type
(`CORTEX_STEP_TYPE_FINISH`) is produced, when a hook terminates it, when a step is
injected, or when an invocation/token budget is hit. `CascadeRunStatus` is
`IDLE|RUNNING|CANCELING|BUSY`.

### 1.2 Step types

`CortexStepType` has 118 named values; the numbers matter because they are persisted
(`steps.step_type` integer column). Recovered with exact values:

```
0 UNSPECIFIED   1 DUMMY        2 FINISH       3 PLAN_INPUT    4 MQUERY
5 CODE_ACTION   6 GIT_COMMIT   7 GREP_SEARCH  8 VIEW_FILE     9 LIST_DIRECTORY
10 COMPILE     13 VIEW_CODE_ITEM 14 USER_INPUT 15 PLANNER_RESPONSE
17 ERROR_MESSAGE 21 RUN_COMMAND 23 CHECKPOINT 24 PROPOSE_CODE
25 FIND        27 SUGGESTED_RESPONSES 28 COMMAND_STATUS 29 MEMORY
31 READ_URL_CONTENT 32 VIEW_CONTENT_CHUNK 33 SEARCH_WEB 34 RETRIEVE_MEMORY
38 MCP_TOOL    39 MANAGER_FEEDBACK 40 TOOL_CALL_PROPOSAL 41 TOOL_CALL_CHOICE
42 TRAJECTORY_CHOICE 45 CLIPBOARD 47 VIEW_FILE_OUTLINE 49 POST_PR_REVIEW
51 LIST_RESOURCES 52 READ_RESOURCE 53 LINT_DIFF 54 FIND_ALL_REFERENCES
55 BRAIN_UPDATE 56 OPEN_BROWSER_URL 57 RUN_EXTENSION_CODE 59 PROPOSAL_FEEDBACK
60 TRAJECTORY_SEARCH 61 EXECUTE_BROWSER_JAVASCRIPT 62 LIST_BROWSER_PAGES
63 CAPTURE_BROWSER_SCREENSHOT 64 CLICK_BROWSER_PIXEL 65 READ_TERMINAL
66 CAPTURE_BROWSER_CONSOLE_LOGS 67 READ_BROWSER_PAGE 68 BROWSER_GET_DOM
73 CODE_SEARCH  74 BROWSER_INPUT ... 125 BROWSER_REFRESH_PAGE
81 TASK_BOUNDARY 82 NOTIFY_USER 83 CODE_ACKNOWLEDGEMENT 84 INTERNAL_SEARCH
85 BROWSER_SUBAGENT 86..113 browser mouse/scroll/resize/drag/wheel
86 FILE_CHANGE 87 MOVE 90 EPHEMERAL_MESSAGE 91 GENERATE_IMAGE
92 DELETE_DIRECTORY 93-95 applet compile/install 98 CONVERSATION_HISTORY
99 KNOWLEDGE_ARTIFACTS 100 SEND_COMMAND_INPUT 101 SYSTEM_MESSAGE 102 WAIT
103 AGENCY_TOOL_CALL 104 CIDER_AGENT_DUMMY 105 BUILD_CLEANER
108 SET_UP_FIREBASE 111 DEPLOY_FIREBASE 112 SHELL_EXEC 114 LINT_APPLET
116 KI_INSERTION 122 WORKSPACE_API 126 EDIT_NOTEBOOK 127 INVOKE_SUBAGENT
128 WRITE_BLOB 129 READ_NOTEBOOK 130 PROPOSE_AI_COMMENTS 131 START_CODE_REVIEW
132 GENERIC 133 SET_UP_CLOUD_SQL 134 EXECUTE_NOTEBOOK 135 CLOUD_SQL_UPDATE_SCHEMA
136 RPC_ACTION 137 CLOUD_SQL_EXECUTE_SQL 138 ASK_QUESTION 139 DIRECTORY_RULES
140 TOOL_SEARCH
```

Derived, not encoded: `cli/steps/steps.DeriveStepType`, `DeriveToolName`,
`enumToolName` and `ExtractToolInfo` live in `cli/steps/steps` (`data/funcs.txt`), i.e.
the CLI *projects* proto steps into display rows. The CLI does not own the loop.

`CortexStepStatus` (12, exact): `0 UNSPECIFIED, 1 PENDING, 2 RUNNING, 3 DONE,
4 INVALID, 5 CLEARED, 6 CANCELED, 7 ERROR, 8 GENERATING, 9 WAITING, 10 ?, 11 QUEUED,
12 INTERRUPTED`. `Step` also carries `status`, `metadata`, `error`, `permissions`,
`task_details`, `custom_extensions`, `attachments`, plus `requested_interaction` and
`completed_interactions` (§3).

### 1.3 Trajectory kinds

`CortexTrajectoryType` (exact): `0 UNSPECIFIED, 1 USER_MAINLINE, 2 USER_GRANULAR,
3 SUPERCOMPLETE, 4 CASCADE, 6 CHECKPOINT, 11 APPLIER, 12 TOOL_CALL_PROPOSAL,
13 TRAJECTORY_CHOICE, 14 LLM_JUDGE, 16 BRAIN_UPDATE, 17 INTERACTIVE_CASCADE,
20 BROWSER, 21 KNOWLEDGE_GENERATION, 22 SIDE_QUESTION, 23 COMMIT_MESSAGE_GENERATION`.

Observed on disk: both conversation DBs on this machine hold exactly one
`trajectory_meta` row with `trajectory_type=4`, `source=17` — i.e. `CASCADE` +
`CORTEX_TRAJECTORY_SOURCE_CLI`.

`CortexTrajectorySource` (exact, 22 values): `1 CASCADE_CLIENT, 2 EXPLAIN_PROBLEM,
3 REFACTOR_FUNCTION, 4 EVAL, 5 EVAL_TASK, 6..10 ASYNC_PRR/CF/SL/PRD/CM,
12 INTERACTIVE_CASCADE, 13 REPLAY, 15 SDK, 16 SUBAGENT, 17 CLI, 18 JETBOX,
19 AGENT_API, 20 CIDER, 21 GEMINI_APP, 22 VSCODE, 23 VISUAL_STUDIO`.

`CortexTrajectoryReferenceType` (exact): `1 PRUNE, 2 FORK, 3 BATTLE_MODE,
4 THOUGHT_REWRITE, 5 BATTLE_MODE_REFERENCE_BACKUP, 6 SUBAGENT_FORK` — this is how a new
trajectory records which parent trajectory and step index it was derived from
(`ForkFromIdentifier{cascade_id, step_index, reference_type}`).

### 1.4 One turn, end to end

```
 user types a message
        |
        v
 [1] USER_INPUT step appended (CortexStepUserInput: query, items[], media, user_config)
        |
        v
 [2] executor loop iteration (ExecutorMetadata.num_generator_invocations++)
        |
        +--> prompt assembly: UnifiedPromptComponents
        |      chat_messages[] + cortex_trajectory + context_module_result
        |      + command_intent_info + code_diagnostics + knowledge_base_groups
        |      scaled by CumulativePromptConfig multipliers; "failed to build prompt
        |      sections: %w" (0x5ac3a64) aborts the iteration
        |
        +--> model request (ExecutionStatus.model_request{attempt,max_attempts,retry_at})
        |      response lands as PLANNER_RESPONSE step:
        |        CortexStepPlannerResponse{response, thinking, signature,
        |        thinking_signature, tool_calls[], stop_reason, ...
        |        grounding_metadata, recitation_metadata}
        |
        +--> if tool_calls empty  -> termination NO_TOOL_CALL
        |
        v
 [3] per tool call: a typed step is queued and executed
        |      permission / interaction needed?
        |        -> step.requested_interaction = RequestedInteraction
        |           surfaced to client; loop parks (CORTEX_STEP_STATUS_WAITING)
        |           client answers with CascadeUserInteraction -> completed_interactions
        |      else execute
        |
        +--> result recorded on the *same* step (e.g. CortexStepRunCommand.exit_code,
             .combined_output, CortexStepViewFile.content, CortexStepMcpTool.result_string)
        |
        v
 [4] results become prompt context on the next iteration (`SnapshotToStepsOptions`
     decides which step types are re-serialised into the prompt: step_type_allow_list,
     max_commands/max_command_age/per_command_max_bytes_output, max_steps_per_checkpoint,
     max_files_in_prompt, max_lines_per_file_in_prompt, max_browser_interactions)
        |
        v
 [5] loop back to [2] until an ExecutorTerminationReason fires
        |
        v
 [6] streamed state + SQLite persistence + UI render
```

Step [4] is the key non-obvious part: tool results are **not** a separate "tool result"
message. They are fields on the tool step, and a separate projection
(`SnapshotToStepsOptions` → `SnapshotToSteps`) decides what of the trajectory is
re-serialised into the next prompt. That is why history compaction
(`CortexStepCheckpoint`, `CompactionInfo.compacted_at_step_indices`) is cheap: it changes
the projection, not the store.

### 1.5 Streaming to the UI

Two transports exist for the same state model:

* **In-process / sidecar** — `exa.language_server_pb.LanguageServerService` streams;
  `StreamAgentStateUpdates(conversation_id, subscriber_id, initial_steps_page_bounds,
  trajectory_verbosity, …) -> AgentStateUpdate`.
* **Remote/cloud proxy** — `StartCascade`, `SendUserCascadeMessage`,
  `StreamAgentStateUpdates` on `google.gca.aicode.v1main` (remote_agent_proxy.proto), with
  `resume_token` and `pending_agent_messages_sequence_number` for reconnect.

`AgentStateUpdate` is a delta envelope, not a snapshot: `main_trajectory_update`,
`subtrajectory_updates{}`, `step_scoped_subtrajectory_updates{}`, `queued_steps_update`,
`artifact_snapshots_update`, `trajectory_file_diffs_update`, `background_tasks_update`,
`background_commands_update`, `pending_agent_messages_update`,
`accumulated_sections_update`, `turns_update`, `battle_mode_infos_update`,
`compaction_info`, `cost_summary`, `context_window_usage`, `active_hook`, `step_hooks{}`,
`execution_status`, `fully_idle`, `open_elsewhere`.

Each `*Update` is `{indices[], values[], total_length, page_bounds}` — index-addressed
pages so a client can patch a long trajectory without re-receiving it.

**Turns** are a UI grouping, not a protocol construct: `TurnsUpdate{turns[],
total_length}`, `TurnHeader{index, start_step_index, start_time}`,
`TurnSummary{start_step_index, first_step, response_preview}`. A turn starts at a
`USER_INPUT` step.

`ClientTrajectoryVerbosity` (`1 DEBUG, 2 PROD_UI, 3 FULL`) gates how much detail is
streamed; `Slice{start_index, end_index_exclusive}` pages step arrays.

Klog anchors for the lifecycle (observed):
`server.go:1182] Creating new cascade trajectory`, `server.go:1225] Created conversation
<uuid> (backend=BACKEND_TYPE_UNSPECIFIED)`, `server.go:1243] Starting conversation update
stream`, `server.go:1300] Stream goroutine exited, sending completion signal`,
`conversation_manager.go:1083/1222] Streaming conversation / Stream completed, clearing
ResponsePending`.

**Prompt sections.** `prompt_pb.UnifiedPromptComponents` is the concrete section list.
`CumulativePromptConfig` carries the budget arithmetic (`persistent_context_multiplier`,
`trajectory_context_multiplier`, `ephemeral_context_multiplier`,
`intent_reservation_tokens`, `persistent_max_tokens_per_open_doc`,
`allowed_cascade_step_types[]`, `trajectory_start_index/increment`). Which sections may
be overridden by a user/prompt file is `SectionOverrideMode` (`OVERRIDE|APPEND|PREPEND`),
and `PROMPT_SECTION_SOURCE_TYPE_TEMPLATE_OVERRIDE` appears in rodata.

Recovered from rodata: a large embedded reference document ("Antigravity 2.0 Reference",
28,494-byte printable run at `0x5cadc19`) that is injected into the model's context and
describes the agent's own UI, settings and permission modes to itself. Prompt content is
therefore partly a *document about the product*, not only task state.

---

## 2. Tool surface

### 2.1 How tools are represented

Each tool is a `CortexStepType` + a payload message + a Go "tool converter" that (a)
renders the tool declaration sent to the model, (b) parses the model's call into the
typed step. Converters found in `data/funcs.txt` under `jetski/cortex/tools`:

`ViewFile, ListDir, Find, GrepSearch, RunCommand, SedFile, ReplaceFileContent,
ReplaceFileContentForTab, MultiReplaceFileContent, SingleReplaceFileContent, WriteToFile,
McpTool, CallMcpTool, ReadResource, ListResources, SearchWeb, GenerateImage, Finish, Wait,
WaitFiveSeconds, SendMessage, SendCommandInput, CommandStatus, AskQuestion, AskPermission,
AskCustomPermission, ListPermissions, ManageTask, ManageInbox, Schedule, ToolCaller,
SplitCompletionProvider`; `subagent.{InvokeSubagent, DefineSubagent, ManageSubagents}`;
`notebook.{NotebookEdit, NotebookExecution}`; `tooloverrides.ToolOverrideMapper`;
`code.BaseToolConverter`, `notebook.BaseToolConverter`.

Literal model-facing tool-name strings observed in `.rodata` / log format strings (these
are *observed*, not derived from the converter names):

```
str_replace_editor   multi_replace_file_content   replace_file_content
create_file          delete_file                  edit_file        file_write
run_shell_command    invoke_subagent              define_subagent
```

(`str_replace_editor`, `multi_replace_file_content`, `replace_file_content` from
`deterministic_vetting_configs.vetTool` strings at `0x59f12de/0x5a64deb/0x5a03480`;
`create_file`/`delete_file`/`edit_file`/`file_write` from the PolicyGuardian log lines
`[PolicyGuardian] Deterministic pre-check: create_file target=%s` etc.;
`run_shell_command` from the `denied_repeat` rule table at `0x59de710`.)

`TOOL_FORMATTER_TYPE_*` (`UNSPECIFIED, NONE, HERMES, XML, LLAMA_3, CHAT_TRANSCRIPT`) is the
**wire encoding of tool calls for non-Gemini models**, not a tool taxonomy — it exists
because Claude/GPT-OSS models on this account need Hermes/XML/Llama-3 tool-call syntax
instead of Gemini `functionCall` parts. `CHAT_TRANSCRIPT` suggests whole-transcript tool
formatting. No descriptor for this enum is embedded, so the numbers are unresolved.

### 2.2 Inventory

Model-facing name ← `CortexStepType` (number where recovered). **Conf** = requires user
confirmation in at least one mode (evidence: a `*InteractionSpec` exists in
`RequestedInteraction`, or the step carries `should_auto_run`/`user_rejected`).

| group | tool | step type | payload message | conf |
|---|---|---|---|---|
| files | `view_file` | 8 VIEW_FILE | `CortexStepViewFile{absolute_path_uri,start_line,end_line,content,...,file_permission_request}` | yes (outside workspace) |
| | `read_file` | – | `tools.ReadFile{file_num_lines,file_num_bytes,path,start_line,end_line,line_range_bytes}` | yes |
| | `list_dir` | 9 LIST_DIRECTORY | `CortexStepListDirectory{results[],children,dir_not_found,file_permission_request}` | yes |
| | `find` | 25 FIND | `CortexStepFind{pattern,includes,excludes,type,max_depth,raw_output}` | yes |
| | `grep_search` | 7 GREP_SEARCH | `CortexStepGrepSearch{query,includes,match_per_line,case_insensitive,is_regex,allow_access_gitignore,results[]}` | yes |
| | `create_file` / `file_write` | 86 FILE_CHANGE / 23 write | `FileEdit{kind:CREATE\|OVERWRITE\|EDIT,path,diff,diff_stats}` | yes |
| | `edit_file` / `str_replace_editor` / `replace_file_content` / `multi_replace_file_content` | 5 CODE_ACTION, 86 FILE_CHANGE | `ActionSpec`/`ActionResult`/`ReplacementChunkInfo` + `file_permission_request` | yes |
| | `delete_file` / `delete_directory` | 92 DELETE_DIRECTORY | `{directory_path_uri,force}` | yes |
| | move | 87 MOVE | `{src_absolute_path_uri,dst_absolute_path_uri}` | yes |
| | `view_file_outline`, view_code_item, find_all_references | 47, 13, 54 | `CortexStepViewFileOutline` etc. | no |
| shell | `run_command` / `run_shell_command` | 21 RUN_COMMAND | `CortexStepRunCommand` (see §4) | yes |
| | `send_command_input` | 100 | `{command_id,input,terminate,wait_ms,output,running,exit_code}` | yes |
| | `command_status` | 28 | `{command_id,status,combined,delta,exit_code,output_priority}` | no |
| | `read_terminal` | 65 | `{process_id,name,contents}` | no |
| | shell exec | 112 SHELL_EXEC | `{command,exit_code,output}` (IDE-hosted) | yes |
| search | `code_search` | 73 | `CortexStepCodeSearch` | no |
| | `internal_search` | 84 | `{query,results[{url,title,content}]}` | no |
| | `trajectory_search` | 60 | search past conversations | no |
| | `tool_search` | 140 | `CortexStepToolSearch{function_names,response}` — deferred MCP tool discovery | no |
| web | `search_web` | 33 | `{query,domain,web_documents,summary,third_party_config,search_type(WEB\|IMAGE)}` | no |
| | `read_url_content` | 31 | `{url,web_document,resolved_url,latency_ms,user_rejected}` | **yes** (`CascadeReadUrlContentInteraction`) |
| browser | `open_browser_url` | 56 | `{url,page_id,auto_run_decision,user_rejected,screenshot}` | **yes** |
| | `execute_browser_javascript` | 61 | `{page_id,javascript_source,javascript_result,waiting_reason}` | **yes** |
| | `capture_browser_screenshot` | 63 | `{page_id,save_screenshot,screenshot_name,capture_beyond_viewport}` | **yes** |
| | `click_browser_pixel` | 64 | `{page_id,x,y,click_type}` | **yes** |
| | `read_browser_page`, `browser_get_dom`, `list_browser_pages`, `capture_browser_console_logs` | 67, 68, 62, 66 | DOM/screenshot payloads | no |
| | `browser_input`, `_move_mouse`, `_select_option`, `_scroll_up/down`, `_click_element`, `_press_key`, `_scroll`, `_resize_window`, `_drag_pixel_to_pixel`, `_mouse_wheel`, `_mouse_up/down`, `_refresh_page`, `_list_network_requests`, `_get_network_request` | 74–125 | each `{page_id, …, page_metadata, browser_state_diff}` | mixed |
| | `browser_subagent` | 85 | `{task,result,task_name,recording_path,recording_generation_status,subagent_id,scratchpad_path}` | no |
| mcp | `call_mcp_tool` / `mcp_<server>_<tool>` | 38 MCP_TOOL | `CortexStepMcpTool{server_name,tool_call,result_string,result_uri,images,media,app_view,progress}` | **yes** (`CascadeMcpInteraction`) |
| | list/read resources | 51, 52 | `McpResource{uri,name,description,mime_type}` | no |
| subagent | `invoke_subagent` | 127 | `CortexStepInvokeSubagent{subagents[],task_mode,results[]}` | no |
| | `define_subagent` | – | `DefineSubagent{name,tool_names,agent_path}` | no |
| | `manage_subagents` (kill/status) | – | `ManageSubagents{action,conversation_ids,subagents{conversation_id,step_index}}` | no |
| | `send_message` | 101 SYSTEM_MESSAGE/AgentMessage | `SendMessage{recipient,recipient_name,message}` | no |
| task | `manage_task`, `manage_inbox`, `schedule` | – | `ManageTask{action,task_ids,input}`, `Schedule{mode,outcome,iterations,cancelled_by}` | no |
| memory | `retrieve_memory` | 34 | `CortexStepRetrieveMemory{run_subagent,cascade_memory_summary,user_memory_summary,retrieved_memories,blocking}` | no |
| | memory write | 29 MEMORY | `CortexStepMemory{memory,prev_memory,action}` | no |
| | `brain_update` | 55 | `CortexStepBrainUpdate{entry_type(PLAN\|TASK),trigger,deltas[]}` | no |
| knowledge | `generate_image` | 91 | `{prompt,image_paths,aspect_ratio,image_name,model_name}` | no |
| | knowledge artifacts / generation | 99, 89 | – | no |
| misc | `finish` | 2 FINISH | `Finish{arguments_json,has_result_schema}` | no |
| | `wait`, `wait_5_seconds` | 102 | `{duration_ms}` | no |
| | `notify_user` | 82 | `{review_absolute_uris,notification_content,is_blocking,confidence_score,diffs_uri,ask_for_user_feedback}` | no |
| | `ask_question` | 138 | `AskQuestion{questions[]}` | always (it *is* a prompt) |
| | `ask_permission`, `ask_custom_permission`, `list_permissions` | – | permission surface | – |
| | `run_extension_code` | 57 | `{code,language,user_facing_explanation}` | **yes** |
| | `clipboard` | 45 | `{content}` | no |
| | `workspace_api` | 122 | `{url,http_method,body,description,status_code,response}` | yes |
| | `agent tool call` | 103 | `CortexStepAgencyToolCall{agent_name,function_name,…}` | no |
| | notebook read/edit/execute | 129/126/134 | typed args + replies | yes (exec) |

`BROWSER_TOOL_SET_MODE` (`0 UNSPECIFIED, 1 ALL_TOOLS, 2 PIXEL_ONLY,
3 ALL_INPUT_PIXEL_OUTPUT`) and `BrowserSubagentMode` (`MAIN_AGENT_ONLY,
SUBAGENT_PRIMARILY, BOTH_AGENTS, SUBAGENT_ONLY`) select *which* browser tools the model is
offered and whether a dedicated browser agent owns the browser. `mcp_chrome_devtools_*`
names appear in the binary per the task brief; I did not recover a literal instance, so
the prefix convention for built-in MCP servers is **unresolved** (see
`telemetry.GetBuiltinMCPServerPrefixes`, which exists but was not disassembled).

Many step types are IDE/Google3-only (`MOMA`, `BLAZE_*`, `CIDER_AGENT_DUMMY`,
`WORKSPACE_API`, `AGENCY_TOOL_CALL`, `SET_UP_FIREBASE`, `DEPLOY_FIREBASE`,
`SET_UP_CLOUD_SQL`, applet compile/install, `POST_PR_REVIEW`, `START_CODE_REVIEW`). The
CLI enables a subset through `CumulativePromptConfig.allowed_cascade_step_types[]` and
`allowed_implicit_step_types{}`.

---

## 3. Permissions and approval

### 3.1 The preset (global mode)

`exa.codeium_common_pb.AgentPermissionPreset` — exact numbers from the descriptor at
`0x61f35c6`:

```
0 UNSPECIFIED   1 DEFAULT   2 REQUEST_REVIEW   3 TURBO   4 VETTED   5 NONE   6 AUTO
```

Read/written by `language_server_pb.GetPermissionSettings` / `UpdatePermissionSettings`
(`{preset, artifact_review_mode, grants}`), and also exposed over the remote agent proxy
as `SetPermissionPreset{scope: GLOBAL|PROJECT, project_id, preset}`.
`ArtifactReviewMode` (`0 UNSPECIFIED, 1 ALWAYS, 2 TURBO, 3 AUTO`) is the parallel
control for reviewing file artifacts.

The human-facing names for the tool-execution policy are in the embedded reference doc
(`0x5cadc1f`): `always-proceed`, `request-review`, `strict`, `proceed-in-sandbox`; the
CLI's own settings value is the klog-reported `toolPermission=request-review`
(`cli_setting_manager.go:92`). `settings.json` on this machine holds
`{"agentMode":"accept-edits", …}`, and the embedded changelog (`0x61c3c70`) documents the
user-visible `--mode` ladder ``default` -> `accept-edits` -> `plan` `` plus "Added
`request-review` (default) mode … automatically pauses before file write operations".
Reconciling the two vocabularies: `--mode`/`agentMode` selects the **agent mode**
(`AGENT_MODE_PLANNING|EXECUTION|VERIFICATION` in `cortex.proto`, plus the CLI's
`accept-edits`/`plan`/`auto`), while the **preset** decides whether a step needs
approval. They are separate fields and both must be modelled.

`AutoRunDecision` (`cortex.proto`) records *why* a command ran or did not:

```
0 UNSPECIFIED 1 USER_ALLOW 2 USER_DENY 3 SYSTEM_ALLOW 4 SYSTEM_DENY
5 MODEL_ALLOW 6 MODEL_DENY 7 DEFAULT_ALLOW 8 DEFAULT_DENY
```

### 3.2 The grant record ("granule")

Two nested shapes, both flat string lists at the bottom:

```protobuf
// exa.codeium_common_pb
message PermissionGrantsConfig {
  repeated string allow = 1;
  repeated string deny  = 2;
  repeated string ask   = 3;
}

// exa.cortex_pb
message PermissionResource { string action = 1; string target = 2; }
message PermissionScope { ONCE=1, CONVERSATION=2, WORKSPACE=3, GLOBAL=4, PROJECT=5 }
message PermissionGrant {
  PermissionResource resource = 1;
  bool allow = 2;
  PermissionScope scope = 3;
  string user_deny_instruction = 4;
  string action = 5;
  string target = 6;
}
```

So a grant is a triple *(action, target, allow)* with a scope ladder, and the persisted
config is three string lists whose entries are the `action`/`target` patterns. The
project-scoped container is `project_pb.Project.permission_grants ->
PermissionGrants{permission_grants: PermissionGrantsConfig, v2_migrated: bool}` (field 9
of `Project`), i.e. grants live **on the project record**, matching the klog
`ApplyProjectPermissionGrants: no grants for project "CLI Project", cleared project
permissions` (`cli_setting_manager.go:218`). `v2_migrated` marks the migration from the
old per-action grants to the `PermissionGrantsConfig` lists. `ANTIGRAVITY_PERM_GRANTS`
(rodata `0x5a3daa7`, `0x5a4bb5a` `ANTIGRAVITY_PERM_GRANTS=`) overrides the grants blob
from the environment. `ListPermissionsToolConverter` + the string
`No permission grants found.` (`0x5a79731`) are the model-visible read-only view.

### 3.3 How a prompt is decided

The decision is *per step*, at step execution time, and is expressed as a proto request
that the client must answer. `Step.requested_interaction` is a `RequestedInteraction`
oneof (`cortex.proto`):

```
permission(21), elicitation(20), ask_question(22),
run_command(3), deploy(2), open_browser_url(4), run_extension_code(5),
execute_browser_javascript(7), capture_browser_screenshot(8), click_browser_pixel(9),
browser_action(13), open_browser_setup(14), confirm_browser_setup(15),
send_command_input(16), task_resolution
```

Each has a `…InteractionSpec` (what to show) and, later, a `…Interaction` (the answer).
The answer path is `CascadeUserInteraction{trajectory_id, step_index, timed_out,
remote_control_details, …}` → recorded in `Step.completed_interactions[]`, so the whole
history of approvals is part of the trajectory.

`PermissionInteractionSpec` is the permission prompt's content:

```
resource(PermissionResource{action,target}), suggested_persist_pattern,
persist_suggestion_type(SUGGESTED|BLOCKED), reason, action_description,
trigger_source{type, detail}, shell_name, approval_scope_filter[]
```

`TriggerSource.Type` is the decision-provenance enum — this is the answer to "how a
permission prompt is decided":

```
1 EFFECTIVE_GRANT   2 DEFAULT_GRANT   3 CROSS_PROJECT_ISOLATION
4 SECURE_MODE       5 DEFAULT_FALLBACK  6 FORCE_ASK_HOOK
```

i.e. in order: an existing grant that matches this action/target wins; otherwise the
preset's default grant; otherwise cross-project isolation or secure mode force a prompt;
otherwise fall back to the default; and a hook may force-ask regardless. `PersistSuggestionType`
is `SUGGESTED` (offer "always allow") or `BLOCKED` (never offer it).

The answer, `PermissionInteraction`, is
`{allow, scope, edited_target, persist_grants(PermissionGrantsConfig),
user_deny_instruction, sandbox_override}` — note `edited_target` (the user may rewrite the
command/path being approved) and `sandbox_override` (approval can force the command out of
the sandbox, or into it).

File-scoped prompts do not use the generic mechanism. They attach
`FilePermissionInteractionSpec{absolute_path_uri, is_directory, block_reason}` directly to
the step (`CortexStepViewFile.file_permission_request`,
`CortexStepListDirectory.…`, `CortexStepCodeAction.file_permission_request`,
`CortexStepViewFileOutline.…`, `CortexStepGrepSearch.…`), with
`BlockReason = 1 OUTSIDE_WORKSPACE, 2 GITIGNORED`. So *outside-workspace* and *gitignored*
are the two structural reasons a read/write is blocked, and they are produced by the tool
executor, not by a policy engine.

`list_permissions` / `ask_permission` / `ask_custom_permission` converters mean the model
can inspect and *request* grants as a tool call.

### 3.4 Flags

* **`--dangerously-skip-permissions`** — derived: the preset `NONE` (5) is the "no
  prompts" state and `AUTO` (6) the "auto-approve" state; the flag is the CLI's way to set
  that and to stamp every `AutoRunDecision` as a system decision. I did not recover the
  flag's exact effect on the wire; treat "sets preset to NONE/AUTO and suppresses
  `RequestedInteraction` delivery" as **derived, unverified**.
* **`--mode {plan,auto,accept-edits}`** — sets the agent mode; `accept-edits` is observed
  in `settings.json` as `agentMode`. In `request-review` presets, plan mode stops before
  writes; `accept-edits` auto-applies file edits but still prompts for shell.
* **`allowNonWorkspaceAccess`** gates exactly `FilePermissionInteractionSpec.BlockReason
  = OUTSIDE_WORKSPACE` and the `FileAccessPolicy` enum
  (`1 DENY, 2 ALWAYS_ASK, 3 NOT_ENFORCED`) in `FetchAdminControlsResponse.AgentControls`
  — the *admin* copy of the same knob, alongside `sandbox_mode_enabled` and
  `terminal_command_auto_execution_policy` (`REQUIRE_REVIEW | PROCEED_IN_SANDBOX |
  NOT_ENFORCED`). These three fields are the server-pushed (enterprise) policy; the
  project-level duplicates live in `project_pb.ProjectSettings{file_access_policy,
  internet_policy, sandbox_mode, auto_execution_policy, artifact_review_mode,
  enable_permissioned_github, shell_setup_script, permission_preset,
  security_plugins{}}`.

### 3.5 PolicyGuardian

`cortex/policyguardian` is a **vetting layer in front of destructive/network tools**, and
it is model-backed, not just a matcher:

* `vet_destruction: no judge model API configured` (`0x5b06914`)
* `vet_destruction judge denied target=%s: deny_conditions=%s (verdict=%q)` (`0x5b779ab`)
* `vet_destruction: judge response could not be parsed after %d attempts: %w` (`0x5b63d38`)

So `vet_destruction` calls an LLM judge, gets a structured verdict with
`deny_conditions`, and fails closed on parse failure after N attempts.

Its **state** (`policyguardian/state.proto`, descriptor `0x5c1fdc0`) is the model of the
environment it defends:

```protobuf
message EnvState {
  repeated string untrusted_downloads, modified_files, executable_files, created_files;
  map<string,string> symlinks, env_changes;
  repeated DeniedAction denied_actions;   // {tool_name, args(Struct), reason}
}
message GuardianState {
  EnvState state; map<int32,PendingEnvUpdates> pending_state;
  repeated UserTurn user_turns;   // {index, text, prior_assistant_text}
  int32 user_turn_count;
}
```

i.e. it tracks downloads, symlinks, `PATH`/env mutations, newly-executable files,
created/modified files, and prior denials — and `UserTurn.prior_assistant_text` means the
vetting decision is conditioned on the conversation.

Deterministic pre-checks exist per tool, from
`cortex/policyguardian/deterministic_vetting_configs`:
`VetCreateFile, VetDeleteFile, VetEditFile, VetFileDelete, VetFileWrite,
VetReplaceFileContent, VetMultiReplaceFileContent, VetStrReplaceEditor, VetDependency,
Vet`, and each logs `[PolicyGuardian] Deterministic pre-check: <tool> target=%s`
(`create_file` `0x5b49190`, `delete_file` `0x5b491cf`, `edit_file` `0x5b42ba3`,
`file_write` `0x5b46065`).

Rule-id taxonomy recovered from `.rodata` (each is a verdict reason string):

| namespace | rules |
|---|---|
| `vet_destruction` / `destruction.*` | `destruction.recursive_system_path`, `destruction.recursive_dangerous_target`, `destruction.sensitive_path`, `destruction.vendor_path`, `destruction.user_home_critical`, `destruction.persistence_path`, `destruction.guardrail_tampering`, `destruction.heredoc_guardrail_write`, `destruction.read_only`, `destruction.judge_denied`, `destruction.judge_approved`, `destruction.judge_needs_confirmation`, `destruction.adjudicated_irreversible`, `destruction.adjudicated_reversible`, `destruction.verdict_deny`, `destruction.system_path`, `destruction.invalid_args` |
| `vet_data_flow` / `data_flow.*` | `source_invalid_github_url`, `source_github_api`, `source_github_search_api`, `source_allowlisted_url`, `source_fetch_failed`, `source_fetch_failed_read_only`, `source_local_ip`, `source_unverified_ip`, `source_placeholder_host`, `source_tool_config_download`, `dest_allowlisted_url`, `dest_invalid_url`, `download from allowlisted URL`, `local_flow_approved`, `flow_approved`, `read_only_fetch`, `remote_script_execution`, `execute_downloaded_file`, `local_exfiltration`, `semantic_exfiltration`, `sensitive_file_exfiltration`, `sensitive_file_read_as_secrets`, `persistence_path_write`, `system_config_modification`, `uri_contains_secrets`, `uri_contains_data`, `data_in_domain`, `invalid_args` |
| `vet_irreversible` | `destruction.*` verdicts above |
| `denied_repeat` | `not_repeat`, `is_repeat`, `llm_error`, `model_unavailable`, `blocked` |
| `dependency` | `invalid_args` |
| others | `violates_user_constraint`, `crypto.credential`/`credential.in_address`, `shell_change.*`, `exit_code.*` |

Two hooks are named in surviving strings: `NewHooks` and
`Guardian.executeToolCallsParallel` (batched vetting), plus `Suite.runGAVSCheck` (Google
Artifact/URL verification), `Suite.fastDeny`, `doWithBackendRetry`,
`vetting.HostSurface`, `vetting.VetDependency`, `profiles.*` (a profile system of
`RegexRuleMatcher`, `ActionClass.IsHazard`, `Paths.IsGuardrailPath/IsScratchPath/
IsVendorPath/IsUserHomeCritical/IsConstraintExemptPath`, `ScriptContentRoutes`,
`RedLineMatches`, `SplitSegments`, `IsReadOnlySegment`) — i.e. shell commands are
segmented, matched segment-by-segment against regex profiles, and only hazardous segments
go to the judge.

**What is enforced client-side:** every deterministic pre-check and every `RequestedInteraction`
is client-side; PolicyGuardian's judge is a *model call from the client* whose verdict
the client enforces. There is no evidence of a server-side veto for these tools (the
remote proxy only relays `PermissionSettings`). The `security_plugins{cli, vetted}`
switches in `ProjectSettings` gate the two layers independently.

---

## 4. Sandboxing

### 4.1 Two different things are called "sandbox"

1. **Terminal sandbox** (`--sandbox`, project `sandboxMode`, admin
   `sandbox_mode_enabled`): wraps the *commands the agent runs*.
2. **Sandbox proxy** (`cortex/command/sandboxproxy`): an HTTP(S) egress proxy + CA used
   *inside* that sandbox so commands get network without getting the host's credentials.

### 4.2 What is actually applied on Linux

The isolating implementation is `google3/devtools/ai/sandbox/*`:

| package | functions (observed) | role |
|---|---|---|
| `sandbox/exebox` | `jailMain`, `installUnotifyHooks`, `forwardConn`, `escapeExec`, `buildFileTree` | the jailed child process; executes the command, relays fds/sockets |
| `sandbox/sbox` | `acceptPipe`, … | sandbox supervisor/handshake |
| `sandbox/selfexec` | `Invocation` | re-execs the binary as its own jailer |
| `sandbox/urpc` | 28 funcs | descriptor-passing RPC over `unix:path=` sockets |
| `sandbox/proxy` | `bufferedConn`, … | in-sandbox network proxy plumbing |
| `sandbox/localcert` | 4 funcs | per-sandbox CA material |

Mechanisms actually present in the binary:

* **seccomp BPF**: `seccomp BPF: %v` (`0x5a67e9c`), `seccomp(SET_MODE_FILTER, TSYNC): %v`
  (`0x5abff77`), `seccomp(SET_MODE_FILTER, NEW_LISTENER): %v` (`0x5aee8bd`), `seccomp: %v`
  (`0x59468f8`), `assembling unotify BPF: %v`, `installUnotifyHooks`, and the env switch
  `GOOGLE_DISABLE_SECCOMP_POLICY` (`0x65504a6`). `TSYNC` applies the filter to all
  threads; `NEW_LISTENER` + `installUnotifyHooks` is the **seccomp user-notify**
  supervisor pattern (the jailer intercepts selected syscalls and decides in userspace).
* **user namespaces**: the runtime's own message
  `runtime: user namespaces are disabled (user.max_user_namespaces = 0)` (`0x5b3acb4`) and
  the variant `(kernel.unprivileged_userns_clone = 0)` (`0x5b6ea90`). This is Go's
  `syscall`/`os/exec`-adjacent runtime text emitted when namespace creation is refused —
  i.e. the sandbox *attempts* a user namespace and fails here because the host has
  `user.max_user_namespaces = 0`.
* **bubblewrap**: `.rodata` holds the source names `bwrapper.cc` (`0x654ca63`) and
  `bwrapper_footer_flag.cc` (`0x654d469`) — a bwrap-based launcher is compiled in, so the
  isolation itself is bwrap's (mount namespace + pivot_root + bind mounts), with seccomp
  layered on top.
* **no landlock**: the string `landlock` does not occur anywhere in the binary (grep
  returned nothing). Landlock is **not** used.
* **not gVisor**: only the unrelated `gvisor/kernel_is_gvisor` probe from a third-party
  library.

Reconciling the two clues: `GOOGLE_DISABLE_SECCOMP_POLICY` is the *opt-out switch for the
seccomp policy* (Google-internal estates set it where the filter blocks legitimate work);
`user.max_user_namespaces = 0` is a *host kernel* state that makes the bwrap/userns path
fail. On this machine both apply — so `--sandbox` degrades. What the user sees then is the
`[3]`-style failure path rather than a hard error, because
`CortexStepRunCommand.ran_in_sandbox` is a plain bool and `sandbox_errors` a string: a
sandbox that cannot start records `ran_in_sandbox=false` plus the reason and (per
`run_command_step_creation_options` / `AutoRunDecision.DEFAULT_ALLOW`) may still run the
command. `sandbox_bypass_disabled` and `sandboxed_trusted_cli` are the two fields that
record whether a bypass was refused.

`sandbox: denied %s %s` (`0x5a2bec3`) is the **mount/file-policy denial** message: two
arguments, an operation and a path. The policy that produces it is
`command.trustedCliBoxFilePolicy` with `pathWithin`, `demoteWritableMountsIfReadOnly`,
`destinationBinaryMounts`, `readOnlySandboxScratch`, `keepWorkspaceAndCredCaches`,
`isLOASCredCachePath`, `isSocketOwnedByUser`. Net effect: the sandbox's writable set is
the workspace + a read-only scratch + credential caches; host binaries are bind-mounted
read-only (`destinationBinaryMounts`, `registerLauncherShims`, `pluginShimEnv`).

### 4.3 The sandbox proxy

`sandboxproxy` is a local HTTPS proxy the sandboxed command is forced through, it:

* terminates TLS with a per-session CA (`command.initCA`, `sandbox/localcert`),
* enforces per-request rules in **CEL** (`celEnforcer.Check/AddExpr/CheckResponse`,
  `envHandlePresentRLocked`, `implicitAllowRLocked`, `globFunc`, `andExprs`), keyed by
  *which credential env var is present* (`buildEnvVarHandles`, `EnvHandles`,
  `credFingerprintFor`) — so the rule set tightens when a token is in the environment,
* **injects auth** rather than exposing secrets (`BasicAuthConfig.Inject`,
  `injectAuthRLocked`, `markNeedsAuth`, `needsAuthError`, `OAuth2Config.performOAuthFlow`,
  `accountTokenSource.Token`, `adoptAccountIdentity`, `accountMatches`,
  `sandboxproxy.Account`, `doIdentityProbe`),
* understands specific protocols so it can allow-list semantically:
  `asJSONFunc`, `asGraphQLFunc`, `asURLEncodedFunc`, `asGitReceivePackFunc`,
  `trustedCliBoxAllowCEL`, `trustedCliBoxAllowRPCs`, `trustedCliBoxProxyAllowList`,
  `trustedCliBoxProxyAllowRPCList`, `trustedCLINetboxAuth`,
  `discoverConnectdSocket`/`forwardConnectdSocket`/`validateConnectdSocket`
  (LOAS/RPC passthrough), `trustedCliBoxFilePolicy`,
* audits: `BlockedRequests`, `recordBlock`, `IsRPCSandboxProxyEnabled`,
  `IsRPCSandboxProxyAuditMode`, `collectBlockedRequestMessage`.

`credentialkey` sub-package derives stable credential fingerprints, and
`ANTIGRAVITY_CREDENTIAL_SCOPE` (rodata `0x5a7da1c`) scopes them.

The `RunCommand` payload records the whole story:
`sandbox_override, ran_in_sandbox, sandbox_errors, sandbox_bypass_disabled,
sandboxed_trusted_cli, is_trusted_cli, should_auto_run, auto_run_decision, is_daemon,
run_persistent, blocking, wait_ms_before_async, notification_timeout_seconds`.

---

## 5. Subagents and battle mode

### 5.1 Subagents

Tools: `invoke_subagent`, `define_subagent`, `manage_subagents`, plus `send_message`
(a peer message between agents — `AgentMessage{id, recipient, sender, priority, content,
step_payload, delivery_strategy, hide_from_user}`).

```protobuf
message SubagentSpec {
  string type_name; string role; string initial_prompt;
  bool inherit;            // inherit the caller's context
  string branch;           // workspace branch for the child
  string workspace_uri;    // isolation target
  string share;
  Model model; ModelTier model_tier;
  bool run_as_task; bool fork;
}
message SubagentMetadata { conversation_id; SubagentDescriptor{type_name, role};
                           SubagentState state; int32 spawn_step_index;
                           repeated string workspace_uris; }
message SubagentResult { conversation_id; string log_absolute_uri; repeated string workspace_uris; }
enum SubagentState { 0 UNSPECIFIED, 1 ALIVE, 2 KILLED }
```

`ModelTier` (`cortex.proto`, exact): `0 UNSPECIFIED, 1 INHERIT, 2 FLASH_LITE, 3 FLASH,
4 PRO` — `MODEL_TIER_INHERIT` means "use the parent's model", which is why the enum exists
at all (a subagent's model is a *relative* choice). `run_as_task=true` turns the subagent
into a background task (`BackgroundTask{task_snapshot, step}`,
`TaskSnapshot{task_id, tool_name, tool_summary, description, start_time, step_index,
log_uri, is_daemon}`).

Spawn/isolation, derived and partially observed: each subagent is
(a) a **new conversation** (`SubagentMetadata.conversation_id`, `AgentMessage.recipient`
is a conversation id, `ManageSubagents{action, conversation_ids[]}`), (b) a **new
trajectory** whose `parent_references[]` carries
`CORTEX_TRAJECTORY_REFERENCE_TYPE_SUBAGENT_FORK` or `…_FORK` with a `step_index`, and
(c) optionally its **own workspace** (`workspace_uri`, `workspace_uris`, `branch`,
`fork`). Termination is `kill` (`SubagentState.KILLED`, `SUBAGENT_STATE_KILLED` in
rodata at `0x5aa47xx`; the function `KillSubagent` does not exist by that name — the
action is `manage_subagents{action:…}`). `CascadeState.has_active_children` and
`CascadeTrajectorySummary.has_running_subagents` are the liveness signals, and
`ensureNoRunningTasksOrSubagents` on `CascadeManager` gates operations (battle mode, model
switch) on it.

Checkpoints snapshot children: `CortexStepCheckpoint{subagent_snapshots[],
total_subagent_count, running_task_snapshots[]}` — so a compaction does not lose the
child registry.

### 5.2 Battle mode ("best of n")

Driven from `cortex/cortex.(*CascadeManager)`, which is otherwise about forking and
workspaces: `ValidateBattleMode`, `EnsureBattleModePreconditions`, `StartBattleMode`,
`setupBattleMode`, `prepareBattleModeForks`, `prepareBattleModeWorkspaces`,
`forkConversationForBattleMode`, `forkTrajectoryForBattleMode`,
`adjustCascadeConfigForBattleMode`, `BattleModePinnedModel`, `EliminateBattleModeArm`,
`EndBattleMode`, `endBattleModeInternal`, `applyBattleModeWinner`,
`mergeBattleModeConversation`, `mergeBattleModeWorkspaces`,
`mergeWorkspacesWithStrategy`, `partitionBattleModeWorkspaces`,
`promoteBattleModeWorktree`, `discardWinnerWorkspaces`, `remountBattleModeOverlays`,
`backUpOriginalState`, `restoreOriginalStateFromReference`,
`revertOriginalConversationToFork`, `copyArtifacts`, `forkArtifactDirectory`,
`artifactDirHasGit`, `DetectBattleModeAutoTrigger`, `unreplaceSyntheticSubagents`,
`appendTrajectorySuffix`.

Workspace forking is `cortex/battleworkspace.(*gitForker)`:
`CaptureBase`, `Fork`, `MergeBack`, `Finalize`, `Cleanup`, `MaxForks`,
`SupportsHistoricalFork`, `applyWinnerPreservingState`, `repoRootLocalPath`,
`BattleClonesBaseDir`, `NewResolver`, `isCitcWorkspace`, over a `realGitOps`
(`CaptureUncommittedState`/`ApplyUncommittedState`, `ApplyPatchString`,
`GenerateCommittedDiffFromBase`, `GenerateFullDiffFromBase`, `CloneLocalRepo`,
`GetBattleBaseRef`, `IsAncestor`, `CheckoutTarget`, `CurrentBranch`, `FetchRemote`,
`GitHeadCommit`). So: **yes, git worktrees/local clones**, one per arm, with the
uncommitted state captured and replayed into each arm; the winner's tree is merged back
and the loser's discarded. `battle: pre-merge stash` (`0x5a3daa7`) is the pre-merge step.

Comparison: `BattleModeInfo`, `BattleModeInfosUpdate`, `BattleEndType` (exact:
`0 UNSPECIFIED, 1 WINNER, 2 TIE, 3 KILL, 4 SKIP, 5 REFERENCE, 6 START_ERROR`),
`mergeWorkspacesWithStrategy` + `MERGE_STRATEGY_{UNSPECIFIED,SAFE_MERGE,
MERGE_WITH_CONFLICTS,OVERWRITE}`, `FORK_TARGET_{NEW_WORKTREE,NEW_WORKSPACE,
CURRENT_WORKSPACE}`. `TurnCostSummary{turn, subagent, cumulative_total}` and
`ConversationCostSummary{subagent_costs{}}` track each arm's cost. The user picks, or
`LLM_JUDGE` (`CORTEX_TRAJECTORY_TYPE_LLM_JUDGE`, `TrajectoryJudgeConfig{max_steps_to_judge}`)
does; rodata holds `You've reached the maximum number of turns for this battle. End battle
mode to continue.` (`0x5b77ab5`) as the arm's turn budget. `BATTLE_MODE_REFERENCE_BACKUP`
reference type backs up the pre-battle state so `EndBattleMode` can revert.

---

## 6. Extension surfaces

| surface | what it is | on disk | can do |
|---|---|---|---|
| **MCP servers** | full MCP client: stdio (`command`,`args`,`env`,`cwd`) and HTTP/SSE (`server_url`,`headers`), OAuth (`McpOAuthConfig{client_id,client_secret}`) and plugin-scoped auth (`auth_provider_type`, `plugin_auth_provider`) | `~/.gemini/config/mcp_config.json` (observed, empty, 0 bytes) + per-plugin `mcp` config (`config_pb.PluginUserConfig.mcp{variables}`) + project config | tools with per-tool policy (`McpServerToolConfig{background,eager,task_options}`), tool discovery deferred behind `tool_search`, resource read, prompts (`McpPromptSpec`), **MCP apps / generative UI** (`McpAppView{resource_uri,call_tool_result_json,entrypoints}`, `McpToolUiMeta{resource_uri,visibility,entrypoints}`, `McpAppEntrypointType{AUX_PANE,FULL_PANE,SIDEBAR}` → `sidecar://` pills), elicitation (`ElicitationInteractionSpec`), progress, `disabled`/`enabled_tools`/`disabled_tools`, `strict_argument_validation`, `omit_from_system_prompt`, `force_all_tools_eager`, `timeout_seconds` |
| **Plugins** | marketplace-installable bundles that ship MCP servers + skills + auth providers + sidecars | `config_pb.PluginUserConfig{enabled, installed_from(MarketplaceInstall), mcp{}}`; secrets in `~/.config/antigravity/plugin-secrets.json` (from context doc); `~/.gemini/antigravity-cli/cache/` for plugin cache (`failed to create the plugin cache directory`) | `ListPlugins/InstallPlugin/UninstallPlugin`, `StartPluginAuthSignIn`/`CompleteLocalPluginSignIn` with `LocalPluginOAuthConfig{client_id,client_secret,authorize_uri,token_uri,scopes,domains,identity_uri}`, `GeneratePluginAccessToken`, `plugin_shim_dir` / `ANTIGRAVITY_PLUGIN_SHIM_DIR`, `plugin_root`/`plugin_data_dir`/`plugin_version` per MCP server |
| **claude / gemini compat** | `cli/plugins/{claude,gemini}` import foreign extension formats. Observed string: *"…is Claude Code's name for a tool, so it never matches"* (`0x5b6d0c9`) and `In request-review mode, file edits are shown for approval before being applied.`; klog `gemini_extensions.go:28/49] Detecting Gemini extensions in ~/.gemini/extensions / No extensions found` | `~/.gemini/extensions/` (Gemini CLI), Claude Code plugin dirs | maps foreign tool names, hooks and commands onto agy's own surface; **the compat layer is name-translation, and a name mismatch is silently non-matching** — a real fidelity trap for a reimplementation |
| **Skills** | `SKILL.md` folders, marketplace-installable | `~/.gemini/antigravity-cli/builtin/skills/<name>/` (observed: `agy-customizations`, `antigravity_guide`, `automation`, `generative_ui`, `migrate-workflows`, `permissioned-github`, `plugin`, `run-workflow`, …) + `config_pb.SkillUserConfig{installed_from}`; checksummed by `builtin/.checksum`; user skills via `cli/model/skills.go` and `Reloading system slash commands and skills` (`manager.go:1455/1463/1498`) | skill text injected as context; `SkillMetadata{name,description,publisher,version,icon,logo}`; `CortexStepViewFile.is_skill_file/skill_metadata` means a skill is read *as a file* by the model |
| **Hooks** | `HOOK_TYPE_{PRE_INVOCATION,POST_INVOCATION,STOP,PRE_TOOL_USE,POST_TOOL_USE}` (exact: 1..5) | `hooks.json` (klog `hooks_manager.go:53] loaded 0 named hooks from 0 hooks.json file(s)`; `failed to parse hooks.json: %w`), `docs/hooks` embedded | `HookToolCall{name,args}`, per-step `HookStatus{hook_type,hook_name,started_at}`, `CascadeState.step_hooks{step_index: HookStatus}`; a hook can terminate a turn (`EXECUTOR_TERMINATION_REASON_TERMINAL_CUSTOM_HOOK`) |
| **Rules** | `CORTEX_STEP_TYPE_DIRECTORY_RULES` step: `{content, rule_file_uris[]}` | per-directory rule files | injected context, scoped by directory |
| **Workflows** | `steps.IsWorkflowStep`, `WorkflowRenderInfo`, `WorkflowName/Description/Headline/Agents`, `ParseWorkflowAgentSubtitle`, `CopyBuiltinWorkflowToWorkspace` | builtin + workspace copy | slash-command-driven multi-agent workflows |
| **Custom agents** | `cortex/customizations/builtin/agents/`; `CustomAgentConfig{name, tool_names[], …}`, `CustomAgentSpec`, `LoadedCustomizations{type,name,path,scope,status,plugin_name,version,installed_from}` | `config_pb` customizations dirs | a named agent = prompt + tool allow-list + model; `GetAllCustomAgentConfigs`, `--agent` flag; `AgentConfigResolution{requested_agent_spec, contrib_variant_metadata{assigned_variant,eligible_variants,applied_variant}}` shows A/B variants of an agent |
| **Sidecars** | long-lived helper processes with UI panes | `config_pb.UserConfig.sidecars{id: SidecarUserConfig}`; `ANTIGRAVITY_SIDECAR_WEB_PORT`; `~/.gemini/antigravity-cli/sidecars`-style dirs (`sidecar %s is disabled`, `sidecar process failed: %v`, `%w: sidecar %q is not found`) | `SidecarStatus{RUNNING,STOPPED,ERRORED,BACKOFF,STARTING}`, `SidecarUIEntrypoint{AUX_PANE,FULL_PANE,SIDEBAR}`; a web server on that port; sandboxed or not per policy |
| **remote-control** | drive a conversation from outside (Antigravity desktop / hub) | `RemoteControlDetails` on interactions; `language_server/remotecontrol` (330 funcs); `cli/remotecontrol` | klog `remote_agent.go:168] Remote agent fastpush pin gate changed: false -> true`, `server.go:3489–4268` RemoteControl handlers; approvals can arrive from the remote side (`CascadeUserInteraction.remote_control_details`, `(denied_repeat?)`); `--remote-control` flag and `remote-control` subcommand |
| **mic-serve** | `ANTIGRAVITY_MIC`; voice input/serve mode | — | speech-to-text surface (`StreamAudioTranscription`, `voice_chat`), `VoiceChat{session, lines}` persisted per conversation |
| **install** | subcommand; writes `webm_encoder` | `~/.gemini/antigravity-cli/bin/webm_encoder` (observed) | klog `encoder_embed.go:85] Installing/updating embedded webm_encoder binary to ~/.gemini/antigravity-cli/bin/webm_encoder` — a **screen-recording encoder**: browser-subagent sessions are recorded (`CortexStepBrowserSubagent.recording_path`,
`recording_generation_status`), so the embedded binary converts captured frames to webm. Not a security component. |
| **slash commands** | built-in + user-defined; `ListSlashCommands` | builtin skills/workflows + `--disable-slash-commands` | `SlashCommandDefinition` |

Additional CLI-side extension seams found in funcs: `cli/mcp` (25), `cli/plugins` (222),
`cli/model/skills.go`, `hooks_manager.go`, `cli/marketplace` (`language_server/marketplace`,
73), `language_server/extension_server_pb` (`extension_server.proto`, 390 lines of
messages: an out-of-process extension host), `cortex/ext`, `cortex/sdk`,
`cortex/sidecars`, `cortex/artifacts`, `cortex/rehydration`, `cortex/implicit`,
`cortex/snapshot_recorder`, `cortex/chatconverters` (345).

---

## 7. Persistence

### 7.1 Layout (all observed on this machine)

```
~/.gemini/antigravity-cli/
  antigravity-oauth-token      0600 JSON token
  settings.json                {agentMode, altScreenMode, editor, trustedWorkspaces[]}
  jetski_state.pbtxt           protobuf text: post_onboarding.completed_steps[],
                               installation_uuid, migrations{int: MIGRATION_STATUS}
  installation_id
  history.jsonl                {"display","timestamp","workspace","type","conversationId"}[]
  conversations/<uuid>.db      SQLite, one per conversation
  conversation_summaries.db    SQLite, all conversations
  jetbox_summaries_proto.pb    protobuf, summary cache
  brain/<uuid>/scratch/ , brain/<uuid>/.user_uploaded/
  implicit/<uuid>.pb
  presence/<uuid>.lock
  builtin/skills/**  builtin/.checksum  builtin/keep.txt
  bin/webm_encoder
  log/cli-<ts>.log, log/cli.log -> newest  (klog format)
  cache/, crashes/, updater/, last_check.timestamp
~/.gemini/config/
  mcp_config.json               MCP servers (observed 0 bytes)
  projects/<id>.json            {"id","name","projectResources":{}}
  .migrated                     marker (AppDataDir -> ~/.gemini/config migration)
~/.config/antigravity/plugin-secrets.json
```

### 7.2 Conversation DB — recovered schema (`PRAGMA user_version = 1`)

Copied `conversations/6a38c834-….db` to `/tmp` and dumped `.schema` (original untouched):

```sql
CREATE TABLE trajectory_meta (
  trajectory_id text, cascade_id text, trajectory_type integer, source integer,
  PRIMARY KEY (trajectory_id));
CREATE TABLE steps (
  idx integer, step_type integer NOT NULL DEFAULT 0, status integer NOT NULL DEFAULT 0,
  has_subtrajectory numeric NOT NULL DEFAULT false,
  metadata blob, error_details blob, permissions blob, task_details blob,
  render_info blob, step_payload blob,
  step_format integer NOT NULL DEFAULT 0,
  PRIMARY KEY (idx));
CREATE INDEX idx_steps_status ON steps(status);
CREATE INDEX idx_steps_step_type ON steps(step_type);
CREATE TABLE gen_metadata      (idx integer, data blob, size integer NOT NULL DEFAULT 0, PRIMARY KEY (idx));
CREATE TABLE executor_metadata (idx integer, data blob, PRIMARY KEY (idx));
CREATE TABLE parent_references (idx integer, data blob, PRIMARY KEY (idx));
CREATE TABLE trajectory_metadata_blob (id text DEFAULT "main", data blob, PRIMARY KEY (id));
CREATE TABLE battle_mode_infos (idx integer, data blob, PRIMARY KEY (idx));
```

Semantics: one DB **per trajectory** is not the model — one DB per conversation, holding
the *main* trajectory's `steps`, with `trajectory_meta` naming it
(`cascade_id` = conversation id). Subtrajectories are out-of-line:
`steps.has_subtrajectory` (numeric/bool) plus `Step.subtrajectory` (field 6) holding the
nested `Trajectory`. `step_format` selects the payload encoding (protobuf vs. an
alternative) — the two DBs here have zero step rows, so the value set is **unresolved**,
but `step_format NOT NULL DEFAULT 0` + "Failed to re-save migrated trajectory" strings
(`0x5af80cb`) show it is a migration axis. `gen_metadata` / `executor_metadata` are the
per-index `CortexStepGeneratorMetadata` / `ExecutorMetadata`
(`size` on `gen_metadata` enables paging without loading the blob).

Observed row: `trajectory_meta = (bd019762-…, 6a38c834-…, 4, 17)` — type `CASCADE`,
source `CLI`.

### 7.3 `conversation_summaries.db`

```sql
CREATE TABLE conversation_summaries (
  conversation_id text, title text NOT NULL DEFAULT "", preview text NOT NULL DEFAULT "",
  step_count integer NOT NULL DEFAULT 0, last_modified_time datetime NOT NULL,
  workspace_uris text NOT NULL, status text NOT NULL DEFAULT "",
  source text NOT NULL DEFAULT "", project_id text NOT NULL DEFAULT "",
  agent_name text NOT NULL DEFAULT "", parent_conversation_id text NOT NULL DEFAULT "",
  nesting_depth integer NOT NULL DEFAULT 0, battle_id text NOT NULL DEFAULT "",
  winning_conversation_id text NOT NULL DEFAULT "",
  not_fully_idle numeric NOT NULL DEFAULT false, killed numeric NOT NULL DEFAULT false,
  last_user_input_time datetime NOT NULL,
  last_user_input_step_index integer NOT NULL DEFAULT -1,
  app_data_dir text NOT NULL DEFAULT "", raw_summary blob,
  group_id text NOT NULL DEFAULT "",
  PRIMARY KEY (conversation_id));
CREATE INDEX idx_conversation_summaries_last_user_input_time …;
CREATE INDEX idx_conversation_summaries_last_modified_time …;
```

This is the index the conversation list and the *subagent tree* are built from:
`parent_conversation_id` + `nesting_depth` + `agent_name` express the subagent hierarchy;
`battle_id` + `winning_conversation_id` express battle mode; `project_id` expresses the
project binding that the klog shows switching the active project
(`conversation_manager.go:729/775] switching to conversation belonging to project ID /
synced active project`). `raw_summary` blob is
`jetski_cortex_pb.CascadeTrajectorySummary` (`cortex_pb.CascadeTrajectorySummary` in
`jetski_cortex.proto`), and `jetbox_summaries_proto.pb` is the same data cached as a
single protobuf.

### 7.4 Dual store and which half is authoritative

Klog (per steering): `manager.go:119] Creating trajectory store manager with proto store
and SQLite store`, against the alternative `Creating trajectory store manager with proto
store only` (`0x5b2a977`). So there are two backends behind one `TrajectoryStore`
interface:

* **proto store** — the `implicit/*.pb`, `jetbox_summaries_proto.pb`,
  `brain/`-adjacent and conversation-bundle protobuf files; the legacy/portable format.
* **SQLite store** — `conversations/*.db` + `conversation_summaries.db`, with its own
  schema versioning (`PRAGMA user_version`, `begin schema rebuild: %w`,
  `initialize schema (%w): %w`, `%s_migration_tmp_%d`, `conversation_search_vacuum`).

Their coexistence is a *migration* state, gated at startup by
`manager.go:95/98/107/113/133` migrations
(`MIGRATION_ID_LIN_BUILTIN_TO_MARKETPLACE`, `MIGRATION_ID_SANITY_CHECK_PROJECT_URIS`) and
by `migrate.go:50/158` moving AppDataDir → `~/.gemini/config` behind a `.migrated` marker,
plus `project_uris_migration.go:42`. The strings `Failed to re-save migrated trajectory
%s`, `failed to list trajectories for store %v`, and `Failed to create SQLite trajectory
store: %v` (`0x5af80cb`) mean SQLite is the **reader of record** once it exists and the
proto store is the fallback/export; the summary above (`conversation_summaries.db` is what
the list UI reads) supports that. I did not find a write-path function that proves
SQLite-only writes, so "SQLite authoritative when present, proto store as fallback" is
**derived, medium confidence**.

### 7.5 Other stores

| store | purpose |
|---|---|
| `brain/<uuid>/{scratch,.user_uploaded}` | per-conversation artifact directory. This is the `Artifact Directory Path: %s` string, the sandbox's writable scratch, the upload drop box, and the target of `forkArtifactDirectory`/`copyArtifacts` in battle mode. The *agent's* brain content (plan/task tree) is a trajectory step (`CORTEX_STEP_TYPE_BRAIN_UPDATE`), not a file. |
| `implicit/<uuid>.pb` | implicit *context* trajectory: `ImplicitTrajectory{trajectory, trajectory_scope}`, referenced by `BaseTrajectoryIdentifier.implicit_trajectory_file_uri`. These are the per-editor-file trajectories that make completions/`Tab` context cheap; `implicit/*.pb` on disk hold the last-active-document trajectory. |
| `presence/<uuid>.lock` | one lock file per live conversation: the multi-process "is this conversation open elsewhere" mechanism behind `open_elsewhere` in `AgentStateUpdate` and `message_manager.go`-style `MessageDeliveryHook: failed to mark read`. |
| `history.jsonl` | CLI *input history*, not conversation history: `{display, timestamp(ms), workspace, type: "slash_command"\|…, conversationId?}`. |
| `jetski_state.pbtxt` | low-frequency device state: onboarding completion, `installation_uuid`, migration status map. |
| `log/cli-<ts>.log` | klog; carries the internal `file.go:line` that makes the recovery above possible. |
| `builtin/skills` + `.checksum` | embedded skill bundle extracted on first run and re-extracted when the checksum changes. |
| `cache/`, `updater/` | model/config cache, `installed_version.json`, archive install status. |

Projects are **files, not DB rows**: `~/.gemini/config/projects/<id>.json` holding
`{id, name, projectResources}` (observed `default-cli-project.json` = `CLI Project`), while
grants and settings for that project live in the protobuf `Project` record (klog
`project.go:78] project: created project "CLI Project" (id=default-cli-project)`). A
conversation belongs to a project and switching conversation can switch the active
project — so project switching is a side effect of conversation selection, not the reverse.

---

## 8. Open questions / unresolved

1. **`step_format` value set.** Both conversation DBs on this machine have zero steps, so
   the encoding enum is unknown. `step_format NOT NULL DEFAULT 0` plus
   "Failed to re-save migrated trajectory" suggests at least two generations; a DB with
   real turns would settle it.
2. **Which store is authoritative** (§7.4). Medium-confidence inference from log priority;
   needs a write-path disassembly of the store manager.
3. **Literal tool-name strings.** Converter type names are known; the exact wire names are
   not, except the handful in §2.1. Recoverable by `strs` on each converter's
   `GetToolName`/`Spec` method — I only landed `McpToolConverter.GetToolNameWithoutPrefix`.
4. **`mcp_chrome_devtools_*` and built-in MCP prefixes.** `telemetry.GetBuiltinMCPServerPrefixes`
   exists; its string table was not dumped. Prefix convention unverified.
5. **`TOOL_FORMATTER_TYPE_*` numbers.** Names only; no embedded descriptor.
6. **`--dangerously-skip-permissions` exact wire effect.** Mapped to preset `NONE`/`AUTO`
   by inference; not observed.
7. **PolicyGuardian's judge model and prompt.** The judge is a client-side model call; which
   model/tier, the verdict schema (`deny_conditions` shape), and the retry count are not
   recovered. `vet_irreversible` exists as a suite name only (a single string at
   `0x59cbe79`), and `VetDependency`/`Vet` were not disassembled.
8. **Sandbox escape hatch breadth.** `newTrustedCLIEscape`, `trustedCLINetboxAuth`, and
   `envDeliveryCLIs` imply a curated list of "trusted CLIs" that get extra network/fs
   latitude. The list itself was not extracted, and it is the most security-relevant
   unknown in §4.
9. **Whether bwrap is executed or reimplemented.** `bwrapper.cc` plus `selfexec.Invocation`
   suggest an in-process bwrap; `escapeExec`/`buildFileTree` suggest a re-exec. Not
   resolved.
10. **Battle-mode adjudication.** `LLM_JUDGE` trajectory type exists; whether "best of n" is
    always user-selected, and the arm count limit (`gitForker.MaxForks`), are unread.
11. **`KillSubagent`** exists as a *behaviour* (`SubagentState.KILLED`,
    `manage_subagents{action}`), not as the function name in the brief; the action strings
    for `ManageSubagents.action` and `ManageTask.action` are not recovered.
12. **Turn/summary generation models.** `CORTEX_TRAJECTORY_TYPE_{CHECKPOINT,
    COMMIT_MESSAGE_GENERATION, KNOWLEDGE_GENERATION}` each run a model; their prompts live
    in `CheckpointConfig` overrides and `KnowledgeConfig`, not extracted.
