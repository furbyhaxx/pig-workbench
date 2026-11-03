---
document:
  type: "plan"
  id: "PLAN-0004"
  title: "Slice 3: UI/UX polish, enhancements and release completion"
  description: "Design review, kit panels and renderers, captures, fork-dependent features, performance, platforms, documentation and the integrated release gate"
  status: "draft"
  revision: 3
  created_at: "2026-11-03T13:53:58+01:00"
  updated_at: "2026-11-03T15:05:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User split delivery into three slices: foundation, feature-complete, completion"
      changes: ["Absorbed PLAN-0001 M0.5, the UI part of M7 and M8"]
    - revision: 2
      reason: "External plan review (2026-11-03)"
      changes: ["C4 lists fork-host rows from PLAN-0002 §6", "C6 drops backend investigations already closed by the backend evaluation"]
    - revision: 3
      reason: "User: evaluate model-aware prompts in Slice 3, now that the harness research is complete"
      changes: ["New C6 model-aware prompts (SPEC-0014 §6.3) moved here from PLAN-0003 W5", "Performance/platforms and release gate renumbered C7, C8"]
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

On fork capability flags: widget view events (activity-line click), `sections` prompt delivery, transcript truth (F-P3), late-section hook (F-P4), H-S1/H-S2 secrets hooks, codemode child rendering, print/JSON output for slash commands and `--list-tools` (P27). Stock PiG keeps the documented fallbacks of PLAN-0002 §6. **Exit:** every fork-host row of PLAN-0002 §6 and PLAN-0003 passes on the fork; the stock rows still pass.

## C5. Enhancements

Automatic session titles; other approved enhancements collected during Slices 1–2 (recorded as decisions in PLAN-0001 before work starts). WB-21 (`ast_grep`/`ast_edit`) only after its design decision. **Exit:** each enhancement has its own acceptance evidence.

## C6. Model-aware prompts (SPEC-0014 §6.3)

The harness research is complete and published: [antigravity-cli](../research/antigravity-cli/README.md) (agy 1.4.8 baseline, 1.4.12), [claude-code-cli](../research/claude-code-cli/README.md) (2.1.310 baseline, 2.1.317), [codex-cli](../research/codex-cli/README.md) (0.167.0-alpha.4 source) and [deepseek-harness](../research/deepseek-harness/README.md) (0.2.2-alpha.3 source). Compare them per instruction topic (autonomy, tool use, editing, verification, safety posture, output style) in the analysis SPEC-0014 §6.3 names, propose family adapters with evidence links, and adopt each into `builtin/workbench` only after an A/B run on fixed tasks against the Slice 1 baseline (golden transcripts, A/B, cache-hit rate; user-authorized inference). Re-check the dossiers against current harness versions first; they track moving targets. **Exit:** the analysis artifact; each adopted adapter with its A/B evidence; rejected ones recorded with the reason.

## C7. Performance and platforms

Record baselines, then set and meet numeric targets (PLAN-0001 §7); macOS and Windows runtime evidence for the backends, watchers, jobs and messaging (backend evaluation §9 has compile-level evidence only). **Exit:** measured budgets and a support matrix.

## C8. Release gate (PLAN-0001 M8)

Re-run every scope row on the assembled extension, fused and subprocess where applicable; package identity, tool conflicts, install/uninstall; notices, product docs, README, examples, bundled skill, privacy and cleanup review. No version bump before the release boundary; no publication without authorization. **Exit:** no unverified required row; residual limitations accepted by the user.
