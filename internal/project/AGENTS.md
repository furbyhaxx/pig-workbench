# project

`document.go` owns the SPEC-0012 §3.2 `.project/Project.toml` schema and create/load/parse API.
- Decode strictly: accept version 1, reject unknown keys, validate IDs, kebab slugs, unique components, exactly one primary, and artifact storage.
- Omitted version, artifact storage and worktree sharing default to `1`, `tree` and `true`; explicit false and empty links must survive loading.
- Bound document reads at 1 MiB. Keep errors source-safe: report file and key without TOML values or source lines.
- `Write`/`Create` render a commented document and publish it without overwriting existing files; never rewrite a user's document.
- `manage.go` owns explicit user actions: `CreateProject` (generated ID, unique slug), `Join` by file or by the local `pig.projectId` git marker. These are the only writers of Project.toml and of the marker.
- `registry.go` and `registry_store.go` own `<agentDir>/projects/<id>/project.toml` and `index.toml`; records contain shared fields plus observed locations, and the index is a disposable cache rebuilt from validated records.
- Serialize registration and rebuilding with the registry lock; atomically replace private 0600 files in private 0700 directories. `List` returns valid entries alongside errors for invalid records.
- Registry privacy checks require Unix ownership; Windows fails closed until ACL validation exists.
- `detect*.go` recognizes project file → git marker → root commit → normalized remote → longest local host/path → global. Ambiguous claims and invalid registry records are errors; root-commit probes use locally accessible registered locations.
- Detection is read-only: `List`/`Lookup` acquire a lock and `List` may rebuild the cache, so use the read-only scan. Git probes suppress optional writes, tracing and inherited repository overrides; never write Git markers during detection (reading `pig.projectId` is allowed).
- Detection returns the repository, project-file ancestor or matched location as `Root`; global outside Git uses the queried directory. The global `RegistryDir` can be absent until storage is adopted.
