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

// TestReadOfAHugeFileStaysBounded measures SPEC-0006 §8.10: reading one page of a 128 MiB text
// file must not allocate in proportion to the file.
func TestReadOfAHugeFileStaysBounded(t *testing.T) {
	if testing.Short() {
		t.Skip("writes a 128 MiB fixture")
	}
	path := filepath.Join(t.TempDir(), "huge.txt")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := bufio.NewWriterSize(f, 1<<20)
	for written, i := 0, 0; written < 128<<20; i++ {
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
	page, err := ReadFile(context.Background(), target, Budget{Bytes: 50 << 10, Lines: 2000})
	elapsed := time.Since(start)
	runtime.ReadMemStats(&after)
	if err != nil {
		t.Fatal(err)
	}
	allocated := after.TotalAlloc - before.TotalAlloc
	t.Logf("128 MiB file: first page %d bytes in %s, %d MiB allocated in total", len(page.Content), elapsed.Round(time.Millisecond), allocated>>20)
	if len(page.Content) > 60<<10 {
		t.Fatalf("page of %d bytes exceeds the budget", len(page.Content))
	}
	if allocated > 48<<20 {
		t.Fatalf("read of a 128 MiB file allocated %d MiB; memory must not scale with file size", allocated>>20)
	}
}
