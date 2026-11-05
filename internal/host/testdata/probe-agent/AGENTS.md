# probe-agent fixture

Independent Go factory used by Phase A host probes to observe SDK behaviour that Slice 1 contracts depend on.
- No model inference of its own and no network access; `/probe` drives `SendMessage`, `SetSessionName`, `GetSessionName`, `GetEntries` and `GetSessionFile`.
- Answers arrive as a single JSON notification whose `probe` field names the observation.
- Findings belong in `.project/analyses/2026-11-05-slice-1-probes/`; keep this fixture minimal and observation-only.
