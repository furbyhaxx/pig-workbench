package prompt

import (
	"embed"
	"io/fs"
)

//go:embed templates
var templateFiles embed.FS

// BuiltinFS is the confined builtin template set, rooted so names are builtin/workbench.
func BuiltinFS() fs.FS {
	sub, err := fs.Sub(templateFiles, "templates")
	if err != nil {
		panic(err)
	}
	return sub
}

// RequiredSections are the blocks builtin/workbench must keep.
var RequiredSections = []string{"preamble", "tools", "rules", "resources", "docs", "addendum", "project_context", "skills", "cwd"}
