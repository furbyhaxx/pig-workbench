package secrets

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

func TestKeyLoadsAndRedaction(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "private")
	path := filepath.Join(dir, "secret.key")
	created, err := CreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 32 {
		t.Fatalf("created key length = %d, want 32", len(created))
	}
	for _, item := range []struct {
		path string
		mode os.FileMode
	}{{dir, 0700}, {path, 0600}} {
		info, err := os.Stat(item.path)
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != item.mode {
			t.Errorf("%s: mode = %04o, want %04o", item.path, info.Mode().Perm(), item.mode)
		}
	}
	first, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := LoadKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(created, first) || !bytes.Equal(first, second) {
		t.Fatal("independently loaded keys differ")
	}
	hmac1, err := DeriveHMACKey(first)
	if err != nil {
		t.Fatal(err)
	}
	hmac2, err := DeriveHMACKey(second)
	if err != nil {
		t.Fatal(err)
	}
	short := "inside-value"
	long := "outer-inside-value-suffix"
	shortPlaceholder := Placeholder(hmac1, []byte(short))
	longPlaceholder := Placeholder(hmac1, []byte(long))
	if longPlaceholder != Placeholder(hmac2, []byte(long)) {
		t.Fatal("placeholder changed across independent key loads")
	}
	known := map[string]string{short: shortPlaceholder, long: longPlaceholder}
	text := "first: " + long + "; second: " + short
	want := "first: " + longPlaceholder + "; second: " + shortPlaceholder
	got, err := Redact(text, hmac1, known)
	if err != nil {
		t.Fatal(err)
	}
	if got != want {
		t.Fatal("redaction did not replace the longer value first")
	}
	again, err := Redact(got, hmac1, known)
	if err != nil {
		t.Fatal(err)
	}
	if again != got {
		t.Fatal("redaction is not idempotent")
	}
}

func TestKeyPermissionsAndExistingKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "secret.key")
	original, err := CreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	again, err := CreateKey(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(original, again) {
		t.Fatal("existing key was replaced")
	}
	if err := os.Chmod(path, 0644); err != nil {
		t.Fatal(err)
	}
	for name, open := range map[string]func(string) ([]byte, error){"load": LoadKey, "create": CreateKey} {
		t.Run(name, func(t *testing.T) {
			_, err := open(path)
			var permissionErr *KeyPermissionError
			if !errors.As(err, &permissionErr) {
				t.Fatalf("expected KeyPermissionError, got %v", err)
			}
			if permissionErr.Mode.Perm() != 0644 || permissionErr.Path != path {
				t.Fatal("permission error does not identify actual mode and path")
			}
			if !strings.Contains(err.Error(), "0644") || !strings.Contains(err.Error(), "chmod") {
				t.Fatalf("permission error lacks mode or fix: %v", err)
			}
		})
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0644 {
		t.Fatal("existing file permissions were silently changed")
	}
}

func TestConcurrentCreateKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "private", "secret.key")
	type result struct {
		key []byte
		err error
	}
	const count = 16
	start := make(chan struct{})
	results := make(chan result, count)
	for range count {
		go func() {
			<-start
			key, err := CreateKey(path)
			results <- result{key: key, err: err}
		}()
	}
	close(start)
	var first []byte
	for range count {
		result := <-results
		if result.err != nil {
			t.Error(result.err)
			continue
		}
		if first == nil {
			first = result.key
		} else if !bytes.Equal(first, result.key) {
			t.Error("concurrent creation returned different keys")
		}
	}
	if t.Failed() {
		return
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name() != "secret.key" {
		t.Fatal("temporary key files were not removed")
	}
}

func TestLoadRejectsInvalidKeyFiles(t *testing.T) {
	for _, size := range []int{0, 31, 33} {
		t.Run(strconv.Itoa(size), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret.key")
			if err := os.WriteFile(path, bytes.Repeat([]byte{'x'}, size), 0600); err != nil {
				t.Fatal(err)
			}
			if _, err := LoadKey(path); err == nil {
				t.Fatal("accepted invalid key size")
			}
			if _, err := CreateKey(path); err == nil {
				t.Fatal("accepted existing invalid key")
			}
			info, err := os.Stat(path)
			if err != nil || info.Size() != int64(size) {
				t.Fatal("invalid existing key was changed")
			}
		})
	}
	if _, err := LoadKey(t.TempDir()); err == nil {
		t.Fatal("accepted non-regular key file")
	}
}

func TestPlaceholderDerivationAndDetection(t *testing.T) {
	master := make([]byte, 32)
	for i := range master {
		master[i] = byte(i)
	}
	key, err := DeriveHMACKey(master)
	if err != nil {
		t.Fatal(err)
	}
	one := Placeholder(key, []byte("workbench-test-value"))
	two := Placeholder(key, []byte("workbench-test-other"))
	if one != "red-sec-24k74dbuggakxbr2" {
		t.Fatal("HKDF/HMAC/base32 does not match independent test vector")
	}
	if one == two {
		t.Fatal("distinct values have the same placeholder")
	}
	valid := regexp.MustCompile(`^red-sec-[a-z2-7]{16}$`)
	if !valid.MatchString(one) || !valid.MatchString(two) {
		t.Fatal("placeholder has invalid length or base32 alphabet")
	}
	found := DetectPlaceholder(one + " " + two + " " + one + " red-sec-0123456789abcdef")
	if len(found) != 2 || found[0] != one || found[1] != two {
		t.Fatalf("expected two distinct placeholders in occurrence order, got %d", len(found))
	}
	if _, err := DeriveHMACKey(nil); err == nil {
		t.Fatal("accepted a missing master key")
	}
}

func TestRedactProtectsPlaceholdersAndFailsClosed(t *testing.T) {
	key, err := DeriveHMACKey(bytes.Repeat([]byte{42}, 32))
	if err != nil {
		t.Fatal(err)
	}
	short := "abcdefgh"
	long := "bcdefghijk"
	placeholder := "red-sec-abcdefghijklmnop"
	known := map[string]string{
		short:              Placeholder(key, []byte(short)),
		long:               Placeholder(key, []byte(long)),
		"abcdefghijklmnop": Placeholder(key, []byte("abcdefghijklmnop")),
	}
	got, err := Redact("abcdefghijk "+placeholder, key, known)
	if err != nil {
		t.Fatal(err)
	}
	if got != "a"+known[long]+" "+placeholder {
		t.Fatal("longest overlapping match or existing placeholder was mishandled")
	}
	for _, invalid := range [][]byte{nil, make([]byte, 31)} {
		got, err := Redact("withheld", invalid, known)
		if err == nil || got != "" {
			t.Fatal("invalid HMAC key did not fail closed")
		}
	}
	got, err = Redact("withheld", key, map[string]string{"withheld": "red-sec-abcdefghijklmnop"})
	if err == nil || got != "" {
		t.Fatal("invalid mapping did not fail closed")
	}
}

func TestRedactReplacesLargerSecretContainingPlaceholder(t *testing.T) {
	key, err := DeriveHMACKey(bytes.Repeat([]byte{43}, 32))
	if err != nil {
		t.Fatal(err)
	}
	canonical := "red-sec-abcdefghijklmnop"
	secret := "prefix " + canonical + " suffix"
	placeholder := Placeholder(key, []byte(secret))
	known := map[string]string{secret: placeholder}
	got, err := Redact("value "+secret+" standalone "+canonical, key, known)
	if err != nil {
		t.Fatal(err)
	}
	if want := "value " + placeholder + " standalone " + canonical; got != want {
		t.Fatalf("redaction = %q, want %q", got, want)
	}
	// A value overlapping a placeholder must not leak; the larger occurrence wins.
	partial := "prefix " + canonical[:12]
	partialPlaceholder := Placeholder(key, []byte(partial))
	got, err = Redact("value "+partial+canonical[12:], key, map[string]string{partial: partialPlaceholder})
	if err != nil {
		t.Fatal(err)
	}
	if got != "value "+partialPlaceholder {
		t.Fatalf("overlapping placeholder secret = %q, want %q", got, partialPlaceholder)
	}
}
