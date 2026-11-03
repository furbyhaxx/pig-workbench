//go:build integration

package host

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeFixtureProvider(t *testing.T, agent, baseURL string) {
	t.Helper()
	if err := os.MkdirAll(agent, 0700); err != nil {
		t.Fatal(err)
	}
	models := map[string]any{"providers": map[string]any{"workbench-fixture": map[string]any{"baseUrl": baseURL + "/v1", "api": "openai-completions", "apiKey": "fixture-not-a-secret", "models": []any{map[string]any{"id": "mock", "name": "Workbench scripted fixture", "reasoning": false, "input": []string{"text"}, "contextWindow": 32000, "maxTokens": 4000}}}}}
	data, err := json.Marshal(models)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(agent, "models.json"), data, 0600); err != nil {
		t.Fatal(err)
	}
}
