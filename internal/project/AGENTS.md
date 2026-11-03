# project

`document.go` owns the SPEC-0012 §3.2 `.project/Project.toml` schema and create/load/parse API.
- Decode strictly: accept version 1, reject unknown keys, validate IDs, kebab slugs, unique components, exactly one primary, and artifact storage.
- Omitted version, artifact storage and worktree sharing default to `1`, `tree` and `true`; explicit false and empty links must survive loading.
- Bound document reads at 1 MiB. Keep errors source-safe: report file and key without TOML values or source lines.
- Create from the commented template without overwriting existing files; never rewrite a user's document.
- Registry, directory recognition, Git markers and session storage are outside this package's current behavior.
