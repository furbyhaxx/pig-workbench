---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-findings-05-crosscheck-existing-research"
  title: "05 Crosscheck Existing Research"
  description: "Development artifact: 05 Crosscheck Existing Research."
  status: "published"
  revision: 2
  created_at: "2026-10-26T12:36:12+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/05-crosscheck-existing-research.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/findings/05-crosscheck-existing-research.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/findings/05-crosscheck-existing-research.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---
# Cross-check against existing project research

The repo already documents Claude quota surfaces in
`../provider-quota-credits-reporting.md` §4, built from a LIVE session against
2.1.306 **plus** a reference client cited as
`.references/oh-my-pi/packages/ai/src/usage/claude.ts`. Those two sources are
mixed in that document, and several behaviours attributed there are the
reference client's choices, not Claude Code's.

**The cited reference is not reproducible.** `ls .references` returns nothing,
`git ls-files | grep '^\.references'` returns 0, and there is no `.gitignore`
entry shadowing it — the directory is simply absent and was never committed.
Every `[CODE: .references/oh-my-pi/…]` citation in
`provider-quota-credits-reporting.md` therefore points at a file a reader cannot
open. `FAILURE_BACKOFF_MS` occurs **0 times** in the Claude bundle, so it is not
a Claude Code identifier either; it is an unreproducible third-party constant.

This matters beyond bookkeeping: three client-behaviour claims below turn out to
be wrong for Claude Code, and a reader following the citation trail would have no
way to tell whose behaviour they were reading.

Verdict per claim: **CONFIRMED**, **NUANCED**,
**NOT-IN-BINARY** (true of the cited reference client, not observed in the Claude binary), or
**CORRECTED**.

---

## Confirmed

| Claim | Verdict | Evidence |
|---|---|---|
| `GET /api/oauth/usage` with `Authorization: Bearer` is the usage endpoint | CONFIRMED | `h = \`${s.url}/api/oauth/usage\`` in the gateway reader; `/api/oauth/usage` appears 10× |
| `oauth-2025-04-20` is the OAuth beta constant | CONFIRMED | `Bu = "oauth-2025-04-20"`, and again as `uAt` in the federation flow |
| `rate_limit_tier` (e.g. `default_claude_max_20x`) is the multiplier, and only `/api/oauth/profile` states it | CONFIRMED | tier appears only in the profile path; the usage payload never carries it |
| `/api/claude_code/policy_limits` is organization **policy**, not quota | CONFIRMED | `DRe = "/api/claude_code/policy_limits"` sits beside the copy `"disabled by your organization's policy"` and `"disabled for the rest of this session because an organization with a restricted policy was signed in earlier"` |
| `utilization` / `percent` are 0..100 percent-used, not fractions | CONFIRMED | matches `v().nonnegative()` on the client schema and the LIVE payload |
| `seven_day_opus` / `seven_day_sonnet` are legacy and now normally null; scoped caps moved to `limits[]` as `weekly_scoped` | CONFIRMED | both names still appear in the `claim` vocabulary, and `weekly_scoped` is a first-class string |
| Named buckets (`iguana_necktie`, …) are opaque and per-account | CONFIRMED | the client treats them structurally; no name→concept table exists in the bundle |

## Nuanced

**"`/api/oauth/usage` stays rate-limited for minutes after a 429; failed
refreshes back off (`FAILURE_BACKOFF_MS = 60_000`)."**

`FAILURE_BACKOFF_MS` is a third-party constant (0 occurrences in the Claude
bundle); it is not a Claude Code identifier. Claude Code instead uses a
**per-bearer failure memo** with a capped hold:

```js
if (ae(r)) m = "auth_rejected", h = xt;
else if (d === 429 || d === 403) { m = "rate_limited"; let S = r.retryAfterMs ?? 0; h = Math.min(S > 0 ? S : Lt, je) }
… pe.remember(s, { kind: m, status: d, body: f, recordedAtMs: y, untilMs: y + h })
```

The observable contract is the same or better — honour `Retry-After`, hold on
failure — but the numbers are `Retry-After`-driven with a cap (`je`), not a flat
60 s. The effective hold cap is **unresolved** (`[UNK]`): the minifier reuses
`xt`/`Lt`/`je` across module scopes and the memo's construction site was not
located.

**"403 is an auth failure."** No — Claude Code folds **403 into
`rate_limited`**, alongside 429. Only a recognisable auth rejection maps to
`auth_rejected`. A quota tool that treats 403 as "bad token" will log the user
out for what is a throttling response.

## Not in the binary

**"404 / 405 / 410 / 501 mean 'this host has no usage endpoint' and justify
switching host."**

That is the reference client's host-failover heuristic. In 2.1.310 the gateway
reader accepts exactly two statuses and does no failover between multiple
candidate hosts:

```js
validateStatus: (K) => K === 200 || K === 404
```

`404` is treated as "no usage route here" and yielded as `undefined`, not as an
error. No 405/410/501 handling was found on this path. Implementing multi-host
failover because of this line would be inventing behaviour Claude Code does not
have.

## Corrected

**"The Claude binary accepts `https://claude.ai/api/oauth/usage` on gateway
hosts, so both origins exist in the wild."**

`CLAUDE_AI_ORIGIN` is `"https://claude.ai"`, but it is **not** used as an API
base. It is used as an **origin allowlist check** on OAuth redirect/error URIs:

```js
function Q(e){ try { return e === new URL(rn().CLAUDE_AI_ORIGIN).origin } catch { return !1 } }
```

The string `claude.ai/api/oauth` does not occur anywhere in either corpus. The
usage base is `BASE_API_URL` (`https://api.anthropic.com`) or the gateway
record's `s.url` — a claude.ai host *could* appear there because a gateway may be
served from one, but not because `CLAUDE_AI_ORIGIN` is a fallback base. Treat
"both origins exist" as unsupported.

---

## Net effect on the existing document

Nothing in §4 of `provider-quota-credits-reporting.md` is *wrong about the
endpoint or the payload*. The corrections are all about **client behaviour**:
the 403 classification, the absence of host failover, the nature of the backoff,
and the `CLAUDE_AI_ORIGIN` misreading. Those should be corrected or re-attributed
as third-party behaviour, and the dangling `.references/oh-my-pi/…` citations
should either be dropped, re-derived from the binary, or the reference vendored
into the repo so the claim is checkable.
