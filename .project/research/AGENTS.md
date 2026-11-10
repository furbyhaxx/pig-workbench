Prior art and third-party dossiers. Artifact type `research`, id `RES-<slug>`; multi-file dossiers are `<slug>/README.md` + version-keyed subdirs + `evidence/`.

## Dossiers

Each has its own `AGENTS.md` with version ownership, verification commands, traps and freshness.

| Dossier | Subject |
|---|---|
| `antigravity-cli/` | Google `agy`, binary RE: 1.4.8 baseline + 1.4.12 harness internals |
| `claude-code-cli/` | Anthropic `claude`, binary RE: 2.1.310 provider surface + 2.1.317 harness internals |
| `codex-cli/` | OpenAI Codex, `codex-rs` source at tag `0.167.0-alpha.4` |
| `deepseek-harness/` | DeepSeek Harness, source at commit `8bd0733` |

- Dossiers of a shipped artifact are keyed by the pinned version: `<slug>/<version>/`. A new version gets a new subdir beside the old one; never overwrite captured evidence.
- Claims cite a location inside the dossier (`1.4.8/06-mechanics.md` §2.2) or a URL with an access date, and mark observed versus inferred. No inference presented as observation.
- Imported dossiers are snapshots, not maintained here. `imported_from:` names the upstream repository, commit and path; re-copy from upstream rather than editing. The only permitted local edits are path repairs that keep reproduction commands runnable.
- Keep large regenerable inventories out of git. Keep the reproduction recipe, and the `.gitignore` beside the inventory.
- Redact credentials and live tokens before committing evidence; mask home paths.
- Research useful beyond this project belongs in the shared knowledge base (`host:/mnt/shared/knowledge-base`), not here. Once its decision is recorded in a spec or plan, delete the research.
