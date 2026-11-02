Measurements, probe runs and backend evaluations from this workspace. Artifact type `analysis`; dated `YYYY-MM-DD-<slug>/README.md` with raw material in `evidence/`.

- Every finding cites a file under its `evidence/`; keep probe sources and scenarios runnable, not just their output.
- Redact credentials and replace home paths with `$HOME` before committing evidence; omit system prompts from captured requests.
- Findings that change a contract go into the affected spec in the same change, or are listed as open spec work.
