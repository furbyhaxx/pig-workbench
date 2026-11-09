package search

import (
	"context"
	"errors"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// indexedProvider answers search itself and fails the test if a walk touches it.
type indexedProvider struct {
	resource.Provider
	t        *testing.T
	hits     []resource.SearchHit
	complete bool
	err      error
	requests []resource.SearchRequest
}

func (p *indexedProvider) Scheme() string  { return "kb" }
func (p *indexedProvider) CanSearch() bool { return true }
func (p *indexedProvider) Search(_ context.Context, request resource.SearchRequest) (resource.SearchResult, error) {
	p.requests = append(p.requests, request)
	return resource.SearchResult{Hits: p.hits, Complete: p.complete}, p.err
}
func (p *indexedProvider) Stat(context.Context, string) (resource.StatResult, error) {
	p.t.Fatal("search must not stat or walk when the provider searches")
	return resource.StatResult{}, nil
}
func (p *indexedProvider) List(context.Context, string, string, int) (resource.Listing, error) {
	p.t.Fatal("search must not list when the provider searches")
	return resource.Listing{}, nil
}

func TestGrepUsesProviderSearchAndKeepsItsRanking(t *testing.T) {
	provider := &indexedProvider{t: t, complete: false, hits: []resource.SearchHit{
		{URI: "kb://articles/zeta.md", Line: 7, Column: 3, Text: "retry budget explained", Revision: "r2", Before: []resource.HitLine{{Line: 6, Text: "context"}}},
		{URI: "kb://articles/alpha.md", Line: 1, Text: "retry budget again"},
		{URI: "kb://articles/skip.txt", Line: 1, Text: "wrong glob"},
	}}
	options := Options{Root: "kb://articles/", Patterns: []string{"retry budget"}, Literal: true, IgnoreCase: true, Glob: "*.md", Limits: Limits{Context: 1, MaxMatches: 50}}
	result, err := GrepProvider(context.Background(), provider, options)
	if err != nil {
		t.Fatal(err)
	}
	if result.Via != ViaProvider || result.Complete {
		t.Fatalf("via/complete = %q %v, want provider and the provider's incomplete claim", result.Via, result.Complete)
	}
	if len(result.Matches) != 2 || result.Matches[0].Path != "kb://articles/zeta.md" || result.Matches[1].Path != "kb://articles/alpha.md" {
		t.Fatalf("provider ranking was reordered or glob not applied: %+v", result.Matches)
	}
	first := result.Matches[0]
	if first.Line != 7 || first.Column != 3 || first.Revision != "r2" || len(first.Before) != 1 || result.Matches[1].Column != 1 {
		t.Fatalf("coordinates lost: %+v", result.Matches)
	}
	request := provider.requests[0]
	if request.Mode != "content" || !request.Literal || !request.IgnoreCase || request.Glob != "*.md" || request.Context != 1 || request.MaxMatches != 50 || request.Root != "kb://articles/" {
		t.Fatalf("query was not forwarded intact: %+v", request)
	}
}

func TestGrepFallsBackToTheWalkOnlyWhenUnsupported(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "needle\n")
	unsupported := &fallbackProvider{Provider: resource.FileProvider{}, err: resource.ErrUnsupported}
	options := Options{Root: resource.FileURI(root), Patterns: []string{"needle"}, Literal: true}
	result, err := GrepProvider(context.Background(), unsupported, options)
	if err != nil || result.Via != ViaWalk || len(result.Matches) != 1 {
		t.Fatalf("unsupported query should walk and say so: %+v %v", result, err)
	}
	failing := &fallbackProvider{Provider: resource.FileProvider{}, err: errors.New("index offline")}
	if _, err := GrepProvider(context.Background(), failing, options); err == nil || err.Error() != "index offline" {
		t.Fatalf("a provider failure must surface, not be hidden by a walk: %v", err)
	}
	options.Selection.Explicit = true
	explicit, err := GrepProvider(context.Background(), &fallbackProvider{Provider: resource.FileProvider{}, err: errors.New("must not be called")}, Options{Root: resource.FileURI(root + "/a.txt"), Patterns: []string{"needle"}, Literal: true, Selection: options.Selection})
	if err != nil || explicit.Via != ViaWalk {
		t.Fatalf("line-selected search must not use provider search: %+v %v", explicit, err)
	}
}

type fallbackProvider struct {
	resource.Provider
	err error
}

func (p *fallbackProvider) CanSearch() bool { return true }
func (p *fallbackProvider) Search(context.Context, resource.SearchRequest) (resource.SearchResult, error) {
	return resource.SearchResult{}, p.err
}

func TestFindUsesProviderNamesSearch(t *testing.T) {
	provider := &indexedProvider{t: t, complete: true, hits: []resource.SearchHit{
		{URI: "kb://articles/go-errors.md", Size: 12},
		{URI: "kb://articles/sub", Dir: true},
		{URI: "kb://articles/readme.txt"},
	}}
	result, err := FindProvider(context.Background(), provider, "kb://articles/", "*.md", nil, 10)
	if err != nil {
		t.Fatal(err)
	}
	if result.Via != ViaProvider || !result.Complete || len(result.Entries) != 1 || result.Entries[0].Path != "kb://articles/go-errors.md" || result.Entries[0].Size != 12 {
		t.Fatalf("names search = %+v", result)
	}
	if request := provider.requests[0]; request.Mode != "names" || len(request.Patterns) != 1 || request.Patterns[0] != "*.md" {
		t.Fatalf("names query = %+v", request)
	}
}

func TestProviderSearchWindowsAHugeLineAndReportsIncomplete(t *testing.T) {
	provider := &indexedProvider{t: t, complete: true, hits: []resource.SearchHit{{URI: "kb://articles/a.md", Line: 1, Text: strings.Repeat("é", 5000)}}}
	result, err := GrepProvider(context.Background(), provider, Options{Root: "kb://articles/", Patterns: []string{"x"}, Literal: true})
	if err != nil || len(result.Matches) != 1 {
		t.Fatalf("%+v %v", result, err)
	}
	if !result.Matches[0].Windowed || result.Complete || len(result.Matches[0].Text) > maxProviderLine || !utf8.ValidString(result.Matches[0].Text) {
		t.Fatalf("huge line was not windowed honestly: windowed=%v complete=%v len=%d", result.Matches[0].Windowed, result.Complete, len(result.Matches[0].Text))
	}
}
