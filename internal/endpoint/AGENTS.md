# endpoint

SDK-independent live-session registry; records and local directory are private and tokens must never appear in diagnostics or UI.
- Registry files are `<sessionID>.json` with `startedAt` and `heartbeatAt`, as required by this package's contract; SPEC-0011 §2 instead specifies endpoint-ID filenames and `processStart`/`stateSince`.
- Pruning uses the PID probe only. SPEC-0011 §2 also requires a socket-connect probe before declaring an endpoint dead; that probe is added with the transport unit and MUST be consulted before this function is wired to automatic cleanup.
- Liveness is conservative on probe errors; never remove or replace a record whose owner cannot be proved dead. Never remove malformed records automatically.
- Windows registry is unavailable until ACL validation exists; never silently use a directory whose privacy is unverified. SPEC-0011 §10 records Windows as compile-level only for this slice.
