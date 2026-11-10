//go:build integration

package host

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeFixtureProvider(t *testing.T, agent, baseURL string) {
	t.Helper()
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	models := map[string]any{"providers": map[string]any{"workbench-fixture": map[string]any{"baseUrl": baseURL + "/v1", "api": "openai-completions", "apiKey": "fixture-not-a-secret", "models": []any{map[string]any{"id": "mock", "name": "Workbench scripted fixture", "reasoning": false, "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 4000}, map[string]any{"id": "mock-reasoning", "name": "Workbench reasoning fixture", "reasoning": true, "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 4000}}}}}
	data, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "models.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}

func repoRoot(t *testing.T) string {
	t.Helper()
	repo, err := filepath.Abs("../..")
	if err != nil {
		t.Fatal(err)
	}
	return repo
}

func runPigJSON(t *testing.T, ctx context.Context, repo, root, dir string, args []string, prompt string) []byte {
	t.Helper()
	return runPigJSONWith(t, ctx, []string{repo}, root, dir, args, prompt)
}

// runPigJSONWith runs one JSON-mode prompt with exactly the listed extensions loaded.
func runPigJSONWith(t *testing.T, ctx context.Context, extensions []string, root, dir string, args []string, prompt string) []byte {
	t.Helper()
	isolateHostFiles(t, filepath.Join(root, "agent"))
	command := []string{"--offline", "--no-session"}
	for _, extension := range extensions {
		command = append(command, "-e", extension)
	}
	command = append(command, "--mode", "json", "--model", "workbench-fixture/mock")
	command = append(command, args...)
	command = append(command, prompt)
	cmd := exec.CommandContext(ctx, "pig", command...)
	cmd.Dir = dir
	for _, value := range os.Environ() {
		if !strings.HasPrefix(value, "PIG_") && !strings.HasPrefix(value, "PI_") {
			cmd.Env = append(cmd.Env, value)
		}
	}
	cmd.Env = append(cmd.Env, "PIG_HOME="+root, "PIG_CODING_AGENT_DIR="+filepath.Join(root, "agent"), "XDG_DATA_HOME="+filepath.Join(root, "data"), "XDG_STATE_HOME="+filepath.Join(root, "state"))
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("live pig: %v\nstderr: %s\nstdout: %s", err, stderr.String(), output)
	}
	return output
}
