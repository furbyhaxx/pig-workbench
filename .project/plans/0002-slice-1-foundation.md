---
document:
  type: "plan"
  id: "PLAN-0002"
  title: "Slice 1: complete foundation — every tool, seam, abstraction and core URI working"
  description: "All model-facing tools, all extension seams, all shared abstractions, the URI system with its core schemes and the system prompt, fully working on their final implementations; depth follows in PLAN-0003"
  status: "draft"
  revision: 68
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-11T02:45:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 68
      reason: "Slice exit state after PLAN-0005"
      changes: ["Exit state table and the row-by-row progress table with test links", "Progress: remediation done except the layout review and the handoff"]
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

- **done:** Phases A to M are implemented and all fourteen specifications are published. The review of 10.11.2026 found one critical and six high defects against this plan; [PLAN-0005](0005-slice-1-remediation.md) fixes them.
- **done:** PLAN-0005 code, documentation and gates; the exit state and the row table are in §5.
- **now:** the user's review of the new layouts (panels, tool rows, activity line, skills rows).
- **next:** the handoff to pig-web naming the exit commit, then deleting the consumed review.
- **Not part of this slice by decision:** `out://`, the instruction file watcher, stored-session `tree` and `messages`, `jobs monitor|write` (all PLAN-0003); `operation.status` is reserved (SPEC-0001).
- **Known limits at the exit:** Windows is compile- and partly wine-tested only; Workbench cannot be fused into a Piglet Binary on PiG 0.7.0; activity-item click needs the fork host (PLAN-0004 C4).

# 2. What is in Slice 1 and what is depth

Target scope by area, not a claim of current implementation; PLAN-0001 §9 is authoritative. See Progress and the acceptance inventory above for observed status.

| Area | Slice 1 (complete) | Depth in PLAN-0003 |
|---|---|---|
| Tools | `read`, `write`, `edit`, `grep`, `find`, `ls`, `bash`, `jobs`, `cd`, `ask_user`: final schemas, descriptions, result envelopes, direct and codemode | — |
| Edit | Full hashline grammar (sections, ranges, gaps, PUT/CUT, blocks, registers, REM/MV), seen guards, D5.b with G5.2 relocation validation, exact-text mode, revision-checked commits, previews | G5.1 payload repair, G5.3 leniency, G5.9 per-model rules, G5.10 streaming, model-assisted repair, notebooks |
| Read and search | Selectors, multi-read, paging, continuation, seen-state, code summaries and enclosing boundaries on the syntax service, supersession, repeated-read hints, directory paging, grep/find on the router | Adapters: JSON queries, SQLite, archives, PDF/documents, images/SVG, binary, video, profiles |
| URI system | Parser, selector grammar, router, pager, walking rules, guides; core schemes `file://`, `http(s)://` (fetch backends + builtin reader), `artifact://`, `local://`, `job://`, `session://` (`self`, `<id>`, `by-id/`, titles of own and stored sessions), `skill://`, `cfg://`, `docs://` (registered docs), `agent://`; external providers and fetch backends; bash scheme commands (G8.20) | `ssh://`, `out://` with a producer, `project://`, `conflict://`, `secret://`, `session://by-project/` and `by-name/`, cross-session search, titles of other live sessions, docs PiG source |
| Location and projects | Location abstraction with revisions and leases, local backend, `cd`; project identity and registry; symlinked session buckets with undo | SSH backend, `/remote`, worktrees and links, shared artifact tree, artifact storage modes, project conventions (SPEC-0012 §4.1–4.2) |
| Jobs | One supervisor: foreground, background, promotion, stdin/EOF/signals, logs with cursors, cancel, cleanup, env injection | PTY backend and key controls, remote jobs |
| Context | Frame ledger, presentation resolver, instruction class engine (AGENTS.md, BACKLOG.md, project private notes), root+nearest, pins, caps with both baselines, diff updates, change detection at boundaries, compaction and branch reconstruction | Remote instruction checks, external notification ingress, further classes and `select = "all"` |
| System prompt | gonja engine, `builtin/workbench` (default, SPEC-0014 §7), conditionals, delivery `auto`, epochs, late-section splice, all six context frames, skills complete (listing ladder, load/unload, `when`, `auto_load`, nested, compaction replay) | `select = "all"` (PLAN-0003 W5); model-family adapters of SPEC-0014 §6.3 with A/B evidence (PLAN-0004 C6) |
| Secrets | Detection, placeholders, vault, boundary hooks, substitution, env injection, prompt section, `ask_user` type `secret` | `secret://` and `/workbench secrets` |
| Interaction | `ask_user` complete in TUI and RPC, `interactionUnavailable` in print/JSON, pending recovery | Remote bridge, replay, answer history, `/answer`, `/tldr` |
| Sessions | Endpoint, transport, auth, wake, state, delivery, loop guards, `agent://` send/read/wait, `sessions` channel | Broadcast, coordinator with `/workbench coordinator` |
| Commands | `/workbench` with `show`, `settings`, `integrations`, `resources`, `prompt`, `skills`, `sessions`, `jobs`, `frames`, `instructions`, `trust`, `projects unlink-storage`; `/tools`, `/cd`, `/wb:cd`, `/wb:jobs`; completions (plain views, modes per §6) | `/remote`, `/answer*`, `/ask:*`, `/tldr`, `/workbench coordinator`, `projects artifacts-storage`, `--list-tools`; hub, `preview` and `statusline` are chrome (PLAN-0004) |

# 3. Phase A: ledger and contract freeze (M0.4)

## A0. Reconciled ledger

Before freezing, reconcile every open decision, outstanding probe, dependency approval and known spec contradiction into one table in this section, each with the phase it blocks. Seed (to be completed by checking every spec's open items):

| Item | Kind | Status and owner |
|---|---|---|
| Dependency policy and TOML/locking selection | resolved by user standing approval, checks performed 03.11.2026 | **Done.** `go list -m -json <module>@latest`: `creachadair/tomledit` v0.0.29 and `BurntSushi/toml` v1.6.0 (MIT), `gofrs/flock` v0.13.1 (BSD-3-Clause) and its `golang.org/x/sys` dependency at v0.49.0 (BSD-3-Clause). Upstream metadata and current READMEs show maintained, non-archived projects with no deprecation notice; flock succeeds theckman/go-flock. Licenses: `THIRD_PARTY_NOTICES.md`. Latest/maintenance checks are required for future additions, not repeated approval questions. |
| pig-web M1: extension-owned inference | decision resolved by user | **Done.** C/I: opt-in `secrets.redact` for trusted owners; unprotected child sessions explicit; SPEC-0013 §4.5 |
| pig-web M2: contributed model fields | decision resolved by user | **Done.** C/F/J: owner-scoped `contextFields` on `frameProducer`, `context.fields.set`; SPEC-0014 §5.1 |
| pig-web M3: descriptor summary | approved with SPEC-0004 | **Done.** D/J: required one-line summary, 1–120 Unicode characters; SPEC-0004 §2 |
| pig-web M4: native-provider credential expansion | probed 05.11.2026 | **Probe done, one gap.** `models.json` literal, `$ENV` and leading `!command` all resolve to the wire value (`TestProbeNativeCredentialExpansion`). `RegisterNativeProvider` with a custom resolver is host-internal and unreachable from an extension: recorded as a detection coverage gap in SPEC-0013 §2, not as covered. Owner: Phase I (11.1). |
| pig-web M5: trailing annotations | probed 05.11.2026 | **Probe done, no contract change.** An intact block grants seen with the annotation block before or after it; an altered block grants nothing (`TestAnnotationOrderCannotChangeSeenVisibility`). The annotation producer still redacts appended text before returning (Phase I). SPEC-0005 §10, SPEC-0013 §4.2. |
| pig-web M6: activity items and URI open | existing contract confirmed | **Done.** M: SPEC-0009 §9 includes placement, glyph, command or URI open, count and severity; click remains PLAN-0004 C4 |
| Messaging probes: wake from a goroutine, idle/busy state, reload rebind (SPEC-0011 §10) | probed 05.11.2026 | **Wake and state done; reload rebind done incidentally.** Idle `followUp`+`TriggerTurn` from a retained context starts a turn; a busy session receives it at the next boundary; `ctx.IsIdle` tracks a streaming and an aborted turn (`TestProbeIdleWakeStartsTurn`, `TestProbeBusyWakeSteers`, `TestProbeIdleStateTransitions`). Reload rebind is evidenced by the reference-producer reload acceptance (fresh clients, fresh subscriptions). Owner: L1. |
| Session title probes T1–T3 (SPEC-0012 §6) | probed 05.11.2026 | **Done.** SetSessionName is visible immediately and an empty name clears; the title persists as a `session_info` entry and survives `--continue`; `session_info` is branch-scoped, so a fork before a rename leaves no title. Owner: L1 (write-back into SPEC-0012 §6 at the freeze). |
| Resumed-session cwd under `--continue` with symlinked buckets (SPEC-0012 §3.5) | probed 05.11.2026 | **Done.** `ctx.Cwd()` after `--continue` in another checkout is the session's recorded cwd, not the process cwd (`TestProbeResumedCwdAcrossSymlinkedBuckets`). Owner: E2. |
| G5.10 abort while arguments stream | probe | **Open**, owner PLAN-0003 W1 (not Slice 1). |
| Widget view events for activity-line click | probe + fork | **Open**, owner PLAN-0004 C4. |
| Archive additions, backend evaluation §10 item 6 | dependency approval | **Open**, owner PLAN-0003 W2. |
| D9.c answer history (provisional) | decision | **Open**, owner PLAN-0003 W6. |
| SPEC-0012 §4.1 `tasks` line in the bundled skill | decision | **Open**, owner PLAN-0003 W7. |
| SPEC-0005 §3 vs D5.b | contradiction | **Fixed** in SPEC-0005 rev 9. |
| PLAN-0001 WB-25 still naming `builtin/pig` | contradiction | **Fixed** in PLAN-0001 rev 23. |
| Slice 1 inventory of every ownership row | analysis | **Done.** ANA-2026-11-05-slice-1-inventory classifies each PLAN-0001 §9 Slice 1 row; no row is counted done without observed evidence. |
| Phase A probe evidence | analysis | **Done.** Runnable cases in `internal/host/live_probes_test.go`; results are stated in SPEC-0011 §10 and SPEC-0012 §3.5 and §5.3.1. |

## A1. Freeze

Freeze everything that fixes data models, persisted records, wire formats, model-facing shapes or safety boundaries, for the **whole** scope, so depth never forces a contract change:

- **Wire:** SPEC-0001 envelope, `discover` capabilities, registration kinds and descriptor fields, events, generations, error codes, limits. Go authoring API as compile tests against the staged SDK (SPEC-0001 §6).
- **Model-facing:** the ten tools' schemas, descriptions and result envelopes; selector grammar and precedence; URI grammar and reserved schemes (SPEC-0004, SPEC-0012 §5); G8.20 command forms.
- **Persisted:** config keys and files (SPEC-0010), frame ledger and delivery records (SPEC-0002 §4), snapshots and seen-state (SPEC-0005 §3), jobs (SPEC-0008), project registry, `Project.toml` and bucket migration state (SPEC-0012 §3), vault (SPEC-0013 §3), prompt epochs (SPEC-0014 §4.3), loaded skills, pending asks; branch vs session ownership and recovery after reload.
- **Behavioral:** root+nearest, pin and cap semantics (PLAN-0001 §6), command routes, the mode and host matrix (§6).

Each frozen specification gets a revision with `status: "published"` after user review. Later changes need a new revision and a migration note.

**Approval state (07.11.2026).** The user approved the remaining five specifications: SPEC-0003 rev 8, SPEC-0006 rev 8, SPEC-0007 rev 7, SPEC-0008 rev 8 and SPEC-0011 rev 8. The previously approved specifications remain SPEC-0001 rev 8, SPEC-0002 rev 8, SPEC-0004 rev 7, SPEC-0005 rev 11, SPEC-0009 rev 11, SPEC-0010 rev 14, SPEC-0012 rev 8, SPEC-0013 rev 6 and SPEC-0014 rev 8, including the required 1–120 character `resourceProvider.summary` (enforced in `resource.ProviderDescriptor.Validate`). Approval freezes the contracts, not the incomplete implementation or Slice 1 acceptance.

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

Class engine with AGENTS.md, BACKLOG.md and `projectPrivate` classes, ancestors, root+nearest, multi-touch union, delivery before mutation, pins, caps with source and delivered-view baselines, diff updates, change detection at boundaries, dedup and withdrawal, compaction and branch reconstruction; `/workbench instructions` and `/workbench frames`.

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
4. `builtin/workbench` is evaluated against PiG's own prompt on fixed tasks (golden transcripts, A/B, cache-hit rate) before it becomes the default (SPEC-0014 §12). Fresh user authorization for provider spend is required before this run.
5. **pig-web seam gate with a reference extension:** co-load Workbench with `internal/host/testdata/reference-producer/`, shaped like pig-web. It renders `web_*` through Workbench; `read git://…:120-180` uses Workbench selectors; `edit git://…` is refused before provider dispatch; a research notice arrives as a frame; `/workbench integrations` lists the reference producer; `read https://…` uses its fetch backend; its `web` status segment appears once; a stored search key is redacted in a `jobs` log. Each seam needs observed evidence; the current `kb://` fixture alone does not close the `git://`, integrations or job-log rows. `cd` into a worktree waits for PLAN-0003 and is named in the gate report. Actual pig-web availability is not this gate's prerequisite.
6. Handoff to pig-web naming the slice-exit commit, reference-gate evidence and depth still pending. The real `pig -e ../pig-workbench -e ../pig-web` co-load run belongs to pig-web's own gate (its PLAN-0001 Slice 5), against that handed-off commit; Workbench does not claim that downstream run passed.

Completion follows PLAN-0001 §7: observed checks only; failures and skipped gates reported with impact.

## Exit state

| # | Exit item | State |
|---|---|---|
| 1 | Gates, no leaks | **Pass** on the tree of `6227b03`: `just check`, `just race`, `just integration` exit 0 ([evidence](evidence/0005-exit-gates/)) |
| 2 | Every Slice 1 row passes, no stubs | **Pass**, table below. No TODO marker in product code; every remaining `unsupported` answer is for a capability or operation that is not advertised; every shown setting has a consumer (`TestEveryShownSettingHasAConsumer`) |
| 3 | Live agent session | **Done 08.11.2026** on deepseek/deepseek-flash, before the remediation; not repeated |
| 4 | Prompt A/B | **Done 08.11.2026**, 79/80 per arm, 23% fewer input tokens; `prompt.delivery = auto` resolves to `force` |
| 5 | Seam gate with the reference producer | **Pass** (`TestLiveReferenceProducerSeams`, `TestLiveStoredSearchKeyIsRedactedInJobLogs`, `TestLiveIntegrationsAndResourcesCommandsListTheContributions`) |
| 6 | Handoff to pig-web | **Open**: waits for the user's layout review |

Each row's test names, scope and limits are in the [acceptance inventory](../analyses/2026-11-05-slice-1-inventory/README.md).

| Spec § | Requirement | Result | Tests |
|---|---|---|---|
| 0001 §2–§7 | Discovery, request/reply, registrations, events, Go API | pass | [live_services_test.go](../../internal/host/live_services_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [client_test.go](../../api/client_test.go), [services_test.go](../../internal/host/services_test.go), [live_presentation_test.go](../../internal/host/live_presentation_test.go), [version_test.go](../../internal/host/version_test.go) |
| 0001 §8 | Collaboration services | pass | [live_reference_test.go](../../internal/host/live_reference_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [live_sessions_test.go](../../internal/host/live_sessions_test.go), [secrets_test.go](../../internal/host/secrets_test.go) |
| 0002 all | Frames, ledger, delivery, trust | pass | [live_frames_test.go](../../internal/host/live_frames_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [authority_test.go](../../internal/frames/authority_test.go), [frame_baseline_test.go](../../internal/host/frame_baseline_test.go), [delivery_test.go](../../internal/frames/delivery_test.go), [frame_restore_test.go](../../internal/host/frame_restore_test.go), [live_frame_rows_test.go](../../internal/host/live_frame_rows_test.go) |
| 0003 §2–§6, §10 | Presentation contract, resolver, path display | pass | [live_reference_test.go](../../internal/host/live_reference_test.go), [presentation_web_test.go](../../internal/host/presentation_web_test.go), [commands_test.go](../../internal/host/commands_test.go), [presentation_builtin_test.go](../../internal/host/presentation_builtin_test.go), [presentation_builtin_render_test.go](../../internal/host/presentation_builtin_render_test.go) |
| 0004 §1–§10 | Router, grammar, policy, builtin routes | pass | [live_reference_test.go](../../internal/host/live_reference_test.go), [live_test.go](../../internal/host/live_test.go), [router_test.go](../../internal/resource/router_test.go), [provider_mutation_test.go](../../internal/host/provider_mutation_test.go), [live_sessiontree_test.go](../../internal/host/live_sessiontree_test.go), [live_sessions_test.go](../../internal/host/live_sessions_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [path_test.go](../../internal/resource/path_test.go) |
| 0004 §11 | Fetch backends and web reader | pass | [live_http_test.go](../../internal/host/live_http_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [http_test.go](../../internal/resource/http_test.go) |
| 0005 §2–§8, §10, G5.2 | Hashline grammar, snapshots, recovery, commit | pass | [live_edit_test.go](../../internal/host/live_edit_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [parser_test.go](../../internal/hashline/parser_test.go), [batch_test.go](../../internal/hashline/batch_test.go), [commit_test.go](../../internal/fileedit/commit_test.go), [lock_test.go](../../internal/fileedit/lock_test.go) |
| 0006 §2–§4, §6–§7 | Read pipeline, summaries, search | pass | [live_test.go](../../internal/host/live_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [search_test.go](../../internal/search/search_test.go), [live_http_test.go](../../internal/host/live_http_test.go), [live_reads_test.go](../../internal/host/live_reads_test.go) |
| 0007 §2–§8 local | Instruction classes, barrier, pins, caps | pass | [live_instructions_test.go](../../internal/host/live_instructions_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [instructions_test.go](../../internal/instructions/instructions_test.go) |
| 0008 §2–§3 local | Location capture, `cd` | pass | [live_test.go](../../internal/host/live_test.go), [location_test.go](../../internal/location/location_test.go) |
| 0008 §5–§7, G8.20 | Bash, jobs, `job://`, scheme commands | pass | [live_jobs_test.go](../../internal/host/live_jobs_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [process_unix_test.go](../../internal/jobs/process_unix_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [live_menu_test.go](../../internal/host/live_menu_test.go) |
| 0009 §2–§3, §5 | `ask_user` | pass | [live_ask_test.go](../../internal/host/live_ask_test.go) |
| 0009 §8, Slice 1 routes | Command routes | pass | [live_sessions_test.go](../../internal/host/live_sessions_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [live_instructions_test.go](../../internal/host/live_instructions_test.go), [commands_test.go](../../internal/host/commands_test.go), [live_commands_test.go](../../internal/host/live_commands_test.go), [live_settings_test.go](../../internal/host/live_settings_test.go), [live_frame_rows_test.go](../../internal/host/live_frame_rows_test.go), [panel_keys_test.go](../../internal/host/panel_keys_test.go) |
| 0009 §9, activity line | Activity line | pass | [live_sessions_test.go](../../internal/host/live_sessions_test.go), [activity_test.go](../../internal/host/activity_test.go) |
| 0010 all | Config, state, trust | pass | [live_settings_test.go](../../internal/host/live_settings_test.go), [manager_test.go](../../internal/config/manager_test.go), [manager_test.go](../../internal/jobs/manager_test.go), [shown_test.go](../../internal/config/shown_test.go), [reference_test.go](../../internal/config/reference_test.go) |
| 0011 §2–§8 | Session endpoint and messaging | pass | [live_sessions_test.go](../../internal/host/live_sessions_test.go), [lifecycle_publish_test.go](../../internal/host/lifecycle_publish_test.go), [registry_test.go](../../internal/endpoint/registry_test.go) |
| 0012 §3.1–§3.4 | Project identity and registry | pass | [live_test.go](../../internal/host/live_test.go), [project_identity_test.go](../../internal/host/project_identity_test.go), [detect_test.go](../../internal/project/detect_test.go), [registry_test.go](../../internal/project/registry_test.go) |
| 0012 §3.5 | Shared session storage | pass | [live_storage_test.go](../../internal/host/live_storage_test.go), [crash_test.go](../../internal/sessionstore/crash_test.go), [store_test.go](../../internal/sessionstore/store_test.go) |
| 0012 §3.6 | Private notes | pass | [live_instructions_test.go](../../internal/host/live_instructions_test.go), [instructions_test.go](../../internal/instructions/instructions_test.go), [registry_test.go](../../internal/project/registry_test.go) |
| 0012 §5.1–§5.2, §5.5 | Naming, tool semantics, guides | pass | [live_reference_test.go](../../internal/host/live_reference_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [contributions_test.go](../../internal/host/contributions_test.go), [router_test.go](../../internal/resource/router_test.go) |
| 0012 §5.3 core | Core scheme trees | pass | [live_sessiontree_test.go](../../internal/host/live_sessiontree_test.go), [live_sessions_test.go](../../internal/host/live_sessions_test.go), [live_jobs_test.go](../../internal/host/live_jobs_test.go), [skill_test.go](../../internal/host/skill_test.go), [services_test.go](../../internal/host/services_test.go), [live_scripted_test.go](../../internal/host/live_scripted_test.go), [sessions_settings_test.go](../../internal/host/sessions_settings_test.go) |
| 0012 §5.4, registered docs | `docs://` registered docs | pass | [live_reference_test.go](../../internal/host/live_reference_test.go) |
| 0013 §2–§5, §7 | Secrets | pass | [live_secrets_test.go](../../internal/host/live_secrets_test.go), [live_reference_test.go](../../internal/host/live_reference_test.go), [secrets_test.go](../../internal/host/secrets_test.go), [vault_test.go](../../internal/secrets/vault_test.go) |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | Prompt engine, frames, skills | pass | [live_scripted_test.go](../../internal/host/live_scripted_test.go), [live_settings_test.go](../../internal/host/live_settings_test.go), [golden_test.go](../../internal/prompt/golden_test.go), [builtin_test.go](../../internal/prompt/builtin_test.go), [prompt_test.go](../../internal/host/prompt_test.go), [catalog_test.go](../../internal/skills/catalog_test.go), [live_frames_test.go](../../internal/host/live_frames_test.go) |

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
