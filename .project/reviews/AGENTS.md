Ranked findings on a commit range, slice or diff. Artifact type `review`; dated `YYYY-MM-DD-<slug>/report.md` with gate logs and captures in `evidence/`.

- Every finding names its location (`path:line`) and how it was observed: live, test, static or reading.
- A review is consumed once each finding is fixed or carried by a plan or backlog item; delete it then, with `Artifact-Deleted` trailers.
- Redact credentials and replace home paths with `$HOME` in evidence.
