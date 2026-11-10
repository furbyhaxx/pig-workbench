---
document:
  type: "plan"
  id: "PLAN-0005"
  title: "Slice 1 remediation: close the review findings before the slice exit"
  description: "Fixes every finding of REV-2026-11-10-slice-1 so PLAN-0002 can exit: shell and scheme paths, frame ledger, instruction discovery, resource trees, settings, skills, jobs, services, tool rows, panels, shared primitives and docs"
  status: "draft"
  revision: 5
  created_at: "2026-11-10T09:40:00+01:00"
  updated_at: "2026-11-10T15:10:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 5
      reason: "User decision 10.11.2026: no file watcher in Slice 1; instruction changes are found at boundaries"
      changes: ["Decision row and area C: watcher settings are removed, not wired"]
---

# 1. Outcome

Every finding of [REV-2026-11-10-slice-1](../reviews/2026-11-10-slice-1/report.md) is fixed, or moved to PLAN-0003 by an explicit plan revision. After this plan [PLAN-0002](0002-slice-1-foundation.md) §5 can exit: all three gates are green, the inventory is re-run at the fix commit, and the handoff to pig-web names that commit.

This plan adds no feature beyond what the specifications already assign to Slice 1, with one approved extension: scheme paths as arguments of ordinary shell commands (area A).

Task list: `.pig/tasks/slice-1-remediation.json`.

## Progress

- **done:** review, this plan, the task list.
- **now:** nothing started.
- **next:** area A, then B, C and D.
- **later:** areas E to M, then the exit (N).

# 2. Decisions

User decisions of 10.11.2026:

| Topic | Decision |
|---|---|
| C1 | Fix the false refusals **and** make scheme paths usable in shell commands, including the `!` editor command |
| H4b | Wire every setting whose feature exists. A setting without a consumer is not shown anywhere |
| H4c | The four unused items are superseded by wired code and are deleted |
| H5 | Skills are delivered **only as a frame**. A later change of availability emits a frame with only the difference: available now, no longer available |
| M6 | `operation.status` stays a reserved name in SPEC-0001 until the first deferred operation exists |
| M9 | Changing the `jobs` result envelope is approved |
| T3 | Move the ten own tools onto the SPEC-0003 presentation path now |
| Layouts | Frame rows, panel actions and the activity line each get a design with captures for review before implementation |
| PLAN-0003 | `session://<id>/tree` and `messages` for stored sessions and `/workbench jobs monitor\|write` move there by revision |
| Watcher | No instruction file watcher in Slice 1. Changes are found by digest at every prompt and tool call; the two `instructions.watch.*` settings are removed |
| `.project` rules | `.project/AGENTS.md` follows the `dot-project` standard: consumed artifacts are deleted with commit trailers, one changelog entry per file |

Background for two of them:

- **H4c.** `jobCatalog` is superseded by the paged `jobProvider.List`, `managerLogWriter` by `streamWriter`, `fetchRouter.list` and `statusBoard.list` by the contribution registry that `/workbench integrations` reads.
- **H5.** Today the listing is a static `<skills>` section of the system prompt, or part of the `session.capabilities` frame when `skills.listing = "frame"`. The template lists Workbench's own catalog with `skill://<name>` locations and, separately, `host.skills` by file path without load state. The host list arrives in `before_agent_start` options, so no new host seam is needed.

# 3. Do not

- Do not weaken or delete a test to get a gate green; the trust-panel test is repaired, not skipped.
- Do not change a wire, persisted or model-facing contract without a revision of its specification in the same change. Approved here: the `jobs` envelope (M9), G8.20 (area A), the frame ledger limit (area B).
- Do not settle a new layout during implementation. Frame rows, panel actions and the activity line need captures at 36, 57, 80 and 140 columns and at 16 rows, and user review.
- Do not start Slice 2 features: no SSH, PTY, coordinator, broadcast, `project://`, `secret://` or format adapters.
- Do not edit donor trees or the user's PiG configuration; live tests use an isolated agent directory.
- Do not spend on provider inference without fresh authorization.

# 4. Work areas

Order: A first, because it blocks ordinary shell use by the implementing agent. Then B, C, D. E to M can run in any order; J before K's capture review. N last.

## A. Shell commands and scheme paths

Current: `parseSchemeCommand` in `internal/host/schemecommand.go` treats any occurrence of `<scheme>://` in the command text as a scheme path and refuses everything that is not a standalone verb. `http` and `https` count as schemes. `userBash` in `internal/host/secrets_hooks.go` runs `!` commands in a plain shell and never dispatches.

Change:

1. **Detection.** A scheme path is a static argv word, or redirect target, that starts with a registered non-locatable scheme prefix. Text inside other words, quoted strings, heredoc bodies and non-static words is never a scheme path. `http` and `https` are excluded: external programs own them.
2. **Standalone verbs** keep dispatching in-process as today.
3. **Materialization.** In any other command, each scheme-path word that the router can read as a file is replaced by the path of a read-only copy in the session scratch directory, bounded by the read limits. A word the router cannot resolve is passed through unchanged. A directory or an over-limit resource fails before the command runs, naming the standalone form.
4. **`!` commands** use the same dispatcher and materialization.
5. `bash.route-to-tools = false` switches 2 to 4 off.
6. SPEC-0008 G8.20 is revised with these rules.

Tests: every command refused during the review now runs (`curl` on a URL, `grep` for a scheme string, a heredoc, a `sed` expression); `cat docs://… | wc -l` works; a write through a non-standalone command is refused; `! ls docs://workbench/` lists.

## B. Frames

Current: `bodySize` in `internal/frames/ledger.go` counts every record ever admitted against 2 MiB. No message renderer is registered. `frames.deliveryChanged` is never emitted. `frames.role` and `frames.display.*` are inert.

Change: limit pending bodies only and keep a bounded history of terminal records (receipt and digest kept, body dropped past the cap), with a restore path for existing sessions; register renderers for `pig-workbench.context`, `pig-workbench.skill` and `pig-workbench.ask` showing one row per frame per SPEC-0002 §7; honour `frames.display.style` and `frames.display.default-visibility`; emit `frames.deliveryChanged`; serve `session://self/frames` and `/workbench frames ls|show|hide|collapse`.

`frames.role` is not shown (area E): stock PiG delivers frames only through the custom-message route, and no verified host capability projects them as native system or developer messages (SPEC-0002 §6). The key returns with the fork-host `sections` delivery in PLAN-0004 C4.

Tests: 1000 delivered 8 KiB frames, then an admit succeeds; a resumed over-limit ledger loads; a live capture shows one row per frame.

## C. Instructions

Current: `toolInstructions` in `internal/host/instruction_context.go` inspects `path` and hashline sections only.

Change: use the same target extraction as the renderer (`readCallTargets`) for every tool, take the union, and serve `session://self/instructions/` as a listing with `ctl`. Wire `instructions.updates.self-edits` and `instructions.boundary`. Remove `instructions.watch.debounce-ms` and `instructions.watch.remote`: there is no watcher.

Tests: a `paths` read of files under two nested `AGENTS.md` delivers both before content; the barrier returns `instructionContextChanged` once.

## D. Resource trees

Current: session, agent and skill providers ignore `limit` and `cursor`; `session://self/system-prompt` is a fixed sentence; several tree entries are missing; a bare `https://host` fails.

Change: page every builtin provider; serve the rendered prompt by section with its epoch; add `out://` root listing, artifact columns, builtin schemes under `docs://workbench/schemes/`, `agent://` columns and `session://self/messages`; normalize an empty HTTP path; accept a selector on authority-only URIs. `session://<id>/tree` and `messages` for stored sessions move to PLAN-0003 by revision.

Tests: listings with 50 sessions, 50 peers and 50 skills page correctly.

## E. Settings

Current: 65 of 100 catalog keys have `Application = "unavailable"` (`internal/config/catalog.go`). They validate, save and are shown.

Change: classify all 65 in a table in this plan (wired here, already consumed but mislabelled, feature not built). Wire each key whose feature exists, in the area that owns it; the groups not owned elsewhere (`read.*`, `edit.snapshot.*`, `guides.*`, `storage.*`, `projects.*`, `context.timezone`, `prompt.template`, `secrets.env-injection`, `activity.*`) are wired here. A key whose feature is not built is **not shown**: absent from the settings panel, `/workbench settings catalog`, `cfg://` and the docs. A value for such a key in an existing `Workbench.toml` is preserved on save and ignored, as for any unknown key. A test fails when a shown key has no consumer.

## F. Skills

Current: the catalog reads `<cwd>/.agents/skills` and `<cwd>/.pig/skills`; host skills are listed in the system prompt by file path without load state.

Change:

1. **One catalog.** Retain the host skill list from `before_agent_start`, merge it by name with Workbench's frontmatter reading of each `SKILL.md`, and address every model-invocable skill as `skill://<name>`.
2. **Frame only.** The `<skills>` block leaves the system prompt template. The listing is delivered as its own frame, source `session.skills`, within `skills.listing-budget`. The setting `skills.listing` is removed.
3. **Diff frames.** When availability changes (a `when` condition starts or stops holding, a parent load reveals sub-skills, an unload withdraws them, a skill appears or disappears on disk), a frame carries only the difference: the entries available now and the names no longer available. No change, no frame.
4. **After compaction** the full listing is delivered again, then diffs resume.
5. SPEC-0014 §8.1 and §8.5 are revised; the prompt template loses its skills section in §7.

Also: wire `skills.conditions`, add `/workbench skills ls|load|unload`, and re-run the abandoned verification of conditions, nested skills and replay after compaction.

Tests: the rendered system prompt contains no skill listing; a user-level skill is listed in the frame as `skill://`, loads, unloads and replays; touching a file that satisfies a `when` emits one diff frame with that skill only; the budget ladder holds with 40 skills.

## G. Jobs and bash

Change: typed camelCase `jobs` results with command, title, cwd, start time, state and omitted count (SPEC-0008 §6 revised); `bash` gains `title` and `cwd`; `jobs wait` without id waits for any active job; states `failedToStart`, `stopping` and `unknown`; foreground commands retire separately from background jobs; six `jobs.*` settings wired; one signal table; short job labels and no `Job … exited` line for plain foreground results; the completion notice no longer HTML-escapes the command.

## H. Services and soundness

Change: assign `s.endpoint` and `s.activity` before the session is published; one cleanup stack in `app.start`; guard background users of the retained start context; lock the prompt fields; evict oldest-first instead of clearing maps; join the prompt render goroutine; real `workbenchVersion`; one in-flight limit constant; staticcheck notes; delete the four superseded items; sweep edit lock files; reserve `operation.status` in SPEC-0001 and add the missing channel rows.

## I. Secrets

Change: before a tool call runs, learn any env-shaped file it references (SPEC-0013 §2), bounded and through the same parser as the startup scan.

Tests: the review's case, an `.env` outside the cwd chain read through `read`, returns placeholders.

## J. Tool rows on the presentation path

Current: `internal/host/own_tools_render.go` builds rows from arguments and result text; contributed tools use `presentation_render.go` with SPEC-0003 snapshots.

Change: each own tool emits a presentation snapshot in its Details (phase, outcome, target, metrics, blocks); the ten tools register builtin descriptors and render through `renderPresentationCall` and `renderPresentationResult`; `own_tools_render.go` is deleted. Rows gain a status glyph and theme tokens for tool name, target, metrics and failure. Result bodies use relative paths. Density levels stay in PLAN-0004.

Tests: the existing own-tool row tests pass against the shared renderer; every preset and state fits 36, 57, 80 and 140 columns.

## K. Panels, commands and activity line

Change: no single printable key triggers an action; every hint is built with `sdk.KeyHint`; one alias table (`ls`/`list`, `rm`/`remove`, `config`/`settings`); completions for `/cd`, `/wb:cd`, `/tools`, `/wb:jobs`; short unknown-verb error with the closest verb; `prompt show|render|diff` and `tools --format`; consistent empty states; hub ellipsis and path display. Activity line: state item, instruction files, away location, event-driven publishing. `jobs monitor|write` moves to PLAN-0003 by revision.

New layouts need captures and user review before they are final.

## L. Shared primitives

Change: `syncDirectory`, `nonblockFlag`, atomic write and lock-file helpers live once in `internal/fileio`; `sameToken` and `validSessionID` have one definition each; text sanitizers share one package. Migrate package by package under `just race`.

## M. Documentation and artifacts

Change: generate the settings reference in `docs/configuration.md` from the shown catalog keys; update README limitations, `docs/tools.md` and `docs/extension-services.md`; add `internal/modelknowledge/AGENTS.md` and complete `internal/AGENTS.md`; fix stale Progress text in PLAN-0001, PLAN-0002 and SPEC-0002 §1; record the stricter relocation rule in SPEC-0005; revise PLAN-0003 for the rows moved there.

`.project` follows the `dot-project` standard since 10.11.2026 (rules, single changelog entries, statuses, consumed handoffs and drafts deleted, `just artifacts` in `just check`). Left here: `BACKLOG.md` pointers with items in `.project/backlog/` for anything this plan leaves open.

## N. Exit

1. `just check`, `just race` and `just integration` pass with the documented `TMPDIR`.
2. The inventory is re-run at the fix commit, with the realistic-size cases added by this plan.
3. PLAN-0002 §5 items 2 and 6 are done: progress table with test links, handoff to pig-web naming the exit commit.
4. The review is deleted with `Artifact-Deleted` trailers once every finding is fixed or carried by a backlog item.

# 5. Verification

Each fix lands with a regression test that fails before it. Sizes are realistic: 50 or more sessions, peers and skills; 1000 frames; 30 finished jobs. Live behaviour is checked in an isolated PiG session under tmux; layout changes carry captures in this plan's evidence directory. A finding is closed only when its test or capture is linked from the Progress table.
