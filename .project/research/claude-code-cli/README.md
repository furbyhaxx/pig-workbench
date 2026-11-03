---
document:
  type: "research"
  id: "RES-claude-code-cli"
  title: "Claude Code CLI dossier"
  description: "Two passes over the shipped claude binary, kept apart: the imported 2.1.310 provider surface (OAuth, tokens, quota headers, model metadata, undocumented endpoints) and the 2.1.317 harness internals (exact prompts, tool declarations, injections, modes, skills, subagents)"
  status: "published"
  revision: 2
  created_at: "2026-11-03T11:23:00+01:00"
  updated_at: "2026-11-03T12:10:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["imported the pi-auth-plus claude-code-cli dossier unchanged apart from frontmatter re-homing"]
    - revision: 2
      reason: "2.1.317 harness-internals pass"
      changes: ["index now covers both versions and states that the 2.1.310 chapters were not re-verified against 2.1.317"]
---

# Claude Code CLI dossier

Two passes over the shipped binary, which is a Bun standalone compile whose embedded
JavaScript bundle can be carved out of the `.bun` section. No endpoint was called and no
credential is reproduced.

The passes answer different questions and are kept apart on purpose:

| Pass | Version | Owns |
|---|---|---|
| Provider surface (imported) | 2.1.310 | auth, tokens, quota headers and resets, model metadata, undocumented endpoints |
| Harness internals (written here) | 2.1.317 | exact prompts, tool declarations, instruction and injection machinery, modes and approvals, skills and subagents |

**The 2.1.310 chapters were not re-verified against 2.1.317.** Where the harness surface
changed, `2.1.317/09-delta-from-2.1.310.md` records it. For wire contracts — OAuth, usage,
rate-limit headers — 2.1.310 remains the reference until someone re-derives them.

The 2.1.310 sections are a **snapshot**: upstream is
`host:/mnt/Projects/furbyhaxx/pi-auth-plus` at commit `64be593`, so re-copy from there
rather than editing them here. The `2.1.317` chapters are maintained in this repository.
See [`../AGENTS.md`](../AGENTS.md).

## Contents — 2.1.317 (harness internals)

| Chapter | Covers |
|---|---|
| [`2.1.317/09-delta-from-2.1.310.md`](2.1.317/09-delta-from-2.1.310.md) | Bounded static comparison: corpus counts, prompts, tools, beta vocabulary, endpoints |
| [`2.1.317/10-system-prompt.md`](2.1.317/10-system-prompt.md) | Identity strings and their selectors, built-in output styles, main composer versus agent override, billing, message packing, cache placement |
| [`2.1.317/11-tool-declarations.md`](2.1.317/11-tool-declarations.md) | Built-in tool names, description factories, the deferred placeholder, schema serialisation, MCP naming, emission order |
| [`2.1.317/12-instructions-and-injections.md`](2.1.317/12-instructions-and-injections.md) | Trust and insertion model, todo/task reminders, plan/auto/file/hook injections, `CLAUDE.md` and rules discovery, compaction and continuation |
| [`2.1.317/13-modes-and-approvals.md`](2.1.317/13-modes-and-approvals.md) | Mode selection, a worked denied-`Write` decision, the approval flow and classifiers, auto mode, sandbox as a separate boundary |
| [`2.1.317/14-skills-and-subagents.md`](2.1.317/14-skills-and-subagents.md) | Skill and command discovery, frontmatter and expansion, subagent definitions, plugins and marketplaces, ToolSearch / deferred-tool mode |

Evidence in [`2.1.317/evidence/`](2.1.317/evidence): verbatim identity and prompt-section
texts, tool schemas, per-chapter extraction dumps and the verification scripts. The corpus
protocol for this version is [`2.1.317/tools/CORPUS.md`](2.1.317/tools/CORPUS.md) — the
`.bun` section offset changes per release and must be re-measured, not remembered.

### The strongest evidence here is a capture, not an inference

[`2.1.317/evidence/capture-local.py`](2.1.317/evidence/capture-local.py) reconstructs a
**complete assembled request** for the real installed binary without touching the network:
it boots a mock server on `127.0.0.1`, answers `400` immediately, and runs the CLI against a
throwaway `HOME`/XDG tree with `ANTHROPIC_BASE_URL` pointed at the mock, a dummy API key,
`NO_PROXY=*` and telemetry, error reporting and the autoupdater disabled. Nothing egresses,
nothing is billed, and the resulting request is the genuine article.

[`2.1.317/evidence/wire/`](2.1.317/evidence/wire) holds those captures: `print-request.json`
(3 system blocks, 12 ordered tools) and `interactive-request.json` (3 system blocks, 13
ordered tools), plus beta-header variants. Reproduced independently on 2026-11-03: tool
names, tool order, model, `max_tokens`, thinking block and two of three system blocks were
byte-identical between runs; the only varying line in the entire request was the temp path
interpolated into the memory-directory injection.

So for 2.1.317 the system prompt and tool list are **observed**, not reconstructed: the live
`-p` surface is `Agent, Bash, Edit, ListAgents, Read, ReportFindings, ScheduleWakeup, Skill,
ToolSearch, Workflow, DeferredToolPlaceholder, Write` — which is not the 22-name set that
exists in the bundle. Read the static chapters for the selection rules and the conditional
families; read the captures for what is actually emitted.

## Contents — 2.1.310 (provider surface, imported)

| Path | Covers |
|---|---|
| [`2.1.310/report.md`](2.1.310/report.md) | Entry point: method, the seven behaviour changes, endpoint safety table, 2.1.306 → 2.1.310 delta, open questions |
| [`2.1.310/findings/00-artifact-and-transport.md`](2.1.310/findings/00-artifact-and-transport.md) | Binary and extraction, usage-poll transport contract (timeouts, dedupe, 401 replay, per-bearer memo, read-through cache) |
| [`2.1.310/findings/01-oauth-tokens.md`](2.1.310/findings/01-oauth-tokens.md) | Authorize/exchange/refresh, PKCE, scopes, credential storage, token kinds and precedence |
| [`2.1.310/findings/02-quota-headers-resets.md`](2.1.310/findings/02-quota-headers-resets.md) | Usage response schema, all `anthropic-ratelimit-unified-*` headers, claim semantics, overage, reset units |
| [`2.1.310/findings/03-model-metadata.md`](2.1.310/findings/03-model-metadata.md) | Model catalogue, aliases, per-model context/output metadata, fast mode and effort |
| [`2.1.310/findings/04-undocumented-apis.md`](2.1.310/findings/04-undocumented-apis.md) | Every non-`/v1/messages` endpoint, base-URL resolution, telemetry, remote config, beta and identity headers |
| [`2.1.310/findings/05-crosscheck-existing-research.md`](2.1.310/findings/05-crosscheck-existing-research.md) | Claim-by-claim verdicts against an earlier project research document |
| `2.1.310/tools/` | `CORPUS.md` (extraction protocol), `extract_runs.py`, `ctx.py` |

## Notes on this copy

- The upstream `tracking.md` — a live wire-alignment log for pi-auth-plus's provider slice —
  was deliberately not imported. It is that repository's runtime state and drifts per release.
- This dossier was written for **pi-auth-plus's quota work**. Almost all of it is provider
  surface: auth, quota, models, telemetry. Its Workbench-relevant material is thin and
  indirect — the transport habits in `00`, the frame/artifact and beta-header inventories in
  `04` §6/§9, and the identity headers in `04` §10.
- The corpus itself is not in the repository: `extract_runs.py` needs the installed binary to
  rebuild `/tmp/cc_code.txt` and `/tmp/cc_strings.txt`. Every `C<line>:<offset>` citation
  refers to that regenerated corpus, not to an ELF offset.

## Limits

- **The 2.1.310 binary is no longer installed** on this host (the versions directory holds
  2.1.311, 2.1.312, 2.1.316 and 2.1.317). An exact 2.1.310 → 2.1.317 string-set delta is
  therefore not reproducible, so `09-delta-from-2.1.310.md` is a bounded comparison against
  what the 2.1.310 dossier documented, not a corpus diff. It does establish that all 50
  documented baseline beta literals still occur in 2.1.317, and names two registry literals
  that were not in the 2.1.310 inventory: `claude-code-side-query-2026-09-29` and
  `program-use-case-ids-2026-09-17`.
- The captures cover the default local configuration with a dummy key. Other accounts,
  providers and gateways, conditional schemas, style injection and the full endpoint delta
  are `[UNK]`. The mock answers `400`, so no upstream acceptance or model response was
  observed.
- 53 `[UNK]` markers across the 2.1.317 chapters, each naming what would settle the point.

## Known defects inherited from the 2.1.310 snapshot

- Two links into the corpus protocol are wrong relative to their file: `findings/00-artifact-and-transport.md`
  cites `tools/CORPUS.md`, which resolves to `findings/tools/`. Use `../tools/CORPUS.md`.
  `findings/03-model-metadata.md` has the correct form.
- `../provider-quota-credits-reporting.md` is cited by `report.md` and `findings/05` but exists in
  neither repository. That document lived in pi-quotas' `.project/research/`, and its claims are
  only summarised in `findings/05`.

## Reproducing

Set `V` to the installed version and re-measure the section offset — it changes per release.

```bash
V=2.1.317
B=$HOME/.local/share/claude/versions/$V
readelf -SW "$B" | grep -E '\.bun\b'        # note the .bun file offset

dd if="$B" of=/tmp/claude.bun bs=1M skip=$((0x8cfd000/1048576)) count=180
cd .project/research/claude-code-cli/$V/tools
python3 extract_runs.py          # /tmp/claude.bun -> /tmp/cc_code.txt, /tmp/cc_strings.txt
python3 ctx.py '<literal>' --files code --window 700 --max 4
```

For 2.1.317 the offset is `0x8cfd000` (141 MiB), size `aef787a`, and the resulting corpus is
53.3 MB of code plus 58.7 MB of strings (decimal, as measured). Full protocol:
[`2.1.317/tools/CORPUS.md`](2.1.317/tools/CORPUS.md). The 2.1.310 binary is no longer
installed, so rerunning the 2.1.310 recipe needs that version re-downloaded.

### Capturing a live request instead of carving

This needs no corpus and gives an assembled request rather than fragments:

```bash
cd <repo root>
python3 .project/research/claude-code-cli/2.1.317/evidence/capture-local.py print
python3 .project/research/claude-code-cli/2.1.317/evidence/capture-local.py interactive
```

It is hermetic (loopback mock, throwaway `HOME`, dummy key, no egress) and refuses to
overwrite an existing capture, so delete the file under `evidence/wire/` to re-run.

`evidence/verify.py` reads `/tmp/cc_code.txt`, so build the corpus first; it validates
citation offsets, links and prompt/tool extraction against that corpus. `split-capture.py`
and `measure-delta.py` need the captured requests and the corpus respectively.
