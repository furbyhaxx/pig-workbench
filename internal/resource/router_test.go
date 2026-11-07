package resource

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func TestOutStoreBudgetAndEviction(t *testing.T) {
	store, err := NewOutStore(filepath.Join(t.TempDir(), "out"), 2048)
	if err != nil {
		t.Fatal(err)
	}
	first, err := store.Spill(context.Background(), make([]byte, 1024))
	if err != nil {
		t.Fatal(err)
	}
	second, err := store.Spill(context.Background(), make([]byte, 1024))
	if err != nil {
		t.Fatal(err)
	}
	third, err := store.Spill(context.Background(), make([]byte, 1024))
	if err != nil {
		t.Fatal(err)
	}
	path, _ := store.Path(first)
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("oldest spill was not evicted")
	}
	for _, uri := range []string{second, third} {
		path, err := store.Path(uri)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("retained spill missing: %v", err)
		}
	}
	if _, err := store.Spill(context.Background(), make([]byte, 4096)); err == nil {
		t.Fatal("oversized spill accepted")
	}
	if _, err := store.Path("out://../escape"); err == nil {
		t.Fatal("traversal accepted")
	}
}

func TestLocalProviderContainment(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	provider, err := NewLocalProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := provider.path("local:/../escape"); err == nil {
		t.Fatal("parent traversal accepted")
	}
	if _, err := provider.path("local:absolute"); err != nil {
		t.Fatalf("plain name refused: %v", err)
	}
	for _, uri := range []string{"local:", "local:/", "local://"} {
		path, err := provider.path(uri)
		if err != nil || path != root {
			t.Fatalf("%q did not resolve to the scratch root: %v", uri, err)
		}
	}
}

func TestLocalProviderReadRefusesSymlinkEscape(t *testing.T) {
	root := filepath.Join(t.TempDir(), "scratch")
	provider, err := NewLocalProvider(root)
	if err != nil {
		t.Fatal(err)
	}
	outside := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outside, []byte("outside data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "escape")); err != nil {
		t.Skipf("symlinks unavailable: %v", err)
	}
	page, err := provider.Read(context.Background(), ReadRequest{URI: "local://escape", Budget: Budget{Bytes: 4096, Lines: 100}})
	if err == nil || strings.Contains(page.Content, "outside data") {
		t.Fatalf("scratch read escaped root: page=%+v err=%v", page, err)
	}
	if err := os.Symlink(filepath.Dir(outside), filepath.Join(root, "outside-dir")); err != nil {
		t.Fatal(err)
	}
	page, err = provider.Read(context.Background(), ReadRequest{URI: "local://outside-dir/outside.txt", Budget: Budget{Bytes: 4096, Lines: 100}})
	if err == nil || strings.Contains(page.Content, "outside data") {
		t.Fatalf("scratch read followed an escaping directory: page=%+v err=%v", page, err)
	}
	if err := os.WriteFile(filepath.Join(root, "inside"), []byte("inside data\n"), 0600); err != nil {
		t.Fatal(err)
	}
	page, err = provider.Read(context.Background(), ReadRequest{URI: "local://inside", Budget: Budget{Bytes: 4096, Lines: 100}})
	if err != nil || !strings.Contains(page.Content, "inside data") || page.Facts.CanonicalURI != "local://inside" || strings.Contains(page.Content, root) {
		t.Fatalf("confined read failed or leaked backing path: page=%+v err=%v", page, err)
	}
}

func TestFileProviderListingIsDeterministic(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"b.txt", "A.txt", "z/c.txt"} {
		path := filepath.Join(root, name)
		if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("x"), 0600); err != nil {
			t.Fatal(err)
		}
	}
	listing, err := FileProvider{}.List(context.Background(), FileURI(root), "", 2)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, entry := range listing.Entries {
		names = append(names, entry.Name)
	}
	if len(names) != 2 || names[0] != "z" || names[1] != "A.txt" {
		t.Fatalf("directories-first order wrong: %v", names)
	}
	if listing.Complete || listing.NextCursor != "A.txt" {
		t.Fatalf("paging facts wrong: %+v", listing)
	}
	rest, err := FileProvider{}.List(context.Background(), FileURI(root), "A.txt", 2)
	if err != nil {
		t.Fatal(err)
	}
	if len(rest.Entries) != 1 || rest.Entries[0].Name != "b.txt" || !rest.Complete {
		t.Fatalf("continuation wrong: %+v", rest)
	}
	if _, err := (FileProvider{}).Stat(context.Background(), FileURI(filepath.Join(root, "absent"))); !errors.Is(err, ErrNotFound) {
		t.Fatal("absent resource not reported")
	}
}

func TestRouterDispatch(t *testing.T) {
	router := NewRouter()
	for _, prov := range []Provider{FileProvider{}, &LocalProvider{root: t.TempDir()}} {
		if err := router.RegisterBuiltin(prov); err != nil {
			t.Fatal(err)
		}
	}
	if err := router.Register(provider{scheme: "build"}); err != nil {
		t.Fatalf("extension scheme refused: %v", err)
	}
	schemes := router.Schemes()
	if !sort.StringsAreSorted(schemes) || len(schemes) != 3 {
		t.Fatalf("schemes not sorted: %v", schemes)
	}
	target, err := ParsePath(t.TempDir(), "session://self/meta", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.Provider(target); err == nil {
		t.Fatal("unregistered reserved scheme routed")
	}
	retired, err := ParsePath(t.TempDir(), "jobs://1", "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := router.Provider(retired); err == nil || !strings.Contains(err.Error(), "job://") {
		t.Fatalf("retired scheme lacks replacement: %v", err)
	}
}

type walkProvider struct {
	pages map[string]Listing
	calls []string
}

func (p *walkProvider) Scheme() string { return "kb" }
func (p *walkProvider) Stat(_ context.Context, uri string) (StatResult, error) {
	return StatResult{URI: uri, Kind: EntryDir}, nil
}
func (p *walkProvider) List(_ context.Context, uri, cursor string, _ int) (Listing, error) {
	p.calls = append(p.calls, uri+"|"+cursor)
	page, ok := p.pages[uri+"|"+cursor]
	if !ok {
		return Listing{}, ErrNotFound
	}
	return page, nil
}
func (*walkProvider) Read(context.Context, ReadRequest) (ReadResult, error) {
	return ReadResult{}, ErrUnsupported
}

func TestWalkHonoursNoWalkAndNeverFollowsLinks(t *testing.T) {
	p := &walkProvider{pages: map[string]Listing{
		"kb://root/|": {Complete: true, Entries: []Entry{
			{Name: "nested", URI: "kb://root/nested/", Kind: EntryDir},
			{Name: "expensive", URI: "kb://root/expensive/", Kind: EntryDir, NoWalk: true},
			{Name: "alias", URI: "kb://root/alias", Kind: EntryLink, Target: "kb://elsewhere/"},
			{Name: "ctl", URI: "kb://root/ctl", Kind: EntryControl},
		}},
		"kb://root/nested/|":    {Complete: true, Entries: []Entry{{Name: "doc.md", URI: "kb://root/nested/doc.md", Kind: EntryFile}}},
		"kb://root/expensive/|": {Complete: true, Entries: []Entry{{Name: "note.md", URI: "kb://root/expensive/note.md", Kind: EntryFile}}},
	}}
	var visited []string
	result, err := Walk(context.Background(), p, "kb://root/", func(entry Entry, _ int) (WalkAction, error) {
		visited = append(visited, entry.Name)
		return WalkContinue, nil
	})
	if err != nil || !result.Complete || strings.Join(visited, ",") != "nested,expensive,alias,doc.md" || strings.Join(p.calls, ",") != "kb://root/|,kb://root/nested/|" {
		t.Fatalf("unsafe walk: result=%+v visited=%v calls=%v err=%v", result, visited, p.calls, err)
	}
	visited = nil
	result, err = Walk(context.Background(), p, "kb://root/expensive/", func(entry Entry, _ int) (WalkAction, error) {
		visited = append(visited, entry.Name)
		return WalkContinue, nil
	})
	if err != nil || !result.Complete || strings.Join(visited, ",") != "note.md" {
		t.Fatalf("explicit noWalk root was not searched: result=%+v visited=%v err=%v", result, visited, err)
	}
}

func TestWalkHonoursCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	p := &walkProvider{}
	if _, err := Walk(ctx, p, "kb://root/", func(Entry, int) (WalkAction, error) { return WalkContinue, nil }); !errors.Is(err, context.Canceled) || len(p.calls) != 0 {
		t.Fatalf("cancelled walk reached provider: calls=%v err=%v", p.calls, err)
	}
}

func TestWalkRejectsEscapesAndRepeatedCursors(t *testing.T) {
	cases := []struct {
		name string
		page Listing
	}{
		{"other authority", Listing{Complete: true, Entries: []Entry{{Name: "doc", URI: "kb://other/doc", Kind: EntryFile}}}},
		{"parent traversal", Listing{Complete: true, Entries: []Entry{{Name: "doc", URI: "kb://root/../doc", Kind: EntryFile}}}},
		{"nonchild", Listing{Complete: true, Entries: []Entry{{Name: "doc", URI: "kb://root/deep/doc", Kind: EntryFile}}}},
		{"repeated cursor", Listing{NextCursor: "again"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &walkProvider{pages: map[string]Listing{"kb://root/|": tc.page, "kb://root/|again": tc.page}}
			if _, err := Walk(context.Background(), p, "kb://root/", func(Entry, int) (WalkAction, error) { return WalkContinue, nil }); err == nil {
				t.Fatal("invalid provider walk was accepted")
			}
		})
	}
}

type provider struct{ scheme string }

func (p provider) Scheme() string { return p.scheme }
func (provider) Stat(context.Context, string) (StatResult, error) {
	return StatResult{}, ErrUnsupported
}
func (provider) List(context.Context, string, string, int) (Listing, error) {
	return Listing{}, ErrUnsupported
}
func (provider) Read(context.Context, ReadRequest) (ReadResult, error) {
	return ReadResult{}, ErrUnsupported
}
