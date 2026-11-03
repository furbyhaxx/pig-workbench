# prompt

Typed, SDK-independent Jinja rendering core for SPEC-0014 §3; use `New(Options)` and `Engine.Render` with fixed request data.
- Supply a `TemplateLoader`; loader names are passed through unchanged. `NewFileSystemLoader` is the confined implementation: the `builtin/` namespace cannot be shadowed, a trusted project root overrides the user root, and `..`, absolute paths, backslashes, NUL, escaping symlinks, directories and other non-regular files are rejected. Callers pass roots and trust per load and own `Close`.
- Render with strict undefined values, whitespace controls, a 64 KiB default output limit, 16-level include depth and a 100 ms deadline.
- A deadline discards the result; gonja does not preempt an in-flight evaluation or a blocked loader.
- gonja v2.9.1 `for.go:202` ignores evaluation errors inside an `if` filter on a `{% for %}` loop, so an undefined variable used only in a loop filter produces empty output instead of a strict-undefined error. An otherwise non-empty render bypasses the empty-output guard.
- Override gonja's `attr` filter to error on missing map keys and struct attributes. Workbench owns its templates; present struct values and non-map/struct behavior are preserved, while present map keys resolve to their values.
- One template file is bounded to 1 MiB by default (`MaxFileBytes`); `List` returns reachable names sorted and confined. Section tags and structure checks, prompt helpers and the builtin template belong to separate components.
