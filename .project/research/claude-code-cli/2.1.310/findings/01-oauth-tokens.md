---
document:
  type: "research"
  id: "RES-claude-code-cli-2.1.310-findings-01-oauth-tokens"
  title: "Claude Code 2.1.310 OAuth and credential handling"
  description: "Static executable-JavaScript evidence for interactive OAuth, workload federation, credential selection, refresh, and account metadata."
  status: "published"
  revision: 3
  created_at: "2026-10-26T12:18:59+01:00"
  updated_at: "2026-11-03T11:23:00+01:00"
  created_by: "worker"
  updated_by: "orchestrator"
  source: "pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/01-oauth-tokens.md"
  imported_from: "host:/mnt/Projects/furbyhaxx/pi-auth-plus@64be5931be6dcc7adade81ac136e6d02190a786e:.project/analyses/claude-code-cli/binary-re/findings/01-oauth-tokens.md"
  imported_at: "2026-11-03T11:23:00+01:00"
  changelog:
    - revision: 2
      reason: "imported into pi-auth-plus"
      changes: ["copied from pi-quotas@78268296fed62ba14601b8cb80f1ff45a5c04796:.project/research/claude-binary-re/findings/01-oauth-tokens.md"]
    - revision: 3
      reason: "imported into pig-workbench"
      changes: ["copied as-is from .project/analyses/claude-code-cli/binary-re/findings/01-oauth-tokens.md", "type reset to research, id re-homed to RES-, provenance recorded"]
---
# OAuth and tokens — installed Linux Claude Code 2.1.310
Evidence is the supplied carved `/tmp/cc_code.txt`, not upstream source or a live authentication run. **Observed** means literal executable text; **inferred** means the control/data flow of that text; **unknown** means not established. No user credential files were opened and no authentication requests were made. Endpoints/client IDs are public constants, never credential values.
Citations below use `ctx.py` **line-offsets**, i.e. character offsets within individual corpus lines, not binary addresses or offsets within the whole corpus. The pattern/function named with each citation disambiguates reused offsets and minified identifiers. Each snippet line is a contiguous, verbatim substring, trimmed at its boundaries; adjacent lines in a fence may be separate excerpts from the cited context. Different modules reuse short names. In particular the profile SDK's `y()` clock is seconds, while interactive-login `Date.now()` storage is milliseconds.
## 1. Full authorize request
### Shared builder for Console and Claude.ai
**Observed:** `loginWithClaudeAi` changes only the authorization endpoint in this builder. Both flows send `code=true`, `client_id`, `response_type=code`, `redirect_uri`, space-joined `scope`, `code_challenge`, `code_challenge_method=S256`, and `state`. Optional parameters are exactly `orgUUID`, `login_hint`, `login_method`. There is **no `beta` query parameter** in this complete builder. A custom `oauthClient` supplies its own client ID and scopes; inference-only asks only for `user:inference`; `alsoRequestScope` appends to the normal union.
`/tmp/cc_code.txt`, `grant_type:"refresh_token"` hit offset **22292** (preceding `wVr`):
```js
function wVr({codeChallenge:e,state:n,port:r,isManual:s,loginWithClaudeAi:h,inferenceOnly:y,orgUUID:S,loginHint:w,loginMethod:C,oauthClient:x,alsoRequestScope:P}){let D=h?rn().CLAUDE_AI_AUTHORIZE_URL:rn().CONSOLE_AUTHORIZE_URL,F=new URL(D);F.searchParams.append("code","true"),F.searchParams.append("client_id",x?.clientId??rn().CLIENT_ID),F.searchParams.append("response_type","code"),F.searchParams.append("redirect_uri",s?rn().MANUAL_REDIRECT_URL:`http://localhost:${r}/callback`);let K=x?x.scopes:y?[fk]:P?[...g9r(),P]:g9r();if(F.searchParams.append("scope",K.join(" ")),F.searchParams.append("code_challenge",e),F.searchParams.append("code_challenge_method","S256"),F.searchParams.append("state",n),S)F.searchParams.append("orgUUID",S);if(w)F.searchParams.append("login_hint",w);if(C)F.searchParams.append("login_method",C);return F.toString()}
```
**Observed:** normal scopes are the unique union of `org:create_api_key`, `user:profile`, `user:inference`, `user:sessions:claude_code`, `user:mcp_servers`, `user:file_upload`, plus `user:plugins` when registered (true in production). Project scopes are not automatically in `g9r()`; `aes()` preserves already-granted project scopes during refresh. Design scopes belong to a separate client override, not the normal default.
`/tmp/cc_code.txt`, `function GAt` hit offset **728**:
```js
fk="user:inference",_2="user:profile",Ukn="user:sessions:claude_code",Wlr=[_2,Ukn],WAt="user:plugins",Glr="user:projects:read",zlr="user:projects:write";function Klr(t){return Array.isArray(t)&&t.includes(fk)}var c="org:create_api_key",Bu="oauth-2025-04-20",n=[c,_2],r=[_2,fk,Ukn,"user:mcp_servers","user:file_upload"];function GAt(){let t=rn(),e=[...r];if(t.PLUGINS_SCOPE_REGISTERED)e.push(WAt);return e}function g9r(){return L([...n,...GAt()])}var K$e=["user:design:read","user:design:write"],S=[Glr,zlr];function h9r(t){if(!Array.isArray(t))return[];let e=S;return t.filter((o)=>e.includes(o))}function aes(t){return L([...GAt(),...h9r(t)])}
```
### PKCE, callback, manual path
**Observed:** verifier and state each use 32 random bytes, base64url without padding. Challenge is SHA-256 of the verifier. The verifier lives in the `E$` flow object's `codeVerifier` property, initialized in its constructor; this path does not persist it to disk. State is a local variable per `startOAuthFlow` invocation.
`/tmp/cc_code.txt`, `codeVerifier=` hit offset **5271**:
```js
function A(e){return e.toString("base64").replaceAll("+","-").replaceAll("/","_").replaceAll("=","")}function L(){return A(k(32))}function O(e){let r=T("sha256");return r.update(e),A(r.digest())}function U(){return A(k(32))}
class E${codeVerifier;authCodeListener=null;manualAuthCodeResolver=null;authorizationCodeReceived=!1;constructor(){this.codeVerifier=L()}
let o=O(this.codeVerifier),n=U(),s={codeChallenge:o,state:n,port:i??0,loginWithClaudeAi:r?.loginWithClaudeAi,inferenceOnly:r?.inferenceOnly,orgUUID:r?.orgUUID,loginHint:r?.loginHint,loginMethod:r?.loginMethod,oauthClient:r?.oauthClient,alsoRequestScope:r?.settingsOnlyGrant?.scope},v=wVr({...s,isManual:!0}),C=i===null?void 0:wVr({...s,isManual:!1})
```
**Observed/inferred:** server binds `127.0.0.1` on an OS-assigned port (`0` default), but redirect URL spells `localhost`. Manual redirect is the configured `MANUAL_REDIRECT_URL` (production `https://platform.claude.com/oauth/code/callback`), **not** another localhost URL. The UI gets a manual URL even when the browser is opened to the local-callback URL. Failed listener startup can fall back to manual when a caller provided `onCallbackListenerUnavailable`, except `EMFILE`/`ENFILE`, which throw. The exchange chooses local/manual based on whether the listener actually has a pending callback response, not merely whether a server started.
`/tmp/cc_code.txt`, `expectedState` hits **666**, **1329**:
```js
constructor(e="/callback"){this.localServer=I(),this.callbackPath=e}async start(e){return new Promise((r,i)=>{this.localServer.once("error",(o)=>{m("oauth_callback_listener","oauth_callback_server_start_failed"),i(Error(`Failed to start OAuth callback server: ${o.message}`,{cause:o}))}),this.localServer.listen(e??0,"127.0.0.1",()=>{let o=this.localServer.address();this.port=o.port,r(this.port)})})}
```
`/tmp/cc_code.txt`, `codeVerifier=` hit **5271**:
```js
catch(c){this.authCodeListener=null,t(`OAuth callback listener could not start: ${d(c)}`,{level:"error"});let u=c instanceof Error?E(c.cause):void 0;if(!r?.onCallbackListenerUnavailable||u==="EMFILE"||u==="ENFILE")throw new c7n(c);r.onCallbackListenerUnavailable()}
if(C===void 0)await e(v);else if(r?.skipBrowserOpen)await e(v,C);else await e(v),await Js(C)
let c=await Tqo(y,{state:n,codeVerifier:this.codeVerifier,port:s.port,useManualRedirect:!h,expiresIn:r?.expiresIn,clientId:r?.oauthClient?.clientId})
```
**Observed:** automatic callback validates state and rejects missing code. State mismatch returns HTTP 400 and rejects the promise. Manual UI expects `code#state`, but the shared `handleManualAuthCodeInput` ignores its input `state`; the token exchange receives the original generated state. Thus do not claim every manual caller locally compares state. A separate cloud-sessions terminal helper does compare it.
`/tmp/cc_code.txt`, `expectedState` hit **2665**:
```js
validateAndRespond(e,r,i){if(r===this.expectedState&&this.answerIfCodeReceived(i))return;if(!e){m("oauth_callback_listener","oauth_callback_no_code"),i.writeHead(400),i.end("Authorization code not found"),this.reject(Error("No authorization code received"));return}if(this.rejectOnStateMismatch(r,i))return;this.codeReceived=!0,this.pendingResponse=i,g("oauth_callback_listener"),this.resolve(e)}
rejectOnStateMismatch(e,r){if(e===this.expectedState)return!1;return m("oauth_callback_listener","oauth_callback_state_mismatch"),r.writeHead(400),r.end("Invalid state parameter"),this.reject(Error("Invalid state parameter")),!0}
```
`/tmp/cc_code.txt`, `authorizationCode:` hit **22312**; `handleManualAuthCode` hit **7675**:
```js
let[Pe,le]=ae.split("#");if(!Pe||!le){_({state:"error",message:"Invalid code. Please make sure the full code was copied",toRetry:{state:"waiting_for_login",url:Ee}});return}a("tengu_oauth_manual_entry",{}),v.handleManualAuthCodeInput({authorizationCode:Pe,state:le})
handleManualAuthCodeInput(e){if(this.manualAuthCodeResolver)this.authorizationCodeReceived=!0,this.manualAuthCodeResolver(e.authorizationCode),this.manualAuthCodeResolver=null,this.authCodeListener?.close()}
```
`/tmp/cc_code.txt`, `p!==s` hit **36** (cloud-sessions helper; error-message text alone is not the evidence—the comparison is):
```js
if(s!==void 0&&p!==s){e.output.write(`That code is not from this sign-in. Paste the code shown after opening the address above.
```
## 2. Token exchange and refresh
### Interactive authorization-code and refresh grants
**Observed:** both are Axios `POST` to configured `TOKEN_URL` (production `https://platform.claude.com/v1/oauth/token`), JSON objects with explicit `Content-Type: application/json`, 30-second timeout. Authorization-code body has `grant_type`, `code`, `redirect_uri`, `client_id`, `code_verifier`, **state**, optional `expires_in`. It sends **no scope field**. Refresh body has `grant_type`, `refresh_token`, `client_id`, space-joined `scope`, optional `expires_in`; it also accepts an abort signal. Neither call specifies `Authorization`, `anthropic-beta`, or `User-Agent` locally; any transport-global additions are not established by these call sites.
`/tmp/cc_code.txt`, `grant_type:"refresh_token"` hit **22292**:
```js
async function Tqo(e,{state:n,codeVerifier:r,port:s,useManualRedirect:h=!1,expiresIn:y,clientId:S}){let w={grant_type:"authorization_code",code:e,redirect_uri:h?rn().MANUAL_REDIRECT_URL:`http://localhost:${s}/callback`,client_id:S??rn().CLIENT_ID,code_verifier:r,state:n};if(y!==void 0)w.expires_in=y;let C=await Ht.post(rn().TOKEN_URL,w,{headers:{"Content-Type":"application/json"},timeout:30000});if(C.status!==200)throw m("oauth_token_exchange",C.status===401?"oauth_exchange_invalid_code":"oauth_exchange_http_error"),Error(C.status===401?"Authentication failed: Invalid authorization code":`Token exchange failed (${C.status}): ${C.statusText}`);return a("tengu_oauth_token_exchange_success",{}),g("oauth_token_exchange"),C.data}
let x=Array.isArray(n)&&n.length?n:GAt(),P={grant_type:"refresh_token",refresh_token:e,client_id:s??rn().CLIENT_ID,scope:x.join(" ")};if(r!==void 0)P.expires_in=r;
let D=await Ht.post(rn().TOKEN_URL,P,{headers:{"Content-Type":"application/json"},timeout:30000,signal:y});if(D.status!==200)throw Error(`Token refresh failed: ${D.statusText}`);let F=D.data,{access_token:K,refresh_token:j=e,expires_in:Z}=F,V=Date.now()+Z*1000,Q=SVr(F.refresh_token_expires_in,!1),le=Ctr(F.scope)
```
**Observed/inferred:** ordinary first-party refresh asks `aes(storedScopes)` (base inference/profile/session/MCP/file/plugin scopes plus already-granted project scopes), whereas custom-client refresh retains its own scopes. An `invalid_scope` response on the normal inference client gets one fallback request with the original stored scopes (see §5). It does not blindly copy the authorize union containing `org:create_api_key`.
**Observed:** access expiry is response `expires_in * 1000 + Date.now()`. Refresh-token expiry uses explicit `refresh_token_expires_in`; initial sign-in alone defaults a missing value to 30 days. Refresh itself returns `undefined` when missing, and storage merge preserves the earlier refresh expiry. Scope is parsed by splitting spaces. Authorization exchange consumes account UUID/email, organization UUID/name and workspace ID/name; refresh consumes account UUID/email and organization UUID. Missing rotated refresh token falls back to the posted token.
`/tmp/cc_code.txt`, `formatTokens(e` hit **7882**; `grant_type:"refresh_token"` hit **22292** (preceding constants):
```js
formatTokens(e,{subscriptionType:r,unconfirmedPlan:i,rateLimitTier:o,profile:n,clientId:s}){return{accessToken:e.access_token,refreshToken:e.refresh_token,expiresAt:Date.now()+e.expires_in*1000,refreshTokenExpiresAt:SVr(e.refresh_token_expires_in,!0),scopes:Ctr(e.scope),subscriptionType:r,...r!==null&&{subscriptionTypeSource:i?"profile":"sign_in"},rateLimitTier:o,profile:n,clientId:s,tokenAccount:e.account?{uuid:e.account.uuid,emailAddress:e.account.email_address,organizationUuid:e.organization?.uuid,organizationName:e.organization?.name,workspaceId:e.workspace?.id,workspaceName:e.workspace?.name}:void 0}}
var Gb=86400000,lF=2592000000;function SVr(e,n){if(typeof e==="number")return Date.now()+e*1000;return n?Date.now()+lF:void 0}
```
`/tmp/cc_code.txt`, `ROLES_URL,` hit **25315**, following refresh result:
```js
tokenAccount:F.account?{uuid:F.account.uuid,emailAddress:F.account.email_address,organizationUuid:F.organization?.uuid}:void 0
```
### Separate SDK/profile subsystem: workload identity federation and user OAuth
**Observed/inferred:** this is a real second credential provider, not an alternative interactive authorize URL. `oidc_federation` exchanges an external identity assertion for an Anthropic bearer scoped to a federation rule/organization, optionally service account/workspace. It has no browser, PKCE, interactive client ID or refresh token in its request/result. Enterprise/workload identity is the intended context: the executable calls it `oidc_federation`, and its 401 diagnostic explicitly refers to the Console **Workload identity** page. The same provider framework also supports **`user_oauth`** credentials refreshed from a different snake_case profile file; that branch is a user path, not workload identity.
`/tmp/cc_code.txt`, `Sci=` hit **2836**; `grant_type:Sci` hit **8702**:
```js
var Sci="urn:ietf:params:oauth:grant-type:jwt-bearer",wci="refresh_token",B3r="/v1/oauth/token",uAt="oauth-2025-04-20",vci="oidc-federation-2026-04-01",Eci=120,fAt=30,kci=5,C=1048576;
j3r(e.baseURL);let n=await e.identityTokenProvider();if(n.length>16384)throw new Cf(`Identity token is ${Math.ceil(n.length/1024)} KiB, exceeds the 16 KiB assertion limit`);let r={grant_type:Sci,assertion:n,federation_rule_id:e.federationRuleId,organization_id:e.organizationId};if(e.serviceAccountId)r.service_account_id=e.serviceAccountId;if(e.workspaceId)r.workspace_id=e.workspaceId;let s=`${e.baseURL}${B3r}`,o;try{o=await e.fetch(s,{method:"POST",headers:{"Content-Type":"application/json","anthropic-beta":`${uAt},${vci}`,"User-Agent":e.userAgent||`anthropic-sdk-typescript/${iR} oidcFederationProvider`},body:JSON.stringify(r)})}
let f=await W3r(o,u),h=Number(f.expires_in);if(!Number.isFinite(h))throw new Cf(`Token endpoint response missing required fields: ${JSON.stringify(Rfe(f))}`,o.status,Rfe(f),u);return{token:f.access_token,expiresAt:y()+h}
```
**Observed:** endpoint is `${baseURL}/v1/oauth/token`, JSON POST. Headers include **both** `oauth-2025-04-20,oidc-federation-2026-04-01` in one `anthropic-beta` value. The shown grant sends no `scope`, even though profile/env config accepts scope. `baseURL` is selected from profile `base_url` or supplied resolver defaults, trailing slashes removed. Do not assume this is the interactive `platform.claude.com` token host.
`/tmp/cc_code.txt`, `Z7s(` hit **12084**:
```js
function fzt(e,n){let r=e.authentication.credentials_path??null,s=(e.base_url||n.baseURL).replace(/\/+$/,""),o=se(e,r,s,n),u={};if(e.workspace_id&&e.authentication.type==="user_oauth")u["anthropic-workspace-id"]=e.workspace_id;return{provider:o,extraHeaders:u,baseURL:e.base_url||void 0}}
```
**Observed:** federation-rule and organization IDs select the env-quad configuration; workspace/service-account IDs are optional. Identity precedence is explicit profile `authentication.identity_token` (only `{source:"file",path}` supported), then `ANTHROPIC_IDENTITY_TOKEN_FILE`, then `ANTHROPIC_IDENTITY_TOKEN`. Files are reread and trimmed by the provider. Profile fields take precedence over environment defaults. `ANTHROPIC_SCOPE` is accepted into config but is absent from the observed grant body.
`/tmp/cc_code.txt`, `ANTHROPIC_IDENTITY_TOKEN"` hits **14034**, **22010**:
```js
function ae(e){if(e.identity_token){let s=e.identity_token.source;if(s!=="file")throw new Cf(`identity_token.source "${s}" is not supported by this SDK version (only "file")`);if(!e.identity_token.path)throw new Cf('identity_token.source "file" requires a non-empty path');return P(e.identity_token.path)}let n=mb("ANTHROPIC_IDENTITY_TOKEN_FILE");if(n)return P(n);let r=mb("ANTHROPIC_IDENTITY_TOKEN");if(r)return j(r);return null}
return{organization_id:n,workspace_id:I("ANTHROPIC_WORKSPACE_ID"),base_url:I("ANTHROPIC_BASE_URL"),authentication:{type:"oidc_federation",federation_rule_id:e,service_account_id:I("ANTHROPIC_SERVICE_ACCOUNT_ID"),identity_token:r?{source:"file",path:r}:void 0,scope:I("ANTHROPIC_SCOPE")}}
```
**Observed:** `ANTHROPIC_ENVIRONMENT_ID`, `ANTHROPIC_ENVIRONMENT_KEY`, `ANTHROPIC_WORK_ID`, `ANTHROPIC_WORK_SECRET` feed **EnvironmentWorker work handling**, not this exchange body. Sessions auth uses a token extracted from a work secret, otherwise the environment key. `ANTHROPIC_TIER_NAMES` is an exported model-tier name array, **not an environment variable read** in the located code. These names' proximity in the binary must not be treated as proof of federation inputs.
`/tmp/cc_code.txt`, `ANTHROPIC_WORK_SECRET` hit **28176**:
```js
async handleItem(n){let r=n?.workId??mb("ANTHROPIC_WORK_ID"),s=n?.environmentId??mb("ANTHROPIC_ENVIRONMENT_ID"),h=n?.sessionId??mb("ANTHROPIC_SESSION_ID"),y=(n?.environmentKey??this.environmentKey??mb("ANTHROPIC_ENVIRONMENT_KEY"))||void 0,w=n?.workSecret||mb("ANTHROPIC_WORK_SECRET")||null;
ji=async function(r,s,h){let y=H_(this.client),w=ih(r.secret),S=w??s;
```
`/tmp/cc_code.txt`, `ANTHROPIC_TIER_NAMES` hit **40205** (array shown only to identify the symbol, not as current model guidance):
```js
rE.ANTHROPIC_TIER_NAMES=["sonnet","opus","haiku","fable","mythos"];
```
**Observed:** 120 = advisory refresh threshold in **seconds**; 30 = blocking refresh threshold and disk-cache expiry margin in seconds; 5 = advisory-failure cooldown in seconds, **not a retry count**; 1,048,576 = token-response reader byte cap, **not assertion size**. Assertion cap is separately 16,384 JS string units. Ordinary `getToken` shares `pendingRefresh`; explicit force can start another refresh. No fixed network retry count or request timeout is set by the shown federation grant function; transport policy must be checked separately before claiming one.
`/tmp/cc_code.txt`, `async getToken()` hit **3682**:
```js
function y(){return Math.floor(Date.now()/1000)}class uzt{constructor(e,n){this.cached=null,this.pendingRefresh=null,this.nextForce=!1,this.lastAdvisoryError=0,this.provider=e,this.onAdvisoryRefreshError=n}async getToken(){let e=this.nextForce;this.nextForce=!1;let n=this.cached;if(e||n==null)return(await this.refresh(e)).token;if(n.expiresAt==null)return n.token;let r=n.expiresAt-y();if(r>Eci)return n.token;if(r>fAt)return this.backgroundRefresh(),n.token;return(await this.refresh()).token}invalidate(){this.cached=null,this.nextForce=!0}refresh(e=!1){if(this.pendingRefresh&&!e)return this.pendingRefresh;return this.doRefresh(e)}backgroundRefresh(){if(this.pendingRefresh)return;if(y()-this.lastAdvisoryError<kci)return;this.doRefresh().catch((e)=>{this.lastAdvisoryError=y(),this.onAdvisoryRefreshError?.(e)})}
```
`/tmp/cc_code.txt`, `Sci=` hit **2836**, subsequent `re` reader:
```js
if(s+l.length>C){let h=C-s;if(h>0)r.push(l.subarray(0,h));await n.cancel();break}r.push(l),s+=l.length
```
**Observed:** TLS gate permits HTTPS or HTTP only for exactly `localhost`, `127.0.0.1`, `::1`. `W3r` parses capped JSON, requires access token, accepts absent token_type or case-insensitive bearer, rejects other token types; provider additionally requires finite numeric expires_in. These helpers throw contextual `Cf` with status/body/request ID. Error-body redaction retains only error/error_description/error_uri in objects.
`/tmp/cc_code.txt`, `function j3r` hit **3015**:
```js
function j3r(e){if(!e)return;let n;try{n=new URL(e)}catch(s){throw new Cf(`Invalid token endpoint base URL "${e}": ${s}`)}if(n.protocol==="https:")return;let r=n.hostname.toLowerCase().replace(/^\[|\]$/g,"");if(n.protocol==="http:"&&(r==="localhost"||r==="127.0.0.1"||r==="::1"))return;throw new Cf(`Refusing to send credential over non-https token endpoint "${e}"`)}
async function W3r(e,n){let r=await re(e),s;try{s=JSON.parse(r)}catch{throw new Cf(`Token endpoint returned non-JSON response (status ${e.status})`,e.status,Rfe(r),n)}if(!s.access_token)throw new Cf(`Token endpoint response missing access_token: ${JSON.stringify(Rfe(s))}`,e.status,Rfe(s),n);if(s.token_type&&s.token_type.toLowerCase()!=="bearer")throw new Cf(`Token endpoint response: unsupported token_type "${s.token_type}" (want Bearer)`,e.status,Rfe(s),n);return s}
```
**Observed:** profile user_oauth refresh uses the same baseURL token endpoint but JSON `{grant_type:"refresh_token",refresh_token,client_id}` with `anthropic-beta: oauth-2025-04-20`, no scope/PKCE/state. It reads snake_case `access_token`, `expires_at` (**seconds**), `refresh_token`; refresh requires configured client ID and refresh token. Persisted object retains unknown fields and writes version `1.0`, type `oauth_token`, rotated access/refresh tokens and expires_at atomically.
`/tmp/cc_code.txt`, `grant_type:wci` hit **10861**:
```js
let f=o.expires_at;if(!n?.forceRefresh&&(f==null||y()<f-fAt))return{token:u,expiresAt:f??null};let h=o.refresh_token;if(!e.clientId||!h)throw new Cf(`Access token at ${e.credentialsPath} has expired and no refresh is available (client_id ${e.clientId?"set":"empty"}, refresh_token ${h?"set":"empty"})`);j3r(e.baseURL);let _={grant_type:wci,refresh_token:h,client_id:e.clientId},p=`${e.baseURL}${B3r}`,w;try{w=await e.fetch(p,{method:"POST",headers:{"Content-Type":"application/json","anthropic-beta":uAt,"User-Agent":e.userAgent||`anthropic-sdk-typescript/${iR} userOAuthProvider`},body:JSON.stringify(_)})}
let D=y()+T,ee=k.refresh_token||h;return await A4e(e.credentialsPath,{...o,version:Ghn,type:"oauth_token",access_token:k.access_token,expires_at:D,refresh_token:ee}),{token:k.access_token,expiresAt:D}
```
## 3. Credential storage
### Interactive store and account config are distinct
**Observed:** this **Linux build** resolves `Kn()` to the plaintext backend (`WVt()` says `file`). It reads/writes `<O0()>/.credentials.json`; writes use decimal mode 384 = **0600**, and chmod again. `O0()` uses `CLAUDE_SECURESTORAGE_CONFIG_DIR` when defined (empty means homedir/.claude), otherwise `Le()` (normal Claude config directory). Default `~/.claude/.credentials.json` is also corroborated by the session-copy code below. No Linux secret-service selection is present in the inspected backend; do not infer GNOME Keyring from the term “secureStorage”.
`/tmp/cc_code.txt`, `function O0(){` hit **318**; `credentials.json` hit **2443**; `function Kn(){` hit **6534**:
```js
function O0(){let e=process.env.CLAUDE_SECURESTORAGE_CONFIG_DIR;if(e!==void 0)return(e||u(o(),".claude")).normalize("NFC");return Le()}
function g(){let e=O0(),r=".credentials.json";return{storageDir:e,storagePath:Z(e,".credentials.json")}}
async write(e){try{let{storageDir:r,storagePath:n}=g();return await ie().mkdir(r),await Dn(n,_(e),384),await q(n,384),{success:!0,warning:g5r}}catch{return{success:!1}}finally{if(z())CNe()}}
function WVt(){return"file"}function Kn(){if(k)return k;return w}
```
`/tmp/cc_code.txt`, `CLAUDE_CONFIG_DIR??` hit **5042**:
```js
let b=n?.CLAUDE_CONFIG_DIR??process.env.CLAUDE_CONFIG_DIR,C=b??A(lt(),".claude"),T;try{T=await Sr(A(C,".credentials.json"),"utf-8")}
```
**Observed:** top-level `claudeAiOauth` contains camelCase `accessToken`, `refreshToken`, `expiresAt`, `refreshTokenExpiresAt`, `scopes`, subscription type/source/verified time, `rateLimitTier`, optional `clientId`; JSON serialization omits undefined fields. Initial normal persistence deliberately skips non-inference scopes and credentials without refresh token or expiresAt. Refresh save CAS avoids overwriting a sibling's rotation. Profile/account metadata is not the complete raw profile inside this token object.
`/tmp/cc_code.txt`, `claudeAiOauth` hit **3197**; `primaryApiKey:e` hit **552**, subsequent `Vw`:
```js
let S={accessToken:r.accessToken,refreshToken:r.refreshToken,expiresAt:r.expiresAt,refreshTokenExpiresAt:r.refreshTokenExpiresAt,scopes:r.scopes,subscriptionType:r.subscriptionType,subscriptionTypeSource:r.subscriptionTypeSource,subscriptionTypeVerifiedAt:r.subscriptionTypeVerifiedAt,rateLimitTier:r.rateLimitTier,clientId:r.clientId}
let j=K.claudeAiOauth?.refreshToken;if(!(K.claudeAiOauth!==void 0&&K.claudeAiOauth!==null&&(j===""||j===n)))return w=!0,K;return{...K,claudeAiOauth:Vw(K.claudeAiOauth,S)}
function Vw(e,n,r="refresh"){return{accessToken:n.accessToken,refreshToken:n.refreshToken,expiresAt:n.expiresAt,refreshTokenExpiresAt:n.refreshTokenExpiresAt??e?.refreshTokenExpiresAt,scopes:n.scopes,...r==="sign_in"?i$(n):a$(e,n),rateLimitTier:n.rateLimitTier??(r==="sign_in"?null:e?.rateLimitTier??null),clientId:n.clientId}}
```
`/tmp/cc_code.txt`, `claudeAiOauth` hit **4704**:
```js
async function Aqr(e,n,r="refresh"){if(!VU(e.scopes))return a("tengu_oauth_tokens_not_claude_ai",{}),{success:!0};if(!e.refreshToken||!e.expiresAt)return a("tengu_oauth_tokens_inference_only",{}),{success:!0};
```
**Correction to the proposed filename premise:** the observed credentials path is literally `.credentials.json` without an OAuth suffix. `-staging-oauth`, `-local-oauth`, `-custom-oauth` apply to **`.claude${y2()}.json` account/global config** and **Keychain service names**, not the shown plaintext credential filename. Legacy `<Le()>/.config.json` takes precedence when it qualifies as an existing legacy config. Build's `E()` returns prod; `y2()` still recognizes custom URL and contains local/staging cases.
`/tmp/cc_code.txt`, `CLAUDE_CONFIG_DIR||` hit **48322**:
```js
function y_(){if(J())return Flr();let t=l(Le(),".config.json"),o=Flr();try{let _=ie().statSync(t);if(_.isFile()&&(_.size>0||(_.mode&146)!==0&&!ie().existsSync(o)))return t}catch{}return o}function Flr(){let t=`.claude${y2()}.json`;return l(process.env.CLAUDE_CONFIG_DIR||Y_(),t)}
```
`/tmp/cc_code.txt`, `function GAt` hit **728**, preceding variant selector:
```js
function E(){return"prod"}var wSe=["","-staging-oauth","-local-oauth","-custom-oauth"];function y2(){if(process.env.CLAUDE_CODE_CUSTOM_OAUTH_URL)return"-custom-oauth";switch(E()){case"local":return"-local-oauth";case"staging":return"-staging-oauth";case"prod":return""}}
```
**Observed:** macOS-related service name construction is `Claude Code${OAUTH_FILE_SUFFIX}${e}${optional-config-path-hash}`; `dsr` is `-credentials`; account uses sanitized `$USER`/OS username with fallback. The executable contains `security find-generic-password` and `add-generic-password` calls. However this Linux build's API-key writer has `let s=!1;if(s)` around the Keychain write, and `Kn()` resolves plaintext. This does **not** establish macOS fallback precedence or Windows Credential Manager behavior. Windows gate names are observed, but backend implementation/selection remains unknown in this artifact; platform binaries are needed. No claim is made that credentials from Linux are encrypted or that all OSes use the same backend.
`/tmp/cc_code.txt`, `function O0(){` hit **318**:
```js
var dsr="-credentials";
function wbe(e=""){let n=process.env.CLAUDE_SECURESTORAGE_CONFIG_DIR,t=n!==void 0?!n:!process.env.CLAUDE_CONFIG_DIR,r=n!==void 0?n.normalize("NFC"):Le(),a=t?"":`-${c("sha256").update(r).digest("hex").substring(0,8)}`;return`Claude Code${rn().OAUTH_FILE_SUFFIX}${e}${a}`}
```
`/tmp/cc_code.txt`, `find-generic-password` hit **2149**:
```js
it("security",["find-generic-password","-a",JV(),"-w","-s",wbe()],{useCwd:!1,preserveOutputOnError:!1,timeout:D})
```
`/tmp/cc_code.txt`, `async function ori` hit **24597**:
```js
let s=!1;if(s){let y=wbe(),S=JV(),w=Buffer.from(e,"utf-8").toString("hex"),C=`add-generic-password -U -a "${S}" -s "${y}" -X "${w}"
```
`/tmp/cc_code.txt`, `function Kn(){` hit **6534**:
```js
function o8o(){if(process.env.CLAUDE_CODE_FORCE_WINDOWS_CREDMAN==="1")return!0;return L().resolve(F,le)}
```
**Observed:** `oauthAccount` is a separate global config record: UUID/email/org UUID, display/full names, extra usage flag, billing type, creation/subscription dates, onboarding flags, trial end/duration, seat tier, plan display name, profile fetch timestamp. Roles fetch adds organizationRole/workspaceRole/organizationName (§6). The token endpoint also supplies tokenAccount metadata (§2); it must not be confused with decoded JWT claims.
`/tmp/cc_code.txt`, `rate_limit_tier` hit **31123**, `t_n`:
```js
let V={accountUuid:e,emailAddress:n,organizationUuid:r,hasExtraUsageEnabled:y,billingType:S,accountCreatedAt:w,subscriptionCreatedAt:C,ccOnboardingFlags:x,claudeCodeTrialEndsAt:P,claudeCodeTrialDurationDays:D,seatTier:F,planDisplayName:K??void 0};if(s)V.displayName=s;if(h)V.fullName=h;if(j!==void 0)V.profileFetchedAt=j;
```
### Profile/federation storage is a different layout
**Observed:** config root is `ANTHROPIC_CONFIG_DIR`, else Windows `%APPDATA%/Anthropic` or `%USERPROFILE%/AppData/Roaming/Anthropic`, else `$XDG_CONFIG_HOME/anthropic` or `$HOME/.config/anthropic`. `ANTHROPIC_PROFILE` overrides `active_config`; absent file defaults to `default`. Files are `configs/<profile>.json` and `credentials/<profile>.json`, unless authentication.credentials_path overrides. Unlike interactive storage, expiry is seconds and fields snake_case (§2). Profile credential files reject group/world read or write permissions, warn for different UID, and writes use 0700 directory/0600 temp file, sync then rename. Federation cache stores no refresh token: it repeats the assertion exchange.
`/tmp/cc_code.txt`, `grant_type:Sci` hit **8702**, preceding resolvers:
```js
return o.join(r,"credentials",`${s}.json`)
if(RZn()["X-Stainless-OS"]==="Windows"){let u=mb("APPDATA");if(u)return e.join(u,"Anthropic");let f=mb("USERPROFILE");if(f)return e.join(f,"AppData","Roaming","Anthropic");return null}let s=mb("XDG_CONFIG_HOME");if(s)return e.join(s,"anthropic");let o=mb("HOME");if(o)return e.join(o,".config","anthropic");return null
```
`/tmp/cc_code.txt`, `Sci=` hit **2836**, subsequent `hEn`/`A4e`:
```js
let a=c.mode&511;if(a&18)throw new Cf(`Credentials file at ${s} is group/world-writable (mode 0o${a.toString(8)}); this allows other local users to plant tokens. Run \`chmod 600 ${s}\`.`);if(a&36)throw new Cf(`Credentials file at ${s} is group/world-readable (mode 0o${a.toString(8)}); run \`chmod 600 ${s}\` before retrying.`);
await r.promises.mkdir(c,{recursive:!0,mode:448});let a=`${e}.${process.pid}.${Math.random().toString(36).slice(2)}.tmp`;try{let l=await r.promises.open(a,"w",384);try{await l.writeFile(JSON.stringify(n,null,2)),await l.sync()}finally{await l.close()}await r.promises.rename(a,e)}
```
`/tmp/cc_code.txt`, `ANTHROPIC_IDENTITY_TOKEN"` hit **14034**, `ce` cache write:
```js
await A4e(n,{...f??{},version:Ghn,type:"oauth_token",access_token:h.token,expires_at:h.expiresAt})
```
**Observed:** CLI federation cache directory is `CLAUDE_CODE_FEDERATION_CACHE_DIR` or `<Anthropic config root>/credentials/federation`. Cache identity includes federation rule, org, workspace, service account, scope, base URL and digest of identity token. Directory must be private and owned by current UID; otherwise cache is disabled. This is unrelated to `.claude/.credentials.json`.
`/tmp/cc_code.txt`, `function yrt` hit **1565**; `ANTHROPIC_IDENTITY_TOKEN"` hit **22010**:
```js
function yrt(){if(J())return null;let e=i.CLAUDE_CODE_FEDERATION_CACHE_DIR;if(e)return x(e);let n=MH();return n===null?null:x(n,"credentials","federation")}
let u=et(JSON.stringify([e.authentication.federation_rule_id,e.organization_id,e.workspace_id??"",e.authentication.service_account_id??"",e.authentication.scope??"",n,et(s)]));return _e(o,`${u}.json`)
```
## 4. Token kinds and precedence
### Selection is contextual, not one unconditional ordering
**Observed/inferred:** first choose provider regime: restored/configured gateway, then Bedrock, Foundry, Anthropic AWS, Anthropic Google Cloud, Mantle, Vertex, else first party. Non-first-party disables the ordinary Claude OAuth eligibility path. These flags select separate SDK auth mechanisms, not subscription tokens to be sent to Anthropic's quota endpoint.
`/tmp/cc_code.txt`, `function Fn(){` hit **31524**:
```js
function Ae(){if(Wr()||Rit()||Pit())return"gateway";return i.CLAUDE_CODE_USE_BEDROCK?"bedrock":i.CLAUDE_CODE_USE_FOUNDRY?"foundry":i.CLAUDE_CODE_USE_ANTHROPIC_AWS?"anthropicAws":i.CLAUDE_CODE_USE_ANTHROPIC_GOOGLE_CLOUD?"anthropicGoogleCloud":i.CLAUDE_CODE_USE_MANTLE?"mantle":i.CLAUDE_CODE_USE_VERTEX?"vertex":"firstParty"}
```
**Observed:** `tl()` OAuth eligibility is false for bare mode `Ej()`, selected profile auth `kc()`, non-first-party provider, effective auth bearer/API key/helper unless suppressed by pinned-login/host rules. Unix socket special case requires `CLAUDE_CODE_OAUTH_TOKEN`. `Et()` additionally requires an inference scope. `ANTHROPIC_AUTH_TOKEN` ordinarily reports first in `ua()` and disables stored-login OAuth; explicit injected OAuth then FD/CCR token precede helper/profile/stored login in that source-reporting function. Source reporting is not by itself proof of every request's headers.
`/tmp/cc_code.txt`, `function tl(){` hit **357**:
```js
function tl(){if(Ej())return!1;if(i.ANTHROPIC_UNIX_SOCKET)return!!i.CLAUDE_CODE_OAUTH_TOKEN;if(kc())return!1;let e=!Fn(),n=pr()||{},r=wr()?Ng():TVe("apiKeyHelper",n.apiKeyHelper),s=U_(),h;try{h=nA({skipRetrievingKeyFromApiKeyHelper:!0}).source}catch{return!1}let y=h==="ANTHROPIC_API_KEY"||h==="apiKeyHelper",S=NZ("CLAUDE_CODE_API_KEY_FILE_DESCRIPTOR"),w=(s||y)&&!j_e()||(r||S)&&!xr();return!(e||w)}
function ua(){if(Ej()){if(Ng())return{source:"apiKeyHelper",hasToken:!0};return{source:"none",hasToken:!1}}if(U_()&&!j_e())return{source:"ANTHROPIC_AUTH_TOKEN",hasToken:!0};if(i.CLAUDE_CODE_OAUTH_TOKEN||mM()!==null)return{source:"CLAUDE_CODE_OAUTH_TOKEN",hasToken:!0};if(dE()){if(NZ("CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR"))return{source:"CLAUDE_CODE_OAUTH_TOKEN_FILE_DESCRIPTOR",hasToken:!0};return{source:"CCR_OAUTH_TOKEN_FILE",hasToken:!0}}if(Ng()&&!xr())return{source:"apiKeyHelper",hasToken:!0};if(kc())return{source:"profile",hasToken:!0};let r=Ln();if(VU(r?.scopes)&&r?.accessToken)return{source:"claude.ai",hasToken:!0};return{source:"none",hasToken:!1}}
```
**Observed:** within OAuth-token loading: explicit `CLAUDE_CODE_OAUTH_TOKEN` wins, then FD token when not pinned to store (or in `pd()` context), then stored claudeAiOauth, then FD fallback. Injected tokens carry no refresh token/expiry; default inferred scope is only user:inference, optionally overridden by `CLAUDE_CODE_OAUTH_SCOPES`; subscription type/tier can be supplied by environment. Thus an injected bearer must not be assumed refreshable or profile-scoped.
`/tmp/cc_code.txt`, `function Ln(){` hit **9399**:
```js
function Qw(e=["user:inference"]){return Zw()??[...e]}function Zw(){let e=i.CLAUDE_CODE_OAUTH_SCOPES?.split(/\s+/).filter(Boolean);return e?.length?e:void 0}
function bp(e){return{accessToken:e,refreshToken:null,expiresAt:null,scopes:Qw(),subscriptionType:i.CLAUDE_CODE_SUBSCRIPTION_TYPE||null,rateLimitTier:i.CLAUDE_CODE_RATE_LIMIT_TIER||null}}
function l$(){if(i.CLAUDE_CODE_OAUTH_TOKEN)return bp(i.CLAUDE_CODE_OAUTH_TOKEN);let e=dE(),n=(r)=>({accessToken:r,refreshToken:null,expiresAt:null,scopes:Irs()??Qw(u$),subscriptionType:i.CLAUDE_CODE_SUBSCRIPTION_TYPE||null,rateLimitTier:i.CLAUDE_CODE_RATE_LIMIT_TIER||null});if(e&&(!_W()||pd()))return n(e);if(dn())return null;try{let h=Kn().read({fromStoreCopy:!0})?.claudeAiOauth;if(h?.accessToken)return h}catch(r){l(r)}if(e)return n(e);return null}
```
**Observed:** API-key selection in `nA`: bare mode accepts env key then helper only; otherwise env key can be suppressed by `Pv()`, special UTn path takes it immediately; headless `Ne(false)` takes FD key then env key; interactive env key requires approval; then FD key, helper, then primaryApiKey from login/config. Thus “ANTHROPIC_API_KEY always wins” is incorrect. Presence of both env key and bearer cannot be reduced to one winner for all HTTP clients: `nA`, `tl`, `ua` and endpoint adapters have distinct purposes.
`/tmp/cc_code.txt`, `function nA(` hit **4438**:
```js
if(n&&Arr(pue(n))==="approved")return{key:n,source:"ANTHROPIC_API_KEY"};let r=tW();if(r)return{key:r,source:"ANTHROPIC_API_KEY"};if(Ng()){if(e.skipRetrievingKeyFromApiKeyHelper)return{key:null,source:"apiKeyHelper"};return{key:hqr(),source:"apiKeyHelper"}}let h=PVe();if(h)return h;return{key:null,source:"none"}
```
**Observed:** for first-party auxiliary endpoints, `Ooe` sends OAuth bearer + beta when Et(), otherwise `x-api-key` when Hw() succeeds. ANTHROPIC_AUTH_TOKEN is only an explicitly opted-in fallback there, not automatically interchangeable with OAuth. Profile auth is tried by its enclosing `kj` path and includes bearer + beta (§2).
`/tmp/cc_code.txt`, `oauthRefreshLatch` hit **4690**:
```js
function Ooe(e={}){if(Ioe())return{headers:{},error:"Anthropic auth not used on third-party providers",reasonCode:"third_party"};if(Et()){let r=Ln();if(!r?.accessToken)return{headers:{},error:"No OAuth token available",reasonCode:"no_oauth_token"};return{headers:{Authorization:`Bearer ${r.accessToken}`,"anthropic-beta":Bu}}}if(Ae()==="gateway")return{headers:{},error:"Not available when using a Cloud gateway",reasonCode:"gateway"};let n=Hw();if(!n){let r=e.envBearerFallbackFor,s=r?U_():void 0;if(s&&r&&of()&&_a(r)&&!i.ANTHROPIC_UNIX_SOCKET)return{headers:{Authorization:`Bearer ${s}`}};return{headers:{},error:"No API key available",reasonCode:"no_api_key"}}return{headers:{"x-api-key":n}}}
```
**Observed:** profile selection is explicit ANTHROPIC_PROFILE, env federation configuration, then implicit active profile. CLI skips profile auth in bare/socket/pinned/injected bearer/OAuth/FD/helper/non-first-party regimes. An implicit **user_oauth** profile yields to an existing inference-scoped claude.ai login; explicit profile does not have that exception. Do not generalize this exception to implicit federation profiles.
`/tmp/cc_code.txt`, `function Av(` hit **6810**; `function pqr` hit **113053**:
```js
function pe(){let e=MH(),n=G();if(n){if(e===null)return null;let r=E(e,n);return r==="oidc_federation"||r==="user_oauth"?"profile-explicit":null}if(Y())return"env-quad";if(e!==null){let r=E(e,sxe(e));if(r==="oidc_federation"||r==="user_oauth")return"profile-implicit"}return null}
function pqr(e={}){return Boolean(Ej()||i.ANTHROPIC_UNIX_SOCKET||xr()||dn()||i.ANTHROPIC_AUTH_TOKEN||i.CLAUDE_CODE_OAUTH_TOKEN||dE()||(e.skipApiKeyHelper?!1:Ng())||!Fn())}function kc(e={}){if(!nst())return!1;if(pqr(e))return!1;let n=e.skipApiKeyHelper?void 0:$Si.of(U().host);if(Av()==="profile-implicit"){let r=Ln();if(nKt(r)&&R4e()){if(n)NK(n);return!1}}if(n)LK(n);return!0}
```
### Session-ingress/gateway tokens
**Observed:** `KU` strips optional sk-ant-si- prefix and parses the middle JWT component; `Jyn` tests scalar/array aud=claude-gateway, `vtr` extracts session_id, `m0` extracts exp. This helper **decodes without signature verification**. That is not proof ordinary Claude OAuth tokens are JWTs, nor that decoded claims establish entitlement. Gateway env configuration uses ANTHROPIC_BASE_URL + ANTHROPIC_AUTH_TOKEN (or descriptor token), derives expiration from JWT, and selects gateway regime (§4 above).
`/tmp/cc_code.txt`, `function KU(` hit **771**:
```js
function KU(e){let r=(e.startsWith("sk-ant-si-")?e.slice(10):e).split(".");if(r.length!==3||!r[1])return null;try{return te(Buffer.from(r[1],"base64url").toString("utf8"))}catch{return null}}function Jyn(e){let n=e?KU(e):null,r=n!==null&&typeof n==="object"&&"aud"in n?n.aud:void 0;return r==="claude-gateway"||Array.isArray(r)&&r.includes("claude-gateway")}function vtr(e){let n=KU(e);return n!==null&&typeof n==="object"&&"session_id"in n&&typeof n.session_id==="string"?n.session_id:void 0}function m0(e){let n=KU(e);if(n!==null&&typeof n==="object"&&"exp"in n&&typeof n.exp==="number")return n.exp;return null}
```
`/tmp/cc_code.txt`, `grant_type:"refresh_token"` hit **98486**, `Su`:
```js
if(!i.CLAUDE_CODE_USE_GATEWAY)return!1;let e=i.ANTHROPIC_BASE_URL,n=i.ANTHROPIC_AUTH_TOKEN||lAt();
let h=m0(n);return lie({url:s,jwt:n,expiresAt:h!==null?h*1000:Number.MAX_SAFE_INTEGER,unpinned:!0,...r?.hostVerdict&&{hostVerdict:r.hostVerdict}}),!0
```
## 5. Token refresh semantics
### Interactive refresh gate, serialization, races
**Observed:** refresh is eligible within **five minutes** of expiresAt; null expiry does not proactively refresh. `Eue` deduplicates non-forced checks by bare/store/backend context and stores pending promises. 401 handling separately deduplicates by failed bearer and context in pending401Handlers. Both gates re-read storage to adopt a sibling's token. Known invalid_grant tokens and account-on-hold tokens are not posted repeatedly.
`/tmp/cc_code.txt`, `API_KEY_URL,` hit **26065**, subsequent expiry helper; `oauth_token_refresh_lock_acquiring` hit **23240**:
```js
function rC(e,n=Date.now()){if(e===null)return!1;return n+300000>=e}
if(e===0&&!n){let w=cA(y)?"pendingOptedInRefreshChecks":"pendingRefreshChecks",C=pA(s,h),x=(j)=>S[w][j],P=gA(x,C);if(P)return P;
if(Oi.has(D.refreshToken))return"known_dead_refresh_token";if(Ri.has(D.refreshToken))return"account_on_hold";
if(K.accessToken!==F)return Sn(),a("tengu_oauth_token_refresh_race_resolved",{}),"refreshed";
```
**Observed:** cross-process lock is `<secure-storage-dir>/.oauth_refresh.lock` plus a legacy canonical-directory `.lock`, stale 60 seconds, updated every 5 seconds; compromised lock aborts the HTTP request. Acquisition retries `n<5` (initial attempt plus up to five retry attempts), each sleeping 1–2 seconds. Contended stable lock waits to a total **7.5-second** watch window; then tries verified dead-holder takeover once. Lock failures are typed outcomes; no generic “five token POST retries” follows from this. Refresh CAS save separately tries up to three writes with 100/200 ms waits (§3).
`/tmp/cc_code.txt`, `Sw=` hit **19035**; `oauth_token_refresh_lock_acquiring` hit **23240**:
```js
var Sw=7500;
function Anr(e,n){return{lockfilePath:hp(e,".oauth_refresh.lock"),realpath:!1,stale:60000,update:5000,onCompromised:(r)=>{t(`OAuth refresh lock compromised: ${r.message}`,{level:"error"}),n?.(r)}}}
if(n<5){a("tengu_oauth_token_refresh_lock_retry",{retryCount:n+1});let we=1000+Math.random()*1000;return await X(we),Ru(e,n+1,r+we,ae,h,F,S,w,C,x)}
if(Ew(ae,ee)&&r<Sw)await X(Sw-r),ee=await up(le,j);
```
**Observed:** refresh scope migration has one explicit network fallback on invalid_scope. On failure, reload and adopt a sibling if access token differs; invalid_grant is remembered and CAS-cleared to empty access/refresh strings with expiresAt 0; account hold is remembered separately. No universal exponential network-refresh backoff was established in Ru; the five-count loop above is lock contention, and scopes fallback is not a general retry.
`/tmp/cc_code.txt`, `oauth_token_refresh_lock_acquiring` hit **23240**:
```js
let fe=Q?aes(ae.scopes):ae.scopes,ee=Q&&Klr(ae.scopes),de;try{de=await koe(ae.refreshToken,{scopes:fe,clientId:ae.clientId,signal:Z.signal,invalidScopeRetried:ee,storageV5:w})}catch(ne){if(!ee||!Rqo(ne))throw ne;a("tengu_oauth_refresh_invalid_scope_fallback",{}),de=await koe(ae.refreshToken,{scopes:ae.scopes,clientId:ae.clientId,signal:Z.signal,storageV5:w})}
```
`/tmp/cc_code.txt`, `claudeAiOauth` hit **5935**, subsequent dead-token clearing:
```js
return r=!0,{...h,claudeAiOauth:{...y,refreshToken:"",accessToken:"",expiresAt:0}}
```
### 401 replay and oauthRefreshLatch
**Observed:** `Sv` retries once after 401 (optionally 403 with literal revoked-token response). When gated to the session credential it only forces refresh if the request actually used a refreshable session bearer. A different already-loaded bearer is replayed immediately. `oauthRefreshLatch.attempted` prevents another force attempt; outcomes in j_n reset it. This is distinct from sharing a pending refresh promise. The policy latch contains `{attempted:false,recoveries:{begun:0,running:0}}`.
`/tmp/cc_code.txt`, `oauthRefreshLatch` hit **4690**:
```js
let h=s.response?.status;if(!(h===401||n?.also403Revoked&&h===403&&typeof s.response?.data==="string"&&s.response.data.includes("OAuth token has been revoked"))||n?.signal?.aborted)throw s;
if(!r)throw s;let w=Ln()?.accessToken;if(w&&w!==r)return await e();let C=n.oauthRefreshLatch;if(C?.attempted)throw s;if(C)C.attempted=!0;
let D=Ln()?.accessToken;if(!D||D===r)throw s;return await e()
```
`/tmp/cc_code.txt`, `oauthRefreshLatch` hit **26302**:
```js
oauthRefreshLatch={attempted:!1,recoveries:{begun:0,running:0}};
```
`/tmp/cc_code.txt`, `function WS(` hit **12996**:
```js
let y=rRe.of(U().host),S=pA(n,r),w=cA(s),C=(j)=>y.pending401Handlers.get(vw(e,j,w)),x=gA(C,S);if(x){if(!h)return x.promise;return x.promise.then((j)=>{if(x.forcedOutcome!==void 0)h(x.forcedOutcome);return j})}
```
**Observed:** usage reads coordinate outstanding 401 reads in set B. On a 401 they await the other reads, resolve current auth, require it to differ from the bearer sent, and replay. This is executable logic, not merely the diagnostic string. A successful result resets the latch. The usage GET itself has timeout 5000 and refreshOAuth true; admission still requires Et() and profile scope kf().
`/tmp/cc_code.txt`, `oauthRefreshLatch` hit **12383**:
```js
let w=[...B].filter((P)=>P!==R);if(j(v)?.status!==401||w.length===0)throw v;let E=await Promise.allSettled(w),x=await G({credentials:e,settle:!0});if(x===null||x===S)throw v;let I=s?Q(x):void 0;if(I!==void 0)throw new K(I);if(!s&&!E.some((P)=>P.status==="fulfilled"&&P.value))throw v;t(`fetchUtilization: ${r} 401 during another read's token refresh; re-sending with the refreshed bearer`),L=await A()
return Fe.attempted=!1,t(`fetchUtilization: 200 after ${m} attempt(s)${m>1?" (401\u2192refresh\u2192retry succeeded)":""}`),L.data
```
**Observed:** injected unrefreshable token recovery can poll the supplied token for rotation (default every 2000 ms) and/or ask SDK host for a new token. Remote-child unrecovered-auth threshold defaults to 600000 ms, rather than inventing a refresh token for the injected bearer.
`/tmp/cc_code.txt`, `function WS(` hit **12996**:
```js
async function GSi(e){let n=e.pollMs??2000,r=e.readToken??(()=>i.CLAUDE_CODE_OAUTH_TOKEN??dE()??void 0),s=e.sleeper??((S)=>X(S)),h=Date.now()+e.timeoutMs;while(Date.now()<h){let S=r();if(S&&S!==e.failedAccessToken)return!0;await s(Math.min(n,Math.max(1,h-Date.now())))}let y=r();return Boolean(y&&y!==e.failedAccessToken)}
```
### Other refresh regimes must not be conflated
**Observed:** profile SDK 120/30/5-second refresh/cooldown is §2. CLI WIF user_oauth locking uses 5 retries fail-closed or 15 fail-open, one-to-two-second sleeps, 60-second stale lock / 5-second update. These counts are lock retries, not the `kci=5` cooldown.
`/tmp/cc_code.txt`, `ANTHROPIC_IDENTITY_TOKEN"` hit **14034**, following SDK providers:
```js
var le={"fail-closed":5,"fail-open":15};
await vl(e,{stale:60000,update:5000,onCompromised:(o)=>t(`WIF credentials lock compromised: ${o}`,{level:"error"})})
```
**Observed:** enterprise gateway IdP refresh is yet another path: within 300000 ms expiry, shared gFe pending operation, POST **form-encoded** grant_type+refresh_token to descriptor tokenEndpoint or `<gateway-url>/oauth/token`; 10-second timeout, no redirects. Invalid_grant clears IdP refresh token; transient failures leave credential for later retry.
`/tmp/cc_code.txt`, `grant_type:"refresh_token"` hit **98486**:
```js
function E5(e,{force:n=!1}={}){if(C_n())Su();let r=Wr();if(!r?.idpRefreshToken||!n&&r.expiresAt-Date.now()>=tK)return Promise.resolve();let s=gFe();if(s)return s;let h=rK(r,r.idpRefreshToken,e).finally(()=>wXr(null));return wXr(h),h}
let s=e.tokenEndpoint??`${e.url}/oauth/token`,h=M_e(await DCe(wj(new URL(s)),r),s),{data:y}=await _l.post(s,new URLSearchParams({grant_type:"refresh_token",refresh_token:n}).toString(),{headers:{"Content-Type":"application/x-www-form-urlencoded","User-Agent":Ds()},timeout:1e4,maxRedirects:0,...h&&{httpsAgent:h}})
```
## 6. Validation and revocation
**Observed:** validate is POST `BASE_API_URL/api/oauth/validate`, **null body**, bearer, JSON content type and CLI User-Agent, 10-second timeout. HCe accepts caller-provided delay list; retries only transient categories, not unauthorized/forbidden/TLS/proxy. It returns response data on success. The successful payload's full schema is not validated here and is not reconstructed from this call alone.
`/tmp/cc_code.txt`, `/api/oauth/profile` hit **17591**, subsequent `nti` and `HCe`:
```js
async function nti(e){let n=`${rn().BASE_API_URL}/api/oauth/validate`;try{return{ok:!0,data:(await Ht.post(n,null,{headers:{"User-Agent":Ds(),Authorization:`Bearer ${e}`,"Content-Type":"application/json"},timeout:1e4})).data}}
async function HCe(e,n=[]){let r=await nti(e);for(let s of n){if(r.ok||!TSi(r))break;await X(s),r=await nti(e)}if(r.ok)return g("oauth_token_validate"),r.data;
```
**Observed:** roles is GET configured ROLES_URL with bearer + User-Agent, timeout/signal 10 seconds; requires existing oauthAccount then stores organization_role, workspace_role, organization_name. Role checks are used for billing/cost/usage-credit UI: admin/billing/owner/primary_owner can administer usage credits; workspace_admin/workspace_billing also qualify for cost warnings. These are not evidence that the client can grant server-side entitlement.
`/tmp/cc_code.txt`, `ROLES_URL,` hit **25315**:
```js
let r=await Ht.get(rn().ROLES_URL,{headers:{"User-Agent":Ds(),Authorization:`Bearer ${e}`},timeout:1e4,signal:AbortSignal.timeout(1e4)});
organizationRole:s.organization_role,workspaceRole:s.workspace_role,organizationName:s.organization_name
```
`/tmp/cc_code.txt`, `organizationRole` hit **29575**:
```js
function Pqr(){let e=ue().oauthAccount?.organizationRole;return!!e&&["admin","billing","owner","primary_owner"].includes(e)}
```
`/tmp/cc_code.txt`, `function lbt(` hit **14527**, preceding `WBr`:
```js
return["admin","billing"].includes(h)||["workspace_admin","workspace_billing"].includes(y)
```
**Observed:** profile is GET BASE_API_URL/api/oauth/profile with bearer, CLI User-Agent, JSON content type, Cache-Control:no-cache, timeout 10 seconds. Minimum accepted shape has account.uuid/account.email and organization.uuid, with passthrough extra fields. `request-id` passing N9 is tracked as origin stamping; subscription attribution may require this stamp (§7). No beta header is specified at this call site.

`/tmp/cc_code.txt`, `/api/oauth/profile` hit **17591**:
```js
var sF=p(()=>Tt({account:Tt({uuid:ge(),email:ge()}).passthrough(),organization:Tt({uuid:ge()}).passthrough()}).passthrough());
```
```js
let n=`${rn().BASE_API_URL}/api/oauth/profile`;try{let r=await Ht.get(n,{headers:{"User-Agent":Ds(),Authorization:`Bearer ${e}`,"Content-Type":"application/json","Cache-Control":"no-cache"},timeout:1e4}),s=Fb(r.data);
```
```js
let h=typeof r.headers?.get==="function"?r.headers.get("request-id"):r.headers?.["request-id"];return{ok:!0,profile:s,stamped:N9(h)}
```

**Observed:** create_api_key POSTs **null** to API_KEY_URL with bearer and User-Agent, consumes `data.raw_key`, validates only permitted characters in ori, persists as primaryApiKey in this Linux build. This establishes a separately minted API key associated with the OAuth-authorized server context. **Unknown:** returned key prefix and exact account/workspace ownership semantics; the parser does not prove `sk-ant-api…` and no live key was minted. Verification-required responses become VerificationRequiredError; other explicit 4xx refusals become KeyMintRefusedError.

`/tmp/cc_code.txt`, `API_KEY_URL,` hit **26065**:
```js
let r=await Ht.post(rn().API_KEY_URL,null,{headers:{Authorization:`Bearer ${e}`,"User-Agent":Ds()}}),s=r.data?.raw_key;if(s)return await ori(s,n),a("tengu_oauth_api_key",{status:b("success"),statusCode:r.status}),g("oauth_create_api_key"),s;
```
`/tmp/cc_code.txt`, `primaryApiKey:e` hit **552**:
```js
primaryApiKey:s?y.primaryApiKey:e
```

**Observed:** refresh-token revocation POSTs JSON to `${TOKEN_URL}/revoke` with token, token_type_hint=refresh_token, client_id, Content-Type JSON, timeout 5 seconds. Failure returns false and explicitly continues local logout. Revocation message detection also recognizes HTTP 403 “OAuth token has been revoked” and HTTP 401 “OAuth access token has been revoked”; these string comparisons are executable classifiers, not proof every revoked server response uses that format.

`/tmp/cc_code.txt`, `ROLES_URL,` hit **25315**:
```js
async function vVr(e,n){try{return await Ht.post(`${rn().TOKEN_URL}/revoke`,{token:e,token_type_hint:"refresh_token",client_id:n??rn().CLIENT_ID},{headers:{"Content-Type":"application/json"},timeout:5000}),g("oauth_token_revoke"),!0}catch(r){let s=Ht.isAxiosError(r)?r.response?.status:void 0;return t(`OAuth token revoke failed (status=${s??"network"}); continuing with local logout.`),f("oauth_token_revoke",`http_${s??"network"}`),!1}}
```

### Hold / denied callbacks and federation validation

**Observed:** OAuthAccountOnHoldError carries sanitized URL; OAuthCallbackError carries error/errorDescription/errorUri/displayMessage. Hold detection is feature-gated by `c4t()`, requires invalid_grant/access_denied + error_description=account_on_hold, and sanitizes URL to allowed Anthropic/Claude origins, otherwise https://claude.ai/restricted. Federation Cf errors are bridged through the same hold parser for status 400/401/403. W3r's access-token/type/expiry validation is §2; it does not cryptographically verify the external assertion locally.

`/tmp/cc_code.txt`, `Sci=` hit **2836**, preceding classes:
```js
class Qbe extends Error{url;constructor(e){super("OAuth account is on hold");this.url=e;this.name="OAuthAccountOnHoldError"}}class FZ extends Error{error;errorDescription;errorUri;displayMessage;constructor(e,n,r){super("OAuth authorization was denied or failed");this.error=e;this.errorDescription=n;this.errorUri=r;this.name="OAuthCallbackError",this.displayMessage=n?`${e}: ${n}`:e}}
```
`/tmp/cc_code.txt`, `function Av(` hit **6810**, preceding `C4e`:
```js
function C4e(e){if(!(e instanceof Cf)||e.statusCode!==400&&e.statusCode!==401&&e.statusCode!==403)return null;return gEn(e.body)}
```

## 7. Token-derived account data

### What is decoded versus fetched

**Observed:** the located normal sign-in path gets subscription/profile fields through h2t/kqo/Bb **HTTP profile**, not by decoding the OAuth access token. Token endpoint account/organization/workspace fields populate tokenAccount (§2), distinct from JWT claims. The explicitly located JWT decoder uses only aud/session_id/exp for gateway/session credentials (§4). Treat ordinary OAuth token format/account claims as **unknown**, not universally JWT.

**Observed:** profile maps organization.organization_type via `claude_max → max`, `claude_pro → pro`, `claude_enterprise → enterprise`, `claude_team → team`, anything else null. It carries organization.rate_limit_tier, seat_tier, has_extra_usage_enabled, billing_type, onboarding flags, trial end/duration, plus account display/full names/creation date and subscription_created_at. Missing quota/tier data remains null; no hard-coded quota is derived here.

`/tmp/cc_code.txt`, `rate_limit_tier` hit **28095**:
```js
var EVr=new Map([["claude_max","max"],["claude_pro","pro"],["claude_enterprise","enterprise"],["claude_team","team"]]);function Rtr(e){return(e!==void 0?EVr.get(e):void 0)??null}
```
```js
async function h2t(e){let n=await kqo(e),r=n?.profile,h={subscriptionType:Rtr(r?.organization?.organization_type),rateLimitTier:r?.organization?.rate_limit_tier??null,seatTier:r?.organization?.seat_tier??null,hasExtraUsageEnabled:r?.organization?.has_extra_usage_enabled??null,billingType:r?.organization?.billing_type??null,ccOnboardingFlags:r?.organization?.cc_onboarding_flags??{},claudeCodeTrialEndsAt:r?.organization?.claude_code_trial_ends_at??null,claudeCodeTrialDurationDays:r?.organization?.claude_code_trial_duration_days??null};
```

**Observed:** account config refresh uses a **24-hour** freshness check (`Gb=86400000`, §2), inference OAuth + profile scope, then stores profile data. Subscription provenance protects existing sign-in tiers: profile writes require origin stamping, CAS matching refresh token, and refuse certain team/enterprise downgrades. These fields drive plan/tier/UI state, not proof of a numerical usage allowance.

`/tmp/cc_code.txt`, `rate_limit_tier` hits **28095**, **31123**; `claudeAiOauth` hit **4704**:
```js
x=C!==void 0&&Date.now()-C<Gb
```
```js
async function sri({expectedRefreshToken:e,subscriptionType:n,rateLimitTier:r,verifiedAt:s,originStamped:h,credentials:y}){if(!h)return"unstamped";
```
```js
if(!x||x.refreshToken!==e)return S="rotated",C;
```
```js
function Eqr(e,n){return(e==="team"||e==="enterprise")&&n!=="team"&&n!=="enterprise"}
```

### Consumers of tier, billing, and subscription flags

**Observed:** `billingType` gates usage-credit eligibility via a fixed allowed set, including subscription, marketplace and trial billing categories; `usage_based` distinguishes billing in FO(). Roles change instructions/admin eligibility (§6). `hasExtraUsageEnabled` is saved to oauthAccount by profile refresh (§3), not inferred from subscription type. Scope kf() gates usage reads (§5). Rate-limit tier is stored alongside subscription type in credentials (§3) and emitted in telemetry; it is not a limit number.

`/tmp/cc_code.txt`, `organizationRole` hit **29575**:
```js
var vA=new Set(["stripe_subscription","stripe_subscription_contracted","stripe_subscription_enterprise_self_serve","aws_marketplace","c4e_consumption_trial","apple_subscription","google_play_subscription"]);function KSi(){let e=gn()?.billingType;if(!Et()||!e)return!1;return vA.has(e)}
```
`/tmp/cc_code.txt`, `function lbt(` hit **14527**:
```js
function FO(){return gn()?.billingType==="usage_based"}
```
`/tmp/cc_code.txt`, `organizationRole` hit **19257**, preceding telemetry output:
```js
...e.subscriptionType&&{subscriptionType:e.subscriptionType},...e.rateLimitTier&&{rateLimitTier:e.rateLimitTier}
```

**Observed:** an extra-usage-enabled team/enterprise account suppresses certain near-limit warnings for non-admin users (`!L_()`). Upgrade handling uses the exact rate-limit-tier string `default_claude_max_20x` with subscriptionType=max to identify the highest Max plan, falling back to organization fields from profile if cached metadata is missing. This is an internal tier classifier, not a measured quota or a current product/pricing assertion.
`/tmp/cc_code.txt`, `hasExtraUsageEnabled` hit **15653**; `rateLimitTier===` hit **665**:
```js
let r=cr(),s=r==="team"||r==="enterprise",d=gn()?.hasExtraUsageEnabled===!0;if(s&&d&&!L_())return null;
if(o?.subscriptionType&&o?.rateLimitTier)n=o.subscriptionType==="max"&&o.rateLimitTier==="default_claude_max_20x";else if(o?.accessToken){let t=await Wvt(o.accessToken);n=t?.organization?.organization_type==="claude_max"&&t?.organization?.rate_limit_tier==="default_claude_max_20x"}
```
**Observed:** `has_claude_max` / `has_claude_pro` are account booleans used to offer switching from API-key use to an existing subscription. That particular path fetches `/api/claude_cli_profile` with x-api-key + account_uuid + beta, rather than assuming the booleans are JWT claims. It skips the notice when already Et(); feature gates suppress the Max/Pro suggestions. These booleans are not the primary organization_type mapping.

`/tmp/cc_code.txt`, `/api/oauth/profile` hit **17591**, preceding Eqo; `has_claude_max` hit **69815**:
```js
let n=ue().oauthAccount?.accountUuid,r=b9(Sue());if(!n||!r)return;let s=`${rn().BASE_API_URL}/api/claude_cli_profile`;try{let h=await Ht.get(s,{headers:{"User-Agent":Ds(),"x-api-key":r,"anthropic-beta":Bu,"Cache-Control":"no-cache"},params:{account_uuid:n},timeout:1e4});return Fb(h.data)}
```
```js
async function ncs(){if(Et())return null;let m=await Eqo();if(!m)return null;if(m.account.has_claude_max&&!lbt())return"Max";if(m.account.has_claude_pro&&!PYs())return"Pro";return null}
```

## Confidence and gaps

- **High confidence, static:** shared authorize parameters, PKCE/state generation, callback comparison, JSON interactive grants, five-minute interactive refresh gate, separate snake_case SDK/profile grants, federation assertion inputs and 120/30/5-second cache thresholds. Quotes are executable logic, not inferred solely from strings.
- **Important boundary:** this is a Linux platform-specialized artifact. It proves plaintext selection here, not macOS/Windows backend policy. Keychain service naming/calls and Windows Credman gate are present, but macOS fallback precedence and Windows actual credential storage require those binaries/runtime evidence. Bounded search for `secret-service` in code returned no match; absence here is not a cross-platform guarantee.
- **Unknown:** successful server payloads/ownership of minted raw_key, actual key prefix, full validate response schema, server-side entitlements/revocation guarantees, and whether this account can use workload federation. No credential was read, login performed, token exchanged/refreshed/revoked, or API key minted.
- **Unknown:** complete transport-global headers and networking timeout/retry policy for the SDK/profile exchange. Grant call sites show exact local headers and no retry/timeout argument; do not transplant interactive Axios timeout policy to this subsystem.
- **Not a universal precedence table:** policy pins, host injection, bare/headless mode, API-key approvals, explicit versus implicit profiles, and endpoint-specific auth adapters materially change selection. The report quotes the gates rather than promising one global env/file/keychain ordering.
- **Manual state nuance:** normal UI splits code#state, but shared flow resolver ignores the passed state and exchanges with original state. Local callback and separate cloud-sessions terminal helper do compare state. Server enforcement for manually pasted code was not exercised.
- **Remaining account-consumer coverage:** storage, billing/admin UI, subscription-switch notice and tier telemetry are traced. Every downstream feature using seat/trial/onboarding/tier fields and every server model entitlement consumer has not been enumerated; no claim of exhaustive downstream business logic is made.
- **Reverification:** reran `ctx.py 'grant_type:"refresh_token"'`, `ctx.py 'grant_type:Sci'`, and `ctx.py 'function rC('` against code corpus; authorization/refresh, federation request and five-minute buffer quotes checked verbatim. A separate substring check of all fenced JS snippets against `/tmp/cc_code.txt` is recorded in the task report.
