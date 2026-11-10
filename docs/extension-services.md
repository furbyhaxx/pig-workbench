# Extension services

Workbench exposes an optional v1 service bus for cooperating PiG extensions. Stock PiG never requires this protocol. Workbench does not replace other extensions' tools, but its secrets hooks can substitute tool inputs and redact tool results at the shared host boundary.

## Channels

| Channel | Direction | Content |
|---|---|---|
| `workbench:v1:request` | consumer → Workbench | validated `api.Request` |
| `workbench:v1:reply` | Workbench → consumer | addressed `api.Reply` |
| `workbench:v1:event` | Workbench → listeners | `ready`, `closing`, `location.changed`, `settings.changed`, `registration.changed`, `resources.changed`, `frames.deliveryChanged`, `sessions.message` |
| `workbench:v1:provider-request` | Workbench → contribution owner | `api.ProviderRequest` addressed to your client identity |
| `workbench:v1:provider-reply` | contribution owner → Workbench | `api.Reply` addressed to Workbench |

## Use the Go client

```go
client, err := api.NewClient(ext)
if err != nil { return err }
defer client.Close()

endpoint, err := client.Discover(ctx, "location")
if err != nil { return err }        // unavailable, ambiguous or missing capability

var location map[string]any
if err := client.Call(ctx, endpoint, "location.get", map[string]any{}, &location, nil); err != nil {
    return err                         // *api.Error for protocol failures
}
```

`Discover` needs exactly one responder: duplicate Workbench instances are ambiguous rather than silently picked. A capability you request must be advertised, so absence fails instead of returning an empty success. The `ready` event lists the common capabilities only; use `discover` for endpoint availability and owner-specific secrets grants.

Create the client while the extension factory runs or from `OnLoaded`, and share it read-only across handlers. Replies are matched by request ID, session, generation and this client's owner/instance. Discovery may precede a generation; every other method must carry the current one, and a stale generation fails with `staleGeneration`. After `ready` or session replacement, discover again.

## Semantics you can rely on

- `client.Call` performs no retries. Mutations returning a `*TransportError` have outcome `unknown` once emitted; reconcile by request ID or query state instead of repeating blindly. When a call ends without a reply (emit failure, no responder, caller deadline) the client also sends a best-effort `cancel` carrying that request ID on `workbench:v1:cancel`, so a cooperative Workbench handler stops; a reply that arrived before the failure is kept as the call's answer.
- `timeoutMs` is a cooperative budget. The shared bus cannot preempt a hung listener.
- `ErrUnavailable` means no Workbench answered within the call; use your own fallback rather than assuming its services ran.
- `TransportError.Error()` omits request payloads; its unwrapped cause is separate and must be handled carefully.
- A request above 64 KiB fails `limitExceeded` locally before emitting.
- Control and provider replies share `api.DecodeReply`: one non-null success result or one actionable error, never both. Malformed/oversized replies are ignored; missing trustworthy mutation receipts remain unknown. Unknown outcomes cannot carry a safe-retry flag.
- `Close` unsubscribes; it does not preempt a call already running in another handler.

## Advertised capabilities today

| Capability | Methods | Contribution kind |
|---|---|---|
| `location` | `location.get`; also `discover` with `location` and `location.changed` events. Each location carries `project {id, slug}` when the session is in a recognised project, and omits it in the builtin `global` project | — |
| `presentation` | `register`, `unregister`, `describe`, `settings.get` | `toolPresentation` (SPEC-0003 §2) |
| `resources` | Workbench → you: `resource.execute` (`stat`, `read`, `list`, `search`, declared mutations); you → Workbench: `resources.changed` | `resourceProvider` (SPEC-0004 §2) |
| `fetch` | Workbench → you: `fetch` | `fetchBackend` (SPEC-0004 §11) |
| `status` | `status.update` | `statusSegment` (SPEC-0009 §9) |
| `docs` | — | `docs` (served at `docs://<owner>/`) |
| `frames` | `frames.publish`, `frames.status`, `frames.get`, `frames.cancel`, `frames.list`, `context.fields.set`; event `frames.deliveryChanged` (`api.FrameDeliveryChanged {frameId, source, eventKey, state}`) with your `registrationId` once a provider request carried your frame. It is a notification: `frames.status` recovers a missed one and reports the other states | `frameProducer` (SPEC-0002 §2, SPEC-0014 §5.1) |
| `snapshots` | `snapshots.view`, `snapshots.status`, `snapshots.touched` | — |
| `sessions` | `sessions.list`, `sessions.send` | — (advertised when the session endpoint is available) |
| `secrets` | `secrets.redact` | — (advertised only to granted owners) |

`resources.changed` (`Client.ResourcesChanged`) takes `{uri?, change, revision?}` with your `registrationId`: `change` is `created`, `modified`, `removed` or `reset`, and an empty `uri` covers the whole registration. Workbench drops retained listing cursors under that prefix and emits a `resources.changed` event. It is a hint, never a substitute for revision checks: a missed notification still ends in `staleRevision` from your provider, because every retained cursor carries the revision it was issued at. The URI must belong to one of the registration's schemes.

`sessions.list` returns the discoverable live sessions of the same user (no tokens). `sessions.send` takes `{target, channel, subject, body, attachments, wake}`. Channel `ext:<your owner>` delivers a `sessions.message` event (`api.SessionsMessageEvent`) on the event channel of the target session; it never wakes the agent and never becomes a frame. Channel `agent` delivers to the model as a peer-attributed data frame, untrusted unless that peer has been trusted. The answer is the synchronous receipt (`accepted` with `woken`, `steered`, `delivered` or `queued`, or `rejected` with a reason). Sending on another owner's channel is `permissionDenied`.

## Redact your own inference requests

An extension that calls a model itself bypasses Workbench's hooks. If the user lists your owner name in `secrets.redact-owners` (user or runtime settings only), `discover` advertises `secrets` to you and `secrets.redact {text}` returns `{text, enabled: true}` with the same placeholders as an ordinary turn. Redact every model-visible field before each request.

- Any failure (disabled detection, unavailable key or vault, text too large for the 64 KiB envelope) is an error that never echoes your input. Withhold the inference; absence of the service is not permission to send plaintext.
- The owner name is a claim on a shared bus, not an authenticated identity: the grant guards against misrouting, not against a hostile extension.
- Placeholders are never resolved back to values through this service.

Resource and fetch requests use the project of the tool's captured Workbench location. A concurrent `cd` does not change the project binding between operations in a read batch or search traversal. Mutation location revisions use the same captured binding.

Provider listing replies keep their own opaque backend cursors. Workbench returns private session-bound handles to tools, binding registration, descriptor revision and resource identity without exposing backend cursor contents. Handles expire after five minutes or bounded-cache eviction (256 entries / 1 MiB). A retained listing revision is sent back as `ResourceRequest.expectedRevision`; providers must reject changed snapshots. Reload/replacement, wrong-resource and expired handles fail `staleRevision`, never empty success.

Presentation registration cross-checks every claimed tool against host source attribution. Unknown or mismatched ownership returns `permissionDenied` with `verificationRequired`, without a dialog or any change to tool execution. The shared bus remains unauthenticated between already-loaded extensions; this check prevents accidental claims, not hostile code impersonation. Because the host caches renderer selection on first use, registration/replacement/withdrawal replies expose `appliesAfterReload`, and `/workbench integrations` retains pending withdrawal diagnostics. Workbench never triggers reload automatically or calls foreign renderer markers as callbacks.

Presentation updates are admitted by opaque tool-call identity before repeated rendering. Older revisions and late running updates cannot resurrect settled calls; contradictory finals retain the previous settled outcome and add an integration diagnostic. Validated snapshots are retained in a bounded session cache (256 calls / 8 MiB payload accounting); plain layouts use a separate bounded 256-entry / 2 MiB cache keyed by data/settings revision, width and host flags. Renderer callbacks read retained data without producer I/O. Historical cards bootstrap once from retained Details when no data event populated the cache; unidentified cards cannot share lifecycle state safely.

`/workbench integrations <owner>` and its cfg resource include recent captured call metadata and lifecycle diagnostics, not duplicated source output.

## Query display settings

`settings.get` requires your own active presentation `registrationId` and empty params. It returns `api.DisplaySettings`: stable owner/key, config revision, reload applicability and declared display fields with scope/file provenance and application status. It does not expose producer execution configuration or another owner's registration. The reply is bounded to 48 KiB. `live` marks a control with an actual renderer consumer; `registered` still means integration is pending, not applied. Effective enabled/density/preview/metric settings are cached once per configuration revision without render-time I/O.

Dynamic fields also appear in `/workbench settings catalog tools.` and command completions. Removed fields remain unavailable in the user-facing catalog rather than being erased.

## Provider mutation outcomes

Existing-resource mutations carry `expectedRevision`; new resources carry `mustNotExist`, and copy/move destinations carry `destinationMustNotExist`. Writes are limited to 32 KiB of content and the 64 KiB encoded control envelope. No recursive operations, implicit parent creation or cross-provider transfers are supported. `write` and scheme-command redirects share dispatch; `rm`, `mv`, `cp` and `mkdir` map to remove, move, copy and mkdir.

Return an `api.MutationReceipt` identifying the operation/key, canonical target, per-target effects and changed resources. Status is committed, notApplied, partial or unknown. A malformed or lost receipt is unknown; an explicit notApplied error is preserved. Workbench never retries automatically. Retain identical-key/identical-payload receipts for the declared period (30 s–24 h); reject different payloads under the same key and disclose restart durability. Do not evict unexpired guarantees to admit new work. The authorization reference is router-issued correlation, not a security boundary between loaded extensions.

## Make your tool output editable

Inside a running call of one of your tools, `snapshots.view` makes Workbench read a local file through its own pipeline and return the tagged block (`[path#TAG]` plus `N:LINE` rows). Include `block` verbatim in your tool result and the model can `edit` those lines without a second `read`.

```go
var view api.SnapshotView
err := client.Call(ctx, endpoint, "snapshots.view", api.SnapshotViewParams{ToolCallID: ctx.ToolCallID(), URI: path}, &view, nil)
return sdk.ToolResult{Content: view.Block}, err
```

- `toolCallId` must be a running call of a tool registered by the calling owner, otherwise `permissionDenied`.
- Seen-line authority is granted only when the exact block survives in the delivered result; a cut or altered block grants nothing. A trailing annotation does not invalidate it.
- Only `file://` resources are anchored; other schemes answer `unsupported` because their edits are refused anyway.
- `snapshots.status {uri}` returns the retained tag and revision, the count of lines seen in this context, and whether the file changed since. `snapshots.touched {uris}` (1–64) returns the same revalidated state for files you changed; it is a hint, edits still check revisions.

## Register contributions

```go
receipt, err := client.Register(ctx, endpoint, "resourceProvider", "kb", descriptor)
stop, err := client.Serve(ext, func(ctx sdk.Context, req api.ProviderRequest) (any, *api.Error) {
    // req.Method is "resource.execute" or "fetch"; decode req.Params into
    // api.ResourceRequest or api.FetchRequest and return a result or *api.Error.
})
```

- `req.Context()` is the request's cooperative stop signal. It ends when the request's `timeoutMs` budget elapses or Workbench aborts the call (tool abort, caller deadline) and sends a cancel on `workbench:v1:provider-cancel`. `Serve` subscribes to that channel for you; a handler doing more than bounded validation polls `req.Context()` and returns a `cancelled` error. A mutation whose caller gave up reports `unknown` unless your error says `notApplied`.
- `req.ReportProgress(ctx, seq, message, completed, total)` sends an advisory note on `workbench:v1:provider-progress` while a long request runs. `seq` must increase; the message is at most 200 characters without control characters; `total` is 0 when unknown. Workbench keeps only the newest note per request, drops stale, foreign or malformed ones without replying, and shows the latest as `running N: message (completed/total)` on the provider's line in `/workbench resources`. Progress never changes the reply and is gone when the request ends.
- Unregistering or replacing a registration closes admission first (new calls fail `unavailable`, `notApplied`), sends the same cancel to every operation still running against it, and waits up to 5 seconds for them to end before the scheme leaves the router. An operation that is cancelled this way and gets no answer reports `unavailable` with outcome `unknown`; a handler that answers the cancel keeps its own outcome. A failed replacement reinstalls the previous descriptor and admits calls again.
- Identical registration is idempotent. A changed descriptor needs `register` with `registrationId` and `params.previousRevision`; a failed replacement restores the previous descriptor.
- Registrations, scheme claims and context-field claims are exclusive per generation. Reserved and retired schemes, overlapping frame source prefixes and second presentation owners for one tool fail `conflict` or `invalidRequest`.
- Providers may declare `write`, `mkdir`, `remove`, `copy` and `move` with `mutations: {revisionChecked:true, retentionMs:60000, durable:false}` and stableRevision/compareAndSwap support. Workbench dispatches typed `ResourceRequest.Mutation` preconditions and validates `ResourceResult.Receipt`, without new permission or confirmation prompts. Providers enforce revisions, absent destinations, nonrecursive semantics and backing-system permissions at commit. `edit` remains refused until provider snapshot editing is implemented. Reads go through Workbench's selector grammar and pager; your page reports `shownRanges`, optional `totalLines` and completeness. Unknown totals stay absent, while a known empty source reports zero. Negative/impossible totals, unrepresentable spans and contradictory complete-source coverage are rejected before rendering. `find` walks paged provider listings within depth, entry and time bounds; directory entries may set `noWalk: true` to exclude expensive subtrees unless explicitly selected. Links and control files are never followed. `grep` traverses bounded provider directories and explicit files with source-coordinate selectors.
- Declare `search` to answer `grep` and `find` yourself (SPEC-0004 §5.1). Workbench sends `operation: "search"` with `ResourceRequest.Search` (`mode` `content` or `names`, `patterns`, `literal`, `ignoreCase`, `glob`, `exclude`, `context`, `maxMatches`) and expects `ResourceResult.Matches` (`uri`, `line`, `column`, `text`, `revision`, `before`/`after`) plus `complete` (omit it only when the answer is whole). Matches outside the searched root, without a line, or beyond `maxMatches` fail the call; your order is kept, so rank by relevance. Answer `unsupported` for a query you cannot serve (for example a regex against a full-text index) and Workbench walks instead, reporting `via: "walk"`; any other error is returned. A grep with a line selector on one file is never sent to `search`.
- Fetch backends: the most specific rule wins (exact host, longer `*.suffix`, `*`, then longer `pathPrefix`, then registration order). Answer with `body` or `locate` (a URI of your own resource provider). A 4xx status is final; unavailability, errors or a third-party egress you did not declare fall back to the builtin static reader with a notice.
- Frame producers publish under their registered dotted key as source prefix, always with data authority. `nextBoundary` is steered into the running turn, `nextPrompt`/`beforePrompt` arrive with the next prompt. `wake: true` (default false, `nextBoundary` only) starts a turn when the session is idle; register the producer with `allowWake: true` or the publication fails `permissionDenied`. Each producer may wake `frames.wake-budget` times per hour; beyond that the frame is delivered without a turn. The receipt says `wake: "woken"`, `"steered"` (the session was already running) or `"suppressed"`, and a repeated publication returns `duplicate: true` and delivers nothing. Frames bind to the current branch and context epoch.
- `context.fields.set` binds `knowledge_cutoff` to the current model; a mismatched model fails `staleRevision` and a model change hides the value until you set it again.
- `status.update` is coalesced to 4 updates per second; the receipt says `composedByWorkbench: true`, so stop your own `SetStatus`.

`location.get` and `describe` are safe to query again. After an unknown mutation outcome, query an appropriate status method when available or reconcile the affected state before retrying; there is no general `operation.status` method.

## Reference producer

`internal/host/testdata/reference-producer/` is a runnable consumer example for presentation, read-only resources, fetch, status, docs and frames. The live integration suite co-loads it with Workbench using a scripted inference provider; it performs no billed inference.

The fixture's `/reference status` reconciles its registration receipts with the live registry and reports its generation. `/reference reload` calls the host's `ctx.Reload()`; the live test verifies two successive reloads, fresh handles and exactly one registration for each of its six contribution kinds, plus successful rediscovery by an independent consumer. Recreate clients/listeners for the replacement factory and close old clients on shutdown; package-level `sync.Once` initialization can retain a disconnected client across reload. These fixture commands are not shipped Workbench commands. Never hold application state locks across synchronous bus calls or unsubscribe calls. Synchronous status updates use the active service-request context, not the retained session-start context, so reload startup cannot deadlock on its own host call lane.

## Guides on first touch

When the model first reads a contributed scheme in a session, Workbench appends that scheme's guide to the read result once. The guide is not repeated on later reads, and it is injected again after the owner re-registers the scheme with a new descriptor. The same text is always available on demand at `docs://workbench/schemes/<scheme>` and `docs://<owner>/`.
