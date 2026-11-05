---
document:
  type: "handoff"
  id: "HO-slice-1-resume-2026-11-05"
  title: "Resume full Slice 1 implementation"
  description: "Exact checkpoint, unfinished integration, retained worktrees, failing check and next actions"
  status: "published"
  revision: 1
  created_at: "2026-11-05T12:04:45+01:00"
  updated_at: "2026-11-05T12:04:45+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "User requested a handover to continue implementation in a fresh session"
      changes: ["Recorded incomplete Slice 1 state, preserved worktrees and dirty integration", "Recorded reproduced failure and concrete continuation sequence"]
---

# Goal and authority

Finish **all of Slice 1**, following `.project/plans/0001-pig-workbench.md` §9, `.project/plans/0002-slice-1-foundation.md` phases A–M and `.project/specs/0001`–`0014`. Isolated engines and green tests are not slice completion. The user repeatedly rejected stopping at intermediate commits and has authorized implementation and required live verification. This turn stops solely because the user explicitly requested this handover.

- Repository: `/mnt/Projects/furbyhaxx/pig-workbench`.
- Integrating branch: `dev`.
- Implementation HEAD before this handoff commit: `e24df61` (`fix: bind project roots before location activation`).
- Project task tree: `P1` and children; external co-load dependency `P2`. Task states lag actual integration in several completed engine units. Inspect the tree; do not equate in-progress labels with active writers.
- All 63 agent runs listed by the harness are completed or failed; **no agents are currently writing**. No background jobs are running at handoff inspection.
- There are substantial intentional **uncommitted implementation changes** on dev and completed, unmerged worktrees. Do not discard, overwrite or blindly stage them.

# Immediate next action: one known failing test

The prompt loader default was corrected to the specification's **256 KiB**, but an existing test still hardcodes its old 1 MiB expectation.

`internal/prompt/filesystem_test.go:222–223` currently checks `exceeds 1048576 bytes` and says `want 1 MiB limit error`. Production `filesystem.go` now enforces `262144`. Update that assertion to the approved 256 KiB contract; do not restore the old production limit just to make it green. Test input is already `defaultMaxFileBytes+1`.

Reproduced during this handoff:

```text
go test -race -modfile=.tmp/go.mod ./internal/host ./internal/location ./internal/hashline ./internal/resource ./internal/prompt
host       PASS 1.410s
location   PASS 1.032s
hashline   PASS 1.142s
resource   PASS 1.031s
prompt     FAIL TestFileSystemLoaderBoundsInput
filesystem_test.go:223: default limit: got ... exceeds 262144 bytes; want 1 MiB limit error
```

The prior `&&`-chained integration run did **not** execute after this failure. Do not claim current integration/full gates passed. Older live test passes below describe prior checkpoints, not the final dirty tree.

# Current dev implementation

## Landed and wired, with limitations

- Go factory and session admission/shutdown supervision.
- Layered scalar `Workbench.toml` configuration, provenance, project restrictions, stale-state admission refusal, round-trip editing, cross-process locking and saved-versus-published outcomes. Many catalog settings still have no consumer; complex configuration remains incomplete.
- Local `read`, `write`, `ls`, `find`, `grep`, `cd`; source selectors, paging, syntax summaries, source tags, snapshot store and epoch-bound seen coverage.
- Resource routing includes local/file/scratch/config/session/artifact trees; inspect `internal/host/router.go` for actual routes. Do not advertise all specified schemes as complete.
- Native-dialog `ask_user` (single/multi/preview/freeform/secret), schema/execution/recovery primitives and encrypted secret-answer storage. Rich TUI panel and pending-ask resume wiring remain incomplete.
- Local `bash` and unified `jobs`, background ownership, stdin/EOF, foreground waits/promotion, deadlines, bounded output, terminal-state retention. No watchdog, complete job resource tree or shell scheme-command routing yet.
- Linux process lifecycle fixed through stable pidfd identity, independent exit observation and serialized signal/reap. Process failure, descriptor reuse and concurrent Wait/Close races have regression tests. Do not replace this with numeric PGID fallbacks.
- Frame ledger/delivery transitions/envelope codec; six context facts and applicable instructions enter host/model context. Instruction discovery runs at prompt/tool boundaries; duplicate host context-file injection is suppressed. Full watching/diffs/pins/multi-touch lifecycle remains incomplete.
- Shared protocol client and location service. Generic contribution registry is implemented but not fully wired to public kind-specific services.
- Project document, registry and read-only detection; project-root binding engine now landed, with host call-site migration still dirty.
- Prompt Jinja engine, confined loader, strict `attr`, strict/cancellable loop adapter and joined range producers. Sections/helpers/builtin template/takeover/skills remain incomplete.
- Vault/key/detection primitives; **general provider/transcript/tool secret boundary is NOT wired**. Secret-answer storage alone does not prove provider safety.

## Recent useful dev commits

- `e24df61`: transactional location project roots.
- `465d214`: strict/cancellable prompt loops (cherry-picked worktree `746e991`).
- `b8e6a55`: contribution registry.
- `8ece156`: local file commit executor.
- `74c20b4`: hashline batch/register preflight.
- `5559368`: preserve secret metadata, private XDG vault path and isolated test data roots.
- `e145ac5`: context/steering frame delivery.
- `e731958`: detection metadata preservation.

Use git history for older landed engines. Plans/README/package guidance are partially stale and are not stronger evidence than actual host wiring and tests.

# Dirty dev integration: preserve and finish

Tracked modified files at handoff:

```text
README.md
internal/host/app.go
internal/host/commands_test.go
internal/host/frame_context.go
internal/host/instruction_context.go
internal/host/replay.go
internal/host/services_test.go
internal/host/snapshots.go
internal/host/snapshots_test.go
internal/host/write.go
internal/prompt/AGENTS.md
internal/prompt/filesystem.go
internal/resource/page.go
internal/resource/page_test.go
```

New implementation files:

```text
internal/hashline/copied.go
internal/hashline/copied_test.go
internal/host/edit.go
internal/host/live_edit_test.go
internal/location/restore_cancellation_test.go
```

What these changes do:

1. `edit.go` wires tagged batch edits, syntax block resolution, exact-text mode, seen checks, registers and `fileedit.Commit` outcomes. Edit mode is captured per turn; no silent fallback. Stale relocation, persistent registers, visibility remapping and full post-edit state handling are unfinished.
2. `copied.go` recognizes complete canonical tagged read copies. `write.go` binds those copies back to a retained exact-byte snapshot so BOM/endings survive; raw=true bypasses interpretation. Existing-file writes use the shared guarded commit path. New-file creation still needs complete race-policy review.
3. `snapshots.go` grants visibility only when **one intact text block** survives the result hook. Separate truncated blocks cannot concatenate into fake completeness. Failed batches may still contain a valid delivered block. Successful writes retain full supplied source visibility but do not grant a tagged handle until a tagged view is delivered.
4. `resource/page.go` hides BOM in tagged logical rows, preserves raw bytes, and includes header lines in the budget. Copied-view and BOM regression tests are present.
5. `app.go`, `replay.go` and tests migrate to `location.New(ctx, origin, lookup)` / `RestoreContext`; host lookup uses `project.Registry.Detect`.
6. `frame_context.go` / `instruction_context.go` tighten authority, stable fact fields, intact envelope detection, nextBoundary delivery, and host-project trust anchoring. A `cd` cannot borrow the original project's trust for another project. Missing/nested project authorization falls back conservatively to user instructions.
7. `restore_cancellation_test.go` ensures cancellation after project lookup cannot activate restored state.
8. Prompt template maximum changed to 256 KiB; its stale test assertion is the immediate failure above.

Unrelated/unattributed untracked paths: `.pi/` and `.project/analyses/2026-11-04-pig-upstream-candidates/`. They were present on handoff inspection. Preserve; do not add them to implementation commits without inspecting ownership/content. In particular `.pi/` was already present earlier.

# Completed worktrees not integrated: do not lose them

All below are based on `b8e6a55`, hold **uncommitted** completed work and have no active writer. Review actual diffs, run narrow verification, commit explicit owned paths in that worktree and cherry-pick onto dev. Shared AGENTS.md/batch files may conflict; integrate deliberately.

| Worktree / branch | Files and purpose | Integration concerns |
|---|---|---|
| `.worktrees/staged-line-endings` / `fix/staged-line-endings` | hashline stage/snapshot/batch and tests; `StageSource`, `StagedSource.Origins`, `RestoreStaged` | Essential before claiming mixed-ending edits: provenance distinguishes repeated identical original lines. Preserve newer dev batch ordering fixes; expose origins for seen remapping. |
| `.worktrees/hashline-relocation` / `feat/hashline-relocation` | `relocate.go`, `relocate_test.go`; `RelocateOps` | Unique original/current anchor windows, line-boundary KMP, ambiguity refusal. Still needs host/current-source/seen-state integration; primitive alone does not satisfy G5.2. |
| `.worktrees/secret-payload-guards` / `feat/secret-payload-guards` | `guard.go`, `guard_test.go` | Atomic typed payload walkers for chat/responses/Anthropic/Gemini. Some shapes derived from host structs, not real captures. Unknown metadata/opaque thinking and multiline presentation secrets require review. Wire fail-closed hooks and capture actual payloads. |
| `.worktrees/html-reader` / `feat/html-reader` | `internal/htmlread/` | Static HTML semantic-main/readability/redirect adapter. Dependencies exist only in its disposable modfile; root manifests, notices and http resource wiring remain. |
| `.worktrees/presentation-snapshots` / `feat/presentation-snapshots` | `internal/presentation/` | Bounded typed snapshot decoder. Some underspecified shapes were chosen by the worker; reconcile against SPEC-0003 before publishing. No renderer/resolver yet. |

Other worktrees mostly contain earlier integrated commits. Do not reapply them wholesale. Particularly `prompt-loop-evaluation` worktree HEAD `746e991` was cherry-picked as dev `465d214`; `location-project-root` `093d564` became `e24df61`. Compare history/diffs rather than assuming equal hashes.

HTML dependency checks already performed on 03.11.2026 (recheck if adding now): `github.com/JohannesKaufmann/html-to-markdown/v2 v2.5.2`, `codeberg.org/readeck/go-readability/v2 v2.1.3`, `golang.org/x/net v0.62.0`; transitive x/text becomes v0.43.0. Module latest/upstream maintenance checks were run; root go.mod/go.sum were not updated for these yet. Add license notices before integration.

# Known pitfalls and unresolved requirements

- **Credential embedding bug:** detector can learn a larger credential containing canonical placeholder text, but `placeholder.go` protects existing placeholder substrings first and can fail to replace the larger secret. This was reported by the worker and is NOT fixed. Reproduce and fix fail-closed before provider-boundary claims.
- Multiline credentials read through line-numbered presentation must be learned from original bytes; learning contaminated presentation text cannot reconstruct the real key for substitution.
- Generic registry claims and decoder tests are not public registration/fetch/presentation/status/context-field service integration. Discovery currently advertises only wired capabilities (inspect services.go).
- Config catalog has guessed/restrictive choices from early work; audit each active key against its owner spec instead of treating the catalog as ratified architecture.
- Typed snapshots/tag selection do not alone prove full hashline recovery, G5.2 remapping, register persistence or post-compaction behavior.
- Frame delivery acknowledgement proves outgoing-context inclusion, not remote provider receipt. Session ledger/body and historic-record capacity remain explicit limits; preserve honest refusal.
- The prompt engine cannot preempt an arbitrary single expression or blocked custom loader. Joined owned range producers are verified; do not claim arbitrary code preemption.
- Linux group safety uses atomically acquired pidfd/group signalling and refuses unsupported capability before user code. Darwin/Windows runtime parity remains unverified; some registries fail closed on Windows without ACL support.
- Vault data is XDG/platform data (`secrets.VaultPath`); key remains `<agentDir>/secret.key`. Fixtures must set isolated XDG_DATA_HOME/LOCALAPPDATA as well as PiG agent/home variables. Never touch real credentials.

# Remaining full-slice work

Use PLAN-0002 for the complete acceptance denominator; the following is orientation, not a reduced scope:

- A: formal whole-contract ledger/freeze, remaining host probes, pig-web reply.
- B: remaining complex config/trust/storage/host presentation foundations and reload cleanup.
- C/F: real registration descriptors and every consumer, all service capabilities, external providers/fetch/status/docs, resolver/plain renderers/codemode integration, contributed model fields.
- D/E: HTML+http fetch safety and all core schemes/adapters, provider capabilities/walking/first-touch guides; project context and session bucket adoption (journal/recovery/undo; requires live-session checks).
- G: integrate dirty public edit/write work and retained worktrees, current-source recovery and seen remapping, persistent registers/snapshots service and donor acceptance.
- H: watchdog/crash ownership, job:// resources, commands/notifications, G8.20 URI shell dispatch, secret env injection and full lifecycle matrix.
- I: Secrets.toml, credential inventories, entry-time/context/provider redaction, abort-on-failure, tool substitution including other extensions, appended blocks and captured payloads.
- J: builtin/workbench template, sections/helpers, model-stable prompt epochs/takeover/splice/fallback, skill catalog/progressive loading/conditions/recovery and authorized A/B evaluation.
- K: instruction pins/diffs/watchers/multi-touch union/branch and compaction reconstruction, commands and pointer-target validation.
- L: real authenticated two-session messaging, loop/wake controls/resources/commands; rich ask TUI, review/dirty dismissal, pending recovery and mode acceptance.
- M: command/completion inventory, activity line and contributions; user-reviewed captures at required widths.
- Final: all acceptance rows/tests/evidence/docs, full gates and real agent workflows. Do not silently replace requirements with unsupported stubs.

# External blocker

`P2` blocks `P1g1`: the real `../pig-web` co-load gate. On 03.11.2026, `../pig-web` contained only `.env`, git files, `.project/` and AGENTS.md: no extension.go/go.mod/implementation. **Recheck** before asserting it remains blocked. Implementing pig-web is outside this repository's authorization. Cooperative fixtures help Workbench integration but do not satisfy the real co-load exit. Finish every independent Workbench requirement; report this dependency honestly if still absent.

# Decisions and operating rules

- Standing dependency approval: check latest release and supersession/maintenance first, record licenses and evidence; do not ask repeatedly to add dependencies.
- Approved extension inference: opt-in `secrets.redact` with explicit trusted-owner grants and shared-bus confidentiality limits. Research children without Workbench are not automatically protected.
- Approved model fields: owner-scoped `frameProducer.contextFields` and `context.fields.set`, model-bound with no core fact overwrite.
- Current+next detail only; broader roadmap stays high-level. Do not turn the handoff into another months-long plan instead of continuing implementation.
- Use isolated worktrees for concurrent writers. Never commit while that unit is still writing. Shared-index mistakes occurred earlier; use explicit-path `git commit --only`, no bare git commit, no broad git add.
- `just _modfile` prepares staged SDK resolution. Use `go ... -modfile=.tmp/go.mod`, not bare builds. Verify SDK via `pig reload --sdk-path`.
- Run live PiG integration only against a quiet source tree; the packer hashes the whole module and races any writer. A build-input-change error is not a product regression.
- Keep every active task open until its own integration/acceptance is complete; preserve external blockers rather than claiming completion from subagent summaries.

# Next slice of execution

1. Fix the stale 256 KiB test assertion; rerun prompt and dirty host/domain checks, then live edit/read/ask/jobs integration. Inspect dirty code and commit coherent explicit-path chunks without unrelated paths.
2. Integrate staged-ending provenance, then relocation and visibility remapping through the public edit workflow. Test mixed endings/repeated lines, stale current source, ambiguous anchors and no accidental seen grants.
3. Integrate payload guards and repair known detector replacement gaps before enabling the general secret boundary; validate captured provider payloads and tool substitutions across read/bash/job/third-party tool cases.
4. Continue the remaining PLAN-0002 phases using bounded, disjoint units while the integrating agent owns shared contracts/manifests/host files. Do not stop at one finished engine or green intermediate gate.

# Evidence and verification navigation

- `internal/host/live_edit_test.go`: real PiG read → tagged edit → exact edit → raw read → copied tagged write; source BOM/CRLF and mode preservation. Passed at an earlier dirty checkpoint, not rerun successfully after the latest failing prompt gate.
- `internal/host/live_test.go`: read, cd/reset, codemode children, configured limits and steering/context delivery.
- `internal/host/live_ask_test.go`: native RPC choice reaching the model.
- `internal/host/live_jobs_test.go`: background cat → stdin write → EOF → wait.
- `live_services_test.go`, `live_settings_test.go`: protocol/session replacement/resume/scoped settings.
- Earlier full check/integration success predates later integration changes. Current full gates remain open.
- Handoff-inspection race output: `host:/home/furbyhaxx/.local/state/pi/workbench/jobs/2218735-31f573c3/j2.log`.
- Temporary read-only exact PiG source checkout: `host:/tmp/wb-pig-source.yqlhGz/repo`, host commit `526c86df885eba5e214355e7f70205e06d59bfa2`, sparse `internal/codingagent/prompts/`. It may disappear; do not edit donor source. Host prompt takeover must preserve exact PiG sections, not invent undocumented host behavior.
