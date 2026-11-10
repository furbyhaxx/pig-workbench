//go:build integration

package host

import (
	"os"
	"path/filepath"
	"testing"
)

// Every verb family accepts the alias spellings, unknown verbs answer in one short line, and the
// prompt and tools verbs produce their views, in a real RPC session.
func TestLiveCommandAliasesUnknownVerbPromptAndToolsFormats(t *testing.T) {
	root := t.TempDir()
	work := filepath.Join(root, "work", "app")
	gitInit(t, work)
	if err := os.MkdirAll(filepath.Join(work, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	server := (&requestRecorder{}).serve(t)
	t.Cleanup(server.Close)
	writeFixtureProvider(t, filepath.Join(root, "agent"), server.URL)
	f := startDefaultBuckets(t, root, work, shortRuntime(t))
	defer f.close()

	shows := func(command, want string) {
		t.Helper()
		t.Log(command)
		f.send("c", "prompt", command)
		notifyContaining(f, want)
	}
	fails := func(command, want string) {
		t.Helper()
		t.Log(command)
		f.send("c", "prompt", command)
		expectCommandError(f, want)
	}
	for _, command := range []string{"/workbench jobs list", "/wb:jobs list", "/workbench jobs ls"} {
		shows(command, "No jobs in this session.")
	}
	shows("/workbench trust list", "trust")
	shows("/workbench trust add allow "+work, "(1 rule(s))")
	shows("/workbench trust remove "+work, "(0 rule(s))")
	shows("/workbench config show read.max-bytes", "read.max-bytes")
	shows("/workbench frames list", "No frames in this session.")
	shows("/workbench instructions list", "No pinned instruction files.")

	fails("/workbench framez", `unknown /workbench verb "framez"; did you mean frames?`)
	fails("/workbench qqqqqqqq", `unknown /workbench verb "qqqqqqqq"; run /workbench for the list`)

	shows("/workbench prompt", "section: ")
	shows("/workbench prompt render --tools read", "not sent")
	fails("/workbench prompt render --model workbench-fixture/absent", "no model workbench-fixture/absent")
	shows("/workbench prompt diff", "prompt in use")
	fails("/workbench prompt nope", "usage: /workbench prompt")

	shows("/workbench tools read --format json", `"name": "read"`)
	shows("/tools --format markdown", "## read")
	shows("/tools read", "read [active]")
	fails("/tools --format xml", "usage: /workbench tools")
}
