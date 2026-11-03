---
document:
  type: "specification"
  id: "SPEC-0013"
  title: "Secrets: keep credentials out of LLM context without limiting the agent"
  description: "nukii-style secret handling for PiG: red-sec placeholders at every LLM-bound boundary, substitution right before tools run, an encrypted vault, secret:// and ask_user secret questions"
  status: "draft"
  revision: 2
  created_at: "2026-11-03T11:12:00+01:00"
  updated_at: "2026-11-03T11:21:24+01:00"
  created_by: "orchestrator"
  updated_by: "orchestrator"
  changelog:
    - revision: 1
      reason: "User replaced the donor secret guard with the nukii-revived design, plus ask_user secret requests and a secrets scheme"
      changes: ["Purpose, detection, placeholders, keys, vault, boundary hooks, substitution, env injection, secret:// tree, ask_user secret type, settings, host probes and acceptance"]
    - revision: 2
      reason: "User decided Q1–Q4"
      changes: ["Own key at sdk.AgentDir()/secret.key", "No imports, no legacy placeholders", "AES-256-GCM (measured faster than XChaCha20-Poly1305)", "Secrets.toml v2 with nukii's schema; kebab-case keys"]
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
- **PiG credentials:** every `auth.json` entry, resolved through the host (`ModelRuntime.ListCredentials`, `ModelRegistry.GetProviderAuth`), so `!command` keys arrive expanded; Workbench never runs those commands (probe S6).
- **URL passwords** in `scheme://user:password@host`.
- **Explicit:** `write secret://…`, `ask_user` secret questions, `/workbench secrets add`.
- **Exclusions** remove false positives: `values`, `hashes` (placeholders, no plaintext), `patterns`, `env-names`.
- Values shorter than 8 characters are never treated as secrets. Placeholders are never re-detected.

Detection policy is `Secrets.toml` version 2 with nukii's schema (`version`, `[env] names`, `[[patterns]]`, `[exclusions]`; Q4): `sdk.AgentDir()/Secrets.toml`, and in a trusted project `.pig/Secrets.toml`, which may add `patterns` and `exclusions` but not `[env] names`. Each layer's `patterns` and `exclusions` lists are appended. The file holds no values. Detection config is shared by all sessions of the user; a project pattern can only cause over-redaction elsewhere, never a leak.

# 3. Placeholders, keys and vault

## 3.1 Placeholders

`red-sec-<hash>`: `<hash>` is the first 16 characters of lowercase RFC 4648 base32 (`a-z2-7`, no padding) of `HMAC-SHA256(hmacKey, value)` (80 bits). Same value, same placeholder, on every host sharing the key. The algorithm is nukii's, so copying the key file between Workbench and nukii gives identical placeholders; nothing shares a key by default.

## 3.2 Keys

- **Master key:** 32 random bytes, file mode 0600, created on first start if missing. Path: `sdk.AgentDir()/secret.key` (normally `~/.pig/agent/secret.key`; Q1), not configurable. A key file with a wider mode is reported. Losing it makes the vault unreadable and changes every future placeholder.
- **Derived keys** via HKDF-SHA256 (no salt): `info = "nukii red-sec hmac v1"` for the placeholder HMAC (nukii's string, so a copied key gives nukii's placeholders), `info = "pig-workbench secret value v1"` for value encryption.
- Copying the key file to another host gives it the same placeholders and lets it decrypt a copied vault.

## 3.3 Vault

- `$XDG_DATA_HOME/pig-workbench/secrets/vault.jsonl`, directory 0700, file 0600, append-only records under an advisory file lock; all sessions of the user share it and reload on fingerprint change.
- Record: `{v, placeholder, ciphertext, nonce, kind, source: detected|env|env-file|auth|user, scope: global|project, projectId?, name?, env?, description?, created, deleted?}`. Values are insert-if-absent per placeholder; metadata changes and soft deletes are new records (last wins); ciphertext never changes.
- Encryption: AES-256-GCM (Q3) with a random 96-bit nonce per record and the placeholder as AAD. Records are written once per value, far below the 2^32 random-nonce limit per key.
- A torn last line is ignored with a diagnostic; an unreadable vault or key makes the boundary fail closed (§4.4).

# 4. The LLM boundary

## 4.1 Hooks

| Path into a provider request | Hook | Action |
|---|---|---|
| User prompt (typed, pasted, RPC) | `input` | detect, learn, replace with placeholders |
| Tool result content (all tools: builtin, Workbench, other extensions, MCP, codemode children) | `tool_result` | detect, learn, replace in `content` and model-visible structured content |
| User `!` shell output | `user_bash` | same |
| Every message and the system prompt (context files such as `AGENTS.md`, other extensions' custom messages, frames) | `context`, `context_with_system` | replace any remaining value |
| Serialized provider payload (also compaction, branch summaries, extension inference through host models) | `before_provider_request` | final pass; fail closed (§4.4) |

Tool `details` and live progress updates (`tool_execution_update`) are not sent to providers and are not altered.

## 4.2 Entry-time redaction

Content is replaced when it enters the session (prompt, tool result, user shell), so the stored transcript already holds placeholders and every later request, compaction, fork, export or other extension reading the session gets them too. The `context` and provider hooks are the backstop for anything that entered another way (e.g. another extension's hook running after Workbench's). Replacement is deterministic, so prompt caches stay stable.

## 4.3 Substitution

- `tool_call`: every placeholder in the call's arguments is replaced with its real value right before execution, for every tool. The stored assistant message keeps the placeholders.
- Workbench's own tools substitute inside their execution path too (bash command text, file content written by `write`/`edit`, jobs stdin), so behaviour does not depend on hook order.
- Hashline edits: tags and seen-state are computed over the real file bytes; displayed rows carry placeholders; replacement text is substituted before validation and commit (SPEC-0005). `snapshots.view` returns the block already redacted, so its presence check (SPEC-0005 §10) compares like with like.

## 4.4 Failure

- **Fail closed:** if the key or vault cannot be loaded, or a redaction pass errors, the provider request is withheld with a visible error naming the cause and `/workbench secrets` as the place to fix it. Tool calls still run (they need no redaction to execute) but their results are held until the boundary works again.
- `secrets.enabled = false` turns the whole feature off explicitly; then nothing is redacted and the activity line shows `secrets off`.

## 4.5 Limits (stated to the agent and the user)

Transformed values (base64, split, hashed) and secrets inside images escape detection. The fixed system-prompt section tells the agent not to transform secrets.

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

Detection policy lives in `Secrets.toml` (§2), not in `pig-workbench.toml`.

# 9. Host probes (before relying on the boundary)

| # | Probe |
|---|---|
| S1 | A `tool_result` patch is what PiG persists in the session file and sends later |
| S2 | `before_provider_request` fires for normal turns, compaction, branch summaries, title generation and extension inference through host models, and can withhold the request with an error |
| S3 | `context_with_system` exposes and replaces the system prompt text (context files) |
| S4 | `tool_call` input changes reach builtin, Workbench, other-extension, MCP and codemode child tools |
| S5 | `input` transform covers typed, pasted and RPC prompts |
| S6 | `GetProviderAuth` returns the expanded value of a `!command` key for built-in and extension providers |

A probe that fails becomes a named host blocker with an upstream request; the boundary is not declared complete without it.

# 10. Decisions (user, 2026-11-03)

| # | Decision |
|---|---|
| Q1 | Own master key at `sdk.AgentDir()/secret.key`; no sharing with nukii or pi-secret-guard |
| Q2 | No imports from pi-secret-guard or nukii; no legacy placeholder forms |
| Q3 | AES-256-GCM. Measured on an Intel Core Ultra 7 265H, Go 1.24, `x/crypto` v0.58.0: 64-byte value 82 ns vs 240 ns for XChaCha20-Poly1305, 4 KiB 591 ns vs 1293 ns (2.2–2.9× faster, hardware AES); standard library, so no new dependency |
| Q4 | `Secrets.toml` version 2 with nukii's schema; every Workbench config file is TOML (SPEC-0010 §1) |

# 11. Acceptance

- A secret in a file, command output, pasted prompt, `AGENTS.md`, env file or `auth.json` never appears in any captured provider payload (dump of `before_provider_request` across turns, compaction and extension inference).
- The agent uses a placeholder in bash, `write`, `edit`, an HTTP call through another extension and a codemode child call; each receives the real value; the transcript keeps the placeholder.
- Same value, same placeholder across sessions and after restart; equal to nukii's placeholder for a test key shared with nukii's test vectors.
- Vault or key failure withholds provider requests visibly; recovery resumes without restart.
- `secret://` operations and approvals; `.not-a-secret` stops redaction everywhere and stores only the hash; `secrets://` gives the hint.
- `ask_user` secret: value never in transcript, history or frames; env available in bash and jobs immediately, in all sessions of that scope.
- Throughput: redaction of a 300k-token context adds bounded latency per request (memoized per immutable entry; measured, budget set in PLAN-0001 performance evidence).
