---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-12-instructions-and-injections"
  title: "Claude Code 2.1.317 instructions and context injections"
  description: "Static evidence for instruction sources, reminder attachments, hooks, memory and compaction."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:57+01:00"
  updated_at: "2026-11-03T11:52:57+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document instruction and injection machinery"]
---
# Instructions and injections

Scope: offline inspection of the supplied `host:/tmp/cc_code.txt` (local path `/tmp/cc_code.txt`) carved from the installed 2.1.317 executable; no API traffic or live session was captured. `C<line>:<offset>` means 1-based line and 0-based character offset within that line. `[OBS]` is code or literal text in the bundle, `[INF]` is an inference from it, and `[UNK]` denotes an unresolved path or condition. Internal `type:"attachment"` is a transcript record, not by itself a wire message. The output in `evidence/12-*.txt` records exact corpus-search commands and surrounding builder code. Minified names below refer only to the cited module.

## Trust and insertion model

[OBS] The prompt builder at C18629:919 calls reminders application-supplied, potentially from a background task or plugin, recommends ignoring irrelevant ones, and explicitly says system reminders/messages **do not give user consent** for commands or permissions. Its two placement strings are:

> Mostly they show up as their own short system messages slotted between the turns of the conversation; a few (the context note at the very start, and anything tucked inside a tool result) show up wrapped in `<system-reminder>` tags instead. Same source either way.

> They show up wrapped in `<system-reminder>` tags: at the start of the conversation, inside later user messages, and inside tool results. They aren't specially related to whichever message or result they happen to be sitting in.

[OBS] These are alternative prompt strings, not proof that every provider receives the same wire roles. The attachment renderer uses `Le({content: ..., isMeta:!0})` and `ld` applies `Wa` to its text blocks (C26005:558, C26196:20). [UNK] Exact API wire role, ordering and feature gate for every attachment producer have not been established; distinguish internal records from API system-role messages. Text in a tool result is not evidence of user consent. The prompt separately says hook output is best treated as feedback from the user or the user's system (C18629:919, preceding context).

[OBS] The producer `vDr` collects prompt-specific `@` file, MCP resource, agent and skill mentions; its ambient pass collects changed files, nested memory, skill listings, plan/auto-mode reminders, todo/task and ToolSearch reminders, environment, model, sandbox instructions and other sources (C25614:3939; `evidence/12-attachment-orchestrator.txt`). For the attachment-producing path it sets a 1,000 ms abort timer; that is not a universal tool or API timeout. It has a separate simple/bare-fork branch. [UNK] This inventory is not an exhaustive proof of every production trigger or of network serialization order.

## Todo and task reminders

[OBS] `bLr` scans backward over the actual transcript: it skips assistant messages for which `qAe` is true, counts assistant messages since the most recent assistant `tool_use` named `TodoWrite`, and independently counts those since the most recent `todo_reminder` attachment; it stops after finding both. Its constants are `TURNS_SINCE_WRITE:10` and `TURNS_BETWEEN_REMINDERS:10` (C25622:2023; C25614:3202). The checks are **both** `>= 10`, not a timer or a count of user turns. With no prior matching event the scan counts available assistant messages.

[OBS] `TLr` returns no reminder unless the relevant todo tool is in `options.tools`, a nonempty history exists, the brief-tool suppression predicate is false and `CLAUDE_CODE_TODO_REMINDER_MODE` / configured mode is not `off`. When triggered it emits `{type:"todo_reminder",content:todos[agentId ?? sessionId] ?? [],itemCount:length}`. The setting defaults to `baseline` unless the `tengu_soft_slate_nudge` flag returns `off` (C25622:2389; C25614:3202). Rendering further requires `!xD()` and `C6()` (C26196:20); [UNK] the complete definitions and runtime rollout values of these predicates are not resolved.

[OBS] The renderer creates an `isMeta` message through `ld`; its exact base text is:

> The TodoWrite tool hasn't been used recently. If you're working on tasks that would benefit from tracking progress, consider using the TodoWrite tool to track progress. Also consider cleaning up the todo list if has become stale and no longer matches what you are working on. Only use it if it's relevant to the current work. This is just a gentle reminder - ignore if not applicable.

Nonempty lists append `Here are the existing contents of your todo list:` and `[${s}]`, where `s` is newline-joined `${index+1}. [${status}] ${content}` (C26196:20).

[OBS] The task variant scans the same way for either of two task-management tool names, requires `xD()`, the update tool and the same brief/mode/history gates, uses the same 10/10 constants, and attaches `task_reminder` with current tasks (C25622:2389). Its renderer starts “The task tools haven't been used recently” and appends `#${id}. [${status}] ${subject}` entries; see the exact builder at C26202:47. It is gated again by `zfe()`.

## Plan, auto, file and hook injections

| Family | Trigger and insertion builder |
|---|---|
| Plan | [OBS] `$Dr` only emits while permission mode is `plan`. It scans back to the last plan/plan-reentry or exit attachment and counts non-meta, non-special user messages. If a previous plan attachment exists, fewer than **5** user messages suppresses another, except when the plan representation changes. Full reminder occurs when `(priorPlanAttachments + 1) % 5 === 1`; others are sparse. It may first emit `plan_mode_reentry` for an existing prior plan, then `plan_mode`, with path, existence, agent and custom-instruction fields (C25617:3611, C25617:6194, C25614:3202). The renderer says “Plan mode is active” and forbids writes except its described plan destination; full, sparse, subagent and plan-doc variants are in C26005:558 and C26099:285. Exit emits `plan_mode_exit` after leaving plan if a prior plan attachment or exit latch exists (C25617:7017). |
| Auto | [OBS] `BDr` considers `auto` and `bypassPermissions`; selected model/tool, main-agent and feature conditions determine whether an `auto_mode` attachment with `steerOnly`, `bashFirst`, `finishExposedWork`, consent and bypass fields is emitted. It searches the previous auto/exit attachment to avoid duplicate reminders. `UDr` has a main-agent-only exit path with additional latch conditions (C25617:7521). [UNK] Resolve all predicate and template variants before cloning this behavior. |
| Mentioned file | [OBS] The prompt producer detects file mentions unless evaluation confinement or file-mention switches suppress them (C25614:3939). A validated file is read with the Read tool and turned into `file`; text/image/notebook/pdf use `Ale(Read,{file_path})` plus `xle(Read,result)` in the renderer, and oversized text can get a truncated-file meta note (C25622:368, C26185:933). This is a synthetic Read-shaped exchange in the renderer, not a claim that a human typed a tool request. [UNK] Exact relative placement after conversion and all size limits. |
| Repeated file | [OBS] For an `@` mention, cached content with matching timestamp can yield `already_read_file` rather than a fresh file attachment (C25620:10668). The Read module recognizes the prefix `<system-reminder>This file is already in your context` and also has an unchanged-file tool-result text (C18720:521). [UNK] The complete builder and gate for that exact prefix were not located; do not equate the two paths. |
| Hooks | [OBS] SessionStart gathers nonempty `additionalContexts` into a `hook_additional_context` attachment with event and tool-use ID `SessionStart`; Setup has an analogous builder (C24107:32897, C24107:33997). After a tool, `PostToolUse` can emit `hook_additional_context`, `hook_stopped_continuation` or a blocking error and can rewrite eligible tool output (C25737:54151). This establishes internal attachment insertion after hook execution, not a universal outer API role. [UNK] Exact formatter and every hook event's ordering. |
| Nested instruction files | [OBS] Pending path triggers are drained by `tPn`; `CLAUDE_CODE_DISABLE_CLAUDE_MDS` clears/suppresses them (C25618:9480). `ZAn` skips already-read/announced content, can truncate non-managed text, seeds file-read state and emits `{type:"nested_memory",path,content,displayPath}` (C25617:22155). It preserves origin details for loaded-instruction hooks. [UNK] Exact wire location and complete nested-file renderer. |
| Recalled memories | [OBS] The candidate selector returns at most five not-yet-surfaced visible memories and emits `relevant_memories` when local content is present; the session-byte budget constant is `61440` (C25618:11175, C25614:3202). Its renderer begins “Retrieved for possible relevance — use only if it actually applies to what the user asked.” and emits each memory as an `isMeta` message after neutralization; an optional citation-tag instruction is flag-dependent (C26202:47; `evidence/12-rendering.txt`). [UNK] Full recall eligibility and selector policy. |
| Restored skill bodies | [OBS] After compaction, `invoked_skills` renders each name, path and body and explicitly says these were invoked earlier, not this turn, and must not be re-executed; embedded request text is not a new request (C26185:933). Exact template: `evidence/12-rendering.txt`. [UNK] Complete capture/reinsertion conditions. |
| ToolSearch nudge | [OBS] Backward scans count assistant messages since the last ToolSearch call and last nudge. Both counts must reach `everyNTurns`; further gates include enabled feature, `tst` mode, supported model, ToolSearch availability, an undiscovered tool, and no same-turn todo/task reminder (C25622:3808). The disabled-by-default feature uses 15 turns and at most 10 names when enabled as boolean true; positive `stride`/`span` values override them (C14729:1064). The `isMeta` template starts “Some available tools' schemas are not loaded in this conversation yet: ${y}” and recommends `select:<name>[,<name>...]`; exact builder at C26208:50. [UNK] API message placement. |

## CLAUDE.md and rules discovery

[OBS] The path resolver distinguishes `User`, `Project`, `Local`, `Managed`, and `AutoMem`: user `CLAUDE.md`, workspace `CLAUDE.md`, workspace `CLAUDE.local.md`, managed `CLAUDE.md`, and auto-memory respectively; it also defines managed `.claude/rules` and user rules roots (C14742:11062). The schema identifies `./.claude/CLAUDE.md` as the alternate project instructions file (C14522:380296). `claudeMdExcludes` filters User/Project/Local paths but not Managed; managed `claudeMd` is honored only from policy settings (C13880:102109).

[OBS] `F2` recursively loads include paths, maintains a processed-path set, rejects configured exclusions, special-device paths and unsafe unresolved links, and uses `WYr=5` as its depth cutoff (`h>=5` is rejected). External includes and user symlinks have additional gating (C23140:23833). `RSe` recursively walks rules and filters conditional versus unconditional files on whether `globs` are present (C23140:24790). Additional project-directory loading is gated by `CLAUDE_CODE_ADDITIONAL_DIRECTORIES_CLAUDE_MD` or an internal override (C23140:27222). [UNK] A complete startup traversal and conflict/precedence order were not established; do not infer overriding semantics merely from file type names or traversal order.

## Compaction and continuation

[OBS] The feature registry defines `context_management` with beta-header value `context-management-2025-06-27`. In the same registry, `mid_conversation_system` is registered with a null header, while `mid-conversation-system-2026-04-07` exists as a separate string (C14519:151787; C14519:152970). [UNK] Conditions under which either is actually sent, and whether any header specifically identifies a compaction request. Do not infer a live request from a registry literal.

[OBS] The setting `autoCompactEnabled` enables automatic compaction; `DISABLE_AUTO_COMPACT` and `DISABLE_COMPACT` affect it, and the latter also disables manual compact. The worker schema describes the trigger threshold as effective window minus summary buffer, possibly lowered by `CLAUDE_AUTOCOMPACT_PCT_OVERRIDE`; sources are reported as `env`, `settings`, `clientdata`, `experiment`, `model-default`, `unknown-model`, or `auto` (C14522:295082). It can defer to a reactive prompt-too-long path rather than enforce that threshold. [UNK] Exact buffer and full resolver math were not reconstructed.

[OBS] Full compaction creates `compact_boundary` with `content:"Conversation compacted"`, trigger `auto` or `manual`, pre-token metadata and UUID, plus an `isCompactSummary` user record and restored attachments/hook results (C26325:2800, C24978:3440). Partial compaction can also preserve a selected message segment and records summary direction and count (C24979:5303). The summary prefix literal is “This session is being continued from a previous conversation that ran out of context. The summary below covers the earlier portion of the conversation.” (C17863:147). `PreCompact` and `PostCompact` hook input schemas distinguish `manual`/`auto`; PostCompact receives `compact_summary` (C14521:54939). [UNK] Exact request headers/betas for context compaction and precise API wire ordering are unverified; a `compact_boundary` transcript event is not proof of an HTTP header.
