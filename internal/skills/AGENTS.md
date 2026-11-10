# skills

Catalog of `SKILL.md` files. No SDK and no session state.

- `Build` merges already discovered sources (the host's skills, which keep their name and description) with a walk of roots; the first name wins and sources come first. `Scan` is `Build` without sources. Nested skills live at `<parent>/skills/<child>/SKILL.md` and name their parent by its catalog name.
- `Visible` hides a nested skill until its parent name is in the loaded set, and hides a skill whose `when.files` match nothing under the root.
- Frontmatter is a small subset (`name`, `description`, `when.files`). Invalid frontmatter is skipped, not invented.
- Loading, unload, `skill://` and persistence belong to `internal/host`.
