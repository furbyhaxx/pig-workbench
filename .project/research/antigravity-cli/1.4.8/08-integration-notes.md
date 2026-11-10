---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-08-integration-notes"
  title: "agy 1.4.8 integration notes for pi-auth-plus"
  description: "Reimplementation contract per subsystem, required deviations, risk table, observed-versus-read split and open questions"
  status: "published"
  revision: 4
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/08-integration-notes.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 4
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/08-integration-notes.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# 08 — Integration notes for pi-auth-plus

What the `antigravity` family in this repo has to reproduce, what may be safely ignored,
what will break if it is got wrong, and what is still unknown. Read alongside
`AGENTS.md` §Provider integration and §Family invariants — this file is the evidence
behind those bullets.

## 1. The contract, subsystem by subsystem

### 1.1 Login and auth

| must reproduce | detail | source |
|---|---|---|
| PKCE (S256), authorization-code flow | `AuthCodeURL(state, code_challenge_method=S256, code_challenge=…)` + `code_verifier` on exchange | 02-auth §2 |
| `prompt=consent` | always sent; every sign-in re-prompts | 02-auth §2 |
| **remote** redirect URI | `https://antigravity.google/oauth-callback` | 02-auth §2 |
| client pair keyed on auth method | `consumer` → `1071006060591-…` + `GOCSPX-…` (redacted; 02-auth §3 gives the address); `gcp` → `884354919052-…` + its own secret | 02-auth §3 |
| 7 scopes, both methods | `cloud-platform`, `userinfo.email`, `userinfo.profile`, `cclog`, `experimentsandconfigs`, `aicode`, `openid` | 02-auth §4 |
| token lifetime | `expires_in 3599`; stored `expiry` = `now + 3599` exactly | 02-auth §6.1 |

**The load-bearing consequence: manual code paste is mandatory, not a fallback.**
`agy` never opens a listening socket for the consumer flow. The browser is sent to
`antigravity.google/oauth-callback`, that page displays the code, and the user pastes it
back (`SubmitAuthorizationCode` `url.Parse`s it). A loopback-callback implementation will
be rejected by Google because that URI is not a registered redirect for these clients.

Repo consequence: `core/auth`'s "PKCE, callback server, manual code paste" trio is
exactly right, and the antigravity provider must select **manual paste**, not the
callback server. Do not "improve" this into a localhost redirect.

Do **not** use the third client id (`1098991175725-…`): it is referenced only from
`cortex/utils/mcp/mcp.init` and is the MCP-server OAuth client.

### 1.2 Models

| must reproduce | detail | source |
|---|---|---|
| wire model ids | `<family>-<thinking-level>`, e.g. `gemini-4-argon-medium` | 03-models, 04-api §6.2 |
| catalogue fetch | `v1internal:fetchAvailableModels`, auth-gated | 03-models, 04-api |
| refuse retired ids early | `gemini-3.1-pro-*`, `gemini-3.1-flash-*`, `gemini-2.5-flash-*` parse but are not offered | 03-models §3 |
| no hardcoded context window | experiment-assigned via `context-window-options` | 03-models §5 |
| thinking level is in the id | `THINKING_LEVEL_{MINIMAL,LOW,MEDIUM,HIGH,MODEL_DEFAULT}` | 03-models §2 |

`--effort` resolves through `thinkingLevelToEffort`, which lives inside the Anthropic
converter's `applyGenerationConfig`. So effort is applied as a *generation-config* value
on the way out, not as a separate request field. This closes the open question left in
03-models §"Open questions" — partly: the function's existence and call site are
established, the value table is not.

### 1.3 Quota and limits

| must reproduce | detail | source |
|---|---|---|
| tier/plan/credits | `v1internal:loadCodeAssist` | 05-quota §1–2 |
| real bucket ids | `gemini-weekly`, `gemini-5h`, `3p-weekly`, `3p-5h`, `gemini-argon-wtu-weekly` | 05-quota §3.2 |
| **two limits per group** | Gemini and Claude/GPT groups each carry a `5h` **and** a `weekly` bucket; Argon carries weekly only | 05-quota §3.2 |
| limits + reset times | `v1internal:retrieveUserQuotaSummary` | 05-quota §2, §8 |
| reset authority | server `reset_time` (`google.protobuf.Timestamp`) per bucket; **no** client-side window, **no** midnight assumption | 05-quota §4.1 |
| `remainingFraction` | cost-weighted fraction, not request or token count | 05-quota §3.2 |
| `window` is display-only | free-form `string` (`5h`, `weekly`), no enum — render verbatim, never parse | 05-quota §4.1 |
| cache policy | cache 300 s, never refetch within 60 s unless login/model change, repaint from cache at 5 s | 05-quota §8 |
| **reset is rolling** | each bucket's window is anchored to its own first use; the two 5-hour buckets reset 4 s apart, so no clock boundary exists to compute | 05-quota §4.1 |
| untouched buckets re-anchor | a bucket with `remainingFraction: 1` returns `now + window` on every read, so never persist it as a deadline | 03-models "Observed live" |
| error mapping | `RATE_LIMIT_EXCEEDED`, `INSUFFICIENT_G1_CREDITS_BALANCE`, `VALIDATION_REQUIRED`, `MODEL_CAPACITY_EXHAUSTED` | 05-quota §8 |

Two reset clocks exist and are easy to conflate: per-bucket `reset_time`, and the AI
credit balance which carries **no timestamp at all**. Render them separately. This maps
directly onto the repo rule that "no limit", "0% used" and "unknown" stay distinguishable.

### 1.4 Inference path

The single most important structural fact:

```
GetChatMessageRequest (exa.chat_pb, from cortex)
      │  modelRequestToGetChatMessageRequest → converter.ToRequestPayload
      ▼
GenerateContentRequest  ──►  per-vendor wire shape
      │
      ├─ Gemini    →  Cloud Code PA  (v1internal / streamGenerateContent)
      ├─ Claude    →  Vertex AI  publishers/anthropic/models/{model}:streamRawPredict
      │              Anthropic Messages wire, anthropic_version vertex-2023-10-16
      └─ GPT-OSS   →  Vertex MaaS, endpoints/openapi/chat/completions
                     model ids gpt-oss-120b-maas / gpt-oss-20b-maas
```

All three carry the user's **Google** OAuth token. Nothing is called vendor-direct.
`SetCacheControlForJetski` sets Anthropic `cache_control` markers so the *Google* backend
does the prompt caching — that alone rules out a direct Anthropic call.

Responses are SSE, decoded by a line scanner (`deserializeStreamResponse`, eight
instantiations) and, on the Anthropic path, by `(*StreamDecoder).DecodeEvent` plus four
content-block processors (text, thinking, tool, …).

**The observed request head is seven headers**, no more:

```
POST /v1internal:streamGenerateContent?alt=sse
Host · User-Agent (the product UA above) · Transfer-Encoding: chunked
Authorization: Bearer <oauth token> · Content-Type: application/json · Accept-Encoding: gzip
```

`Authorization` is a plain `Bearer` OAuth token — observed, not inferred. None of the
`x-goog-*` headers that the header-construction functions suggest are actually sent on this
path, so a port that copies the code's full list differs from the real client. Body is chunked
JSON; `?alt=sse` selects streaming. See 04-api §8.1.

Every model call carries `exa.codeium_common_pb.Metadata` — the attribution payload
(`ide_name`, `extension_version`, `session_id`, `device_fingerprint`, `user_tier_id`,
`product_name`, `environment_type`, …). That is the privacy-relevant part of the wire
format and it belongs in the tracking file.

### 1.5 Mechanics worth matching

Not required for parity of *numbers*, but they explain observed state:

- Permission presets: `0 UNSPECIFIED, 1 DEFAULT, 2 REQUEST_REVIEW, 3 TURBO, 4 VETTED,
  5 NONE, 6 AUTO`; `settings.json` default is `toolPermission: request-review`.
- Trajectory persistence is dual-store (proto + SQLite); conversation DBs are
  `PRAGMA user_version = 1`.
- Projects are files: `~/.gemini/config/projects/<id>.json`, default
  `default-cli-project`.

## 2. Deviations a Pi-native implementation must make

| `agy` does | we must do instead | why |
|---|---|---|
| token in `~/.gemini/antigravity-cli/antigravity-oauth-token` | Pi `auth.json` slot under our provider id | repo rule: login/logout via Pi, one credential home |
| own OAuth flow + browser + paste prompt | Pi `/login` via provider `oauth` config, manual-paste variant | repo rule |
| reads `~/.gemini/config/config.json` (shared with Gemini CLI) | our own settings catalog | isolation; never read another tool's config |
| `window` rendered verbatim | same, but as a display string only | honesty rule |
| keyring-first, SSH→file, timeout-marker fallback | Pi's credential store | Pi owns persistence |

The SSH-detection branch (`composite_token_storage.go:123`) is worth knowing because it
explains why this machine has a plaintext token file at all: on an SSH session the CLI
deliberately avoids the OS keyring. Do not read that file as "a keyring failure".

## 3. Risk table

Ranked by consequence for this repo.

| risk | likelihood | impact | mitigation |
|---|---|---|---|
| Using a loopback redirect for these clients | high if overlooked | login fails outright | manual code paste, remote redirect |
| Polling `retrieveUserQuotaSummary` faster than the vendor client | medium | account flagging | 300 s cache / 60 s floor from 05-quota §8 |
| Treating `loadCodeAssist` as a quota source | medium | shows no limits, looks broken | it carries tier/credits only; limits come from `retrieveUserQuotaSummary` |
| Hardcoding context windows | medium | silent truncation or over-long prompts | read from catalogue / experiment |
| Serving a retired model id | low | 4xx mid-turn | refuse before credential work |
| Assuming a daily reset boundary | medium | wrong countdown in UI | render server `reset_time` only |
| Sending Claude traffic vendor-direct | low (but tempting) | 401 + wrong billing path | Vertex `streamRawPredict` with the Google token |
| Replaying `thinking`/`signature` blocks across models | medium | 400 corrupted signature | see 06-mechanics §1.5 step shape; sanitize per model |
| Reading `~/.gemini/config/*` | medium | cross-tool coupling | own settings only |

The last-rows deserve emphasis: `thinking` and `thinking_signature` are carried in the
planner response shape, so the port must decide what it does with them per model family.
The repo invariant already says "strip thinking blocks for Claude models; cross-model
signature sanitizing" — this analysis did not add proof beyond the field's existence, so
treat that invariant as still needing its own verification pass against a live stream.

## 4. What was actually observed vs read

Honest split, because it changes how much weight each claim deserves.

**Observed live on this machine**

- `agy --version` → 1.4.8; sha256 and size of the binary.
- `agy models` → the 13-model catalogue in 03-models.
- Updater manifest for `linux_amd64` → version, GCS URL, sha512 (`evidence/`).
- `install.sh` fetched and read (`evidence/install.sh`).
- On-disk state: token file shape, `settings.json`, `jetski_state.pbtxt`, SQLite
  conversation DBs, updater lock/status, `last_check.timestamp`.
- klog call-site inventory (107 distinct sites, `evidence/klog-sites.txt`), including the
  child-language-server process and its two loopback ports.
- **One full agent turn**, behind a local TLS-terminating proxy, which captured every request
  head on the wire: the exact inference request (`POST /v1internal:streamGenerateContent?alt=sse`,
  seven headers, `Authorization: Bearer`), the three Clearcut batches to `play.googleapis.com/log`
  (~117 KB, same token, default Go UA), `aicode.googleapis.com/v1alpha:fetchConfig`, and
  `www.googleapis.com/oauth2/v2/userinfo` (`evidence/live-2026-10-27/`).

**Read from the binary only**

- The inference envelope's *contents*: converter routing, streaming decode, request-body
  assembly. The request **heads** are observed; the bodies are not.
- OAuth client/secret pairing and scope list (instruction immediates — strong, but static).
- Quota *semantics* behind the observed values: the proto schema, the fields the panel reads,
  the throttle constants, `window` being a free-form string.
- Permission presets, sandbox behaviour, PolicyGuardian, battle mode.

So §1.3 and §1.4's header surface are behaviourally verified, §1.2 is behaviourally verified
for the catalogue and still structural for the request it generates, and the request *bodies*
remain structural. Exactly one model turn was spent, on the cheapest offerable model, with a
prompt that could not trigger a tool call.

## 5. Consolidated open questions

Ordered by how much they block the port.

| # | question | blocks | owner doc |
|---|---|---|---|
| 1 | ~~Is `reset_time` fixed or rolling?~~ **Resolved: rolling.** Per-bucket, anchored to each window's first use; the two 5-hour buckets sit 4 s apart, nothing aligns to a clock boundary | — | 05-quota §4.1 |
| 2 | Does `data_collection_optin=false` suppress the Clearcut hop? The hop itself is now proven (a turn posts three batches, ~117 KB, to `play.googleapis.com/log` with the user's token); only the switch is untested, because that needs an account-level setting change | privacy statement | 04-api §11.6 |
| 3 | ~~Outbound header set on an inference call~~ **Resolved.** Seven headers, `Authorization: Bearer`, and none of the `x-goog-*` set the code suggested | — | 04-api §8.1 |
| 4 | `daily-` vs `cloudcode-pa` discriminator | correct host | 04-api §11.1 |
| 5 | The value table behind `thinkingLevelToEffort` | `--effort` mapping | 04-api §7.3 |
| 6 | Which client id the WIF/enterprise path reuses | enterprise login | 02-auth §11.4 |
| 7 | Keyring timeout constants (marker TTL, per-op timeout) | only if we emulate that fallback — we do not | 02-auth §11.1–2 |
| 8 | `ContextWindow.QuotaMultiplier` values | long-context cost display | 05-quota §9.7 |
| 9 | Trusted-CLI escape list in the sandbox | security review only | 06-mechanics §8.8 |
| 10 | `step_format` enum values | nothing for us (we do not read their DBs) | 06-mechanics §8.1 |

Items 2–3: item 3 is closed and item 2 is narrowed to the switch alone. Both came from a single
`agy -p` turn behind a local TLS proxy; the CLI's own `-v=5` logging covered the metadata calls
for free, so only one model turn was ever needed.

## 6. Suggested next steps

1. Write `tracking.md` at the client root (done — pinned to 1.4.8 with the surface we
   imitate).
2. When the `antigravity` family lands, pin the version in code and add this directory's
   path to the tracking row.
3. Settle open questions 1–3 with one captured turn, then bump the analysis revision.
4. Re-run the symbol recovery on the next version bump: `tools/goresym.py` needs only the
   binary path, and `data/` regenerates from it.
