---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-report"
  title: "Claude Code 2.1.310 — binary reverse-engineering report"
  description: "Static analysis of the shipped claude CLI binary: bundle extraction, OAuth and token handling, quota endpoints and reset semantics, rate-limit headers, model metadata, undocumented APIs, and the usage-polling contract that pi-quotas should copy."
  status: "published"
  revision: 3
  created_at: "2026-10-26T11:52:00+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi-bolt/orchestrator"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/report.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/report.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pi-auth-plus"
      changes: ["copied from pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/report.md"]
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/report.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# Claude Code 2.1.310 — binary reverse-engineering report

Everything below was recovered from the installed binary
(`/home/furbyhaxx/.local/share/claude/versions/2.1.310`) by carving its embedded
JavaScript bundle. No endpoint was called; no credential is reproduced anywhere
in this report or its tooling.

## Read this first

Seven things that change how `pi-quotas` should treat Claude:

| # | Finding | Consequence |
|---|---|---|
| 1 | **`403` is a rate-limit signal, not an auth failure.** The client folds `403` and `429` into the same `rate_limited` state. | Never invalidate a credential on 403. Honour `Retry-After` instead. |
| 2 | **The usage poll is a 5 s-timeout, no-redirect, no-retry call with a ~5-minute read-through cache.** | Our poller should cache, not poll. 5 minutes is the first-party budget. |
| 3 | **Failures are memoised per bearer, not globally**, with a `Retry-After`-derived hold capped by a constant. | One 429 must silence the whole client until the hold expires, per credential. |
| 4 | **`limits[]` in the usage response is the normalised view and the server owns its severity and order.** | Render `limits[]` as given; do not sort, re-rank or re-derive it from the legacy buckets. |
| 5 | **The `?cedar_ember` / `?at_wall` / `?starry_waterfall` query flags are discovery reads for reset grants and promotions, not quota options.** | Poll the plain `/api/oauth/usage`. Those arms exist to find and redeem resets and offers. |
| 6 | **Limit resets are a real, per-account write surface.** `POST /api/organizations/{org}/reset_rate_limits` takes `{program, grant_id, request_id}` (cedar) or `{program}` (juniper) and answers `reset \| already_used \| not_limited \| cooldown \| ineligible \| unavailable`. | Resets are redeemable, not just reported — but never as a side effect of polling. Report availability; redeem only on an explicit user action. |
| 7 | **Headers use fractions, the response body uses percentages.** `anthropic-ratelimit-unified-*` utilization and threshold headers are `0..1` and may exceed `1`; the JSON body's `utilization`/`percent` are `0..100`. Reset headers are Unix epoch **seconds**. | Mixing the two scales silently renders 100× errors. Pick one unit at the parse boundary and convert once. |

The prior project research (`../provider-quota-credits-reporting.md` §4) is
accurate about the endpoint and the payload but wrong about four client
behaviours, and it cites a reference implementation that is not in this
repository. See `findings/05-crosscheck-existing-research.md`.

## Method

| Step | Result |
|---|---|
| Identify artifact | ELF 64-bit x86-64, 291,638,640 bytes, BuildID `b47f28de…`, a **Bun standalone compile** (not Node SEA) |
| Locate the payload | `.bun` section at `0x6e14000`, ~176 MB, containing a Bun interned-string table plus the minified JS module graph |
| Carve | `dd` the section, then `tools/extract_runs.py` → `/tmp/cc_code.txt` (75,594 runs, ~52 MB) and `/tmp/cc_strings.txt` (134,876 runs, ~57 MB) |
| Search | `tools/ctx.py` for character windows around matches; plain `grep -o -E` for inventory sweeps |
| Cross-check | Prior LIVE capture from 2.1.306, and a 2.1.306 build carved the same way for a version delta |

`.symtab` was checked and is useless for this target: 2,426 `FUNC` symbols, all
libc imports (`strlen@GLIBC_2.2.5`, `pthread_create@GLIBC_2.2.5`, …), zero
application symbols. The JavaScript is compiled away, so the value is entirely
in `.bun`.

Evidence grades used throughout: **[OBS]** a literal present in the bundle,
**[INF]** what the code demonstrably does, **[UNK]** not resolvable statically.

Reproduction steps and corpus rules: `tools/CORPUS.md`.

## Findings by topic

| File | Covers |
|---|---|
| `findings/00-artifact-and-transport.md` | The binary, extraction method, 2.1.306 → 2.1.310 delta, and the full usage-poll transport contract (timeouts, dedupe, 401 replay, per-bearer memo, caching) |
| `findings/01-oauth-tokens.md` | OAuth authorize/exchange/refresh, PKCE, scopes, credential storage and precedence, token kinds, validation, the jwt-bearer / OIDC-federation flow |
| `findings/02-quota-headers-resets.md` | The `/api/oauth/usage` response schema, all 31 `anthropic-ratelimit-unified-*` headers, claim and `exceededLimits` semantics, status enums, overage/credits/trial/grace, reset units |
| `findings/03-model-metadata.md` | Model catalogue and aliases, per-model context/output/1M metadata, `/v1/models` and `/api/model_selector`, fast mode and effort, gateway model injection |
| `findings/04-undocumented-apis.md` | Every non-`/v1/messages` endpoint, base-URL and gateway resolution, session ingress, telemetry, remote config incl. GrowthBook, frame/design/marketplace, beta and identity headers |
| `findings/05-crosscheck-existing-research.md` | Claim-by-claim verdicts against the existing project research, plus the dangling-citation defect |

## Endpoint safety for a read-only tool

Everything `pi-quotas` needs is a `GET`. The write surfaces exist and are worth
knowing about, but none of them belong on a refresh path.

| Endpoint | Verb | What it is | Safe to call from a poller |
|---|---|---|---|
| `/api/oauth/usage` | GET | Quota windows, `limits[]`, `extra_usage`, spend | **Yes** — the only endpoint we actually need |
| `/api/oauth/profile` | GET | Account/org identity, `rate_limit_tier`, billing type | Yes, cache it; it changes rarely |
| `/api/oauth/validate` | GET | Token validity check | Yes |
| `/api/oauth/claude_cli/roles` | GET | Org/workspace roles | Yes, cache |
| `/api/claude_code/policy_limits` | GET | Organization policy gating | Yes — but it is **policy**, not quota |
| `/v1/oauth/token` | POST | Refresh / exchange | Only when the access token is actually expired; it **rotates the refresh token** |
| `/api/organizations/{org}/reset_rate_limits` | POST | Redeems a rate-limit reset | **Never** — state-changing, irreversible per grant |
| `/api/oauth/claude_cli/create_api_key` | POST | Mints an `sk-ant-api…` key | **Never** |
| `/api/event_logging/v2/batch`, `/api/v2/rum`, `/api/v2/logs` | POST | Telemetry | **Never** |

Two traps in that table:

- **Refreshing is not read-only.** A refresh response can carry a new
  `refresh_token` **and** a `refresh_token_expires_in`, so a careless refresh loop
  can invalidate the user's session. Claude Code's own margins, which are two
  different subsystems and must not be conflated:
  - interactive CLI token: refresh becomes eligible **5 minutes** before expiry;
  - profile / federation SDK token: **120 s** advisory background refresh, **30 s**
    blocking refresh, **5 s** cooldown after an advisory failure.

  Refresh on 401 or on expiry with those margins; never speculatively.
- **The reset endpoint is per-account gated** and consumes a grant from a finite
  pool (`resets_left`). Calling it to "see what happens" burns a real reset.

## Version delta: 2.1.306 → 2.1.310

Both builds were carved and diffed with the offset prefix stripped (a naive
`comm` on the prefixed files reports 100 % churn and is wrong).

| Set | 2.1.306 | 2.1.310 |
|---|---|---|
| Distinct string runs | 132,895 | 134,876 |
| Only in 2.1.310 | — | 19,452 |
| Only in 2.1.306 | 17,604 | — |
| `anthropic-beta` dated values | 54 | 54 — **identical set** |
| `/api/*` paths | — | **identical** |
| `anthropic-ratelimit-unified-*` headers | — | **identical** |

The **wire contract for quota, auth and betas did not change** between these
builds. The ~19 k changed strings are internal: feature-flag names, telemetry
events, UI copy, error text. Findings verified against 2.1.310 therefore hold for
2.1.306 for endpoints, headers and authentication; UI behaviour and internal
gating may differ.

## What this means for pi-quotas

The actionable contract, condensed from `findings/00-artifact-and-transport.md`:

1. Poll `GET /api/oauth/usage` — the **plain** path, no query flags.
2. **5 s timeout**, no redirects, no retries on the call itself.
3. Cache the result for **~5 minutes** per credential.
4. On **429 or 403**, honour `Retry-After` and stop polling that credential until
   it expires. Do not clear the token.
5. On **401**, refresh once and retry exactly once; if the bearer did not change,
   surface the failure.
6. Serialize concurrent reads per credential instead of firing N requests.
7. Treat a gateway **404** as "no usage route on this host", not as an error.
8. Render `limits[]` as the server ordered it. Derive label text from
   `limits[].kind`/`group`/`scope`, never from the opaque bucket names.
9. Keep the two unit systems apart: rate-limit **headers are fractions of the
   window (`0..1`)**, the **body is percent (`0..100`)**. Reset values in both
   are Unix epoch seconds.
10. **Check credential-file permissions before blaming the token.** Claude Code
    refuses to load a credential file that is group/world-readable or writable
    (`"… is group/world-readable (mode 0o644); run `chmod 600 …`"`). If we read
    the same file and see a permissive mode, report that as the cause instead of
    degrading the row to "unknown".
11. **Never redeem a rate-limit reset from a background refresh.**
    `POST /api/organizations/{org}/reset_rate_limits` changes account state.
    Surface that a reset is available; let the user trigger it.

## Open questions

Confirmed-resolved entries were removed from an earlier draft: `cedar_ember` turned
out to be a reset-grant program (`findings/02` §2) and the jwt-bearer /
OIDC-federation flow is documented in `findings/01` §2 as a workload-identity
path with its own token endpoint, beta and credential layout.

| Question | Status |
|---|---|
| Values of the failure-memo hold constants (`xt` / `Lt` / `je`) | `[UNK]` — the minifier reuses those identifiers across module scopes and the memo's construction site was not located. Do not guess a number. |
| Exact server-side effect of `skip_spend=1` (which fields are omitted) | `[UNK]` — inferred as "omit the spend block", not proven |
| Whether `extra_usage.utilization` is a percent for every account type | `[UNK]` — the gateway lane is explicitly percent; other lanes are not proven |
| Meaning of `_t()` inside the promo gate `zgt()` | `[UNK]` |
| Production semantics of trial / billing-access override / `graceBand` | `[INF]` only |
| Live corroboration against 2.1.310 | Not possible on this host — its `claude` reports `authMethod: none`, so no first-party credential exists here. Static findings are corroborated against the 2.1.306 LIVE capture in `../provider-quota-credits-reporting.md` §4. |
