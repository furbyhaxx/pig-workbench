---
document:
  type: "specification"
  id: "SPEC-0012"
  title: "Projects, artifact conventions and the URI namespace"
  description: "Stable project identity with .project/Project.toml, project-shared session storage, builtin .project/AGENTS.md conventions shared across worktrees, and every scheme as a browsable pseudo-filesystem including docs:// and first-touch guides"
  status: "draft"
  revision: 7
  created_at: "2026-11-03T10:00:00+01:00"
  updated_at: "2026-11-05T17:10:00+01:00"
  created_by: "orchestrator"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Approved DRAFT-0002 rev 3 with decisions N0–N6 and Q1–Q3"
      changes: ["Specified project model, identity, storage, conventions, artifact storage modes, naming rule, tool semantics, scheme trees, docs:// and guides"]
    - revision: 2
      reason: "Secrets redesigned after nukii-revived (SPEC-0013)"
      changes: ["secret:// in the tree list"]
    - revision: 3
      reason: "All config files are TOML (SPEC-0010 §1)"
      changes: ["Setting keys renamed to kebab-case"]
    - revision: 4
      reason: "User: agents read and change session titles through URIs"
      changes: ["session://<id>/title read/write with per-target mechanism and permissions", "Title = PiG session name; by-name resolves titles", "Probes T1–T3"]
    - revision: 5
      reason: "Progressive skill disclosure and served prompt (SPEC-0014)"
      changes: ["skill:// load through read, unload through ctl, loaded/ listing", "session://<id>/system-prompt", "docs://workbench/prompts"]
    - revision: 6
      reason: "Host changes go to a personal PiG fork (PLAN-0001 D13)"
      changes: ["Session dedup by real path is a fork change"]
    - revision: 7
      reason: "Probes T1–T3 and the resumed-cwd question are answered (ANA-2026-11-05-slice-1-probes)"
      changes: ["Titles: immediate visibility, empty name clears, persistence as session_info, branch-scoped precedence", "Resumed cwd through symlinked buckets follows the recorded session cwd"]
---

# 1. Scope

What Workbench provides for projects and for addressing everything it exposes. Source proposal with prior art (oh-my-pi, nukii, nugent, agentd): [DRAFT-0002](../drafts/projects-and-agent-communication.md). Live-session messaging and the coordinator: [SPEC-0011](0011-inter-session-messaging.md). Provider contract: [SPEC-0004](0004-resource-providers.md).

# 2. Host facts

| Fact | Source |
|---|---|
| Sessions in `~/.pig/agent/sessions/--<encoded cwd>--/`, append-only JSONL; bucket created at startup | `docs/sessions.md`, probe |
| Symlinked buckets: writes, per-cwd listing and `--continue` work through them; `ListAll` lists each file once per symlink | [ANA session-bucket probe](../analyses/2026-11-03-session-bucket-symlink-probe/README.md) |
| Extensions can create/open/fork/list sessions in any directory and switch to any file | SDK `session_manager_class.go`, `context.go` |
| Own-session control: `SendUserMessage`, `SendMessage`, `Abort`, `IsIdle`, `WaitForIdle`, `Compact`, `SetModel`, `SetThinkingLevel`, `NewSession`, `Shutdown` | SDK `context.go` |
| No hook to choose where the host creates its session file | SDK, absence |
| Session title = PiG session name: `GetSessionName`/`SetSessionName` for the own session, `SessionManager.AppendSessionInfo` (a `session_info` entry) for an opened file | SDK `context.go`, `session_manager_class.go`, `docs/session-format.md` |

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

[artifacts]
storage = "tree"                      # or "branch" (§4.5)

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
- **Known host effects.** The all-projects listing shows each session once per location (Workbench catalogs deduplicate by session ID; the PiG fork deduplicates by real path, PLAN-0001 D13). `--continue` may pick a session recorded in another checkout of the same project (by design of B). Probed 05.11.2026 ([ANA](../analyses/2026-11-05-slice-1-probes/README.md)): the resumed session uses its **recorded** cwd, not the process cwd of the checkout that ran `--continue`, and reports its file through the current checkout's bucket symlink.
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

On top of the URI, Workbench makes linked worktrees consistent ([verified](../analyses/2026-11-03-session-bucket-symlink-probe/README.md) §3): per-worktree sparse checkout excludes `.project/`, a `.project` symlink points to the main worktree's tree, `/.project` goes into the common `info/exclude`. The order matters (`sparse.expectFilesOutsideOfPatterns` first); Workbench does it when it creates a worktree and repairs existing ones on request.

Consequences to accept:
- Artifacts are committed from the main worktree, on whatever branch it has checked out. Worktree branches never carry `.project/` changes and never conflict on them.
- An artifact describing unmerged work in a worktree is visible to everyone immediately. This is the point.

`share_project_tree = false` keeps per-branch `.project/` copies (plain git behaviour); `artifact://` then resolves to the current checkout.

## 4.4 Worktree links

`[worktrees] links` lists untracked files or directories that every worktree gets as symlinks to the main worktree (`.env`, local config, `.references/`). Created when Workbench creates a worktree, checked on `cd` into one, listed under `project://<slug>/locations`. Tracked paths are rejected (they would need the sparse technique of §4.3). A link whose target is missing is reported, not created empty.

## 4.5 Artifact storage: tree or branch

`[artifacts] storage` selects where `.project/` content is committed:

- **`tree`** (default): `.project/` lives in the code tree. Worktrees share the main worktree's copy (§4.3). Spec and code changes can land in one commit or PR, as the `dot-project` rule "specs track reality" expects.
- **`branch`**: `.project/` content lives on an orphan branch `artifacts` (no shared history with code), checked out by Workbench as worktree `<repo>/.worktrees/.artifacts`. `.project` in every code checkout, including the main one, is a symlink to it, excluded through the common `info/exclude`; no sparse checkout is needed because code branches do not track `.project/`. `artifact://` resolves to that worktree. Code history stays free of artifacts. Workbench adds a `Code-Commit: <sha>` trailer to artifact commits it makes, recording the main worktree's `HEAD`. Teammates fetch the branch like any other.

Switching modes is an explicit migration command (`/workbench projects artifacts-storage <mode>`) that moves the files and commits on both sides; it never happens implicitly. A bare ref without a checkout (`refs/pig/artifacts`) was evaluated and rejected: humans cannot open the files, agents' file tools cannot reach them, and concurrent writes would need Workbench's own tree merging ([DRAFT-0002 rev 3 §4.5](../drafts/projects-and-agent-communication.md)).

# 5. Pseudo-filesystem URIs

## 5.1 Naming rule

- One **singular** noun per scheme. The root lists the items; `self` is the current one.
- `by-<key>/` directories are **link directories** (like `/dev/disk/by-id/`): alternative ways to reach the same canonical entry. `ls` shows them as `link → <canonical URI>`.
- Every result reports the canonical URI, so stored references never split across names.
- Reserved segments in every scheme: `self`, `by-*`. Slugs and names cannot use them.
- Names that do not exist by this rule (`history://`, `branch://`, `proc://`, `skills://`, `jobs://`, `harness://`, `self://`, `workbench://`, `projects://`) return "unknown scheme" with a "did you mean" pointing to the replacement. No aliases.

## 5.2 Tool semantics

Every scheme is a tree, like `/proc` and `/sys`. The builtin tools work on it as on files:

| Tool | On a scheme |
|---|---|
| `ls` | Directory entries with typed columns (`ls job://` = jobs with state, runtime, command) |
| `read` | File content; selectors `:N`, `:A-B`, `:-N`, `:raw` |
| `find` / glob | Walk the tree by name pattern (`find session://by-name -name '*parser*'` finds sessions by title) |
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
  meta                                   header, title, project, cwd, model, counts, timestamps
  title                                  read: current title (empty when untitled); write: set it (§5.3.1)
  system-prompt                          prompt served in the current epoch, by section (SPEC-0014 §4.3); read only
  transcript                             current branch as Markdown, paged
  tree                                   entry tree with branches and labels
  entries/<entryId>
  messages                               inter-session messages recorded in this session
  frames/<frameId>                       frames delivered in this session
  instructions/                          instruction files selected for this session, with reasons; ctl: pin/unpin
session://by-id/<id> → session://<id>    all sessions of all projects
session://by-project/<slug>/<id> → session://<id>
session://by-name/<title> → session://<id>   ambiguous titles list all candidates
```

### 5.3.1 Session titles

The title is PiG's session name; `by-name/` resolves it, listings show it as `Title`. Writing `session://<id>/title` sets it:

| Target | Mechanism | Allowed |
|---|---|---|
| Own session (`self`) | `ctx.SetSessionName` | always, in every mode (session metadata only, like `/name`) |
| Another live session | `control` request over its endpoint (SPEC-0011); the target calls `ctx.SetSessionName` | coordinator with class `lifecycle`; otherwise `policyDenied` |
| A stored session nobody runs | `SessionManagerOpen` + `AppendSessionInfo` | after user confirmation; refused with `sessionLive` when an endpoint for it appears |

- Value: one line, control characters stripped, at most 200 characters; an empty write clears the title. Secrets placeholders stay placeholders (the title is shown to agents and users).
- Workbench's own session listings and frames pick up the change on the next read; PiG's header follows its own state update.
- Automatic title generation comes later (PLAN-0001); it will write through the same path.

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
cfg://ctl                                write: reload
cfg://workbench/status                   Workbench runtime status
cfg://workbench/integrations/<owner>     registrations and their state
skill://<name>/{SKILL.md,…}              listed skills and their files; reading skill://<name> loads it (SPEC-0014 §8.2)
skill://<name>/ctl                       write: unload
skill://loaded/                          skills loaded in this session, with reason (explicit, auto)
secret://{global,project/<slug>}/<name>  placeholder + metadata, never the value; write stores (SPEC-0013 §6)
```

Documentation (§5.4):

```text
docs://                                  documentation sets: owner, title, version, has source
docs://harness/                          PiG docs bundle of the running build
docs://harness/source/                   PiG source of the running version (read-only sparse checkout)
docs://workbench/                        Workbench docs
docs://workbench/schemes/<scheme>        full guide per scheme, including contributed ones
docs://workbench/prompts                 template variables, helpers, families, gonja differences (SPEC-0014)
docs://<owner>/ , docs://<owner>/source/ registered by other extensions (e.g. docs://pig-web/)
```

Contributed resource providers (SPEC-0004): `kb://public/…`, `kb://project/…`, `git://…`, `research://…` (pig-web), any registered scheme.

## 5.4 `docs://`

PiG materializes its docs for the running build in `~/.pig/docs/bundle-<hash>/` and names them in the system prompt. Workbench wraps that as `docs://harness/` and becomes the single entry point for documentation of the harness, Workbench and other extensions.

- **PiG docs.** `docs://harness/` maps to the running build's bundle (`pig docs path`). Workbench replaces the bundle paths in the system prompt with `docs://harness/…` (mechanism to verify: system prompt hook).
- **PiG source.** A read-only sparse checkout of the PiG repository at the running version (`pig --version`, e.g. `0.5.3+1.4.0.74f46864` → commit `74f46864`), reachable as `docs://harness/source/`. Stored in Workbench's data directory (`$XDG_DATA_HOME/pig-workbench/docs/source/<version>/`), with a convenience symlink `~/.pig/docs/source` → that directory, so PiG's bundle pruning can never remove it and users still find it beside the docs.
- **Fetching.** First access clones (blobless, sparse: docs, `extensions/sdk`, `cmd`, `internal` or a configured set) and checks out the build's commit. Network access is explicit, never at startup; offline shows what is cached. Versions other than the running one are retained by count (default 3).
- **Extensions.** Registration kind `docs`: `{key, title, version, root, source?: {repo, ref, paths} | {dir}}`. `root` must be a directory inside the extension's package or data directory. Contributions appear as `docs://<owner>/` and `docs://<owner>/source/`, read-only, searchable.
- **Every docs tree** is `immutable`, `grep`/`find`/`ls` work, hashline anchors are off (nothing to edit).

## 5.5 Progressive guides on first touch

Tool descriptions and the system prompt carry only what is always needed: one line per scheme ("`session://` sessions: ls, read, by-id/by-project/by-name"). The full usage guide arrives when the agent first touches a scheme or a sub-namespace in a session:

- Each scheme (builtin or registered) declares a **guide**: short Markdown (≤ 2 KiB) about its tree, link directories, writable files, `ctl` commands, limits and examples. The full version lives at `docs://workbench/schemes/<scheme>`.
- The first `ls`/`read`/`find`/`grep`/`write` on a scheme (and on a sub-namespace that declares its own guide, e.g. `session://by-project/`) injects the guide as a frame (SPEC-0002, kind `instruction`, source `workbench.guide.<scheme>`), alongside the tool result, like `AGENTS.md` on first path touch (SPEC-0007).
- Once per session and guide revision. Re-injected after compaction drops it, or when the guide revision changes (provider update).
- Settings: `guides.enabled` (true), `guides.max-bytes`, per-scheme opt-out.
- A provider's guide is data authority unless the user trusts that provider (SPEC-0002 authority rules), so a contributed scheme cannot smuggle instructions.


# 6. Acceptance

- Identity: `Project.toml` wins; a repository without it matches by root commit, remote, then (host, path); unknown directories land in `global` and are never auto-registered; conflicting claims are shown.
- Storage: bucket conversion at session start, waits when another live session uses the bucket or filesystems differ; Workbench catalogs list each session once; undo restores real directories.
- Worktrees: creating a worktree applies sparse checkout, `.project` symlink, exclude and `[worktrees] links` in the verified order; `git status` stays clean; repair command fixes worktrees made outside Workbench.
- `branch` storage: migration in both directions; artifact commits carry `Code-Commit`.
- Conventions: frontmatter and `BACKLOG.md` pointer validation report problems without blocking writes.
- URIs: every tree of §5.3 supports `ls`, `read`, `find` and `grep` within bounds; links are not followed by recursive tools; `ctl` writes obey modes; unknown names give "did you mean".
- Guides: injected once per session and revision on first touch, re-injected after compaction, untrusted provider guides stay data.
- `docs://harness/` follows the running build; source fetch only on first access, offline shows cache; extension docs confined to their roots.
- Titles: own, other live (coordinator `lifecycle` only) and stored sessions; the new title appears in `meta`, `by-name/`, `ls session://` and PiG's session picker. Probes answered 05.11.2026 ([ANA](../analyses/2026-11-05-slice-1-probes/README.md)): **T1** `SetSessionName` is visible to `GetSessionName` and the replicated `SessionName()` without reload, and an empty name clears the title; **T2** the title is persisted as a `session_info` entry in the session file and survives `--continue`; **T3** `session_info` is branch-scoped — forking before a rename leaves the session unnamed, so `by-name/` resolves the active branch's latest entry and never an entry from an inactive branch.
- Evidence for storage and worktree behaviour: [ANA-2026-11-03](../analyses/2026-11-03-session-bucket-symlink-probe/README.md).
