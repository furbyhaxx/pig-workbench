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

A later complete same-revision source read can replace an earlier partial single-resource read's source block in model context with a provenance pointer. Stored session history is unchanged; annotations stay intact. Derived, summary, clipped, redacted or otherwise unproved source blocks do not authorize this projection.

By default, the third identical normalized read at the same revision adds a repetition hint without suppressing content; `read.loop-hint-after` changes that threshold. Changing the selection, view or revision resets the counter; unknown revisions do not produce hints.

First-touch provider guides share the read budget. If the full guide cannot fit, the result links to `docs://workbench/schemes/<scheme>` instead of appending unbounded guide text; a changed guide revision is delivered again.

Output is bounded by `read.max-bytes`/`read.max-lines` (default 50 KiB / 2000 lines) for the whole batch. `Details.resources[]` carries the canonical URI, exact-byte revision, known total lines (absent when unknown, zero for a known empty source), shown and fully-shown ranges, selection vs resource completeness, and a continuation selector when the page was cut. A line larger than the budget is emitted as a partial line, is never marked seen, and the continuation repeats it.

Builtin schemes include `file://`, `http://`, `https://`, `local://`, `out://`, `cfg://`, `session://`, `artifact://`, `job://`, `skill://`, `agent://` and registered `docs://` resources. Other extensions can register read-only providers or declare revision-checked mutation capabilities with receipts; Workbench adds no permission prompts.

Reading a directory-like resource (`session://self`, `docs://workbench/`, `skill://`, `job://`, a registered provider's directory) returns the same paged listing view a local directory does; a provider only needs to implement listing.

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

The tool description the model sees follows the session's `edit.mode`: in `hashline` (the default) it explains only the tagged form with worked operations, in `replace` only exact replacements, in `auto` both. Tagged edits support PUT, CUT (`CUT 7.=8` deletes lines 7-8 and keeps them in a register), REM, MV, structural blocks and registers. Seen-line enforcement covers complete consumed ranges. Changed sources are relocated only with unambiguous evidence; otherwise read the current file again. Exact replacements must be unique and non-overlapping. Edits are preflighted and revision-checked; a partial I/O failure reports what landed rather than claiming rollback.

## `write`

```text
write {"path": "notes.md", "content": "text"}
write {"path": "notes.md", "content": "text", "expectedRevision": "<64-hex Revision from a write receipt>"}
```

Creates or fully replaces one local file. A supplied `expectedRevision` must be the file's 64-hex digest, as printed in a `write` receipt (`Revision:`), or the write is refused with `staleRevision`; a read header's `#TAG` is not a revision and is refused with an explanation. The replacement is atomic (same-directory temporary file, flush, rename), an existing file keeps its mode, a new file is created `0600`, and the parent directory must already exist (the error says to create it first). Line selectors are rejected; use `edit` for partial changes.

`local://<name>` writes a file into the session scratch: the parent directory must already exist, the content is limited to 4 MiB, links and directories are refused, nothing outside the scratch root is reachable, and `expectedRevision` is rejected. The replacement is atomic.

Declared writable provider resources also accept `write`, with a 32 KiB content limit and the encoded 64 KiB control-envelope limit. Workbench captures the current revision (or absence) and the provider rechecks it at commit; an explicit `expectedRevision` must match. Results report committed, notApplied, partial or unknown, never automatic retries. There are no new permission or confirmation prompts for contributed mutations.

Control writes also support `job://<id>/stdin|ctl`, `skill://` unload, `agent://<id>/inbox`, `session://<id>/title` and `session://self/instructions/ctl`. They use the corresponding action's policy and confirmation rules. An `expectedRevision` on a control write is rejected before the action; it is not silently ignored.

## `ls`

```text
ls {"path": "."}
ls {"path": "session://self"}
ls {"path": ".", "cursor": "…", "limit": 200}
```

Lists containers supported by the selected provider. Entries have types (`file`, `dir`, `link`, `ctl`), a continuation cursor and an explicit `complete` flag in `Details`. Content stays within 50 KiB / 2000 lines and entry metadata is bounded; unshown rows remain resumable rather than being skipped. `includeHidden` defaults to `true`; pass `false` to omit dot-prefixed names. Details report the effective policy and the number omitted. Listing cursors are bound to the resource, hidden-entry policy and captured tool context; contributed-provider cursors also bind registration and descriptor revision; they expire on reload/replacement, after five minutes or when the bounded cache evicts them. Restart the listing after `staleRevision`. Native file/scratch backend cursors also bind the directory snapshot: insertion, removal, rename or visible entry-metadata changes invalidate continuation instead of silently skipping rows.

## `grep`

```text
grep {"pattern": "timeout", "path": "internal"}
grep {"pattern": "needle", "ignoreCase": true, "glob": "*.go", "exclude": ["vendor"]}
grep {"patterns": ["TODO", "FIXME"], "literal": true}
```

Local `grep` and `find` honor repository and nested `.gitignore` rules, including negation. Explicitly named file roots remain searchable. Ignore files must be regular files and share a 1 MiB parsing budget; unsupported or unsafe ignore inputs fail rather than silently widening the search.

With `filesOnly: true`, output contains matching file paths only (also returned as `Details.paths`), without source text, coordinates or context windows. It does not grant source seen-state.

`patterns[]` requires `literal: true`; `pattern` is a regex unless `literal: true` is set. Case-insensitive matching uses Go/RE2 Unicode folding for both modes, and columns always refer to original-source UTF-8 byte positions. `context` emits up to ten source lines on either side of each match, marked separately from matching lines. Binary and oversized files are skipped and listed in `Details.skipped`. Reaching the match, file or output limit reports `complete: false` — never a silent "that is all". Long matching lines are shown as UTF-8-safe windows with their original byte column and window start; this makes the result incomplete. Requested context that cannot fit is reduced first, preserving matching lines where possible and marking the result incomplete. Registered provider directory roots are walked with depth, entry, time and source-byte limits. Links, control files and `noWalk` subtrees are skipped; incomplete, changing or oversized provider files are reported as skipped rather than searched partially. Explicit provider-file targets and line selectors keep source coordinates; directory roots do not accept line selectors. Provider files are read within source limits before selected lines are searched.

Search globs (including exclusions) support `*`, `?`, character classes such as `[ab]`, brace alternatives such as `*.{go,md}`, and `**` as a whole path component. Use `/` separators on every platform. Patterns without `/` match names at any walked depth; slash patterns also match paths relative to the explicit root. `**/*.go` includes root-level files. Malformed patterns fail before traversal; glob syntax never expands the search root.

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

### Codemode child calls

`/workbench codemode [call id]` lists nested calls Workbench observed in this runtime: parent ID, child ID, tool name and `running`/`ok`/`error`. Arguments and output are never retained. Records are bounded (128 parents, 64 children each, with an omitted count) and are lost on reload or resume; stock PiG still draws codemode's own child rows.

## `bash` and `jobs`

```text
bash {"command":"make test"}
bash {"command":"cat","background":true,"interactive":true}
jobs {"action":"write","id":"<job id>","text":"input\n"}
jobs {"action":"eof","id":"<job id>"}
jobs {"action":"wait","id":"<job id>","timeout_seconds":30}
jobs {"action":"output","id":"<job id>","cursor":0}
```

Commands capture the Workbench location, use Bash when installed (otherwise `/bin/sh`), and default to a 120-second hard deadline. `timeout_seconds: 0` disables that deadline. `yield_seconds` promotes a still-running foreground command after that delay. `background` returns immediately; request cancellation then does not cancel the owned job. Foreground cancellation and `jobs kill` terminate the process group. Stdin is closed unless `interactive` is true. When a background or promoted job ends on its own (not killed on request), Workbench sends one `workbench.jobs` notice and shows a user notice. An idle session wakes and starts a turn for it (set `jobs.wake-on-completion = false` to deliver it with the next prompt instead); a running session receives it at its next boundary. The notice carries no output and is not consumed by viewing `job://` or `/workbench jobs`.

The single `jobs` tool supports `list`, `status`, `output`, `wait`, `kill`, `write` and `eof`. Output pages report their next byte cursor and any eviction gap. Completed jobs are retained up to the manager cap; retired IDs fail explicitly. Session shutdown owns cleanup. Linux requires pidfd process-group signalling and refuses unsupported kernels before starting commands. Windows group cleanup remains unsupported.

A job also outlives the runtime that started it when that runtime dies without shutting down. To keep such a job from becoming an unsupervised process, each runtime claims a liveness lock for its generation under `$XDG_STATE_HOME/pig-workbench/jobs/`, records the pid and kernel start time of every job it starts, and the next start terminates the process groups whose owner lock is free — after rechecking the recorded pid against its start time, so a recycled pid is never signalled. Records of a live runtime are left untouched, including another session's.

`job://` resources, `/workbench jobs` and `/wb:jobs` are wired. Standalone `ls` and `cat` are routed for non-locatable scheme paths. The limited form `value > URI` sends one static shell word as content to the same supported control targets as `write`, including its confirmation rules; static quotes and escapes are recognized, but expansions and shell composition are refused when a scheme path is detected. This is not general shell redirection; paths constructed dynamically by the shell cannot always be recognized before execution. Scheme-path `rm`, `mv`, `cp` and `mkdir` dispatch declared provider operations without new permission prompts. Copy/move stay within one provider and require an absent destination; recursive operations are unsupported. Quoted static words are recognized, while pipes and compounds are rejected. Do not rely on this guard as shell sandboxing. Provider hashline editing and job panels remain pending.

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
