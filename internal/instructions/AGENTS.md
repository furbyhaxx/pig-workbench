# instructions

Local, SDK-free ambient-file discovery and capped views (SPEC-0007 §§2–3, 7).
- Steering defaults to `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, first existing per directory; extra names are explicit configuration.
- `ancestors` selects all applicable directories; `root+nearest` selects the broadest existing and closest, once when identical. `root` is the exact boundary.
- User roots are separately authorized; search each once before project ancestry. Never permit a candidate symlink to escape its authorizing root.
- The caller supplies the effective project boundary. Refuse nested `.git` boundaries unless `AllowNested` explicitly permits and records crossing. A linked worktree confirmed by the main checkout's back-link is not a boundary; its identical-byte files deduplicate.
- Source digests cover exact bytes read; views strip a BOM and normalize CRLF. Apply budgets to whole lines and report unknown omission counts honestly. Truncated steering is never complete.
- Validate backlog pointer line shape only. Target existence and `status: "open"` checks belong to the `instructions.backlog.validate-pointers` setting owner; `ValidatePointers` reports the deferred check and never reads targets.
- Host files (`host.go`): `ResolveHost` picks `/etc/agents/AGENTS.md` and `HOST.md` per file name from readers ordered by priority (a remote host replaces a local copy; a refused file is never replaced by another host's). `LocalHost` refuses world-writable files or directories (group-writable load) or ones owned by neither root nor the user. Trust is `system`.
- Watching, pins, delivery, mutation barriers, lifecycle integration and diffs are not implemented here.
