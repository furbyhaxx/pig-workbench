---
document:
  type: "analysis"
  id: "ANA-2026-11-04-pig-upstream-candidates"
  title: "PiG host bugs, gaps and seams found while building Workbench"
  description: "Consolidated list of every PiG bug, missing feature, API seam and developer-experience problem found in the pig-workbench sessions and .project artifacts, with impact, workaround and evidence, as input for the personal PiG fork or upstream"
  status: "published"
  revision: 1
  created_at: "2026-11-04T11:55:00+01:00"
  updated_at: "2026-11-04T11:55:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User asked for a report of every bug, missing feature, seam or change for upstream PiG found in the pi/pig sessions of this project"
      changes: ["Consolidated host probes P1-P30, S1-S6, PR1-PR8, donor audit, specs, handoffs, implementation and subagent sessions"]
---

# 1. Scope and method

A consolidation, not a new probe run. Sources:

- Analyses: [host integration probes](../2026-11-02-host-integration-probes/README.md) (P1–P30), [secrets probes](../2026-11-03-secrets-host-probes/README.md) (S1–S6), [system prompt probes](../2026-11-03-system-prompt-host-probes/README.md) (PR1–PR8), [session bucket symlink probe](../2026-11-03-session-bucket-symlink-probe/README.md), [donor parity audit](../2026-11-02-donor-parity-audit/README.md) §4–5.
- Specs and plans that record host limits: SPEC-0002/0003/0009/0011/0012/0013/0014, PLAN-0001 D3/D12/D13, PLAN-0004 C4; pig-web's M0.4 input M4.
- Sessions (one bucket, shared by `~/.pi` and `~/.pig`): design/probe session of 02.–03.11., research import, plan review, Slice 1 implementation, 81 subagent transcripts under `.workbench/agents/`, and 19 stray Pi-Bolt sessions. Excerpts and file names: [`evidence/sources.md`](evidence/sources.md).

Host for all probes: `pig 0.5.3+1.4.0`, staged SDK `sdk-899c7964784a0050`. The installed build on 04.11.2026 (`0.5.3+1.4.0.74f46864`) stages the **same SDK hash**, and three API facts were rechecked against it today (no masked input, `viewEvent` overlay-only, dialog methods return `any`). Behavioural rows were not re-run.

PLAN-0001 D13 (user, 03.11.2026): host changes go to a personal PiG fork first; nothing here has been filed upstream. External filing still needs a go-ahead.

# 2. Summary

| ID | Kind | Finding | Impact | Workbench workaround | Origin |
|---|---|---|---|---|---|
| U-01 | Bug | Packed Go cell `/reload` can hang forever in `awaitConnection` | High | None | P13, P29 |
| U-02 | Bug | `before_provider_request` error/panic sends the **original** request (fail-open) | High (security) | `ctx.Abort()` + error | S2, H-S2 |
| U-03 | Gap | No print/JSON output channel for extension commands and flags | High | Read state via URIs; `--list-tools` blocked | P27 |
| U-04 | Gap | No request hook for compaction, branch summaries, extension inference | High (security) | Entry-time redaction; opt-in `secrets.redact` | S2, H-S1 |
| U-05 | Bug | Built-in MCP `before_agent_start` runs after `-e` extensions; under `force` `<mcp_servers>` is lost | High | Payload splice in `before_provider_request` | PR8, F-P4 |
| U-06 | Bug | Transcript does not match what was sent (forced prompt not recorded; later sections recorded but never sent) | Medium | Own record at `session://<id>/system-prompt` | PR1, PR2, F-P3 |
| U-07 | Gap | Sections cannot replace preamble, delete or reorder sections | Medium | `force` delivery | F-P1 |
| U-08 | Bug | Queued `nextTurn` message lands on another branch after tree navigation | Medium | Avoid `nextTurn` | P20 |
| U-09 | Bug | Codemode drops `tool_call` block reasons for children (`reason: {}`) | Medium | Record own reason by call ID | P7 |
| U-10 | Gap | No renderer hook for codemode child rows | Medium | Take over `codemode` rendering | P9 |
| U-11 | Gap | Widget/header/footer kit views receive no view events (overlay only) | Medium | Keyboard only | activity-line §7 |
| U-12 | Gap | No masked/secret input dialog | Medium | Plain `Input` | ask subagent, `internal/host/ask.go` |
| U-13 | Seam | Dialog `ok=false` means cancel, timeout or no UI; RPC dialogs block forever without timeout | Medium | Own timer, `HasUI` check | P28 |
| U-14 | Gap | No process/session retention across extension reload | Medium | Jobs terminated on reload | D8.a |
| U-15 | Seam | No handler ordering/priority between extensions | Medium | Cooperating producers redact themselves | PR2, M5 |
| U-16 | Bug | `!command` keys: `RegisterNativeProvider` `Resolve` and `GetApiKeyForProvider` return the raw string (pig-web) | Medium | — | pig-web probe row 9 |
| U-17 | Bug | `session_start.reason` is `startup` for `--continue` | Low | Infer from entries | P22 |
| U-18 | Bug | Invalid `deliverAs` accepted silently | Low | Validate client-side | P19 |
| U-19 | Bug | Builtin tool factory `cwd` silently ignored | Low | Resolve paths; `SpawnHook.Cwd` | P18 |
| U-20 | Bug | Historical tool rows re-render with empty args after reload (seen once) | Low | Render from Details | P11 |
| U-21 | Bug | `ListAll` lists a session once per symlinked bucket | Low | Dedup by session ID | symlink probe |
| U-22 | Seam | Renderer claims cached per tool until `/reload` | Low | "applies after reload" notice | P10 |
| U-23 | Seam | Bus listener keeps running after caller abort | Low | Explicit `cancel` message | P3 |
| U-24 | Seam | `PIG_HOME` does not isolate `~/.agents/skills` | Low | — | PR5 |
| U-25 | DX | Packed Go build packs the whole module, races concurrent writers, has no build tags | Medium (DX) | Quiet tree; ship all grammars | Slice 1 session, backend eval |
| U-26 | DX | SDK only resolvable through staged copy + `replace` | Low (DX) | `just _modfile` | Justfile |
| U-27…U-35 | Seam/doc | Smaller ergonomics and doc items | Low | — | §6 |

# 3. Bugs

**U-01 Reload hang (P13, P29).** 1/28 and 2/40 reloads hung; 3/8 runs in an early series when reload followed tree navigation and an agent run. Stack: `Host.Reload → stagePackedGo → startGoPackedCellOnce → acceptPackedExt → awaitConnection`, no deadline. The re-dial gets `connection refused` on the old socket path; the new instance's `events.on` fails; the old instance keeps serving. Fix: deadline plus re-dial against the new socket, and surface the failure. Evidence: `../2026-11-02-host-integration-probes/evidence/runs/reload-hunt/`, `reload-loop.txt`.

**U-02 Fail-open provider hook (S2, H-S2).** A handler returning an error or panicking does not withhold the request; the unredacted original goes out. Only `ctx.Abort()` withholds it. Fix: error/panic withholds, or an explicit `{block: true, reason}` result. Evidence: `../2026-11-03-secrets-host-probes/evidence/runs/block*.txt`.

**U-05 Late MCP section (PR8).** PiG's built-in MCP extension (`coding/mcpext/extension.go`) sets `mcp_servers` in its own `before_agent_start`, which runs after every `-e` extension. Any extension that forces the prompt silently drops the MCP server list from every request; the section exists only in the recorded system message. Fix: F-P4, a late phase (or ordering hint) that sees all handlers' sections, or run built-in section producers first. Evidence: `../2026-11-03-system-prompt-host-probes/evidence/runs/pr8-*.txt`.

**U-06 Transcript truth (PR1, PR2, F-P3).** Under `force` the session records PiG's base sections, not the sent text; compaction entries also store the base prompt. Sections set by a handler after a force are recorded but never sent, so the transcript claims content the model never saw. Fix: record the forced prompt (as sections or a marked system message). Evidence: `runs/force-steer.txt`, `ab-force-sections.txt`.

**U-08 `nextTurn` crosses branches (P20).** A message queued before `NavigateTree` is delivered on the new branch; an idle `steer` stays on the old one. Fix: drop or rebind queued messages on navigation. Evidence: `runs/stale`.

**U-09 Codemode block reason lost (P7).** A child blocked via `tool_call` produces no `tool_result` event; `tool_execution_end` fires; the script sees `{"status":"rejected","reason":{}}`. Evidence: `runs/codemode`.

**U-16 `!command` expansion depends on registration kind.** Workbench S6: `GetProviderAuth` expands `!command` for `RegisterProvider(ProviderConfig)` and models.json providers. pig-web: with `RegisterNativeProvider` + custom `Resolve`, `Resolve` receives the raw `!printf …` and `GetApiKeyForProvider` returns it unexpanded. Not reproduced here. Evidence: `host:/mnt/Projects/furbyhaxx/pig-web/.project/analyses/2026-11-02-auth-json-probe/README.md` row 9, [handoff M4](../../handoff/pig-web-m0-4-inputs.md).

**U-17 to U-21.**
- U-17: `--continue` reports `session_start.reason = "startup"`, not `resume` (P22, `runs/resume`).
- U-18: unknown `deliverAs` values append a persisted custom message instead of erroring (P19, `runs/idle-*`).
- U-19: the `cwd` argument to builtin tool factories is ignored whenever a context exists; tools use the session cwd (P18, `runs/override-plain`). Either honour it or drop the parameter.
- U-20: after `/reload`, a historical call row was re-rendered with `map[]` args while the live row had them (P11, one run, `runs/claim-reload`).
- U-21: `SessionManagerListAll` and the `/resume` all-projects view list a session once per symlinked bucket; dedupe by real path (`../2026-11-03-session-bucket-symlink-probe/evidence/catalog.json`).

# 4. Missing features and hooks

**U-03 Print/JSON output for extension commands and flags (P27).** `pig -p "/tools"` and `--list-tools` run extension code but print nothing; `Notify` is dropped and extension stdout/stderr are not forwarded. Ask: print-mode `Notify` to stderr or an explicit `ctx.Print`. Only feature fully blocked on the host (`--list-tools`).

**U-04 Request hook coverage (S2, H-S1).** `before_provider_request`, `context` and `context_with_system` fire only for agent turns, not for compaction, branch summaries or `ModelRegistry.Complete/Stream` and `ModelRuntime`. Blocks a host-wide secrets boundary and pig-web's inference (handoff M1).

**U-07 Full section control (F-P1).** `before_agent_start` cannot replace the preamble (except via `customPrompt`, which also drops tools/rules/docs), delete or reorder sections; new sections append after `<cwd>`; empty values mean "unchanged". Ask: accept a complete ordered section list and record it, with a capability flag. Needed for `sections` delivery and native mid-conversation patches.

**U-10 Codemode child renderer hook (P9).** Codemode draws child rows itself; extension resolvers are not consulted.

**U-11 View events for widgets (activity-line §7).** `viewEvent` routes `ui.view.event` only to overlays (`$SDK/view.go:311`); widget, header and footer `MouseRegion`s are dead. Ask: e.g. `ext.OnWidgetEvent(key, func(kit.Event))`. Blocks the clickable activity line in fullscreen.

**U-12 Masked input.** The SDK has `Input`, `Select`, `Confirm` but no password/secret dialog; `ask_user` secret questions use plain `Input` (`internal/host/ask.go:71`).

**U-14 Session retention (D8.a).** No `sessionLifetime.retain` equivalent: background jobs die on extension reload.

**U-15 Handler ordering.** Extensions run in `-e` load order and cannot request a position (PR2). Consequences: the last force wins, a later `tool_result` annotator may run after a redactor (pig-web M5, SPEC-0013 §4). Ask: priority or before/after hints per hook.

Nice to have, lower priority:
- Live-session registry and cross-process bus (SPEC-0011 §1); the event bus is per session, so Workbench ships its own socket transport.
- Let an extension answer host dialogs and other extensions' `ctx.UI` prompts for remote control (SPEC-0011 §9).
- Readable partial tool-call arguments plus clean mid-stream abort for streaming edit previews (G5.10, unprobed) and speculative reads (D6.e).
- Provider-receipt acknowledgement event; today delivery can only be inferred from the `context` hook (P5).
- Session title change event: open probes T1–T3 in SPEC-0012 §8.

# 5. Seams and API ergonomics

- **U-13 Dialog results (P28).** RPC dialogs block forever unless `DialogOptions.Timeout` is set. Print/JSON return `ok=false` immediately, timeout returns `ok=false`, cancel returns `ok=false`. Ask: a typed reason (`cancelled|timeout|unavailable`). `Select`/`Confirm`/`Input` also return `any` (`$SDK/context.go:478,543,563`).
- **U-22 Renderer cache (P10).** The resolver result is cached per tool until `/reload`; late registrations cannot take over.
- **U-23 Bus cancellation (P3).** Caller abort cancels its `Emit` but the listener runs on; no request-scoped cancel.
- **U-24 Isolation.** With an isolated `PIG_HOME`, skills from `~/.agents/skills/` still load, and `PIG_HOME/skills/` is not a skill root (`agent/skills/` is). Tests cannot fully isolate.
- `GetBranch` projection omits custom entry data; restoring state needs `GetEntries` and a manual parent-chain walk (`internal/host/AGENTS.md`).
- Context-file suppression works only by editing `systemPromptOptions.contextFiles` in every `before_agent_start` (P23); returning a `systemPrompt` string is the wrong lever (the value is `*sdk.SystemPrompt`). A declarative suppression API would help.
- `SessionManagerList/ListAll` always include `allMessagesText` (full transcripts, potentially megabytes); a lazy or paged option would help (P24).
- `Nothing to compact` is reported only via `OnError`/`compaction_end` (P21).

# 6. Behaviour to document or confirm as intended

- `--append-system-prompt` replaces `APPEND_SYSTEM.md` instead of adding to it (PR5).
- `--tools` hides MCP and codemode tools; `-ne` disables both built-ins (secrets analysis §2).
- The rendered system message, including `project_context`, is stored in the session file and compaction entries.
- Anthropic OAuth requests get a fixed first system block from provider code, visible only in `before_provider_request`.
- `nextTurn` is ordered after the user prompt, not before it; summary requests bypass the `context` hook (P5, P21).

# 7. Developer experience

- **U-25 Packed builds.** PiG packs the entire module, so a file written mid-build fails with "the inputs of cell … changed while it was being built" (hit repeatedly with parallel subagents; now a rule in `AGENTS.md`). `.project/` evidence is packed too. There is no documented build-tag option, so feature subsets (e.g. a curated tree-sitter grammar set) are unreachable. Ask: snapshot inputs before building, honour `.gitignore` or a pack-ignore file, and expose build tags.
- **U-26 SDK resolution.** `go build`/`go test` work only with a generated modfile that `replace`s the SDK with `pig reload --sdk-path`. A versioned, importable SDK module would remove the `.tmp/go.mod` step.

# 8. Findings outside PiG

| Finding | Owner | Evidence |
|---|---|---|
| 19 stray sessions (03.11.2026, 21:59–22:49 local) whose prompt is `…/pi-accordion/conductors/ws/thermocline/runner.mjs`, each a full turn on `gpt-6.1-terra` at `xhigh`. pi-accordion spawns `spawn(process.execPath, [runnerPath])` (`extension/accordion.ts:761`); in Pi-Bolt `process.execPath` is the harness binary, so each subagent start launched a new agent session. Under PiG, extensions run in `/usr/bin/node`, so the runner starts correctly | pi-accordion / Pi-Bolt | `evidence/sources.md` §3 |
| Subagent children reported the `extending-pig` skill as unavailable | pi-workbench agents / Pi-Bolt skill roots | `evidence/sources.md` §4 |
| Subagents read `host:/…` paths in AGENTS.md as an SSH alias and could not reach donor trees | this project's path convention | `evidence/sources.md` §4 |
| Shared git index between orchestrator and subagents swept files into one commit | process; fixed by `git commit -- <paths>` rule | Slice 1 session, commit `e3a364e` |

# 9. Suggested batches for the fork or upstream

1. **Reliability and safety:** U-01, U-02, U-08, U-09, U-18.
2. **Prompt ownership:** U-05 (F-P4), U-06 (F-P3), U-07 (F-P1), U-15.
3. **Headless and UI surfaces:** U-03, U-11, U-12, U-13, U-10.
4. **Secrets coverage:** U-04 (H-S1), U-16.
5. **Lifecycle and catalogs:** U-14, U-17, U-19, U-21, U-22, U-24.
6. **Build DX:** U-25, U-26.

Before filing any of them, re-run the matching probe on the target PiG version; probe harnesses are runnable under each analysis's `evidence/`.
