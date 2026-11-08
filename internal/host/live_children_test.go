//go:build integration

package host

import (
	"fmt"
	"strings"
	"testing"
)

func TestLiveCodemodeChildIdentitiesAreRetainedForInspection(t *testing.T) {
	root := t.TempDir()
	recorder := &requestRecorder{calls: []scriptedCall{{"codemode", map[string]any{"code": "const r = await Promise.allSettled([tools.ls({path:'.'}), tools.read({path:'missing.txt'})]); text(String(r.length));"}}}}
	server := recorder.serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, root+"/agent", server.URL)
	f := startRPCWithExtensions(t, root, true, nil, "--model", "workbench-fixture/mock", "--tools", "read,ls,codemode")
	f.send("run", "prompt", "Run the script.")
	f.wait(func(e map[string]any) bool { return e["type"] == "agent_end" })
	f.send("inspect", "prompt", "/workbench codemode")
	event := f.wait(func(e map[string]any) bool {
		return e["type"] == "extension_ui_request" && e["method"] == "notify" && strings.Contains(fmt.Sprint(e["message"]), "codemodeChildren")
	})
	message := fmt.Sprint(event["message"])
	for _, want := range []string{`"status": "ok"`, `"status": "error"`, `"tool": "ls"`, `"tool": "read"`, "/1", "/2"} {
		if !strings.Contains(message, want) {
			t.Fatalf("retained children lack %q: %s", want, message)
		}
	}
	if strings.Contains(message, "missing.txt") {
		t.Fatalf("child arguments were retained: %s", message)
	}
}
