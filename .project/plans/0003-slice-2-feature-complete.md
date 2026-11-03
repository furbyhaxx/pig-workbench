---
document:
  type: "plan"
  id: "PLAN-0003"
  title: "Slice 2: every feature implemented and usable by agents"
  description: "Full hashline, advanced reads, catalogs, SSH, worktrees, jobs, instruction lifecycle, secrets, messaging and interaction flows on the Slice 1 foundation, with plain UI"
  status: "draft"
  revision: 1
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T13:53:58+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed the functional scope of PLAN-0001 M2–M7 on top of PLAN-0002"]
---

# 1. Outcome

Workbench loads in PiG and agents can use **every feature** in PLAN-0001's scope table through tools, URIs and commands, in TUI, RPC and print. Behavior is complete and tested; what is missing is expensive UI/UX: kit panels, designed layouts, density tuning, captures and polish ([PLAN-0004](0004-slice-3-completion.md)). Commands and views work with plain renderers.

Depends on [PLAN-0002](0002-slice-1-foundation.md) exit. Umbrella: [PLAN-0001](0001-pig-workbench.md). Detail level: work packages with exits; expand each into steps when it starts, after re-reading its specification at the current commit.

## Progress

- **later:** all work packages. Starts when PLAN-0002 exits.

# 2. Work packages

Order follows PLAN-0001 §4: W1 before W2's code views; W3, W4 independent after the foundation; W5 needs W2–W4; W6 overlaps once APIs are stable.

## W1. Full hashline engine (PLAN-0001 M2; SPEC-0005, WB-03)

Complete grammar and tokenization, multi-section staging, all PUT/CUT/range/gap/block forms on the approved tree-sitter backend, named and anonymous registers, REM/MV, prefix handling, mismatch reveals, evidence-based recovery (G5.2) and parser leniency (G5.3), no-op guards, streaming previews, post-edit results, per-model mode rules (G5.9), D5.b/D5.d as decided in PLAN-0002 A1, optional model-assisted repair (opt-in). Notebooks through cell views.

**Exit:** donor fixture corpus plus collision, concurrency, CRLF, BOM, symlink and failed-commit tests pass; block and cross-file register cases run through the public tool; supported-language matrix stated.

## W2. Advanced reads and search (PLAN-0001 M3; SPEC-0006, WB-04/05)

Summary folding, enclosing boundaries, seen-row recording, repeated-read hints, supersession, directory paging, grep/find enhancements. Adapters on the approved backends: JSON/JSONL queries, SQLite read plus row writes (D6.a), archives, PDF/documents, notebooks, raster/SVG, builtin static web reader (fallback when no `fetchBackend` serves a URL), binary inspection, video and profile views.

**Exit:** every approved adapter goes through the same URI/selector/budget pipeline; bombs, runaway queries, special files and hostile URLs are bounded; derived views issue no editable anchors without a reversible mapping.

## W3. Catalog and control schemes (PLAN-0001 M4; SPEC-0012 §5, SPEC-0014 §8, WB-07/08/10/22)

Full `session://` trees (`by-id/`, `by-project/`, `by-name/`, titles, search, transcript/tree/entry projections), `agent://`, `project://`, `artifact://` browsing, `skill://` with progressive disclosure, `cfg://` controls and `cfg://workbench/`, `docs://` with PiG source, conflict registry, `secret://`, first-touch guides.

**Exit:** read/list/search/control tests with pagination per scheme; no credential exposure or trust widening through project config; reload executes at a verified safe boundary.

## W4. SSH, worktrees and jobs (PLAN-0001 M5; SPEC-0008, WB-09/12)

Shared SSH transport, remote file/search/edit with snapshot validation, location transitions, worktrees with the shared artifact tree and links (SPEC-0012 §4.3–4.5), session bucket symlinks, `/remote` host management preserving SSH config. Jobs: promotion, stdin/EOF/signals/PTY keys, waits, notifications, secret env injection (SPEC-0013 §5), G8.20 scheme commands.

**Exit:** local and SSH integration tests for quoted paths, auth failure, disconnect, changed remote files, cancellation, descendants, adoption races, reload and crash; no unverified remote-kill guarantee.

## W5. Instruction and ambient-context lifecycle (PLAN-0001 M6 and §6; SPEC-0007, SPEC-0014 §5/§7/§9, WB-13/14/18)

File classes, ancestors, root/nearest/root+nearest/`select = "all"`, pins, local and remote watchers, new-candidate discovery, BACKLOG caps with two baselines, diff updates, dedup/relocation/withdrawal, self-edits, branch and compaction reconstruction, remaining context frames, `builtin/workbench`, external project notification ingress.

**Exit:** full instruction matrix including parallel codemode touches and changes during compaction; model-context evidence matches `/workbench instructions` facts.

## W6. Interaction, messaging and commands (functional part of PLAN-0001 M7; SPEC-0009, SPEC-0011, WB-15/16/17/20)

`ask_user` full (all types, notes, images, remote bridge, replay, answer history per D9.c), `/answer`, `/answer:again`, `/ask:replay`, `/ask-settings`; `/tldr` checkpoints; codemode call trees and replay; `agent://` messaging with loop guards, broadcast and the opt-in coordinator; every command inventory row with completions and machine-readable output; `--list-tools` as far as the host allows.

**Exit:** every command row exercised in TUI, RPC and print; nested call/cancel/error/replay correct; ask submit/reject/cancel verified; two-session wake/steer/loop-guard and coordinator safeguard tests.

# 3. Slice exit

1. Every PLAN-0001 scope row (except WB-21 and chrome parts of WB-02/16/17/23) passes its acceptance anchor with plain UI; evidence linked in a progress table here.
2. PLAN-0001 §7 cross-feature scenarios and failure matrix pass.
3. A live agent session (user-authorized inference) uses read, edit, grep, jobs, cd, SSH, `session://` and ask end to end; transcript kept as evidence.
4. pig-web's deferred gate items (worktree `cd` keeps project KB scope) pass; reply handoff to pig-web.
