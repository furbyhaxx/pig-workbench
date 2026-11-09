//go:build integration

package host

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/project"
)

func gitInit(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if output, err := exec.Command("git", "-C", dir, "init", "--quiet").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v: %s", err, output)
	}
}

// Commands create a project and join further repositories by file and by marker, in a real RPC session.
func TestLiveProjectsCommandsCreateAndJoin(t *testing.T) {
	root := t.TempDir()
	first := filepath.Join(root, "work", "first")
	second := filepath.Join(root, "work", "second")
	third := filepath.Join(root, "work", "third")
	for _, dir := range []string{first, second, third} {
		gitInit(t, dir)
	}
	server := (&requestRecorder{}).serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	runtime := shortRuntime(t)

	a := startDefaultBuckets(t, root, first, runtime)
	a.send("c", "prompt", "/workbench projects create demo Demo Project")
	notifyContaining(a, "Created project demo")
	loaded, found, err := project.Load(first)
	if err != nil || !found || loaded.Slug != "demo" || loaded.Name != "Demo Project" {
		t.Fatalf("Project.toml not written: %+v %v %v", loaded, found, err)
	}
	a.send("l", "prompt", "/workbench projects list")
	notifyContaining(a, loaded.ID)
	a.send("again", "prompt", "/workbench projects create other")
	expectCommandError(a, "already belongs to project demo")
	a.send("s", "prompt", "/workbench projects status")
	notifyContaining(a, "recognised by project-file")
	a.close()

	b := startDefaultBuckets(t, root, second, runtime)
	b.send("j", "prompt", "/workbench projects join demo marker")
	notifyContaining(b, "recorded the local git marker")
	if _, err := os.Stat(filepath.Join(second, ".project")); !os.IsNotExist(err) {
		t.Fatal("a marker join must not write files")
	}
	b.send("s", "prompt", "/workbench projects status")
	notifyContaining(b, "recognised by marker")
	b.close()

	c := startDefaultBuckets(t, root, third, runtime)
	c.send("j", "prompt", "/workbench projects join "+loaded.ID+" file third-part")
	notifyContaining(c, "wrote .project/Project.toml")
	c.close()
	joined, found, err := project.Load(third)
	if err != nil || !found || joined.ID != loaded.ID || joined.Component.Slug != "third-part" || len(joined.Components) != 1 {
		t.Fatalf("joined document wrong: %+v %v %v", joined, found, err)
	}
	agent := filepath.Join(root, "agent", "projects", loaded.ID)
	if text, err := os.ReadFile(filepath.Join(agent, "project.toml")); err != nil || !strings.Contains(string(text), third) {
		t.Fatalf("registry must list the checkouts: %v\n%s", err, text)
	}
}

func expectCommandError(f *rpcFixture, want string) {
	f.t.Helper()
	for {
		select {
		case record := <-f.records:
			if record["type"] == "extension_error" {
				if strings.Contains(fmt.Sprint(record["error"]), want) {
					return
				}
				f.t.Fatalf("command failed with %v, want %q", record["error"], want)
			}
		case <-f.done:
			f.t.Fatalf("RPC exited before the expected error: %v", f.exitErr)
		case <-f.ctx.Done():
			f.t.Fatal("RPC deadline exceeded")
		}
	}
}
