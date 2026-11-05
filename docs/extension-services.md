# Extension services

Workbench exposes an optional v1 service bus for cooperating PiG extensions. Stock PiG never requires this protocol, and Workbench does not mask or intercept other extensions' tools.

## Channels

| Channel | Direction | Content |
|---|---|---|
| `workbench:v1:request` | consumer → Workbench | validated `api.Request` |
| `workbench:v1:reply` | Workbench → consumer | addressed `api.Reply` |
| `workbench:v1:event` | Workbench → listeners | `ready`, `closing`, `location.changed`, `settings.changed`, `registration.changed` |
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

`Discover` needs exactly one responder: duplicate Workbench instances are ambiguous rather than silently picked. A capability you request must be advertised, so absence fails instead of returning an empty success.

Create the client while the extension factory runs or from `OnLoaded`, and share it read-only across handlers. Replies are matched by request ID, session, generation and this client's owner/instance. Discovery may precede a generation; every other method must carry the current one, and a stale generation fails with `staleGeneration`. After `ready` or session replacement, discover again.

## Semantics you can rely on

- `client.Call` performs no retries. Mutations returning a `*TransportError` have outcome `unknown` once emitted; reconcile by request ID or query state instead of repeating blindly.
- `timeoutMs` is a cooperative budget (except `discover`, which only negotiates). The shared bus cannot preempt a hung listener.
- `ErrUnavailable` means no Workbench answered within the call; use your own fallback rather than assuming its services ran.
- Plaintext never appears in transport-error messages.
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

`secrets` and `sessions` are specified but not advertised.

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
- Resource providers are read-only in this version: descriptors advertising `write`, `edit`, `remove` or `move` are refused `unsupported`, and Workbench's `edit`/`write` never send anything to a provider. Reads go through Workbench's selector grammar and pager; your page reports `shownRanges`, optional `totalLines` and completeness, and Workbench checks the line count before rendering.
- Fetch backends: the most specific rule wins (exact host, longer `*.suffix`, `*`, then longer `pathPrefix`, then registration order). Answer with `body` or `locate` (a URI of your own resource provider). A 4xx status is final; unavailability, errors or a third-party egress you did not declare fall back to the builtin static reader with a notice.
- Frame producers publish under their registered dotted key as source prefix, always with data authority. `followUp` wake is not granted; `nextBoundary` is steered into the running turn, `nextPrompt`/`beforePrompt` arrive with the next prompt. Frames bind to the current branch and context epoch.
- `context.fields.set` binds `knowledge_cutoff` to the current model; a mismatched model fails `staleRevision` and a model change hides the value until you set it again.
- `status.update` is coalesced to 4 updates per second; the receipt says `composedByWorkbench: true`, so stop your own `SetStatus`.

`location.get` and `describe` are safe to reconcile from any later request. Reconcile mutating calls through their status methods before retrying.

## Reference producer

`internal/host/testdata/reference-producer/` is a runnable consumer example for presentation, read-only resources, fetch, status, docs and frames. The live integration suite co-loads it with Workbench using a scripted inference provider; it performs no billed inference.

The fixture's `/reference status` reconciles its registration receipts with the live registry and reports its generation. `/reference reload` calls the host's `ctx.Reload()`; the live test verifies two successive reloads, fresh handles and exactly one registration for each of its six contribution kinds, plus successful rediscovery by an independent consumer. Recreate clients/listeners for the replacement factory and close old clients on shutdown; package-level `sync.Once` initialization can retain a disconnected client across reload. These fixture commands are not shipped Workbench commands. Never hold application state locks across synchronous bus calls or unsubscribe calls. Synchronous status updates use the active service-request context, not the retained session-start context, so reload startup cannot deadlock on its own host call lane.
