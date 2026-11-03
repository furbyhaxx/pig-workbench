package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/sha256"
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

func vaultTestRecord(t *testing.T, master, value []byte) Record {
	t.Helper()
	key, err := DeriveHMACKey(master)
	if err != nil {
		t.Fatal(err)
	}
	return Record{Placeholder: Placeholder(key, value), Kind: "token", Source: "user", Scope: "global"}
}

func TestVaultIndependentConcurrentInstances(t *testing.T) {
	master := bytes.Repeat([]byte{9}, 32)
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "vault.jsonl")
	first, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	second, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	const count = 40
	records := make([]Record, count)
	values := make([][]byte, count)
	for i := range count {
		values[i] = []byte(fmt.Sprintf("test-private-value-%04d", i))
		records[i] = vaultTestRecord(t, master, values[i])
	}
	var wg sync.WaitGroup
	errs := make(chan error, count)
	for i := range count {
		wg.Add(1)
		go func() {
			defer wg.Done()
			vault := first
			if i%2 == 1 {
				vault = second
			}
			errs <- vault.Put(records[i], values[i])
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	third, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	loaded, err := third.Load()
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded) != count {
		t.Fatalf("loaded %d records, want %d", len(loaded), count)
	}
	for i, record := range records {
		got, err := third.Value(record.Placeholder)
		if err != nil || !bytes.Equal(got, values[i]) {
			t.Fatalf("record %d: value mismatch or error: %v", i, err)
		}
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range values {
		if bytes.Contains(data, value) {
			t.Fatal("vault contains plaintext")
		}
	}
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{{dir, 0700}, {path, 0600}, {path + ".lock", 0600}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != item.mode {
			t.Errorf("%s: mode = %04o, want %04o", item.path, info.Mode().Perm(), item.mode)
		}
	}
}

func TestVaultFirstValueMetadataAndDelete(t *testing.T) {
	master := bytes.Repeat([]byte{11}, 32)
	vault, err := OpenVault(filepath.Join(t.TempDir(), "private", "vault.jsonl"), master)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("original-secret-value")
	record := vaultTestRecord(t, master, value)
	if err := vault.Put(record, value); err != nil {
		t.Fatal(err)
	}
	first, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	record.Name = "updated by duplicate"
	if err := vault.Put(record, []byte("replacement-secret-value")); err != nil {
		t.Fatal(err)
	}
	second, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	if second[record.Placeholder].Ciphertext != first[record.Placeholder].Ciphertext || second[record.Placeholder].Nonce != first[record.Placeholder].Nonce {
		t.Fatal("duplicate value changed ciphertext")
	}
	forged := second[record.Placeholder]
	forged.Ciphertext = base64.StdEncoding.EncodeToString([]byte("different-ciphertext"))
	line, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, vault.path, append(line, '\n'))
	loadedFirst, err := vault.Load()
	if err != nil || loadedFirst[record.Placeholder].Ciphertext != first[record.Placeholder].Ciphertext {
		t.Fatalf("a later ciphertext replaced the first: %v", err)
	}
	got, err := vault.Value(record.Placeholder)
	if err != nil || !bytes.Equal(got, value) {
		t.Fatalf("duplicate replaced first value: %v", err)
	}
	updated := second[record.Placeholder]
	updated.Name = "renamed"
	updated.Env = "APP_TOKEN"
	if err := vault.SetMetadata(updated); err != nil {
		t.Fatal(err)
	}
	loaded, err := vault.Load()
	if err != nil || loaded[record.Placeholder].Name != "renamed" || loaded[record.Placeholder].Env != "APP_TOKEN" {
		t.Fatalf("metadata update missing: %v", err)
	}
	if err := vault.Delete(record.Placeholder); err != nil {
		t.Fatal(err)
	}
	loaded, err = vault.Load()
	if err != nil || !loaded[record.Placeholder].Deleted {
		t.Fatalf("soft delete missing: %v", err)
	}
	for _, placeholder := range []string{record.Placeholder, vaultTestRecord(t, master, []byte("unknown-secret-value")).Placeholder} {
		_, err := vault.Value(placeholder)
		var invalid *ErrRecordInvalid
		if !errors.As(err, &invalid) {
			t.Fatalf("deleted or unknown value error = %v", err)
		}
	}
	all, err := vault.Values()
	if err != nil || len(all) != 0 {
		t.Fatalf("deleted value still present: %v", err)
	}
}

func TestVaultEncryptionFormatAndMetadataSafety(t *testing.T) {
	master := bytes.Repeat([]byte{12}, 32)
	path := filepath.Join(t.TempDir(), "private", "vault.jsonl")
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("metadata-sensitive-test-value")
	record := vaultTestRecord(t, master, value)
	record.Name = string(value)
	var invalid *ErrRecordInvalid
	if err := vault.Put(record, value); !errors.As(err, &invalid) || strings.Contains(err.Error(), string(value)) {
		t.Fatalf("plaintext metadata was accepted or leaked in an error")
	}
	record.Name = "safe"
	if err := vault.Put(record, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	stored := loaded[record.Placeholder]
	key, err := hkdf.Key(sha256.New, master, nil, "pig-workbench secret value v1", 32)
	if err != nil {
		t.Fatal(err)
	}
	block, err := aes.NewCipher(key)
	if err != nil {
		t.Fatal(err)
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		t.Fatal(err)
	}
	nonce, err := base64.StdEncoding.DecodeString(stored.Nonce)
	if err != nil || len(nonce) != 12 {
		t.Fatal("nonce is not a base64-encoded 96-bit value")
	}
	ciphertext, err := base64.StdEncoding.DecodeString(stored.Ciphertext)
	if err != nil {
		t.Fatal("ciphertext is not base64 encoded")
	}
	plain, err := aead.Open(nil, nonce, ciphertext, []byte(record.Placeholder))
	if err != nil || !bytes.Equal(plain, value) {
		t.Fatal("stored value does not match the specified encryption format")
	}
	stored.Description = string(value)
	if err := vault.SetMetadata(stored); !errors.As(err, &invalid) || strings.Contains(err.Error(), string(value)) {
		t.Fatal("plaintext metadata update was accepted or leaked in an error")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, value) {
		t.Fatal("vault contains plaintext in metadata")
	}
}

func TestVaultTornFinalLine(t *testing.T) {
	master := bytes.Repeat([]byte{13}, 32)
	path := filepath.Join(t.TempDir(), "private", "vault.jsonl")
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("first-secret-value")
	record := vaultTestRecord(t, master, value)
	if err := vault.Put(record, value); err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, path, []byte(`{"v":1,"placeholder":"torn`))
	loaded, err := vault.Load()
	if err != nil || len(loaded) != 1 {
		t.Fatalf("torn line was not ignored: %v", err)
	}
	if warnings := vault.Warnings(); len(warnings) != 1 || !strings.Contains(warnings[0], "incomplete") {
		t.Fatalf("missing torn-line warning: %v", warnings)
	}
	another := []byte("another-secret-value")
	var invalid *ErrRecordInvalid
	if err := vault.Put(vaultTestRecord(t, master, another), another); !errors.As(err, &invalid) {
		t.Fatalf("write after torn line must fail safely: %v", err)
	}
}

func TestVaultDecryptIsolatedAndAuthenticated(t *testing.T) {
	master := bytes.Repeat([]byte{15}, 32)
	path := filepath.Join(t.TempDir(), "private", "vault.jsonl")
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	value := []byte("auth-original-secret")
	record := vaultTestRecord(t, master, value)
	if err := vault.Put(record, value); err != nil {
		t.Fatal(err)
	}
	loaded, err := vault.Load()
	if err != nil {
		t.Fatal(err)
	}
	forged := loaded[record.Placeholder]
	forged.Placeholder = vaultTestRecord(t, master, []byte("different-auth-secret")).Placeholder
	data, err := json.Marshal(forged)
	if err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, path, append(data, '\n'))
	corrupt := loaded[record.Placeholder]
	corrupt.Placeholder = vaultTestRecord(t, master, []byte("corrupted-auth-secret")).Placeholder
	corrupt.Ciphertext = "invalid-base64!"
	data, err = json.Marshal(corrupt)
	if err != nil {
		t.Fatal(err)
	}
	appendVaultTestBytes(t, path, append(data, '\n'))
	if got, err := vault.Value(record.Placeholder); err != nil || !bytes.Equal(got, value) {
		t.Fatalf("valid record affected by corrupted record: %v", err)
	}
	_, err = vault.Value(forged.Placeholder)
	var decrypt *ErrDecrypt
	if !errors.As(err, &decrypt) {
		t.Fatalf("wrong AAD must return ErrDecrypt, got %v", err)
	}
	if _, err := vault.Value(corrupt.Placeholder); !errors.As(err, &decrypt) {
		t.Fatalf("corrupt ciphertext must return ErrDecrypt, got %v", err)
	}
	values, err := vault.Values()
	if !errors.As(err, &decrypt) || !bytes.Equal(values[record.Placeholder], value) || len(values) != 1 {
		t.Fatalf("corrupt record was not isolated: %v", err)
	}
}

func TestVaultRejectsUnsafeFilesAndMalformedRecords(t *testing.T) {
	master := bytes.Repeat([]byte{17}, 32)
	if _, err := OpenVault(filepath.Join(t.TempDir(), "vault"), []byte("short")); err == nil {
		t.Fatal("accepted invalid key")
	}
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "vault")
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Load(); err == nil {
		t.Fatal("accepted unsafe vault permissions")
	}
	if err := os.Chmod(path, 0600); err != nil {
		t.Fatal(err)
	}
	malformed := "sensitive-malformed-vault-record"
	appendVaultTestBytes(t, path, []byte(malformed+"\n"))
	var invalid *ErrRecordInvalid
	if _, err := vault.Load(); !errors.As(err, &invalid) || strings.Contains(err.Error(), malformed) {
		t.Fatalf("malformed committed line must fail closed without echoing content: %v", err)
	}
	if err := os.Chmod(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Load(); err == nil {
		t.Fatal("accepted unsafe directory permissions")
	}
}

func TestVaultRejectsSymlinks(t *testing.T) {
	master := bytes.Repeat([]byte{18}, 32)
	root := t.TempDir()
	private := filepath.Join(root, "private")
	if err := os.Mkdir(private, 0700); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "target")
	original := []byte("unchanged target")
	if err := os.WriteFile(target, original, 0600); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(private, "vault.jsonl")
	if err := os.Symlink(target, path); err != nil {
		t.Fatal(err)
	}
	if _, err := OpenVault(path, master); err == nil {
		t.Fatal("accepted a symlink vault")
	}
	contents, err := os.ReadFile(target)
	if err != nil || !bytes.Equal(contents, original) {
		t.Fatal("symlink target was modified")
	}
}

func TestVaultDoesNotRecreateMissingFilesOrTrustReplacedLock(t *testing.T) {
	master := bytes.Repeat([]byte{19}, 32)
	path := filepath.Join(t.TempDir(), "private", "vault.jsonl")
	vault, err := OpenVault(path, master)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path, path+".saved"); err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Load(); err == nil {
		t.Fatal("missing vault was silently recreated")
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("missing vault file was recreated")
	}
	if err := os.Rename(path+".saved", path); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(path+".lock", path+".lock.saved"); err != nil {
		t.Fatal(err)
	}
	var locked *ErrVaultLocked
	if _, err := vault.Load(); !errors.As(err, &locked) {
		t.Fatalf("replaced lock was not rejected: %v", err)
	}
}

func TestEnvInjectionScopePrecedence(t *testing.T) {
	records := map[string]Record{
		"global":      {Scope: "global", Env: "TOKEN"},
		"project":     {Scope: "project", ProjectID: "project-a", Env: "TOKEN"},
		"other":       {Scope: "project", ProjectID: "project-b", Env: "TOKEN"},
		"global-only": {Scope: "global", Env: "GLOBAL"},
		"deleted":     {Scope: "project", ProjectID: "project-a", Env: "DELETED", Deleted: true},
		"missing":     {Scope: "global", Env: "MISSING"},
	}
	values := map[string][]byte{
		"global": []byte("global-value"), "project": []byte("project-value"),
		"other": []byte("other-value"), "global-only": []byte("shared-value"),
		"deleted": []byte("deleted-value"),
	}
	got := EnvInjection(records, values, "project-a")
	if len(got) != 2 || got["TOKEN"] != "project-value" || got["GLOBAL"] != "shared-value" {
		t.Fatal("project precedence or scope filtering failed")
	}
	got = EnvInjection(records, values, "")
	if len(got) != 2 || got["TOKEN"] != "global-value" {
		t.Fatal("global-only session selected project secret")
	}
}

func appendVaultTestBytes(t *testing.T, path string, data []byte) {
	t.Helper()
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_APPEND, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	if _, err := file.Write(data); err != nil {
		t.Fatal(err)
	}
}
