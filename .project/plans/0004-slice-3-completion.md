---
document:
  type: "plan"
  id: "PLAN-0004"
  title: "Slice 3: UI/UX polish, enhancements and release completion"
  description: "Design review, kit panels and renderers, captures, fork-dependent features, performance, platforms, documentation and the integrated release gate"
  status: "draft"
  revision: 1
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T13:53:58+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed PLAN-0001 M0.5, the UI part of M7 and M8"]
---

# 1. Outcome

Workbench is complete: designed and captured UI for every surface, the remaining enhancements, measured performance, platform coverage, product documentation and a passed release gate. Depends on [PLAN-0003](0003-slice-2-feature-complete.md) exit. Umbrella: [PLAN-0001](0001-pig-workbench.md). Milestone level only; detail each milestone when PLAN-0003 is close to its exit.

## Progress

- **later:** all milestones.

# 2. Milestones

## C1. Design review (PLAN-0001 M0.5)

Review SKETCH-0001, the extension-API and workflow sketches against the implemented command and feature inventory. Settle navigation, density, scope visibility, destructive confirmations, pin semantics, questionnaire flow and the 140-column split view with the user. **Exit:** revised design artifacts with an approved fixture and capture checklist.

## C2. Kit prototypes and shared UI abstractions

Fixture-data prototypes of the common shell, tool and codemode rows, settings, jobs, instructions and ask; captures at 36/57/80/140 columns and ~16 rows; user approval before shared panel and renderer abstractions are finalized. **Exit:** approved captures.

## C3. Panels, renderers and chrome

Replace plain views with the approved designs: `/workbench` hub and all panels, settings editors, tools/jobs/frames/instructions/trust/remote/secrets panels, compact/normal/full renderers with donor density defaults (G3.1), codemode trees, activity line and statusline modes (D9.d), headless equivalents. **Exit:** captures for every surface, resize and hostile-text cases reviewed.

## C4. Fork-dependent features (D13)

On fork capability flags: widget view events (activity-line click), `sections` prompt delivery, transcript truth (F-P3), late-section hook (F-P4), H-S1/H-S2 secrets hooks, codemode child rendering, `--list-tools` output. Stock PiG keeps the documented fallbacks. **Exit:** both host variants tested.

## C5. Enhancements

Automatic session titles; other approved enhancements collected during Slices 1–2 (recorded as decisions in PLAN-0001 before work starts). WB-21 (`ast_grep`/`ast_edit`) only after its design decision. **Exit:** each enhancement has its own acceptance evidence.

## C6. Performance and platforms

Record baselines, then set and meet numeric targets (PLAN-0001 §7); remaining backend coverage checks (cpio and other archives, grammar budget, HTML quality); macOS and Windows coverage. **Exit:** measured budgets and a support matrix.

## C7. Release gate (PLAN-0001 M8)

Re-run every scope row on the assembled extension, fused and subprocess where applicable; package identity, tool conflicts, install/uninstall; notices, product docs, README, examples, bundled skill, privacy and cleanup review. No version bump before the release boundary; no publication without authorization. **Exit:** no unverified required row; residual limitations accepted by the user.
