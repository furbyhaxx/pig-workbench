---
document:
  type: "handoff"
  id: "HO-pig-web-r6-r7"
  title: "pig-web follow-up requests R6 (unreadable secret sources), R7 (fetch backend shape) and R8 (project identity in provider requests)"
  description: "Three open questions from pig-web after adopting the Workbench collaboration services of commit 09e1030"
  status: "archived"
  revision: 3
  created_at: "2026-11-03T08:45:44+01:00"
  updated_at: "2026-11-03T10:00:00+01:00"
  created_by: "coding-assistant"
  updated_by: "orchestrator"
  changelog:
    - revision: 3
      reason: "Consumed by pig-workbench SPEC-0001 rev 4, SPEC-0004 rev 5, SPEC-0008 rev 5; reply HO-workbench-r6-r8 in pig-web"
      changes: ["Archived"]
---

# Goal

Answer three requests from pig-web so its Workbench integration (pig-web SPEC-0001 §3.7, §3.8, §3.13) can be finalized. Reply the same way as last time: a handoff into `host:/mnt/Projects/furbyhaxx/pig-web/.project/handoff/`, plus edits to the Workbench specs named below.

# Current state

- pig-workbench: branch `main`, commit `ac1df52` (specs `09e1030` unchanged since). Untracked `.project/analyses/2026-11-03-session-bucket-symlink-probe/` was present when this was written and is not touched.
- pig-web: branch `dev`, SPEC-0001 rev 5 (commit after `6089cf7`). §3 adopts R1–R5 and the `location`, `secrets`, `status`, `fetch` and optional `sessions` capabilities. Your handoff `HO-workbench-answers` is archived there. Since rev 5 the KB is central per user (R8) and pig-web serves all `read https://` (R7).
- pig-web credentials: no separate file any more. Search vendors (`exa`, `tavily`, `serper`, `serpapi`, `brave`, `parallel`, `kagi`, `firecrawl`, `jina`, `perplexity`, `ollama-web`, `tinyfish`, `searxng`) register as key-only native providers, so PiG stores their keys in `<agent-dir>/auth.json` like any other provider (`/login exa` works). Evidence: `host:/mnt/Projects/furbyhaxx/pig-web/.project/analyses/2026-11-02-auth-json-probe/README.md`.

# R6: secrets Workbench cannot read by reference

**Context.** `secrets.registerSources` accepts `{kind:"env"}` and `{kind:"jsonFile"}` inside the agent dir or the owner's config root (your SPEC-0001 §8). pig-web has two credential sources that neither reference reaches:

1. **Project `.env`.** With the user-only setting `auth.allowDotEnv` and a trusted project, pig-web reads `<host session cwd>/.env` into memory. The file is outside the agent dir and is not JSON. Values are never exported to the process environment, so an `env` reference names nothing.
2. **`!command` keys.** A stored key like `"key": "!pass show exa"` reaches an extension provider's `Resolve` unexpanded (probe row 9); pig-web runs the command itself. The value exists only in pig-web's memory.

**Related question.** Your SPEC-0008 G8.1 inventory already includes "PiG credential stores". If that means Workbench reads all of `auth.json` itself, pig-web's `jsonFile` pointers to `auth.json` are redundant. Does the core inventory (a) cover every entry of `auth.json`, including the search vendors, and (b) expand `!command` values the way the host does for built-in providers?

**Options pig-web sees.**

| Option | What Workbench adds | Trade-off |
|---|---|---|
| A. `dotenvFile` reference | `{kind:"dotenvFile", path, names}`, allowed only for `<host session cwd>/.env` of a trusted project, user-owned, not group/world-writable | Covers source 1. Workbench parses one more format and must recheck trust when it reads |
| B. Expand `!command` for referenced pointers | When a referenced JSON value starts with `!`, Workbench runs it with the host's semantics (same timeout, no shell interpolation of other data) and guards the output | Covers source 2. Workbench runs user commands it did not configure; must never run them for unreferenced pointers |
| C. Document as unguarded | Nothing | pig-web labels such keys "not covered by Workbench redaction" in `/web auth`; leaked into a `jobs` log they would appear in clear |

**pig-web's preference:** A for `.env`. For `!command`, B only if the core inventory already expands `!command` for built-in providers (then it is one code path); otherwise C. Never values over the bus.

**Where to answer:** your SPEC-0001 §8 (reference kinds) and SPEC-0008 G8.1 (inventory scope).

# R7: fetch backend registration and answer shape

**Context.** pig-web's user decided that while pig-web is loaded it serves **every** `read http(s)://…`, and Workbench's builtin reader serves it otherwise (pig-web PLAN-0001 D6). pig-web registers one `fetchBackend` (pig-web SPEC-0001 §3.7):

```json
{"key":"pig-web","match":[{"host":"*"}],"capabilities":["fetch","locate","offline"],"egress":"thirdParty","limits":{…}}
```

`egress` is `thirdParty` only while remote reader services are enabled in pig-web; otherwise `none`. pig-web answers from its central KB when it holds a fresh snapshot, else runs its tier pipeline (structured site APIs → native Markdown sources and `llms.txt` → local HTML extraction → remote readers when local results fail a quality check).

**Questions.**

1. May a `fetch` answer `locate` with a provider URI (`kb://sources/7c/7c41…`, served by pig-web's own `kb` resource provider), with Workbench then resolving that URI through the provider's `locate`? Or must `locate` in a fetch answer be a backing file reference directly?
2. Every page read through pig-web is stored in its central KB (that is the point: later sessions and projects reuse it). Is that side effect acceptable for `read https://`, given Workbench's own HTTP cache? pig-web discloses it in its settings and the answer's `handler`/`notices`.
3. `egress` is declared per registration, but actual third-party use depends on whether local tiers sufficed; pig-web reports `usedThirdParty` per answer. Is "registration declares the upper bound, the answer reports the truth" the intended model?
4. Re-registration when reader settings change (egress flips): replace with `descriptorRevision` as usual?
5. With a catch-all `*` from pig-web, is the builtin reader still used for hosts pig-web reports as failed (timeout, cooldown), with your notice? (pig-web SPEC-0001 §3.7 step 3 assumes yes.)

**pig-web's preference:** `locate` to the `kb://` URI (one resolution path, your router authorizes once); KB storage disclosed, not optional per read; upper-bound egress with per-answer truth.

**Where to answer:** your SPEC-0004 §11.

# R8: caller project identity in provider requests

**Context.** pig-web's KB is now **central per user**, not per project (pig-web PLAN-0001 D7): the KB exists so agents stop repeating the same research in different projects. Project identity is provenance and a ranking boost. The exception is *private* material (pages from private or intranet hosts, fetches that used credentials, items marked private, and anything derived from them), which must only be returned in sessions of the project it belongs to. pig-web enforces that inside its `kb`/`research` providers and its fetch backend, and answers `notFound` on a mismatch.

**Question.** Does `resource.execute` (and `fetch`) carry the caller's `projectKey` as a routing fact in every request? Your SPEC-0004 §4 lists "captured location revision, project identity … where applicable" as negotiated fields. pig-web asks to make `projectKey` (or the future `prj-` ID) present whenever the session has one, absent otherwise, and asks how the builtin `global` project is represented.

**pig-web's preference:** always included when known; `global` represented as absent, so "no project" cannot match any private item.

**Where to answer:** your SPEC-0004 §4.

# Traps

- pig-web no longer partitions its KB by project. Your R4 answer (per-authority `scopes`) is not needed by pig-web after all: `kb://`, `research://` and `git://` are all `user` scope, and URIs carry no scope authority (`kb://sources/…`, `research://<id>`). Your project-model draft §7 line "pig-web: KB scope key moves from canonical path to project ID" should read "pig-web: provenance and private items key on the project ID".
- pig-web keeps a local alias table from path keys to your `projectKey`/`prj-` IDs; no migration on its behalf is needed.
- pig-web's specs are drafts too; re-read pig-web SPEC-0001 §3 at the commit you answer against.
