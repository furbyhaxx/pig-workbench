---
document:
  type: "specification"
  id: "SPEC-0011"
  title: "Live session discovery and inter-session messaging"
  description: "How live PiG sessions with Workbench find each other across projects and exchange messages that can wake an idle agent"
  status: "draft"
  revision: 3
  created_at: "2026-11-02T20:55:00+01:00"
  updated_at: "2026-11-03T10:00:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User requested cross-project session discovery and bidirectional messaging that wakes idle sessions"
      changes: ["Defined endpoint registry, local socket transport, message envelope, delivery and wake policy, loop guards, tool, commands and extension API"]
    - revision: 2
      reason: "Approved DRAFT-0002 rev 3: agents addressed as agent:// files, project IDs, omp-style receipts, broadcast and the coordinator"
      changes: ["Project identity by SPEC-0012 project ID", "Agent interface moved from the sessions tool to the agent:// tree", "Receipts delivered/woken/queued/failed, unanswered-send tracking, waits", "Project broadcast", "Coordinator with configurable safeguards"]
    - revision: 3
      reason: "Secrets redesigned after nukii-revived (SPEC-0013); donor secret guard removed"
      changes: ["Bodies are not masked by the sender; the receiver's SPEC-0013 boundary applies"]
---

# 1. Purpose and boundary

A live PiG session running Workbench can be discovered by other live sessions of the **same OS user on the same machine**, in any project. Sessions can exchange messages in both directions. A message to an idle session can start a turn there ("wake").

PiG has no live-session registry and no cross-process bus (its event bus is per session). Workbench therefore provides its own local transport. This is the "separately approved bridge" that SPEC-0007 §9 and SPEC-0009 §4 refer to.

Out of scope: other machines, other OS users, offline mailboxes for sessions that are not running, and direct remote tool execution. A peer can only send text and resource references; it never runs tools in another session. The coordinator (§9) can additionally steer and control sessions through host APIs, never by running tools on their behalf.

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
{"v":1,"endpointId":"k3f9q2m7ta","token":"<32 random bytes, base64url>","pid":41233,"processStart":"2026-11-02T19:58:12+01:00","sessionId":"0193f2c4-…","sessionName":"parser refactor","persistent":true,"pigVersion":"…","workbenchVersion":"…","project":{"id":"prj-01jc9z8k2m","slug":"app"},"cwd":"/home/u/src/app/.worktrees/feat-parser","state":"idle","stateSince":"2026-11-02T20:41:03+01:00","model":"anthropic/claude-…","discoverable":true,"accepts":["message"]}
```

- `state` is `idle`, `busy` or `waitingForUser` (a pending `ask_user`). It is updated from `agent_start`/`agent_end` and ask state, coalesced to at most one write per second.
- `project` is the SPEC-0012 §3.3 project of the cwd, so worktrees and components share it. Sessions in the builtin `global` project omit `project`. SSH locations publish their `ssh://` URI as `cwd`.
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

Requests: `hello`, `info`, `control` (coordinator only, §9) (returns the record without the token, plus live `state`), `message` (§4) and `receipt` (asynchronous delivery status, §5).

# 4. Message envelope

```json
{"v":1,"type":"message","id":"01JC9Z…","conversationId":"01JC9Y…","inReplyTo":null,"hop":0,
 "from":{"endpointId":"p8w2…","sessionId":"…","sessionName":"api server","project":{"id":"prj-01jd2…","slug":"api"}},
 "to":{"endpointId":"k3f9q2m7ta","sessionId":"0193f2c4-…","channel":"agent"},
 "subject":"Schema change","body":"The /v1/orders response now has `items[].sku`. Please update the parser fixtures.",
 "attachments":["session://0193f1aa-…/entries/e-812"],"wake":"ifIdle","progress":false,"expiresAt":"2026-11-02T21:41:00+01:00"}
```

- `body` is plain text, ≤ 32 KiB. An agent's text already holds placeholders only; the receiving session's SPEC-0013 boundary applies when the frame enters its context, and placeholders resolve there because all sessions of the user share the vault.
- `attachments` are resource URIs (≤ 16). They are never content. The receiver resolves them under its own policy when the agent reads them.
- `channel` is `agent` (delivered to the model) or an extension channel such as `ext:pig-web` (delivered to that extension, §8).
- `to.sessionId` must match the target's current session; otherwise the message is rejected with `sessionChanged` and the current identity, so a message meant for one conversation never lands in another.
- `hop` counts automatic replies within a conversation (§6). `conversationId` is the first message's ID.
- `progress: true` marks an interim update: a waiting receiver keeps waiting (§7).
- Messages to yourself are rejected.

The synchronous answer to `message` is a receipt: `accepted`, or `rejected` with `notDiscoverable`, `sessionChanged`, `policyDenied`, `rateLimited`, `tooLarge`, `expired` or `busy`. `accepted` means stored in the receiver's inbound queue, not read by a model.

# 5. Delivery in the receiving session

An accepted `agent` message becomes a Workbench frame (SPEC-0002) of kind `message`:
- source `session:<sessionName or short ID>`, title "Message from <sessionName> (<project name>)", subject and body, attachments as resource links;
- **dataTrust `untrusted`, authority `data`.** Another agent's text is information, never an instruction with system authority. A user can mark a peer as trusted (`/workbench sessions trust <peer>`). This raises it to `trusted` data; it still never becomes system authority;
- a reply hint naming `agent://<sender>/inbox` and the conversation.

Delivery and wake:

| Receiver state | Delivery |
|---|---|
| busy | Next boundary (steer). No wake needed |
| idle, wake allowed | `followUp` with `TriggerTurn: true`: starts a turn |
| idle, wake not allowed or budget spent | Next boundary; the user sees a notification and status indicator |
| waitingForUser | Next boundary; never answers or dismisses the pending ask |

The receiver sends a `receipt` back to the sender with the outcome: `delivered` (frame reached a model request of a running turn, SPEC-0002 delivery ledger), `woken` (an idle agent started a turn for it), `queued` (wake declined by policy or budget; delivered at the receiver's next turn) or `failed` with a reason (expired, session ended). Delivered messages leave the inbox; only queued and failed ones stay readable there. The receipt enum is open, so a later `revived` (headless start of a stopped session) needs no protocol change. The sender's agent sees receipts as compact `notice` frames only when it asked for them (`expectReceipt: true`).

Inbound messages are persisted in the receiver's session as frame records, so they replay with the branch. Outbound messages are recorded in the sender's session as tool results. `session://self/messages` and `agent://<peer>/messages` list both directions.

# 6. Policy and loop guards

| Setting | Default | Meaning |
|---|---|---|
| `sessions.enabled` | true | Endpoint, `agent://` tree and commands |
| `sessions.discoverable` | true | Publish the record |
| `sessions.inbound` | all | `all` (same user), `sameProject`, `allowlist` or `off` |
| `sessions.wake` | always | `always`, `trustedPeers` or `never`. A wake starts a paid turn |
| `sessions.wakeBudget` | 10 per peer per hour | Further messages are delivered without waking |
| `sessions.maxHops` | 8 per conversation | Beyond this, delivery continues without waking, and both users are notified |
| `sessions.rateLimit` | 30 messages per peer per minute | Further messages are rejected with `rateLimited` |
| `sessions.broadcast` | project | `project` (to `agent://by-project/<slug>/inbox`), `all`, or `off`. A broadcast wakes nobody beyond each receiver's own wake policy and budget |

All are user/runtime settings. A project config cannot enable inbound messages, raise budgets or trust peers.

Loop guard: an outbound message sent by an agent during a turn that was itself started by a peer message carries `hop + 1` of that conversation. Two agents replying to each other therefore stop waking each other after `maxHops` exchanges, and the users see why. A user-initiated message (`/workbench sessions send`) resets `hop` to 0.

Blocking: `/workbench sessions block <peer>` rejects a peer by session ID or project ID until unblocked.

# 7. Agent interface: `agent://`

Agents use the `agent://` tree of [SPEC-0012](0012-projects-uri-namespace.md) §5.3 with the ordinary tools; there is no separate `sessions` tool. An agent is a live session's main agent, addressed by its session short ID; children of other extensions would appear as `agent://<id>/children/<childId>/` without changing the grammar.

| Action | URI form |
|---|---|
| List | `ls agent://` (current project), `ls agent://by-project/<slug>/`, `ls agent://by-name/`; columns: short ID, name, project, cwd, state, idle time, model, trusted/blocked |
| Look | `read agent://<id>/status` |
| Send | `write agent://<id>/inbox` with text, or JSON5 `{subject, body, attachments, wake, expectReceipt, replyTo, progress}`; the write result is the synchronous receipt plus message and conversation IDs |
| Receive | `read agent://self/inbox?wait=60&from=<id>`: unread messages, waiting up to the given seconds; a `progress` message extends the wait |
| History | `read agent://<id>/messages` |
| Broadcast | `write agent://by-project/<slug>/inbox` (§6 `sessions.broadcast`) |

Sends the receiver has not answered are tracked per conversation and shown in `agent://self/status`, so an agent sees what it is still waiting for. A name or prefix that matches more than one session lists the candidates; Workbench never guesses.

Commands: `/workbench sessions` opens the panel, which lists live sessions grouped by project with state, and lets you send a message, view a conversation, and trust, block or hide. Subcommands: `ls`, `send <session> <text>`, `trust`, `untrust`, `block`, `unblock`, `hide`, `show`. Headless runs print the list or receipts.

Presentation uses SPEC-0003 preset `process`-style rows: sender/recipient, subject, receipt state. Message frames use the `message` frame kind (SPEC-0002 §2) in rail style.

# 8. Extension API

Over the SPEC-0001 bus, capability `sessions`:
- `sessions.list` → the records of `ls agent://` (no tokens).
- `sessions.send` with `channel: "ext:<owner>"` sends to the same owner's extension in another session. The receiving extension gets a `sessions.message` event on `workbench:v1:event`. Extension channels never wake the agent and never become frames unless the receiving extension publishes one.
- `sessions.send` with `channel: "agent"` is allowed for registered owners. The message is attributed to the extension (`from.owner`) as well as the session.

Example: pig-web can tell its counterparts in other sessions that a shared `kb://public/` article was revised, without waking any agent.

# 9. Coordinator

## 9.1 What it is

A session the user marks as coordinator (`/workbench coordinator on`). Its agent can observe and operate every live session of the user in any project, in the user's name. Enabling is a user command only: never a tool call, never project config.

## 9.2 Capabilities

| Class | Operations | Mechanism in the target |
|---|---|---|
| observe | Status, activity, transcripts, pending asks, jobs | Endpoint `info`, `session://` |
| message | Messages without wake budget | SPEC-0011 |
| steer | Prompt as the user, steer, follow-up | `ctx.SendUserMessage` |
| answer | Answer or cancel Workbench `ask_user` forms | Workbench owns the form |
| lifecycle | Abort, compact, model and thinking changes, new session | `ctx.Abort`, `Compact`, `SetModel`, `NewSession` |
| jobs | List, write stdin, kill | SPEC-0008 supervisor |

Host limit: an extension cannot answer PiG's own dialogs, permission prompts or other extensions' `ctx.UI` prompts. The coordinator sees "waiting on host dialog".

## 9.3 Safeguards: all configurable

Every safeguard is a setting with a default; each can be relaxed or switched off.

| Setting | Default | Off / alternatives |
|---|---|---|
| `coordinator.classes` | observe, message | any subset up to all six |
| `coordinator.leaseHours` | 8 | any number, `0` = no expiry |
| `coordinator.targetConsent` | `firstAction` (notice in the target, user can refuse) | `never` (no prompt), `always` |
| `coordinator.single` | true (one coordinator per user) | false |
| `coordinator.confirmDestructive` | true (coordinator asks the user before abort/new session) | false |
| `coordinator.markTargets` | true (status segment "coordinated by …") | false |
| `coordinator.frames` | true (each action as an attributed frame in the target) | false |
| `coordinator.audit` | true (`projects/<id>/coordinator.log`) | false |
| `coordinator.answerAttribution` | true (`answeredBy: coordinator`) | false |

Fixed, because they are not friction but correctness: enabling stays a user action (otherwise any prompt injection could appoint itself); peer messages stay untrusted data and never grant classes; `/workbench coordinator off` works from any session.

## 9.4 Interface

`agent://<id>/asks/<askId>` and `agent://<id>/ctl` ([SPEC-0012](0012-projects-uri-namespace.md) §5.3). Without a granted class they return `policyDenied` naming the class. Over the socket these are `control` requests carrying the coordinator's endpoint and lease; `/workbench coordinator on` writes a coordinator record (endpoint, granted classes, lease expiry; mode 0600) into the runtime directory, and the target checks each request against it. Like the endpoint token, this guards against mistakes and other users, not against a malicious extension of the same user.

# 10. Verification and host probes

Required before claiming the feature works:
- **Wake from a background goroutine.** `SendMessage` with `DeliverAs:"followUp"` and `TriggerTurn:true` from a listener goroutine, using a Context retained from `session_start`, starts a turn in an idle TUI, RPC and print session. Busy sessions receive a steer.
- **State tracking.** Idle/busy transitions from `agent_start`/`agent_end` match the TUI, including aborted turns and compaction.
- **Reload.** The listener is released and rebound without losing the endpoint ID. No second listener survives (P13, P14).

Acceptance:
- Two sessions in different projects discover each other; a third user's process cannot connect (Linux peer UID check plus a wrong-token test).
- Idle wake, busy steer, waitingForUser untouched, `sessionChanged` after `/new`, expiry, and stale-record cleanup after `kill -9`.
- Ping-pong between two agents stops waking after `maxHops`; the budget and rate limits hold; blocked peers are rejected.
- A raw secret typed into `/workbench sessions send` never reaches the receiver's provider requests; untrusted text cannot escape the frame envelope.
- Coordinator: enabling only by user command; each class denied with `policyDenied` until granted; lease expiry; target consent, destructive confirmation, marks, frames and audit each on and off; answering a Workbench `ask_user` form from the coordinator.
- `agent://` list/send/wait/broadcast through `ls`, `read`, `write`; receipts `delivered`, `woken`, `queued`, `failed`.
- Linux runtime verification; macOS and Windows compile-level only until run there (as with the backend evaluation §9).
