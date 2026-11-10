---
document:
  type: "research"
  id: "RES-antigravity-cli-tracking"
  title: "Antigravity CLI — upstream client tracking"
  description: "Pinned agy version, the client surface we imitate, and the live-verification log"
  status: "published"
  revision: 5
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/tracking.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 5
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/tracking.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# Antigravity CLI — upstream client tracking

We imitate Google's Antigravity CLI (`agy`) in the `antigravity` provider. It changes
without notice, so this file records what we are pinned to, what we imitate, and when we
last proved it still works.

Full reverse-engineering dossier: [`1.4.8/`](1.4.8/README.md).

## Pinned version

| field | value |
|---|---|
| client | Google Antigravity CLI (`agy`) |
| pinned version | **1.4.8** |
| binary sha256 | `70b1d27f1812f5ebccc8f13daf2209112447413437dd1ab6d87c58b8a04fe35b` |
| binary size | 228,897,168 bytes |
| observed via | `agy --version`, `sha256sum` |
| pinned in `src/` | `src/families/google/transport.ts` (`AGY_VERSION`, `AGY_CHANGE_LIST`, `BASE_URL`, `cliHeaders`); OAuth client and redirect in `src/families/google/oauth.ts` |
| analysis baseline | [`1.4.8/`](1.4.8/README.md) |

## Surface we imitate

| surface | what we reproduce | analysis |
|---|---|---|
| OAuth clients | `consumer` client id + secret, `gcp` client id + secret, PKCE S256, `prompt=consent`, remote redirect `https://antigravity.google/oauth-callback` | [`02-auth.md`](1.4.8/02-auth.md) §2–4 |
| OAuth scopes | the 7-scope set (`cloud-platform`, `userinfo.email`, `userinfo.profile`, `cclog`, `experimentsandconfigs`, `aicode`, `openid`) | [`02-auth.md`](1.4.8/02-auth.md) §4 |
| token lifecycle | `expires_in 3599`, `expiry = now + 3599` | [`02-auth.md`](1.4.8/02-auth.md) §6 |
| model ids | `<family>-<thinking-level>`; Vertex ids `gpt-oss-120b-maas`, `gpt-oss-20b-maas`; `anthropic_version vertex-2023-10-16` | [`03-models.md`](1.4.8/03-models.md), [`04-api.md`](1.4.8/04-api.md) §7.3 |
| endpoints | `daily-cloudcode-pa.googleapis.com` `:loadCodeAssist`, `:fetchAvailableModels`, `:retrieveUserQuotaSummary`; Vertex `publishers/anthropic/models/{model}:streamRawPredict`; Vertex MaaS `endpoints/openapi/chat/completions` | [`04-api.md`](1.4.8/04-api.md) §2–4 |
| body shape | `GetChatMessageRequest` → `GenerateContentRequest`; Anthropic Messages wire for Claude, OpenAI chat-completions wire for GPT-OSS; SSE decode | [`04-api.md`](1.4.8/04-api.md) §6–7 |
| request metadata | `exa.codeium_common_pb.Metadata` (`ide_name`, `extension_version`, `session_id`, `device_fingerprint`, `user_tier_id`, `product_name`, `environment_type`, …) | [`04-api.md`](1.4.8/04-api.md) §6.2 |
| quota caching | 300 s cache, 60 s floor, 5 s repaint; `reset_time` authoritative, `window` display-only | [`05-quota.md`](1.4.8/05-quota.md) §8 |
| user agent / headers | `antigravity/cli/1.4.8 (aidev_client; os_type=linux; arch=amd64; cl=1007749967; auth_method=consumer)`; `Authorization: Bearer <oauth token>`; `Content-Type: application/json`; the inference call sends exactly seven headers and **none** of the `x-goog-*` set; `X-Cloudaicompanion-Trace-Id` is response-only; no quota or rate-limit headers exist | [`04-api.md`](1.4.8/04-api.md) §8.1 |

## Last live verification

| date | what was verified | evidence |
|---|---|---|
| 2026-10-27 | binary identity, `agy --version` 1.4.8, `agy models` catalogue (13 of 26 advertised ids), updater manifest for `linux_amd64`, `install.sh` contents, on-disk state layout, 107 klog call sites | [`1.4.8/evidence/`](1.4.8/evidence/) |
| 2026-10-27 | **wire data**: exact `User-Agent`, response header set, and full bodies of `loadCodeAssist`, `fetchUserInfo`, `fetchAvailableModels`, `retrieveUserQuotaSummary` from `agy -v=5`; real context windows, output caps and thinking budgets; five quota buckets across three groups with their reset times | [`1.4.8/evidence/live-2026-10-27/`](1.4.8/evidence/live-2026-10-27/) |
| 2026-10-27 | **inference and telemetry request heads**, captured behind a local TLS-terminating proxy during one `agy -p` turn: the seven-header inference request with `Authorization: Bearer`, the Clearcut batch to `play.googleapis.com/log`, `aicode.googleapis.com/v1alpha:fetchConfig`, `www.googleapis.com/oauth2/v2/userinfo` | [`inference-request-headers.txt`](1.4.8/evidence/live-2026-10-27/inference-request-headers.txt), [`all-request-heads-masked.txt`](1.4.8/evidence/live-2026-10-27/all-request-heads-masked.txt) |

## pi-auth-plus as sent (2026-10-27)

Captured with a raw-head logging proxy: a preloaded extension rewrote
`https://daily-cloudcode-pa.googleapis.com` to `http://127.0.0.1:18472`, and the proxy forwarded
to the real host. Host and Connection are the proxy hop's; the token is redacted at capture.

Pi-Bolt (Bun `fetch`), inference:

```text
POST /v1internal:streamGenerateContent?alt=sse HTTP/1.1
Accept-Encoding: gzip
Authorization: Bearer <redacted>
Content-Type: application/json
User-Agent: antigravity/cli/1.4.8 (aidev_client; os_type=linux; arch=amd64; cl=1007749967; auth_method=consumer)
Connection: keep-alive
Accept: */*
Host: 127.0.0.1:18472
Content-Length: 9251
[body keys: project,requestId,request,model,userAgent,requestType]
```

Stock Pi (Node `fetch`) sends the same four headers we set, plus `accept: */*`,
`accept-language: *` and `sec-fetch-mode: cors`, lower-cased by undici.
`loadCodeAssist` and `fetchAvailableModels` carry the same header set.

Deliberate deltas against the CLI, all accepted by the server:

- The runtimes' `fetch` adds `Accept: */*` (both) and `accept-language`, `sec-fetch-mode` (Node).
  The fork's raw TLS client stays unported, per the review decision "fetch first".
- Bodies go out with `Content-Length`; the CLI streams them chunked.
- No `writeTrajectoryAcls` before inference, no Clearcut `play.googleapis.com/log`, no
  `aicode:fetchConfig`, no `userinfo` call.
- One `loadCodeAssist` per Pi session and token, cached in memory; the project id is never stored.

| date | what was verified | evidence |
|---|---|---|
| 2026-10-27 | Pi 1.4.0 on Pi-Bolt and stock Pi, stored `/login antigravity` OAuth: startup catalog refresh (5 families, levels in `thinkingLevelMap`, stored in `models-store.json`, footer `$0.000 (sub)`); print-mode `bash` tool loops on `gemini-4-argon:high`, `claude-sonnet-5-5:low`, `gemini-3.8-flash:medium`, `gpt-oss-120b`; RPC sessions switching level high → low → medium and model argon → opus → argon, sonnet → flash → opus, each recalling earlier tool output; HTTP 200 on every request | the heads above; runs in this session, not kept |

Test-harness trap: a fresh `PI_CODING_AGENT_DIR` has no `trust.json`, so in a repository with
`.pi/` interactive Pi waits on the "Trust project folder?" dialog before `session_start` and
the catalog refresh. Answer it (tmux `send-keys`) instead of sending Ctrl+C, which races exit
against the refresh.

Still static: the request *bodies* (converter output, streaming encode) and everything about how
the loopback language-server hop is framed. Request and response **heads** are observed. See
[`08-integration-notes.md`](1.4.8/08-integration-notes.md) §4 for the exact split.

## Change discipline

Per `AGENTS.md`: bumping the pinned version requires, in the same change,

1. a `1.4.9/report.md`-style delta recording what changed (or an updated revision of
   `1.4.8/` if only our understanding changed),
2. refreshed fixtures (`tests/fixtures/`),
3. a live verification with captured evidence,
4. the row above updated.

Regenerating the symbol inventory after a bump is one command:

```bash
python3 .project/research/antigravity-cli/1.4.8/tools/goresym.py --bin ~/.local/bin/agy init
```
