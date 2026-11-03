---
document:
  type: "research"
  id: "RES-antigravity-cli-1.4.12-09-delta-from-1.4.8"
  title: "agy 1.4.12 delta from 1.4.8"
  description: "Observed binary and harness RPC differences and newly recovered prompt and tool evidence"
  status: "published"
  revision: 1
  created_at: "2026-11-03T12:02:33+01:00"
  updated_at: "2026-11-03T12:02:33+01:00"
  created_by: "worker"
  updated_by: "worker"
  changelog:
    - revision: 1
      reason: "document creation"
      changes: ["compare binary inventory and generated language-server methods", "identify newly recovered prompt and tool evidence"]
---

# agy 1.4.12 — delta from 1.4.8

## Method and boundary

[OBS] The new executable was inspected statically with the prebuilt `goresym`
cache, disassembly, embedded Go string/descriptor data and the baseline's
recorded inventory; no new model request was sent. Current identity, hash and
RPC summary are in [`evidence/identity-and-rpc-summary.txt`](evidence/identity-and-rpc-summary.txt).
The baseline is [`../1.4.8/01-overview.md`](../1.4.8/01-overview.md) and its
[`data/rpc_methods.txt`](../1.4.8/data/rpc_methods.txt).

[UNK] The 1.4.8 executable was not independently compared byte-for-byte in
this pass. A fact absent from its dossier is not necessarily a feature added
in 1.4.12. The default-session assembled prompt, ordered tool list and exact
request schemas remain unknown; see chapters [10](10-system-prompt.md) and
[11](11-tool-declarations.md).

## Binary inventory

[OBS] Counts below compare the baseline documentation with the new cache
([`cache-metadata.txt`](evidence/cache-metadata.txt)); size is from `stat`.
The file figures count distinct recovered `.go` names, rather than the raw
pclntab `nfiles` header.

| Measure | 1.4.8 | 1.4.12 | Difference |
|---|---:|---:|---:|
| Binary size, bytes | 228,897,168 | 234,144,144 | +5,246,976 |
| Pclntab functions | 171,928 | 174,452 | +2,524 |
| Distinct recovered `.go` files | 6,825 | 6,875 | +50 |
| `.text` virtual base | `0x6596000` | `0x6819000` | `+0x283000` |

[OBS] The new cache's raw pclntab header reports `nfiles=6931`, while its
recovered distinct-file list contains 6,875 names. These are different
measurements and should not be interchanged. The installed 1.4.12 binary's
SHA-256 is `1edfb30477447825f0aaa05fd9a91d82f9adf26686f41cd4f5c70b90abe383e8`.

## Harness RPC surface

[OBS] Comparing the 1.4.8 index's `LanguageServerService` entries with the
1.4.12 generated `UnimplementedLanguageServerServiceServer` methods yields
359 versus 363 distinct names: four present only in the latter and none only
in the former. The complete comparison, source hash and reproducible command
are [`evidence/rpc-delta.json`](evidence/rpc-delta.json) and
`python3 evidence/compare-rpcs.py`.

```text
GetGitHubPullRequests
GetGoogleChatLinkInfo
GetWorkspaceGitHubPullRequests
RunTrigger
```

[INF] These are additions relative to the recorded generated-symbol index,
not proof that the methods are registered or usable in every CLI session.
[UNK] Other services and changed request or response fields were not compared.

## Prompt and tool knowledge gained

[OBS] The 1.4.12 binary contains an 821-byte parameterized identity template,
other prompt-section templates, and Hermes/XML tool-format templates. [INF] Its
instructions also reconstruct two complete cumulative-default outputs and a
coding-worker prompt. Full bytes and evidence are in [chapter 10](10-system-prompt.md).
[UNK] The prior dossier did not establish these variants or their selectors, so
a prompt-text change between versions cannot be concluded from this comparison.

[INF] In this binary, `FindToolConverter` assigns `find_by_name` as its tool
name, and direct constructors or oneharness methods also expose `write_to_file`,
`write_file`, `sed_file`, `notebook_execution` and `run_workflow`; see
[chapter 11](11-tool-declarations.md). [UNK] The 1.4.8 dossier did not establish
whether these literal names existed there; this is a correction or extension
of the recorded inventory, not a demonstrated version introduction.

[OBS] Unlike the unresolved enum numbers recorded in
[`../1.4.8/06-mechanics.md` §2.1](../1.4.8/06-mechanics.md), the 1.4.12
embedded descriptor resolves `ToolFormatterType`: `UNSPECIFIED=0`, `LLAMA_3=1`,
`HERMES=2`, `XML=3`, `CHAT_TRANSCRIPT=4`, `NONE=5`
([evidence](evidence/system-prompt/tool-formatter-enum.txt)). [UNK] The actual
format selected per model and whether the enum differed in 1.4.8 are unverified.
