# internal/pathdisplay

Pure string rendering of a recorded target for tool rows, frame titles, panels and the footer.

- `Display(Target, width)` is the only entry point: give it the recorded value and the root it was resolved against, never a resolved relative path.
- No I/O, no provider calls, no environment reads beyond the user's home prefix. Rendering a row must never touch the filesystem.
- Widths are terminal cells via `tui.VisibleWidth`; wide runes count two. The result never exceeds the width.
- Golden cases live in `display_test.go` and mirror SPEC-0003 §10.3. Change a golden only with the spec.
- Rule 7 (shell-command word shortening) is not implemented here yet; bash rows still show the command as typed.
