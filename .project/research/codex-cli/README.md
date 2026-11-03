---
document:
  type: "research"
  id: "RES-codex-cli"
  title: "Codex CLI source dossier"
  description: "Source analysis of openai/codex (codex-rs only): architecture, exact base prompts, model-facing tool declarations, the instruction/injection catalogue, modes and approvals, skills and commands"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:10:00+01:00"
  updated_at: "2026-11-03T12:10:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["dossier index for the 0.167.0-alpha.4 source pass"]
---

# OpenAI Codex CLI — source dossier

Prior art for reimplementing a coding-agent harness, taken from **source**, not from a
shipped binary. Scope is `codex-rs` only; the TypeScript/npm surface, the VS Code
extension and the cloud app are out of scope.

## Target

| Field | Value |
|---|---|
| Repository | `https://github.com/openai/codex`, `codex-rs/` subtree |
| Tag | `rust-v0.167.0-alpha.4` |
| Commit | `fb036d4ebd89f263d34a1f888c279ce85bb383c6` |
| Commit date | `2026-11-02T17:19:36-08:00` (upstream author date) |
| Crates | 133 |
| Analysed | 2026-11-03 |

The version is in the source: `[workspace.package] version = "0.167.0-alpha.4"`
(`codex-rs/Cargo.toml:173`), matching the tag. Upstream zeroes that field on `main`
(`0.0.0` as of 2026-11-03) and the release tag carries the real value, which is exactly
why this dossier pins the tag: on a moving branch the version is not recoverable from the
tree. The `codex` binary is **not installed** on this host and nothing was executed: every
claim is read from the pinned checkout.

## Reproduce

```bash
git clone --depth 1 --branch rust-v0.167.0-alpha.4 \
  --filter=blob:none --sparse https://github.com/openai/codex.git codex
cd codex && git sparse-checkout set codex-rs
```

## Contents

| Chapter | Covers |
|---|---|
| [`0.167.0-alpha.4/00-context.md`](0.167.0-alpha.4/00-context.md) | Pinned identity, source-only method, scope, checkout reproduction |
| [`0.167.0-alpha.4/01-architecture.md`](0.167.0-alpha.4/01-architecture.md) | Thread/session ownership, turn sampling, tool dispatch, prompt construction, Responses request shape |
| [`0.167.0-alpha.4/02-system-prompt.md`](0.167.0-alpha.4/02-system-prompt.md) | The exact bundled base prompt, model selection, message roles and order, conditional text families |
| [`0.167.0-alpha.4/03-tool-declarations.md`](0.167.0-alpha.4/03-tool-declarations.md) | Registration order, built-in and extension tool families, schema sources, gates, Responses serialisation |
| [`0.167.0-alpha.4/04-instructions-and-injections.md`](0.167.0-alpha.4/04-instructions-and-injections.md) | The injection catalogue: environment/world state, instructions, time and post-tool notices, inter-agent, guardian, budget/compaction/hooks |
| [`0.167.0-alpha.4/05-modes-and-approvals.md`](0.167.0-alpha.4/05-modes-and-approvals.md) | Verbatim sandbox and approval templates, escalation semantics, command segmentation, guardian review |
| [`0.167.0-alpha.4/06-skills-and-commands.md`](0.167.0-alpha.4/06-skills-and-commands.md) | Skill discovery, parsing, selection, how skills reach context, plugins, slash commands, MCP |
| [`0.167.0-alpha.4/07-notable-mechanisms.md`](0.167.0-alpha.4/07-notable-mechanisms.md) | Compaction and history, budgets and state, agents/realtime, code mode and incremental tools, review mode |

Evidence is beside the chapters in [`0.167.0-alpha.4/evidence/`](0.167.0-alpha.4/evidence):
the base prompt and every bundled template copied out verbatim with source paths, the
injection inventory, tool-spec dumps, and the commands used.

## How to read it

Chapter 02 and its evidence file `evidence/prompt/base_instructions-default.md` are the
authoritative record of the base prompt: the evidence copy is byte-identical to
`protocol/src/prompts/base_instructions/default.md` apart from the frontmatter this
repository requires on Markdown artifacts. Chapter 04 is the densest and most
transferable: `core/src/context/` is one file per injection, and the chapter groups them
by trigger.

## Limits

- **Nothing was executed.** There is no captured, serialised request for any feature
  configuration; where a prompt is assembled from several fragments the chapter shows
  the fragments and the builder, not one rendered string.
- The exact bytes contributed by user configuration, extensions and MCP servers are
  `[UNK]`: they are inputs, not code.
- Precedence after a provider interprets the request is `[UNK]`; this is a client-side
  reading only.
- 46 `[UNK]` markers across the eight chapters, each naming what would settle the point.
