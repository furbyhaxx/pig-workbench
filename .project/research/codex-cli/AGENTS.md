# AGENTS.md — codex-cli dossier

Source analysis of `openai/codex`, `codex-rs/` subtree only. The binary is not installed and nothing was executed; every claim is read from the pinned checkout.

- Pinned to tag `rust-v0.167.0-alpha.4`, commit `fb036d4ebd89f263d34a1f888c279ce85bb383c6`. `0.167.0-alpha.4/` is the only version dir.
- Reproduce: `git clone --depth 1 --branch rust-v0.167.0-alpha.4 --filter=blob:none --sparse https://github.com/openai/codex.git && git sparse-checkout set codex-rs`.
- **Don't infer the version from a branch checkout.** `main` reports `[workspace.package] version = "0.0.0"`; only the release tag carries the real value (`codex-rs/Cargo.toml:173`).
- `core/src/context/` is one file per injection. `04-instructions-and-injections.md` covers all 53 non-test modules, including the `contextual_user_message` reverse-mapping registry. A new upstream module means that chapter is stale.
- `evidence/prompt/base_instructions-default.md` is byte-identical to the upstream `protocol/src/prompts/base_instructions/default.md` (paths in this paragraph are relative to the pinned `codex-rs/` checkout) apart from the required frontmatter. Quote from the evidence file, not from the chapter.

## Verify

```bash
python3 0.167.0-alpha.4/evidence/verify.py <checkout>/codex-rs
python3 0.167.0-alpha.4/evidence/verify-worker-dossier.py <checkout>/codex-rs
```

Both need a checkout of that tag and its `codex-rs` path. They check frontmatter (revision/changelog coherence), prompt and template byte equality, captured sources, citation ranges and links.

## Traps

- Nothing was executed: there is no serialised request for any feature configuration. Where a prompt is composed from fragments, the chapter shows the fragments and the builder, not one rendered string.
- Prompts are gated: `personality = none` strips the `# Personality` section; disabling `update_plan` strips planning and checklist sections. Never quote a prompt without its gates.
- Base instructions ship as a `developer` message with classification `model.base_instructions`, not as a wire `system` message.
- The misleadingly named `instructions_template` is literal text, not a template.

## Cross-updates

- Re-pin the tag → rename the version dir, update `README.md`, and re-run both verifiers.
- New injection module upstream → extend the catalogue in `04` and re-run the verifiers.

## Freshness

Base prompt and permission templates age in weeks; crate architecture in months. Re-pin and re-run the verifiers before trusting either after an upstream bump.
