# config

SPEC-0010 scoped TOML settings and safe saves. `catalog.go` is the single validation/default/scope catalog; host consumers use typed snapshot projections.
- BurntSushi/toml validates semantics; tomledit changes human-maintained documents. Never serialize an entire user document from a map.
- Defaults < user < trusted host-cwd project < runtime. Mutable cd never changes config roots. Recheck host trust immediately before project writes.
- Never echo TOML source lines or invalid values in diagnostics. Unknown keys warn and survive saves; malformed/unsupported files block writes.
- Serialize in-process and cross-process; merge disjoint saves and reject changed-key conflicts against the caller's captured baseline. Resolve symlink targets without replacing the symlink.
- Publish immutable snapshots only after full validation. Reload failure retains a stale last-valid snapshot and blocks mutations.
