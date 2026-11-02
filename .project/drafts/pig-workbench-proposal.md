---
document:
  type: "draft"
  id: "DRAFT-0001"
  title: "pig-workbench: full Go-native workbench, hashline engine, scheme VFS, and ambient context injection"
  description: "Complete adoption scope and architecture across pi-workbench, oh-my-pi, and nugent (excluding only subagents and task/todo tracking)"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T14:49:32+01:00"
  updated_at: "2026-11-02T15:08:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Initial research and proposal"
      changes: ["Inspected reference implementations and staged PiG SDK; proposed adoption scope and delivery gates"]
    - revision: 2
      reason: "Restore full scope per user feedback"
      changes:
        - "Adopted the full oh-my-pi hashline edit engine (section headers [PATH#TAG], PUT/CUT/REM/MV, syntax block N*, @registers, seen-line guard, shift/auto-repair, no-op loop detection, and configurable edit mode)"
        - "Adopted the full oh-my-pi + nugent read tool (inline selector grammar :N, :A-B, :A+C, :-N, :raw, :conflicts, :img, multi-range comma lists, delimited multi-path reads, structural code summaries, enclosing block context, SQLite/JSON/archive/binary/PDF/URL readers, loop guard, and complete-read supersede tracking)"
        - "Added scheme:// handlers for sessions/history (history://, session://, branch:// across current, project, and global scopes), skills (skill:// and skills://), and jobs (jobs:// and proc://), alongside file://, ssh://, artifact://, local://, cfg://, conflict://, and harness://"
        - "Restored /workbench as primary command plus top-level /tools, /remote, /cd, /answer, /answer:again, /tldr, /wb:jobs, /wb:cd, and --list-tools CLI flag"
        - "Restored codemode nested call capture, scriptFacts parsing, and tree/cost rendering"
        - "Redesigned instruction & ambient file injection to support user-defined file classes with placement policies (ancestors vs root+nearest vs pinned), item/line caps (+K more -> path), BACKLOG.md first-class rule, live diff frames, relocation/withdrawal, and deduplication"
---

# Summary of Changes (Revision 2)

The previous draft pruned away the defining mechanics of `oh-my-pi`, `nugent`, and `pi-workbench`. This revision restores the full capability set—excluding **only** subagents/agent definitions and task/todo tracking.

1. **Command surface:** Primary command is `/workbench`, plus top-level commands `/tools`, `/remote`, `/cd` (`/wb:cd`), `/answer`, `/answer:again` (`/ask:replay`), `/tldr`, `/wb:jobs`, and the `--list-tools` flag.
2. **Full Hashline Edit Engine:** Complete port of `oh-my-pi/crates/pi-edit/src/modes/hashline/` + `EditStore`: `[PATH#TAG]` 4-hex XXH32 snapshots, `N:LINE` numbering, `PUT N.=M:`, `PUT <N:`, `PUT >N:`, `PUT >$:`, syntax-block `PUT N*:`, `PUT >N*:`, `CUT N.=M [@reg]`, `CUT N* [@reg]`, register pastes, `REM`, `MV DEST`, seen-line enforcement, shift rescue, and no-op loop detection—plus configurable mode fallback (`hashline` default, `replace` exact-text mode).
3. **Full Optimized `read` Tool (`oh-my-pi` + `nugent`):** Inline selector suffixes (`:N`, `:N-`, `:N-M`, `:N+K`, `:-N`, comma-separated multi-ranges, `:raw`, `:conflicts`, `:img`, `?q=`), delimited multi-path batch reads, contextual windowing (`+1` leading / `+3` trailing lines + enclosing AST/bracket block opener/closer lines), structural code summaries for bare large code files, directory/container pagination, SQLite (`file.db`, `:table`, `:table:key`, `?q=SELECT`), JSON/JSONL (`?q=<jq filter>&offset=&limit=`), archives (`archive.zip:member`), binary/hex/symbol inspection, PDF/documents, static URLs, read-loop guard, and complete-read supersede metadata.
4. **First-Class `scheme://` VFS Router:**
   - `history://` & `session://` (plus `branch://`): current branch, project sessions, and global sessions (`~/.pig/agent/sessions/`), with search, metadata index, and entry/range reads.
   - `skill://` & `skills://`: list all trust-resolved skills, read `SKILL.md` (frontmatter stripped or `:raw`), and read/locate relative supporting files inside a skill's directory.
   - `jobs://` & `proc://`: list background jobs, read status/stream logs (`jobs://<id>`, `jobs://<id>/streams`), write stdin (`write jobs://<id>`), or cancel (`write jobs://<id>/kill`).
   - `file://`, `ssh://`, `artifact://`, `local://`, `conflict://`, `cfg://`, `harness://` (`status`, `reload`).
5. **Codemode Integration:** Full port of `pi-workbench/src/core/codemode.ts` (`NestedCaptures`, `scriptFacts`, `codemodeCalls`, nested tool preview trees, per-call duration/cost/error rendering) + Workbench tool namespace exposure for PiG codemode scripts.
6. **Enhanced Instruction & Ambient Context Injection:** Full port of `pi-workbench/src/instructions/` and `nugent`'s `InstructionLedger`, extended with **configurable ambient file classes** supporting:
   - **`ancestors` policy** (default for `AGENTS.override.md`, `AGENTS.md`, `BACKLOG.md`): inject from boundary root down to touched directory / cwd.
   - **`root+nearest` policy** (for multi-location project files where intermediate directories should be skipped): inject only the boundary/root instance and the nearest instance to the active/touched path.
   - **Agent/user pinning** (`pin` / `unpin` via `/workbench instructions` or `harness://instructions/pin`): keep specific files pinned across `cd` transitions.
   - **Item & line caps with pointer footer:** e.g. for `BACKLOG.md`, cap at `maxItems` (or `maxLines`) and append `(+K more → <path>)` so agents never trim files to save tokens.
   - **Live diff frames** on file edits during a run, coalesced updates while idle, byte-identical deduplication (`SkipIdentical`), zero-copy relocation (`Relocated`), withdrawal on `cd`, and self-edit notices.

---

# 1. Command & Tool Inventory

## 1.1 Registered Tools (10 tools + `codemode` renderer)

| Tool | Source / Role |
|---|---|
| `read` | Overrides PiG `read`. Full `oh-my-pi` + `nugent` routed reader with inline selectors, hashline `[PATH#TAG]` headers, structural summaries, block context, multi-path, SQLite, JSON, archives, images, and `scheme://` URIs. |
| `edit` | Overrides PiG `edit`. Full `oh-my-pi` hashline patch engine (`input` patch mode by default; configurable or schema-union `replace` mode with `path` + `edits[]`). |
| `write` | Overrides PiG `write`. Routes through the scheme VFS (`file://`, `ssh://`, `local://`, `jobs://`, `conflict://`, `cfg://`, `harness://`), strips accidental hashline prefixes on text writes, rejects truncation footers, and records full-file seen snapshots in `EditStore`. |
| `grep` | Overrides PiG `grep`. Local + SSH ripgrep/fallback search with `pattern` or literal `patterns[]` (OR), `output: "lines" \| "files"`, `exclude[]` globs, context lines, hashline-numbered matches (so grep output lines count as seen in `EditStore`), and exact completeness metrics. |
| `find` | Overrides PiG `find`. Local + SSH + virtual scheme globbing with brace/class/doublestar compilation and explicit limit reporting. |
| `ls` | Overrides PiG `ls`. Local + SSH + `scheme://` container listing. |
| `bash` | Overrides PiG `bash`. Executes in the active Workbench working location (local or SSH), supports `background: true`, configurable foreground-to-background yield/promotion, process-group watchdog, SecretGuard redaction, and `EditStore` invalidation on filesystem-mutating commands. |
| `cd` | Session working-location controller (`{}` status, `path`, `branch` worktree create/reuse under `.worktrees/<slug>`, `reset`), backed by branch-persisted `WorkingLocation` revisions and leases. |
| `jobs` | Single unified background-job tool combining `pi-workbench`'s `job_status`, `job_wait`, `job_kill`, and `job_write` plus `list` and `output`: actions `list`, `status`, `wait`, `output`, `write` (stdin + special keys `Enter`, `Ctrl+C`, `Ctrl+D`, `Tab`, `Esc`, `Up`, `Down` + `eof`), and `kill` (`cancel` alias). |
| `ask_user` | Full interactive questionnaire tool (`single`, `multi`, `preview`, freeform custom input, `recommended` badges, tab navigation, review screen, dismiss guard, auto-submit option, RPC dialog fallback, pending-ask recovery, and remote-ask event bus bridge). |
| *(renderer)* `codemode` | Custom call/result renderer for PiG's built-in `codemode` tool using `NestedCaptures` and `scriptFacts`. |

## 1.2 Slash Commands & CLI Flags

- **`/workbench`** — Master command and TUI menu:
  - `/workbench settings [work|edit|read|ask|tldr|jobs|frames|instructions|trust|statusline]`
  - `/workbench show` — Current effective settings, working location, active edit mode, and status summary.
  - `/workbench preview <compact|normal|full|all> [tool]` — Render live or synthetic tool/codemode previews at each density.
  - `/workbench tools [name] [--format text|json|markdown]` — Interactive tools & schema inspector panel or formatted dump.
  - `/workbench jobs [list|monitor|kill|write|settings] [job]` — Live jobs TUI panel (output viewer, kill, stdin writer).
  - `/workbench frames [ls|show|hide|collapse] [target]` — Inspect branch frames and configure per-kind/per-source visibility.
  - `/workbench instructions [ls|pin|unpin|reload|rules]` — Inspect loaded/withdrawn instruction & ambient files, pin/unpin files, or inspect class rules.
  - `/workbench trust [ls|add|rm]` — Manage instruction/host trust globs.
  - `/workbench tldr [regenerate|instructions|config|model|cancel|help]` — Manual branch checkpoint summary generator.
  - `/workbench statusline` — Optional statusline/footer segment editor (when enabled in config).
- **Top-level commands (matching `pi-workbench`):**
  - `/tools [name]` — Alias to `/workbench tools`.
  - `/remote [alias[:/path] | off | reload | add | config | <host> show|note|shell|hide|unhide|rename]` — Full SSH host manager panel and CLI.
  - `/cd [path | --branch <branch> | --reset]` and `/wb:cd` — Interactive directory/worktree picker or direct location switch.
  - `/wb:jobs` — Alias to `/workbench jobs`.
  - `/answer` and `/answer:again` (`/ask:replay`, `/ask-settings`) — Extract questions from the last assistant turn into an `ask_user` form, or replay the last questionnaire on the branch.
  - `/tldr [...]` — Top-level entry point for `/workbench tldr`.
- **Flags:**
  - `--list-tools` — CLI flag to dump registered tool schemas (`tools-inspector`).

---

# 2. Full Hashline Edit Engine (`oh-my-pi` Port)

We will port the `oh-my-pi` hashline engine (`crates/pi-edit/src/modes/hashline/` + `crates/pi-edit/src/store.rs`) to Go in `internal/hashline/`:

## 2.1 Snapshot Tags & `EditStore`
- **4-hex XXH32 tag (`[PATH#TAG]`):** Computed identically to `oh-my-pi` (`Xxh32(0)` over LF-normalized, BOM-stripped text with trailing spaces/tabs/CRs trimmed per line, masked `& 0xffff` as `%04X`).
- **Session `EditStore`:** LRU-bounded store (256 paths, 4 versions per path, 64 MiB max retained bytes, 4 MiB max file size):
  - Records full-file snapshots and `seen_lines` (`BTreeSet<uint32>`) whenever `read` or `grep` emits numbered lines, and unrestricted snapshots after `write` or `edit`.
  - Retains session clipboard registers (`@name` across calls; anonymous register within a batch).
  - Tracks consecutive identical no-op payloads per path (`NOOP_HARD_LIMIT = 3`) to break model edit loops.

## 2.2 Patch Grammar & Operations
- **Section headers:** `[path#TAG]` (plus tolerance for `[*** Update File: path#TAG]` noise and optional `*** Begin Patch` / `*** End Patch` envelopes).
- **Operations:**
  - `PUT N.=M:` — Replace inclusive line range `N..=M` with `+` payload lines (`N.=N` for single line).
  - `PUT <N:` / `PUT >N:` / `PUT <1:` / `PUT >$:` — Insert before line `N`, after line `N`, at BOF, or at EOF.
  - `PUT N*:` — Replace the syntactic block opening at line `N`.
  - `PUT >N*:` — Insert after the syntactic block opening at line `N` at sibling depth.
  - `CUT N.=M [@name]` / `CUT N* [@name]` — Delete range or syntactic block and capture into anonymous or `@name` register.
  - `PUT <N [@name]` / `PUT >N [@name]` / `PUT >N* [@name]` / `PUT N.=M @name` / `PUT N* @name` — Paste from register.
  - `REM` — Delete the file.
  - `MV <dest>` — Move/rename the file after applying section edits.
- **Body & Prefix Rules:**
  - Payload rows start with `+` (`+` alone is an empty line; `+-` / `++` escapes leading `-`/`+` bullets).
  - Automatic recovery/warnings for common model mistakes (copied `N:` hashline prefixes inside `+` rows, bare ranges auto-lowered to `PUT`, empty `PUT` lowered to `CUT`, `-` diff rows stripped with warning).

## 2.3 Block Resolution (`N*`)
- Resolves the multi-line construct starting at line `N` (including leading decorators/attributes/doc-comments, or Markdown heading sections through deeper headings):
  - **AST / structural resolver in Go:** Native Go parser for `.go` (`go/parser`), Markdown heading block scanner for `.md`, indentation/decorator-aware lexical + brace/statement-sequence block resolver for TS/JS/Rust/Python/C/JSON/TOML/YAML (matching `pi-ast/src/block.rs` rules: climb to outermost construct starting on `row` without swallowing statement-sequence containers, reject single-line spans with enclosing/next block suggestions).

## 2.4 Safety, Seen-Line Guard & Auto-Repair
- **Seen-line enforcement (`edit.enforceSeenLines = true`):** Rejects edits touching lines not yet displayed in `read`/`grep` (or cut by column caps/elisions), returning an `UnseenLinesReveal` diagnostic that prints the unseen lines around the anchors and marks them seen for the retry.
- **Tag mismatch & shift rescue:** If `TAG` does not match the current file on disk:
  - Checks if `TAG` matches an earlier `EditStore` snapshot of the file and uses Myers diff (`shifted_images`) to safely relocate replacement anchors when the targeted lines are inside an unchanged run and carry `>= 8` non-whitespace evidence chars.
  - Checks if `TAG` came from a *different* file in `EditStore` (`tag_origin_paths`) and tells the model which file that tag belonged to.
  - Otherwise returns a rich mismatch error with the current `[path#NEW_TAG]` and numbered context windows around every attempted anchor (recording those revealed lines as seen).
- **Dual Edit Mode:** Supports `edit.mode = "hashline" | "replace" | "auto"` in config so models/users can use `hashline` patches (`{"input": "..."}`) or exact-text multi-replacement (`{"path": "...", "edits": [{"oldText": "...", "newText": "..."}]}`).

---

# 3. Full Optimized `read` Tool (`oh-my-pi` + `nugent` Port)

`read` takes `{"path": "..."}` (or explicit `{"paths": [...]}`) and implements the complete pipeline:

1. **Inline Selector Grammar (`parseSel` + `ToolPath`):**
   - Literal file probe first so real filenames containing `:` always win.
   - `:N` or `:N-` (from line `N`), `:N-M` (inclusive range), `:N+K` (`K` lines starting at `N`), `:-N` (last `N` lines), comma-separated multi-ranges (`:5-16,960-973`, `:1-12,-15`).
   - `:raw` (verbatim without `[path#TAG]` header, line numbers, or context expansion; combinable as `:50-100:raw` or `:raw:50-100`).
   - `:conflicts` (lists unresolved `<<<<<<<` merge conflict blocks and registers `conflict://<id>` URIs).
   - `:img` (rasterizes `.svg`/`.svgz`) and `?q=<question>` (optional image question hook when configured).
2. **Delimited Multi-Path Reads:**
   - Supports reading multiple paths in one call (via `paths: string[]` or comma/whitespace-delimited path lists when each part resolves to a path/URI), running concurrently with deterministic output order and a shared output budget.
3. **Hashline Output & Contextual Windowing:**
   - Prepends `[displayPath#TAG]` and formats lines as `LINE:TEXT`.
   - Single non-raw range automatically includes `1` leading and `3` trailing context lines.
   - **Enclosing Block Context (`block-context`):** For windowed or multi-range non-raw reads, scans enclosing syntax/brackets and injects outer opener/closer boundary lines (`⋯` elision markers) so the model always sees the function/struct signature and closing brace around a slice.
   - Records only fully emitted, non-truncated lines in `EditStore` (`recordFullySeenRows`).
4. **Structural Code Summaries (Bare Reads of Large Code Files):**
   - When reading a bare code file exceeding `read.summarize.minTotalLines` (and not prose/Markdown unless opted in), produces a declaration/signature skeleton with line-range fold markers (`… lines A-B elided …`) and a footer instructing the model to `read path:A-B` for bodies.
5. **Specialized Content Readers:**
   - **Directories & Virtual Containers:** Depth-1 or tree listing (child cap with `… N more`), pageable via `:N-M` / `:-N`.
   - **SQLite (`.db`, `.sqlite`, `.sqlite3`):** Sniffs SQLite header; `file.db` lists tables + row counts; `file.db:table` shows schema + sample rows; `file.db:table:key` looks up by PK/rowid; `file.db?q=SELECT...` runs read-only queries (via `sqlite3` CLI or pure-Go reader).
   - **JSON / JSONL / NDJSON (`file.json?q=<filter>`):** Runs `jq`/`jaq` or built-in query with `&raw=true`, `&compact=true`, `&offset=&limit=` pagination and continuation hints.
   - **Archives (`.zip`, `.tar.gz`, `.tgz`, `.tar.bz2`, `.tar.xz`, `.jar`, `.whl`):** Lists archive entries (`archive.zip`) and reads members with line selectors (`archive.zip:path/to/file.go:10-50`), backed by an inode/mtime/size-keyed LRU index cache.
   - **Binaries (ELF/PE/Mach-O):** Sniffs magic bytes; instead of dumping garbage, renders header overview, symbols/exports/imports/strings (`bin:imports`, `bin:exports`, `bin:strings`, `bin:<sym>:asm` via `nm`/`objdump`/`readelf` when available), pageable by line range.
   - **PDF / Documents & Static Web URLs (`http://`, `https://`):** Extracts readable text/markdown with `:raw` fallback and line-range paging.
   - **Images (`.png`, `.jpg`, `.gif`, `.webp`, `.bmp`):** Decoded inline as image content blocks with dimension notes.
6. **Read Loop Guard & Supersede Tracking:**
   - Tracks repeated identical `(path, selector, contentHash)` reads in the session; after repeated identical reads, appends a loop-breaking hint (`[You have received this identical output N times...]`).
   - Attaches `ReadDetails` (`complete`, `shown`, `totalLines`, `next`, `sourcePath`, `summary`) so a subsequent complete read of a file marks earlier partial reads as superseded.

---

# 4. `scheme://` Virtual Filesystem Router

Unified `SchemeRouter` (`internal/scheme/`) modelled after `oh-my-pi`'s `InternalUrlRouter` and `nugent`'s `SchemeProvider` trait. Every scheme declares a `SchemeSpec` (`Backing`: `file | virtual | remote | device`, `Selectors`: `lines | none | opaque`, `Immutable`, `Unbounded`, `Linkable`, `WritePolicy`: `none | file | handler`).

## 4.1 Sessions & History (`history://`, `session://`, `branch://`)
Combines `oh-my-pi`'s `history-protocol.ts` and `nugent`'s `HistorySchemeProvider` + `BranchSchemeProvider`, adapted to PiG's session storage (`~/.pig/agent/sessions/<encoded-project-cwd>/*.jsonl`):

- **Scopes & Listing:**
  - `history://` or `session://` or `session://project` — Lists all sessions for the **current project** (active session + on-disk JSONL files in the project's session directory), sorted by recent activity, with session ID, name, timestamp (`Nm/Nh/Nd ago`), entry count, and first user prompt preview.
  - `session://global` or `history://global` — Lists sessions across **all projects** under `<agent-dir>/sessions/`, grouped/annotated by project cwd.
  - `session://search?q=<query>[&scope=project|global]` — Searches user/assistant/tool messages and session names across project or global JSONL files without hydrating full transcripts into memory.
- **Current Session & Branch Inspection:**
  - `branch://` or `history://current` — Lists the entry index of the active branch (entry IDs, roles/types, timestamps, tool names, token/summary markers) without hydrating large payloads.
  - `history://current/full` (supports `:N-M`, `:-N`) — Renders the full current branch transcript in clean Markdown sections (`### user`, `### assistant`, `#### tool call`, `### tool result`, `### compaction summary`) with base64 images and encrypted thinking blobs redacted to metadata markers.
- **Reading Specific Sessions & Entries:**
  - `session://<session-id>` or `history://<session-id>` — Renders the overview and entry table of that session (project or global), or `/full` for its transcript (pageable with `:N-M` / `:-N`).
  - `history://<session-id>/<entry-id>` or `branch://<entry-id>` — Reads one specific entry or tool result payload in full.

## 4.2 Skills (`skill://` and `skills://` alias)
Combines `oh-my-pi`'s `SkillProtocolHandler` and `nugent`'s `SkillSchemeProvider`:
- `skill://` or `skills://` — Lists all discovered, trust-resolved skills (`name`, description, source scope, size).
- `skill://<name>` or `skills://<name>` — Reads the skill's `SKILL.md` body (frontmatter stripped by default; `skill://<name>:raw` keeps frontmatter; `unbounded: true` so skill instructions are never truncated mid-rule).
- `skill://<name>/<relative-path>` (and `skill://<namespace>/<name>/<relative-path>`) — Reads or locates relative supporting files, scripts, references, or assets inside the skill's directory, with strict path-traversal and package-root containment checks (`resolveContainedPath`).
- Supports `locate()` so `read`, `grep`, and `find` can search inside `skill://<name>/references/**`.

## 4.3 Background Jobs (`jobs://` and `proc://` alias)
Combines `oh-my-pi`'s `proc-protocol.ts` with `pi-workbench`'s job manager:
- `jobs://` or `proc://` — Lists all running and retained finished background jobs (`id`, `state`, `exit_code`, `duration_s`, `location`, `command`, `log_path`).
- `jobs://<id>` — Reads job status header plus the tail/page of its output log (supports line selectors `:1-100`, `:-80`, `:raw`).
- `jobs://<id>/log` or `jobs://<id>/streams` — Locates/reads the underlying durable log file directly through the streaming pager.
- `write jobs://<id>` with `content` — Sends `content` to the running job's stdin.
- `write jobs://<id>/eof` — Closes stdin (`EOF`).
- `write jobs://<id>/kill` (no content required) — Terminates the job's process group (allowed even in read-only/plan modes because `cancels: true`).

## 4.4 Additional Core Schemes
- **`file://`** — Local filesystem access (bypasses active SSH working location when explicitly specified as `file:///abs/path`).
- **`ssh://`** — `ssh://[user@]host[:port]/abs/path` and `ssh://host/~/path`. Routes `read`, `write`, `edit`, `ls`, `find`, `grep`, and `bash` over SSH using the shared `Remote` transport.
- **`artifact://`** — Session-scoped overflow artifacts (`artifact://0`, `artifact://1:100-200`). Large tool outputs spill to `<state-dir>/artifacts/<session>/<id>.txt` and can be paged without re-spilling.
- **`local://`** — Session-scoped writable scratchpad directory (`<tmpdir>/pig-workbench-local/<session-id>/...`, aliased from `local:/...` and mistyped `/cwd/local://...`), writable via `write` and `edit`.
- **`conflict://`** — Git merge-conflict blocks registered by `read <path>:conflicts`. Read `conflict://<N>` or `conflict://<N>/<ours|theirs|base>`; resolve by calling `write conflict://<N>` with the merged text.
- **`cfg://`** — Inspect (`read cfg://`, `read cfg://instructions`) and update (`write cfg://<key>` / `write cfg://<key>/save`) Workbench settings with user confirmation in TUI/RPC when mutating persistent config.
- **`harness://` (aliases `self://`, `workbench://`)** —
  - `read harness://status` — Active location, revision, edit mode, loaded instruction/ambient files, running jobs count, and reload epoch.
  - `write harness://reload` (`1` / `true` / `reload`) — Queues a clean extension/resource reload at the next step boundary.
  - `read harness://instructions` / `write harness://instructions/pin` — Inspect or pin/unpin ambient files.

---

# 5. Instruction & Ambient File Injection (`AGENTS.md`, `BACKLOG.md`, Custom Classes)

We will port `pi-workbench/src/instructions/` + `nugent`'s `InstructionLedger` and upgrade it with **Rule-Based Ambient File Classes**.

## 5.1 File Classes & Placement Policies
Instead of treating all instruction files identically, `instructions.classes` defines ordered file rules. Built-in defaults:

1. **Steering instructions (`AGENTS.override.md`, `AGENTS.md`):**
   - `filenames`: `["AGENTS.override.md", "AGENTS.md"]` (first match per directory wins).
   - `placement`: `"ancestors"` — Injects global (`<agent-dir>/AGENTS.md`), SSH host notes (`HOST_AGENTS.md`, `hosts/<alias>.md`), and every directory from boundary root (`project` | `home` | `root`) down to `cwd` and any directory touched by `read`/`ls`/`edit`/`write`/`grep`/`find`.
   - `updateMode`: `"diff"` (unified diff during active runs if shorter than full text; full replacement otherwise).
2. **Backlog pointers (`BACKLOG.md`):**
   - `filenames`: `["BACKLOG.md"]`
   - `placement`: `"ancestors"` — Auto-injected for the active/touched directory plus all ancestors up to the boundary root, matching the `dot-project` / `nukular` standard.
   - `maxItems`: `25` (configurable; counts `- B-NNNN ...` or non-empty item lines). When a `BACKLOG.md` exceeds `maxItems` (or `maxLines` / `maxBytes`), the harness injects the first `N` items followed by:
     `(+K more → <repo-relative-or-URI-path>)`
     so agents know more items exist, can `read <path>` when needed, and **never** trim `BACKLOG.md` to keep it small.
   - `updateMode`: `"diff"` — Emits a `<system-instruction>` diff frame whenever `BACKLOG.md` changes on disk (including when an agent or user adds/removes a pointer line).
3. **User-Defined Multi-Location Files (`root_and_nearest` / `pinned`):**
   - Users can configure additional file patterns in `instructions.classes` (or `instructions.additionalFiles`), e.g.:
     ```json
     {
       "id": "architecture-notes",
       "filenames": ["ARCHITECTURE.md", "DESIGN.md", ".context.md"],
       "placement": "root+nearest",
       "allowPin": true,
       "maxLines": 200,
       "updateMode": "diff"
     }
     ```
   - **Placement modes supported per class:**
     - `"ancestors"` — Every matching directory from boundary root to target directory (broadest first).
     - `"root+nearest"` — If the file appears at multiple levels in the tree, inject **only** the broadest (root-most) file **and** the narrowest (nearest to the active `cwd` or touched file) file. Intermediate copies are skipped automatically, and moving deeper via `cd` or file touch withdraws the previous "nearest" file and injects the new "nearest" file (or emits a `relocated` frame if byte-identical).
     - `"nearest"` — Only the closest ancestor match to the active/touched path.
     - `"root"` — Only the boundary/project root match.
   - **Pinning (`/workbench instructions pin <path>` or `write harness://instructions/pin`):**
     - Either the user or the agent can pin a specific multi-location file instance (e.g. `crates/foo/BACKLOG.md` or `services/api/ARCHITECTURE.md`) for the session/branch.
     - Pinned files stay active in the ledger across `cd` transitions (never withdrawn until unpinned or removed) and still receive live diff updates when modified.

## 5.2 Ledger, Diff Updates, Deduplication & Compaction
- **Suppression of Built-in Duplicate Context:** Strips PiG's default `project_context` system-prompt section (when `instructions.enabled = true`) so `AGENTS.md` is never injected twice, and adds the `harness_frames` and `instruction_files` system-prompt sections.
- **Live Change Detection:**
  - Local files: `fsnotify` / poll-debounced watcher (`debounceMs`), plus pre-turn verification at `before_agent_start` and post-tool checks on mutating tools (`write`, `edit`, `bash`).
  - Remote SSH hosts: `inotifywait` stream when available (`watch.remote = "auto"`), falling back to `on-turn` stat/hash checks.
- **Diff & Lifecycle Frames (`<system-instruction>`):**
  - **Initial load:** Full content (or capped first `N` items/lines + `+K more → path` footer).
  - **Changed on disk:** Unified diff (`+`/`-` hunks with configurable context lines) when shorter than full text; otherwise full text.
  - **Self-edits:** When the agent itself edits a loaded file via `edit`/`write`, emits a concise 1-line `self-edit` notice (`instructions.selfEdits = "notice"`) and updates the ledger baseline without echoing the diff the agent just wrote.
  - **Byte-identical deduplication (`SkipIdentical`):** Linked git worktrees (`.worktrees/<branch>`) or duplicate files with equal trust and identical bytes are skipped with an info row (`Skipped Identical Instructions: <path> · same as <other>`).
  - **Relocation (`Relocated`):** On `cd`, if a newly applicable file is byte-identical to a withdrawn file in the same epoch, emits a 1-line relocation frame referencing the earlier `seq` instead of resending the body.
  - **Withdrawal & Removal:** Emits a `withdrawn` frame when `cd` moves out of a file's scope (or when a nearer file supersedes the previous nearest in `root+nearest` mode), and a `warning` frame if a loaded file is deleted.
  - **Compaction & Branch Replay:** Rebuilds the `InstructionLedger` from `wb-frame` entries on the active branch (`GetBranch()`) on startup, `/tree` navigation, `/fork`, and `/resume`. After compaction, increments the ledger epoch and re-injects the full active baseline (plus pinned files; nested files follow `afterCompaction = "reinject" | "on-touch"`).

---

# 6. Message Framing, Event Bus, `cd`, `ask_user`, `jobs`, `tldr`, and Renderers

## 6.1 Harness Frames (`message-framing`)
- Exact port of `pi-workbench/src/message-framing/`:
  - Frame kinds & glyphs: `instruction` (`◆`), `reminder` (`↻`), `warning` (`▲`), `error` (`✗`), `notice` (`●`), `info` (`i`).
  - Envelope: `<system-{kind} src="..." file="..." seq="..." n="..." ts="..." [trust="untrusted"]>\n{body}\n</system-{kind}>` with XML attribute escaping and `</system-` body escaping (`<\/system-`).
  - Session 8-hex random nonce `n` announced in the `harness_frames` system prompt section and persisted across branch replay.
  - Stored as custom entries (`customType: "wb-frame"`, structured `FrameDetails`), projected before provider requests to native mid-conversation `system`/`developer` role messages when supported by the model (`supportsMidConvoSystemMessages`) and `trust != "untrusted"` and not trailing an assistant message; otherwise user role.
  - TUI rendering styles (`row`, `rail`, `card`) and visibility rules (`shown`, `collapsed`, `hidden` per source glob and kind).

## 6.2 Event Bus (`ctx.Events()`)
- Bridges `pi-workbench` event channels over PiG's `EventBus`:
  - `workbench:frame` — Any extension can emit a `FrameRequest` (`kind`, `source`, `file`, `title`, `summary`, `body`, `trust`) and Workbench sequences, persists, renders, and delivers it.
  - `workbench:location` — Broadcasts `{ location, origin, revision, reason }` on every `cd` / `/remote` transition.
  - `workbench:service` / `workbench:service-request` — Exposes location resolution and statusline/footer row contributions to cooperating extensions.
  - `pi-ask:*` (`bridge-discover`, `bridge-ready`, `started`, `completed`, `submit`, `submit-result`) — Full remote questionnaire contract for external bridges.

## 6.3 Working Locations, SSH & `cd`
- Full port of `WorkingLocation` (`workbench-location-v2` session records, origin tracking, revision counter, foreground leases, exclusive transition lock).
- Full SSH host config parser & writer (`~/.ssh/config` + `Include` files), `<agent-dir>/hosts/<alias>.md` TOML frontmatter notes (`description`, `shell`, `hidden`), `/remote` TUI host manager panel, and SSH Multiplexing (`ControlMaster auto`) transport.
- Git worktree resolution (`cd` with `branch`): locates repo root locally or over SSH, reuses existing worktree for `branch` or creates `<repo>/.worktrees/<slug>`, checks dirty status, and reconciles instruction/ambient files.

## 6.4 Unified `jobs` Tool & Background Shell Supervisor
- `bash` runs commands in the resolved local or SSH working location. Supports `background: true` and foreground yield timeout.
- Process-group isolation (`Setpgid: true`) + pipe-based crash watchdog process so if the extension or PiG process dies with `SIGKILL`, child process groups (and remote SSH PIDs) are reaped without polling.
- Single `jobs` tool with `action`:
  - `list`: running jobs + up to `maxFinished` (default 20) recent jobs.
  - `status`: single job details + last `lines` (default 40) of log output.
  - `output`: read log output by `lines` tail or byte `offset` cursor.
  - `wait`: wait up to `timeout` seconds for one or any running job to finish or produce output.
  - `write`: send `input` string and/or `key` (`Enter`, `Ctrl+C`, `Ctrl+D`, `Tab`, `Esc`, `Up`, `Down`) or `eof: true` to job stdin.
  - `kill`: send `SIGTERM` / `SIGKILL` (or specified signal) to the local or remote process group.
- When a background job finishes, emits a `jobs.finished` notice frame and wakes the agent (when configured).

## 6.5 `ask_user` & `/answer`
- Full port of `pi-workbench/src/ask/`:
  - Multi-question tabbed flow, `single` / `multi` / `preview` question types, `recommended` markers, always-available custom freeform input editor, review screen before submission, dismiss confirmation guard, and optional auto-submit on single-question single-select.
  - Pending questionnaire persistence in session entries so interrupted sessions recover unanswered forms on resume.
  - `/answer` extracts questions from the latest assistant message using the configured extraction model and opens the `ask_user` UI; `/answer:again` replays the latest form on the branch.
  - RPC mode fallback using `ctx.Select` / `ctx.Input` / `ctx.Editor` / `ctx.Confirm`.

## 6.6 `/tldr` Branch Checkpoints & `/tools` Inspector
- `/tldr`: Segments the current branch since the last checkpoint, runs bounded parallel note extraction + synthesis on the configured model/thinking level, redacts secrets via `SecretGuard`, and appends a collapsible `tldr` recovery card entry.
- `/tools` (`/workbench tools`, `--list-tools`): Inspects `ctx.GetAllTools()`, formats parameters/schemas in text, Markdown, or JSON, and provides a searchable kit `SelectList` + `ScrollView` detail inspector in the TUI.

## 6.7 Renderers & Config
- Compact, normal, and full call/result renderers for all Workbench tools plus `codemode`, using one consistent header grammar (`✓`/`✗`/`▲`/spinner + target + exact metrics from `Details`), fish-style path shortening at narrow widths, and strict width fitting at `36 / 57 / 80 / 140` columns.
- Layered config (`defaults < <agent-dir>/pig-workbench.json < trusted <cwd>/.pig/pig-workbench.json < runtime`) with staged TUI settings editor (`/workbench settings`), unknown-key preservation, atomic `0600` writes, and file locking.

---

# 7. Proposed Repository Layout & Implementation Phases

```text
pig-workbench/
  go.mod
  extension.go                  # Extension() factory: registers tools, commands, events, renderers
  AGENTS.md
  README.md
  internal/
    config/                     # Layered JSON config, trust rules, settings panel
    location/                   # WorkingLocation, file:// & ssh:// resolver, SSH config/transport, git worktrees
    scheme/                     # SchemeRouter, ToolPath & Selector parser, Pager
      file.go                   # file:// and ssh:// provider
      history.go                # history://, session://, branch:// (current, project, global)
      skill.go                  # skill:// and skills:// provider
      jobs.go                   # jobs:// and proc:// provider
      artifact.go               # artifact:// and local:// scratchpad providers
      conflict.go               # conflict:// merge conflict registry & writer
      cfg.go                    # cfg:// settings reader/writer
      harness.go                # harness:// (status, reload, instructions pin)
    hashline/                   # Full oh-my-pi hashline engine
      store.go                  # XXH32 4-hex tags, EditStore snapshots, seen_lines, clipboard, no-op guard
      tokenizer.go              # Patch tokenizer & prefix stripper
      parser.go                 # PUT/CUT/REM/MV &N* block/range/gap executor
      block.go                  # AST & lexical block resolver (N*, >N*, enclosing boundaries)
      patcher.go                # Stage/commit, shift rescue, mismatch reveal, auto-repair
    read/                       # Full read tool: selectors, multi-path, summaries, block context, sqlite/json/archive/binary/url
    tools/                      # write, edit, grep, find, ls, bash, cd, jobs tool handlers
    jobs/                       # Background job supervisor, watchdog, log tailer, /workbench jobs panel
    frames/                     # wb-frame envelope, nonce, projection, TUI renderer, /workbench frames
    instructions/               # Ambient & nested discovery, classes (ancestors, root+nearest, pinned), item caps (+K more), diff engine, ledger
    ask/                        # ask_user tool, TUI kit view, RPC fallback, pending store, /answer extractor, remote bridge
    codemode/                   # NestedCaptures, scriptFacts, codemode call/result renderer
    tldr/                       # /tldr branch summary generator & card renderer
    inspector/                  # /tools & --list-tools schema inspector
    ui/                         # Shared width-fitting, plain() sanitizer, fish path shortener, header/row builders
```

## Implementation Waves

1. **Wave 1 — Core Foundation, Config, Location & `scheme://` Router:**
   - Scaffold Go factory (`extension.go`), `internal/ui`, `internal/config`, `internal/location` (local + SSH + `cd` + worktrees), and `internal/scheme` (`ToolPath`, `Selector`, `Pager`, `file://`, `ssh://`, `artifact://`, `local://`, `harness://`, `cfg://`).
2. **Wave 2 — Full Hashline Engine & Enhanced Builtin Tools (`read`, `edit`, `write`, `grep`, `find`, `ls`):**
   - Implement `internal/hashline` (`EditStore`, 4-hex XXH32 tags, parser, block resolver, seen-line guard, shift rescue, `replace` fallback).
   - Implement `internal/read` (inline selectors, multi-path, `[path#TAG]` formatting, enclosing block context, structural code summaries, SQLite/JSON/archive/binary/URL readers, loop guard, `conflict://` registration).
   - Wire `write`, `grep` (with seen-line recording), `find`, and `ls`.
3. **Wave 3 — Background Jobs (`bash`, `jobs`, `jobs://`), `session://`/`history://`, and `skill://`:**
   - Implement process-group job supervisor, crash watchdog, unified `jobs` tool, `jobs://` (`proc://`) scheme, `/workbench jobs` panel.
   - Implement `history://` / `session://` / `branch://` (current, project, global JSONL index & search) and `skill://` / `skills://`.
4. **Wave 4 — Harness Frames & Enhanced Instruction / `BACKLOG.md` Injection:**
   - Implement `wb-frame` envelope, nonce, role projection, and event bus (`workbench:frame`, `workbench:location`, `workbench:service`).
   - Implement `internal/instructions` with configurable classes (`ancestors`, `root+nearest`, `nearest`, `root`), pinning (`/workbench instructions pin`), item/line caps (`+K more → path`) for `BACKLOG.md`, live diff frames, deduplication, relocation, and compaction recovery.
5. **Wave 5 — `ask_user`, `/answer`, `codemode` Renderer, `/tldr`, `/tools`, `/remote` & Verification Gate:**
   - Implement `ask_user` TUI/RPC flows, `/answer` & `/answer:again`, remote-ask bus protocol.
   - Implement `codemode` nested capture & tree renderer, `/tldr`, `/tools` (`--list-tools`), `/remote` host panel, and run the full verification matrix (`36 / 57 / 80 / 140` TUI captures, race tests, `pig install --validate-only`).
