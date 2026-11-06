---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-inventory"
  title: "Slice 1 acceptance inventory"
  description: "PLAN-0001 Slice 1 ownership rows, observed representative tests and remaining acceptance gaps"
  status: "published"
  revision: 7
  created_at: "2026-11-05T16:40:00+01:00"
  updated_at: "2026-11-07T00:37:26+01:00"
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
---

# Scope and evidence

This table follows [PLAN-0001 §9](../../plans/0001-pig-workbench.md), including the Slice 1 portions of split rows. A passing test proves only the behavior named in its cell, **not the full ownership row**. A dash means no representative execution was established in this run. No row is claimed frozen or fully accepted by this inventory.

The following subset ran against a real PiG host with an isolated local scripted provider, without billed inference; its output is [acceptance-subset.txt](evidence/acceptance-subset.txt). All test names in the table refer to `internal/host/` integration tests and appear in that output unless marked otherwise. Reproduce with `just _modfile`, then `go test -race -tags=integration -modfile=.tmp/go.mod ./internal/host -run '<test-name>' -count=1 -v`. Source findings are supported by [source-audit.txt](evidence/source-audit.txt) and the named implementation files.

| PLAN-0001 §9 row | Owner | Observed representative behavior | Remaining acceptance or known gap |
|---|---|---|---|
| 0001 §2–§7 | S1 C | `TestLiveServiceDiscoveryRPC` discovers the service. | Full protocol failure, cancellation and receipt matrix needs reconciliation with the acceptance anchor. |
| 0001 §8 | S1 C + consumer | `TestLiveReferenceProducerSeams` exercises registered contributions and consumers. | Each collaboration method and conditional grant still needs an explicit acceptance mapping. |
| 0002 all | S1 F | `TestLiveResumeDoesNotResendDeliveredContextFrames` checks resume delivery. | Branch, compaction, trust and projection cases are not all established by that run. |
| 0003 §2–§6, §10 | S1 F, B | `TestLiveReferenceProducerSeams` exercises reference presentation. | SPEC-0003 is draft; plain-state and width cases require complete row reconciliation. |
| 0004 §1–§10 | S1 D | `TestLiveReferenceProducerSeams` exercises a contributed provider and edit refusal. | Provider mutations are refused at registration; grep/find do not route provider roots. Full policy and mutation acceptance is not established. |
| 0004 §11 | S1 D | `TestLiveHTTPReadUsesBuiltinStaticReader` and `TestLiveReferenceProducerSeams` exercise builtin and contributed fetching. | Redirect and policy matrix needs reconciliation against the anchor. |
| 0005 §2–§8, §10, G5.2 | S1 G | `TestLiveTaggedAndExactEditUseObservedSource` exercises both edit forms. | This test does not prove every grammar, recovery and snapshot case. |
| 0006 §2–§4, §6–§7 | S1 D | `TestLiveReadLocationAndCodemode` and `TestLiveGrepAcceptsLiteralPatternList` exercise local reads and literal OR search. | SPEC-0006 is draft. Grep context is accepted but ignored; grep/find treat provider URIs as local paths. Loop guard and supersession need explicit evidence. |
| 0007 §2–§8 local | S1 K | `TestLiveNewSteeringIsAdmittedAndDeletedSteeringIsWithdrawn` exercises instruction admission and withdrawal. | SPEC-0007 is draft; project-private instruction discovery is not in the default classes. Other acceptance cases need reconciliation. |
| 0008 §2–§3 local | S1 E1 | `TestLiveReadLocationAndCodemode` exercises captured cwd and `cd`. | SPEC-0008 is draft; full location acceptance requires reconciliation. |
| 0008 §5–§7, G8.20 | S1 H | `TestLiveBackgroundJobStdinAndCompletion` exercises job input and completion. | SPEC-0008 is draft. `rm`, `mv`, `cp` and `mkdir` are unconditionally refused for scheme paths; redirects only route to `job://`. Provider approval parity is unproven. |
| 0009 §2–§3, §5 | S1 L2 | `TestLiveAskUsesNativeRPCAndReturnsUserChoice` exercises an RPC choice; `TestLiveAskMultiAndPreviewThroughNativeDialogs` exercises multi-selection and preview text ([ask-multi-preview.log](evidence/ask-multi-preview.log)). | The rich TUI panel is deferred chrome; remaining mode and failure cases need reconciliation. |
| 0009 §8, Slice 1 routes | S1 M | `TestLiveEveryCompletedCommandIsAcceptedByItsHandler` exercises command handlers. | Reconcile the complete Slice 1 command and mode matrix; print has no command output channel. |
| 0009 §9, activity line | S1 M, F | `TestLiveActivityLineCountsMatchTheRegistries` compares shared activity items with live registries. | That RPC test did not observe a widget. `TestActivityCountsFailedJobsAfterRealExit` checks one successful and one failed real process ([activity-failed.log](evidence/activity-failed.log)); real TUI placement has separate coverage. Click requires a fork host; configured shortcut is not applied. |
| 0010 all | S1 B | `TestLiveSettingsScopesAndReload` exercises settings scopes and reload. | Many validated settings remain unavailable; their catalog entries identify them. Full storage, trust and lifecycle matrix is not established by this one test. |
| 0011 §2–§8 | S1 L1 | `TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes` exercises actual two-session wake; `TestLiveAgentReplyIncrementsHopAndStopsWake` exercises a real reply at hop one ([agent-reply-hop.log](evidence/agent-reply-hop.log)). | SPEC-0011 is draft; Windows messaging is compile-level only. |
| 0012 §3.1–§3.4 | S1 E1 | `TestLiveReadLocationAndCodemode` exercises location and project context in the host. | Full project recognition precedence needs separate acceptance evidence. |
| 0012 §3.5 | S1 E2 | `TestLiveSessionStorageIsSharedThenUnlinked` exercises conversion and undo. | The complete crash-step and conflict matrix needs reconciliation; Windows is compile-level only. |
| 0012 §3.6 | S1 K | — | Private notes class is not present in `instructions.DefaultClasses`, which the host uses; no live test proves it. |
| 0012 §5.1–§5.2, §5.5 | S1 D | `TestLiveReferenceProducerSeams` exercises registered scheme routing. | Provider-root search/walking is not wired; first-touch guide and naming acceptance need separate mapping. |
| 0012 §5.3 core | S1 D, H, J, L1 | `TestLiveStoredSessionsAreListedReadAndTitled` exercises stored session trees; `TestLiveStoredSessionTitleRequiresConfirmation` checks decline and acceptance in native RPC dialogs ([stored-title-confirmation.log](evidence/stored-title-confirmation.log)). | The confirmation was not visually tested in TUI; each core scheme needs separate evidence. |
| 0012 §5.4, registered docs | S1 C, D | `TestLiveReferenceProducerSeams` exercises contributed docs. | PiG-source docs are Slice 2; reconcile registered-doc operations individually. |
| 0013 §2–§5, §7 | S1 I | `TestLiveStoredSearchKeyIsRedactedInJobLogs` verifies a stored key is redacted in a job log. | Native custom credential resolver coverage is unavailable; all redaction/substitution boundaries need individual acceptance evidence. |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | S1 J | `TestLivePromptSettingsControlTakeover` verifies forced, disabled and auto behavior. | Auto retains PiG's prompt until user-authorized A/B evaluation; prompt takeover also depends on `instructions.enabled`. Skills and every frame require separate evidence. |

# Baseline gates

At commit `9da6882` on Linux, `just check` (including validation), `just race` and `just integration` passed. Their redacted output is [check.log](evidence/check.log), [race.log](evidence/race.log) and [integration.log](evidence/integration.log); `$REPO` replaces the local checkout path. These are baseline checks, not the final gate after the blocked implementation work.

Three repeated race-enabled runs passed for job manager cancellation, watcher joins and lifecycle shutdown ([leaks-unit.log](evidence/leaks-unit.log)), and for live crash reclamation, session-end job termination and reference-producer reload ([leaks-integration.log](evidence/leaks-integration.log)). These targeted checks are not a comprehensive heap or goroutine leak proof. The real TUI job/scratch reload path is not repeated by these three cases.

# Acceptance blockers

- The five draft specifications SPEC-0003, SPEC-0006, SPEC-0007, SPEC-0008 and SPEC-0011 await user approval. Slice 1 cannot be called frozen before that decision.
- G8.20 dispatch, provider-root grep/find, grep context and project-private instruction discovery are not complete. The task list retains them; no acceptance row above should be read as a waiver.
- Live-provider A/B and the end-to-end inference transcript require fresh authorization for spend. The evidence above uses a scripted local provider, not live inference.
- `agent://by-project`, broadcast and coordinator are Slice 2. Activity-item click requires the fork host. Windows messaging, session storage and registry are compile-level only.
