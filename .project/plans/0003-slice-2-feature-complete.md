---
document:
  type: "plan"
  id: "PLAN-0003"
  title: "Slice 2: full depth, every feature usable by agents"
  description: "Depth on the Slice 1 foundation: format adapters, remaining schemes, SSH, worktrees, PTY, donor-parity heuristics, remaining instruction, interaction and messaging flows, with plain UI"
  status: "draft"
  revision: 2
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T14:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed the functional scope of PLAN-0001 M2–M7 on top of PLAN-0002"]
    - revision: 2
      reason: "User defined foundation as everything fully working; this slice adds depth only"
      changes: ["Work packages reduced to the depth column of PLAN-0002 §2", "Depth extends Slice 1 engines; it never replaces them"]
---

# 1. Outcome

Workbench reaches full functional scope: every PLAN-0001 scope row passes with plain UI, and agents can use all of it through tools, URIs and commands in TUI, RPC and print. What remains afterwards is UI/UX chrome and release work ([PLAN-0004](0004-slice-3-completion.md)).

This slice adds **depth** to the [PLAN-0002](0002-slice-1-foundation.md) foundation: the right-hand column of its §2 table. Every package extends a Slice 1 engine behind its existing abstraction and frozen contract. Replacing a Slice 1 engine or changing a frozen contract needs a decision recorded in PLAN-0001 first.

Umbrella: [PLAN-0001](0001-pig-workbench.md). Detail level: work packages with exits; expand each into steps when it starts, after re-reading its specification at the current commit.

## Progress

- **later:** all work packages. Starts when PLAN-0002 exits.

# 2. Work packages

W1–W6 are independent of each other once PLAN-0002 exits, except W5's remote watching, which needs W3's SSH backend.

## W1. Edit depth (SPEC-0005 §9, WB-03)

Donor parity heuristics G5.1–G5.3 (payload repair, recovery ladder, leniency with warnings), streaming previews and abort while arguments stream (G5.10), per-model mode rules (G5.9), opt-in model-assisted repair, notebook cell editing.

**Exit:** the donor fixture corpus passes in full; supported-language matrix stated.

## W2. Read adapters (SPEC-0006, WB-05)

On the approved backends: JSON/JSONL queries and pagination, SQLite read plus row writes (D6.a), archives with the donor format inventory, PDF and documents, notebooks, raster and SVG, binary inspection, video and profile views.

**Exit:** each adapter runs through the same URI/selector/budget pipeline with an end-to-end fixture; bombs, runaway queries, special files and hostile inputs are bounded; derived views issue no editable anchors without a reversible mapping.

## W3. Remote and worktrees (SPEC-0008, SPEC-0012 §4, WB-12)

SSH backend for the location, resource and job abstractions; `ssh://`; remote edits with snapshot validation; `/remote` host management preserving SSH config; worktrees with shared artifact tree and links (SPEC-0012 §4.3–4.5); PTY backend and key controls; remote jobs.

**Exit:** local and SSH integration tests for quoted paths, auth failure, disconnect, changed remote files, cancellation, descendants, adoption races, reload and crash; no unverified remote-kill guarantee.

## W4. Remaining schemes (SPEC-0012 §5, SPEC-0013 §6, WB-07/10/22/24)

`project://`, `conflict://` with its registry, `secret://` and `/workbench secrets`, `session://by-project/` and `by-name/`, cross-session search, titles, `docs://` PiG source checkout.

**Exit:** read/list/search/control tests with pagination per scheme; no credential exposure or trust widening through project config.

## W5. Context depth (SPEC-0007, SPEC-0014 §6/§7/§9, WB-13/18/25)

Remote watching with on-turn fallback, external project notification ingress, further instruction classes and `select = "all"`, `builtin/workbench`, model-family adapters beyond the probed set.

**Exit:** full instruction matrix including remote locations; model-context evidence matches `/workbench instructions`.

## W6. Interaction and messaging depth (SPEC-0009, SPEC-0011 §9, WB-15/16/17/20)

`ask_user` remote bridge, replay and answer history (D9.c); `/answer`, `/answer:again`, `/ask:replay`, `/ask-settings`; `/tldr` checkpoints; codemode call trees and replay; broadcast and the opt-in coordinator; `--list-tools` as far as the host allows.

**Exit:** every command inventory row exercised in TUI, RPC and print; ask submit/reject/cancel over the bridge; coordinator safeguard tests.

# 3. Slice exit

1. Every PLAN-0001 scope row (except WB-21 and the chrome parts of WB-02/16/17/23) passes its acceptance anchor with plain UI; evidence linked in a progress table here.
2. PLAN-0001 §7 cross-feature scenarios and failure matrix pass.
3. A live agent session (user-authorized inference) exercises the depth features end to end, including SSH and worktrees; transcript kept as evidence.
4. pig-web's deferred gate item (worktree `cd` keeps project KB scope) passes; reply handoff to pig-web.
