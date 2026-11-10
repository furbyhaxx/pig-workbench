---
document:
  type: "draft"
  id: "DRAFT-0003"
  title: "Activity line above the editor"
  description: "Proposal for a one-row live summary above the editor (state, agents, jobs, instructions, skills, contributions) that opens the matching Workbench inspector, adapted from the pi-workbench showcase"
  status: "archived"
  revision: 3
  created_at: "2026-11-03T10:45:00+01:00"
  updated_at: "2026-11-09T09:00:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 3
      reason: "Fix a dangling reference found by the artifact check"
      changes: ["Marked the donor sketch path as a host path"]
---

> **Archived.** Approved with the recommendations of §8; the user targets fullscreen mode only, so clicking is the primary path once the host gap of §7 is closed. Successor: [SPEC-0009](../specs/0009-interaction-commands.md) §9, settings in [SPEC-0010](../specs/0010-configuration-state-trust.md) §3, WB-23 in [PLAN-0001](../plans/0001-pig-workbench.md).

# 1. Problem

Nothing on screen says what the session has running or loaded: background jobs, peer agents, unread messages, active instruction files, enabled skills, a pending question. The footer holds model, context and short status entries; the rest is only reachable through commands.

# 2. Source: pi-workbench showcase

`host:/mnt/Projects/furbyhaxx/pi-workbench/bin/agent-tui-showcase.ts` (commit `bc7ce0e`), `renderBottomSection` lines 1835–1866, `bounceDots` lines 1294–1306:

```text
 ●●● Ready · 3 agents · 1 background job · 3 instructions · 4 skills
╭──────────────────────────────────────────────────────────────────╮
│ ❯ Ask the agent anything you want                                │
```

- Three dots: idle = first dot green, others dim; working = an accent dot bounces across the three.
- `Ready`/`Working` bold; items separated by ` · `; each item clickable and opens its modal (`agents`, `jobs`, `instructions`, `skills`); the open one is highlighted.
- Aligned to the editor's inner span; shares the bottom stack with an optional task/agent work widget (`host:/mnt/Projects/furbyhaxx/pi-workbench/.project/sketches/nugents-tui/specification.md` §7.1), which Workbench does not adopt (task tracking and subagents are out of scope).

The showcase owns the whole screen. Workbench is an extension inside PiG's TUI, so the adaptation keeps the row and its behaviour and drops the screen ownership.

# 3. PiG seams (verified in the SDK, `pig reload --sdk-path`)

| Need | Mechanism | Status |
|---|---|---|
| Row above the editor | `ctx.SetWidget(key, kit.View, {"placement":"aboveEditor"})` (`context.go` `SetWidget`, `view.go` `setWidgetView`); the host lays a kit view out at its width every frame | available |
| Animated dots without traffic | `kit.Loader` with `Indicator.Frames` and `IntervalMs`; "the host animates it" (`kit/kit.go`) | available |
| Fitting segments | `kit.HStack` of `kit.TruncatedText` entries with shrink options | available |
| Clickable items | `kit.MouseRegion` per item (`kit/mouse_region.go`) | **gap**: the SDK routes `ui.view.event` only to open `ctx.Custom` overlays (`view.go` `viewEvent`); events from a widget view are dropped. Mouse also reaches components only in fullscreen mode (`mouse.go`) |
| Keyboard entry | `ext.Shortcut(key, description, handler)` (`extension.go`) | available |
| Host working indicator | `ctx.SetWorkingVisible`, `SetWorkingMessage`, `SetWorkingIndicator` | available |
| Counts | own registries (jobs SPEC-0008, peers/messages SPEC-0011, instructions SPEC-0007, asks SPEC-0009); skills from `GetSystemPromptOptions`; MCP from `GetMcpServers` | available |

# 4. Proposed design

## 4.1 Content

```text
 ●●● Working 1m12s · 2 agents · 1 message · 2 jobs, 1 failed · 5 instructions, 1 changed · 12 skills · ⎇ feat-parser
```

Left to right, in priority order (lowest priority drops first when narrow):

| # | Item | Shown when | Source | Opens |
|---|---|---|---|---|
| 1 | State: `Ready`, `Working <elapsed>`, `Waiting for you`, `Compacting`, `Retrying 2/3` | always | agent and ask events | hub |
| 2 | `N agents` (live peers in this project), `N messages` unread | peers > 0 / unread > 0 | SPEC-0011 | `/workbench sessions` |
| 3 | `N jobs`, `, N failed` | running or unacknowledged failed > 0 | SPEC-0008 | `/workbench jobs` |
| 4 | `N instructions`, `, N changed` | active > 0 | SPEC-0007 | `/workbench instructions` |
| 5 | `N skills` | enabled > 0 | host | skill catalog (`ls skill://`) |
| 6 | Location: `⎇ <branch>` in a worktree, `ssh <host>` remote | not the session's starting checkout | SPEC-0008 | `/cd` panel |
| 7 | Contributed items (§5), e.g. `1 research` | producer sets it | other extensions | producer's target |
| 8 | `coordinated by <name>` | coordinator active on this session | SPEC-0011 §9 | coordinator info |

- Zero-count items are hidden, so the line grows with activity and stays short when idle. The user can pin items so they always show (the showcase look).
- Severity pairs color with a word: `1 failed` in error color, `Waiting for you` in warning color, `1 changed` in accent. Color is never the only signal.
- Dots: idle = `●` success + two dim; working = host-animated bounce (frames `●∙∙`, `∙●∙`, `∙∙●`, `∙●∙`, colored by token); waiting = static warning `●●●`.
- Elapsed time is the only value that changes without an event; the view is re-sent once per second while working, otherwise only on change, coalesced to ≤ 4 sends per second.

## 4.2 Widths

```text
140  ●●● Working 1m12s · 2 agents · 1 message · 2 jobs, 1 failed · 5 instructions, 1 changed · 12 skills · ⎇ feat-parser
 80  ●●● Working 1m12s · 2 agents · 2 jobs, 1 failed · 5 instructions · 12 skills
 57  ●●● Working · 2 agents · 2 jobs, 1 failed · 5 instr
 36  ●●● Working · ⚇2 ⚙2✗1 ◆5
```

Below 50 columns items fall back to glyph + number (`⚇` agents, `⚙` jobs, `◆` instructions, `✦` skills, `✉` messages), with `✗N` for failures. Captures at 36/57/80/140 in fullscreen and regular mode are required before approval (house standard).

## 4.3 Opening the inspector

The "menu" is the existing Workbench hub (SKETCH-0001 §3), not a new surface: the line is a one-row summary of the hub, and the hub's rows already are jobs, instructions, frames, locations.

- **Keyboard (always works):** shortcut `workbench.activity` opens the hub with the first non-zero item selected; `Enter` opens it, `Esc` returns to the editor. `/workbench` opens the hub as today.
- **Mouse (fullscreen only):** clicking an item opens its target directly; clicking the state opens the hub. Needs the SDK gap in §3 closed (§7).
- **Headless:** no widget. `/workbench show` and `read cfg://workbench/status` return the same items as text.

## 4.4 Relation to existing status output

One fact, one place:
- With the line on, Workbench stops publishing its own `SetStatus` segments for jobs, peers and pending ask (SPEC-0009 §9); they live in the line. Location stays a footer segment unless the line shows it.
- The host working indicator ("Working…" row) duplicates item 1. See Q1.

# 5. Contributions from other extensions

Extend the existing `statusSegment` registration (SPEC-0009 §9) instead of adding a parallel registry:

```json
{"kind":"statusSegment","key":"research","descriptor":{"label":"Research","placement":"activity","priority":40,"open":{"command":"/web research"}}}
```

- `placement`: `status` (today's behaviour) or `activity`. `open`: a command or a resource URI; Workbench runs it only on an explicit user click or Enter, never on update.
- `status.update` gains optional `count` and `severity`; Workbench renders `N <label>`, hides zero, and applies the same width fallbacks (producer supplies a one-cell `glyph`).
- The user orders, pins and hides contributed items in `/workbench statusline`, next to the builtin ones.

# 6. Settings (SPEC-0010)

| Key | Default | Scope |
|---|---|---|
| `activity.enabled` | true | user/runtime |
| `activity.placement` | `aboveEditor` | user/runtime (`belowEditor` possible, same API) |
| `activity.items` | builtin order above | user/runtime; per item `show: auto|always|never` |
| `activity.replaceWorkingIndicator` | see Q1 | user/runtime |

# 7. Host dependency

Clickable widgets need the SDK (and possibly the host) to deliver kit view events from widget, header and footer surfaces to the extension, e.g. `ext.OnWidgetEvent(key, func(kit.Event))`. Today `viewEvent` looks up only overlays. Two steps:
1. Probe: a widget view with a `MouseRegion` in fullscreen mode; record whether the host emits `ui.view.event` for it at all.
2. Request upstream (PiG SDK) accordingly. Until it lands, the keyboard path is the feature, and the line renders without dead click affordances (no hover styling).

# 8. Decisions needed

| # | Question | Recommendation |
|---|---|---|
| Q1 | Hide PiG's own working indicator while the line is on (`SetWorkingVisible(false)`), so state shows once? Risk: other extensions' `SetWorkingMessage` text and host retry/compaction notices would then need mirroring, which needs verified event names | Keep the host indicator by default; offer `replaceWorkingIndicator` once a probe shows retry/compaction events are observable |
| Q2 | On by default for every Workbench user, or opt-in? The house standard says widgets are for live state and clear when idle; this row is persistent by design | On by default: one row, and it is the feature you missed. Easy off switch in settings |
| Q3 | Default shortcut for `workbench.activity` | None hard-coded until a free key is checked against `keybindings.md` for all four platform columns; candidate `alt+w` |
| Q4 | Zero-count items hidden (grows with activity) or always shown like the showcase | Hidden, with per-item `always` |
| Q5 | Include item 6 (location) or leave location in the footer | Include only when away from the starting checkout |

After approval: SPEC-0009 §9 (activity line, contribution fields), SPEC-0010 settings, SKETCH-0001 §3/§14 wireframes and captures, PLAN-0001 WB-17 or a new WB item in M7, and the probe of §7 in M0.2 style.
