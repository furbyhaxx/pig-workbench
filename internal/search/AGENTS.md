# search

Bounded grep and glob walking for the `grep`/`find` tools (SPEC-0006 §6).

- Results always report scanned counts and honest completeness; hitting a limit is never a silent "no more matches".
- Only regular files are opened (via `internal/fileio`); binary and oversized files are skipped and listed, not silently ignored.
- Patterns are matched in Go; never interpolate a pattern or path into a shell command.
- Symlinked directories are not followed, so a walk cannot escape the requested root.
