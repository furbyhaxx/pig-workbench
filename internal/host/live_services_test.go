//go:build integration

package host

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type rpcFixture struct {
	t         *testing.T
	ctx       context.Context
	cancel    context.CancelFunc
	cmd       *exec.Cmd
	stdin     io.WriteCloser
	records   chan map[string]any
	done      chan struct{}
	stderr    bytes.Buffer
	exitErr   error
	root      string
	closeOnce sync.Once
}

func startRPC(t *testing.T, root string, args ...string) *rpcFixture {
	t.Helper()
	return startRPCWithWorkbench(t, root, true, args...)
}

func startRPCWithWorkbench(t *testing.T, root string, workbench bool, args ...string) *rpcFixture {
	t.Helper()
	return startRPCWithExtensions(t, root, workbench, []string{"service-client"}, args...)
}

// startRPCWithExtensions co-loads the named testdata fixtures beside Workbench.
func startRPCWithExtensions(t *testing.T, root string, workbench bool, fixtures []string, args ...string) *rpcFixture {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 45*time.Second)
	options := []string{"--offline", "--session-dir", filepath.Join(root, "sessions")}
	if workbench {
		options = append(options, "-e", repo)
	}
	for _, fixture := range fixtures {
		options = append(options, "-e", filepath.Join(repo, "internal/host/testdata", fixture))
	}
	options = append(options, "--mode", "rpc")
	options = append(options, args...)
	cmd := exec.CommandContext(ctx, "pig", options...)
	cmd.Dir = root
	for _, v := range os.Environ() {
		if !strings.HasPrefix(v, "PIG_") && !strings.HasPrefix(v, "PI_") {
			cmd.Env = append(cmd.Env, v)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	fixture := newRPCFixture(t, cmd, ctx, cancel)
	fixture.root = root
	return fixture
}

// newRPCFixture starts an already configured pig command and reads its JSONL records.
func newRPCFixture(t *testing.T, cmd *exec.Cmd, ctx context.Context, cancel context.CancelFunc) *rpcFixture {
	t.Helper()
	fixture := &rpcFixture{t: t, ctx: ctx, cancel: cancel, cmd: cmd, records: make(chan map[string]any, 32), done: make(chan struct{})}
	cmd.Stderr = &fixture.stderr
	stdin, err := cmd.StdinPipe()
	if err != nil {
		cancel()
		t.Fatal(err)
	}
	fixture.stdin = stdin
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		stdin.Close()
		t.Fatal(err)
	}
	if err := cmd.Start(); err != nil {
		cancel()
		stdin.Close()
		t.Fatal(err)
	}
	go func() {
		var scanErr error
		defer func() {
			if scanErr != nil {
				cancel()
			}
			fixture.exitErr = cmd.Wait()
			if scanErr != nil {
				fixture.exitErr = scanErr
			}
			close(fixture.done)
		}()
		scanner := bufio.NewScanner(stdout)
		scanner.Buffer(make([]byte, 4096), 1024*1024)
		for scanner.Scan() {
			var record map[string]any
			if err := json.Unmarshal(scanner.Bytes(), &record); err != nil {
				scanErr = err
				return
			}
			select {
			case fixture.records <- record:
			case <-ctx.Done():
				return
			}
		}
		scanErr = scanner.Err()
	}()
	t.Cleanup(fixture.close)
	return fixture
}

func (f *rpcFixture) close() {
	f.closeOnce.Do(func() {
		defer f.cancel()
		f.stdin.Close()
		select {
		case <-f.done:
		case <-time.After(5 * time.Second):
			f.cancel()
			select {
			case <-f.done:
			case <-time.After(5 * time.Second):
				f.t.Error("RPC process did not join")
				return
			}
			f.t.Error("RPC did not shut down after stdin closed")
		}
		if f.exitErr != nil {
			f.t.Errorf("RPC exited with %v; stderr: %s", f.exitErr, f.stderr.String())
		}
	})
}

func (f *rpcFixture) send(id, kind, message string) {
	f.t.Helper()
	request := map[string]any{"id": id, "type": kind}
	if message != "" {
		request["message"] = message
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f.stdin, "%s\n", encoded); err != nil {
		f.t.Fatal(err)
	}
}

// sendRaw writes one RPC command object verbatim.
func (f *rpcFixture) sendRaw(id, kind string, extra map[string]any) {
	f.t.Helper()
	request := map[string]any{"id": id, "type": kind}
	for key, value := range extra {
		request[key] = value
	}
	encoded, err := json.Marshal(request)
	if err != nil {
		f.t.Fatal(err)
	}
	if _, err := fmt.Fprintf(f.stdin, "%s\n", encoded); err != nil {
		f.t.Fatal(err)
	}
}

func (f *rpcFixture) wait(match func(map[string]any) bool) map[string]any {
	f.t.Helper()
	for {
		select {
		case record := <-f.records:
			if record["type"] == "extension_error" {
				f.t.Fatalf("extension error: %+v", record)
			}
			if record["type"] == "response" && record["success"] == false {
				f.t.Fatalf("RPC error: %+v", record)
			}
			if match(record) {
				return record
			}
		case <-f.done:
			f.t.Fatalf("RPC exited before response: %v, stderr: %s", f.exitErr, f.stderr.String())
		case <-f.ctx.Done():
			f.t.Fatal("RPC deadline exceeded")
		}
	}
}

func (f *rpcFixture) probe(id string) map[string]any {
	f.t.Helper()
	f.send(id, "prompt", "/fixture")
	record := f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"fixture":"passed"`)
	})
	var result map[string]any
	if err := json.Unmarshal([]byte(record["message"].(string)), &result); err != nil {
		f.t.Fatal(err)
	}
	return result
}

func (f *rpcFixture) command(id, command string) {
	f.t.Helper()
	f.send(id, "prompt", command)
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == id })
}

func TestLiveServiceFailuresRPC(t *testing.T) {
	f := startRPCWithWorkbench(t, t.TempDir(), false)
	f.send("failures", "prompt", "/fixture-errors")
	f.wait(func(record map[string]any) bool {
		return record["type"] == "extension_ui_request" && record["method"] == "notify" && strings.Contains(fmt.Sprint(record["message"]), `"fixtureErrors":"passed"`)
	})
}

func TestLiveServiceDiscoveryRPC(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "next"), 0700); err != nil {
		t.Fatal(err)
	}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		fmt.Fprint(w, "data: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{\"role\":\"assistant\",\"content\":\"Fixture initialized.\"}}]}\n\ndata: {\"id\":\"fixture\",\"object\":\"chat.completion.chunk\",\"created\":0,\"model\":\"mock\",\"choices\":[{\"index\":0,\"delta\":{},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	}))
	defer server.Close()
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	f := startRPC(t, root, "--model", "workbench-fixture/mock")
	first := f.probe("first")
	f.send("persist", "prompt", "Initialize persisted fixture.")
	f.wait(func(record map[string]any) bool { return record["type"] == "agent_settled" })
	f.command("cd", "/cd next")
	second := f.probe("second")
	if first["generation"] != second["generation"] {
		t.Fatal("location transition changed service generation")
	}
	location := second["location"].(map[string]any)
	if location["cwd"] != filepath.Join(root, "next") {
		t.Fatalf("service did not observe cd: %+v", location)
	}
	f.send("replace", "new_session", "")
	f.wait(func(record map[string]any) bool { return record["type"] == "response" && record["id"] == "replace" })
	third := f.probe("third")
	if third["generation"] == first["generation"] {
		t.Fatal("session replacement reused service generation")
	}
	location = third["location"].(map[string]any)
	if location["cwd"] != root {
		t.Fatalf("session replacement retained prior runtime location: %+v", location)
	}
	f.close()
	resumed := startRPC(t, root, "--session", first["sessionId"].(string))
	fourth := resumed.probe("resumed")
	location = fourth["location"].(map[string]any)
	if location["cwd"] != filepath.Join(root, "next") {
		t.Fatalf("resume lost persisted location: %+v", location)
	}
	if fourth["generation"] == first["generation"] {
		t.Fatal("resume reused an old service generation")
	}
}
