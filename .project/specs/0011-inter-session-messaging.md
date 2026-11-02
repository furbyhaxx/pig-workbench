---
document:
  type: "specification"
  id: "SPEC-0011"
  title: "Live session discovery and inter-session messaging"
  description: "How live PiG sessions with Workbench find each other across projects and exchange messages that can wake an idle agent"
  status: "draft"
  revision: 1
  created_at: "2026-11-02T20:55:00+01:00"
  updated_at: "2026-11-02T20:55:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User requested cross-project session discovery and bidirectional messaging that wakes idle sessions"
      changes: ["Defined endpoint registry, local socket transport, message envelope, delivery and wake policy, loop guards, tool, commands and extension API"]
---

# 1. Purpose and boundary

A live PiG session running Workbench can be discovered by other live sessions of the **same OS user on the same machine**, in any project. Sessions can exchange messages in both directions. A message to an idle session can start a turn there ("wake").

PiG has no live-session registry and no cross-process bus (its event bus is per session). Workbench therefore provides its own local transport. This is the "separately approved bridge" that SPEC-0007 §9 and SPEC-0009 §4 refer to.

Out of scope for v1: other machines, other OS users, offline mailboxes for sessions that are not running, and direct remote tool execution. A peer can only send text and resource references; it never runs tools in another session.

# 2. Endpoint registry

Each live session owns one **endpoint**: a Unix domain socket plus a record file in the runtime directory.

| Platform | Runtime directory |
|---|---|
| Linux, BSD | `$XDG_RUNTIME_DIR/pig-workbench/v1/`, else `$XDG_STATE_HOME/pig-workbench/run/v1/` |
| macOS | `$TMPDIR/pig-workbench/v1/` (per-user) |
| Windows 10 1803+ | `%LOCALAPPDATA%\pig-workbench\run\v1\` (AF_UNIX) |

The directory is created with mode 0700 and must be owned by the current user; otherwise the feature is disabled with a diagnostic. File names are `<endpointId>.sock` and `<endpointId>.json`, where `endpointId` is 10 random base32 characters. This keeps socket paths under the 104-byte `sun_path` limit.

The record file is written atomically (temporary file plus rename, mode 0600):

```json
{"v":1,"endpointId":"k3f9q2m7ta","token":"<32 random bytes, base64url>","pid":41233,"processStart":"2026-11-02T19:58:12+01:00","sessionId":"0193f2c4-…","sessionName":"parser refactor","persistent":true,"pigVersion":"…","workbenchVersion":"…","project":{"key":"/home/u/src/app","name":"app"},"cwd":"/home/u/src/app/.worktrees/feat-parser","state":"idle","stateSince":"2026-11-02T20:41:03+01:00","model":"anthropic/claude-…","discoverable":true,"accepts":["message"]}
```

- `state` is `idle`, `busy` or `waitingForUser` (a pending `ask_user`). It is updated from `agent_start`/`agent_end` and ask state, coalesced to at most one write per second.
- `project.key` uses the SPEC-0004 §9 membership rule, so worktrees share their project's key. SSH locations publish their `ssh://` URI as `cwd`.
- `token` authenticates connections (§3). Record files are readable only by the user, so the token is never shown in any UI, log or listing.
- No-session mode (`--no-session`) publishes `persistent:false` with an ephemeral session ID.
- `discoverable:false` (setting or `/workbench sessions hide`) removes the record but keeps the socket for peers that already hold a conversation with this session.

**Lifecycle.** The endpoint starts at `session_start` and is torn down at shutdown, removing both files. Reload re-invokes the factory in the same process (SPEC-0001 §4), so the old instance closes its listener before the new one binds. The endpoint ID is kept for the PiG session, so peers keep their address across reloads. A session switch (`/resume`, `/new`) republishes the record with the new session identity.

**Stale records.** A discoverer treats a record as stale when connecting fails *and* the PID is gone, or the PID now belongs to a process with a different start time. A stale record is removed together with its socket. A connectable endpoint is never removed by another process.

# 3. Transport and authentication

- Framing: 4-byte big-endian length plus one UTF-8 JSON object, the same shape as PiG's extension wire. Maximum frame 256 KiB. Oversized or malformed frames close the connection.
- Every connection begins with `hello` carrying the target's token from its record file, the protocol major and the caller's endpoint ID. A wrong token closes the connection without a reply.
- Where the OS supports it, the listener also checks the peer UID (`SO_PEERCRED` on Linux, `getpeereid` on macOS/BSD) and rejects other users. Directory permissions and the token are the boundary on platforms without peer credentials.
- Connections are short-lived (one request, one reply, close). Each side sends to the other side's socket, so both directions work without a shared connection.
- Calls time out after 5 s. A listener serves at most 16 concurrent connections and queues at most 64 accepted messages, then answers `busy`.

Requests: `hello`, `info` (returns the record without the token, plus live `state`), `message` (§4) and `receipt` (asynchronous delivery status, §5).

# 4. Message envelope

```json
{"v":1,"type":"message","id":"01JC9Z…","conversationId":"01JC9Y…","inReplyTo":null,"hop":0,
 "from":{"endpointId":"p8w2…","sessionId":"…","sessionName":"api server","project":{"key":"/home/u/src/api","name":"api"}},
 "to":{"endpointId":"k3f9q2m7ta","sessionId":"0193f2c4-…","channel":"agent"},
 "subject":"Schema change","body":"The /v1/orders response now has `items[].sku`. Please update the parser fixtures.",
 "attachments":["session://0193f1aa-…/entry/e-812"],"wake":"ifIdle","expiresAt":"2026-11-02T21:41:00+01:00"}
```

- `body` is plain text, ≤ 32 KiB after redaction. The sender's secret guard (SPEC-0008 G8.1) redacts it before sending.
- `attachments` are resource URIs (≤ 16). They are never content. The receiver resolves them under its own policy when the agent reads them.
- `channel` is `agent` (delivered to the model) or an extension channel such as `ext:pig-web` (delivered to that extension, §8).
- `to.sessionId` must match the target's current session; otherwise the message is rejected with `sessionChanged` and the current identity, so a message meant for one conversation never lands in another.
- `hop` counts automatic replies within a conversation (§6). `conversationId` is the first message's ID.
- Messages to yourself are rejected.

The synchronous answer to `message` is a receipt: `accepted`, or `rejected` with `notDiscoverable`, `sessionChanged`, `policyDenied`, `rateLimited`, `tooLarge`, `expired` or `busy`. `accepted` means stored in the receiver's inbound queue, not read by a model.

# 5. Delivery in the receiving session

An accepted `agent` message becomes a Workbench frame (SPEC-0002) of kind `message`:
- source `session:<sessionName or short ID>`, title "Message from <sessionName> (<project name>)", subject and body, attachments as resource links;
- **dataTrust `untrusted`, authority `data`.** Another agent's text is information, never an instruction with system authority. A user can mark a peer as trusted (`/workbench sessions trust <peer>`). This raises it to `trusted` data; it still never becomes system authority;
- a reply hint naming the sender's endpoint and the conversation, so the agent can answer with the `sessions` tool.

Delivery and wake:

| Receiver state | Delivery |
|---|---|
| busy | Next boundary (steer). No wake needed |
| idle, wake allowed | `followUp` with `TriggerTurn: true`: starts a turn |
| idle, wake not allowed or budget spent | Next boundary; the user sees a notification and status indicator |
| waitingForUser | Next boundary; never answers or dismisses the pending ask |

The receiver sends a `receipt` back to the sender when the frame is actually delivered to a model request (SPEC-0002 delivery ledger), or when it expires undelivered. The sender's agent sees receipts as compact `notice` frames only when it asked for them (`expectReceipt: true`).

Inbound messages are persisted in the receiver's session as frame records, so they replay with the branch. Outbound messages are recorded in the sender's session as tool results. `harness://messages` lists both directions for the current session.

# 6. Policy and loop guards

| Setting | Default | Meaning |
|---|---|---|
| `sessions.enabled` | true | Endpoint, tool and commands |
| `sessions.discoverable` | true | Publish the record |
| `sessions.inbound` | all | `all` (same user), `sameProject`, `allowlist` or `off` |
| `sessions.wake` | always | `always`, `trustedPeers` or `never`. A wake starts a paid turn |
| `sessions.wakeBudget` | 10 per peer per hour | Further messages are delivered without waking |
| `sessions.maxHops` | 8 per conversation | Beyond this, delivery continues without waking, and both users are notified |
| `sessions.rateLimit` | 30 messages per peer per minute | Further messages are rejected with `rateLimited` |

All are user/runtime settings. A project config cannot enable inbound messages, raise budgets or trust peers.

Loop guard: an outbound message sent by an agent during a turn that was itself started by a peer message carries `hop + 1` of that conversation. Two agents replying to each other therefore stop waking each other after `maxHops` exchanges, and the users see why. A user-initiated message (`/workbench sessions send`) resets `hop` to 0.

Blocking: `/workbench sessions block <peer>` rejects a peer by session ID and project key until unblocked.

# 7. Agent tool and commands

Tool `sessions` (registered when `sessions.enabled`):

| Action | Arguments | Result |
|---|---|---|
| list | `scope`: `project` (default) or `all`; `includeSelf` | Live discoverable sessions: short ID, name, project, cwd, state, idle time, model, trusted/blocked flags |
| info | `session` | One record plus the conversation history with this peer in the current session |
| send | `session` (ID, unique prefix or unique name), `body`, optional `subject`, `attachments[]`, `inReplyTo`, `wake` (`ifIdle` default, `never`), `expectReceipt` | Receipt; the message ID and conversation ID |

A name or prefix that matches more than one session is an error listing the candidates. The tool never guesses.

Commands: `/workbench sessions` opens the panel, which lists live sessions grouped by project with state, and lets you send a message, view a conversation, and trust, block or hide. Subcommands: `ls`, `send <session> <text>`, `trust`, `untrust`, `block`, `unblock`, `hide`, `show`. Headless runs print the list or receipts.

Presentation uses SPEC-0003 preset `process`-style rows: sender/recipient, subject, receipt state. Message frames use the `message` frame kind (SPEC-0002 §2) in rail style.

# 8. Extension API

Over the SPEC-0001 bus, capability `sessions`:
- `sessions.list` → same records as the tool (no tokens).
- `sessions.send` with `channel: "ext:<owner>"` sends to the same owner's extension in another session. The receiving extension gets a `sessions.message` event on `workbench:v1:event`. Extension channels never wake the agent and never become frames unless the receiving extension publishes one.
- `sessions.send` with `channel: "agent"` is allowed for registered owners. The message is attributed to the extension (`from.owner`) as well as the session.

Example: pig-web can tell its counterparts in other sessions that a shared `kb://public/` article was revised, without waking any agent.

# 9. Verification and host probes

Required before claiming the feature works:
- **Wake from a background goroutine.** `SendMessage` with `DeliverAs:"followUp"` and `TriggerTurn:true` from a listener goroutine, using a Context retained from `session_start`, starts a turn in an idle TUI, RPC and print session. Busy sessions receive a steer.
- **State tracking.** Idle/busy transitions from `agent_start`/`agent_end` match the TUI, including aborted turns and compaction.
- **Reload.** The listener is released and rebound without losing the endpoint ID. No second listener survives (P13, P14).

Acceptance:
- Two sessions in different projects discover each other; a third user's process cannot connect (Linux peer UID check plus a wrong-token test).
- Idle wake, busy steer, waitingForUser untouched, `sessionChanged` after `/new`, expiry, and stale-record cleanup after `kill -9`.
- Ping-pong between two agents stops waking after `maxHops`; the budget and rate limits hold; blocked peers are rejected.
- Secrets in a body are redacted before they leave the sender; untrusted text cannot escape the frame envelope.
- Linux runtime verification; macOS and Windows compile-level only until run there (as with the backend evaluation §9).
