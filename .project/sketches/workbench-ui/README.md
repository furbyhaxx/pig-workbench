---
document:
  type: "sketch"
  id: "SKETCH-0001"
  title: "Workbench UI and UX design"
  description: "Reviewable terminal wireframes, navigation, interaction states and responsive/headless behavior"
  status: "draft"
  revision: 6
  created_at: "2026-11-02T15:31:51+01:00"
  updated_at: "2026-11-03T12:29:54+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 6
      reason: "System prompt inspector (SPEC-0014)"
      changes: ["Surface row and §13 wireframe for /workbench prompt"]
---

# Workbench UI/UX

Detailed companions: [extension APIs and renderer states](../extension-apis/README.md), [remaining feature workflows](../workflow-details/README.md). Contracts: `.project/specs/0001-extension-services.md` through `0010-configuration-state-trust.md`.

Companion to `.project/plans/0001-pig-workbench.md`. These are **proposed wireframes, not live captures or approved layouts**. Example paths, IDs, counts and times are fixture data. Text layouts illustrate information hierarchy; actual rendering must measure terminal columns, not copy padding from these diagrams.

## 1. Design direction

- One recognizable panel shell: accent border, breadcrumb/title, scoped state, searchable content, selected-item description, contextual hints, closing border.
- Kit views first. No custom terminal editor, competing footer owner or decorative dashboard.
- Identity → primary state → action → secondary metrics is the priority at every width.
- Commands are stable entrypoints: `/workbench` is the hub; `/tools`, `/remote`, `/cd`, `/answer`, `/tldr` and other specified companions enter the same views directly.
- Inspection is read-only unless an action explicitly mutates something. Merely opening a URI, row or preview does not run a command, change location, pin a file or write configuration.
- Color reinforces words/glyphs; errors and incomplete results cannot disappear into collapsed success groups.

### Navigation grammar

Hints below use semantic labels such as `[open]`, `[back]`, `[expand]`. The implementation substitutes the user's bindings through SDK key helpers. Default conventions are Enter to open/confirm, Esc to clear search then go back, Ctrl+C to dismiss, arrows/page keys to navigate, Space to toggle, Tab/Shift+Tab to change focus/section. Only panel-owned modified accelerators may be literal, such as Ctrl+S for save. Never use bare-letter actions in a searchable list.

Preserve search, selection and scroll when returning from detail. Restore by stable entity ID, not list position. Mouse first selects, second activates; every mouse action has a keyboard path. Async updates never steal focus. Session switch closes or invalidates session-bound views with an explicit notice.

## 2. Surface inventory

| Surface | Entry | Main purpose | Design section |
|---|---|---|---|
| Hub/status | `/workbench`, `/workbench show` | Find capabilities and inspect current state | 3 |
| Settings/trust | `/workbench settings`, `/workbench trust` | Stage scoped changes, inspect provenance | 4 |
| Tool catalog | `/tools`, `/workbench tools`, `--list-tools` | Discover schemas, exposure and owners | 5 |
| Tool transcript | Tool calls, `/workbench preview` | Understand intent, result, partiality and failure | 6 |
| Codemode transcript | Builtin codemode | Inspect script and nested call tree | 7 |
| Location/hosts | `/cd`, `/remote` | Navigate local/SSH/worktrees; manage host settings | 8 |
| Jobs | `/workbench jobs`, `/wb:jobs` | Monitor, inspect, input and cancel | 9 |
| Frames/context | `/workbench frames`, `/workbench instructions` | Inspect provenance, selection, updates and pins | 10 |
| Questionnaire | `ask_user`, `/answer`, replay | Answer single/multi/preview/freeform questions | 11 |
| Checkpoints | `/tldr` | Generate, inspect, regenerate and cancel summaries | 12 |
| Resource views | `read scheme://...` | Consistent catalog/history/skill/job displays | 13 |
| System prompt | `/workbench prompt`, `/workbench skills` | Template chain, family, delivery, parity, served sections; skill catalog | 13 |
| Status composition | `/workbench statusline` | Configure contributed segments without taking over host UI | 14 |

## 3. Hub

```text
────────────────────────────────────────────────────────
 Workbench                         local · ~/src/project
 Search features…
 → Tools                  10 registered
   Jobs                   2 running · 1 failed
   Instructions           5 active · 1 pinned
   Frames                 latest: backlog changed
   Locations & hosts      local
   Settings               runtime overrides: 2
   Checkpoints            latest: build investigation
 Inspect tool schemas, source and exposure.
 [open] inspect · [back] close
────────────────────────────────────────────────────────
```

Counts are omitted until known. The hub is not a second session dashboard. `/workbench show` produces a stable textual snapshot without opening the menu. Empty sections say “No jobs” or “No checkpoint”, not zero-shaped guesses after a failed query. A failed section retains its label and an inspectable error; it does not make the whole hub unusable.

## 4. Settings and trust

```text
────────────────────────────────────────────────────────
 Workbench › Settings › Read          user · unsaved
 Search settings…
 → Summary mode           structural       [user]
   Context before         1                [default]
   Context after          3                [project]
   Output byte limit      51200            [runtime]

 Summary mode
 Fold code bodies; exact ranges remain available.
 Saved: off  →  Staged: structural
 Effective after save: off (runtime override)
 [edit] value · [scope] change scope · Ctrl+S save
 [back] back
────────────────────────────────────────────────────────
```

- Scope is always visible: user, trusted project, runtime. A project denied trust is explained, not offered as an enabled save target.
- Reset removes an override from the selected scope. Show inherited value/source before confirming.
- Staged changes remain local until Save. Back/dismiss while dirty offers Save / Discard / Continue editing. Save failure preserves staged changes and gives the exact file/key diagnostic.
- Detect intervening file changes and rebase known keys safely or ask for reconciliation; never overwrite an externally edited file silently.
- Restart-only settings show “applies after reload”. Saving does not imply that a consumer already changed.
- Trust rules have a separate list/detail showing matched scope, precedence and effective decision. Adding a broader rule previews the newly included roots. Project settings cannot widen trust.
- `cfg://` mutations reuse these approval/validation semantics; a headless unavailable approval returns an error, not success.

## 5. Tool inspector

```text
────────────────────────────────────────────────────────
 Workbench › Tools
 Search name, description or namespace…
 → read        active   pig-workbench   read-only
   edit        active   pig-workbench   writes files
   jobs        active   pig-workbench   action-dependent
   codemode    active   host            delegates
 [open] schema · [back] close
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 Tools › read                       owner: pig-workbench
 Read files and resources with inline selectors.
 Exposure: active        Execution: parallel
 Parameters
   path   string         one target, including suffix
   paths  string[]       explicit batch; excludes path
 Examples
   src/main.go:20-60
   job://j12/log:-40
 Output: shown spans · completion · continuation
 [scroll] more · [back] tools
────────────────────────────────────────────────────────
```

Schema detail is scrollable; complex schema branches are not replaced by an ellipsis permanently. Catalog reflects actual registry metadata and conflicts. `--list-tools` exposes JSON/Markdown/text without terminal styling. It must not start an agent turn.

## 6. Tool transcript and preview gallery

### Density

```text
Compact
 ✓ read src/main.go:20-60 · 45 lines · partial

Normal
 ✓ read src/main.go:20-60 · 45 lines · partial
   [src/main.go#1A2B]
   19:func serve() {
   20:    ...
   … preview hidden · [expand]
   More source: src/main.go:64-

Expanded
 ✓ read src/main.go:20-60 · 45 lines · partial
   [source view, scrollable where supported]
   Requested: 20-60 · shown: 19-63
   Whole file: no · selected range: complete
   More source: src/main.go:64-
```

Preview clipping, tool-result truncation and source elision are separate signals. Expanding the transcript does not pretend to fetch omitted source. `:raw` content stays byte-faithful in model output; the TUI may style/sanitize its display without changing the underlying result.

### Edit and recovery

```text
 ✓ edit src/main.go · 2 operations · +4 −2
   PUT 20.=22 · PUT >40
   [bounded unified diff preview]
   Snapshot: 1A2B → 3C4D

 ▲ edit src/main.go · stale snapshot · not applied
   Expected 1A2B; current 7E90
   [numbered recovery context]
   Read current anchors before retrying.

 ▲ edit 3 files · 2 committed · 1 failed
   ✓ src/a.go  updated
   ✓ src/b.go  moved → src/c.go
   ✗ src/d.go  permission denied
```

Do not show a failed/partially committed batch with an all-green header. Automatic recovery shows what was changed in interpretation and why; it is not hidden under success. Previews during streaming are labeled “preview”; only the final commit result is authoritative. File deletion/rename remain visually distinct from line replacement.

### Multi-read and specialized views

```text
 ▲ read 3 targets · 2 read · 1 failed
   ✓ src/a.go:1-30     30 lines
   ✓ data.db:users    schema + 5 rows
   ✗ notes.md        not found
```

Children retain requested URI and resolved identity. SQLite/JSON rows show query, returned count and next-page hint. Archives show container/member identity. Binary/document/media adapters show view type and backend errors. Full result details include provenance and whether coordinates are original-source or derived-view coordinates.

`/workbench preview` selects tool, density and fixture/live sample. Required samples: running, success, partial, failed, cancelled, stale edit, multi-file partial commit and hostile text. Replaying a sample renders it; it never re-executes the original tool.

## 7. Codemode

```text
 ▲ codemode · 3 calls · 1 failed · 1.2 s
   const results = await Promise.allSettled(...)
   ├ ✓ read src/a.go:1-40       40 lines
   ├ ✓ grep "serve"            6 matches
   └ ✗ read missing.go         not found
   Outputs: 2 text blocks
   [expand] script, calls and output
```

Expandable script, nested calls and output sections preserve the actual parent/child relation. Each nested Workbench call uses its normal renderer, not a second formatter. Full arguments/results come from captured structured data; a truncated host argument preview is labeled as such. Costs are displayed only when supplied and understood. Cancelled/running children stay visible; late updates cannot resurrect completed spinners. Historical calls without retained child payloads show “details not retained”, not fabricated output.

## 8. Locations, SSH hosts and worktrees

```text
────────────────────────────────────────────────────────
 Workbench › Location
 Current: ssh://build/srv/app         revision 7
 Origin: ~/src/app
 Search directory or branch…
 → ..                        parent
   services/                 directory
   feat/parser               existing worktree
 [open] inspect destination · [back] close
────────────────────────────────────────────────────────
```

Opening a candidate shows destination and instruction-scope changes before a mutating transition. Direct `/cd path` still reports previous/current/origin and worktree reuse/create result. A busy transition offers an actionable retry message; no spinner with an indefinite wait behind nested calls. State that this is **Workbench location, not host cwd**.

```text
────────────────────────────────────────────────────────
 Remote › build
 SSH alias: build          source: ~/.ssh/config:18
 Host: build.example       User: dev
 Command shell: inherited
 Note: Integration build host
 → Connect…
   Edit note…
   Command shell…
   Hide from picker
   Rename alias…
 [open] action · [back] hosts
────────────────────────────────────────────────────────
```

Host list is searchable and retains hidden-host indicators when configured to show them. Add/edit/rename use staged forms and explicit save; preview affected config files/aliases before write. Do not display private key contents or resolved credentials. Authentication/connection failures preserve the requested destination and offer retry/back, not a false location change. Worktree create confirms the exact branch/path; cancellation does not delete pre-existing worktrees.

## 9. Jobs

```text
────────────────────────────────────────────────────────
 Workbench › Jobs                  2 running · 1 failed
 Search title, command or ID…
 → j12  unit tests          running   12 s
   j13  dev server          running   4 m
   j11  release build       exited 1
 [open] inspect · [back] close
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 Jobs › unit tests (j12)                    running
 Location: ~/src/app
 Command: go test ./...
 [scrollable output]
 Follow: on · cursor 18420 · stderr merged
 → Send input…     Close stdin…     Stop job…
 [scroll] output · [focus] actions · [back] jobs
────────────────────────────────────────────────────────
```

Follow mode pauses when scrolling away; explicit Resume follow returns to tail. Rotation/quota gaps and output omissions are visible. Input editor identifies pipe vs PTY behavior and provides a send preview; key actions are disabled with explanations where unsupported. Stop confirms the specific running job (or group for a separately explicit bulk action). Closing the panel does not stop a job. Waiting cancellation stops waiting; Stop terminates the owned process. Finished jobs show known exit/signal facts; restarted sessions say interrupted/unknown instead of treating retained metadata as a live process.

## 10. Instructions, ambient files and frames

```text
────────────────────────────────────────────────────────
 Workbench › Instructions                  5 active
 Search class or path…
 → AGENTS.md                    root · trusted
   src/AGENTS.md                ancestor · trusted
   BACKLOG.md                   25 shown · 8 more
   src/BACKLOG.md                nearest path · active
   services/api/DESIGN.md        pinned
 [open] content and reason · [back] close
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 Instructions › BACKLOG.md
 Class: backlog              Policy: ancestors
 Reason: ancestor of src/runtime/worker.go
 Source version: 8           Delivered version: 7
 Pending: changed            View: first 25 items
 [+K more → BACKLOG.md shown in injected view]
 → Delivered view
   Source content
   Pending diff
   Pin this file
 [open] inspect/action · [back] files
────────────────────────────────────────────────────────
```

Default list is active context; filters expose shadowed/withdrawn/ignored/untrusted candidates. Explain root+nearest selection and pins. Source, delivered view and pending diff are distinct views. Pin/unpin names lifetime and scope; it cannot grant trust. Deleted pins remain inspectable as unavailable until cleared, not active content. Class editor stages filenames, placement, budgets and update policy with a preview of selected paths before saving.

```text
 ● instruction BACKLOG.md · changed · +1 −1
   [expand] delivered-view diff
 i instruction DESIGN.md · relocated · seq 18 reused
 ▲ instruction src/AGENTS.md · removed
```

Frame inspector shows sequence, source, time, trust, resource, delivery state and body. Visibility controls say “transcript only”; Hide never withdraws model context. Row/rail/card previews use the same content and metrics. Errors and lost-baseline recovery are inspectable. Do not call a frame trusted solely because its text has a matching nonce.

## 11. Ask and answer replay

```text
────────────────────────────────────────────────────────
 Workbench › Questions                      1 of 3
 Goal                 Scope                 Review
 Which storage behavior do you want?
 → ○ Project only
     Keep artifacts within this project.
   ○ Project and global          recommended
     Browse both with explicit scope.
   ○ Other…
 [choose] answer · [section] next · [back] dismiss
────────────────────────────────────────────────────────
```

Question tabs/steps are peers, not navigation depth. Single, multi, preview and freeform types share IDs and state; selected options use marks plus words, not color alone. Multi-select has explicit Next/Done rather than implicitly submitting on the first toggle. Preview content is inspectable before selection; narrow layouts open a separate detail view. Freeform entry never loses text when switching questions.

```text
────────────────────────────────────────────────────────
 Questions › Review
 ✓ Goal     project and global
 ✓ Scope    code, documentation
 ○ Notes    skipped
 → Submit answers
   Edit Goal…
   Edit Scope…
 [confirm] submit · [back] questions
────────────────────────────────────────────────────────
```

Dismiss with answers offers Continue / Discard; cancellation is not a submitted response. Required is advisory only (SPEC-0009): mark importance, but never block submission or invent an answer. Recovery offers Resume pending / Discard; replay identifies original branch entry and creates a fresh submission identity. Remote bridge submissions show source and reject expired/duplicate IDs. `/answer` shows cancellable extraction progress and an inspectable failure; it does not invent choices when extraction fails. Sensitive answers are not copied into an extra history archive by default.

## 12. Checkpoints

```text
 ● checkpoint · build investigation · 18 entries
   Goal · findings · decisions · remaining questions
   [expand] full recovery card

 ○ checkpoint generation · extracting notes 2/4
   [cancel] stop generation
```

Progress is stage-based, not a fabricated percentage. Regenerate previews the chosen source interval/instructions and leaves the old card available until success. Cancel stops generation; no partial result is labeled a completed checkpoint. Settings clearly distinguish summary model selection from the active conversation. Preserve user instructions and provider errors without logging credentials.

## 13. Scheme/resource UX

Resource inspection primarily uses enhanced read/list/search and their renderers; do not create an unrelated dashboard for every scheme.

```text
 ✓ ls session:// · 12 sessions
   ID       Title                  Last activity
   s-a12    parser investigation   4 m ago
   s-b34    storage review         2 h ago
   More: session://s-a12/transcript

 ✓ write session://self/title · "parser investigation" → "parser: hashline edge cases"

 ✓ ls skill:// · 8 enabled skills
   Name          Scope      Description
   review        user       Review changes

 ✓ read job://j12/log:-40 · 40 lines
   [job log view; current snapshot/cursor]
```

```text
 Workbench › Prompt
 Template  builtin/pig ← ~/.pig/agent/system-prompts/system.md
 Family    openai  (matched "*/gpt-*", user table)
 Delivery  force · parity ok · epoch 3 (since compaction 12:04)
 → preamble          212 B
   tools             640 B
   rules           1 980 B   changed in system.md (next epoch; diff frame sent)
   project_context 4 310 B   3 files
   cwd                31 B
 render --model …   diff vs PiG   open template
```

- Project/global scope is always visible. Global history access requiring approval explains which scope will be exposed; denying it leaks no search snippets.
- Session overview vs active branch vs full history have explicit labels; image payloads render as metadata unless intentionally requested through an appropriate view.
- Skill overview shows source and trust-resolved enablement. Supporting asset reads retain skill identity. Large bodies use continuation, never a silent partial rule set.
- Jobs URI controls and the jobs panel share confirmation/policy and input semantics.
- Unknown schemes show supported routes. Misspelled resource names do not silently trigger network work.

## 14. Status contribution and layout editor

### Paths at each width (SPEC-0003 §10)

Values computed with the donor formatter; row layout is illustrative.

```text
140  ✓ read services/api/internal/runtime/worker/scheduler.go:20-60 · 41 lines
80   ✓ read services/api/internal/runtime/worker/scheduler.go:20-60 · 41 lines
57   ✓ read s/a/i/r/worker/scheduler.go:20-60 · 41 lines
36   ✓ read …/scheduler.go:20-60
     ✓ read build:src/a.go                   (SSH authority is never shortened)
     ✓ read local:/tmp/x.log                 (local target beside a remote root)
     ✓ bash cd . && go test ./...            (literal root path shown relative)
```

Footer path segment, `statusline.path-style`, cwd `~/src/pig-workbench/.worktrees/feat-parser`, session started in `~/src/pig-workbench`, 24 cells:

```text
fish      ~/s/p/.w/feat-parser
full      ~/src/pig-workbench/.worktrees/feat-parser   (not shortened; layout priority decides)
relative  .worktrees/feat-parser
basename  feat-parser
```

Default contribution is short: `workbench local` or `jobs 2 running`, width-fitted and removable. A multi-line running-job widget is opt-in and clears when idle. Do not install a replacement footer as a side effect of loading Workbench.

`/workbench statusline` configures available contributed segments, ordering and visibility with a narrow-width preview. If a full layout requires a dedicated host/Piglet owner, the panel explains that prerequisite and offers the contribution-only configuration; it does not pretend to own the host footer. Designs for a full footer owner require a separate capture/review before activation.

### Activity line (SPEC-0009 §9)

```text
 ●●● Working 1m12s · 2 agents · 1 message · 2 jobs, 1 failed · 5 instructions, 1 changed · 12 skills · ⎇ feat-parser
╭──────────────────────────────────────────────────────────────────────────────────────────────────────────────────────╮
│ ❯ Ask anything                                                                                                       │
```

```text
80  ●●● Working 1m12s · 2 agents · 2 jobs, 1 failed · 5 instructions · 12 skills
57  ●●● Working · 2 agents · 2 jobs, 1 failed · 5 instr
36  ●●● Working · ⚇2 ⚙2✗1 ◆5
idle ● ∙ ∙ Ready · 12 skills            (zero counts hidden; skills pinned here as an example)
```

Fixture data, not captures. Click on an item opens its inspector; click on the state or `alt+w` opens the hub. Captures at 36/57/80/140 in fullscreen are required before implementation is accepted.

## 15. Responsive layouts

### 36-column detail: stack, preserve identity

```text
────────────────────────────────────
 Jobs › unit tests (j12)
 running · 12 s
 ~/src/app
 go test ./...
 [scrollable output]
 Follow: on
 → Send input…
   Stop job…
 [open] action · [back] jobs
────────────────────────────────────
```

### 57-column list: primary columns only

```text
─────────────────────────────────────────────────────────
 Workbench › Instructions
 Search path…
 → src/runtime/AGENTS.md        trusted · active
   BACKLOG.md                  25 shown · 8 more
   services/api/DESIGN.md       pinned
 [open] details · [back] close
─────────────────────────────────────────────────────────
```

At 80 columns use the standard single-column panels above, with bounded inline descriptions. At 140 columns **propose** a master/detail split for tools, jobs, instructions and preview questions if kit focus/layout behavior supports it; otherwise retain the same stacked view with a capped readable width. This is a design choice to demonstrate in captures, not an assumed SDK feature.

```text
140-column concept
┌──────────────────────── list ──────────────────────┬──────────────────────── detail ────────────────────────────────────────────────────────┐
│ Search…                                            │ Instructions › BACKLOG.md                                                             │
│ → BACKLOG.md         25 shown · 8 more              │ Selected because: ancestor of src/runtime/worker.go                                   │
│   src/AGENTS.md      trusted                       │ Delivered content / pending diff / source                                             │
│   DESIGN.md          pinned                        │ [scrollable detail]                                                                    │
└────────────────────────────────────────────────────┴───────────────────────────────────────────────────────────────────────────────────────┘
```

This split diagram shows regions, not a proposed second border style. Use the shared panel shell in actual UI. On resize, preserve focus/entity/scroll; collapsing the split must not discard edits. At ~16 rows, fix title and essential hints, scroll the body and open descriptions in detail; do not make all sections one-line tall. Drop secondary crumbs, duration and source columns before ID, state or failure. Full paths remain available in detail.

## 16. Interaction states and mode equivalents

| State | Required behavior |
|---|---|
| Loading | Named activity, cancellation where supported, no fake totals |
| Empty | Clear reason plus relevant action; distinguish filtered-out from no data |
| Failed | Operation/target/reason/remedy, retained input and retry/back |
| Partial | Explicit incomplete state, successful children retained, continuation |
| Permission denied | Scope/reason visible; no mutation or silent fallback |
| Dirty | Unsaved marker, explicit Save/Discard/Continue |
| Stale | Version/session changed notice; no action against an obsolete entity |
| Cancelled | Distinct from failure and submitted/success; no late UI resurrection |

| Surface | TUI | RPC | Print/JSON |
|---|---|---|---|
| Inspector/status/catalog | Kit panel/text detail | Text/custom message and basic dialogs where useful | Structured/text dump via tool or supported CLI flag |
| Settings/trust/host changes | Staged form and confirm | Native dialogs or explicit command arguments with confirmation | Explicit noninteractive contract only; refuse if approval is needed |
| Ask | Full questionnaire | Sequential select/input/editor/confirm using same answer state | Interaction-unavailable result; never hang |
| Jobs | List/detail/log/input | Text plus dialogs | Unified jobs tool and jobs URIs |
| Frames/instructions | Inspect/filter/pin | Text and scoped confirmations | Diagnostic/control schemes; no dropped-toast-only response |
| Preview/layout editor | Live kit view | Textual preview/unsupported action explanation | No interactive renderer; serialized samples where supported |
| Answer extraction/TLDR | Progress and result | Result/progress messages if supported | Explicit invocation/output contract; no implicit paid background request |

## 17. Design acceptance before UI implementation is settled

1. Review this information architecture, surface coverage and interaction rules with the user.
2. Produce fixture-backed kit prototypes of the common shell, tool/codemode rows, settings, jobs, instructions and ask flows before committing to shared layout abstractions.
3. Capture each prototype at 36, 57, 80 and 140 columns and short height; attach captures and evidence under this artifact or the producing analysis artifact. No real credentials, sessions or remote hosts in fixtures.
4. Present selected captures for layout approval. Record accepted changes and unresolved decisions in a new artifact revision.
5. During implementation, exercise keyboard, focus, scrolling, resize, error and headless states using real host paths—not only static render tests.
6. Before release, every inventory surface has an accepted layout, observed mode behavior and regression evidence for meaningful state transitions.

Current evidence: design/wireframes only. No kit prototype or live capture has been produced yet; those are explicit plan gates, not implied completion.
