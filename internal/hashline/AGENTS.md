# hashline

Pure source snapshots, tags and rendering for hashline editing; no SDK or filesystem dependencies.
- `Normalized.Bytes` is the UTF-8, BOM-free LF view; source lines are one-based and offsets exclude the LF terminator.
- Preserve original BOM and individual line endings on restore; inserted and changed lines use the dominant ending. Refuse ambiguous mixed-ending alignments.
- Grammar, patch staging, seen guards and registers are not implemented in this package.
