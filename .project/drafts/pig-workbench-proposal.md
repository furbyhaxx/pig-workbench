---
document:
  type: "draft"
  id: "DRAFT-0001"
  title: "pig-workbench: a Go-native tool and harness foundation"
  description: "Source-grounded adoption scope, architecture, contracts, risks and delivery proposal"
  status: "draft"
  revision: 1
  created_at: "2026-11-02T14:49:32+01:00"
  updated_at: "2026-11-02T14:49:32+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Initial research and proposal"
      changes: ["Inspected reference implementations and staged PiG SDK; proposed adoption scope and delivery gates"]
---

# Recommendation

Build one **Go factory extension**, `pig-workbench`, with a small set of cooperating services, not a line-by-line port or a second harness. Override PiG's seven file/shell builtins; add `cd`, `ask_user`, and exactly one `jobs` tool. Use one `/pig-workbench` command family. Reuse PiG's builtin factories, event bus, session storage and kit UI where they fit. Implement the genuinely new behavior in plain Go.

The core is a resource router and execution-location service, shared by enhanced reads, safe edits, search, shell execution and jobs. Frames and questionnaires use those services but do not become dependencies of filesystem algorithms.

No subagents, agent definitions, tasks or todos. No migration or historical compatibility machinery from either donor. Preserve formats we introduce after release, not formats we never shipped.

## Source observations

Research used these local checkouts; no remote service received project content:

| Source | Inspected revision | Relevant evidence |
|---|---|---|
| pi-workbench | `e8ee24772cb28a0f1ba6ea497485328657fb60fb` | `src/index.ts`, `src/core/{cd,working-location,operations,jobs,grep}.ts`, `src/message-framing/`, `src/ask/schema.ts`, `docs/{features,frames}.md` |
| oh-my-pi | `553f0d10e813a5e1fb65e545efe7c814bc03a879` | `packages/coding-agent/src/internal-urls/`, `tools/read.ts`, `edit/{schemas.ts,hashline-compact.md}`, `crates/pi-edit/src/modes/hashline/` |
| nukular-ai / nugent | `0080fed7064639d184b5d3ce90099892507741e7` | `crates/nukular-nugent/src/schemes/{harness_scheme,skill_scheme}.rs` |
| Running PiG | `0.5.3+1.4.0.74f46864` | Bundled extension/API/event/RPC UI documentation; staged Go SDK `sdk-899c7964784a0050` |

External source roots are `host:/mnt/Projects/furbyhaxx/pi-workbench`, `host:/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi`, and `host:/mnt/Projects/nukular/nukular-ai`. The pi-workbench and nukular-ai trees contain unrelated local changes; they were not modified. Revision IDs identify baselines, not clean-tree guarantees.

Important verified facts:

- pi-workbench's entrypoint combines remote tools, jobs, instruction loading, statusline composition and excluded agent/task features. Copying that entrypoint would import the wrong boundaries.
- Its working-location owner already has useful branch-local records, revisions and admission leases. Its `cd` deliberately does **not** change the host cwd.
- Its bus service publishes callable JavaScript functions. PiG's Go `EventBus` crosses realms as JSON values; that service shape cannot be carried across unchanged.
- PiG's `builtin_tool.go` exposes `CreateReadToolDefinition`, the other builtin factories, execution wrappers and operation callbacks. `BuiltinToolDefinition.ExecuteWith` prefers the **host context cwd** over the factory cwd. Merely constructing a builtin with the Workbench cwd is therefore insufficient.
- PiG's bus is synchronous: listeners finish before `Emit` returns; listener errors do not propagate as service responses. Request/reply needs explicit response envelopes.
- Current oh-my-pi hashline is not just `line:hash` labels: its compact contract uses `[PATH#TAG]`, snapshot tags, PUT/CUT operations, seen-line tracking, syntax blocks and move/delete operations. Importing the name does not require importing that entire language.
- oh-my-pi's read pipeline includes selectors, archives, database queries, media, conversion and summarization. Those are separate product choices, not prerequisites for multi-read.
- Nugent demonstrates a smaller capability-declared router, a skill catalog backed by a shared snapshot, and queued harness reload. Its task provider is explicitly outside this project.

This is source research, not runtime proof. No extension has been scaffolded, built or exercised. Integration gates below remain open.

# 1. Adoption scope

| Area | Adopt | Adapt or leave out |
|---|---|---|
| Builtins | Override `read`, `write`, `edit`, `ls`, `find`, `grep`, `bash`; consistent metrics, limits, rendering and config | Reuse host behavior rather than duplicating every builtin implementation |
| Read | Explicit multi-target requests, ranges/tail, paged text, images through supported host behavior, snapshot/anchor metadata | No delimiter guessing, fuzzy path substitution, automatic LLM summaries, database/media/converter stack in baseline |
| Edit | Hash-anchored range replacement/insertion/deletion; exact-text multi-edit alternative; preflight validation | No fuzzy repair, syntax-block editing, clipboard language, implicit move/delete or whitespace normalization |
| Search | Literal multi-pattern OR, regex mode, include/exclude globs, files-only results, context, exact completeness | No silent changes to ignore rules or regex dialect between local and SSH backends |
| Locations | Local and SSH locations, explicit per-call override, persistent `cd`, reset, branch worktree entry | No process-wide `os.Chdir`; no automatic host config rewriting |
| Frames | Structured attributed messages, branch replay, display densities/visibility, delivery policy | No claim that XML tags or a nonce are a security boundary |
| Instructions | Nested AGENTS.md discovery, reconciliation on location changes, compaction/resume handling, trust and bounded updates | No CLAUDE.md/GEMINI.md aliases; no duplicate injection alongside host project context |
| Event bus | Versioned JSON events and bounded request/reply services on PiG's existing bus | No second transport, function-valued payloads or unbounded event history |
| Ask | One `ask_user` tool: single/multi/free-text answers, previews, review, cancellation, pending recovery | No duplicate `ask` tool; model-based `/answer` extraction and remote bridge integrations deferred |
| Jobs | Foreground/background execution, status/list/output/wait/cancel/stdin, completion frames, durable logs | One `jobs` tool, not job_* siblings; no daemon/subagent/service manager |
| Settings/UI | Layered extension JSON; compact/normal/full renderers; kit panels; tool/schema inspection | No general-purpose footer/editor replacement, configurable statusline framework or branding |

SSH and instruction lifecycle are included in the intended base because they make `cd` and frames coherent. Their implementation follows local correctness rather than blocking the first working slice. Host editing/rename panels, manual branch summaries and model pickers are not baseline requirements.

# 2. Architecture

```text
PiG registration: tools · commands · lifecycle/context hooks · renderers
                              |
                    session-owned Workbench
                /         |          |          \
          config       locations    frames       ask
                          |            ^
                     resources      instruction ledger
                   /     |      \
                local   SSH    virtual handlers
                   \     |      /
                read · edit · write · search
                          |
                   execution / jobs
                          |
                output budgets + artifacts
```

`extension.go` wires dependencies only. Start with files grouped by responsibility; extract `internal/resource`, `internal/location`, `internal/edit`, `internal/jobs`, `internal/frames`, `internal/ask`, and `internal/config` when each has enough code to justify a package. Do not generate a framework-sized directory tree before the features exist.

One state owner per extension/session, no mutable process globals. Separate:

- **Branch state:** location records, frame/instruction ledger, pending questionnaire identity.
- **Session runtime:** running processes, cancellation roots, event subscriptions and UI state.
- **Machine storage:** bounded logs, artifacts and indexes under host-derived state paths.

Tree navigation restores branch facts; it does not rewind processes or recreate completed commands. Reload/shutdown drains session-owned work. Historical job records never imply a live process.

## Builtin reuse boundary

Wrap original builtin definitions where contracts are unchanged, preserve their streaming/error behavior, and attach Workbench metadata/renderers. Do not delegate by calling the public tool name being overridden: that can recurse.

Use explicit location resolution or operation callbacks for changed-cwd and SSH behavior. Confirm the builtin wrapper's context and operation semantics before selecting a delegation path. Hashline editing, multi-read aggregation and URI routing are Workbench-owned algorithms; they are not thin render wrappers.

Permissions are an explicit integration gate: establish how extension-owned filesystem/process work participates in PiG policy. Never assume direct Go I/O has inherited the builtin's authorization. Disable an unsupported execution path with a diagnostic rather than silently bypassing host controls. Project trust controls configuration/instructions; it is not equivalent to permission to execute a shell command.

# 3. Model-facing contracts

Keep builtin names because these are replacements. Proposed ergonomic exceptions to the normal extension-prefix convention are `cd` and `ask_user`; `jobs` is explicitly required by the request. Everything else uses the `pig-workbench` identity. Document these exceptions and detect conflicting registrations; do not register duplicate prefixed aliases.

Schemas reject unknown properties, bound arrays/strings/numbers and enforce cross-field constraints in Go. Examples below describe proposed contracts, not existing SDK calls.

## Read

Accept exactly one of `path` or `targets`. Each target has a path and optional explicit range selection. A range API should express contiguous ranges, disjoint ranges and tail without appending ambiguous syntax to filenames.

```json
{"targets":[{"path":"src/a.go","ranges":[{"start":20,"end":65}]},{"path":"src/b.go","tail":40}]}
```

- Bounded worker pool; deterministic input-order results; stop workers on cancellation and join them.
- One global output budget, not 50 KiB independently for every target. Start with 50 KiB / 2,000 textual lines per result, including headers and notices, and a small bounded target count.
- Per-target status, canonical resource identity, returned spans, snapshot revision, completeness and continuation. One target failure does not erase successful targets or masquerade as total success.
- Oversized content stays in the original resource when possible. Spill generated output once to `artifact://`; never recursively spill an artifact read.
- Only fully emitted source lines become seen/editable anchors. Clipped lines, elisions and transformed content do not.
- Preserve CRLF, final newline and Unicode in storage. Sanitize terminal display separately; never alter file bytes just to render them.
- Streaming reads bound memory; tail uses a bounded buffer. Total line counts are optional unless actually measured.

## Edit: safety first, smaller than oh-my-pi

Use one `edit` tool with mutually exclusive modes:

1. **Anchored edits (default):** resource path, snapshot revision, and typed replace/insert/delete operations referencing original line anchors.
2. **Exact-text edits:** a list of unique `oldText`/`newText` replacements, all matched against the original snapshot, not against prior replacements.

A structured operation schema is easier to validate and extend than initially importing PUT/CUT plus block syntax. Adopt the semantics, not its patch grammar. Use a sufficiently strong snapshot digest and compact line anchors; do not rely on a four-hex display tag as a write-integrity check.

Preflight every operation against one original snapshot, reject overlaps/ambiguous text/unseen anchors, then commit once per file. Detect changed files before committing; return current anchored context for repair, never relocate edits heuristically. Serialize cooperating writers by canonical identity. Atomic replacement prevents torn writes but is **not** a filesystem compare-and-swap: document the residual race with external writers. Preserve modes and define symlink behavior explicitly.

Support several edits in one file immediately. Multi-file edit batches can follow after single-file correctness, with all-file preflight and explicit per-file commit outcomes; do not advertise cross-file atomicity. A failed exact-text mode does not fall back to fuzzy or anchored mode.

## Shell and jobs

`bash` starts commands. Explicit `background: true` returns an owned job ID promptly. Default foreground work stays cancellable; offer opt-in promotion after a configured yield interval, not a hidden hard-coded conversion of every slow command.

One `jobs` tool uses a discriminated `action`:

| Action | Purpose |
|---|---|
| `list` | Bounded running/recent job summaries |
| `status` | State, timestamps, location, exit facts and log cursor |
| `output` | Bounded output since cursor or tail; next cursor and completeness |
| `wait` | Cancellable bounded wait; explicitly report whether it timed out |
| `cancel` | Idempotent process-group cancellation with escalation |
| `stdin` | Write bounded text; explicit close/EOF operation |

Do not offer `start` in both tools. Capture the immutable execution location at start. Distinguish success, nonzero exit, cancellation, timeout and interrupted/unknown after restart. Exit code remains null until known.

Pipes are not a terminal: text input and EOF work; arrow-key emulation and Ctrl+D-as-a-byte are not promised. PTY support is a separate dependency/platform decision.

Background jobs are adopted by a session supervisor before the launch handler returns. Request cancellation stops an unadopted launch; cancellation of a `jobs wait` stops waiting, not the running job. Shutdown kills and joins owned work; crash cleanup and remote orphan prevention need explicit platform tests. Do not resume from a persisted PID alone.

Logs use private files, quotas and retention. Completion events are deduplicated; coalesce bursts into one frame. Keep raw command output untrusted. Prevent active jobs or their logs from leaking across session ownership boundaries.

## Ask

One `ask_user` tool, with stable question and option IDs, bounded questions/options, optional preview text, single/multi/free-text types, and explicit submitted/skipped/cancelled/unavailable results. Partial answers stay partial; cancellation is never converted to an answer.

- TUI: kit-based flow with review and editable answers. Show captures before settling the layout.
- RPC: native select/input/editor dialogs using the same questionnaire state machine; multi-select can use repeated selection plus an explicit Done action.
- Print/JSON: return an actionable interaction-unavailable error promptly, without blocking or inventing answers.
- Persist pending payloads minimally for recovery, bind submissions to session/request identity, reject duplicate/late submissions. No automatic cross-session bridge.
- Persist completed answers only as needed for the normal transcript; do not add a separate plaintext history archive by default. Never ask for credentials.

# 4. Resource schemes

Use a session-owned registry with declared operations and backing kind. Tool dispatch consults capabilities instead of testing scheme strings throughout the code.

A handler declares canonicalization, local/remote/virtual backing, read/list/search/locate capabilities, immutability and explicit mutation support. File-backed does not mean writable. A virtual resource's located source file is a link, not permission to edit it.

| Scheme | Recommendation | Semantics |
|---|---|---|
| `file://` | Core | Explicit local paths, even while the default location is SSH |
| `ssh://` | Core, after local slice | Explicit host/path; same location resolver as `cd` and shell |
| `artifact://` | Core | Read-only session-scoped overflow output and logs, paged without re-spilling |
| `local://` | Core | Writable session scratch, independent of cwd, bounded retention and traversal/symlink containment |
| `skill://` | Include after catalog probe | Read/list only over PiG's actual enabled, trust-resolved skill catalog; contained supporting files if catalog provenance supports them |
| `harness://status` | Include narrowly | Read-only extension state, capabilities and diagnostics; no credentials or full prompt dump |
| `harness://reload` | Defer mutation | Nugent's queued-reload idea is useful, but safe boundary scheduling and authorization must be demonstrated first |
| `proc://` | Omit initially | Would duplicate `jobs`; expose logs through artifact URIs and controls through one explicit tool |
| `cfg://` | Defer | Read-only redacted extension settings may later be useful; no model-controlled host/trust/security writes |
| `history://`, `attachment://`, `mcp://` | Later adapters | Only if the host exposes an authoritative scoped catalog; no transcript scraping or second MCP client |
| `agent://`, task/todo schemes | Exclude | Explicit non-goals |
| rule/memory/vault/security/issue/PR/device schemes | Exclude baseline | Require subsystems not requested here |

Parse schemes strictly; reject unsupported ones with available capabilities. No automatic network fetch, MCP fallback, shell expansion, or URI typo repair. Decode once and test traversal, encoded separators, NUL, symlink escape and SSH authority injection. Keep ranges as tool arguments, not colon suffixes that collide with ports and real filenames.

Initially extensible means a small Go handler interface and declarative capability table. Add cross-extension resource-provider registration only for a concrete consumer: that requires lifecycle, cancellation, ownership and quotas, not a global map of arbitrary callbacks.

# 5. Locations, instructions, frames and events

## Location transitions

`cd {}` reports status; `path` changes location; `reset` restores origin; `branch` enters/reuses a local branch worktree. Path calls capture a location revision. A transition takes exclusive admission, validates destination and instruction changes, persists the commit, and publishes the new revision. Concurrent relative-path work receives a retryable location-changing error rather than observing half a transition. Jobs already started keep their captured root.

Workbench cwd is not PiG cwd. Other extensions opt into the resolver over the bus. State that clearly in the tool description and status display. Do not claim `cd` changes host resource discovery, project trust, shell state in another extension or the entire PiG process.

Keep project config anchored to the trusted host launch project. Moving to another directory or SSH host never implicitly loads that destination's extension settings or inherits launch-project trust. Worktree creation is explicit and collision-safe; no checkout deletion/reset or silent branch creation.

## Instructions and frames

Preserve pi-workbench's useful frame kinds: instruction, reminder, warning, error, notice, info. Store versioned facts including source, resource, sequence, timestamp, trust and body; render separately for model and UI. Avoid embedding command/file output as trusted instructions. Escape envelope fields and delimiters.

The nonce is a provenance cue for the model, not authentication; copied tags remain untrusted data. Trusted extension code is already inside the harness trust boundary. Source registration is attribution, not a sandbox.

Deliver ordinary frames at safe context boundaries without starting a new model turn. Job completion may request a follow-up according to explicit user policy. Bound and coalesce pending events, never silently discard instruction invalidations or errors.

Default to the host-supported custom-message path. Native system/developer role projection is optional only after verifying accepted message shapes and provider behavior; never infer support from a provider name. TUI visibility is independent of model visibility.

Instruction lifecycle should own AGENTS.md reconciliation only when host `project_context` suppression and reinjection are proven. The staged SDK exposes ordered system-prompt sections, but this alone does not prove complete interception of every retry/compaction path. Until the spike passes, keep host instruction behavior and avoid duplicate delivery; report any unavailable dynamic behavior rather than claiming parity.

Target behavior: broad-to-narrow discovery, nested loads on relevant path access, local on-turn content checks first, debounced watching later, remote on-turn checks, additions/changes/withdrawals, selected-branch reconstruction and post-compaction baseline. Reuse unchanged content by identity/digest, not just basename. Cross-root and SSH instructions default to untrusted unless explicitly authorized in user/runtime policy. Project config cannot widen trust.

## Bus contract

Reuse `ctx.Events()` and channels under `pig-workbench:v1:*`: capability discovery, location changed, frame request, job changed and ask lifecycle. Payloads are bounded JSON with version, session identity and correlation IDs where applicable.

Read-only resolver/service requests use explicit request/reply envelopes, deadlines and errors. Do not hold Workbench mutexes while emitting synchronous bus events. Do not expect listener errors to answer a request. External events are validated; a malformed request gets a diagnostic/response rather than disappearing silently.

Keep initial bus services small: discover capabilities, snapshot/resolve location, request a frame. Do not publish unrestricted arbitrary shell execution as a convenience service. In-process feature code uses ordinary Go interfaces, not event round trips.

# 6. Configuration, rendering and operational quality

Config: defaults < user `<agent-dir>/pig-workbench.json` < trusted launch-project `<config-dir>/pig-workbench.json` < runtime. Use SDK path helpers. Security-widening policy is user/runtime only. Preserve unknown keys, reject malformed files, save atomically with private modes and inter-session locking. Reset deletes the scoped override. Start with feature enablement, display density, output/jobs budgets and trust policy; avoid inheriting the donors' entire settings inventory.

Use compact/normal/full rows for tools and collapsed frame rows by default. Metrics come from structured details: bytes/lines returned, duration, match counts when known, job exit state, complete flag and continuation. Unknown totals stay absent. Error, cancelled and running states remain visible. Sanitize terminal sequences before styling, without mutating execution arguments or persisted source bytes.

Kit views for settings, jobs and questionnaire flow. Native dialogs for simple questions. All lines fit 36/57/80/140 columns; short-terminal checks at roughly 16 rows. Cache render-ready data outside render calls. Use a short status contribution, not a replacement footer/editor; general-purpose extensions should not fight for those single-owner surfaces.

No additional runtime dependencies proposed. OpenSSH is an optional external prerequisite for SSH, not an implicit embedded client. Reuse the host search backend when its semantics fit; if enhanced search requires an external executable not already supplied by PiG, request approval and declare it. Do not implement a silent divergent fallback. A local-only installation must work without SSH.

Copied/adapted MIT code requires retained notices and an attribution inventory. Check file/subpackage notices before importing individual pieces; the root license alone is not a complete provenance audit.

# 7. Delivery and acceptance

These are proposed slices, not approved implementation tasks. No speculative time estimates before host probes.

1. **Integration spikes and contracts.** Scaffold a Go factory after approval. Prove same-name overrides, original builtin delegation at a changed location, policy behavior, JSON bus request/reply, session replay, instruction suppression, frame delivery and RPC ask cancellation. Report host gaps separately; no host patch is assumed authorized.
2. **Useful local base.** Shared output envelope and budgets, local router, enhanced multi-read, anchored/exact multi-edit, search, write/ls/find wrappers, config and renderers. Include stale-edit and partial-output regression tests.
3. **Location and harness layer.** Local `cd`/worktrees, frame ledger, instruction ownership if the spike passes, artifact/local schemes and read-only harness diagnostics.
4. **Shell lifecycle.** `bash` supervisor, single `jobs` tool, bounded logs/cursors, completion frames and headless behavior. Exercise cancel/shutdown/reload/crash paths.
5. **Questionnaires and remote operations.** `ask_user` state machine and TUI/RPC flows; SSH transport integrated through the same resource/location contracts; skill scheme once its catalog source is proven. Keep these as independently reviewable slices.
6. **Release hardening.** Live width captures, policy/trust probes, concurrency/race checks, benchmarks, source/notice audit and concise product documentation.

Required evidence before claiming the intended base complete:

- Go formatting, vet, unit tests and race detector using the staged SDK; PiG factory/registration validation with exact tool inventory and packability.
- Builtin override tests proving no recursive dispatch and correct local/remote/cd execution roots.
- Edits: stale snapshots, ambiguous text, overlapping operations, unseen/clipped lines, CRLF/no-final-newline, symlinks, concurrent writers and failed commit reporting.
- Multi-read/search: deterministic ordering, partial failures, cancellation, global budget enforcement, exact completeness and continuations. No invented total counts.
- Jobs: noisy stdout/stderr without deadlock, UTF-8 split boundaries, stdin/EOF, descendants killed, timeout vs cancel, repeated cancellation, quota behavior, no false restart recovery and bounded shutdown.
- Branch/fork/resume/compaction: no duplicate instruction baselines, stale location commits, resurrected jobs or questionnaire answers from another branch/session.
- RPC/print/JSON: ask never hangs without interaction; commands do not disappear into dropped notifications; script-required operations are tools.
- Live TUI captures at 36/57/80/140 columns and short height, including hostile terminal text, narrow paths, long questions, errors and resize.
- SSH tests: noninteractive auth, paths with quotes/newlines, ports/home expansion, disconnect, cancellation and remote cleanup. No weakened ignore/search semantics hidden in a fallback.
- Benchmarks for large-file range/tail reading, multi-read throughput/memory, search volume, edit preflight and render cost. Record baselines before setting numeric latency budgets; bounded memory and no render-time I/O are immediate requirements.

# Decision requested

Approve this scope and architecture as the implementation direction, especially:

1. One override set plus `cd`, `ask_user`, and exactly one `jobs` tool; no redundant alias tools.
2. A small structured hash-anchored edit contract, not oh-my-pi's full patch/block language.
3. Initial schemes `file`, `ssh`, `artifact`, `local`, read-only `harness`, and catalog-backed `skill`; process/config mutations stay explicit or deferred.
4. SSH and dynamic instructions belong in the intended base, but follow local correctness and proven host integration.

The main tradeoff is deliberate: this ports fewer donor mechanisms, but produces one consistent tool/resource/lifecycle model rather than three implementations wearing the same extension name.
