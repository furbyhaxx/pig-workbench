---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-inventory"
  title: "Slice 1 acceptance inventory"
  description: "PLAN-0001 Slice 1 ownership rows, observed representative tests and remaining acceptance gaps"
  status: "published"
  revision: 22
  created_at: "2026-11-05T16:40:00+01:00"
  updated_at: "2026-11-07T09:53:29+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Separate completed evidence from missing wiring, behavior and acceptance without narrowing scope"
      changes: ["Recorded per-row status with file and test anchors", "Named the ordered remaining work per phase", "Confirmed no row is satisfied by an unadvertised or stub path"]
    - revision: 2
      reason: "Reconcile the current implementation and retain observed representative acceptance evidence"
      changes: ["Enumerated every Slice 1 ownership row", "Recorded passing real-host cases without equating them to complete row acceptance", "Exposed missing scheme commands, provider search and private instruction discovery"]
    - revision: 3
      reason: "Retain the current baseline quality and cleanup checks"
      changes: ["Recorded green check, race and integration gates", "Recorded repeated shutdown, crash-reclamation and reference-producer reload checks", "Kept final acceptance and live-provider evaluation open"]
    - revision: 4
      reason: "Verify activity failure counting against real jobs"
      changes: ["Recorded a repeatable race-enabled test using successful and failed processes"]
    - revision: 5
      reason: "Exercise multi and preview questions through real native RPC dialogs"
      changes: ["Recorded selected values and preview display with a local scripted provider"]
    - revision: 6
      reason: "Exercise stored-session title confirmation"
      changes: ["Recorded actual PiG RPC confirmation, decline, acceptance and title reads"]
    - revision: 7
      reason: "Exercise agent-reply hop limit in two real sessions"
      changes: ["Recorded a real reply retaining its conversation and being queued at the hop limit"]
    - revision: 8
      reason: "Rerun baseline gates after adding independent acceptance cases"
      changes: ["Recorded successful check, race and integration on the updated tree"]
    - revision: 9
      reason: "User approved the remaining five specifications"
      changes: ["Removed the resolved approval blocker while retaining implementation and acceptance gaps"]
    - revision: 10
      reason: "Verify scheme redirects against the shared control write policy"
      changes: ["Recorded real-host self-title redirect and stored-title confirmation behavior", "Retained other scheme commands and general redirect syntax as open"]
    - revision: 11
      reason: "Exercise grep context and its output ceiling"
      changes: ["Recorded real-host context lines and output-budget truncation", "Retained long-line windows and provider-root searches as open"]
    - revision: 12
      reason: "Exercise bounded find over provider listings"
      changes: ["Recorded real-host pagination and recursive provider traversal", "Retained local scratch safety and provider grep as open"]
    - revision: 13
      reason: "Fix an outside-scratch symlink read"
      changes: ["Recorded a reproduced escape and confined descriptor-based read regression", "Kept scratch listing and stat containment open"]
    - revision: 14
      reason: "Confine scratch listing and stat"
      changes: ["Recorded root-confined stat and bounded listing tests", "Exercised real scratch provider traversal and symlink refusal"]
    - revision: 15
      reason: "Add confined project-private note discovery"
      changes: ["Recorded project isolation, symlink and source-budget tests", "Kept host delivery and pin wiring open"]
    - revision: 16
      reason: "Integrate trusted project-private instruction delivery"
      changes: ["Recorded live trust, isolation, pin, update and withdrawal cases", "Retained truncated-steering mutation safety as open work"]
    - revision: 17
      reason: "Show bounded windows for long grep matches"
      changes: ["Recorded repeatable package and real-host long-line window tests", "Kept oversized-context omissions explicit"]
    - revision: 18
      reason: "Enforce the truncated-instruction mutation prerequisite"
      changes: ["Recorded real-host repeated refusal, partial reads and recovery", "Verified that a different source revision is not authorized"]
    - revision: 19
      reason: "Recognize static quoted and escaped shell words"
      changes: ["Recorded parser and real-host composition-refusal tests", "Kept dynamic shell path recognition and provider mutations explicit"]
    - revision: 20
      reason: "Exercise grep on provider directory roots"
      changes: ["Recorded paginated traversal and source-revision tests", "Kept explicit provider-file targets and selectors open"]
    - revision: 21
      reason: "Verify instruction prerequisites for tagged batch targets"
      changes: ["Recorded multi-file and move-destination tests with no pre-prerequisite mutations"]
    - revision: 22
      reason: "Search provider files with line selectors"
      changes: ["Recorded real-host explicit file and source-coordinate selection tests"]
---

# Scope and evidence

This table follows [PLAN-0001 §9](../../plans/0001-pig-workbench.md), including the Slice 1 portions of split rows. A passing test proves only the behavior named in its cell, **not the full ownership row**. A dash means no representative execution was established in this run. No row is claimed frozen or fully accepted by this inventory.

The following subset ran against a real PiG host with an isolated local scripted provider, without billed inference; its output is [acceptance-subset.txt](evidence/acceptance-subset.txt). All test names in the table refer to `internal/host/` integration tests and appear in that output unless marked otherwise. Reproduce with `just _modfile`, then `go test -race -tags=integration -modfile=.tmp/go.mod ./internal/host -run '<test-name>' -count=1 -v`. The historical pre-fix audit is [source-audit.txt](evidence/source-audit.txt); current implementation and limitations are identified in the table.

| PLAN-0001 §9 row | Owner | Observed representative behavior | Remaining acceptance or known gap |
|---|---|---|---|
| 0001 §2–§7 | S1 C | `TestLiveServiceDiscoveryRPC` discovers the service. | Full protocol failure, cancellation and receipt matrix needs reconciliation with the acceptance anchor. |
| 0001 §8 | S1 C + consumer | `TestLiveReferenceProducerSeams` exercises registered contributions and consumers. | Each collaboration method and conditional grant still needs an explicit acceptance mapping. |
| 0002 all | S1 F | `TestLiveResumeDoesNotResendDeliveredContextFrames` checks resume delivery. | Branch, compaction, trust and projection cases are not all established by that run. |
| 0003 §2–§6, §10 | S1 F, B | `TestLiveReferenceProducerSeams` exercises reference presentation. | Plain-state and width cases require complete row reconciliation. |
| 0004 §1–§10 | S1 D | `TestLiveReferenceProducerSeams` exercises a contributed provider and edit refusal. | Provider mutations are refused at registration. Full policy and mutation acceptance is not established. |
| 0004 §11 | S1 D | `TestLiveHTTPReadUsesBuiltinStaticReader` and `TestLiveReferenceProducerSeams` exercise builtin and contributed fetching. | Redirect and policy matrix needs reconciliation against the anchor. |
| 0005 §2–§8, §10, G5.2 | S1 G | `TestLiveTaggedAndExactEditUseObservedSource` exercises both edit forms. | This test does not prove every grammar, recovery and snapshot case. |
| 0006 §2–§4, §6–§7 | S1 D | Read, literal OR search and bounded context are exercised in [grep-context.log](evidence/grep-context.log); long-line windows are covered in [grep-long-lines.log](evidence/grep-long-lines.log) and [grep-long-lines-live.log](evidence/grep-long-lines-live.log). Provider and scratch finds are in [find-provider.log](evidence/find-provider.log) and [find-scratch.log](evidence/find-scratch.log); provider grep and source pagination are in [provider-grep-live.log](evidence/provider-grep-live.log) and [provider-grep.log](evidence/provider-grep.log). | Explicit provider-file selection is covered by [provider-grep-files.log](evidence/provider-grep-files.log); oversized requested context may still omit a match. Loop guard and supersession need implementation or evidence. |
| 0007 §2–§8 local | S1 K | `TestLiveNewSteeringIsAdmittedAndDeletedSteeringIsWithdrawn` exercises instruction admission and withdrawal; private note lifecycle is exercised in [private-integration.log](evidence/private-integration.log). `TestLiveTruncatedSteeringBlocksMutationsUntilFullyRead` exercises repeated refusal, partial reads and recovery ([instruction-barrier.log](evidence/instruction-barrier.log)); tagged batch sources and move destinations are covered in [tagged-instruction-union.log](evidence/tagged-instruction-union.log). | Remaining acceptance cases need reconciliation. |
| 0008 §2–§3 local | S1 E1 | `TestLiveReadLocationAndCodemode` exercises captured cwd and `cd`. | Full location acceptance requires reconciliation. |
| 0008 §5–§7, G8.20 | S1 H | `TestLiveBackgroundJobStdinAndCompletion` exercises job input and completion. Control redirects and confirmation parity are in [control-redirect.log](evidence/control-redirect.log); quoted-path routing and refusal of shell composition are in [scheme-parser-live.log](evidence/scheme-parser-live.log). | `rm`, `mv`, `cp` and `mkdir` are unconditionally refused for scheme paths. Redirects only accept one static word; full provider routing and general redirect syntax are not implemented. Dynamic shell-generated paths cannot always be recognized. |
| 0009 §2–§3, §5 | S1 L2 | `TestLiveAskUsesNativeRPCAndReturnsUserChoice` exercises an RPC choice; `TestLiveAskMultiAndPreviewThroughNativeDialogs` exercises multi-selection and preview text ([ask-multi-preview.log](evidence/ask-multi-preview.log)). | The rich TUI panel is deferred chrome; remaining mode and failure cases need reconciliation. |
| 0009 §8, Slice 1 routes | S1 M | `TestLiveEveryCompletedCommandIsAcceptedByItsHandler` exercises command handlers. | Reconcile the complete Slice 1 command and mode matrix; print has no command output channel. |
| 0009 §9, activity line | S1 M, F | `TestLiveActivityLineCountsMatchTheRegistries` compares shared activity items with live registries. | That RPC test did not observe a widget. `TestActivityCountsFailedJobsAfterRealExit` checks one successful and one failed real process ([activity-failed.log](evidence/activity-failed.log)); real TUI placement has separate coverage. Click requires a fork host; configured shortcut is not applied. |
| 0010 all | S1 B | `TestLiveSettingsScopesAndReload` exercises settings scopes and reload. | Many validated settings remain unavailable; their catalog entries identify them. Full storage, trust and lifecycle matrix is not established by this one test. |
| 0011 §2–§8 | S1 L1 | `TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes` exercises actual two-session wake; `TestLiveAgentReplyIncrementsHopAndStopsWake` exercises a real reply at hop one ([agent-reply-hop.log](evidence/agent-reply-hop.log)). | Windows messaging is compile-level only. |
| 0012 §3.1–§3.4 | S1 E1 | `TestLiveReadLocationAndCodemode` exercises location and project context in the host. | Full project recognition precedence needs separate acceptance evidence. |
| 0012 §3.5 | S1 E2 | `TestLiveSessionStorageIsSharedThenUnlinked` exercises conversion and undo. | The complete crash-step and conflict matrix needs reconciliation; Windows is compile-level only. |
| 0012 §3.6 | S1 K | Private-note discovery confines notes to an authorized root, applies user trust and budgets, and refuses symlink escapes ([private-discovery.log](evidence/private-discovery.log)); actual PiG sessions exercise trust, project isolation, pins, diffs and withdrawal ([private-integration.log](evidence/private-integration.log), [private-registry-trust.log](evidence/private-registry-trust.log)). | Full compaction and all cap interactions still need explicit acceptance mapping. |
| 0012 §5.1–§5.2, §5.5 | S1 D | `TestLiveReferenceProducerSeams` exercises registered scheme routing; `TestLiveFindWalksPaginatedProvider` and `TestLiveFindWalksScratchWithoutFollowingLinks` verify paginated and scratch traversal ([find-provider.log](evidence/find-provider.log), [find-scratch.log](evidence/find-scratch.log)). | First-touch guide and naming acceptance need separate mapping; explicit file grep is covered in [provider-grep-files.log](evidence/provider-grep-files.log). |
| 0012 §5.3 core | S1 D, H, J, L1 | `TestLiveStoredSessionsAreListedReadAndTitled` exercises stored session trees; `TestLiveStoredSessionTitleRequiresConfirmation` checks native RPC confirmation ([stored-title-confirmation.log](evidence/stored-title-confirmation.log)). Scratch read, stat and listing confinement are covered in [local-read-confinement.log](evidence/local-read-confinement.log), [live-local-read-confinement.log](evidence/live-local-read-confinement.log) and [scratch-list-safety.log](evidence/scratch-list-safety.log). | The confirmation was not visually tested in TUI; each core scheme needs separate evidence. |
| 0012 §5.4, registered docs | S1 C, D | `TestLiveReferenceProducerSeams` exercises contributed docs. | PiG-source docs are Slice 2; reconcile registered-doc operations individually. |
| 0013 §2–§5, §7 | S1 I | `TestLiveStoredSearchKeyIsRedactedInJobLogs` verifies a stored key is redacted in a job log. | Native custom credential resolver coverage is unavailable; all redaction/substitution boundaries need individual acceptance evidence. |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | S1 J | `TestLivePromptSettingsControlTakeover` verifies forced, disabled and auto behavior. | Auto retains PiG's prompt until user-authorized A/B evaluation; prompt takeover also depends on `instructions.enabled`. Skills and every frame require separate evidence. |

# Baseline gates

At commit `9da6882` on Linux, `just check` (including validation), `just race` and `just integration` passed. Their redacted output is [check.log](evidence/check.log), [race.log](evidence/race.log) and [integration.log](evidence/integration.log); `$REPO` replaces the local checkout path. The same gates passed again at `31ab95a` after the new behavioral cases: [check](evidence/check-updated.log), [race](evidence/race-updated.log), [integration](evidence/integration-updated.log). These are baseline checks, not the final gate after the blocked implementation work.

Three repeated race-enabled runs passed for job manager cancellation, watcher joins and lifecycle shutdown ([leaks-unit.log](evidence/leaks-unit.log)), and for live crash reclamation, session-end job termination and reference-producer reload ([leaks-integration.log](evidence/leaks-integration.log)). These targeted checks are not a comprehensive heap or goroutine leak proof. The real TUI job/scratch reload path is not repeated by these three cases.

# Acceptance blockers

- All fourteen specifications are approved; that approval does not close the remaining implementation or acceptance gaps.
- G8.20 provider mutations, general redirect syntax and oversized grep context are not complete. The task list retains them; no acceptance row above should be read as a waiver.
- Live-provider A/B and the end-to-end inference transcript require fresh authorization for spend. The evidence above uses a scripted local provider, not live inference.
- `agent://by-project`, broadcast and coordinator are Slice 2. Activity-item click requires the fork host. Windows messaging, session storage and registry are compile-level only.
