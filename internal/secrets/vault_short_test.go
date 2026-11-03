package secrets

import (
	"bytes"
	"path/filepath"
	"testing"
)

func TestVaultDoesNotRejectIncidentalPlaceholderOrEnumOverlap(t *testing.T) {
	master := bytes.Repeat([]byte{7}, 32)
	vault, err := OpenVault(filepath.Join(t.TempDir(), "private", "vault.jsonl"), master)
	if err != nil {
		t.Fatal(err)
	}
	for _, value := range []string{"red", "user", "global"} {
		record := vaultTestRecord(t, master, []byte(value))
		if err := vault.Put(record, []byte(value)); err != nil {
			t.Fatalf("valid short secret rejected: %v", err)
		}
		actual, err := vault.Value(record.Placeholder)
		if err != nil {
			t.Fatal(err)
		}
		if string(actual) != value {
			t.Fatal("short secret did not round-trip")
		}
	}
}
