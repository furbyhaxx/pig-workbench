package endpoint

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// Registry serializes registry operations through a directory-wide advisory lock.
type Registry struct {
	dir  string
	info os.FileInfo
}

// Open opens or creates an absolute, private registry directory.
func Open(dir string) (*Registry, error) {
	if !filepath.IsAbs(dir) {
		return nil, errors.New("endpoint registry directory must be absolute")
	}
	if err := platformSupport(); err != nil {
		return nil, err
	}
	dir = filepath.Clean(dir)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return nil, fmt.Errorf("create endpoint registry: %w", err)
	}
	info, err := os.Lstat(dir)
	if err != nil {
		return nil, fmt.Errorf("inspect endpoint registry: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("endpoint registry must be a directory, not a symlink")
	}
	if err := checkPrivate(dir, info, 0700); err != nil {
		return nil, err
	}
	return &Registry{dir: dir, info: info}, nil
}

func (r *Registry) checkDirectory() error {
	info, err := os.Lstat(r.dir)
	if err != nil {
		return fmt.Errorf("inspect endpoint registry: %w", err)
	}
	if !info.IsDir() || !os.SameFile(r.info, info) {
		return errors.New("endpoint registry directory identity changed")
	}
	return checkPrivate(r.dir, info, 0700)
}

func (r *Registry) lock(ctx context.Context) (*flock.Flock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if err := r.checkDirectory(); err != nil {
		return nil, err
	}
	path := filepath.Join(r.dir, ".registry.lock")
	lock := flock.New(path, flock.SetFlag(fileio.LockFlags()), flock.SetPermissions(0600))
	locked, err := lock.TryLockContext(ctx, 10*time.Millisecond)
	if err != nil || !locked {
		lock.Close()
		if err == nil {
			err = ctx.Err()
		}
		return nil, fmt.Errorf("endpoint registry lock unavailable: %w", err)
	}
	fail := func(err error) (*flock.Flock, error) {
		lock.Close()
		return nil, err
	}
	if err := r.checkDirectory(); err != nil {
		return fail(err)
	}
	info, err := lock.Stat()
	if err != nil {
		return fail(fmt.Errorf("inspect endpoint registry lock: %w", err))
	}
	current, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() || !current.Mode().IsRegular() || !os.SameFile(info, current) {
		return fail(errors.New("endpoint registry lock must be an unchanged regular file"))
	}
	if err := checkPrivate(path, info, 0600); err != nil {
		return fail(err)
	}
	return lock, nil
}

func (r *Registry) recordPath(id string) (string, error) {
	if !validSessionID(id) {
		return "", errInvalidRecord
	}
	return filepath.Join(r.dir, id+".json"), nil
}

func readRecord(path, id string) (Record, error) {
	var record Record
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return record, ErrNotFound
	}
	if err != nil {
		return record, fmt.Errorf("inspect endpoint record: %w", err)
	}
	if !before.Mode().IsRegular() {
		return record, errInvalidRecord
	}
	file, info, err := fileio.OpenRegular(path)
	if errors.Is(err, os.ErrNotExist) {
		return record, ErrNotFound
	}
	if err != nil {
		return record, fmt.Errorf("open endpoint record: %w", err)
	}
	defer file.Close()
	if !os.SameFile(before, info) {
		return record, errors.New("endpoint record identity changed while opening")
	}
	if err := checkPrivate(path, info, 0600); err != nil {
		return record, err
	}
	if info.Size() > maxRecordSize {
		return record, ErrRecordTooLarge
	}
	data, err := io.ReadAll(io.LimitReader(file, maxRecordSize+1))
	if err != nil {
		return record, fmt.Errorf("read endpoint record: %w", err)
	}
	if len(data) > maxRecordSize {
		return record, ErrRecordTooLarge
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&record); err != nil {
		return Record{}, errInvalidRecord
	}
	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		return Record{}, errInvalidRecord
	}
	if err := validateRecord(record); err != nil || record.SessionID != id {
		return Record{}, errInvalidRecord
	}
	return record, nil
}

func encodeRecord(record Record) ([]byte, error) {
	if err := validateRecord(record); err != nil {
		return nil, err
	}
	if len(record.SessionID)+len(record.Token)+len(record.Cwd)+len(record.ProjectID)+len(record.State) > maxRecordSize {
		return nil, ErrRecordTooLarge
	}
	data, err := json.Marshal(record)
	if err != nil {
		return nil, errors.New("cannot encode endpoint record")
	}
	if len(data) > maxRecordSize {
		return nil, ErrRecordTooLarge
	}
	return data, nil
}

func writeRecord(ctx context.Context, path string, data []byte) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), "."+filepath.Base(path)+".*")
	if err != nil {
		return fmt.Errorf("create endpoint temporary file: %w", err)
	}
	name := file.Name()
	defer os.Remove(name)
	closed := false
	defer func() {
		if !closed {
			file.Close()
		}
	}()
	if err := file.Chmod(0600); err != nil {
		return fmt.Errorf("set endpoint temporary permissions: %w", err)
	}
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write endpoint record: %w", err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("flush endpoint record: %w", err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close endpoint record: %w", err)
	}
	closed = true
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Rename(name, path); err != nil {
		return fmt.Errorf("replace endpoint record: %w", err)
	}
	if err := fileio.SyncDirectory(filepath.Dir(path)); err != nil {
		return fmt.Errorf("endpoint record saved but directory durability is uncertain: %w", err)
	}
	return nil
}
