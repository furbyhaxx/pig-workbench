package secrets

import (
	"bytes"
	"crypto/aes"
	"crypto/cipher"
	"crypto/hkdf"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Record is an encrypted value and its latest metadata in the vault.
type Record struct {
	Version     int       `json:"v"`
	Placeholder string    `json:"placeholder"`
	Ciphertext  string    `json:"ciphertext"`
	Nonce       string    `json:"nonce"`
	Kind        string    `json:"kind"`
	Source      string    `json:"source"`
	Scope       string    `json:"scope"`
	ProjectID   string    `json:"projectId,omitempty"`
	Name        string    `json:"name,omitempty"`
	Env         string    `json:"env,omitempty"`
	Description string    `json:"description,omitempty"`
	Created     time.Time `json:"created"`
	Deleted     bool      `json:"deleted,omitempty"`
}

// ErrVaultLocked reports failure to obtain or verify the vault lock.
type ErrVaultLocked struct{}

// Error describes the lock failure without including vault content.
func (*ErrVaultLocked) Error() string { return "secret vault lock unavailable or unsafe" }

// ErrRecordInvalid reports an invalid, missing, or deleted record.
type ErrRecordInvalid struct{}

// Error describes the invalid record without including its contents.
func (*ErrRecordInvalid) Error() string { return "invalid or unavailable secret vault record" }

// ErrDecrypt reports that a record could not be authenticated and decrypted.
type ErrDecrypt struct{}

// Error describes the decryption failure without including record contents.
func (*ErrDecrypt) Error() string { return "secret vault record could not be decrypted" }

// Vault accesses one encrypted append-only vault. Its methods are safe for concurrent use.
type Vault struct {
	path     string
	dir      string
	dirInfo  os.FileInfo
	lockInfo os.FileInfo
	key      []byte
	mu       sync.RWMutex
	warnings []string
}

// OpenVault opens or creates a vault under a private directory.
func OpenVault(path string, master []byte) (*Vault, error) {
	if len(master) != keySize {
		return nil, errors.New("master key must contain exactly 32 bytes")
	}
	key, err := hkdf.Key(sha256.New, master, nil, "pig-workbench secret value v1", keySize)
	if err != nil {
		return nil, errors.New("cannot derive secret vault key")
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return nil, errors.New("cannot resolve secret vault path")
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, errors.New("cannot create secret vault directory")
	}
	info, err := os.Lstat(dir)
	if err != nil || !info.IsDir() {
		return nil, errors.New("secret vault directory must be a directory, not a symlink")
	}
	if err := checkVaultMode(info, 0700); err != nil {
		return nil, err
	}
	v := &Vault{path: path, dir: dir, dirInfo: info, key: key}
	lock, err := v.lock()
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	v.lockInfo, err = lock.Stat()
	if err != nil {
		return nil, &ErrVaultLocked{}
	}
	file, err := v.openFile(true)
	if err != nil {
		return nil, err
	}
	if err := file.Close(); err != nil {
		return nil, errors.New("cannot close secret vault")
	}
	return v, nil
}

// Load returns the latest metadata and first ciphertext for every placeholder.
// An incomplete final line is ignored and reported through Warnings.
func (v *Vault) Load() (map[string]Record, error) {
	lock, err := v.lock()
	if err != nil {
		return nil, err
	}
	defer lock.Close()
	file, err := v.openFile(false)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	records, torn, err := readVault(file)
	if err != nil {
		return nil, err
	}
	v.mu.Lock()
	v.warnings = nil
	if torn {
		v.warnings = []string{"ignored incomplete final secret vault record; writes are blocked until it is repaired"}
	}
	v.mu.Unlock()
	return records, nil
}

// Warnings returns diagnostics from the most recent successful Load.
func (v *Vault) Warnings() []string {
	v.mu.RLock()
	defer v.mu.RUnlock()
	return append([]string(nil), v.warnings...)
}

// Put inserts a value if absent and appends its latest metadata.
// Source must be detected, env, env-file, auth or user; scope must be global
// or project, with a project ID for project scope. Metadata must not contain
// the supplied value or the value already stored for that placeholder.
func (v *Vault) Put(record Record, value []byte) error {
	return v.put(record, value, false)
}

func (v *Vault) put(record Record, value []byte, preserveActive bool) error {
	return v.mutate(func(records map[string]Record) (Record, error) {
		if record.Version != 0 && record.Version != 1 {
			return Record{}, &ErrRecordInvalid{}
		}
		record.Version = 1
		record.Deleted = false
		old, exists := records[record.Placeholder]
		if preserveActive && exists && !old.Deleted {
			if _, err := v.decrypt(old); err != nil {
				return Record{}, err
			}
			return Record{}, errVaultUnchanged
		}
		if record.Created.IsZero() {
			if exists {
				record.Created = old.Created
			} else {
				record.Created = time.Now().UTC()
			}
		}
		if !validVaultRecord(record) || containsVaultValue(record, value) {
			return Record{}, &ErrRecordInvalid{}
		}
		if exists {
			original, err := v.decrypt(old)
			if err != nil {
				return Record{}, err
			}
			if containsVaultValue(record, original) {
				return Record{}, &ErrRecordInvalid{}
			}
			record.Ciphertext, record.Nonce = old.Ciphertext, old.Nonce
			return record, nil
		}
		aead, err := v.aead()
		if err != nil {
			return Record{}, err
		}
		nonce := make([]byte, aead.NonceSize())
		if _, err := rand.Read(nonce); err != nil {
			return Record{}, errors.New("cannot generate secret vault nonce")
		}
		sealed := aead.Seal(nil, nonce, value, []byte(record.Placeholder))
		record.Ciphertext = base64.StdEncoding.EncodeToString(sealed)
		record.Nonce = base64.StdEncoding.EncodeToString(nonce)
		return record, nil
	})
}

// SetMetadata replaces metadata for an existing, non-deleted record.
func (v *Vault) SetMetadata(record Record) error {
	return v.mutate(func(records map[string]Record) (Record, error) {
		old, ok := records[record.Placeholder]
		if !ok || old.Deleted || (record.Version != 0 && record.Version != 1) {
			return Record{}, &ErrRecordInvalid{}
		}
		record.Version = 1
		record.Deleted = false
		record.Ciphertext, record.Nonce = old.Ciphertext, old.Nonce
		if record.Created.IsZero() {
			record.Created = old.Created
		}
		value, err := v.decrypt(old)
		if err != nil {
			return Record{}, err
		}
		if !validVaultRecord(record) || containsVaultValue(record, value) {
			return Record{}, &ErrRecordInvalid{}
		}
		return record, nil
	})
}

// Delete appends a soft deletion for an existing, non-deleted record.
func (v *Vault) Delete(placeholder string) error {
	return v.mutate(func(records map[string]Record) (Record, error) {
		record, ok := records[placeholder]
		if !ok || record.Deleted {
			return Record{}, &ErrRecordInvalid{}
		}
		record.Deleted = true
		return record, nil
	})
}

// Value decrypts one active record; missing and deleted records return ErrRecordInvalid.
func (v *Vault) Value(placeholder string) ([]byte, error) {
	records, err := v.Load()
	if err != nil {
		return nil, err
	}
	record, ok := records[placeholder]
	if !ok || record.Deleted {
		return nil, &ErrRecordInvalid{}
	}
	return v.decrypt(record)
}

// Values decrypts active records, returning successful values and ErrDecrypt if any fail.
// Callers enforcing a redaction boundary must withhold output on any error.
func (v *Vault) Values() (map[string][]byte, error) {
	records, err := v.Load()
	if err != nil {
		return nil, err
	}
	values := make(map[string][]byte, len(records))
	var decryptErr error
	for placeholder, record := range records {
		if record.Deleted {
			continue
		}
		value, err := v.decrypt(record)
		if err != nil {
			decryptErr = err
			continue
		}
		values[placeholder] = value
	}
	return values, decryptErr
}

func containsVaultValue(record Record, value []byte) bool {
	if len(value) == 0 {
		return false
	}
	for _, field := range []string{record.Name, record.Env, record.Description} {
		if bytes.Contains([]byte(field), value) {
			return true
		}
	}
	return false
}

func (v *Vault) aead() (cipher.AEAD, error) {
	block, err := aes.NewCipher(v.key)
	if err != nil {
		return nil, errors.New("cannot initialize secret vault cipher")
	}
	aead, err := cipher.NewGCM(block)
	if err != nil {
		return nil, errors.New("cannot initialize secret vault cipher")
	}
	return aead, nil
}

func (v *Vault) decrypt(record Record) ([]byte, error) {
	aead, err := v.aead()
	if err != nil {
		return nil, err
	}
	nonce, err := base64.StdEncoding.DecodeString(record.Nonce)
	if err != nil || len(nonce) != aead.NonceSize() {
		return nil, &ErrDecrypt{}
	}
	data, err := base64.StdEncoding.DecodeString(record.Ciphertext)
	if err != nil {
		return nil, &ErrDecrypt{}
	}
	value, err := aead.Open(nil, nonce, data, []byte(record.Placeholder))
	if err != nil {
		return nil, &ErrDecrypt{}
	}
	return value, nil
}

// EnvInjection selects active project and global values by environment name.
// Project scope wins; ties within a scope use the lexicographically first placeholder.
func EnvInjection(records map[string]Record, values map[string][]byte, projectID string) map[string]string {
	type selected struct {
		placeholder string
		rank        int
	}
	choices := make(map[string]selected)
	result := make(map[string]string)
	for placeholder, record := range records {
		value, ok := values[placeholder]
		if !ok || record.Deleted || record.Env == "" {
			continue
		}
		rank := 0
		switch {
		case record.Scope == "global":
			rank = 1
		case record.Scope == "project" && projectID != "" && record.ProjectID == projectID:
			rank = 2
		default:
			continue
		}
		old, exists := choices[record.Env]
		if exists && (old.rank > rank || (old.rank == rank && old.placeholder < placeholder)) {
			continue
		}
		choices[record.Env] = selected{placeholder: placeholder, rank: rank}
		result[record.Env] = string(value)
	}
	return result
}
