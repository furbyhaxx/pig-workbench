package host

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/furbyhaxx/pig-workbench/internal/config"
)

func testBoundary(t *testing.T) (*secretsBoundary, string) {
	t.Helper()
	root := t.TempDir()
	t.Setenv("XDG_DATA_HOME", filepath.Join(root, "data"))
	t.Setenv("LOCALAPPDATA", filepath.Join(root, "data"))
	t.Setenv("HOME", root)
	agent := filepath.Join(root, "agent")
	if err := os.MkdirAll(agent, 0o700); err != nil {
		t.Fatal(err)
	}
	settings, err := config.New(config.Paths{User: filepath.Join(agent, "Workbench.toml"), Project: filepath.Join(root, "project", "Workbench.toml")}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if err := settings.Load(t.Context(), false); err != nil {
		t.Fatal(err)
	}
	return newSecretsBoundary(settings, agent, func() string { return filepath.Join(root, "project", "Secrets.toml") }, func() bool { return false }), agent
}

func TestSecretsBoundaryCreatesKeyAndRedactsDetectedTokens(t *testing.T) {
	b, agent := testBoundary(t)
	guard, enabled, err := b.Guard()
	if err != nil || !enabled {
		t.Fatalf("guard = %v enabled=%v", err, enabled)
	}
	token := "ghp_" + strings.Repeat("a1B2", 6)
	out, err := guard.Redact("token " + token)
	if err != nil || strings.Contains(out, token) || !strings.Contains(out, "red-sec-") {
		t.Fatalf("redaction = %q, %v", out, err)
	}
	if info, err := os.Stat(filepath.Join(agent, "secret.key")); err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("key file = %v, %v", info, err)
	}
}

func TestSecretsBoundaryRecoversWithoutRestartAfterAFailedBuild(t *testing.T) {
	b, agent := testBoundary(t)
	policy := filepath.Join(agent, "Secrets.toml")
	if err := os.WriteFile(policy, []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, enabled, err := b.Guard(); err == nil || !enabled {
		t.Fatalf("a malformed policy must fail closed, got %v", err)
	}
	if err := os.WriteFile(policy, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Guard(); err != nil {
		t.Fatalf("the repaired policy was not picked up: %v", err)
	}
}

func TestSecretsBoundaryHonoursTheEnabledSetting(t *testing.T) {
	b, _ := testBoundary(t)
	base, err := b.settings.Current()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := b.settings.Save(t.Context(), config.User, base, []config.Edit{{Key: "secrets.enabled", Value: false}}, func() (bool, error) { return false, nil }); err != nil {
		t.Fatal(err)
	}
	if guard, enabled, err := b.Guard(); guard != nil || enabled || err != nil {
		t.Fatalf("disabled boundary = %v %v %v", guard, enabled, err)
	}
}

func TestSecretsBoundaryNotifiesOncePerCauseAndAgainAfterRecovery(t *testing.T) {
	b, agent := testBoundary(t)
	policy := filepath.Join(agent, "Secrets.toml")
	if err := os.WriteFile(policy, []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, _, err := b.Guard()
	if _, fresh := b.Withheld(err); !fresh {
		t.Fatal("the first failure must notify")
	}
	if _, fresh := b.Withheld(err); fresh {
		t.Fatal("the same failure must not notify twice")
	}
	if err := os.WriteFile(policy, []byte("version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := b.Guard(); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(policy, []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.Reload()
	_, _, err = b.Guard()
	if _, fresh := b.Withheld(err); !fresh {
		t.Fatal("a failure after recovery must notify again")
	}
}
