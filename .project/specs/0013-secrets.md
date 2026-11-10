---
document:
  type: "specification"
  id: "SPEC-0013"
  title: "Secrets: keep credentials out of LLM context without limiting the agent"
  description: "nukii-style secret handling for PiG: red-sec placeholders at every LLM-bound boundary, substitution right before tools run, an encrypted vault, secret:// and ask_user secret questions"
  status: "published"
  revision: 10
  created_at: "2026-11-03T11:12:00+01:00"
  updated_at: "2026-11-10T10:50:00+01:00"
  created_by: "orchestrator"
  updated_by: "coding-assistant"
  changelog:
    - revision: 10
      reason: "Align with the dot-project standard"
      changes: ["Status approved becomes published; the user approval of revision 9 and its frozen contracts are unchanged", "Changelog keeps the current revision only; references to deleted drafts removed"]
---

# 1. Purpose

LLM providers receive everything the agent sees. This feature keeps real secret values out of every request sent to a provider, so they never need rotating because a conversation contained them, **without reducing what the agent can do with them**: the agent works with stable placeholders, and Workbench puts the real values back right before a tool runs.

Not goals: hiding secrets from the user, from the screen, from local logs or from local files. Workbench does not refuse to show, log or store anything because it contains a secret. Transcripts hold placeholders only because the transcript is what the next request is built from (§4.2).

It does not stop secrets reaching other destinations: a tool that sends a placeholder somewhere sends the real value.

Source design: `host:/mnt/Projects/nukular/nukii-revived/.project/specs/secrets/spec.md` and its decisions log (2026-10-15 to 2026-10-17). Pi prior art with the same hooks: `host:/mnt/Projects/furbyhaxx/pi-secret-guard/index.ts` (commit `441cdf8`). This replaces the donor secret guard (pi-workbench `src/core/secret-guard.ts`), which masked output with `[redacted]` and is not ported.

# 2. Detection

Secrets are found, never listed by hand:
- **Token shapes:** built-in high-confidence patterns (GitHub, GitLab, Anthropic, OpenAI, Slack, AWS access keys, JWTs, PEM private keys, …) plus user `patterns` (`name` + exactly one of `regex`, `prefix`, `suffix`, with `min-length` ≥ 8 for prefix/suffix).
- **Environment values** whose variable **name** matches an `[env] names` glob (case-insensitive; default `*KEY*`, `*SECRET*`, `*TOKEN*`, `*PASSWORD*`, `*PASS`, `*AUTH*`, `*CREDENTIAL*`, `*PRIVATE*`): the process environment, `~/.env`, `.env*` files from the session cwd up to the repository root, and any env file a tool call references (loaded before the call runs, so its values are already known when the result comes back).
- **PiG credentials:** every `auth.json` entry, resolved through the host (`ModelRuntime.ListCredentials`, `ModelRegistry.GetProviderAuth`), with expansion verified for models.json and `RegisterProvider` key-only registrations (probe S6). `RegisterNativeProvider` with a custom resolver may return literal `!command` strings; S6 does not establish expansion there. Workbench never executes credential command strings itself or learns the literal command as though it were the resolved secret. Native-resolver coverage needs a separate probe; an unresolved inventory entry is reported as a coverage gap, not complete detection.
- **URL passwords** in `scheme://user:password@host`.
- **Explicit:** `write secret://…`, `ask_user` secret questions, `/workbench secrets add`.
- **Exclusions** remove false positives: `values`, `hashes` (placeholders, no plaintext), `patterns`, `env-names`.
- Values shorter than 8 characters are never treated as secrets. Placeholders are never re-detected.

Detection policy is `Secrets.toml` version 2 with nukii's schema (`version`, `[env] names`, `[[patterns]]`, `[exclusions]`; Q4): `sdk.AgentDir()/Secrets.toml`, and in a trusted project `.pig/Secrets.toml`, which may add `patterns` and `exclusions` but not `[env] names`. Each layer's `patterns` and `exclusions` lists are appended. The file holds no values. Detection config is shared by all sessions of the user; a project pattern can only cause over-redaction elsewhere, never a leak.

# 3. Placeholders, keys and vault

## 3.1 Placeholders

`red-sec-<hash>`: `<hash>` is the first 16 characters of lowercase RFC 4648 base32 (`a-z2-7`, no padding) of `HMAC-SHA256(hmacKey, value)` (80 bits). Same value, same placeholder, on every host sharing the key. The algorithm is nukii's, so copying the key file between Workbench and nukii gives identical placeholders; nothing shares a key by default.

## 3.2 Keys

- **Master key:** 32 random bytes, file mode 0600, created on first start if missing. Path: `sdk.AgentDir()/secret.key` (normally `~/.pig/agent/secret.key`; Q1), not configurable. A key file with a wider mode is narrowed to 0600 when the user owns it (reported once); otherwise it is refused. Losing it makes the vault unreadable and changes every future placeholder.
- **Derived keys** via HKDF-SHA256 (no salt): `info = "nukii red-sec hmac v1"` for the placeholder HMAC (nukii's string, so a copied key gives nukii's placeholders), `info = "pig-workbench secret value v1"` for value encryption.
- Copying the key file to another host gives it the same placeholders and lets it decrypt a copied vault.

## 3.3 Vault

- `$XDG_DATA_HOME/pig-workbench/secrets/vault.jsonl`, directory 0700, file 0600, append-only records under an advisory file lock; all sessions of the user share it and reload on fingerprint change.
- Record: `{v, placeholder, ciphertext, nonce, kind, source: detected|env|env-file|auth|user, scope: global|project, projectId?, name?, env?, description?, created, deleted?}`. Values are insert-if-absent per placeholder; metadata changes and soft deletes are new records (last wins); ciphertext never changes.
- Encryption: AES-256-GCM (Q3) with a random 96-bit nonce per record and the placeholder as AAD. Records are written once per value, far below the 2^32 random-nonce limit per key.
- A torn last line is ignored with a diagnostic; an unreadable vault file or key makes the boundary fail closed (§4.4). A single record the current key cannot decrypt (written under another agent directory's key, or after the key file was replaced) is ignored and reported once; it stays in the vault untouched and its value is re-learned under the current key.

# 4. The LLM boundary

## 4.1 Hooks

Verified in [probes S1–S6](../analyses/2026-11-03-secrets-host-probes/README.md).

| Path into a provider request | Hook | Action |
|---|---|---|
| User prompt (typed, pasted, RPC) | `input` (transform) | detect, learn, replace with placeholders |
| Tool result (builtin, Workbench, other extensions, MCP, codemode and its children) | `tool_result` (`content`, `details`, `structuredContent` patch) | detect, learn and replace before model delivery or session persistence |
| User `!` command | `user_bash` returning `operations` | run the command with placeholders substituted; redact streamed output |
| Every message of an agent turn, including the system message (context files such as `AGENTS.md`, other extensions' messages, frames) | `context_with_system` | replace any remaining value (`context` alone does not include the system message) |
| Serialized payload of an agent turn | `before_provider_request` | final pass; fail closed (§4.4) |
| Compaction and branch-summary requests | none fires | clean because they are built from session entries already redacted at entry (§4.2) |
| Extension inference (`ModelRegistry`/`ModelRuntime` `Complete`/`Stream`) | none fires | owner explicitly redacts its request, using `secrets.redact` when opted in (§4.5); Workbench's own inference redacts its request itself |

Tool `details` and `structuredContent` may persist even though they are not provider text. Treat their JSON strings and keys as a retained-data boundary: redact known values, reject unsupported shapes or redacted-key collisions, and never expose raw keys in failure paths. Native binary content blocks keep the existing opaque handling; arbitrary JSON metadata has no opaque exemption. Workbench protects presentation update copies before retaining them; this does not claim global transformation of other extensions' native live progress rendering.

## 4.2 Entry-time redaction

**This is the primary boundary.** Content is replaced when it enters the session (prompt, tool result, user shell), so the stored transcript already holds placeholders and every later request, compaction, branch summary, fork, export or other extension reading the session gets them too. No request hook fires for compaction or branch summaries, so nothing else protects them. `context_with_system` and `before_provider_request` are the per-turn backstop for content that entered another way (another extension's hook after Workbench's, the system prompt, new context files). Replacement is deterministic, so prompt caches stay stable.

Other extensions may append trailing tool-result blocks without modifying the owner's existing blocks or Details. Workbench redacts every model-visible block present when its handler runs, but stock PiG does not guarantee that it runs last. A cooperating extension that appends text must redact that text through §4.5 before returning it, regardless of load order; if redaction is unavailable it omits the annotation with a diagnostic, never appends plaintext. The later per-turn backstops do not retroactively sanitize stored entries used for compaction or branch summaries. Both handler orders require captured transcript and provider-payload tests before claiming coverage.

PiG stores the rendered system message, including `project_context`, in the session file and in compaction entries. It is rebuilt every turn and redacted by `context_with_system`; the stored copy is local only (§1).

## 4.3 Substitution

- `tool_call`: every placeholder in the call's arguments is replaced with its real value right before execution, for every tool. The stored assistant message keeps the placeholders.
- `codemode` itself is skipped: its children fire their own `tool_call` (with `parentToolCallId`) and are substituted there. Substituting in the script would store real values in the result's `nestedCalls` and `details`.
- Workbench's own tools substitute inside their execution path too (bash command text, file content written by `write`/`edit`, jobs stdin), so behaviour does not depend on hook order.
- Hashline edits: tags and seen-state are computed over the real file bytes; displayed rows carry placeholders; replacement text is substituted before validation and commit (SPEC-0005). `snapshots.view` returns the block already redacted, so its presence check (SPEC-0005 §10) compares like with like.

## 4.4 Failure

- **Fail closed:** if the key or vault cannot be loaded, or a redaction pass errors:
  - `before_provider_request` calls `ctx.Abort()` and returns an error, which withholds the request (probe: the turn ends `Request aborted`, nothing reaches the provider). Returning an error alone is not enough: PiG then sends the original payload. Workbench shows a notice naming the cause and `/workbench secrets`.
  - `tool_result` replaces content with a withheld-output error and clears details and structured content atomically, so partially protected output never enters the session. `input` does not send the prompt with the same notice, and returns the typed text to the editor so nothing the user wrote is lost.
  - Recovery needs no restart: the next request after the key and vault load again goes through.
- `secrets.enabled = false` turns the whole feature off explicitly; then nothing is redacted and the activity line shows `secrets off`.

## 4.5 Limits (stated to the agent and the user)

Transformed values (base64, split, hashed) and secrets inside images escape detection. Another extension's own inference request (`ModelRegistry.Complete`) passes no hook: content it took from the session is already redacted, content it read elsewhere is not (host request H-S1). The fixed system-prompt section tells the agent not to transform secrets.

**Approved opt-in service (user, 2026-11-03).** Trusted extensions may request capability `secrets` and call `secrets.redact` through SPEC-0001. Parameters are the closed object `{text: string}`; success is `{text: string, enabled: true}`. Workbench applies the same detector, vault and deterministic placeholders as its own boundary before replying. The whole request and reply must fit SPEC-0001's 64 KiB control envelope; oversized input fails `limitExceeded`, never truncates. The caller redacts every model-visible text field before each request. Arbitrarily chunking an oversized field is not safe: a secret split between calls may evade detection. Such input needs a boundary-preserving larger-input API or must be withheld; this v1 method does not advertise streaming redaction. The API never resolves placeholders back to values.

- Access requires the user/runtime owner grant `secrets.redact-owners`; default `[]`. Project settings cannot grant access. Workbench checks the claimed owner against the grant, but SPEC-0001's bus cannot authenticate that claim; this is an accidental-misrouting guard, not protection against a malicious loaded extension. A grant acknowledges that the existing shared bus exposes plaintext requests to other loaded listeners; it is not a private or authenticated channel. This service is only for a runtime whose loaded extensions the user trusts, not for an external bridge. Workbench never logs the plaintext envelope or stores it in operation receipts.
- Disabled detection, unavailable key/vault, deadline, cancellation or redaction failure returns an error without echoing the input. No raw-input success fallback. The caller MUST withhold its inference on any failure or unavailable service unless the user explicitly chooses an unprotected mode; discovery absence is not permission to send plaintext.
- Redaction does not intercept subsequent calls. The extension remains responsible for fields added after the service returns, appended annotations, retries and outputs re-entering its own inference pipeline.
- A research child session created without Workbench has no Workbench hooks. The parent may explicitly redact each request it constructs, but pre-redacting the initial prompt does not protect later child tool results or internal continuations. An autonomous child needs Workbench loaded or an equivalent verified request boundary before handling secret-bearing content. No claim of automatic child protection on stock PiG.
- Fork-wide interception is a Slice 3 enhancement, not a prerequisite or a stock-host guarantee.

## 4.6 Agent awareness

No list of secrets. One fixed system-prompt section (via `before_agent_start` system prompt options), adapted from nukii `prompt.rs`:

> **Secrets, certificates and credentials** are redacted automatically. You only ever see `red-sec-<hash>` placeholders, never real values from tools, commands or files. Use these exact strings wherever you would use the real secret: in commands, config, env vars, HTTP headers and code; the harness substitutes real values right before a tool runs. Environment variables such as `$GITHUB_TOKEN` expand to real values inside bash, and their output comes back redacted. This keeps secrets out of your context so they need not be rotated; it does not stop leaks elsewhere: a placeholder you post somewhere external publishes the real secret. Never request rotation or refuse a task because a placeholder appeared. Do not transform secret values (base64, splitting, hashing). Use `secret://` to list, name or store secrets, and `ask_user` with type `secret` when one is missing.

# 5. Environment injection

Secrets with an `env` name in the session's scope chain (project, then global; the nearer scope wins) are set as real values in the environment of Workbench `bash` calls and jobs, local and SSH. The command text the model sees stays as written. The host process environment is not changed. SSH sends them as part of the remote launch, never in the displayed command.

Job logs keep raw output (nukii decision A11, 2026-10-16): logs are local files; anything read back through `read job://…` or the jobs tool passes the boundary.

# 6. `secret://`

Follows SPEC-0012 §5 (singular name; `secrets://` answers "did you mean `secret://`").

```text
secret://                              effective secrets of this session: project, then global; name, scope, kind, source, env, placeholder
secret://global/<name>                 global secrets; unnamed detected ones appear as <kind>-<hash>
secret://project/self/ → secret://project/<slug>/
secret://project/<slug>/<name>         a project's secrets (keyed by project ID, SPEC-0012 §3)
secret://by-hash/<hash> → canonical entry
secret://.not-a-secret/                move target only (false positives)
```

| Operation | Effect | Approval |
|---|---|---|
| `ls`, `find`, `grep` | metadata only | none |
| `read secret://<path>` | placeholder + metadata (name, scope, kind, source, env, description, created). **Never the value** | none |
| `write` with a value | stores it encrypted in that scope, redacted from then on; returns the placeholder | none |
| `write` with an existing placeholder | names or moves that secret to the path (metadata only) | none |
| `rm` | soft delete | user |
| `mv <a> <b>` | rename or move between scopes | none |
| `mv <a> secret://.not-a-secret/` | stops redacting that value everywhere: adds its placeholder to `[exclusions] hashes` in the user `Secrets.toml` with a round-trip edit (never plaintext, never project config), then soft-deletes. The exclusion is persisted before the vault changes; an interrupted operation reports uncertainty | user |

`rm` and `mv` reach the router through the bash scheme-command path (SPEC-0008 G8.20). Approvals use the Workbench confirm dialog in the session the user sees; denial, timeout (only when one was set) and cancellation are distinct results.

User command `/workbench secrets`: list by scope (name, kind, source, env, placeholder), add (input dialog), rename/move, mark not-a-secret, soft-delete, show key and vault status. Same operations as the scheme; the user's action is its own approval. A `reveal` action shows a value in the panel: the panel is local UI, never model context.

# 7. Asking the user for a secret

`ask_user` question type `secret` (SPEC-0009 §2):

```json
{"id":"gh","type":"secret","prompt":"GitHub token with repo scope","secret":{"name":"github-token","scope":"project","env":"GITHUB_TOKEN","description":"Used to push the release branch"}}
```

- `secret`: `name` 1–64 of `[A-Za-z0-9._-]`, not starting with `.`; `scope` `project` (default) or `global`; optional `env` (shell variable name ≤ 64); optional `description` ≤ 200 characters, no control characters. No options, no freeform-vs-choice split.
- The value is entered in a dedicated input (TUI kit view; RPC input dialog) that never touches the editor. It goes straight to the vault (`source: user`) and is never written to the transcript, answer history or frames; the tool result carries only the placeholder and metadata.
- Answering with an existing placeholder is allowed (e.g. by the coordinator, SPEC-0011 §9, whose context only holds placeholders).
- The value is available immediately: as the placeholder to every tool, and as `env` in bash and jobs of every session of that scope.

# 8. Settings and storage (SPEC-0010)

| Key | Default | Scope |
|---|---|---|
| `secrets.enabled` | true | user/runtime |
| `secrets.env-injection` | true | user/runtime |
| `secrets.redact-owners` | [] | user/runtime only; explicit trusted-owner grants for §4.5 |

Detection policy lives in `Secrets.toml` (§2), not in `Workbench.toml`.

# 9. Host probes

Run 2026-11-03 against pig 0.5.3+1.4.0: [ANA-2026-11-03-secrets-host-probes](../analyses/2026-11-03-secrets-host-probes/README.md).

| # | Probe | Result |
|---|---|---|
| S1 | `tool_result` patch persisted and sent later | yes; also `input` transforms and `user_bash` output |
| S2 | `before_provider_request` coverage and withholding | agent turns only; error or panic is fail-open; `ctx.Abort()` withholds |
| S3 | `context_with_system` replaces the system prompt | yes |
| S4 | `tool_call` changes reach every tool kind | yes, including MCP and codemode children |
| S5 | `input` covers typed, pasted and RPC prompts | yes |
| S6 | `GetProviderAuth` expands `!command` keys | yes for models.json and `RegisterProvider` key-only providers; not a claim about `RegisterNativeProvider` custom resolvers |

Host requests (not filed; external publication needs user go-ahead):
- **H-S1:** fire `before_provider_request` (or a dedicated hook) for compaction, branch summaries and extension-initiated model streams.
- **H-S2:** let a `before_provider_request` error withhold the request, or add a `{block, reason}` result.

The boundary does not depend on either: entry-time redaction covers compaction and branch summaries, and `ctx.Abort()` covers withholding.

# 10. Decisions (user, 2026-11-03)

| # | Decision |
|---|---|
| Q1 | Own master key at `sdk.AgentDir()/secret.key`; no sharing with nukii or pi-secret-guard |
| Q2 | No imports from pi-secret-guard or nukii; no legacy placeholder forms |
| Q3 | AES-256-GCM. Measured on an Intel Core Ultra 7 265H, Go 1.24, `x/crypto` v0.58.0: 64-byte value 82 ns vs 240 ns for XChaCha20-Poly1305, 4 KiB 591 ns vs 1293 ns (2.2–2.9× faster, hardware AES); standard library, so no new dependency |
| Q4 | `Secrets.toml` version 2 with nukii's schema; every Workbench config file is TOML (SPEC-0010 §1) |
| Q5 | Opt-in `secrets.redact` for trusted extension-owned inference on stock PiG; child sessions without Workbench remain outside automatic protection; fork-wide interception stays in Slice 3 |

# 11. Acceptance

- A secret in a file, command output, pasted prompt, `AGENTS.md`, env file or `auth.json` never appears in any provider request captured at a mock provider: agent turns, compaction, branch summaries and Workbench's own inference (the probe harness of §9 is the starting point).
- The agent uses a placeholder in bash, `write`, `edit`, an HTTP call through another extension and a codemode child call; each receives the real value; the transcript keeps the placeholder.
- Same value, same placeholder across sessions and after restart; equal to nukii's placeholder for a test key shared with nukii's test vectors.
- Vault or key failure withholds provider requests visibly; recovery resumes without restart.
- `secret://` operations and approvals; `.not-a-secret` stops redaction everywhere and stores only the hash; `secrets://` gives the hint.
- `ask_user` secret: value never in transcript, history or frames; env available in bash and jobs immediately, in all sessions of that scope.
- Opt-in extension inference: granted owner receives the same placeholders as an ordinary agent turn; unknown or ungranted owner is refused; key/vault failure, disabled detection and oversized requests never return raw input as success. Capture the extension's final provider payload, not just the service reply.
- Child-session coverage: a child without Workbench is reported as unprotected; pre-redacting its initial input is not advertised as protecting subsequent tool results or continuations.
- Throughput: redaction of a 300k-token context adds bounded latency per request (memoized per immutable entry; measured, budget set in PLAN-0001 performance evidence).
