# Extension services

Workbench exposes an optional v1 service bus for cooperating PiG extensions. Stock PiG never requires this protocol. Workbench does not replace other extensions' tools, but its secrets hooks can substitute tool inputs and redact tool results at the shared host boundary.

## Channels

| Channel | Direction | Content |
|---|---|---|
| `workbench:v1:request` | consumer → Workbench | validated `api.Request` |
| `workbench:v1:reply` | Workbench → consumer | addressed `api.Reply` |
| `workbench:v1:event` | Workbench → listeners | `ready`, `closing`, `location.changed`, `settings.changed`, `registration.changed`, `resources.changed`, `sessions.message` |
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

- `client.Call` performs no retries. Mutations returning a `*TransportError` have outcome `unknown` once emitted; reconcile by request ID or query state instead of repeating blindly.
- `timeoutMs` is a cooperative budget. The shared bus cannot preempt a hung listener.
- `ErrUnavailable` means no Workbench answered within the call; use your own fallback rather than assuming its services ran.
- `TransportError.Error()` omits request payloads; its unwrapped cause is separate and must be handled carefully.
- A request above 64 KiB fails `limitExceeded` locally before emitting.
- `Close` unsubscribes; it does not preempt a call already running in another handler.

## Advertised capabilities today

| Capability | Methods | Contribution kind |
|---|---|---|
| `location` | `location.get` | — |
| `presentation` | `register`, `unregister`, `describe` | `toolPresentation` (SPEC-0003 §2) |
| `resources` | Workbench → you: `resource.execute` (`stat`, `read`, `list`) | `resourceProvider` (SPEC-0004 §2), read-only |
| `fetch` | Workbench → you: `fetch` | `fetchBackend` (SPEC-0004 §11) |
| `status` | `status.update` | `statusSegment` (SPEC-0009 §9) |
| `docs` | — | `docs` (served at `docs://<owner>/`) |
| `frames` | `frames.publish`, `frames.status`, `frames.get`, `frames.cancel`, `frames.list`, `context.fields.set` | `frameProducer` (SPEC-0002 §2, SPEC-0014 §5.1) |
| `snapshots` | `snapshots.view`, `snapshots.status`, `snapshots.touched` | — |
| `sessions` | `sessions.list`, `sessions.send` | — (advertised when the session endpoint is available) |
| `secrets` | `secrets.redact` | — (advertised only to granted owners) |

`sessions.list` returns the discoverable live sessions of the same user (no tokens). `sessions.send` takes `{target, channel, subject, body, attachments, wake}`. Channel `ext:<your owner>` delivers a `sessions.message` event (`api.SessionsMessageEvent`) on the event channel of the target session; it never wakes the agent and never becomes a frame. Channel `agent` delivers to the model as a peer-attributed data frame, untrusted unless that peer has been trusted. The answer is the synchronous receipt (`accepted` with `woken`, `steered`, `delivered` or `queued`, or `rejected` with a reason). Sending on another owner's channel is `permissionDenied`.

## Redact your own inference requests

An extension that calls a model itself bypasses Workbench's hooks. If the user lists your owner name in `secrets.redact-owners` (user or runtime settings only), `discover` advertises `secrets` to you and `secrets.redact {text}` returns `{text, enabled: true}` with the same placeholders as an ordinary turn. Redact every model-visible field before each request.

- Any failure (disabled detection, unavailable key or vault, text too large for the 64 KiB envelope) is an error that never echoes your input. Withhold the inference; absence of the service is not permission to send plaintext.
- The owner name is a claim on a shared bus, not an authenticated identity: the grant guards against misrouting, not against a hostile extension.
- Placeholders are never resolved back to values through this service.

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

- Identical registration is idempotent. A changed descriptor needs `register` with `registrationId` and `params.previousRevision`; a failed replacement restores the previous descriptor.
- Registrations, scheme claims and context-field claims are exclusive per generation. Reserved and retired schemes, overlapping frame source prefixes and second presentation owners for one tool fail `conflict` or `invalidRequest`.
- Resource providers are read-only in this version: descriptors advertising `write`, `edit`, `remove` or `move` are refused `unsupported`, and Workbench's `edit`/`write` never send anything to a contributed provider. Reads go through Workbench's selector grammar and pager; your page reports `shownRanges`, optional `totalLines` and completeness, and Workbench checks the line count before rendering. `find` walks paged provider listings within depth, entry and time bounds; directory entries may set `noWalk: true` to exclude expensive subtrees unless explicitly selected. Links and control files are never followed. `grep` does not yet traverse contributed provider roots.
- Fetch backends: the most specific rule wins (exact host, longer `*.suffix`, `*`, then longer `pathPrefix`, then registration order). Answer with `body` or `locate` (a URI of your own resource provider). A 4xx status is final; unavailability, errors or a third-party egress you did not declare fall back to the builtin static reader with a notice.
- Frame producers publish under their registered dotted key as source prefix, always with data authority. `followUp` wake is not granted; `nextBoundary` is steered into the running turn, `nextPrompt`/`beforePrompt` arrive with the next prompt. Frames bind to the current branch and context epoch.
- `context.fields.set` binds `knowledge_cutoff` to the current model; a mismatched model fails `staleRevision` and a model change hides the value until you set it again.
- `status.update` is coalesced to 4 updates per second; the receipt says `composedByWorkbench: true`, so stop your own `SetStatus`.

`location.get` and `describe` are safe to query again. After an unknown mutation outcome, query an appropriate status method when available or reconcile the affected state before retrying; there is no general `operation.status` method.

## Reference producer

`internal/host/testdata/reference-producer/` is a runnable consumer example for presentation, read-only resources, fetch, status, docs and frames. The live integration suite co-loads it with Workbench using a scripted inference provider; it performs no billed inference.

The fixture's `/reference status` reconciles its registration receipts with the live registry and reports its generation. `/reference reload` calls the host's `ctx.Reload()`; the live test verifies two successive reloads, fresh handles and exactly one registration for each of its six contribution kinds, plus successful rediscovery by an independent consumer. Recreate clients/listeners for the replacement factory and close old clients on shutdown; package-level `sync.Once` initialization can retain a disconnected client across reload. These fixture commands are not shipped Workbench commands. Never hold application state locks across synchronous bus calls or unsubscribe calls. Synchronous status updates use the active service-request context, not the retained session-start context, so reload startup cannot deadlock on its own host call lane.

## Guides on first touch

When the model first reads a contributed scheme in a session, Workbench appends that scheme's guide to the read result once. The guide is not repeated on later reads, and it is injected again after the owner re-registers the scheme with a new descriptor. The same text is always available on demand at `docs://workbench/schemes/<scheme>` and `docs://<owner>/`.
