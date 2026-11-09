package resource

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"
)

// firstPageCost reads one page of a generated text file of the given size and reports the bytes
// allocated by the read and how long it took.
func firstPageCost(t *testing.T, size int) (allocated uint64, page Page, elapsed time.Duration) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "huge.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for written, i := 0, 0; written < size; i++ {
		n, _ := fmt.Fprintf(w, "line %08d with some ordinary source-like text content\n", i)
		written += n
	}
	if err := w.Flush(); err != nil {
		t.Fatal(err)
	}
	f.Close()
	target, err := ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	start := time.Now()
	page, err = ReadFile(context.Background(), target, Budget{Bytes: 50 << 10, Lines: 2000})
	elapsed = time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	return after.TotalAlloc - before.TotalAlloc, page, elapsed
}

// TestReadOfAHugeFileStaysBounded measures SPEC-0006 §8.10: reading one page of a 128 MiB text
// file must not allocate in proportion to the file. The comparison against a 32 MiB file keeps the
// check independent of the race detector's constant allocation overhead.
func TestReadOfAHugeFileStaysBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 128 MiB fixture")
	}
	small, _, _ := firstPageCost(t, 32<<20)
	large, page, elapsed := firstPageCost(t, 128<<20)
	t.Logf("first page %d bytes in %s; allocated %d MiB (32 MiB file) and %d MiB (128 MiB file)", len(page.Content), elapsed.Round(time.Millisecond), small>>20, large>>20)
	if len(page.Content) > 60<<10 {
		t.Fatalf("page of %d bytes exceeds the budget", len(page.Content))
	}
	if large > small+8<<20 {
		t.Fatalf("allocation grew from %d MiB to %d MiB with a 4x larger file; memory must not scale with file size", small>>20, large>>20)
	}
}
