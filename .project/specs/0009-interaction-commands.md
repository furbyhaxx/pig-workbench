---
document:
  type: "specification"
  id: "SPEC-0009"
  title: "Questionnaires, answer extraction, checkpoints and command UX"
  description: "Interactive state machines, remote answers, inference boundaries and the complete user command contract"
  status: "approved"
  revision: 11
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-05T18:05:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 11
      reason: "User approved the contract freeze on 05.11.2026"
      changes: ["approved for Slice 1 implementation; ask_user state machine, modes and command routes frozen"]
---

# 1. Scope and sources

Draft behavior for ask_user, /answer/replay, /tldr, codemode interaction, tools inspector and all user command routes. No concrete inference model is prescribed; use configured host model references and capabilities.

Depends on [services](0001-extension-services.md), [presentation](0003-tool-presentation.md), [locations/jobs](0008-locations-jobs.md), [settings/state](0010-configuration-state-trust.md). Sources: `host:/mnt/Projects/furbyhaxx/pi-workbench/src/ask/{types,schema,answer-commands,remote-ask}.ts`, ask/config/schema.ts, core/commands.ts and tldr/. SDK mode constraints verified in bundled rpc-extension-ui.md: custom views do not exist in RPC; select/input/editor/confirm dialogs do.

# 2. ask_user contract

Input:
```json
{"title":"Storage policy","questions":[{"id":"scope","label":"Scope","prompt":"Where should history be discoverable?","type":"single","required":true,"options":[{"value":"project","label":"Project only","description":"Restrict discovery to this project"},{"value":"global","label":"Project and global","description":"Require explicit authorization for global content","recommended":true}]}]}
```

Question fields: id unique stable string, optional label, prompt, type single|multi|preview|secret (default single), required advisory boolean (default false), options[]. Option values are unique within question; label required, description/preview/recommended optional. Preview type requires nonempty preview on every option. An empty options[] permits freeform-only input without fabricated choices. Title/IDs/text are bounded: 32 questions, 100 options/question, 64 KiB total prompt payload by default. Validation names the exact field.

Type `secret` requests a credential from the user (SPEC-0013 §7): it carries `secret: {name, scope: project|global, env?, description?}`, has no options and no freeform choice, and its answer is the placeholder plus metadata; the value goes to the vault, never into the transcript, answer history or frames. In a form, a secret question is one step like any other; review shows `stored as red-sec-…`.

Custom freeform is always available, separate from model-provided choices; no need to append a fake Other option. Several custom answers can exist for multi mode. Requested type and presented type are retained: user may change single↔multi or use configured presentSingleAsMulti without changing what the original request asked. Preview content is data, never executable HTML/commands.

Advisory required marks importance but **never blocks submission or dismissal**. Omitted question remains unanswered, not a default selection. Recommended is visual metadata with a reason in description; it never auto-selects or submits.

Output retains donor semantic fields:
- cancelled boolean, mode submit|elaborate, title and question summaries.
- answers keyed by question ID: selected values/labels/zero-based indices, customTexts, question note and per-option notes.
- optional questionnaire note.
- optional elaboration payload with affected questions/options, notes and nextAction clarify|clarify_then_reask.
- optional continuation with preserved answers, per-question answered/needs_clarification/unanswered state, strategy refine_only|resume.
- actual presented type when user changed it.

Normal cancellation is a successful interaction outcome with cancelled true, not a system error or submitted answers. Malformed input, unavailable UI/bridge and persistence errors are typed tool failures. Return only final confirmed answers to the model; partial drafts belong to recovery state, not silent submissions.

# 3. Interaction state machine

```text
validate → pending → answering ↔ custom editor / question note / option note
                       │
                       ├→ review → submit → completed
                       ├→ clarify selection → elaborate → completed-with-continuation
                       └→ dismiss guard → cancelled / continue
```

Peer question tabs/steps, arrows/page movement, multi-toggle and preview-detail navigation use host/user bindings. Search and editor input own ordinary typing; no bare-letter shortcut can hijack a text field. Numerical selection shortcuts only apply in list focus. Full-flow dismiss differs from closing a note editor/back navigation.

Notes are supported at questionnaire, question and option levels, including unselected options. Review shows what will be submitted or sent for clarification, not just chosen labels. Clarification retains unaffected answers; re-asking does not discard user work. A single-question auto-submit option is allowed only when enabled, one selection is sufficient, no unresolved notes/clarification and no pending custom input. Default explicit review is preserved.

Dirty dismiss asks Continue / Discard; Ctrl+C cancels the flow through the same loss-aware boundary. Saving a note does not submit the questionnaire. Double-press review shortcuts, footer hints and notification preferences are display/behavior settings, never execution permissions.

Default notifications are nonintrusive; terminal bell/OSC variants are opt-in and capability-aware. Custom notification commands are user-only explicit configuration, quoted argument execution without model string interpolation, and subject to policy. Never enable them from a project-supplied question.

# 4. Pending recovery and remote bridge

Each form has askId, sessionId, branch leaf/generation, source tool/message, schema version and state revision. Persist pending payload before showing UI; draft edits may be checkpointed in bounded coalesced updates. Submit uses compare-and-set pending→completed so local UI and remote bridge cannot both win.

On resume, offer Resume / Discard pending form; do not auto-open a blocking modal before the user sees the session. Branch switch invalidates active presentation but retains historical pending facts for that branch. Replay creates a fresh askId tied to the same source questions; it never overwrites a previous answer result. Removed/changed source entry is explicit.

Remote bridge channel names retained for cooperating pi-workbench integrations (versioned Workbench envelopes below; unmodified donor clients are not assumed compatible):
- pi-ask:bridge-discover / bridge-ready: identify bridge instance, supported protocol version, session and modes.
- pi-ask:started / completed: lifecycle with askId and state revision.
- pi-ask:submit / submit-result: addressed validated answers, expected revision and accepted/rejected outcome.

Messages are JSON, session-scoped, versioned (v=1), with correlation ID, askId, instance and generation. Treat these as application protocol over the same bus mechanics as SPEC-0001, not authenticated transport. A bridge crossing processes/projects/networks must provide its own approved authentication; no server/socket exposed automatically.

Remote submit example:
```json
{"v":1,"id":"answer-1","sessionId":"s-1","generation":"g-1","bridgeInstance":"bridge-1","askId":"ask-7","expectedRevision":2,"answers":{"scope":{"values":["project"],"customTexts":[]}},"action":"submit"}
```

Validate option values/custom limits and revision before atomically completing. Duplicate identical submit returns existing receipt; conflicting duplicate, expired form, wrong session or unknown values reject. Timeout/cancel invalidates admission and informs both bridge and local UI. Remote response cannot change question definitions or trust.

Timeout default 5 minutes for remote interaction, configurable user-side; timeout is cancelled/unanswered with reason, not a selected default. Local forms have no forced timeout unless configured. Remote-first vs TUI preference is explicit; never let two independent frontends submit without shared arbitration.

# 5. RPC and headless equivalence

TUI: kit questionnaire, preview pane, tabs, notes and review.
RPC: sequential native dialogs implement the same state; multi uses repeated choice/toggle plus Done, notes use editor, preview text is inspectable before choosing, and final confirm reviews answers. Distinguish cancelled/absent responses from empty text. No ctx.Custom call in RPC.
Print/JSON: if no authorized remote bridge exists, return interactionUnavailable promptly. Do not wait on UI calls the host drops.

Observed ([probes](../analyses/2026-11-02-host-integration-probes/README.md) P28): RPC dialogs arrive as `extension_ui_request` (select/input/confirm) and resume on `extension_ui_response`; an unanswered request blocks forever, so every RPC dialog sets `DialogOptions.Timeout` from the ask settings and maps expiry to an explicit `timedOut` state rather than cancel. In print and JSON modes the host returns `ok=false` / `false` immediately, which looks exactly like a user cancel — Workbench checks `ctx.HasUI()` first and returns interactionUnavailable instead of calling dialogs. If bridge exists, bounded remote interaction is possible and is explicitly reported.

Answer history (user decision D9.c, provisional): every questionnaire outcome (submitted, cancelled, pending, invalid) is also recorded outside the session, per project, as described in G9.8. It is **on by default** with retention `ask.history.retention-days` (default 90) and `/workbench ask history purge [--project|--all]`. It enables searching and reusing earlier answers across sessions. Drafts are never recorded. Secret answers are recorded as their placeholder.

# 6. /answer extraction and replay

/answer captures latest assistant text on current branch plus immediately relevant user prompt/context, excluding tool outputs and unrelated history unless explicitly needed. Source entry ID/digest and generation bind the inference result. Extract questions actually present, not new recommendations disguised as user requests.

One-shot host inference uses configured extraction model preference(s), timeout default 30 s and at most one schema-correction retry. Fallback provider is attempted only when explicitly configured and authorized, not silently to a different billing account. Validate output with the ask schema. No concrete choices offered → freeform. No questions → clear informational result, no empty form. Source changed or session switched while extracting → discard stale result.

The user invokes /answer knowing it performs extraction; UI indicates inference/cancellation and selected provider/model from runtime config. Additional spend/resource permissions follow host policy. Extraction results are suggestions presented to user, not answers sent to the agent.

/answer:again and /ask:replay replay the latest form on the active branch, not an unrelated session. /ask-settings opens shared Ask settings. Source and previous submission timestamps are inspectable; replay never repeats an inference call unless explicitly requested to regenerate extraction.

# 7. /tldr branch checkpoints

Purpose: manual recovery checkpoint without changing conversation branch or pretending to compact host history. Uses one-shot inference, not subagents.

Algorithm:
1. Capture branch leaf/generation and last applicable checkpoint's covered source interval.
2. Select delta since it (or entire branch for first checkpoint); include prior checkpoint context and explicit extra instructions as needed. Expand relevant compaction records from persisted history only under normal permission.
3. Segment by token budget using host tokenizer/estimator, label estimates, keep tool call/result associations. Avoid treating bytes as exact tokens.
4. Generate bounded parallel notes (default concurrency 2) and synthesis, with session cancellation. Requests go through host models and therefore the SPEC-0013 boundary; never use unconfigured external inference endpoints.
5. Validate scope/branch still applicable, append a completed versioned recovery card with covered IDs, source digest, instructions digest, model reference and known usage/duration.
6. Failure/cancel retains old checkpoint; partial notes are not published as complete checkpoint.

Commands: /tldr [extra instructions], regenerate [extra instructions], instructions, config, model [provider/id[:thinking]], cancel, help; same under /workbench tldr. Model resolution checks full reference first before parsing a thinking suffix; do not mistake colon in model ID for a level. Allowed thinking levels come from current host capability, not a hardcoded list copied from another runtime.

Regenerate targets the selected/latest checkpoint interval and creates a new revision linked to the prior card. It does not append a duplicate checkpoint over an accidental newer branch interval. /tldr instructions edits user-configured synthesis instructions through explicit save. Unknown usage/cost stays absent. Default whole operation timeout 120 s; large jobs report stage progress, not fake percentage.

# 8. Complete command route table

Primary /workbench; companion commands delegate to the same handlers. Parse once into typed command variants; unknown input reports exact usage and cheap suggestions. Names/IDs can disambiguate human targets without silently picking.

| Route | No-argument behavior and actions |
|---|---|
| /workbench | Hub with tools/jobs/instructions/frames/locations/settings/checkpoints/integrations/resources |
| /workbench show | Effective snapshot; no model-context side effect |
| /workbench settings [work|edit|read|ask|tldr|jobs|frames|instructions|trust|statusline|tools] | Staged scoped settings; no-UI explicit text/alternative |
| /tools, /workbench tools [name] [--format text|json|markdown] | Searchable actual registry; full schema detail or formatted dump |
| --list-tools | Print registry without starting inference, deterministic format; explicit format selection via associated flag. **Blocked on host:** the flag is parsed and `ctx.Shutdown()` in `session_start` prevents inference, but print/JSON mode has no channel for extension output (P27). Ship once the host provides one; until then the flag reports the limitation in TUI/RPC and `/tools` covers interactive use |
| /workbench preview <compact|normal|full|all> [tool] | Retained/synthetic sample rendering only, no execution |
| /workbench jobs [list|monitor|kill|write|settings] [id], /wb:jobs | Shared jobs service and policy |
| /cd [path|--branch name|--reset], /wb:cd | Location service; TUI picker or status |
| /remote [alias[:/path]|off|reload|add|config] | Host/location manager; per-alias show/note/shell/hide/unhide/rename |
| /workbench frames [ls|show|hide|collapse] [target] | Frame inspection/display policy; not deletion from model context |
| /workbench instructions [ls|pin|unpin|reload|rules] | Applicability reasons, class/pin actions |
| /workbench prompt [show|render --model <ref> [--tools …]|diff] | Template chain, family, delivery, parity, epoch, sections; preview without sending (SPEC-0014 §10) |
| /workbench skills [ls|load|unload] [name] | Catalog with conditions, listed/hidden reason, loaded set, diagnostics (SPEC-0014 §8) |
| /workbench trust [ls|add|rm] | Workbench source trust rules; distinct from host /trust |
| /workbench integrations [owner], /workbench resources [scheme] | Read-only contribution/capability inspection |
| /answer, /answer:again, /ask:replay, /ask-settings | Extraction/replay/shared Ask settings |
| /tldr, /workbench tldr [...] | Checkpoint actions above |
| /workbench statusline | Segment editor (status and activity items: order, pin, hide) and `segments`/`footer` mode switch; footer only by explicit user choice |
| /workbench sessions [ls|send|trust|untrust|block|unblock|hide|show] | Live sessions and messages (SPEC-0011 §7) |

Aliases list/ls, config/settings, rm/remove are canonicalized only where valid. Completion supplies argument-context choices from cached state; never blocks on remote enumeration. Entity actions revalidate IDs after selection. Commands do not inject model text except their explicitly stated frame/location/answer/checkpoint behaviors.

Script-relevant state uses tools/resource APIs rather than interactive slash commands: jobs, cd, read `cfg://workbench/status`, `cfg://workbench/integrations/`, `session://self/instructions/`, `session://self/frames/`, `docs://workbench/schemes/`, write `cfg://ctl`, settings and control routes. No UI toast is the sole headless output.

# 9. Inspector, codemode and status composition

Tools inspector reflects actual schemas, active exposure, namespace, source and Workbench presentation registration. Unknown metadata is labeled unknown. Schema formatting handles unions, objects, arrays and optional fields without losing inspectability. JSON dump is machine-readable and contains no terminal styling.

Observed (P6, P7, P9, P15): child calls raise `tool_call`, `tool_execution_start/end` with `parentToolCallId`, and a same-name Workbench builtin override is used for children too. A child blocked through `tool_call` produces no `tool_result` event and the script sees `reason: {}` (upstream issue) — Workbench records its own block reason by `toolCallId`. `/tools` registers without collision and appears in TUI autocomplete (P26).

Codemode remains PiG's builtin execution engine. Workbench adds semantic nested captures and renders via SPEC-0003, including script facts derived from real source parsing where supported. Do not replace QuickJS or invent filesystem access. Cancellation/error/parent-child IDs come from verified host hooks; if child provenance is unavailable, report missing capture, not parse guessed IDs. Display settings affect rendering only.

Statusline (user decision D9.d): `statusline.mode = segments|footer`, default `segments`.
- `segments`: Workbench publishes short status entries through the host (`ctx.SetStatus`): location, running jobs, pending ask, context warning, live peers (SPEC-0011). At most a short ambient status; multi-line widgets are opt-in and cleared after completion.
- `footer`: Workbench composes the whole footer with the donor layout engine (lines, items, priority, minimum columns, merge, right alignment, conditions, wrapping, hit regions) and builtin segments (usage, cost, agent, model, path with branch in the `statusline.path-style` style (SPEC-0003 §10), remote identity). Only an explicit user choice enables it; installing an extension never does.
- **Activity line**: one row above the editor, on by default (`activity.enabled`), drawn as a kit view widget (`SetWidget` with `placement: aboveEditor`).
  - Content, by priority: state (`Ready`, `Working <elapsed>`, `Waiting for you`, `Compacting`, `Retrying n/m`) with host-animated dots (`kit.Loader` frames); live agents and unread messages (SPEC-0011); running and failed jobs (SPEC-0008); active and changed instruction files (SPEC-0007); enabled skills; location only when away from the starting checkout (SPEC-0003 §10); contributed items; coordinator mark.
  - Zero counts are hidden unless the user pins the item (`show: always`). Severity pairs color with a word (`1 failed`). Below 50 columns items become glyph + number (`⚇2 ⚙2✗1 ◆5`). Lowest priority drops first; no item is cut mid-word.
  - Sent on change, coalesced to ≤ 4 per second; once per second while working (elapsed time). No I/O in render.
  - Click (fullscreen mode, the target mode): an item opens its inspector (`/workbench sessions`, `jobs`, `instructions`, `ls skill://`, `/cd`, a contributed `open` target); the state opens the hub (SKETCH-0001 §3). Keyboard: shortcut `alt+w` (setting `activity.shortcut`) opens the hub with the first non-zero item selected. Headless: `/workbench show` and `cfg://workbench/status` carry the same items.
  - While the line is on, Workbench publishes no `SetStatus` segments for jobs, peers or pending ask. PiG's own working indicator stays visible; `activity.replace-working-indicator` (default false) hides it only after the retry/compaction events it shows are verified observable.
  - Host gap: the SDK delivers kit view events only to open `ctx.Custom` overlays (`view.go` `viewEvent`), so clicks on a widget `MouseRegion` are dropped today. Until the PiG change lands (PLAN-0001 M0.2), the line renders without hover affordances and the shortcut is the entry.
- Other extensions register `statusSegment` contributions `{key, label, priority, minColumns, maxColumns, align, placement?, glyph?, open?}`, where `placement` is `status` (default) or `activity`, `glyph` is one cell for narrow widths, and `open` is a command or resource URI run only on an explicit click or Enter. `status.update` also accepts `count` and `severity` for activity items, which Workbench renders as `N <label>` and hides at zero. and push values with `status.update` `{text ≤ 64 columns plain, style: normal|dim|success|warning|error}`, coalesced to 4 updates per second. In both modes the user orders and hides them in `/workbench statusline`. The receipt says `composedByWorkbench: true`, so the producer stops its own `SetStatus` to avoid duplicates; it resumes when Workbench closes.

# 10. Acceptance

- All ask types, freeform-only, multi custom answers, question/option/form notes, advisory-required omission and clarification continuation.
- Single/multi presentation changes and auto-submit eligibility; dirty back/dismiss never loses text silently.
- Pending recovery, duplicate local/remote submission, wrong generation, timeout and branch change.
- RPC equivalence and print/JSON failure without UI; optional bridge explicit bounded wait.
- /answer no questions, malformed extraction, empty concrete choices, stale source, cancel, configured retry and replay without inference.
- Checkpoint source interval, compaction, cancellation, regeneration and unknown metrics.
- Secret question in TUI and RPC: value reaches the vault, the tool result and history hold only the placeholder, `env` is usable in the next bash call.
- Every route/completion/headless alternative; top-level and family handlers agree.
- Codemode handled child failure remains visible; historical preview never executes tools.
- Width/height/focus/keybinding captures per SKETCH-0001/2 and workflow designs in SKETCH-0003.

No inference-dependent test may be reported live-verified without authorized execution. Mocks prove parsing/state only, not provider/API integration.

# 11. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), oh-my-pi `tools/ask` and pi-workbench `ask/*`, `tldr/*`. Normative unless listed as open in the audit.

**ask_user**
- **G9.1 Images in answers.** Custom answers and notes can carry images. They are returned after the text with numbered markers, renumbered per answer. An optional vision description (setting) can follow each image; a failed description keeps the answer and its images, and cancelled asks make no vision call.
- **G9.2 Answer in chat.** The user can leave the form to reply in chat. The result says so explicitly and is distinct from cancel.
- **G9.3 Input hygiene.**
  - A double-encoded `questions` string is decoded. Carriage-return runs are stripped.
  - A label that collides with a reserved UI label (Other, Done), or labels/IDs that sanitisation would merge, fail closed with the field named.
  - Blank optional presentation fields count as absent. Validation messages expand tabs and clamp long values.
- **G9.4 Pending scan.** Pick the newest unresolved `ask_user` call on the branch and ignore failed assistant responses. Prefer the persisted payload over recorded arguments. A newer invalid call does not hide an older valid one. Dismissal is persisted, but manual replay still works. Scan only on startup, resume and fork.
- **G9.5 Abort races.** An already-aborted signal resolves cancelled without mounting the form. An abort during config load never mounts. A session replacement during load records nothing in the new session. An abort after mount cancels a dirty form without confirmation. A local/remote race completes once.
- **G9.6 Answer editing.** Switching multi → single confirms before discarding extra answers and keeps custom text and notes. Notes on deselected options stay in state, but only selected options' notes are submitted. An empty custom draft clears the answer and keeps its notes.
- **G9.9** Untrusted project notification commands are ignored. Trust is refreshed on each call.

**/answer**
- **G9.7 Extraction.** Tool-call arguments win over text; multiple tool calls are rejected; fenced JSON is a fallback. Option spam is flagged, options are capped, and generic conversational prompts are dropped. The extraction model is the first configured model with auth inside the session's model scope; a fallback outside that scope is rejected.
- **G9.8 History store** (when enabled). Per project and session under XDG data, as append-only whole records. The file mode is tightened. Errors are value-free. Disabled means nothing is written. A failed history write never loses the answer.

**/tldr**
- **G9.10 Evidence filter.** Context, instruction and alias frames are excluded even when explicitly included; job event frames are kept; hidden or empty extension messages are dropped. Unfinished calls cross checkpoints as background without repeating work. Late corrections to already-summarised messages are kept. Folded edits apply locally and never to a shared ancestor.
- **G9.11 Segmentation.** Blocks start at each user request and are packed without splitting. An oversize block is split into continued pieces with every character kept. Suggested length grows with the amount of work and has no fixed ceiling.
- **G9.12 Run control.** The first failing part aborts its siblings, and a stalled part is stopped by its timeout. Compaction during generation discards the card and keeps the checkpoint. A late result is discarded after a context edit, fork or session switch. A malformed secret inventory stops the run before any request.
- **D9.b (user decision):** cancelling a form returns `cancelled: true` as a normal result and does not abort the agent turn.
