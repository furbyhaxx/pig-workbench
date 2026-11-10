---
document:
  type: "sketch"
  id: "SKETCH-0002"
  title: "Extension integration, frames and shared renderer design"
  description: "Wireframes and interaction models for external contributions, message delivery, renderer states and resource providers"
  status: "draft"
  revision: 3
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-09T09:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 3
      reason: "Fix a dangling reference found by the artifact check"
      changes: ["Used a placeholder id in the instruction-diff example"]
---

# Design scope

Complements [general Workbench UX](../workbench-ui/README.md) and specifications [services](../../specs/0001-extension-services.md), [frames](../../specs/0002-message-frames.md), [presentation](../../specs/0003-tool-presentation.md), [resources](../../specs/0004-resource-providers.md).

**Static wireframes and proposed interaction contracts, not implemented views or live evidence.** Example identities, results and times are fixture data. Shared shell, keyboard bindings, searchable lists, staged saves and responsive behavior follow SKETCH-0001. Semantic key hints below resolve through the user's bindings. No new footer/editor ownership.

## 1. Information architecture

```text
/workbench
  tools                         existing /tools entrypoint
    tool → schema / presentation / owner
  settings
    tools → owner → tool        builtin and contributed settings together
    frames → source             display vs delivery policy kept distinct
  frames
    frame → content / delivery / provenance
  resources                     provider/scheme inspector
    scheme → capabilities / limits / owner
  integrations
    owner → frames / tools / schemes / health
```

`/workbench integrations` and `/workbench resources` are additional inspection routes. They expose services already registered by extensions; they do not install, enable or execute extensions. No global top-level `/integrations` or `/resources` aliases are needed. Every route supports argument completion and a textual show/list form; full panels are TUI-only.

The user should answer three questions without reading logs:
1. Who owns this contribution?
2. What is Workbench doing with it, and what does disabling that behavior change?
3. Is it active now, unavailable, or only retained for historical display?

## 2. Integration flow and failure behavior

```text
Extension                    Workbench                     User
register normal tool
subscribe + discover  ──────→ version/capability check
                       ←──── active service + generation
register contributions ────→ validate owner/schema/collision
                       ←──── accepted handles
                             contributes settings ───────→ inspect / customize
execute own tool
return semantic facts ─────→ render retained data ───────→ transcript
publish frame ─────────────→ persist → safe delivery ────→ frame + receipt state
serve resource request ←──── authorize + route
return bounded result ─────→ common pager/rendering ─────→ read result
```

Absent Workbench: normal tool still works with original rendering; optional integration says unavailable. Incompatible version: normal execution stays available; diagnostic explains required/supported major versions. Unavailable scheme: new reads fail explicitly; past results remain visible. Invalid registration: reject just that contribution, retain prior accepted descriptor.

```text
────────────────────────────────────────────────────────
 Workbench › Integrations
 Search extension or capability…
 → build-tools       active       frames · tools · build://
   review-tools      unavailable  saved display settings
   legacy-tools      incompatible requires API v2
 [open] inspect · [back] close
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 Integrations › build-tools                   active
 API: v1.0           Runtime: current session
 Contributions
 → Build notices     frame source · data authority
   build_run         shared process renderer
   build://          read · list · stat
 Latest issue: none recorded
 [open] contribution · [back] integrations
────────────────────────────────────────────────────────
```

Generation/registration IDs belong in diagnostic detail, not every main row. “None recorded” is not an uptime guarantee. A diagnostic retains safe error code, contribution, action and remediation; omit raw payloads by default. No credentials, full call arguments or hidden global session snippets in health views.

## 3. Message frame styles and state

### Row (compact)

```text
▲ build Build 412 failed · pending
◆ instructions BACKLOG.md · changed · +1 −1
● jobs unit tests finished · exit 0
```

### Rail (normal)

```text
▲ build Build 412 failed · delivered
│ Build exited with code 1.
│ Inspect build://412/log for compiler diagnostics.
└ build-tools.build · data · [expand] provenance
```

### Card (expanded)

```text
────────────────────────────────────────────────────────
 ▲ Build 412 failed                             delivered
 Source: build-tools.build       Authority: data
 Resource: build://412/log
 Build exited with code 1.
 Inspect build://412/log for compiler diagnostics.
 Seq 12 · 02.11.2026 16:00 · [back] collapse
────────────────────────────────────────────────────────
```

Exact glyph mapping: instruction ◆, reminder ↻, warning ▲, error ✗, notice ●, info i. Theme tokens add severity, never replace labels. Titles containing fake borders/escape sequences are sanitized. These samples use fixture times in Europe/Zurich.

### Delivery is not visibility

```text
────────────────────────────────────────────────────────
 Frames › Build 412 failed
 Display: collapsed             Model: pending
 Delivery: next safe boundary   Wake: not requested
 Source: build-tools.build      Trust: untrusted data
 Body: [scrollable]
 → Expand transcript display
   Hide transcript display
   Inspect delivery receipt
   Cancel pending delivery…
 [open] action · [back] frames
────────────────────────────────────────────────────────
```

Hide confirmation is not destructive: explanatory text says “This only hides the transcript row; model delivery is unchanged.” Cancel pending delivery is a distinct action, disabled after delivery, with a reason. Delivered means included in an outgoing context only when the host supports acknowledgement; otherwise show “queued to host”. Never use “model acknowledged”.

Required states: pending; queued to host; delivered; superseded pending notice; cancelled; failed; non-durable in-memory admission. Filter labels reflect real ledger facts. Producer disappearance cannot remove a historical warning; active instruction withdrawal is visible as a new event.

### Instruction updates and caps

```text
◆ BACKLOG.md · changed · view 7 → 8
│ - B-nnnn bug P2 — old pointer
│ + B-nnnn feature P2 — new pointer
│ 25 items shown · 8 more → BACKLOG.md
└ [expand] delivered view / source / provenance

i DESIGN.md · relocated · reuses frame 18
▲ AGENTS.md · removed · prior scope withdrawn
```

Source bytes and injected view are separate choices in detail. Hidden content changes show a concise source-version/count notice when appropriate, not a false visible diff. If the old baseline left context, show “baseline refreshed after compaction” with the new capped body. Controls do not offer a misleading toggle that disables safety/instructions merely by collapsing a card.

## 4. Tool renderer state gallery

Each example is a dedicated fixture for `/workbench preview` and later live captures. Header order: identity, outcome/effects, then optional metrics. Full retention and source continuation are independent.

```text
Preparing
 ○ build_run app · arguments arriving

Running
 [spinner] build_run app · compiling
   src/main.go

Success
 ✓ build_run app · 1.28 s
   Build completed.

Success with incomplete output
 ▲ grep "serve" · succeeded · output incomplete
   200 matches shown · more available
   Continue: out://search-1

Failure
 ✗ build_run app · compile failed · 1.28 s
   src/main.go:42: undefined: serve

Cancelled
 ▲ build_run app · cancelled
   Process terminated; partial output retained.

Timed out
 ✗ build_run app · timed out after 30 s

Denied before execution
 ▲ build_run app · denied · not started
   Project policy requires approval.

Partial side effects
 ▲ edit 3 files · failed · 2 committed
   ✓ a.go updated   ✓ b.go moved   ✗ c.go permission denied

Unknown outcome
 ▲ publish_report · outcome unknown
   Connection lost after dispatch. Inspect operation status before retry.
```

Preparing has no elapsed-time guess. Running only shows progress percentages when a real denominator exists. Final spinner removal is monotonic. Expanded errors include action/target/remedy; unknown exit codes remain unknown. A malformed presentation falls back to ordinary host output and an inspectable integration diagnostic, not a blank result.

### Semantic presets

| Preset | Normal preview | Expanded view |
|---|---|---|
| generic/text | summary and bounded text | retained blocks and original target |
| fileRead | requested range, numbered source | actual spans/revision/continuation |
| fileEdit | operation and bounded diff | per-file effects, recovery/provenance |
| search | matches with path/line | grouped matches and completeness |
| process | tail, phase, known duration | retained stdout/stderr, exit facts |
| questionnaire | pending/submitted/cancelled summary | answers according to privacy policy |
| composite | nested child rows | script, retained children and outputs |

A third-party tool selects a preset and semantic fields, not its own border renderer. Preset-specific display options are exposed through shared settings; tool execution remains under producer ownership.

### Codemode: handled failure remains visible

```text
✓ codemode · completed · 3 nested calls
  ├ ✓ read src/a.go:1-40 · 40 lines
  ├ ✗ build_run app · compile failed
  └ ✓ read build://412/log · 21 lines
  Script handled the build failure.
  [expand] script / nested calls / outputs
```

The final explanatory line appears only if present in actual retained output, not inferred by Workbench. Parent success does not erase a failed child. Missing child payloads say “details not retained”. A preview is rendering only; it never launches a build or replays a mutation.

## 5. Shared settings for contributed tools

```text
────────────────────────────────────────────────────────
 Settings › Tools › build-tools › build_run
 Scope: user                                  unsaved
 → Use Workbench rendering    on
   Density                    normal
   Preview lines              8
   Show duration              on
   Shorten paths              on

 Execution remains owned by build-tools.
 Disabling rendering restores the next renderer.
 [edit] value · [scope] target · Ctrl+S save
 [back] back
────────────────────────────────────────────────────────
```

Selected setting detail shows saved/staged/effective values and provenance. A runtime override can shadow the saved user value; display that before Save. Disabled/dormant contributions keep preferences under their stable owner/key and are labeled unavailable. Remove saved display preferences is explicit, not a side effect of unloading.

Custom display field example: process preset may expose an enum “output view: tail / head”. It must have type, bounds/choices, label/default and no executable callback. Execution flags, credentials and provider API keys do not enter this settings API. Unknown descriptor fields produce an actionable registration error.

## 6. Resource provider UX

```text
────────────────────────────────────────────────────────
 Workbench › Resources
 Search scheme or owner…
 → build://     build-tools      read · list · stat
   job://       pig-workbench    read · control
   skill://     pig-workbench    read · list
   review://    review-tools     unavailable
 [open] capabilities · [back] close
────────────────────────────────────────────────────────
```

```text
────────────────────────────────────────────────────────
 Resources › build://
 Owner: build-tools          Scope: project
 Backing: virtual           Editing: unsupported
 Selectors: line/range/tail/raw
 Reads: 50 KiB / 2000 lines per page
 Revision: stable           Cursor: snapshot-bound
 → Examples
   Capability details
   Integration status
 [open] inspect · [back] resources
────────────────────────────────────────────────────────
```

Opening a scheme row must not fetch the remote resource automatically. Read actions, if offered, are explicitly named and permission-checked. Mutation operations are never generic clickable links. Example URIs can be copied or pasted to an input; they do not execute on selection.

```text
✓ read build://412/log:20-40 · 2 lines · EOF
  20:compiler started
  21:compiled app
  Snapshot log-7 · source lines 20-21

▲ read build://412/log · stale cursor
  Log revision changed. Start a new read for a consistent page.

✗ edit build://412/log · unsupported
  This provider exposes read-only log content.
```

Derived views show “query rows”, “summary” or “converted text”, not source-line anchors. Alias display includes canonical identity in detail. URI errors distinguish unavailable provider, unknown scheme, unauthorized scope, unsupported operation, stale revision and provider failure. A retry button is only appropriate for safe idempotent operations; unknown mutation outcome offers Inspect status instead.

## 7. Responsive variants

### 36 columns: stacked identity and action

```text
────────────────────────────────────
 build_run › Display
 build-tools · user · unsaved
 → Renderer       Workbench
   Density        normal
   Preview lines  8
 Execution: owned by build-tools
 Ctrl+S save · [back] back
────────────────────────────────────

▲ edit 3 files · partial effects
 2 committed · 1 failed
 [expand] per-file outcome
```

Drop earlier breadcrumbs before contribution identity. State/effects cannot disappear behind metrics. Owner wraps to its own row when needed.

### 57 columns: compact list plus inspectable description

```text
─────────────────────────────────────────────────────────
 Workbench › Resources
 → build://    build-tools      read-only
   job://      pig-workbench    control
   review://   review-tools     unavailable
 Selected: bounded build status and logs
 [open] inspect · [back] close
─────────────────────────────────────────────────────────
```

### 80 columns

Use the normal single-column panel samples. Descriptions and provenance wrap in detail; list rows stay short. Truncation operates on terminal columns, preserves resource authority and basename, and offers the exact string in detail.

### 140 columns

Master/detail proposal follows SKETCH-0001: provider/tool/source list on left, selected capability/settings/body on right. Shared top/bottom panel borders only. Focus moves with semantic section navigation. Scrolling detail does not change selection. If supported kit focus/layout cannot deliver this reliably, present a capped-width stacked layout for review rather than simulating unsupported widgets.

At short height (~16 rows), preserve heading, selected identity and essential hints; scroll body. Search/focus/unsaved edits survive resizing. Expanded transcript cards still obey available width; no horizontally overflowing diff lines.

## 8. Mode equivalents and visibility

| Surface/action | TUI | RPC | Print/JSON |
|---|---|---|---|
| Integration/resource catalog | search/list/detail | text or basic selection | explicit structured listing via harness resource |
| Shared settings | staged kit form | typed dialog/explicit args with approval | explicit settings contract; refuse required unavailable confirmation |
| Frame receipt inspection | filter/detail | text result | frames API/status or harness resource |
| Tool presentation | width-aware views | structured Details + normal content | structured Details + normal content; no ANSI |
| Frame display toggle | settings action | configuration action | display config only; no model-context effect |
| Mutation retry | status-first action | explicit confirmation | typed safe/unknown outcome; no automatic repeat |

No UI-only feature silently discards a result in RPC/headless mode. User-facing diagnostics distinguish missing UI from missing provider/service.

## 9. Review and prototype checklist

Design decisions needing user review:
- Owner-grouped third-party tools in shared settings rather than a parallel plugin settings tree.
- Row/rail/card frame styles and independently visible delivery state.
- Compact headers retain failed/partial/unknown outcome even when other details collapse.
- Additional `/workbench integrations` and `/workbench resources` inspection routes.
- Semantic preset contribution API instead of arbitrary foreign drawing callbacks.

Prototype after design approval: common contribution inspector; staged contributed-tool settings; all frame styles/states; tool state gallery and codemode failure tree; resource capabilities/unsupported mutation/stale cursor. Capture each at 36/57/80/140 columns and short height with terminal escape, long ID/path, empty/error/loading/dirty and provider-unload fixtures.

Acceptance evidence must include actual host interaction, no render-time bus I/O, focus/resize preservation, async session invalidation and non-TUI output. **No prototype/capture has been produced in this design-only change.**
