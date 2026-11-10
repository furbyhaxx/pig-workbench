---
document:
  type: "research"
  id: "RES-antigravity-cli"
  title: "Antigravity CLI (agy) dossier"
  description: "Two passes over Google's agy binary: the imported 1.4.8 baseline (architecture, auth, models, API, quota, mechanics) and the 1.4.12 harness internals (exact prompts, tool declarations, injections, modes, skills)"
  status: "published"
  revision: 2
  created_at: "2026-11-03T11:23:00+01:00"
  updated_at: "2026-11-03T12:15:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "1.4.12 harness-internals pass"
      changes: ["index now covers both versions", "state that the 1.4.8 chapters were not re-verified against 1.4.12"]
---

# Antigravity CLI (`agy`) dossier

Reverse engineering of Google's Antigravity CLI (`agy`), recovered from the shipped binary
through its surviving `pclntab`, plus read-only live captures.

Two passes, kept apart on purpose:

| Pass | Version | Owns |
|---|---|---|
| Harness internals (written here) | 1.4.12 | exact system prompt variants, tool declarations, instruction and injection machinery, modes and approvals, skills and commands |
| Baseline (imported snapshot) | 1.4.8 | architecture, auth, model catalogue, network API, quota and resets, install/config, mechanics, upstream integration notes |

**The 1.4.8 chapters were not re-verified against 1.4.12.** Where the binary and harness
surface changed, `1.4.12/09-delta-from-1.4.8.md` records it. For network, auth and quota
contracts, 1.4.8 remains the reference. `1.4.8/06-mechanics.md` is still the baseline for the
turn loop and tool inventory, and the 1.4.12 chapters cite it rather than restating it.

The 1.4.8 sections are a **snapshot**: upstream is
`host:/mnt/Projects/furbyhaxx/pi-auth-plus` at commit `64be593`, so re-copy from there rather
than editing them here. The `1.4.12` chapters are maintained in this repository. See
[`../AGENTS.md`](../AGENTS.md).

## Contents — 1.4.12 (harness internals)

| Chapter | Covers |
|---|---|
| [`1.4.12/09-delta-from-1.4.8.md`](1.4.12/09-delta-from-1.4.8.md) | Binary and inventory delta, harness RPC changes, and the prompt/tool knowledge gained in this pass |
| [`1.4.12/10-system-prompt.md`](1.4.12/10-system-prompt.md) | Recovered prompt fragments and templates: four identity variants, the cumulative defaults (Antigravity Agent, Cascade), the built-in coding worker, assembly symbols and formatter types |
| [`1.4.12/11-tool-declarations.md`](1.4.12/11-tool-declarations.md) | Statically assigned converter names, conditional and oneharness declarations, parameter-schema evidence, relation to the 1.4.8 inventory |
| [`1.4.12/12-instructions-and-injections.md`](1.4.12/12-instructions-and-injections.md) | Workspace and rule context, skills/time/mentions/media, tool feedback and tasks, hooks, summaries and child briefs |
| [`1.4.12/13-modes-and-approvals.md`](1.4.12/13-modes-and-approvals.md) | The two mode surfaces, project grants and prompt records, one worked approval path, deterministic vetting and PolicyGuardian, sandbox controls |
| [`1.4.12/14-skills-and-commands.md`](1.4.12/14-skills-and-commands.md) | Skill discovery and metadata, how a skill enters context, slash enumeration and expansion, plugins and subagent definitions |

Evidence is in [`1.4.12/evidence/`](1.4.12/evidence): extracted prompt templates and identity
variants, tool-declaration scans, mode/skill/slash rodata captures, the RPC delta and the
extraction scripts. Symbol tooling lives in [`1.4.12/tools/`](1.4.12/tools) (`goresym.py`,
`pbdump.py`, `xwalk.py`); its 27 MB `funcs.json` symbol cache is regenerable and git-ignored.

## Contents — 1.4.8 (imported baseline)

| Path | Covers |
|---|---|
| [`1.4.8/README.md`](1.4.8/README.md) | Dossier index, the ten headline findings, reproduction commands, method limits |
| [`1.4.8/00-context.md`](1.4.8/00-context.md) | Target identity, symbol-recovery toolkit, established facts |
| [`1.4.8/01-overview.md`](1.4.8/01-overview.md) | Artifact, architecture, component map, process model |
| [`1.4.8/02-auth.md`](1.4.8/02-auth.md) | OAuth, client/scope mapping, six auth methods, token storage |
| [`1.4.8/03-models.md`](1.4.8/03-models.md) | Model catalogue, id grammar, thinking levels, context windows |
| [`1.4.8/04-api.md`](1.4.8/04-api.md) | Endpoints, transport, inference envelope, routing, telemetry, flags |
| [`1.4.8/05-quota.md`](1.4.8/05-quota.md) | Quota model, buckets, resets, credits, tiers, polling policy |
| [`1.4.8/06-mechanics.md`](1.4.8/06-mechanics.md) | Turn loop, tool surface, permissions, sandbox, subagents, persistence |
| [`1.4.8/07-install-and-config.md`](1.4.8/07-install-and-config.md) | Installer, self-update, config/state layout, environment surface |
| [`1.4.8/08-integration-notes.md`](1.4.8/08-integration-notes.md) | Upstream's integration contract, deviations, risks |
| [`tracking.md`](tracking.md) | Pinned version, imitated surface, live-verification log |
| `1.4.8/data/` | Recovered inventories: RPC services and methods per service, per-model functions |
| `1.4.8/evidence/` | Binary identity, install script, log sites, one live capture (2026-10-27) |
| `1.4.8/tools/` | `goresym.py` (pclntab recovery), `pbdump.py` (embedded protobuf descriptors), `xwalk.py` (rodata cross-reference) |

## Notes on this copy

- `1.4.8/data/{funcs.txt,funcmap.json,pkgs.txt}` and `1.4.8/tools/funcs.json` are excluded: ~70 MB of
  inventories regenerable in seconds. The recipes in `1.4.8/README.md` rebuild them, and the
  donor's `.gitignore` files are kept beside them.
- `1.4.8/08-integration-notes.md` is written for **pi-auth-plus**, not for Workbench. It prescribes
  provider-family work; treat its "contract" section as that repository's decision, not ours.
- Chapters 02, 03, 05 (auth, models, quota) are provider surface. Workbench's own scope — tools,
  locations, frames, questionnaires, shell jobs — is concentrated in 06, with 01 and 07 for context.
- Path repairs only: reproduction commands pointing at `.project/analyses/antigravity-cli/` were
  updated to this location, and two of them were missing the `1.4.8/` segment. No finding was
  altered.
- Redaction: the account owner's address was replaced with `<redacted-email>` in `02-auth.md` and
  `evidence/klog-sites.txt`. The finding is that the ID token carries an email claim; the value is
  not load-bearing. Local binary paths were kept because the reproduction commands need them.

## Reproducing

```bash
T=.project/research/antigravity-cli/1.4.8/tools/goresym.py
A=~/.local/bin/agy

python3 $T --bin $A init                        # build the function cache (tools/funcs.json)
python3 $T --bin $A funcs 'cli/backend/auth'    # regex over 171,928 function names
python3 $T --bin $A strs  '<exact func name>'   # string literals referenced by it
python3 $T --bin $A disas '<exact func name>'
python3 $T --bin $A grep  '<regex>'             # rodata scan
```
