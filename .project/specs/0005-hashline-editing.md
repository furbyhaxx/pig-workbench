---
document:
  type: "specification"
  id: "SPEC-0005"
  title: "Full hashline editing and shared source snapshots"
  description: "Grammar, syntax blocks, registers, provenance, recovery and mutation integrity"
  status: "draft"
  revision: 8
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-03T14:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Complete the remaining Workbench specification and design areas"
      changes: ["Defined behavior, state, safety, examples, dependencies and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes verified builtin replacement"
      changes: ["Recorded how Workbench edit/write replace the host builtins"]
    - revision: 3
      reason: "M0.1 donor parity audit"
      changes: ["Added donor parity rules for payload repair, recovery, leniency, streaming, path policy, notebooks and per-model mode"]
    - revision: 4
      reason: "pig-web harmonisation and user decisions"
      changes: ["Added the snapshots edit-tracking service for other extensions"]
    - revision: 5
      reason: "SPEC-0012 project model and URI namespace"
      changes: ["artifact:// files carry hashline anchors through their backing file"]
    - revision: 6
      reason: "Secrets redesigned after nukii-revived (SPEC-0013)"
      changes: ["Placeholders in snapshots and replacement text"]
    - revision: 7
      reason: "All config files are TOML (SPEC-0010 §1)"
      changes: ["Setting keys renamed to kebab-case"]
    - revision: 8
      reason: "User decisions D5.b and D5.d (2026-11-03)"
      changes: ["Tag collisions recover from the newest snapshot of the same resource", "No separate apply_patch edit mode"]
---

# 1. Authority and scope

Draft normative behavior for the full hashline engine, exact-text edit mode and coordinated write path. Not an implementation report. Depends on [resources](0004-resource-providers.md), [presentation](0003-tool-presentation.md), [reads](0006-advanced-read.md), [instructions](0007-ambient-context.md) and [state/trust](0010-configuration-state-trust.md).

Verified donor surfaces: `host:/mnt/Projects/nukular/nukular-ai/.references/oh-my-pi/crates/pi-edit/prompts/hashline.md`, `src/modes/hashline/{types,parser,block,clipboard,recovery,mismatch,patcher,preview}.rs`, `src/store.rs` and `crates/pi-ast/src/block.rs`. The prompt/types were inspected for this specification; full parser/recovery fixtures remain the implementation parity oracle, not claimed as completely audited here. No new parser/hash/diff dependency is authorized by this document.

Builtin replacement (observed, [probes](../analyses/2026-11-02-host-integration-probes/README.md) P15–P17): registering a tool with a builtin's name replaces it for direct calls and codemode children, and `GetAllTools` reports Workbench as its source. A wrapper around `sdk.Create*ToolDefinition(...).Execute` delegates to the host implementation without recursion, keeps `tool_execution_update` streaming, and propagates caller abort (the delegated shell child is killed). The factory `cwd` is ignored; see SPEC-0008 for location handling.

“Full” includes block editing, registers, cross-file patches, file operations, seen-line checks, stale-snapshot recovery, diagnostics, previews and loop protection. A range-only editor cannot satisfy this specification.

# 2. Public tools and modes

`edit` accepts exactly one of:
- Hashline: `{"input":"[path#TAG]\nPUT ..."}`.
- Exact text: `{"path":"...","edits":[{"oldText":"...","newText":"..."}]}`.

Both branches reject unknown fields and mixed forms. `edit.mode = hashline|replace|auto`: default hashline; auto advertises both forms and chooses by supplied shape, **never retries a failed hashline patch as a weaker text replacement**. Mode changes apply at a turn boundary so descriptions, schemas and parser agree.

Exact-text mode matches every oldText uniquely against the original file; ranges cannot overlap. Empty oldText is invalid; creation uses write. All edits stage together. Zero or multiple matches produce actionable errors, not fuzzy fallback. It uses the same authorization, instruction checks, locking, output facts and preservation rules as hashline.

`write` is for creation or intentional complete replacement: path, content, optional expectedRevision and raw boolean (default false). Existing-file overwrite needs the ordinary write permission; a supplied revision must match. A handler URI's write semantics remain SPEC-0004, not assumed file replacement. Prefix stripping is limited to positively recognized copied hashline output; ambiguous content is rejected for clarification rather than altered silently. Output truncation footers/elisions in a purported whole-file payload trigger a diagnostic. Legitimate text containing such strings must remain expressible via explicit raw write mode, not an unescapable heuristic prohibition.

# 3. Snapshot model and line identity

Snapshot identity: canonical URI + provider identity + exact source revision + full byte digest. Store raw bytes/line endings and a normalized parser view (LF, BOM removed). User tag is donor-compatible uppercase four-hex XXH32 low 16 bits over normalized content with trailing spaces/tabs/CR trimmed per line. The tag is a locator, **not uniqueness or integrity proof**. Resources backed by a real file (`artifact://`, SPEC-0012 §4.3) share the snapshot of that file, so an edit through either name is validated against the same revision.

Retained snapshot stores normalized text, raw digest/revision, line-ending map, shown full-line intervals, branch/context epoch, tag and last-use counter. Defaults: 256 resources, 4 versions/resource, 64 MiB estimated retained bytes, 4 MiB snapshot source ceiling. These limits bound retention, not product format support. A file too large for retained hashline snapshots stays readable and gives an explicit edit prerequisite/limit error; do not emit a usable-looking tag and silently bypass validation.

Physical source lines follow the shared reader convention: one-based, terminal newline does not create an extra source line; empty source has zero lines. EOF/BOF gap operations remain valid for empty files. Donor fixture differences at newline boundaries must be resolved with explicit tests, not two coordinate systems.

Seen provenance:
- Only fully emitted source lines delivered to the model count as seen; a UI preview alone does not.
- Summary fold markers, partially column-capped lines, binary strings and query rows never mark underlying lines seen.
- Successful write marks the authored replacement known. Successful edit maps previously seen unchanged lines and newly authored lines into the new version; it does not mark every unseen unchanged line seen.
- Failed edit reveals can mark only lines actually returned within result limits.
- Compaction/branch navigation invalidate visibility assumptions that are not retained in the active context. Snapshots can remain cached, but stale-epoch seen sets cannot authorize edits.
- Raw reads retain exact source spans in Details; line/tag output is absent, so the model must obtain a tagged view before supplying hashline anchors.

Multiple retained snapshots with the same short tag and different relevant bytes are ambiguous: reject and return current context, never pick “latest” arbitrarily. Raw-byte differences hidden by tag normalization still require revalidation; whitespace normalization does not authorize overwriting unrelated changes.

# 4. Grammar

Section `[PATH#TAG]` is required for each existing source. Paths can be quoted where the donor grammar supports quoting; parser must handle spaces, URI authorities and literal # without guessing. TAG is exactly four hex characters. The model-facing grammar advertises canonical forms below; tolerant donor forms are recovery paths with warnings, not an undocumented second language.

| Operation | Meaning |
|---|---|
| PUT N.=M: | Replace inclusive original-source range with following + payload rows |
| PUT <N: / PUT >N: | Insert before/after original line |
| PUT <1: / PUT >$: | Insert at head/tail; valid for empty existing file |
| PUT N*: | Replace multi-line syntactic block beginning at N |
| PUT >N*: | Insert after that block at sibling depth |
| CUT N.=M / CUT N* | Remove and capture range/block to batch anonymous clipboard |
| CUT N.=M @name / CUT N* @name | Remove and capture to named clipboard |
| PUT <N @name / PUT >N @name | Paste into gap; anonymous when name omitted |
| PUT >N* @name | Paste after block; anonymous gap paste allowed by donor grammar |
| PUT N.=M @name / PUT N* @name | Replace range/block from named register (name required) |
| REM | Delete file |
| MV DEST | Rename after section edits; quoted destination for spaces |

Colon forms consume `+TEXT` rows verbatim. `+` is a blank line; `+- item` and `++ item` encode literal leading minus/plus. A register paste has no payload. Creation uses write, not tagless headers. All line numbers address each section's original snapshot, never earlier hunk-adjusted output.

Canonical example (tags illustrative, must come from real output in use):
```text
[src/old.go#A1B2]
CUT 10* @helper
[src/new.go#C3D4]
PUT >20 @helper
PUT >$:
+
+// end
```

Preflight resolves duplicate canonical targets and move aliases; ambiguous repeated sections, conflicting REM/MV or overlapping replacements fail before writes. Several inserts at the same gap preserve patch order. CUT captures original bytes before deletion; paste dependencies are resolved before commit. Duplicate register definitions and forward/cyclic uses need donor fixture parity; the implementation must reject ambiguous cases rather than pick a convenient value.

Named registers persist across calls on the branch; anonymous cuts only within a batch. Register names refer to content, not paths/commands. Registers retain source provenance and trust. Apply them only after the section/batch outcome makes capture valid; partial cross-file commit reports precisely which cuts/pastes committed. No capture from a failed validation becomes a later invisible mutation source.

# 5. Syntax-block semantics

One syntax service powers hashline blocks, read summaries and enclosing-boundary context. Use a maintained language parser/grammar backend approved in M0; lexical bracket scanning is display-only fallback, not full block edit parity.

Resolve outermost eligible named node beginning on the anchor's first content column, excluding the whole-file root and sibling statement-sequence containers. Include decorators/attributes/doc-comments only when anchored at their beginning and recognized as part of the construct. Reject syntax-recovery spans in the resolved subtree; unrelated syntax errors elsewhere do not disable a valid block. Blank/continuation/closing lines and single-line constructs are not valid replacement block anchors.

Markdown heading block extends through nested headings up to the next same/higher-level heading. Respect fenced code so a heading-looking code line does not terminate a section. Trailing whitespace/separator behavior follows donor fixtures. `PUT >N*:` means after the section/block, not inside its closer; ordinary gap PUT is the inside operation.

Error response gives next/enclosing candidate spans and explicit range/gap alternatives when known. Preserve donor safe lowering for insert-after-block anchored on a closer/unresolved block only when the resulting gap is unambiguous; report the lowering. Never lower failed block replacement/CUT to a guessed brace range.

Grammar support matrix must enumerate donor languages and representative constructs (decorators, nested declarations, indentation blocks, statement sequences, Markdown). Parser/backend gaps block parity for that language and remain visible; no “all languages” claim from Go-only tests.

# 6. Seen guards and recovery

Default enforceSeenLines true. Replacements/CUT require touched source lines known, including block interior. Inserts require a known neighboring anchor except explicit BOF/EOF and genuinely empty file cases; endpoint recognition alone does not make a hidden block replaceable. REM requires whole-source awareness or an explicit user-approved deletion operation; the four-hex tag alone is insufficient consent.

Recovery stages are ordered:
1. Parse canonical grammar; tolerate only donor-recognized wrappers/copied prefixes/bare-op mistakes with explicit diagnostics.
2. Resolve tag against canonical resource snapshots, refuse wrong-resource tags; a collision within one resource resolves to the newest matching snapshot (G5.2, D5.b).
3. If source drifted, compute snapshot→current changes. Relocate only unchanged spans with unique adequate non-whitespace evidence (donor threshold >=8 chars), preserving all intervening external edits. Block bounds and seen coverage must still validate.
4. Unseen or stale/ambiguous target: no mutation; return bounded current numbered context with new tag and requested next read.
5. Optional model-assisted repair: separate opt-in, maximum one proposal per call, uses configured authorized inference and reruns all validation. It cannot widen target scope, suppress policy, fabricate seen-state or auto-commit ambiguous semantic changes. Present altered interpretation explicitly.

Copied `N:` payload prefixes, diff deletion rows and empty PUT are compatibility-recovery concerns. Tests distinguish clear transport artifacts from legitimate code/text with matching prefixes. A correction that changes intended content requires retry/confirmation, not silent “helpfulness”.

Identical no-op payload tracking is per canonical resource + payload digest + source revision. Three consecutive identical no-ops escalate to an actionable loop error. A genuine source change resets the loop; separate sessions do not share it.

# 7. Stage and commit

```text
parse → resolve/authorize → instruction prerequisite → snapshot/seen validation
      → block/register resolution → stage all changes → preview facts
      → revalidate revisions → commit per resource → receipt + new snapshots
```

Acquire multi-resource locks in sorted canonical-identity order; include rename destinations and clipboard transaction state. Do not hold locks while waiting for the model, user confirmation or inference. Revalidate afterward. Reject target symlink/identity changes; authorization follows the resolved target and its containing boundary.

Preserve encoding/BOM/newline convention and permissions where supported; do not restyle unchanged text. Mixed endings use the retained map; inserted rows use the local neighboring convention, or document LF default for new files. Non-UTF8 source needs an approved lossless codec; do not decode replacements with lossy Unicode.

Local commit uses same-directory temporary file, flush/close and atomic replacement with mode preservation. Locks coordinate Workbench callers only: ordinary filesystem rename is not CAS against arbitrary external writers. Record the limited guarantee and recheck identity as late as feasible. Remote/provider writes use declared revision checks and receipts from SPEC-0004.

Preflight failure commits nothing. A later I/O failure may produce a partial batch; report every committed/failed/unattempted resource. Do not blindly roll back over external changes. Recovery data is private, bounded and referenced by receipt; retaining backups requires configured policy. Cancellation after a commit reports effects, not “not applied”. Cross-provider move requires an explicit coordinator; unsupported is safer than a disguised copy/delete.

Secret placeholders (SPEC-0013 §4.3): tags and seen-state cover the real file bytes; rows shown to the model carry placeholders; placeholders in replacement text are substituted before validation and commit, so an edit never writes a placeholder into a file by accident.

# 8. Result, invalidation and verification

Result Details: outcome/effects, per-file before/after revisions/tags, operations, actual changed spans, warnings/recovery steps, first changed line, bounded diff, register changes and continuation. Do not expose full unseen source merely because it was retained internally. Error payload names stage/path/anchor and a concrete next action.

Commit invalidates syntax/read/search/conflict caches and reconciles instruction candidates; seen bookkeeping occurs from committed facts. Bash mutations are discovered by revision revalidation, not command-string guessing.

Acceptance:
- All donor grammar/apply/parser/block/clipboard/recovery/preview fixtures, pinned to source revision with license notices.
- Single/multi-line replacements, same-gap insertion order, empty file, EOF without newline, CRLF/BOM and mixed endings.
- Cross-file CUT/paste, named registers over calls, duplicate/undefined/cyclic usage and partial commit.
- Full-language block cases, decorators/comments, statement-sequence anti-swallowing and Markdown fences.
- Tag collision, wrong resource, stale shifted spans, ambiguous repeated code, eight-character evidence boundary.
- Hidden summary lines/column caps/unseen block interiors rejected and revealed correctly; compaction cannot resurrect seen state.
- Concurrent edits/rename destination races, symlink swaps, read-only provider, crash/cancel between commits and unrelated-work preservation.
- Exact-text mode applies disjoint edits to original source; write never silently installs truncated output.
- Direct and codemode calls yield identical safety/results; renderer preview never changes filesystem.

Open approval gates: syntax/grammar and diff/hash backend selection, model-assisted repair default, oversized snapshot policy and optional backup retention. They are explicit decisions, not permission to omit the full engine.

# 9. Donor parity (M0.1)

From the [donor parity audit](../analyses/2026-11-02-donor-parity-audit/README.md), oh-my-pi `crates/pi-edit` and `edit-*` tests at `553f0d1`. These rules are normative and carry the donor test names as fixture anchors.

**G5.1 Apply-time payload repair.** Each repair is reported as a warning:
- Duplicated boundary lines echoed around a range replacement are dropped. An echo too short to fill the widened range is refused, with or without a parser.
- A deleted closing line is kept when the payload omits it, and not when the payload restates it.
- A uniformly omitted base indent is restored from the unchanged structural rows. Intentional indentation-only replacements are kept.
- When a trailing comma is what keeps a one-row replacement from parsing, the error names it. The edit is still rejected if dropping the comma would not parse.
- A body shallower than its block slides past the closing line, across several closer levels, stopping at body depth, with a warning. It never crosses a line that another hunk targets.

**G5.2 Recovery.** Body comparison has a character (not byte) budget shared across all hunks and is skipped past that budget. A body written for the tagged line is remapped; one written for the current line is refused. A remap that breaks the parse, or that moves into a sibling construct, is refused. An isolated unique line with no neighbour offset is refused. A duplicated range is remapped only when its context matches. A refused remap leaves the clipboard untouched; an accepted remap publishes its registers. An unchanged anchor replays onto the current text. Tag collision within one canonical resource (user decision D5.b): recover from the newest retained snapshot of that resource whose tag matches, then run the full relocation validation; a collision across different resources is refused.

**G5.3 Parser leniency.** Each case is recognised and warned about, never silent:
- lenient range separators and dangling ranges;
- a bare range header becomes an implicit `PUT`;
- copied read elisions are ignored;
- `CUT` with a colon is accepted;
- a `CUT` supersedes a placeholder `PUT` on the exact same range ("kept only the last");
- a trailing bare blank before the next hunk is dropped;
- `ABORT` terminates the input;
- legacy `DEL`/`COPY` headers become orphan rows;
- `apply_patch` envelope noise and nested headers are recovered;
- same-path sections merge, and conflicting tags fail;
- multi-group headers are rejected instead of inventing a path;
- `#` is allowed only in tagged paths;
- a literal payload that looks like an op warns.

Overlapping hunks are still rejected.

**G5.4 Streaming.** Partial JSON arguments are repaired incrementally. Matcher paths and entries are available from partial input for approval and inspection. A streaming preview keeps completed sections when a trailing header arrives. Incomplete empty-body ops are dropped while streaming; body-less ops are kept.

**G5.5 Snapshot store.** Identical snapshots dedupe, are promoted, and union their seen lines. Seen-line provenance is shared across lookups. Budgets count UTF-8 bytes. `MV` relocates store entries and merges duplicates. An oversized file is not read at all. Zero limits disable snapshots.

**G5.6 Path policy.**
- Generated files are guarded, detected by name and by a leading generated-code comment. A hand-authored file with a generated-looking name (e.g. `openapi.json`) is allowed; an ambiguous basename needs the content marker.
- FIFOs are refused without blocking.
- A unique suffix is recovered; an ambiguous suffix is refused.
- Symlink-equivalent paths share one canonical key.
- URI-shaped targets are never treated as working-tree paths.
- Read-only and process handlers are refused at the approval gate. A URL target that carries a line selector is refused. `local:/../` traversal is refused. A working-tree destination raises the approval tier (G5.13).

**G5.7 Notebooks.** `.ipynb` cells are edited as source while outputs, metadata and key order are preserved. Marker-like source lines are escaped. A new notebook is nbformat 4.5. JSON serialisation is byte-stable for unchanged content.

**G5.8 Diff preview.** Unchanged gaps between distant edits collapse. A matching bracket line beyond the context is added. Compact previews count, renumber and collapse long added runs. Unified diffs are numbered and have no duplicate boundary rows.

**G5.9 Per-model mode.** `edit.mode-rules` maps model-family patterns to a mode and ships the donor list as defaults: Kimi/K3, MiMo, DeepSeek, Step 3.7 Flash, Codex Spark, MiniMax and GLM 5.3 Flash use replace. An explicit user mode wins. Rules apply only when the resolved mode is hashline.

**G5.10 Early abort while streaming (conditional on a host probe).** When the final streamed preview already fails, abort the assistant stream and re-drive with the diagnostic. The recovery budget is per prompt and is skipped once the prompt is superseded or the session generation changes. Execution always uses the final arguments and re-resolves URL targets.

**G5.11** Warn when an edit turns a parseable file unparseable (valid → invalid only). The optional transition log is off by default.

**G5.12** `MV` keeps the source's mode bits, including setuid, and completes when the destination refuses chmod.

# 10. Edit tracking for other extensions

Other tools can show file content that the model may then edit with hashline anchors, without a second `read`. Capability `snapshots`:

- **`snapshots.view`** `{toolCallId, uri, ranges?, raw?}`. `toolCallId` must belong to a running call of a tool the caller owns (cross-checked with host source metadata). Workbench reads the resource itself, through its own authorisation and read pipeline, records a **pending** snapshot and returns the rendered block (`[path#TAG]` header plus `N:LINE` rows), tag, revision and shown ranges. The caller includes the block verbatim in its tool result.
- **Confirmation without trust in the caller.** When that call's `tool_execution_end` arrives, Workbench checks that the exact rendered block is present in the result content delivered to the model. Only then are the shown lines marked seen. A block that was cut, altered or not included marks nothing, and the pending snapshot expires.
- **`snapshots.status`** `{uri}` → current tag and revision, whether a seen snapshot exists, and whether it is stale. Counts only, never content.
- **`snapshots.touched`** `{uris}` → hint that the caller changed these files. Workbench revalidates them (hint only; edits still revalidate revisions).

Limits are those of `read`. Provider-backed resources need the SPEC-0004 §7 prerequisites; a read-only provider's content can be viewed and anchored for reference, but edits on it stay `unsupported`.
