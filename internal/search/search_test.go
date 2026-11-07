package search

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
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
	write(t, root, "large.txt", strings.Repeat("x", 100000)+" needle\n")
	limited, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, Limits: Limits{Context: 1}})
	if err != nil {
		t.Fatal(err)
	}
	if limited.Complete {
		t.Fatalf("oversized result claimed to be complete: %+v", limited)
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
