---
document:
  type: "draft"
  id: "DRAFT-0002"
  title: "Projects, the project artifact tree, pseudo-filesystem URIs, agent addressing and the coordinator"
  description: "Proposal for a stable Project entity with .project/Project.toml, the builtin .project convention shared across worktrees, all schemes as browsable pseudo-filesystems, agent:// communication and an opt-in coordinator"
  status: "draft"
  revision: 2
  created_at: "2026-11-03T08:25:00+01:00"
  updated_at: "2026-11-03T08:55:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User asked for a Project type, agent:// addressing, a global coordinator and a URI overview, informed by oh-my-pi, nukii, nugent and agentd"
      changes: ["Recorded prior art with sources, PiG storage facts, a Project model, agent:// grammar, coordinator model, URI map and open questions"]
    - revision: 2
      reason: "User feedback: configurable coordinator safeguards, schemes as pseudo-filesystems, .project/Project.toml on by default, builtin dot-project and AGENTS.md conventions, shared artifacts and worktree links, option B, components now, no revival design, no release labels"
      changes:
        - "Coordinator: every safeguard is a setting and can be disabled"
        - "Replaced the URI map with a pseudo-filesystem model and per-scheme trees"
        - "Project identity from .project/Project.toml first; fallbacks explained"
        - "Added the builtin project conventions, project:// artifact tree shared across worktrees, and worktree links"
        - "Session storage: option B, verified by ANA-2026-11-03-session-bucket-symlink-probe"
        - "Components in scope; revival recorded as a later idea only"
---

# 1. Scope

This draft designs what Workbench should have. Delivery order belongs to PLAN-0001, not here; nothing below is labelled by release.

Covers:
1. **Project**: identity, `Project.toml`, locations, components, session storage.
2. **Project conventions**: `.project/` artifacts and `AGENTS.md`/`BACKLOG.md` as builtin, `project://` shared across worktrees, worktree links.
3. **Pseudo-filesystem URIs**: every scheme browsable with `ls`, `find`, `grep`, `read`, `write`.
4. **Agent communication** on `agent://`.
5. **Coordinator**.

Out of scope stays: task/todo tracking, agent definitions, spawning agents. Section 6.5 records revival as a later idea without design.

# 2. Prior art and host facts

## 2.1 oh-my-pi (`host:/home/furbyhaxx/.cache/pi-searxng/git/github.com/can1357/oh-my-pi`, commit `95db18e`)

In-process agent messaging plus cross-process Collab; no project entity (path-encoded session buckets, `docs/session.md`).

| Mechanism | Source | Adopted idea |
|---|---|---|
| `IrcBus` | `packages/coding-agent/src/irc/bus.ts` | Send never blocks; receipt says how it arrived (`injected`, `woken`, `revived`, `failed`); only failed hand-offs stay in the mailbox (cap 100) |
| `wait` | `src/tools/wait.ts` | Wait with `from` filter; unanswered-send tracking; `progress` keeps the waiter waiting |
| `agent://` | `src/internal-urls/agent-protocol.ts` | `read` = published output or live progress; `write` = message; `?progress`; `agent://all` broadcast |
| `history://` | `src/internal-urls/history-protocol.ts` | Index plus concise transcripts, live or from JSONL |
| Collab registry | `src/collab/registry.ts`, `docs/collab.md` | Owner-only metadata + bearer token + socket per process, generation per session; matches SPEC-0011 §2–3 |
| Collab guests | `docs/collab.md` | Writable guests prompt, interrupt and **answer the host's select/editor requests**; first answer wins |

## 2.2 nukii-revived (`host:/mnt/Projects/nukular/nukii-revived/crates/nukii-store/src/projects/mod.rs`)

Project = ULID + namespace + name + remote + trust. Sightings per host: path, git common dir, root commit SHAs, normalized remotes, marker. Resolution: marker (`nukii.projectId` in local git config) → root SHA → remote → (host, path). Unknown directories are inspected, created only after confirmation.

## 2.3 nugent and agentd

- nugent `crates/nukular-nugent/src/config/projects.rs`: `projects.toml` (id, unique name, path, repo URL, default branch, description, preferred model/agent), comment-preserving edits, lookup by id/name/path/prefix.
- agentd-ts `.project/specs/SPEC-0001-architecture-and-daemon.md` §4: Workspace > Project (repo root `file://`/`ssh://`, default branch, worktrees) > Session (parent, location, branch).

## 2.4 PiG

| Fact | Source |
|---|---|
| Sessions in `~/.pig/agent/sessions/--<encoded cwd>--/`, append-only JSONL; bucket created at startup | `docs/sessions.md`, probe |
| Symlinked buckets: writes, per-cwd listing and `--continue` work through them; `ListAll` lists each file once per symlink | [ANA session-bucket probe](../analyses/2026-11-03-session-bucket-symlink-probe/README.md) |
| Extensions can create/open/fork/list sessions in any directory and switch to any file | SDK `session_manager_class.go`, `context.go` |
| Own-session control: `SendUserMessage`, `SendMessage`, `Abort`, `IsIdle`, `WaitForIdle`, `Compact`, `SetModel`, `SetThinkingLevel`, `NewSession`, `Shutdown` | SDK `context.go` |
| No hook to choose where the host creates its session file | SDK, absence |

# 3. Project

## 3.1 Model

```text
Project ─┬─ Component+  (a repository; single-repo projects have one)
         │     └─ Location*  (a checkout of it: host + path, kind main|worktree|clone|ssh)
         ├─ Sessions        (transcripts, under the project directory)
         └─ Private notes   (personal, never in a repository)
```

## 3.2 `.project/Project.toml`

Committed in each component repository, created by default when Workbench registers a project. Teammates and other hosts get the same identity.

```toml
id = "prj-01jc9z8k2m"                 # generated once, never reused
slug = "pig-extensions"               # unique per user registry, used in URIs
name = "PiG extensions"               # optional
description = "Workbench and web extensions for PiG"

[component]
slug = "pig-workbench"                # this repository's role in the project
default_remote = "github.com/furbyhaxx/pig-workbench"
default_branch = "main"

[[components]]                         # the other repositories of the project
slug = "pig-web"
default_remote = "github.com/furbyhaxx/pig-web"

[worktrees]
links = [".env", ".env.local", ".references"]   # §4.4
share_project_tree = true                        # §4.3

[conventions]
artifacts = true                      # .project/ tree is first-class (§4.1)
agents_md = true                      # AGENTS.md and BACKLOG.md standards (§4.2)
```

A single-repo project has one `[component]` and no `[[components]]`. Every component repository carries its own `Project.toml` with the same `id`.

## 3.3 How Workbench recognises the project of a directory

This answers "which project am I in?" whenever a session starts or `cd` moves:

1. **`Project.toml` found** in `.project/` at the repository top (or an ancestor for non-git directories): its `id` is the answer. This is the normal case.
2. **No `Project.toml`**, for repositories you cannot or do not want to add files to (third-party clones, read-only checkouts, someone else's repo): Workbench matches the repository against projects it already knows, by root commit SHA (same history, any clone, any host), then normalized remote, then a recorded (host, path). A match is remembered in the repository's **local** git config (`pig.projectId`, never committed).
3. **Nothing matched**: the directory belongs to the builtin `global` project. Workbench offers once to create a project (writes `Project.toml`, or only the local git marker when chosen). It never creates one silently.

Conflicts (two projects claim one directory, e.g. a fork sharing root commits) are shown, never resolved by guessing; the remote breaks most ties.

## 3.4 Registry and storage

```text
{agentDir}/projects/
  index.toml                    # id ↔ slug ↔ component remotes ↔ locations; rebuildable cache
  global/                       # builtin project for sessions outside any project
  prj-01jc9z8k2m/
    project.toml                # registry copy of Project.toml + observed locations
    private/                    # personal notes and instructions, never committed
    sessions/                   # all transcripts of the project (§3.5)
    sessions.d/<session-id>/    # Workbench sidecars: job logs, snapshots, artifacts, messages
```

`Project.toml` in the repository is the source of truth for shared fields; the registry copy adds locations and anything personal. A changed `Project.toml` (pull, edit) refreshes the registry.

## 3.5 Session storage: symlinked buckets (option B)

Every location's PiG bucket `sessions/--<encoded path>--` becomes a symlink to `projects/<id>/sessions/`. Verified in the [probe](../analyses/2026-11-03-session-bucket-symlink-probe/README.md): PiG writes, lists and continues through the symlink, so `/resume` in any checkout shows all sessions of the project.

- **Adoption.** PiG creates the bucket as a real directory before extensions start. Workbench converts it at session start: move existing files into the project directory (rename on the same filesystem), replace the directory with the symlink. If another live session uses the same bucket (SPEC-0011 registry) or the filesystems differ, conversion waits for the next start and the reason is shown.
- **Known host effects.** The all-projects listing shows each session once per location (Workbench catalogs deduplicate by session ID; ask PiG upstream to deduplicate by real path). `--continue` may pick a session recorded in another checkout of the same project (by design of B; the cwd used afterwards still needs a probe).
- **Move and link.** A session has one home project and optional links. Moving rewrites nothing in the transcript; the file moves between project directories and links update.
- **Undo.** `/workbench projects unlink-storage` replaces symlinks with real directories again.
- **Windows.** Directory junctions instead of symlinks; untested.

## 3.6 Private notes

`projects/<id>/private/` holds personal instructions and notes for a project that do not belong in the team's repository. Injected through the SPEC-0007 ambient-context pipeline as class `projectPrivate` (frames, pins, diffs, caps). Not task tracking.

# 4. Builtin project conventions

The `dot-project` and `agents-md` skills become Workbench conventions: shipped as bundled skills (adapted to Workbench paths and tools) and enforced by Workbench features, not only by agent discipline.

## 4.1 Artifact tree

- `.project/` types, naming, frontmatter, revisions and evidence follow the `dot-project` standard.
- Workbench validates artifact frontmatter on `write`/`edit` under `.project/` (missing `document:` block, wrong type for the directory, revision not bumped) and reports problems in the tool result. It does not block the write.
- `ls project://specs/` shows id, title, status and revision as columns from frontmatter.
- `Project.toml` lives in `.project/` and is not an artifact (no frontmatter).
- Dependency to resolve: the standard says live claims go to project-scope `tasks`, which Workbench does not provide (task tracking is excluded). The bundled skill must name whatever tasks tool is present, or drop the line when none is.

## 4.2 AGENTS.md and BACKLOG.md

SPEC-0007 already injects `AGENTS.md` and `BACKLOG.md` (ancestors, nearest, caps, diffs). Adoption adds: the bundled `agents-md` skill; a reminder frame when an agent creates a new module directory without `AGENTS.md` (configurable); and `BACKLOG.md` pointer validation (target item exists, item `status: "open"`).

## 4.3 `project://`: one artifact tree for all worktrees

`project://<path>` resolves to `<main worktree>/.project/<path>`, whichever checkout the agent works in. All agents of a project read and write the same artifacts.

On top of the URI, Workbench makes linked worktrees consistent (verified in the probe): per-worktree sparse checkout excludes `.project/`, a `.project` symlink points to the main worktree's tree, `/.project` goes into the common `info/exclude`. The order matters (`sparse.expectFilesOutsideOfPatterns` first); Workbench does it when it creates a worktree and repairs existing ones on request.

Consequences to accept:
- Artifacts are committed from the main worktree, on whatever branch it has checked out. Worktree branches never carry `.project/` changes and never conflict on them.
- An artifact describing unmerged work in a worktree is visible to everyone immediately. This is the point.

`share_project_tree = false` keeps per-branch `.project/` copies (plain git behaviour); `project://` then resolves to the current checkout.

## 4.4 Worktree links

`[worktrees] links` lists untracked files or directories that every worktree gets as symlinks to the main worktree (`.env`, local config, `.references/`). Created when Workbench creates a worktree, checked on `cd` into one, listed under `projects://<slug>/locations/`. Tracked paths are rejected (they would need the sparse technique of §4.3). A link whose target is missing is reported, not created empty.

# 5. Pseudo-filesystem URIs

## 5.1 Model

Every scheme is a tree, like `/proc` and `/sys`. The builtin tools work on it as on files:

| Tool | On a scheme |
|---|---|
| `ls` | Directory entries with typed columns (`ls jobs://` = jobs with state, runtime, command) |
| `read` | File content; selectors `:N`, `:A-B`, `:-N`, `:raw` |
| `find` / glob | Walk the tree by name pattern (`find session:// -name '*parser*'`) |
| `grep` | Search file contents across the tree (`grep -r timeout jobs://*/log`, `grep -r "hashline" session://`) |
| `write` | Write a file: content, a message, a setting, or a command to a control file |
| `edit` | Hashline edit where the file is writable |
| stat | Through `ls -l` / `read` metadata |

Entry kinds: `file`, `dir`, `link` (to another URI), `ctl` (write-only control file, like `/sys` attributes). Providers implement SPEC-0004 `stat`/`list`/`read`/`search`; when a provider has no `search`, Workbench walks with `list`+`read` under bounds (depth, entries, bytes, time) and reports incomplete results explicitly. Providers mark files `volatile` (changes on every read, e.g. status) or `immutable` (transcripts, artifacts), and can mark subtrees `noWalk` (expensive) so recursive tools skip them unless named directly.

Writes to `ctl` files are actions: they obey permissions and plan/read-only modes unless they only cancel (`kill`).

## 5.2 Trees

Files and locations:

```text
path | file:///abs | ssh://host/abs      real files (local or remote), all tools
local://                                 per-session scratch directory
artifact://<n>                           spilled large outputs, immutable
conflict://<n>/{ours,theirs,base,resolved}   merge conflict hunks; write resolved
http(s)://…                              web content via static reader or fetch backend; read only
<file>:<table>:<key>, <archive>/<inner>  adapters inside real paths (SPEC-0006)
```

Projects and artifacts:

```text
project://                               current project's .project/ tree (shared, §4.3)
  Project.toml
  AGENTS.md
  specs/0001-extension-services.md       ls columns: id, title, status, revision
  plans/ analyses/ drafts/ backlog/ …
projects://                              all projects: slug, id, components, locations, live agents
projects://<slug>/
  Project.toml                           registry view
  components/<component>/                link → that component's .project/ tree
  locations                              table: host, path, kind, last seen, link status
  sessions/                              link → session catalog filtered to the project
  agents/                                link → live agents of the project
  private/                               personal notes (writable)
```

Sessions:

```text
session://                               sessions of the current project (newest first); ?scope=all
session://self/ → session://<id>/
session://<id>/
  meta                                   header, name, project, cwd, model, counts, timestamps
  transcript                             full current branch as Markdown, paged
  tree                                   entry tree with branches and labels
  entries/<entryId>                      one entry
  messages                               inter-session messages recorded in this session
```

Agents:

```text
agent://                                 live agents of the current project; ?scope=all
agent://self/ → agent://<id>/
agent://<id>/                            <id> = session short ID; also by name
  status                                 state, idle time, model, activity, pending ask
  inbox                                  read: own unread messages (?wait=60&from=<id>); write: send to that agent
  messages                               conversation with this agent
  session → session://<id>/
  asks/<askId>                           coordinator: read the pending form, write the answer
  ctl                                    coordinator: abort | compact | model <ref> | new | prompt <text>
agent://by-project/<slug>/<name> → agent://<id>/
```

Runtime:

```text
jobs://                                  jobs: id, state, runtime, command
jobs://<id>/
  status cmdline cwd
  log stdout stderr                      paged, selectors, grep-able
  stdin                                  write: send to stdin
  ctl                                    write: eof | kill | signal <name>
harness://                               Workbench itself
  status
  instructions/                          selected instruction files with reasons; write pin/unpin to ctl
  frames/<frameId>
  integrations/<owner>
  schemes/<scheme>
  ctl                                    write: reload
cfg://<group>/<key>                      effective value + provenance; write sets (typed, confirmed)
skill://<name>/{SKILL.md,…}              enabled skills and their files; read only
```

Contributed (SPEC-0004 providers): `kb://public/…`, `kb://project/…`, `git://…`, `research://…` (pig-web), any registered scheme.

# 6. Agent communication

## 6.1 Terms

An agent is an addressable conversation participant: today each live session's main agent; children of other extensions would appear as `agent://<id>/children/<childId>/` without changing the grammar.

## 6.2 Operations

| Action | URI form |
|---|---|
| List | `ls agent://`, `ls agent://?scope=all`, `ls projects://<slug>/agents/` |
| Look | `read agent://<id>/status` |
| Send | `write agent://<id>/inbox` (text, or JSON5 `{subject, body, attachments, wake, expectReceipt, replyTo, progress}`) |
| Receive | `read agent://self/inbox?wait=60&from=<id>` |
| History | `read agent://<id>/messages` |

## 6.3 Delivery (from omp)

Receipt `delivered` (aside at the next boundary), `woken` (idle agent started a turn), `queued` (receiver policy declined wake; delivered at its next turn), `failed` (reason). Delivered messages do not stay in the inbox. Unanswered sends are tracked; `progress: true` keeps a waiter waiting. Transport, authentication, loop guards, wake budgets and untrusted framing stay as in SPEC-0011.

## 6.4 Broadcast

`write projects://<slug>/agents/inbox` sends to every live agent of a project. Setting `sessions.broadcast` (default `project`; `off`, `all`).

## 6.5 Later idea, not designed

Delivering to a session that is not running by starting it headless from its transcript (omp `revived`). Recorded so the receipt enum leaves room for it.

# 7. Coordinator

## 7.1 What it is

A session the user marks as coordinator (`/workbench coordinator on`). Its agent can observe and operate every live session of the user in any project, in the user's name. Enabling is a user command only: never a tool call, never project config.

## 7.2 Capabilities

| Class | Operations | Mechanism in the target |
|---|---|---|
| observe | Status, activity, transcripts, pending asks, jobs | Endpoint `info`, `session://` |
| message | Messages without wake budget | SPEC-0011 |
| steer | Prompt as the user, steer, follow-up | `ctx.SendUserMessage` |
| answer | Answer or cancel Workbench `ask_user` forms | Workbench owns the form |
| lifecycle | Abort, compact, model and thinking changes, new session | `ctx.Abort`, `Compact`, `SetModel`, `NewSession` |
| jobs | List, write stdin, kill | SPEC-0008 supervisor |

Host limit: an extension cannot answer PiG's own dialogs, permission prompts or other extensions' `ctx.UI` prompts. The coordinator sees "waiting on host dialog".

## 7.3 Safeguards: all configurable

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

## 7.4 Interface

`agent://<id>/asks/<askId>` and `agent://<id>/ctl` (§5.2). Without a granted class they return `policyDenied` naming the class.

# 8. Effects on specs (after approval)

| Spec | Change |
|---|---|
| SPEC-0004 | §5: walking rules for `find`/`grep` over providers (bounds, `volatile`, `immutable`, `noWalk`, `ctl` entries). §9: replace route list with the trees of §5.2; project membership by project ID |
| SPEC-0005 | Hashline anchors on `project://` files (real backing files) |
| SPEC-0007 | Ambient class `projectPrivate`; AGENTS.md reminder; BACKLOG pointer validation |
| SPEC-0008 | Worktree creation applies §4.3 and §4.4; jobs tree with `stdin` and `ctl` |
| SPEC-0010 | Project registry, `Project.toml` schema, bucket symlinks, `coordinator.*` and `projects.*` settings |
| SPEC-0011 | Addressing via `agent://` trees, project-scoped discovery, broadcast, coordinator requests |
| New spec | Projects and conventions (§3–§4) |
| pig-web | KB project scope keyed by project ID (handoff `HO-workbench-answers`) |

# 9. Open questions

| # | Question |
|---|---|
| N1 | `project://` = current project's artifact tree and `projects://` = all projects (this draft), or `project://` = all projects with the current one under `project://self/`? |
| N2 | Keep `history://` and `branch://` as aliases of `session://`, or drop them? |
| N3 | Keep `proc://` as alias of `jobs://`; `self://`/`workbench://` as aliases of `harness://`? |
| N4 | `skill://` vs `skills://`: one name? |
| N5 | Session catalog: `session://` (current project) plus `projects://<slug>/sessions/`, or only one of them? |
| N6 | `.project/` per component repository (this draft) or one tree for the whole project in the primary component? |
