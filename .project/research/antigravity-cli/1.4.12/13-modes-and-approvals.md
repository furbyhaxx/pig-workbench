---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-13-modes-and-approvals"
  title: "agy 1.4.12 modes and approvals"
  description: "Execution modes, permission grants, approval decisions, vetting and sandbox controls"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:02:40+01:00"
  updated_at: "2026-11-03T12:02:40+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["verify 1.4.12 permission schemas and document approval decision evidence"]
---

# agy 1.4.12 — modes and approvals

`[OBS]` denotes bytes, schemas, CLI output or installed guide text; it does not imply
an executed approval. `[INF]` identifies an interpretation or a contract-derived
example. `[UNK]` marks an unresolved behavior. No settings or live state were changed,
and no model turn was run. The pinned binary identity, CLI help and non-sensitive
settings projection are in [cli-and-settings.txt](evidence/context-recovery/cli-and-settings.txt).

## 1. Two mode surfaces

[OBS] The CLI's `--mode` accepts `accept-edits`, `auto`, and `plan`; it is separate
from `--sandbox`, `--dangerously-skip-permissions`, the session-only `--settings`
override, and `--project` ([CLI help](evidence/context-recovery/cli-and-settings.txt)).
The installed settings on this host contain `agentMode: "accept-edits"`. This is a
stored choice, **not evidence of the shipped default**. A historical release note in
the current binary calls `request-review` the default and describes an interactive
line-level preview before file writes; its exact scope and whether it applies to every
1.4.12 configuration remain [UNK]
([release notes](evidence/context-recovery/release-notes-extracts.txt)).

[OBS] Embedded mode descriptions distinguish:

| Mode | Documented behavior |
|---|---|
| `accept-edits` | Auto-approve file edits; prompt for commands. |
| `plan` | Research and plan without making changes. |
| `auto` | Auto-approve edits and safe commands; prompt for irreversible actions. Commands leaving the terminal sandbox receive separate review. |

The literal descriptions are at `0x5e1978b`, `0x5d8de63`, `0x5dba49b`, and
`0x5dc3bad`; see [mode descriptions](evidence/context-recovery/mode-descriptions.txt).
The binary has the warning `auto mode is unavailable because the terminal sandbox is
unavailable, falling back to default` (`0x5decbe4`). [UNK] The precise platform,
account, policy and sandbox-availability gates and the complete definition of a safe
or irreversible command need branch-level or live verification. Do not equate `auto`
with unconditional approval. The help says `--dangerously-skip-permissions`
auto-approves tool permission requests without prompting; whether it disables separate
PolicyGuardian, enterprise, or sandbox checks is [UNK].

[OBS] The separate protobuf `AgentPermissionPreset` in the 1.4.12
`codeium_common.proto` descriptor at `0x641a046` retains exactly the 1.4.8 numeric
enumeration ([1.4.8 §3.1](../1.4.8/06-mechanics.md),
[current enum dump](evidence/context-recovery/proto-schemas.txt)):

```text
0 UNSPECIFIED   1 DEFAULT   2 REQUEST_REVIEW   3 TURBO
4 VETTED        5 NONE      6 AUTO
```

The independent `ArtifactReviewMode` is `0 UNSPECIFIED, 1 ALWAYS, 2 TURBO, 3 AUTO`.
The desktop guide also names terminal execution policies `always-proceed`,
`request-review`, `strict`, `proceed-in-sandbox`; a release note describes the last as
auto-approving sandboxed terminal commands and asking for sandbox bypass
([release extract](evidence/context-recovery/release-notes-extracts.txt)). [UNK] A
complete mapping from these UI strings or every CLI mode to the preset enum, including
the exact `TURBO` and `VETTED` branch semantics, was not established. In particular,
the 1.4.8 suggestion that `--dangerously-skip-permissions` sets `NONE` or `AUTO` is
not proved by the 1.4.12 help text.

## 2. Project grants and prompt records

[OBS] The 1.4.12 schema confirms the unchanged string-list grant format described in
[1.4.8 §3.2](../1.4.8/06-mechanics.md): `PermissionGrantsConfig { repeated string
allow=1; repeated string deny=2; repeated string ask=3; }`. The project descriptor at
`0x5f1e37c` adds exact containment and field numbers:

```protobuf
Project.permission_grants = 9;  // PermissionGrants
Project.settings = 10;           // ProjectSettings
PermissionGrants.permission_grants = 2;  // PermissionGrantsConfig
PermissionGrants.v2_migrated = 3;
ProjectSettings.permission_preset = 8;
ProjectSettings.security_plugins = 9;
```

`ProjectSettings` also has `file_access_policy=1`, `internet_policy=2`,
`sandbox_mode=3`, `auto_execution_policy=4`, `artifact_review_mode=5`,
`enable_permissioned_github=6`, and `shell_setup_script=7`
([schema dump](evidence/context-recovery/proto-schemas.txt)). These proto fields do
not by themselves establish their actual on-disk JSON spelling. The binary contains
`ANTIGRAVITY_PERM_GRANTS` (`0x5ca9f0b`); its caller and exact override precedence are
[UNK] for this version ([environment extract](evidence/context-recovery/sandbox-environment.txt)).

[OBS] A runtime `PermissionGrant` has `resource {action,target}` field 1, `allow=2`,
`scope=3`, `user_deny_instruction=4`, plus flat `action=5` and `target=6`. Scope
numbers are `0 UNSPECIFIED, 1 ONCE, 2 CONVERSATION, 3 WORKSPACE, 4 GLOBAL, 5 PROJECT`.
A generic prompt's `PermissionInteractionSpec` carries the resource, persistence
suggestion and type, reason, action description, trigger source, shell name and
allowed approval scopes; the response carries `allow`, `scope`, `edited_target`,
`persist_grants`, `user_deny_instruction`, and `sandbox_override`. For descriptor
numbers and the 1.4.8 discussion, see
[schema dump](evidence/context-recovery/proto-schemas.txt) and
[1.4.8 §3.2–3.3](../1.4.8/06-mechanics.md).

[OBS] `TriggerSource.Type` still has `EFFECTIVE_GRANT`, `DEFAULT_GRANT`,
`CROSS_PROJECT_ISOLATION`, `SECURE_MODE`, `DEFAULT_FALLBACK`, and `FORCE_ASK_HOOK`.
These are provenance labels. **Correction:** their numeric order does not establish
precedence, contrary to the ordered reading in 1.4.8 §3.3. The binary has
`checkSingleTargetOnce`, `LookupEffective`, `LookupDefault`, `grantMatchesResource`,
and `mergeScopedGrants`; [UNK] their complete branch and tie-breaking order, including
conflicting allow/deny/ask patterns, requires control-flow analysis or controlled live
cases ([function index](evidence/context-recovery/function-index.txt)). The presence
of `write_file(%s)`, `read_file(%s)`, and `command(*)` in the grant-construction code
is evidence of examples, not a complete grammar
([additional strings](evidence/context-recovery/additional-strings.txt)).

## 3. One concrete approval path

[INF] The following is a contract-derived case, not a captured execution. It is
reproducible without guessing the unresolved general precedence: assume a CLI run
with permission prompts enabled, no enterprise/sandbox/guardian block or override,
and a matching `PreToolUse` hook for a `run_command` call whose command is `npm test`.
The hook returns:

```json
{"decision":"force_ask","reason":"Requires confirmation for test execution."}
```

1. [OBS] The installed hook contract says `force_ask` always prompts, ignoring cached
   permissions; `ask` differs by respecting the always-allow cache. The matcher is
   evaluated against the normalized tool name. The hook precedes execution, but its
   exact position relative to every built-in permission/guardian check is [UNK].
2. [INF] Construct a generic permission interaction for the requested resource and
   reason, with `FORCE_ASK_HOOK` as provenance when that branch is selected; park the
   step pending the client's decision. The `RequestedInteraction` and completed
   response are recorded on the step as in
   [1.4.8 §3.3](../1.4.8/06-mechanics.md). The descriptor defines the interaction
   fields; it does not prove exact prompt rendering.
3. [INF] If the user denies with `allow=false` and `scope=ONCE`, do not execute this
   call; record the response on the step, with no persistent grant requested. A later
   tool result/denial is projected into the next model request. A different response
   can specify persisted grants or an edited target; neither is assumed here.

The hook input/output contract and example grant `command(npm test)` are in
[builtin-customization-docs.txt](evidence/context-recovery/builtin-customization-docs.txt).
[UNK] This case has not been tested against a live `agy` hook; a disposable isolated
installation and captured trajectory would settle the exact resource formatting,
interaction ordering, and outcome. File reads/writes can additionally encounter the
outside-workspace/gitignored file-access interaction described in
[1.4.8 §3.3](../1.4.8/06-mechanics.md).

## 4. Deterministic vetting and PolicyGuardian

[OBS] The 1.4.12 binary still contains `policyguardian.NewHooks`, a pre-hook, post-hook,
and deterministic pre-checks. Its strings explicitly name checks for `create_file`,
`delete_file`, `file_delete`, `edit_file`, `file_write`, `write_file`,
`write_to_file`, `replace_file_content`, `multi_replace_file_content`, and
`str_replace_editor`. Additional strings say a deterministic pre-check can block, and
that a check that cannot complete is `fail-closed` (`0x5de6df5`). A denied-repeat
pre-check is separately named. Scope, state and rule-family context are in
[1.4.8 §3.5](../1.4.8/06-mechanics.md);
[current extracts](evidence/context-recovery/rodata-modes.txt).

[OBS] A current guardian pre-hook string records `decision=ask_user`; another says a
`vet_destruction` judge that allowed a target on user intent alone asks for
confirmation (`0x5e06ca8`). The 1.4.8 chapter describes the model-backed destruction
judge and its environment ledger; 1.4.12 still contains corresponding guardian and
vetting functions ([additional strings](evidence/context-recovery/additional-strings.txt),
[mode descriptions](evidence/context-recovery/mode-descriptions.txt)). [UNK] Exact
configuration enabling each guardian, ordering versus grant checks, retry count,
judge model, and the full rule table require call-graph/behavioral verification.
The recorded existence of a fail-closed branch does not prove all guardian failures
are fail-closed.

## 5. Sandbox controls and effects

[OBS] `--sandbox` requests terminal restrictions. `ProjectSettings.sandbox_mode` is a
boolean, not the permission preset; admin controls can enforce sandbox mode. The
1.4.12 binary retains the sandbox proxy and trusted CLI file-policy functions, including
`CLIEnv` and `GetSandboxConfig`; the latter logs handling of gated paths overlapping a
workspace root ([sandbox extracts](evidence/context-recovery/sandbox-strings.txt)).
The 1.4.8 sandbox and proxy architecture is described in
[1.4.8 §4](../1.4.8/06-mechanics.md).

[OBS] Binary literals include `ANTIGRAVITY_CREDENTIAL_SCOPE`,
`GOOGLE_DISABLE_SECCOMP_POLICY`, `HTTP_PROXY`, `HTTPS_PROXY`, `NO_PROXY`, and
`SSL_CERT_FILE` ([environment extract](evidence/context-recovery/sandbox-environment.txt)).
Their occurrence alone does **not** establish that the CLI sandbox sets, reads, or
honors each variable in this version. The output environment, mount set, proxy/CA
injection, actual Linux isolation backend, and bypass/failure behavior are [UNK];
inspect the relevant call sites or capture a controlled sandboxed command's environment.
Do not repeat 1.4.8's host-specific degradation claim as a 1.4.12 observation.
