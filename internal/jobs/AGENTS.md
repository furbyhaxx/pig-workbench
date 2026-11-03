# jobs

Local process supervision for session-owned jobs; no SDK dependencies.
- `Process` owns its direct child and drains combined stdout and stderr to the caller's writer. The writer must return and must not call back into the process. Unclosed inherited output pipes report `ErrOutputIncomplete`.
- `Spec.Stdin` opens a writable pipe; otherwise stdin receives EOF. `CloseStdin`, `Kill`, and `Close` close it and unblock concurrent writes. Do not hold the lifecycle mutex across a write. The first `Close` reports recorded errors; later calls return nil without work.
- Unix jobs use dedicated process groups. Escalate before reaping: the zombie leader pins the PGID. After reaping, `Kill` and `Signal` refuse to touch the PGID. Descendants that leave the group are outside this guarantee. Group disappearance is checked after reaping and failure is reported.
- Linux `waitid(WNOWAIT)`, Darwin kqueue plus a zombie check, and Windows process handles observe exit without reaping. Other platforms reject startup with `ErrWatcherUnsupported`; never pretend safe escalation is available there.
- Windows group termination is unsupported and reports `ErrGroupKillUnsupported` after terminating the direct child.
- `LogStore` owns raw output bytes. Readers use immutable snapshots; they never hold the writer lock. Pages are at most 64 KiB, and cursor gaps identify the first retained byte.
- Session quota evicts whole logs FIFO by the start of the current retention window, breaking ties by job ID; the appending job is protected and tail-capped to the smaller quota. `Facts` reports evicted logs. Appending after eviction or `Drop` resumes at the old offset with any loss recorded.
- Dropped and evicted IDs retain offset metadata until the store closes; the byte quotas do not bound that metadata.
- `Manager` owns background admission, processes and logs. Request contexts end at admission; `MaxActive` bounds in-flight and active jobs, not retained states or tombstones.
- On natural leader exit, kill the remaining group before reaping; `JobState.Cancelled` records whether cancellation or shutdown won before observed exit. Windows reports group termination unsupported even on natural exit.
- Watcher failure leaves `Running=true` and `Exit=nil` because termination is unproven; report `ErrJobCleanupIncomplete`. This path cannot guarantee child or descendant cleanup and blocks orphan-free shutdown claims.
- The package does not provide foreground promotion, stdin forwarding, finished-job retention, or the jobs tool.
