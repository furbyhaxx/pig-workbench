---
document:
  type: "research"
  id: "RES-deepseek-harness"
  title: "DeepSeek Harness source dossier"
  description: "Source analysis of deepseek-ai/deepseek-harness: architecture, system prompt assembly and scoping, model-facing tools, the instruction/injection catalogue, plan mode and guards, skills and commands, session and compaction"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:10:00+01:00"
  updated_at: "2026-11-03T12:10:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["dossier index for the 0.2.2-alpha.3 source pass"]
---

# DeepSeek Harness — source dossier

Prior art for reimplementing a coding-agent harness, taken from **source**. The
repository is a TypeScript pnpm monorepo: a package per concern, with `packages/core/`
holding the agent loop, system-prompt assembler and tool executors.

## Target

| Field | Value |
|---|---|
| Repository | `https://github.com/deepseek-ai/deepseek-harness` |
| Package | `@deepseek-ai/dsh-root` |
| Version | `0.2.2-alpha.3` |
| Commit | `8bd073392eef6f54e921a093b95076248eb08cbb` |
| Commit date | `2026-11-03T15:27:29+08:00` (upstream author date) |
| Analysed | 2026-11-03 |

The harness is **not installed** on this host and nothing was executed: every claim is
read from the pinned checkout.

## Reproduce

```bash
git clone --depth 1 https://github.com/deepseek-ai/deepseek-harness.git dsh
cd dsh && git rev-parse HEAD   # expect 8bd073392eef6f54e921a093b95076248eb08cbb
```

## Contents

| Chapter | Covers |
|---|---|
| [`0.2.2-alpha.3/00-context.md`](0.2.2-alpha.3/00-context.md) | Identity, pinned source, method, scope, reproduction |
| [`0.2.2-alpha.3/01-architecture.md`](0.2.2-alpha.3/01-architecture.md) | Package map, agent/session/turn, request shape and client boundary |
| [`0.2.2-alpha.3/02-system-prompt.md`](0.2.2-alpha.3/02-system-prompt.md) | Prompt assembly and section order, source fragments, the scoped-prompt mechanism, persona and locale, instruction hierarchy |
| [`0.2.2-alpha.3/03-tool-declarations.md`](0.2.2-alpha.3/03-tool-declarations.md) | Tool definitions, presentation and dispatch, native vs PTC executor projection, ordering, conditional surfaces |
| [`0.2.2-alpha.3/04-instructions-and-injections.md`](0.2.2-alpha.3/04-instructions-and-injections.md) | Context producers: time and environment, workspace instruction discovery and precedence, references/skills/model changes, todo/goal/delegation/jobs, guards, compaction, hooks |
| [`0.2.2-alpha.3/05-modes-and-approvals.md`](0.2.2-alpha.3/05-modes-and-approvals.md) | Plan mode, sandbox and permission presets, approval and escalation, other guards, persona |
| [`0.2.2-alpha.3/06-skills-and-commands.md`](0.2.2-alpha.3/06-skills-and-commands.md) | Skill discovery and selection, how skills reach the model, commands, extensions, MCP and ACP |
| [`0.2.2-alpha.3/07-session-and-compaction.md`](0.2.2-alpha.3/07-session-and-compaction.md) | Session log and model surface, persistence, compaction, spill, todo/goal/plan state, subagents and teams, jobs and schedule |

Evidence is beside the chapters in [`0.2.2-alpha.3/evidence/`](0.2.2-alpha.3/evidence):
verbatim prompt and template fragments with `path:line` attribution, an injection
inventory as TSV, SDK scenario captures of system prompt plus tool schemas, and the
commands used.

## How to read it

`02-system-prompt.md` is the entry point: the harness builds its prompt from named
sections with an explicit order (`packages/core/system-prompt/src/index.ts`), and the
section list is the real map of what this harness can put in front of a model. Chapter 03
documents the two executors (native and PTC) — a reimplementation needs to understand
which one a tool declaration is projected through, because the projection shapes the
schema the model sees.

## Limits

- **Nothing was executed.** Runtime prompts, enabled plugins and provider wire messages
  depend on deployment, scope, model and configuration; the dossier records the code that
  builds them, not one rendered instance.
- A fully rendered, unmodified shipped-profile prompt is `[UNK]`; the evidence carries
  verbatim fragments with their source lines, plus SDK scenario captures.
- Not every conditional plugin contribution or activation combination is transcribed.
- Transport and per-adapter wire encoding are outside this pass.
- 16 `[UNK]` markers across the chapters, each naming what would settle the point.
