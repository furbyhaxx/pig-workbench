package secrets

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/gofrs/flock"
)

func checkVaultMode(info os.FileInfo, allowed os.FileMode) error {
	mode := info.Mode()
	if mode.Perm() & ^allowed != 0 || mode&(os.ModeSetuid|os.ModeSetgid|os.ModeSticky) != 0 {
		return fmt.Errorf("secret vault permissions %04o are too wide; restrict to %04o", mode.Perm(), allowed)
	}
	return nil
}

func (v *Vault) checkDirectory() error {
	info, err := os.Lstat(v.dir)
	if err != nil || !info.IsDir() || !os.SameFile(v.dirInfo, info) {
		return errors.New("secret vault directory is unavailable or changed")
	}
	return checkVaultMode(info, 0700)
}

func (v *Vault) lock() (*flock.Flock, error) {
	if err := v.checkDirectory(); err != nil {
		return nil, err
	}
	path := v.path + ".lock"
	lock := flock.New(path, flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !locked {
		lock.Close()
		return nil, &ErrVaultLocked{}
	}
	fail := func() (*flock.Flock, error) {
		lock.Close()
		return nil, &ErrVaultLocked{}
	}
	if err := v.checkDirectory(); err != nil {
		lock.Close()
		return nil, err
	}
	info, err := lock.Stat()
	if err != nil || !info.Mode().IsRegular() || checkVaultMode(info, 0600) != nil || (v.lockInfo != nil && !os.SameFile(v.lockInfo, info)) {
		return fail()
	}
	current, err := os.Lstat(path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return fail()
	}
	return lock, nil
}

func (v *Vault) openFile(create bool) (*os.File, error) {
	flags := fileio.LockFlags() | os.O_APPEND
	if !create {
		flags &^= os.O_CREATE
	}
	file, err := os.OpenFile(v.path, flags, 0600)
	if err != nil {
		return nil, errors.New("cannot open secret vault")
	}
	fail := func(err error) (*os.File, error) {
		file.Close()
		return nil, err
	}
	info, err := file.Stat()
	if err != nil || !info.Mode().IsRegular() {
		return fail(errors.New("secret vault must be a regular file"))
	}
	current, err := os.Lstat(v.path)
	if err != nil || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return fail(errors.New("secret vault file changed while opening"))
	}
	if err := checkVaultMode(info, 0600); err != nil {
		return fail(err)
	}
	return file, nil
}

func readVault(file *os.File) (map[string]Record, bool, error) {
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return nil, false, errors.New("cannot seek secret vault")
	}
	records := make(map[string]Record)
	reader := bufio.NewReader(file)
	for {
		line, err := reader.ReadBytes('\n')
		if err != nil && !errors.Is(err, io.EOF) {
			return nil, false, errors.New("cannot read secret vault")
		}
		if errors.Is(err, io.EOF) {
			return records, len(line) != 0, nil
		}
		var record Record
		if json.Unmarshal(line, &record) != nil || !validVaultRecord(record) {
			return nil, false, &ErrRecordInvalid{}
		}
		if old, ok := records[record.Placeholder]; ok {
			record.Ciphertext, record.Nonce = old.Ciphertext, old.Nonce
		}
		records[record.Placeholder] = record
	}
}

func validVaultRecord(record Record) bool {
	if record.Version != 1 || len(record.Placeholder) != len("red-sec-")+16 || placeholderPattern.FindString(record.Placeholder) != record.Placeholder || record.Created.IsZero() {
		return false
	}
	switch record.Source {
	case "detected", "env", "env-file", "auth", "user":
	default:
		return false
	}
	switch record.Scope {
	case "global":
		return true
	case "project":
		return record.ProjectID != ""
	default:
		return false
	}
}

var errVaultUnchanged = errors.New("vault record is unchanged")

func (v *Vault) mutate(change func(map[string]Record) (Record, error)) error {
	lock, err := v.lock()
	if err != nil {
		return err
	}
	defer lock.Close()
	file, err := v.openFile(false)
	if err != nil {
		return err
	}
	defer file.Close()
	records, torn, err := readVault(file)
	if err != nil {
		return err
	}
	if torn {
		return &ErrRecordInvalid{}
	}
	record, err := change(records)
	if errors.Is(err, errVaultUnchanged) {
		return nil
	}
	if err != nil {
		return err
	}
	line, err := json.Marshal(record)
	if err != nil {
		return &ErrRecordInvalid{}
	}
	line = append(line, '\n')
	if n, err := file.Write(line); err != nil || n != len(line) {
		return errors.New("cannot append secret vault record; vault may contain an incomplete line")
	}
	if err := file.Sync(); err != nil {
		return errors.New("cannot flush secret vault record; write durability is uncertain")
	}
	return nil
}
