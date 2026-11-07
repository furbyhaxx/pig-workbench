# Tools

Workbench registers these tools in place of the builtin names. Local relative paths use the captured Workbench working location (see [locations](#locations)); PiG's session cwd does not change. Individual tool results report the details available for their operation.

## `read`

Exactly one of `path` or `paths` (1–32 entries). Optional `selector` for a single path.

```text
read {"path": "src/main.go:20-40"}
read {"path": "src/main.go:raw"}
read {"paths": ["a.go", "b.go:1-5"]}
read {"path": "notes.md", "selector": ":10-20"}
```

An exact (non-raw) read of a local file starts with a tagged header `[file:///path#A1B2]`. The four-hex tag is derived from the normalized content (LF view, BOM removed, per-line trailing blanks trimmed), so it identifies the content, not the path. Raw reads carry no tag, and a file above 4 MiB is read without one. The tag locates a retained snapshot; the full source digest proves its content. Workbench grants seen-line authority only after the emitted block survives the tool-result hook unchanged. Truncated blocks grant nothing; trailing annotations do not invalidate an intact block. Branch and compaction events invalidate prior visibility.

Selectors: `:N` (onward), `:A-B`, `:A+C`, `:-N` (tail), comma lists, and `:raw` before or after a numeric part. A lone numeric part in a comma list means that single line. Ordinary ranges add one preceding and three trailing lines; raw is exact. A literal filename containing `:` wins over selector parsing.

For a multi-read, one failing target does not turn the whole result into an error: the successful content stays valid, `Details.failedTargets` counts the failures, and the per-resource error is kept in `Details.resources[]`. The tool result is an error only when every target failed.

Output is bounded by `read.max-bytes`/`read.max-lines` (default 50 KiB / 2000 lines) for the whole batch. `Details.resources[]` carries the canonical URI, exact-byte revision, total lines, shown and fully-shown ranges, selection vs resource completeness, and a continuation selector when the page was cut. A line larger than the budget is emitted as a partial line, is never marked seen, and the continuation repeats it.

Builtin schemes include `file://`, `http://`, `https://`, `local://`, `out://`, `cfg://`, `session://`, `artifact://`, `job://`, `skill://`, `agent://` and registered `docs://` resources. Other extensions can register read-only providers.

`http(s)://` reads can use a registered fetch backend or the builtin static reader. In the builtin reader, HTML is converted to readable Markdown by default, while `:raw` keeps the decoded document. Credentials in URLs, bare hosts without an explicit `/`, non-http(s) redirects and redirect cycles are refused; redirects are reauthorized per hop within an 8-hop limit. Charset metadata is decoded, bodies and rendered output are capped at 4 MiB, binary media is refused, and page facts name the handler, content type, final URI and notices. Scripts are never executed and meta/script redirects are reported, not followed.

`artifact://specs/0001-x.md` resolves to the nearest `.project/` directory above the session's project root, so every worktree and subdirectory of a project sees the same artifact tree without absolute paths. Paths cannot escape it, and a working location with no artifact tree reports not-found.

Reading a **directory** returns a stable listing view instead of an error: `dir/` for subdirectories, `name (N bytes)` for files, `name -> target` for links, directories first then lexical order. A single range or a tail pages the listing (`:10-20`, `:-5`); multiple ranges and `:raw` are refused because a listing is not source text.

A **bare** read of a code file with at least `read.summarize.min-total-lines` lines (default 200) returns a declaration skeleton instead of exact text: declaration lines with real line numbers, an explicit `⋯ lines A-B omitted` marker per folded body, and a continuation to the full text. A summary never marks folded lines seen, reports `truncation: "summary"` with `omittedRanges`, and is never used for an explicit selector, a `:raw` read, a prose file, or a file above 4 MiB. `read.summarize.enabled = false` disables it.

## `edit`

Use a tagged batch from an observed local read, or exact-text replacements when the configured `edit.mode` permits them:

```text
edit {"hashline": "[notes.md#A1B2]\nPUT 2.=2:\n+replacement\n"}
edit {"path": "notes.md", "edits": [{"oldText": "before", "newText": "after"}]}
```

Tagged edits support PUT, CUT, REM, MV, structural blocks and registers. Seen-line enforcement covers complete consumed ranges. Changed sources are relocated only with unambiguous evidence; otherwise read the current file again. Exact replacements must be unique and non-overlapping. Edits are preflighted and revision-checked; a partial I/O failure reports what landed rather than claiming rollback.

## `write`

```text
write {"path": "notes.md", "content": "text"}
write {"path": "notes.md", "content": "text", "expectedRevision": "<revision from read>"}
```

Creates or fully replaces one local file. A supplied `expectedRevision` must match the file's current digest or the write is refused with `staleRevision`. The replacement is atomic (same-directory temporary file, flush, rename), an existing file keeps its mode, a new file is created `0600`, and the parent directory must already exist. Line selectors are rejected; use `edit` for partial changes.

Control writes also support `job://<id>/stdin|ctl`, `skill://` unload, `agent://<id>/inbox`, `session://<id>/title` and `session://self/instructions/ctl`. They use the corresponding action's policy and confirmation rules. An `expectedRevision` on a control write is rejected before the action; it is not silently ignored.

## `ls`

```text
ls {"path": "."}
ls {"path": "session://self"}
ls {"path": ".", "cursor": "…", "limit": 200}
```

Lists containers supported by the selected provider. Entries have types (`file`, `dir`, `link`, `ctl`), a continuation cursor and an explicit `complete` flag in `Details`.

## `grep`

```text
grep {"pattern": "timeout", "path": "internal"}
grep {"pattern": "needle", "ignoreCase": true, "glob": "*.go", "exclude": ["vendor"]}
grep {"patterns": ["TODO", "FIXME"], "literal": true}
```

`patterns[]` requires `literal: true`; `pattern` is a regex unless `literal: true` is set. `context` emits up to ten source lines on either side of each match, marked separately from matching lines. Binary and oversized files are skipped and listed in `Details.skipped`. Reaching the match, file or output limit reports `complete: false` — never a silent "that is all". Long matching lines are shown as UTF-8-safe windows with their original byte column and window start; this makes the result incomplete. Requested context that cannot fit is reduced first, preserving matching lines where possible and marking the result incomplete. Registered provider directory roots are walked with depth, entry, time and source-byte limits. Links, control files and `noWalk` subtrees are skipped; incomplete, changing or oversized provider files are reported as skipped rather than searched partially. Explicit provider-file targets and line selectors keep source coordinates; directory roots do not accept line selectors. Provider files are read within source limits before selected lines are searched.

## `find`

```text
find {"pattern": "*.go", "exclude": ["vendor"], "limit": 200}
```

Walks file names under a local directory or registered provider directory. `find {"path": "kb://sources/", "pattern": "*.md"}` traverses paged provider listings with depth, entry, time and output limits. Links are not followed, control files and `.git` are skipped, and `noWalk` subtrees are skipped unless named directly. `local://` scratch traversal stays within the session root; scratch directories over 5000 entries return an error rather than an unbounded listing. Providers without listing support return an error. Excluded names are pruned before descending.

## `cd`

```text
cd {}
cd {"path": "services/api"}
cd {"reset": true}
```

Inspects or changes the Workbench working location. It never changes PiG's process or session `cwd`. A transition waits at most two seconds for active mutations, persists a revisioned record, and only then becomes active. Relative `cd` against an unavailable stored location is refused instead of silently falling back.

## `bash` and `jobs`

```text
bash {"command":"make test"}
bash {"command":"cat","background":true,"interactive":true}
jobs {"action":"write","id":"<job id>","text":"input\n"}
jobs {"action":"eof","id":"<job id>"}
jobs {"action":"wait","id":"<job id>","timeout_seconds":30}
jobs {"action":"output","id":"<job id>","cursor":0}
```

Commands capture the Workbench location, use Bash when installed (otherwise `/bin/sh`), and default to a 120-second hard deadline. `timeout_seconds: 0` disables that deadline. `yield_seconds` promotes a still-running foreground command after that delay. `background` returns immediately; request cancellation then does not cancel the owned job. Foreground cancellation and `jobs kill` terminate the process group. Stdin is closed unless `interactive` is true.

The single `jobs` tool supports `list`, `status`, `output`, `wait`, `kill`, `write` and `eof`. Output pages report their next byte cursor and any eviction gap. Completed jobs are retained up to the manager cap; retired IDs fail explicitly. Session shutdown owns cleanup. Linux requires pidfd process-group signalling and refuses unsupported kernels before starting commands. Windows group cleanup remains unsupported.

A job also outlives the runtime that started it when that runtime dies without shutting down. To keep such a job from becoming an unsupervised process, each runtime claims a liveness lock for its generation under `$XDG_STATE_HOME/pig-workbench/jobs/`, records the pid and kernel start time of every job it starts, and the next start terminates the process groups whose owner lock is free — after rechecking the recorded pid against its start time, so a recycled pid is never signalled. Records of a live runtime are left untouched, including another session's.

`job://` resources, `/workbench jobs` and `/wb:jobs` are wired. Standalone `ls` and `cat` are routed for non-locatable scheme paths. The limited form `value > URI` sends one static shell word as content to the same supported control targets as `write`, including its confirmation rules; static quotes and escapes are recognized, but expansions and shell composition are refused when a scheme path is detected. This is not general shell redirection; paths constructed dynamically by the shell cannot always be recognized before execution. Scheme-path `rm`, `mv`, `cp` and `mkdir` are unconditionally refused. Commands containing a directly visible scheme URI reject quoting, pipes and compounds; interception is incomplete for dynamically constructed or split-quoted scheme names. Do not rely on this guard as shell sandboxing. Full provider-directed scheme-command dispatch and job panels remain pending.

## `ask_user`

Native TUI/RPC dialogs support single, multi, preview, freeform and secret questions, with custom answers and notes. Cancellation returns no partial answers. Headless print/JSON execution returns `interactionUnavailable`; it never invents choices. RPC questions use the configured dialog deadline.

Secret values are collected before finalization, stored encrypted, and returned as placeholders only. Local typing need not be masked. Successful completed-record persistence is the submission commit point. If secret storage partially succeeds, the error reports retained placeholder IDs rather than claiming rollback or ordinary cancellation. `/workbench ask show|resume|discard` manages pending forms. The rich questionnaire panel is pending. Multi and preview questions have been exercised through native RPC dialogs with a scripted provider.

## Locations

The working location is captured per call. Mutating tools take a lease, so a `cd` cannot move the ground under an in-flight write. The location survives resume (replayed from the session's parent chain), and PiG's own session `cwd` stays untouched, which also means PiG's builtin tools keep using it until they are replaced.

## Ambient instructions

For a trusted project, Workbench selects root-level `*.md` personal notes from `<agent-dir>/projects/<project-id>/private/` as user-authority instruction frames. Create the directory and notes yourself; the project and `private` directories must be owner-only. Notes and pins are scoped to the active trusted project: an untrusted project cannot claim another project's notes by copying its ID. A user `trust.rules` allow rule can authorize another project. Changes are reconciled at prompt and tool boundaries; changing projects withdraws inapplicable pins. Use `/workbench instructions rules` to inspect classes and `/workbench instructions pin <path>` or `unpin <path>` to manage pins. Discovery is bounded to 4096 directory entries and the shared source budget; symlinked notes are not followed.

## Sessions and settings resources

- `session://self/meta` — session id, name, cwd, mode, entry counts, whether it is durable.
- `session://self/title` — current title.
- `session://self/transcript` — the active branch as Markdown.
- `session://self/entries` — raw session entries, one JSON object per line.
- `cfg://workbench/status` — configuration readiness, revision and effective read policy.
- `cfg://<group>/<key>` — one effective setting with its source scope and file.

These resources are read with the selectors supported by their providers. `session://self/title` is also writable; stored-session title changes require confirmation and are refused for a live session.
