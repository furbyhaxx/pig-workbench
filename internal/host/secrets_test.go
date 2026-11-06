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
	return newSecretsBoundary(settings, agent, func() string { return filepath.Join(root, "project", "Secrets.toml") }, func() bool { return false }, func() string { return root }), agent
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

func TestSecretsBoundaryLearnsNamedEnvironmentAndEnvFilesUpToTheRepositoryRoot(t *testing.T) {
	b, _ := testBoundary(t)
	outer := t.TempDir()
	repo := filepath.Join(outer, "repo")
	sub := filepath.Join(repo, "svc", "api")
	for _, dir := range []string{filepath.Join(repo, ".git"), sub} {
		if err := os.MkdirAll(dir, 0o700); err != nil {
			t.Fatal(err)
		}
	}
	above := outer
	files := map[string]string{
		filepath.Join(b.home, ".env"):    "HOME_TOKEN=home-token-value-1\n",
		filepath.Join(repo, ".env"):      "export ROOT_API_KEY=\"root-key-value-22\"\nPLAIN=not-a-secret-name-1\n",
		filepath.Join(sub, ".env.local"): "DB_PASSWORD='db-password-value-3' # comment\nPUBLIC_ID=public-identifier-4\n",
		filepath.Join(above, ".env"):     "ABOVE_TOKEN=above-the-repository-5\n",
	}
	for file, text := range files {
		if err := os.WriteFile(file, []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	b.cwd = func() string { return sub }
	b.environ = func() []string { return []string{"GITHUB_TOKEN=env-token-value-6", "EDITOR=nano-editor-long-7"} }
	guard, _, err := b.Guard()
	if err != nil {
		t.Fatal(err)
	}
	text := strings.Join([]string{"home-token-value-1", "root-key-value-22", "db-password-value-3", "env-token-value-6", "not-a-secret-name-1", "public-identifier-4", "nano-editor-long-7", "above-the-repository-5"}, " ")
	got, err := guard.Redact(text)
	if err != nil {
		t.Fatal(err)
	}
	for _, learned := range []string{"home-token-value-1", "root-key-value-22", "db-password-value-3", "env-token-value-6"} {
		if strings.Contains(got, learned) {
			t.Errorf("%s was not redacted", learned)
		}
	}
	for _, kept := range []string{"not-a-secret-name-1", "public-identifier-4", "nano-editor-long-7", "above-the-repository-5"} {
		if !strings.Contains(got, kept) {
			t.Errorf("%s must stay readable: only secret-named values inside the repository are learned", kept)
		}
	}
}
