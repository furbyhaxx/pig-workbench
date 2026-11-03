---
document:
  type: "draft"
  id: "DRAFT-0002"
  title: "Projects, agent addressing and the global coordinator"
  description: "Brainstorm and proposal for a stable Project entity, agent:// inter-session communication shaped by it, an opt-in coordinator that acts for the user, and the full URI map"
  status: "draft"
  revision: 1
  created_at: "2026-11-03T08:25:00+01:00"
  updated_at: "2026-11-03T08:25:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User asked for a Project type, agent:// addressing, a global coordinator and a URI overview, informed by oh-my-pi, nukii, nugent and agentd"
      changes: ["Recorded prior art with sources, PiG storage facts, a Project model, agent:// grammar, coordinator model, URI map and open questions"]
---

# 1. Questions this draft answers

1. What is a **Project**, how is it identified across paths and hosts, and can session transcripts live under it?
2. How do live sessions address and talk to each other (`agent://`), now that projects exist?
3. What does an opt-in **global coordinator** that acts in the user's name need?
4. What is the complete `scheme://` map?

Nothing here is decided. Section 8 lists the decisions needed.

# 2. Prior art

## 2.1 oh-my-pi (`host:/home/furbyhaxx/.cache/pi-searxng/git/github.com/can1357/oh-my-pi`, commit `95db18e`)

Inter-agent messaging is **in-process** (main agent ↔ subagents of one omp process). Cross-process is a separate feature, Collab.

| Mechanism | Source | What we learn |
|---|---|---|
| `IrcBus`, a process-global mailbox bus | `packages/coding-agent/src/irc/bus.ts` | `send` never blocks on the recipient. The receipt says **how** it arrived: `injected` (aside at next step if busy, or a waiting `wait`), `woken` (idle agent gets a real turn), `revived` (parked/killed agent restarted from its transcript), `failed`. Only a failed hand-off is buffered (mailbox cap 100, drop oldest), so a delivered message is never delivered twice |
| `wait` tool | `src/tools/wait.ts`, `irc/bus.ts#wait` | Block for a message, optional `from` filter, timeout, abort. Tracks **unanswered sends**; a `progress` message marks "reply will follow" so the waiter keeps waiting |
| `agent://` | `src/internal-urls/agent-protocol.ts` | `read agent://<id>` = the agent's published output (`<id>.md`), JSON-path extraction `agent://A.B/reports/0`; a running agent returns status + progress. `write agent://<id>` = message; `?progress` = progress note; `agent://all` = broadcast (write-only) |
| `history://` | `src/internal-urls/history-protocol.ts` | `history://` index of agents, `history://<id>` concise transcript, live or from JSONL |
| Agent Hub | `docs/agent-hub.md` | Roster with status (running/idle/parked/aborted), unread counts, usage; steer, revive, kill; transcript viewer |
| Collab registry | `src/collab/registry.ts`, `docs/collab.md` | Per-process private IPC endpoint (UDS / named pipe), owner-only metadata files with a random bearer token, protocol version, **generation** that changes on `/new`/`/resume` (`stale_generation` rejects old links), pruning of stale entries, listing across projects. This is nearly identical to our SPEC-0011 §2–3, which is reassuring |
| Collab guests | `docs/collab.md` "Guest permission model" | Full-link guests can prompt, interrupt, steer subagents **and answer the host's interactive select/editor requests** (first response wins, others dismissed). View-only guests read only. Machine commands (`/model`, `/compact`, bash) stay host-only |

omp has **no project entity**: sessions live in path-encoded buckets (`docs/session.md`).

## 2.2 nukii-revived (`host:/mnt/Projects/nukular/nukii-revived`)

`crates/nukii-store/src/projects/mod.rs`: the most complete identity design.
- Project = stable ULID + namespace + confirmed name + confirmed remote + trust override.
- **Sighting** = one observation of the project on a host: canonical path, git common dir, **root commit SHAs**, normalized remotes, local marker, time.
- Resolution order: local marker → root commit SHA → normalized remote → (host, path) sighting. Unknown directories are *inspected* first and only created after confirmation.
- Marker: `nukii.projectId` written into the repository's **local git config** (common dir, so all worktrees share it, never committed).
- Namespaces group projects (rules by path prefix or remote owner); trust inherits namespace → project.

## 2.3 nugent (`host:/mnt/Projects/nukular/nukular-ai/crates/nukular-nugent/src/config/projects.rs`)

Simple `projects.toml` registry, comment-preserving edits: id, unique name, path, repo URL, default branch, description, preferred model, preferred agent, timestamps. Lookup by id, name, path or unique prefix; `None` = project of the current directory.

## 2.4 agentd-ts (`host:/mnt/Projects/furbyhaxx/agentd-ts/.project/specs/SPEC-0001-architecture-and-daemon.md` §4)

Workspace (default exists) > Project (repo root `file://` or `ssh://`, default branch, project agents config, worktrees) > Session (linked to workspace and project, parent session, location, branch/worktree). Project auto-detected from the git root.

## 2.5 PiG facts (docs bundle and SDK `sdk-899c7964784a0050`)

| Fact | Source |
|---|---|
| Sessions: `~/.pig/agent/sessions/--<encoded cwd>--/<file>.jsonl`, append-only | `docs/sessions.md` |
| `sessionDir` setting (also project settings, read **before** trust), `--session-dir`, `PIG_CODING_AGENT_SESSION_DIR` | `docs/config.md`, `docs/settings.md`, `docs/security.md` |
| Extensions can create, open, fork, find and list sessions **in any directory**: `SessionManagerCreate(cwd, sessionDir)`, `SessionManagerOpen`, `SessionManagerForkFrom(source, targetCwd, sessionDir)`, `SessionManagerList/ListAll(sessionDir)` | `session_manager_class.go` |
| Switch the live session to any file: `ctx.SwitchSession(path)`; also `NewSession`, `Fork` | `context.go` |
| Act on the own session: `SendUserMessage`, `SendMessage`, `Abort`, `IsIdle`, `WaitForIdle`, `Compact`, `SetModel`, `SetThinkingLevel`, `Shutdown` | `context.go` |
| No hook to choose where the **host** creates a new session file | absence in SDK; needs confirmation |

# 3. Project model

## 3.1 Entities

```text
Project ─┬─ Location*   (where it is checked out: host + path, kind main|worktree|clone|ssh)
         ├─ Component*  (optional: several repositories forming one project)
         ├─ Knowledge   (personal notes and always-on instructions, outside the repo)
         └─ SessionLink* (home or linked)
```

```toml
# {agentDir}/projects/prj-01JC9Z…/project.toml
id = "prj-01jc9z8k2m"            # generated, never reused
slug = "pig-workbench"           # unique, user-editable, used in URIs
name = "PiG Workbench"           # optional display name
description = "Go factory extension for PiG"

[git]
default_remote = "github.com/furbyhaxx/pig-workbench"   # normalized
default_branch = "main"
root_commits = ["3f1c…"]         # identity across clones and hosts

[[locations]]                    # observed, not hand-maintained
host = "devlap05"
path = "/mnt/Projects/furbyhaxx/pig-workbench"
kind = "main"
last_seen = 2026-11-03T08:20:00+01:00

[[components]]                   # only for multi-repo projects
slug = "pig-web"
git.default_remote = "github.com/furbyhaxx/pig-web"
```

## 3.2 Identity resolution

On every session start and `cd`, Workbench resolves the location to a project, in this order (nukii's order, which worked):

1. **Local marker** `pig.projectId` in the repository's local git config (common dir, so worktrees share it; never committed). For non-git directories: none, fall through.
2. **Root commit SHA** match against known projects. Survives re-cloning and moving to another host.
3. **Normalized remote** match (`git@github.com:o/r.git` = `https://github.com/o/r`).
4. **(host, path)** match against recorded locations; also covers non-git directories.
5. Nothing matched → the location belongs to the builtin **`global`** project, and Workbench offers "Create project `<dir name>`?" once (TUI) or reports `unregisteredLocation` (headless). It never auto-creates silently, so random `/tmp` directories do not become projects.

A match by 2–4 writes the marker (1) and records the location. Two projects matching the same location is a visible conflict, never a silent pick. Forks share root SHAs: the remote (3) breaks the tie, otherwise the user chooses.

Optional committed marker `.pig/project.toml` with only `id`/`slug`: makes the identity portable to teammates and hosts without git history access. Decision Q3.

## 3.3 Multiple paths and "workspaces"

These are two different things, and conflating them is how data models rot:
- **Same project, several places**: clones, worktrees, the SSH copy on a server, a moved directory. These are **Locations** of one project. Nested `.worktrees/*` resolve through the marker automatically.
- **Several repositories, one product** (e.g. pig-workbench + pig-web as "PiG extensions"): either **Components** of one project, or a **group** of projects. Recommendation: start with Components only if you have a concrete case; a group/namespace layer (agentd "workspace", nukii "namespace") is deferrable until there are many projects.

## 3.4 Storage and relocation

Target layout:

```text
{agentDir}/projects/
  index.toml                          # id ↔ slug ↔ markers cache; rebuildable
  global/project.toml                 # builtin, sessions without a project
  prj-01jc9z8k2m/
    project.toml
    knowledge/notes.md                # personal, never committed anywhere
    knowledge/instructions/*.md       # always-on, injected as frames
    sessions/                         # transcripts (see options)
    sessions.d/<session-id>/          # Workbench sidecars: job logs, snapshots, artifacts
```

The transcript is written by the host, so "where" depends on what PiG allows:

| Option | How | Pro | Con |
|---|---|---|---|
| A. Index only | Leave transcripts in PiG buckets; Workbench records `session → project` links | Zero risk, Pi-compatible | Files are not physically under the project; moving a repo leaves sessions in the old path bucket (still linked by ID) |
| B. Bucket symlinks | Make each location's bucket `sessions/--<encoded path>--` a symlink to `projects/<id>/sessions/` | Physical relocation without host changes; all locations of a project share one session dir, so `/resume` in any checkout shows all project sessions | Must probe that PiG follows symlinked buckets and does not canonicalize them; a second PiG writing the same dir concurrently is already the case today |
| C. `sessionDir` in project settings | Write `.pig/settings.json` `sessionDir` | Host-native | Lands in the repository (often committed), host-specific path, read before trust |
| D. Upstream hook | Ask PiG for a session-directory resolver hook for extensions | Clean | Not available; timeline unknown |
| E. Extension-owned creation | Workbench creates sessions with `SessionManagerCreate(cwd, projectDir)` and `SwitchSession` | Works with today's SDK | The host's own initial session file is already in the default bucket; fights `/new`, `/resume`, `--continue` |

Recommendation: **A now, B as opt-in after a probe, D as an upstream request.** E is a trap.

## 3.5 Session ↔ project

- Every session has exactly one **home** project (derived from its cwd at creation, `global` otherwise) and may be **linked** to more (read-only references, e.g. a design discussion relevant to two projects).
- **Move** changes the home: under A, rewrite the link; under B, move the file between project dirs (the transcript's header `cwd` stays as recorded; history is immutable).
- `session://` catalogs filter by project ID, replacing the path-prefix membership rule of SPEC-0004 §9.

## 3.6 Project knowledge

`knowledge/instructions/*.md` are personal, always-on instructions for a project that do not belong in the repository's `AGENTS.md` (personal preferences, private context, cross-repo facts). They enter through the SPEC-0007 ambient-context pipeline as a new class `projectKnowledge`, rendered as frames, with the same pinning, diffing and caps. A project card (slug, description, remote, branch, locations, live agents) can be injected once per session as a compact frame. Writable through `project://` (section 6) with confirmation.

This is not task tracking: no items, states or assignments.

# 4. Agent communication on top of projects

## 4.1 Terminology

An **agent** is an addressable conversation participant. Today: each live PiG session's main agent. Later, if subagents arrive (from PiG or another extension), they slot in as children: `agent://<project>/<session>/<child>`. Using `agent://` now avoids a rename later.

## 4.2 Grammar

| URI | read | write |
|---|---|---|
| `agent://` | Live agents in the current project | — |
| `agent://*` | Live agents in all projects | — |
| `agent://<project>` | Live agents of that project | Broadcast to the project (off by default) |
| `agent://<project>/<agent>` | Status card: state, idle time, model, current activity, pending ask, last assistant text | Send a message (content = body) |
| `agent://<project>/<agent>/messages` | Conversation with that agent, paged | — |
| `agent://<project>/<agent>/transcript` | Same as `session://<id>/full` | — |
| `agent://self/inbox` | Unread messages; `?wait=60&from=<agent>` blocks until one arrives (bounded, cancellable) | — |
| `agent://<project>/<agent>?progress` | — | Progress note: "reply follows" (keeps the other side's `wait` alive) |

`<agent>` is the session name, short ID or `self`. `<project>` is the slug, `prj-` ID or `.` for the current project. A single segment `agent://<x>` is accepted when `x` is a unique agent name across live agents; ambiguity lists candidates.

`write` content may be plain text or a JSON5 envelope `{subject, body, attachments, wake, expectReceipt, replyTo}`.

## 4.3 Delivery semantics (adopted from omp)

- Fire-and-forget with receipt `delivered` (aside at next boundary), `woken` (idle agent started a turn), `queued` (receiver rejected wake by policy; delivered at its next turn), `failed` (reason).
- No duplicate delivery: a message delivered into context is not kept in the inbox.
- `wait` tracks unanswered sends; progress notes keep it alive.
- **Revival** (message to a session that is not running starts it headless from its transcript) is omp's `revived`. For us that means spawning `pig --mode rpc --session <file>` — real agent spawning. Recommend: not in v1; Q6.

SPEC-0011 keeps transport, auth, loop guards, wake budgets and untrusted framing unchanged; only the addressing and project scoping move to this grammar.

# 5. Global coordinator

## 5.1 What it is

One session the **user** marks as coordinator (`/workbench coordinator on`, user-only: never a tool, never project config). Its agent can inspect and operate every live session of that user, in any project, in the user's name.

## 5.2 Capability classes

Each class is granted separately when enabling (default in bold):

| Class | Operations | Implemented in the target by |
|---|---|---|
| **observe** | Status, live activity, transcripts, pending asks, jobs | Endpoint `info` + `session://` |
| **message** | Same as any peer, without wake budget | SPEC-0011 |
| steer | Prompt as the user (`SendUserMessage`), steer, follow-up | `ctx.SendUserMessage` |
| answer | Answer or cancel pending Workbench `ask_user` forms | Workbench owns the form, so it can complete it |
| lifecycle | Abort, compact, model/thinking change, new session | `ctx.Abort`, `Compact`, `SetModel`, `NewSession` |
| jobs | List and kill background jobs | SPEC-0008 supervisor |

Not possible from an extension: answering host dialogs or other extensions' `ctx.UI` prompts, approving host permission prompts, running bash in the target outside its own tools. These stay with the human; the coordinator sees "blocked on host dialog".

## 5.3 Safety

- **Lease**: enabling creates a time-bound lease (default 8 h, renewable), stored in user state; ends on session exit.
- **One coordinator** per user at a time; a second request shows who holds the lease.
- **Target consent**: `sessions.coordinator.accept = all|ask|none` per user (default `ask`: the first control action per target shows a notice in that session, the user can refuse).
- **Visible**: every controlled session shows a status segment "coordinated by <name>" and each action as a frame attributed `coordinator:<name> on behalf of user`.
- **Audit**: append-only log per project under `projects/<id>/coordinator.log`.
- **Answers to ask_user** record `answeredBy: coordinator`, so `/answer` history and replay show it was not typed by the human.
- **Injection boundary**: messages from peers remain untrusted data in the coordinator's context; a peer message never grants a class. Destructive lifecycle actions (new session, abort during a job) require the coordinator's own `ask_user` confirmation unless the user disabled that.
- **Kill switch**: `/workbench coordinator off` in any session revokes the lease everywhere.

## 5.4 Interface

Under `agent://`: `agent://<project>/<agent>/asks` (read pending forms; `write …/asks/<askId>` answers), `write agent://<project>/<agent>/control` with `{op: "abort"|"compact"|"model"|"new"|"prompt", …}`. Without a lease these return `policyDenied`.

# 6. URI map

Convention for every scheme (SPEC-0004 §3): `scheme://authority/path` + selectors `:N`, `:A-B`, `:A+C`, `:-N`, `:raw`, `:conflicts` + `?query`. `read` reads/lists, `write` sends/sets/creates, `edit` hashline-edits where `writable`, `grep`/`find`/`ls` work where the provider can `locate()` to real files. `write` on a control path is an action and obeys plan/read-only modes unless it only cancels.

## 6.1 Files and locations

| URI | What | read | write/edit |
|---|---|---|---|
| `path`, `./x`, `~/x` | Local file relative to the active location | Content, selectors, adapters | Yes |
| `file:///abs` | Explicit local file, ignores active SSH location | Same | Yes |
| `ssh://host/abs` | Remote file over the SSH transport | Same | Yes |
| `local://x` | Private per-session scratch directory | Yes | Yes |
| `artifact://N` | Spilled large tool output, immutable | Paged | No |
| `conflict://N` | One merge-conflict hunk (`ours`/`theirs`/`base` views) | Yes | Resolution write |
| `http(s)://…` | Web page/document via static reader or fetch backend | Converted content | No |

## 6.2 Conversations and agents

| URI | What | read | write |
|---|---|---|---|
| `session://` | Current session overview + entry table | Yes | — |
| `session://<id>` | Any session; `/full`, `/entry/<id>` | Yes | — |
| `session://project`, `session://all`, `?q=` | Catalogs and search | Yes | — |
| `history://…`, `branch://…` | Aliases: transcript-centric and current-branch views | Yes | — |
| `agent://…` | Live agents, messages, inbox (section 4) | Yes | Message, control |
| `project://` (proposed) | Current project card | Yes | — |
| `project://<slug>` | Project card, `/locations`, `/sessions` (= `session://` catalog), `/knowledge/…` | Yes | Knowledge files, metadata (confirmed) |

## 6.3 Harness and runtime

| URI | What | read | write |
|---|---|---|---|
| `jobs://`, `proc://` | Background jobs; `/<id>`, `/<id>/log` | Status, logs | stdin, `/eof`, `/kill` |
| `harness://status` (`self://`, `workbench://`) | Workbench status | Yes | — |
| `harness://instructions` | Selected instruction files and reasons | Yes | Pin/unpin |
| `harness://frames`, `/<id>` | Frame receipts | Yes | — |
| `harness://integrations`, `harness://schemes` | Registrations, provider catalog | Yes | — |
| `harness://messages` | Inter-session messages of this session | Yes | — |
| `harness://reload` | Deferred safe reload | — | Trigger |
| `cfg://<key>` | Effective settings with provenance | Yes | Typed, confirmed |
| `skill://`, `skills://`, `skill://<name>/<path>` | Enabled skills and their files | Yes | No |

## 6.4 Adapters inside a path (not schemes)

`db.sqlite:table:key`, `data.json` queries, archives (`x.zip/inner/file`), PDFs/documents, images, notebooks, binaries: selected by content, addressed with path suffixes and selectors (SPEC-0006).

## 6.5 Contributed by other extensions

| URI | Owner | What |
|---|---|---|
| `kb://public/…`, `kb://project/…` | pig-web | Knowledge base articles and sources |
| `git://…` | pig-web | Repository snapshots fetched for research |
| `research://…` | pig-web | Research runs and reports |
| any `scheme://` | any extension | Through `resourceProvider` registration (SPEC-0004) |

## 6.6 Overlaps to resolve

- `session://project` vs `project://<slug>/sessions`: one handler, two names; keep one canonical.
- `history://` vs `session://` vs `branch://`: three names for transcript views. Consider keeping `session://` canonical and the others as documented aliases only.
- `agent://<…>/transcript` vs `session://<id>/full`: alias, not a second implementation.
- `harness://messages` vs `agent://self/messages`: pick one.

# 7. Effects on existing specs (when approved)

- SPEC-0004 §9: project membership by project ID; add `project://`, `agent://`.
- SPEC-0007: new ambient class `projectKnowledge`.
- SPEC-0010: project store, `index.toml`, markers, `global` project, retention.
- SPEC-0011: addressing via `agent://`; project-scoped discovery; coordinator lease and control requests.
- pig-web: KB scope key moves from canonical path to project ID (handoff `HO-workbench-answers`).

# 8. Decisions needed

| # | Question | Recommendation |
|---|---|---|
| Q1 | Project identity order: marker → root SHA → remote → (host, path)? | Yes |
| Q2 | Create projects only after confirmation; unconfirmed → `global`? | Yes |
| Q3 | Optional committed `.pig/project.toml` for portable identity? | Optional, off by default |
| Q4 | Transcript relocation: A now, B after probe, D upstream? | Yes |
| Q5 | Multi-repo: Components now, groups/namespaces later? | Only with a concrete case |
| Q6 | Revive non-running sessions on message (spawn headless PiG)? | Not in v1 |
| Q7 | Coordinator classes, lease length, target consent default `ask`? | As in §5 |
| Q8 | Canonical names among `session://`/`history://`/`branch://`/`project://…/sessions` | `session://` canonical |
