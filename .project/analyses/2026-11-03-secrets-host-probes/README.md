---
document:
  type: "analysis"
  id: "ANA-2026-11-03-secrets-host-probes"
  title: "Probes S1–S6: can PiG host the SPEC-0013 secrets boundary?"
  description: "Runtime evidence for entry-time redaction, provider-request hooks, system prompt replacement, tool-call substitution across tool kinds, prompt input transforms, !command credential resolution, and fail-closed behaviour"
  status: "published"
  revision: 1
  created_at: "2026-11-03T11:45:00+01:00"
  updated_at: "2026-11-03T11:45:00+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User asked to run the SPEC-0013 §9 probes"
      changes: ["Ran S1–S6 plus fail-closed, user shell, codemode and branch-summary probes against pig 0.5.3+1.4.0"]
---

# 1. Method

- `pig` 0.5.3+1.4.0 from `PATH`, staged SDK `sdk-899c7964784a0050`, RPC mode with sessions persisted, isolated `PIG_HOME` copy, built-in extensions on (codemode, MCP). No billed inference: scripted mock provider (`evidence/harness/mock.py`, now also logging the `Authorization` header and the raw body).
- Probe extension [`evidence/extension/probe-secrets`](evidence/extension/probe-secrets/extension.go): one fake secret and one fixed placeholder `red-sec-probeprobeprobep`. Each hook logs what it sees; env `PROBE_HOOKS` switches redaction per hook so every hook is tested alone and in combination. Tool `probe_echo` (an extension tool), command `/probe-infer` (extension inference through `ModelRegistry.Complete`), `/probe-auth`, `/probe-tree-sum`.
- MCP: a minimal stdio server [`mcp_echo.py`](evidence/harness/mcp_echo.py), exposure `direct`, which logs whether it received the real value.
- Proof of substitution without reading the value: commands run `echo red-sec-… | sha256sum`; the output `1af7534e…` is the hash of the real value.
- Main scenario ([`s_main`/`st_main`](evidence/harness/scenarios/)): prompt with the secret; one turn calling `read secret.txt`, `bash` (placeholder, `cat`, `$GITHUB_TOKEN`), `probe_echo`, `mcp__echo__echo` and `codemode` (children `probe_echo`, `bash`, `mcp__echo__echo`); a user `!` command; two more prompts; `/probe-infer` for a models.json provider and an extension-registered provider; `/probe-auth`; manual compaction. Request indexes in every run file: 0–3 agent turns, 4–5 extension inference, 6–8 compaction. `AGENTS.md` in the work dir holds the secret.
- Each `evidence/runs/<run>.txt` is the condensed output of `show.py` (which provider request carried the secret, in which message), the probe log, and the session-file count. Raw requests are not committed (system prompts, 40k-word fillers).

# 2. Results

| # | Question | Verdict | Evidence |
|---|---|---|---|
| S1 | Is a `tool_result` patch what PiG persists and sends later? | **Yes.** Patched content is stored in the session file and sent in every later request. The same holds for `input` transforms and user `!` output | `runs/entry-cm.txt` (2 occurrences left, both the stored system message), `runs/raw.txt` (17 without redaction) |
| S2 | Does `before_provider_request` see every provider request and can it withhold one? | **Agent turns only.** It does not fire for compaction, branch summaries or extension inference (`ModelRegistry.Complete`); `context`/`context_with_system` don't either. A returned payload replaces the request. **An error or panic does not withhold it: the original, unredacted request is sent** (fail-open). `ctx.Abort()` inside the handler does withhold it: no request reached the provider and the turn ends `Request aborted`. No title-generation request was observed | `runs/bpr.txt`, `runs/block.txt`, `runs/block-panic.txt`, `runs/block-abort.txt`, `runs/tree-raw.txt` |
| S2a | Are compaction and branch summaries clean anyway? | **Yes, with entry-time redaction.** They are built from session entries, which `input`/`tool_result`/user-bash redaction already cleaned | `runs/entry.txt` req 6–8, `runs/tree-entry.txt` req 2 |
| S3 | Does `context_with_system` expose and replace the system prompt? | **Yes.** Its `messages` include the system message (with `AGENTS.md` as `project_context`); replacing the value there removes it from the request. `context` alone does not include the system message. `before_agent_start` sees the rendered prompt too | `runs/cws.txt`, `runs/ctx.txt` |
| S4 | Do `tool_call` input changes reach every tool kind? | **Yes:** builtin `read`/`bash`, extension tool, MCP tool, codemode, and codemode children, which fire their own `tool_call` with `parentToolCallId` | `runs/raw.txt` (`probe_echo` saw the value; MCP log `secret: true`; hash `1af7534e`) |
| S4a | Codemode detail | Substituting in the codemode **script** stores real values in the session (`nestedCalls[].arguments`, `details.calls[].args`). Skipping codemode itself and substituting only its children keeps placeholders there and still runs every child with the real value | `runs/entry.txt` (8 occurrences), `runs/entry-cm.txt` (2, none in tool data) |
| S5 | Does `input` cover typed, pasted and RPC prompts? | **Yes:** `source` is `rpc` or `interactive`; typed text and bracketed paste were both transformed before the request | `runs/tui.txt`, `runs/entry.txt` |
| S6 | Does `GetProviderAuth` expand `!command` keys? | **Yes,** for a models.json provider and for an extension-registered key-only provider; `ListCredentials` (from `ModelRuntimeCreate`) lists both. The provider also received the expanded key as bearer token | `runs/raw.txt` (`auth_get keyIsSecret: true, keyIsCommand: false`) |
| — | User `!` commands | `user_bash` returning `operations` with an `Exec` wrapper substitutes placeholders in the command and redacts streamed output; RPC output, session entry and later requests hold the placeholder | `runs/entry.txt` (`ub` response) |

Also observed:
- PiG stores the rendered system message (including `project_context`) in the session file and in compaction entries. It is rebuilt every turn and was not part of any compaction or branch-summary request, so only the local file holds the value.
- `tool_result` `details` are not sent to the provider (MCP details hold only server/tool names; codemode details are covered by S4a).
- With `--tools`, MCP and codemode tools were missing, and `-ne` disables both built-ins; probes ran without either flag.

# 3. Consequences for SPEC-0013

1. **Entry-time redaction is the primary boundary, not a nice-to-have** (`input`, `tool_result`, `user_bash` operations). It alone covers compaction and branch summaries, which no request hook sees.
2. `context_with_system` is the per-turn backstop (system prompt, other extensions' messages). `before_provider_request` is the last pass for agent turns.
3. Fail closed by calling `ctx.Abort()` in `before_provider_request` and returning an error, plus a visible notice. Returning an error alone sends the unredacted request.
4. Do not substitute inside the `codemode` script; substitute in its children.
5. Extension inference (`ModelRegistry.Complete`/`Stream`) passes no hook. Workbench redacts its own inference requests itself. Another extension's self-built request is outside the boundary unless the content came from the session (already redacted). Host request below.

# 4. Upstream requests (not filed; external publication needs user go-ahead)

1. Fire `before_provider_request` (or a dedicated hook) for compaction, branch summaries and extension-initiated `ModelRegistry`/`ModelRuntime` streams.
2. Let a `before_provider_request` handler error or panic withhold the request instead of sending the original payload, or add an explicit `{block: true, reason}` result.
