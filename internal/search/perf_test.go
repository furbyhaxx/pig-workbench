package search

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// TestSearchOverALargeTreeStaysBoundedAndHonest measures SPEC-0006 §8.10: a 20000-file tree must
// yield a bounded, honestly incomplete result without scaling memory with the tree.
func TestSearchOverALargeTreeStaysBoundedAndHonest(t *testing.T) {
	if testing.Short() {
		t.Skip("creates 20000 files")
	}
	root := t.TempDir()
	for i := 0; i < 20000; i++ {
		dir := filepath.Join(root, fmt.Sprintf("d%03d", i%200))
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, fmt.Sprintf("f%05d.go", i)), []byte("package x\n// needle here\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	grep, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"needle"}, Literal: true})
	grepTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	miss, err := Grep(context.Background(), Options{Root: root, Patterns: []string{"absent-needle"}, Literal: true})
	missTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	start = time.Now()
	find, err := Find(context.Background(), root, "**/*.go", nil, 200)
	findTime := time.Since(start)
	if err != nil {
		t.Fatal(err)
	}
	runtime.ReadMemStats(&after)
	allocated := (after.TotalAlloc - before.TotalAlloc) >> 20
	t.Logf("20000 files, no match: %d files scanned complete=%v in %s", miss.FilesScanned, miss.Complete, missTime.Round(time.Millisecond))
	if miss.Complete || miss.FilesScanned > 5001 {
		t.Fatalf("a miss over 20000 files scanned %d with complete=%v; the file bound must stop it and say so", miss.FilesScanned, miss.Complete)
	}
	t.Logf("20000 files: grep %d matches over %d files complete=%v in %s; find %d paths in %s; %d MiB allocated",
		len(grep.Matches), grep.FilesScanned, grep.Complete, grepTime.Round(time.Millisecond), len(find.Entries), findTime.Round(time.Millisecond), allocated)
	if len(grep.Matches) > 200 || grep.Complete {
		t.Fatalf("grep returned %d matches with complete=%v; the default bound must stop it and say so", len(grep.Matches), grep.Complete)
	}
	if len(find.Entries) > 200 {
		t.Fatalf("find returned %d paths, bound is 200", len(find.Entries))
	}
	if allocated > 64 {
		t.Fatalf("search allocated %d MiB for a bounded result", allocated)
	}
}
