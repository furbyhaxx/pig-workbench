# skills

Catalog of `SKILL.md` files. No SDK and no session state.

- `Build` merges already discovered sources (the host's skills, which keep their name and description) with a walk of roots; the first name wins and sources come first. `Scan` is `Build` without sources. Nested skills live at `<parent>/skills/<child>/SKILL.md` and name their parent by its catalog name.
- `Conditions` (`conditions.go`) evaluates `when` (files, env) and `auto_load` (files, touched): every listed kind must hold, within a kind any entry; conditions apply to top-level skills only, and a nested skill is hidden until its parent name is in the loaded set. Callers pass the repository root. A glob costs one gitignore-aware `search.Find` walk, reused for `globTTL`; plain paths are checked every call.
- Frontmatter is a small subset (`name`, `description`, `bundle`, `when`, `auto_load`), not a YAML parser. An invalid `when` lists the skill unconditionally, an invalid `auto_load` is ignored, and both land in `Skill.Problems` for the user; never show them to the model.
- Loading, unload, `skill://` and persistence belong to `internal/host`.
