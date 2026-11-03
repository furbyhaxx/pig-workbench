---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-02-system-prompt"
  title: "Codex CLI base prompts and instruction assembly"
  description: "Exact bundled prompt evidence, model selection, instruction roles, and conditional fragments"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["documented source-derived base prompt resolution and conditional context"]
---

# System prompt and instruction assembly

## The prompt is not one fixed file

[OBS] `protocol/src/prompts/base_instructions/default.md:1-275` is reproduced
in full in [evidence/prompt/base_instructions-default.md](evidence/prompt/base_instructions-default.md).
Its Markdown `document` frontmatter was added locally; **everything after that
frontmatter is the exact source file**. The source is included as
`BASE_INSTRUCTIONS_DEFAULT`, used by `BaseInstructions::default()`
(`protocol/src/models.rs:1694-1724`). It is **not** the normal-session selection
rule. The source's sections cover identity/capabilities, personality, AGENTS.md,
responsiveness, planning, task execution, validation, ambition, progress updates,
final-answer formatting, shell and plan-tool use (`protocol/src/prompts/base_instructions/default.md:1-275`). Its text is evidence, not guidance
for this analysis.

[OBS] Session initialization chooses, in priority order: explicit
`config.base_instructions`; base instructions restored from conversation history;
otherwise the current model's rendered instruction text
(`core/src/session/mod.rs:834-837,871-875`). The misleadingly named
`instructions_template` is literal text (`protocol/src/openai_models.rs:584-585`);
the renderer returns that text, or warns and returns an empty string if absent
(`prompts/src/model_instructions.rs:8-16`). An unknown model gets a synthetic
model descriptor whose instruction text comes from `models-manager/prompt.md`
(`models-manager/src/model_info.rs:100-112,150-154`); its complete bytes are in
[evidence/prompt/unknown-model-fallback.txt](evidence/prompt/unknown-model-fallback.txt).
Do not confuse that file with `protocol/.../default.md`.

[OBS] The bundled catalog holds eight distinct literal instruction strings. The
complete decoded strings and exact source-line/slug mapping are in
[evidence/prompt/index.txt](evidence/prompt/index.txt). For example, catalog lines
`models-manager/models.json:76,425` share one string, while lines `:250`, `:600`,
`:767`, `:943`, `:1358`, `:1474`, and `:1584` identify the other seven. These are
catalog identifiers in this snapshot, not a claim about current availability.
[OBS] Catalog refresh may fetch `/models`, use cache, and merge remote entries over
same-slug bundled entries, or use remote-only data under conditions; therefore the
bundled text alone cannot establish the bytes of an unobserved live session
(`models-manager/src/manager.rs:616-649,653-738,764-801`). An explicit custom
base override replaces model instructions. Otherwise `personality = none` removes
the exact H1 `# Personality` section until the next H1, if present
(`models-manager/src/model_info.rs:46-88`).

[OBS] When `update_plan` is disabled, Codex-owned instruction text can have the
planning/checklist sections removed; custom catalog or configured text has
separate preservation conditions (`core/src/session/world_state.rs:81-99`;
`prompts/src/update_plan_instructions.rs:4-51`).

## Message roles and order

[OBS] The Responses request builder prepends a nonempty base as a `developer`
message, classified `model.base_instructions`, rather than proving a separate
wire `system` message (`core/src/client.rs:1053-1090`;
`core/src/context/base_instructions.rs:12-56`). For Responses Lite, a developer
`AdditionalTools` prefix precedes that base when there are tool declarations
(`core/src/client.rs:1053-1078`). The client later inserts history after the
prefix. Ordinary developer instructions have role `developer`
(`core/src/context/developer_instructions.rs:20-44`); loaded `AGENTS.md`
instructions are wrapped in `# AGENTS.md instructions` / `<INSTRUCTIONS>` and
carry role `user` (`core/src/context/user_instructions.rs:13-42`;
`core/src/agents_md.rs:473-486`). These roles describe client construction;
[UNK] effective precedence after a provider interprets the request is not
observable from this source snapshot.

[OBS] The world-state builder visits these sections in this order, although a
section can be absent, unchanged, coalesced, or emitted separately: incremental
tools; model; token-budget/context-window guidance; realtime; AGENTS.md;
permissions; collaboration mode; persistent mode; environment and deferred
executor; apps; plugins; deferred tools; extension-contributed state; multi-agent
usage and mode; model catalog; managed developer requirements
(`core/src/session/world_state.rs:178-446`). Initial rendering groups developer
and contextual-user fragments, handles model-switch instructions specially,
and can emit standalone guardian and managed developer messages
(`core/src/session/mod.rs:4810-4836,4950-5070`). This is a construction order,
not a claim that every section appears in every request or stays contiguous.

[OBS] Ordinary model changes may emit a developer `<model_switch>` fragment;
in incremental-tool windows the base can be part of persisted prefix state and
later changes emit a developer `<model_instructions>` replacement or removal
notice (`core/src/context/world_state/model.rs:83-139`;
`core/src/context/model_switch_instructions.rs:19-41`;
`core/src/context/model_instructions.rs:8-53`).

## Conditional text families

All bundled template files listed below are copied without elision in
[evidence/prompt/index.txt](evidence/prompt/index.txt); `.txt` copies preserve
source bytes. Catalog overrides and runtime data can change the result.

- **Sandbox and approvals.** [OBS] If permission instructions are enabled, a
  developer fragment selects `danger_full_access`, `workspace_write`, or
  `read_only`, substitutes network access, and appends approval-policy prose,
  allowed prefixes, writable roots and denied reads. Approval policies include
  `never`, `unless_trusted`, `on_request` and granular; reviewer selection,
  execution-permission approvals and `request_permissions` add further text.
  Catalog strings can replace bundled sections
  (`core/src/session/world_state.rs:238-274`;
  `prompts/src/permissions_instructions.rs:102-170,310-457`;
  `prompts/src/model_messages/permissions.rs:8-21,33-55,75-115`).
- **Collaboration modes.** [OBS] Default and Plan select catalog or bundled
  mode text, subject to configuration; custom developer instructions can be
  supplied and built-in plan guidance can be stripped if the tool is disabled
  (`core/src/context/world_state/collaboration_mode.rs:27-77`;
  `collaboration-mode-templates/src/lib.rs:1-2`).
- **Persistent mode.** [OBS] An enabled non-guardian session gets developer
  instructions; `{{ approval_request_channel }}` is replaced with a channel
  string or empty text based on asynchronous-user-message availability. State
  transitions also generate replacement/removal notices
  (`core/src/session/world_state.rs:284-298`;
  `core/src/context/world_state/persistent_mode.rs:16-17,52-70,88-113`).
- **Multi-agent.** [OBS] V2 selects root or subagent role text by session source;
  configured roles take precedence over catalog/bundled text. Composed text can
  add shared filesystem guidance, concurrency, wait and model-override hints;
  effective mode also depends on configuration and reasoning effort
  (`core/src/session/multi_agents.rs:14-109`;
  `prompts/src/multi_agent_instructions.rs:10-21,39-96`).
- **Realtime.** [OBS] Realtime start/end transitions use supplied start/end text
  or bundled templates; a separate backend prompt is also exported
  (`core/src/context/world_state/realtime.rs:26-52`;
  `prompts/src/realtime.rs:1-3`).
- **Compaction.** [OBS] Inline automatic compaction uses configured
  `compact_prompt` or the bundled summarization prompt as synthesized user text;
  a summary prefix is also exported (`core/src/compact.rs:119-141`;
  `prompts/src/compact.rs:1-2`).
- **Review.** [OBS] A review subagent receives the review rubric as custom base,
  disables web search and collaboration, and uses approval policy `Never`; the
  user-facing review prompt varies by requested target
  (`core/src/tasks/review.rs:105-128`; `prompts/src/review_request.rs:9-38,63-108`).
- **Guardian.** [OBS] A reviewer policy substitutes tenant and extra-policy
  placeholders and appends an output contract; the classifier has its own
  policy, output contract, optional truncation and transcript-format branch
  (`prompts/src/guardian_instructions.rs:11-81,85-149`). Guardian is a separate
  review context, not an unconditional append to the coding base.
- **Goals and memories.** [OBS] Goal continuation, budget-limit and objective
  updates render separate templates using goal state; continuation can remove
  plan-tool guidance when that tool is disabled (`ext/goal/src/steering.rs:10-94`).
  Memory read-path instructions select a V1 or V2 template and interpolate a
  nonempty, truncated stored summary and base path
  (`ext/memories/src/prompts.rs:10-62`). Their template copies are in the prompt
  evidence index; stored memory contents are not part of this snapshot.
- **Other context.** [OBS] Runtime world state can add environment, tool,
  app/plugin, extension, managed requirements and model-catalog text under their
  respective gates (`core/src/session/world_state.rs:300-446`).

[UNK] The exact bytes of user/configuration/extension/MCP contributions, remote
catalog overrides and every possible conditional composition are not fixed by
this checkout and were not observed in a live session.
