package host

import (
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/hashline"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
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

func TestSecretsBoundaryLearnsResolvedAuthButNeverACommandLiteral(t *testing.T) {
	b, _ := testBoundary(t)
	b.auth = func() (map[string]map[string]any, []string) {
		return map[string]map[string]any{
			"acme":    {"auth": map[string]any{"apiKey": "acme-resolved-key-value-1", "headers": map[string]any{"Authorization": "Bearer acme-bearer-token-value-2", "X-Region": "eu-central-region-3"}}},
			"cmdonly": {"auth": map[string]any{"apiKey": "!op read vault/item-secret-command"}},
		}, []string{"credential list unavailable: probe"}
	}
	guard, _, err := b.Guard()
	if err != nil {
		t.Fatal(err)
	}
	got, err := guard.Redact("acme-resolved-key-value-1 acme-bearer-token-value-2 eu-central-region-3 !op read vault/item-secret-command")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(got, "acme-resolved-key-value-1") || strings.Contains(got, "acme-bearer-token-value-2") {
		t.Fatalf("a credential survived redaction: %q", got)
	}
	if !strings.Contains(got, "eu-central-region-3") || !strings.Contains(got, "!op read vault/item-secret-command") {
		t.Fatalf("a non-secret header or an unresolved command literal was learned as a secret: %q", got)
	}
	if gaps := b.Gaps(); len(gaps) != 1 {
		t.Fatalf("an enumeration gap must be reported, got %v", gaps)
	}
}

func TestSecretsBoundaryRedactTextReportsDisabledAndFailureStates(t *testing.T) {
	b, agent := testBoundary(t)
	token := "ghp_" + strings.Repeat("q9Z8", 6)
	out, active, err := b.redactText("use " + token)
	if err != nil || !active || strings.Contains(out, token) {
		t.Fatalf("redactText = %q active=%v err=%v", out, active, err)
	}
	if err := os.WriteFile(filepath.Join(agent, "Secrets.toml"), []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	b.Reload()
	if out, _, err := b.redactText("use " + token); err == nil || out != "" {
		t.Fatalf("a failing boundary must return an error and no text, got %q %v", out, err)
	}
}

func TestRedactResultPatchesBlocksAndKeepsOpaqueOnes(t *testing.T) {
	b, _ := testBoundary(t)
	token := "ghp_" + strings.Repeat("w3X4", 6)
	got, err := b.resultPatch([]any{
		map[string]any{"type": "text", "text": "key " + token},
		map[string]any{"type": "image", "data": "AAAA", "mimeType": "image/png"},
	})
	if err != nil {
		t.Fatal(err)
	}
	blocks := got.(map[string]any)["content"].([]any)
	if text := blocks[0].(map[string]any)["text"].(string); strings.Contains(text, token) || !strings.Contains(text, "red-sec-") {
		t.Fatalf("text block = %q", text)
	}
	if blocks[1].(map[string]any)["data"] != "AAAA" {
		t.Fatal("an image block must stay intact")
	}
	if got, err := b.resultPatch("nothing secret"); err != nil || got.(map[string]any)["content"] != "nothing secret" {
		t.Fatalf("a clean string result must be returned unchanged, got %v %v", got, err)
	}
}

func TestRedactResultWithholdsOutputWhenTheBoundaryFails(t *testing.T) {
	b, agent := testBoundary(t)
	if err := os.WriteFile(filepath.Join(agent, "Secrets.toml"), []byte("[[patterns\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	raw := "ghp_" + strings.Repeat("e5R6", 6)
	got, cause := b.resultPatch(raw)
	if cause == nil {
		t.Fatal("the failure cause must be reported")
	}
	patch := got.(map[string]any)
	text := patch["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(text, raw) || patch["isError"] != true || !strings.Contains(text, "withheld") {
		t.Fatalf("patch = %+v", patch)
	}
}

func TestSeenStateSurvivesRedactionOfTheDeliveredBlock(t *testing.T) {
	b, _ := testBoundary(t)
	token := "ghp_" + strings.Repeat("t7Y8", 6)
	path := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(path, []byte("one\n"+token+"\nthree\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	target, err := resource.ParsePath(filepath.Dir(path), path, "")
	if err != nil {
		t.Fatal(err)
	}
	page, err := (resource.FileProvider{}).Read(t.Context(), resource.ReadRequest{URI: target.URI, Requested: path, Selector: target.Selection, Budget: resource.Budget{Bytes: 4096, Lines: 100}})
	if err != nil {
		t.Fatal(err)
	}
	store, err := hashline.NewStore(hashline.StoreLimits{}, "epoch")
	if err != nil {
		t.Fatal(err)
	}
	s := &session{snapshots: store, pendingViews: make(map[string][]pendingView), secrets: b}
	a := &app{session: s}
	s.rememberRead("call", "epoch", target, page)
	patch, cause := b.resultPatch([]any{map[string]any{"type": "text", "text": page.Content}})
	if cause != nil {
		t.Fatal(cause)
	}
	redacted := patch.(map[string]any)["content"]
	if _, err := a.confirmRead(sdk.Context{}, map[string]any{"toolName": "read", "toolCallId": "call", "content": redacted}); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Resolve(page.Facts.CanonicalURI, page.Facts.Tag); err != nil {
		t.Fatalf("a read whose block was redacted must still count as seen: %v", err)
	}
}

func TestUserBashSubstitutesPlaceholdersAndRedactsOutput(t *testing.T) {
	b, _ := testBoundary(t)
	guard, _, err := b.Guard()
	if err != nil {
		t.Fatal(err)
	}
	value := "user-bash-secret-value-9"
	if err := guard.Learn([]byte(value), "token", "user"); err != nil {
		t.Fatal(err)
	}
	placeholder, err := guard.Redact(value)
	if err != nil || placeholder == value {
		t.Fatalf("placeholder = %q %v", placeholder, err)
	}
	a := &app{session: &session{secrets: b}}
	result, err := a.userBash(sdk.Context{}, nil)
	if err != nil {
		t.Fatal(err)
	}
	exec := result.(map[string]any)["operations"].(sdk.BashOperations).Exec
	var streamed strings.Builder
	exit, err := exec("echo "+placeholder+" && test "+placeholder+" = "+value, t.TempDir(), sdk.BashExecOptions{OnData: func(data []byte) { streamed.Write(data) }})
	if err != nil || exit.ExitCode == nil {
		t.Fatalf("exec = %v %v", exit, err)
	}
	if *exit.ExitCode != 0 {
		t.Fatalf("the command did not receive the real value (exit %d)", *exit.ExitCode)
	}
	if strings.Contains(streamed.String(), value) || !strings.Contains(streamed.String(), placeholder) {
		t.Fatalf("streamed output = %q", streamed.String())
	}
	if _, err := exec("echo red-sec-unknownunknownun", t.TempDir(), sdk.BashExecOptions{}); err == nil {
		t.Fatal("an unknown placeholder must refuse the command")
	}
}

func TestRedactContextCleansTheSystemMessageAndOtherMessages(t *testing.T) {
	b, _ := testBoundary(t)
	a := &app{session: &session{secrets: b}}
	token := "ghp_" + strings.Repeat("u1V2", 6)
	got, err := a.redactContext(sdk.Context{}, map[string]any{"messages": []any{
		map[string]any{"role": "system", "content": "project_context: " + token},
		map[string]any{"role": "user", "content": []any{map[string]any{"type": "text", "text": "again " + token}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	messages := got.(map[string]any)["messages"].([]any)
	system := messages[0].(map[string]any)["content"].(string)
	user := messages[1].(map[string]any)["content"].([]any)[0].(map[string]any)["text"].(string)
	if strings.Contains(system, token) || strings.Contains(user, token) || messages[0].(map[string]any)["role"] != "system" {
		t.Fatalf("messages = %+v", messages)
	}
}
