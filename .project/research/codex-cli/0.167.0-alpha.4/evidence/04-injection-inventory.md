---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-evidence-04-injection-inventory"
  title: "Codex context injection inventory"
  description: "Source-traced fragment inventory copied from the chapter tables"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:06+01:00"
  updated_at: "2026-11-03T11:54:06+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["captured the injection inventory"]
---

# Injection inventory


## Assembly and ordering


## Environment, state and instructions

| Fragment | Trigger and sequence placement | Role | Exact builder and bounds |
|---|---|---|---|
| Environment | Enabled environment section; full state initially and subsequent changes as world-state diff. | user | `core/src/context/world_state/environment.rs:43-195,205-223,246-263`; filesystem/network XML: `core/src/context/environment_context.rs:32-71,92-164,218-242`. |
| Internal extension context | A producer supplies a validated source and body; [UNK] all producers and timing. | user | `core/src/context/internal_model_context.rs:21-38,64-120`; source matches `[a-z][a-z0-9_]*`. |
| Base model instructions | Incremental-tools window has no known prior model snapshot; prefix item, omitted when empty. | developer | `core/src/context/base_instructions.rs:41-60`; `core/src/context/world_state/model.rs:81-100`. |
| Changed model/instructions | Incremental window model or instruction hash changes; standalone world-state update. | developer | `core/src/context/model_instructions.rs:8-53`; `core/src/context/world_state/model.rs:101-140`. |
| Model switch | Ordinary window model changes and instructions are nonempty; standalone update. | developer | `core/src/context/model_switch_instructions.rs:19-41`; `core/src/context/world_state/model.rs:101-140`. |
| AGENTS.md user instructions | Loaded AGENTS.md initially, replacement on change, removal if previously present; world-state update. | user | `core/src/context/user_instructions.rs:13-41`; `core/src/context/world_state/agents_md.rs:11-77`. |
| Developer instructions | Producer-provided instructions; [UNK] exhaustive producers/placement. | developer | `core/src/context/developer_instructions.rs:20-43`. |
| Managed developer requirements | Additional requirements change; standalone update with replacement/removal semantics, excluding basic guardian sessions. | developer | `core/src/context/world_state/managed_developer_instructions.rs:17-80,117-155`; oversized instructions rejected at 10,000 estimated tokens, not silently truncated. |
| Permissions instructions | Enabled permission section at initial context or when settings change; prefix-only additions may use a separate smaller notice. | developer | `prompts/src/permissions_instructions.rs:115-174,281-303`; `core/src/context/world_state/permissions.rs:80-139`; 32 KiB shared path budget. |
| Available skills and selected skill | Catalog contributions update as available skills change; explicitly selected skill body is wrapped separately. | developer / user | `ext/skills/src/fragments.rs:20-69,87-127`; `ext/skills/src/world_state.rs:41-114`. |
| Apps/connectors | Enabled, accessible app and model includes app usage instructions; world-state section. | developer | `core/src/context/apps_instructions.rs:13-34`; `core/src/session/world_state.rs:337-351`. |
| Available plugin usage | Plugins available and model includes plugin usage instructions; world-state section. | developer | `core/src/context/available_plugins_instructions.rs:12-50`; `core/src/session/world_state.rs:352-361`. |
| Plugin instructions | Producer supplies plugin-specific text; [UNK] exact producer and placement. | developer | `core/src/context/plugin_instructions.rs:11-36`. |
| Plugin recommendations | Nonempty candidates; at most 50; [UNK] full selection conditions. | developer | `core/src/context/recommended_plugins_instructions.rs:7-64`; `core/src/session/mod.rs:5015`. |
| Multiple execution environments | Environment context enabled and deferred executor feature enabled; world-state section. | developer | `core/src/context/environments_instructions.rs:11-39`; `core/src/session/world_state.rs:330-336`. |
| Collaboration mode | Enabled collaboration-mode section changes; built-in or custom mode instructions. | developer | `core/src/context/world_state/collaboration_mode.rs:27-78,95-175`. |
| Persistent mode | Feature enabled/changed, except basic guardian sessions; world-state update includes removal/replacement. | developer | `core/src/context/world_state/persistent_mode.rs:16-45,52-117`; `core/src/session/world_state.rs:288-304`. |
| Multi-agent mode and usage | Effective mode or configured hint changes; hint is standalone. | developer | `core/src/context/multi_agent_mode_instructions.rs:15-57`; `core/src/context/multi_agent_usage_hint.rs:6-41`; `core/src/session/world_state.rs:393-403`. |
| Multi-agent role | Configured or composed role guidance is admitted; [UNK] exact producer gates. | developer | `prompts/src/multi_agent_instructions.rs:10-22,43-98`; conditional wait/override hints and concurrency count. |
| Spawn model catalog | Spawn tool overrides exposed and model-catalog feature enabled; changes emit refreshed catalog or removal. | developer | `core/src/context/world_state/model_catalog.rs:23-89,100-153`; max rendered 1,000 bytes, descriptions max 250 characters; `core/src/session/world_state.rs:405-440`. |
| Deferred tool namespaces | Deferred-tool world-state feature enabled; emits initial names or changed/removed namespaces. | developer | `core/src/context/world_state/tools.rs:28-31,48-79,89-154`; 4,096-byte rendered cap and 250-character descriptions. |
| Incremental top-level tools | Incremental Responses Lite catalog enabled; emits changed declarations as prefix items and removed-tool notices. | developer | `core/src/context/world_state/top_level_tools.rs:124-250,270-324`; schema updates use `AdditionalTools` items, not plain text. |
| Memory | Read instructions or extraction evidence; extension-produced, [UNK] full selection. | developer / user | `core/src/context/memory.rs:10-46`; truncated with `TruncationPolicy::Bytes(8_900)`. |
| User goal mutation | User-facing goal API accepts set/status/clear, [UNK] precise sequencing. | user | `core/src/context/user_goal.rs:14-24,63-119`; JSON-encoded objective over 700 bytes omitted whole. |

## Time, notices and post-tool context

| Fragment | Trigger and sequence placement | Role | Exact builder and bounds |
|---|---|---|---|
| Time reminder | Feature and reminder config enabled; new window or interval due, subject to configured user/tool-output boundary delivery. Recorded before inference. | developer | `core/src/context/current_time_reminder.rs:11-50`; due/read: `core/src/session/time_reminder.rs:63-95,156-199`. |
| Clock unavailable | Nonfatal clock-read failure; at most once per turn/window; recorded in conversation. | developer | `core/src/context/current_time_reminder.rs:53-82`; `core/src/session/time_reminder.rs:99-152`. |
| Image resized | Image preparation reports resized user/tool image; [UNK] exact final assembly ordering for each source. | developer | `core/src/context/image_resize_notice.rs:22-76`; producers `core/src/image_preparation.rs:163,250`. |
| Unsupported image/audio | Unsupported media represented by a replacement fragment; [UNK] feature detection and insertion ordering. | user | `core/src/context/unsupported_media.rs:12-46`. |
| Verification | User-verification elicitation becomes realtime handoff text. | realtime handoff text; fragment declares developer | `core/src/context/user_verification_notice.rs:8-29`; caller renders only its content, not its role: `core/src/session/turn.rs:2261-2278`. |
| Content filter | Recovery guidance on unspecified content filter; [UNK] complete retry policy. | developer | `core/src/context/content_filter_guidance.rs:8-31`; `core/src/responses_retry.rs:72`. |
| Saved command prefix | Approved prefixes added without other permission changes; world-state diff. | developer | `core/src/context/approved_command_prefix_saved.rs:6-49`; `core/src/context/world_state/permissions.rs:96-127`; catalog heading max 500 UTF-8 bytes. |
| Saved network rule | A network policy amendment is saved. | developer | `core/src/context/network_rule_saved.rs:14-49`; `core/src/session/mod.rs:3060`. |
| User shell command | User-run command and result are rendered into a user fragment. | user | `core/src/context/user_shell_command.rs:16-54`; `core/src/user_shell_command.rs:20`. |
| Turn aborted | Interrupted turn records interruption guidance; actual running commands may still continue. | user | `core/src/context/turn_aborted.rs:11-41`; `core/src/tasks/mod.rs:112-115`. |
| Three legacy warnings | Old-session text recognizers only; their source says they are no longer produced. | user | `core/src/context/legacy_apply_patch_exec_command_warning.rs:6-35`; `core/src/context/legacy_model_mismatch_warning.rs:6-35`; `core/src/context/legacy_unified_exec_process_limit_warning.rs:6-35`. |

## Inter-agent and realtime

| Fragment | Trigger and sequence placement | Role | Exact builder and bounds |
|---|---|---|---|
| Inter-agent message | Delivery of message/new task from another agent. | assistant | `core/src/context/inter_agent_message.rs:8-20,47-72`; `core/src/agent/control/delivery.rs:32`. |
| Inter-agent final answer | Completion message supplied at session prefix. | assistant | `core/src/context/inter_agent_completion_message.rs:25-47`; `core/src/session_prefix.rs:37-51`. |
| Subagent notification | Child status delivered to controller. | user | `core/src/context/subagent_notification.rs:23-48`; `core/src/agent/control.rs:562`. |
| Message-board post | Post-preview notification; if truncated, directs reader to the rest. | assistant | `core/src/context/agent_message_board_notification.rs:9-43`; `core/src/agent_message_board.rs:230`. |
| Realtime delegation | Handoff or transcript tail flush; [UNK] exact caller ordering. | user | `core/src/context/realtime_delegation.rs:22-77,86-115`; escaped input and normally transcript each capped at 4,096 bytes, preserving input start and transcript end; transcript cap bypassed when preservation is requested. |
| Realtime start, custom start and end | Realtime state active/inactive transition; start on initial active state. | developer | `core/src/context/realtime_start_instructions.rs:12-34`; `core/src/context/realtime_start_with_instructions.rs:21-43`; `core/src/context/realtime_end_instructions.rs:26-52`; transition `core/src/context/world_state/realtime.rs:34-92`. |

## Guardian review context

| Fragment | Trigger and sequence placement | Role | Exact builder and bounds |
|---|---|---|---|
| Guardian policy | Policy supplied to isolated reviewer; [UNK] full composition trigger. | developer | `core/src/context/guardian_policy.rs:6-39`. |
| Guardian node REPL policy | Reviewer resolves model's node-REPL policy; [UNK] exact gate. | developer | `core/src/context/guardian_node_repl_policy.rs:12-40`; evidence mode gate `core/src/context/node_repl_review_evidence.rs:10-21`. |
| Conversation-history access | Optional reviewer history prompt, optionally overridden. | developer | `core/src/context/guardian_conversation_history.rs:9-36`. |
| Follow-up review | Follow-up reminder requested by reviewer; [UNK] decision path. | developer | `core/src/context/guardian_followup_review_reminder.rs:9-28`. |
| Reviewer assistant context | Complete retained assistant text admitted as evidence; does not grant authorization. | assistant | `core/src/context/guardian_assistant_context.rs:11-51`. |
| Sender messages | Bounded sender snapshot admitted for review, not worker prompt. | user | `core/src/context/guardian_sender_messages.rs:12-77`; up to three recent user messages; each user/assistant exchange shares a 900-byte bound with user prioritized. |
| Evidence omission | Aggregate review input budget omits/shortens evidence; reviewer notice. | user | `core/src/context/guardian_budget_omission.rs:8-33`. |
| Sync-review evidence | Host-supplied bounded synchronous-review evidence for async classifier input. | developer | `core/src/context/guardian_review_evidence.rs:12-44`; [UNK] upstream aggregate limits. |
| Retained instructions | Restored reviewer evidence after compaction; rejects records above 900-token byte budget. | user | `core/src/context/guardian_retained_instructions.rs:11-49`. |
| Approved action | A denied action is later explicitly approved; exact-action-scoped instruction in worker context. | developer | `core/src/context/guardian_approved_action.rs:12-51`; producer `core/src/session/handlers.rs:820`. |

## Budget, compaction and hooks

| Fragment | Trigger and sequence placement | Role | Exact builder and bounds |
|---|---|---|---|
| Context-window identity | Feature enabled and model context window known; initial identity/hints and selected state changes. | developer | `core/src/context/token_budget_context.rs:19-109`; registration `core/src/session/world_state.rs:193-214`. |
| Context-window guidance | Configured nonempty guidance or change/removal of previous guidance; world-state section. | developer | `core/src/context/token_budget_context.rs:112-147`; `core/src/context/world_state/context_window_guidance.rs:8-74`. |
| Remaining context tokens | Context-remaining tool output; unknown value has a distinct text. | tool response; fragment declares developer | `core/src/context/token_budget_context.rs:150-191`; caller renders only its content into the tool response: `core/src/tools/handlers/get_context_remaining.rs:23-51`. |
| Token budget reminder | Runtime budget threshold condition; [UNK] exact thresholds. | developer | `core/src/context/token_budget_context.rs:194-226`; `core/src/session/token_budget.rs:186`. |
| Automatic-compaction fallback | Fallback prompt provided during automatic compaction; [UNK] complete triggering conditions. | developer | `core/src/context/token_budget_context.rs:229-261`; `core/src/session/token_budget.rs:215`. |
| Shared rollout budget | Session budget record when producer selects it; [UNK] exact cadence. | developer | `core/src/context/rollout_budget.rs:6-33`; `core/src/session/rollout_budget.rs:27`. |
| Compaction summary | Generated summary inserted into rebuilt history. | user | `core/src/context/compaction_summary.rs:11-40`; `core/src/compact.rs:815`. |
| Hook additional context | Hook-provided context converted to developer fragment, with producer metadata preserved. | developer | `core/src/context/hook_additional_context.rs:13-42`; `core/src/hook_runtime.rs:911`. |
