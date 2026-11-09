package secrets

import (
	"bytes"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func newTestDetector(t *testing.T, master []byte, path string, policy DetectionPolicy) (*Detector, *Vault) {
	t.Helper()
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	detector, err := NewDetector(master, vault, policy)
	if err != nil {
		t.Fatal(err)
	}
	return detector, vault
}

func testVaultPath(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "private", "vault.jsonl")
}

func TestDetectorLearnsRedactsAndSubstitutes(t *testing.T) {
	master := bytes.Repeat([]byte{21}, 32)
	path := testVaultPath(t)
	detector, vault := newTestDetector(t, master, path, DetectionPolicy{})
	key := mustHMACKey(t, master)
	value := "test-credential-value-0001"
	placeholder := Placeholder(key, []byte(value))
	if err := detector.Learn([]byte(value), "token", "user"); err != nil {
		t.Fatal(err)
	}
	text := "push with " + value + " please"
	redacted, err := detector.Redact(text)
	if err != nil || redacted != "push with "+placeholder+" please" {
		t.Fatalf("redaction = %q, err %v", redacted, err)
	}
	again, err := detector.Redact(text)
	if err != nil || again != redacted {
		t.Fatalf("placeholder is not deterministic: %q, %v", again, err)
	}
	if got, err := detector.Redact("see " + placeholder + " and " + value); err != nil || got != "see "+placeholder+" and "+placeholder {
		t.Fatalf("existing placeholder was not immune: %q, %v", got, err)
	}
	restored, err := detector.Substitute(redacted + " again " + redacted)
	if err != nil || restored != text+" again "+text {
		t.Fatalf("substitution = %q, err %v", restored, err)
	}
	other, _ := newTestDetector(t, master, path, DetectionPolicy{})
	if seen, err := other.Redact("value=" + value); err != nil || seen != "value="+placeholder {
		t.Fatalf("second detector did not see the learned value: %q, %v", seen, err)
	}
	stored, ok := mustLoad(t, vault)[placeholder]
	if !ok || stored.Kind != "token" || stored.Source != "user" || stored.Scope != "global" {
		t.Fatalf("stored metadata = %q/%q/%q", stored.Kind, stored.Source, stored.Scope)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(value)) {
		t.Fatal("vault contains the plaintext value")
	}
}

func TestDetectorDetectsTokenShapesURLPasswordsAndPatterns(t *testing.T) {
	master := bytes.Repeat([]byte{22}, 32)
	policy := DetectionPolicy{Patterns: []Pattern{
		{Name: "acme_token", Regex: `acme_[a-z0-9]{20,}`},
		{Name: "acme_prefixed", Prefix: "qatk_", MinLength: 20},
		{Name: "acme_suffixed", Suffix: "-ops"},
	}}
	detector, vault := newTestDetector(t, master, testVaultPath(t), policy)
	key := mustHMACKey(t, master)
	fixtures := map[string]string{
		"github_pat":    "github_pat_" + strings.Repeat("0", 30),
		"github_token":  "ghp_" + strings.Repeat("1", 36),
		"openai_key":    "sk-" + strings.Repeat("a", 32),
		"acme_token":    "acme_" + strings.Repeat("b", 24),
		"acme_prefixed": "qatk_" + strings.Repeat("c", 16),
		"acme_suffixed": strings.Repeat("d", 10) + "-ops",
		"url_password":  "url-pass-value-1",
		"private_key":   "-----BEGIN PRIVATE KEY-----\nsynthetic-test-body\n-----END PRIVATE KEY-----",
	}
	var lines []string
	for kind, value := range fixtures {
		lines = append(lines, kind+": "+value)
	}
	text := strings.Join(lines, "\n")
	// The URL password is detected inside its credential URL, not on its own.
	text += "\nremote: https://build-user:url-pass-value-1@example.test/path"
	redacted, err := detector.Redact(text)
	if err != nil {
		t.Fatal(err)
	}
	for kind, value := range fixtures {
		if strings.Contains(redacted, value) {
			t.Fatalf("%s value was not redacted", kind)
		}
	}
	if restored, err := detector.Substitute(redacted); err != nil || restored != text {
		t.Fatalf("substitution did not recover the original text: %v", err)
	}
	records := mustLoad(t, vault)
	for kind, value := range fixtures {
		record, ok := records[Placeholder(key, []byte(value))]
		if !ok || record.Kind != kind || record.Source != "detected" {
			t.Fatalf("%s record = %q/%q, present %v", kind, record.Kind, record.Source, ok)
		}
	}
}

func TestDetectorHonoursExclusionsAndMinimumLength(t *testing.T) {
	master := bytes.Repeat([]byte{23}, 32)
	path := testVaultPath(t)
	key := mustHMACKey(t, master)
	byValue := "acme_excludedbyvalue"
	byHash := "acme_excludedbyhash"
	detected := "acme_detectedvalue"
	learning, _ := newTestDetector(t, master, path, DetectionPolicy{})
	if err := learning.Learn([]byte(byValue), "token", "user"); err != nil {
		t.Fatal(err)
	}
	policy := DetectionPolicy{
		Patterns: []Pattern{{Name: "acme_token", Regex: `acme_[a-z0-9]+`}},
		Exclusions: Exclusions{
			Values: []string{byValue},
			Hashes: []string{strings.TrimPrefix(Placeholder(key, []byte(byHash)), "red-sec-")},
		},
	}
	detector, vault := newTestDetector(t, master, path, policy)
	text := strings.Join([]string{byValue, byHash, detected, "acme_sh"}, " ")
	redacted, err := detector.Redact(text)
	if err != nil {
		t.Fatal(err)
	}
	want := strings.Join([]string{byValue, byHash, Placeholder(key, []byte(detected)), "acme_sh"}, " ")
	if redacted != want {
		t.Fatalf("redaction = %q, want %q", redacted, want)
	}
	records := mustLoad(t, vault)
	if _, ok := records[Placeholder(key, []byte(byHash))]; ok {
		t.Fatal("excluded value was learned")
	}
	if _, ok := records[Placeholder(key, []byte("acme_sh"))]; ok {
		t.Fatal("value shorter than 8 characters was learned")
	}
	if err := detector.Learn([]byte("acme_sh"), "token", "user"); err != nil {
		t.Fatal(err)
	}
	// A placeholder answer (SPEC-0013 §7) is not a value.
	if err := detector.Learn([]byte(Placeholder(key, []byte(byValue))), "token", "user"); err != nil {
		t.Fatal(err)
	}
	if len(mustLoad(t, vault)) != 2 {
		t.Fatal("a short value or a placeholder was learned")
	}
}

func TestDetectorReusesActiveRecordsWithoutGrowingVault(t *testing.T) {
	master := bytes.Repeat([]byte{32}, 32)
	path := testVaultPath(t)
	detector, _ := newTestDetector(t, master, path, DetectionPolicy{})
	value := "ghp_" + strings.Repeat("2", 36)
	if err := detector.Learn([]byte(value), "github_token", "env"); err != nil {
		t.Fatal(err)
	}
	grown := vaultLines(t, path)
	for range 3 {
		if err := detector.Learn([]byte(value), "github_token", "env"); err != nil {
			t.Fatal(err)
		}
		if _, err := detector.Redact("token " + value); err != nil {
			t.Fatal(err)
		}
	}
	if lines := vaultLines(t, path); lines != grown {
		t.Fatalf("repeated observation grew the vault from %d to %d records", grown, lines)
	}
}

func TestDetectorPreservesExplicitRecordMetadata(t *testing.T) {
	master := bytes.Repeat([]byte{33}, 32)
	path := testVaultPath(t)
	detector, vault := newTestDetector(t, master, path, DetectionPolicy{})
	key := mustHMACKey(t, master)
	value := "ghp_" + strings.Repeat("3", 36)
	placeholder := Placeholder(key, []byte(value))
	if err := vault.Put(Record{
		Placeholder: placeholder, Kind: "github_token", Source: "user", Scope: "project",
		ProjectID: "project-a", Name: "ci-token", Env: "CI_TOKEN",
	}, []byte(value)); err != nil {
		t.Fatal(err)
	}
	initial := vaultLines(t, path)
	if got, err := detector.Redact("token " + value); err != nil || got != "token "+placeholder {
		t.Fatalf("redaction = %q, err %v", got, err)
	}
	if err := detector.Learn([]byte(value), "token", "env"); err != nil {
		t.Fatal(err)
	}
	record := mustLoad(t, vault)[placeholder]
	if record.Kind != "github_token" || record.Source != "user" || record.Scope != "project" ||
		record.ProjectID != "project-a" || record.Name != "ci-token" || record.Env != "CI_TOKEN" {
		t.Fatalf("explicit metadata was rewritten by detection or Learn")
	}
	if lines := vaultLines(t, path); lines != initial {
		t.Fatalf("reuse appended %d records", lines-initial)
	}
	// A soft delete is not permanent: the next detection learns the value again.
	if err := vault.Delete(placeholder); err != nil {
		t.Fatal(err)
	}
	if _, err := detector.Redact("token " + value); err != nil {
		t.Fatal(err)
	}
	record = mustLoad(t, vault)[placeholder]
	if record.Deleted || record.Source != "detected" || record.Scope != "global" {
		t.Fatalf("deleted record was not learned again: deleted=%v source=%q", record.Deleted, record.Source)
	}
}

func TestDetectorRedactsShortStoredValues(t *testing.T) {
	master := bytes.Repeat([]byte{30}, 32)
	detector, vault := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	key := mustHMACKey(t, master)
	// Below the autodetection minimum, but explicitly registered by the user.
	short := "sh0rt"
	placeholder := Placeholder(key, []byte(short))
	if err := vault.Put(Record{
		Placeholder: placeholder, Kind: "token", Source: "user", Scope: "global",
	}, []byte(short)); err != nil {
		t.Fatal(err)
	}
	text := "pw=" + short + "; again " + short
	redacted, err := detector.Redact(text)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(redacted, short) || redacted != "pw="+placeholder+"; again "+placeholder {
		t.Fatalf("short stored value was not redacted: %q", redacted)
	}
	if restored, err := detector.Substitute(redacted); err != nil || restored != text {
		t.Fatalf("substitution = %q, err %v", restored, err)
	}
	if got, err := detector.Redact("see " + placeholder + " and " + short); err != nil || got != "see "+placeholder+" and "+placeholder {
		t.Fatalf("existing placeholder was not protected: %q, err %v", got, err)
	}
}

func TestDetectorLearnsValuesContainingPlaceholderText(t *testing.T) {
	master := bytes.Repeat([]byte{31}, 32)
	detector, vault := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	key := mustHMACKey(t, master)
	embedded := Placeholder(key, []byte("embedded-placeholder-value"))
	value := "cred-" + embedded + "-tail"
	if err := detector.Learn([]byte(value), "token", "user"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustLoad(t, vault)[Placeholder(key, []byte(value))]; !ok {
		t.Fatal("a credential containing placeholder text was not learned")
	}
	// Immunity covers a whole placeholder, not a larger credential.
	if err := detector.Learn([]byte(embedded), "token", "user"); err != nil {
		t.Fatal(err)
	}
	if _, ok := mustLoad(t, vault)[Placeholder(key, []byte(embedded))]; ok {
		t.Fatal("a canonical placeholder was learned as a value")
	}
	if got, err := detector.Redact("see " + embedded + " and " + embedded); err != nil || got != "see "+embedded+" and "+embedded {
		t.Fatalf("placeholder was altered: %q, err %v", got, err)
	}
}

func TestDetectorReplacesOverlappingValuesOnce(t *testing.T) {
	master := bytes.Repeat([]byte{24}, 32)
	detector, _ := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	key := mustHMACKey(t, master)
	outer := "overlapping-secret-value-outer"
	inner := "overlapping-secret-value"
	middle := "secret-value"
	for _, value := range []string{outer, inner, middle} {
		if err := detector.Learn([]byte(value), "token", "user"); err != nil {
			t.Fatal(err)
		}
	}
	text := outer + " | " + inner + " | " + middle
	want := Placeholder(key, []byte(outer)) + " | " + Placeholder(key, []byte(inner)) + " | " + Placeholder(key, []byte(middle))
	got, err := detector.Redact(text)
	if err != nil || got != want {
		t.Fatalf("overlapping replacement = %q, err %v", got, err)
	}
	if restored, err := detector.Substitute(got); err != nil || restored != text {
		t.Fatalf("overlapping substitution = %q, err %v", restored, err)
	}
}

func TestDetectorSubstituteRejectsUnknownDeletedAndTampered(t *testing.T) {
	master := bytes.Repeat([]byte{25}, 32)
	detector, vault := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	key := mustHMACKey(t, master)
	value := []byte("substitute-known-value")
	if err := detector.Learn(value, "token", "user"); err != nil {
		t.Fatal(err)
	}
	placeholder := Placeholder(key, value)
	if got, err := detector.Substitute("plain text"); err != nil || got != "plain text" {
		t.Fatalf("text without placeholders = %q, err %v", got, err)
	}
	if err := vault.Delete(placeholder); err != nil {
		t.Fatal(err)
	}
	for _, text := range []string{placeholder, "before " + placeholder + " after", "red-sec-aaaaaaaaaaaaaaaa"} {
		got, err := detector.Substitute(text)
		var invalid *ErrRecordInvalid
		if !errors.As(err, &invalid) || got != "" {
			t.Fatalf("unknown or deleted placeholder returned %q, err %v", got, err)
		}
	}

	tampered, tamperedVault := newTestDetector(t, master, testVaultPath(t), DetectionPolicy{})
	forged := Placeholder(key, []byte("forged-record-placeholder"))
	if err := tamperedVault.Put(Record{
		Placeholder: forged, Kind: "token", Source: "user", Scope: "global",
	}, []byte("value-that-hashes-elsewhere")); err != nil {
		t.Fatal(err)
	}
	if got, err := tampered.Substitute(forged); err == nil || got != "" {
		t.Fatalf("tampered placeholder returned %q, err %v", got, err)
	}
	if got, err := tampered.Redact("withheld"); err == nil || got != "" {
		t.Fatalf("redaction over a tampered record returned %q, err %v", got, err)
	}
}

func TestDetectorFailsClosedOnVaultFailures(t *testing.T) {
	master := bytes.Repeat([]byte{26}, 32)
	path := testVaultPath(t)
	detector, _ := newTestDetector(t, master, path, DetectionPolicy{})
	if err := detector.Learn([]byte("learned-before-the-torn-line"), "token", "user"); err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, path, []byte(`{"v":1,"placeholder":"torn`))
	if got, err := detector.Redact("token ghp_" + strings.Repeat("1", 36)); err == nil || got != "" {
		t.Fatalf("vault append failure returned %q, err %v", got, err)
	}
	if err := detector.Learn([]byte("another-secret-value"), "token", "user"); err == nil {
		t.Fatal("Learn succeeded while the vault rejected writes")
	}
	if err := detector.Learn([]byte("learned-before-the-torn-line"), "token", "user"); err == nil {
		t.Fatal("reusing an active record hid the incomplete final record")
	}

	corruptPath := testVaultPath(t)
	corrupt, corruptVault := newTestDetector(t, master, corruptPath, DetectionPolicy{})
	if err := corrupt.Learn([]byte("corrupted-ciphertext-value"), "token", "user"); err != nil {
		t.Fatal(err)
	}
	record, ok := mustLoad(t, corruptVault)[Placeholder(mustHMACKey(t, master), []byte("corrupted-ciphertext-value"))]
	if !ok {
		t.Fatal("record missing")
	}
	record.Placeholder = Placeholder(mustHMACKey(t, master), []byte("corrupted-ciphertext-other"))
	record.Ciphertext = base64.StdEncoding.EncodeToString([]byte("not-a-sealed-value"))
	line, err := json.Marshal(record)
	if err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, corruptPath, append(line, '\n'))
	got, err := corrupt.Redact("nothing to see")
	if err != nil || got != "nothing to see" {
		t.Fatalf("an undecryptable record must not make the boundary unavailable: %q, err %v", got, err)
	}
	if len(corrupt.Unreadable()) != 1 {
		t.Fatalf("unreadable record was not reported: %v", corrupt.Unreadable())
	}
	known := []byte("still-readable-secret-value")
	if err := corrupt.Learn(known, "token", "user"); err != nil {
		t.Fatal(err)
	}
	if got, err := corrupt.Redact("x " + string(known)); err != nil || strings.Contains(got, string(known)) {
		t.Fatalf("readable secret leaked next to an unreadable record: %q, err %v", got, err)
	}
}

func TestDetectorIgnoresRecordsEncryptedWithAForeignKey(t *testing.T) {
	path := testVaultPath(t)
	foreign, _ := newTestDetector(t, bytes.Repeat([]byte{41}, 32), path, DetectionPolicy{})
	if err := foreign.Learn([]byte("secret-written-by-another-agent-dir"), "token", "env"); err != nil {
		t.Fatal(err)
	}
	master := bytes.Repeat([]byte{42}, 32)
	own, _ := newTestDetector(t, master, path, DetectionPolicy{})
	if got, err := own.Redact("hello"); err != nil || got != "hello" {
		t.Fatalf("foreign-key record made the boundary fail: %q, err %v", got, err)
	}
	if len(own.Unreadable()) != 1 {
		t.Fatalf("foreign record not reported: %v", own.Unreadable())
	}
}

func TestDetectorRejectsInvalidPolicy(t *testing.T) {
	master := bytes.Repeat([]byte{28}, 32)
	vault, err := OpenVault(testVaultPath(t), master)
	if err != nil {
		t.Fatal(err)
	}
	cases := map[string]DetectionPolicy{
		"two kinds":            {Patterns: []Pattern{{Name: "both", Regex: `a{8}`, Prefix: "a"}}},
		"no kind":              {Patterns: []Pattern{{Name: "neither"}}},
		"duplicate name":       {Patterns: []Pattern{{Name: "twice", Regex: `a{8}`}, {Name: "twice", Regex: `b{8}`}}},
		"builtin name":         {Patterns: []Pattern{{Name: "jwt", Regex: `a{8}`}}},
		"short min length":     {Patterns: []Pattern{{Name: "short", Prefix: "sh", MinLength: 4}}},
		"min length on suffix": {Patterns: []Pattern{{Name: "suffix", Suffix: "-ops", MinLength: 12}}},
		"empty match":          {Patterns: []Pattern{{Name: "empty", Regex: `a*`}}},
		"invalid regex":        {Patterns: []Pattern{{Name: "broken", Regex: `[`}}},
		"invalid name":         {Patterns: []Pattern{{Name: "Not-A-Name", Regex: `a{8}`}}},
		"invalid hash":         {Exclusions: Exclusions{Hashes: []string{"not-a-hash"}}},
	}
	for name, policy := range cases {
		t.Run(name, func(t *testing.T) {
			if _, err := NewDetector(master, vault, policy); err == nil {
				t.Fatal("accepted an invalid detection policy")
			}
		})
	}
	if _, err := NewDetector(master, nil, DetectionPolicy{}); err == nil {
		t.Fatal("accepted a missing vault")
	}
	if _, err := NewDetector([]byte("short"), vault, DetectionPolicy{}); err == nil {
		t.Fatal("accepted an invalid master key")
	}
}

func TestDetectorConcurrentCalls(t *testing.T) {
	master := bytes.Repeat([]byte{29}, 32)
	path := testVaultPath(t)
	policy := DetectionPolicy{Patterns: []Pattern{{Name: "acme_token", Regex: `acme_[a-z0-9]{20,}`}}}
	first, vault := newTestDetector(t, master, path, policy)
	second, _ := newTestDetector(t, master, path, policy)
	key := mustHMACKey(t, master)
	const count = 16
	values := make([]string, count)
	for i := range values {
		values[i] = "acme_concurrentsecret" + fmt.Sprintf("%04d", i)
	}
	for i := 0; i < count; i += 2 {
		if err := first.Learn([]byte(values[i]), "token", "user"); err != nil {
			t.Fatal(err)
		}
	}
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range values {
		wg.Add(1)
		go func() {
			defer wg.Done()
			detector := first
			if i%3 == 1 {
				detector = second
			}
			text := "value=" + values[i]
			redacted, err := detector.Redact(text)
			if err != nil {
				errs <- err
				return
			}
			if redacted != "value="+Placeholder(key, []byte(values[i])) {
				errs <- fmt.Errorf("redaction %d is %q", i, redacted)
				return
			}
			if restored, err := detector.Substitute(redacted); err != nil || restored != text {
				errs <- fmt.Errorf("substitution %d failed: %w", i, err)
				return
			}
			errs <- nil
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	if records := mustLoad(t, vault); len(records) != count {
		t.Fatalf("vault holds %d records, want %d", len(records), count)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if bytes.Contains(data, []byte(value)) {
			t.Fatal("vault contains a plaintext concurrent value")
		}
	}
}

func vaultLines(t *testing.T, path string) int {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return bytes.Count(data, []byte{'\n'})
}

func mustLoad(t *testing.T, vault *Vault) map[string]Record {
	t.Helper()
	records, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	return records
}

func mustHMACKey(t *testing.T, master []byte) []byte {
	t.Helper()
	key, err := DeriveHMACKey(master)
	if err != nil {
		t.Fatal(err)
	}
	return key
}

func TestDetectorExcludesValuesMatchingAnExclusionPattern(t *testing.T) {
	master := bytes.Repeat([]byte{29}, 32)
	policy := DetectionPolicy{
		Patterns:   []Pattern{{Name: "acme_token", Regex: `acme_[a-z0-9]+`}},
		Exclusions: Exclusions{Patterns: []string{`acme_demo[a-z0-9]*`}},
	}
	detector, _ := newTestDetector(t, master, testVaultPath(t), policy)
	got, err := detector.Redact("acme_demovalue1 acme_realvalue1")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(got, "acme_demovalue1 red-sec-") {
		t.Fatalf("an excluded shape was redacted or a real value leaked: %q", got)
	}
	vault, err := OpenVault(testVaultPath(t), master)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := NewDetector(master, vault, DetectionPolicy{Exclusions: Exclusions{Patterns: []string{"("}}}); err == nil {
		t.Fatal("an invalid exclusion pattern was accepted")
	}
}
