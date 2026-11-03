---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-findings-00-artifact-and-transport"
  title: "00 Artifact And Transport"
  description: "Development artifact: 00 Artifact And Transport."
  status: "published"
  revision: 2
  created_at: "2026-10-26T12:36:12+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/00-artifact-and-transport.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/findings/00-artifact-and-transport.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      reason: "imported into pi-auth-plus"
      changes: ["copied from pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/00-artifact-and-transport.md"]
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/findings/00-artifact-and-transport.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---
# Artifact, extraction and usage-poll transport

Author: orchestrator. Covers the binary itself, how the bundled source was
recovered, the 2.1.306 → 2.1.310 delta, and the transport behaviour of the
`/api/oauth/usage` poller (the part that straddles the OAuth and quota scopes).

Evidence grades: **[OBS]** literal present in the bundle, **[INF]** what the
code does, **[UNK]** not resolvable statically.

---

## 1. The artifact

| Property | Value |
|---|---|
| Path | `/home/furbyhaxx/.local/bin/claude` → `/home/furbyhaxx/.local/share/claude/versions/2.1.310` |
| Version | `2.1.310 (Claude Code)` |
| Format | ELF 64-bit LSB executable, x86-64, SYSV, dynamically linked, not stripped |
| Size | 291,638,640 bytes (278 MiB) |
| BuildID | `b47f28de6c0d808a4d3b1d7d26b10f8bcc939db5` |
| Interpreter | `/lib64/ld-linux-x86-64.so.2` |
| `.comment` | GCC 13.1.0 (Ubuntu 20.04 toolchain), clang 23.1.1, **LLD 23.1.1**, `rustc 1.100.0-nightly (574ff7d98 2026-09-14)`, clang 23.1.2 |

`NEEDED` libraries: `librt.so.1`, `libc.so.6`, `ld-linux-x86-64.so.2`,
`libpthread.so.0`, `libdl.so.2`, `libm.so.6`. Nothing else — the JavaScript
runtime and all JS dependencies are inside the file.

### Packaging

It is a **Bun standalone compile** (`bun build --compile`), not a Node SEA.
Evidence: a `.bun_builtins` section, `Bun v${Bun.version}` template literals, and
JavaScriptCore symbols (`JavaScriptCoreUseJIT`, `__DATA,__wtf…`,
`__DATA,__jsc…`). The `.comment` section confirms a Rust component — Bun's
bundler/runtime is partly Rust, and `rustc 1.100.0-nightly` dates the build to
mid-September 2026.

### Section layout (the part that matters)

| Section | File offset | Size |
|---|---|---|
| `.rodata` | `0x2ac000` | 24.1 MB |
| `.text` | `0x19e6900` | 85.9 MB |
| **`.bun`** | **`0x6e14000`** | **0xa7fea02 ≈ 176 MB** |
| `.symtab` | `0x11613268` | 29.6 KB (4.6 KB `.strtab`) |

`.bun` is the compiled module graph: a Bun interned string table plus the
minified JavaScript source of every module. It carries a header magic
`fa e9 7f 0a` followed by 64-bit offsets into the module graph.

Note `.symtab` is only ~30 KB, i.e. it holds exported/libc symbols but **no
Claude Code function names** — the JS is compiled away, so ELF-level symbol
analysis buys almost nothing here. The value is entirely in `.bun`.

### Secret hygiene **[OBS, counts only]**

A pattern sweep of the whole file for credential shapes found **zero** embedded
`sk-ant-api03-…` keys, zero AWS access key IDs, zero Google API keys. The
`sk-ant-oat` (3), `sk-ant-si-` (4) and `ghp_` (4) hits are format examples,
prefix checks and error copy, not live credentials. No credential values are
reproduced in this report or its tooling.

---

## 2. Recovering the source

Full procedure is in `tools/CORPUS.md`; short form:

```bash
dd if=.../2.1.310 of=/tmp/claude.bun bs=1M skip=$((0x6e14000/1048576)) count=200
python3 tools/extract_runs.py     # -> /tmp/cc_code.txt, /tmp/cc_strings.txt
```

Two corpora result:

- `/tmp/cc_code.txt` — 75,594 printable runs ≥ 160 chars, ~52 MB. This is the
  minified JavaScript, interleaved with string-table clusters.
- `/tmp/cc_strings.txt` — 134,876 runs ≥ 24 chars, ~57 MB, offset-prefixed.

Together they are 210,470 searchable runs covering the entire client. There is
**no source map and no original file/module names** — module identity only
survives as chunk references of the form `/$bunfs/root/chunk-<hash>.js` inside
`import` statements **[OBS]**, which is enough to see module seams but not to
name them.

`tools/ctx.py` prints character windows around a match; necessary because a
single corpus "line" can be megabytes long.

**Recovered-surface sanity check:** 5,354 `function` occurrences in the code
corpus, 3,252 distinct `tengu_*` telemetry event names, 22 distinct
`anthropic-ratelimit-unified-*` header names (plus the `-grace-*` prefix family),
1,096 `ANTHROPIC_*`/`CLAUDE_*`-shaped env var names. The corpus is materially
complete, not a fragment.

---

## 3. Version delta 2.1.306 → 2.1.310

Both versions were carved and diffed (offset-prefix stripped before comparing;
a naive `comm` on the offset-prefixed files reports 100 % churn and is wrong).

| Set | 2.1.306 | 2.1.310 |
|---|---|---|
| Distinct string runs | 132,895 | 134,876 |
| Only in 2.1.310 | — | 19,452 |
| Only in 2.1.306 | 17,604 | — |
| `anthropic-beta` dated values | 54 | 54 (**identical set**) |
| `/api/*` paths | identical | identical |
| `anthropic-ratelimit-unified-*` | identical | identical |

**What this means:** the wire contract for quota, auth and betas is *unchanged*
between these two builds. The ~19 k changed strings are internal — feature-flag
names, telemetry event names, UI copy, error messages, e.g.
`tengu_starry_waterfall_shown`, `tengu_remote_rate_limits_dropped`,
`tengu_skill_descriptions_truncated`, `allowUnattendedToolCalls`,
`Rewinds file changes made since a specific user message.`

Practical consequence: any finding in this report verified against 2.1.310
applies to 2.1.306 too for endpoints, headers and auth. UI copy and internal
gating may differ.

New in 2.1.310 and worth noting for telemetry-sensitive users: **GrowthBook**
remote feature flags. The SDK's default host string `https://cdn.growthbook.io`
is present, but this CLI does **not** use it — it runs **remote evaluation
against `https://api.anthropic.com/`** (`POST /api/eval/{key}`, or
`/api/eval-authed/{key}` behind a flag) with a build-inlined SDK key. See
`04-undocumented-apis.md` §5; do not repeat the CDN as the effective host.

---

## 4. How the usage endpoint is actually polled

This is the load-bearing section for `pi-quotas`: the client's own retry, cache
and backoff contract, so we poll the same way instead of inventing one.

### 4.1 Request shape **[OBS/INF]**

```js
let x = await jt.get(r, {
  timeout: 5000,
  headers: { "Content-Type": "application/json" },
  refreshOAuth: true,
  credentials: e,
  validateStatus: Ut
});
```

- Timeout is **5 000 ms**, hard: `timeout: 5000` and, on the gateway path,
  `kHt = 5000` with `AbortSignal.timeout(kHt)` and `maxRedirects: 0`.
- `refreshOAuth: true` means the transport layer is allowed to refresh the
  bearer transparently as part of the call.
- The gateway path (`TOo`) sends only two headers explicitly:
  `Authorization: Bearer <jwt>` and `User-Agent: <UA>`; it accepts **only**
  `200` and `404` (`validateStatus: (K) => K === 200 || K === 404`) and treats
  `404` as "this gateway has no usage route", not as an error.
- `maxRedirects: 0` on the gateway path is deliberate: a redirect off the
  vouched host must fail rather than silently leak the bearer.

### 4.2 Read modes

`le(read)` maps a mode to a URL **[OBS]**:

| Mode | URL |
|---|---|
| `plain` / undefined | `/api/oauth/usage` |
| `cedar_ember` | `/api/oauth/usage?cedar_ember=1&skip_spend=1` |
| `at_wall` | `/api/oauth/usage?at_wall=1&skip_spend=1` (or `…&starry_waterfall=<surface>&skip_spend=1` when gated) |
| `starry_waterfall` | `/api/oauth/usage?starry_waterfall=<surface>&skip_spend=1` |

`me = "starry_waterfall"` is the query-key constant; `Be(e)` returns
`e.surface ?? "cli_menu"` — the default surface is **`cli_menu`**.

Each mode is wrapped in its own telemetry event:
`api_usage_fetch`, `api_usage_fetch_at_wall`, `api_usage_fetch_cedar_ember`,
`api_usage_fetch_starry_waterfall`.

**These are server-side feature codenames, not throwaway experiment arms.**
Correction to an earlier reading of this pattern: the same identifiers also
appear as **top-level keys of the usage response**. The prior LIVE capture
against 2.1.306 (`.project/research/provider-quota-credits-reporting.md` §4.1)
returned `"cedar_ember":null` and `"starry_waterfall":null` among ~20 opaque
codenamed buckets, alongside `iguana_necktie`, `tangelo`, `nimbus_quill`,
`cinder_cove`, `copper_kite`, … `"at_wall"` was **not** present in that payload.
So `read` selects *which variant of the response* the client asks the server for,
and the query flag is what turns that variant on. `cedar_ember` additionally
appears in the bundle's telemetry event-name vocabulary.

`skip_spend=1` rides on every non-`plain` arm while the LIVE payload carries a
full `spend` block, so the most likely meaning is "omit the spend/credits block"
**[INF]** — cheaper or pruned for callers that only want windows.

### 4.3 What the codenamed arms actually are

**Superseded draft — corrected after the quota deep-dive.** An earlier version of
this section called `at_wall` a promo-only variant and left `cedar_ember`
unexplained. Both are wrong. Full detail is in
`02-quota-headers-resets.md` §2; the short version:

| Arm | What it requests |
|---|---|
| `cedar_ember=1` | The **`cedar_ember` reset-grant block** — a reset *program*, not a nickname |
| `at_wall=1` | **Wall-time recovery eligibility**; the client consumes `juniper_tide` from the same response and can obtain cedar grants with it |
| `starry_waterfall=<surface>` | A **usage-credit promotion block** (`kind: none \| pct_off \| extra_credit`) |

So `at_wall` means "at a usage wall", not a timestamp or timezone. The promo
coupling I originally observed is only this: `starry_waterfall` is appended to an
`at_wall` read when the promo gate passes.

```js
function zgt(){ return de().enabled === !0 && !_t() && on("allow_promo_offers") }
```

**The reset programs are a real write surface.** Both POST
`/api/organizations/${org}/reset_rate_limits` — cedar with
`{program, grant_id, request_id}`, juniper with `{program}` — returning results
such as `reset | already_used | not_limited | cooldown | ineligible | unavailable`.
That is a genuine "reset my limit now" capability, gated per account, and it is
the single most important part of this section for a tool that reports resets.

**Consequence:** a quota tool must still poll the **plain** path. The codenamed
arms exist to discover and redeem resets and offers, not to read quota. Adopting
`at_wall` would attach recovery/promo machinery to every quota read for no gain —
and redeeming a reset is a state-changing action that must never happen as a side
effect of polling.

**Why the plain path:** it is what the code itself falls back to for
`read: undefined`, and it is the only variant whose shape has been observed
end-to-end. The codenamed arms are visibly in flux — `at_wall` was not a bucket in
the 2.1.306 capture yet is a first-class read mode in 2.1.310.

### 4.4 In-flight deduplication **[OBS]**

A single per-process record holds `{pending, requester, credentials,
offerSurface}`. A second read with the same requester + credentials returns the
**same promise** instead of issuing a second request:

```js
let d = r.pending !== null && r.requester === s && r.credentials === e;
if (n.read === "starry_waterfall") { … }
if (d) return r.pending;
```

The `starry_waterfall` arm is exempt from sharing (it keys on `offerSurface`),
because two surfaces want different data.

### 4.5 401 → refresh → retry, exactly once **[OBS]**

The client does not refresh proactively when it gets a 401. It:

1. issues the call; on failure records the error against the bearer key that
   was actually sent (`sentAuthKey`);
2. calls `G({credentials, settle:true})` to force the token refresh;
3. if a **different** bearer is now available, re-sends once;
4. if the bearer is unchanged, rethrows.

```js
if (j(v)?.status !== 401 || w.length === 0) throw v;
let E = await Promise.allSettled(w), x = await G({credentials:e, settle:!0});
if (x === null || x === S) throw v;
…
t(`fetchUtilization: ${r} 401 during another read's token refresh; re-sending with the refreshed bearer`), L = await A()
```

A concurrent-refresh guard set `B` collects in-flight refresh promises so two
parallel reads cannot stampede the token endpoint; the `oauthRefreshLatch`
object is reset after success (`Fe.attempted = !1`).

### 4.6 Failure memo — the part to copy **[OBS]**

Failures are remembered **per bearer**, with a capped hold, so a bad credential
or a 429 does not get re-probed on every render:

```js
class se { maxHoldMs; #e = new Map;
  remember(e, n) { … this.#e.set(e, {...n, untilMs: Math.min(n.untilMs, n.recordedAtMs + this.maxHoldMs)}) }
  recall(e, n = Date.now()) { let r = this.#e.get(e); … }
}
```

Recording rule:

```js
if (ae(r)) m = "auth_rejected", h = xt;
else if (d === 429 || d === 403) {
  m = "rate_limited";
  let S = r.retryAfterMs ?? 0;
  h = Math.min(S > 0 ? S : Lt, je);
}
… pe.remember(s, {kind:m, status:d, body:f, recordedAtMs:y, untilMs:y+h})
```

So:

- **`auth_rejected`** (expired/revoked token) → hold for `xt`.
- **`429` or `403`** → hold for `min(retry-after-ms, je)`, defaulting to `Lt`
  when no `Retry-After` is present. **403 is treated as a rate-limit signal, not
  as an auth failure** — a detail many clients get wrong.
- A recalled memo surfaces to the caller as a synthetic
  `ERR_REMEMBERED_RESPONSE` error carrying the original status:
  *"the endpoint answered `<status>` `<n>`s ago; not asked again for `<m>`s"*.

`PFs()` exposes the hold to the rest of the UI: it returns
`n?.kind === "rate_limited" ? n.untilMs : undefined`, i.e. the whole client
learns "do not poll before `untilMs`" from one failed read.

**`xt`, `Lt` and `je` are unresolved** — the minifier reuses those identifiers in
several module scopes and the memo's construction site was not located
(`[UNK]`). The cap `je` is at most a few minutes by behaviour, but we should not
guess a number.

### 4.7 Read-through cache **[OBS]**

The gateway reader keeps `{last, signIn, lastReadAt, reading}` and re-uses `last`
only while it belongs to the same sign-in key
(`SHt(s) = <url>\0<b64-token-suffix-or-16-char-prefix>`) and is younger than
`bOo = 300000` ms:

```js
var bOo = 300000, kHt = 5000, wOo = new W(() => ({last: void 0, signIn: void 0, lastReadAt: 0, reading: void 0}));
function bHt(e, n) { let {lastReadAt: r} = sme(); if (e > r && Date.now() - r > bOo) iNt(n) }
```

**5 minutes** is the client's own staleness budget for a cached usage read.
`kHt = 5000` is the 5 s request timeout on the same path.

Note `reading` is a promise, so concurrent reads coalesce onto one request — a
second dedupe layer beyond §4.4.

### 4.8 Takeaways for pi-quotas

1. Poll the **plain** `/api/oauth/usage`. The query flags are experiment arms.
2. **5 s timeout**, no redirects, no retries on the usage call itself.
3. Cache for **~5 minutes**; that is the first-party budget.
4. On **429 or 403** honour `Retry-After` and stop polling until it expires.
   Treat 403 as rate limiting.
5. On **401**, refresh once and retry exactly once; if the bearer did not
   change, fail loudly.
6. Serialize concurrent reads per credential rather than firing N requests.
7. Do not treat gateway `404` as an error — it means "no usage route here".

---

## 5. Open questions from this slice

| Question | Status |
|---|---|
| Exact values of the memo hold constants `xt`, `Lt`, `je` | `[UNK]` |
| Whether `maxRedirects: 0` applies identically on the first-party (non-gateway) path | `[UNK]` — only observed on the gateway reader |
| Meaning of the `at_wall` / `cedar_ember` / `starry_waterfall` flag names | `[INF]` experiment arms; semantics are server-side |
| Why 403 is folded into `rate_limited` rather than `auth_rejected` | `[INF]` observed behaviour; server-side reason unknown |
