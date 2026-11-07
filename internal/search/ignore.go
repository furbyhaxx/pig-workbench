package search

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	gitignore "github.com/boyter/gocodewalker/go-gitignore"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

type ignoreScope struct {
	dir   string
	rules gitignore.GitIgnore
}
type ignoreTree struct {
	root      string
	scopes    []ignoreScope
	remaining int64
}

func newIgnoreTree(ctx context.Context, root string) (*ignoreTree, error) {
	absolute, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	tree := &ignoreTree{root: absolute, remaining: 1 << 20}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	info, err := os.Lstat(absolute)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return tree, nil
	}
	boundary, err := repositoryBoundary(absolute)
	if err != nil {
		return nil, err
	}
	if boundary {
		return tree, nil
	}
	dir := filepath.Dir(absolute)
	var parents []string
	for {
		parents = append(parents, dir)
		boundary, err := repositoryBoundary(dir)
		if err != nil {
			return nil, err
		}
		if boundary {
			break
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			break
		}
		dir = parent
	}
	for i := len(parents) - 1; i >= 0; i-- {
		if err := tree.load(ctx, parents[i]); err != nil {
			return nil, err
		}
	}
	return tree, nil
}

func repositoryBoundary(dir string) (bool, error) {
	_, err := os.Lstat(filepath.Join(dir, ".git"))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

func (t *ignoreTree) visit(ctx context.Context, path string, entry fs.DirEntry) (bool, error) {
	absolute, err := filepath.Abs(path)
	if err != nil {
		return false, err
	}
	parent := filepath.Dir(absolute)
	for len(t.scopes) > 0 {
		last := t.scopes[len(t.scopes)-1]
		rel, err := filepath.Rel(last.dir, parent)
		if err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
			break
		}
		t.scopes = t.scopes[:len(t.scopes)-1]
	}
	if absolute != t.root {
		for i := len(t.scopes) - 1; i >= 0; i-- {
			if match := t.scopes[i].rules.Absolute(absolute, entry.IsDir()); match != nil {
				if match.Ignore() {
					return true, nil
				}
				break
			}
		}
	}
	if entry.IsDir() {
		return false, t.load(ctx, absolute)
	}
	return false, nil
}

func (t *ignoreTree) load(ctx context.Context, dir string) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	path := filepath.Join(dir, ".gitignore")
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if !before.Mode().IsRegular() {
		return fmt.Errorf("ignore file is not regular: %s", path)
	}
	file, opened, err := fileio.OpenRegular(path)
	if err != nil {
		return err
	}
	defer file.Close()
	if !os.SameFile(before, opened) {
		return fmt.Errorf("ignore file changed while opening: %s", path)
	}
	data, err := io.ReadAll(io.LimitReader(file, t.remaining+1))
	if err != nil {
		return err
	}
	if int64(len(data)) > t.remaining {
		return fmt.Errorf("ignore files exceed the 1 MiB search budget: %s", path)
	}
	t.remaining -= int64(len(data))
	var invalid bool
	matcher := gitignore.New(bytes.NewReader(data), dir, func(gitignore.Error) bool { invalid = true; return false })
	if invalid {
		return fmt.Errorf("invalid ignore pattern in %s", path)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	t.scopes = append(t.scopes, ignoreScope{dir: dir, rules: matcher})
	return nil
}
