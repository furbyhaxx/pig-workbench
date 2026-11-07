package resource

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"
)

// WalkAction controls traversal after a provider entry is visited.
type WalkAction uint8

const (
	// WalkContinue continues traversal normally.
	WalkContinue WalkAction = iota
	// WalkSkipDir excludes the entry's subtree.
	WalkSkipDir
	// WalkStop stops with an incomplete result.
	WalkStop
)

// WalkResult reports the amount visited and whether the walk finished.
type WalkResult struct {
	Scanned  int
	Complete bool
}

// Walk visits provider entries without following links or noWalk subtrees.
func Walk(ctx context.Context, provider Provider, root string, visit func(Entry, int) (WalkAction, error)) (WalkResult, error) {
	const maxDepth, maxEntries, maxPages, pageSize = 16, 5000, 5000, 128
	work, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	result := WalkResult{Complete: true}
	if err := ctx.Err(); err != nil {
		return result, err
	}
	requested, requestedQuery, err := walkURI(root)
	if err != nil {
		return result, err
	}
	stat, err := provider.Stat(work, root)
	if err != nil {
		return result, err
	}
	canonical, canonicalQuery, err := walkURI(stat.URI)
	if err != nil {
		return result, err
	}
	if len(requested) != len(canonical) || requestedQuery != canonicalQuery {
		return result, errors.New("provider changed the requested walk root")
	}
	for i := 1; i < len(requested); i++ {
		if requested[i] != canonical[i] {
			return result, errors.New("provider changed the requested walk root")
		}
	}
	if stat.Kind != EntryDir {
		return result, errors.New("provider walk root is not a directory")
	}
	type directory struct {
		uri   string
		depth int
	}
	queue := []directory{{uri: stat.URI}}
	visited := map[string]bool{}
	pages := 0
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		parent, query, _ := walkURI(current.uri)
		key := strings.Join(parent, "\x00") + "\x00" + query
		if visited[key] {
			continue
		}
		visited[key] = true
		cursor := ""
		cursors := map[string]bool{"": true}
		for {
			if err := work.Err(); err != nil {
				if ctx.Err() != nil {
					return result, ctx.Err()
				}
				result.Complete = false
				return result, nil
			}
			if pages >= maxPages || result.Scanned >= maxEntries {
				result.Complete = false
				return result, nil
			}
			listing, err := provider.List(work, current.uri, cursor, min(pageSize, maxEntries-result.Scanned))
			if err != nil {
				if errors.Is(err, context.DeadlineExceeded) && ctx.Err() == nil {
					result.Complete = false
					return result, nil
				}
				return result, err
			}
			pages++
			if len(listing.Entries) > min(pageSize, maxEntries-result.Scanned) {
				return result, errors.New("provider exceeded the requested listing limit")
			}
			for _, entry := range listing.Entries {
				if err := work.Err(); err != nil {
					if ctx.Err() != nil {
						return result, ctx.Err()
					}
					result.Complete = false
					return result, nil
				}
				parts, childQuery, err := walkURI(entry.URI)
				if err != nil || childQuery != query || len(parts) != len(parent)+1 || parts[len(parts)-1] != entry.Name {
					return result, fmt.Errorf("provider returned an entry outside its listed directory: %q", entry.URI)
				}
				for i := range parent {
					if parts[i] != parent[i] {
						return result, fmt.Errorf("provider returned an entry outside its listed directory: %q", entry.URI)
					}
				}
				result.Scanned++
				if entry.Kind == EntryControl {
					continue
				}
				action, err := visit(entry, current.depth+1)
				if err != nil {
					return result, err
				}
				if action == WalkStop {
					result.Complete = false
					return result, nil
				}
				if entry.Kind == EntryDir && !entry.NoWalk && action != WalkSkipDir {
					if current.depth+1 >= maxDepth {
						result.Complete = false
					} else {
						queue = append(queue, directory{uri: entry.URI, depth: current.depth + 1})
					}
				}
			}
			if listing.Complete {
				if listing.NextCursor != "" {
					return result, errors.New("complete provider listing has a continuation")
				}
				break
			}
			if listing.NextCursor == "" {
				result.Complete = false
				break
			}
			if cursors[listing.NextCursor] {
				return result, errors.New("provider listing repeated a cursor")
			}
			cursor = listing.NextCursor
			cursors[cursor] = true
		}
	}
	return result, nil
}

func walkURI(raw string) ([]string, string, error) {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme == "" || u.Opaque != "" || u.User != nil || u.Fragment != "" {
		return nil, "", errors.New("invalid provider walk URI")
	}
	parts := []string{u.Scheme}
	if u.Host != "" {
		parts = append(parts, u.Host)
	}
	if path := strings.Trim(u.Path, "/"); path != "" {
		for _, part := range strings.Split(path, "/") {
			if part == "" || part == "." || part == ".." || strings.ContainsRune(part, 0) {
				return nil, "", errors.New("invalid provider walk URI path")
			}
			parts = append(parts, part)
		}
	}
	return parts, u.RawQuery, nil
}
