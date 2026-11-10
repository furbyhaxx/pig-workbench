---
document:
  type: "specification"
  id: "SPEC-0001"
  title: "Shared extension services and event protocol"
  description: "Discovery, identity, request/reply, registrations, lifecycle and common safety boundaries"
  status: "published"
  revision: 14
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-10T11:30:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 14
      reason: "Consumed analyses and the proposal draft are deleted; their facts are stated in place"
      changes: ["Links to deleted analyses replaced by the fact or its new location", "Host probe table P1–P30 added as §9"]
---

# 1. Status, boundaries and terminology

Normative **proposed Workbench v1 contract**, pending review and M0 runtime probes. MUST/SHOULD describe implementation requirements, not shipped behavior. The factory currently registers no services. Authorizing this specification work does not approve dependency additions or host modifications.

Companions: [frames](0002-message-frames.md), [presentation](0003-tool-presentation.md), [resources](0004-resource-providers.md). Design: [extension integration UX](../sketches/extension-apis/README.md). Plan: [PLAN-0001](../plans/0001-pig-workbench.md).

- **Owner:** stable extension identity, e.g. `build-tools`.
- **Instance:** opaque random identifier for one owner runtime incarnation.
- **Generation:** Workbench-issued incarnation for a host session/runtime binding.
- **Registration:** a session-generation-scoped contribution handle; not an executable callback encoded in JSON.
- **Client:** optional Workbench adapter owned by another extension.
- **Local API:** direct typed Go calls for bundled services. Shared-process packing does not authorize sharing globals between unrelated extensions.

Observed runtime (host probes (§9) P1–P4, P13–P14): a reply emitted inside the receiver's listener reaches the caller's reply listener before the caller's Emit returns, both when packed and across processes. Aborting the caller's request returns its Emit with `host call events.emit cancelled with its parent request`, but the receiving listener keeps running and still emits its reply. A second Emit reaches a listener that is still busy.

Verified source: staged SDK `event_bus.go`, `tool_renderer_resolvers.go`, `tool_renderers.go` under `host:/home/furbyhaxx/.pig/state/pigsdk/sdk-899c7964784a0050`; bundled `extension-api.md` and `runtime-cells.md`. PiG's bus carries JSON across Go realms, listeners finish before Emit returns, and listener errors are not application replies. These constraints determine this protocol.

# 2. Optional integration and discovery

A producer registers its normal PiG tools/commands regardless of Workbench. Failure to discover Workbench MUST NOT disable those tools. Presentation falls back to the producer/host; frame-dependent features report unavailable rather than injecting a raw user message; schemes exist only while a provider and Workbench are both available.

Channels:
- `workbench:v1:request`: client → Workbench control/service calls.
- `workbench:v1:reply`: Workbench → addressed client replies.
- `workbench:v1:event`: ready, closing, registration/settings changes, frame delivery changes.
- `workbench:v1:provider-request`, `provider-reply`, `provider-progress`: Workbench ↔ registered resource providers.

All listeners filter session, generation, recipient and correlation before acting. Subscribe before discovery; unsubscribe on shutdown. Ready is an optimization, not the only handshake: a late-loading client calls `discover`. Do not rely on factory load order or periodic polling.

`discover` sends the caller's supported major versions and requested capabilities. Response advertises selected major/minor, Workbench version, Workbench instance identity, sessionId, generation, supported capabilities and negotiated limits. Providers address replies to that discovered instance. Require exact major agreement; minor versions add optional features only. Callers ignore unknown response fields; unknown methods and unsupported capabilities fail explicitly. Unknown request fields are rejected unless enabled by a negotiated minor feature. No v0/donor-channel compatibility layer is required.

```json
{"v":1,"id":"req-1","sessionId":"s-1","from":{"owner":"build-tools","instance":"i-1"},"method":"discover","params":{"majors":[1],"capabilities":["frames","presentation","resources"]}}
```

```json
{"v":1,"id":"req-1","sessionId":"s-1","generation":"g-1","to":{"owner":"build-tools","instance":"i-1"},"ok":true,"result":{"major":1,"minor":0,"workbenchVersion":"development","instance":"wb-1","capabilities":["frames","presentation","resources"],"limits":{"controlBytes":65536,"pageBytes":51200,"pageLines":2000,"inFlightPerOwner":8}}}
```

No reply after Emit completes means service unavailable, not an empty successful result. Multiple valid discovery replies for different Workbench generations mean ambiguous service ownership; refuse attachment and show a diagnostic.

# 3. Request/reply and execution

All requests except discover carry generation. IDs are unique per caller instance, at most 128 UTF-8 bytes. Objects use camelCase fields, RFC3339 timestamps when needed, nonnegative safe JSON integers (at most 2^53−1); larger offsets use decimal strings. No secrets, function references or SDK contexts in payloads, except plaintext input to the explicitly opted-in `secrets.redact` method (§8). That exception is visible to shared-bus listeners and MUST NOT be logged or retained by Workbench.

Common fields:
| Field | Meaning |
|---|---|
| v, id, sessionId, generation | Protocol and binding identity |
| from | owner + instance |
| method, params | Closed method name and method-specific typed object |
| timeoutMs | Positive requested execution budget, default 30000, max 300000 |
| registrationId | Required for operations on registered contributions |

Reply has exactly one of `ok:true,result` or `ok:false,error`. Errors contain `code,message,retryable`, optional `field` and `outcome` (`notApplied|applied|unknown` for mutations). Codes: unavailable, incompatibleVersion, invalidRequest, conflict, permissionDenied, unsupported, staleGeneration, notFound, staleRevision, busy, limitExceeded, cancelled, deadlineExceeded, providerFailed, outcomeUnknown, internal.

```json
{"v":1,"id":"req-2","sessionId":"s-1","generation":"g-1","to":{"owner":"build-tools","instance":"i-1"},"ok":false,"error":{"code":"staleGeneration","message":"Rediscover Workbench before registering.","retryable":true,"outcome":"notApplied"}}
```

## Synchronous bus adapter, explicit limitations

1. Caller installs reply correlation before emitting.
2. Receiver validates, performs bounded work under the current request context, then emits the reply before returning.
3. Reply listener only copies/validates the result into the pending slot and returns. It MUST NOT synchronously wait for the original request to finish.
4. Caller consumes the reply after Emit returns. Nested provider calls follow the same pattern. No mutex may be held across Emit or another host call.
5. Bulk resource operations run under the tool/request handler; provider progress can emit during execution. Workbench's progress listener updates caches and returns, never re-enters the provider.
6. Reload and queued frames return an admission receipt, not a fabricated completion.

TimeoutMs is a cooperative execution budget, not a promise to preempt an arbitrary bus listener. Host cancellation releases only the caller (P3); it never reaches the receiving listener. Therefore:

- `cancel` is an explicit method: `{"method":"cancel","params":{"requestId":"…"}}`, sent by the original caller after its Emit was cancelled or its own deadline passed. It needs no reply.
- A receiver doing more than bounded validation polls a per-request cancellation flag set by `cancel` (observed latency: one poll interval) and stops with `cancelled`. Work that may outlive the caller's budget is not run in the listener: admit it, reply with an operation receipt and run it under the session supervisor.
- A reply arriving after the caller gave up is discarded by correlation ID. Mutations cancelled mid-flight report `outcome: unknown` via `operation.status`.

A listener that hangs can block Emit; abandoning it in an unowned goroutine is forbidden. No automatic second IPC daemon.

No background handler retains the request Context after return. Frame queues, reload work and registration maintenance belong to a session supervisor with shutdown draining and independent context. Do not advertise such deferred operations until that lifecycle is proven.

# 4. Registrations and ownership

Methods:
| Method | Parameters/result |
|---|---|
| discover | Version/capability negotiation |
| register | `kind: frameProducer|toolPresentation|resourceProvider|fetchBackend|statusSegment|docs`, stable contribution `key`, descriptor → registrationId + accepted descriptor revision |
| unregister | registrationId → removed (idempotent for same owner/generation) |
| describe | optional kind/key, cursor → bounded registration descriptors and nextCursor |
| settings.get | registrationId → effective presentation settings and sources |
| operation.status | operationId → receipt state for admitted deferred work |
| cancel | requestId of the caller's own pending request → no reply; idempotent |
| location.get | → current location (§8) |
| resources.changed | registrationId of a `resourceProvider`, `{uri?, change, revision?}` → `{invalidated}`; drops retained cursors under the prefix and re-emits the event; a hint, not a revision substitute (SPEC-0004 §8) |
| status.update | registrationId of a `statusSegment`, text and style → accepted (SPEC-0009 §9) |
| snapshots.view / snapshots.status / snapshots.touched | Edit-tracking service for other tools (SPEC-0005 §10) |
| sessions.list / sessions.send | Live-session messaging (SPEC-0011 §8) |
| secrets.redact | Opt-in extension inference redaction (SPEC-0013 §4.5); no secret lookup or substitution |
| context.fields.set | registrationId of a `frameProducer` → replace its declared context fields (SPEC-0014 §5.1) |

Register validates the whole descriptor atomically. Identical register for (owner, instance, kind, key) is idempotent. Changed descriptor requires explicit replace using registrationId and previous descriptorRevision; invalid replacement leaves the old descriptor intact. Cross-owner collisions never silently replace registrations. Resource aliases participate in collision detection too.

Stable contribution keys survive reload for settings identity; runtime registration IDs do not. Owner identity is cross-checked with host tool source metadata where available. **The shared bus does not provide authenticated sender identity in its JSON payload.** Instance IDs are routing identifiers, not secrets or security tokens. Trusted extensions already execute code; this protocol prevents accidents and validates policy, but is not a sandbox against a hostile loaded extension. External/untrusted transports require a separate authenticated bridge.

```text
detached --discover--> attached --register--> active
   ^                       |                  |
   |                       +--closing---------+
   +------ shutdown / generation change ------+
```

Workbench emits ready after session initialization, closing before disposal when possible. Reload re-invokes the factory in the same process (P13): package-level state survives and is not a reset point (P14), so generation and registry state live on the extension instance, never in globals. `ctx.Reload()` did not return in 1 of 28 runs while the old instance kept serving; nothing may wait on it. Clients re-register from the new instance's ready event, and a client that sees no ready re-discovers on its next request. Client clears cached registrations on generation change and re-registers once. Explicit unregister drains active requests before disposal; new requests fail unavailable. Runtime crash cleanup requires host lifecycle notification or discovery reconciliation; a surviving orphan registration fails closed on dispatch and becomes unavailable. Do not claim guaranteed crash detection from bus events alone.

A bounded registration registry defaults to 128 contributions per owner, descriptor size 64 KiB and 8 in-flight provider operations per owner. Busy/limitExceeded errors name the limit. Settings can reduce limits; any increase requires supported configured capacity.

Read-only diagnostics are exposed at `cfg://workbench/integrations/<owner>` (registrations), `docs://workbench/schemes/<scheme>` (provider catalog and guides) and `session://self/frames` (bounded frame receipt index; `/<frameId>` for detail). They mirror describe/list/status without exposing private session payloads beyond the caller's permissions. `/workbench integrations` and `/workbench resources` are their user-facing inspectors. Provider settings expose availability and configured limits only; capability declarations cannot be edited into new permissions.

# 5. Events, persistence and reliability

Event envelope: `{v,sessionId,generation,eventId,type,registrationId?,data}`. Event IDs are unique in the generation. Types initially: ready, closing, registration.changed, settings.changed, frames.deliveryChanged, resources.changed, location.changed, sessions.message.

Events are notifications, not transactional acknowledgements. Describe/get/status APIs recover state after a missed notification. No event subscriber can veto another service's successful operation. Capture/report listener errors without changing committed facts.

Persist contribution settings and replayable result/frame facts, not active handles, providers, credentials or executable callbacks. Replay can render retained data without re-running the producer. Different generations cannot answer old pending requests. Request IDs deduplicate in-flight calls; retrying a mutation after an unknown outcome requires its operation ID/status, never blind re-execution.

# 6. Go authoring API direction

A future public `api` package defines typed descriptor/envelope/result types, a `Client` with Discover/Register/Unregister/Describe and service methods, and a provider adapter. These are **new Workbench APIs**, not existing SDK methods. Every operation accepts cancellation context; Close unsubscribes and drains.

Bundled providers use typed direct interfaces with identical validation and policy. Other extensions use the bus adapter even if fused into the same process. Renderers are deterministic functions over retained presentation data; rendering never calls the bus.

Public SDK wrappers must return typed errors, expose capability absence, document lifecycle, and include a working small producer example after implementation. Final Go signatures are frozen with runnable compile tests during M0/M1; this specification freezes wire semantics first.

## 6.1 Implemented surface

`api` (module `github.com/furbyhaxx/pig-workbench/api`) is the frozen authoring surface:

| Symbol | Purpose |
|---|---|
| `NewClient(*sdk.Extension) (*Client, error)` | Subscribes to replies before any request can emit; the extension owns `Close`. |
| `Client.Identity() Identity` | This client's `owner`/`instance`, the address Workbench uses for provider calls. |
| `Client.Discover(ctx, capabilities...) (Discovery, error)` | Exactly one compatible responder, or `ErrUnavailable`/`ErrAmbiguous`/`*Error`. |
| `Client.Call(ctx, endpoint, method, params, result, *CallOptions) error` | One synchronous, never-retried call; `*TransportError` carries the outcome. |
| `Client.Register/Unregister` | Contribution admission and withdrawal with a `Receipt`. |
| `Client.Serve(ext, ProviderHandler) (stop func(), error)` | Answers `resource.execute` and `fetch` requests addressed to this client. |
| `ProviderRequest.ReportProgress(ctx, seq, message, completed, total)` | Advisory `provider-progress` note for one in-flight request; newest `seq` wins, bounded to 200 characters, never answered. |
| `Client.Close()` | Unsubscribes once; does not preempt an Emit already running elsewhere. |
| Typed payloads | `Discovery`, `Limits`, `Receipt`, `ResourceRequest/Result`, `TextPage`, `ResourceEntry`, `Selector`, `LineRange`, `FetchRequest/Result`, `ProviderRequest`, `Error`. |

`api/contract_test.go` compiles a full producer flow (client, serve, register, describe, unregister) against the staged SDK and pins every documented JSON name, so a rename cannot silently break a consumer.

## 6.2 Migration notes by revision

| Revision | Change | Consumer action |
|---|---|---|
| 1–2 | Contract and observed bus semantics; explicit `cancel`; fire-and-forget reload | Subscribe before discover; re-register from the new generation's `ready`. |
| 3–4 | Versioned request/reply, generation binding, capability negotiation | Send `generation` on everything but `discover`; rediscover after `staleGeneration`. |
| 5 | Resource providers became read-only for this version | Do not advertise `edit`/`write`/`remove`/`move`; they are refused at registration. |
| 6 | Opt-in `secrets.redact` and owner-scoped context fields recorded as approved decisions | Nothing until the capability is advertised; requesting it fails today. |
| 7 | Authoring API frozen; `snapshots`, `sessions` and `secrets` remain unadvertised | Request only advertised capabilities; absence fails explicitly and is not an empty success. |
| 7 (impl.) | `snapshots` advertised with `view`, `status`, `touched` per SPEC-0005 §10; `view` anchors local files only | Additive; no wire change. Caller ownership is the registering extension name of the running tool. |

# 7. Acceptance and blockers

- Discovery before/after Workbench load, absent service, incompatible major, duplicate service, reload and session switch.
- Identical/changed/colliding registrations; atomic failed replace; stale handles cannot mutate new generation.
- JSON payloads from Go and another supported runtime behave identically; no function-valued compatibility assumptions.
- Nested control/provider request and progress/reply handlers do not deadlock; shutdown joins all owned work.
- Deadline/cancel and provider failure produce typed results; unknown mutation outcomes are never presented as safely retryable.
- Response limits, malformed envelopes, counters, duplicate IDs and lost notifications exercised.
- Replay works with producer absent; settings remain dormant rather than being deleted.
- Cancel after caller abort stops a busy listener and the late reply is discarded (packed and isolated).
- Reload hang: Workbench state stays consistent when `ctx.Reload()` never returns.
- Remaining host issues (not blockers): intermittent `ctx.Reload()` hang; no request-scoped listener cancellation in the bus. Owner identity still cannot be authenticated from bus payloads.

# 8. Collaboration services and answers to integrating extensions

Capabilities a client can request in `discover`, beyond `frames`, `presentation` and `resources`: `location`, `status`, `snapshots`, `fetch`, `sessions`, `docs` and `secrets`. Each is optional; an unsupported capability fails explicitly. Owner-scoped context fields are part of `frames`.

**Location (`location`).** The `discover` result includes `location` when requested: `{authority, cwd, displayCwd, projectRoot, project?, origin, revision}`. `project` is `{id, slug, component}` from SPEC-0012 §3.3, so clients use the same project identity as Workbench; it is absent in the builtin `global` project. `location.get` returns the same object. `location.changed` events carry `{previous, current, origin, revision, reason}`. Config roots are still the host session cwd (SPEC-0010 §1); the location is for scoping, not for loading config.

**Secrets (`secrets`).** Ordinary agent turns and tool calls use SPEC-0013's boundary without producer registration. Extension-owned inference is different: no stock-host hook intercepts it. The user approved an explicit opt-in `secrets.redact` service for trusted extensions (SPEC-0013 §4.5). It returns placeholders, never real values, and does not make the shared bus a private channel. Research child sessions without Workbench remain outside automatic protection; their owner must redact each request itself.

**Context fields (`frames`).** A `frameProducer` may declare typed fields on `session.model` and replace its own contribution through `context.fields.set` (SPEC-0014 §5.1). It cannot overwrite core facts, impersonate an internal source, or change another owner's contribution. Workbench composes and delivers the single context frame.

**Status segments (`status`).** See SPEC-0009 §9.

**Edit tracking (`snapshots`).** See SPEC-0005 §10.

**Fetch backends (`fetch`).** See SPEC-0004 §11.

**Sessions (`sessions`).** See SPEC-0011 §8.

**Documentation (`docs`).** Register kind `docs` with `{key, title, version, root, source?}`, where `root` is a directory inside the owner's package or data directory and `source` is `{repo, ref, paths}` (sparse checkout on first access) or `{dir}`. It appears read-only as `docs://<owner>/` and `docs://<owner>/source/` (SPEC-0012 §5.4).

**Scheme guides.** A `resourceProvider` descriptor may carry `guide`: Markdown of at most 2 KiB plus optional per-sub-namespace guides `{path, markdown}`, and a `guideRevision`. Workbench injects it on first touch (SPEC-0012 §5.5).

## Answers to pig-web (its SPEC-0001 §3.7)

| Request | Answer |
|---|---|
| R1 location at attach time | Accepted: `location` capability above |
| R2 registry timing | Accepted: the resolver reads the live registry when the host first asks about a tool name, not a snapshot from Workbench's `session_start`; see SPEC-0003 §5 |
| R3 `search` preset with URI rows | Accepted: table cells are typed and include `uri` (SPEC-0003 §3) |
| R4 per-authority scope | Withdrawn by pig-web (central KB); removed from SPEC-0004 |
| R5 secret contribution | Superseded: SPEC-0013 needs no contribution |
| R6 unreadable secret sources | Superseded: SPEC-0013 §2 detects `.env*` files and resolves `auth.json` through the host |
| R7 fetch backend shape | SPEC-0004 §11 |
| R8 project identity in requests | SPEC-0004 §4: `project` present when known, absent for `global` |
| Edit tracking (dropped by pig-web) | Offered as the `snapshots` service (SPEC-0005 §10) |
| `https://` reads (dropped by pig-web) | Offered as `fetchBackend` contributions: Workbench keeps routing, policy and paging; pig-web supplies content for the URLs it handles (SPEC-0004 §11) |

# 9. Host facts from probes

Observed on 02.11.2026 in an isolated PiG home with a scripted mock provider (no billed inference), against the staged SDK `sdk-899c7964784a0050`; packed and isolated-process producers both ran. Other specifications cite these rows by number. Secrets probes S1–S6 are in SPEC-0013 §9, prompt probes PR1–PR8 in SPEC-0014 §11. Kit views were not probed.

| # | Seam | Observed |
|---|---|---|
| P1 | Bus request/reply, packed | Works synchronously: reply listener runs inside the receiver's Emit, before the caller's Emit returns |
| P2 | Bus request/reply, isolated process | Same ordering across PIDs |
| P3 | Caller abort during a pending Emit | Caller's Emit returns `host call events.emit cancelled with its parent request`; the **listener keeps running** to completion and its late reply is still delivered |
| P4 | Explicit cancel message | A second Emit (`wb.cancel`) after abort reaches the busy listener (both packed and isolated); listener stops within one poll interval |
| P5 | Frame delivery modes | See §3 |
| P6 | Codemode child observation | Children emit `tool_execution_start/end` and `tool_call` with `parentToolCallId`; IDs are `call_0_0/1`… |
| P7 | Blocked codemode child | `tool_call` block → no `tool_result` event, but `tool_execution_end` fires; script sees `{"status":"rejected","reason":{}}` — block reason lost |
| P8 | Renderer resolver | Resolver claims foreign (`producer_echo`) and builtin (`read`) tools; render width = terminal − 2 (98 @100, 38 @40) |
| P9 | Codemode nested rows | Codemode draws children itself; our resolver is **not** consulted for nested rows |
| P10 | Dynamic renderer claim | Resolver result is cached per tool; changing the claim at runtime has no effect until `/reload` |
| P11 | Claim after `/reload` | Resolver re-runs, history re-renders with new renderer; the historical call row received **empty args** (`map[]`) while the live row had args (observed in one run) |
| P12 | Branch-scoped custom entries | `AppendEntry` entries follow the branch: tree navigation drops later entries, clone keeps the branch, `new_session` starts empty |
| P13 | `ctx.Reload()` from a command | Factory re-invoked, call returns, branch state intact in 27/28 runs. **1/28 hang**: no `loaded`, no return, no RPC response; old instance kept serving commands; root cause in P29 |
| P14 | Package globals across reload | Survive (same process); not a reset point |
| P15 | Same-name override of `read`/`bash` | `RegisterTool` with the builtin's name replaces it (source = extension); wrapping `def.Execute` delegates to the host original without recursion, directly and from codemode children |
| P16 | Update streaming through override | `tool_execution_update` events still flow (11 for a 3-tick bash) |
| P17 | Cancellation through override | Abort cancels the delegated call (`host call builtinTool.execute cancelled…`), kills the shell child (no leftover process) and ends the row as error |
| P18 | Location without host chdir | The factory `cwd` argument is ignored when a context exists (tools use the session cwd). Resolving relative paths before delegating (`read`) and `BashToolOptions.SpawnHook` setting `Cwd` (`bash`) work; `ctx.Cwd()` stays the session cwd |
| P19 | Idle delivery | While idle, `steer`, `followUp` and unknown `deliverAs` values append a persisted custom message without starting a turn; `nextTurn` stays in an in-memory queue until the next prompt; `TriggerTurn: true` starts a turn; `SendUserMessage(…, "followUp")` starts a turn; an invalid `deliverAs` is not rejected |
| P20 | Stale delivery after tree navigation | A `nextTurn` message queued before `NavigateTree` is delivered on the **new** branch; an idle `steer` stays on the old branch |
| P21 | Compaction | `session_before_compact`/`session_compact` fire; summary requests do **not** pass through the `context` hook; afterwards the hook sees `compactionSummary` + kept messages, and the re-added ambient message appears exactly once per request. Custom entries survive on the branch. Small sessions fail with `Nothing to compact` (reported only via `OnError`/`compaction_end`) |
| P22 | Resume (`--continue`) | `session_start.reason` is `startup`, not `resume`; branch, compaction and custom entries are restored; context files reload from the cwd |
| P23 | Host context-file suppression | Removing a file from `data["systemPromptOptions"]["contextFiles"]` in `before_agent_start` removes it from the rendered system prompt for that prompt (must be repeated every prompt). Returning a `systemPrompt` string is the wrong lever: the value is a `*sdk.SystemPrompt` |
| P24 | Session catalog | `SessionManagerList` (project) and `SessionManagerListAll` (all projects) work from any extension, including `--no-session` runs; entries include `allMessagesText` (full transcript text). `GetSessionFile` is nil in-memory while `GetSessionID` is set |
| P25 | Skill/command catalog | `GetSystemPromptOptions().Skills` gives enabled skills with path, scope and `disableModelInvocation`; `GetCommands` lists extension and skill commands (not builtin), with source info; `IsProjectTrusted` and `ConfigHome` work |
| P26 | `/tools` command | Registers without collision, appears in TUI autocomplete, runs in TUI and RPC; `GetAllTools` reports source and exposure per tool |
| P27 | `--list-tools` flag | Parsed in every mode (`GetFlag` = true in `session_start`); `ctx.Shutdown()` there prevents the prompt. **No output channel in print/JSON mode**: `Notify` is dropped, extension stdout/stderr are not forwarded |
| P28 | Dialogs | RPC emits `extension_ui_request` (select/input/confirm) and resumes on `extension_ui_response`; unanswered requests block indefinitely unless `DialogOptions.Timeout` is set. Print and JSON modes return `ok=false` / `confirm=false` immediately (`HasUI()=false`) — indistinguishable from a user cancel except via `HasUI` |
| P29 | Reload hang root cause | 2/40 in a dedicated loop. Host goroutine blocks in `Host.Reload → stagePackedGo → startGoPackedCellOnce → acceptPackedExt → awaitConnection` with no deadline; the packed cell's re-dial gets `connection refused` on the old socket path, and the new instance's `events.on` fails |
| P30 | Messages from `before_agent_start` | Returning `{"message": …}` adds a persisted custom message right after the user prompt; `SendMessage(steer)` from the same hook lands before the prompt |

Print mode (P27): `pig -p "/tools"` and `pig -p --list-tools` run the extension code but produce no stdout in text mode and only the session header in JSON mode. A flag can change behaviour but cannot print a listing until the host has an output channel.
