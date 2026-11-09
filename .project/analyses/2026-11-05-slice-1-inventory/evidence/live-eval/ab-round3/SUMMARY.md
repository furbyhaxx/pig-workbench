# A/B round 3 (deepseek/deepseek-flash, 09.11.2026)

16 fixed tasks (5 from the original A/B, 11 from the tool-text suite incl. 4 held-out), 5 repetitions per task and arm,
fresh session and fixture per run, pass judged from disk or the exact answer. Arms: PiG's own prompt (`pig-prompt`) and
`builtin/workbench` (`workbench-prompt`). Per-run records: `ab-results.json`, `tools-r3.json`. Raw event traces were not retained (9 MB).

| Arm | Runs | Pass | Tool errors | Input tokens | Output tokens | Cache hit (5-task part) |
|---|---|---|---|---|---|---|
| pig-prompt | 80 | 79 | 4 | 501 547 | 12 244 | 69.4% |
| workbench-prompt | 80 | 79 | 0 | 387 379 | 12 873 | 76.9% |

Failures (one per arm, both single model slips, not reproducible):
- pig-prompt `cd-relative` #2 answered "1" instead of 8123.
- workbench-prompt `grep-line-number` #1 read the `path:26:9:` column as the line. Ten further repetitions per arm passed 10/10 each (`gl0`), so no text change was made.

Observation, not a defect: on `rename-across-files` the Workbench prompt never used `sed -i`; it read and edited each file (35 vs 19 calls over 5 runs).
