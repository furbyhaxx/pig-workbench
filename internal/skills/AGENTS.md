# skills

Catalog of `SKILL.md` files. No SDK and no session state.

- `Scan` walks roots and keeps the first name. Nested skills live at `<parent>/skills/<child>/SKILL.md`.
- `Visible` hides a nested skill until its parent name is in the loaded set, and hides a skill whose `when.files` match nothing under the root.
- Frontmatter is a small subset (`name`, `description`, `when.files`). Invalid frontmatter is skipped, not invented.
- Loading, unload, `skill://` and persistence belong to `internal/host`.
