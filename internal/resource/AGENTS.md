# resource

Shared ToolPath grammar, provider router and pager (SPEC-0004/0006).
- Literal local filenames win over suffix parsing. URI paths decode once; opaque queries/fragments are not selectors.
- One normalized selector type; distinguish selection completeness from resource completeness.
- Track exact source coordinates. Partial lines must never become seen; continuation repeats a partial line.
- Open only regular files; bound memory independently of source size. Canonical identity and exact-byte revisions govern snapshots.
