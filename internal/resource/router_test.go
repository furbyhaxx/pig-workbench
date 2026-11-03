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
