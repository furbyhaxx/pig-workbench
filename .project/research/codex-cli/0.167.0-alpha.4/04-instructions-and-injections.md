---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-04-instructions-and-injections"
  title: "Codex CLI instruction and injection catalogue"
  description: "Source-observed context fragments, roles, entry conditions and builders"
  status: "published"
  revision: 2
  created_at: "2026-11-03T11:54:06+01:00"
  updated_at: "2026-11-03T12:20:00+01:00"
  created_by: "worker"
  updated_by: "orchestrator"
  changelog:
    - revision: 2
      reason: "close the last uncovered context module"
      changes: ["added the contextual_user_message reverse-mapping registry and its parse consequences", "recorded that the registry also runs during live notification filtering"]
---

# Instructions and injections

Source snapshot: `host:/tmp/re-recon/codex-tag/codex-rs`, tag `rust-v0.167.0-alpha.4`, commit `fb036d4ebd89f263d34a1f888c279ce85bb383c6` (checked against the checkout). This is source analysis; no Codex binary was run. Paths below are relative to that `codex-rs` root. **[OBS]** denotes source text or control flow, **[INF]** inference, and **[UNK]** a path or behavior not traced. Verbatim context builders are captured in [context builders](evidence/04-context-builders.txt), with [additional builders](evidence/04-additional-builders.txt) and [skill builders](evidence/06-skills-builders.txt); standalone permission templates are captured in [permission templates](evidence/05-permission-templates.md). The [inventory table](evidence/04-injection-inventory.md) reproduces the tables below.

## Assembly and ordering

[OBS] `ContextualUserFragment` supplies the role, kind, metadata, start/end markers and body. Its renderer concatenates start marker + input text + end marker without separators; non-text passes through. Producers are responsible for enforcing size bounds (`context-fragments/src/fragment.rs:48-107`). A fragment's role is not determined by its filename.

[OBS] A world-state update is `Prefix`, `Standalone` or `Mergeable`; prefix updates are separated to the beginning of initial context (`core/src/context/world_state/mod.rs:58-131`). `WorldState` maintains insertion order; it compares saved section snapshots and emits updates only as needed. An extension section named `host_skills` is inserted before permissions if that section is present (`core/src/context/world_state/mod.rs:519-605`). The session registers model and, conditionally, tool declarations and token state, then realtime, AGENTS.md, permissions, collaboration, persistence, environment, apps, plugins, tools, extension contributions, agent mode/catalog and managed developer instructions (`core/src/session/world_state.rs:177-457`). The entries below are not all world-state sections: some are emitted during a tool/turn, into an isolated reviewer context, or during compaction, as their individual source paths indicate.

[OBS] Initial-context assembly groups developer fragments and user fragments separately, retains standalone developer messages, puts model-switch instructions before the other developer sections, emits multi-agent mode after its usage hint, and places guardian policy in a separate developer item when configured (`core/src/session/mod.rs:4950-5064`). The time reminder is recorded before the next world-state update in the sampling loop (`core/src/session/turn.rs:564-575`).

[UNK] Exact interleaving of every direct fragment with every user/tool item and exhaustive extensions outside these modules were not independently traced. Entries marked [UNK] identify a known builder whose exact producer condition or insertion point remains open. The source may construct other model messages without this trait; the tables are not proof of a complete product-wide inventory.

### Reverse mapping of contextual user messages

[OBS] The `InputText` reverse matcher recognizes hook-prompt fragments or any entry in the following registry, in order (`core/src/context/contextual_user_message.rs:24-39,86-97`):

1. `UserInstructions::matches_text`; 2. `EnvironmentsState::matches_text`; 3. `AdditionalContextUserFragment::matches_text`; 4. `AgentMessageBoardNotification::matches_text`; 5. `codex_skills_extension::is_skill_prompt_fragment`; 6. `UserShellCommand::matches_text`; 7. `TurnAborted::matches_text`; 8. `SubagentNotification::matches_text`; 9. `InternalModelContextFragment::matches_text`; 10. `RecommendedPluginsInstructions::matches_text` (compatibility for old user-role recommendations); 11. `LegacyUnifiedExecProcessLimitWarning::matches_text`; 12. `LegacyApplyPatchExecCommandWarning::matches_text`; 13. `LegacyModelMismatchWarning::matches_text` (the last three recognize old warnings and are no longer produced). Current recommendations use developer role (`core/src/context/recommended_plugins_instructions.rs:38-45`; legacy definitions: `core/src/context/legacy_unified_exec_process_limit_warning.rs:6-8`; `core/src/context/legacy_apply_patch_exec_command_warning.rs:6-8`; `core/src/context/legacy_model_mismatch_warning.rs:6-8`).

[OBS] Rollout reconstruction checks recorded response items with `is_user_turn_boundary`, which excludes user-role messages containing a recognized contextual fragment; rollout truncation also calls `parse_turn_item` to identify actual user messages (`core/src/session/rollout_reconstruction.rs:431-435`; `core/src/context_manager/history.rs:1471-1485`; `core/src/thread_rollout_truncation.rs:43-54`). Fresh user-prompt emission instead creates `TurnItem::UserMessage` directly from `UserInput` (`core/src/session/mod.rs:5611-5622,5656-5673`). The registry is **not exclusive to replay**: live first-attempt legacy notification filters the current prompt with it (`core/src/session/turn.rs:1872-1873`; `core/src/hook_runtime.rs:621-643`).

[OBS] For user-role history, `parse_turn_item` first tries to build a visible `HookPromptItem`: every content part must be text and either a parsed hook fragment or a recognized standard fragment, with at least one hook; standard fragments are omitted from that item. The protocol parser requires a nonempty hook-run ID (`core/src/context/contextual_user_message.rs:99-124`; `protocol/src/items.rs:782-790`; `core/src/event_mapping.rs:194-205`). Otherwise, if **any** content part matches contextual text, ordinary user-message parsing returns `None` for the entire message, including mixed messages; if none matches, it can become a `UserMessage` (`core/src/event_mapping.rs:65-67,126-130,194-205`). Guardian classification additionally excludes recognized context from user authorization evidence, except a host-annotated user-goal update; authorization provenance has its own metadata checks (`core/src/context/contextual_user_message.rs:42-84`; `core/src/context_manager/history.rs:224-228`).

[UNK] Matching alone does not establish that a fragment survives resume or compaction. Proving persistence for a specific fragment requires tracing its rollout record and reconstruction or compaction path. Verbatim registry, parsing and call-site excerpts: [evidence/04-contextual-user-message.txt](evidence/04-contextual-user-message.txt).

## Environment, state and instructions

Each row is [OBS] for the stated role and builder. A trigger explicitly marked [UNK] was not traced beyond construction/state-transition code. A builder path is an exact executable template, including dynamic fields and wrapper; it is not a claim that each field is present in every message.

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

These rows describe a separate reviewer input unless explicitly noted. They do not establish that reviewer-only content is added to the worker's prompt.

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

[UNK] Other extension contributions are not individually enumerated above; consult the extension registry before treating this as exhaustive. Guardian prompts resolved from other crates and runtime-provided template bodies also require separate collection.
