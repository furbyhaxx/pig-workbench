package search

import (
	"fmt"
	"net/url"
	"path"
	"path/filepath"
	"strings"

	"github.com/bmatcuk/doublestar/v4"
)

func validateGlobs(pattern string, exclude []string) error {
	if !doublestar.ValidatePattern(pattern) {
		return fmt.Errorf("%w: malformed glob", ErrInvalidPattern)
	}
	for i, pattern := range exclude {
		if !doublestar.ValidatePattern(pattern) {
			return fmt.Errorf("%w: malformed exclusion %d", ErrInvalidPattern, i+1)
		}
	}
	return nil
}

func matchGlob(pattern, root, target, name string) bool {
	if !strings.Contains(pattern, "/") {
		return doublestar.MatchUnvalidated(pattern, name)
	}
	relative := name
	if strings.Contains(root, "://") {
		base, err := url.Parse(root)
		child, otherErr := url.Parse(target)
		if err == nil && otherErr == nil && base.Scheme == child.Scheme && base.Host == child.Host && base.Path != child.Path {
			relative = strings.TrimPrefix(child.Path, strings.TrimSuffix(base.Path, "/")+"/")
		}
	} else if rel, err := filepath.Rel(root, target); err == nil && rel != "." {
		relative = filepath.ToSlash(rel)
	}
	return doublestar.MatchUnvalidated(pattern, relative) || doublestar.MatchUnvalidated(pattern, filepath.ToSlash(target))
}

func excluded(root, target string, patterns []string) bool {
	name := filepath.Base(target)
	if strings.Contains(target, "://") {
		if u, err := url.Parse(target); err == nil {
			name = path.Base(strings.TrimSuffix(u.Path, "/"))
		}
	}
	for _, pattern := range patterns {
		if matchGlob(pattern, root, target, name) {
			return true
		}
	}
	return false
}
