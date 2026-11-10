---
document:
  type: "analysis"
  id: "ANA-2026-11-02-donor-parity-audit"
  title: "M0.1 donor parity audit"
  description: "Behavior inventory of oh-my-pi, pi-workbench and the nukular frame ledger from their test suites, mapped onto SPEC-0002 to SPEC-0010 with gaps, divergences and decisions"
  status: "draft"
  revision: 2
  created_at: "2026-11-02T20:20:00+01:00"
  updated_at: "2026-11-03T14:10:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 2
      reason: "User decisions on D5.b and D5.d"
      changes: ["D5.b adopted, D5.d rejected"]
---

# 1. Method and limits

Each donor test name is a concrete behavior claim. I extracted every test title from the in-scope donor suites (`evidence/extract.sh`), grouped them by area, and checked each group against the matching specification text. A behavior counts as covered only if the spec states it or something stricter.

| Donor | Revision | Local state | License | Titles |
|---|---|---|---|---|
| oh-my-pi `crates/pi-edit` (hashline engine) | `553f0d1` (2026-10-26) | clean | MIT | 305 (`evidence/omp-edit-tests.tsv`) |
| oh-my-pi `packages/coding-agent` tests (read, search, fetch, edit, bash, ask; task/subagent/MCP/marketplace excluded) | `553f0d1` | clean | MIT | 1,202 (`evidence/omp-inscope.tsv`) |
| pi-workbench `tests/` | `bc7ce0e` (2026-11-02) | clean | MIT; carries pi-ask, pi-tools, pi-tldr notices in `THIRD_PARTY_NOTICES.md` | 1,746 (`evidence/piwb-tests.tsv`; `agents/` and `tasks/` are out of scope) |
| nukular-ai `nukular-agent-sdk/src/frame` | `8a40f60` | one untracked review dir, unrelated | MIT (Cargo.toml) | 16 (`evidence/nuk-tests.tsv`) |

**Limits.** This is a test-level audit, not a line-by-line source read. Behavior that donors implement without a test is not inventoried. Two divergence claims were confirmed by reading the test bodies (D5.a, D5.b). Ported code and fixtures still need the notices listed above; nothing has been copied yet.

Raw per-area notes with every finding: `evidence/findings-raw.txt`.

# 2. Result

The specs cover the donors' architecture: state machines, ownership, safety and the extension APIs. What they miss is the **repair and leniency layer**, meaning the dozens of small rules that make the donors forgiving of model mistakes without becoming unsafe. They also miss some concrete defaults (budgets, column caps, line counts) and a handful of host-race rules. None of the gaps contradicts a spec rule. Section 4 lists the places where a donor and a spec actually disagree.

| Spec | Gaps | Main missing areas |
|---|---|---|
| SPEC-0002 frames | 1 | Injective escaping, timestamp validation, sequence exhaustion |
| SPEC-0003 presentation | 6 | Donor density budgets, header fact sources, shared animation clock, preview pairing |
| SPEC-0005 hashline/edit | 14 | Apply-time payload repairs, recovery budget and refusals, parser leniency list, streaming preview/abort, generated-file guard, notebook editing, per-model mode |
| SPEC-0006 read/search | 15 | Column cap, HTTP content dispatch and feeds, grep paging/exclusions/regex dialect, path-list rules, SQLite details, write header |
| SPEC-0007 ambient context | 8 | Remote watcher lifecycle, watch modes, idle delivery, BOM, self-edit races |
| SPEC-0008 locations/jobs/bash | 19 | Secret-guard specifics, branch rollback, `.worktrees` ignore check, SSH config write rules, remote shell selection, command routing guard, result shape |
| SPEC-0009 ask/tldr | 12 | Image answers, chat redirect, input hygiene, pending scan, abort races, extraction rules, tldr evidence filter |
| SPEC-0010 settings | 1 | Project cannot define trust, concurrent saves, symlinked config |

Every gap is adopted as donor behavior unless section 4 says otherwise. The specs now carry them in a "Donor parity" section with the gap IDs.

# 3. Row classification

M0.1 asks for each row to be marked as donor behavior, an intentional improvement, or unresolved:
- **Donor behavior:** all G-rows in `evidence/findings-raw.txt`, added to the specs.
- **Intentional improvement:** already in the specs and stricter than the donors. Examples: exact-text edits never fall back to fuzzy matching; timeouts never pick an answer; jobs are terminated on reload because PiG has no session retention; bash commands are never rewritten.
- **Unresolved:** the decisions in section 4.

# 4. Divergences and decisions

| ID | Donor behavior | Spec today | Recommendation |
|---|---|---|---|
| D5.a | Rejects overlapping hunks; merges only a bare-row recovery placeholder with an explicit op | Same | No divergence (verified in `hashline_parse.rs:493`) |
| D5.b | On a tag collision within one file, recovers from the **newest** snapshot (`hashline_apply.rs:1153`) | Rejects | **Decided 2026-11-03 (user):** adopt the donor rule within one canonical resource, followed by full relocation validation |
| D5.c | `replace` mode runs a fuzzy ladder (whitespace and indentation normalisation, similarity threshold) | Exact only | Keep exact. A fuzzy match edits text the model never saw |
| D5.d | Extra edit modes: `apply_patch` (Codex envelope), unified `patch`, prose-style `sloppy` | Hashline and replace only | **Decided 2026-11-03 (user): no.** Hashline and exact-text modes only; hashline keeps recovering `apply_patch` envelope noise |
| D5.e | Model families that do badly with hashline fall back to `replace` by default | One global mode | Adopt as a configurable per-model rule list seeded with the donor families (G5.9) |
| D6.a | SQLite rows can be written through `write` (JSON5 insert/update/delete, schema enforced) | Read-only | **Decision needed** |
| D6.b | `ast_grep` / `ast_edit` structural search and rewrite tools | Not in scope | **Decision needed.** The tree-sitter backend is already approved |
| D6.c | `jfind`: LLM-judged semantic find | Not in scope | Keep out: paid inference on every search |
| D6.d | Third-party readers (Jina, Firecrawl, Parallel, Kagi, X/Grok) | Forbidden by the no-upload rule | Keep out |
| D6.e | Speculative reads while arguments stream | — | Keep out (host feature) |
| D8.a | Jobs survive extension reload through the host's `sessionLifetime.retain` | Reload terminates jobs | Keep the spec. The PiG SDK (`sdk-899c7964784a0050`) has no retention capability; filed as a host gap |
| D8.b | Persistent shell session (env, aliases, functions and cd persist; rc files loaded) | One-shot | Keep one-shot |
| D8.c | Rewrites `git worktree` commands; resolves URLs inside shell text | Never rewrites shell text | Keep the spec |
| D9.a | A timeout auto-selects the recommended or first option | Timeout leaves the question unanswered | Keep the spec |
| D9.b | Cancelling ask aborts the agent turn | `cancelled: true` as a normal result | **Decision needed** |
| D9.c | Answer history archived by default | Off by default | **Decision needed** |
| D9.d | Owns the whole footer: layout engine, builtin segments, registry for other extensions | Contributed segments only | **Decision needed.** Taking over the footer conflicts with other extensions |

# 5. Host capability checks this audit triggered

- **Streaming edit preview and early abort (G5.10).** The SDK exposes `message_update` events and `Context.Abort()`. A probe must show that partial tool-call arguments are readable and that aborting the stream mid-call is clean. Until then G5.10 is conditional.
- **Session retention (D8.a).** Absent in the SDK. Reload behavior stays as specified.
