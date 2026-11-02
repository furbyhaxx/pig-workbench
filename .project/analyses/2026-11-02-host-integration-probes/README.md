---
document:
  type: "analysis"
  id: "ANA-2026-11-02-host-integration-probes"
  title: "M0 host integration probes"
  description: "Runtime evidence for bus request/reply and cancellation, frame delivery, codemode child observation, renderer resolution, branch lifecycle and reload against the running PiG host"
  status: "draft"
  revision: 1
  created_at: "2026-11-02T17:45:00+01:00"
  updated_at: "2026-11-02T17:45:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["Recorded M0.2 probe results, open host blockers and seams not yet probed"]
---

# 1. Scope and method

Answers PLAN-0001 M0.2 for the seams the API specs (0001–0004) depend on. Every claim below is backed by a run in `evidence/runs/`; nothing here is inferred from SDK source alone.

- Host: `pig` from `PATH`, staged SDK `host:$HOME/.pig/state/pigsdk/sdk-899c7964784a0050`, isolated `PIG_HOME` and work dir under `/tmp`, `--no-session --offline`.
- Model: scripted mock provider (`evidence/harness/mock.py`), no billed inference. Scenario `s_*.json` = model script, `st_*.json` = RPC stdin script.
- Extensions: `evidence/extensions/probe-wb` (Workbench stand-in), `probe-producer` (third-party producer, packed into the same process) and `probe-producer-iso` (same producer as an isolated Go binary, separate PID).
- Drivers: `run.py` (RPC mode, captures `rpc.jsonl`, `requests.jsonl`, `probe.jsonl`), tmux for TUI captures.

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
| P13 | `ctx.Reload()` from a command | Factory re-invoked, call returns, branch state intact in 27/28 runs. **1/28 hang**: no `loaded`, no return, no RPC response; old instance kept serving commands | `runs/reload-loop.txt`, `runs/reload-hang` |
| P14 | Package globals across reload | Survive (same process); not a reset point | `runs/claim-reload` |

# 3. Frame delivery (P5)

Observed per outgoing provider request (`runs/inject/requests.jsonl`) and stored transcript (`get_messages`):

| Mechanism | Lands in request | Persisted | Shape |
|---|---|---|---|
| `OnToolResult` content rewrite | Same turn, inside the tool result | Yes, part of the toolResult | Tool content |
| `SendMessage(..., DeliverAs: "steer")` from a tool hook | After the current tool batch, before the next assistant call | Yes, `role: custom`, `customType` kept | Sent to the model as user role |
| `SendMessage(..., DeliverAs: "nextTurn")` | After the next user prompt, not before it | Yes, `role: custom` | User role |
| `context` event message append | Every request while the hook keeps adding it | **No** — ephemeral projection | Whatever role the hook writes |

Consequences for SPEC-0002: `nextBoundary` maps to steer, `nextPrompt` to nextTurn (ordering is *after* the prompt), and ambient instruction views (SPEC-0007) must be re-projected by the `context` hook on every request because the host does not persist them. Inclusion acknowledgement can be derived from the `context` hook having run for a request; no host event confirms provider receipt.

# 4. Implications for the specs

- **SPEC-0001 cancellation (blocker resolved with a contract change):** host cancellation only unblocks the caller. Workbench must define `cancel` as an explicit bus message keyed by request id and listeners must poll it; a late reply after cancel must be discarded by correlation id. Long work must not run inside a listener at all — admit, return a receipt, run under the session supervisor.
- **SPEC-0001 reload:** `ctx.Reload()` is not reliably completing (P13). Treat reload as fire-and-forget: re-register from the new factory's `OnLoaded`, never wait for `Reload()` to return, and invalidate generations on `loaded`. Report the hang upstream.
- **SPEC-0003 registration:** a third-party presentation registration that arrives after a tool has rendered cannot take effect until reload (P10). Either register before first render (on `session_start`) or document that late registration applies after `/reload`. Historical rows must render from stored Details, not call args (P11).
- **SPEC-0003 / SPEC-0009 codemode:** nested rows are drawn by codemode (P9). Unified child presentation needs either a host hook into codemode's child row rendering or a Workbench-owned codemode renderer that reconstructs children from `parentToolCallId` events. Blocked-child reasons are lost (P7) — upstream fix needed.
- **SPEC-0002:** delivery table in §3 replaces the assumed semantics; `followUp` (start an inference turn) was not probed.

# 5. Not yet probed

M0.2 rows still without evidence; each blocks its acceptance rows, not the whole milestone:

- Same-name builtin override with delegation to the original (`read`/`bash`), directly and via codemode.
- Workbench location without host cwd change.
- Instruction hooks across compaction, resume and tree navigation (only custom entries were checked).
- Session/skill catalog access, including in-memory sessions.
- Command/flag registration: `/tools`, `--list-tools`, print/JSON mode, RPC dialogs (`ask_user`).
- `followUp` delivery and stale queued delivery after a branch switch.
- Reload hang root cause (only reproduced once, no host stack captured).

# 6. Upstream issues to file

1. `ctx.Reload()` intermittently never returns (P13).
2. Codemode drops `tool_call` block reasons for children (P7).
3. Historical tool call rows re-render with empty args after reload (P11).
4. No hook for extension renderers on codemode child rows (P9).
5. Bus has no request-scoped cancellation for listeners (P3) — optional; the explicit cancel message works.
