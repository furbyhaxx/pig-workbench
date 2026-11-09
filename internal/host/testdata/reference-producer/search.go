package fixture

import (
	"fmt"
	"os"
	"regexp"
	"strings"

	"github.com/furbyhaxx/pig-workbench/api"
)

// indexedDocuments back the idx:// provider, which declares the search capability.
var indexedDocuments = map[string]string{
	"idx://articles/errors.md": "errors wrap with context\nretry budget mentioned here too\n",
	"idx://articles/notes.txt": "retry budget in a text file\n",
	"idx://articles/retry.md":  "# Retry\nthe retry budget caps attempts\nbudget exhausted\n",
}

// indexedRanking is the provider's relevance order, deliberately not alphabetical.
var indexedRanking = []string{"idx://articles/retry.md", "idx://articles/notes.txt", "idx://articles/errors.md"}

func executeIndexed(request api.ResourceRequest) (any, *api.Error) {
	switch request.Operation {
	case "stat":
		if request.URI == "idx://articles/" || request.URI == "idx://articles" {
			return api.ResourceResult{CanonicalURI: "idx://articles/", Kind: "dir"}, nil
		}
		text, ok := indexedDocuments[request.URI]
		if !ok {
			return nil, &api.Error{Code: "notFound", Message: "no such article"}
		}
		return api.ResourceResult{CanonicalURI: request.URI, Kind: "file", Size: int64(len(text)), Revision: "i1"}, nil
	case "list":
		complete := true
		entries := []api.ResourceEntry{}
		for _, uri := range []string{"idx://articles/errors.md", "idx://articles/notes.txt", "idx://articles/retry.md"} {
			entries = append(entries, api.ResourceEntry{Name: strings.TrimPrefix(uri, "idx://articles/"), URI: uri, Kind: "file", Size: int64(len(indexedDocuments[uri]))})
		}
		return api.ResourceResult{CanonicalURI: "idx://articles/", Kind: "dir", Complete: &complete, Entries: entries}, nil
	case "read":
		text, ok := indexedDocuments[request.URI]
		if !ok {
			return nil, &api.Error{Code: "notFound", Message: "no such article"}
		}
		total := strings.Count(text, "\n")
		return api.ResourceResult{CanonicalURI: request.URI, Kind: "file", Revision: "i1", Page: &api.TextPage{Text: text, ShownRanges: []api.LineRange{{Start: 1, End: total}}, TotalLines: &total, SelectionComplete: true, ResourceComplete: true}}, nil
	case "search":
		return searchIndexed(request)
	default:
		return nil, &api.Error{Code: "unsupported", Message: "operation is not provided"}
	}
}

func searchIndexed(request api.ResourceRequest) (any, *api.Error) {
	query := request.Search
	if query == nil {
		return nil, &api.Error{Code: "invalidRequest", Message: "search query missing"}
	}
	if marker := os.Getenv("WORKBENCH_SEARCH_FIXTURE_REQUESTS"); marker != "" {
		file, err := os.OpenFile(marker, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
		if err == nil {
			fmt.Fprintf(file, "%s %s %v\n", query.Mode, request.URI, query.Patterns)
			file.Close()
		}
	}
	if !query.Literal && query.Mode == "content" {
		return nil, &api.Error{Code: "unsupported", Message: "the full-text index serves literal queries only"}
	}
	pattern := strings.Join(query.Patterns, "|")
	if query.IgnoreCase {
		pattern = strings.ToLower(pattern)
	}
	matches := []api.SearchMatch{}
	for _, uri := range indexedRanking {
		if query.Mode == "names" {
			if ok, _ := regexp.MatchString("^"+strings.NewReplacer(".", `\.`, "*", ".*").Replace(query.Patterns[0])+"$", uri[strings.LastIndex(uri, "/")+1:]); ok {
				matches = append(matches, api.SearchMatch{URI: uri, Kind: "file", Size: int64(len(indexedDocuments[uri]))})
			}
			continue
		}
		for i, line := range strings.Split(strings.TrimSuffix(indexedDocuments[uri], "\n"), "\n") {
			haystack := line
			if query.IgnoreCase {
				haystack = strings.ToLower(line)
			}
			if at := strings.Index(haystack, pattern); at >= 0 {
				matches = append(matches, api.SearchMatch{URI: uri, Line: i + 1, Column: at + 1, Text: line, Revision: "i1"})
			}
		}
	}
	return api.ResourceResult{CanonicalURI: "idx://articles/", Kind: "dir", Matches: matches}, nil
}
