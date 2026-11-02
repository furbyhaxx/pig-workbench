---
document:
  type: "specification"
  id: "SPEC-0005"
  title: "Full hashline editing and shared source snapshots"
  description: "Grammar, syntax blocks, registers, provenance, recovery and mutation integrity"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-02T18:45:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Complete the remaining Workbench specification and design areas"
      changes: ["Defined behavior, state, safety, examples, dependencies and acceptance criteria"]
    - revision: 2
      reason: "M0.2 probes verified builtin replacement"
      changes: ["Recorded how Workbench edit/write replace the host builtins"]
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

Snapshot identity: canonical URI + provider identity + exact source revision + full byte digest. Store raw bytes/line endings and a normalized parser view (LF, BOM removed). User tag is donor-compatible uppercase four-hex XXH32 low 16 bits over normalized content with trailing spaces/tabs/CR trimmed per line. The tag is a locator, **not uniqueness or integrity proof**.

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
2. Resolve tag against canonical resource snapshots, detect wrong-resource tags and collisions.
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
