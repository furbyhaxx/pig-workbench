package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func write(t *testing.T, root, name, body string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(body), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

type changingRevisionProvider struct {
	resource.Provider
	reads int
}

func (p *changingRevisionProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	page, err := p.Provider.Read(ctx, request)
	p.reads++
	if p.reads > 1 {
		page.Facts.Revision = "changed"
	}
	return page, err
}

func TestGrepProviderPagesAndHonoursBoundaries(t *testing.T) {
	root := t.TempDir()
	body := strings.Repeat("ordinary source line\n", 2499) + "needle at end\n"
	write(t, root, "nested/data.txt", body)
	outside := t.TempDir()
	write(t, outside, "secret.txt", "needle outside\n")
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	options := Options{Root: resource.FileURI(root), Patterns: []string{"needle"}, Literal: true, Glob: "*.txt", Limits: Limits{Context: 1}}
	result, err := GrepProvider(context.Background(), resource.FileProvider{}, options)
	if err != nil || !result.Complete || len(result.Matches) != 1 {
		t.Fatalf("provider paging: %+v %v", result, err)
	}
	match := result.Matches[0]
	if match.Revision == "" || match.Line != 2500 || match.Column != 1 || len(match.Before) != 1 || match.Before[0].Line != 2499 || !strings.Contains(match.Path, "nested/data.txt") {
		t.Fatalf("provider match lost coordinates or context: %+v", match)
	}
	unstable := &changingRevisionProvider{Provider: resource.FileProvider{}}
	changed, err := GrepProvider(context.Background(), unstable, options)
	if err != nil || changed.Complete || len(changed.Skipped) != 1 || len(changed.Matches) != 0 {
		t.Fatalf("changed provider revision was mixed: %+v %v", changed, err)
	}
	options.Limits.MaxFileBytes = 100
	limited, err := GrepProvider(context.Background(), resource.FileProvider{}, options)
	if err != nil || limited.Complete || len(limited.Skipped) != 1 || len(limited.Matches) != 0 {
		t.Fatalf("provider size limit not reported: %+v %v", limited, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := GrepProvider(ctx, resource.FileProvider{}, options); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled provider walk: %v", err)
	}
}

func TestGrepBoundsAndHonesty(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.go", "package a\n// Needle one\nfunc A() {}\n")
	write(t, root, "sub/b.go", "package b\n// needle two\n")
	write(t, root, "binary.bin", "abc\x00needle")
	result, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, IgnoreCase: true, Limits: Limits{MaxMatches: 10, MaxFiles: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 2 || !result.Complete {
		t.Fatalf("unexpected matches: %+v", result)
	}
	if result.Matches[0].Line != 2 || result.Matches[0].Column != 4 {
		t.Fatalf("wrong coordinates: %+v", result.Matches[0])
	}
	if len(result.Skipped) != 1 || filepath.Base(result.Skipped[0]) != "binary.bin" {
		t.Fatalf("binary file not reported as skipped: %+v", result.Skipped)
	}
	limited, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, IgnoreCase: true, Limits: Limits{MaxMatches: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Complete || len(limited.Matches) != 1 {
		t.Fatalf("limit reported as complete: %+v", limited)
	}
}

func TestGrepContextAndOutputBudget(t *testing.T) {
	root := t.TempDir()
	write(t, root, "context.txt", "one\nneedle\nthree\nfour\n")
	result, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, Limits: Limits{Context: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Matches) != 1 || len(result.Matches[0].Before) != 1 || result.Matches[0].Before[0].Line != 1 || result.Matches[0].Before[0].Text != "one" || len(result.Matches[0].After) != 1 || result.Matches[0].After[0].Line != 3 || result.Matches[0].After[0].Text != "three" {
		t.Fatalf("unexpected surrounding lines: %+v", result)
	}
	write(t, root, "large.txt", strings.Repeat("x", 100000)+"\nneedle\n")
	limited, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, Limits: Limits{Context: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Complete || len(limited.Matches) != 2 {
		t.Fatalf("oversized context hid a matching line or claimed completeness: %+v", limited)
	}
}

func TestGrepWindowsLongMatchingLines(t *testing.T) {
	root := t.TempDir()
	for name, prefix := range map[string]int{"start": 0, "middle": 50000, "end": 100000} {
		line := strings.Repeat("é", prefix/2) + "needle" + strings.Repeat("é", (100000-prefix)/2)
		path := write(t, root, name, line+"\n")
		result, err := Grep(context.Background(), Options{Root: path, Patterns: []string{"needle"}, Literal: true})
		if err != nil || len(result.Matches) != 1 {
			t.Fatalf("%s: matching line was lost: %+v %v", name, result, err)
		}
		match := result.Matches[0]
		if match.Column != prefix+1 || !strings.Contains(match.Text, "needle") || len(match.Text) > 8192 || !utf8.ValidString(match.Text) || !match.Windowed || match.WindowStart < 1 || result.Complete {
			t.Errorf("%s: invalid match window: %+v complete=%v", name, match, result.Complete)
		}
	}
}

func TestGrepValidationAndCancellation(t *testing.T) {
	root := t.TempDir()
	write(t, root, "a.txt", "x\n")
	if _, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"("}}); !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("invalid regex accepted: %v", err)
	}
	if _, err := Grep(context.Background(), Options{Root: root, Patterns: []string{""}}); err == nil {
		t.Fatal("empty pattern accepted")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := Grep(ctx, Options{Root: root, Patterns: []string{"x"}}); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation ignored: %v", err)
	}
}

func TestFindGlobAndExclude(t *testing.T) {
	root := t.TempDir()
	write(t, root, "one.go", "package one\n")
	write(t, root, "sub/two.go", "package two\n")
	write(t, root, "sub/three.txt", "three\n")
	write(t, root, "vendor/four.go", "package four\n")
	result, err := Find(context.Background(), root, "*.go", []string{"vendor"}, 10)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Entries) != 2 || !result.Complete {
		t.Fatalf("unexpected find result: %+v", result)
	}
	for _, entry := range result.Entries {
		if filepath.Base(entry.Path) == "four.go" {
			t.Fatal("excluded directory was walked")
		}
	}
	if _, err := Find(context.Background(), root, "[", nil, 5); !errors.Is(err, ErrInvalidPattern) {
		t.Fatalf("invalid glob accepted: %v", err)
	}
}
