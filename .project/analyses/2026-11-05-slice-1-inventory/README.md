---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-inventory"
  title: "Slice 1 acceptance inventory"
  description: "PLAN-0001 Slice 1 ownership rows, observed representative tests and remaining acceptance gaps"
  status: "published"
  revision: 27
  created_at: "2026-11-05T16:40:00+01:00"
  updated_at: "2026-11-07T16:26:32+01:00"
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
    - revision: 23
      reason: "Preserve matches when grep context exceeds its output budget"
      changes: ["Recorded real-host bounded context reduction without hiding matching lines"]
    - revision: 24
      reason: "Rerun full gates after approved-contract implementation"
      changes: ["Recorded successful check, race and integration at 9126aff", "Retained the initial stale refusal-text assertion failure and its correction"]
    - revision: 25
      reason: "Wire provider mutations without new permission prompts"
      changes: ["Recorded real-host write, mkdir, remove, move, copy and redirect execution", "Verified stale refusal, provider denial, partial and unknown outcomes"]
    - revision: 26
      reason: "Exercise the capable-provider verb and failure matrix"
      changes: ["Verified ls/cat and every declared mutation verb", "Checked missing parents, nonempty directories, cross-provider refusal and malformed receipts"]
    - revision: 27
      reason: "Compile all Slice 1 ownership rows against current observed tests"
      changes: ["Linked passing unit and live cases for all 24 ownership rows", "Retained remaining unproved behavior instead of treating a table as acceptance"]
---

# Scope and evidence

This table follows all 24 [PLAN-0001 §9](../../plans/0001-pig-workbench.md) Slice 1 ownership rows, including Slice 1 portions of split rows. A passing test proves the named behavior, **not every sentence of its ownership row**. Limits remain explicit; this table does not declare Slice 1 complete.

At `eb55bf7`, fresh race-enabled full unit and host integration runs passed. U means a case in [final-unit.log](evidence/final-unit.log); L means an actual PiG session driven by a local scripted provider in [final-live.log](evidence/final-live.log), not billed inference. Every listed case was checked against the observed PASS records. Run `just _modfile`, then `go test -race -modfile=.tmp/go.mod ./... -count=1 -v` and `go test -race -tags=integration -modfile=.tmp/go.mod ./internal/host -count=1 -v`. The unit log identifies each package; live cases are in `internal/host/`. Synthetic credential-header values are redacted from the live log. Earlier targeted evidence is retained beside these runs.

| PLAN-0001 §9 row | Owner | Observed passing cases | Evidence scope and remaining limits |
|---|---|---|---|
| 0001 §2–§7 | S1 C | L: `TestLiveServiceFailuresRPC`, `TestLiveServiceDiscoveryRPC`, `TestLiveReferenceProducerReload`; U: `TestClientDiscardsLateReplies`, `TestCancellationIsOwnerScoped`. | Discovery, failure/correlation and reload paths; cooperative bus cannot preempt a hung listener. |
| 0001 §8 | S1 C + consumer | L: `TestLiveReferenceProducerSeams`, `TestLiveProviderMutationsWithoutConfirmation`, `TestLiveSnapshotsViewLetsAnotherToolsOutputBeEditedWithoutARead`, `TestLiveExtensionChannelReachesTheOtherSessionsExtensionWithoutWakingIt`; U: `TestSecretsRedactIsGrantedPerOwnerAndNeverEchoesInputOnFailure`. | Live consumers, snapshots, session channel and owner grants. |
| 0002 all | S1 F | L: `TestLiveResumeDoesNotResendDeliveredContextFrames`, `TestLiveCompactionResendsContextFactsOnce`, `TestLiveReferenceProducerFrameRules`; U: `TestOnlyHostGrantCanElevateFrameAuthority`, `TestFrameBaselineOmitsFramesFromAnotherBranch`. | Resume, compaction, branch and authority boundaries. |
| 0003 §2–§6, §10 | S1 F, B | L: `TestLiveReferenceProducerSeams`; U: `TestWebSearchRendersTypedURICellsAtEveryWidth`, `TestPresentationPlainRowPerLifecycleState`, `TestNoticesFitAndStripUntrustedControls`. | Typed URI cells, plain lifecycle states, terminal safety and width fitting. Rich chrome is Slice 3. |
| 0004 §1–§10 | S1 D | L: `TestLiveReferenceProducerSeams`, `TestLiveProviderMutationsWithoutConfirmation`, `TestLiveReadLocationAndCodemode`; U: `TestWalkRejectsEscapesAndRepeatedCursors`, `TestMutationReceiptCannotInventCommitOrHidePartialEffects`. | Provider routing, selectors, edit refusal, mutations, walking and receipt integrity. Provider edit stays unadvertised. |
| 0004 §11 | S1 D | L: `TestLiveHTTPReadUsesBuiltinStaticReader`, `TestLiveReferenceProducerSeams`; U: `TestHTTPProviderTracksRedirectsAndRejectsCycles`, `TestHTTPProviderLimitsDecodesAndRefusesUnsupportedContent`. | Builtin/contributed fetching, redirects and byte/media limits. |
| 0005 §2–§8, §10, G5.2 | S1 G | L: `TestLiveTaggedAndExactEditUseObservedSource`, `TestLiveEditAnOlderTagRecoversAgainstTheNewestSnapshot`, `TestLiveBlockCutPastesIntoAnotherFileThroughARegister`, `TestLiveEditRefusesLinesTheModelNeverSaw`; U: `TestParseAllCanonicalOperations`, `TestPlanBatchPreservesBOMAndEndings`, `TestCommitReplacementThroughSymlinkPreservesLinkAndMode`. | Grammar, current-source recovery, blocks/registers, seen guards and byte-safe commits. |
| 0006 §2–§4, §6–§7 | S1 D | L: `TestLiveReadLocationAndCodemode`, `TestLiveGrepWalksPaginatedProvider`, `TestLiveGrepExplicitProviderFileAndLineSelection`, `TestLiveFindWalksPaginatedProvider`, `TestLiveGrepContextHonoursOutputBudget`; U: `TestGrepProviderPagesAndHonoursBoundaries`. | Direct/codemode read and bounded local/provider searches. Loop guard and supersession behavior have not been established by these cases. |
| 0007 §2–§8 local | S1 K | L: `TestLiveChangedInstructionsArriveAsADiffAgainstTheDeliveredView`, `TestLiveNewSteeringIsAdmittedAndDeletedSteeringIsWithdrawn`, `TestLiveInstructionChangedAcrossCompactionArrivesAsOneFullView`, `TestLiveTaggedBatchRequiresEveryTargetsInstructions`, `TestLiveTaggedMoveRequiresDestinationInstructions`; U: `TestRootNearestAndSymlinkDirectoryBoundaries`. | Diffs, pins/discovery, compaction, target unions and mutation prerequisites; polling occurs at boundaries, not native file watching. |
| 0008 §2–§3 local | S1 E1 | L: `TestLiveReadLocationAndCodemode`; U: `TestMutationLeaseAndTransitionTimeout`, `TestTransitionPersistsBeforeActivation`, `TestSymlinkedLocationHasTheTargetIdentity`. | Captured cwd, mutation leases, persisted transitions and symlink identity. |
| 0008 §5–§7, G8.20 | S1 H | L: `TestLiveBackgroundJobStdinAndCompletion`, `TestLiveCrashReclaimsOrphanedJob`, `TestLiveSessionEndTerminatesJobs`, `TestLiveProviderMutationsWithoutConfirmation`, `TestLiveSchemeCommandRefusesQuotedComposition`; U: `TestCloseJoinsWatchers`. | Jobs and all provider verbs without a new prompt layer. Redirects accept one static word, not general shell execution; dynamic paths cannot always be detected. |
| 0009 §2–§3, §5 | S1 L2 | L: `TestLiveAskUsesNativeRPCAndReturnsUserChoice`, `TestLiveAskFreeformAndSecretReturnTheResultShape`, `TestLiveAskMultiAndPreviewThroughNativeDialogs`, `TestLiveUnansweredAskIsPendingAfterRestartAndNeverAnsweredByDefault`, `TestLiveAskDialogTimeoutReportsTimedOutNeverADefaultAnswer`. | Native RPC question types, pending state and timeout. Rich TUI questionnaire chrome remains Slice 3. |
| 0009 §8, Slice 1 routes | S1 M | L: `TestLiveEveryCompletedCommandIsAcceptedByItsHandler`, `TestLiveIntegrationsAndResourcesCommandsListTheContributions`, `TestLiveFramesAndInstructionRulesCommands`; U: `TestCompletionOffersLiveOwnersAndSchemesAfterTheirVerb`. | Handler and completion paths. Print mode has no slash-command output channel. |
| 0009 §9, activity line | S1 M, F | L: `TestLiveActivitySettingsControlWidget`, `TestLiveActivityLineCountsMatchTheRegistries`; U: `TestActivityCountsFailedJobsAfterRealExit`, `TestActivityLineDropsLowestPriorityFirstAndNeverCutsAnItem`. | Counts include real failures; widget captures have separate historical evidence. Click needs the fork host; activity.shortcut is not applied. |
| 0010 all | S1 B | L: `TestLiveSettingsScopesAndReload`, `TestLiveCatalogSettingsHaveBehavioralConsumers`; U: `TestProjectCannotWidenPolicyOrSetTrust`, `TestRoundTripUnknownFieldsAndExplicitValues`, `TestSymlinkSaveAndTrustRevocation`, `TestManagerCancelCloseConcurrent`. | Scopes, trust, safe saves, consumers and cleanup. Validated but unavailable catalog entries remain explicitly unavailable. |
| 0011 §2–§8 | S1 L1 | L: `TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes`, `TestLiveBusyReceiverIsSteeredNotWoken`, `TestLiveAgentReplyIncrementsHopAndStopsWake`, `TestLiveSecretTypedByTheSenderNeverReachesTheReceiversProvider`. | Actual two-session wake, steer, hop limit and redaction. Windows messaging is compile-level only. |
| 0012 §3.1–§3.4 | S1 E1 | L: `TestLiveReadLocationAndCodemode`; U: `TestProjectIdentityUsesTheParentProject`, `TestDetectRootCommitAcrossClones`, `TestDetectLongestLocalLocation`, `TestRegistryCrossProcessRegistration`. | Project identity, recognition precedence and concurrent registry persistence. |
| 0012 §3.5 | S1 E2 | L: `TestLiveSessionStorageIsSharedThenUnlinked`, `TestLiveAnotherLiveSessionInTheDirectoryDefersConversion`, `TestLiveTwoCheckoutsShareSessionsAndTheCatalogListsEachOnce`; U: `TestARealProcessDeathAtEachStepIsRecovered`, `TestConvertDefersAcrossFilesystems`. | Conversion, undo, live conflicts, deduplication and process-death recovery. Windows execution is unverified. |
| 0012 §3.6 | S1 K | L: `TestLivePrivateNotesUpdateAndPinnedNotesWithdrawOnProjectChange`, `TestLiveProjectPrivateNotesRespectTrustAndProjectIdentity`; U: `TestDiscoverProjectPrivateNotesAreConfinedAndCapped`, `TestPrivateNotesRootRejectsUnsafeAndOtherProjectDirectories`. | Trusted project-private notes, project isolation, pins, diffs, withdrawal and confinement. |
| 0012 §5.1–§5.2, §5.5 | S1 D | L: `TestLiveReferenceProducerSeams`, `TestLiveFindWalksScratchWithoutFollowingLinks`, `TestLiveGrepWalksPaginatedProvider`; U: `TestFirstTouchGuideIsInjectedOncePerRevision`, `TestWalkRejectsEscapesAndRepeatedCursors`. | Scheme/provider routing, first-touch guides and bounded walking. |
| 0012 §5.3 core | S1 D, H, J, L1 | L: `TestLiveStoredSessionsAreListedReadAndTitled`, `TestLiveStoredSessionTitleRequiresConfirmation`, `TestLiveWaitingReceiverGetsTheMessageAsTheReadResult`, `TestLiveBackgroundJobStdinAndCompletion`; U: `TestSkillReadLoadsAndUnloadDeclines`, `TestCfgProviderExposesSettings`, `TestArtifactProviderResolvesProjectTree`. | Core trees and native RPC title confirmation. This existing builtin title policy is unchanged; contributed mutations add no confirmation layer. |
| 0012 §5.4, registered docs | S1 C, D | L: `TestLiveReferenceProducerSeams`. | Registered documentation and scheme guides. PiG-source docs remain Slice 2. |
| 0013 §2–§5, §7 | S1 I | L: `TestLiveProviderRequestsNeverCarryAKnownSecret`, `TestLiveBashReceivesAnInjectedEnvSecretAndItsOutputStaysRedacted`, `TestLiveStoredSearchKeyIsRedactedInJobLogs`; U: `TestRedactContextCleansTheSystemMessageAndOtherMessages`, `TestSecretsRedactIsGrantedPerOwnerAndNeverEchoesInputOnFailure`, `TestVaultEncryptionFormatAndMetadataSafety`. | Read/bash/jobs/provider payloads, context, grants and encrypted vault. Native custom credential resolvers remain a coverage gap. |
| 0014 §3–§5, §6.1–§6.2, §7, §8 | S1 J | L: `TestLivePromptSettingsControlTakeover`, `TestLiveSkillsListingBudgetChangesTheCommandOutput`; U: `TestFixtureSectionOrder`, `TestBuiltinWorkbenchRendersRequiredSections`, `TestPromptEpochIsStableForTheSameText`, `TestUnmatchedLateSectionIsDeliveredOnTheNextPromptOfItsBranch`, `TestScanReadsFrontmatterAndHidesNestedUntilLoad`. | Engine, six frames, epochs and skills evidence. Live-provider A/B remains unauthorized; auto retains PiG's prompt and force requires instructions enabled. |

# Baseline gates

At `9126aff`, `just check`, `just race` and `just integration` pass on Linux after the search, private-instruction, mutation-barrier and shell-parser changes. Redacted output: [approved-check.log](evidence/approved-check.log), [approved-race.log](evidence/approved-race.log), [approved-integration-rerun.log](evidence/approved-integration-rerun.log). The first integration run failed only because the old composition-refusal regression expected the word `quoting` after static quotes became supported ([approved-integration.log](evidence/approved-integration.log)); the assertion now checks composition refusal and still verifies that no shell marker was created. The targeted case passed three times before the full rerun. These checks use scripted local inference, not a live-provider A/B evaluation.

At commit `9da6882` on Linux, `just check` (including validation), `just race` and `just integration` passed. Their redacted output is [check.log](evidence/check.log), [race.log](evidence/race.log) and [integration.log](evidence/integration.log); `$REPO` replaces the local checkout path. The same gates passed again at `31ab95a` after the new behavioral cases: [check](evidence/check-updated.log), [race](evidence/race-updated.log), [integration](evidence/integration-updated.log). These are baseline checks, not the final gate after the blocked implementation work.

Three repeated race-enabled runs passed for job manager cancellation, watcher joins and lifecycle shutdown ([leaks-unit.log](evidence/leaks-unit.log)), and for live crash reclamation, session-end job termination and reference-producer reload ([leaks-integration.log](evidence/leaks-integration.log)). These targeted checks are not a comprehensive heap or goroutine leak proof. The real TUI job/scratch reload path is not repeated by these three cases.

# Acceptance blockers

- All fourteen specifications are approved; that approval does not close the remaining implementation or acceptance gaps.
- Declared provider mutations are wired without new permission prompts under SPEC-0004 revision 8. Provider hashline editing remains unavailable; the full ownership-row evidence mapping and slice exit remain open. Redirects implement a static-word form, not general shell redirection.
- Live-provider A/B and the end-to-end inference transcript require fresh authorization for spend. The evidence above uses a scripted local provider, not live inference.
- `agent://by-project`, broadcast and coordinator are Slice 2. Activity-item click requires the fork host. Windows messaging, session storage and registry are compile-level only.
