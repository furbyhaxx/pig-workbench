# prompt

Typed, SDK-independent Jinja rendering core for SPEC-0014 §3; use `New(Options)` and `Engine.Render` with fixed request data.
- Supply a `TemplateLoader`; loader names are passed through unchanged. Callers must provide a confined loader before rendering untrusted templates.
- Render with strict undefined values, whitespace controls, a 64 KiB default output limit, 16-level include depth and a 100 ms deadline.
- A deadline discards the result; gonja does not preempt an in-flight evaluation or a blocked loader.
- gonja v2.9.1 `for.go:202` ignores evaluation errors inside an `if` filter on a `{% for %}` loop, so an undefined variable used only in a loop filter produces empty output instead of a strict-undefined error.
- Override gonja's `attr` filter to error on missing map keys and struct attributes. Workbench owns its templates; present struct values and non-map/struct behavior are preserved, while present map keys resolve to their values.
- Confined filesystem loading, section tags and structure checks, prompt helpers and the builtin template belong to separate components.
