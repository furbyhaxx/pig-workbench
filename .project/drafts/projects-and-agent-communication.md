---
document:
  type: "draft"
  id: "DRAFT-0002"
  title: "Projects, the project artifact tree, pseudo-filesystem URIs, agent addressing and the coordinator"
  description: "Proposal for a stable Project entity with .project/Project.toml, the builtin .project convention shared across worktrees, all schemes as browsable pseudo-filesystems, agent:// communication and an opt-in coordinator"
  status: "archived"
  revision: 5
  created_at: "2026-11-03T08:25:00+01:00"
  updated_at: "2026-11-09T09:00:00+01:00"
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
    - revision: 3
      reason: "User decisions N0\u2013N6, docs:// scheme, first-touch scheme guides, question on storing artifacts in git refs"
      changes:
        - "Naming rule: singular schemes, root lists items, self = current; project:// lists projects; artifact:// = .project tree; out:// = spilled outputs; job://; skill:// only; history://, branch://, proc://, skills:// dropped"
        - "by-id/by-name/by-project link directories for session://, agent:// and project://"
        - "docs:// for PiG, Workbench and extension documentation and source; harness:// dissolved"
        - "Progressive scheme guides injected as frames on first touch"
        - "Analysed storing artifacts in git refs"
    - revision: 4
      reason: "Approved with Q1–Q3; superseded by SPEC-0012 and SPEC-0011 rev 2"
      changes: ["Archived with successor pointers", "pig-web line corrected per HO-pig-web-r6-r7 traps"]
    - revision: 5
      reason: "Fix a dangling reference found by the artifact check"
      changes: ["Marked the agentd-ts spec path as a host path"]
---

> **Archived.** Successors: [SPEC-0012](../specs/0012-projects-uri-namespace.md) (projects, conventions, URI namespace) and [SPEC-0011](../specs/0011-inter-session-messaging.md) rev 2 (agent://, coordinator). Decisions Q1–Q3 took the recommendations of §9.

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
- agentd-ts `host:/mnt/Projects/furbyhaxx/agentd-ts/.project/specs/SPEC-0001-architecture-and-daemon.md` §4: Workspace > Project (repo root `file://`/`ssh://`, default branch, worktrees) > Session (parent, location, branch).

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
primary = true                        # holds project-wide artifacts (§4.3); exactly one component
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
- `ls artifact://specs/` shows id, title, status and revision as columns from frontmatter.
- `Project.toml` lives in `.project/` and is not an artifact (no frontmatter).
- Dependency to resolve: the standard says live claims go to project-scope `tasks`, which Workbench does not provide (task tracking is excluded). The bundled skill must name whatever tasks tool is present, or drop the line when none is.

## 4.2 AGENTS.md and BACKLOG.md

SPEC-0007 already injects `AGENTS.md` and `BACKLOG.md` (ancestors, nearest, caps, diffs). Adoption adds: the bundled `agents-md` skill; a reminder frame when an agent creates a new module directory without `AGENTS.md` (configurable); and `BACKLOG.md` pointer validation (target item exists, item `status: "open"`).

## 4.3 `artifact://`: one artifact tree for all worktrees

`artifact://<path>` resolves to `<main worktree>/.project/<path>` of the current component, whichever checkout the agent works in. All agents of a project read and write the same artifacts. `project://self/artifacts/` links to the **primary** component's tree, which holds artifacts spanning components; `project://<slug>/components/<c>/artifacts/` reaches any component. Rule for the bundled skill: component-specific artifacts go into this repository's tree, artifacts spanning components into the primary's. A primary not checked out on this host is reported as unavailable with its remote.

On top of the URI, Workbench makes linked worktrees consistent (verified in the probe): per-worktree sparse checkout excludes `.project/`, a `.project` symlink points to the main worktree's tree, `/.project` goes into the common `info/exclude`. The order matters (`sparse.expectFilesOutsideOfPatterns` first); Workbench does it when it creates a worktree and repairs existing ones on request.

Consequences to accept:
- Artifacts are committed from the main worktree, on whatever branch it has checked out. Worktree branches never carry `.project/` changes and never conflict on them.
- An artifact describing unmerged work in a worktree is visible to everyone immediately. This is the point.

`share_project_tree = false` keeps per-branch `.project/` copies (plain git behaviour); `artifact://` then resolves to the current checkout.

## 4.5 Storing artifacts in git refs instead (analysis, not proposed as default)

Idea: keep `.project/` content out of the code branches, in its own ref.

Two variants:
- **Raw ref** `refs/pig/artifacts` written with plumbing (`hash-object`, a temporary index, `commit-tree`, `update-ref` with the expected old value as compare-and-swap). No files on disk.
- **Orphan branch** `artifacts` (no shared history with code), materialized as a worktree, e.g. `<repo>/.worktrees/.artifacts`, with `.project` in every code checkout a symlink to it.

| Aspect | Raw ref | Orphan branch + worktree | Current design (§4.3) |
|---|---|---|---|
| Shared across worktrees | Yes (refs live in the common dir) | Yes | Yes (sparse + symlink) |
| Humans open as files | No; only `git show artifacts:specs/x.md`, forge UI, or a FUSE mount | Yes, normal files in any editor | Yes |
| Agents' plain file tools | Need `artifact://` for everything | Work | Work |
| Code PRs carry spec changes | No | No | Yes, when committed in the main worktree on the PR branch |
| Teammates get it | Needs a fetch refspec `+refs/pig/*:refs/pig/*` on every clone | Fetched by default (it is a branch) | Fetched by default |
| Concurrent writers | Workbench must merge trees itself on CAS failure | Normal git | Normal git |
| Which artifact matched which code commit | Needs a trailer (`Code-Commit: <sha>`) per artifact commit | Same | Implicit in history |
| Effort | High (plumbing, merging, conflict UX) | Low (worktree + symlinks, as §4.3/§4.4) | Low |

Assessment: the raw ref is easy to read but hard to write correctly and hostile to humans; not worth it. The orphan branch is easy and keeps code history free of artifacts, at the price of losing "spec and code change in one PR" (which the `dot-project` rule "specs track reality, update in the same change" relies on). It fits as an option `[artifacts] storage = "tree" | "branch"` with `tree` as default. Decision Q1 in §9.

## 4.4 Worktree links

`[worktrees] links` lists untracked files or directories that every worktree gets as symlinks to the main worktree (`.env`, local config, `.references/`). Created when Workbench creates a worktree, checked on `cd` into one, listed under `project://<slug>/locations`. Tracked paths are rejected (they would need the sparse technique of §4.3). A link whose target is missing is reported, not created empty.

# 5. Pseudo-filesystem URIs

## 5.1 Naming rule (decided N0)

- One **singular** noun per scheme. The root lists the items; `self` is the current one.
- `by-<key>/` directories are **link directories** (like `/dev/disk/by-id/`): alternative ways to reach the same canonical entry. `ls` shows them as `link → <canonical URI>`.
- Every result reports the canonical URI, so stored references never split across names.
- Reserved segments in every scheme: `self`, `by-*`. Slugs and names cannot use them.
- Dropped names (`history://`, `branch://`, `proc://`, `skills://`, `jobs://`, `harness://`, `self://`, `workbench://`, `projects://`) return "unknown scheme" with a "did you mean" pointing to the replacement. No aliases.

## 5.2 Tool semantics

Every scheme is a tree, like `/proc` and `/sys`. The builtin tools work on it as on files:

| Tool | On a scheme |
|---|---|
| `ls` | Directory entries with typed columns (`ls job://` = jobs with state, runtime, command) |
| `read` | File content; selectors `:N`, `:A-B`, `:-N`, `:raw` |
| `find` / glob | Walk the tree by name pattern (`find session://by-name -name '*parser*'`) |
| `grep` | Search file contents across the tree (`grep -r timeout job://*/log`) |
| `write` | Write a file: content, a message, a setting, or a command to a control file |
| `edit` | Hashline edit where the file is writable |
| stat | Through `ls -l` / `read` metadata |

Entry kinds: `file`, `dir`, `link` (to another URI), `ctl` (write-only control file, like `/sys` attributes). Providers implement SPEC-0004 `stat`/`list`/`read`/`search`; when a provider has no `search`, Workbench walks with `list`+`read` under bounds (depth, entries, bytes, time) and reports incomplete results explicitly. Providers mark files `volatile` (changes on every read, e.g. status) or `immutable` (transcripts, artifacts), and subtrees `noWalk` (expensive) so recursive tools skip them unless named directly. Recursive tools never follow `by-*` links, so a walk visits each entry once.

Writes to `ctl` files are actions: they obey permissions and plan/read-only modes unless they only cancel (`kill`).

## 5.3 Trees

Files and locations:

```text
path | file:///abs | ssh://host/abs      real files (local or remote), all tools
local://                                 per-session scratch directory
out://<n>                                spilled large tool outputs, immutable
conflict://<n>/{ours,theirs,base,resolved}   merge conflict hunks; write resolved
http(s)://…                              web content via static reader or fetch backend; read only
<file>:<table>:<key>, <archive>/<inner>  adapters inside real paths (SPEC-0006)
```

Artifacts and projects:

```text
artifact://                              current component's .project/ tree (shared by all worktrees, §4.3)
  Project.toml  AGENTS.md
  specs/0001-extension-services.md       ls columns: id, title, status, revision
  plans/ analyses/ drafts/ backlog/ …
project://                               all projects: slug, id, components, locations, live agents
project://self → project://<slug>
project://<slug>/
  Project.toml                           registry view
  artifacts → primary component's artifact tree
  components/<component>/artifacts → that component's tree
  locations                              host, path, kind, last seen, worktree link status
  sessions → session://by-project/<slug>/
  agents → agent://by-project/<slug>/
  private/                               personal notes (writable)
project://by-id/<prj-id> → project://<slug>
```

Sessions (canonical `session://<id>`):

```text
session://                               sessions of the current project, newest first
session://self → session://<id>
session://<id>/
  meta                                   header, name, project, cwd, model, counts, timestamps
  transcript                             current branch as Markdown, paged
  tree                                   entry tree with branches and labels
  entries/<entryId>
  messages                               inter-session messages recorded in this session
  frames/<frameId>                       frames delivered in this session (was harness://frames)
  instructions/                          instruction files selected for this session, with reasons; ctl: pin/unpin (was harness://instructions)
session://by-id/<id> → session://<id>    all sessions of all projects
session://by-project/<slug>/<id> → session://<id>
session://by-name/<name> → session://<id>   ambiguous names list all candidates
```

Agents (canonical `agent://<id>`, `<id>` = session short ID):

```text
agent://                                 live agents of the current project
agent://self → agent://<id>
agent://<id>/
  status                                 state, idle time, model, activity, pending ask
  inbox                                  read: own unread messages (?wait=60&from=<id>); write: send to that agent
  messages                               conversation with this agent
  session → session://<id>
  asks/<askId>                           coordinator: read the pending form, write the answer
  ctl                                    coordinator: abort | compact | model <ref> | new | prompt <text>
agent://by-project/<slug>/<id> → agent://<id>
agent://by-name/<name> → agent://<id>
```

Runtime and configuration:

```text
job://                                   jobs: id, state, runtime, command
job://<id>/
  status cmdline cwd
  log stdout stderr                      paged, selectors, grep-able
  stdin                                  write: send to stdin
  ctl                                    write: eof | kill | signal <name>
cfg://<group>/<key>                      effective value + provenance; write sets (typed, confirmed)
cfg://ctl                                write: reload (was harness://reload)
skill://<name>/{SKILL.md,…}              enabled skills and their files; read only
```

Documentation (§5.4):

```text
docs://                                  documentation sets: owner, title, version, has source
docs://harness/                          PiG docs bundle of the running build
docs://harness/source/                   PiG source of the running version (read-only sparse checkout)
docs://workbench/                        Workbench docs
docs://workbench/schemes/<scheme>        full guide per scheme, including contributed ones (was harness://schemes)
docs://workbench/integrations/<owner>    registrations and their state (was harness://integrations)
docs://workbench/status                  Workbench runtime status (was harness://status)
docs://<owner>/ , docs://<owner>/source/ registered by other extensions (e.g. docs://pig-web/)
```

Contributed resource providers (SPEC-0004): `kb://public/…`, `kb://project/…`, `git://…`, `research://…` (pig-web), any registered scheme.

## 5.4 `docs://`

PiG materializes its docs for the running build in `~/.pig/docs/bundle-<hash>/` and names them in the system prompt. Workbench wraps that as `docs://harness/` and becomes the single entry point for documentation of the harness, Workbench and other extensions.

- **PiG docs.** `docs://harness/` maps to the running build's bundle (`pig docs path`). Workbench replaces the bundle paths in the system prompt with `docs://harness/…` (mechanism to verify: system prompt hook).
- **PiG source.** A read-only sparse checkout of the PiG repository at the running version (`pig --version` → `0.5.3+1.4.0.74f46864`, commit `74f46864`), reachable as `docs://harness/source/`. Location: you asked for `~/.pig/docs/source/{version|current}/`. Risk: PiG owns `~/.pig/docs/` and prunes old bundles on sync; it documents pruning only for `bundle-*`, so a sibling `source/` probably survives, but that is PiG's directory. Alternative: Workbench data dir with the same URI. Decision Q2.
- **Fetching.** First access clones (blobless, sparse: docs, `extensions/sdk`, `cmd`, `internal` or a configured set) and checks out the build's commit. Network access is explicit, never at startup; offline shows what is cached. Versions other than the running one are retained by count (default 3).
- **Extensions.** Registration kind `docs`: `{key, title, version, root, source?: {repo, ref, paths} | {dir}}`. `root` must be a directory inside the extension's package or data directory. Contributions appear as `docs://<owner>/` and `docs://<owner>/source/`, read-only, searchable.
- **Every docs tree** is `immutable`, `grep`/`find`/`ls` work, hashline anchors are off (nothing to edit).

## 5.5 Progressive guides on first touch

Tool descriptions and the system prompt carry only what is always needed: one line per scheme ("`session://` sessions: ls, read, by-id/by-project/by-name"). The full usage guide arrives when the agent first touches a scheme or a sub-namespace in a session:

- Each scheme (builtin or registered) declares a **guide**: short Markdown (≤ 2 KiB) about its tree, link directories, writable files, `ctl` commands, limits and examples. The full version lives at `docs://workbench/schemes/<scheme>`.
- The first `ls`/`read`/`find`/`grep`/`write` on a scheme (and on a sub-namespace that declares its own guide, e.g. `session://by-project/`) injects the guide as a frame (SPEC-0002, kind `instruction`, source `workbench.guide.<scheme>`), alongside the tool result, like `AGENTS.md` on first path touch (SPEC-0007).
- Once per session and guide revision. Re-injected after compaction drops it, or when the guide revision changes (provider update).
- Settings: `guides.enabled` (true), `guides.maxBytes`, per-scheme opt-out.
- A provider's guide is data authority unless the user trusts that provider (SPEC-0002 authority rules), so a contributed scheme cannot smuggle instructions.

# 6. Agent communication

## 6.1 Terms

An agent is an addressable conversation participant: today each live session's main agent; children of other extensions would appear as `agent://<id>/children/<childId>/` without changing the grammar.

## 6.2 Operations

| Action | URI form |
|---|---|
| List | `ls agent://`, `ls agent://by-project/<slug>/`, `ls agent://by-name/` |
| Look | `read agent://<id>/status` |
| Send | `write agent://<id>/inbox` (text, or JSON5 `{subject, body, attachments, wake, expectReceipt, replyTo, progress}`) |
| Receive | `read agent://self/inbox?wait=60&from=<id>` |
| History | `read agent://<id>/messages` |

## 6.3 Delivery (from omp)

Receipt `delivered` (aside at the next boundary), `woken` (idle agent started a turn), `queued` (receiver policy declined wake; delivered at its next turn), `failed` (reason). Delivered messages do not stay in the inbox. Unanswered sends are tracked; `progress: true` keeps a waiter waiting. Transport, authentication, loop guards, wake budgets and untrusted framing stay as in SPEC-0011.

## 6.4 Broadcast

`write agent://by-project/<slug>/inbox` sends to every live agent of a project. Setting `sessions.broadcast` (default `project`; `off`, `all`).

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

`agent://<id>/asks/<askId>` and `agent://<id>/ctl` (§5.3). Without a granted class they return `policyDenied` naming the class.

# 8. Effects on specs (after approval)

| Spec | Change |
|---|---|
| SPEC-0001 | Registration kind `docs`; scheme `guide` field |
| SPEC-0002 | Guide frames (`workbench.guide.<scheme>`) |
| SPEC-0004 | §2 reserved scheme list per §5.1; §5 walking rules (bounds, `volatile`, `immutable`, `noWalk`, `ctl`, links not followed); §9 replaced by the trees of §5.3; project membership by project ID |
| SPEC-0005 | Hashline anchors on `artifact://` files (real backing files) |
| SPEC-0006 | `artifact://` → `out://` for spilled outputs |
| SPEC-0007 | Ambient class `projectPrivate`; guides on first touch; AGENTS.md reminder; BACKLOG pointer validation |
| SPEC-0008 | Worktree creation applies §4.3 and §4.4; `job://` tree with `stdin` and `ctl` |
| SPEC-0009 | Inspection routes moved to `session://self/frames`, `session://self/instructions`, `docs://workbench/…`, `cfg://ctl` |
| SPEC-0010 | Project registry, `Project.toml` schema, bucket symlinks, `coordinator.*`, `projects.*`, `guides.*`, docs cache settings |
| SPEC-0011 | Addressing via `agent://` trees, project-scoped discovery, broadcast, coordinator requests |
| PLAN-0001, sketches | Scheme names in WB-07…WB-10 and the UI captures |
| New spec | Projects, conventions and URI namespace (§3–§5) |
| pig-web | Provenance and private items key on the project ID; `docs://pig-web/` registration |

# 9. Decisions

Decided (rev 3): N0 singular rule; N1 `project://` = all projects, `artifact://` = artifact tree, `out://` = spilled outputs; N2 drop `history://`/`branch://`; N3 `job://`, `docs://` replaces `harness://`; N4 `skill://`; N5 `by-id/`, `by-name/`, `by-project/` link directories with canonical `session://<id>`; N6 per-component trees plus a primary.

Open:

| # | Question |
|---|---|
| Q1 | Artifacts storage option `tree` (default) / `branch` (orphan branch + worktree), or tree only? (§4.5) |
| Q2 | PiG source checkout under `~/.pig/docs/source/` (PiG's directory) or Workbench's data directory, same URI either way? (§5.4) |
| Q3 | Where Workbench runtime state goes now that `harness://` is gone: frames/instructions under `session://self/`, status and integrations under `docs://workbench/`, reload as `cfg://ctl` (this draft)? |
