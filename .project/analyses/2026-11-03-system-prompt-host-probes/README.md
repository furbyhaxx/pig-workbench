---
document:
  type: "analysis"
  id: "ANA-2026-11-03-system-prompt-host-probes"
  title: "Probes PR1–PR6: can PiG host Workbench's system prompt takeover?"
  description: "Runtime evidence for forced prompts across a run and compaction, handler order between extensions, section patches with and without mid-conversation system messages, frames before the user prompt in RPC/print/TUI, host inputs for templates, and model facts"
  status: "published"
  revision: 1
  created_at: "2026-11-03T13:16:00+01:00"
  updated_at: "2026-11-03T13:16:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User approved SPEC-0014 direction and asked to proceed with the host probes"
      changes: ["Ran PR1–PR6 against pig 0.5.3+1.4.0 with a mock provider", "Catalog check for supportsMidConvoSystemMessages on PiG main@9b4efdd6"]
---

# 1. Method

- `pig` 0.5.3+1.4.0 from `PATH`, staged SDK `sdk-899c7964784a0050`, isolated `PIG_HOME`, sessions persisted. No billed inference: scripted OpenAI-compatible mock (`evidence/harness/mock.py`) logging every request body.
- Two mock models: `mock-1` (no compat flags) and `mock-mid` (`reasoning: true`, `compat.supportsMidConvoSystemMessages: true`).
- Probe extension [`evidence/extension/probe-prompt`](evidence/extension/probe-prompt/extension.go), loaded as copy A and copy B for order tests. In `before_agent_start` it logs the options, the rendered prompt, model info, thinking level and `ModelRegistry.Find`, and per env does `force` (return `systemPrompt`), `forceopt` (set `options.forceSystemPrompt`), `sections` (`probe` and `docs` custom sections, changing per prompt or stable) and `steer` (`SendMessage(steer)` frame). Tool `probe_add_tool_<role>` activates `probe_extra_<role>` mid-run.
- Main scenario: prompt with two tool turns (the first adds a tool), two more prompts with 40k-word replies, manual compaction, prompt after compaction, `get_messages`.
- `evidence/runs/<run>.txt`: condensed output of `show.py` (per request: system/developer messages with section tags, message sequence, tools; probe log; session entries). `evidence/fixtures/`: options per PR5 fixture and excerpts of the host-rendered prompts (full prompts omitted).
- PR3 catalog: a throwaway test against PiG `main@9b4efdd6` listing built-in models with `supportsMidConvoSystemMessages` (`runs/midconvo-catalog.txt`).

# 2. Results

| # | Question | Verdict | Evidence |
|---|---|---|---|
| PR1 | Does a forced prompt reach every request of the run, and what is recorded? | **Every agent request of the run, re-forced per prompt.** The session records PiG's base sections, plus a `tools` patch when a tool is activated mid-run. The forced text is **not** re-rendered mid-run: the request's tool declarations gained `probe_extra_a`, the forced `<tools>` text did not until the next prompt. Compaction requests use PiG's own summarizer prompt (forced and base prompt both absent); the compaction entry's `systemMessage` holds PiG's base sections | `runs/force-steer.txt` (req 0–2 same forced text, req 5–7 summarizer, compaction entry) |
| PR2 | Handler order; what do later handlers see? | **Extension load order** (`-e` order). A later handler sees the earlier forced prompt in `options.forceSystemPrompt` and as `systemPrompt`. The last force wins. Sections set by a handler after a force are **recorded in the session but never sent** (the transcript claims `DOCS-B-2`; the request had the forced text). Sections set before the force are part of the text the forcing handler reads | `runs/ab-force-sections.txt`, `ba-force-sections.txt`, `ab-force-force.txt`, `ab-forceopt-sections.txt` |
| PR3 | Who gets section patches mid-conversation? | **Without the flag**, the request carries one system message, re-rendered with the latest sections every prompt (cache prefix breaks at each change). **With the flag**, the first system message stays; each change is appended as a `developer` message `Updated system prompt section "docs": …` before the user message. **Unchanged sections produce no patch.** Built-in catalog: Anthropic Claude 4.8+/5.x, OpenAI Responses and Codex GPT-5.4+, Kimi K2.6+/K3, DeepSeek V4 Pro (direct and via Copilot, OpenCode, OpenRouter, Fireworks, Azure); no other provider | `runs/sec-mock-1.txt`, `sec-mock-mid.txt`, `sec-stable.txt`, `midconvo-catalog.txt` |
| PR4 | Does a `steer` frame from `before_agent_start` land before the user prompt? | **Yes, in RPC, print (`-p`) and TUI**, on the first prompt of a session, on later prompts and after compaction (after the compaction summary, before the user message). Persisted as `custom_message`; sent as a `user` message. The TUI renders it as a `[wb-frame]` block above the prompt | `runs/force-steer.txt` req 8, `print.txt`, `tui.txt`, `tui-screen.txt` |
| PR5 | Do templates get every host input? | **Mostly.** Options carry selected and hidden tools, snippets, guidelines, prompt guidelines, custom prompt (SYSTEM.md), append text, context files with content, skills with `disableModelInvocation` and cwd. **Not in options:** PiG's docs paths and constant texts. The `mcp_servers` section is a custom section that PiG's built-in MCP extension sets in its own `before_agent_start` handler (`coding/mcpext/extension.go`), so Workbench sees it in `options.sections` only if that handler ran first (not probed: no MCP fixture). `--append-system-prompt` flags (joined by a blank line) replace `APPEND_SYSTEM.md` instead of adding to it. Hidden tools (codemode `only`) leave `<tools>` with `codemode` alone; without `read`, the skills hint says "Use bash to load a skill's file" and rules gain "Use bash for file operations". `disable-model-invocation` skills are in options but not in the prompt. Windows cwd not testable on this host | `fixtures/*-options.json`, `fixtures/prompt-excerpts.txt` |
| PR6 | Model facts for family matching? | **Complete.** `ctx.Model()`, `ModelProvider()`, `ModelQualified()`, `GetModelInfo()` (context window, max output, reasoning, costs) and `GetThinkingLevel()` work in `before_agent_start`. `ModelInfo` has no `api` or compat; `ModelRegistry().Find(provider, id)` returns both (`api`, `compat.supportsMidConvoSystemMessages`) | `runs/pr6.txt` |

Also observed:
- Skills from the real `~/.agents/skills/` leak into an isolated `PIG_HOME`; `PIG_HOME/skills/` is not a skill root (`agent/skills/` is).
- On the first prompt, the steer frame is persisted before the session's first system message; requests still put the system message first.

# 3. Consequences for SPEC-0014

1. `force` delivery works as specified, with three gaps to design for:
   - **Mid-run tool changes** reach the forced prompt only at the next user prompt. Tool declarations are current, so this is a stale description, not a missing tool. Fix candidates: re-render in `context_with_system` per turn (S3 showed it can replace the system message; needs a probe, PR7) or accept it.
   - **Later handlers**: Workbench cannot choose its load order. A later extension's force replaces Workbench's prompt; a later extension's sections are recorded but not sent. Detect it in `before_provider_request` (runs after every `before_agent_start` handler and sees the final payload) by comparing the system text with Workbench's render, not at the next `before_agent_start`.
   - **Transcript truth**: the session shows PiG's base prompt; `session://<id>/system-prompt` must serve Workbench's own record.
2. `sections` delivery on the fork (F-P1) is worth it for flagged models only; for others PiG re-renders the single system message anyway. Stable sections cost nothing in both cases, which confirms the epoch design (§4.3).
3. Workbench can decide per model whether diff frames or section patches are cheaper: `ModelRegistry().Find` exposes `compat.supportsMidConvoSystemMessages`.
4. Context frames via `beforePrompt` (steer) work in every mode and after compaction; no host change needed.
5. `<docs>` content must come from Workbench's `docs://` registry (options carry no docs paths), which SPEC-0014 §7 already plans. `<mcp_servers>` comes from PiG's built-in MCP extension as a custom section and reaches templates through `host.sections` (PR2 order applies; to probe with an MCP fixture, PR8). `builtin/workbench` should place it explicitly rather than only appending unknown sections.
6. `host.append` is whatever PiG resolved (flags replace the file); templates must not assume both.

# 4. Fork candidates (PLAN-0001 D13)

- F-P1 (already in SPEC-0014 §11): complete ordered section list from `before_agent_start`, recorded in the session.
- F-P2: re-render or re-invoke the forced prompt after a mid-run tool change, or expose the change to the forcing extension.
- F-P3: record a forced prompt in the session (as sections or as a marked system message), so the transcript matches what was sent.
