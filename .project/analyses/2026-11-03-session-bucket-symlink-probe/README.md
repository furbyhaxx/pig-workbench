---
document:
  type: "analysis"
  id: "ANA-2026-11-03-session-bucket-symlink-probe"
  title: "Probe: project-shared session buckets via symlinks, and worktrees sharing .project/"
  description: "Whether PiG writes, lists and continues sessions through symlinked session buckets, and whether a linked git worktree can use the main worktree's .project/ and .env without git noise"
  status: "published"
  revision: 1
  created_at: "2026-11-03T08:50:00+01:00"
  updated_at: "2026-11-03T08:50:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "Symlinked session buckets (SPEC-0012 §3.5) and the shared artifact tree needed evidence before design"
      changes: ["Ran both probes with a mock model and a scratch git repository"]
---

# 1. Method

- `pig` from `PATH` in RPC mode, isolated `PIG_HOME` copy at `/tmp/wb-symprobe/home`, scripted mock model from the [host probes harness](../2026-11-02-host-integration-probes/evidence/harness/mock.py). No billed inference.
- Two working directories `work/repo` and `work/clone`. Their buckets `sessions/--tmp-wb-symprobe-work-repo--` and `…-clone--` were created beforehand as symlinks to one directory `projects/prj-test/sessions/`.
- Driver [`evidence/run.sh`](evidence/run.sh): one prompt, wait for `agent_end`, `get_state`.
- Catalog: probe extension `probe-m0b` command `/probe-catalog` (`SessionManagerList` for the cwd, `SessionManagerListAll`).
- Worktree sharing: [`evidence/worktree-share.sh`](evidence/worktree-share.sh), git 2.56.0.

# 2. Results: symlinked session buckets

| Check | Result | Evidence |
|---|---|---|
| New session in `repo` | File written through the symlink into `projects/prj-test/sessions/`; `sessionFile` reports the symlink path | `rpc-r1.jsonl`, `tree.txt` |
| New session in `clone` | Same directory | `rpc-c1.jsonl` |
| `--continue` in `repo` | Continued the **clone's** session (most recent in the shared directory); the session header keeps `cwd: …/clone` | `rpc-r2.jsonl` |
| Symlinks after runs | Untouched; PiG did not replace or canonicalize them | `tree.txt` |
| `SessionManagerList(cwd)` | 2 sessions (both locations) | `catalog.json` |
| `SessionManagerListAll` | **4**: every file once per symlinked bucket | `catalog.json` |
| Bucket creation | PiG creates the bucket directory at startup, before the first message | first baseline run |

Consequences:
- Option B works without host changes.
- PiG's all-projects listing (`/resume` all view, `ListAll`) shows a session once per location of its project. Workbench catalogs deduplicate by session ID; the host view stays duplicated until upstream dedups by real path.
- `--continue`/`/resume` in one checkout can pick a session recorded in another checkout. Not yet probed: which cwd tools use in that resumed session.
- The bucket already exists as a real directory when an extension starts, so adoption must migrate a real directory into a symlink (§4).

# 3. Results: worktree sharing `.project/` and `.env`

With `extensions.worktreeConfig`, per-worktree `sparse.expectFilesOutsideOfPatterns=true` **set before** `git sparse-checkout set --no-cone '/*' '!/.project/'`, a `.project → ../../.project` symlink and `/.project` in the common `info/exclude`:
- `git status` in the worktree is clean, the index keeps `S` (skip-worktree) for `.project/**`.
- Commits in the worktree contain only code; `.project` edits land in the main worktree's working copy.
- Merging the branch into `main` touches no `.project` file.

Wrong order (symlink or sparse set before the config) clears the skip-worktree bit and `git status` shows `D .project/…` permanently; `git update-index --skip-worktree` repairs it.

`.env` (untracked, ignored) as a plain symlink needs no git changes.

# 4. Open follow-ups

- cwd used by tools after `--continue` picks another checkout's session.
- Migrating an existing real bucket into the project directory while another PiG process has a session open in it: rename on the same filesystem keeps open file handles valid; across filesystems it does not. Requires checking live endpoints first (SPEC-0011 registry).
- Windows: symlinks need developer mode or admin; directory junctions are the likely substitute. Untested.
