//go:build integration

package host

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestLiveRetainedToolMetadataNeverPersistsKnownSecrets(t *testing.T) {
	root := t.TempDir()
	token := "ghp_" + strings.Repeat("Rt9vK3", 5)
	if err := os.WriteFile(filepath.Join(root, "notes.txt"), []byte("credential="+token+"\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, token+".txt"), []byte("nonsecret resource body\n"), 0600); err != nil {
		t.Fatal(err)
	}
	placeholder := func(results []string) string {
		value := regexp.MustCompile(`red-sec-[a-z2-7]{16}`).FindString(strings.Join(results, "\n"))
		if value == "" {
			t.Error("no learned placeholder was returned")
		}
		return value
	}
	runScriptedWith(t, root, "read,bash,web_metadata_probe", "", []string{"reference-producer"},
		func([]string) scriptedCall { return scriptedCall{"read", map[string]any{"path": "notes.txt"}} },
		func(results []string) scriptedCall {
			return scriptedCall{"bash", map[string]any{"command": "printf %s " + placeholder(results) + " > secret-copy.txt"}}
		},
		func(results []string) scriptedCall {
			return scriptedCall{"read", map[string]any{"path": placeholder(results) + ".txt"}}
		},
		func(results []string) scriptedCall {
			return scriptedCall{"web_metadata_probe", map[string]any{"value": placeholder(results)}}
		},
	)
	copied, err := os.ReadFile(filepath.Join(root, "secret-copy.txt"))
	if err != nil || string(copied) != token {
		t.Fatal("substituted command did not receive the real value")
	}
	files, _ := filepath.Glob(filepath.Join(root, "sessions", "*", "*.jsonl"))
	if len(files) == 0 {
		files, _ = filepath.Glob(filepath.Join(root, "sessions", "*.jsonl"))
	}
	if len(files) == 0 {
		t.Fatal("no persisted session to verify")
	}
	for _, file := range files {
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(string(data), token) {
			t.Fatal("known secret persisted in tool details, structured content or resource metadata")
		}
	}
}
