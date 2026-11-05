---
document:
  type: "plan"
  id: "PLAN-0002"
  title: "Slice 1: complete foundation — every tool, seam, abstraction and core URI working"
  description: "All model-facing tools, all extension seams, all shared abstractions, the URI system with its core schemes and the system prompt, fully working on their final implementations; depth follows in PLAN-0003"
  status: "draft"
  revision: 10
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-05T15:30:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed PLAN-0001 M0 remainder (M0.1 decisions, M0.4 freeze) and M1", "Pulled the foundation of every later milestone forward so each public seam runs", "Defined the pig-web co-load gate as the slice exit"]
    - revision: 2
      reason: "User defined foundation: everything fully working on its final implementation, only depth deferred; decided D5.b and D5.d"
      changes: ["Foundation rule rewritten: no stubs, no unwired parts, no interim implementations", "All ten tools, all commands of founded features and the core URI set in this slice", "D5.b adopted, D5.d rejected (SPEC-0005 rev 8)"]
    - revision: 3
      reason: "External plan review (2026-11-03): eight findings, all confirmed"
      changes: ["builtin/workbench instead of the removed builtin/pig", "Project identity (E1) split from session-storage adoption (E2), which now follows the endpoint registry and has its own gate", "G8.20 scheme commands owned by Phase H", "Mode and host matrix (§6); ask_user headless result is interactionUnavailable", "Commands /workbench skills, sessions, projects unlink-storage added", "Skills, all six context frames and G5.2 relocation validation in scope", "Bus acceptance scoped to cooperative listeners", "Phase A produces a reconciled decision/probe/dependency ledger", "Requirement ownership moved to PLAN-0001 §9"]
    - revision: 4
      reason: "User approved the default-prompt A/B run; model-aware prompts moved to Slice 3"
      changes: ["Exit 4 approved", "Family adapters point to PLAN-0004 C6"]
    - revision: 5
      reason: "User approved pig-web M1 and M2 contract choices"
      changes: ["Recorded opt-in secrets.redact for trusted extension inference", "Recorded owner-scoped session.model fields", "Reconciled pig-web M3–M6 with explicit validation gaps", "Assigned both approvals to their owning Slice 1 phases; freeze remains pending"]
    - revision: 6
      reason: "Implementation resumed after the user rejected documentation-only progress"
      changes: ["Recorded the wired local read/location runtime and live verification", "Kept every phase exit and the full slice explicitly incomplete", "Recorded the configuration dependency approval needed for strict TOML validation"]
    - revision: 7
      reason: "User granted standing dependency approval conditioned on current-version and supersession checks"
      changes: ["Recorded verified dependencies and license notices", "Implemented scoped scalar TOML configuration, source-safe validation, merge/conflict handling, trust rechecks and private atomic saves", "Wired settings commands and live read budgets; verified separate-process writers and real-host reload/scope behavior", "Added the documented consumer client and exercised it from a separate extension", "Retained incomplete Slice 1 exits explicitly"]
    - revision: 8
      reason: "Parallel single-unit delegation for the remaining Slice 1 engines"
      changes: ["Recorded ten in-flight units and the six correctly refused mis-sized briefs", "Recorded every lead API decision that unblocked a unit", "Retained the no-stubs rule: ask_user and other tools stay unregistered until their full path exists"]
    - revision: 9
      reason: "Consolidated the committed engine inventory and remaining one-unit completions"
      changes: ["Recorded which engines are wired versus tested-but-unwired", "Recorded the exact unit that unblocks each remaining feature"]
    - revision: 10
      reason: "Resume checkpoint: integrated pending worktrees, current-source edit recovery, HTTP/HTML, presentation registration and full gates"
      changes: ["Recorded green check/race/integration evidence at 14d7fb3 plus later commits", "Recorded the create-only write race fix, current-source relocation and presence of all ten tools", "Retained every open phase exit and the real pig-web co-load blocker explicitly"]
---

# 1. Outcome

At the end of this slice Workbench loads in PiG and **everything exists and works fully**: every model-facing tool, every extension seam, every shared abstraction, the URI system with its core schemes, the system prompt and the commands of all founded features. pig-web can run its real-Workbench gate (its PLAN-0001 Slice 5) and close its design and planning against running services.

**What "foundation" means here (user, 2026-11-03):**

- Built on the **final implementation**. Nothing is built differently now and rebuilt later. Slice 2 adds depth to the same code; it never replaces a Slice 1 engine.
- **Fully working and wired.** No stubs, no placeholder results, no `unsupported` answers for operations a founded feature advertises, no registered-but-unused code. If a capability is not in this slice, it is not advertised yet.
- **Depth is what waits:** additional schemes and trees, format adapters, further backends (SSH, PTY), donor-parity heuristics, secondary flows. Each is listed in [PLAN-0003](0003-slice-2-feature-complete.md).
- **Chrome waits:** plain renderers that follow the SPEC-0003 data contract; kit panels, designed layouts, captures and polish are [PLAN-0004](0004-slice-3-completion.md).

Umbrella and scope: [PLAN-0001](0001-pig-workbench.md). **Requirement ownership** per specification section, with slice, phase and acceptance anchor, is PLAN-0001 §9; this plan's phases must satisfy every row assigned to Slice 1 there.

## Progress

- **done:** M0.2 host probes (P1–P30, S1–S6, PR1–PR8), M0.3 backend matrix, M0.1 test-level audit, 14 draft specifications, bootstrap factory. D5.b adopted and D5.d rejected (SPEC-0005 rev 9). External plan review applied (rev 3).
- **now:** Phase A (ledger and contract freeze), including the pig-web handoff [HO-pig-web-m0-4-inputs](../handoff/pig-web-m0-4-inputs.md). M1 and M2 choices approved by the user and recorded in SPEC-0001/0002/0010/0013/0014; these decisions are not runtime implementation or approval of the entire freeze.
- **implementation checkpoint:** `extension.go` now wires `internal/host/`; `api/` contains v1 wire types. Owned shutdown draining, local location leases/transitions/replay, selector parsing, bounded UTF-8 source paging, `read`, `cd`, plain inspection commands and the `location` service are implemented. These are parts of B/C/D/E1/G, not completed phases. Discovery advertises only the wired `location` capability; no placeholder registration kinds or other services are advertised.
- **verified:** `just check`, `just race`, `just integration`. `internal/host/live_test.go` drives the real PiG agent loop against a localhost scripted provider: direct raw selection, cd-relative reads, codemode success/error child captures and reset. `internal/host/live_services_test.go` co-loads the independent `service-client` factory in RPC: discovery, stale-generation rejection, location changes, session replacement and persisted-location resume. No billed inference. Width/control-character tests cover notices at 36/57/80/140 columns; these are not live TUI captures.
- **configuration checkpoint:** `internal/config/` now owns scoped scalar loading, validation/provenance, last-valid stale state, whole-array replacement, runtime reset, strict project reductions and user/runtime-only fields. Saves merge disjoint key updates under native cross-process locks, reject same-key/canonical-target conflicts, preserve comments/unknown data, validate the round-trip result, use private temporary files and distinguish saved versus published. `internal/host/settings.go` wires plain inspection/set/reset/reload and completions; read byte/line ceilings are captured per operation and included in Details. Other scalar entries are explicitly unavailable, not falsely applied. Complex instruction classes, trust-rule editing and dynamic producer settings are not implemented.
- **configuration evidence:** `internal/config/*_test.go` covers malformed reloads, project denial, source-safe diagnostics, symlink retention/retarget rejection, comments on reset, reconciliation failure, cancellation and separate-process disjoint saves against the same captured baseline. `internal/host/live_settings_test.go` verifies user/project/runtime precedence, reset, persistence, reload, ignored untrusted project settings and config roots surviving cd. `live_test.go` verifies configured read limits in a real agent loop. Windows/amd64 and Darwin/arm64 config test binaries compile; neither platform was run. `just check`, `just race` and `just integration` pass on Linux.
- **service-consumer checkpoint:** `api/client.go` provides the optional v1 consumer (`Discover`, synchronous `Call`, `Close`) with correlation by request/session/generation/identity, duplicate-responder and missing-capability refusal, local oversized rejection, non-blocking deadlock avoidance and outcome-classified transport errors. The integration fixture uses it instead of a private reimplementation; live tests cover discovery, stale-generation rejection and local `limitExceeded`. Documented in `docs/extension-services.md`. Timeouts remain cooperative because the shared bus cannot preempt a hung listener (SPEC-0001 host limit).
- **delegation checkpoint:** remaining Slice 1 engines are being built as single-unit delegations with lead-owned shared contracts (`internal/resource` provider router, `api` protocol, `internal/config` snapshots). Units in flight: syntax, hashline (byte model), jobs (process supervision), secrets (keys/placeholders), prompt (render core), project (document), frames (admission), instructions (discovery/render), ask (schema/persistence), endpoint (registry). Six first-round briefs were correctly refused as mis-sized; each was re-briefed to one unit after the lead resolved its API objection:
  - hashline: `Render(path, tag, rows)`; `Restore` must reject ambiguous ending mappings instead of guessing.
  - jobs: `Start` keeps the ID contract; `RunForeground` returns the state; Windows group-kill is a typed unsupported error.
  - secrets: `Redact(text) (string, error)` fails closed; `Policy` supplies no key or vault.
  - prompt: `go.mod`/`go.sum`/`THIRD_PARTY_NOTICES.md` own dependency pinning (gonja + gotreesitter), not the unit.
  - project: detection belongs to `Registry`, not a free function.
  - frames: admission takes an explicit `Context{SessionID, ProducerPrefix, BranchLeaf, Epoch}`.
  - instructions: steering defaults exclude `CLAUDE.md`/`GEMINI.md`; digests are exact-byte, so CRLF and LF differ; user roots are user-trust and searched once.
  - ask: the tool stays unregistered until schema, dialogs, TUI panel and secret storage all exist; SDK `ok=false` ambiguity is resolved in the dialogs unit.
- **engine inventory (committed):** `resource` (selectors, provider router, file/local/out/artifact providers, paging, tags), `config` (scoped TOML, provenance, safe saves), `search`, `syntax` (summaries + blocks), `hashline` (byte model, XXH32 tags, tagged headers, patch grammar), `jobs` (process supervision, group escalation, exit watcher), `secrets` (placeholders + fail-closed redaction), `frames` (session ledger), `instructions` (discovery + caps), `ask` (schema + records), `project` (document model), `endpoint` (session records), `prompt` (render core, strict `attr`, `ErrEmptyRender`), `api` (protocol + consumer client).
- **wired to the model:** `read` (selectors, tags, code summaries, directory listings), `write`, `ls`, `grep`, `find`, `cd`; schemes `file`, `local`, `out`, `cfg`, `session`, `artifact`; location service and consumer client. Everything else is a tested engine awaiting its wiring unit and is not advertised.
- **next completions, one unit each:** `edit` needs hashline staging/seen guards/exact-text; `bash`+`jobs`+`job://` need the jobs Manager; `ask_user` needs the vault plus dialogs; prompt takeover needs confinement, section tags and `builtin/workbench`; `session.project` needs the project registry detection unit.
- **resume checkpoint 2026-11-05:** the retained completed worktrees for staged line endings, relocation, secret payload guards, HTML reading and presentation snapshots are integrated. The public edit path now re-reads exact current source, authorizes in the model's coordinates, relocates through unique anchors, refuses ambiguous/absent targets and carries authored/delivered seen spans forward. Create-only `write` uses atomic no-replace publication after a reproduced overwrite race. The prompt limit test, documentation and package guidance agree on 256 KiB. `http(s)://` is wired through bounded, redirect-reauthorized static fetching with readable Markdown/raw HTML and charset/media policy. The v1 bus now registers, unregisters and describes `toolPresentation` contributions and routes opted-in tools through a live semantic renderer; other contribution kinds fail `unsupported` rather than pretending to work. `just check`, `just race` and `just integration` pass on this checkpoint. No billed inference was used.
- **next:** Continue C–M. The remaining contribution kinds and their consumers, trusted secret grants, Phase A ledger/freeze, full prompt/skills takeover, instruction lifecycle, session messaging, journaled session storage, ask panels/recovery, command/activity coverage and the complete acceptance table remain open. Full syntax blocks, donor recovery/leniency, persistent register replay and path-display goldens are still missing. The real `../pig-web` co-load gate rechecked on 05.11.2026 still has no extension implementation, so P2 is an external blocker and cooperative fixtures do not close it. No Slice 1 exit is claimed.
- **parallel:** personal PiG fork (D13). Nothing in this slice depends on it; Workbench detects fork capabilities by flag.

# 2. What is in Slice 1 and what is depth

Summary by area; PLAN-0001 §9 is authoritative.

| Area | Slice 1 (complete) | Depth in PLAN-0003 |
|---|---|---|
| Tools | `read`, `write`, `edit`, `grep`, `find`, `ls`, `bash`, `jobs`, `cd`, `ask_user`: final schemas, descriptions, result envelopes, direct and codemode | — |
| Edit | Full hashline grammar (sections, ranges, gaps, PUT/CUT, blocks, registers, REM/MV), seen guards, D5.b with G5.2 relocation validation, exact-text mode, revision-checked commits, previews | G5.1 payload repair, G5.3 leniency, G5.9 per-model rules, G5.10 streaming, model-assisted repair, notebooks |
| Read and search | Selectors, multi-read, paging, continuation, seen-state, code summaries and enclosing boundaries on the syntax service, supersession, repeated-read hints, directory paging, grep/find on the router | Adapters: JSON queries, SQLite, archives, PDF/documents, images/SVG, binary, video, profiles |
| URI system | Parser, selector grammar, router, pager, walking rules, guides; core schemes `file://`, `http(s)://` (fetch backends + builtin reader), `artifact://`, `out://`, `local://`, `job://`, `session://` (`self`, `<id>`, `by-id/`, titles of own and stored sessions), `skill://`, `cfg://`, `docs://` (registered docs), `agent://`; external providers and fetch backends; bash scheme commands (G8.20) | `ssh://`, `project://`, `conflict://`, `secret://`, `session://by-project/` and `by-name/`, cross-session search, titles of other live sessions, docs PiG source |
| Location and projects | Location abstraction with revisions and leases, local backend, `cd`; project identity and registry; symlinked session buckets with undo | SSH backend, `/remote`, worktrees and links, shared artifact tree, artifact storage modes, project conventions (SPEC-0012 §4.1–4.2) |
| Jobs | One supervisor: foreground, background, promotion, stdin/EOF/signals, logs with cursors, cancel, cleanup, env injection | PTY backend and key controls, remote jobs |
| Context | Frame ledger, presentation resolver, instruction class engine (AGENTS.md, BACKLOG.md, project private notes), root+nearest, pins, caps with both baselines, diff updates, local watching, compaction and branch reconstruction | Remote watching, external notification ingress, further classes and `select = "all"` |
| System prompt | gonja engine, `builtin/workbench` (default, SPEC-0014 §7), conditionals, delivery `auto`, epochs, late-section splice, all six context frames, skills complete (listing ladder, load/unload, `when`, `auto_load`, nested, compaction replay) | `select = "all"` (PLAN-0003 W5); model-family adapters of SPEC-0014 §6.3 with A/B evidence (PLAN-0004 C6) |
| Secrets | Detection, placeholders, vault, boundary hooks, substitution, env injection, prompt section, `ask_user` type `secret` | `secret://` and `/workbench secrets` |
| Interaction | `ask_user` complete in TUI and RPC, `interactionUnavailable` in print/JSON, pending recovery | Remote bridge, replay, answer history, `/answer`, `/tldr` |
| Sessions | Endpoint, transport, auth, wake, state, delivery, loop guards, `agent://` send/read/wait, `sessions` channel | Broadcast, coordinator with `/workbench coordinator` |
| Commands | `/workbench` with `show`, `settings`, `integrations`, `resources`, `prompt`, `skills`, `sessions`, `jobs`, `frames`, `instructions`, `trust`, `projects unlink-storage`; `/tools`, `/cd`, `/wb:cd`, `/wb:jobs`; completions (plain views, modes per §6) | `/remote`, `/answer*`, `/ask:*`, `/tldr`, `/workbench coordinator`, `projects artifacts-storage`, `--list-tools`; hub, `preview` and `statusline` are chrome (PLAN-0004) |

# 3. Phase A: ledger and contract freeze (M0.4)

## A0. Reconciled ledger

Before freezing, reconcile every open decision, outstanding probe, dependency approval and known spec contradiction into one table in this section, each with the phase it blocks. Seed (to be completed by checking every spec's open items):

| Item | Kind | Blocks |
|---|---|---|
| Dependency policy and TOML/locking selection | resolved by user standing approval, checks performed 03.11.2026 | `go list -m -json <module>@latest`: `creachadair/tomledit` v0.0.29 and `BurntSushi/toml` v1.6.0 (MIT), `gofrs/flock` v0.13.1 (BSD-3-Clause) and its `golang.org/x/sys` dependency updated to latest v0.49.0 (BSD-3-Clause). Upstream GitHub repository metadata and current READMEs show maintained/non-archived projects with no deprecation/replacement notice; flock is the successor to theckman/go-flock. Latest/maintenance checks are required for future additions, not repeated approval questions. Licenses: `THIRD_PARTY_NOTICES.md`. |
| pig-web M1: extension-owned inference | decision resolved by user | C/I: opt-in `secrets.redact` for trusted owners; unprotected child sessions explicit; SPEC-0013 §4.5 |
| pig-web M2: contributed model fields | decision resolved by user | C/F/J: owner-scoped `contextFields` on `frameProducer`, `context.fields.set`; SPEC-0014 §5.1 |
| pig-web M3: descriptor summary | review text prepared | D/J: required one-line summary, ≤120 Unicode characters; SPEC-0004 §2 |
| pig-web M4: native-provider credential expansion | probe pending, prior claim qualified | I acceptance: implementation owner probes native custom resolver and EnvAPIKeyAuth separately; SPEC-0013 §2/§9 |
| pig-web M5: trailing annotations | review text prepared; both-order integration tests pending | F/G/I acceptance: intact snapshot block stays valid; annotation producer redacts appended text before returning; SPEC-0005 §10 and SPEC-0013 §4.2 |
| pig-web M6: activity items and URI open | existing contract confirmed | M: SPEC-0009 §9 includes placement, glyph, command or URI open, count and severity; click remains PLAN-0004 C4 |
| Messaging probes: wake from a goroutine, idle/busy state, reload rebind (SPEC-0011 §10) | probe | L1 acceptance |
| Session title probes T1–T3 (SPEC-0012 §6) | probe | L1 titles |
| Resumed-session cwd under `--continue` with symlinked buckets (SPEC-0012 §3.5) | probe | E2 |
| G5.10 abort while arguments stream | probe | PLAN-0003 W1 |
| Widget view events for activity-line click | probe + fork | PLAN-0004 C4 |
| Archive additions, backend evaluation §10 item 6 | dependency approval | PLAN-0003 W2 archives |
| D9.c answer history (provisional) | decision | PLAN-0003 W6 |
| SPEC-0012 §4.1 `tasks` line in the bundled skill | decision | PLAN-0003 W7 |
| SPEC-0005 §3 vs D5.b | contradiction | fixed in SPEC-0005 rev 9 |
| PLAN-0001 WB-25 still naming `builtin/pig` | contradiction | fixed in PLAN-0001 rev 23 |

## A1. Freeze

Freeze everything that fixes data models, persisted records, wire formats, model-facing shapes or safety boundaries, for the **whole** scope, so depth never forces a contract change:

- **Wire:** SPEC-0001 envelope, `discover` capabilities, registration kinds and descriptor fields, events, generations, error codes, limits. Go authoring API as compile tests against the staged SDK (SPEC-0001 §6).
- **Model-facing:** the ten tools' schemas, descriptions and result envelopes; selector grammar and precedence; URI grammar and reserved schemes (SPEC-0004, SPEC-0012 §5); G8.20 command forms.
- **Persisted:** config keys and files (SPEC-0010), frame ledger and delivery records (SPEC-0002 §4), snapshots and seen-state (SPEC-0005 §3), jobs (SPEC-0008), project registry, `Project.toml` and bucket migration state (SPEC-0012 §3), vault (SPEC-0013 §3), prompt epochs (SPEC-0014 §4.3), loaded skills, pending asks; branch vs session ownership and recovery after reload.
- **Behavioral:** root+nearest, pin and cap semantics (PLAN-0001 §6), command routes, the mode and host matrix (§6).

Each frozen specification gets a revision with `status: "approved"` after user review. Later changes need a new revision and a migration note.

## A2. Answer pig-web's M0.4 inputs

Answer M1–M6 inside the freeze: extension inference and secrets (incl. research child sessions), `knowledge_cutoff` in `session.model`, the descriptor `summary` field, `!command` expansion by registration kind (qualify SPEC-0013 S6), trailing tool-result blocks and redaction order, activity item fields and URI `open`. Choices that need the user are asked before the freeze commit. Reply with a handoff into pig-web naming the freeze commit; archive the inbound handoff.

**Phase A exit:** ledger complete with an owner for every row; frozen specifications committed; no open decision that changes a data model or safety boundary; pig-web reply delivered.

# 4. Phases B–M

Order: B; then C, D, E1 in parallel; F–K as dependencies land; L1 before E2; M last. Development of independent phases runs in parallel; a phase is complete only when its integration dependencies are complete too. Only the integration owner edits registration and shared types (PLAN-0001 §8).

## B. Factory, configuration and host adapter (PLAN-0001 M1.1, M1.2)

- Thin `Extension()`, lifecycle owner with shutdown joins, generation counter that survives same-process reload (P13/P14).
- Layered TOML (`Workbench.toml`, `Secrets.toml`), round-trip-safe saves, trust restrictions, source attribution, inter-session locking, atomic private writes (SPEC-0010 §1–§8).
- Host adapter: SDK contexts, hooks, original-builtin delegation, persistence; domain packages stay host-independent.
- Sanitization, width fitting, path display with donor golden cases (SPEC-0003 §10), result and metrics types.
- Test harness: isolated agent dir, fixture projects, RPC and print drivers; `just check`, `just race`, `pig install . --validate-only --json`.

**Acceptance:** malformed config refused with location; unknown keys preserved; concurrent save from two sessions; untrusted project cannot widen trust; reload leaves no goroutines or temp files.

## C. Extension services v1 (SPEC-0001)

- `discover` with every capability: `frames`, `presentation`, `resources`, `location`, `status`, `snapshots`, `fetch`, `sessions`, `docs`, `secrets`. `secrets.redact` requires explicit trusted-owner grants; context field contributions use `frames`.
- `register`/`unregister`/`cancel`, idempotent registration, owner scoping, generations, late-reply discard, bounded waits, no mutex held across Emit.
- Work that can outlive a caller's budget is admitted and run under the session supervisor with an operation receipt (SPEC-0001 §3).
- Every registration kind consumed by its subsystem: `toolPresentation`, `frameProducer` (incl. `contextFields` and `context.fields.set`), `resourceProvider` (incl. `guide`, `summary`), `fetchBackend`, `statusSegment`, `docs`.
- Reference producer extension kept as an integration fixture, plus a small documented example.

**Acceptance:** absent, incompatible and ambiguous Workbench cases; stale generation rejected; reload re-registers cleanly; a cooperative listener stops within one poll interval after `cancel`; late replies are discarded by correlation ID; a mutation cancelled mid-flight reports `outcome: unknown`. An uncooperative listener can block Emit (SPEC-0001 §3); that limit is documented, not tested away.

## D. URI system, read and search (SPEC-0004, SPEC-0006 §2–§4 and §6–§7, SPEC-0012 §5)

- Parser with literal-first disambiguation; selector grammar in one normalized form; `paths:[]` multi-read in input order within one budget.
- Pager with shown spans, continuation, selection- vs resource-complete, artifact spill without recursion.
- Router with capability-declared providers, walking rules, first-touch guides; external providers through every tool; `resources.changed` invalidation.
- Syntax service on the approved tree-sitter backend: code summaries and enclosing boundaries for read, block spans for edit (Phase G).
- `http(s)://`: fetch-backend routing with policy, redirect safety, paging and locate; builtin static reader (readable and raw views on the approved HTML backend) when no backend serves the URL.
- Core schemes from §2; `read`, `ls`, `find`, `grep` overrides complete.

**Acceptance:** SPEC-0004 §10 and SPEC-0006 §8 rows for the core schemes; direct and codemode calls resolve identically; colon/comma/space filenames stay addressable; cancel joins workers; sessions cannot reach each other's scratch; an external `build://` provider pages through `read` with Workbench selectors and an unadvertised `edit` is refused before dispatch.

## E1. Location and project identity (SPEC-0008 §2–§3 local, SPEC-0012 §3.1–§3.4)

- Location abstraction with backend interface, revisions and leases; local backend; `cd` and `/cd`; `location` service and `location.changed`.
- `Project.toml`, recognition (§3.3), registry with `prj-` IDs, `global`, project in provider requests (SPEC-0004 §4). No bucket changes yet.

**Acceptance:** host cwd never changes; relative paths resolve against the captured location; nested project directories resolve to the parent project's ID.

## E2. Session-storage adoption (SPEC-0012 §3.5)

Runs after L1, because conversion must check the endpoint registry for another live session on the same bucket. Conversion at session start, journaled so an interrupted move can be completed or rolled back; `/workbench projects unlink-storage` undo; catalogs deduplicate by session ID.

**Gate (all required before conversion is enabled by default):** another live session on the bucket defers conversion with the reason shown; a crash at each step of the move recovers without losing or duplicating a session file; undo restores real directories and PiG lists every session; the resumed-session cwd probe passes or its result is written into SPEC-0012 §3.5; different filesystems defer.

## F. Frames and presentation (SPEC-0002, SPEC-0003 §2–§6, §10)

- Frame ledger with branch generation guards, receipts, replay on resume, compaction, trust classes, `beforePrompt` delivery.
- Presentation resolver reading the live registry at first use, all presets with typed cells including `uri`, plain renderers for every lifecycle state, contributed display settings, codemode nested capture and touch metadata.

**Acceptance:** SPEC-0002 §8 and SPEC-0003 §8 data rows; no duplicate frame after resume or branch switch; an untrusted producer cannot raise trust; a pig-web-style `web_*` tool renders through the resolver in TUI, RPC and print.

## G. Edit, write and snapshots (SPEC-0005 §2–§8, §10, G5.2)

- Snapshot store: canonical identity, revisions over exact bytes, retained spans, seen-state from fully emitted lines, tag binding.
- Full hashline grammar and staging, syntax blocks via Phase D's service, registers, REM/MV, seen guards, mismatch reveals, no-op guards, post-edit results; exact-text mode.
- Recovery per G5.2 (body comparison, remapping, refusal rules), which D5.b's newest-snapshot rule depends on.
- Commit path: sorted multi-resource locks, revision check before commit, atomic replace with mode preservation, exact outcome reporting; `write` with `expectedRevision`; `snapshots` service.

**Acceptance:** SPEC-0005 §8; donor grammar and G5.2 fixtures; same-tag collision on one file recovers from the newest snapshot or is refused when relocation fails; block and cross-file register edits through the public tool; CRLF, BOM, symlink, stale-revision and failed-commit cases; unseen lines never marked seen.

## H. Jobs, bash and scheme commands (SPEC-0008 §5–§7, G8.20)

- Supervisor with backend interface: foreground, background, promotion, stdin/EOF/signals, process groups, logs with cursors and limits, waits, notifications, cancel, crash/shutdown cleanup, ownership through branch navigation, secret env injection.
- `bash`, `jobs`, `job://`, `/workbench jobs`, `/wb:jobs` on one permission check.
- G8.20: a standalone `ls`, `cat`, `rm`, `mv`, `cp`, `mkdir` or `>` redirect on a registered non-locatable scheme runs in-process through the router with that provider's approval rules; every other command with such a path fails naming the supported forms. Slice 2's `secret://` `rm`/`mv` uses this path unchanged.

**Acceptance:** SPEC-0008 §8 local rows; noisy output stays bounded; cancel kills local descendants; reload terminates owned jobs (D8.a); URI and tool controls share policy; G8.20 dispatch for each verb on a core scheme, approval identical to the tool path, refusal of pipes, compound commands and substitutions, refusal under an `ssh://` cwd once W3 exists.

## I. Secrets (SPEC-0013 §2–§5, §7)

Detection (auth.json incl. `!command` per A2, environment, `.env*`, token shapes), placeholders, keys, vault; entry-time redaction, `context_with_system`, withhold via `ctx.Abort()`, substitution before every tool call incl. other extensions' tools, redaction order for appended blocks, agent-awareness section, env injection, `ask_user` type `secret`.

**Acceptance:** SPEC-0013 §11 rows outside §6; captured provider payloads contain no known secret across a read, a bash run and a job log; tool calls receive real values; another extension's appended block is redacted.

## J. System prompt and skills (SPEC-0014 §3–§5, §6.1–§6.2, §7, §8)

- gonja engine with confined loader, sections, helpers, conditionals, limits; `builtin/workbench` as the default, keeping PiG's sections with adapted content; structure check; delivery `auto`, fallback, epochs, `before_provider_request` splice.
- All six context frames: `session.date`, `session.environment`, `session.project`, `session.location`, `session.model` (incl. approved owner-scoped fields, SPEC-0014 §5.1), `session.capabilities`.
- Skills: catalog, listing budget ladder, `skill://` load/unload/loaded state, `when` and `auto_load`, declined after unload, nested skills, replay after compaction; `/workbench prompt`, `/workbench skills`, `session://self/system-prompt`.

**Acceptance:** SPEC-0014 §12 except its `select = "all"` and adapter rows: golden renders for every PR5 fixture, template chain and conditionals, stability within an epoch, frame rules, every skills case.

## K. Instructions and ambient context (SPEC-0007 §2–§8 local, SPEC-0012 §3.6, PLAN-0001 §6)

Class engine with AGENTS.md, BACKLOG.md and `projectPrivate` classes, ancestors, root+nearest, multi-touch union, delivery before mutation, pins, caps with source and delivered-view baselines, diff updates, local watchers including new candidates, dedup and withdrawal, compaction and branch reconstruction; `/workbench instructions` and `/workbench frames`.

**Acceptance:** SPEC-0007 §10 local rows; parallel codemode touches select the union; a capped BACKLOG change emits a reconstructible diff; changes during compaction recover with full views; no duplicate host injection.

## L1. Sessions (SPEC-0011 §2–§8, SPEC-0012 §5.3 core session trees)

Messaging probes from the ledger first. Endpoint registry, transport, authentication, envelope, wake and state tracking, delivery, loop guards, `agent://` send/read/wait, `sessions` channel, `/workbench sessions`; `session://self/`, `<id>`, `by-id/`; titles of the own and stored sessions after probes T1–T3.

**Acceptance:** SPEC-0011 §10 rows except broadcast and coordinator; two sessions discover each other, an idle receiver wakes, a busy one gets a steer; a third user cannot connect; ping-pong stops at `maxHops`; a raw secret typed into `/workbench sessions send` never reaches the receiver's provider.

## L2. Ask (SPEC-0009 §2–§3, §5)

`ask_user` with every question type, pending record and recovery, TUI questionnaire, RPC native dialogs with timeout, results per §6.

**Acceptance:** SPEC-0009 §10 rows outside the bridge, `/answer` and `/tldr`; an ask survives reload as pending; the §6 matrix holds in every mode.

## M. Commands and activity line

Every command from §2 with completions; activity line (SPEC-0009 §9) with builtin counts and contributed `statusSegment` items and the shortcut target. Click is a fork-host row (PLAN-0004 C4).

**Acceptance:** every command per §6; activity counts match registries.

# 5. Slice exit

1. `just check`, `just race`, package validation pass; reload and shutdown leak nothing.
2. Every PLAN-0001 §9 row assigned to Slice 1 passes its acceptance anchor, recorded in a progress table here with links to tests. A grep for unwired registrations, `unsupported` paths of advertised capabilities and TODO markers comes back empty.
3. A live agent session (user-authorized inference) uses every tool, the core schemes and `ask_user` end to end; transcript kept as evidence.
4. `builtin/workbench` is evaluated against PiG's own prompt on fixed tasks (golden transcripts, A/B, cache-hit rate) before it becomes the default (SPEC-0014 §12). Inference for this run approved by the user on 2026-11-03.
5. **pig-web co-load gate** (its PLAN-0001 Slice 5, real Workbench): `pig -e ../pig-workbench -e ../pig-web` renders `web_*` through Workbench; `read git://…:120-180` uses Workbench selectors; `edit git://…` is refused; a research notice arrives as a frame; `/workbench integrations` lists pig-web; `read https://…` is served by pig-web's fetch backend; the `web` status segment appears once; a stored search key is redacted in a `jobs` log. `cd` into a worktree waits for PLAN-0003 and is named in the gate report.
6. Handoff to pig-web naming the slice-exit commit and the depth still pending.

Completion follows PLAN-0001 §7: observed checks only; failures and skipped gates reported with impact.

# 6. Mode and host matrix

Every acceptance row states its mode and host. **Stock host** rows are required in Slices 1–2; **fork host** rows belong to PLAN-0004 C4; a row deferred on purpose names its slice.

| Surface | TUI | RPC | Print / JSON |
|---|---|---|---|
| Tools | Same behavior in every mode, direct and codemode | same | same |
| `ask_user` answered | `answers` | Native dialogs, same state machine | — |
| `ask_user` cancelled by the user | `cancelled: true`, normal result (D9.b) | same | — |
| `ask_user` dialog timeout | `timedOut` with reason, never a default answer | same; every dialog sets `DialogOptions.Timeout` (P28) | — |
| `ask_user` without UI | — | — | `interactionUnavailable` returned promptly unless an authorized bridge exists (bridge: PLAN-0003 W6) |
| Slash commands | Plain views | Notifications and native dialogs; no custom views | **Stock:** no output channel (P27). Script-facing state comes from tools and resources (`cfg://workbench/status`, `cfg://workbench/integrations/`, `session://self/instructions/`, `job://`). **Fork:** command output, C4 |
| `--list-tools` | PLAN-0003 W6 | PLAN-0003 W6 | Fork host, C4 |
| Activity line | Line and shortcut; click on fork host, C4 | Status text | — |
