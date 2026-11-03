# Tools

Workbench registers these tools in place of the builtin names. Every tool resolves relative paths against the Workbench working location (see [locations](#locations)), never against a directory changed by `cd` on PiG's side, and reports the location and configuration revision it acted on in `Details`.

## `read`

Exactly one of `path` or `paths` (1–32 entries). Optional `selector` for a single path.

```text
read {"path": "src/main.go:20-40"}
read {"path": "src/main.go:raw"}
read {"paths": ["a.go", "b.go:1-5"]}
read {"path": "https://example.com/doc:10-20", "selector": ":10-20"}
```

An exact (non-raw) read of a local file starts with a tagged header `[file:///path#A1B2]`. The four-hex tag is derived from the normalized content (LF view, BOM removed, per-line trailing blanks trimmed), so it identifies the content, not the path. Raw reads carry no tag, and a file above 4 MiB is read without one. The tag is the handle a later edit uses to prove which content it was written against.

Selectors: `:N` (onward), `:A-B`, `:A+C`, `:-N` (tail), comma lists, and `:raw` before or after a numeric part. A lone numeric part in a comma list means that single line. Ordinary ranges add one preceding and three trailing lines; raw is exact. A literal filename containing `:` wins over selector parsing.

Output is bounded by `read.max-bytes`/`read.max-lines` (default 50 KiB / 2000 lines) for the whole batch. `Details.resources[]` carries the canonical URI, exact-byte revision, total lines, shown and fully-shown ranges, selection vs resource completeness, and a continuation selector when the page was cut. A line larger than the budget is emitted as a partial line, is never marked seen, and the continuation repeats it.

Supported schemes today: `file://`, `local://`, `out://`, `cfg://`, `session://`.

Reading a **directory** returns a stable listing view instead of an error: `dir/` for subdirectories, `name (N bytes)` for files, `name -> target` for links, directories first then lexical order. A single range or a tail pages the listing (`:10-20`, `:-5`); multiple ranges and `:raw` are refused because a listing is not source text.

A **bare** read of a code file with at least `read.summarize.min-total-lines` lines (default 200) returns a declaration skeleton instead of exact text: declaration lines with real line numbers, an explicit `⋯ lines A-B omitted` marker per folded body, and a continuation to the full text. A summary never marks folded lines seen, reports `truncation: "summary"` with `omittedRanges`, and is never used for an explicit selector, a `:raw` read, a prose file, or a file above 4 MiB. `read.summarize.enabled = false` disables it.

## `write`

```text
write {"path": "notes.md", "content": "text"}
write {"path": "notes.md", "content": "text", "expectedRevision": "<revision from read>"}
```

Creates or fully replaces one local file. A supplied `expectedRevision` must match the file's current digest or the write is refused with `staleRevision`. The replacement is atomic (same-directory temporary file, flush, rename), an existing file keeps its mode, a new file is created `0600`, and the parent directory must already exist. Line selectors are rejected; use `edit` for partial changes.

## `ls`

```text
ls {"path": "."}
ls {"path": "session://self"}
ls {"path": ".", "cursor": "…", "limit": 200}
```

Typed entries (`file`, `dir`, `link`, `ctl`), directories first, then lexical order, with a continuation cursor and an explicit `complete` flag in `Details`.

## `grep`

```text
grep {"pattern": "timeout", "path": "internal"}
grep {"pattern": "needle", "ignoreCase": true, "glob": "*.go", "exclude": ["vendor"]}
grep {"patterns": ["TODO", "FIXME"], "literal": true}
```

`patterns[]` requires `literal: true`; `pattern` is a regex. Binary and oversized files are skipped and listed in `Details.skipped`. Reaching the match or file limit reports `complete: false` — never a silent "that is all". `context` is accepted but surrounding lines are not emitted yet.

## `find`

```text
find {"pattern": "*.go", "exclude": ["vendor"], "limit": 200}
```

Walks file names under the location. Symlinked directories are not followed; `.git` is skipped; excluded names are pruned before descending.

## `cd`

```text
cd {}
cd {"path": "services/api"}
cd {"reset": true}
```

Inspects or changes the Workbench working location. It never changes PiG's process or session `cwd`. A transition waits at most two seconds for active mutations, persists a revisioned record, and only then becomes active. Relative `cd` against an unavailable stored location is refused instead of silently falling back.

## Locations

The working location is captured per call. Mutating tools take a lease, so a `cd` cannot move the ground under an in-flight write. The location survives resume (replayed from the session's parent chain), and PiG's own session `cwd` stays untouched, which also means PiG's builtin tools keep using it until they are replaced.

## Sessions and settings resources

- `session://self/meta` — session id, name, cwd, mode, entry counts, whether it is durable.
- `session://self/title` — current title.
- `session://self/transcript` — the active branch as Markdown.
- `session://self/entries` — raw session entries, one JSON object per line.
- `cfg://workbench/status` — configuration readiness, revision and effective read policy.
- `cfg://<group>/<key>` — one effective setting with its source scope and file.

All are read-only and accept the same selectors as `read`.
