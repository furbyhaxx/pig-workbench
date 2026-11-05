---
document:
  type: "analysis"
  id: "ANA-2026-11-05-slice-1-probes"
  title: "Phase A probes: idle/busy wake, session state, titles T1–T3, resumed cwd, model credentials, annotation order"
  description: "Observed host behaviour for the open Phase A probe rows, run against the current PiG binary with a scripted provider"
  status: "published"
  revision: 2
  created_at: "2026-11-05T17:05:00+01:00"
  updated_at: "2026-11-05T18:40:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Phase A ledger rows required observed results instead of assumptions"
      changes: ["Ran eight probes through real pig RPC/JSON runs with a scripted provider", "Recorded the one remaining coverage gap (native custom resolver)", "Added the runnable probes as integration tests"]
    - revision: 2
      reason: "Reload probe found that /reload never delivers session_shutdown"
      changes: ["Recorded the reload finding and its consequence for shutdown-time cleanup", "Added a regression guard for the host behaviour"]
---

# Method

Every probe runs the real `pig` binary as a subprocess in RPC or JSON mode with an isolated `PIG_HOME`,
agent dir and XDG roots, and a localhost scripted inference provider. No billed inference is used.

- Probe fixture: `internal/host/testdata/probe-agent/` — `/probe wake`, `wake-status`, `name`, `clear`,
  `title`, `entries`, `ids`, `cwd`, `state`, `label`, plus tool `probe_wake`.
- Runnable cases: `internal/host/live_probes_test.go` (build tag `integration`).
- Run: `go test -race -tags=integration -modfile=.tmp/go.mod ./internal/host -run TestProbe`.

# Results

| Probe | Result | Evidence |
|---|---|---|
| Idle wake from a background goroutine | **Works.** `SendMessage(TriggerTurn:true, DeliverAs:"followUp")` from a goroutine using the `session_start` context starts a new turn in an idle session, and the wake text reaches the model. | `TestProbeIdleWakeStartsTurn` |
| Busy session wake | **Steers.** A wake scheduled by a tool during a running turn reaches the model at the next boundary of that same turn. | `TestProbeBusyWakeSteers` |
| Idle/busy state (`ctx.IsIdle`) | **Accurate.** False while a turn streams, true after an aborted turn settles. | `TestProbeIdleStateTransitions` |
| **T1** `SetSessionName` visibility | **Immediate.** `GetSessionName` and the replicated `ctx.SessionName()` both return the new value without a reload. An empty name clears the title (`GetSessionName` → nil). | `TestProbeSessionTitleT1` |
| **T2** title persistence | **Persisted as `session_info`.** After a turn, the session JSONL contains a `session_info` entry carrying the title, and `--continue` on the same session restores it. | `TestProbeSessionInfoT2` |
| **T3** session_info on an inactive branch | **Branch-scoped.** Forking before the rename leaves `GetSessionName` → nil: an entry that is not on the active branch does not name the session. | `TestProbeSessionInfoBranchT3` |
| Resumed cwd through symlinked buckets | **Session cwd wins.** With both buckets symlinked to one project directory, `--continue` in `clone` resumes the session recorded in `repo`, and `ctx.Cwd()` is `repo` — not the process cwd. `sessionFile` is reported through the current checkout's bucket path. | `TestProbeResumedCwdAcrossSymlinkedBuckets` |
| `models.json` credential forms | **All expand.** A literal, `$ENV` and a leading `!command` each reach the wire as `Authorization: Bearer <resolved value>`; for `!command` the command's output is used. | `TestProbeNativeCredentialExpansion` |
| Trailing annotation order | **Order-independent.** An intact block grants seen whether the annotation block is appended before or after it; a block altered in place grants nothing. | `internal/host/snapshots_test.go` `TestAnnotationOrderCannotChangeSeenVisibility` |
| `/reload` and `session_shutdown` | **No shutdown event.** `/reload` builds a fresh runtime but never delivers `session_shutdown` to the outgoing one, so shutdown-time cleanup does not run on reload. An in-process `sleep 300` background job started before the reload was still running 10s after it. | `TestProbeReloadSkipsSessionShutdown`, live reload job probe |
| `RegisterNativeProvider` custom resolver | **Not probed — coverage gap.** The API is host-internal; no extension surface reaches it, so Workbench cannot observe whether a custom resolver returns a literal `!command`. Recorded as a gap, not as detection coverage. | — |

# Consequences for the contracts

1. **L1 wake policy is viable as specified.** `followUp` + `TriggerTurn` is the documented wake path and works
   from a retained context in a goroutine. No host change is needed for idle wake or busy steer.
2. **State tracking can be derived from `agent_start`/`agent_end`.** `ctx.IsIdle` already reflects an aborted
   turn; endpoint records can be written from the same transitions without a second source of truth.
3. **Titles are the PiG session name, branch-scoped.** `SetSessionName` is sufficient for `session://<id>/title`;
   `by-name/` must resolve the *active branch's* latest `session_info`, and a fork before a rename legitimately
   loses the name. SPEC-0012 §6's open rows T1–T3 are settled by these results.
4. **SPEC-0012 §3.5's open follow-up is answered.** The cwd a resumed session uses is the recorded session cwd,
   so a shared project bucket does not silently move the agent's working directory to the checkout that ran
   `--continue`. Workbench's captured location therefore follows the session, not the process.
5. **Secret inventory must cover command-backed credentials.** `!command` resolves through the host for
   `models.json`; the inventory must treat both the literal command text and its output as credential material,
   and must not execute the command itself. The native-resolver path stays an explicit gap (SPEC-0013 §2).
6. **The trailing-annotation contract needs no ordering assumption.** A verbatim-block presence check is enough
   for both handler orders.
7. **Reload cleanup cannot use `session_shutdown`.** Because the outgoing runtime never sees the event, job
   termination, scratch removal and similar work must either be reconciled on the next `session_start` or be
   tied to a host teardown hook that does not exist yet. The concrete defect: a background job survives
   `/reload` until its own timeout. Tracked as Phase H work with a persisted job-process registry
   (generation-tagged pid/pgid records, remnant killed on the next start), which also covers `kill -9`
   crash ownership.
