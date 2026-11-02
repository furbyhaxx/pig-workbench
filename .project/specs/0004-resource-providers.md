---
document:
  type: "specification"
  id: "SPEC-0004"
  title: "Resource provider API and URI contracts"
  description: "Pluggable schemes, selectors, operations, paging, policy and source provenance"
  status: "draft"
  revision: 3
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-02T20:30:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Define the public Workbench extension APIs before implementation"
      changes: ["Initial contract, lifecycle, examples and acceptance criteria"]
    - revision: 2
      reason: "Align shared selectors with the detailed advanced-read contract"
      changes: ["Specified raw and comma-list singleton semantics from donor parser and linked detailed behavior specs"]
    - revision: 3
      reason: "User defined session:// routes and removed session access restrictions"
      changes: ["Added the session:// route grammar and project membership rule", "Removed cross-project consent and session-scope authorization"]
---

# 1. Purpose and architectural boundary

Other extensions can register `scheme://` providers into Workbench's resource router. Builtin and external providers obey the same selectors, output budgets, permission policy and source-provenance rules. Provider APIs are semantic operations, not arbitrary shell commands or filesystem escape hatches.

Depends on [SPEC-0001](0001-extension-services.md). Rendering uses [SPEC-0003](0003-tool-presentation.md); notices use [SPEC-0002](0002-message-frames.md). Draft contract, not implemented SDK capability. Prior art: nugent `scheme/{tool_path,selector,pager,scheme_provider}.rs` and oh-my-pi `internal-urls/` under the donor paths in root AGENTS.md.

# 2. Provider registration

Register kind `resourceProvider` with:
| Field | Meaning |
|---|---|
| key, label, description | Stable owner-scoped identity and inspection text |
| schemes | One canonical lowercase scheme and optional aliases |
| capabilities | stat, read, list, search, locate, write, edit, remove, move, watch as supported |
| selectorMode | lines, provider or none |
| backing | file, virtual, remote or device |
| scope | session, project, user or external; operation may narrow further |
| contentKinds | text, binary, image, container |
| mutations | Operation-specific side-effect class and required policy; immutable providers omit these |
| snapshots | stableRevision, rangedRead, compareAndSwap and expiry capabilities |
| display | Default preset/view labels and URI completion hints |
| limits | Provider maximum page bytes/lines, query time and concurrency, bounded by host |
| querySchema | Optional closed query-parameter schema; unknown parameters rejected |

Scheme grammar is `[a-z][a-z0-9+.-]*`, maximum 32 bytes. Registration reserves canonical name and aliases atomically. Builtin schemes including file, ssh, http, https, history, session, branch, skill, skills, jobs, proc, artifact, local, conflict, cfg, harness, self and workbench cannot be shadowed. An optional integration adapter can implement a reserved builtin only through a separately approved host configuration, never via normal register.

```json
{"v":1,"id":"register-build","sessionId":"s-1","generation":"g-1","from":{"owner":"build-tools","instance":"i-1"},"method":"register","params":{"kind":"resourceProvider","key":"builds","descriptor":{"label":"Build results","description":"Inspect build status and logs","schemes":["build"],"capabilities":["stat","read","list"],"selectorMode":"lines","backing":"virtual","scope":"project","contentKinds":["text","container"],"snapshots":{"stableRevision":true,"rangedRead":true,"compareAndSwap":false},"limits":{"pageBytes":51200,"pageLines":2000,"concurrency":4}}}}
```

Registering a provider does not expose a new model tool automatically. Existing read/ls/find/grep/write/edit route supported operations; the catalog informs agents which schemes exist. Unsupported operations return unsupported with advertised alternatives rather than guessing local paths.

# 3. URI and selector grammar

Parse in order:
1. For local file targets, probe the entire literal spelling before peeling selector suffixes. Preserve actual filenames with colons/commas/spaces.
2. Recognize scheme/authority; do not confuse an SSH port, URL fragment or query colon with a line selector.
3. Split explicitly recognized Workbench selector from the path portion only, before a query/fragment. Example `build://412/log:20-40?stream=stderr`.
4. Decode the resource path exactly once according to the provider's declared URI grammar. Keep raw requested spelling for diagnostics. Normalize scheme case, not arbitrary case-sensitive paths.
5. Canonicalize through provider stat/read metadata; compare authorization against canonical target too.

Selectors: `:N`, `:N-`, `:A-B`, `:A+C`, `:-N`, comma-separated ranges and `:raw` before/after a numeric selection. A lone non-raw N means onward; N beside raw or inside a comma list means exactly that line (`:17:raw`, `:19,59`). Use `:17-:raw` for raw onward. Positive one-based integers only; reject overflow/reversed/empty ranges. `:1-N` is head. Named selectors are provider capabilities, not an excuse to reinterpret unknown typos. A single-target explicit `selector` field handles ambiguous URLs; any suffix plus explicit selector fails invalidRequest. Detailed selection and adapter contract: [SPEC-0006](0006-advanced-read.md); snapshot/edit contract: [SPEC-0005](0005-hashline-editing.md).

For opaque HTTP query/fragment and literal virtual path ambiguity, clients use an explicit selector field or percent-encoded path delimiters. A provider cannot treat encoded `..` or `/` differently between authorization and execution.

`paths:[]` is the unambiguous multi-read form. Delimited-path convenience applies only after literal resolution fails and all components have an unambiguous parse; otherwise return an ambiguity error. Do not split arbitrary URI queries on commas or whitespace.

Local context expansion is one preceding/three trailing lines for single non-raw ranges; tails and multi-ranges are exact. Semantic block context may add separately marked spans for real code resources. Raw suppresses anchors/context but not hard safety budgets. Shared pager owns line/byte caps and exact continuation semantics, unless provider mode owns paging explicitly.

# 4. Execution envelope and adapters

The resource service resolves provider, authorizes operation, captures context, then dispatches `resource.execute` over provider-request (or the same typed interface locally). Provider reply uses provider-reply and the common response shape.

Request includes:
- registrationId, operationId, request ID, target owner/instance, session/generation.
- operation, requestedUri, normalizedUri, selector/query, cursor and limits.
- captured location revision, project identity, branch/context binding where applicable.
- expectedRevision for mutations/consistent continuation; idempotencyKey for mutations.
- authorization decision reference issued by the router; not a bearer token supplied by a model.
- content/edit payload for mutations, bounded; no private host credentials.

```json
{"v":1,"id":"read-1","sessionId":"s-1","generation":"g-1","to":{"owner":"build-tools","instance":"i-1"},"registrationId":"reg-build","method":"resource.execute","params":{"operationId":"op-1","operation":"read","requestedUri":"build://412/log:20-40","uri":"build://412/log","selector":{"kind":"lines","ranges":[{"start":20,"end":40}],"raw":false},"limits":{"bytes":51200,"lines":2000},"expectedRevision":"log-7"}}
```

This is the operation payload after router context/policy validation; when required for that provider, project/location/authorization references are included as negotiated fields. They are routing facts, not proof against a malicious loaded extension.

Conceptual Go boundary: `Provider.Describe()`, `Provider.Execute(context, ResourceRequest) (ResourceResponse, error)`; typed discriminated request/response variants enforce each operation. Public wrappers provide convenient Stat/Read/List/etc without inventing separate semantics. Final compilable types belong to implementation; no dependence on internal Workbench packages by external authors.

# 5. Operations and results

| Operation | Required semantics |
|---|---|
| stat | Canonical URI, content kind/media type, revision if stable, optional bytes/lines, operation capabilities |
| read | Bounded content blocks or text page; source/derived view identity and coverage |
| list | Typed entries (name, canonical URI, kind, optional size/revision), opaque nextCursor |
| search | Pattern/literal mode + declared options, exact matches/coordinates when known, incomplete/cursor facts |
| locate | A constrained backing-resource reference, never a blanket permission grant |
| write | Replace/create according to mode and expectedRevision; explicit receipt |
| edit | Structured snapshot mutation or provider-supported edits, not interpreted shell text |
| remove/move | Exact targets, revision preconditions and receipt; permission for both move endpoints |
| watch | Change subscription with invalidation events, cleanup and stat-based recovery |

Response includes `canonicalUri,kind,revision?,view,capabilities` and operation-specific payload. Text page includes `text,shownRanges,totalLines?,selectionComplete,resourceComplete,truncated?,nextCursor?`. Derived view coordinates identify `coordinateSpace`; real source spans are separate. A header/summary/table cannot masquerade as original file lines.

```json
{"v":1,"id":"read-1","sessionId":"s-1","generation":"g-1","to":{"owner":"pig-workbench","instance":"wb-1"},"ok":true,"result":{"canonicalUri":"build://412/log","kind":"text","revision":"log-7","view":{"kind":"verbatim","coordinateSpace":"source"},"page":{"text":"compiler started\ncompiled app\n","shownRanges":[{"start":20,"end":21}],"totalLines":21,"selectionComplete":true,"resourceComplete":false}}}
```

This example reaches EOF at line 21: the requested selection is complete, but earlier file lines were not emitted, so resourceComplete remains false. Containers and queries use opaque cursors, not fake file-line offsets. Cursor is bound to provider generation, resource/query and snapshot revision. Mutation invalidates old cursors; provider returns staleRevision instead of silently skipping/duplicating rows. Append-only logs can offer a separate append cursor with explicit consistency semantics.

Partial first lines caused by a huge line are marked incomplete; do not mark them fully seen. UTF-8 boundaries are preserved. Unknown totals stay absent. Batch output has one shared budget, deterministic input order and per-target errors; one failure does not discard successful reads. Search/find do not recursively enumerate an unbounded virtual catalog without provider support or explicit traversal bounds.

# 6. Policy and mutation integrity

Capabilities describe possible operations, not authorization. Workbench enforces tool permission, path containment, resource limits and canonical target before dispatch. Provider enforces its own backing-system rules. Controls such as `jobs://id/kill`, `cfg://key/save` and `harness://reload` require declared handler mutation capabilities; no implicit write-to-any-URI.

locate results:
- Canonical resource reference or approved backing file reference with revision and permitted operations.
- Router reauthorizes the backing target.
- Provider-returned aliases/redirects are bounded (max 8), cycle-checked and reauthorized on each step.

Read-only execution (SQL/query/converters) must not cause hidden writes/downloads/credential forwarding. External networks require explicit provider/policy disclosure. Preview UI never runs mutations. Errors redact secrets without losing actionable target/operation information.

Mutation receipt:
`operationId,idempotencyKey,status,canonicalUri,beforeRevision?,afterRevision?,effects,changedResources,warnings?`.
Status is committed, notApplied, partial or unknown. Retrying an idempotency key with identical payload returns retained receipt; different payload is conflict. Provider must state retention/durability guarantees; without durable idempotency, timeout-after-write is outcomeUnknown and requires reconciliation, not automatic retry. A cancellation after commit returns committed with cancellation notice, not notApplied.

Cross-provider move/transaction is unsupported unless an explicit coordinator can give truthful partial outcomes. No all-or-nothing claim from sequential rename/write calls.

# 7. Hashline and provenance

Full hashline editing requires:
1. Canonical source identity and stable revision over exact bytes.
2. Lossless source text and stable line coordinates.
3. Snapshot retention with actually shown fully emitted spans.
4. A mutation backend that validates the expected revision immediately before commit, with declared concurrency guarantees.

A short displayed tag is not the provider revision or security hash. Bind it to URI + snapshot bytes + source coordinate map. A provider's JSON/table/summary view does not make hidden underlying lines seen. Derived views need an explicit reversible mapping and original bytes; otherwise they are read-only for hashline purposes. Plain text virtual resources can opt into snapshot editing only with a stable write contract.

Shared syntax/edit core operates on validated snapshots, not provider-specific hashline dialects. External providers implement snapshot read and revision-checked commit; host retains full grammar, recovery, registers and diagnostics. If genuine compare-and-swap cannot be supplied, declare the weaker locking guarantee and revalidation limitation; never label a blind remote upload atomic against external writers.

# 8. Change notifications, lifecycle and limits

Provider emits resources.changed with registration, canonical URI/prefix, change kind and new revision if known. Invalidate stat/parse/query caches and instruction candidate sets as appropriate; a notification does not directly inject instruction content. Watch loss recovers by stat/re-read at operation/turn boundaries. Invalidations are hints, not substitutes for edit revision checks.

Unregister stops admission, cancels/drains outstanding requests and closes watches. Retained results remain renderable without the provider; new reads of its scheme fail unavailable. No silent fallback to interpreting `build://...` as a local filename. Reload creates new registration/cursor generations; no stale reply may satisfy a new call.

Page bytes default 50 KiB, lines 2000; control envelope 64 KiB. Provider chunks including JSON overhead must fit control budget (escape expansion counts); lower text payload or chunk explicitly. Adapter may negotiate a separate bounded page envelope, never assume the host's large framing ceiling is an output budget. Query/converter time, archive expansion and binary/image sizes require separate caps, reported in provider descriptor. Progress updates coalesce; final replies cannot be dropped for progress.

# 9. Builtin routes to preserve

| Family | Contract surface |
|---|---|
| session/history/branch | Current session, any session by ID, project and all-session catalogs and search, active branch, full/entry projections (grammar below) |
| skills/skill | Enabled trust-resolved skills and contained supporting assets; no hidden skill rediscovery |
| jobs/proc | Owned status/log/cursors and explicit stdin/EOF/cancel handler mutations |
| file/ssh | Explicit authority overrides active location; local/remote tool parity |
| artifact/local | Session-scoped overflow/scratch, bounded retention and containment |
| conflict | Registered source-version-bound conflict views and validated resolution writes |
| cfg | Inspect and confirmed settings updates; no trust widening |
| harness/self/workbench | Status, instruction inspection/pins and deferred safe reload |
| http/https | Static content with network policy, safe redirects and format adapters |

## session:// grammar

| URI | Target |
|---|---|
| `session://` | Current session: overview and entry table |
| `session://{id}` | Any of the user's sessions, any project; full ID or unique prefix |
| `session://project` | Catalog of the current project's sessions, most recent first |
| `session://project/{id}` | A session of the current project; `notInProject` if it belongs elsewhere |
| `session://all` | Catalog of all sessions, grouped by project cwd |

`project` and `all` are reserved segments. Suffixes `/full` and `/entry/{entryId}` select the transcript or one entry; `:N-M`/`:-N` page entries; `?q=` searches a catalog's names and message text. Every session of the user is readable; there is no consent step or scope gate.

Project membership: the project root is the top level of the main worktree (parent of `git rev-parse --git-common-dir`) when the cwd is in a git repository, otherwise the session cwd. A session belongs to the project when its header cwd is the root or lies beneath it, so sessions started in subdirectories and in worktrees nested inside the project (for example `.worktrees/feat-x`) count as the same project. Catalogs come from `SessionManagerListAll` filtered by that rule; `allMessagesText` feeds search and is never returned whole, because one transcript can exceed the output budget.

Catalog `read harness://schemes` and `/workbench resources` show active registrations, aliases, capabilities, owner, limits and availability. This adds an inspection route, not a second provider administration system. Completion uses descriptors/cached bounded list results, never synchronous network discovery on every keystroke.

# 10. Acceptance matrix

- External build:// provider loads before/after Workbench, reads via read and codemode, lists via ls, is visible in schemes/settings, unregisters safely.
- Duplicate/reserved/alias collision, invalid capability combinations and unsupported operations fail atomically.
- SSH ports, encoded traversal, query punctuation, literal filenames, raw ranges, huge lines, tail and disjoint selections.
- Paging consistency across append/mutation/reload; unknown totals and cursor expiry surfaced.
- `session://`, `session://{id}`, `session://project/{id}` and `session://all?q=` resolve correctly, including a session started in a nested worktree counting as the project's.
- Full hashline source seen-state vs derived view false anchors; read-only resource cannot be edited through aliases.
- Mutation idempotency, stale revision, cancellation-before/after commit and provider disconnect preserve honest outcomes.
- Slow/noisy provider bounded concurrency and progress; no render-loop provider calls.
- Watch invalidation loss reconciled by stat; no automatic model instructions from untrusted changed content.
- Same semantics for direct Go providers and foreign bus adapter in isolated/fused host runs.

Host blockers remain SPEC-0001's bus cancellation/lifecycle guarantees. Backend-dependent rich read behavior remains in PLAN-0001; this contract adds extensibility without replacing full builtin reader scope.
