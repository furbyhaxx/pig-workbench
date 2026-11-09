package search

import (
	"context"
	"net/url"
	"path"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// Via values report which engine answered a provider search.
const (
	ViaProvider = "provider"
	ViaWalk     = "walk"
)

const (
	providerSearchBytes = 48 << 10
	providerSearchLines = 1900
	maxProviderLine     = 2000
)

// searcher returns the provider's own search when it declares one.
func searcher(provider resource.Provider) (resource.Searcher, bool) {
	s, ok := provider.(resource.Searcher)
	return s, ok && s.CanSearch()
}

func uriName(uri string) string {
	if u, err := url.Parse(uri); err == nil && u.Path != "" {
		return path.Base(strings.TrimSuffix(u.Path, "/"))
	}
	return path.Base(uri)
}

// grepSearcher runs grep on the provider. resource.ErrUnsupported means the query needs a walk.
func grepSearcher(ctx context.Context, s resource.Searcher, options Options, limits Limits) (Result, error) {
	found, err := s.Search(ctx, resource.SearchRequest{
		Root: options.Root, Mode: "content", Patterns: options.Patterns, Literal: options.Literal, IgnoreCase: options.IgnoreCase,
		Glob: options.Glob, Exclude: options.Exclude, Context: limits.Context, MaxMatches: limits.MaxMatches,
		Budget: resource.Budget{Bytes: providerSearchBytes, Lines: providerSearchLines},
	})
	if err != nil {
		return Result{}, err
	}
	result := Result{Complete: found.Complete}
	remainingBytes, remainingLines := providerSearchBytes, providerSearchLines
	for _, hit := range found.Hits {
		if excluded(options.Root, hit.URI, options.Exclude) || options.Glob != "" && !matchGlob(options.Glob, options.Root, hit.URI, uriName(hit.URI)) {
			continue
		}
		match := Match{Path: hit.URI, Revision: hit.Revision, Line: hit.Line, Column: max(hit.Column, 1), Text: hit.Text}
		if len(match.Text) > maxProviderLine {
			match.Text = strings.ToValidUTF8(match.Text[:maxProviderLine], "")
			match.Windowed = true
		}
		for _, line := range hit.Before {
			match.Before = append(match.Before, ContextLine{Line: line.Line, Text: line.Text})
		}
		for _, line := range hit.After {
			match.After = append(match.After, ContextLine{Line: line.Line, Text: line.Text})
		}
		if !appendMatches(&result, []Match{match}, limits.MaxMatches, &remainingBytes, &remainingLines) {
			break
		}
	}
	return result, nil
}

// findSearcher runs find on the provider. resource.ErrUnsupported means the query needs a walk.
func findSearcher(ctx context.Context, s resource.Searcher, root, pattern string, exclude []string, limit int) (FindResult, error) {
	found, err := s.Search(ctx, resource.SearchRequest{
		Root: root, Mode: "names", Patterns: []string{pattern}, Exclude: exclude, MaxMatches: limit,
		Budget: resource.Budget{Bytes: providerSearchBytes, Lines: providerSearchLines},
	})
	if err != nil {
		return FindResult{}, err
	}
	result := FindResult{Complete: found.Complete}
	remaining := providerSearchBytes
	for _, hit := range found.Hits {
		if hit.Dir || excluded(root, hit.URI, exclude) || !matchGlob(pattern, root, hit.URI, uriName(hit.URI)) {
			continue
		}
		if len(result.Entries) >= limit || len(hit.URI)+1 > remaining {
			result.Complete = false
			break
		}
		result.Entries = append(result.Entries, FindEntry{Path: hit.URI, Size: hit.Size})
		remaining -= len(hit.URI) + 1
	}
	return result, nil
}
