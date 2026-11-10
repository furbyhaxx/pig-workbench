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

func TestTaggedReadHidesBOMWithoutChangingRawBytes(t *testing.T) {
	source := "\ufeffone\r\ntwo\r\n"
	tagged := readFixture(t, source, "", Budget{Bytes: 4096, Lines: 100})
	if strings.Contains(tagged.Content, "\ufeff") || !strings.Contains(tagged.Content, "1:one") {
		t.Fatalf("BOM leaked into logical source rows: %q", tagged.Content)
	}
	raw := readFixture(t, source, ":raw", Budget{Bytes: 4096, Lines: 100})
	if raw.Content != source {
		t.Fatal("raw source bytes changed")
	}
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
	if _, err := ReadFile(context.Background(), target, Budget{Bytes: 4096, Lines: 100}); err == nil {
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
	if _, err := ReadFile(ctx, target, Budget{Bytes: 4096, Lines: 100}); err == nil {
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

func TestDirectoryReadIsAStableListing(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "sub"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "b.txt"), []byte("hello"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "a.txt"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := ParsePath(filepath.Dir(root), root, "")
	if err != nil {
		t.Fatal(err)
	}
	page, err := (FileProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: root, Budget: Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(page.Content, "\n"), "\n")
	if len(lines) != 4 || lines[0] != "["+target.URI+"]" || lines[1] != "1:sub/" || lines[2] != "2:a.txt (1 bytes)" || lines[3] != "3:b.txt (5 bytes)" {
		t.Fatalf("directory view is not a deterministic listing: %q", page.Content)
	}
	if page.Facts.CoordinateSpace != "directory-lines" || !page.Facts.ResourceComplete {
		t.Fatalf("directory facts wrong: %+v", page.Facts)
	}
	raw, err := ParseSelector(":raw")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (FileProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: root, Selector: raw, Budget: Budget{Bytes: 4096, Lines: 100}}); err == nil {
		t.Fatal("raw directory read accepted")
	}
	multi, err := ParseSelector(":1,3")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := (FileProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: root, Selector: multi, Budget: Budget{Bytes: 4096, Lines: 100}}); err == nil {
		t.Fatal("multi-range directory read accepted")
	}
	selected, err := ParseSelector(":-1")
	if err != nil {
		t.Fatal(err)
	}
	partial, err := (FileProvider{}).Read(context.Background(), ReadRequest{URI: target.URI, Requested: root, Selector: selected, Budget: Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(partial.Content, "sub/") || !strings.Contains(partial.Content, "b.txt") {
		t.Fatalf("directory tail not honoured: %q", partial.Content)
	}
	if !partial.Facts.SelectionComplete || partial.Facts.ResourceComplete {
		t.Fatalf("directory tail facts wrong: %+v", partial.Facts)
	}
}

func TestColumnCapCutsTheDisplayedLineAndLeavesItUnseen(t *testing.T) {
	path := filepath.Join(t.TempDir(), "wide.txt")
	if err := os.WriteFile(path, []byte("short\n"+strings.Repeat("é", 40)+"\nlast\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	p, err := ReadFile(context.Background(), target, Budget{Bytes: 4096, Lines: 100, Columns: 10})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(p.Content, "2:"+strings.Repeat("é", 10)+"… (+60 bytes)\n3:last") {
		t.Fatalf("line is not cut at ten characters with its omitted bytes:\n%s", p.Content)
	}
	if !reflect.DeepEqual(p.FullyShownRanges, []Range{{1, 1}, {3, 3}}) || !reflect.DeepEqual(p.ShownRanges, []Range{{1, 3}}) {
		t.Fatalf("coverage shown=%v fully=%v", p.ShownRanges, p.FullyShownRanges)
	}
	if !p.SelectionComplete || p.ResourceComplete || p.Truncation != "columns" || p.Continuation != "" {
		t.Fatalf("completeness facts: %+v", p)
	}
	raw, err := ParsePath(filepath.Dir(path), path, "2:raw")
	if err != nil {
		t.Fatal(err)
	}
	whole, err := ReadFile(context.Background(), raw, Budget{Bytes: 4096, Lines: 100, Columns: 10})
	if err != nil || !strings.Contains(whole.Content, strings.Repeat("é", 40)) || !reflect.DeepEqual(whole.FullyShownRanges, []Range{{2, 2}}) {
		t.Fatalf("raw read is capped or not seen: %v %+v", err, whole)
	}
}
