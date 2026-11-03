Development artifacts; committed, never shipped or placed in `docs/`. Load skill `dot-project` before creating or revising artifacts.

- `drafts/` — undecided proposals and RFCs; rules in `drafts/AGENTS.md`.
- `plans/` — numbered implementation plans, dependencies and acceptance gates; rules in `plans/AGENTS.md`.
- `specs/` — numbered API and architecture contracts; rules in `specs/AGENTS.md`.
- `analyses/` — dated probe runs, measurements and backend evaluations with evidence; rules in `analyses/AGENTS.md`.
- `research/` — imported third-party dossiers and prior art, version-keyed and snapshot-only; rules in `research/AGENTS.md`.
- `sketches/` — UI/UX designs, wireframes and capture evidence; rules in `sketches/AGENTS.md`.
- `handoff/` — resume briefs and cross-repository requests; rules in `handoff/AGENTS.md`.
- Every Markdown artifact needs `document:` frontmatter: type, id, title, description, status, revision, created_at, updated_at, created_by, updated_by, changelog. Quote YAML strings; timestamps use Europe/Zurich offsets.
- Revisions increment revision, update updated_* and append changelog; never rewrite earlier entries.
- Sources outside this repository use `host:/absolute/path`. Evidence belongs beside its artifact in `evidence/`; redact secrets.
- Superseded artifacts are archived with a successor pointer, not deleted. Add other artifact types only when needed, with their own AGENTS.md and a row here.
