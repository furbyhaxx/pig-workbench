---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-12-instructions-and-injections"
  title: "agy 1.4.12 instructions and context injections"
  description: "Prompt assembly, rules, dynamic context, tool feedback and hook injections"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:02:40+01:00"
  updated_at: "2026-11-03T12:02:40+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document instruction discovery and context injection evidence"]
---

# agy 1.4.12 — instructions and injections

**Evidence and limits.** `[OBS]` means bytes, a protobuf descriptor, a function or
installed documentation was observed; it does not mean the corresponding branch was
executed. `[INF]` is an interpretation of those observations. `[UNK]` identifies a
boundary not established here. The binary is 234,144,144 bytes, version 1.4.12,
SHA-256 `1edfb30477447825f0aaa05fd9a91d82f9adf26686f41cd4f5c70b90abe383e8`
([identity and commands](evidence/context-recovery/cli-and-settings.txt)). No model
request was made: exact wire roles, ordering and branch conditions must not be inferred
from a string's mere presence. Offsets are file offsets in this binary.

[OBS] The 1.4.12 binary still contains separate system-prompt builders and typed step
converters ([function index](evidence/context-recovery/function-index.txt)); the
1.4.8 trajectory and projection model is described in
[1.4.8/06-mechanics.md §1.4](../1.4.8/06-mechanics.md). A converter's
`ToOutputString` establishes a trajectory output to be projected, not by itself the
vendor-specific message role. The earlier chapter's statement that tool results are
not separate model messages should not be used to predict the final wire representation.

## 1. Workspace and rule context

[OBS] `utils.BuildWorkspacePrompt` constructs the exact strings below. With active
workspaces it emits `Active Workspaces:` and an item `- %s`; it also has
`Command Working Directory: %s`. When there is no active workspace its template is:

> The user does not have any active workspace. If the user's request involves creating a new project, you should create a reasonable subdirectory inside the default project directory at %v. If you do this, you should also recommend the user to set that subdirectory as the active workspace.

The recovered addresses and lengths are in
[exact templates](evidence/context-recovery/exact-templates.txt), starting at
`0x5c4c185`, `0x5b2167a`, `0x5cf467c` and `0x5e24320`. [UNK] The final message
role and its order relative to system sections have not been traced. `ActiveUserState`
also has `active_document`, `open_documents`, and `active_node` fields; their presence
is not proof that the CLI includes each one in every request. A git-status injection
has not been established ([schemas](evidence/context-recovery/proto-schemas.txt)).

[OBS] The installed customization guide describes workspace roots `.agents/`,
`.agent/`, `_agents/`, `_agent/`, found walking from the working directory toward the
repository root; hierarchical rules can be `GEMINI.md`, `AGENTS.md`, or
`.agents/rules/*.md`. Directory files apply in their own directory and descendants.
The guide describes initial discovery from CWD and additional discovery as files are
opened or edited. Standalone `AGENTS.md` and `GEMINI.md` are plain Markdown,
always-active directory rules. Plugin rules are loaded when the plugin is enabled.
Resolved paths are deduplicated. [OBS] These are installed-guide statements, not a
runtime trace; the guide variously says deduplication per conversation and per turn,
so the exact lifetime is [UNK].
([guide](evidence/context-recovery/builtin-customization-docs.txt),
[referencing functions](evidence/context-recovery/xrefs.txt)).

[OBS] The guide's customization collision priority, highest first, is workspace
project, workspace-declared JSON, global discovery, built-ins, then globally declared
JSON. This is a claim about customization name collisions; do not treat it as a
proved ordering of every simultaneously applicable rule. The system section's rule
preamble explicitly gives user rules precedence over other instructions; an embedded
release note confirms that earlier built-in instructions are included in that claim
([release extract](evidence/context-recovery/release-notes-extracts.txt)). A separate
higher-level safety policy could still constrain execution; none was tested here.

[OBS] `UserRulesSection.Content` builds a **system-prompt section** named
`user_rules`, with these exact components (`0x5e09cdd`, `0x5ded1b4`,
`0x5d2ef39`; [disassembly](evidence/context-recovery/template-disassembly.txt)):

```text
The following are user-defined rules that you MUST ALWAYS FOLLOW WITHOUT ANY EXCEPTION. These rules take precedence over any other instructions.

Review them carefully and always take them into account when you generate responses and code:

<RULE[%[1]s]>
%[2]s
</RULE[%[1]s]>
```

The displayed blank-line placement is explanatory; the exact recovered strings are in
[exact templates](evidence/context-recovery/exact-templates.txt). The wrapper is a Go
format string, with positional substitutions for the rule label and content. [OBS]
`DirectoryRulesStringConverter.ToOutputString` emits a separate trajectory-step
representation with `content` and `rule_file_uris`; its exact preamble is:

> These rules provide important context when working with this part of the codebase, so take them into account as you work. Their contents are injected below:

[INF] This second path carries rules discovered while operating in a directory into a
subsequent prompt. [UNK] Its final wire role and exact position relative to the tool
result need a captured request; existence of the converter does not prove that it is a
new user message.

[OBS] The installed guide states a **24,000-byte per-rule limit after
`@[label](path)` includes**, truncating at line boundaries, and a separate
**20,000-token aggregate budget** for global and always-on rules. Overflow is
converted to path/description pointers; built-in plugin rules are described as exempt.
The system builder's exact overflow text is `The following user-defined rules exceeded
the rules token budget and were excluded from inline injection. View the file for
further instructions when relevant:`. Conditional rules are listed as
`- [absolute path]: [description]` and, when relevant, the model is told to view the
file. `model_decision` and `glob` triggers and `description`/`globs` validation are
visible in the guide and release notes. [UNK] Exact accounting and conditional-rule
activation order need an execution trace. Sources: [guide](evidence/context-recovery/builtin-customization-docs.txt),
[release notes](evidence/context-recovery/release-notes-extracts.txt),
[templates](evidence/context-recovery/exact-templates.txt).

## 2. Skills, time, mentions and media

[OBS] Skills have a dedicated **system-prompt section**. Its complete 1,757-byte Go
template is [skill-template.txt](evidence/context-recovery/skill-template.txt),
offset `0x5e8c9be`. Ordinarily the template lists each skill's name, exact path and
description, instructing the model to read a relevant `SKILL.md`; preloaded skills
instead include full content inside `<preloaded_skill name="…" path="…"
description="…">`. `.Data.TruncatedPrompt` is a separate optional template value.
Discovery, frontmatter, slash invocation and the unknown budget are in
[14-skills-and-commands.md](14-skills-and-commands.md).

[OBS] `UserInputStringConverter.getOutputStringHelper` builds the exact text
`The current local time is: %s.` using Go's time layout
`2006-01-02T15:04:05Z07:00` (`0x5d00df7`, `0x5cc3814`); it also references
`USER_REQUEST` and scope-item conversion. Thus the timestamp is assembled in the
**user-input step conversion**, not proved to be a system message. [UNK] Its
conditional inclusion and position within the final user message require a request
capture ([disassembly and templates](evidence/context-recovery/feedback-disassembly.txt)).

[OBS] `CortexStepUserInput` has `items[]` (`TextOrScopeItem`), `query`, `images[]`,
`media[]`, `active_user_state`, and `side_channels[]`; the converter has distinct
`Media` and `SideChannels` methods. `ConvertMentionsToPrompt`,
`ScopeItemToMentionMetadataString`, and CLI `splitTextAndMentions` are present.
Rule-file `@[label](path)` inclusion is separate from a user's mention. [INF] User
mentions are converted with the user-input trajectory and media can travel as
non-text parts; the schema alone does not establish eager file-content expansion.
[UNK] File-reference wrapper, media type limits, truncation, relative ordering and
provider wire format need a captured request with a file and media attachment.
([schemas](evidence/context-recovery/proto-schemas.txt),
[mention functions](evidence/context-recovery/mentions-and-edits.txt)).

## 3. Tool feedback, tasks and hooks

[OBS] `CodeActionStringConverter.ToOutputString` constructs text **on the edit's
trajectory output**. Applied and merely suggested edits use different templates:

```text
The following changes were made by %v to: %v. If relevant, proactively run terminal commands to execute this code for the USER. Don't ask for permission.

The previous CodeEdit was suggested by %v to: %v. The diff was not immediately applied to the file due to the changes being flagged as potentially incomplete or incorrect. The user will manually review the changes and may decide to apply them.
```

An edit whose stored diff exceeded its limit can instead carry `(The edit was applied,
but its diff exceeded the storage limit and was not stored.)`. An artifact-feedback
branch adds `Note: You have just created an artifact and requested user feedback. Stop
calling tools to end your turn, and allow the user to review the artifact, approve it,
or provide feedback`. These are model-facing strings in a converter, not evidence that
the model has authority to bypass permission checks. Offsets, complete lint text and
lengths: [exact templates](evidence/context-recovery/exact-templates.txt),
[feedback disassembly](evidence/context-recovery/feedback-disassembly.txt).

[OBS] The same converter's `appendLintErrorMessages` adds IDE diagnostics after edits,
separately introducing new potentially related lint errors and previously reported errors
that persist. It uses `... and %d more lints (truncated for brevity)` and contains a
comparison with 30 (`0x1e`) before truncation. [INF] The comparison is evidence of a
30-item limiting branch, not proof that every diagnostic source has a 30-item global
limit. [UNK] Exact final wire role and all branch conditions require a tool-result
capture.

[OBS] The task-boundary configuration has separate `no_active_task_soft_reminder_tool_threshold`
and `no_active_task_strict_reminder_tool_threshold`; the binary also contains a
`SubagentReminderSection` and `stophooks.buildReminderMessage`
([function index](evidence/context-recovery/function-index.txt)). [UNK] Default
thresholds, todo/task reminder wording, and whether these variants are enabled in the
CLI are not established. Locate their configuration and hook call sites or capture a
turn after the thresholds fire; do not invent a generic todo reminder.

[OBS] Hooks are user-defined context, not ordinary tool output. The installed hook
contract describes `SessionStart` once per conversation, `PreInvocation` before a
model call, `PostInvocation` after tool calls, and `Stop` on attempted termination.
`injectSteps` accepts nested `systemMessage`, plain `userMessage`, or transient
`ephemeralMessage`; a `Stop` hook's continuation `reason` is injected as a system
message. A `PreToolUse` hook may shallowly overwrite tool arguments: the guide says a
notice naming changed keys is prefixed to the resulting tool output. `PostToolUse`
expects `{}`. The exact override-notice template and hook-output budgets are [UNK].
([hook contract](evidence/context-recovery/builtin-customization-docs.txt),
[`HookInjectedStep` schema](evidence/context-recovery/proto-schemas.txt)).

## 4. Summaries and child briefs

[OBS] `CortexStepCheckpoint` carries `session_summary`, `user_intent`,
`code_change_summary`, included-step bounds/indices, captured requests, and child/task
snapshots. The current schema also records `model_summarization_failed` and
`used_fallback_summary`. [INF] The structure is consistent with the earlier
compaction model in [1.4.8/06-mechanics.md §1.4](../1.4.8/06-mechanics.md), but does
not prove identical projection. [UNK] The summary prompt, exact summary wrapper,
budget, and final message role were not recovered. Inspect checkpoint conversion and
capture a compacted model request.
([schema](evidence/context-recovery/proto-schemas.txt)).

[OBS] A `SubagentSpec` carries `type_name`, `role`, `initial_prompt`, and `inherit`;
`SubagentsSection.Content` builds an available-subagents system section. Explicit
subagent slash invocations have the exact `<SUBAGENT>…</SUBAGENT>` template reproduced
in [14 §3](14-skills-and-commands.md). [UNK] The precise ordering and role of a
child's initial brief, inherited trajectory, and generated worker reminders are not
proven; compare parent and child model requests. The underlying child trajectory and
isolation structure is already covered by
[1.4.8/06-mechanics.md §5.1](../1.4.8/06-mechanics.md).
