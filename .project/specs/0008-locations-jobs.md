---
document:
  type: "specification"
  id: "SPEC-0008"
  title: "Working locations, SSH, worktrees and jobs"
  description: "Location leases, safe remote execution, shell process ownership, logs and unified job controls"
  status: "published"
  revision: 12
  created_at: "2026-11-02T16:17:52+01:00"
  updated_at: "2026-11-10T11:30:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 12
      reason: "Consumed analyses and the proposal draft are deleted; their facts are stated in place"
      changes: ["Links to deleted analyses replaced by the fact or its new location"]
---

# 1. Scope and references

Contract for cd/bash/jobs, /remote and working-location/job services. Depends on [services](0001-extension-services.md), [frames](0002-message-frames.md), [resources](0004-resource-providers.md), [ambient context](0007-ambient-context.md) and [persistence/trust](0010-configuration-state-trust.md).

Prior art: `host:/mnt/Projects/furbyhaxx/pi-workbench/src/core/{location,ssh,jobs,job-controls,retained-jobs}.ts` and its index.ts command wiring. Donor Jobs distinguishes exitCode, signal, timedOut, background and reported state; controls do not acknowledge model completion notifications just because a user viewed the panel. Workbench preserves those distinctions and strengthens crash/remote-input guarantees with explicit capability reporting.

No subagent/process-as-agent abstraction. Jobs are shell processes owned by this session, not task records.

# 2. WorkingLocation and capture

WorkingLocation fields: authority (local or SSH endpoint identity), canonical cwd, display cwd, projectRoot, origin, monotonically increasing revision and branch binding. Endpoint identity includes user, hostname, port and selected SSH config identity; two aliases are not assumed identical merely because they display the same hostname.

Every tool captures one immutable location/config revision at admission. Relative paths/commands resolve there for the entire call, including nested work. Explicit file:/// and ssh:// URIs choose their own authority and never inherit a mismatched cwd. Do not call os.Chdir or mutate PiG host cwd.

Delegation to host builtins (host probes (SPEC-0001 §9) P18): the `cwd` passed to `sdk.Create*ToolDefinition` is ignored whenever a context exists — host tools always use the session cwd. Local file tools therefore resolve relative paths to absolute paths under the captured location before delegating, and local `bash` sets `BashSpawnContext.Cwd` through `BashToolOptions.SpawnHook`. Both were verified directly and from codemode children; `ctx.Cwd()` stays the session cwd throughout.

Default cwd/origin is host session cwd. Branch entries preserve location transitions; resume validates availability before activation. An unavailable stored location is displayed as unavailable and blocks relative execution until explicit reset/change. Do not silently execute locally after an SSH failure.

Leases: foreground filesystem operations lease captured location. Transition waits for current mutating leases only within a bounded deadline; if busy, return locationBusy and the conflicting operation IDs, not indefinite waiting. Read leases may finish under prior revision; their results remain labeled. A running background job keeps its launch snapshot and does not hold the current cwd hostage. Codemode batch children use the same captured batch location unless the script explicitly invokes a completed cd between sequential calls; parallel cd with relative mutations is rejected as ambiguous.

# 3. cd and worktrees

Tool shapes (mutually exclusive):
- `{}`: status only.
- `{"path":"..."}`: local/SSH absolute or captured-location-relative destination.
- `{"branch":"feat/parser"}`: create/reuse worktree in current repository.
- `{"reset":true}`: return to original location.

Commands /cd and /wb:cd use the same parser/service; no arguments opens picker in TUI, returns status in RPC/noninteractive surfaces. /remote off returns to local origin, not arbitrary process cwd.

Transition stages:
```text
validate destination → authorize → acquire transition lease → resolve availability
 → prepare instruction reconciliation → persist transition → activate revision → publish event
```
Any pre-activation failure leaves old location unchanged. Persistence failure is not a successful cd. Post-commit notification failure is an inspectable delivery failure, not cause to perform the transition twice. Emit versioned location.changed with previous/current/origin/revision/reason through the shared event service.

Worktrees:
- Discover repository via git argument arrays; parse porcelain -z output.
- Reuse an existing worktree for the exact branch. Never force-checkout a branch already checked out elsewhere.
- Otherwise create under `<repo>/.worktrees/<sanitized-branch>`, collision-resistant suffix when different branches sanitize alike; show branch→path mapping.
- A new worktree gets the shared artifact tree and `[worktrees] links` of SPEC-0012 §4.3–4.5 in the order specified there; a failure in that step is reported and leaves the worktree usable as a plain checkout.
- Validate branch with git check-ref-format, reject unsafe path components, never interpolate branch into shell syntax.
- Existing directory not registered as intended worktree → conflict, not delete/reuse.
- New branch base defaults to current HEAD, shown before user-driven creation; tool invocation explicitly authorizes only requested branch creation.
- Dirty source is not reset/stashed. Reuse dirty worktree reports status and preserves all work.
- Failed/cancelled creation reconciles whether git actually created the worktree; no blind directory removal.
- Workbench does not commit, push, prune or remove worktrees as part of cd.

# 4. SSH transport and host manager

Use installed SSH client/config semantics rather than implementing SSH authentication. Respect host-key checking, ProxyJump/Include/identity/port options; never add StrictHostKeyChecking=no to avoid a blocker. Connection prompts require an appropriate user-facing channel; headless runs fail explicitly if interaction is unavailable.

Shared transport lifecycle is session-owned and endpoint-keyed. Multiplexing may reuse ControlMaster with private short control paths. Shut down only control masters created/owned by Workbench; do not terminate a user's existing shared master.

Remote command composition uses a fixed transport program plus safely quoted positional data; script text is the explicitly authorized bash payload, not concatenated path/user values. Binary file transfer does not pass bytes through shell interpolation. Remote shell detection and user-selected shell are explicit; unsupported shell semantics produce actionable errors.

Remote read/write/edit/search follow the same resource and snapshot contracts. Obtain revision/content identity before and immediately before writes; stage in same remote directory and rename when supported. Transport loss after dispatch may yield outcomeUnknown. No download/edit/blind-upload claim of CAS.

Host config manager /remote:
- list/select alias; direct alias[:/path], off, reload, add, config.
- Per-host show, note, shell, hide/unhide, rename.
- Parse ~/.ssh/config and Includes while preserving comments, ordering and untouched blocks.
- Default additions use a user-approved dedicated include file; changing main config to include it is a separate previewed write.
- In-place edits only for uniquely identified owned/supported blocks; complex Match/Include arrangements stay inspectable without pretending full safe rewrite support.
- Rename previews affected alias references and notes; never rewrite unrelated aliases or known_hosts automatically.
- Host notes are Markdown with round-trip-preserved TOML metadata for description/shell/hidden; they are not arbitrary executable configuration.
- Remote project configuration is not auto-loaded as local executable policy; note/instruction trust is explicit.

# 5. Bash execution and supervision

Bash request: command (required), optional timeout seconds, background boolean, title, cwd override and pty boolean. Command is intentionally shell source; cwd/title/transport parameters are data. Cwd override uses resource authorization and does not change session location. No generic environment dump in Details.

Default foreground, foreground timeout 120 s, no automatic promotion unless configured yieldAfterMs >0. Explicit background jobs have no implicit lifetime timeout; a supplied timeout still applies. Promotion preserves the original explicit/deadline policy rather than silently extending it. Explicit background returns after successful spawn/admission; result says running, not succeeded. Foreground promotion transfers ownership once to session supervisor and returns the same job ID, not a second process. Caller cancellation before transfer kills the group; after background receipt it cancels the request, not an already acknowledged detached job.

Process launch:
1. Authorize command/location and load cwd instructions.
2. Reserve job ID/log/quota and private launch metadata.
3. Spawn dedicated process group (or platform job object), attach pipe/PTY.
4. Establish crash-cleanup supervisor before acknowledging background success.
5. Drain stdout/stderr without deadlock, journal terminal facts, close descriptors and reap.

Backends (approved 02.11.2026): PTY jobs use `github.com/aymanbagabas/go-pty` (Ctrl-C, EOF, SIGWINCH verified); pipe jobs expose close-stdin and signal as distinct operations because control bytes on a pipe are only data. Host-delegated foreground `bash` already kills its child on caller abort (P17).

Hard timeout terminates owned process group with grace (default 2 s) then escalates. Normal cancellation has the same bounded cleanup. A process that daemonizes out of the group is an explicit platform/supervision gap to test; no unsupported universal “no orphans” claim.

Crash watchdog must own a live parent-liveness primitive and verifiable child identity, not a stale persisted PID. Local SIGKILL and remote SSH disconnect tests prove cleanup. Killing an SSH client does not prove remote descendants died; a remote supervisor/liveness mechanism is required for that guarantee. Dependency/remote deployment changes need approval. If crash cleanup is unavailable, report capability and fail background admission when the configured guarantee requires it.

# 6. Job state and unified tool

State machine:
```text
reserved → starting → running → stopping → exited
                    │    └──────────────→ exited
                    └→ failedToStart
running/starting --lost ownership evidence--> unknown
```
Terminal reason separately records normal, nonzeroExit, killed, timedOut, cancelled or interrupted. ExitCode and signal remain nullable. No invented exit code 0 for a lost process.

`jobs` action schema:
| Action | Arguments/defaults | Result |
|---|---|---|
| list | maxFinished default 20, bounded cursor | All active + retained finished; explicit omitted count |
| status | id, lines default 40 | Facts plus bounded tail |
| output | id; exactly one tailLines or cursor; stream selection | Output page + next cursor/gap facts |
| wait | id optional (any active), until output|exit, timeout default 30 s/max 300 s, cursor for output | changed/timedOutWait/current facts; wait expiry is not job failure |
| write | id; input and/or keys[] or eof | Accepted bytes/keys, current stdin status |
| kill | id, signal default TERM; approved force option | Signal request receipt and observed state; killed only after evidence |

IDs are stable session-scoped identities; human names are accepted in commands only with disambiguation. Foreign-session IDs cannot be controlled by guessing. Viewing a UI/job URI does not mark a model notification delivered.

Input:
- pipe: Enter newline, Tab tab, ordinary input bytes; EOF closes stdin; Ctrl+C is explicit interrupt signal action, not byte 0x03.
- PTY: key sequences follow the negotiated terminal backend; Ctrl+D sends terminal EOF character, distinct from forcibly closing transport.
- Up/Down/Esc require PTY/key capability; reject unsupported piped key requests with alternatives.
- Input limit default 64 KiB per request; reject writes after stdin closed/exited and handle partial acceptance honestly.

# 7. Logs, URIs and notification policy

One supervisor serves the jobs tool, /workbench jobs, /wb:jobs and the `job://` tree (SPEC-0012 §5.3):
- `ls job://` catalog; `job://<id>/status`, `cmdline`, `cwd`; `log`, `stdout`, `stderr` paged and grep-able; write `stdin`; write `ctl` with `eof`, `kill` or `signal <name>`.
- Same authorization and identity checks on every path; URIs cannot bypass approval.

Log storage is private session-owned state. Default per-job 10 MiB retained output, per-session 100 MiB; bounded rolling segments with monotonic cursor (job ID, stream, segment/generation, offset). Rotation reports lost byte intervals. No quota-triggered process blocking/deadlock; continue draining and record loss. Order across stdout/stderr is observed ingestion order, not a guaranteed global write order. UTF-8 chunk boundaries require stateful decoding.

Logs keep raw output, including secret values a command prints; output read back through the jobs tool or `job://` passes the SPEC-0013 boundary like any tool result. Terminal controls are sanitized for display without interpreting them as UI commands.

Completion emits one frame keyed by job ID/terminal revision. The frame is published with `wake` (SPEC-0002 §3): an idle session starts a turn, a running one receives it at the next boundary. `jobs.wake-on-completion` (default true) turns the wake off; the notice is then delivered with the next prompt. A job killed on request emits no frame, and neither does a job whose terminal state a `jobs` result (wait, status or output) already showed the model; a `jobs wait` still in flight when the job ends is settled first. UI viewing does not consume completion notification state. Output progress is coalesced, not one frame per line.

Reload/shutdown default: terminate owned jobs and record final/interrupted state. Resume shows retained jobs/logs but never relaunches commands or controls old PIDs. Surviving-job adoption requires a separately proven broker capability and explicit policy, not ordinary metadata replay. Process lifetime is session-wide; branch navigation only changes visibility/provenance, not execution.

# 8. Acceptance and platform gates

- Local/SSH explicit URI vs relative location, concurrent cd/read/edit, branch restore and unavailable remote refusal.
- Worktree existing/new/collision/dirty/checked-out branch, cancellation and no unrelated cleanup.
- SSH Include/Match preservation, hostile paths/aliases, host-key refusal, auth prompt unavailable, disconnect mid-write.
- Foreground/background/promotion ownership race, spawn failure, output-before-ready and cancel-after-transfer.
- Pipe vs PTY keys/EOF/signals, partial writes, closed stdin and output/wait timeout distinction.
- Noisy dual streams, giant lines, disk quota/rotation and cursor gaps.
- Kill local process descendants, remote watchdog loss, parent SIGKILL, reload/shutdown; document observed platform guarantees.
- Same results through tools/URIs/panels, no notification acknowledgement merely by inspection.
- Retained stale PID cannot control a reused process; unknown outcomes stay unknown.

POSIX local+SSH is the primary target. Windows requires job-object/process and path implementations before claiming equivalent support; absence is an explicit release matrix decision. PTY/watchdog libraries and any remote helper installation remain approval gates, not silent dependencies.

# 9. Donor parity (M0.1)

From the donor test audit of 02.11.2026, pi-workbench `core/*` at `bc7ce0e` and oh-my-pi `bash-*` tests. Normative.

**Jobs and output**
- **G8.1 Secrets** (SPEC-0013). Placeholders in command text, cwd, stdin writes and job input are substituted right before execution; the displayed and stored call keeps them. Secrets with an `env` name in the session's scope chain are injected into the environment of bash calls and jobs, local and SSH. Results go back through the SPEC-0013 boundary.
- **G8.2** Non-interactive commands get EOF on stdin instead of hanging.
- **G8.3** Aborting a `jobs wait` call leaves the job running.
- **G8.4** Shutdown closes admission first, coalesces kill attempts, reports failed and unfinished kills, and permits a retry.
- **G8.11 Jobs panel.**
  - The monitor follows a bounded, sanitised tail and supports pause and filter. An evicted log is reported.
  - Stop confirmation runs exactly once, even on a double click. A write draft is sent explicitly, signals are labelled, and a closed stdin is refused.
  - An ambiguous selector opens a picker. Session abort closes the screens and leaves the job untouched.

**Locations and worktrees**
- **G8.5 cd to a branch.**
  - Headless with a missing branch: return the exact creation command and change nothing. Interactive: confirm, then create.
  - A branch created by this command is deleted again if worktree creation fails or is cancelled. Pre-existing branches are never touched.
  - Dismissal during creation waits for the rollback and never switches.
- **G8.6 Worktrees.**
  - The `.worktrees` parent must be git-ignored. If not, fail with the exact fix and create nothing. An absolute external parent is configurable (`locations.worktree-parent`).
  - A bare repository is not a checkout. Prunable worktrees are rejected; locked and detached ones are allowed. Paths containing newlines work.
  - Git runs in its own process group, killed on timeout or abort.
- **G8.14** Forks and tree branches keep their own current location and share the origin. A session without records inherits a snapshot or establishes the launch origin. Returned locations are immutable.

**SSH and hosts**
- **G8.7 SSH config.**
  - Discovery: aliases through relative, quoted, glob, nested and cyclic Includes. A fingerprint over the config and all included files drives reload.
  - Writes preserve CRLF and mixed line endings and follow config/include symlinks while keeping modes, including a dangling include target. A stale Host line is refused. An existing wildcard Include is reused; a new Include goes after the header and before the defaults.
  - Injected directives, invalid names or ports, and collisions are rejected. Rename edits the included file and keeps alternative aliases.
- **G8.8 Remote shell.** Bash regardless of `$SHELL`. Fall back to `sh` only when bash is absent, and never retry a command that failed. An explicit per-host shell has no fallback. Shell values must be absolute paths with no flags or shell syntax.
- **G8.9 Host notes** live at `<state>/hosts/<name>.md` with TOML frontmatter. Edits keep the body and other keys, follow symlinks, and remove a file that becomes empty. Rename moves the note and refuses to overwrite one. Unparsable frontmatter contributes nothing and is reported.
- **G8.10** Host descriptions and SSH definitions are sanitised against terminal-control injection.

**Bash**
- **G8.15 Command routing guard** (`bash.route-to-tools`, default on). A standalone `cat file`, a recursive grep, `find`, or an unquoted `> file` / heredoc write is refused with a suggestion to use read/grep/find/write. Pipe stages consuming stdin, `/dev` sinks, and quoted or commented text are not matched. The guard never blocks when the suggested tool is unavailable. Both the original and the leading-`cd`-normalised command are checked.
- **G8.16** Terminal graphics (Sixel, kitty) are extracted before sanitisation and kept even on timeout or failure.
- **G8.17** Non-interactive environment defaults: pager off, no prompts. `exec` cannot replace the supervisor. A grandchild holding stdout open does not block completion. Grandchildren are killed on timeout and abort.
- **G8.18 Result shape.** A non-zero exit is an error result with details, not a thrown tool failure. A timeout is a warning state with one notice; cancellation is distinct from timeout. A pipeline ending in `head`/`tail` keeps the final-stage output. Late results never flush into a replacement session or branch.
- **G8.19** The cwd may be a locatable URI (`skill://`, `local://`) and resolves to its backing path. A non-locatable or read-only scheme as cwd or redirect target fails explicitly. Shell text is never rewritten, except for SPEC-0013 placeholder substitution.
- **G8.20 Scheme commands.** A standalone `ls`, `cat`, `rm`, `mv`, `cp` or `mkdir` (or a `>` redirect) whose path arguments use a registered non-locatable scheme (e.g. `rm secret://global/old-token`) runs in-process through the resource router with that provider's approval rules, and returns shell-like output and exit status. Any other command with such a path fails with an error naming the supported forms. Scheme paths in commands for an `ssh://` cwd fail explicitly.
