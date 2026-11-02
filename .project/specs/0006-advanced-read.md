---
document:
  type: "specification"
  id: "SPEC-0006"
  title: "Advanced reads, format adapters and search contracts"
  description: "Selector semantics, bounded pipelines, structural views, rich formats and exact source coverage"
  status: "draft"
  revision: 3
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-02T22:40:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Complete the remaining Workbench specification and design areas"
      changes: ["Defined behavior, state, safety, examples, dependencies and acceptance criteria"]
    - revision: 2
      reason: "Backend matrix approved by the user; M0.2 probes verified builtin replacement"
      changes: ["Bound adapters to the approved backends and listed remaining coverage checks", "Recorded builtin replacement and delegation behaviour"]
    - revision: 3
      reason: "Backend coverage checks completed"
      changes: ["Updated syntax, archive and HTML rows from the coverage checks"]
---

# 1. Scope and evidence

Draft specification for enhanced read/grep/find/ls. Depends on [resource providers](0004-resource-providers.md), [source snapshots](0005-hashline-editing.md), [ambient context](0007-ambient-context.md) and [configuration](0010-configuration-state-trust.md). Full advanced reader scope is retained; “use bash instead” is not adapter implementation.

Inspected donor contract: `host:/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi/packages/coding-agent/src/prompts/tools/read.md`, `src/tools/read-selector.ts`, read.ts imports/dispatch surface and read-summary/json/sqlite/archive modules; nugent scheme ToolPath/Selector/Pager. The donor prompt and selector distinguish raw bare numbers and comma lists; Workbench adopts those precise semantics below rather than nugent's simpler onward-number interpretation.

Backends are approved as proposed in the [backend evaluation](../analyses/2026-11-02-backend-evaluation/README.md) (user decision 2026-11-02, including HEIC via `gen2brain/heic` with embedded libheif/libde265 WASM and system libheif when present). Required capabilities and observable errors below remain the contract; the libraries are how they are met:

| Adapter | Approved backend | Measured caveat Workbench must handle |
|---|---|---|
| Syntax summaries/blocks | `odvcencio/gotreesitter` + full `grammars` registry (206 languages, ≤ 18 MiB) | C++ may yield ERROR nodes → block selector reports ambiguity; diffs use a unified-diff parser, not tree-sitter |
| SQLite | `ncruces/go-sqlite3` with read-only authorizer | Reject any non-empty statement tail |
| JSON query | `itchyny/gojq` | No `env`/`input`; preserve number lexemes |
| Archives | `mholt/archives`; ISO `go-filesystems/iso9660`; cpio `go-filesystems/cpio`; CAB `NextronSystems/go-cab`; RPM `sassoftware/go-rpmutils`; ASAR `depthbomb/go-asar`; ar/deb own reader; LZH, ARJ, `.Z` and unsupported 7z/RAR/CAB methods via external 7-Zip (additions pending approval) | Normalise member names, never follow member links, cap expansion; multi-volume RAR opens by name with an FS; unsupported method → 7-Zip if present, else `backendUnavailable` |
| ELF/PE/Mach-O | stdlib `debug/*` + `golang.org/x/arch` | Pseudocode only through optional external Ghidra; otherwise report `backendUnavailable` |
| PDF | `klippa-app/go-pdfium` (WASM) | Lazy init (~1.8 s, ~290 MB); corrupt files give typed errors |
| SVG raster | resvg (WASI) under `wazero` | No resource dir, read-only font mount, 256 MiB memory cap |
| WebP / HEIC | `golang.org/x/image/webp` / `gen2brain/heic` | HEIC decode 130–250 ms; LGPL WASM notice shipped |
| Video | external `ffprobe`/`ffmpeg` | Seek past end exits 0 without output — check the file |
| HTML | `html-to-markdown/v2`; semantic main element first, `go-readability/v2` only as fallback | Resolve links against the page URL (not `WithDomain`), strip chrome and permalinks, layout tables as text, report script-only redirects |
| Profiles | `google/pprof/profile` | — |

Builtin replacement (observed, [probes](../analyses/2026-11-02-host-integration-probes/README.md) P15–P17): registering a tool with a builtin's name replaces it for direct calls and codemode children, and `GetAllTools` reports Workbench as its source. A wrapper around `sdk.Create*ToolDefinition(...).Execute` delegates to the host implementation without recursion, keeps `tool_execution_update` streaming, and propagates caller abort (the delegated shell child is killed). The factory `cwd` is ignored; see SPEC-0008 for location handling.

# 2. Read schema and parsing

Read request: exactly one of `path:string` or `paths:string[]` (1–32 entries); optional explicit `selector:string` for a single target with ambiguous URI punctuation. Do not advertise offset/limit as a competing source-line grammar. Explicit selector plus a suffix is invalid even if they appear equivalent. Query offset/limit belongs to a format adapter's rows/results, not text source lines.

Default model description favors path suffixes on every ranged request. A repeated bare read does not implicitly advance a cursor.

| Spelling | Selection before bounded context |
|---|---|
| file:50 / file:50- | Starts at line 50, default page length 2000 subject to hard caps |
| file:50-80 | Inclusive 50–80 |
| file:50+31 | Same 31 lines |
| file:-60 | Last 60 lines, clamped to source size |
| file:19,59 | Exactly lines 19 and 59 before structural context |
| file:1-12,-15 | Merge/sort absolute ranges and last 15 lines |
| file:raw | Verbatim source view, bounded by hard caps |
| file:17:raw / file:raw:17 | Exactly line 17, no context |
| file:17-:raw | From 17 onward, no context |
| file:2-4:raw | Exactly 2–4, no anchors/context |

Bare N inside any comma list is a singleton; a lone non-raw N is onward. Adjacent/overlapping ranges merge without duplicate lines. Zero, negative counts other than tail syntax, overflow and reversed ranges fail. `.. ` is not an advertised alternate syntax. No silent widening of malformed `:conflicts:1-2`.

Literal path lookup wins before selector parsing. Resolution order: explicit resource provider → exact local/remote path → supported container/member path → optional unique suffix-resolution fallback for not-found local paths. If suffix fallback finds more than one target, return candidates and no content. Never perform speculative whole-remote-tree suffix search. Display requested/resolved identity and record snapshots only under canonical resolved identity.

For delimited path convenience, apply SPEC-0004 ambiguity rules; explicit paths[] is always preferred. Multi-read uses bounded concurrency (default 4), deterministic input order and one total output budget. An error for one target is retained beside successful results; none of the successful results are thrown away. Invalid request grammar fails before work.

# 3. Read pipeline

```text
capture location/config → parse → resolve + authorize → discover applicable context
 → sniff bounded bytes → choose adapter/view → acquire consistent source/snapshot
 → select/query → syntax context/summary → enforce shared budget
 → record fully delivered source coverage → render facts + continuation
```

Do not read devices/FIFOs/sockets as regular text. Use descriptor-based type/identity checks to reduce TOCTOU; unsupported special files produce explicit errors. Binary detection is bounded and separate from format extension guessing. Invalid UTF-8 is not silently repaired into editable source.

Text output default: 50 KiB/2000 lines total, including headers/context/footer. Reserve footer budget first. Single non-raw range adds one leading and three trailing lines; tails and comma ranges do not get ordinary context padding. Non-raw slices may add enclosing syntax opener/closer lines with explicit gaps. Only raw is exact. Hard limits can cut context too; report actual spans.

One line larger than byte cap: emit a UTF-8-safe prefix marked partial, repeat that line in continuation and identify that a larger budget/another view is required for progress. No endless “next line” advance that loses the remainder. Empty source has zero lines; a start beyond EOF errors, an end beyond EOF clamps. Preserve original text/newlines in raw view.

Result facts: requestedUri, canonicalUri, view kind, revision, coordinateSpace, shownRanges, fullyShownRanges, totalLines if known, selectionComplete, resourceComplete, truncation reason, continuation, adapter warnings, hashline tag only when usable. Source completeness is independent of model/UI preview clipping.

# 4. Structural summaries and context

Bare sufficiently large code files use a declaration/signature skeleton. Numeric/raw reads never silently summarize the requested body. Initial proposed thresholds: 200 source lines, minimum folded body 12 lines, comment block 8 lines; configurable and benchmarked before release. Prose/Markdown summarization off by default. Keep declaration headers, annotations and signatures, preserve actual source coordinates, and list omitted intervals with actionable reads.

The shared syntax service supplies summaries and exact multi-line construct boundaries. Cache key includes canonical resource, exact revision/content digest, grammar version and fold options. Bound cache bytes/count; stat-only reuse is insufficient when content could have changed.

Parser unavailable/unparseable: return ordinary bounded text plus diagnostic if the user explicitly requested structure; a bare automatic summary attempt may fall back to text with view facts explaining it. Lexical bracket context is display-only fallback and cannot authorize block edits. Summary ranges/fold marker endpoints do not mark the interior seen.

Expanded TUI renders only retained results. It does not fetch omitted bodies or make hidden source known automatically. Read continuation passes through normal scope/authorization and instruction discovery.

# 5. Adapter contract and required matrix

Every adapter returns typed content plus view/source provenance through the shared budget. Invocation must be cancellable and bounded in input bytes, output, time and decompression/pixel expansion. No automatic installation, grammar download, paid inference or network upload. Backend absent → named backendUnavailable error with supported alternatives; feature remains unverified until delivered.

| Adapter | Required views and selectors | Integrity/boundary conditions |
|---|---|---|
| Directories | Root entries and bounded child previews, :N-M / :-N over stable listing | Deterministic ordering, root completeness explicit, default 12 children per expanded directory, +K more path when known |
| SQLite | Bare tables/counts; :table schema/sample; :table:key PK/rowid; ?limit/offset/where/q read queries | Read-only connection, single read statement, reject mutation/attach/extension/filesystem functions; parameterize keys, bound rows/time |
| JSON/JSONL/NDJSON | ?q= jq-compatible filter; raw/compact; result offset/limit | Preserve number lexemes, percent-decode once, + remains plus, %26 inside filter, no shell interpolation, query failure distinct from empty result |
| Archives | ZIP/JAR/APK/WHL, compressed TAR, RAR/7z/ISO/CAB/DEB/RPM/CPIO/AR/LZH/ARJ/ASAR and compressed streams; member then source selector | Containment, no extraction-to-workspace by default, link traversal rejected, expanded byte/member/depth caps, solid archives bounded |
| ELF/PE/Mach-O | Overview/function list; symbol/address pseudocode; :asm; :imports/:exports/:strings; :xrefs:symbol; universal Mach-O :@arch | Inspect without execution; architecture explicit; disassembly is not passed off as pseudocode; optional licensed backend requires approval |
| PDF/documents | Extracted text/Markdown and explicit page-image view | Page/source mapping and converter name; page-image request `file.pdf:p2.png` (page 2; donor also recognizes `page-2.png`); reject zero/out-of-range pages rather than defaulting silently; scanned page without OCR is not empty-document success |
| Notebook | Editable cell source view, source/output/metadata distinctions | Stable cell IDs and reversible map, preserve unmodified metadata/output, reject ambiguous mapping; original JSON via raw |
| Raster/HEIF-class images | Inline decoded content, dimensions, resize notes | Decoder/pixel/frame limits; preserve source identity; unsupported model modality gives actionable result, not dropped image |
| SVG/SVGZ | Default text; :img rasterization; :raw original | No active scripts/external fetch; raster image :img is invalid |
| Video | Metadata/contact sheet; :412 frame; :90s, :1h5m42s, :01:23 time | ffmpeg/ffprobe or approved equivalent; exact extracted timestamps, seek errors, frame/pixel/time caps, never execute metadata |
| CPU/sample profiles | Bounded function/hot-path/table views with units and provenance | Profile percentages use known totals only; invalid profile distinct from no samples |
| HTTP(S) | Static readable text/Markdown; :raw HTML; same text paging | Redirect reauthorization, network limits, content type/charset handling, no script/browser execution; bare host:port requires slash |
| Conflicts | :conflicts summary + conflict:// identifiers, ours/theirs/base views | Register source-revision-bound spans; resolution uses validated write path, no guessed offsets |

JSON result limit defaults to 100/max 1000 rows before shared byte caps; SQLite and directory/query catalogs also respect result limits. Archive index cache defaults to 4 sources/200000 entries with byte cap; only identity/revision-safe indexes are reusable. Stream/solid containers must not pin unrestricted expanded payloads. Document/media backend-specific ceilings must be in the settings catalog with units, not magic unreported truncation.

A question query on an image is optional configured inference: explicit question, selected provider capability, permission and cost policy. Plain image read never triggers inference. Pseudocode and OCR dependencies are gates, not reasons to relabel asm/text as equivalent features.

# 6. Search and listing tool contracts

`grep`: path (default captured location), exactly one regex pattern or literal patterns[] OR-list, case mode, glob/exclude[], context count, output lines|files, bounded limit. Reject incompatible literal/regex options. Use ripgrep when available with argument arrays and safe flags; fallback must preserve advertised semantics or report unsupported regex features. Remote search uses same contract without interpolating patterns into shell code.

Matches include canonical identity, revision, exact line/column units and complete-line bytes only when acquired consistently. Hashline tags/seen spans require the matching source snapshot; a changed file between search and snapshot yields stale result warning/re-read, not false anchors. Output files mode does not mark file contents seen.

`find`: path/provider root, glob pattern with documented brace/class/doublestar behavior, exclusions and result cap. No unbounded virtual-provider traversal fallback. `ls`: one container/path, typed entries, deterministic directories-first then lexical names, hidden entries controlled explicitly and surfaced in Details. Ignore semantics follow repository .gitignore for grep/find where appropriate; explicit paths remain addressable. Add regression fixtures for symlinks and negated ignore rules before claiming parity.

All return totalKnown/shown/complete or unknown totals, not a fabricated number of hidden matches. Read filesystem errors remain errors, never treated as a no-match success.

# 7. Loop guard, supersession and caching

Track repeated normalized (URI, view/selector/query, exact revision) results per session. At third identical result, append a concise repetition hint and continuation/alternative where meaningful; do not block deliberate re-reading or replace content with a stub. Different revisions and selections reset the repeat key.

Read supersession is context projection only. Later complete same-revision source coverage can supersede earlier partial coverage for the same canonical resource, while retaining tool/error provenance. A structural summary, transformed view, cropped/column-truncated page or directory listing cannot supersede full source. Never alter historical JSONL; compaction/replay rebuild projection state.

Cache invalidation uses source revision/digest and provider events plus revalidation; render-time functions perform no I/O. Range/tail reads use streaming/range access when possible; structural summaries may require bounded whole-source parsing. Large files remain pageable even beyond syntax/snapshot budgets.

# 8. Acceptance gates

- Golden selector cases including :17, :17:raw, :19,59, mixed tails, exact raw bytes, context clipping and continuation.
- Literal colon/comma/space paths, SSH port, URL query, archive member and binary view chains resolve unambiguously.
- Summaries retain true headers/coordinates; omitted or partially emitted lines never become seen.
- Each matrix row has at least one real representative fixture plus malformed/oversize/cancel test; broad archive/architecture support needs enumerated fixtures, not one ZIP/ELF.
- JSON precision and literal +/%26, read-only SQLite escapes, archive bombs/traversal and SVG external loads.
- Notebook source edit roundtrip preserves untouched JSON data; derived text edits rejected without mapping.
- Video exact timestamp/index, PDF text vs page image, profiles with known/unknown totals, unavailable decompiler/OCR clearly reported.
- Direct/codemode/provider outputs share bounds; multi-read deterministic under errors/cancellation.
- Static URL access obeys redirect/network policy; no automatic paid question answering.
- Benchmarks record large-file tail/range memory, parse cache hit/miss, directory listing, remote search and archive indexing.

Final support/backend matrix and adapter-specific query schemas must be approved in M0. No unresolved backend may be advertised as working at release.
