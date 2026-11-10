---
document:
  type: "analysis"
  id: "ANA-2026-11-04-pig-upstream-candidates-sources"
  title: "Sources and session excerpts for the PiG upstream candidate list"
  description: "Session files, extracted assistant statements and verification commands backing ANA-2026-11-04-pig-upstream-candidates"
  status: "published"
  revision: 1
  created_at: "2026-11-04T11:55:00+01:00"
  updated_at: "2026-11-04T11:55:00+01:00"
  created_by: "coding-assistant"
  updated_by: "coding-assistant"
  changelog:
    - revision: 1
      reason: "Evidence for the consolidated report"
      changes: ["Session inventory, excerpts, SDK rechecks"]
---

# 1. Session inventory

Bucket `$HOME/.pig/agent/sessions/--mnt-Projects-furbyhaxx-pig-workbench--/` (the `~/.pi` bucket is the same directory, both resolve to `/mnt/shared/data/sessions/…`).

| File (prefix) | Content |
|---|---|
| `2026-11-02T13-43-03-337Z_01a19c7f…` | Proposal, specs, M0.2 probes P1–P30, S1–S6, PR1–PR8, fork decision D13 |
| `2026-11-03T10-20-20-237Z_01a1a0ec…` | Research import (no host findings) |
| `2026-11-03T13-07-57-477Z_01a1a186…` | Plan review (no host findings) |
| `2026-11-03T13-23-30-324Z_01a1a194…` | Slice 1 implementation with parallel subagents |
| `.workbench/agents/sessions/agt_*/transcript.jsonl` | 81 subagent transcripts |
| `2026-11-03T20-59…` to `2026-11-03T21-49-53…` (19 files) | Stray Pi-Bolt sessions, §3 |

# 2. Excerpts (assistant statements, timestamps UTC)

- 02.11. 15:55 — "possible host bug … happened in about 3 of 8 runs, seemingly when a reload follows tree navigation and an agent run." (U-01)
- 02.11. 18:58 — "PiG compiles its extensions itself and has no documented build-tag option." (U-25)
- 02.11. 19:07 — "PiG has no session retention." (U-14)
- 03.11. 09:42 — "the host only routes [view events] to overlays, and widget events get dropped … a click on a widget isn't supported in the SDK." (U-11)
- 03.11. 10:29 — "The spec's claim that bpr covers extension inference through host models doesn't hold up." (U-04)
- 03.11. 12:14 — "the 'reverse-engineer-anything' skill is being picked up from the real ~/.agents/skills directory, since PIG_HOME doesn't isolate it." (U-24)
- 03.11. 12:35 — "PiG's built-in MCP extension runs after ours, so Workbench never sees `mcp_servers`, and `force` drops that section entirely." (U-05)
- 03.11. 15:30 — "SDK dialogs return ok=false for both timeout and cancellation → cannot distinguish." (U-13)
- 03.11. 15:46 — "'inputs changed while being built' — because concurrent agents are writing files in the tree while PiG builds the extension (packed cell)." (U-25)
- Subagent `…xnpqqc` — "No SDK masked input; `SecretInput` has no verified adapter — needs host/fork decision." (U-12)

Not a PiG finding: the 03.11. 15:22 "Stream ended without finish_reason" failures came from a test mock that put `finish_reason` inside `delta`.

# 3. Stray Pi-Bolt sessions

- 19 sessions created in bursts (20:59, 21:02, 21:17, 21:38, 21:49 UTC) matching subagent spawns in `.workbench/agents/log.jsonl`.
- Each: model `openai/gpt-6.1-terra`, thinking `xhigh`, preamble "operating inside Pi-Bolt, a build of pi … compiled ahead of time", single user message `/home/furbyhaxx/.pi/agent/git/github.com/furbyhaxx/pi-accordion/conductors/ws/thermocline/runner.mjs`; the agent then calls `ask_user`, which fails headless.
- Spawn site: `host:$HOME/.pi/agent/git/github.com/furbyhaxx/pi-accordion/extension/accordion.ts:761` — `spawn(process.execPath, [runnerPath], …)`.
- Under PiG on 04.11.2026, `ps` shows `/usr/bin/node …/thermocline/runner.mjs` as the child of PiG's node cell, so the runner starts correctly there.

# 4. Subagent environment notes

- `…1gmxhh`, `…nzk8r8`: "`host:` alias does not resolve (`ssh://host` → exit 255)"; donor trees unreachable.
- `…1z28e4`, `…a2660p`: "The required `extending-pig` skill was unavailable."

# 5. SDK rechecks on 04.11.2026

`pig reload --sdk-path` → `$HOME/.pig/state/pigsdk/sdk-899c7964784a0050` (same as all probes).

- `grep -iE 'password|masked'` over `$SDK/*.go`: no dialog API (U-12).
- `$SDK/view.go:311`: "viewEvent routes a ui.view.event to its overlay's input queue" (U-11).
- `$SDK/context.go:478,543,563`: `Select`, `Confirm`, `Input` return `any` (U-13).
- No `sessionLifetime`/retain symbol in `$SDK/*.go` (U-14).
