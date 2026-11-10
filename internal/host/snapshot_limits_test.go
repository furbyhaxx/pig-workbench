package host

import (
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func TestSnapshotStoreFollowsTheEditSnapshotSettings(t *testing.T) {
	manager, err := config.New(config.Paths{User: t.TempDir() + "/a.toml", Project: t.TempDir() + "/b.toml"}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	store, err := hashline.NewStore(hashline.StoreLimits{}, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	s := &session{settings: manager, snapshots: store, pendingViews: newRecent[[]pendingView](256)}
	set := func(key string, value int64) {
		t.Helper()
		baseline, err := manager.Current()
		if err != nil {
			t.Fatal(err)
		}
		if _, err := manager.Save(t.Context(), config.Runtime, baseline, []config.Edit{{Key: key, Value: value}}, nil); err != nil {
			t.Fatal(err)
		}
	}
	write := func(uri, text string) {
		s.rememberWritten("call", "epoch", resource.Target{URI: uri}, []byte(text), "receipt")
	}
	held := func(uri string) bool { _, ok := store.Latest(uri); return ok }

	set("edit.snapshot.max-file-bytes", 1024)
	write("file:///big", strings.Repeat("x", 1025)+"\n")
	if held("file:///big") {
		t.Fatal("a file above edit.snapshot.max-file-bytes was retained")
	}
	set("edit.snapshot.max-versions", 1)
	write("file:///a", "one\n")
	write("file:///a", "two\n")
	old := hashline.SourceRevision([]byte("one\n"))
	if err := store.Delivered("file:///a", old, "epoch", nil, false); err == nil {
		t.Fatal("an older version survived edit.snapshot.max-versions = 1")
	}
	set("edit.snapshot.max-paths", 2)
	write("file:///b", "b\n")
	write("file:///c", "c\n")
	if held("file:///a") || !held("file:///b") || !held("file:///c") {
		t.Fatal("edit.snapshot.max-paths = 2 must keep only the two newest paths")
	}
	set("edit.snapshot.max-bytes", 1024)
	write("file:///d", strings.Repeat("d", 400)+"\n")
	write("file:///e", strings.Repeat("e", 400)+"\n")
	if held("file:///d") || !held("file:///e") {
		t.Fatal("edit.snapshot.max-bytes = 1024 must evict the older of two 401-byte sources (each costs twice its size)")
	}
}
