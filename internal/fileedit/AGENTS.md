# fileedit

Local commit executor for already-staged edit effects: validate, lock, replace, move, delete. No SDK, no parsing, no snapshots. Contract: `.project/specs/0005-hashline-editing.md` §7 ("Stage and commit").

- `commit.go` — `Commit` orchestration: preflight → sorted advisory locks → locked validation → per-effect commit; `Effect`/`Outcome`, stage/path errors, failure-injection seams.
- `resource.go` — canonical identities, exact-bytes SHA256 revisions, source/destination revalidation before every mutation.
- `lock.go` — user-private lock directory, one `gofrs/flock` file per canonical identity, hashed name.
- `write.go` — same-directory temp file, atomic replace, no-replace publish and move.
- `platform_unix.go` / `platform_windows.go` — directory fsync, cross-device detection, lock directory privacy.

Rules
- Source bytes are streamed through SHA256 in 32 KiB chunks with a per-chunk ctx check (`digestStream`); never buffer a whole file, and never retain source content after observation. No-op replacement = equal replacement digest and equal byte length.
- Paths are absolute at the boundary: `Commit` rejects a relative `Effect.Path`, `MoveTo` or `lockDir`. Never resolve an untrusted relative path against the process working directory.
- Revision = lowercase hex SHA256 of the exact current bytes, no prefix, same encoding as `hashline.SourceRevision`. Don't import `hashline` from here; it is spec-facing and pure.
- One effect per canonical resource. Symlinks, `./` forms and duplicate paths are resolved before aliasing is checked; aliasing is `ErrAliasConflict`, never last-writer-wins. Move destinations take part in the identity set and the lock order.
- Replacement renames a same-directory temp over the *resolved target*, so the symlink entry survives. Deletion removes the resolved target; the link entry is never removed, moved or replaced, so deleting through a symlink leaves a dangling link.
- Never publish or move with a check-then-`os.Rename`. Destinations are validated missing in preflight and again before commit; publish uses `os.Link` (EEXIST) and moves use link+unlink. A cross-filesystem move is `ErrUnsupported`, not a copy.
- Replacement preserves the source permission bits only; don't copy setuid/setgid.
- Multi-resource atomicity is not promised. A failure after the first mutation returns one outcome per effect: committed ones stay committed, the failed one carries its error, later ones report `not attempted:`. Never describe or implement a rollback over external changes.
- Cancellation is checked at effect boundaries and per read chunk during observation. A write that already landed is reported committed; only never-started effects are reported unattempted.
- `Outcome`: `Committed` is true when the requested mutation landed, including a move whose source unlink failed or a replacement whose directory fsync failed (`Error` then names the incomplete step; re-read both paths, and `AfterRevision` is still the landed digest). A replacement whose bytes equal the current bytes reports `Committed:false`, empty `Error`, and equal before/after revisions without touching the file. `AfterRevision` stays empty for deletions.
- A mid-batch failure marks every later effect `not attempted:` and counts landings that only failed their durability step in the committed count.
- Errors name the stage, the path and the next action; never include source content, rejected bytes or lock metadata.
- Remove only the temp files this package creates. Lock files stay in the caller's lock directory and are opened with `fileio.LockFlags()`.
- `renamePath`, `removePath`, `linkPath`, `syncDir` are package vars so tests can inject failures. Tests must stay sequential (no `t.Parallel`) and must pass a private lock directory: `t.TempDir()` is group readable and `Commit` rejects it.
- Callers own session snapshots, seen state, preview and receipts.

Cross-updates
- Change `Effect`/`Outcome` or the error contract → `internal/host` editor wiring and SPEC-0005 §8 result details.
