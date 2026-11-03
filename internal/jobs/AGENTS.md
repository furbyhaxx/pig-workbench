# jobs

Local process supervision for session-owned jobs; no SDK dependencies.
- `Process` owns its direct child and drains combined stdout and stderr to the caller's writer. The writer must return and must not call back into the process. Unclosed inherited output pipes report `ErrOutputIncomplete`.
- Unix jobs use dedicated process groups. Escalate before reaping: the zombie leader pins the PGID. After reaping, `Kill` and `Signal` refuse to touch the PGID. Descendants that leave the group are outside this guarantee. Group disappearance is checked after reaping and failure is reported.
- Linux `waitid(WNOWAIT)`, Darwin kqueue plus a zombie check, and Windows process handles observe exit without reaping. Other platforms reject startup with `ErrWatcherUnsupported`; never pretend safe escalation is available there.
- Windows group termination is unsupported and reports `ErrGroupKillUnsupported` after terminating the direct child.
- The package does not provide job admission, IDs, log retention, or the jobs tool.
