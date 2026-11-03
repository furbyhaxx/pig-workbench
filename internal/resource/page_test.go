package resource

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func readFixture(t *testing.T, source, selector string, budget Budget) Page {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source")
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := ParsePath(filepath.Dir(path), path, selector)
	if err != nil {
		t.Fatal(err)
	}
	page, err := ReadFile(context.Background(), target, budget)
	if err != nil {
		t.Fatal(err)
	}
	return page
}

func TestRawPreservesBytesAndLineConvention(t *testing.T) {
	for _, source := range []string{"", "a", "a\r\nb\r\n", "é\n\n", "a\n"} {
		p := readFixture(t, source, ":raw", Budget{Bytes: 4096, Lines: 100})
		if p.Content != source || !p.SelectionComplete || !p.ResourceComplete {
			t.Fatalf("raw mismatch: %+v", p)
		}
		want := strings.Count(source, "\n")
		if source != "" && !strings.HasSuffix(source, "\n") {
			want++
		}
		if p.TotalLines != want {
			t.Fatalf("line count %d != %d", p.TotalLines, want)
		}
	}
}

func TestBoundedPagingTracksCoverageAndContinuation(t *testing.T) {
	var source strings.Builder
	for i := 1; i <= 30; i++ {
		fmt.Fprintf(&source, "line %d\n", i)
	}
	p := readFixture(t, source.String(), ":5-10:raw", Budget{Bytes: 4096, Lines: 100})
	if !p.SelectionComplete || p.ResourceComplete || !reflect.DeepEqual(p.FullyShownRanges, []Range{{5, 10}}) {
		t.Fatalf("wrong selection facts: %+v", p)
	}
	p = readFixture(t, source.String(), "", Budget{Bytes: 1024, Lines: 8})
	if p.SelectionComplete || p.ResourceComplete || p.Continuation == "" {
		t.Fatalf("cut page reported complete: %+v", p)
	}
	if len(p.Content) > 1024 || strings.Count(p.Content, "\n") > 8 {
		t.Fatal("budget exceeded")
	}
}

func TestLargeLineNeverBecomesSeen(t *testing.T) {
	p := readFixture(t, strings.Repeat("界", 100000)+"\nend\n", "", Budget{Bytes: 1024, Lines: 20})
	if len(p.FullyShownRanges) != 0 || p.Truncation != "partialLine" || !strings.Contains(p.Continuation, ":1-2") {
		t.Fatalf("partial line seen/skipped: %+v", p)
	}
	if len(p.Content) > 1024 {
		t.Fatal("oversize result")
	}
}

func TestSpecialFileAndCancellation(t *testing.T) {
	target, err := ParsePath(t.TempDir(), os.DevNull, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ReadFile(context.Background(), target, Budget{4096, 100}); err == nil {
		t.Fatal("read device")
	}
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("text"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err = ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := ReadFile(ctx, target, Budget{4096, 100}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestTaggedHeaderOnlyForExactNonRawReads(t *testing.T) {
	source := "package a\r\n\r\nfunc f() {}\r\n"
	page := readFixture(t, source, "", Budget{Bytes: 4096, Lines: 100})
	if page.Tag == "" || len(page.Tag) != 4 {
		t.Fatalf("missing hashline tag: %q", page.Tag)
	}
	if !strings.Contains(page.Content, "#"+page.Tag+"]") {
		t.Fatalf("tag absent from header: %q", page.Content)
	}
	raw := readFixture(t, source, ":raw", Budget{Bytes: 4096, Lines: 100})
	if raw.Tag != "" || strings.Contains(raw.Content, "#") {
		t.Fatalf("raw read exposed a tag: %+v", raw)
	}
	changed := readFixture(t, "package b\n", "", Budget{Bytes: 4096, Lines: 100})
	if changed.Tag == page.Tag {
		t.Fatal("different content produced the same tag")
	}
	stable := readFixture(t, "package a\n\nfunc f() {}\n", "", Budget{Bytes: 4096, Lines: 100})
	if stable.Tag != page.Tag {
		t.Fatalf("tag is not stable across CRLF/LF: %q vs %q", stable.Tag, page.Tag)
	}
}
