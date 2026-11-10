---
document:
  type: "plan"
  id: "PLAN-0001"
  title: "Implement the full pig-workbench extension"
  description: "Scope-traced delivery plan for Go tools, full hashline editing, advanced reads, resource schemes, context injection and Workbench UX"
  status: "draft"
  revision: 25
  created_at: "2026-11-02T15:26:05+01:00"
  updated_at: "2026-11-03T15:57:29+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 25
      reason: "Record explicit implementation authorization and standing dependency approval"
      changes: ["Slice 1 runtime/configuration work is underway, not complete", "Runtime dependencies may be added after latest-version and maintenance/supersession checks, without another approval question"]
---

# 1. Outcome and authority

Deliver `pig-workbench` as a Go factory extension that makes PiG's tools substantially more capable while providing consistent configuration, rendering, working locations, frames, questionnaires and background jobs.

**The user has explicitly authorized Slice 1 implementation. This plan defines scope, not evidence of completion.** The user corrections are requirements: full hashline, advanced read behavior including suffixes, session/history/skills/jobs schemes, codemode integration, enhanced instruction injection, `/workbench` and companion commands such as `/tools`. Dependency and host limitations must be resolved, not used to quietly remove features.

Scope reference: `.project/drafts/pig-workbench-proposal.md`, revision 2. That document remains an unapproved proposal; its implementation claims are not established facts. This plan qualifies its unsafe or unsupported assumptions in section 3 rather than silently treating them as decisions. Do not overwrite its editor swap file or unrelated reference-repository work.

UI/UX companion: `.project/sketches/workbench-ui/README.md` (SKETCH-0001). It covers the hub, settings/trust, tools, transcript and codemode, location/remote, jobs, frames/instructions, questionnaires, checkpoints, schemes and status composition, with responsive wireframes and mode equivalents. Static wireframes are not live verification.

First API specification batch (drafts; runtime probes pending):
- `.project/specs/0001-extension-services.md` — optional discovery, versioning, registrations and JSON event protocol.
- `.project/specs/0002-message-frames.md` — external producers, delivery receipts, trust and replay.
- `.project/specs/0003-tool-presentation.md` — shared renderers, lifecycle states and contributed display settings.
- `.project/specs/0004-resource-providers.md` — pluggable scheme handlers, policy, paging and snapshot integrity.
- `.project/sketches/extension-apis/README.md` — API flows, frame styles, state gallery and integration/provider/settings wireframes.

Remaining feature specifications (drafts):
- `.project/specs/0005-hashline-editing.md` — WB-02/03: full grammar, snapshots, seen guards, syntax blocks, registers, recovery and commits.
- `.project/specs/0006-advanced-read.md` — WB-02/04/05: selectors, code summaries, adapters, search and paging.
- `.project/specs/0007-ambient-context.md` — WB-11/13/14/18: class selection, pins, capped views, watchers, diff/context epochs.
- `.project/specs/0008-locations-jobs.md` — WB-09/12: cwd/SSH/worktrees, process lifecycle and job controls.
- `.project/specs/0009-interaction-commands.md` — WB-15/16/17: ask/answer/replay, checkpoints, codemode and complete command UX.
- `.project/specs/0010-configuration-state-trust.md` — WB-02/19 and all domains: settings, durable state, trust and privacy.
- `.project/sketches/workflow-details/README.md` — recovery, adapter, class/pin, worktree/job, notes/clarification, checkpoint and config-conflict wireframes.

Together these ten specifications and three design artifacts cover the planned feature areas as reviewable drafts. They do not close M0 until host probes and contract review pass. `/workbench integrations` and `/workbench resources` are proposed inspection additions; include them in the command inventory review.

## Progress

- **done:** Source reconnaissance, proposal revisions, minimal loadable Go factory bootstrap, ten draft specifications and three static design/wireframe artifacts. No service/tool implementation yet.
- **done (M0.2):** All probe rows except kit views (deferred to M0.5/M1 fixture prototypes) — [host integration probes](../analyses/2026-11-02-host-integration-probes/README.md) P1–P30. Findings written into SPEC-0001/0002/0003/0005–0010. Eight upstream issues listed; `--list-tools` output is the only host-blocked feature.
- **done (M0.2, secrets):** [Secrets probes](../analyses/2026-11-03-secrets-host-probes/README.md) S1–S6: the SPEC-0013 boundary is hostable (entry-time redaction, `context_with_system`, `ctx.Abort()` to withhold). Two upstream requests H-S1/H-S2 join the queue.
- **done (M0.3):** Backend matrix approved as proposed, including HEIC — [backend evaluation](../analyses/2026-11-02-backend-evaluation/README.md). Open coverage checks: cpio and the remaining archive formats, grammar subset/size budget, HTML conversion quality, macOS/Windows.
- **now:** [PLAN-0002](0002-slice-1-foundation.md): local runtime and scalar configuration foundations implemented and verified; remaining phases and Phase A formal freeze/ledger/handoff closeout are still open. Personal PiG fork (D13) in parallel.
- **next:** PLAN-0002 Phases B–L, then [PLAN-0003](0003-slice-2-feature-complete.md) and [PLAN-0004](0004-slice-3-completion.md).
- **done (M0.2, prompts):** [System prompt probes](../analyses/2026-11-03-system-prompt-host-probes/README.md) PR1–PR8: `force` delivery and `beforePrompt` frames work in RPC, print and TUI; gaps are later handlers and transcript truth (fork candidate F-P3); mid-run tool changes become `tools` frames (SPEC-0014 §4.3). PR7: mid-run `tools` frames work; PR8: late sections (`mcp_servers`) need the `before_provider_request` splice (fork candidate F-P4). SPEC-0014 decisions Q1–Q6 accepted.
- **later:** Automatic session title generation (agents already read and set titles through `session://<id>/title`, SPEC-0012 §5.3), in PLAN-0004 C5.

## Delivery slices

The milestones in section 5 define scope and exits; three slice plans own the order and the detail; §9 assigns every specification section to a slice and phase. No milestone requirement is dropped by the split.

| Slice | Plan | Contains | Exit |
|---|---|---|---|
| 1 Foundation | [PLAN-0002](0002-slice-1-foundation.md) | M0.4, M1 and the foundation of M2–M7: every tool, extension seam and abstraction, the URI system with core schemes, the system prompt, fully working on final implementations; no stubs | pig-web co-load gate and a live agent run |
| 2 Depth | [PLAN-0003](0003-slice-2-feature-complete.md) | Depth of M2–M7 on the same engines (adapters, remaining schemes, SSH, worktrees, PTY, parity heuristics, secondary flows), plain UI | Every scope row usable by agents in TUI, RPC and print |
| 3 Completion | [PLAN-0004](0004-slice-3-completion.md) | M0.5, UI part of M7, fork-dependent features, enhancements, performance, platforms, M8 | Release gate |

A milestone can be implemented incrementally. Shipping the complete extension requires all scope rows to pass; an early milestone is not a reduced-scope release.

# 2. Scope and traceability

| ID | Required outcome | Delivery | Acceptance anchor |
|---|---|---|---|
| WB-01 | Go factory identity `pig-workbench`; primary command `/workbench`; builtin override ownership | M0–M1 | Registration exposes the intended names with no recursive dispatch or competing override |
| WB-02 | Enhanced read, write, edit, grep, find, ls, bash; layered config and compact/normal/full renderers | M1–M5 | Same behavior in direct and nested calls; structured facts and bounded honest output |
| WB-03 | Full hashline grammar, syntax blocks, registers, move/delete, seen-state, recovery, previews and diagnostics | M2 | Donor fixture behavior and integrated read→edit tests; no lexical approximation presented as AST parity |
| WB-04 | Inline line/range/count/head/tail/raw selectors, multi-read, context boundaries, summaries, continuation, loop guard, supersede handling | M1–M3 | Source-coordinate/seen-line integrity, filename disambiguation and deterministic bounded batches |
| WB-05 | Advanced read adapters: JSON queries, SQLite, archives, documents/PDF, images/SVG, notebooks, static web, binary inspection, video/profile views | M3 | Explicit format/operation matrix and representative end-to-end fixtures, not just adapter registration |
| WB-06 | Shared capability-declared scheme router and extension integration contract | M1, M4 | One resolver/pager/policy path; handlers do not duplicate tool logic |
| WB-07 | Sessions and agents as trees: `session://` (with `by-id/`, `by-project/`, `by-name/`), `agent://`, `project://` (SPEC-0012 §5.3) | M4 | Transcript/tree/entries projections, project membership by project ID incl. nested worktrees, links not walked twice |
| WB-08 | `skill://`, supporting files and enabled skill catalog | M4 | Trust-resolved catalog, safe containment, no disabled-skill rediscovery |
| WB-09 | `job://` with logs, `stdin` and `ctl` backed by the same manager as one `jobs` tool | M5 | Tool and URI operations have identical ownership, cancellation and policy semantics |
| WB-10 | `file://`, `ssh://`, `artifact://`, `out://`, `local://`, `conflict://`, `cfg://`, `docs://`; no aliases, retired names answer with the replacement | M1–M5 | Capability-driven routing, mutation guards, scoped persistence and useful errors |
| WB-11 | Frames, event bus, session replay and native role projection where supported | M1, M6 | Attributed delivery, no duplicate frames, no fabricated provider support |
| WB-12 | Local/SSH cwd, explicit paths, `cd`, worktrees, `/remote` host management | M1, M5 | Captured location revisions, no host/process chdir, safe SSH and worktree transitions |
| WB-13 | Full instruction lifecycle plus configurable additional file classes, root+nearest, ancestors, pinning and diff updates | M6 | Multi-directory, concurrent-touch, resume/compaction and watcher fixtures |
| WB-14 | BACKLOG.md ancestor injection, item caps and exact overflow pointers | M6 | Files remain unmodified; capped-view diffs and hidden-change accounting are correct |
| WB-15 | `ask_user`, pending recovery, remote bridge, `/answer`, replay and settings | M7 | TUI/RPC interactions, duplicate/late submissions and headless cancellation |
| WB-16 | Codemode nested captures, call trees, output previews, config and replay | M1, M7 | Live nested execution plus historical rendering; failures/cancellation stay visible |
| WB-17 | `/tools`, `--list-tools`, previews, settings, jobs/frames/instructions/trust commands and `/tldr` | M1, M7 | Command inventory, completions and usable non-TUI behavior |
| WB-18 | Extensibility for external project notifications without implementing task tracking | M6 | Validated project/path-scoped frame inputs; explicit transport boundary |
| WB-20 | Live session discovery across projects, messaging through `agent://` that can wake idle sessions, broadcast and the opt-in coordinator (SPEC-0011); collaboration services for other extensions: location, secret sources, status segments, edit tracking, fetch backends, docs (SPEC-0001 §8) | M5 (endpoint, `agent://`), M7 (panel, coordinator), services with their owning milestone | Two-session wake/steer/loop-guard tests; coordinator class and safeguard tests; pig-web integration against the services |
| WB-22 | Projects (SPEC-0012): `Project.toml` identity, registry, symlinked session buckets, components, `.project/` and `AGENTS.md` conventions, shared artifact tree (`tree`/`branch`), worktree links, `docs://` with PiG source, first-touch scheme guides | M4 (identity, trees, docs), M5 (storage, worktrees), M6 (conventions, guides) | SPEC-0012 §6 |
| WB-23 | Activity line above the editor with click-to-inspect and contributed items (SPEC-0009 §9) | M1 (line, shortcut), M7 (contributions, click once the host delivers widget events) | Captures at 36/57/80/140 in fullscreen; counts match registries; click and shortcut open the same targets |
| WB-24 | Secrets (SPEC-0013): `red-sec-` placeholders at every provider-bound boundary, substitution before every tool call, encrypted vault, env injection, `secret://`, `ask_user` type `secret`, `/workbench secrets` | M0 (probes S1–S6), M1 (detection, vault, boundary hooks, substitution, prompt section), M4 (`secret://`), M5 (env injection, G8.20 scheme commands), M7 (`ask_user` secret, panel) | SPEC-0013 §11; captured provider payloads contain no known secret |
| WB-25 | System prompt templates (SPEC-0014): gonja engine, `builtin/workbench` default keeping PiG's sections with adapted content and a structure check, model families and conditionals, context frames, progressive skill disclosure, `select = "all"` instruction classes | Slice 1 (engine, `builtin/workbench`, frames, skills), Slice 2 (`select`), Slice 3 (family adapters, fork `sections` delivery); see §9 | SPEC-0014 §12 |
| WB-21 | Structural search/rewrite tools (`ast_grep`/`ast_edit`) on the approved tree-sitter backend (user decision D6.b) | After v1 | Not yet designed |
| WB-19 | Performance, security, packaging, documentation and live verification | Every milestone, M8 | Recorded acceptance evidence and measured budgets |

Explicit exclusions remain subagents, agent definitions and task/todo tracking. Reading BACKLOG.md and accepting external claim/completion notifications does not create a task manager. Do not import unrelated oh-my-pi subsystems merely because they have a URI scheme.

## Command contract inventory

Keep the extension identity separate from command spelling: **`/workbench`, never `/pig-workbench`.**

- `/workbench`: settings, show, preview, tools, jobs, frames, instructions, trust, tldr, statusline.
- `/tools`, `/remote`, `/cd`, `/wb:cd`, `/wb:jobs`.
- `/answer`, `/answer:again`, `/ask:replay`, `/ask-settings`, `/tldr`.
- `--list-tools` with documented machine-readable output.

Inventory dynamically registered donor commands in M0 as well as literal `registerCommand` calls. Record any additional relevant commands before freezing the command table. Top-level entrypoints and family subcommands call the same handlers, share completions and obey the same authorization. No separate `jobs_*` tools.

# 3. Decisions and corrections needed before implementation

These are engineering decisions, not scope cuts. M0 must produce evidence and a recommendation for each; dependency additions have standing user approval (03.11.2026) after checking the latest available version and whether the project is maintained or superseded; record licenses and verification. **Resolved 2026-11-02:** D1, D2 and D7 by the approved [backend evaluation](../analyses/2026-11-02-backend-evaluation/README.md); D3 by the [host probes](../analyses/2026-11-02-host-integration-probes/README.md) (overrides, suppression, projection and codemode observation verified; codemode child rendering and `--list-tools` output remain upstream issues).

| Decision | Why it matters | Planned resolution |
|---|---|---|
| D1: syntax engine | The proposal's lexical block resolver cannot promise full tree-sitter semantics across languages | Evaluate Go tree-sitter bindings/grammar packaging against donor block and summary fixtures; recommend a maintained parser backend. Keep the extension Go. Any native/helper dependency needs explicit approval and packability checks |
| D2: advanced reader backends | Standard library alone does not implement SQLite, jq, PDF conversion, SVG rasterization, video or pseudocode | Produce backend/license/platform matrix, preferring existing host capabilities and maintained libraries. Specify required bundled vs external backends and unavailable-backend errors. No automatic installation or network grammar download |
| D3: host interception | Same-name overrides, instruction suppression, provider context projection and codemode observation are host-sensitive | Run isolated probes before relying on them. A host gap becomes a named blocker with a proposed fork change (D13), not an omitted feature or invented SDK API |
| D4: full hashline recovery | Parser tolerance, evidence-based relocation and model-assisted repair are different behaviors | Inventory all donor recovery paths and fixtures; preserve full functionality with explicit warnings/policy. Model-assisted repair must be opt-in, bounded and disclose inference. Reject ambiguous mutations |
| D5: short snapshot tags | Four-hex normalized tags are not unique integrity identifiers | Keep the user-facing grammar, but bind tags to canonical paths, retained bytes and seen spans; detect collisions and raw-byte changes before mutation. Do not mark unseen bytes seen after a partial edit |
| D6: session addressing | Agents need the current session, any session and project sessions | **Resolved:** `session://<id>` canonical, `self`, `by-id/`, `by-project/<slug>/`, `by-name/` link directories; nested worktrees belong to the project; no consent gates (SPEC-0012 §5.3). Use host session listing, not hardcoded paths |
| D7: terminal job input | Sending a control byte through a pipe is not a terminal signal/EOF | Distinguish stdin bytes, close, signals and PTY input. Preserve key controls with defined backend semantics and a tested PTY backend where needed; unsupported terminal operations fail explicitly |
| D8: instruction context semantics | Parallel touches can select several nearest files; capped views complicate diffs | Specify selection sets, context epochs, pin scope and delivered-view baselines in M0; implement the concrete rules in section 6 |
| D9: UI ownership | A general extension must not silently replace the host footer/editor | Provide statusline segment contribution and editor UI through host-supported composition. Full footer ownership, if needed, requires an explicitly selected owner/Piglet arrangement, not competing setters |
| D11: secrets | Key sharing with nukii, pi-secret-guard import, cipher, config file format | **Resolved 2026-11-03** (SPEC-0013 §10): own key in the agent dir, no imports, AES-256-GCM, `Secrets.toml` |
| D12: system prompt ownership | Templates need a delivery that PiG accepts; custom sections cannot replace the preamble, reorder or delete sections | **Decided 2026-11-03 (user):** SPEC-0014 §13 Q1–Q6 as recommended; delivery `auto` (`force` on stock PiG, probed PR1–PR6; `sections` on the fork for models with mid-conversation system messages); mid-epoch changes as frames |
| D13: host changes | Probes found host gaps (P-series, S-series, F-P1) | **Decided 2026-11-03 (user):** fixed in a personal PiG fork, not filed upstream. The "upstream" lists in the probe analyses are fork candidates. Workbench keeps working on stock PiG and detects fork capabilities by flag |
| D10: bus and project fan-out | PiG's event bus is JSON and session-scoped, not a cross-process project broadcast service. **Resolved for live sessions:** Workbench's own per-user local socket endpoints (SPEC-0011); project notifications from non-PiG tools still need their own ingress | Define versioned serializable services and external bridge ingress. Identify an existing project notification transport or propose a small approved transport; do not claim a session bus alone reaches every agent |

Further corrections carried into implementation contracts:

- Skill reads obey hard result bounds. Preserve complete instructions through paging/continuation; do not use `unbounded: true` to violate output safety.
- `cfg://`, reload, pin and job cancellation writes obey the same policy through tools and URIs. A `cancels` annotation is not permission to bypass host safeguards.
- A frame nonce is attribution metadata, not protection against prompt injection. Raw tool/file content never becomes trusted merely by wrapping it.
- Historical session JSONL remains immutable. Superseding reads is a context projection decision, not deletion of history; only same-resource, same-revision complete coverage can supersede earlier coverage.
- Cannot reliably infer every file changed by arbitrary bash. Revalidate content identities on read/edit and reconcile watched instruction candidates; do not depend on shell-command guessing.
- Local/remote crash cleanup guarantees require proof, including disconnects and PID reuse. Persistent PID numbers are not recovery handles.
- Atomic single-file replacement is not an all-files transaction or compare-and-swap against non-cooperating external writers. Multi-file edit results must report exact commit outcomes and recovery options.

# 4. Architecture and dependency order

```text
M0 contracts + host probes + dependency decisions
                |
M1 factory / config / resource-location core / frame-bus seam / test harness
       |                |                  |
M2 full hashline     M4 catalogs        M5 SSH + shell/jobs
       |                |                  |
M3 advanced reads ------+------------------+
                         |
M6 complete instruction/context lifecycle and external frame ingress
                         |
M7 full commands / ask / codemode UX / summaries / host UI
                         |
M8 integrated release gate
```

M4/M5 can proceed independently once M1 contracts stabilize; M3 needs the M2 syntax/seen-state interfaces, but non-code adapters can be developed in parallel. Frame/instruction interception is probed in M0 and wired in M1, not left as a late architectural surprise. Each milestone delivers its renderers, docs and narrow verification; M7 finishes shared UX rather than postponing all UI work.

Ownership boundaries:

- **Resource layer:** canonical identity, capability/policy routing, location snapshot, byte access; no TUI dependencies.
- **Text/syntax layer:** selector parsing, coordinates, syntax index, summaries, snapshots and editing; one parser service shared by read and hashline.
- **Session services:** branch ledger, jobs supervisor, catalogs, asks and event subscriptions. Each has an explicit lifetime and shutdown path.
- **Host adapter:** SDK contexts, hooks, original builtin delegation, persistence and UI; plain domain code stays host-independent.
- **Output layer:** shared budgets, structured details, artifacts, rendering and codemode child capture. Terminal sanitization does not modify stored file bytes.

Create packages when they have concrete responsibilities, not empty scaffolding for every box. Public integration contracts are versioned; internal functions use direct Go calls rather than event round trips.

# 5. Work packages and milestone exits

Sequencing and detail live in the slice plans (section 1, Delivery slices); the "next"/"later" labels below are historical.

## M0 — Close contracts and prove host seams (next; detailed)

**Inputs:** proposal revision 2, user corrections, running PiG SDK, donor source trees. **No product implementation before plan approval.**

### M0.1 — Behavior inventory and provenance

1. Read donor implementations and tests in full for each affected path; current reconnaissance sampled several large files and is not a parity audit.
2. Record source revisions plus local diff status, relevant license notices and fixture origins.
3. Enumerate hashline grammar/recovery/preview behaviors, read adapters/selectors, dynamic commands and instruction hooks into a feature matrix tied to WB IDs.
4. Mark each row as donor behavior, intentional improvement, or unresolved design—not simply “ported”.

**Status (2026-11-02):** test-level audit done: [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), 3,269 donor test titles mapped. Gaps were added to SPEC-0002, 0003, and 0005–0010 as "Donor parity" sections. Audit §4 decisions closed 2026-11-03 (D5.b adopted, D5.d rejected; the rest recorded in the specs). Open: the G5.10 streaming probe (PLAN-0003 W1). Line-by-line source reading happens per work package, when code is ported.

**Deliverable:** the ten existing `.project/specs/0001-*.md` through `0010-*.md` contracts revised with full donor/probe evidence and an approved backend/format matrix. The draft behavior/command inventories exist; source-parity audit and runtime verification remain. Do not reuse allocated specification IDs. Raw probe evidence belongs under `.project/analyses/<probe>/evidence/`.

### M0.2 — Isolated host probes

Use a temporary test agent directory and controlled fixtures; do not modify user configuration or run billed inference without permission.

| Probe | Evidence required |
|---|---|
| Override original builtin, then invoke directly and through codemode | Correct dispatch, cancellation, update callbacks, policy checks, nonrecursive delegation |
| Set Workbench location without changing host cwd | Explicit and relative local paths resolve correctly; host context cwd cannot override the captured root |
| Hook instructions/context and compact/navigate/resume | Exactly one instruction baseline; correct before-tool timing; no stale queued delivery to another branch |
| Frames and native role projection | Stored facts vs outgoing context shapes; unsupported capabilities handled explicitly |
| Nested codemode call observation | Stable parent/child identity, outputs and errors; do not assume slash-separated IDs without proof |
| Session and skill catalog access | Actual enabled/trusted resources and configured sessionDir, including in-memory session |
| Bus service plus cancellation/reload | JSON request/reply, bounded waits, unsubscribe, no mutex-held synchronous emit deadlock |
| UI and command/flag registration | `/workbench`, `/tools`, `--list-tools`, kit views, RPC dialogs and print/JSON behavior |
| System prompt takeover (SPEC-0014 §11) | PR1–PR6: forced prompt scope, handler order, interim sections recording, steer before the prompt, `builtin/pig` parity fixtures, model facts |
| Widget view events (fullscreen) | Whether the host emits `ui.view.event` for a `MouseRegion` inside an `aboveEditor` widget view; then a fork change (D13) to route widget/header/footer view events to the extension (today `viewEvent` serves overlays only) |

**Exit:** runnable probe evidence for every seam, or a documented host blocker. Isolated probes are not product completion and do not justify weakening the contract.

### M0.3 — Dependency/platform decision

Evaluate syntax grammars, hashing/diff, queries, archive codecs, documents/media, binary analysis, filesystem watching and PTY transport. For each candidate: source/license, API, packaging/fusion implications, platform coverage, failure behavior, resource limits and alternatives. Prefer reuse; do not build a toy jq, SQL engine or language parser.

**Exit:** approved dependency/backend choices and support matrix. No claim of “full read” while advertised formats lack working backends. A missing prerequisite blocks the affected acceptance row while unrelated work continues.

### M0.4 — Freeze model-facing and persisted contracts

Define tool descriptions/schemas, selector precedence, URI route grammar, errors, output envelopes, safety policies and config keys. Define persisted records, branch vs session ownership, recovery after reload, and limits. Finalize root+nearest/pinning/cap semantics and command routes. Capture any tradeoff needing user approval before writing dependent code.

**Exit:** reviewed contract specification; no unresolved decision that changes the foundation's data model or safety boundary.

### M0.5 — UI/UX design review

Review SKETCH-0001 against the command/feature inventory. Resolve navigation, density, scope visibility, destructive confirmations, selection/pin semantics, questionnaire flow and 140-column split-view preference. Record accepted direction and changes; do not silently settle layouts during implementation.

**Deliverable:** revised design artifact with user-reviewed direction and explicit fixture/capture checklist. M1 then creates kit prototypes using isolated fixture data: common shell, tool/codemode rows, settings, jobs, instructions and ask. Present live captures at 36/57/80/140 columns and short height for approval before finalizing shared UI abstractions. Prototype creation does not mark those features implemented.

## M1 — Integrated foundation (next after M0; detailed)

### M1.1 — Factory and development gate

- Scaffold using the running PiG Go factory generator in the existing repository without overwriting artifacts.
- Establish module identity, thin `Extension()` registration, lifecycle owner and shutdown joins.
- Add a Justfile for formatting, vet, tests, race tests, host validation and capture commands using the staged SDK; no unsolicited CI workflow.
- Add README, module steering and third-party notices as code is adopted.

**Acceptance:** isolated PiG load and factory validation succeed; unload/reload leaves no owned goroutines or temporary resources.

### M1.2 — Configuration and command skeleton with real behavior

- Implement layered TOML config (`Workbench.toml`, `Secrets.toml`; round-trip-safe saves), trust restrictions, source attribution, explicit save/reset, inter-session locking and atomic private writes.
- Register `/workbench show`, settings and `/tools` using actual effective state/host schemas, not placeholders. Add completions and CLI output contract.
- Build common sanitization, width fitting, path display (SPEC-0003 §10, donor tests as golden cases), metrics and preview/result types.
- Prototype the approved UI direction from M0.5 using fixture data; capture and review layouts before finalizing common panel/renderer abstractions. Carry accepted layouts forward to each feature milestone.

**Acceptance:** malformed-file refusal, unknown-key preservation, concurrent save, untrusted project and real TUI/RPC output tests.

### M1.3 — Resource, selector and location core

- Implement canonical URI/ToolPath parsing with literal-first disambiguation; protect SSH ports, URL queries/fragments, archive members and filenames containing commas/spaces/colons.
- One normalized selector representation for `:N`, `:N-`, `:A-B`, `:A+C`, `:-N`, multi-ranges and raw combinations. Head is expressible as `:1-N`; any extra head/tail spelling must be specified rather than guessed.
- Implement bounded paging, actual shown spans, continuation and selection-complete vs whole-resource-complete distinction.
- Implement file/artifact/local capabilities and session-owned storage; no recursive artifact spilling.
- Implement local location revision/lease handling, `cd` status/change/reset, persistence and conflict diagnostics. Create read-only harness diagnostics from real state.

**Acceptance:** direct and codemode calls resolve identically; multi-read finishes in input order within one budget; cancel joins workers; real colon/comma filenames remain addressable; two sessions cannot resolve each other's scratch/artifacts accidentally.

### M1.4 — Frame/bus and instruction interception seam

- Wire versioned frame records, branch generation guards, JSON services and host context hooks proven by M0.
- Minimal actual AGENTS.md baseline through the intended injection path; preserve trust and avoid duplicate host injection.
- Make tool touch metadata and codemode nested capture available to later modules.

**Acceptance:** one controlled read receives the correct baseline before relevant model continuation; branch change invalidates queued work; untrusted payload cannot request elevated trust; bus timeouts do not deadlock tools.

### M1.5 — System prompt engine

- gonja environment with the confined loader, section tag, helpers and limits (SPEC-0014 §3); `builtin/workbench` with PiG's section structure (§7), PR5 fixtures as golden cases.
- `force` delivery, structure check, render-failure fallback and epochs (§4); `/workbench prompt` and `session://self/system-prompt`.
- `session.date` and `session.location` frames through `beforePrompt` delivery (§5).

**Acceptance:** golden renders for every fixture; an unknown host section is reported; prompt bytes stable within an epoch; frames precede each user input.

**M1 exit:** integrated local vertical slice plus stable contracts. Do not label hashline, full instruction lifecycle or advanced readers complete yet.

## M2 — Full hashline engine and coordinated mutations (later)

Depends on M1 and approved syntax backend. Port grammar/tokenization, multi-section staging, all PUT/CUT/range/gap/block forms, named/anonymous registers, REM/MV, prefix handling, seen guards, mismatch reveals, evidence-based recovery, no-op guards, streaming previews and post-edit results. Preserve exact-text multi-edit as a deliberate alternate contract, not an automatic escape from hashline failures.

Share canonical mutation locks and byte-preservation rules across write/edit/conflict handling. Register moves/deletes and editor changes invalidate dependent indexes and instruction candidates. Preserve snapshot provenance through errors and truncated previews.

**Exit:** donor fixture corpus plus new collision/concurrency/CRLF/BOM/symlink/failed-commit tests pass. Syntax-block and register cross-file cases run through the public tool. Supported-language matrix is explicit. No full-parity claim based only on range edits.

## M3 — Complete advanced read pipeline and search (later)

Depends on M1/M2. Integrate inline selectors, explicit/delimited multi-read, summary folding, enclosing boundaries, cache invalidation, seen-row recording, repeated-read hints, safe context supersession, directory paging and grep/find enhancements.

Deliver specialized adapters as independent, testable slices: JSON/JSONL query and pagination; SQLite schema/rows/key/query; archive listing/member reads including donor format inventory; PDF/document text and page images; notebook cell views with edit roundtrip; raster/SVG conversion; static URL readable/raw views; executable overview/symbol/import/export/string/assembly/pseudocode capabilities; video frame/time/contact-sheet and profile views. M0 specifies exact backend/format support; sequencing is not permission to drop a reader.

**Exit:** every approved adapter works through the same URI/selector/budget/output pipeline. Streaming/ranged access avoids whole-source allocation where feasible. Decompression bombs, runaway queries, special files, hostile URLs and converters are bounded. Derived views cannot issue editable source anchors without a reversible mapping. HTTP redirects obey network policy and never forward unrelated credentials.

## M4 — Catalog and control schemes (later)

Depends on M1; integrates with M3. Implement the SPEC-0012 trees: `session://`, `agent://`, `project://`, `artifact://` browsing, search and transcript/tree/entry projections; `skill://` catalog/supporting files with progressive disclosure (SPEC-0014 §8); `cfg://` controls and `cfg://workbench/`; `docs://`; conflict resource registry; walking rules and first-touch guides.

Use host-owned catalogs or a documented indexed reader when no API exists. Account for configured session roots, partially appended JSONL, renamed/deleted sessions, compaction, branch selection and no-session mode. Bind pages to a stable revision/cursor rather than renumbering an actively growing log mid-read. Global authorization applies to search snippets as well as full reads.

**Exit:** schemes have end-to-end read/list/search/control tests and pagination. No settings credential exposure or trust widening via project config. Reload acknowledges a scheduled action and executes at a verified safe boundary.

## M5 — SSH, worktrees and jobs (later)

Depends on M1 and integrates M2–M4. Deliver shared SSH transport, remote file/search/edit operations, location transitions/worktrees, host notes/config management and `/remote`. Preserve SSH config includes/comments and require appropriate confirmation for changes. Remote edits need snapshot validation and atomic commit semantics, not a download/edit/blind-upload shortcut.

Implement bash foreground/background/promotion and one supervisor used by `jobs`, `job://` and command panels. Worktree creation applies the shared artifact tree and worktree links (SPEC-0012 §4.3–4.5); session buckets become symlinks to the project directory (SPEC-0012 §3.5). Include stdin/EOF/signals/PTY keys, cursors, waits, notifications, log limits, secret-safe display and crash/shutdown cleanup. Jobs retain their launch location and ownership through branch navigation; navigating history does not rerun a process.

**Exit:** controlled local/SSH integration tests cover quoted paths, auth failure, disconnect, changed remote files, cancellation, noisy pipes, process descendants, launch/adoption races, reload and crash. URI controls cannot bypass tool permission checks. No unverified guarantee that killing the SSH client killed every remote descendant.

## M6 — Full instruction and ambient-context lifecycle (later)

Depends on M1, M3–M5. Deliver the section 6 contract: file classes, ancestors/root+nearest/root/nearest selection, pins, local/remote watchers, discovery of newly created candidates, diff updates, capped views, deduplication/relocation/withdrawal, self-edit behavior and branch/compaction reconstruction.

Add `select = "all"` classes, the remaining context frames and `builtin/workbench` (SPEC-0014 §5, §7, §9). Provide `/workbench instructions` inspection with reasons: selected, shadowed, pinned, ignored, untrusted, capped, withdrawn. Implement external project/path-scoped notification ingress without storing tasks or agent definitions.

**Exit:** full instruction matrix passes, including parallel codemode touches and changes during compaction. User-visible captures and model-context evidence show the same facts with independently configurable display visibility.

## M7 — Complete Workbench interaction and rendering (later)

Depends on service milestones; work can overlap once APIs stabilize. Finish ask_user and `/answer` flows, pending recovery, remote bridge, replay, optional answer history/redaction policy; `/tldr` checkpoints; codemode nested trees/replay; settings/preview/tools/jobs/frames/trust/remote panels and companion commands. Preserve the donor's useful user-facing controls without importing excluded agent/task panels.

Use kit views, keybindings and headless alternatives. Present captures before settling layouts. One-shot inference uses host providers and configured models; no new subagent abstraction. Billed live inference checks require authorization.

**Exit:** all command inventory rows exercised; live nested call/cancel/error/replay rendered correctly; TUI/RPC ask submission/rejection/cancel verified; print/JSON never block waiting for dropped UI. Unknown metrics are absent, not invented zeroes.

## M8 — Integrated release gate (later)

Re-run all scope rows against the assembled extension, including fused/subprocess behavior where applicable. Validate package identity, tool conflicts, resource discovery, documented prerequisites and installation/uninstallation. Review notices, product docs, privacy and cleanup. No version bump before the release boundary; no push/publication without authorization.

**Exit:** no unverified required scope row. Residual host/platform limitations are explicitly accepted by the user, not relabeled as finished features.

# 6. Ambient-file contract to implement

This is the concrete new feature, not just “add BACKLOG.md to a filename list”.

1. **Classes:** each rule has ID, ordered candidate filenames/patterns, boundary, selection policy, trust classification, update mode, item/line/byte budgets and pin permission. Candidate-name precedence is separate from whether all ancestors participate.
2. **Root semantics:** distinguish an exact boundary-root file from the broadest existing ancestor match. The specification must name the chosen root+nearest rule; proposed default is broadest applicable match plus closest, deduplicated when identical. `root` means exact configured boundary. Show the effective root in inspection output.
3. **Touched paths:** resolve before discovery. For a multi-file batch, compute the union of applicable files for all targets; do not let whichever goroutine finishes last select the sole nearest file. Reconcile deterministically at the batch boundary; hold needed contexts through the batch. Define retention for subsequent touches in the contract, with branch-local pins for enduring relevance.
4. **Before mutation:** applicable instructions must be visible before an agent-authored change proceeds. Probe how newly discovered instructions can reach the model in a running tool batch; if a retry is necessary, return a typed instruction-context-changed result with no mutation rather than injecting after the write.
5. **BACKLOG.md:** all ancestors by default, not root+nearest. Cap complete pointer items, never rewrite the source file. Keep exact omitted-item count and canonical continuation path. Avoid guessing item totals after a byte-limited scan; bounded scan failures report unknown totals explicitly.
6. **Two baselines:** retain source identity/version and the exact delivered capped view. Diff the delivered view against the previous delivered view. A change beyond the cap updates source version/count and a concise notice where relevant; never send a diff against unseen lines. Reordering items around the cap must be reconstructible by applying the emitted diff.
7. **Pinning:** branch-local persistent pin/unpin facts; pins retain updates across cwd changes but cannot widen trust, escape authorized resources or survive deletion as if content remained valid. Root+nearest remains automatic unless the rule explicitly defines a pin as replacing nearest selection. Inspect why each file is active.
8. **Change detection:** watch candidate directories as well as loaded files so new/deleted/atomically replaced files are discovered. Debounce and coalesce; recheck at turn boundaries to heal missed events. Cache negatives only with invalidation. Remote on-turn checks preserve behavior when remote watching is unavailable.
9. **Delivery:** identify branch generation, epoch, class, canonical resource and base/new version. Only advance the delivered baseline after successful delivery. On compaction or missing baseline, send full current capped views rather than unusable diffs.
10. **Deduplication:** equal content plus compatible trust and scope is required. A linked worktree is not automatically identical. Withdraw scopes explicitly; nonce/tag syntax does not confer authority.
11. **External notices:** accept project identity, event ID, source, affected paths and bounded payload from authorized bridges; deduplicate and filter path overlap. Project-wide claims/completions are external facts, not Workbench task records. Cross-process fan-out must use a proven transport, not merely a session emitter.

# 7. Verification, performance and definition of done

## Continuous gates

Every implemented slice includes behavior tests, docs and a scoped diff review. Run formatting/vet/unit tests first, then race/integration checks for concurrent or lifecycle changes. Validate actual SDK registration using `pig install ... --validate-only --json`. Do not weaken donor behavior tests to make a partial port pass; document intentional semantic improvements and add tests for them.

- **Differential fixtures:** compare hashline operations, block spans, recovery and read selections against pinned donor fixtures/reference execution where practical. No live user files in mutation tests.
- **Cross-feature scenarios:** read summary→range→hashline block edit; grep→edit; archive/member paging; JSON query continuation; cd→new instructions→remote edit; codemode multi-read→seen-state; jobs URI vs tool control; resume→pin→diff; nested-worktree session→`session://project` search.
- **Failure matrix:** cancellation, partial I/O, malformed config/URI/input, storage quota, slow consumer, watcher loss, disconnect, stale revision, branch switch, reload, shutdown and crash.
- **Security:** terminal escapes, path traversal/symlinks, SSH argument injection, read-only SQL enforcement, archive bombs, credential redaction, and URI policy parity.
- **TUI:** captures at 36/57/80/140 columns, plus short height (~16 rows), resize, long paths, hostile text and failures. Review captures, not just process exit codes.
- **Modes:** TUI, RPC, print and JSON. Script-facing behavior must be reachable without slash-command UI.

## Performance evidence

Measure startup, IPC count, render cost, parser cache hit/miss, large-file range/tail reads, multi-read memory, query/archive limits, history indexing/search, job output throughput and instruction discovery under many touches. Set numerical targets after recording the initial reference/environment baseline; no invented latency promises.

Immediate invariants: bounded queues/caches/results; no filesystem/network/parsing work during render; no whole-session hydration to list metadata; no full-tree instruction scan per tool call; no per-line host IPC; only fully emitted lines count as seen. Document exceptions needing whole-source parsing and cap them explicitly without pretending a partial parse is complete.

## Evidence and completion rule

Each milestone updates this plan's progress and links its producing analysis/test/capture artifact. Completion requires observed checks, relevant user-facing paths and WB traceability, not file count or scaffolding. Report failed/unavailable gates with impact. Documentation states supported formats/platforms, configuration, command names, trust behavior, persistence, privacy and cleanup accurately.

# 8. Execution discipline and approval boundaries

- After approval, start with M0—not the previous proposal's broad “Wave 1” implementation list.
- Keep commits granular by coherent behavior; stage explicit owned paths. Parallel work uses isolated worktrees and stable shared contracts; only the integration owner edits registration/common types.
- Do not install dependencies, alter the host, modify shared systems, launch billed inference or publish without the required approval.
- A blocker produces evidence, options and a concrete recommendation. Continue independent work, but retain the blocked requirement in scope.
- Update plans/specs when decisions change. The proposal's “full port” wording never authorizes a silent weaker substitute.

# 9. Requirement ownership

Authoritative assignment of every specification section to a slice and phase. Slice plans may refine steps but not move a row without a revision here. S1 = [PLAN-0002](0002-slice-1-foundation.md), S2 = [PLAN-0003](0003-slice-2-feature-complete.md), S3 = [PLAN-0004](0004-slice-3-completion.md). Modes and host variants follow PLAN-0002 §6.

| Spec § | Requirement | Owner | Acceptance anchor |
|---|---|---|---|
| 0001 §2–§7 | Discovery, request/reply, registrations, events, Go API | S1 C | SPEC-0001 §7 |
| 0001 §8 | Collaboration services (location, status, snapshots, fetch, sessions, docs) | S1 C with the consuming phase | SPEC-0001 §8; pig-web gate |
| 0002 all | Frames, ledger, delivery, projection, trust | S1 F | SPEC-0002 §8 |
| 0003 §2–§6, §10 | Presentation data contract, resolver, settings, path display | S1 F, B | SPEC-0003 §8 data rows, §10.3 |
| 0003 §7 | Density, codemode rows, accessibility | S3 C3 | SPEC-0003 §8 captures |
| 0004 §1–§10 | Router, grammar, envelope, policy, provenance, builtin routes | S1 D | SPEC-0004 §10 |
| 0004 §11 | Fetch backends and builtin web reader | S1 D | SPEC-0004 §11 |
| 0005 §2–§8, §10, G5.2 | Hashline grammar, snapshots, recovery, commit, edit tracking | S1 G | SPEC-0005 §8, G5.2 fixtures |
| 0005 §9 except G5.2 | Payload repair, leniency, per-model mode, streaming, repair, notebooks | S2 W1 | Donor fixture corpus |
| 0006 §2–§4, §6–§7 | Read schema, pipeline, summaries, search tools, loop guard | S1 D | SPEC-0006 §8 |
| 0006 §5 | Adapter matrix | S2 W2 | SPEC-0006 §5, §8 |
| 0007 §2–§8 local | Classes, roots, touches, barrier, pins, caps, local watchers | S1 K | SPEC-0007 §10 |
| 0007 §8 remote, §9 | Remote watching, external notifications, further classes | S2 W5 | SPEC-0007 §10 |
| 0008 §2–§3 local | Location capture, `cd` | S1 E1 | SPEC-0008 §8 |
| 0008 §3 worktrees, §4 | Worktrees, SSH, `/remote` | S2 W3 | SPEC-0008 §8 |
| 0008 §5–§7, G8.20 | Bash, jobs, logs, `job://`, scheme commands | S1 H | SPEC-0008 §8; PLAN-0002 H |
| 0008 PTY, remote jobs | PTY backend and keys, jobs over SSH | S2 W3 | SPEC-0008 §8 |
| 0009 §2–§3, §5 | `ask_user`, state machine, modes | S1 L2 | SPEC-0009 §10; PLAN-0002 §6 |
| 0009 §4, §6–§7 | Remote bridge, `/answer`, `/tldr` | S2 W6 | SPEC-0009 §10 |
| 0009 §8 | Command routes | Split per PLAN-0002 §2 Commands row | SPEC-0009 §10 |
| 0009 §9 | Activity line and capture (S1 M, F); call trees and replay (S2 W6); statusline modes and click (S3 C3, C4) | as listed | SPEC-0009 §10 |
| 0010 all | Config, state, trust, storage, cleanup, lifecycle | S1 B | SPEC-0010 §9 |
| 0011 §2–§8 | Endpoint, transport, envelope, delivery, guards, `agent://`, API | S1 L1 | SPEC-0011 §10 |
| 0011 broadcast, §9 | Broadcast, coordinator, `/workbench coordinator` | S2 W6 | SPEC-0011 §10 |
| 0012 §3.1–§3.4 | Project model, `Project.toml`, recognition, registry | S1 E1 | SPEC-0012 §6 |
| 0012 §3.5 | Symlinked buckets, `projects unlink-storage` | S1 E2 | PLAN-0002 E2 gate |
| 0012 §3.6 | Private notes class | S1 K | SPEC-0007 §10 |
| 0012 §4.1–§4.2 | Bundled conventions, artifact and BACKLOG validation, module reminder | S2 W7 | PLAN-0003 W7 |
| 0012 §4.3–§4.5 | Shared artifact tree, worktree links, artifact storage modes | S2 W3 | SPEC-0012 §6 |
| 0012 §5.1–§5.2, §5.5 | Naming rule, tool semantics, first-touch guides | S1 D | SPEC-0012 §6 |
| 0012 §5.3 core | `session://` self/`<id>`/`by-id/`, own and stored titles, `agent://`, `skill://`, `cfg://`, `job://`, `artifact://`, `out://`, `local://` | S1 D, H, J, L1 | SPEC-0012 §6 |
| 0012 §5.3 rest | `project://`, `conflict://`, `by-project/`, `by-name/`, search | S2 W4 | SPEC-0012 §6 |
| 0012 §5.3 titles of live sessions | Other live sessions under coordinator `lifecycle` | S2 W6 | SPEC-0012 §6 |
| 0012 §5.4 | `docs://` registered docs (S1 C, D); PiG source (S2 W4) | as listed | SPEC-0012 §6 |
| 0013 §2–§5, §7 | Detection, vault, boundary, env injection, ask secret | S1 I | SPEC-0013 §11 |
| 0013 §6, §8 UI | `secret://`, `/workbench secrets` | S2 W4 | SPEC-0013 §11 |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | Engine, delivery, frames, conditionals, `builtin/workbench`, skills | S1 J | SPEC-0014 §12; PLAN-0002 exit 4 |
| 0014 §9 | `select = "all"` | S2 W5 | SPEC-0014 §12 |
| 0014 §6.3 | Family adapters from the harness research, each with A/B evidence | S3 C6 | SPEC-0014 §12; PLAN-0004 C6 |
| 0014 fork delivery | `sections` delivery, F-P3, F-P4 | S3 C4 | Fork-host rows |
| All UI chrome | Hub, panels, renderers by density, captures | S3 C1–C3 | SPEC-0003 §8 captures |
