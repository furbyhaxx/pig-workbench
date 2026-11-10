---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-inventory"
  title: "Slice 1 acceptance inventory"
  description: "PLAN-0001 Slice 1 ownership rows, observed representative tests and remaining acceptance gaps"
  status: "published"
  revision: 62
  created_at: "2026-11-05T16:40:00+01:00"
  updated_at: "2026-11-09T17:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 62
      reason: "Record fixes for defects reported in live use after revision 61"
      changes: ["Vault, key-permission, instruction churn, nonce, model cutoff, renderer and project creation work with their evidence"]
---

# Scope and evidence

This table follows all 24 [PLAN-0001 §9](../../plans/0001-pig-workbench.md) Slice 1 ownership rows, including Slice 1 portions of split rows. A passing test proves the named behavior, **not every sentence of its ownership row**. Limits remain explicit; see the recount below for the totals.

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
| 0007 §2–§8 local | S1 K | L: `TestLiveChangedInstructionsArriveAsADiffAgainstTheDeliveredView`, `TestLiveNewSteeringIsAdmittedAndDeletedSteeringStaysSilent`, `TestLiveInstructionChangedAcrossCompactionArrivesAsOneFullView`, `TestLiveTaggedBatchRequiresEveryTargetsInstructions`, `TestLiveTaggedMoveRequiresDestinationInstructions`; U: `TestRootNearestAndSymlinkDirectoryBoundaries`. | Diffs, pins/discovery, compaction, target unions and mutation prerequisites; polling occurs at boundaries, not native file watching. |
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
| 0014 §3–§5, §6.1–§6.2, §7, §8 | S1 J | L: `TestLivePromptSettingsControlTakeover`, `TestLiveSkillsListingBudgetChangesTheCommandOutput`; U: `TestFixtureSectionOrder`, `TestBuiltinWorkbenchRendersRequiredSections`, `TestPromptEpochIsStableForTheSameText`, `TestUnmatchedLateSectionIsDeliveredOnTheNextPromptOfItsBranch`, `TestScanReadsFrontmatterAndHidesNestedUntilLoad`. | Engine, six frames, epochs and skills evidence. Round 3 live A/B (79/80 per arm) justified `auto` resolving to `force` on stock PiG; takeover requires instructions enabled. |

# Recount (continuation 09.11.2026)

Every anchor table below was re-verified against code: each named test was checked to exist, stale OPEN and PARTIAL rows that pointed at finished tasks were restated, and a row that had been overwritten by mistake was restored.

| Measure | Count |
|---|---|
| PLAN-0001 §9 Slice 1 ownership rows with named passing evidence for their Slice 1 scope | 24 of 24 |
| Sub-anchors (SPEC-0001, 0003, 0004, 0005, 0006, 0007, 0008, 0009, 0011, 0013, 0014) passing, including those fixed during this slice | 101 |
| Sub-anchors deferred to a later slice by the plan | 6 |
| Sub-anchors not passing, with reason | 4 |

The four that do not pass:

- **Forced completion order of parallel instruction discovery:** overlap is exercised, but no deterministic hook orders completions.
- **Failed persistence never marks a frame delivered:** the diff base moves only on a confirmed frame; no test can force the append to fail.
- **Donor fixtures pinned to a source revision:** not claimed; behavior is covered by this repository's tests.
- **Eight-character evidence boundary with payload body comparison:** deliberately stricter here (see the relocation row).

Rows that pass for Slice 1 but carry a host or platform limit, each named where it appears: fused Workbench Piglet Binary (PiG merge), codemode child formatting and activity click (fork host), `activity.shortcut` registration order, Windows runtime for messaging, session storage and registry, and native custom credential resolvers that no extension can reach.

# Exact local resource anchors

Numbered anchors below follow the bullets in SPEC-0004 §10 and SPEC-0006 §8, scoped by PLAN-0001 §9 and PLAN-0002. PASS means only the stated behavior; OPEN requires the named follow-up task. Evidence for this audit is [resource-anchors-unit.log](evidence/resource-anchors-unit.log) and [resource-anchors-live.log](evidence/resource-anchors-live.log). This replaces the earlier assumption that a few green examples established the complete row.

| Anchor | Observed status | Evidence or precise remaining work |
|---|---|---|
| 0004 §10.1 provider lifecycle/direct/codemode/list/catalog | PASS (stock isolated Go host) | Reverse load order `TestLiveReferenceProducerLoadsBeforeWorkbench`; unregister stops admission and drains `TestLiveUnregisterStopsAndDrainsARunningProviderRead`; live reference seams and read/codemode tests. Fused-host form is the host limit under §10.10. |
| 0004 §10.2 atomic invalid/duplicate/reserved registrations | PASS | TestParseProviderDescriptor and TestRegisterContributedIsAtomic; live reference registration/reload has separate protocol evidence. |
| 0004 §10.3 path and selector grammar | PASS (local/core) | TestLiteralFilesAndURIPunctuation, TestSelectors, TestRawPreservesBytesAndLineConvention and TestLargeLineNeverBecomesSeen. Remote/archive adapters are outside Slice 1. |
| 0004 §10.4 revision/paging/unknown totals | PASS | Source revisions revalidate on each read. Cursor bindings and reload expiry pass [live](evidence/listing-cursors-live.log) and [retention/privacy](evidence/listing-cursors-safety.log); unknown versus known-empty totals pass [unit](evidence/provider-totals-unit.log) and [actual SDK](evidence/provider-totals-live.log); native directory cursors pass [unit](evidence/native-directory-cursors-unit.log) and [actual PiG](evidence/native-directory-cursors-live.log). |
| 0004 §10.5 core trees | PASS for the core routes; by-project Slice 2 | Stored-session/title, scratch, job, skill, cfg, artifact and reference-doc tests run. `session://by-project/` and `agent://by-project` are PLAN-0001 Slice 2 rows, not claimed here. |
| 0004 §10.6 source/derived seen state and alias edit refusal | PASS | TestLargeLineNeverBecomesSeen, TestRenderSpansValidatesProviderPages and live reference edit refusal; full edit matrix is 17.8.4.1. |
| 0004 §10.7 mutation outcomes | PASS | `TestLiveProviderMutationsWithoutConfirmation` (verb and failure matrix, stale revision), `TestMutationReceiptCannotInventCommitOrHidePartialEffects`, `TestLiveMalformedProviderReplyCannotClaimNotAppliedAfterCommit`, `TestLiveAbortedReadCancelsTheCooperativeProvider`; the reference provider answers a changed payload under a reused idempotency key with `conflict`. Retention of receipts is the provider's stated guarantee; Workbench never retries an unknown outcome. |
| 0004 §10.8 bounded slow/noisy providers | PASS | `TestProviderBusAdmissionIsBoundedPerOwnerAndReleasedOnce` (8 per owner), page/byte/line limits clamp every read, unregister drains (`TestLiveUnregisterStopsAndDrainsARunningProviderRead`); renderers do no provider calls. |
| 0004 §10.9 invalidation/revalidation | PASS | `TestLiveProviderResourcesChangedInvalidatesRetainedCursors`, `TestListingCursorInvalidationDropsOnlyTheAffectedRegistrationAndPrefix`; file reads recompute source digests and syntax cache keys include content. A lost notification is recovered by revalidation at the next read. |
| 0004 §10.10 direct/bus semantics and host forms | PASS (stock isolated, Python foreign runtime); fused host limit | Local provider and real bus tests pass; a Python extension speaks the wire protocol. A Piglet Binary fusing the reference producer registers; fusing Workbench itself is blocked by PiG v0.7.0's merge ([repro](evidence/fused-workbench-repro.log)). |
| 0004 §11 fetch routing/policy/fallback | PASS | Builtin redirect/media/policy, most-specific backend selection, no fallback on a definitive answer, egress mismatch rejected, locate through the router once; per-backend enable toggle `fetch.disabled-backends` (`TestFetchBackendSelectionSkipsDisabledBackends`, `TestLiveDisabledFetchBackendFallsBackToTheBuiltinReader`). Backend ordering is registration order; reordering is not offered. |
| 0006 §8.1 selector goldens and bounded bytes | PASS (source) | Selector, raw-byte, continuation and large-line tests pass. Aggregate guide/continuation framing passes in [read-guide-budgets.log](evidence/read-guide-budgets.log). |
| 0006 §8.2 literal punctuation and view chains | PASS (local/core) | Literal/query/port parser cases pass; archive/binary format adapters remain Slice 2. |
| 0006 §8.3 summary coordinates/seen | PASS | Summary and source coverage tests pass; summaries are ineligible as superseding complete source. |
| 0006 §8.4 representative/malformed/oversize/cancel matrix | PASS (local, provider, static HTTP) | Local source, directory, provider and static-HTTP fixtures including aggregate guide/continuation bounds; provider drain on unregister and abort. Rich adapters are §8.5–§8.7. |
| 0006 §8.5–§8.7 rich adapters/notebooks/media | DEFERRED | PLAN-0001 assigns these adapters to later slices; they are not advertised as implemented. |
| 0006 §8.8 direct/codemode/provider bounds and batches | PASS | Live direct/codemode and guide/continuation budget tests; captured-project routing in [captured-provider-project.log](evidence/captured-provider-project.log). |
| 0006 §8.9 static URL policy/no billed question answering | PASS | Builtin HTTP and reference backend tests pass without inference spend. |
| 0006 §8.10 performance | PASS (local) | `TestReadOfAHugeFileStaysBounded` (128 MiB file, first page in under a second, 36 MiB allocated, sublinear in file size) and `TestSearchOverALargeTreeStaysBoundedAndHonest` (20000 files; stops at 5000 scanned or 200 matches and says incomplete); [log](evidence/read-search-performance.log). Remote and archive benchmarks depend on deferred backends. |
| 0006 §7 repetition and supersession | PASS (implemented safety path) | Repeated read and history-preserving projection tests pass; the existing threshold setting must be wired in 17.8.2.4. |
| 0006 §6 search ignores and glob dialect | PASS | Nested .gitignore/negation and explicit-root semantics pass [real-host](evidence/search-ignore-live.log) and [safety](evidence/search-ignore-safety.log) tests. Brace/class/doublestar dialect parity and invalid-pattern safety pass [unit](evidence/search-glob-unit.log) and [real-host](evidence/search-glob-live.log) regressions. |
| 0006 §6 listing/search output details | PASS (covered modes) | Bounded ls/find output and explicit hidden-entry facts pass [safety](evidence/list-output-safety.log), [actual PiG](evidence/list-output-live.log) and [find path-byte](evidence/find-output-bytes.log) cases. Unicode literal byte columns pass [scanner](evidence/literal-case-columns-unit.log) and [actual PiG](evidence/literal-case-columns-live.log) regressions. Filename-only output passes [scanner](evidence/grep-files-only-unit.log) and [actual PiG](evidence/grep-files-only-live.log) tests without retaining source/context. |
| 0012 §5.3 captured routing facts | PASS | Original current-location routing fails under concurrent cd ([repro](evidence/captured-provider-project-repro.log)); captured resource/search/fetch contexts and mutation revisions pass repeated real-host tests ([evidence](evidence/captured-provider-project.log)). |

# Exact local protocol anchors

SPEC-0001 §7 bullets are audited independently of representative discovery success. Current observed suites are [protocol-final-unit.log](evidence/protocol-final-unit.log) and [protocol-final-live.log](evidence/protocol-final-live.log) (earlier: [unit](evidence/protocol-anchors-unit.log), [live](evidence/protocol-anchors-live.log)).

| Anchor | Status | Evidence and precise remaining work |
|---|---|---|
| §7.1 discovery/load order/absence/version/duplicates/reload/session switch | PASS (stock isolated Go) | Actual service-failure/discovery, before-Workbench producer, reference reload and session replacement/resume cases pass. Fused/foreign host forms remain §7.3/8. |
| §7.2 identical/changed/colliding/atomic replace/stale handles | PASS | Registry unit tests; live stale-generation and reload cases; replacement and unregister now close admission, cancel and drain running operations (`TestLiveUnregisterStopsAndDrainsARunningProviderRead`). |
| §7.3 foreign-runtime JSON equivalence | PASS (Python, stock host) | A Python extension speaks the wire protocol with plain JSON: discover, `location.get`, a stale generation (`staleGeneration`, retryable), an oversized request (`limitExceeded`), an unknown method (`unsupported`) and a served `pyfix://` provider read with line selection (`TestLivePythonClientSpeaksTheWireProtocol`, `TestLivePythonProviderServesAReadThroughWorkbench`; [foreign-fused-runtime.log](evidence/foreign-fused-runtime.log)). Rust, Node and Lisp runtimes were not exercised. |
| §7.4 nested control/provider/progress and owned shutdown | PASS | Real nested reference calls and reloads pass. Provider progress is consumed on `provider-progress` without re-entering the provider: newest correlated note wins, stale/foreign/malformed/oversized notes are dropped (`TestProviderProgressIsBoundedCorrelatedAndNewestWins`), and the live inspector shows it while a read runs (`TestLiveProviderProgressShowsInTheResourcesInspectorWhileRunning`). Per-owner admission is bounded at 8. |
| §7.5 deadline/cancel/failure/unknown mutation outcome | PASS | The provider handler observes the request deadline and an explicit cancel (`TestLiveProviderHandlerObservesTheRequestDeadline`, `TestLiveAbortedReadCancelsTheCooperativeProvider`); the control client sends cancel for any call that ends without a reply and keeps a correlated reply that arrived before an emit error; a malformed reply cannot claim `notApplied` after a commit (`TestLiveMalformedProviderReplyCannotClaimNotAppliedAfterCommit`); unknown outcomes are never retryable. |
| §7.6 response/envelope/counters/duplicate IDs/lost notifications | PASS | Strict provider/control reply validation ([unit](evidence/provider-reply-validation-unit.log), [live](evidence/provider-reply-validation-live.log)), oversized/malformed envelopes, duplicate request IDs, the 8-per-owner counter, and listing/resource state recovered by revalidation after a missed notification. |
| §7.7 absent-producer replay and dormant settings | PASS | Dormant typed schemas and overrides survive resume without the producer and retained results render without producer I/O (`TestLiveResumeWithoutProducerRetainsUnavailableDisplaySchemaAndOverrides`, `TestLivePresentationDefaultsAndDormantOverridesFollowStableKeys`). |
| §7.8 cancellation-after-abort, late replies, isolated/fused | PASS isolated; fused Workbench is a host limit | Abort and withdrawal stop a busy provider; late replies are discarded by correlation. A Piglet Binary fusing the reference producer registers all six kinds with the subprocess Workbench (`TestLiveFusedReferenceProducerRegistersWithSubprocessWorkbench`, opt-in `WORKBENCH_FUSED_PIG`). Workbench itself does not fuse: PiG v0.7.0's merge does not add modules Workbench needs that PiG's go.mod lacks ([repro](evidence/fused-workbench-repro.log)). |
| §7.9 reload hang consistency | PASS for normal reloads; never-returning reload is a host limit | Two real reloads and independent consumer rediscovery pass. A never-returning `ctx.Reload()` cannot be provoked deterministically (SPEC-0001 §7 host issues). |
| §8 collaboration services | PASS | Snapshots, conditional secrets, context fields, fetch, status, docs, frames and two-session extension messaging each have actual consumer evidence. |

Remaining protocol limits are host or platform facts, not Workbench gaps: fused Workbench cannot be built by PiG v0.7.0, and a never-returning reload cannot be provoked. No provider mutation adds a permission prompt; synchronous provider mutations return receipts directly and an unknown outcome requires state reconciliation, per SPEC-0004 §6.1.

# Exact local presentation anchors

SPEC-0003 §8 bullets audited against the in-scope §2–§6 and §10 contract after all presentation work. Final observed runs: [unit](evidence/presentation-final-unit.log) and [real PiG hosts](evidence/presentation-final-live.log).

| Anchor | Status | Evidence |
|---|---|---|
| §8.1 foreign tool renders, appears in settings, keeps working after Workbench unload | PASS | `TestLiveReferenceProducerSeams` (renderer on real data), `TestLiveForeignToolKeepsWorkingWithoutWorkbench` (producer alone, tool result intact), `TestLiveDisplaySettingsGetReportsOnlyEffectiveDisplayValues`, `TestLiveGroupedDisplaySettingsEditResetAndInspection` |
| §8.2 no override for non-opted-in tools; visible conflicts | PASS | `TestToolPresentationRegistrationDrivesTheResolver` (unregistered tool reaches next), `TestPresentationClaimRequiresVerifiedToolOwnership`, `TestLivePresentationOwnerCannotClaimAnotherExtensionsTool`, first-use `appliesAfterReload` receipts |
| §8.3 host error and monotonic final state | PASS | `TestPresentationCacheNeverResurrectsSettledOrConflictingFinals`, `TestLivePresentationUpdatesRetainFailedFinalAndExposeConflict`, host-error row in `TestPresentationPlainRowPerLifecycleState` |
| §8.4 incomplete success vs partial-commit failure | PASS | `TestPresentationPlainRowPerLifecycleState` |
| §8.5 every preset and state at 36/57/80/140 columns, bounded height, hostile text | PASS (plain renderer) | `TestEveryPresetAndStateFitsEveryWidthWithBoundedHeight`: 8 presets x 3 densities x 8 states x 4 widths on the typed path with wide runes and long URIs; it fails when the width bound is tightened by one cell. `TestPresentationHostileTerminalBytesNeverReachTheRow` covers escape bytes. Short height is the row-count bound; kit panels are Slice 3. |
| §8.6 donor path display, authority and basename | PASS | `internal/pathdisplay` goldens; `TestPresentationCallUsesRecordedRootAndLabelsMissingOrPartialArguments` for recorded roots |
| §8.7 unknown metrics, images, malformed Details, unsupported versions, omitted children | PASS | `TestDecode*` schema cases, `TestPresentationUnusableDetailsKeepTheRawResult`, `TestLiveCodemodeChildIdentitiesAreRetainedForInspection` |
| §8.8 precedence, reset, dormancy, replacement, invalidation | PASS for enabled/density/previewLines/metric visibility/target shortening; child-density and producer custom toggles are a host limit | `TestLivePresentationDefaultsAndDormantOverridesFollowStableKeys`, `TestLiveDisplaySettingsChangedCarriesOnlyBoundDisplayValues`, dynamic display config unit tests. Child-density and custom toggles are stored and reported but nothing renders from them: stock PiG draws codemode child rows itself (fork hook, PLAN-0004 C4). |
| §8.9 direct vs codemode semantics, replay without producer performs no I/O | PASS for replay; codemode child formatting is a host limit | `TestLiveRetainedProducerResultIsReadableAfterProducerRemoval`, `TestLiveResumeWithoutProducerRetainsUnavailableDisplaySchemaAndOverrides`, `TestLiveCodemodeChildIdentitiesAreRetainedForInspection`. Unified child formatting needs the fork hook (PLAN-0004 C4). |
| §8.10 cached resolver claims, unregister, no foreign next() call | PASS | resolver first-use tracking tests, `TestLiveReferenceProducerReload`; no foreign renderer marker is ever called |
| §5 hot render path | PASS | `TestPresentationCacheOwnsValidatedDataAndSeparatesCalls`, `TestPresentationCacheRetentionIsBoundedAndFinalCannotRemainRunning` |
| §10 recorded target root, host metric authority | PASS | `TestPresentationCallUsesRecordedRootAndLabelsMissingOrPartialArguments`, `TestPresentationHeaderHostDurationOverridesProducerApproximationOnly` |

Remaining PARTIAL rows are host limits (stock codemode child rows) and are named, not hidden; kit panels and designed layouts stay Slice 3.

# Provisional grouped display settings view

The plain `/workbench settings tools` surface has actual RPC scoped edit/reset and display-only settings.changed evidence: [unit](evidence/display-settings-commands-unit.log), [live](evidence/display-settings-commands-live.log). The new TUI view is **awaiting user layout review**, not approved chrome. Short-height (16-row) captures: [36 columns](evidence/display-settings-tui/w36.txt), [57](evidence/display-settings-tui/w57.txt), [80](evidence/display-settings-tui/w80.txt), [140](evidence/display-settings-tui/w140.txt); ANSI counterparts are alongside them. Captures use an isolated fixture model with an unreachable local endpoint and slash commands only; no inference was requested.

The first full integration run failed because source ownership verification correctly refused a presented tool hidden by the CLI --tools selection, and the reference fixture aborted its unrelated registrations ([first run](evidence/display-settings-full-integration-first.log)). The fixture now skips only that unavailable presentation after host metadata confirms the tool is absent, retaining resources/fetch/frames; ownership policy is not bypassed. The full suite then passed ([rerun](evidence/display-settings-full-integration-rerun.log)).

# Retained result secret boundary

`TestLiveRetainedToolMetadataNeverPersistsKnownSecrets` reproduced known values in actual JSONL result metadata after command substitution and secret-bearing resource paths. Tool-result content, details and structured content are now protected before persistence; metadata JSON keys/values are redacted, collisions/unsupported data fail closed, and Workbench's captured presentation update copies are protected too. This corrects SPEC-0013's earlier unsafe assumption that non-provider Details could remain untouched.

Observed checks: [check](evidence/retained-redaction-check.log), [race](evidence/retained-redaction-race.log), [full host integration](evidence/retained-redaction-all.log), [repeated unit cases](evidence/retained-redaction-unit.log), and [three actual-host runs](evidence/retained-redaction-live.log). The shell still receives real substituted values, while retained commands, resource URIs, JSON keys and structured fields do not persist them. Existing session files are **not** rewritten or claimed repaired. Native third-party live progress rendering is not globally transformed; only Workbench's retained update copies are protected.

# Absent-producer replay and codemode capture

Observed in actual PiG hosts ([log](evidence/replay-and-children-live.log), repeated twice): dormant display schemas and overrides survive resume without the producer and report `application: unavailable` with no live registration; a retained producer result stays readable through `session://by-id/<id>/transcript`; `/workbench codemode` lists nested call IDs and ok/error status from `parentToolCallId`, bounded and without arguments. Stock PiG still draws codemode's own child rows, and child records are runtime-only. SPEC-0003 §8 rows 8.1, 8.8 and 8.9 are therefore evidenced for data and settings; rich replay chrome is not claimed.

# Recorded target roots and host metric authority

Call headers now shorten only `path`-format fields, against the working root recorded at `tool_call` (bounded to 512 calls, runtime-only), so a later `cd` cannot rewrite an earlier row; calls from before this runtime show absolute paths. Missing arguments render `arguments not retained` and streaming ones `partial arguments`. A host-recorded duration replaces a producer `durationMs` metric but never creates one the producer did not report. Covered by unit tests ([check](evidence/target-roots-check.log), [race](evidence/target-roots-race.log), [full integration](evidence/target-roots-all.log)); a real TUI capture of a call row after `cd` was not taken, so renderer output is verified at function level only.

# Provider admission bound

\`providerBus.call\` now reserves one of 8 per-owner slots before any emit and releases it exactly once; the ninth concurrent operation returns \`busy\`, \`retryable\`, \`notApplied\` without reaching the provider, and another owner is unaffected. SPEC-0001 §4 defines only the per-owner bound: the resource-provider descriptor has **no** per-registration limit field, so none is implemented and none is claimed. Unit tests cover the bound, single release and no-emit-at-limit. [check](evidence/provider-admission-check.log), [race](evidence/provider-admission-race.log). The first full integration run ([log](evidence/provider-admission-all-first-flaked.log)) failed once in \`TestLiveCatalogSettingsHaveBehavioralConsumers/activity/disabled\` with \`TempDir RemoveAll cleanup: directory not empty\` (a pig process still writing into its agent dir during teardown); that test passed 3 of 3 and its activity subtest 5 of 5 alone, and the full rerun passed ([log](evidence/provider-admission-all-rerun.log)). This is an unfixed, intermittent harness race, not evidence of a product defect.

# Interactive menu (user-reported defect)

Reproduced in a real `pig -e .` TUI: `/workbench tools settings` threw `tool "settings" not found`, and `/workbench` was only a 16-item autocomplete list whose entries printed text into the transcript (`/workbench settings` dumped ~100 lines of JSON; long views were cut to the terminal width). Nothing in the extension used a kit panel. This contradicted PLAN-0002's own "plain views" deferral only in degree: the views existed but were not usable.

Now, in the TUI: `/workbench` opens a hub (SKETCH-0001 §3 shape) listing 17 views; every entry opens a titled, scrollable, width-wrapped panel and Esc returns to the hub with the selection kept; `/workbench settings` opens a settings panel with search, value and source per setting, staged edits (toggle, choice cycle, typed integers and strings with bounds), per-scope save, reset staging and a discard guard. An unknown tool name now warns instead of failing the command. RPC and print keep plain text. Real-TUI tests: `TestLiveEveryHubEntryOpensATitledPanelWithoutAnError`, `TestLiveWorkbenchMenuOpensInteractivePanelsInTUI` (including a save that is read back from `Workbench.toml`), `TestLiveWorkbenchSettingsPanelRefusesInvalidIntegerAndGuardsDiscard`, `TestLiveToolsWithAnUnknownNameWarnsInsteadOfFailingTheCommand` (confirmed to fail on the old behaviour); logs: [check](evidence/menu-check.log), [race](evidence/menu-race.log), [full integration](evidence/menu-all.log).

**Awaiting user layout review** (these are new surfaces): captures at 36/57/80/140 columns and 16 rows in `evidence/menu-tui/{hub,settings,tools}/`. Later (review follow-up below): the hub is searchable, array settings are editable and the jobs, pins, trust and sessions panels are interactive. Still not exercised: mouse input; sizing was measured on one terminal emulator (tmux).

# Tool display view (second user report)

The user ran plain `pig -e .` and saw only "No registered or dormant display schemas." My earlier captures had the reference producer loaded, so I never showed the state a normal session has. Workbench's own tools have no display preferences (their density renderers are PLAN-0004 C1-C3), so that screen is correct but explained nothing and the hub offered it as if usable.

Now: the hub entry reads "Nothing to configure: no extension registered one" or "N tool(s) with display preferences"; with no schema the panel says why and points to `/workbench integrations`; with schemas `Tool display` opens the interactive settings panel limited to `tools.`, labelled by field with `owner/tool` and a plain-language status ("takes effect now" / "saved, but nothing reads it yet"). Rows used to show only `tools.reference-producer.web_s` (truncated, indistinguishable) - fixed. `target-shortening` is now wired (`pathdisplay.Target.NoFish`) and reports `live`; `child-density` stays stored-only because Workbench draws no child rows. Test: `TestLiveToolDisplayViewExplainsAnEmptyStateAndEditsWhenASchemaExists` (real TUI, both states). Captures: `evidence/display-settings-tui/plain/` (what a bare `pig -e .` shows) and `with-producer/`, at 36/57/80/140 columns and 16 rows.

Environment findings: the first full run failed with `no space left on device` because `/tmp` is a 32 GB tmpfs at 99% (mostly other programs' data, which I did not touch) and every integration test builds the extension into a temp cache. I removed only my own `TestLive*` and scratch files. Gates now run with `TMPDIR=$HOME/.cache/pig-workbench-test-tmp/work` (outside any repository: a `TMPDIR` inside this repo made project-detection tests find the repo's own `.git`). A real harness race was also fixed: TUI tests killed tmux and removed the temp dir while `pig` was still writing (`TempDir RemoveAll cleanup: directory not empty`); `stopTUI` now waits for the process. One transient failure of `TestLiveWorkbenchSettingsPanelRefusesInvalidIntegerAndGuardsDiscard` (two runs of 15.7 s, in a wide group while another job loaded the machine) did not reproduce in 9 later runs; its cause is **not established**. Logs: [check](evidence/tool-display-check.log), [race](evidence/tool-display-race.log), [full integration](evidence/tool-display-all.log).

# Live evaluation (deepseek/deepseek-flash, user-authorized 08.11.2026)

Harness: `just live-eval` (build tags `integration liveeval`, key from `pig auth print-api-key` into the environment only, never retained). Transcripts are in [evidence/live-eval](evidence/live-eval/) with the key and key-shaped tokens removed (zero hits).

**End-to-end workflow** ([workflow.jsonl](evidence/live-eval/workflow.jsonl)): one real agent run drove all ten tools (`ls`, `find`, `grep`, `read`, `edit`, `write`, `bash`, `jobs`, `cd`, `ask_user`) and the `cfg`, `docs`, `session`, `artifact`, `skill` and `local` schemes. Each step is judged from disk, events and tool arguments, not from the model's report. Its first attempt exposed three defects (tree roots listed nothing, `local://` scratch was not writable, `ls` on a bare scheme root failed), fixed in `307681c`. The retained run passed every step; tokens: 14549 in, 1169 out, 116736 cache-read.

**Prompt A/B**, fixed tasks (bug fix, rename across files, count answer, run-and-write, read line 317 of a long file), 3 repetitions per task, fresh session and fixture each, pass judged from disk:

| Round | Arm | Pass | Tool calls | Tool errors | Input tokens | Cache hit |
|---|---|---|---|---|---|---|
| 1 | PiG prompt (`auto`) | 14/15 | 39 | 4 | 91 925 | 70.8% |
| 1 | `builtin/workbench` (`force`) | 15/15 | 63 | 12 | 71 632 | 74.1% |
| 2 | PiG prompt (`auto`) | 15/15 | 39 | 1 | 91 494 | 69.2% |
| 2 | `builtin/workbench` (`force`) | 15/15 | 40 | 2 | 70 591 | 75.8% |

Round 1 is **not a verdict**: the served prompt was 473 bytes against PiG's 6 899 because `renderBuiltinPrompt` passed only the project root, so `<tools>`, rules, skills and the append text rendered empty ([round 1](evidence/live-eval/ab-round1/)). That is a Slice 1 defect against SPEC-0014 §7 and was fixed in `977c84b`: the template now renders declared tools with snippets, tool and prompt guidelines, the active edit mode, host skills, registered schemes and append text (6 116 bytes served; [round 2](evidence/live-eval/ab-round2/)).

Verdict on round 2: no regression and a measurable saving. Both arms passed 15/15; the Workbench prompt used 23% fewer input tokens, a higher cache-hit share and equal tool calls and time. The tool-error difference (1 vs 2) and the PiG arm's drop from 4 errors to 1 between rounds with no change to that arm show the per-call error counts are noise at n=15, so they support no ranking. Limits: one model, five small tasks, three repetitions, no confidence intervals, and the PiG arm's prompt included this machine's user-level skill catalog while the Workbench arm lists the same host skills. **`prompt.delivery = "auto"` still serves PiG's prompt**: this evidence shows no regression, not a case for flipping the default, and SPEC-0014 §12's golden-transcript comparison and per-family adapters remain Slice 3.

Defects the live runs surfaced that this evidence does not close: DeepSeek's first `edit` attempt is often the exact-text form that the default `edit.mode = hashline` refuses (the donor's per-model rule that maps DeepSeek to `replace` is G5.9, Slice 2), and a `{path, hashline}` call fails validation with "edits: missing". Both recover on the next call.

**Round 3** ([summary](evidence/live-eval/ab-round3/SUMMARY.md)): 16 tasks (the five above plus the eleven-task tool-text suite with four held-out), 5 repetitions per task and arm, 80 runs each. PiG's prompt passed 79/80 with 4 tool errors and 501 547 input tokens; `builtin/workbench` passed 79/80 with 0 tool errors and 387 379 input tokens (-23%). The single failure per arm did not reproduce in 10 further repetitions of its task, so no guideline text was changed. Decision: `prompt.delivery = auto` now resolves to `force` on stock PiG as SPEC-0014 §4 specifies (`prompt.enabled = false` keeps PiG's prompt); the frames, instructions, pin, capped-BACKLOG and compaction live runs were repeated under the new default and pass. Still one model; family adapters remain Slice 3.

# Baseline gates

At `733298a`, the exact `just check`, `just race` and `just integration` recipes passed after provider mutation wiring: [mutations-final-check.log](evidence/mutations-final-check.log), [mutations-final-race.log](evidence/mutations-final-race.log), [mutations-final-integration.log](evidence/mutations-final-integration.log). No new permission or confirmation layer is installed for contributed mutations. Full fresh, uncached verbose race evidence at the same implementation is linked above. No billed inference was used.

At `9126aff`, `just check`, `just race` and `just integration` pass on Linux after the search, private-instruction, mutation-barrier and shell-parser changes. Redacted output: [approved-check.log](evidence/approved-check.log), [approved-race.log](evidence/approved-race.log), [approved-integration-rerun.log](evidence/approved-integration-rerun.log). The first integration run failed only because the old composition-refusal regression expected the word `quoting` after static quotes became supported ([approved-integration.log](evidence/approved-integration.log)); the assertion now checks composition refusal and still verifies that no shell marker was created. The targeted case passed three times before the full rerun. These checks use scripted local inference, not a live-provider A/B evaluation.

At commit `9da6882` on Linux, `just check` (including validation), `just race` and `just integration` passed. Their redacted output is [check.log](evidence/check.log), [race.log](evidence/race.log) and [integration.log](evidence/integration.log); `$REPO` replaces the local checkout path. The same gates passed again at `31ab95a` after the new behavioral cases: [check](evidence/check-updated.log), [race](evidence/race-updated.log), [integration](evidence/integration-updated.log). These are baseline checks, not the final gate after the blocked implementation work.

Three repeated race-enabled runs passed for job manager cancellation, watcher joins and lifecycle shutdown ([leaks-unit.log](evidence/leaks-unit.log)), and for live crash reclamation, session-end job termination and reference-producer reload ([leaks-integration.log](evidence/leaks-integration.log)). These targeted checks are not a comprehensive heap or goroutine leak proof. The real TUI job/scratch reload path is not repeated by these three cases.

# Exact edit anchors (SPEC-0005 §8)

| Acceptance bullet | Status | Evidence |
|---|---|---|
| Replacements, same-gap order, empty file, EOF without newline, CRLF/BOM, mixed endings | PASS | `TestStageLinesGapOrderingAndRangeBoundaries`, `TestStageLinesEmptySourceAndEndpointGaps`, `TestStageLinesTerminalNewline`, `TestPlanBatchPreservesBOMAndEndings`, `TestRestoreMixedEndings` |
| Cross-file CUT/paste, named registers over calls, duplicate/undefined use | PASS | `TestPlanBatchCrossFileBlockCutAndPaste`, `TestPlanBatchRefusesUnseenInteriorAndUndefinedRegister`, `TestPlanBatchRegisterOrderAcrossRepeatedSections`, `TestLiveBlockCutPastesIntoAnotherFileThroughARegister`. Cyclic use cannot be expressed: a register is captured from the original coordinates before any paste resolves |
| Block cases: decorators, comments, anti-swallowing, Markdown fences | PASS for Go, Python and Markdown | `TestGoBlockAnchors`, `TestPythonIndentationAndDecorators`, `TestMarkdownHeadingSectionsRespectFences`, `TestLiveBlockReplaceUsesSyntaxBoundariesAndRefusesAnUnanchoredLine`. Other grammars use the shared analysis but have no per-language fixtures |
| Tag collision, wrong resource, stale shifted spans, ambiguous repeated code | PASS | `TestStoreChoosesNewestVersionWithNormalizedTag`, `TestStoreLimitsAndCanonicalResourceBinding`, `TestRelocateOpsRefusesAmbiguousCurrentMatches`, `TestRelocateOpsRefusesDuplicateAfterIntendedOccurrenceChanged`, `TestLiveEditAnOlderTagRecoversAgainstTheNewestSnapshot`, `TestLiveEditRelocatesOnlyWhenTheTargetIsUnambiguous` |
| Eight-character evidence boundary, payload body comparison | NOT IMPLEMENTED AS WRITTEN | Relocation is stricter by design: an anchor moves only when its exact content plus up to three neighbours on each side matches once in both sources, else the target alone must be unique; everything else is refused. The donor's payload-body comparison with a shared character budget does not exist here. This refuses more than the donor, never less; it is recorded as an intentional difference, not a pass |
| Hidden summary lines, column caps, unseen block interiors rejected | PASS | `TestLiveEditRefusesLinesTheModelNeverSaw`, `TestLiveEditRefusesALineTheReadBudgetCutInHalf`, `TestCheckSeenPreciseMissingSpans` |
| Compaction cannot resurrect seen state | PASS | `TestLiveCompactionInvalidatesSeenSnapshots` (real `/compact`, then an edit with the pre-compaction tag is refused and the file is unchanged) |
| Rename races, symlink swaps, read-only provider, crash/cancel between commits | PASS | `TestCommitMoveNeverOverwritesRacingDestination`, `TestCommitRejectsSourceReplacedDuringBatch`, `TestCommitReplacementThroughSymlinkPreservesLinkAndMode`, `TestCommitInjectedFailureKeepsFirstCommit`, `TestCommitCancellation`, `TestAtomicWriteFileRefusesRacingDestination`, `TestLiveReferenceProducerSeams` (unadvertised edit refused before dispatch) |
| Exact-text mode on original source; write never installs truncated output | PASS | `TestExactEditsUseOriginalSourceAndPreserveBytes`, `TestLiveTaggedAndExactEditUseObservedSource`, `TestLiveWriteNeverInstallsATruncatedReadCopy` |
| Direct and codemode yield identical safety | PASS for the unseen-line refusal | `TestLiveCodemodeEditRefusesUnseenLinesLikeTheDirectTool` |
| All donor fixtures pinned to a source revision | NOT PROVEN | The behaviors are covered by this repository's own tests; no pinned donor corpus is run, so this bullet is not claimed |

# Exact instruction anchors (SPEC-0007 §10, local)

| Acceptance bullet | Status | Evidence |
|---|---|---|
| Selection table, filename precedence, root missing, same root/nearest, boundaries | PASS | `TestOverrideWinsAndBoundaryStops`, `TestRootNearestAndSymlinkDirectoryBoundaries`, `TestUserRootsPrecedeProject`, `TestNestedRepositoryRequiresExplicitConsent`, `TestLiveNestedRepositoryInstructionsLoadOnlyAfterATrustRule`. `select = "all"` ordering is Slice 2 (`TestPlacementsAndSelectAll` covers only the engine). A linked-worktree boundary has no dedicated test |
| Parallel multi-read batches in different subtrees | PASS, after a defect fix | `TestLiveCodemodeParallelReadsDeliverTheUnionOfInstructions`. It reproduced one pending frame sent twice when two tool calls overlapped; fixed in `frame_context.go` by tracking frames already steered until confirmed |
| Completion out of order, cwd transition during pending discovery | PARTIAL | Overlap is exercised (`TestLiveCodemodeParallelReadsDeliverTheUnionOfInstructions`). A `cd` between deliveries delivers the nested file once and does not repeat the root (`TestLiveCdIntoANestedTreeDeliversItsInstructionsWithoutRepeatingTheRoot`). Forced completion order has no deterministic hook and is not claimed. |
| Pin add/unpin, branch fork, authority separation | PASS | `TestLivePinnedBacklogStaysDeliveredAndPoliciesHold`, `TestPinsFollowTheActiveBranchOnly`, `TestPinnableClassRefusesSteeringAndNamesAmbiguity`, `TestLivePrivateNotesUpdateAndPinnedNotesWithdrawOnProjectChange`. Pin replace-nearest and trust revocation of an existing pin are not covered by a named test |
| BACKLOG counts, malformed pointers, oversized source, cap change, reorder | PASS | `TestBoundedScanReportsUnknownOmissions`, `TestMalformedPointerRemainsVisibleWithDiagnostic`, `TestContinuationBudgetDoesNotOscillate`, `TestViewsReserveFooterAndKeepWholeLines`, `TestLiveBacklogShowsAMissingPointerTarget` |
| Diff reconstructs the delivered view byte for byte; no absent baseline | PASS | `TestUnifiedDiffAppliesByteForByte`, `TestApplyDiffRefusesAWrongBase`, `TestLiveChangedInstructionsArriveAsADiffAgainstTheDeliveredView`, `TestLiveInstructionChangedAcrossCompactionArrivesAsOneFullView` |
| New file, deletion, turn-boundary healing | PASS | `TestLiveNewSteeringIsAdmittedAndDeletedSteeringStaysSilent`. There is no filesystem watcher: sources are re-evaluated at each prompt and tool boundary, so watcher overflow and remote disconnect do not apply locally |
| Mutation blocked before new instructions; codemode cannot hide targets | PASS, after a defect fix | `TestLiveMutationIsBlockedOnceWhenNewInstructionsApply`, `TestLiveTruncatedSteeringBlocksMutationsUntilFullyRead`, `TestLiveCodemodeCannotHideAMutationTargetFromTheInstructionBarrier`. The codemode retry was blocked forever because a target-less parent call withdrew the touched instruction; target-less calls no longer withdraw |
| Self-edit transformation, failed persistence never marks delivered | PARTIAL | The diff base advances only when the frame is confirmed in context (`confirmContextFrames`). No test forces a persistence failure; not claimed. |
| Equal bytes, different trust do not deduplicate | PASS | `TestDuplicateUsesMostRestrictiveRoleAndLimits` (engine) and `TestLiveEqualBytesInUserAndProjectRootsAreNotMerged` (identical text in a user and a project root arrives as two frames). Frames carry one data trust; root origin is what separates them. |
| Host duplicate context suppression | PASS | `TestLiveCompactionResendsContextFactsOnce`, `TestLiveFramesAndInstructionRulesCommands` |

# SPEC-0014 §12 results (prompt, skills, contributed fields)

Verified against code in the continuation; two bullets were not met as written and were fixed, one stays open.

| Acceptance bullet | Status | Evidence |
|---|---|---|
| Golden renders, section order, `<cwd>` unchanged after `cd` | PASS | `TestFixtureSectionOrder`, `TestBuiltinWorkbenchRendersRequiredSections`, `TestBuiltinWorkbenchCarriesHostInputs`, `TestLiveForcedPromptCarriesHostInputs` |
| Template chain: shadowing, `builtin/` not shadowable, traversal, symlink, strict undefined, limits | PASS | `internal/prompt/filesystem_test.go`, `TestRenderStableAndStrictUndefined`, `TestIncludeDepth`, `TestLoopCancellation` |
| Conditionals `family`, `glob`, `regex` | PASS | `TestFamilyGlobAndRegexMatchAndMiss` |
| Conditionals `has_tool`, `variant` | **Missing, now implemented** | These two globals did not exist. `helpers.go` adds them; `TestHasToolAndVariantSelectTextPerModelAndLoadout` covers a family, an unmatched family, an absent tool, a missing variant and a non-string argument |
| Epoch stability, model/tool change re-render | PASS | `TestPromptEpochIsStableForTheSameText`, `TestPromptEpochChangesWhenTheToolListChanges`, `TestWalkReplaceChangesOnlyTheExactServedPrompt` |
| Late-section splice, render fallback | PASS | `TestUnmatchedLateSectionIsDeliveredOnTheNextPromptOfItsBranch`, `TestRenderErrorDoesNotReplaceTheHostPrompt` |
| Frames: date every input, compaction, branch, precede user message | PASS | `TestBeforePromptRequiresAllSixContextFrames`, `TestLiveCompactionResendsContextFactsOnce`, `TestLiveResumeDoesNotResendDeliveredContextFrames` |
| Contributed model fields: wrong owner, unknown field, bad date, explicit vs estimated, stale model | PASS, after a refactor | Validation was inline in a handler that needs a live SDK context, so it had no test. It is now `contributedFieldValues`; `TestContributedFieldValuesRefuseInvalidClaims`, `TestContributedFieldsApplyOnlyToTheirModel`, `TestFrameProducerDescriptorRejectsInternalAndUnknownFields`. Delivery and rendering: `TestLiveReferenceProducer*` |
| Contributed fields: duplicate claim, unregister withdrawal, compaction repeat | PASS | Duplicate claim refused at registration (`internal/contribution/registry_test.go`); withdrawal on unregister is covered by the new withdrawal test in `internal/host`; compaction repeat rides the `session.model` frame covered by the compaction test. |
| Skills: budget ladder, load, repeated load, `when` | PASS | `TestListDropsBundledDescriptionsFirst`, `TestSkillReadLoadsAndUnloadDeclines`, `TestWhenFilesHidesUnmatchedSkill`, `TestLiveSkillsListingBudgetChangesTheCommandOutput` |
| Skills: unload withdraws revealed sub-skills | **Not met, fixed** | Unload removed only the named skill, leaving a revealed child recorded as loaded. `TestUnloadWithdrawsRevealedSubSkills`; a frame now tells the model to stop applying the withdrawn skills |
| Skills: nested name collision loads as `<parent>/<child>` | **Not met, fixed** | A nested skill whose name matched a top-level one was silently dropped, or hid the top-level one depending on walk order. `TestNestedSkillCollidingWithTopLevelNameIsQualifiedByParent` |
| Skills: replay after compaction as pointers, never fresh | **Not met, fixed** | Replay only ran on resume. `session_compact` now sends a newest-first pointer list (20 skills at most) and marks the skills replayed. `TestLiveCompactionReplaysLoadedSkillsAsPointers` fails without the handler |
| Skills: `auto_load` delivery, declined after unload | PASS | `TestLiveAutoLoadSkillIsDeliveredOnceAndStaysDeclinedAfterUnload` |
| Skills: `listing = "frame"` leaves the system prompt unchanged | PASS | `skills.listing` is a live setting; `TestLiveSkillsListingFrameMovesTheListingOutOfThePrompt`, `TestBuiltinWorkbenchLeavesTheSkillsSectionOutForFrameListing` in `internal/prompt`. |
| Instructions `select = "all"` | Slice 2 | PLAN-0003 W5 |
| Prompt A/B on fixed tasks | PASS for no regression | See the live A/B section above; per-family adapters remain Slice 3 |

# Acceptance blockers

- All fourteen specifications are approved; that approval does not close the remaining implementation or acceptance gaps.
- Declared provider mutations are wired without new permission prompts under SPEC-0004 revision 8. Provider hashline editing remains unavailable; the full ownership-row evidence mapping and slice exit remain open. Redirects implement a static-word form, not general shell redirection.
- Live evaluation ran on deepseek/deepseek-flash after the user authorized it: the prompt A/B, the tool-description suite, an end-to-end workflow, wake delivery, frames and instructions (see the review follow-up). The scripted-provider evidence above is still the proof for mechanics a live model does not exercise, such as compaction, pins and the capped-BACKLOG diff.
- `agent://by-project`, broadcast and coordinator are Slice 2. Activity-item click requires the fork host. Windows is not a supported platform: the module builds and vets for windows/amd64 (`just windows`), 14 packages pass under wine, and the resource, search, fileedit, endpoint, project-registry and secret-privacy paths fail or fail closed there.

## SPEC-0008 §8 local rows (audit)

| Row | Result | Evidence |
|---|---|---|
| Local explicit vs relative location, concurrent cd/read/edit, branch restore | PASS | `TestLiveReadLocationAndCodemode`, `TestMutationLeaseAndTransitionTimeout`, `TestRestoringEarlierFactKeepsRevisionMonotonic`, concurrent `/cd` project binding test |
| Foreground/background/promotion, cancel-after-transfer | PASS after fix | Promotion and completion were untested and completion notices did not exist; `TestLivePromotedJobWakesTheModelOnceWhenItEnds` (fails without the change) |
| Noisy streams, giant lines, quota/rotation, cursor gaps | PASS | `internal/jobs` log tests (`TestLogSegmentRotation`, `TestLogTailGapAndPaging`, `TestSlowOutputLargerThanPipe`) |
| Descendant kill, parent SIGKILL, reload/shutdown | PASS | `TestKillProcessGroup`, `TestLiveCrashReclaimsOrphanedJob`, `TestLiveSessionEndTerminatesJobs` |
| Notification not consumed by inspection | PASS | the notice is a frame delivered on the next prompt; viewing `job://` does not touch it (same test) |
| Stale PID cannot control a reused process | PASS | `TestUnverifiedIdentityNeverSignals`, `TestSweepKeepsAnotherLiveSession` |
| PTY keys, remote/SSH, worktrees | Slice 2 (W3) | not claimed |

`jobs wait until=output`, `kill` with a signal and `stream` selection are implemented. Not implemented from SPEC-0008 §6: `failedToStart` and `stopping` as distinct states; the state label set is running, exited, cancelled, timedOut.

## SPEC-0011 §10 and SPEC-0009 §10 local rows (audit)

| Row | Result | Evidence |
|---|---|---|
| Discovery, third-user and wrong-token refusal | PASS | `TestLiveTwoSessionsDiscoverEachOtherAndAnIdleReceiverWakes`, `TestWrongTokenGetsNoReplyAndNothingIsDelivered`, `TestAuthenticatedPeerReachesTheHandler` |
| Idle wake, busy steer, waitingForUser untouched | PASS | `TestLiveBusyReceiverIsSteeredNotWoken`, `TestLiveWaitingReceiverGetsTheMessageAsTheReadResult` |
| `sessionChanged`, expiry | PASS after test added | `TestLiveStaleAddressesAreRefusedWithTheCurrentSession` (protocol level) and `TestLiveARealSessionReplacementRefusesTheOldAddress` (a real `new_session` replacement: the old ID answers `sessionChanged`, the new one accepts and wakes) |
| Stale record cleanup after `kill -9` | PASS | `TestLiveACrashedSessionsRecordIsPrunedByTheNextStart` |
| maxHops, rate, blocked peers | PASS | `TestLiveAgentReplyIncrementsHopAndStopsWake`, `TestSessionMessageSettingsAffectDelivery`, `TestLiveSessionsCommandSendsRedactsAndHonoursBlock` |
| Secret typed by the sender, envelope escape | PASS | `TestLiveSecretTypedByTheSenderNeverReachesTheReceiversProvider`, frames envelope unit tests |
| Ask types, pending recovery, timeout, print | PASS | `TestLiveAsk*` set, `internal/ask` dialog and record tests |
| Secret question: vault, placeholder, usable `env` in the next bash | PASS after test added | `TestLiveAskedSecretIsUsableAsEnvInTheNextBashCall` |
| Duplicate local/remote submission, wrong generation, bridge, `/answer`, `/tldr` | Slice 2 (W6) | not claimed |
| Coordinator, broadcast, `agent://by-project` | Slice 2 | not claimed |

## SPEC-0013 §11 and SPEC-0010 §9 local rows (audit)

| Row | Result | Evidence |
|---|---|---|
| No known secret in provider requests (turns, files, commands, prompts) | PASS | `TestLiveProviderRequestsNeverCarryAKnownSecret`, `TestRedactProviderPayload*` |
| Compaction and branch-summary requests | PASS | `TestLiveCompactionRequestCarriesThePlaceholderNotTheSecret` and the captured real branch-summary request, which carries a `red-sec-` placeholder and never the token. |
| Placeholder in bash, write; transcript keeps the placeholder | PASS after test added | `TestLiveToolReceivesTheRealValueWhileTheTranscriptKeepsThePlaceholder`, `TestLiveWriteReceivesTheRealValueFromAPlaceholder` |
| Placeholder in edit, another extension's HTTP call, codemode child | PASS | `TestLiveEditSubstitutesAPlaceholderIntoTheFileAndKeepsTheTokenFromTheProvider`, `TestLiveAnotherExtensionsFetchBackendBodyIsRedactedBeforeTheProvider` (reference producer's fetch backend), `TestLiveHTTPBodyWithATokenIsRedactedBeforeTheProvider` (builtin reader), `TestLiveCodemodeChildResultsAreRedactedBeforeTheProvider` |
| Same value, same placeholder across sessions, nukii vector | PASS | `TestPlaceholderDerivationAndDetection` (independent vector), persisted key tests |
| Key or vault failure withholds visibly, recovers without restart | PASS | `TestLiveBrokenSecretsPolicySendsNothingToTheProvider`, `TestSecretsBoundaryRecoversWithoutRestartAfterAFailedBuild` |
| `ask_user` secret, env in bash and jobs | PASS | `TestLiveAskedSecretIsUsableAsEnvInTheNextBashCall`, `TestLiveBashReceivesAnInjectedEnvSecretAndItsOutputStaysRedacted` |
| Opt-in extension inference, grants | PASS | `TestSecretsRedactIsGrantedPerOwnerAndNeverEchoesInputOnFailure` |
| Throughput | MEASURED | ~1.2 MB (about 300k tokens) of 1,300 distinct messages with a 61-entry vault: 450 ms per pass before memoization (each string reloaded and decrypted the whole vault), 120 ms cold and 2 ms warm after it. The detector now caches the decrypted known-value snapshot and per-text results keyed by a vault version (size and mtime), so any append, soft delete or learn invalidates both. `TestRedactMessagesMemoizesUnchangedEntriesAgainstTheVaultVersion`, `TestMemoDoesNotOutliveAVaultChange`, `TestMemoStopsHidingAValueAfterItIsSoftDeleted`. `TestRedactMessagesLargeContextStaysBounded` keeps a loose 5 s guard; no latency budget is promised. |
| `secret://`, `.not-a-secret`, `secrets://` | Slice 2 | not claimed |
| SPEC-0010 §9 configuration rows | PASS | Phase B evidence plus `TestLiveCatalogSettingsHaveBehavioralConsumers` |

## Review follow-up: wake, tuning, frames and instructions (live)

All runs use deepseek/deepseek-flash with the key from the environment; transcripts and captured provider requests are redacted and kept under `evidence/`. Samples are small (n=1 to 3) and one model, so they show behavior, not statistics.

| Area | Result | Evidence |
|---|---|---|
| Frame wake | `wake` is a publish flag (SPEC-0002 rev 9); a finished background job wakes an idle model, a `wake=false` frame never starts a turn and reaches the model with the next prompt, `wake=true` starts one | `TestLiveEvalFinishedJobWakesTheModelAndAQuietFrameDoesNot`; `jobs.wake-on-completion` (default true) |
| Tool, parameter and error text | Tuned only against observed DeepSeek failures: tool errors 21 to 4, calls 173 to 131, shell reads 14 to 1, 66 of 66 tasks pass, held-out tasks 0 errors | `liveeval_tools_test.go`, commit `b00f7a9` |
| Frames inject once and deduplicate | The startup notice appears exactly once in every request over five turns; a duplicate publish is delivered once; a restarted session replays history without repeating a frame; a fork does not inherit a frame delivered only on the abandoned branch | `TestLiveEvalFramesInjectOnceAndDeduplicate`, `evidence/live-frames/` |
| Frames are useful and untrusted data does not steer | 9 of 9: a relevant frame is used unprompted, an irrelevant one neither derails nor leaks, an injected instruction in untrusted frame data is not obeyed | `TestLiveEvalFramesAreUsefulAndUntrustedFramesDoNotSteerTheModel` |
| Instructions inject, update and are obeyed | Root AGENTS.md once with no host duplicate; nested rule once on touch and obeyed; an external edit arrives as one diff update and is obeyed; root BACKLOG.md pointers arrive once from the first request and are used | `TestLiveEvalInstructionsInjectOnceDiffAndAreObeyed`, `evidence/live-instructions/` |

Defects the live runs found, each fixed with a regression test that failed first:

- **Fork deadlock.** `fork` (and any session replacement) hung forever when a contributed status segment existed, because `statusBoard.close` called the host through the retained session-start context. It now uses the live shutdown context. `TestLiveForkWithAContributedStatusSegmentDoesNotDeadlock`.
- **Nested instructions withdrawn every prompt.** A nested AGENTS.md delivered by a touch was withdrawn at the next prompt and re-delivered on the next touch, costing a blocked write and a duplicate view. SPEC-0007 retains the most recent completed target set; the host now does until the location, the context epoch or the next explicit target changes. `TestLiveNestedInstructionsSurviveTheNextPrompt`.
- **Untested refusal.** A malformed `expectedRevision` was refused with a helpful message but no test; `TestLiveWriteRevisionAndLs` now covers it and uses a well-formed wrong revision for the stale case.

Still not evaluated with a live model: pins, the capped-BACKLOG diff, compaction with instructions, and a bash-capable injection against frame data (only `read` was available in that scenario).

## Defects reported in live use (09.11.2026)

Each was reproduced or covered by a test that failed before the fix.

| Report | Fix | Evidence |
|---|---|---|
| A submitted prompt vanished with "Secrets boundary unavailable" | An undecryptable vault record is ignored with one notice; any other boundary failure returns the prompt to the editor; a user-owned `secret.key` with a wide mode is narrowed to `0600` (SPEC-0013 rev 8, 9) | `TestLiveSubmittedPromptSurvivesAnUnavailableSecretsBoundary`, `TestLiveSubmittedPromptIsSentDespiteAVaultRecordFromAnotherKey`, `TestLiveWideSecretKeyIsNarrowedAndThePromptIsSent` |
| Root AGENTS.md withdrawn and re-delivered when a path outside the project was touched | The active scope is the union of cwd, touched and pinned scopes (SPEC-0007); a boundary notice is withdrawn on return | two live regression tests in `live_scripted_test.go` |
| Frame nonce changed with every frame | One nonce per session lineage, persisted and inherited by forks and branches | commit 7924152 |
| `session.model` lacked the knowledge cutoff and the stale-data instruction | Attributed cutoff from a models.dev snapshot, a `models.knowledge-cutoffs` setting and always-present guidance | `internal/modelknowledge`, `model_frame_test.go` |
| `read ...` rows and the "no retained snapshot" edit failure | Call rows render path and selector; the error names the recovery | live and unit regression tests |
| No commented `Project.toml`, no way to create or select a project | `docs/projects.md` embeds the parsed example; `/workbench projects` panel and commands; one offer per repository | `TestLiveProjectsPanelCreatesAProjectAndTheFrameNamesIt`, `TestLiveProjectsPanelJoinsARegisteredProjectByMarker`, `TestLiveRepositoryInNoProjectIsOfferedAProjectOnce`, captures in `evidence/projects-panel-tui/` |

Gates at this point: `just check`, `just race` and `just integration` pass. The offer has no setting to switch it off, and it is not shown in print mode.
