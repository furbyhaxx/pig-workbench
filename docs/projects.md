# Projects

A project is the identity that ties together every checkout, worktree and repository of one piece of work. It decides which sessions `/resume` lists together, where private notes live and which instructions apply. A directory that belongs to no project is in the builtin `global` project.

## `.project/Project.toml`

Commit `.project/Project.toml` in each repository of the project; all of them share one `id`. This example uses every supported key; only `id`, `slug` and `[component]` are required.

```toml
# .project/Project.toml: the shared identity of a project.
# Commit this file in every repository of the project. All of them carry the same `id`.
# Everything here is read strictly: an unknown key or a wrong type is refused with its location.

# Schema version. Optional; the only supported value is 1.
version = 1

# Generated once, never reused or edited: "prj-" followed by lowercase letters or digits.
# Sessions, private notes and cross-repository matching are keyed on it.
id = "prj-01jc9z8k2m"

# Short name for URIs and commands, unique in your project registry: lowercase words joined by "-".
slug = "pig-extensions"

# Optional human-readable name and description. Shown in `/workbench projects`.
name = "PiG extensions"
description = "Workbench and web extensions for PiG"

# This repository's role in the project. Required.
[component]
# Lowercase kebab slug, unique among the project's components.
slug = "pig-workbench"
# Exactly one component of the project is primary; it holds project-wide artifacts.
primary = true
# Optional hints for matching a clone of this repository to the project.
default_remote = "github.com/furbyhaxx/pig-workbench"
default_branch = "main"

# The project's other repositories. Optional; repeat the table once per repository.
# A project with a single repository has no [[components]].
[[components]]
slug = "pig-web"
default_remote = "github.com/furbyhaxx/pig-web"
default_branch = "main"
# primary = false is the default; only one component of the whole project may be primary.

# Optional. How git worktrees of this repository behave.
[worktrees]
# Untracked files linked from the main checkout into new worktrees.
links = [".env", ".env.local", ".references"]
# Whether all worktrees share one project artifact tree.
share_project_tree = true

# Optional. Where project artifacts live.
[artifacts]
# "tree" keeps one shared tree (default); "branch" keeps artifacts per branch.
storage = "tree"

# Optional. Project conventions Workbench applies.
[conventions]
# Treat the .project/ tree as first-class development artifacts.
artifacts = true
# Apply the AGENTS.md and BACKLOG.md standards.
agents_md = true
```

## How Workbench recognises a project

For the current directory, in order:

1. `.project/Project.toml` at the repository top (or in an ancestor for directories outside git).
2. The repository's local git marker `pig.projectId` (set by joining a project without adding a file).
3. A project already in your registry that matches the repository by root commit, then normalised remote, then recorded host and path.
4. Otherwise the directory is in `global`. Workbench never creates a project silently.

Matches that disagree are reported, not guessed.
