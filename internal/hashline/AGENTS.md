# hashline

Pure source snapshots, tags and rendering for hashline editing; no SDK or filesystem dependencies.
- `Normalized.Bytes` is the UTF-8, BOM-free LF view; source lines are one-based and offsets exclude the LF terminator.
- Preserve original BOM and individual line endings on restore; inserted and changed lines use the dominant ending. Refuse ambiguous mixed-ending alignments.
- `Restore` needs the complete value returned by `Normalize`; it carries private source state. `Normalized.Warnings` reports LF/CRLF ties, resolved to LF.
- Grammar, patch staging, seen guards and registers are not implemented in this package.
