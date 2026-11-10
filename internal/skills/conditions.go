package skills

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/bmatcuk/doublestar/v4"

	"github.com/furbyhaxx/pig-workbench/internal/search"
)

// globTTL bounds how long a glob result is reused before the tree is walked again.
const globTTL = 30 * time.Second

// Conditions evaluates when and auto_load conditions. A plain path is checked on every call; a glob
// costs one gitignore-aware walk of the root per pattern and its result is reused for globTTL. The
// zero value is ready, and a nil receiver evaluates without reuse.
type Conditions struct {
	mu    sync.Mutex
	globs map[string]globResult
}

type globResult struct {
	holds bool
	at    time.Time
}

// Visible reports the skills whose when conditions hold under root, hiding nested skills until
// their parent is loaded. Conditions apply to top-level skills only.
func (c *Conditions) Visible(ctx context.Context, catalog Catalog, root string, loaded map[string]bool) []Skill {
	var out []Skill
	for _, skill := range catalog.Skills {
		if skill.Parent != "" {
			if loaded[skill.Parent] {
				out = append(out, skill)
			}
			continue
		}
		if len(skill.WhenFiles) > 0 && !c.anyFile(ctx, root, skill.WhenFiles) {
			continue
		}
		if len(skill.WhenEnv) > 0 && !anyEnv(skill.WhenEnv) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

// AutoLoad reports the top-level skills among items whose auto_load conditions hold: every listed
// kind must hold, and within a kind any entry.
func (c *Conditions) AutoLoad(ctx context.Context, items []Skill, root string, touched []string, declined map[string]bool) []Skill {
	var out []Skill
	for _, skill := range items {
		if skill.Parent != "" || declined[skill.Name] || len(skill.AutoFiles) == 0 && len(skill.AutoTouched) == 0 {
			continue
		}
		if len(skill.AutoFiles) > 0 && !c.anyFile(ctx, root, skill.AutoFiles) {
			continue
		}
		if len(skill.AutoTouched) > 0 && !touchedAny(root, skill.AutoTouched, touched) {
			continue
		}
		out = append(out, skill)
	}
	return out
}

// Visible evaluates the when conditions without reusing glob results.
func Visible(catalog Catalog, root string, loaded map[string]bool) []Skill {
	return (*Conditions)(nil).Visible(context.Background(), catalog, root, loaded)
}

func anyEnv(names []string) bool {
	for _, name := range names {
		if _, ok := os.LookupEnv(name); ok {
			return true
		}
	}
	return false
}

func (c *Conditions) anyFile(ctx context.Context, root string, patterns []string) bool {
	for _, pattern := range patterns {
		if !strings.ContainsAny(pattern, "*?[{") {
			if _, err := os.Lstat(filepath.Join(root, filepath.FromSlash(pattern))); err == nil {
				return true
			}
			continue
		}
		if c.glob(ctx, root, pattern) {
			return true
		}
	}
	return false
}

func (c *Conditions) glob(ctx context.Context, root, pattern string) bool {
	key := root + "\x00" + pattern
	if c != nil {
		c.mu.Lock()
		cached, ok := c.globs[key]
		c.mu.Unlock()
		if ok && time.Since(cached.at) < globTTL {
			return cached.holds
		}
	}
	found, err := search.Find(ctx, root, pattern, nil, 1)
	holds := err == nil && len(found.Entries) > 0
	if c != nil && err == nil {
		c.mu.Lock()
		if c.globs == nil {
			c.globs = map[string]globResult{}
		}
		c.globs[key] = globResult{holds, time.Now()}
		c.mu.Unlock()
	}
	return holds
}

// touchedAny matches each touched path against the patterns: one without a slash by file name, any
// other against the path below root.
func touchedAny(root string, patterns, touched []string) bool {
	for _, path := range touched {
		relative := filepath.ToSlash(path)
		if rel, err := filepath.Rel(root, path); err == nil && !strings.HasPrefix(rel, "..") {
			relative = filepath.ToSlash(rel)
		}
		for _, pattern := range patterns {
			target := relative
			if !strings.Contains(pattern, "/") {
				target = filepath.Base(path)
			}
			if doublestar.MatchUnvalidated(pattern, target) {
				return true
			}
		}
	}
	return false
}
