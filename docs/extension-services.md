# Extension services

Workbench exposes an optional v1 service bus for cooperating PiG extensions. Stock PiG never requires this protocol, and Workbench does not mask or intercept other extensions' tools.

## Channels

| Channel | Direction | Content |
|---|---|---|
| `workbench:v1:request` | consumer → Workbench | validated `api.Request` |
| `workbench:v1:reply` | Workbench → consumer | addressed `api.Reply` |
| `workbench:v1:event` | Workbench → listeners | `ready`, `closing`, `location.changed`, `settings.changed` |

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

Only `location` is implemented: `location.get` returns the immutable local working location. Frame, resource, status, snapshot, fetch, session, docs and secrets methods are specified but not available, and are not advertised.

`location.get` is safe to reconcile from any later request. When Workbench later ships mutating services, reconcile them through their status methods before retrying.
