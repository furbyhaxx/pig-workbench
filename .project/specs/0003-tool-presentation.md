---
document:
  type: "specification"
  id: "SPEC-0003"
  title: "Tool presentation and renderer contribution API"
  description: "Semantic render models, lifecycle states, third-party settings and replay"
  status: "draft"
  revision: 4
  created_at: "2026-11-02T16:00:47+01:00"
  updated_at: "2026-11-02T20:55:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Define the public Workbench extension APIs before implementation"
      changes: ["Initial contract, lifecycle, examples and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes measured renderer resolution and codemode rows"
      changes: ["Recorded resolver caching, reload re-render and codemode child ownership", "Required registration before first render and history rendering from stored Details"]
    - revision: 3
      reason: "M0.1 donor parity audit"
      changes: ["Added donor density budgets, header fact sources and preview rules"]
    - revision: 4
      reason: "pig-web harmonisation and user decisions"
      changes: ["Resolver reads the live registry at first use (pig-web R2)", "Typed table cells including uri (R3)"]
---

# 1. Goal and verified host mechanisms

Let other extensions opt their tools into Workbench renderers and display settings without surrendering tool execution ownership. Workbench consumes a **semantic presentation model**, not remotely executed drawing callbacks. This draft depends on [SPEC-0001](0001-extension-services.md) and [integration UX](../sketches/extension-apis/README.md).

Open probe: whether the host asks the resolver about historical tool names before extensions run `session_start` on resume; if it does, those names report `appliesAfterReload` as above.

Verified SDK: `Extension.ToolRenderer` resolves tool renderers, including foreign tools, and `next()` preserves the next renderer. Its returned marker cannot be wrapped to call foreign rendering (SDK divergence D89). `SetToolRenderers` updates locally registered tools; it is not sufficient evidence for arbitrary foreign-tool takeover. Use the resolver to opt in registered contributions and return next() for others. CallView/ResultView are supported; a renderer error triggers host fallback. Observed ([probes](../analyses/2026-11-02-host-integration-probes/README.md) P8–P11): the resolver is called once per tool name and cached for the process; a later claim change has no effect until `/reload`, after which history re-renders with the new renderer. Render width is terminal width − 2. In one run a historical call row re-rendered after reload received empty args.

# 2. Ownership and registration

The producer registers its normal tool with PiG, then registers `toolPresentation`:
| Field | Contract |
|---|---|
| key | Stable contribution ID, normally tool name |
| toolNames | Explicit nonempty list; no wildcard takeover |
| label, description, category | Settings/inspection labels |
| presentationVersion | 1 |
| preset | generic, text, fileRead, fileEdit, search, process, questionnaire or composite |
| defaultDisplay | density: compact/normal/full; previewLines (0–100) |
| callFields | Bounded declarative extraction rules from arguments |
| detailsPath | JSON Pointer to the presentation object in tool Details; default /workbenchPresentation |
| metricDefinitions | Stable IDs, labels, units and priority |
| settings | Allowlisted display options only |

Caller must own the tool or the user explicitly grants an integration adapter permission for that tool. Cross-check host source metadata when available. If ownership cannot be established, report verificationRequired; do not silently claim arbitrary tools. A tool cannot have two active Workbench presentation owners. Replacing an existing external renderer is opt-in, visible in /tools and reversible.

Call extraction uses JSON Pointer and a closed formatting set (plain, path, URI, count, duration); no evaluation, templates with execution, regex scripts or host calls. Optional fields can be absent. Any required secret-bearing argument is omitted/redacted by the producer; Workbench additionally sanitizes terminal/control sequences.

```json
{"v":1,"id":"register-view","sessionId":"s-1","generation":"g-1","from":{"owner":"build-tools","instance":"i-1"},"method":"register","params":{"kind":"toolPresentation","key":"build_run","descriptor":{"toolNames":["build_run"],"label":"Build","description":"Build command output","category":"Build tools","presentationVersion":1,"preset":"process","defaultDisplay":{"density":"normal","previewLines":8},"detailsPath":"/workbenchPresentation","callFields":[{"pointer":"/target","label":"target","format":"plain"}],"metricDefinitions":[{"id":"durationMs","label":"duration","unit":"ms","priority":20}],"settings":[]}}}
```

Reply is the common registration receipt with registrationId and descriptorRevision. Registration metadata does not re-register or activate the tool, alter its schema, grant execution permission or change tool results sent to the model.

# 3. Presentation data contract

Producer places a complete presentation snapshot in each relevant tool update/final Details. Existing producer fields remain untouched.

```json
{"workbenchPresentation":{"version":1,"revision":3,"phase":"settled","outcome":"failed","title":"Build app","target":{"kind":"path","value":"packages/app"},"summary":"Compiler rejected two files","complete":true,"effects":"none","metrics":[{"id":"durationMs","value":1280,"unit":"ms"}],"blocks":[{"kind":"text","text":"src/main.go:42: undefined: serve","role":"error"}],"error":{"code":"compileFailed","message":"Fix the compiler errors and retry."}}}
```

| Field | Semantics |
|---|---|
| version, revision | Schema version and monotonically increasing snapshot sequence per call |
| phase | preparing, running or settled |
| outcome | Required when settled: succeeded, failed, cancelled, timedOut, denied or unknown |
| title, target, summary | Plain text and structured target (path/URI/label), original value retained |
| complete | Whether requested output coverage is complete, not whether operation succeeded |
| effects | none, committed, partial or unknown; optional for read-only calls |
| metrics | Typed known facts; units: count, bytes, lines, ms, percent or explicit currency code |
| blocks | Ordered bounded text, code, diff, table, imageRef, notice or children blocks |
| continuation | Optional URI/cursor and explanation of omitted content |
| error | Stable code + actionable message; no stack/credential dump |
| provenance | Optional resource revision, source coordinates, adapter/view kind |
| parentCallId | Only if the producer/host actually supplies nesting identity |

Block contracts: text/code contain bounded text and optional language; diff contains file operation plus before/after identities and bounded hunks; table contains columns and typed rows (cell types: text, count, bytes, duration, path, uri, label; a `uri` cell needs no path or line, so web results use the `search` preset unchanged); imageRef references an existing content block/resource, not arbitrary base64 duplication; children contains child call IDs and retained child presentation snapshots. Notices declare info/warning/error. No raw ANSI/style code or arbitrary kit node graphs cross the API.

Maximum presentation payload 64 KiB, at most 32 blocks, 16 metrics, 100 table rows and nesting depth 4. Overflow is explicit with continuation. Defaults are adjustable downward through negotiated limits. Tool model-output budgets remain independent; duplicating an entire large result into Details is discouraged.

# 4. Lifecycle and precedence

```text
preparing → running → settled(succeeded|failed|cancelled|timedOut|denied|unknown)
     └───────────────────────→ settled(denied|cancelled|failed)
```

Streaming is a running update, not a terminal result. Partial output and partial side effects are **orthogonal** to outcome. A successful search can be incomplete; a cancelled edit can have partial committed effects. An unknown final outcome is never drawn as success.

Authority order:
1. Host execution/error/partial flags constrain the presentation. A producer cannot turn host IsError into green success.
2. Producer final semantics refine cancellation/denial/effects where the host provides insufficient detail.
3. Metrics from host (e.g. final duration) win over conflicting computed approximations; contradictory facts produce a visible diagnostic.
4. Missing data stays unknown. Do not infer success from an empty result or cancellation from a string containing “cancel”.

Ignore older presentation revisions. Once settled, late running updates cannot resurrect a spinner; final duplicate with identical revision/content is idempotent. A conflicting final snapshot becomes a diagnostic, not retroactive success. Preparing argument previews are labeled partial until ArgsComplete.

# 5. Renderer selection, execution and fallback

Resolver reads the in-memory registration and validated snapshot only. No bus/filesystem/network access, parsing of large outputs or state mutations during render. Parsing/validation and diff processing occur on data change, cached by card revision/theme/width/density.

- Opted-in tool with supported valid presentation: Workbench renderer.
- Opted-in tool without custom Details yet: bounded generic renderer over host arguments/content.
- Invalid/unsupported presentation: visible integration diagnostic outside the hot render loop; use host/producer fallback, preserving raw result/error.
- Unregistered tool: next().
- User disables Workbench rendering for the tool: next(), without disabling execution.
- Workbench absent: producer's own renderer/host default.
- Another later resolver overrides Workbench: inspector reports actual resolver ownership if host exposes it; never claim enforced ownership based only on registration.

Because resolution is cached per tool name, Workbench answers the resolver from its **live** registry at the moment the host first asks about that tool name, so registration order between extensions' `session_start` handlers does not matter. Workbench records which tool names it has already answered. A registration for a name already answered reports `appliesAfterReload` in its register result and in `/workbench integrations`, and Workbench never triggers reload on its own. Call views must not depend on args for historical rows; they render from the stored presentation Details and show "arguments not retained" when both are missing.

Historical rendering uses retained presentation version and safe snapshot labels, not current producer callbacks. If producer is absent but a compatible stored presentation exists, a guarded historical resolver may render it without claiming current tool execution. If the host resolver cannot select by stored data, default fallback remains available and the limitation is reported. Unknown schema versions render generic content with no guessed metrics. No destructive rewriting of session records.

# 6. Display settings and third-party controls

Every registered tool appears in `/workbench settings tools`, grouped by owner. Configuration identity is owner + stable key, not ephemeral registrationId.

Supported built-in controls: enabled (presentation only), density, previewLines, metric visibility, target shortening, nested child density, and allowed preset-specific display toggles. External descriptors may add boolean, bounded integer or enum display fields with stable key, label, description and default. They cannot add arbitrary command buttons, execute scripts or modify producer execution settings.

Effective precedence: builtin preset < producer defaults < user < trusted project < runtime. Reset deletes the chosen scope's override. Producer cannot write user overrides via registration. Dynamic descriptors preserve unknown/dormant keys; removed fields are labeled unavailable rather than erased. Changing descriptor type requires a new key or explicit migration—never reinterpret a saved boolean as a numeric mode.

Settings changes produce a versioned settings.changed event with registration identity and **display** values only. Renderer caches invalidate once per revision. Producer may query settings.get, but render-time behavior cannot depend on a network round trip.

Custom execution configuration belongs to the producer's own settings API; future settings delegation needs an explicit separate contract. This v1 API fulfills “expose in our settings” for shared presentation without making Workbench a generic settings-write proxy.

# 7. Density, codemode and accessibility

Compact: header with status/target/primary metric; incomplete/error/effects markers remain visible.
Normal: header + bounded preview + explicit expand/continuation.
Full: all retained content allowed by viewport; omitted upstream output still requires resource read.

Header priority: tool/subject identity, terminal status, partial-effect warning, then metrics. Paths preserve authority and basename. Text is stripped of terminal sequences before styling. Color always pairs with a word/glyph. Theme tokens and user keybinding hints only.

Codemode draws its child rows itself; extension resolvers are not consulted for them (P9). Unified child presentation therefore needs either an upstream hook for child rows or a Workbench-owned `codemode` renderer that reconstructs children from `tool_execution_start/end` events (they carry `parentToolCallId`; IDs look like `call_0_0/1` but are treated as opaque). Until one exists, child rows keep codemode's native rendering and the inspector shows Workbench's captured child data.

Composite/codemode renderer uses the same child snapshots/presets. It must show failed/cancelled/running children even when successes collapse. Parent outcome derives from actual codemode result, not a guessed aggregate (a script may handle a child failure). Child failures remain visible even when parent succeeded. Missing historical children say “not retained”. Scripts/arguments are inspectable, never re-executed by preview.

# 8. Acceptance

- A foreign tool registers, uses normal Workbench renderer, appears in settings and keeps working after Workbench unload.
- No override for non-opted-in tools; explicit renderer conflict is visible.
- Host failure + claimed producer success stays error; stale update cannot regress final state.
- Success+incomplete and failure+partial-commit have different truthful headers.
- Every preset/state at 36/57/80/140 columns and short height; hostile ANSI and very long URI fixtures.
- Unknown metrics, image-only results, malformed Details, unsupported schema versions and omitted child captures preserve usable output.
- User/project/runtime precedence, reset, dormant provider, descriptor replacement and settings-change invalidation.
- Direct tool vs codemode child have identical formatting semantics; replay without producer performs no I/O.
- Verify registration/unregister changes are reflected in live host resolver caches. D89 prohibits designs that call foreign next() renderers as ordinary Go callbacks.

# 9. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), pi-workbench renderer tests. These are the defaults; settings can change them.
- G3.1 Normal density: bash shows the last 5 visual lines; codemode shows 10 script lines plus the configured output budget and hides the oldest successes behind a count; frame bodies show 40 lines with an exact overflow count. Compact groups runs of the same tool. Running, cancelled and failed rows are never hidden. Wrapped Unicode, whitespace-only and blank rows count as visual rows.
- G3.2 Header numbers come from result metrics first, then arguments or the returned diff, and are never guessed (a read shows the range actually returned). Bash failures lead with the exit status. An error's first line goes in the header and the rest in the body. Empty or limited results warn; a silent zero-output success does not. Malformed numeric arguments are omitted on replay. Argument targets are stamped with the root they were resolved against, so a later `cd` does not rewrite old rows. Job rows keep the job ID at 40 columns.
- G3.3 Running rows animate on one shared clock and release it when they settle. Queued and preparing rows are dimmed. Settled layouts are cached and invalidated by width, settings, theme or result changes.
- G3.4 Persisted render metadata keeps location, root, job and metrics, and does not duplicate output. Binary and control bytes never reach the terminal.
- G3.5 `/workbench preview` pairs exact call IDs (the latest invocation, even without a result), uses built-in examples only for tools with no recorded call, persists references only (never model context) and is width-safe.
- G3.6 Frame rows: hidden means zero lines; a click toggles one row and the expand binding resets it; the untrusted label appears before secondary metrics; diff fences render with add/remove tokens; invalid details fall back to sanitized plain text.
- G5.14 Edit rows show the target path from partial arguments while they stream, window a streaming diff to the viewport tail, render a move-only result as `src → dst`, put change stats in the header, and bound oversized hunks.
- G6.15 Consecutive reads group into one row with same-file ranges merged. A late result repaints a pending preview; a call that never resolves is sealed instead of left spinning.
- G8.13 Paths: home becomes `~`; a local path uses the shorter of root-relative and absolute; a remote path keeps its authority and is shortened only against the same authority; fish-style shortening keeps the basename and never exceeds the width; shell command display shortens only standalone literal paths.
