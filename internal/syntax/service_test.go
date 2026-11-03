package syntax

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
)

func fixture(t *testing.T, name, source string) (string, []byte) {
	t.Helper()
	path := filepath.Join(t.TempDir(), name)
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, data
}

func analyzeFixture(t *testing.T, name, source string) (Summary, map[int]Block) {
	t.Helper()
	path, data := fixture(t, name, source)
	summary, blocks, err := New().Analyze(context.Background(), "revision", path, data)
	if err != nil {
		t.Fatal(err)
	}
	return summary, blocks
}

func requireBlock(t *testing.T, blocks map[int]Block, line, end, column int, kind string, ambiguous bool) {
	t.Helper()
	want := Block{Kind: kind, StartLine: line, EndLine: end, StartColumn: column, Ambiguous: ambiguous}
	if got := blocks[line]; got != want {
		t.Fatalf("block at %d = %+v, want %+v (all: %+v)", line, got, want, blocks)
	}
}

func TestGoBlockAnchors(t *testing.T) {
	source := "package sample\n\nfunc first() {\n\tif true {\n\t\tprintln(1)\n\t}\n}\n\nfunc second() {\n\tprintln(2)\n}\n"
	summary, blocks := analyzeFixture(t, "sample.go", source)
	if summary.Language != "go" || summary.ParseErrors {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	requireBlock(t, blocks, 3, 7, 1, "function_declaration", false)
	requireBlock(t, blocks, 4, 6, 2, "if_statement", false)
	requireBlock(t, blocks, 9, 11, 1, "function_declaration", false)
	for _, line := range []int{1, 5, 6, 7, 8, 10, 11} {
		if b, ok := blocks[line]; ok {
			t.Errorf("invalid anchor %d: %+v", line, b)
		}
	}
}

func TestBrokenBlockIsAmbiguousButOtherFunctionIsUsable(t *testing.T) {
	for _, expression := range []string{"x :=", "x := 1 + / 3"} {
		t.Run(expression, func(t *testing.T) {
			source := "package sample\n\nfunc good() {\n\tprintln(1)\n}\n\nfunc broken() {\n\tif true {\n\t\t" + expression + "\n\t}\n}\n"
			summary, blocks := analyzeFixture(t, "broken.go", source)
			if !summary.ParseErrors {
				t.Fatal("missing parse error")
			}
			requireBlock(t, blocks, 3, 5, 1, "function_declaration", false)
			requireBlock(t, blocks, 7, 11, 1, "function_declaration", true)
			requireBlock(t, blocks, 8, 10, 2, "if_statement", true)
		})
	}
}

func TestContinuationIsNotAnAnchor(t *testing.T) {
	source := "package sample\n\nfunc first() {\n\tprintln(\n\t\tcall(\n\t\t\t1,\n\t\t),\n\t)\n}\n"
	_, blocks := analyzeFixture(t, "continuation.go", source)
	if b, ok := blocks[5]; ok {
		t.Fatalf("continuation became a block anchor: %+v", b)
	}
}

func TestPythonIndentationAndDecorators(t *testing.T) {
	source := "@decorator\ndef outer():\n    if True:\n        print(1)\n    print(2)\n\ndef other():\n    return 3\n"
	_, blocks := analyzeFixture(t, "sample.py", source)
	requireBlock(t, blocks, 1, 5, 1, "decorated_definition", false)
	requireBlock(t, blocks, 2, 5, 1, "function_definition", false)
	requireBlock(t, blocks, 3, 4, 5, "if_statement", false)
	if b, ok := blocks[4]; ok {
		t.Fatalf("statement-sequence container became an anchor: %+v", b)
	}
}

func TestMarkdownHeadingSectionsRespectFences(t *testing.T) {
	source := "# One\nintro\n```go\n# Not a heading\n```\n## Nested\ntext\n\n# Two\nend\n"
	_, blocks := analyzeFixture(t, "sample.md", source)
	requireBlock(t, blocks, 1, 8, 1, "section", false)
	requireBlock(t, blocks, 6, 8, 1, "section", false)
	requireBlock(t, blocks, 9, 10, 1, "section", false)
	if b, ok := blocks[4]; ok {
		t.Fatalf("fenced code became a heading: %+v", b)
	}
}

func TestMarkdownIndentedAndSetextHeadings(t *testing.T) {
	source := "  # Indented\nbody\n\nOther\n=====\ntext\n"
	_, blocks := analyzeFixture(t, "headings.md", source)
	requireBlock(t, blocks, 1, 3, 3, "section", false)
	requireBlock(t, blocks, 4, 6, 1, "section", false)
}

func TestSummaryPreservesHeaderCoordinatesAndOmissions(t *testing.T) {
	var source strings.Builder
	source.WriteString("package sample\n\nfunc first(\n\targ int,\n) int {\n")
	for i := 0; i < 14; i++ {
		source.WriteString("\targ++\n")
	}
	source.WriteString("\treturn arg\n}\n\nfunc second() {}\n")
	summary, _ := analyzeFixture(t, "summary.go", source.String())
	var first, second *Declaration
	for i := range summary.Declarations {
		switch summary.Declarations[i].Line {
		case 3:
			first = &summary.Declarations[i]
		case 23:
			second = &summary.Declarations[i]
		}
	}
	if first == nil || first.Kind != "function_declaration" || first.Indent != 0 || first.Text != "func first(\n\targ int,\n) int {" {
		t.Fatalf("first declaration loses signature: %+v", summary.Declarations)
	}
	if second == nil || second.Text != "func second() {}" {
		t.Fatalf("second declaration lost true line 23: %+v", summary.Declarations)
	}
	if !reflect.DeepEqual(summary.Omitted, [][2]int{{6, 20}}) {
		t.Fatalf("omitted ranges = %v", summary.Omitted)
	}
}

func TestFoldThresholds(t *testing.T) {
	var source strings.Builder
	source.WriteString("package sample\n")
	for i := 0; i < 8; i++ {
		fmt.Fprintf(&source, "// comment %d\n", i)
	}
	source.WriteString("func short() {\n")
	for i := 0; i < 11; i++ {
		source.WriteString("println(1)\n")
	}
	source.WriteString("}\nfunc long() {\n")
	for i := 0; i < 12; i++ {
		source.WriteString("println(2)\n")
	}
	source.WriteString("}\n")
	summary, _ := analyzeFixture(t, "thresholds.go", source.String())
	if !reflect.DeepEqual(summary.Omitted, [][2]int{{3, 8}, {24, 35}}) {
		t.Fatalf("threshold folds = %v", summary.Omitted)
	}
}

func TestRejectionsAndCancellation(t *testing.T) {
	service := New()
	path, source := fixture(t, "unknown.unregistered_extension_xyz", "plain text\n")
	if _, _, err := service.Analyze(context.Background(), "r", path, source); !errors.Is(err, ErrNoGrammar) {
		t.Fatalf("unknown extension: %v", err)
	}
	path, source = fixture(t, "large.go", strings.Repeat("x", 4*1024*1024+1))
	if _, _, err := service.Analyze(context.Background(), "r", path, source); !errors.Is(err, ErrTooLarge) {
		t.Fatalf("oversized source: %v", err)
	}
	for _, bad := range []string{"package sample\n//\xff\n", "package sample\n//\x00\n"} {
		path, source := fixture(t, "bad.go", bad)
		if _, _, err := service.Analyze(context.Background(), "r", path, source); !errors.Is(err, ErrUnparseable) {
			t.Fatalf("invalid source should use text fallback: %v", err)
		}
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, _, err := service.Analyze(ctx, "r", path, source); !errors.Is(err, context.Canceled) {
		t.Fatalf("cancelled request: %v", err)
	}
}

type admissionContext struct {
	context.Context
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (c *admissionContext) Done() <-chan struct{} {
	c.once.Do(func() {
		close(c.entered)
		<-c.release
	})
	return c.Context.Done()
}

func TestCancellationDuringAnalysis(t *testing.T) {
	path, source := fixture(t, "cancel.go", "package sample\nfunc f() {\n println(1)\n}\n")
	base, cancel := context.WithCancel(context.Background())
	ctx := &admissionContext{Context: base, entered: make(chan struct{}), release: make(chan struct{})}
	result := make(chan error, 1)
	go func() {
		_, _, err := New().Analyze(ctx, "r", path, source)
		result <- err
	}()
	<-ctx.entered
	cancel()
	close(ctx.release)
	if err := <-result; !errors.Is(err, context.Canceled) {
		t.Fatalf("cancellation during analysis: %v", err)
	}
}

func TestCacheIsolationAndExactRevision(t *testing.T) {
	service := New()
	path, source := fixture(t, "cache.go", "package sample\nfunc first() {\n println(1)\n}\n")
	one, blocks, err := service.Analyze(context.Background(), "same", path, source)
	if err != nil {
		t.Fatal(err)
	}
	two, again, err := service.Analyze(context.Background(), "same", path, source)
	if err != nil || !reflect.DeepEqual(one, two) || !reflect.DeepEqual(blocks, again) {
		t.Fatalf("cache hit differs: %+v %+v %v", two, again, err)
	}
	delete(again, 2)
	if len(two.Declarations) > 0 {
		two.Declarations[0].Text = "changed"
	}
	three, retained, err := service.Analyze(context.Background(), "same", path, source)
	if err != nil || !reflect.DeepEqual(one, three) || !reflect.DeepEqual(blocks, retained) {
		t.Fatal("caller mutated cached result")
	}
	changed := []byte("package sample\nfunc changed() {\n println(1)\n println(2)\n}\n")
	_, changedBlocks, err := service.Analyze(context.Background(), "same", path, changed)
	if err != nil {
		t.Fatal(err)
	}
	if got := changedBlocks[2].EndLine; got != 5 {
		t.Fatalf("reused stale content with same revision: end = %d", got)
	}
}

func TestCacheEvictionAndCanonicalIdentity(t *testing.T) {
	service := New()
	path, source := fixture(t, "cache.go", "package sample\nfunc f() {\n println(1)\n}\n")
	link := filepath.Join(filepath.Dir(path), "link.go")
	if err := os.Symlink(path, link); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{path, link, "file://" + path} {
		if _, _, err := service.Analyze(context.Background(), "hot", name, source); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(service.cache.entries); got != 1 {
		t.Fatalf("equivalent identities created %d entries", got)
	}
	for i := 0; i < maxCacheEntries; i++ {
		if _, _, err := service.Analyze(context.Background(), fmt.Sprint(i), path, source); err != nil {
			t.Fatal(err)
		}
	}
	if got := len(service.cache.entries); got != maxCacheEntries || service.cache.bytes > maxCacheBytes {
		t.Fatalf("unbounded cache: %d entries, %d bytes", got, service.cache.bytes)
	}
	if _, _, err := service.Analyze(context.Background(), "hot", path, source); err != nil {
		t.Fatal(err)
	}
	if got := len(service.cache.entries); got != maxCacheEntries {
		t.Fatalf("cache did not evict an old revision: %d", got)
	}
}

func TestConcurrentAnalyze(t *testing.T) {
	service := New()
	path, source := fixture(t, "concurrent.go", "package sample\nfunc f() {\n println(1)\n}\n")
	var wg sync.WaitGroup
	for i := 0; i < 12; i++ {
		wg.Go(func() {
			for j := 0; j < 3; j++ {
				_, blocks, err := service.Analyze(context.Background(), fmt.Sprint(j%2), path, source)
				if err != nil || blocks[2].EndLine != 4 {
					t.Errorf("concurrent analysis: %+v %v", blocks, err)
				}
			}
		})
	}
	wg.Wait()
}
