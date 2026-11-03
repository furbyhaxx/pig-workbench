# Workbench internals

Feature packages keep domain logic independent of the SDK; `host/` adapts contexts and registrations.
- `fileio/` — nonblocking regular-file admission shared by config and resource readers.
- `config/` — scoped TOML, catalog validation, immutable snapshots and serialized safe saves.
- `lifecycle/` — admission, owned cancellation and shutdown joins.
- `location/` — immutable local locations, mutation leases and transactional transitions.
- `resource/` — ToolPath selectors, provider router (file/local/out) and bounded source paging.
- `search/` — bounded grep and glob walking with honest completeness.
- `syntax/` — tree-sitter summaries and block spans (bounded cache, byte-confident coordinates).
- `hashline/` — byte model, XXH32 tags, tagged rendering and ending-safe restore.
- `frames/` — session-wide frame ledger: dedup, bounds, replay, per-producer views.
- `instructions/` — instruction class discovery, root+nearest selection, caps and rendering.
- `ask/` — questionnaire schema, validation and pending-record recovery.
- `project/` — `Project.toml` document model and creation.
- `secrets/` — deterministic placeholders, key handling and fail-closed redaction.
- `jobs/` — process supervision (see its AGENTS.md before touching process groups).
- `endpoint/` — live-session endpoint records and liveness probing.
- `host/` — SDK lifecycle, commands and tool wiring; no duplicated domain policy.
- Do not retain request-scoped SDK contexts in session state. Read captured configuration/location once per operation.
