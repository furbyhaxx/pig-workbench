---
document:
  type: "plan"
  id: "PLAN-0002"
  title: "Slice 1: complete foundation — every tool, seam, abstraction and core URI working"
  description: "All model-facing tools, all extension seams, all shared abstractions, the URI system with its core schemes and the system prompt, fully working on their final implementations; depth follows in PLAN-0003"
  status: "draft"
  revision: 2
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T14:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed PLAN-0001 M0 remainder (M0.1 decisions, M0.4 freeze) and M1", "Pulled the foundation of every later milestone forward so each public seam runs", "Defined the pig-web co-load gate as the slice exit"]
    - revision: 2
      reason: "User defined foundation: everything fully working on its final implementation, only depth deferred; decided D5.b and D5.d"
      changes: ["Foundation rule rewritten: no stubs, no unwired parts, no interim implementations", "All ten tools, all commands of founded features and the core URI set in this slice", "D5.b adopted, D5.d rejected (SPEC-0005 rev 8)"]
---

# 1. Outcome

At the end of this slice Workbench loads in PiG and **everything exists and works fully**: every model-facing tool, every extension seam, every shared abstraction, the URI system with its core schemes, the system prompt and the commands of all founded features. pig-web can run its real-Workbench gate (its PLAN-0001 Slice 5) and close its design and planning against running services.

**What "foundation" means here (user, 2026-11-03):**

- Built on the **final implementation**. Nothing is built differently now and rebuilt later. Slice 2 adds depth to the same code; it never replaces a Slice 1 engine.
- **Fully working and wired.** No stubs, no placeholder results, no `unsupported` answers for operations a founded feature advertises, no registered-but-unused code. If a capability is not in this slice, it is not advertised yet.
- **Depth is what waits:** additional schemes and trees, format adapters, further backends (SSH, PTY), donor-parity heuristics, secondary flows. Each is listed in [PLAN-0003](0003-slice-2-feature-complete.md).
- **Chrome waits:** plain renderers that follow the SPEC-0003 data contract; kit panels, designed layouts, captures and polish are [PLAN-0004](0004-slice-3-completion.md).

Umbrella and scope: [PLAN-0001](0001-pig-workbench.md) (WB rows, decisions, verification gates).

## Progress

- **done:** M0.2 host probes (P1–P30, S1–S6, PR1–PR8), M0.3 backend matrix, M0.1 test-level audit, 14 draft specifications, bootstrap factory. D5.b adopted and D5.d rejected (SPEC-0005 rev 8).
- **now:** Phase A (contract freeze), including the pig-web handoff [HO-pig-web-m0-4-inputs](../handoff/pig-web-m0-4-inputs.md).
- **next:** Phases B–M.
- **parallel:** personal PiG fork (D13). Nothing in this slice depends on it; Workbench detects fork capabilities by flag.

# 2. What is in Slice 1 and what is depth

| Area | Slice 1 (complete) | Depth in PLAN-0003 |
|---|---|---|
| Tools | `read`, `write`, `edit`, `grep`, `find`, `ls`, `bash`, `jobs`, `cd`, `ask_user`: final schemas, descriptions, result envelopes, direct and codemode | — |
| Edit | Full hashline grammar (sections, ranges, gaps, PUT/CUT, blocks, registers, REM/MV), seen guards, D5.b recovery, exact-text mode, revision-checked commits, previews | Donor parity heuristics G5.1–G5.3 (payload repair, recovery ladder, leniency), streaming previews (G5.10), per-model rules (G5.9), model-assisted repair, notebooks |
| Read and search | Selectors, multi-read, paging, continuation, seen-state, code summaries and enclosing boundaries on the syntax service, supersession, repeated-read hints, directory paging, grep/find on the router | Adapters: JSON queries, SQLite, archives, PDF/documents, images/SVG, binary, video, profiles |
| URI system | Parser, selector grammar, router, pager, walking rules, guides; core schemes `file://`, `http(s)://` (fetch backends + builtin reader), `artifact://`, `out://`, `local://`, `job://`, `session://` (`self`, `<id>`, `by-id/`), `skill://`, `cfg://`, `docs://`, `agent://`; external providers and fetch backends | `ssh://`, `project://`, `conflict://`, `secret://`, `session://by-project/` and `by-name/`, cross-session search, docs PiG source checkout |
| Location | Location abstraction with revisions and leases, local backend, `cd`, project identity and registry, session buckets | SSH backend, `/remote`, worktrees and links, shared artifact tree |
| Jobs | One supervisor: foreground, background, promotion, stdin/EOF/signals, logs with cursors, cancel, cleanup, env injection | PTY backend and key controls, remote jobs |
| Context | Frame ledger, presentation resolver, instruction class engine (AGENTS.md, BACKLOG.md), root+nearest, pins, caps with both baselines, diff updates, local watching, compaction and branch reconstruction | Remote watching, external notification ingress, further classes and `select = "all"` |
| System prompt | gonja engine, `builtin/pig` structure, delivery `auto`, epochs, late-section splice, context frames, skill listing and load/unload | `builtin/workbench`, model-family adapters beyond the probed set |
| Secrets | Detection, placeholders, vault, boundary hooks, substitution, prompt section, `ask_user` type `secret` | `secret://`, `/workbench secrets` management |
| Interaction | `ask_user` complete in TUI, RPC and print; pending recovery | Remote bridge, replay, answer history, `/answer`, `/tldr` |
| Sessions | Endpoint, transport, auth, delivery, loop guards, `agent://` send/read, `sessions` channel | Broadcast, coordinator |
| Commands | `/workbench` with `show`, `settings`, `integrations`, `resources`, `prompt`, `jobs`, `frames`, `instructions`, `trust`; `/tools`, `/cd`, `/wb:cd`, `/wb:jobs`; completions, machine-readable output (plain views) | `/remote`, `/answer*`, `/ask:*`, `/tldr`, `--list-tools` |

# 3. Phase A: contract freeze (M0.4)

## A1. Freeze

Freeze everything that fixes data models, persisted records, wire formats, model-facing shapes or safety boundaries — for the **whole** scope, not just this slice, so depth never forces a contract change:

- **Wire:** SPEC-0001 envelope, `discover` capabilities, registration kinds and descriptor fields, events, generations, error codes, limits. Go authoring API as compile tests against the staged SDK (SPEC-0001 §6).
- **Model-facing:** the ten tools' schemas, descriptions and result envelopes; selector grammar and precedence; URI grammar and reserved schemes (SPEC-0004, SPEC-0012 §5).
- **Persisted:** config keys and files (SPEC-0010), frame ledger and delivery records (SPEC-0002 §4), snapshots and seen-state (SPEC-0005 §3), jobs (SPEC-0008), project registry and `Project.toml` (SPEC-0012 §3), vault (SPEC-0013 §3), prompt epochs (SPEC-0014 §4.3), loaded skills, pending asks; branch vs session ownership and recovery after reload.
- **Behavioral:** root+nearest, pin and cap semantics (PLAN-0001 §6), command routes.

Each frozen specification gets a revision with `status: "approved"` after user review. Later changes need a new revision and a migration note.

## A2. Answer pig-web's M0.4 inputs

Answer M1–M6 from [HO-pig-web-m0-4-inputs](../handoff/pig-web-m0-4-inputs.md) inside the freeze: extension inference and secrets (incl. research child sessions), `knowledge_cutoff` in `session.model`, the descriptor `summary` field, `!command` expansion by registration kind (qualify SPEC-0013 S6), trailing tool-result blocks and redaction order, activity item fields and URI `open`. Choices that need the user are asked before the freeze commit. Reply with a handoff into pig-web naming the freeze commit; archive the inbound handoff.

**Phase A exit:** frozen specifications committed; no open decision that changes a data model or safety boundary; pig-web reply delivered.

# 4. Phases B–M

Order: B first; C, D, E in parallel; then F–L as their dependencies land; M last. Only the integration owner edits registration and shared types (PLAN-0001 §8).

## B. Factory, configuration and host adapter (PLAN-0001 M1.1, M1.2)

- Thin `Extension()`, lifecycle owner with shutdown joins, generation counter that survives same-process reload (P13/P14).
- Layered TOML (`Workbench.toml`, `Secrets.toml`), round-trip-safe saves, trust restrictions, source attribution, inter-session locking, atomic private writes (SPEC-0010 §1–§8).
- Host adapter: SDK contexts, hooks, original-builtin delegation, persistence; domain packages stay host-independent.
- Sanitization, width fitting, path display with donor golden cases (SPEC-0003 §10), result and metrics types.
- Test harness: isolated agent dir, fixture projects, RPC and print drivers; `just check`, `just race`, `pig install . --validate-only --json`.

**Acceptance:** malformed config refused with location; unknown keys preserved; concurrent save from two sessions; untrusted project cannot widen trust; reload leaves no goroutines or temp files.

## C. Extension services v1 (SPEC-0001)

- `discover` with every capability: `frames`, `presentation`, `resources`, `location`, `status`, `snapshots`, `fetch`, `sessions`, `docs`.
- `register`/`unregister`/`cancel`, idempotent registration, owner scoping, generations, late-reply discard, bounded waits, no emit under a held mutex.
- Every registration kind consumed by its subsystem: `toolPresentation`, `frameProducer`, `resourceProvider` (incl. `guide`, `summary`), `fetchBackend`, `statusSegment`, `docs`.
- Reference producer extension kept as an integration fixture, plus a small documented example.

**Acceptance:** absent, incompatible and ambiguous Workbench cases; stale generation rejected; reload re-registers cleanly; listener timeouts never block a tool call.

## D. URI system, read and search (SPEC-0004, SPEC-0006 §2–§4, SPEC-0012 §5)

- Parser with literal-first disambiguation; selector grammar in one normalized form; `paths:[]` multi-read in input order within one budget.
- Pager with shown spans, continuation, selection- vs resource-complete, artifact spill without recursion.
- Router with capability-declared providers, walking rules, first-touch guides; external providers through every tool; `resources.changed` invalidation.
- Syntax service on the approved tree-sitter backend: code summaries and enclosing boundaries for read, block spans for edit (Phase G).
- `http(s)://`: fetch-backend routing with policy, redirect safety, paging and locate; builtin static reader (readable and raw views on the approved HTML backend) when no backend serves the URL.
- Core schemes from §2; `read`, `ls`, `find`, `grep` overrides complete.

**Acceptance:** direct and codemode calls resolve identically; colon/comma/space filenames stay addressable; cancel joins workers; sessions cannot reach each other's scratch; an external `build://` provider pages through `read` with Workbench selectors and an unadvertised `edit` is refused before dispatch.

## E. Location and projects (SPEC-0008 locations, SPEC-0012 §3)

- Location abstraction with backend interface, revisions and leases; local backend; `cd` and `/cd`; `location` service and `location.changed`.
- `Project.toml`, recognition (§3.3), registry with `prj-` IDs, `global`, project in provider requests (SPEC-0004 §4), session buckets (§3.5).

**Acceptance:** host cwd never changes; relative paths resolve against the captured location; nested project directories resolve to the parent project's ID.

## F. Frames and presentation (SPEC-0002, SPEC-0003)

- Frame ledger with branch generation guards, receipts, replay on resume, compaction, trust classes, `beforePrompt` delivery.
- Presentation resolver reading the live registry at first use, all presets with typed cells including `uri`, plain renderers for every lifecycle state, contributed display settings, codemode nested capture and touch metadata.

**Acceptance:** no duplicate frame after resume or branch switch; an untrusted producer cannot raise trust; a pig-web-style `web_*` tool renders through the resolver in TUI, RPC and print.

## G. Edit, write and snapshots (SPEC-0005 §2–§8, §10)

- Snapshot store: canonical identity, revisions over exact bytes, retained spans, seen-state from fully emitted lines, tag binding with D5.b collision recovery.
- Full hashline grammar and staging, syntax blocks via Phase D's service, registers, REM/MV, seen guards, mismatch reveals, no-op guards, post-edit results; exact-text mode.
- Commit path: sorted multi-resource locks, revision check before commit, atomic replace with mode preservation, exact outcome reporting; `write` with `expectedRevision`; `snapshots` service.

**Acceptance:** donor grammar fixtures; block and cross-file register edits through the public tool; CRLF, BOM, symlink, stale-revision and failed-commit cases; unseen lines never marked seen.

## H. Jobs and bash (SPEC-0008 jobs)

- Supervisor with backend interface: foreground, background, promotion, stdin/EOF/signals, process groups, logs with cursors and limits, waits, notifications, cancel, crash/shutdown cleanup, ownership through branch navigation, secret env injection.
- `bash`, `jobs`, `job://`, `/workbench jobs`, `/wb:jobs` on one permission check.

**Acceptance:** noisy output stays bounded; cancel kills local descendants; reload terminates owned jobs (D8.a); URI and tool controls share policy.

## I. Secrets (SPEC-0013 §2–§5, §7)

Detection (auth.json incl. `!command` per A2, environment, `.env*`, token shapes), placeholders, keys, vault; entry-time redaction, `context_with_system`, withhold via `ctx.Abort()`, substitution before every tool call incl. other extensions' tools, redaction order for appended blocks, agent-awareness section, `ask_user` type `secret`.

**Acceptance:** captured provider payloads contain no known secret across a read, a bash run and a job log; tool calls receive real values; another extension's appended block is redacted.

## J. System prompt and skills (SPEC-0014 §3–§5, §8)

gonja engine with confined loader, sections, helpers, limits; `builtin/pig` structure; delivery `auto`, structure check, fallback, epochs, `before_provider_request` splice; context frames `session.date`, `session.location`, `session.model` (incl. contributed fields per A2); `/workbench prompt`, `session://self/system-prompt`; skill listing and `skill://` load/unload/loaded state.

**Acceptance:** golden renders for every PR5 fixture; prompt bytes stable within an epoch; mid-epoch changes arrive as frames; loaded skills survive reload and forks.

## K. Instructions and ambient context (SPEC-0007, PLAN-0001 §6)

Class engine with AGENTS.md and BACKLOG.md classes, ancestors, root+nearest, multi-touch union, delivery before mutation, pins, caps with source and delivered-view baselines, diff updates, local watchers including new candidates, dedup and withdrawal, compaction and branch reconstruction; `/workbench instructions` and `/workbench frames`.

**Acceptance:** parallel codemode touches select the union; a capped BACKLOG change emits a reconstructible diff; changes during compaction recover with full views; no duplicate host injection.

## L. Sessions and ask (SPEC-0011 §2–§8, SPEC-0009 ask)

- Endpoint registry, transport, authentication, envelope, delivery into live sessions with wake, loop guards, `agent://` send/read, `sessions` channel; `session://self/`, `<id>`, `by-id/`.
- `ask_user` with every question type, pending record and recovery, host dialogs in TUI and RPC, headless cancellation, `cancelled` per D9.b.

**Acceptance:** two live sessions exchange a message and the receiver wakes; a loop is cut by the guard; an ask survives reload as pending; print mode never blocks on a dropped dialog.

## M. Commands and activity line

Every command from §2 with completions and machine-readable output; activity line (SPEC-0009 §9) with builtin counts and contributed `statusSegment` items, shortcut target; click waits for the fork.

**Acceptance:** every command works in TUI, RPC and print; activity counts match registries.

# 5. Slice exit

1. `just check`, `just race`, package validation pass; reload and shutdown leak nothing.
2. Every row in §2's Slice 1 column passes, recorded in a progress table here with links to tests. A grep for unwired registrations, `unsupported` paths of advertised capabilities and TODO markers comes back empty.
3. A live agent session (user-authorized inference) uses every tool, the core schemes and `ask_user` end to end; transcript kept as evidence.
4. **pig-web co-load gate** (its PLAN-0001 Slice 5, real Workbench): `pig -e ../pig-workbench -e ../pig-web` renders `web_*` through Workbench; `read git://…:120-180` uses Workbench selectors; `edit git://…` is refused; a research notice arrives as a frame; `/workbench integrations` lists pig-web; `read https://…` is served by pig-web's fetch backend; the `web` status segment appears once; a stored search key is redacted in a `jobs` log. `cd` into a worktree waits for PLAN-0003 and is named in the gate report.
5. Handoff to pig-web naming the slice-exit commit and the depth still pending.

Completion follows PLAN-0001 §7: observed checks only; failures and skipped gates reported with impact.
