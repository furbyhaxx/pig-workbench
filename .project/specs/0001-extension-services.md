---
document:
  type: "specification"
  id: "SPEC-0001"
  title: "Shared extension services and event protocol"
  description: "Discovery, identity, request/reply, registrations, lifecycle and common safety boundaries"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-02T18:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Define the public Workbench extension APIs before implementation"
      changes: ["Initial contract, lifecycle, examples and acceptance criteria"]
    - revision: 2
      reason: "M0.2 host probes measured bus ordering, cancellation and reload"
      changes: ["Recorded observed bus semantics", "Added explicit cancel method and late-reply rule", "Made reload fire-and-forget with re-registration on load", "Replaced resolved M0 blockers with remaining host issues"]
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

Observed runtime ([host integration probes](../analyses/2026-11-02-host-integration-probes/README.md) P1–P4, P13–P14): a reply emitted inside the receiver's listener reaches the caller's reply listener before the caller's Emit returns, both when packed and across processes. Aborting the caller's request returns its Emit with `host call events.emit cancelled with its parent request`, but the receiving listener keeps running and still emits its reply. A second Emit reaches a listener that is still busy.

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

All requests except discover carry generation. IDs are unique per caller instance, at most 128 UTF-8 bytes. Objects use camelCase fields, RFC3339 timestamps when needed, nonnegative safe JSON integers (at most 2^53−1); larger offsets use decimal strings. No secrets, function references or SDK contexts in payloads.

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
| register | `kind: frameProducer|toolPresentation|resourceProvider`, stable contribution `key`, descriptor → registrationId + accepted descriptor revision |
| unregister | registrationId → removed (idempotent for same owner/generation) |
| describe | optional kind/key, cursor → bounded registration descriptors and nextCursor |
| settings.get | registrationId → effective presentation settings and sources |
| operation.status | operationId → receipt state for admitted deferred work |
| cancel | requestId of the caller's own pending request → no reply; idempotent |

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

Read-only diagnostics are exposed at `harness://integrations` (registrations), `harness://schemes` (provider catalog) and `harness://frames` (bounded frame receipt index; `/<frameId>` for detail). They mirror describe/list/status without exposing private session payloads beyond the caller's permissions. `/workbench integrations` and `/workbench resources` are their user-facing inspectors. Provider settings expose availability and configured limits only; capability declarations cannot be edited into new permissions.

# 5. Events, persistence and reliability

Event envelope: `{v,sessionId,generation,eventId,type,registrationId?,data}`. Event IDs are unique in the generation. Types initially: ready, closing, registration.changed, settings.changed, frames.deliveryChanged, resources.changed.

Events are notifications, not transactional acknowledgements. Describe/get/status APIs recover state after a missed notification. No event subscriber can veto another service's successful operation. Capture/report listener errors without changing committed facts.

Persist contribution settings and replayable result/frame facts, not active handles, providers, credentials or executable callbacks. Replay can render retained data without re-running the producer. Different generations cannot answer old pending requests. Request IDs deduplicate in-flight calls; retrying a mutation after an unknown outcome requires its operation ID/status, never blind re-execution.

# 6. Go authoring API direction

A future public `api` package defines typed descriptor/envelope/result types, a `Client` with Discover/Register/Unregister/Describe and service methods, and a provider adapter. These are **new Workbench APIs**, not existing SDK methods. Every operation accepts cancellation context; Close unsubscribes and drains.

Bundled providers use typed direct interfaces with identical validation and policy. Other extensions use the bus adapter even if fused into the same process. Renderers are deterministic functions over retained presentation data; rendering never calls the bus.

Public SDK wrappers must return typed errors, expose capability absence, document lifecycle, and include a working small producer example after implementation. Final Go signatures are frozen with runnable compile tests during M0/M1; this specification freezes wire semantics first.

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
