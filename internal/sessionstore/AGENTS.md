# sessionstore

SDK-independent, journaled conversion of a PiG session bucket (a real directory) into a symlink to `projects/<id>/sessions/` (SPEC-0012 §3.5), and its undo.
- Never move a session file: hard-link it into the target first, swap the directory for a symlink in one `RENAME_EXCHANGE`, then unlink the old names. A running PiG appends by path or fd and never sees a missing file.
- Every step is idempotent and judged from the filesystem state, not from the journal alone, so `Resume` finishes any interrupted conversion. Crash points are exercised through `Hooks`.
- Refuse, with a typed reason, instead of guessing: another live session, different filesystem, name collision with different content, entries that are not regular files, a symlink pointing elsewhere.
- Linux swaps atomically; other platforms fall back to remove-then-symlink with a window of microseconds. Windows is unsupported.
