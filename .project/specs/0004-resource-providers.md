---
document:
  type: "specification"
  id: "SPEC-0004"
  title: "Resource provider API and URI contracts"
  description: "Pluggable schemes, selectors, operations, paging, policy and source provenance"
  status: "approved"
  revision: 10
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-09T15:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 10
      reason: "pig-web R10: a declared search capability was accepted but never called"
      changes: ["Defined the search request and result (§5.1)", "grep and find dispatch to a provider that declares search; an unsupported answer falls back to the bounded walk and is reported", "Provider search results are validated against the searched root and bounded like local matches"]
    - revision: 9
      reason: "Implement safe contributed listing cursor bindings"
      changes: ["Bound opaque tool cursor handles to registration/descriptor/resource identity without exposing backend payloads","Forwarded known listing revisions for consistent continuation","Verified resource mismatch, snapshot changes, reload expiry and bounded retention"]
---

# 1. Purpose and architectural boundary

Other extensions can register `scheme://` providers into Workbench's resource router. Builtin and external providers obey the same selectors, output budgets, permission policy and source-provenance rules. Provider APIs are semantic operations, not arbitrary shell commands or filesystem escape hatches.

Depends on [SPEC-0001](0001-extension-services.md). Rendering uses [SPEC-0003](0003-tool-presentation.md); notices use [SPEC-0002](0002-message-frames.md). Draft contract, not implemented SDK capability. Prior art: nugent `scheme/{tool_path,selector,pager,scheme_provider}.rs` and oh-my-pi `internal-urls/` under the donor paths in root AGENTS.md.

# 2. Provider registration

Register kind `resourceProvider` with:
| Field | Meaning |
|---|---|
| key, label, description | Stable owner-scoped identity and inspection text |
| summary | Required nonempty plain-text one-line summary, at most 120 Unicode characters; no control characters or terminal sequences. Used for each declared scheme in prompt/context catalogs (SPEC-0014 §3.4); never inferred from a guide or fetched resource |
| schemes | One canonical lowercase scheme and optional aliases |
| capabilities | stat, read, list, search, locate, write, edit, remove, move, copy, mkdir, watch as supported |
| selectorMode | lines, provider or none |
| backing | file, virtual, remote or device |
| scope | session, project, user or external; operation may narrow further. |
| contentKinds | text, binary, image, container |
| mutations | `{revisionChecked:true, retentionMs:30000..86400000, durable:boolean}`; required for mutations, omitted for immutable providers. Stable revisions and compareAndSwap are required. This is an integrity declaration, not a user permission prompt |
| snapshots | stableRevision, rangedRead, compareAndSwap and expiry capabilities |
| guide, guideRevision | Optional first-touch usage guide (SPEC-0001 §8, SPEC-0012 §5.5) |
| display | Default preset/view labels and URI completion hints |
| limits | Provider maximum page bytes/lines, query time and concurrency, bounded by host |
| querySchema | Optional closed query-parameter schema; unknown parameters rejected |

Scheme grammar is `[a-z][a-z0-9+.-]*`, maximum 32 bytes. Registration reserves canonical name and aliases atomically. Builtin schemes file, ssh, http, https, local, out, conflict, artifact, project, session, agent, job, cfg, skill and docs cannot be shadowed, nor can the retired names history, branch, proc, skills, jobs, harness, self, workbench and projects (they answer "unknown scheme" with the replacement, SPEC-0012 §5.1). Path segments `self` and `by-*` are reserved in every scheme. An optional integration adapter can implement a reserved builtin only through a separately approved host configuration, never via normal register.

```json
{"v":1,"id":"register-build","sessionId":"s-1","generation":"g-1","from":{"owner":"build-tools","instance":"i-1"},"method":"register","params":{"kind":"resourceProvider","key":"builds","descriptor":{"label":"Build results","description":"Inspect build status and logs","summary":"Build status and bounded logs","schemes":["build"],"capabilities":["stat","read","list"],"selectorMode":"lines","backing":"virtual","scope":"project","contentKinds":["text","container"],"snapshots":{"stableRevision":true,"rangedRead":true,"compareAndSwap":false},"limits":{"pageBytes":51200,"pageLines":2000,"concurrency":4}}}}
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
- captured location revision and branch/context binding where applicable.
- `project`: `{id, slug, component}` of the calling session (SPEC-0012 §3.3) in every `resource.execute` and `fetch` request whenever the session has a project; absent in the builtin `global` project, so "no project" never matches project-bound data. A routing fact, not proof.
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
| list | Typed entries (name, canonical URI, kind `file`/`dir`/`link`/`ctl`, optional size/revision, `link` target), provider-declared columns, opaque nextCursor |
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

Walking (SPEC-0012 §5.2): without provider `search`, `find` and `grep` walk with `list`+`read` under depth, entry, byte and time bounds and report incomplete results. They never follow `link` entries, skip `ctl` files and `noWalk` subtrees unless named directly, and treat `volatile` files as snapshots of the moment read. `immutable` files get no hashline anchors.

## 5.1 Search operation

A provider that declares `search` receives `grep` and `find` as `operation: "search"` instead of a `list`+`read` walk. The request carries `uri` (the searched root, canonical) and `search`:

```json
{"operation":"search","uri":"kb://articles/","limits":{"bytes":49152,"lines":1900},"search":{"mode":"content","patterns":["retry budget"],"literal":true,"ignoreCase":true,"glob":"*.md","exclude":["drafts"],"context":1,"maxMatches":200}}
```

- `mode` is `content` (grep) or `names` (find; `patterns` holds one glob, matched against names exactly as local `find`).
- `literal:false` means the patterns are regular expressions in Go RE2 syntax. A provider that cannot serve a flag combination (for example regex against a full-text index) answers `unsupported`.
- The result carries `matches[]`: `{uri, line?, column?, text?, revision?, kind?, size?, before?, after?}` and `complete`. Content matches have `uri`, `line` (1-based) and `text`; names matches have `uri` and optionally `kind` and `size`. `complete:false` means the provider knows its answer is partial; it must not be omitted when the provider hit its own limit.
- Workbench validates every match: the canonical URI must be inside the searched root, line numbers are positive, counts stay within `maxMatches`, text is sanitized and long lines are windowed. A violation fails the call naming the provider; it does not fall back silently.
- `unsupported` falls back to the bounded walk of §5. Any other error is returned. Details report `via: "provider"` or `via: "walk"`.
- Matches from a provider search are not seen-state: a later `read` of the URI is still needed before an `edit`.

# 6. Policy and mutation integrity

Capabilities describe possible operations, not authorization. Workbench enforces tool permission, path containment, resource limits and canonical target before dispatch. Provider enforces its own backing-system rules. Controls such as writes to `job://<id>/ctl`, `cfg://<group>/<key>` and `cfg://ctl` require declared handler mutation capabilities; no implicit write-to-any-URI.

locate results:
- Canonical resource reference or approved backing file reference with revision and permitted operations.
- Router reauthorizes the backing target.
- Provider-returned aliases/redirects are bounded (max 8), cycle-checked and reauthorized on each step.

Read-only execution (SQL/query/converters) must not cause hidden writes/downloads/credential forwarding. External networks require explicit provider/policy disclosure. Preview UI never runs mutations. Errors redact secrets without losing actionable target/operation information.

Mutation receipt:
`operationId,idempotencyKey,status,canonicalUri,beforeRevision?,afterRevision?,effects,changedResources,warnings?`.
Status is committed, notApplied, partial or unknown. Retrying an idempotency key with identical payload returns retained receipt; different payload is conflict. Provider must state retention/durability guarantees; without durable idempotency, timeout-after-write is outcomeUnknown and requires reconciliation, not automatic retry. A cancellation after commit returns committed with cancellation notice, not notApplied.

Cross-provider move/transaction is unsupported unless an explicit coordinator can give truthful partial outcomes. No all-or-nothing claim from sequential rename/write calls.

## 6.1 Implemented mutation wire contract

`resource.execute.operation` supports `write`, `mkdir`, `remove`, `copy` and `move` only when declared. Workbench adds no new confirmation or permission dialog: existing host/tool policy and the provider's own backing permissions remain in force. The router-issued authorization reference is correlation metadata, not a security token against other loaded extensions.

`api.ResourceRequest.mutation` carries `idempotencyKey`, `authorizationRef`, `locationRevision`, optional `expectedRevision`, `mustNotExist`, `destinationUri`, `destinationMustNotExist`, and `content` (write only, up to 32 KiB, subject to the encoded 64 KiB envelope). Workbench creates fresh operation/key references, preflights canonical targets and current revisions, and never retries. Existing-resource operations carry the preflight revision; explicit `write.expectedRevision` must match it. Creation requires absence. Providers enforce these preconditions at commit, including absent copy/move destinations. Copy is file-only; remove requires a file or empty directory; mkdir requires an existing parent; no recursive operations or cross-provider transfers. Links and control resources are not ordinary mutation targets. Existing builtin control handlers retain their own contracts.

The response is `api.ResourceResult.receipt` with operationId, idempotencyKey, status, canonicalUri, effects (uri/status and optional before/after revisions), changedResources and optional warnings. Correlation and target sets must match; committed moves identify both endpoints and committed copies identify the destination. A missing, contradictory or malformed receipt after dispatch is `unknown`. Explicit provider errors with outcome `notApplied` remain notApplied; other failed calls are unknown. Known committed receipts survive cancellation or transport failure after their delivery. No automatic retry or rollback follows partial/unknown outcomes.

Providers retain the identical-key/identical-payload receipt for their declared retention period, reject key reuse with different payloads, and state restart durability. Workbench has no general receipt lookup/retry service; after loss of a non-durable receipt the caller reconciles state. Stop admission rather than evict an unexpired idempotency guarantee. Providers enforce their backing-system authorization themselves, without Workbench inventing confirmation requirements.

Migration: read-only descriptors and requests are unchanged. Previously all contributed mutations were refused; new mutable providers must supply the typed integrity declaration and return receipts. `edit` remains refused at registration until the snapshot commit backend exists; declaring write does not implicitly enable edit.

## 6.2 Listing cursor implementation

Model-facing listing cursors are private session-bound handles, not encoded producer payloads. They bind registration ID, descriptor revision, generation, instance and requested/canonical resource identity. A bounded cache retains at most 256 handles and 1 MiB of cursor data, with five-minute validity; eviction, expiry and mismatched bindings return staleRevision. Stable opaque handle generation preserves repeated-cursor detection without disclosing backend cursor contents. A returned listing revision is forwarded as ResourceRequest.expectedRevision on continuation; missing or changed subsequent revisions are rejected. Producers keep their own backend cursor format and snapshot checks.

Migration: raw backend cursor strings no longer appear in tool output. Tool consumers must retain only the returned opaque handle and restart after staleRevision; readonly providers that report listing revisions receive the already specified optional expectedRevision field. No new permission or confirmation behavior is introduced.

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

# 9. Builtin routes

The builtin trees, link directories and project membership are specified in [SPEC-0012](0012-projects-uri-namespace.md) §3.3 and §5. They use this contract like any external provider. `ls docs://workbench/schemes/` and `/workbench resources` show active registrations, capabilities, owner, limits and availability. Completion uses descriptors and cached bounded list results, never synchronous network discovery on every keystroke.

# 10. Acceptance matrix

- External build:// provider loads before/after Workbench, reads via read and codemode, lists via ls, is visible in schemes/settings, unregisters safely.
- Duplicate/reserved/alias collision, invalid capability combinations and unsupported operations fail atomically.
- SSH ports, encoded traversal, query punctuation, literal filenames, raw ranges, huge lines, tail and disjoint selections.
- Paging consistency across append/mutation/reload; unknown totals and cursor expiry surfaced.
- The SPEC-0012 §5.3 trees resolve correctly, including `session://by-project/<slug>/` containing a session started in a nested worktree, and recursive tools visiting each session once.
- Full hashline source seen-state vs derived view false anchors; read-only resource cannot be edited through aliases.
- Mutation idempotency, stale revision, cancellation-before/after commit and provider disconnect preserve honest outcomes.
- Slow/noisy provider bounded concurrency and progress; no render-loop provider calls.
- Watch invalidation loss reconciled by stat; no automatic model instructions from untrusted changed content.
- Same semantics for direct Go providers and foreign bus adapter in isolated/fused host runs.

Host blockers remain SPEC-0001's bus cancellation/lifecycle guarantees. Backend-dependent rich read behavior remains in PLAN-0001; this contract adds extensibility without replacing full builtin reader scope.

# 11. Fetch backends for http(s)

`http`/`https` stay Workbench routes: URL parsing, selectors, network policy, redirect reauthorisation, content dispatch (SPEC-0006 G6.2), paging, caching and presentation are Workbench's. Another extension can contribute **content** for the URLs it understands better, through registration kind `fetchBackend`:

| Field | Contract |
|---|---|
| key, label, description | Stable identity and inspection text |
| match | Ordered rules `{host, pathPrefix?}`; `host` is exact or `*.suffix`; `"*"` is a catch-all |
| capabilities | `fetch`, optional `locate` (backing file of a stored snapshot), `offline` |
| egress | Upper bound: `none` or `thirdParty` (the backend may send the URL to third-party services such as reader APIs). Each answer reports `usedThirdParty`; an answer with `usedThirdParty: true` from a backend registered with `none` is rejected and reported |
| retention | Optional: `user` or `project` when the backend stores every fetched page beyond the request (e.g. a knowledge base). Shown in settings and on each result that it applies to |
| limits | Body bytes and time, bounded by Workbench |

Flow for `read https://…`:
1. Workbench parses and authorises the URL.
2. It picks the enabled backend with the most specific matching rule (host, then path prefix, then registration order). No match, or none enabled, uses the builtin static reader.
3. It sends `fetch` `{url, refresh, offline, accept, timeoutMs, maxBytes}` as a provider request.
4. The backend answers with either a body, or `locate` to a stored snapshot (preferred for large bodies). `locate` may be a backing file reference or a URI of a resource provider registered by the same owner (e.g. pig-web's `kb://sources/7c/7c41…`); Workbench reads it through the router once, under the caller's authorization, with no further `locate` hop from there, plus `{finalUrl, redirects[], status, contentType, charset, revision, handler, usedThirdParty, notices[]}`.
5. Workbench reauthorises `finalUrl` and every redirect hop, then applies its own pipeline to the content. The result names the backend and handler, and says when a third party saw the URL.

Failure handling: unavailable, timeout or a backend error (including a host the backend reports as cooling down) → builtin static reader, with a visible notice. This holds for catch-all (`"*"`) backends too. A definitive answer from the origin (404, 410, auth required) is final and does not fall back. A backend never receives credentials from Workbench, and Workbench never sends the URL to a backend that declares third-party egress when the user disabled that backend's egress in its own settings; the backend reports its effective policy in the descriptor. When that policy changes (e.g. remote readers switched on), the backend replaces its registration with the previous `descriptorRevision` (SPEC-0001 §4); in-flight fetches finish under the old descriptor.

Workbench's own HTTP cache applies to the builtin reader only. A backend that declares `retention` is its own cache; Workbench does not cache its answers again beyond the result page.

Settings: `/workbench settings resources › fetch backends` lists backends with enable toggles and order. A newly registered backend is enabled by default. Disabling it changes only `read https://`; the producer's own tools (e.g. `web_fetch`) keep working.
