# Workbench internals

Feature packages keep domain logic independent of the SDK; `host/` adapts contexts and registrations.
- `fileio/` — nonblocking regular-file admission shared by config and resource readers.
- `config/` — scoped TOML, catalog validation, immutable snapshots and serialized safe saves.
- `lifecycle/` — admission, owned cancellation and shutdown joins.
- `location/` — immutable local locations, mutation leases and transactional transitions.
- `resource/` — ToolPath selectors, provider router (file/local/out) and bounded source paging.
- `search/` — bounded grep and glob walking with honest completeness.
- `host/` — SDK lifecycle, commands and tool wiring; no duplicated domain policy.
- Do not retain request-scoped SDK contexts in session state. Read captured configuration/location once per operation.
