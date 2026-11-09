---
document:
  type: "handoff"
  id: "HO-pig-web-r9-r10"
  title: "pig-web requests R9 (project in location) and R10 (provider search dispatch)"
  description: "Two gaps pig-web found when aligning its specs with the implemented Slice 1 seams"
  status: "published"
  revision: 2
  created_at: "2026-11-09T11:50:00+01:00"
  updated_at: "2026-11-09T15:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 2
      reason: "Both requests implemented and answered in pig-web HO-workbench-r9-r10"
      changes: ["R9 landed in 805610d, R10 in 6751215; archived"]
---

# Goal

Answer R9 and R10 so pig-web can drop two interim workarounds. Neither blocks pig-web's start: both have a working fallback. Reply as before with a handoff into `host:/mnt/Projects/furbyhaxx/pig-web/.project/handoff/`, naming the commit.

# Current state

- pig-workbench `dev` at `0e20c0c` (Slice 1 final verification). Uncommitted `internal/host/liveeval_ab_test.go`, `.pi/`, `.pig/` and analysis evidence were present and are not touched.
- pig-web `dev` at `82227c0`: SPEC-0001 rev 7 §3 is written against your implemented `api` package (`api.Client`, `api.Serve`), not the drafts. Your `HO-pig-web-m0-4-answers` rev 4 is applied there. pig-web asks its user whether to import your `api` module (pig-web PLAN-0001 D12); the requests below hold either way.

# R9: project identity in the location snapshot

**Fact.** `internal/location/location.go` `Snapshot` has `authority, cwd, displayCwd, projectRoot, origin, revision, available`. `project {id, slug, component?}` appears only in `api.ResourceRequest` and fetch requests (SPEC-0004 §4).

**Why pig-web needs it.** pig-web's KB is one central store per user. Private items, provenance (`seenIn[] = {projectId, at}`) and the project relevance boost are keyed on your project ID. Most of that happens in pig-web's own tools (`web_search`, `web_fetch`, `web_kb`), which receive no provider request, so they only have the location.

**pig-web's interim.** It reads `.project/Project.toml` under `location.projectRoot` itself, else uses a path key, and keeps an alias table that merges a path key with a `prj-` ID once seen. That misses your git-identity matching (root commit, remote) for repositories without `Project.toml`, so such projects get a different identity in pig-web than in Workbench.

**Request.** Add the same `project` object to `location.get`, the `discover` result's location and both sides of `location.changed`, absent in the builtin `global` project exactly as in provider requests. Alternatively a `project.get` method returning it for the caller's current location.

# R10: provider `search` dispatch

**Fact.** `internal/resource/contributed.go` accepts `search` in `providerCapabilities`, but `internal/host/contributed_resources.go` only dispatches `stat`, `list` and `read` (plus mutations). `grep`/`find` on a contributed scheme therefore walk with `list`+`read` (`internal/resource/walk.go`), bounded and honouring `noWalk`.

**Why it matters.** `kb://` holds thousands of articles and source snapshots; pig-web marks `kb://sources/` and `research://` `noWalk`, so `grep kb://…` is incomplete by design. pig-web has an FTS5 + BM25 index that answers the same question exactly and cheaply.

**pig-web's interim.** Its first-touch guides tell the agent to use `web_kb search`; `grep kb://articles/` still walks the article subtree within your bounds.

**Request.** When a provider declares `search`, dispatch `grep` (and `find` by name) to it as `operation: "search"` with the pattern, literal/regex flag, case flag, path prefix and limits, and render the provider's matches (canonical URI, line, text) like local matches, with the provider's `complete` flag. If you prefer a different shape (a provider-supplied query result read as a virtual file), say which; pig-web adapts.

# Traps

- Do not add per-project partitioning of contributed schemes for R9; pig-web wants the ID as a fact, not a scope.
- pig-web's specs are drafts; re-read pig-web SPEC-0001 §3.3 and §3.6 at the commit you answer against.
