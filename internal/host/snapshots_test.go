package host

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestReadVisibilityRequiresIntactDeliveredBlock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("one\ntwo\nthree\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := resource.ParsePath(filepath.Dir(path), path, ":1:raw")
	if err != nil {
		t.Fatal(err)
	}
	target.Selection, err = resource.ParseSelector(":1-1")
	if err != nil {
		t.Fatal(err)
	}
	page, err := (resource.FileProvider{}).Read(context.Background(), resource.ReadRequest{URI: target.URI, Requested: path, Selector: target.Selection, Budget: resource.Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := hashline.NewStore(hashline.StoreLimits{}, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	s := &session{snapshots: store, pendingViews: newRecent[[]pendingView](256)}
	a := &app{session: s}
	s.rememberRead("cut", "epoch", target, page)
	if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": "cut", "content": []any{map[string]any{"type": "text", "text": "truncated"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag); err == nil {
		t.Fatal("truncated result marked seen")
	}
	s.rememberRead("split", "epoch", target, page)
	midpoint := len(page.Content) / 2
	if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": "split", "content": []any{map[string]any{"type": "text", "text": page.Content[:midpoint]}, map[string]any{"type": "text", "text": page.Content[midpoint:]}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag); err == nil {
		t.Fatal("separate modified blocks synthesized a complete seen view")
	}
	s.rememberRead("ok", "epoch", target, page)
	if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": "ok", "isError": true, "content": []any{map[string]any{"type": "text", "text": page.Content}, map[string]any{"type": "text", "text": "annotation"}}}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag); err != nil {
		t.Fatal(err)
	}
	if err := store.SetEpoch("new"); err != nil {
		t.Fatal(err)
	}
	s.rememberRead("late", "epoch", target, page)
	if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": "late", "content": page.Content}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag); err == nil {
		t.Fatal("late result crossed epoch")
	}
}

// TestAnnotationOrderCannotChangeSeenVisibility proves that an annotating
// handler running before or after Workbench's confirmation cannot alter which
// lines become seen: only the intact block counts.
func TestAnnotationOrderCannotChangeSeenVisibility(t *testing.T) {
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("alpha\nbravo\n"), 0600); err != nil {
		t.Fatal(err)
	}
	target, err := resource.ParsePath(filepath.Dir(path), path, ":1")
	if err != nil {
		t.Fatal(err)
	}
	if target.Selection, err = resource.ParseSelector(":1-2"); err != nil {
		t.Fatal(err)
	}
	page, err := (resource.FileProvider{}).Read(context.Background(), resource.ReadRequest{URI: target.URI, Requested: path, Selector: target.Selection, Budget: resource.Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	run := func(name string, annotate func(blocks []any) []any, wantSeen bool) {
		t.Helper()
		store, err := hashline.NewStore(hashline.StoreLimits{}, "epoch")
		if err != nil {
			t.Fatal(err)
		}
		s := &session{snapshots: store, pendingViews: newRecent[[]pendingView](256)}
		a := &app{session: s}
		s.rememberRead(name, "epoch", target, page)
		blocks := annotate([]any{map[string]any{"type": "text", "text": page.Content}})
		if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": name, "content": blocks}); err != nil {
			t.Fatal(err)
		}
		_, resolveErr := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag)
		if wantSeen && resolveErr != nil {
			t.Fatalf("%s: intact block with a trailing annotation lost visibility: %v", name, resolveErr)
		}
		if !wantSeen && resolveErr == nil {
			t.Fatalf("%s: an altered block granted visibility", name)
		}
	}
	run("annotation-before", func(blocks []any) []any {
		return append([]any{map[string]any{"type": "text", "text": "appended annotation"}}, blocks...)
	}, true)
	run("annotation-after", func(blocks []any) []any {
		return append(blocks, map[string]any{"type": "text", "text": "appended annotation"})
	}, true)
	run("annotation-altered", func(blocks []any) []any {
		return []any{map[string]any{"type": "text", "text": strings.Replace(page.Content, "alpha", "ALPHA", 1)}, map[string]any{"type": "text", "text": "appended annotation"}}
	}, false)
}
