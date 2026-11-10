---
document:
  type: "analysis"
  id: "ANA-2026-11-02-host-integration-probes"
  title: "M0 host integration probes"
  description: "Runtime evidence for bus request/reply and cancellation, frame delivery, codemode child observation, renderer resolution, branch lifecycle and reload against the running PiG host"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T17:45:00+01:00"
  updated_at: "2026-11-02T18:40:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 2
      reason: "User asked to run the remaining M0.2 probes"
      changes: ["Added builtin override, location, idle delivery, stale delivery, compaction, resume, context-file suppression, catalogs, command/flag and dialog probes (P15-P30)", "Captured reload hang root cause (host stack)", "Updated implications, open items and upstream issues"]
---

# 1. Scope and method

Answers every PLAN-0001 M0.2 probe row except kit views (see §5). Every claim below is backed by a run in `evidence/runs/`; nothing here is inferred from SDK source alone.

- Host: `pig` from `PATH`, staged SDK `host:$HOME/.pig/state/pigsdk/sdk-899c7964784a0050`, isolated `PIG_HOME` and work dir under `/tmp`, `--no-session --offline`.
- Model: scripted mock provider (`evidence/harness/mock.py`), no billed inference. Scenario `s_*.json` = model script, `st_*.json` = RPC stdin script.
- Extensions: `evidence/extensions/probe-wb` (Workbench stand-in), `probe-producer` (third-party producer, packed into the same process), `probe-producer-iso` (same producer as an isolated Go binary, separate PID) and `probe-m0b` (builtin overrides, location, delivery, catalogs, flag, dialogs; behaviour toggled by `PROBE_*` env).
- Drivers: `run.py`/`run2.py` (RPC mode, captures `rpc.jsonl`, `requests.jsonl`, `probe.jsonl`), `run3.py` (adds automatic RPC dialog answers via `UI_ANSWERS`), `reloadhunt.py` (reload loop with SIGQUIT stack capture on hang), tmux for TUI captures.

Re-run: build each extension, start `mock.py <port> <scenario> <out>`, then `run.py <name> <s_*.json> <st_*.json>` with `PRODUCER=` selecting the packed or isolated producer.

# 2. Results

| # | Seam | Verdict | Evidence |
|---|---|---|---|
| P1 | Bus request/reply, packed | Works synchronously: reply listener runs inside the receiver's Emit, before the caller's Emit returns | `runs/bus` |
| P2 | Bus request/reply, isolated process | Same ordering across PIDs | `runs/bus-isolated` |
| P3 | Caller abort during a pending Emit | Caller's Emit returns `host call events.emit cancelled with its parent request`; the **listener keeps running** to completion and its late reply is still delivered | `runs/abort`, `runs/abort-isolated` |
| P4 | Explicit cancel message | A second Emit (`wb.cancel`) after abort reaches the busy listener (both packed and isolated); listener stops within one poll interval | `runs/cancel`, `runs/cancel-isolated` |
| P5 | Frame delivery modes | See §3 | `runs/inject` |
| P6 | Codemode child observation | Children emit `tool_execution_start/end` and `tool_call` with `parentToolCallId`; IDs are `call_0_0/1`… | `runs/codemode` |
| P7 | Blocked codemode child | `tool_call` block → no `tool_result` event, but `tool_execution_end` fires; script sees `{"status":"rejected","reason":{}}` — block reason lost | `runs/codemode` |
| P8 | Renderer resolver | Resolver claims foreign (`producer_echo`) and builtin (`read`) tools; render width = terminal − 2 (98 @100, 38 @40) | `runs/tui` |
| P9 | Codemode nested rows | Codemode draws children itself; our resolver is **not** consulted for nested rows | `runs/tui` |
| P10 | Dynamic renderer claim | Resolver result is cached per tool; changing the claim at runtime has no effect until `/reload` | `runs/claim` |
| P11 | Claim after `/reload` | Resolver re-runs, history re-renders with new renderer; the historical call row received **empty args** (`map[]`) while the live row had args (observed in one run) | `runs/claim-reload` |
| P12 | Branch-scoped custom entries | `AppendEntry` entries follow the branch: tree navigation drops later entries, clone keeps the branch, `new_session` starts empty | `runs/lifecycle` |
| P13 | `ctx.Reload()` from a command | Factory re-invoked, call returns, branch state intact in 27/28 runs. **1/28 hang**: no `loaded`, no return, no RPC response; old instance kept serving commands; root cause in P29 | `runs/reload-loop.txt`, `runs/reload-hang` |
| P14 | Package globals across reload | Survive (same process); not a reset point | `runs/claim-reload` |
| P15 | Same-name override of `read`/`bash` | `RegisterTool` with the builtin's name replaces it (source = extension); wrapping `def.Execute` delegates to the host original without recursion, directly and from codemode children | `runs/override-plain` |
| P16 | Update streaming through override | `tool_execution_update` events still flow (11 for a 3-tick bash) | `runs/override-plain` |
| P17 | Cancellation through override | Abort cancels the delegated call (`host call builtinTool.execute cancelled…`), kills the shell child (no leftover process) and ends the row as error | `runs/override-abort` |
| P18 | Location without host chdir | The factory `cwd` argument is ignored when a context exists (tools use the session cwd). Resolving relative paths before delegating (`read`) and `BashToolOptions.SpawnHook` setting `Cwd` (`bash`) work; `ctx.Cwd()` stays the session cwd | `runs/override-plain`, `runs/override-fixed` |
| P19 | Idle delivery | While idle, `steer`, `followUp` and unknown `deliverAs` values append a persisted custom message without starting a turn; `nextTurn` stays in an in-memory queue until the next prompt; `TriggerTurn: true` starts a turn; `SendUserMessage(…, "followUp")` starts a turn; an invalid `deliverAs` is not rejected | `runs/idle-*`, `runs/followup` |
| P20 | Stale delivery after tree navigation | A `nextTurn` message queued before `NavigateTree` is delivered on the **new** branch; an idle `steer` stays on the old branch | `runs/stale` |
| P21 | Compaction | `session_before_compact`/`session_compact` fire; summary requests do **not** pass through the `context` hook; afterwards the hook sees `compactionSummary` + kept messages, and the re-added ambient message appears exactly once per request. Custom entries survive on the branch. Small sessions fail with `Nothing to compact` (reported only via `OnError`/`compaction_end`) | `runs/compact` |
| P22 | Resume (`--continue`) | `session_start.reason` is `startup`, not `resume`; branch, compaction and custom entries are restored; context files reload from the cwd | `runs/resume` |
| P23 | Host context-file suppression | Removing a file from `data["systemPromptOptions"]["contextFiles"]` in `before_agent_start` removes it from the rendered system prompt for that prompt (must be repeated every prompt). Returning a `systemPrompt` string is the wrong lever: the value is a `*sdk.SystemPrompt` | `runs/suppress`, `runs/nosuppress` |
| P24 | Session catalog | `SessionManagerList` (project) and `SessionManagerListAll` (all projects) work from any extension, including `--no-session` runs; entries include `allMessagesText` (full transcript text). `GetSessionFile` is nil in-memory while `GetSessionID` is set | `runs/m0b-smoke` |
| P25 | Skill/command catalog | `GetSystemPromptOptions().Skills` gives enabled skills with path, scope and `disableModelInvocation`; `GetCommands` lists extension and skill commands (not builtin), with source info; `IsProjectTrusted` and `ConfigHome` work | `runs/m0b-smoke` |
| P26 | `/tools` command | Registers without collision, appears in TUI autocomplete, runs in TUI and RPC; `GetAllTools` reports source and exposure per tool | `runs/tui-tools`, `runs/m0b-smoke` |
| P27 | `--list-tools` flag | Parsed in every mode (`GetFlag` = true in `session_start`); `ctx.Shutdown()` there prevents the prompt. **No output channel in print/JSON mode**: `Notify` is dropped, extension stdout/stderr are not forwarded | `runs/m0b-smoke`, §3a |
| P28 | Dialogs | RPC emits `extension_ui_request` (select/input/confirm) and resumes on `extension_ui_response`; unanswered requests block indefinitely unless `DialogOptions.Timeout` is set. Print and JSON modes return `ok=false` / `confirm=false` immediately (`HasUI()=false`) — indistinguishable from a user cancel except via `HasUI` | `runs/dialog-rpc*` |
| P30 | Messages from `before_agent_start` | Returning `{"message": …}` adds a persisted custom message right after the user prompt; `SendMessage(steer)` from the same hook lands before the prompt | `runs/bas-msg`, `runs/bas-steer` |
| P29 | Reload hang root cause | 2/40 in a dedicated loop. Host goroutine blocks in `Host.Reload → stagePackedGo → startGoPackedCellOnce → acceptPackedExt → awaitConnection` with no deadline; the packed cell's re-dial gets `connection refused` on the old socket path, and the new instance's `events.on` fails | `runs/reload-hunt` |

# 3. Frame delivery (P5)

Observed per outgoing provider request (`runs/inject/requests.jsonl`) and stored transcript (`get_messages`):

| Mechanism | Lands in request | Persisted | Shape |
|---|---|---|---|
| `OnToolResult` content rewrite | Same turn, inside the tool result | Yes, part of the toolResult | Tool content |
| `SendMessage(..., DeliverAs: "steer")` from a tool hook | After the current tool batch, before the next assistant call | Yes, `role: custom`, `customType` kept | Sent to the model as user role |
| `SendMessage(..., DeliverAs: "nextTurn")` | After the next user prompt, not before it | Yes, `role: custom` | User role |
| `context` event message append | Every request while the hook keeps adding it | **No** — ephemeral projection | Whatever role the hook writes |

Consequences for SPEC-0002: `nextBoundary` maps to steer, `nextPrompt` to nextTurn (ordering is *after* the prompt), and ambient instruction views (SPEC-0007) must be re-projected by the `context` hook on every request because the host does not persist them. Inclusion acknowledgement can be derived from the `context` hook having run for a request; no host event confirms provider receipt.

## 3a. Print-mode command output (P27)

`pig -p "/tools"` and `pig -p --list-tools` both run the extension code (command logged, flag read) but produce no stdout in text mode and only the session header in JSON mode. A flag can therefore change behaviour but cannot print a listing. `--list-tools` needs a host output channel (e.g. print-mode `Notify` to stderr or an explicit `ctx.Print`) before it can be delivered.

# 4. Implications for the specs

All findings are now written into the specs:

| Spec | Change |
|---|---|
| SPEC-0001 | Explicit `cancel` method, late-reply discard, fire-and-forget reload, state off package globals (P1–P4, P13–P14, P29) |
| SPEC-0002 | Delivery mapping, idle behaviour, nextTurn leak guard, compaction re-projection (P5, P19–P21) |
| SPEC-0003 | Registration before first render, history from Details, codemode child rows (P9–P11) |
| SPEC-0005/0006 | Builtin override and delegation contract (P15–P17) |
| SPEC-0007 | Per-request projection, structural host context-file suppression, resume reason (P21–P23) |
| SPEC-0008 | Location via path resolution and SpawnHook, never factory cwd (P18) |
| SPEC-0009 | Codemode child rows, blocked reason loss, dialog modes, `/tools`, `--list-tools` blocker (P7, P9, P26–P28) |
| SPEC-0010 | Session catalog privacy boundary (P24) |

# 5. Not yet probed

- Kit views/panels: deferred to the M0.5/M1 fixture prototypes; registration of commands and dialogs is covered above.
- Isolated (non-packed) Workbench reload: P29 is specific to packed Go cells; Workbench ships packed.
- Multi-agent/project fan-out (D10) is out of M0.2 scope.

# 6. Upstream issues to file

1. Packed Go cell reload can block forever in `awaitConnection` (P13, P29) — needs a deadline and re-dial against the new socket.
2. Codemode drops `tool_call` block reasons for children (P7).
3. Historical tool call rows re-render with empty args after reload (P11).
4. No hook for extension renderers on codemode child rows (P9).
5. Queued `nextTurn` messages survive tree navigation onto another branch (P20).
6. No print/JSON-mode output channel for extension commands and flags (P27).
7. `session_start` reports `startup` for `--continue` (P22); invalid `deliverAs` is accepted silently (P19).
8. Bus has no request-scoped cancellation for listeners (P3) — optional; the explicit cancel message works.
