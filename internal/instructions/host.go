package instructions

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// Host file names under a host's system directory (SPEC-0007 §3.1).
const (
	HostSteeringName = "AGENTS.md"
	HostFactsName    = "HOST.md"
)

const hostReadLimit = 1 << 20

// HostData is one host file as read from a host.
type HostData struct {
	// Path is the file's absolute path on its host.
	Path     string
	Data     []byte
	Complete bool
}

// RefusedError reports a host file that exists but must not be delivered.
type RefusedError struct {
	Path   string
	Reason string
}

// Error describes why the file was refused.
func (e *RefusedError) Error() string { return fmt.Sprintf("%s refused: %s", e.Path, e.Reason) }

// HostReader reads the system files of one host. Read returns os.ErrNotExist for an absent file
// and a *RefusedError for a file that fails the safety checks.
type HostReader interface {
	Authority() string
	Read(ctx context.Context, name string) (HostData, error)
}

// LocalHost reads host files from dir on this machine, refusing any file or directory that
// another user could have written.
func LocalHost(dir string) HostReader { return localHost{dir: dir} }

type localHost struct{ dir string }

func (localHost) Authority() string { return "local" }

func (h localHost) Read(ctx context.Context, name string) (HostData, error) {
	if !filepath.IsAbs(h.dir) {
		return HostData{}, errors.New("host directory must be absolute")
	}
	path := filepath.Join(h.dir, name)
	if err := ctx.Err(); err != nil {
		return HostData{}, err
	}
	dirInfo, err := os.Stat(h.dir)
	if err != nil {
		return HostData{}, err
	}
	if reason := untrustedWriter(dirInfo); reason != "" {
		if _, statErr := os.Stat(path); errors.Is(statErr, os.ErrNotExist) {
			return HostData{}, os.ErrNotExist
		}
		return HostData{}, &RefusedError{Path: path, Reason: "directory " + reason}
	}
	file, info, err := fileio.OpenRegular(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return HostData{}, os.ErrNotExist
		}
		return HostData{}, &RefusedError{Path: path, Reason: err.Error()}
	}
	defer file.Close()
	if reason := untrustedWriter(info); reason != "" {
		return HostData{}, &RefusedError{Path: path, Reason: "file " + reason}
	}
	data, err := io.ReadAll(io.LimitReader(file, hostReadLimit+1))
	if err != nil {
		return HostData{}, &RefusedError{Path: path, Reason: err.Error()}
	}
	complete := len(data) <= hostReadLimit
	if !complete {
		data = data[:hostReadLimit]
	}
	return HostData{Path: path, Data: data, Complete: complete}, nil
}

// HostClasses returns the view policies of the two host files.
func HostClasses() (steering, facts Class) {
	steering = DefaultClasses()[0]
	facts = Class{ID: "host", Filenames: []string{HostFactsName}, Placement: "root", Select: "first", Role: "context", MaxLines: 400, MaxBytes: 32768}
	return steering, facts
}

// ResolveHost selects the host files from readers ordered by priority: for each file name the
// first reader that has it wins, so a remote host's copy replaces the same file from another host
// and a missing file falls back to the next host. A refused file stays refused: the next reader's
// copy is not substituted for it. Files of a non-local authority are displayed as "authority:path".
func ResolveHost(ctx context.Context, readers ...HostReader) (Set, error) {
	steering, facts := HostClasses()
	var set Set
	for _, entry := range []struct {
		name  string
		class Class
	}{{HostSteeringName, steering}, {HostFactsName, facts}} {
	readers:
		for _, reader := range readers {
			data, err := reader.Read(ctx, entry.name)
			var refused *RefusedError
			switch {
			case errors.Is(err, os.ErrNotExist):
				continue
			case errors.As(err, &refused):
				set.Reasons = append(set.Reasons, Reason{Class: entry.class.ID, Path: refused.Path, DisplayPath: hostDisplay(reader, refused.Path), Status: "refused", Detail: refused.Reason})
				break readers
			case err != nil:
				return Set{}, err
			}
			display := hostDisplay(reader, data.Path)
			view, err := makeView(ctx, data.Data, data.Complete, entry.class, display, false)
			if err != nil {
				return Set{}, err
			}
			if !view.fits {
				set.Reasons = append(set.Reasons, Reason{Class: entry.class.ID, Path: data.Path, DisplayPath: display, Status: "capped", Detail: "view budget cannot contain its continuation"})
				break readers
			}
			sum := sha256.Sum256(data.Data)
			file := Selected{Class: entry.class.ID, Path: display, DisplayPath: display, Role: entry.class.Role, Trust: "system", Content: view.content, Digest: hex.EncodeToString(sum[:]), Truncated: view.truncated}
			if view.truncated {
				file.Continuation = display
				set.Reasons = append(set.Reasons, Reason{Class: entry.class.ID, Path: display, DisplayPath: display, Status: "capped", Detail: "view or source scan limit reached"})
			}
			set.Files = append(set.Files, file)
			set.Reasons = append(set.Reasons, Reason{Class: entry.class.ID, Path: display, DisplayPath: display, Status: "selected", Detail: "host file of " + reader.Authority()})
			break
		}
	}
	return set, nil
}

func hostDisplay(reader HostReader, path string) string {
	if reader.Authority() == "local" {
		return path
	}
	return reader.Authority() + ":" + path
}
