---
document:
  type: "plan"
  id: "PLAN-0003"
  title: "Slice 2: full depth, every feature usable by agents"
  description: "Depth on the Slice 1 foundation: format adapters, remaining schemes, SSH, worktrees, PTY, donor-parity heuristics, project conventions, remaining instruction, interaction and messaging flows, with plain UI"
  status: "draft"
  revision: 8
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-10T17:05:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 8
      reason: "User decision 10.11.2026: out:// had no producer and leaves Slice 1; it returns in Slice 2 with one"
      changes: ["W4 owns out:// together with its producer"]
---

# 1. Outcome

Workbench reaches full functional scope on the stock host: every PLAN-0001 §9 row assigned to Slice 2 passes with plain UI, and agents can use all of it through tools, URIs and commands in the modes of PLAN-0002 §6. What remains afterwards is UI/UX chrome, fork-host rows and release work ([PLAN-0004](0004-slice-3-completion.md)).

This slice adds **depth** to the [PLAN-0002](0002-slice-1-foundation.md) foundation. Every package extends a Slice 1 engine behind its existing abstraction and frozen contract. Replacing a Slice 1 engine or changing a frozen contract needs a decision recorded in PLAN-0001 first.

Umbrella: [PLAN-0001](0001-pig-workbench.md). Detail level: work packages with exits; expand each into steps when it starts, after re-reading its specification at the current commit.

## Progress

- **later:** all work packages. Starts when PLAN-0002 exits.

# 2. Dependencies

All packages can be developed in parallel. These are **completion** dependencies: a package is complete only when the named interface exists and its integration test passes.

| Shared interface | Owner | Consumers |
|---|---|---|
| Reversible notebook cell mapping (cell view ↔ source bytes) | W2 | W1 notebook editing |
| SSH backend for location, resources and jobs | W3 | W4 `ssh://` remote search paths, W5 remote instruction checks |
| Worktree state and artifact storage (`tree`/`branch`) | W3 | W4 `project://` locations and artifact projections, W7 artifact validation paths |
| Coordinator `lifecycle` grant | W6 | W6 titles of other live sessions |
| Archive backends approved (PLAN-0002 A0 ledger) | user | W2 archives |

# 3. Work packages

## W1. Edit depth (SPEC-0005 §9, WB-03)

G5.1 payload repair, G5.3 leniency with warnings, G5.9 per-model mode rules, G5.10 streaming previews and abort (probe first), opt-in model-assisted repair, notebook cell editing on W2's mapping.

**Exit:** the donor fixture corpus passes in full; supported-language matrix stated.

## W2. Read adapters (SPEC-0006 §5, WB-05)

On the approved backends: JSON/JSONL queries and pagination, SQLite read plus row writes (D6.a), archives with the donor format inventory, PDF and documents, notebooks with the reversible cell mapping, raster and SVG, binary inspection, video and profile views.

**Exit:** SPEC-0006 §5 matrix and §8 gates; each adapter runs through the same URI/selector/budget pipeline with an end-to-end fixture; bombs, runaway queries, special files and hostile inputs are bounded; derived views issue no editable anchors without a reversible mapping.

## W3. Remote and worktrees (SPEC-0008 §3–§4, SPEC-0012 §4.3–§4.5, WB-12)

SSH backend for the location, resource and job abstractions; `ssh://`; remote edits with snapshot validation; `/remote` host management preserving SSH config; worktrees with shared artifact tree and links; `/workbench projects artifacts-storage`; PTY backend and key controls; remote jobs; G8.20 refusal under an `ssh://` cwd; a remote `instructions.HostReader` so the active SSH host's `/etc/agents/AGENTS.md` and `HOST.md` replace the local copies (`instructions.ResolveHost` already resolves per file name; SPEC-0007 §3.1).

**Exit:** SPEC-0008 §8 rows; local and SSH integration tests for quoted paths, auth failure, disconnect, changed remote files, cancellation, descendants, adoption races, reload and crash; no unverified remote-kill guarantee.

## W4. Remaining schemes (SPEC-0012 §5.3–§5.4, SPEC-0013 §6, WB-07/10/24)

`out://` with a producer (`grep`/`find` results beyond the display limit spill up to a hard cap and the truncated result names the URI; store with budget and oldest-first eviction, root listing, `out` in the name-authority selector set of `resource.ParsePath`, guide page), `project://`, `conflict://` with its registry, `secret://` (with `rm`/`mv` through the Slice 1 G8.20 bridge) and `/workbench secrets`, `session://by-project/` and `by-name/`, `session://<id>/tree` (entry tree with branches and labels, own and stored sessions), `session://<id>/messages` for stored sessions (the own session has it since Slice 1), cross-session search, `docs://` PiG source checkout.

**Exit:** SPEC-0012 §6 and SPEC-0013 §11 rows for these schemes; read/list/search/control tests with pagination; false-positive removal through `mv … secret://.not-a-secret/` with the approval states of SPEC-0013 §6; no credential exposure or trust widening through project config.

## W5. Context depth (SPEC-0007 §8–§9, SPEC-0014 §9, WB-13/18/25)

Remote instruction checks by on-turn stat/hash, external project notification ingress, further instruction classes and `select = "all"`. Model-family adapters are PLAN-0004 C6.

**Exit:** SPEC-0007 §10 remaining rows; SPEC-0014 §12 `select = "all"` row; model-context evidence matches `/workbench instructions`.

## W6. Interaction and messaging depth (SPEC-0009 §4, §6–§7, §9; SPEC-0011 §9; WB-15/16/17/20)

`ask_user` remote bridge, replay and answer history (D9.c); `/answer`, `/answer:again`, `/ask:replay`, `/ask-settings`; `/tldr` checkpoints; codemode call trees and replay; broadcast; the opt-in coordinator with `/workbench coordinator on|off` as the only way to grant it; titles of other live sessions under the `lifecycle` grant; `--list-tools` in TUI and RPC.

**Exit:** SPEC-0009 §10 and SPEC-0011 §10 remaining rows; ask submit/reject/cancel over the bridge; coordinator enabled only by user command, each class `policyDenied` until granted.

## W7. Project conventions (SPEC-0012 §4.1–§4.2, WB-22)

Bundled `dot-project` and `agents-md` skills adapted to Workbench (the `tasks` line per the A0 decision), artifact frontmatter validation on `write`/`edit` under `.project/` reported without blocking, `ls artifact://specs/` frontmatter columns, reminder frame for a new module directory without `AGENTS.md`, `BACKLOG.md` pointer validation.

**Exit:** each check fires on a fixture with the violation and stays silent on a clean tree; validation never blocks a write.

# 4. Slice exit

1. Every PLAN-0001 §9 row assigned to Slice 2 passes on the stock host; evidence linked in a progress table here. Fork-host rows and chrome are named as PLAN-0004 rows in the report, not counted as passed.
2. PLAN-0001 §7 cross-feature scenarios and failure matrix pass.
3. A live agent session (user-authorized inference) exercises the depth features end to end, including SSH and worktrees; transcript kept as evidence.
4. pig-web's deferred gate item (worktree `cd` keeps project KB scope) passes; reply handoff to pig-web.
