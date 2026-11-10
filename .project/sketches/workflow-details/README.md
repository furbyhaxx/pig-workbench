---
document:
  type: "sketch"
  id: "SKETCH-0003"
  title: "Editing, reading, ambient context and interactive workflow designs"
  description: "Detailed flows and wireframes for remaining Workbench tools, locations, jobs, questionnaires, checkpoints and config recovery"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-10T15:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 2
      reason: "User decision 10.11.2026: no file watcher in Slice 1; instruction changes are found at boundaries"
      changes: ["The settings-conflict example no longer uses a removed watcher setting"]
---

# Scope and status

Companion to [SKETCH-0001](../workbench-ui/README.md), [API designs](../extension-apis/README.md) and [SPEC-0005](../../specs/0005-hashline-editing.md) through [SPEC-0010](../../specs/0010-configuration-state-trust.md).

These are **static proposed layouts**, not live captures. Existing panel shell, semantic key hints, theme tokens, accessibility and no-render-time-I/O rules apply. Fixture tags, revisions, counts and paths are illustrative, not usable edit anchors.

## 1. Hashline editing: outcome first, recovery second

```text
read tagged source → prepare patch → validate context/snapshot → apply
                            │                  │
                            │                  ├→ needs fresh instruction context
                            │                  ├→ unseen/stale/ambiguous source
                            │                  └→ denied/unsupported
                            └→ preview only; no mutation until validated
```

### Successful multi-file edit

```text
✓ edit 2 files · 3 operations · +12 −8
  ✓ src/old.go  CUT block 10-18 → @helper
  ✓ src/new.go  paste @helper after 20 · append 2 lines
  [expand] diff / new source anchors / provenance
```

Expanded view retains per-file before/after identities and exact applied spans. It does not show a global green “atomic” claim. The user can inspect a diff, but no new mandatory interactive review dialog is imposed on every already-authorized agent edit.

### Instruction barrier (not an edit failure to hide)

```text
▲ edit services/api/router.go · not applied
  New steering applies: services/api/AGENTS.md
  Instruction baseline queued before retry.
  Reconsider the change after reading the new instructions.
```

The result is available to the model, not just a human toast. Retrying the exact tool call without a new model continuation must not race around the barrier. UI may open the frame; opening it does not itself authorize execution.

### Stale or unseen anchors

```text
▲ edit src/parser.go · stale snapshot · not applied
  Expected: 1A2B   Current: 3C4D
  External edits intersect the requested block.
  41:func parse(...) {
  … bounded current context …
  Read: src/parser.go:41-88
  [expand] attempted operations / diagnostic
```

Wrong-resource tag names the likely source without guessing a new target. Collision/unseen diagnostics never claim an auto-repair succeeded. If safe shift rescue is applied, success header carries a warning detail: “anchors relocated through unchanged source”. Model-assisted repair is distinctly labeled and remains opt-in.

### Partial commit or unknown effect

```text
▲ edit 3 files · 2 committed · 1 not applied
  ✓ a.go updated
  ✓ b.go moved → c.go
  ✗ d.go permission denied
  Recovery: inspect receipt before retrying the batch.
```

No single Retry entire batch button. If an operation's effect is unknown, first action is Inspect status/reconcile. Clipboard detail shows only committed register changes; failed captures cannot look reusable.

## 2. Advanced reads: source vs derived view

### Structural code summary

```text
✓ read src/server.go · structural summary · partial source
  [src/server.go#1A2B]
  1:package server
  12:type Server struct {
  13-32:… body omitted …
  35:func (s *Server) Run() error {
  36-184:… body omitted …
  Read omitted bodies: src/server.go:13-32,36-184
```

Collapsed ranges are visibly not actual source rows. Expanding the transcript reveals retained summary only; a separate read obtains omitted bodies. Footer identifies whether omissions came from summarization, byte limit, line limit or column clipping.

### Multi-read

```text
▲ read 3 targets · 2 read · 1 failed
  ✓ src/a.go:19,59       2 selected lines + block context
  ✓ app.db:users        schema + 5 rows
  ✗ docs/missing.md     not found
  [expand] each result / source coverage
```

Output order matches input even if the database finishes first. Full requested targets are available in detail. Shared budget exhaustion marks remaining targets not emitted, not failed reads.

### Adapter views

```text
✓ read app.db:users · derived table · 5 rows
  Columns: id, name, active
  [bounded rows]
  More rows: app.db:users?offset=5&limit=5
  Source editing: use a supported database operation, not hashline.

✓ read package.zip:src/main.go:1-30 · archive member
  Archive: package.zip  Member: src/main.go
  [numbered member content; read-only source provenance]

✓ read manual.pdf:p2.png · page 2 image
  [image content]
  View: rendered PDF page · not extracted text

✗ read app:main · pseudocode backend unavailable
  Disassembly is available at app:main:asm.
  No pseudocode was generated.
```

Not every derived view needs a repetitive warning; source/editability appears in Details/expanded metadata and on attempted edit. Error alternatives must actually be available. Backend absence never triggers install, paid inference or network conversion automatically.

### Other required view states

| View | Normal presentation | Failure/expanded detail |
|---|---|---|
| JSON query | Filter summary, result count, next page | Parse/query error vs empty match; precision-preserved values |
| Binary | Format/architecture and functions | Selected slice, symbols/asm/pseudocode provenance |
| Notebook | Cell ID/type and source preview | Source-vs-output, metadata preserved, mapping/edit receipt |
| Raster/SVG | Dimensions and inline image | Decoded/resized vs original source; unsupported modality |
| Video | Contact sheet or frame/timestamp | Exact seek position, missing backend, timeout |
| Profile | Hot function/sample table with units | Known denominator, invalid/empty sample distinction |
| HTTP | Requested/final authorized URL, title, readable text | Redirect policy, extraction failure, raw alternative |
| Conflict | Source path, count and conflict URIs | Source revision, ours/theirs/base, resolved/stale state |

## 3. Instruction class editor and selection preview

```text
────────────────────────────────────────────────────────
 Settings › Instructions › architecture     user · unsaved
 → Filenames       ARCHITECTURE.md, DESIGN.md
   Placement       root+nearest
   Pin behavior    replace nearest
   View budget     200 lines / 16 KiB
   Update mode     auto (diff when smaller)
   Preview selection…
 [edit] value · Ctrl+S save · [back] back
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 architecture › Selection preview
 Target: services/api/router.go
 Boundary: /repo
 ✓ /repo/ARCHITECTURE.md                broadest match
 ○ /repo/services/ARCHITECTURE.md       intermediate
 ✓ /repo/services/api/DESIGN.md         nearest match
 Rule: first existing candidate per directory
 No changes applied.
 [back] class settings
────────────────────────────────────────────────────────
```

Preview resolves only authorized paths and is bounded. It is an inspection action, not a render-time scan. Show when selection used cached/stale data; Save revalidates before application. For BACKLOG.md the same preview shows every ancestor, not root+nearest.

### Pin review

```text
────────────────────────────────────────────────────────
 Pin › services/payments/DESIGN.md
 Class: architecture · branch-local
 Effect: replace automatic nearest on this project
 Root context stays: /repo/ARCHITECTURE.md
 Replaces: services/api/DESIGN.md
 Live updates: enabled
 → Pin this instance
   Cancel
 [confirm] choose · [back] cancel
────────────────────────────────────────────────────────
```

Agent pin request follows the same rules but returns a structured result, not an unexpected modal unless approval is required. Pin deletion produces an unavailable pin row and removal frame; no stale content displayed as active.

### Capped backlog and hidden changes

```text
◆ BACKLOG.md · 25 items injected · 8 more
  [delivered pointer view]
  (+8 more → BACKLOG.md)

i BACKLOG.md · source changed outside injected view
  View unchanged · 9 more items now available
```

If a full count was not obtained, show “more items; count unavailable”, never “+0”. Source view, injected view and pending diff are separate detail choices. No “trim file” action exists.

### Parallel scopes and compaction

```text
Instructions › architecture
Active reasons
  /repo/ARCHITECTURE.md           root
  /repo/api/DESIGN.md             batch 14 lease
  /repo/worker/DESIGN.md          batch 14 lease
Retained selection: batch 14 (2 targets)
```

After compaction: “baseline refreshed” rather than presenting a diff against absent content. Watcher failure shows stale state with next check/retry; it must not silently withdraw rules. Scope withdrawal says “no longer applies”, not “erased from history”.

## 4. Location transition and worktree UX

```text
────────────────────────────────────────────────────────
 Location › Switch to feat/parser
 Current: ~/src/app
 Destination: ~/src/app/.worktrees/feat-parser
 Branch: feat/parser · create from current HEAD
 Source worktree: modified (preserved)
 Context: project baseline will be reconciled
 → Create worktree and switch
   Cancel
 [confirm] choose · [back] locations
────────────────────────────────────────────────────────
```

Programmatic cd with branch explicitly requests creation and returns status; ordinary host policy determines whether confirmation is required. Preview is for user picker flow, not a silent new permission system.

Busy state lists pending mutating operation IDs and offers Back/Retry after completion; it does not offer Kill everything. An SSH failure leaves current location intact:

```text
✗ cd ssh://build/srv/app · connection failed
  Location unchanged: ~/src/app
  Host key verification failed; resolve SSH trust explicitly.
```

Host alias edit previews exactly which file/include block and notes change. Dirty/save conflicts use the shared config recovery design. Secret keys/passwords never appear in host detail. Remote origin and Workbench cwd are clearly distinguished from PiG host cwd.

## 5. Jobs: ownership, input and recovery

```text
────────────────────────────────────────────────────────
 Jobs › dev server (j13)               running · PTY
 Launch location: ssh://build/srv/app
 Output: follow on · cursor 5:18420
 [scrollable log]
 → Send text…
   Send key…
   Close input…
   Stop process group…
 [open] action · [focus] output · [back] jobs
────────────────────────────────────────────────────────
```

Input editor includes backend label and exact send action. Pipe Enter sends newline; Ctrl+C action is signal. PTY Ctrl+D is a terminal character, while Close input is separate. Unsupported keys remain discoverable but disabled with reason; they do not report success after writing meaningless bytes.

### Wait and process termination are different

```text
● wait j13 · no change within 30 s · still running
▲ j13 · timeout reached · stopping process group
● j13 · terminated by TERM · output retained
```

No “wait timed out” red failure badge on a healthy running job. Stop request remains stopping until observed exit. Unknown/disconnected state offers Inspect/reconcile, not a live Kill action against a saved PID.

### Output retention

```text
▲ j13 output · earlier output expired
  Lost bytes: 0–1048575 due to retention limit
  Continuing from available cursor 6:0
```

Follow stops when user scrolls away, resumes explicitly. Reload confirmation explains that default policy terminates owned background jobs. Viewing a finished job does not acknowledge its agent notification. Completed output remains inspectable with producer/supervisor absent.

## 6. Questionnaires: notes and clarification

```text
────────────────────────────────────────────────────────
 Questions › Scope                         1 of 3
 Which behavior should be implemented?
 → ☑ Project scope
   ☐ Global scope              recommended
   + Add custom answer…
   Question note…
   Note on selected option…
 Presented as multi · originally single
 [toggle] choice · [section] next · [back] review/dismiss
────────────────────────────────────────────────────────
```

Checkbox glyphs are illustrative; implementation uses width-tested theme/kit marks and textual fallbacks. The user's ability to change presentation type is explicit, not silently normalized away. Advisory required is marked “important”, never an asterisk implying blocking validation.

```text
────────────────────────────────────────────────────────
 Questions › Review
 ✓ Scope    project; custom: exclude scratch files
 ○ Privacy  unanswered (important)
 Notes
   Global scope: clarify cross-project confirmation
 → Submit current answers
   Ask for clarification on noted items
   Edit answers…
 [confirm] action · [back] questions
────────────────────────────────────────────────────────
```

Clarification review names affected options/questions and shows preserved answers. Submit doesn't manufacture defaults for unanswered questions. A note editor saves only the note; it doesn't submit. Multi custom answers can be individually edited/toggled. Empty freeform and cancelled freeform are distinct states.

### Pending/remote race

```text
Questions › Recovery
A pending form from this branch is available.
→ Resume draft
  Discard draft

● Answers submitted through bridge “desktop”
  Local form closed · receipt ask-7 revision 3
```

If a local user is still editing when a remote result wins, show outcome with retained submission facts; no second submit. A rejected stale remote response is a bridge diagnostic, not a new questionnaire answer. Sensitive draft persistence/extra archives are not hidden settings.

## 7. Answer extraction and checkpoints

```text
○ /answer · extracting questions
  Source: assistant entry a42
  [cancel] stop extraction

i /answer · no questions found
  No form opened; no answers submitted.
```

No spinner after cancellation/source switch. Extraction model is inspectable; no hardcoded brand/model claims in the design.

```text
────────────────────────────────────────────────────────
 Checkpoints › Regenerate
 Existing: Build investigation
 Covered entries: u12 → a42 (18 entries)
 Source interval: unchanged
 Extra instructions: emphasize unresolved compiler errors
 → Regenerate this interval
   Edit instructions…
   Cancel
 [confirm] choose · [back] checkpoints
────────────────────────────────────────────────────────

○ checkpoint · note extraction 2/4
○ checkpoint · synthesizing
● checkpoint · completed · 18 entries covered
  [expand] goal / findings / decisions / remaining questions
```

A completed checkpoint is not labeled host compaction. Old card stays until regeneration succeeds; failed/cancelled notes aren't published as a valid replacement. Unknown usage/duration fields are omitted. Clarify when a new interval will be summarized instead of silently changing it.

## 8. Configuration conflict and trust recovery

```text
────────────────────────────────────────────────────────
 Settings › Save conflict                 user · unsaved
 File changed since this panel opened.
 Key: read.maxLines
 Loaded: 2000    Your edit: 1000    On disk: 1500
 → Keep my value for this key
   Keep disk value
   Back without saving
 Other externally changed keys will be preserved.
 [confirm] resolve · [back] continue editing
────────────────────────────────────────────────────────
```

```text
▲ settings · saved, not fully applied
  jobs.timeout-seconds saved as 300
  Consumer reconciliation failed; the old value remains active.
  [inspect] effective values and remediation
```

Malformed file blocks save and names file/key safely; no “reset file to defaults” shortcut that deletes unknown data. User vs project vs runtime scope remains visible. Lower-trust project attempted grant shows “ignored: user-only security setting”, not an enabled switch.

```text
Trust › remote build
Source: user rule for ssh://build/srv/app
Effective: data-only
Requested: instruction authority
→ Review scope and grant…
  Keep data-only
```

Grant describes exact scope and source, not a generic Trust everything action. Workbench trust cannot change PiG's current session trust; if host trust requires restart, say so.

## 9. Responsive and mode coverage

### 36 columns

```text
────────────────────────────────────
 Save conflict › read.maxLines
 user · unsaved
 Loaded: 2000
 Yours: 1000
 Disk: 1500
 → Keep mine
   Keep disk
   Back
 [confirm] choose · [back] edit
────────────────────────────────────
```

Stack source/target/revisions instead of truncating the distinguishing parts. Edit partial-effect state and job ID survive when metrics drop. Preview questions open detail as a separate screen; no microscopic split pane.

### 57 columns

Use single-column lists with one selected-item description. Put host authority/location on a second line. Instruction selection preview may wrap path + reason but retains both; no global ellipsis replacing all reasons.

### 80 columns

Use the panels above. Body scrolls at short height (~16 rows), with title/primary state and essential hint line fixed. Staged state persists across tabs and resize.

### 140 columns

Proposed master/detail for questions, instruction selection, job logs and read/diff inspection: left list, right detail, shared panel shell. Configuration conflict can align loaded/yours/disk columns. No feature depends on split layout; actual kit feasibility and user capture approval remain required.

| Workflow | RPC | Print/JSON |
|---|---|---|
| Read/edit recovery | Structured results + content | Same tools, exact errors/continuation |
| Class/pin/settings | Native dialogs or explicit control request | Typed controls; required approval unavailable → error |
| Location/jobs | Text status and native confirmations | cd/jobs tools and URI controls |
| Questionnaire/notes/review | Sequential equivalent dialogs | Authorized bridge or immediate interactionUnavailable |
| /answer/checkpoint | Supported host messages/dialogs | Explicit supported invocation only; no dropped-UI wait |
| Config conflict | Select per-key resolution or cancel | conflict error with safe changed-key metadata |

## 10. Design verification checklist

All ten specifications now have associated static design coverage across SKETCH-0001/2/3. Implementation still needs:
- User review of selection/pin retention, edit recovery, questionnaire notes/clarification and partial-effect presentation.
- Fixture-backed kit prototypes and captures for every panel/state at 36/57/80/140 and short height.
- End-to-end live tests for instruction barriers, SSH failure/no local fallback, remote answer arbitration and configuration conflict.
- Renderer purity/no I/O, user keybindings, focus preservation and asynchronous session invalidation checks.
- Approval of backend, trust and inference decisions noted in the specifications.

No backend, host probe, prototype or live UI capture is represented as complete by these wireframes.
