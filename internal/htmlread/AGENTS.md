# htmlread

Static HTML to readable Markdown. No SDK, no network, no script execution and no redirect following.

- `Convert(ctx, source, baseURL, Options) (Document, error)` is the only entry point. `baseURL` must be the complete absolute page URL, not the origin.
- `Document.Method` is `main` (semantic container), `readability` (fallback) or `redirect` (no content, `Redirect` set, `Markdown` empty). `Redirect` also reports a refresh target next to real content.
- `convert.go` — public API, limits, selection order, Markdown rendering, cancellation reader.
- `dom.go` — preparation walk (chrome, permalinks, absolute references, redirect capture, depth/node bounds), main-element selection, layout-table marking, text helpers.

## Pipeline
1. `html.Parse` through a context-checking reader, so parsing is abandonable.
2. One pre-order walk strips chrome and permalinks, resolves `href`/`src` against `baseURL`, records redirect candidates and enforces `maxDepth`/`maxNodes`.
3. Tables without `<th>` get `role="presentation"`, so the table plugin falls through to text.
4. `.markdown-body`, `article`, `main`, `[role=main]`, `#content`/`#main-content` win; readability runs only when none matched.
5. The chosen node converts through `ConvertNode` and its size is checked against `MaxOutputBytes`; output is never truncated.

## Bounds
4 MiB input, 4 MiB Markdown, 512 levels, 200000 elements, 40 characters of visible text before content is accepted. Cancellation is checked before parsing, between parse reads, every 1024 elements and after rendering. Tests lower the bounds by calling `convert` with a `limits` value.

## Traps
- Don't add `converter.WithDomain`; it resolves `#anchor` against the origin instead of the page URL. References are already absolute by the time the converter runs.
- Readability is fallback-only on purpose: it drops `language-*` classes and a heading that repeats the title, so fences lose languages and headings disappear on that path. `Document.Title` carries the title.
- Layout-table cells render as adjacent text (`Left cellRight cell`); accepted residue, matching the benchmark.
- `Document.Redirect` is a report. Following it, the redirect-hop limit and the `:raw` HTML view belong to the HTTP layer.

Source: `.project/specs/0006-advanced-read.md` HTML row.
