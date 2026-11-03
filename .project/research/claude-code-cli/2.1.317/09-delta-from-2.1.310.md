---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-09-delta-from-2.1.310"
  title: "Claude Code 2.1.317 harness delta from 2.1.310"
  description: "Bounded comparison of corpus counts, prompts, tools and beta vocabulary with isolated captures."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:53:02+01:00"
  updated_at: "2026-11-03T11:53:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["record reproducible corpus measurements and bounded version comparison"]
---

# Harness-surface delta from 2.1.310

[OBS] The 2.1.317 inputs are the already-prepared `/tmp/cc_code.txt` and
`/tmp/cc_strings.txt`; no binary was re-carved. Baseline facts below come from
`../2.1.310/report.md` and `../2.1.310/findings/04-undocumented-apis.md`.
See [`evidence/measure-delta.py`](evidence/measure-delta.py) and
[`evidence/commands.txt`](evidence/commands.txt). Citations of the form
`C<line>:<offset>` address the 2.1.317 code corpus, not the binary.

## Corpus counts

| Measurement | 2.1.310 | 2.1.317 |
|---|---:|---:|
| [OBS] Recorded string-run count in prior report | 134,876 | 127,139 corpus lines |
| [OBS] Distinct current strings after stripping decimal offset and tab | [UNK] | 112,302 |
| [OBS] Current code-corpus lines / line-initial run-prefix markers | [UNK] | 61,805 / 7,458 |

[OBS] The baseline report labels 134,876 both a run count and a distinct count;
its raw corpus is not present in this dossier for independent verification.
[UNK] Accordingly the two counts in the first row are reported measurements,
not a proven like-for-like distinct-count delta. [UNK] Added and removed string
sets cannot be computed without the 2.1.310 corpus. [INF] When comparing those
sets, remove each `decimal-offset<TAB>` prefix from the strings corpus before
set comparison; comparing location prefixes would falsely report churn. A code
line can contain multiple source lines per printable run, so code line count is
not its run count (`../2.1.310/tools/CORPUS.md`).

## Prompt and tools

- [OBS] 2.1.317 has three identity strings and a selector depending on provider,
  recorded prefix, noninteractive mode and appended-system-prompt state
  (C19772:691–1148). Exact strings and branch details are in
  [`10-system-prompt.md`](10-system-prompt.md). [UNK] The corresponding
  2.1.310 prompt selector and full prompt texts were not in the baseline dossier,
  so additions, removals and textual deltas are not established.
- [OBS] 2.1.317 has the `DeferredToolPlaceholder` declaration with the same
  name, description, empty object schema and deferred flag recorded by the
  earlier 2.1.311 wire capture (C18027:0, C24987:3970;
  [`evidence/tools/deferred-tool-placeholder.json`](evidence/tools/deferred-tool-placeholder.json);
  `host:/mnt/Projects/furbyhaxx/pi-auth-plus/.project/analyses/claude-code-cli/tracking.md`
  §“Logged-in CLI captures”). [UNK] This comparison is 2.1.311 versus 2.1.317,
  not a direct 2.1.310 placeholder comparison.
- [OBS] 2.1.310 print-mode wire evidence lists bare tool names including `Bash`,
  `Read`, `Edit`, `Write`, `Agent`, `Skill` and `ToolSearch` (tracking file,
  §“Before slice 3b”); those names occur in the captured 2.1.317 print request
  (see [`11-tool-declarations.md`](11-tool-declarations.md)). [UNK] This does not
  establish an unchanged complete tool set, description text, schemas or order.
- [OBS] In the isolated 2.1.317 print and interactive examples, the identity
  differs as documented in [`10-system-prompt.md`](10-system-prompt.md), and
  the interactive example has `AskUserQuestion` in addition to the print list.
  The shared `Agent` and `Bash` declarations differ. [UNK] These are differences
  between two current configurations, not version deltas.

## Beta literals and endpoints

- [OBS] All 50 literal beta values listed in the 2.1.310 dossier occur somewhere
  in the 2.1.317 code corpus. This is literal presence, not proof of continued
  registration or transmission. The exact comparison is in
  [`evidence/measure-delta.py`](evidence/measure-delta.py).
- [OBS] The 2.1.317 explicit beta registry contains
  `program-use-case-ids-2026-09-17` and `claude-code-side-query-2026-09-29`
  (C14519:153318, C14519:153750); neither appears in the 2.1.310 dossier's
  50-value inventory. [INF] They are additions to the **documented inventory**;
  without the old binary or corpus, their absence from 2.1.310 itself is
  unproven. A registry literal is not an always-sent header.
- [OBS] Separate fresh 2.1.317 local-server captures preserve exact ordered beta
  header values in [`print-beta-headers.json`](evidence/wire/print-beta-headers.json)
  and [`interactive-beta-headers.json`](evidence/wire/interactive-beta-headers.json).
  The associated bodies are `print-beta-request.json` and
  `interactive-beta-request.json` in the same directory. These use a dummy API
  key and a loopback base URL in an isolated network namespace; the interactive
  run selected auto mode. [UNK] They do not establish the beta set for an OAuth
  account, a different provider, or a remote feature-flag configuration.
- [UNK] The endpoint set delta was not re-derived. The historical endpoint
  inventory is `../2.1.310/findings/04-undocumented-apis.md` §2; no claim of
  unchanged or changed 2.1.317 endpoints follows from this partial study.
