---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-inventory"
  title: "Slice 1 inventory: implementation state against every PLAN-0001 §9 ownership row"
  description: "Which Slice 1 ownership rows are implemented with evidence, which are partial, and which are missing, as observed at eeb3e27 plus later commits"
  status: "published"
  revision: 1
  created_at: "2026-11-05T16:40:00+01:00"
  updated_at: "2026-11-05T16:40:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Separate completed evidence from missing wiring, behavior and acceptance without narrowing scope"
      changes: ["Recorded per-row status with file and test anchors", "Named the ordered remaining work per phase", "Confirmed no row is satisfied by an unadvertised or stub path"]
---

# Scope

Inventory of the Slice 1 rows in [PLAN-0001 §9](../../plans/0001-pig-workbench.md) against the implementation
observed at `8fb437a` (after Phase C). Status words are used strictly:

- **done** — implemented, wired and covered by an observed test or live run.
- **partial** — the named subset works and is tested; the listed remainder does not exist yet.
- **missing** — no implementation.

Nothing in this table is satisfied by a stub, an unadvertised registration or an `unsupported` answer to an
advertised operation. Acceptance anchors of the specifications are **not** rewritten here; where a row is
partial, the spec anchor has not been passed.

# Row status

| Spec § | Owner | Status | Observed evidence | Missing |
|---|---|---|---|---|
| 0001 §2–§7 | C | done | `internal/host/services.go` (discover/register/unregister/describe/cancel, generation checks, in-flight limits, owner/instance correlation), `api/client.go` (`Discover`/`Call`/`Close`, late-reply discard, outcome-classified transport errors), `internal/host/services_test.go`, `api/client_test.go`, `internal/host/live_services_test.go` (absent/incompatible/ambiguous, stale generation, session replacement, resume) | `operation.status` and admitted deferred work are not implemented, consistent with SPEC-0001 §3's rule not to advertise unproven deferred operations |
| 0001 §8 | C + consumer | partial | Advertised: `location`, `presentation`, `resources`, `fetch`, `status`, `docs`, `frames`. Events: `ready`, `closing`, `location.changed`, `settings.changed`, `registration.changed` | `snapshots`, `sessions`, `secrets` services; `resources.changed`, `frames.deliveryChanged`, `sessions.message` events |
| 0002 all | F | partial | `internal/frames` ledger with determinism, dedup, expiry, ownership, branch scope, epoch binding; `internal/host/contributed_frames.go` (publish/status/get/cancel/list, trust, nextBoundary steer, nextPrompt), `internal/host/frame_context.go`, `internal/frames/*_test.go`, live notice delivery in `internal/host/live_reference_test.go` | Ledger replay across resume and branch switch, compaction reconstruction, `beforePrompt` delivery path verification, bounded receipt index at `session://self/frames` |
| 0003 §2–§6, §10 | F, B | partial | `internal/presentation` typed snapshots with validation, `internal/host/presentation.go` + `presentation_render.go` (live resolver reading the registry, presets, sanitizing, width fitting), `internal/host/contributions_test.go` | Resolver coverage for every preset/cell kind and plain renderers for every lifecycle state; `codemode` captures; §10.3 path-display goldens (no `displayPath` helper exists) |
| 0004 §1–§10 | D | partial | `internal/resource` parser, selector grammar, pager, router, walking; providers `file`, `http`, `https`, `cfg`, `artifact`, `session`, `local`, `out`, `docs`; `internal/resource/*_test.go`, `internal/host/live_http_test.go`, `internal/host/live_reference_test.go` | `job://`, `skill://`, `agent://` providers; first-touch guide injection on read (guides are served but not injected on first touch); `resources.changed` invalidation |
| 0004 §11 | D | done | `internal/resource/http.go`, `internal/host/contributed_fetch.go` (most-specific rule, locate, 4xx final, fallback with notice), `internal/host/contributed_test.go`, live backend/locate/404 rows in `internal/host/live_reference_test.go` | Redirect hardening is covered by `internal/htmlread` plus policy tests; no known gap |
| 0005 §2–§8, §10, G5.2 | G | partial | `internal/hashline` (normalize with staged origins, XXH32 tags, staged restore, seen spans, relocation, store with epoch+delivered gating), `internal/fileedit` commit executor, `internal/host/edit.go` (tagged and exact-text, current-source re-read, relocation, seen guards, per-turn mode), `internal/host/write.go`, `internal/host/snapshots.go`, `internal/host/live_edit_test.go`, `internal/host/live_write_test.go`, `internal/hashline/*_test.go` | Public `snapshots.view`/`snapshots.status`/`snapshots.touched` service (SPEC-0005 §10); persistent register replay across sessions; notebooks; donor fixture corpus for §8 edge rows |
| 0006 §2–§4, §6–§7 | D | partial | `internal/resource/page.go` (shown spans, continuation, selection- vs resource-complete), `internal/host/tools.go` read/write/ls, `internal/host/search.go` grep/find, `internal/syntax` summaries and enclosing boundaries, `internal/host/live_test.go` | Supersession and repeated-read hints; loop-guard rows; multi-read parity in codemode |
| 0007 §2–§8 local | K | partial | `internal/instructions` discovery with exact-byte digests, caps and steering defaults; `internal/host/instruction_context.go` root+nearest delivery with project trust anchoring; duplicate host injection suppressed; `internal/host/live_test.go` frame delivery | Pins, diff updates, multi-touch unions, watchers for new candidates, withdrawal, compaction and branch reconstruction, `projectPrivate` class wiring |
| 0008 §2–§3 local | E1 | done | `internal/location` (capture, revisions, leases, transitions, persistence), `internal/host/tools.go` `cd` tool, `/cd`, `/wb:cd`, `location` service and `location.changed`, `internal/host/live_test.go`, `internal/host/live_services_test.go` (cd, session replacement, resume) | — |
| 0008 §5–§7, G8.20 | H | partial | `internal/jobs` supervision (process groups, pidfd identity, bounded logs with cursors, stdin/EOF, waits, kill), `internal/host/jobs.go` (`bash`, `jobs` tools), `internal/host/live_jobs_test.go` | `job://` resources, `/workbench jobs` and `/wb:jobs`, G8.20 scheme command dispatch, reload job termination verification, secret env injection, crash/ownership matrix |
| 0009 §2–§3, §5 | L2 | partial | `internal/ask` (schema, dialogs, records, results), `internal/host/ask.go` including vault-backed secret answers, `internal/host/live_ask_test.go` (native RPC choice reaching the model) | Pending-ask persistence and recovery across reload, rich TUI questionnaire, full question-type and mode matrix (§6) |
| 0009 §8 | M | partial | `/workbench show|tools|cd|settings`, `/tools`, `/cd`, `/wb:cd`; completions on `/workbench` | `/workbench integrations|resources|prompt|skills|sessions|jobs|frames|instructions|trust|projects unlink-storage`, `/wb:jobs` |
| 0009 §9 | M, F | partial | Contributed `statusSegment` with priority, fitting, severity wording, 4/s coalescing (`internal/host/contributed_status.go`, `internal/host/contributed_test.go`); fixture status row observed live | Builtin activity counts, shortcut target, activity placement wiring in the status line, TUI captures |
| 0010 all | B | partial | `internal/config` scoped TOML with provenance, strict validation, last-valid stale state, merge-preserving atomic saves, cross-process locking, trust restrictions; `internal/host/settings.go`, `internal/config/*_test.go`, `internal/host/live_settings_test.go` | `Secrets.toml`, complex instruction classes, trust-rule editing, dynamic producer settings, reload leak verification |
| 0011 §2–§8 | L1 | missing | `internal/endpoint` holds record/store/registry primitives with tests only | Transport, authentication, wake/state tracking, envelope delivery, loop guards, `agent://` send/read/wait, `sessions` channel, `/workbench sessions` |
| 0012 §3.1–§3.4 | E1 | partial | `internal/project` document/registry/detection, `Project.toml` parsing, `prj-` ids, `global`, project facts entering `session.project`, `internal/project/*_test.go`, `internal/host/live_test.go` | Full recognition precedence rows and provider request `project` field (no `project` in `api.ResourceRequest` yet) |
| 0012 §3.5 | E2 | missing | — | Journaled bucket conversion, live-session conflict deferral, crash recovery, `projects unlink-storage` undo |
| 0012 §3.6 | K | missing | — | `projectPrivate` instruction class |
| 0012 §5.1–§5.2, §5.5 | D | partial | Scheme naming, tool semantics over the router, contributed per-sub-namespace guides served at `docs://workbench/schemes/<scheme>` | First-touch guide injection into read results |
| 0012 §5.3 core | D, H, J, L1 | partial | `file://`, `artifact://`, `out://`, `local://`, `cfg://`, `session://self/*` | `session://<id>`, `by-id/`, titles of own and stored sessions, `agent://`, `skill://`, `job://` |
| 0012 §5.4 | C, D | done | `internal/host/contributed_docs.go` serving `docs://<owner>/` and `docs://workbench/schemes/<scheme>`; live rows in `internal/host/live_reference_test.go` | PiG source docs are S2 |
| 0013 §2–§5, §7 | I | partial | Detection, placeholders, keys, vault primitives with tests; `ask_user` secret answers stored encrypted with placeholder-only records | Entry-time/context redaction, provider-payload fail-closed hooks, tool-call substitution for every tool including other extensions, appended-block redaction order, injected environment, agent-awareness prompt section |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | J | partial | `internal/prompt` gonja engine with confined loader, strict `attr`, cancellable loops; six context facts delivered (`session.date/environment/project/location/model/capabilities`), owner-scoped `knowledge_cutoff` via `context.fields.set`; `internal/host/frame_context.go`, `internal/host/live_test.go`, `internal/host/live_reference_test.go` | `builtin/workbench` template and takeover, sections/helpers, delivery `auto` with fallback, epochs, late-section splice, skill catalog/listing ladder/load-unload/`skill://`/compaction replay, `/workbench prompt|skills` |

# Cross-cutting observations

- The advertised capability list is honest: `advertisedCapabilities` contains only wired services, and the
  unknown-capability test refuses the rest.
- Three ownership rows are dominated by one missing subsystem each: `snapshots` (G), `job://` + G8.20 (H),
  and the secret boundary (I). They are the cheapest large wins because their engines exist.
- The largest genuinely missing subsystems are messaging (L1) and session-storage adoption (E2), which E2
  depends on.
- Phase M is the only row group that is mostly a matter of wiring existing state into commands.

# Ordered remaining work

Follows the plan's phase order, not by cost:

1. **A** — ledger, frozen contracts, pig-web answer.
2. **B** — `Secrets.toml`, remaining config/trust editing, path-display goldens at 36/57/80/140.
3. **C** — done.
4. **D** — `job://`, `skill://`, `agent://` providers, first-touch guides, `resources.changed`, remaining §10 rows.
5. **E1** — project request field and recognition rows; **E2** after L1.
6. **F** — replay/compaction, `beforePrompt` verification, full preset/cell renderers.
7. **G** — `snapshots` service, register persistence, §8 donor fixtures.
8. **H** — `job://`, `/workbench jobs`, `/wb:jobs`, G8.20, reload cleanup.
9. **I** — the whole redaction/substitution boundary plus `secrets.redact`.
10. **J** — prompt takeover, sections, epochs, skills.
11. **K** — pins, diffs, unions, watchers, reconstruction.
12. **L1** — messaging; then **E2**.
13. **L2** — pending recovery and TUI questionnaire.
14. **M** — command inventory and activity line.
15. **17** — acceptance table, gates, docs, seam gate with the reference extension.
