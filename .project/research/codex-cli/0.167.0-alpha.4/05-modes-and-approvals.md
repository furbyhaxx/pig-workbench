---
document:
  type: "research"
  id: "RES-codex-cli-0.167.0-alpha.4-05-modes-and-approvals"
  title: "Codex CLI modes and approvals"
  description: "Bundled permission templates, policy composition, command decisions and guardian review"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:54:06+01:00"
  updated_at: "2026-11-03T11:54:06+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["documented source-observed permission policy and templates"]
---

# Modes and approvals

Source root: `host:/tmp/re-recon/codex-tag/codex-rs`. [OBS] means read from source; [INF] is a source-based derivation; [UNK] means not established. No Codex binary was run. These templates are model-facing guidance, not a substitute for the enforcement code. The blocks reproduce the complete bundled files, with no elisions; they are also captured in [template evidence](evidence/05-permission-templates.md). Catalog-provided text can replace bundled text (`prompts/src/permissions_instructions.rs:313-341,385-403`).

## Verbatim sandbox and approval templates

### `prompts/templates/permissions/sandbox_mode/read_only.md`

~~~text
Filesystem sandboxing defines which files can be read or written. `sandbox_mode` is `read-only`: The sandbox only permits reading files. Network access is {{ network_access }}.
~~~

### `prompts/templates/permissions/sandbox_mode/workspace_write.md`

~~~text
Filesystem sandboxing defines which files can be read or written. `sandbox_mode` is `workspace-write`: The sandbox permits reading files, and editing files in `cwd` and `writable_roots`. Editing files in other directories requires approval. Network access is {{ network_access }}.
~~~

### `prompts/templates/permissions/sandbox_mode/danger_full_access.md`

~~~text
Filesystem sandboxing defines which files can be read or written. `sandbox_mode` is `danger-full-access`: No filesystem sandboxing - all commands are permitted. Network access is {{ network_access }}.
~~~

### `prompts/templates/permissions/approval_policy/never.md`

~~~text
Approval policy is currently never. Do not provide the `sandbox_permissions` for any reason, commands will be rejected.
~~~

### `prompts/templates/permissions/approval_policy/on_request.md`

~~~text
# Escalation Requests

Commands are run outside the sandbox if they are approved by the user, or match an existing rule that allows it to run unrestricted. The command string is split into independent command segments at shell control operators, including but not limited to:

- Pipes: |
- Logical operators: &&, ||
- Command separators: ;
- Subshell boundaries: (...), $(...)

Each resulting segment is evaluated independently for sandbox restrictions and approval requirements.

Example:

git pull | tee output.txt

This is treated as two command segments:

["git", "pull"]

["tee", "output.txt"]

Commands that use more advanced shell features like redirection (>, >>, <), substitutions ($(...), ...), environment variables (FOO=bar), or wildcard patterns (*, ?) will not be evaluated against rules, to limit the scope of what an approved rule allows.

## How to request escalation

IMPORTANT: To request approval to execute a command that will require escalated privileges:

- Provide the `sandbox_permissions` parameter with the value `"require_escalated"`
- Include a short question asking the user if they want to allow the action in `justification` parameter. e.g. "Do you want to download and install dependencies for this project?"
- Optionally suggest a `prefix_rule` - this will be shown to the user with an option to persist the rule approval for future sessions.

If you run a command that is important to solving the user's query, but it fails because of sandboxing or with a likely sandbox-related network error (for example DNS/host resolution, registry/index access, or dependency download failure), rerun the command with "require_escalated". ALWAYS proceed to use the `justification` parameter - do not message the user before requesting approval for the command.

## When to request escalation

While commands are running inside the sandbox, here are some scenarios that will require escalation outside the sandbox:

- You need to run a command that writes to a directory that requires it (e.g. running tests that write to /var)
- You need to run a GUI app (e.g., open/xdg-open/osascript) to open browsers or files.
- If you run a command that is important to solving the user's query, but it fails because of sandboxing or with a likely sandbox-related network error (for example DNS/host resolution, registry/index access, or dependency download failure), rerun the command with `require_escalated`. ALWAYS proceed to use the `sandbox_permissions` and `justification` parameters. do not message the user before requesting approval for the command.
- You are about to take a potentially destructive action such as an `rm` or `git reset` that the user did not explicitly ask for.
- Be judicious with escalating, but if completing the user's request requires it, you should do so - don't try and circumvent approvals by using other tools.

## prefix_rule guidance

When choosing a `prefix_rule`, request one that will allow you to fulfill similar requests from the user in the future without re-requesting escalation. It should be categorical and reasonably scoped to similar capabilities. You should rarely pass the entire command into `prefix_rule`.

### Banned prefix_rules 
Avoid requesting overly broad prefixes that the user would be ill-advised to approve. For example, do not request ["python3"], ["python", "-"], or other similar prefixes that would allow arbitrary scripting.
NEVER provide a prefix_rule argument for destructive commands like rm.
NEVER provide a prefix_rule if your command uses a heredoc or herestring. 

### Examples
Good examples of prefixes:
- ["npm", "run", "dev"]
- ["gh", "pr", "check"]
- ["cargo", "test"]
~~~

### `prompts/templates/permissions/approval_policy/on_request_rule_request_permission.md`

~~~text
# Permission Requests

Commands may require user approval before execution. Prefer requesting sandboxed additional permissions instead of asking to run fully outside the sandbox.

## Preferred request mode

When you need extra sandboxed permissions for one command, use:

- `sandbox_permissions: "with_additional_permissions"`
- `additional_permissions` with one or more of:
  - `network.enabled`: set to `true` to enable network access
  - `file_system.read`: list of paths that need read access
  - `file_system.write`: list of paths that need write access

When using the `request_permissions` tool directly, only request `network` and `file_system` permissions.

This keeps execution inside the current sandbox policy, while adding only the requested permissions for that command, unless an exec-policy allow rule applies and authorizes running the command outside the sandbox.

If the command already matches an exec-policy allow rule, the command can be auto-approved without an extra prompt. In that case, exec-policy allow behavior (including any sandbox bypass) takes precedence.

## Escalation Requests

Use full escalation only when sandboxed additional permissions cannot satisfy the task.

- `sandbox_permissions: "require_escalated"`
- Include `justification` as a short question asking for approval.
- Optionally include `prefix_rule` to suggest a reusable allow rule.

## Command segmentation reminder

The command string is split into independent command segments at shell control operators, including pipes (`|`), logical operators (`&&`, `||`), command separators (`;`), and subshell boundaries (`(...)`, `$()`).

Each segment is evaluated independently for sandbox restrictions and approval requirements.
~~~

### `prompts/templates/permissions/approval_policy/unless_trusted.md`

~~~text
 `approval_policy` is `unless-trusted`: The harness will require user approval before running commands unless an explicit exec policy rule allows them.
~~~

## Resolution, configuration and limits

[OBS] `SandboxMode` has `read-only` (the enum default), `workspace-write`, and `danger-full-access` (`protocol/src/config_types.rs:104-115`). Prompt rendering derives the displayed mode from the effective filesystem policy: full-disk write gives danger-full-access, no writable roots gives read-only, and otherwise workspace-write; independently, the network placeholder is `enabled` or `restricted` (`prompts/src/permissions_instructions.rs:175-280`; `protocol/src/protocol.rs:1227-1243`). These are rendering decisions, not proof that the platform sandbox can enforce a policy.

[OBS] `AskForApproval` contains `UnlessTrusted` (serialized as `untrusted`), `OnRequest` (the enum default; `on-failure` accepted as alias), `Granular`, and `Never` (`protocol/src/protocol.rs:1163-1204`). The prompt itself calls the first `unless-trusted`, but that is not its serialized enum spelling. There is no separate on-request-with-rule enum: when `features.exec_permission_approvals` is enabled, OnRequest substitutes `on_request_rule_request_permission.md` for the ordinary bundled on-request section (`prompts/src/permissions_instructions.rs:41-42,344-369`). Granular's booleans separately govern `sandbox_approval`, `rules`, `skill_approval`, `request_permissions` and `mcp_elicitations`; disabled categories are rejected rather than surfaced (`protocol/src/protocol.rs:1184-1224`; `prompts/src/permissions_instructions.rs:415-479`).

[OBS] `PermissionsInstructions` is a developer fragment wrapped by the permission markers. It composes sandbox prose, approval prose, writable roots, denied-read paths/globs and omission warning, in that order. Paths share a 32 KiB byte budget and are omitted at whole-entry boundaries (`prompts/src/permissions_instructions.rs:52,115-174,281-303`). A catalog override may replace bundled prose; `request_permissions` availability and auto-review add separately selected text (`prompts/src/permissions_instructions.rs:313-383`; `prompts/src/permissions_instructions/approval_instructions.rs:12-54`). Prefix-only additions can be emitted as a smaller notification (`core/src/context/world_state/permissions.rs:96-127`).

[OBS] Interactive `-a` / `--ask-for-approval` exposes only `on-request` and `never` through its CLI enum (`tui/src/cli.rs:69-71`; `utils/cli/src/approval_mode_cli_arg.rs:7-24`); `-s` / `--sandbox` selects sandbox mode. `--approve-for-me` (alias `--not-so-yolo`) sets `approvals_reviewer=auto_review`, `approval_policy=on-request`, and `sandbox_mode=workspace-write`. `--dangerously-bypass-approvals-and-sandbox` (alias `--yolo`) maps to `never` plus `danger-full-access` in the shown CLI path. `--add-dir` adds writable roots (`utils/cli/src/shared_options.rs:37-85`; `cli/src/main.rs:2084-2104`). Config also carries `approval_policy`, `approvals_reviewer`, `sandbox_mode`, permission profiles, and legacy `[sandbox_workspace_write]` settings; the loader distinguishes explicit profile syntax from legacy workspace settings (`core/src/config/mod.rs:2743-2745,3743-3869`). The shared option definition examined contains no `--full-auto`; [UNK] any alternative entry point outside that definition.

## Command decision pipeline

1. [OBS] Exec policy parses command segments and checks multiple commands with explicit rules and an unmatched-command fallback; explicit forbid produces `Forbidden`, prompt produces either approval or rejection depending on policy, `PromptUser` requires fresh human approval, and allow produces `Skip`. On allow, sandbox bypass is true only if every segment has an explicit policy allow match (`core/src/exec_policy.rs:352-477`). `Never` rejects prompts; `Granular` differentiates rules from sandbox requests (`core/src/exec_policy.rs:213-241`).
2. [OBS] For an unmatched dangerous command, or managed Windows restrictions without an enabled Windows sandbox backend, the fallback forbids under `Never` and prompts otherwise. For ordinary unmatched restricted-sandbox commands under `OnRequest`, a sandbox override request prompts; without one the fallback allows. `UnlessTrusted` prompts on unmatched commands, whereas `Never` allows ordinary commands relying on the sandbox (`core/src/exec_policy.rs:807-882`).
3. [OBS] The tool orchestrator handles `Skip`, `Forbidden`, `NeedsApproval`, and `NeedsHumanApproval`; strict auto-review can review even `Skip`. Otherwise it runs the first attempt under the selected sandbox. An approved escalation may use an escalated sandbox with restrictions where unsandboxed execution is not permitted; a policy allow alone does not authorize removing those restrictions (`core/src/tools/orchestrator.rs:137-244`; `core/src/tools/sandboxing.rs:156-181,249-287`). A different tool may use `default_exec_approval_requirement`, which has its own policy-based behavior (`core/src/tools/sandboxing.rs:190-235`); do not substitute that fallback for the command-specific exec-policy path.
4. [OBS] A sandbox denial does not automatically retry without a sandbox under ordinary `Never` or `OnRequest`. An eligible managed-network denial is a separate OnRequest exception; other retry decisions, preapprovals and human-once behavior are checked before a retry (`core/src/tools/orchestrator.rs:400-530`; `core/src/tools/sandboxing.rs:347-356`).
5. [OBS] `with_additional_permissions` requires the feature unless preapproved, requires OnRequest unless preapproved, and must include normalized nonempty network or filesystem permissions. Supplying `additional_permissions` without that mode is invalid (`core/src/tools/handlers/mod.rs:224-282`).

### Concrete pairing: workspace-write + on-request

[INF] Consider a normal shell command with no matching explicit rule, no dangerous-command classification, no sandbox override, strict auto-review disabled, and an effective restricted workspace-write filesystem policy. The unmatched-command fallback returns `Allow`; because no segment has an explicit allow rule, the requirement is `Skip` with `bypass_sandbox=false`. The orchestrator therefore runs its first attempt with its selected sandbox instead of prompting. If that attempt is denied, ordinary OnRequest does not automatically rerun unsandboxed; a model may issue an explicit escalation request, which enters the prompt path, while managed-network approval has its separately gated exception. This is a source-derived scenario, not an observed command execution (`core/src/exec_policy.rs:366-477,847-874`; `core/src/tools/orchestrator.rs:160-245,438-477`).

## Network and Guardian

[OBS] The network approval flow is eligible only when policy is not `Never` and the effective permission profile is managed; decisions distinguish one-shot allow, session allow and deny, and session keys include environment, lowercase host, protocol and port (`core/src/tools/network_approval.rs:139-161,198-211,805-822`). [UNK] Full precedence of every proxy allowlist, denylist and per-environment restriction was not traced.

[OBS] `approvals_reviewer` defaults to `user`; `auto_review` (legacy alias `guardian_subagent`) selects a review subagent (`protocol/src/config_types.rs:175-198`). Normal routing selects Guardian for `OnRequest` or `Granular` with the auto reviewer; managed requirements can require Guardian independently. Contributors may return a reviewed decision, a cached allow, or ask for a user; a fresh review is required for selected conditions including retries and escalated exec (`ext/guardian-reviewer/src/routing.rs:58-65,80-150`; `core/src/guardian/decision.rs:99-161`). Review receives a bounded transcript and exact planned-action JSON with policy in the review session's developer context (`core/src/guardian/prompt.rs:40-51,104-141`).

[OBS] Guardian asks for JSON with required `outcome` (`allow` or `deny`) and optional risk, user authorization and rationale. The parser also accepts JSON embedded in surrounding prose; omitted risk defaults low on allow/high on deny and authorization defaults unknown (`ext/guardian-reviewer/src/assessment.rs:11-88`). Completed allow/deny maps to approval/denial; timeout and cancellation are separate, and most review errors fail closed; an input-budget error can return to the user flow when Guardian is not required (`ext/guardian-reviewer/src/completion.rs:38-169`). [UNK] Full model prompt, policy-template selection and all async scoring/cache invalidation conditions require further tracing.
