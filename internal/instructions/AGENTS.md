# instructions

Local, SDK-free ambient-file discovery and capped views (SPEC-0007 §§2–3, 7).
- Steering defaults to `AGENTS.override.md`, `AGENTS.md`, `AGENTS.MD`, first existing per directory; extra names are explicit configuration.
- `ancestors` selects all applicable directories; `root+nearest` selects the broadest existing and closest, once when identical. `root` is the exact boundary.
- User roots are separately authorized; search each once before project ancestry. Never permit a candidate symlink to escape its authorizing root.
- Source digests cover exact bytes read; views strip a BOM and normalize CRLF. Apply budgets to whole lines and report unknown omission counts honestly.
- Watching, pins, delivery, mutation barriers, lifecycle integration and diffs are not implemented here.
