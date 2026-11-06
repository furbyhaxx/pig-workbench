# messaging

SDK-free transport and limits for messages between live sessions of one user (SPEC-0011). Host policy and delivery live in `internal/host/sessions.go`, not here.
- Frame: 4-byte big-endian length plus one JSON object, at most 256 KiB; body at most 32 KiB, 16 attachments. A connection is one `hello` (token, protocol major) then one request, then close.
- A wrong token, a missing `hello` or an oversized frame closes the connection without a reply. Linux also checks the peer UID with SO_PEERCRED; other platforms rely on the 0700 directory and the token. Tokens never appear in logs or errors.
- `Listen` replaces a stale socket file but never a socket that answers. Socket paths over 100 bytes are refused (`sun_path`).
- `Limiter` holds per-peer sliding windows (messages per minute, wakes per hour); it knows no policy defaults.
- Receipts: `accepted` with a delivery of `woken`, `steered`, `delivered` or `queued`, or `rejected` with a reason.
- Change to the envelope or receipt → also update `api/sessions.go`, `docs/extension-services.md` and SPEC-0011.
