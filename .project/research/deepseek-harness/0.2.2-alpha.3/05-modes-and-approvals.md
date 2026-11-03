---
document:
  type: "research"
  id: "RES-deepseek-harness-0.2.2-alpha.3-05-modes-and-approvals"
  title: "DeepSeek Harness 0.2.2-alpha.3 modes and approvals"
  description: "Source-derived plan state, permission presets, guards, sandbox escalation and review"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:02+01:00"
  updated_at: "2026-11-03T11:54:02+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document source-observed modes and decisions"]
---

# Modes, guards and approvals

[OBS] Source snapshot: `/tmp/re-recon/dsh`, commit `8bd073392eef6f54e921a093b95076248eb08cbb`; source paths below are relative to that checkout. The package was not installed or run. All defaults below are **plugin schema or source fallbacks**, not claims about the deployed profile.

## Plan mode

[OBS] Plan mode is a session projection of `plan/mode` events; initial state is inactive. `/plan` selects on, `/plan off` selects off. Between turns the event is appended immediately; inside an open turn the selection is pending until an accepted pre-step. That step appends the event; rejected steps leave it pending. Guidance is the required deployment-supplied `PlanModeConfig.section`, registered as `plan:policy` at `PLAN_POLICY` order. It is rendered for the selected active state. Plan state is separate from sandbox and approval enforcement (`packages/plan/plan-mode/src/index.ts:1`, `:64`, `:135`, `:197`, `:211`, `:237`, `:431`; `packages/core/system-prompt/src/index.ts:136`).

[OBS] `exit_plan_mode` is disclosed from the first selected active step and remains admitted after entry; execution still checks committed plan state. The tool requires a nonempty Markdown plan beginning with `# ` and a user-questions service. Its review offers `Approve` and `Keep planning`. Exactly one selected `Approve` and no custom answer queues inactive state for the next accepted pre-step; a dismissed review concludes the turn; keep-planning or feedback produces an error and leaves planning active (`packages/plan/plan-mode/src/index.ts:14`, `:281`, `:304`, `:310`, `:321`, `:339`, `:350`, `:361`).

[OBS] A model-facing notice is generated when a user switch differs from the last request-header mode: `The user switched this session to plan mode.` or `The user switched this session back to the default mode.` Its source is `plan-mode`, form `notice` (`packages/plan/plan-mode/src/index.ts:469`).

## Sandbox and permission presets

[OBS] `sandbox-policy` configuration keys are `mode` (schema default `read-only`), `workspaceRoot` (fallback `process.cwd()`), and `additionalWritableRoots` (default `[]`). Modes are `read-only`, `workspace-write`, and `danger-full-access`. A call-specific approved mode outranks the session's latest `sandbox/mode`, which outranks the configured default; session cwd supplies the workspace root where present. The policy is also rendered in the runtime-context snapshot (`packages/sandbox/sandbox-policy/src/index.ts:60`, `:97`, `:136`, `:169`, `:190`).

[OBS] Permission preset rows pair `sandbox` and `approval`. The schema's configured default table has `workspace-write` = (`workspace-write`, `ask`) and `danger-full-access` = (`danger-full-access`, `never`). Config keys are `presets` and `defaultPreset`; when the latter is absent, the service derives a matching default from the composed knob values. `/permission <preset>` records `permission/preset` and updates changed knobs through their own setters. `custom` is a derived, nonselectable unmatched value (`packages/interaction/permission-presets/src/index.ts:59`, `:70`, `:152`, `:176`, `:247`, `:348`, `:403`).

[OBS] The `auto` preset is available only when an integration registers it; its fixed bundle is (`danger-full-access`, `ask`). The experimental auto-review plugin supplies such an integration: it reviews native tool calls and started PTC inner calls before their bodies, excluding outer `run_code`. Its reviewer prompt classifies effects as low, medium, or high: low must allow, medium requires explicit authorization for action/target/scope, high is always denied. Under `ask`, a reviewer denial can be sent to the user; under `never` it is final (`packages/interaction/permission-presets/src/index.ts:76`, `:306`; `packages/experimental/auto-review/src/index.ts:1`, `:54`, `:783`). The full reviewer policy is not reproduced here; its literal source is the authority.

## Approval and escalation

[OBS] Approval configuration key `policy` accepts `ask` (schema default) and `never`. `never` returns `rejected` before answerer dispatch; `ask` dispatches the `approval/request` waterfall, whose empty or throwing chain resolves `unavailable`. An already aborted request resolves `cancelled`; only `allowed-once` grants. Requests require an open turn and append paired `approval/asked` and `approval/decided` audit records (`packages/interaction/user-approval/src/index.ts:56`, `:170`, `:242`, `:293`). The model-facing policy and switch-notice templates are in `evidence/injection-templates.txt` (`packages/interaction/user-approval/src/index.ts:74`, `:198`, `:215`).

[OBS] Escalation arguments `sandbox_permissions` and `justification` must occur together, and the reason must be nonblank. Target vocabulary is `workspace-write` or `danger-full-access`. Equal effective/requested mode returns unchanged; otherwise `read-only` may widen to either target, and `workspace-write` only to `danger-full-access`. Missing approver or agent, a non-wider target, rejection, cancellation, or an unavailable answerer throws before execution. An `allowed-once` grant applies to that call only (`packages/sandbox/sandbox/src/escalation.ts:29`, `:42`, `:52`, `:172`).

**Concrete decision.** [OBS] Given active plan mode, effective sandbox `workspace-write`, approval policy `never`, and a tool call requesting `danger-full-access` with a nonblank justification: the target is strictly wider, so the escalation asks the approval service; `never` resolves `rejected` without asking a human, and `approveEscalation` throws `the user rejected escalating this ${subject} to "danger-full-access"; it stays denied, so stop and explain instead of working around it`. The tool body does not receive the wider mode. Plan mode contributes guidance but does not independently change this decision (`packages/plan/plan-mode/src/index.ts:1`; `packages/sandbox/sandbox/src/escalation.ts:172`; `packages/interaction/user-approval/src/index.ts:293`). This is a source trace, not an executed test.

## Other guards and persona

[OBS] `repeat-tool-reminder` is advisory: it observes post-execution calls including denials, compares tool name and recursively key-sorted full JSON arguments, and emits on configurable `thresholds` (schema default `[3, 5, 8]`). First threshold uses gentle text; later thresholds use detailed text. `argumentsPreviewChars` defaults to 500 for presentation only; detection uses full arguments. `include` and `exclude` wildcard patterns choose tracked calls, while untracked calls are transparent; an explicit user message resets the chain (`packages/guard/repeat-tool-reminder/src/index.ts:27`, `:53`, `:70`, `:77`, `:125`, `:196`, `:220`, `:236`).

[OBS] `timeout-policy` wraps a tool only when its definition declares `timeoutMs`. It supplies a cooperative abort signal and replaces the result with `Error: tool call timed out after ${timeoutMs}ms` and code `TOOL_TIMEOUT` only if its own deadline fired (`packages/guard/timeout-policy/src/index.ts:24`, `:40`, `:56`).

[OBS] A scoped persona accepts `prefix`, `suffix`, `complete`, and `includeRuntimeContext`. Defaults are empty suffix, false complete, true runtime context. `complete` makes the prefix the complete prompt; false `includeRuntimeContext` suppresses dynamic snapshots. An agent preset is an `id` plus optional display fields and child plugin rows; its persona shadows deployment persona within that scope (`packages/preset/persona/src/index.ts:25`, `:49`, `:62`; `packages/preset/agent-preset-registry/src/definition.ts:4`).

## Gaps

[UNK] The complete profile-specific setting paths, shipped preset selections, and every auto-review failure and override path were not traced. Resolve them by following the selected profile composition and `packages/experimental/auto-review/src/index.ts` end to end. The above examples assume the specified effective policies rather than claiming those are deployment defaults.
