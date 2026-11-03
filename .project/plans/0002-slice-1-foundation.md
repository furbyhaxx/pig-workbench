---
document:
  type: "plan"
  id: "PLAN-0002"
  title: "Slice 1: integrated foundation and frozen extension seams"
  description: "Every Workbench subsystem built with its real data model, persistence, public contract and one working path, so pig-web can finish its design and planning against running services"
  status: "draft"
  revision: 1
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T13:53:58+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed PLAN-0001 M0 remainder (M0.1 decisions, M0.4 freeze) and M1", "Pulled the foundation of every later milestone forward so each public seam runs", "Defined the pig-web co-load gate as the slice exit"]
---

# 1. Outcome

At the end of this slice Workbench loads in PiG and **every subsystem exists** with its real data model, persistence, public contract and at least one end-to-end working path. Every public extension seam (SPEC-0001 §8 and the registration kinds) is served by real code, not a stand-in. pig-web can run its real-Workbench gate (its PLAN-0001 Slice 5) and freeze its own design against behavior instead of drafts.

Not in this slice: breadth (every adapter, every hashline form, every scheme tree, SSH), and all chrome: kit panels, captures, density tuning, animation, the M0.5 design review. Output uses plain renderers that follow the SPEC-0003 data contract, so later chrome replaces drawing, not data.

**Rule for "foundation":** a subsystem counts as founded when its records, persistence, lifecycle, policy checks and public contract are final and one real path through it works and is tested. A stub that returns `unsupported` for everything is not a foundation.

Umbrella and scope: [PLAN-0001](0001-pig-workbench.md) (WB rows, decisions, verification gates). Next: [PLAN-0003](0003-slice-2-feature-complete.md). Last: [PLAN-0004](0004-slice-3-completion.md).

## Progress

- **done:** M0.2 host probes (P1–P30, S1–S6, PR1–PR8), M0.3 backend matrix, M0.1 test-level audit, 14 draft specifications, bootstrap factory.
- **now:** Phase A (decisions and contract freeze), including the pig-web handoff [HO-pig-web-m0-4-inputs](../handoff/pig-web-m0-4-inputs.md).
- **next:** Phase B onward.
- **parallel:** personal PiG fork (D13). Nothing in this slice depends on it; Workbench detects fork capabilities by flag.

# 2. Phases and dependencies

```text
A decisions + contract freeze (M0.1, M0.4, pig-web M1–M6)
|
B factory, config/state/trust, host adapter, test harness
|
C bus v1 + registrations ---- D resources/selectors/pager ---- E location + project identity
|                              |                                |
F frames + presentation        G edit/snapshot core             H jobs core + bash
|                              |                                |
I secrets boundary ----------- J prompts + instruction baseline  K catalogs, sessions, ask core
|
L commands (plain), activity line, codemode capture
|
Exit: pig-web co-load gate, handoff with freeze commit
```

C, D and E can run in parallel once B lands. Only the integration owner edits registration and shared types (PLAN-0001 §8).

# 3. Phase A: decisions and contract freeze

## A1. Close the open M0.1 decisions

| ID | Question | State |
|---|---|---|
| D5.b | Tag collision within one canonical resource: recover from the newest snapshot (donor) or reject | Recommendation in audit §4; needs the user's answer |
| D5.d | Separate `apply_patch` edit mode for GPT-family models | Benefits were discussed; no recorded decision |
| D6.a | SQLite row writes | Decided: allowed (SPEC-0006 G6.16) |
| D6.b | `ast_grep`/`ast_edit` | Decided: after v1, WB-21 |
| D9.b, D9.c, D9.d | Ask cancel, answer history, footer | Decided (SPEC-0009); D9.c provisional |

Record D5.b and D5.d in SPEC-0005 before Phase G. G5.10 (abort an edit while arguments stream) needs one host probe; run it in Phase G, it does not change the data model.

## A2. Freeze (M0.4)

Freeze, per specification, everything that changes data models, persisted records, wire formats or safety boundaries:

- **Wire:** SPEC-0001 envelope, `discover` capabilities, registration kinds and descriptor fields, events, generations, error codes, limits. Go authoring API frozen as compile tests against the staged SDK (SPEC-0001 §6).
- **Model-facing:** tool names, schemas, descriptions and result envelopes for read/write/edit/grep/find/ls/bash/jobs/cd/ask_user; selector grammar and precedence; URI grammar and reserved schemes (SPEC-0004, SPEC-0012 §5).
- **Persisted:** config keys and files (SPEC-0010), frame ledger and delivery records (SPEC-0002 §4), snapshot and seen-state records (SPEC-0005 §3), job records (SPEC-0008), project registry and `Project.toml` (SPEC-0012 §3), vault format (SPEC-0013 §3), prompt epochs (SPEC-0014 §4.3), branch vs session ownership and recovery after reload.
- **Behavioral:** root+nearest, pin and cap semantics (PLAN-0001 §6), command routes.

Each frozen specification gets a revision with `status: "approved"` only after user review. Later changes need a new revision and a migration note.

## A3. Answer pig-web's M0.4 inputs

Answer M1–M6 from [HO-pig-web-m0-4-inputs](../handoff/pig-web-m0-4-inputs.md) inside the freeze: extension inference and secrets (incl. research child sessions), `knowledge_cutoff` in `session.model`, the descriptor `summary` field, `!command` expansion by registration kind (qualify SPEC-0013 S6), trailing tool-result blocks and redaction order, activity item fields and URI `open`. Choices that need the user go into A1's table first. Reply with a handoff into pig-web naming the freeze commit; archive the inbound handoff.

**Phase A exit:** frozen specifications committed; no open decision that changes a data model or safety boundary; pig-web reply delivered.

# 4. Phases B–L

## B. Factory, configuration and host adapter (PLAN-0001 M1.1, M1.2)

- Thin `Extension()`, lifecycle owner with shutdown joins, generation counter that survives same-process reload (P13/P14).
- Layered TOML (`Workbench.toml`, `Secrets.toml`) with round-trip-safe saves, trust restrictions, source attribution, inter-session locking, atomic private writes (SPEC-0010 §1–§8).
- Host adapter package: SDK contexts, hooks, original-builtin delegation, persistence; domain packages stay host-independent.
- Common sanitization, width fitting, path display with donor golden cases (SPEC-0003 §10), result and metrics types.
- Test harness: isolated agent dir, fixture projects, RPC and print drivers, `just check`, `just race`, `pig install . --validate-only --json`.

**Acceptance:** malformed config refused with location; unknown keys preserved on save; concurrent save from two sessions; untrusted project cannot widen trust; reload leaves no goroutines or temp files.

## C. Extension services v1 (SPEC-0001)

- `discover` with all capabilities: `frames`, `presentation`, `resources`, `location`, `status`, `snapshots`, `fetch`, `sessions`, `docs`.
- `register`/`unregister`/`cancel`, idempotent registration, owner scoping, generations, late-reply discard, bounded waits, no emit under a held mutex.
- Every registration kind accepted, validated and visible to its consuming subsystem: `toolPresentation`, `frameProducer`, `resourceProvider` (incl. `guide`, `summary` per A3), `fetchBackend`, `statusSegment`, `docs`.
- `/workbench integrations` and `/workbench resources` list owners, kinds, health and refusals.
- Reference producer extension (from `probe-wb`/`probe-producer`) kept as an integration fixture, and a small documented example.

**Acceptance:** absent, incompatible and ambiguous Workbench cases; stale generation rejected; reload re-registers cleanly; listener timeouts never block a tool call.

## D. Resources, selectors and pager (PLAN-0001 M1.3; SPEC-0004, SPEC-0006 §2–§4)

- Canonical URI/ToolPath parser with literal-first disambiguation; full selector grammar in one normalized representation; `paths:[]` multi-read in input order within one budget.
- Pager: byte/line budgets, actual shown spans, continuation, selection-complete vs resource-complete, artifact spill without recursion.
- Router: capability-declared providers, unsupported-with-alternatives errors, no silent local fallback for unknown schemes, walking rules for recursive tools.
- Builtin providers in this slice: `file://` (local), `artifact://`, `out://`, `local://`, `docs://` (registered docs, read-only), `job://` (Phase H), `session://self/` (Phase K), `cfg://workbench/` read-only diagnostics.
- External providers end to end: stat/read/list/search/locate through read/ls/find/grep, snapshots for opted-in providers, `resources.changed` invalidation.
- `fetchBackend` routing for `http(s)://`: policy, redirect safety, paging and locate; with no backend registered, `http(s)` answers a typed `unavailable` until the builtin reader lands in PLAN-0003.
- Enhanced read/ls/find/grep override the builtins for local files with selectors and plain code reads. Advanced adapters are PLAN-0003.

**Acceptance:** direct and codemode calls resolve identically; colon/comma/space filenames stay addressable; cancel joins workers; two sessions cannot reach each other's scratch; an external `build://` provider pages through `read` with Workbench selectors and `edit` on it returns `unsupported`.

## E. Location and project identity (SPEC-0008 locations, SPEC-0012 §3)

- Local location with revisions and leases, `cd` status/change/reset, persistence, `location` service and `location.changed` event.
- `Project.toml`, project recognition (§3.3), registry with `prj-` IDs, `global` project, project in provider requests (SPEC-0004 §4).
- Session bucket layout decision applied (SPEC-0012 §3.5) for new sessions; worktrees and SSH are PLAN-0003.

**Acceptance:** host cwd never changes; relative paths resolve against the captured location; nested worktree directories resolve to the parent project's ID.

## F. Frames and presentation (SPEC-0002, SPEC-0003)

- Frame ledger with branch generation guards, delivery receipts, replay on resume, compaction handling, trust classes, `beforePrompt` delivery as probed.
- Presentation resolver reading the live registry at first use (pig-web R2), all presets with typed cells including `uri`, plain renderers for every lifecycle state, display settings contributed by other extensions.

**Acceptance:** no duplicate frame after resume or branch switch; an untrusted producer cannot raise trust; a pig-web-style `web_*` tool renders through the resolver in TUI, RPC and print.

## G. Edit, snapshot and write core (SPEC-0005 §2–§3, §10)

- Snapshot store: canonical identity, revision over exact bytes, retained spans, seen-state from fully emitted lines only, tag binding per D5.
- Commit path: sorted multi-resource locks, revision check before commit, atomic local replace with mode preservation, exact outcome reporting.
- `edit` in exact-text mode complete; hashline core grammar (sections, ranges, PUT/CUT, seen guards, mismatch reveal) so a read→edit loop works. Blocks, registers, REM/MV and the recovery ladder are PLAN-0003.
- `write` with `expectedRevision`; `snapshots` service for other extensions (edit tracking).
- G5.10 probe: abort while arguments stream.

**Acceptance:** read→hashline edit and read→exact edit on CRLF, BOM and symlinked fixtures; stale revision refused; unseen lines never marked seen; an external snapshot provider commits only with matching revision.

## H. Jobs core and bash (SPEC-0008 jobs)

- One supervisor: foreground and background runs, process groups, log store with cursors and limits, cancel, crash/shutdown cleanup, ownership through branch navigation.
- `bash` override and the `jobs` tool on the supervisor; `job://` stat/read/list with the same policy. PTY, promotion, stdin keys and SSH are PLAN-0003.

**Acceptance:** noisy output stays bounded; cancel kills descendants locally; reload terminates owned jobs (D8.a); URI and tool controls share one permission check.

## I. Secrets boundary (SPEC-0013 §2–§4)

- Detection (auth.json incl. `!command` keys per A3, environment, `.env*`, token shapes), placeholders, keys, encrypted vault.
- Entry-time redaction, `context_with_system`, withhold via `ctx.Abort()`, substitution before every tool call including other extensions' tools, redaction order with trailing blocks per A3, agent-awareness prompt section.

**Acceptance:** captured provider payloads contain no known secret across a read, a bash run and a job log; tool calls receive real values; another extension's appended block is redacted.

## J. System prompts and instruction baseline (PLAN-0001 M1.4, M1.5; SPEC-0014 §3–§5, SPEC-0007 core)

- gonja engine, confined loader, sections, helpers, limits; `force` delivery, structure check, fallback, epochs; `/workbench prompt` and `session://self/system-prompt`; `before_provider_request` splice for late sections (F-P4 fallback).
- Context frames `session.date`, `session.location`, `session.model` (incl. contributed fields per A3).
- Instruction class model and records final; AGENTS.md root+nearest delivered before mutation through the probed path, no duplicate host injection. Pins, watchers, BACKLOG caps and diff updates are PLAN-0003.

**Acceptance:** golden renders for every PR5 fixture; prompt bytes stable within an epoch; a controlled read receives its baseline before the next model continuation; branch change drops queued delivery.

## K. Catalogs, sessions and ask core

- `session://self/` and `session://<id>` read projections via host listing; `skill://` catalog of enabled, trust-resolved skills; `docs://` (Phase C kind).
- Inter-session endpoint (SPEC-0011 §2–§5): registry, transport, authentication, envelope, delivery into a live session; `sessions` extension channel. `agent://`, loop guards and the coordinator are PLAN-0003.
- `ask_user` core: schema, pending record and recovery, host dialog in TUI and RPC, headless cancellation; `secret` type wired to the vault.

**Acceptance:** two live sessions exchange a message and the receiver wakes; an ask survives reload as pending; print mode never blocks on a dropped dialog.

## L. Commands, activity line and codemode capture

- `/workbench show|settings|integrations|resources|prompt`, `/tools`, `/cd`, with completions and machine-readable output; plain views only.
- Activity line (SPEC-0009 §9) with builtin counts and contributed `statusSegment` items; shortcut opens the same target; click waits for the fork.
- Codemode nested call capture and touch metadata available to every subsystem.

**Acceptance:** every command works in TUI, RPC and print; activity counts match registries.

# 5. Slice exit

1. `just check`, `just race`, package validation pass; reload and shutdown leak nothing.
2. Every WB row has its foundation per §1's rule, recorded in a progress table in this plan with links to tests.
3. **pig-web co-load gate** (its PLAN-0001 Slice 5, real Workbench): `pig -e ../pig-workbench -e ../pig-web` renders `web_*` through Workbench; `read git://…:120-180` uses Workbench selectors; `edit git://…` is `unsupported`; a research notice arrives as a frame; `/workbench integrations` lists pig-web; `read https://…` is served by pig-web's fetch backend; the `web` status segment appears once; a stored search key is redacted in a `jobs` log. `cd` into a worktree is deferred to PLAN-0003 and named as such in the gate report.
4. Handoff to pig-web naming the slice-exit commit and listing anything still deferred.

Completion follows PLAN-0001 §7: observed checks only; failures and skipped gates reported with impact.
