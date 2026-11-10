# textclean

Control-character handling for text shown or stored as plain text; no SDK dependency.
- Terminal escape sequences are stripped by the caller with `tui.StripTerminalSequences` first; this package only handles the remaining control characters.
- Callers name the characters they keep (`'\n'`, `'\t'`); nothing is kept by default.
