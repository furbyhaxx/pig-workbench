# hashline

Pure source snapshots, tags and rendering for hashline editing; no SDK or filesystem dependencies.
- `Normalized.Bytes` is the UTF-8, BOM-free LF view; source lines are one-based and offsets exclude the LF terminator.
- Preserve original BOM and individual line endings on restore; inserted and changed lines use the dominant ending. Refuse ambiguous mixed-ending alignments.
- `Restore` needs the complete value returned by `Normalize`; it carries private source state. `Normalized.Warnings` reports LF/CRLF ties, resolved to LF.
- `Parse`/`Format` implement canonical tagged sections, PUT/CUT/REM/MV, named or anonymous gap-register pastes and verbatim `+` payload rows. They do not apply edits or resolve registers.
- Line numbers are one-based original-source coordinates; `Line == 0` only denotes EOF for `PUT_INSERT_AFTER`, never a source line. `Op.Target` is set only for `PUT_FROM_REGISTER`.
- A CUT without `@name` uses the batch-only anonymous register. `Document.NoFinalNewline` retains the patch's terminal-newline choice.
- `StageLines` transforms validated line operations against an intact `Normalize` snapshot; it returns LF bytes and separate, sorted consumed source spans. Callers resolve blocks and register pastes, capture CUT registers, and validate seen state and file operations separately.
