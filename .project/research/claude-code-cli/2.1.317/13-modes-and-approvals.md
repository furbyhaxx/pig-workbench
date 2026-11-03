---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.317-13-modes-and-approvals"
  title: "Claude Code 2.1.317 modes and approvals"
  description: "Permission modes, decision branches, automatic classification and sandbox boundaries."
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:52:57+01:00"
  updated_at: "2026-11-03T11:52:57+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["document permission modes and decision paths"]
---
# Modes and approvals

Offline static analysis only. `C<line>:<offset>` addresses `host:/tmp/cc_code.txt` (local path `/tmp/cc_code.txt`) using 1-based lines and 0-based character offsets; `[OBS]` is code or literal, `[INF]` is an inference, `[UNK]` is unresolved. Supporting search output and exact search invocations are in `evidence/13-*.txt`. Identically named minified functions in unrelated modules are not interchangeable.

## Mode selection

[OBS] The SDK schema enumerates exactly `default`, `acceptEdits`, `bypassPermissions`, `plan`, `dontAsk`, `auto`; it describes respectively normal prompting, automatic file-edit approval, bypass, planning, denial rather than prompting, and model-classified approvals (C14521:44811). Treat “plan — no actual tool execution” in that schema as a coarse description: its prompt and checks allow read-only work and a designated plan destination. `manual` is accepted as a settings alias for `default` (C13880:13937).

[OBS] At the CLI dispatch parser, `--dangerously-skip-permissions` sets `bypassPermissions`; `--allow-dangerously-skip-permissions` sets `allowBypass` but does not by itself select the mode. `--permission-mode` overrides `--inherit-permission-mode`; if only the latter is supplied, `permissionModeInherited:true` is attached. `--model`, `--effort`, and `--agent` are also carried in `dispatchDefaults` (C13191:1844). Startup applies root/policy gates and, on the shown agent-view path, workspace trust and bypass consent before dispatch (C13197:9818). This is one startup path, not a universal precedence specification.

[OBS] `permissions.defaultMode`, `permissions.allow`, `permissions.ask`, `permissions.deny`, `permissions.disableBypassPermissionsMode:"disable"`, and the working-directory read/write blocks are settings schema fields (C13880:13937). The mode resolver explicitly rejects repository-controlled project/local sources as the authority granting bypass. In an IDE-owned session, settings bypass additionally needs explicit `allowDangerouslySkipPermissions` or an invocation bypass; remote mode is constrained to `acceptEdits`, `plan`, `default`, `auto` in this resolver (C17939:5479). An inherited agent mode cannot be widened by repository-controlled agent frontmatter in the shown path. [UNK] Complete precedence across every launch lane, session-mode change and policy state.

[OBS] The bundled SDK warning states that bypass auto-approves tool calls **except explicit deny rules** before `canUseTool`; bare `allowedTools` names similarly shadow the callback. It recommends `PreToolUse` for a hook that gates every call (C40689:2433). This warning is not a claim that bypass defeats every independent sandbox, input validation, hard block or hook.

## Concrete decision: denied Write

This is a static trace, not a live test. Consider an otherwise valid `Write` request with `{file_path:"/work/private.txt",content:"x"}`, a path-matching `edit` deny rule, no hook rewriting the request, and ordinary local execution. Assume the matching and path normalization have resolved the spelling to that rule.

1. [OBS] The Write implementation normalizes `file_path`, computes path spellings, stashes them for the tool-use ID, then calls the shared edit check `ab($T,n,permissions,h)` (C23146:33364).
2. [OBS] `ab` first calls `lto`; `lto` calls `_d` before its other write-boundary checks. `_d` tests each spelling with `za(path,permissions,"edit","deny")` and returns `{behavior:"deny",message:"Permission to edit … has been denied.",decisionReason:{type:"rule",rule}}` on a match (C19134:112740, C19134:121821).
3. [OBS] The Write checker preserves a deny rather than replacing it with the symlink check; the main checker `Iwn` also returns a tool `deny` before its mode-allow branch (C23146:33364, C25240:9657). [INF] This request is denied before Write executes, even when a later mode branch would otherwise allow it. The example is deliberately bounded to this path; a complete implementation must also implement normalization, matching and the other hard checks.

For an ordinary safe in-workspace path with no rule or hard boundary, `ab` processes explicit ask, special-path and safety checks before `plan` and `acceptEdits`. In `plan` it returns ask; in `acceptEdits`, `I_(path,context,spellings)` permits the in-working-directory edit; then applicable allow rules are checked, otherwise it asks (C19134:112740). Session-scoped edit rules have an earlier guarded branch; do not simplify all rules to one universal ordering. `Write.validateInput` separately checks invalid/protected paths and edit-deny rules (C23146:33364).

## General approval flow and classifications

[OBS] `Iwn` checks a whole-tool denial and a matching deny rule first; it handles a whole-tool ask, parses the tool schema and calls the tool's `checkPermissions`, runs additional safety transformers, preserves deny and explicit ask, honors required human interaction, and then evaluates mode bypass/allow-rule/fallback-to-ask branches (C25240:9657; `evidence/13-decision-final.txt`). Some safety checks, sandbox overrides, plan floors and person-only conditions retain an ask despite an otherwise permitting mode. Inputs set aside by the parser cause `checkIncomplete` rather than an unconditional allow. A failed check can fail closed; a missing working directory or internal fault has explicit deny branches (C25240:4396).

[OBS] Result-reason vocabulary in the same permission subsystem includes `rule`, `mode`, `subcommandResults`, `permissionPromptTool`, `hook`, `asyncAgent`, `sandboxOverride`, `workingDir`, `safetyCheck`, `classifier`, `other` (C13657:4083). SDK approval responses can include `updatedInput`, `updatedPermissions` and tool-use ID; deny responses include a message and optional interrupt (C14521:44811). Hook aggregation ranks `deny` above `ask` above `allow` in the cited hook-composition module (C22384:42875). [UNK] The exact total ordering across all hook, plugin, classifier and UI/SDK callback paths is not fully reconstructed; the ranks for that aggregator must not be generalized to every subsystem.

[OBS] For shell commands, an `acceptEdits` helper recognizes `mkdir`, `touch`, `rm`, `rmdir`, `mv`, `cp`, `sed` subject to parser and other checks; this is a helper result, not proof every such command runs unprompted. Bypass and dontAsk are expressly delegated to the main permission flow (C23112:14048).

## Auto mode

[OBS] The classifier receives custom `autoMode.allow`, `soft_deny`, `hard_deny`, `environment`; a bundled classifier-rule-review prompt describes soft deny as overridable by clear user intent and hard deny as unconditional (C7393:4). The settings merger reads rules from trusted user, flag and managed sources and ignores project/local classifier rules; `classifyAllShell` is a separate restrictive setting (C14187:4297). [UNK] Full compiled default rule texts and classifier prompts were not reconstructed here.

[OBS] The auto decision path can simulate the tool's check in `acceptEdits` mode after filtering applicable allow rules, allowing a qualifying result without a classifier; it can also use a guarded safe-allowlist path. It explicitly accounts for linked worktrees and rechecks mode after queueing (C25238:29097; C25238:31030). Other paths use a local or server safety classifier; a `classifierApprovable:false` safety check can require a person (C13657:6254). Logs describe fallback and failure states, including classifier unavailable, no safety verdict, transcript too long, and changed tool input after review (C13657:6889). These do not establish that all classifier failures have the same final behavior.

[OBS] The bundle describes `CLAUDE_CODE_AUTO_MODE_SERVER=0` as disabling server-side checking and reports that a rejected dangerous-tool-use beta/safeguards request may lead to local classification or classifier-unavailable denial depending on the path (C9409:529); `=1` or unset is recommended by a separate eligibility message (C9189:33). [UNK] All rollout gates, classifier verdict format, retries, and exact safeguards request shape are not established.

## Sandbox is a separate boundary

[OBS] `sandbox.enabled` controls sandbox activation. The sandbox schema describes `autoAllowBashIfSandboxed` as independently defaulting to true, while filesystem/network controls still apply (C13866:73850). A startup policy can require sandbox startup rather than falling back unsandboxed; `dangerouslyDisableSandbox` is a per-command escape whose effect depends on the sandbox configuration (C7708:44). Explicit ask/deny rules are described as still respected on the sandbox path (C13027:129). `permissions.blockReadsOutsideWorkingDirectories` and `blockWritesOutsideWorkingDirectories` apply to file tools across modes, with explicitly documented exceptions; the latter does not itself constrain shell commands, MCP tools or hook programs (C13880:13937). [UNK] Full OS-specific isolation implementation and override precedence.
