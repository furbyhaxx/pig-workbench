//go:build integration

package host

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/endpoint"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

// startDefaultBuckets starts a session that uses PiG's default per-directory buckets under agent/sessions.
func startDefaultBuckets(t *testing.T, root, work, runtime string, args ...string) *rpcFixture {
	t.Helper()
	repo := repoRoot(t)
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	options := append([]string{"--offline", "-e", repo, "--mode", "rpc", "--model", "workbench-fixture/mock"}, args...)
	cmd := exec.CommandContext(ctx, "pig", options...)
	cmd.Dir = work
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"), "XDG_RUNTIME_DIR="+runtime)
	f := newRPCFixture(t, cmd, ctx, cancel)
	f.root = root
	return f
}

func storageFixture(t *testing.T) (root, work string) {
	t.Helper()
	root = t.TempDir()
	work = filepath.Join(root, "work", "app")
	if err := os.MkdirAll(filepath.Join(work, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Create(work, "prj-storagetest", "storage-test"); err != nil {
		t.Fatal(err)
	}
	server := (&requestRecorder{}).serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	return root, work
}

func TestLiveSessionStorageIsSharedThenUnlinked(t *testing.T) {
	root, work := storageFixture(t)
	first := startDefaultBuckets(t, root, work, shortRuntime(t))
	first.send("a", "prompt", "first session")
	first.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	first.close()

	bucket := filepath.Join(root, "agent", "sessions", "--"+strings.ReplaceAll(strings.TrimLeft(work, "/"), "/", "-")+"--")
	target := filepath.Join(root, "agent", "projects", "prj-storagetest", "sessions")
	if info, err := os.Lstat(bucket); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("the first start creates the bucket and the second converts it; after one run it is %v (%v)", info, err)
	}
	second := startDefaultBuckets(t, root, work, shortRuntime(t))
	second.send("b", "prompt", "second session")
	second.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	files, _ := filepath.Glob(filepath.Join(target, "*.jsonl"))
	if len(files) != 2 {
		t.Fatalf("both sessions must live in the project storage, found %v", files)
	}
	second.send("undo", "prompt", "/workbench projects unlink-storage")
	notifyContaining(second, "Restored 1 session directories")
	second.close()
	if info, err := os.Lstat(bucket); err != nil || !info.IsDir() {
		t.Fatalf("after the undo the bucket must be a real directory, got %v (%v)", info, err)
	}
	if listed, _ := filepath.Glob(filepath.Join(bucket, "*.jsonl")); len(listed) != 2 {
		t.Fatalf("PiG must list both sessions again, found %v", listed)
	}
}

func TestLiveSessionStorageCanBeDisabled(t *testing.T) {
	root, work := storageFixture(t)
	if err := os.WriteFile(filepath.Join(root, "agent", "Workbench.toml"), []byte("[projects]\nsession-storage = 'off'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	f := startDefaultBuckets(t, root, work, shortRuntime(t))
	f.send("go", "prompt", "first session")
	f.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	f.close()
	bucket := filepath.Join(root, "agent", "sessions", "--"+strings.ReplaceAll(strings.TrimLeft(work, "/"), "/", "-")+"--")
	if info, err := os.Lstat(bucket); err != nil || !info.IsDir() {
		t.Fatalf("disabled session storage must leave a real bucket, got %v (%v)", info, err)
	}
}

func TestLiveAnotherLiveSessionInTheDirectoryDefersConversion(t *testing.T) {
	root, work := storageFixture(t)
	runtime := shortRuntime(t)
	registry, err := endpoint.Open(filepath.Join(runtime, "pig-workbench", "v1"))
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	neighbour := endpoint.Record{SchemaVersion: 1, SessionID: "neighbour-session-0001", Token: "t", Cwd: work, State: "idle", PID: os.Getpid(), StartedAt: now, HeartbeatAt: now, Discoverable: true}
	if err := registry.Register(context.Background(), neighbour); err != nil {
		t.Fatal(err)
	}
	bucket := filepath.Join(root, "agent", "sessions", "--"+strings.ReplaceAll(strings.TrimLeft(work, "/"), "/", "-")+"--")
	f := startDefaultBuckets(t, root, work, runtime)
	notifyContaining(f, "conversion deferred")
	f.send("a", "prompt", "hello")
	f.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	if info, err := os.Lstat(bucket); err != nil || !info.IsDir() {
		t.Fatalf("a live neighbour must keep the bucket a real directory, got %v (%v)", info, err)
	}
	f.close()
	if err := registry.Unregister(context.Background(), neighbour.SessionID, neighbour.Token); err != nil {
		t.Fatal(err)
	}
	again := startDefaultBuckets(t, root, work, runtime)
	again.send("b", "prompt", "hello again")
	again.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	again.close()
	if info, err := os.Lstat(bucket); err != nil || info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("once the neighbour is gone the next start converts, got %v (%v)", info, err)
	}
}

func TestLiveTwoCheckoutsShareSessionsAndTheCatalogListsEachOnce(t *testing.T) {
	root, work := storageFixture(t)
	clone := filepath.Join(root, "work", "clone")
	if err := os.MkdirAll(filepath.Join(clone, ".git"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := project.Create(clone, "prj-storagetest", "storage-test"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{work, clone, work} {
		f := startDefaultBuckets(t, root, dir, shortRuntime(t))
		f.send("a", "prompt", "a session in "+filepath.Base(dir))
		f.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
		f.close()
	}
	target := filepath.Join(root, "agent", "projects", "prj-storagetest", "sessions")
	if files, _ := filepath.Glob(filepath.Join(target, "*.jsonl")); len(files) != 3 {
		t.Fatalf("both checkouts must share one directory with all 3 sessions, found %v", files)
	}
	for _, name := range []string{"app", "clone"} {
		bucket := filepath.Join(root, "agent", "sessions", "--"+strings.ReplaceAll(strings.TrimLeft(filepath.Join(root, "work", name), "/"), "/", "-")+"--")
		if info, err := os.Lstat(bucket); err != nil || info.Mode()&os.ModeSymlink == 0 {
			t.Fatalf("%s bucket must be a symlink: %v (%v)", name, info, err)
		}
	}
	recorder := &requestRecorder{calls: []scriptedCall{{"ls", map[string]any{"path": "session://by-id/"}}}}
	server := recorder.serve(t)
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	probe := startDefaultBuckets(t, root, work, shortRuntime(t), "--tools", "ls")
	probe.send("p", "prompt", "list")
	probe.wait(func(event map[string]any) bool { return event["type"] == "agent_end" })
	probe.close()
	recorder.mu.Lock()
	defer recorder.mu.Unlock()
	last := recorder.bodies[len(recorder.bodies)-1]
	results := toolResults(t, last)
	if len(results) != 1 {
		t.Fatalf("want one ls result, got %v", results)
	}
	// Three sessions from the fixtures plus this probe session once it has a file; none may repeat.
	seen := map[string]int{}
	for _, line := range strings.Split(results[0], "\n") {
		if fields := strings.Fields(line); len(fields) >= 2 && fields[0] == "link" {
			seen[fields[1]]++
		}
	}
	if len(seen) < 3 {
		t.Fatalf("the catalog must list the shared sessions: %q", results[0])
	}
	for id, count := range seen {
		if count != 1 {
			t.Fatalf("session %s is listed %d times; the catalog must deduplicate by ID", id, count)
		}
	}
}
