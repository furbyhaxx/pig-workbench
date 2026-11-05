# api

Public Workbench v1 wire types and optional client integrations (SPEC-0001).
- Keep JSON shapes versioned, strict and SDK-independent where possible. Every exported symbol is documented.
- Capability discovery reports only implemented services; absence is not a successful empty result.
- No mutex held across host Emit; correlate before emitting and discard unmatched late replies.
- Changing envelopes requires reviewing all service specs and consumer fixtures.
- `client.go` is the optional shared-bus consumer: subscribe first, correlate by request/session/generation/identity, never retry mutations blindly, never echo transport payloads. Integration fixture: `internal/host/testdata/service-client/`.
- `provider.go` is the contribution side: `Client.Register/Unregister`, `Client.Serve` (answers `workbench:v1:provider-request` addressed to this client, replies on `provider-reply`), and the typed `ResourceRequest/ResourceResult`, `FetchRequest/FetchResult` payloads. Reference fixture: `internal/host/testdata/reference-producer/`; change a payload → update that fixture and `internal/host/live_reference_test.go`.
