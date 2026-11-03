---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.8-02-auth"
  title: "agy 1.4.8 authentication and identity"
  description: "OAuth flow, client and scope mapping, six auth methods, token storage backends and refresh semantics"
  status: "published"
  revision: 2
  created_at: "2026-10-27T09:59:34+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "pi"
  updated_by: "orchestrator"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/antigravity-cli/1.4.8/02-auth.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 1
      at: "2026-10-27T09:59:34+01:00"
      note: "Initial analysis of agy 1.4.8."
    - revision: 2
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/antigravity-cli/1.4.8/02-auth.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---

# agy 1.4.8 — authentication and identity subsystem

Scope: `google3/third_party/jetski/cli/backend/auth/` plus the two collaborating
implementations in the same binary — `language_server/code_assist_client/codeassistclient`
(token storages, `AntigravityHubAuthProvider`, `StandaloneAuthProvider`) and
`language_server/auth_client/authclient` (callback handler, `TerminalTokenAcquirer`).
Read `00-context.md` first; this section assumes its symbol-recovery toolkit and does not
re-derive the endpoints it already lists.

Evidence classes used below: **observed** (bytes in the binary at a stated address, or a klog
line with `file:line`), **derived** (instruction-level structure, reordered by BOLT, or a
consequence of two observed facts), **unknown** (stated as such).

---

## 1. Overview

Antigravity CLI authenticates a human once, then keeps a single OAuth 2.0 *installed-app*
credential on disk and reuses it for two consumers: the CLI itself and the Cloud Code Assist
backend (`daily-cloudcode-pa.googleapis.com`). The credential is a `*oauth2.Token` wrapped in a
record that also carries the auth method, the OIDC e-mail, and (for enterprise/paid paths) the
user tier and the quota project/region.

Four layers stack, from the UI downwards:

| layer | package | role |
|---|---|---|
| TUI | `cli/model` `AuthModel` | "Select login method:", `startOAuth`, `startGCPOAuth` |
| orchestration | `cli/store` `(*Manager).StartOAuthProcess` / `.Logout` | `auth_manager.go:158` / `:287` |
| auth chain | `cli/backend/auth` `ChainedAuth` | picks and runs one of six methods, persists the record |
| provider adapter | `codeassistclient` `AntigravityHubAuthProvider` | turns the record into `oauth2.TokenSource`, syncs tier/project |

The CLI spawns a language-server process at startup (`server.go:1836`) and then hands it the
*same* auth object: `server_oauth.go:235] b.codeAssistClient.AuthProvider (0x…) is same as
b.cliAuth (0x…)` — one token-source object serves both the CLI's own calls and code assist.

Two OAuth client identities are in play, both shipped as constants: `consumer` (the normal
Antigravity sign-in) and `gcp` (Google Cloud / enterprise). A third embedded client ID exists
but belongs to MCP server OAuth, not to this subsystem (§3).

---

## 2. The flow, step by step

```
AuthModel.renderSelectionMenu           ui: "Select login method:"
  └─ AuthModel.startOAuth / startGCPOAuth
       └─ store.(*Manager).StartOAuthProcess      klog auth_manager.go:158 "Starting OAuth authentication flow"
            └─ auth.(*ChainedAuth).StartInteractive
                 ├─ for each method: IsMethodSupported / TryAuth    (silent restore, see §6)
                 └─ first interactive method:
                      auth.(*oauthMethod).StartInteractive        klog browser.go:56 "%sOAuth: starting OAuth flow"
                        ├─ getOAuthConfig()      → oauth2.Config{ClientID, ClientSecret,
                        │                            Endpoint: google.Endpoint,
                        │                            RedirectURL: https://antigravity.google/oauth-callback}
                        ├─ oauth2.GenerateVerifier + oauth2.S256ChallengeFromVerifier    (PKCE, S256)
                        ├─ oauth2.(*Config).AuthCodeURL(state, 3 opts)
                        │     opts: code_challenge_method=S256, code_challenge=<…>, prompt=consent
                        ├─ os.LookupEnv("SSH_CLIENT"), ("SSH_TTY"), ("SSH_CONNECTION")
                        ├─ go func(){ OpenBrowser(url) }             (browser.go:56 path)
                        └─ returns; pending state/verifier kept on the method

  ── user authorises in the browser ──────────────────────────────────────────────────────

  Google → 302 → https://antigravity.google/oauth-callback?code=…&state=…
       └─ user copies the full redirect URL (or the code) back into the TUI
            └─ auth.(*ChainedAuth).SubmitAuthorizationCode
                 └─ auth.(*oauthMethod).SubmitAuthorizationCode
                      ├─ net/url.Parse + (*URL).RequestURI     (accepts URL or bare code)
                      └─ completeOAuth  →  go func(){ … }       (browser.go:145 area)
                           ├─ oauth2 exchange: POST https://oauth2.googleapis.com/token
                           │     grant_type=authorization_code, code, redirect_uri,
                           │     client_id, client_secret, code_verifier
                           ├─ fetchUserInfo(ctx, token)  → GET https://www.googleapis.com/oauth2/v2/userinfo
                           ├─ SaveToken(record)                      klog browser.go:145 "token acquired, expiry=%v"
                           ├─ SaveUserTier / SaveProjectAndRegion    (when present)
                           └─ log                                    klog browser.go:152, browser.go:167
            └─ auth.(*ChainedAuth).StartInteractive returns         klog auth.go:174 "interactive auth completed via consumer"
                 └─ backend applyAuthResult                          klog server_oauth.go:224/229
                      └─ store.Manager → feature/experiment refresh   klog experiment_manager.go:66/70
```

**Authorization-code with PKCE *and* an embedded client secret.** Both are present and both are
used:

- PKCE: `S256` and `code_challenge` literals plus a call to
  `oauth2.S256ChallengeFromVerifier` inside `(*oauthMethod).StartInteractive`
  (0x9226ec9 → 0x6e482a0; `S256` at 0x58b6d20, `code_challenge` at 0x598f31e), and a
  `code_verifier` literal at 0x5977e4c referenced by the token-exchange closure
  `(*oauthMethod).completeOAuth.func1.1` (0x922ce80+2816) and by
  `(*wifOAuthMethod).completeOAuth.func1.1` (0x922ef20+2848).
- Client secret: `getOauthParams` returns `(clientID, clientSecret, error)`; the secret is a
  35-byte `GOCSPX-…` constant (§3). It is passed into the exchange, so the token request
  carries `client_secret` and the code is bound to the client, not only to the verifier.
- No device-code flow is used by this subsystem. The `google.Endpoint` value in the binary does
  contain `https://oauth2.googleapis.com/device/code` (it is the standard
  `golang.org/x/oauth2/google` endpoint struct), but no auth path in `cli/backend/auth` calls it.

**The redirect target.** `https://antigravity.google/oauth-callback` (41 bytes, 0x5ae5ff3) is
written into `oauth2.Config.RedirectURL` (`[rax+0x58]=ptr`, `[rax+0x60]=0x29`) by
`(*oauthMethod).getOAuthConfig`, called from `StartInteractive`. It is a **remote** page, not a
loopback URI, so nothing can be delivered to a socket on the user's machine by the redirect
itself. Two mechanisms exist in the binary, and they belong to different components:

| mechanism | where | evidence |
|---|---|---|
| hand the code back by pasting the redirect URL | CLI in-process flow receives it through `SubmitAuthorizationCode`, which `url.Parse`s the input | `(*oauthMethod).SubmitAuthorizationCode` calls `net/url.Parse` (0x68a7020) then `(*URL).RequestURI` (0x68ab080) |
| local callback listener on `localhost:0`, path `/auth/callback` | **language server only**: `codeassistclient.(*StandaloneAuthProvider).performAuthFlow` | `tcp`, `localhost:0`, `failed to start local server: %w`, `http://localhost:%d/auth/callback`, `/auth/callback`, `failed to exchange code for token: %w` |
| stdin fallback prompt | same function's `.func3` | `Enter the redirect URL (if redirect failed):` (0x5af7b1f), then `failed to read input: %w` |

So the two strings the brief asks to reconcile are **not** two branches of one code path:
`Enter the redirect URL (if redirect failed):` is owned by
`(*StandaloneAuthProvider).performAuthFlow.func3` (0x84196a0+544, in `codeassistclient`), and
there is no `failed to start sign-in callback listener` literal in `StartInteractive`; the
listener failure message in this binary is `failed to start local server: %w`
(0x5aa3fb8, `performAuthFlow`). The CLI's own path never opens a socket: `StartInteractive`
only builds the URL, launches a browser goroutine, and waits for a submitted code.

`StartInteractive` also probes `SSH_CLIENT`, `SSH_TTY` and `SSH_CONNECTION` via three
`os.LookupEnv` calls (0x9227140/0x922715a/0x9227175) immediately before the browser goroutine is
started. **Derived:** the probe exists to suppress the browser launch in a remote session; the
return value is not used to select the listener, because the CLI has none. (The *storage*
decision "SSH session detected" is a separate, confirmed mechanism — §7.)

The success page `https://antigravity.google/auth-success?app=%s` (0x5b02c86) is served by the
**language server's** callback handler
(`authclient.createAuthCallbackHandler.func1`, 0x8cb1976 references it), not by the CLI.

---

## 3. Client identity mapping

`getOauthParams` (0x9227720, entered at 0x9227720 = `0x92276a0+0x80`; BOLT puts the prologue
after the pclntab entry) is a two-way switch on the auth-method string:

| auth method | length check | client ID (length) | client secret (35 B) |
|---|---|---|---|
| `gcp` (3) | `cmp word [rax],0x6367` = "gc", `cmp byte [rax+2],0x70` = "p" | `884354919052-36trc1jjb3tguiac32ov6cod268c5blh.apps.googleusercontent.com` (72, 0x5b5f9dc) | `GOCSPX-…` @ 0x5abce62 |
| `consumer` (8) | `movabs rdx,0x72656d75736e6f63` = "consumer" | `1071006060591-tmhssin2h21lcre235vtolojh4g403ep.apps.googleusercontent.com` (73, 0x5b61fd9) | `GOCSPX-…` @ 0x5abce3f |

Secret values are redacted here per this repo's no-credentials-in-docs rule. They are
35 bytes each and trivially recoverable from the binary at the stated address:

```bash
python3 .project/research/antigravity-cli/1.4.8/tools/goresym.py \
  --bin ~/.local/bin/agy grep 'GOCSPX-'
```

Both are installed-app OAuth clients: the "secret" is a non-confidential component
shipped inside a desktop binary, which Google's installed-app flow treats as public by
design. Treat them as protocol constants, not as user credentials.
| anything else | — | — | returns `Unknown auth method: %s` / `unknown auth method: %s` (0x5a38d7a / 0x5a38d91, both 23 B) |

The immediates are decisive: the `gcp` branch does `mov eax,0x48` (72), `lea rcx,[0x5abce62]`,
`lea rdx,[0x5b5f9dc]`; the `consumer` branch does `mov eax,0x49` (73), `lea rcx,[0x5abce3f]`,
`lea rdx,[0x5b61fd9]`; both then `mov ebx,eax; mov edi,0x23` (35 = secret length) and return
`(clientID ptr, len, secret ptr, 35, nil error)`.

Third client, `1098991175725-mdu1oe5cbbe1ckggsebobh94toiap657.apps.googleusercontent.com`
(0x5b62022), is referenced by exactly one site in the whole binary:
`cortex/utils/mcp/mcp.init` (0x7d4a76c). It is the MCP-server OAuth client and plays no part in
CLI or code-assist auth.

What the two selection functions select on:

- `getOauthParams(method)` → client id + secret, keyed on the **auth method string**
  (`consumer` / `gcp`), nothing else.
- `getScopesByAuthMethod(method)` → scope list, keyed on the same string.
- `(*oauthMethod).getOAuthConfig()` → assembles `oauth2.Config` from the above two plus
  `oauth2/google.Endpoint` and the redirect URL.
- `oauthConfigFor(method)` → the shared factory used by both the CLI and the language server;
  it also has an enterprise branch (§5).

These four are duplicated in the language server: `authclient.getOAuthConfig`,
`codeassistclient.getOAuth2Config`, and `(*{IDEAuthProvider,HostAuthProvider,AntigravityHubAuthProvider}).GetTokenSource`
all embed the same two client IDs and the same two secrets (reference scan:
0x83d23ee/0x83d303f/0x83d8655 for client `884…`, 0x83ee2df/0x8caf135 for client `1071…`,
0x8caf065/0x923afff for both). The credentials are therefore one shared constant set across
CLI, language server and backend config, not a per-surface choice.

Both are **installed-app OAuth clients**: the "secret" is a non-confidential client
identifier component shipped inside a desktop binary, which Google's installed-app flow treats
as public by design. Recorded here as recovered constants (`GOCSPX-…` at 0x5abce3f and
0x5abce62) for protocol parity; they are not per-user secrets.

---

## 4. Scope selection

Two scope slices exist as globals; `getScopesByAuthMethod` then normalises whichever it returns.

Underlying `[]string` array in `.data` (address = array start, 16-byte string headers):

| idx | address | scope | len |
|---|---|---|---|
| 0 | 0xbe2eee8 | `https://www.googleapis.com/auth/cloud-platform` | 46 |
| 1 | 0xbe2eef8 | `https://www.googleapis.com/auth/userinfo.email` | 46 |
| 2 | 0xbe2ef08 | `https://www.googleapis.com/auth/userinfo.profile` | 48 |
| 3 | 0xbe2ef18 | `https://www.googleapis.com/auth/cclog` | 37 |
| 4 | 0xbe2ef28 | `https://www.googleapis.com/auth/experimentsandconfigs` | 53 |
| 5 | 0xbe2ef38 | `https://www.googleapis.com/auth/cloud-platform` | 46 |
| 6 | 0xbe2ef48 | `openid` | 6 |
| 7 | 0xbe2ef58 | `email` | 5 |
| 8 | 0xbe2ef68 | `profile` | 7 |
| 9 | 0xbe2ef78 | `offline_access` | 14 |

Two slice headers point into it (three words each: ptr, len, cap):

- **set A** at 0xbdd2d10 → `{ptr 0xbe2eee8, len 5, cap 5}` = idx 0–4
  (`cloud-platform`, `userinfo.email`, `userinfo.profile`, `cclog`, `experimentsandconfigs`)
- **set B** at 0xbdd2d30 → `{ptr 0xbe2ef38, len 5, cap 5}` = idx 5–9
  (`cloud-platform`, `openid`, `email`, `profile`, `offline_access`)

`https://www.googleapis.com/auth/aicode` (38 B, 0x5ad2b01) is **not** in either slice; it is a
standalone literal (single relocation slot at 0xbdc3620) that the scopes function appends.

`getScopesByAuthMethod` (entry 0x92278e0) compares the method to `gcp` and, in both branches,
loads the *same* slice header (0xbdd2d10, set A) with `cap` zeroed to force a copy, then runs the
normaliser inline twice. The normaliser (read directly from the instruction stream):

1. copy the slice;
2. if no element equals the 38-byte `aicode` scope, append it
   (`cmp r15,0x26` … `memequal` against 0x5ad2b01, append at 0x9227af4–0x9227b50);
3. if no element equals the 6-byte `openid` scope, append it
   (`cmp [rdx+8],0x6` … `cmp dword [rdx],0x6e65706f` = "open", `cmp word [rdx+4],0x6469` = "id",
   append at 0x9227bbe–0x9227c12 against 0x58cf7f4).

**Result — `getScopesByAuthMethod("consumer")` and `("gcp")` are identical** (that is why one
branch is dead-code-merged into the same three blocks):

```
https://www.googleapis.com/auth/cloud-platform
https://www.googleapis.com/auth/userinfo.email
https://www.googleapis.com/auth/userinfo.profile
https://www.googleapis.com/auth/cclog
https://www.googleapis.com/auth/experimentsandconfigs
https://www.googleapis.com/auth/aicode
openid
```

Method → ordered scope list:

| auth method | ordered scopes | evidence |
|---|---|---|
| `consumer` (interactive Antigravity sign-in) | set A + `aicode` + `openid` (7 entries, above) | `oauthConfigFor` calls `getScopesByAuthMethod` (0x922bba0) in its Google-endpoint branch, where `ClientID/Secret` come from `getOauthParams` (0x922bb66) |
| `gcp` (Google Cloud / enterprise interactive) | same 7 entries | second branch of the same switch in `getScopesByAuthMethod`, loading 0xbdd2d10 |
| enterprise / WIF (`wifOAuthMethod`, `oauthConfigFor` enterprise branch) | set B: `cloud-platform`, `openid`, `email`, `profile`, `offline_access` | set B (0xbdd2d30) is referenced by `oauthConfigFor` (0x922ba96) and `(*wifOAuthMethod).StartInteractive` (0x922c346), both in the block that also loads the enterprise endpoints (0x922ba4b, 0x922c2f2) |
| ADC (`adcTokenStorage.LoadCredentials`) | set A raw (no `aicode`, no `openid` appended) | direct `mov` of 0xbdd2d10 at 0x922ae7a |

`openid` **is** requested (appended by the normaliser, so it is always present for the
interactive methods), and `userinfo.profile` is requested only through set A. `email`/`profile`/
`offline_access` (the OIDC aliases) appear only in the enterprise set.

What the two non-obvious scopes are for:

- `…/auth/experimentsandconfigs` — the experiment/config channel. Its consumer is visible in the
  logs: after every successful auth the CLI refreshes experiments
  (`experiment_manager.go:66 Starting experiment refresh after login`, `:70 Experiments
  refreshed after login`), and the failing pre-login probe is
  `Failed to poll ListExperiments` against the same token. A reimplementation needs this scope
  for the Unleash/experiment machinery (`antigravity-unleash.goog`) and for
  `FetchCLIFeatureProvider` to return server-side flags.
- `…/auth/cclog` — Cloud Code logging/telemetry ingest: the client-side half of the
  `google.internal.cloud.code.v1internal.CloudCode/RecordCodeAssistMetrics` channel (that RPC
  name is present in `.rodata`); `cclog` is what authorises shipping those records. Not needed
  for inference, needed for parity of telemetry calls.

---

## 5. Auth methods

`ChainedAuth` holds an ordered list of methods and tries them in order. Method names come from
each `Name()`; the two 32-byte `Name` thunks that are not elided return literals seen in
`TryAuth`/`StartInteractive` initialisation: `gemini_api_key` (0x598f578), `keyring`
(0x58e1786), `gateway` (0x58e177f), `adc` (0x58afd24). `oauthMethod.Name()` returns the auth
method string itself (`consumer` / `gcp`), which is why the log says
`ChainedAuth: interactive auth completed via consumer` (auth.go:174).

**Precedence — observed, not guessed.** On a run with a valid stored token the log is
`auth.go:157] ChainedAuth: authenticated via keyring (effective: keyring)`, i.e. the stored-token
method wins over a fresh login. On a run with no token every method fails and
`store.(*Manager).StartOAuthProcess` falls through to interactive. The observable order is
therefore:

1. `adcAuth` / `geminiAPIKeyAuth` / `gatewayAuth` — non-interactive; succeed only when their
   inputs exist (ADC file, `GEMINI_API_KEY`, gateway env);
2. `keyringAuth` — non-interactive; succeeds when a stored record exists and validates;
3. `oauthMethod` — interactive (`consumer`, or `gcp` when forced);
4. `externalCorpLoginAuth` / `wifOAuthMethod` — interactive enterprise/WIF.

Derived ranking caveat: `(*ChainedAuth).StartInteractive` iterates its slice and logs
`ChainedAuth: authenticated via %s (effective: %s)` (0x5b10f6b); the exact slice order is
constructed at the call site in `backend.NewServerBackend` / `(*ServerBackendConfig).chainedAuthOrDefault`
(0x923afff–0x923b015 area, which references both client IDs and both secrets) and was not
recovered instruction by instruction.

### 5.1 `oauthMethod` — consumer Google sign-in (browser.go)

- **Trigger**: `ChainedAuth.StartInteractive` when nothing else validates. `TryAuth` is a
  32-byte stub, so this method never attempts a silent authentication; the outlined body
  (`xor eax,eax; ret`) sits outside the pclntab span thanks to BOLT.
- **Stores**: the full record (§7) with `auth_method` = its own name.
- **`StartInteractive`** (0x9226dc0): `ctxlogf("%sOAuth: starting OAuth flow")` →
  `getOAuthConfig()` → PKCE verifier/challenge → `AuthCodeURL(state, code_challenge_method=S256,
  code_challenge=…, prompt=consent)` → SSH env probe → `go func(){ OpenBrowser(url) }`.
  Returns without waiting.
- **`SubmitAuthorizationCode`**: `url.Parse` + `RequestURI`, then `completeOAuth` in a goroutine.
- **`completeOAuth`** (0x92272c0): guards a `sync.Once`/mutex-protected pending-state slot
  (`no OAuth process in progress` when empty), then the closure `completeOAuth.func1.1`
  performs the exchange and persistence: `token_exchange` step name, `OAuth token exchange
  failed`, `%sOAuth: token exchange failed`, `token exchange failed: %w`, network variant
  `%sOAuth: token exchange failed due to network error (check firewall/network restrictions)`,
  `%sOAuth: token acquired, expiry=%v`, `%sOAuth: failed to persist token to keyring`,
  `fetch_user_info` step name, `%sOAuth: failed to fetch user info`, final
  `%sOAuth: authentication completed successfully` then `%sOAuth: authenticated successfully as %s`.
  `(*savingTokenSource).Token` (browser.go:265 in logs) is the refresh half.
- **`Logout`**: `%sOAuth: failed to remove token: %v` (0x5abce85) — removes the record from the
  active storage. `Cancel` returns `no OAuth process in progress` when idle.

### 5.2 `keyringAuth` — the stored-token method

This is the one that actually authenticates on a warm start (`auth.go:157`,
`keyring.go:64] keyringAuth: loaded token, expiry=… expired=false|true`). Despite the name it
reads through the *composite* storage, so on an SSH session it reads the JSON file (§7).

- **Trigger**: always (non-interactive). `StartInteractive` is a 32-byte no-op.
- **`TryAuth`** (0x922a2e0) → `TryAuth.func1` (0x922df80+2336) with the observable decision
  points as literals: `keyringAuth: failed to get oauth params: %v`,
  `keyringAuth: loaded token, expiry=%v expired=%v`,
  `keyringAuth: token validation failed due to network error (token may still be valid): %v`,
  `keyringAuth: saved token invalid: %v`,
  `keyringAuth: failed to fetch user info: %v`,
  `keyringAuth: incomplete GCP onboarding (no userTier or projectID), treating as
  unauthenticated`, and `keyringAuth: context cancelled, skipping keyring auth` in `Logout`.
- **Semantics**: load record → if expired, refresh (§8) → validate → fetch user info → report
  the method as `keyring` (not the stored `auth_method`, which is what
  `server_oauth.go:224` prints separately). The `incomplete GCP onboarding` branch is the
  enterprise gate: a `gcp` record without `userTier`/`projectID` is treated as signed out.
- **`Logout`**: removes the record; `keyringAuth: failed to remove token: %v` (0x5ad96d3).

### 5.3 `geminiAPIKeyAuth`

- **Trigger**: `GEMINI_API_KEY` (0x598eeb0) or `GOOGLE_API_KEY` present. `StartInteractive`
  fails with `geminiAPIKeyAuth: %s is not set` (0x5a9aa41) when neither is.
- **Stores**: nothing on disk; the key is the credential. `TryAuth` (0x922a0c0) is 224 B and
  validates presence/shape. `Logout` is a 32-byte stub (nothing to clear) — which is why the
  logout command cannot revoke it.
- Behavioural evidence that this is a first-class path: the changelog text embedded in `.rodata`
  mentions `GEMINI_API_KEY` sessions, model-catalog fixes and thinking-block handling, and
  `AUTH_MODE_GEMINI_API_KEY` exists as an enum value.

### 5.4 `adcAuth` — Application Default Credentials

- **Trigger**: `AGY_ADC_AUTH` set. Referenced from `main.main`, `backend.NewServerBackend`,
  `commands.(*logoutCommand).Execute.func1`, `commands.(*remoteControlCommand).Execute`,
  `model.AuthModel.renderSelectionMenu`, `store.FetchCLIFeatureProvider`,
  `authclient.(*AuthClient).isAdcEnabled` and `auth.RunLogin`. `AGY_CDE_SEAMLESS_AUTH` is the
  "CDE seamless" twin, checked by `RunLogin`, `AuthClient.isAdcEnabled` and
  `AuthClient.tryRestoreAdcToken`.
- **`Authenticate`** (0x92254c0, 1792 B): `google.FindDefaultCredentialsWithParams` /
  `credentials.DetectDefault` (the two references to `GOOGLE_APPLICATION_CREDENTIALS`),
  scope `https://www.googleapis.com/auth/aicode` (0x5ad2b01 used directly here), quota project
  from `GOOGLE_CLOUD_QUOTA_PROJECT` via `internal.GetQuotaProject`. Literals:
  `adcAuth: no credentials found`, `adcAuth: failed to load credentials: %v|%w`,
  `adcAuth: failed to get token from token source: %v|%w`,
  `adcAuth: authenticated successfully with quota project %s, token is non-empty: %v`,
  `adcAuth: fetching user info failed: %v`.
- **Stores**: no token file; `adcTokenStorage` is a *read* adapter over the ADC token source
  (`LoadCredentials` only, 640 B). `Logout` is a 32-byte stub — the CLI refuses to touch the
  user's gcloud state, which is also why the ADC failure text tells the user to run
  `gcloud auth application-default login` or unset `AGY_ADC_AUTH` (`AuthModel.renderSelectionMenu`
  literals: `  1. Run: gcloud auth application-default login`,
  `  2. Or check GOOGLE_APPLICATION_CREDENTIALS env var.`,
  `  3. Or unset AGY_ADC_AUTH env var to use other sign in methods.`).

### 5.5 `gatewayAuth` — enterprise LLM gateway

- **Trigger**: gateway environment. Keys recovered from `.rodata`:
  `AGY_LLM_GATEWAY_URL`, `AGY_LLM_GATEWAY_MODELS`, `AGY_LLM_GATEWAY_API_KEY`,
  `AGY_LLM_GATEWAY_HEADERS`, `AGY_LLM_GATEWAY_CA_CERT`, `AGY_LLM_GATEWAY_TIER_PRO`,
  `AGY_LLM_GATEWAY_TIER_FLASH`, `AGY_LLM_GATEWAY_TIER_FLASH_LITE`, `AGY_LLM_GATEWAY_WIRE_PROTOCOL`,
  `AGY_LLM_GATEWAY_PROXY_URL`. `StartInteractive` reports `gatewayAuth: %s is not set` (0x5a64db7)
  naming the missing variable, and `%s is not set` (0x5978013).
- **Stores**: nothing; credentials are the gateway API key/headers. `Logout` refers to the same
  `%s is not set` message — nothing to clear. `TryAuth` (0x9229e80, 256 B) checks configuration
  presence only.
- This is the path behind the `AUTH_MODE_ENTERPRISE_GATEWAY` enum value; it bypasses Google
  OAuth entirely and the request goes to the gateway URL rather than
  `daily-cloudcode-pa.googleapis.com`.

### 5.6 `externalCorpLoginAuth` / `wifOAuthMethod` / `cde_login.go` — workforce identity

- `(*externalCorpLoginAuth)` is a five-method shell: `TryAuth`, `StartInteractive`, `Logout`
  are all 32-byte stubs, `IsSupported` and `Name` 32-byte stubs too. It contributes no strings.
  **Derived:** it is a capability gate that reports "unsupported" unless constructed with a
  provider; the real work happens in `wifOAuthMethod`, which `SetWIFProvider` configures
  (`[AuthProvider] SetSavedWIFProvider called with wifProvider: %q`), and in
  `cde_login.go` for Cloud Developer Environment.
- `(*wifOAuthMethod).StartInteractive` (0x922c160, 1568 B) duplicates the consumer flow with the
  enterprise endpoints: `Endpoint{AuthURL: "https://auth.cloud.google/authorize" (0x5abd0fb),
  TokenURL: "https://sts.googleapis.com/v1/oauthtoken" (0x5adfa7f, 40 B)}`, scope set B (§4),
  the same `https://antigravity.google/oauth-callback` redirect, and the same
  `S256`/`code_challenge`/SSH-probe shape.
- `completeOAuth` for WIF differs in one place: it extracts the identity from the **OIDC id
  token**, not from `userinfo` —
  `%sOAuth: failed to extract OIDC claims for email: %v` (0x5b1e13f). That is the workforce
  identity signal; `RememberIDToken` (0x8405380) stores `id_token` (0x58f8711) in the record.
- `cde_login.go` is gated by `AGY_CDE_ENTERPRISE_AUTH` (0x5a38da8, referenced by
  `refreshAndSaveStoredToken`), `AGY_CDE_SEAMLESS_AUTH` (0x5a13d64, `RunLogin`,
  `AuthClient.isAdcEnabled`, `tryRestoreAdcToken`) and `AGY_CDE_AUTH_ACTION`
  (`RunAuthActionWithOptions`: `Unknown AGY_CLI_CDE_AUTH_ACTION: %q (supported: check, login)`).
  The seamless variant fetches cloud licenses during bootstrap (`CDE seamless auth bootstrap:
  failed to fetch licenses: %v`).
- `(*ChainedAuth).StartGCPAuthProcess` (0x9226460) is the `gcp` entry point: it logs
  `StartGCPAuthProcess called but gcpMethod is nil` and
  `GCP authentication is not supported in this configuration`, then delegates.

`isEnterpriseAuth` (0x9228d40) is the four-instruction predicate used by callers to pick the
enterprise scope/endpoint branch; it has no string literals (it compares the method value).

---

## 6. Token record and storage backends

### 6.1 Record format (observed)

`~/.gemini/antigravity-cli/antigravity-oauth-token`, mode `0600`, 560 bytes, one-line JSON:

```json
{
  "token": {
    "access_token": "<258 chars>",
    "token_type": "Bearer",
    "refresh_token": "<103 chars>",
    "expiry": "2026-10-27T10:37:02.369671058+01:00",
    "expires_in": 3599
  },
  "auth_method": "consumer",
  "id_token_email": "<redacted-email>"
}
```

The `token` object is a plain `golang.org/x/oauth2.Token` (field set and `json` names match
`AccessToken/TokenType/RefreshToken/Expiry/ExpiresIn`), and `expiry` is a nanosecond-precision
RFC 3339 timestamp with the local offset. The enclosing type is
`codeassistclient.StoredToken`, which has a custom `UnmarshalJSON` (0x8405460) plus
`IsEmpty`, `ApplyToken`, `RememberIDToken` methods.

Fields confirmed by type metadata in `.data.rel.ro`: `AuthMethod json:"auth_method,omitempty"`
(0xab8cb2d), `IDTokenEmail json:"id_token_email,omitempty"` (0xabc4636),
`IDToken json:"id_token,omitempty"` (0xab8cc78, written by `RememberIDToken`).
Fields written by dedicated setters but absent from this particular file:
`user_tier`, `tier_display_name`, `project_id`, `region` (see §9) — all `omitempty`, so their
absence here only means the consumer path had no tier/project to store (`quotaProject=` empty in
`server_oauth.go:224`). Their exact JSON names come from the setter arguments and from
`SaveUserTier`/`SaveProjectAndRegion`, not from a dump; treat the four names as **derived but
consistent** with the surrounding tag vocabulary (`json:"user_tier,omitempty"` at 0xab6253b,
`json:"tier_display_name,omitempty"` at 0xac0779a, `json:"region,omitempty"` at 0xab2f64c).

Path construction: `auth.NewCLITokenStorage` (0x922a6c0) joins the file name from a prefix and
the literal `-oauth-token` (0x595f0c6, 12 B) — `defaultTokenStorage` supplies the prefix
`antigravity` (0x59440cb), producing `antigravity-oauth-token`. The same file is read by
`cli/remotecontrol/remotecontrol.(*Daemon).LoadOAuthToken` (0x98adf16), so the remote-control
daemon shares the credential. Every write path reports `failed to create directory for token:
%w`, `failed to marshal token: %w`, `failed to write token to file: %w`; reads report
`failed to read token from file: %w` and `failed to unmarshal token: %w`. The 0600 mode of the
token file is **observed on disk**, not read from code; the temp-file + `chmod` + `rename`
sequence is confirmed for the keyring *marker* (§6.3) and is the same pattern the token writers
are shaped like, but the token file's own syscall sequence was not decoded.

### 6.2 Three storages, two of them on each side of the process boundary

`cli/backend/auth` defines `cliFileTokenStorage` (the file backend, plus `SaveUserTier`,
`SaveProjectAndRegion`, `updateStoredToken`, `RemoveToken`) and `adcTokenStorage` (read-only ADC
adapter), and picks one in `defaultTokenStorage`. The language server defines a richer triple in
`codeassistclient`:

| type | methods | notes |
|---|---|---|
| `KeyringTokenStorage` | LoadToken, LoadStoredToken, SaveToken, SaveUserTier, SaveProjectAndRegion, updateStoredToken, RemoveToken | OS keyring, `zalando/go-keyring` v0 with the `secret_service` (D-Bus Secret Service) provider — `ss.NewSecretService`, `OpenSession`, `GetLoginCollection`, `CreateItem`, `SearchItems`, `GetSecret` |
| `fileTokenStorage` | same surface | plain JSON file |
| `compositeTokenStorage` | same surface | prefers keyring, falls back to file per operation |

The keyring **entry name** is built from the same prefixing scheme: `NewCLITokenStorage`
references the literal `keyring` (0x58e1786) as the service and `-oauth-token` as the entry
suffix, so the keyring item is `antigravity` / `antigravity-oauth-token` (service/entry). The
language server's own keyring storage uses the same names so both processes see one credential.

`CLIAuthProvider.SetTokenStorage` (0x83d4860) is how the CLI injects its `cliFileTokenStorage`
into the language-server provider; `server_oauth.go:235`'s "same object" log is the result.

### 6.3 Composite-storage decision and the keyring timeout marker

`NewCompositeTokenStorage` (0x83df8a0) chooses **file-only** in three cases, each with its own
message, checked in this order:

| condition | message | evidence |
|---|---|---|
| caller asked for file storage | `Using file-based token storage because the caller requested it` (0x5b43261) | 0x83df996 |
| SSH session detected | `Using file-based token storage because %s detected` (0x5b15b5a) with `%s` = `SSH session` | 0x83dfafd — **observed** as `composite_token_storage.go:123] Using file-based token storage because SSH session detected`, on every CLI start in `~/.gemini/antigravity-cli/log/*.log` |
| a keyring timeout was recorded within the last `%v` | `Using file-based token storage because a keyring timeout was recorded within the last %v` (0x5b76ef2) | 0x83dfa4f |

"SSH session detected" is a terminal/transport fact, not an auth decision: it comes from the
terminal-info manager (`terminal_info_manager.go:354/503` log `TERM`, `altScreen`, `ssh`, `osc8`,
`osc52` and the SSH DA2 probe response) and its effect is to bypass the D-Bus Secret Service,
which is either absent or unusable in a remote session. This is why the run in the logs used the
file even though a keyring exists — **not** because of a timeout. No marker file exists in
`~/.gemini/antigravity-cli/` in the observed state (`find` for `*keyring*` returns nothing), which
confirms the SSH branch fires first.

The timeout marker is a file named `<prefix>-keyring-unavailable` (0x5a03430, literal
`-keyring-unavailable`, referenced both by `auth.NewCLITokenStorage` at 0x922a905 and by
`codeassistclient.keyringRecentlyUnavailable` at 0x83dfd76) whose path is produced by
`DefaultKeyringMarkerPath` (0x83dfbc0; failure text
`Failed to get user home directory for keyring marker: %v`). Mechanics, from
`(*compositeTokenStorage).memoizeKeyringTimeout` (0x83e0040, 1632 B):

1. remove a stale marker: `failed to remove stale keyring timeout marker %s: %v` (0x5b1e20f);
2. `MkdirAll` the marker directory: `failed to create keyring marker directory: %v` (0x5afd749);
3. `os.CreateTemp(dir, "keyring-marker-*")` (0x59b99b9):
   `failed to create temporary keyring marker file: %v` (0x5b15b8c);
4. write the **current time** formatted with the layout `2006-01-02T15:04:05Z07:00` (0x5a574d4,
   RFC 3339) — `failed to write keyring marker: %v` (0x5ab576c);
5. `chmod` the temp file: `failed to set permissions on temporary keyring marker file: %v`
   (0x5b4329f);
6. close (`failed to close temporary keyring marker file: %v`, 0x5b11091) and
   `rename` it onto the final path (`failed to rename keyring marker file: %v`, 0x5adfb47), so
   readers never observe a partial timestamp.

`keyringRecentlyUnavailable` (0x83dfd60) then reads that timestamp and compares it with the
current time against a TTL (the `%v` in the message; the TTL constant itself was not recovered —
**unknown**, see §11). Every composite operation is individually time-boxed, one message per
operation and a file fallback on expiry:

```
Keyring LoadToken timed out after %v, falling back to file storage            (0x5b501e3)
Failed to load token from keyring, falling back to file: %v                   (0x5b3926e)
Keyring LoadStoredToken timed out after %v, falling back to file storage      (0x5b5fab4)
Failed to load stored token from keyring, falling back to file: %v            (0x5b50225)
Keyring SaveToken timed out after %v, falling back to file storage            (0x5b501a1)
Failed to save token to keyring, falling back to file: %v                     (0x5b31f82)
Keyring SaveUserTier timed out after %v, falling back to file storage         (0x5b58558)
Failed to save user tier to keyring, falling back to file: %v                 (0x5b3fe20)
Keyring SaveProjectAndRegion timed out after %v, falling back to file storage (0x5b6972f)
Failed to save project and region to keyring, falling back to file: %v        (0x5b5adce)
keyring RemoveToken timed out after %v                                        (0x5ad2dd3)
Failed to remove token from keyring (%v) and file (%v)                        (0x5b392a9)
Failed to clear user tier in keyring: %v                                      (0x5adfb74 area)
```

So the timeout does not merely degrade one call: a single timed-out operation *writes the
marker*, and for the whole TTL every later process (and every later operation) goes straight to
the file. That is the mechanism the brief asks to describe precisely:

> keyring operation exceeds its timeout → `memoizeKeyringTimeout` writes
> `<prefix>-keyring-unavailable` containing an RFC 3339 timestamp (atomic rename) → on next
> start `NewCompositeTokenStorage` reads the timestamp via `keyringRecentlyUnavailable` and, if
> it is younger than the TTL, logs `Using file-based token storage because a keyring timeout was
> recorded within the last %v` and never touches D-Bus again until the marker goes stale.

Note the ordering consequence for a reimplementation: the SSH and caller-forced branches are
evaluated before the marker, so a marker written by an earlier GUI session does not override an
explicit file request.

### 6.4 `cliFileTokenStorage` vs `adcTokenStorage` vs the keyring path

| | `cliFileTokenStorage` | `adcTokenStorage` | keyring path |
|---|---|---|---|
| backing store | `~/.gemini/antigravity-cli/antigravity-oauth-token`, 0600 | Google ADC (`GOOGLE_APPLICATION_CREDENTIALS` / gcloud well-known file) | D-Bus Secret Service |
| read | `LoadStoredToken` → read/`UnmarshalJSON`; `LoadToken` → `*oauth2.Token` only | `LoadCredentials` only | `KeyringTokenStorage.{LoadStoredToken,LoadToken}` |
| write | `SaveToken`, `SaveUserTier`, `SaveProjectAndRegion`, `updateStoredToken` (all via temp+rename) | never | `SaveToken`, `SaveUserTier`, `SaveProjectAndRegion` |
| clear | `RemoveToken` (`failed to write token to file: %w` is reused on the remove path) | never (`Logout` stub) | `RemoveToken` |
| used when | composite fell back, or the CLI injected it | `AGY_ADC_AUTH` / `AGY_CDE_SEAMLESS_AUTH` | default on a GUI session |

---

## 7. Refresh and lifetime

**Expiry is stored, not computed.** `expires_in: 3599` is written verbatim, and `expiry` is
computed once as `now + expires_in` at the moment the token is acquired or refreshed. The logs
make the relationship exact:

- `browser.go:145] consumerOAuth: token acquired, expiry=2026-10-27 09:00:58.413042423 +0100 CET`
  at `08:00:59.413257`. `09:00:58.413 - 3599 s = 08:00:59.413`, i.e. `expiry` is exactly the
  acquisition instant plus `expires_in`; nothing is rounded.
- `browser.go:265] token refreshed, new expiry=2026-10-27 10:37:02.369671058 +0100 CET` at
  `09:37:03.369690`. `10:37:02.370 - 3599 s = 09:37:03.370`, the refresh instant again plus
  `expires_in`. (The `m=+3599.348` monotonic stamp on that line is a process-uptime value and
  just happens to be near 3599; it is not the delta and must not be read as one.)
- The stored file holds the same `10:37:02.369671058`, so persistence and the log agree to the
  nanosecond.
- `keyring.go:64] keyringAuth: loaded token, expiry=2026-10-27 09:21:59.923679408 +0100 CET
  expired=true` at `09:37:03.279461` — expiry is compared as an absolute instant against the
  wall clock, with no skew allowance visible: the token had been expired for ~15 minutes.

**What triggers a refresh.** `(*savingTokenSource).Token` (0x9227ec0; it owns the `browser.go:265`
line and the literals `token refresh failed due to network error: %v`, `Failed to save refreshed
token: %v`, `token refreshed, new expiry=%v`) is an `oauth2.TokenSource` wrapper: it asks the wrapped
source for a token, and when the underlying `oauth2.ReuseTokenSource` returns a *changed*
token it persists it through the storage's `updateStoredToken`
(`failed to persist refreshed token: %w`, 0x5acbf11, also used by
`refreshAndSaveStoredToken`). The underlying `x/oauth2` logic is what decides *when*: refresh
when `expiry` is within the library's 10-second delta, refresh unconditionally when the token
has no expiry. Practically, the CLI refreshes on the first API call after expiry — which is why
the 09:37 session refreshed 90 ms after loading a token that had been dead for 15 minutes.

**Validation.** `auth.ValidateStoredToken` (0x922bc80) checks the record: `%w: %v` (0x58cf800)
and `%w: no refresh token` (0x5a03444), i.e. a record whose access token has expired *and* has no
refresh token is rejected outright. `refreshAndSaveStoredToken` (0x9228de0) is the explicit
refresh entry (`failed to persist refreshed token: %w`), and `generateState`'s neighbourhood
carries `token refresh failed: %v` (0x5a4a912).

**On refresh failure.** Two distinct outcomes are observable in the strings:

1. **Network** failures do not invalidate: `token refresh failed due to network error: %v`
   (`savingTokenSource.Token`) and, in `keyringAuth.TryAuth`,
   `keyringAuth: token validation failed due to network error (token may still be valid): %v` —
   the stored credential is kept and the method fails softly.
2. **Credential** failures clear: `keyringAuth: saved token invalid: %v`, and in
   `RunLogin.func2` the pair `stored credentials are invalid or expired: %w` (0x5afd587, two
   call sites at 0x922de10 and 0x922de51) next to `not authenticated: no stored credentials
   found` (0x5b02c58) and `failed to load stored credentials: %w` (0x5acbf5b).

Correction for the record: `Clearing stored token for %s after failure` (0x5b02e52, single
occurrence) is **not** in this subsystem — its only referrer is
`cortex/utils/mcp/mcp.(*StreamableHTTPConnector).Connect` (0x7d4beb8). MCP server OAuth clears
its own stored token on failure; the CLI auth path reports
`stored credentials are invalid or expired` instead and leaves the decision to `RunLogin`.

---

## 8. Identity resolution

**`fetchUserInfo`** (0x9228860) and **`fetchAdcUserInfo`** (0x9225c80) both GET
`https://www.googleapis.com/oauth2/v2/userinfo` (45 B, 0x5afd52d; also a relocation slot at
0xbdc35f0 that `fetchUserInfo` loads at 0x922893a) with the bearer token, and report
`userinfo request failed: %w`, `userinfo returned status %d`, `failed to decode userinfo: %w`.
`fetchUserInfo`'s literal scan additionally surfaces `Failed to open browser: %v` (0x5a64d83) and
`fetchAdcUserInfo`'s surfaces `Failed to load credentials: %w`, both almost certainly BOLT span
bleed from neighbours rather than real references — an unresolved detail, not a behaviour claim.
`fetchAdcUserInfo` is fed by the ADC token source and names itself `adc` (0x58afd24).

**What identifies the user.** Two independent values, both observed in the stored record:

- `id_token_email` — the e-mail carried in the `id_token` (set through `RememberIDToken`, which
  writes the `id_token` field, and extracted by the WIF path via
  `%sOAuth: failed to extract OIDC claims for email: %v`).
- the live `userinfo` e-mail, fetched after every successful login and restore; that is the
  value the TUI and logs print — `browser.go:167] consumerOAuth: authenticated successfully as
  <redacted-email>`, `server_oauth.go:229] OAuth: authenticated successfully as <email>`.

The subject is never used as the key: there is no `sub` handling in this package, and
`id_token_email` is the field that survives restarts. `errorreport`/plugin code that needs "the
signed-in user" goes through the token source, not the record
(`rpcs_plugin_auth.go:106] Not reloading plugin MCP servers: cannot get the signed-in user:
error getting token source: You are not logged into Antigravity.`), so the e-mail gates
plugin/MCP loading.

**`userTier` / `tierDisplayName`.** Produced server-side, cached client-side:

- the setter logs `[AuthProvider] SetUserTier called with userTier: %q, tierDisplayName: %q`
  (0x5b5fa6c) from `(*AntigravityHubAuthProvider).SetUserTier` (0x83d785d) — the provider used
  for the Cloud Code backend;
- the values come from the Cloud Code onboarding response (`loadCodeAssist` / `OnboardUser`;
  `OnboardUser operation failed: %v` appears in `performAuthFlow`), whose proto fields are
  `user_tier` and `tier_display_name` (proto tags `bytes,1,opt,name=user_tier,json=userTier` and
  `bytes,2,opt,name=tier_display_name,json=tierDisplayName`);
- they are persisted through `SaveUserTier` (`codeassistclient` composite/keyring/file variants
  and `cliFileTokenStorage.SaveUserTier`), and cleared on logout: `failed to clear user tier: %w`
  (`RunLogin`), `Failed to clear user tier in keyring: %v`;
- `keyringAuth` treats a `gcp` record without a tier or project as signed out
  (`keyringAuth: incomplete GCP onboarding (no userTier or projectID), treating as
  unauthenticated`), which is the enterprise gate.

**`projectID` + `region`.** `SaveProjectAndRegion(projectID, region)` exists on every storage
(`cliFileTokenStorage.SaveProjectAndRegion` 0x922b8e0, `KeyringTokenStorage…`, `compositeTokenStorage…`)
and has an inverse used by logout: `failed to clear project and region: %w` (0x5ad2b4d in
`RunLogin`, `failed to save project and region: %w` 0x5acbf36). The values are the resolved
Cloud Code Assist target: the logs show the *local* project id
(`server.go:2217] Backend project ID updated dynamically to: default-cli-project`) is a separate
concept, while the *quota* project comes from the credential
(`applyAuthResult: … quotaProject=` — empty for consumer, and `GOOGLE_CLOUD_QUOTA_PROJECT` for
ADC). Practically: `project_id`/`region` on the record describe which Cloud Code project the
token is entitled to use, which is what enterprise/GCP logins need and consumer logins leave
empty.

---

## 9. Login, logout and status entry points

**`RunLogin`** (0x92290a0, 2656 B) is the top-level CLI login. Literals it contains, in
suggested reading order:

```
AGY_ADC_AUTH, AGY_CDE_SEAMLESS_AUTH, consumer, gcp
Already authenticated as %s.
failed to initialize token storage: %w
failed to load stored credentials: %w            (also in RunLogin.func2)
failed to save credentials: %w
failed to save project and region: %w
failed to clear project and region: %w
failed to clear user tier: %w
empty token returned          -keyring-unavailable          %w: no refresh token
GOOGLE_CLOUD_PROJECT
Successfully authenticated as %s.
```

Semantics, derived from those branches plus the logs: initialise storage → if a valid record
exists, print `Already authenticated as %s.` and return; otherwise run `ChainedAuth` (with the
ADC/CDE env overrides) → on success save the record, print
`Successfully authenticated as %s.` → then `SaveProjectAndRegion` and (for qualified records)
`SaveUserTier`. `RunLogin.func1`/`func2` are the deferred/error halves that translate failures
into `stored credentials are invalid or expired: %w` and `not authenticated: no stored
credentials found`.

**`RunAuthActionWithOptions`** (0x9229b00) is the CDE action dispatcher: it reads
`AGY_CDE_AUTH_ACTION` and accepts only `check` and `login`
(`Unknown AGY_CDE_AUTH_ACTION: %q (supported: check, login)`), reporting
`Authentication failed: %v`. `check` validates the stored credential without an interactive
flow; `login` runs the full path.

**Logout** is `store.(*Manager).Logout` (`auth_manager.go:287] Logging out current user`) →
`backend.(*ServerBackend).Logout` → `ChainedAuth.Logout` → each method's `Logout`, and the
`logoutCommand` (`commands.(*logoutCommand).Execute`, 0x9638140, name `logout`, description
`Log out`) is the UI. What is cleared, exactly:

| cleared | evidence |
|---|---|
| the whole token record (access, refresh, expiry, id_token, id_token_email, auth_method, tier, project/region) | `RemoveToken` on file **and** keyring; `failed to remove token from both keyring (%v) and file (%v)` |
| cached user tier | `failed to clear user tier: %w`, `Failed to clear user tier in keyring: %v` |
| cached project and region | `failed to clear project and region: %w` |
| OAuth in-flight state | `no OAuth process in progress` from `Cancel`/`completeOAuth` |

Not cleared: the ADC credential (stub `Logout`), the gateway/API-key configuration (stub
`Logout`), `~/.config/antigravity/plugin-secrets.json`
(`auth_store.go:199] Using file-based secret storage at …`), and the local project store
(`default-cli-project`).

**Status surfaces.** `model.AuthModel` renders the account state
(`AuthModel.renderFullView`, `renderSelectionMenu`, `updateWait`, `updateFailure`, with
`No authent…`, `ApplyProjectPermissionGrants: no grants for project …`); the
`AuthModel.getOptions` list is what drives "Select login method:". Server-side state is refreshed
after auth by `server.go:3469] Auth succeeded, refreshing features and managers` and the
experiment refresh pair.

---

## 10. Endpoint / RPC summary

| step | target | method | evidence |
|---|---|---|---|
| authorise | `https://accounts.google.com/o/oauth2/auth` | GET (browser) | `oauth2.Endpoint` struct at 0xbe28b60 (AuthURL 0xbe28b60, DeviceAuthURL 0xbe28b70 = `…/device/code`, TokenURL 0xbe28b80) loaded by `oauthConfigFor` (0x922bc5b) and `savingTokenSource.Token` |
| authorise (enterprise/WIF) | `https://auth.cloud.google/authorize` | GET (browser) | 0x5abd0fb, slot 0xbdc3630, loaded by `oauthConfigFor` (0x922ba4b) and `(*wifOAuthMethod).StartInteractive` (0x922c2f2) |
| token exchange (consumer) | `https://oauth2.googleapis.com/token` | POST | 0x5abf329, slot 0xbe28b80 |
| token exchange (enterprise) | `https://sts.googleapis.com/v1/oauthtoken` | POST | 0x5adfa7f (40 B), slot 0xbdc3640, loaded at 0x922ba63 / 0x922c310 |
| redirect | `https://antigravity.google/oauth-callback` | 302 target | 0x5ae5ff3 (41 B) |
| success page | `https://antigravity.google/auth-success?app=%s` | served by LS callback handler | 0x5b02c86 → `authclient.createAuthCallbackHandler.func1` (0x8cb1976) |
| identity | `https://www.googleapis.com/oauth2/v2/userinfo` | GET | 0x5afd52d, `fetchUserInfo` (0x922893a), `fetchAdcUserInfo` |
| token introspection | `https://www.googleapis.com/oauth2/v3/tokeninfo?access_token=%s` | GET | 0x5b43169, referenced by `[AuthProvider] ResolveAuthMode` |
| entitlement | `https://daily-cloudcode-pa.googleapis.com/v1internal:loadCodeAssist` (observed in logs), `…/v1internal:fetchAvailableModels` | POST | context §Backend hosts |

The `sts.googleapis.com/v1/oauthtoken` string is what the binary contains, byte for byte
(`https://sts.googleapis.com/v1/oauth` + `token` are one 40-byte literal); it is not the
`/v1/token` form that public STS documentation uses. Reproduced as observed, flagged in §11.

---

## 11. Open questions / unresolved

1. **Keyring-marker TTL.** `Using file-based token storage because a keyring timeout was
   recorded within the last %v` prints a duration that is a constant in
   `keyringRecentlyUnavailable` (0x83dfd60) or in `NewCompositeTokenStorage` (0x83df8a0). The
   constant was not recovered; neither function's immediates were decoded to a duration because
   both are BOLT-split and the `%v` argument is materialised several instructions away.
2. **Keyring operation timeout value.** Same gap for the per-operation timeout behind
   `Keyring LoadToken timed out after %v`; the timeout wrapper is a closure
   (`(*compositeTokenStorage).LoadToken.func1`, 0x8413980) and the duration was not recovered.
3. **Enterprise scope/endpoint assignment.** That `oauthConfigFor`'s enterprise branch uses scope
   set B with the `auth.cloud.google`/`sts` endpoint pair is derived from code locality
   (0x922ba4b, 0x922ba96 in one block; 0x922bb66/0x922bba0/0x922bc5b in the other) plus the
   independent confirmation from `(*wifOAuthMethod).StartInteractive`. The branch predicate
   itself (the comparison that selects the enterprise block) was not read.
4. **Which client ID the enterprise/WIF config uses.** `getOauthParams` accepts only `gcp` and
   `consumer`, and the third client ID belongs to MCP. So the WIF path reuses one of the two;
   which one was not determined.
5. **`getScopesByAuthMethod` returning the same 7 scopes for both methods** conflicts with the
   brief's premise that it "returns different scope sets per auth method". The two branches of
   the function load the *same* global (0xbdd2d10, verified by two independent reference scans),
   so the per-method difference is not in this function; the differentiation lives in
   `oauthConfigFor`'s enterprise branch (set B). If a future build does differentiate, the
   function is the place to re-check.
6. **The `a2a6f0e3e-3ff0-462b-869a-47277df4c806`-shaped constant** stored into the config by
   `oauthConfigFor` (0x5acbeec) was not identified: it reads like a UUID with a truncated first
   group, so it may be the tail of a longer literal or a WIF audience/target-audience value.
7. **`externalCorpLoginAuth.Name()`** and `IsSupported()` are 32-byte stubs whose literal was
   elided by the compiler, so the exact method string that appears in `ChainedAuth` logs for the
   corporate path is unknown; the WIF provider string arrives at runtime via
   `SetWIFProvider` (`[AuthProvider] SetSavedWIFProvider called with wifProvider: %q`).
8. **`cliFileTokenStorage.SaveUserTier`/`SaveProjectAndRegion` JSON names** are derived from tag
   vocabulary, not from an observed file (the consumer record has no tier/project). A run that
   authenticates a paid/enterprise account would settle it; not attempted here because the brief
   forbids repeated auth attempts.
9. **Not exercised live.** All network behaviour above is read from the binary and from klog
   lines produced by earlier sessions; no request/response body was captured in this run.
