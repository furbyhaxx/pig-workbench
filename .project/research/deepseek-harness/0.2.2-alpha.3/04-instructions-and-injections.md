---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-04-instructions-and-injections"
  title: "DeepSeek Harness 0.2.2-alpha.3 instructions and injections"
  description: "Source-derived prompt assembly and model-visible context producers"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:02+01:00"
  updated_at: "2026-11-03T11:54:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document inspected prompt and context producers"]
---

# Instructions and injections

[OBS] Source snapshot: `/tmp/re-recon/dsh`, commit `8bd073392eef6f54e921a093b95076248eb08cbb`. Source paths below are relative to that checkout; no target execution was performed. `evidence/injection-inventory.tsv` lists inspected producers, triggers, roles, builder locations, limits and explicit unknowns. `evidence/injection-templates.txt`, `evidence/state-templates.txt`, and `evidence/system-section-templates.txt` copy source builders verbatim; their `${...}` expressions are source syntax, not claimed concrete output. `evidence/inventory-commands.txt` records discovery commands and output.

## Assembly and attribution

[OBS] Before a step, the loop claims inbox messages, resolves tools and assembles prompt contributions, then runs the `agent/pre-step` waterfall. Its default decision appends a changed runtime-context snapshot to claimed messages; listeners can reject, extend, prepend or replace a decision. Contributions are logged as messages. System sections are ordered by numeric order, with equal orders broken by code-unit name; runtime contexts have a separate ordered list. The central section slots include identity -1000, persona prefix 0, plan 500, team 600, file reference 900, tool-specific sections 1000 onward, MCP 3100 and persona suffix 10200; runtime context slots include cwd 100, sandbox 110, approval 115, delegation 120 and harness source 200 (`packages/core/agent-loop/src/agent.ts:282`; `packages/core/system-prompt/src/index.ts:136`, `:177`, `:249`, `:289`).

[OBS] A `system-prompt` message has system role. Most dynamic producers call `createUserMessage` and therefore enter as user role with a typed `source.kind`; forms include `instructions`, `catalog`, `snapshot`, `notice`, `relay`, and `recall`. A source is attribution, not automatically a separate authority level. The tool-registry change producer instead emits a developer-role message with structured tool-addition/removal blocks (`packages/llm/llm/src/message.ts:35`, `:110`; `packages/core/agent-loop/src/agent.ts:685`).

[OBS] Runtime context is emitted only when the retained snapshot changes. On clearing a prior snapshot the exact text is `Current runtime context: none. Earlier runtime-context snapshots no longer apply.` (`packages/core/agent-loop/src/runtime-context.ts:19`, `:155`). Cwd context is `Current working directory: ${JSON.stringify(this.get(session))}.`; committed changes and unavailable-directory recovery add separately attributed user messages (`packages/session/working-directory/src/index.ts:170`, `:191`). Sandbox and approval policy render alongside it; their policy and literal builders are in `05-modes-and-approvals.md` and the evidence files (`packages/sandbox/sandbox-policy/src/index.ts:60`; `packages/interaction/user-approval/src/index.ts:74`).

## Time and environment

[OBS] `time-context` appends a user-role snapshot on an accepted pre-step unless a prior injection remains within `refreshIntervalMs`; the source fallback is 600,000 ms and zero disables interval suppression. The body is built as `Time sampled while preparing turn ${turn}, step ${step}: ${formatTimestamp(...)}\n`, then one browser-zone policy line, then `Elapsed since the preceding ${baseline}: ${elapsed}.` The baseline is `model-visible message` on step 1, otherwise `step context`. A unique valid browser zone is used for interpretation; mixed or missing zones ask the user to clarify unqualified dates. The timestamp includes offset and `[zone]` (`packages/context/time-context/src/index.ts:98`, `:133`, `:199`; `packages/context/time-context/src/request-zone.ts:62`, `:81`; `packages/context/time-context/src/timestamp.ts:30`).

[OBS] `tmux-context` runs only for accepted first steps when a shell exists; it verifies that `TMUX_PANE` matches the process tty, optionally rate-limits probes, and prepends a user-role snapshot only when the tmux state changes. Its builder is `tmux location (turn ${turn}):\nsession ${sessionName}, window ${windowIndex} ${JSON.stringify(windowName)}, pane ${paneIndex} ${paneId}\nwindow active=${windowActive}, pane active=${paneActive}, layout ${windowLayout}`. Failed queries and absent/mismatched panes inject nothing (`packages/context/tmux-context/src/index.ts:86`, `:120`, `:175`, `:240`).

[OBS] The optional harness-dev context adds system sections for the source checkout and Web GUI orientation plus runtime-context values for the current checkout and advertised GUI URL. Both literal paragraphs and source builders are captured in `evidence/injection-templates.txt`; the plugin also recognizes legacy prompt paragraphs rather than always replacing them (`packages/context/harness-dev-context/src/index.ts:28`, `:36`, `:89`, `:122`).

## Workspace instruction discovery and precedence

[OBS] Baseline discovery checks `$DSH_HOME/AGENTS.md` and `$DSH_AGENTS_HOME/AGENTS.md`, then walks project root to cwd. The first upward `projectRootMarkers` match identifies root, default `.git`; with no marker, cwd is root. In each directory it examines all ordinary candidates, default `AGENTS.md`, `CLAUDE.md`, followed by local candidates `AGENTS.local.md`, `CLAUDE.local.md`. Files are ordered broad to specific; within one candidate group, later files with identical trimmed-content digest are deduplicated. Both global roots share a candidate group (`packages/context/agent-instructions/src/config.ts:11`, `:41`; `packages/context/agent-instructions/src/files.ts:79`, `:232`, `:323`, `:430`).

[OBS] Baseline messages use source `agent-instructions`, form `instructions`, a `<system-reminder>` frame and headings `Instructions from: ${file.displayPath}`. The intro says more specific instructions take precedence, but do not override system, developer, or direct user instructions. Changes use distinct additional, updated and removed builders; closing tags in body text are escaped. After successful `read`, `write`, or `edit`, touched descendant instruction scopes are reconciled and inserted immediately after claimed messages on an accepted step. A changed workspace baseline replaces previous baseline guidance (`packages/context/agent-instructions/src/render.ts:16`, `:99`, `:221`, `:257`, `:315`; `packages/context/agent-instructions/src/index.ts:72`, `:330`, `:356`).

[OBS] `maxBytes` is a required per-chain rendered UTF-8 budget; `maxSourceBytes` defaults to 1,048,576 per source file, and oversized files are skipped. If total does not fit, the renderer drops broadest files first, then UTF-8-safely truncates the most specific file and emits omission/truncation markers; small budgets can fall back to a compact intro, heading or marker. User-global and project chains can each consume the configured budget and the merged frame is bounded by twice that value. No aggregate source-read cap was established (`packages/context/agent-instructions/src/config.ts:14`, `:41`; `packages/context/agent-instructions/src/files.ts:387`; `packages/context/agent-instructions/src/render.ts:303`, `:333`, `:363`, `:440`).

## References, skills and model changes

[OBS] File mentions contribute a system instruction only when the scoped `read` tool exists: the exact `FILE_REFERENCE_PROMPT` at `packages/context/file-reference/src/index.ts:17` describes `@`-prefixed paths, trailing-directory slashes and quoted spaces; it does not itself read file contents (`packages/context/file-reference-local/src/index.ts:71`).

[OBS] A direct user message with canonical session mentions is rewritten and followed immediately by a `session-reference` user recall. Its frame begins `## Referenced sessions`, describes the JSON as an untrusted read-only snapshot, includes `REFERENCE_WARNING`, then wraps tag-safe JSON in `<referenced-sessions>`. Only user and assistant conversation is projected; tools, reasoning and other injected context are excluded. At most three distinct references are allowed. Explicit `maxReferenceBytes` applies per source; otherwise the automatic estimate has a 65,536-byte floor and uses `floor(contextWindow * 4 * referenceContextFraction)`, default fraction 0.2. Omitted previews may be spilled, with explicit unavailable status on storage failure (`packages/context/session-reference/src/config.ts:3`; `packages/context/session-reference/src/index.ts:55`, `:128`, `:343`, `:371`; `packages/context/session-reference/src/projection.ts:37`, `:73`; `packages/context/session-reference/src/spill.ts:8`).

[OBS] Skills have two context forms: a changed model-visible catalog is a user `skill-catalog` message, while explicit first-line user invocation appends the loaded body as `skill-invocation` instructions. A model-call `skill` result uses the same `<skill_content>` builder. Full source builders and trigger details are in `06-skills-and-commands.md` and the evidence files (`packages/skill/tool-skill/src/index.ts:154`, `:181`, `:217`, `:258`; `packages/skill/skill/src/index.ts:165`).

[OBS] If the selected provider/model differs from the preceding request's route, the accepted step appends `[model changed: assistant turns above this point were generated by ${from}; the session continues with ${to}]`; an effort-only change does not do so (`packages/core/agent/src/model-selection.ts:129`, `:150`).

## Todo, goal, delegation and jobs

[OBS] `todo_write` updates a per-turn durable todo projection and returns a tool-role result; the inspected todo package does not declare its own injected-message source. Its tool description gives either single-active or parallel-active guidance according to required configuration (`packages/todo/tool-todo/src/index.ts:25`, `:44`, `:119`, `:189`).

[OBS] An armed active goal can enqueue a user-role `<goal_round>` follow-up with objective and round number. When an autonomous round marks a goal complete or blocked, `tool-goal` defers a user-role `<goal_complete>` or `<goal_blocked>` instruction telling the agent to provide a grounded final report. Exact builders are in `evidence/state-templates.txt` (`packages/goal/goal-round-driver/src/prompt.ts:11`; `packages/goal/goal-round-driver/src/index.ts:172`, `:192`; `packages/goal/tool-goal/src/wrapup.ts:5`; `packages/goal/tool-goal/src/index.ts:300`).

[OBS] An agent-to-agent message is a user-role relay prefixed `Agent ${sender.id} sent a message: `. Parent-delivered child completion uses a `subagent-settled` user notice whose first line depends on completion, abortion, token limit, refusal or error, followed by available closing text, structured result and diagnostic. A separate `team-message` type is documented as a historical mailbox source; a current producer was not established (`packages/subagent/subagent/src/continuation-messages.ts:62`, `:99`, `:137`; `packages/team/agent-team/src/types.ts:108`).

[OBS] Uncollected job settlement can inject a user notice for a busy owner or wake an idle owner; its exact inline and fallback builders are in the evidence. Inline output is bounded by `inlineOutputBytes` (default 2,048 UTF-8 bytes) and any producer output cap; otherwise the notice directs the model to `job_output` (`packages/jobs/tool-jobs/src/index.ts:40`, `:127`, `:365`). Schedule delivery uses a user follow-up built from `[SCHEDULE REMINDER]` or `[SCHEDULE REMINDER BATCH]` and JSON-escaped fields. A separate `[SCHEDULE CHANGE]` notice informs involved live sessions without waking them (`packages/schedule/schedule/src/domain.ts:1151`; `packages/schedule/schedule/src/runtime.ts:349`; `packages/schedule/schedule/src/index.ts:703`).

## Guards, compaction and hooks

[OBS] Repeat-tool reminders are user `notice` contexts attached after post-execution; default thresholds are 3, 5 and 8, and only the argument preview is capped (500 characters by default). The gentle and detailed strings are preserved in evidence. Timeout failures appear as tool results, not as an independently sourced user message (`packages/guard/repeat-tool-reminder/src/index.ts:53`, `:70`, `:77`, `:196`, `:220`; `packages/guard/timeout-policy/src/index.ts:40`).

[OBS] Successful compaction replaces a selected surface span with a user `compact-checkpoint` message framed by a fixed preamble and `<compacted-summary>` tags. Its auxiliary summarization call ends with the exact structured directive copied to `evidence/state-templates.txt`. Configured token policies and the distinction between the append-only log and replacement surface are described in `07-session-and-compaction.md` (`packages/compaction/compaction-basic/src/summarizer.ts:21`, `:31`, `:70`, `:188`; `packages/compaction/compaction-basic/src/region.ts:491`).

[OBS] Optional experimental Codex and Claude Code hook bridges can return hook-supplied `additionalContext` as user messages at SessionStart, prompt submission, or post-tool; a blocking Stop hook steers its reason, defaulting to `continue: blocked by Stop hook`. Both can veto pre-step and pre-tool decisions; the Claude bridge also maps an ask decision. The context text is hook output, not a fixed universal template (`packages/experimental/hooks-codex/src/index.ts:174`, `:187`, `:201`, `:225`, `:235`, `:254`; `packages/experimental/hooks-claude-code/src/index.ts:196`, `:207`, `:224`, `:245`, `:251`, `:270`).

## Other sources and unresolved coverage

[OBS] Other source-declared model inputs include deferred PTC images, late user-question answer JSON, optional action-card status, webhook delivery and dynamic Cordis failure messages. Their observed producer sites and known gaps are in `evidence/injection-inventory.tsv` (`packages/core/tool-executor-ptc/src/ptc.ts:603`; `packages/interaction/user-questions/src/index.ts:191`; `packages/experimental/action-cards/src/index.ts:238`; `packages/experimental/webhook/src/session.ts:128`; `packages/extensions/cordis-host-runner/src/index.ts:1062`).

[OBS] The inventory also names inspected system-section registrations for filesystem, shell, LSP, web, MCP resources, schedules, goals, teams, subagent orchestration, deliverables and optional experimental tools. Their registration conditions and literal source excerpts are in `evidence/system-section-templates.txt`; executor-owned sections are dynamically registered and can vary (`packages/core/tools/src/index.ts:331`).

[UNK] This is not yet a proven exhaustive catalogue: arbitrary mounted plugins can add producers, some registered section builders depend on other values, and several dynamic output limits and Cordis/webhook templates remain untraced. Compare every hit in `evidence/inventory-commands.txt` with its builder and inspect the mounted profile and hook runner to settle those gaps. The session-title-LLM source is an auxiliary model call, not evidence of another ordinary-turn injection (`packages/session/session-title-llm/src/index.ts:252`).
