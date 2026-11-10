package prompt

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

const (
	builtinNamespace    = "builtin/"
	defaultMaxFileBytes = 256 << 10
)

// FileSystemOptions configures a layered confined template loader.
type FileSystemOptions struct {
	// Builtin holds the always-available templates under the builtin/ namespace.
	Builtin fs.FS
	// UserRoot is the user template directory; an empty or missing directory disables the layer.
	UserRoot string
	// ProjectRoot is the project template directory, opened only while TrustProject is set.
	ProjectRoot string
	// TrustProject admits project templates ahead of user templates.
	TrustProject bool
	// MaxFileBytes bounds one template file; zero selects 256 KiB.
	MaxFileBytes int
}

// FileSystemLoader resolves templates from the builtin namespace, the trusted project
// directory and the user directory, rejecting names and symlinks that leave them.
type FileSystemLoader struct {
	builtin      fs.FS
	user         *os.Root
	project      *os.Root
	maxFileBytes int
}

// NewFileSystemLoader validates options and opens the permitted template roots.
func NewFileSystemLoader(options FileSystemOptions) (*FileSystemLoader, error) {
	if options.Builtin == nil {
		return nil, errors.New("builtin template filesystem is required")
	}
	if options.MaxFileBytes < 0 {
		return nil, errors.New("template byte limit must not be negative")
	}
	loader := &FileSystemLoader{builtin: options.Builtin, maxFileBytes: options.MaxFileBytes}
	if loader.maxFileBytes == 0 {
		loader.maxFileBytes = defaultMaxFileBytes
	}
	user, err := openTemplateRoot(options.UserRoot)
	if err != nil {
		return nil, fmt.Errorf("open user template root: %w", err)
	}
	loader.user = user
	if options.TrustProject {
		project, err := openTemplateRoot(options.ProjectRoot)
		if err != nil {
			loader.Close()
			return nil, fmt.Errorf("open project template root: %w", err)
		}
		loader.project = project
	}
	return loader, nil
}

func openTemplateRoot(directory string) (*os.Root, error) {
	if directory == "" {
		return nil, nil
	}
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return nil, err
	}
	canonical, err := filepath.EvalSymlinks(absolute)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return os.OpenRoot(canonical)
}

// Close releases the open template roots; the loader must not be used afterwards.
func (l *FileSystemLoader) Close() error {
	roots := []*os.Root{l.project, l.user}
	l.project, l.user = nil, nil
	var errs []error
	for _, root := range roots {
		if root != nil {
			errs = append(errs, root.Close())
		}
	}
	return errors.Join(errs...)
}

// Open returns one template, preferring the project layer while it is trusted.
func (l *FileSystemLoader) Open(name string) (io.ReadCloser, error) {
	if err := validTemplateName(name); err != nil {
		return nil, err
	}
	if strings.HasPrefix(name, builtinNamespace) {
		return l.openBuiltin(name)
	}
	for _, layer := range []struct {
		kind string
		root *os.Root
	}{{"project", l.project}, {"user", l.user}} {
		if layer.root == nil {
			continue
		}
		reader, err := l.openRooted(layer.root, name)
		if err == nil {
			return reader, nil
		}
		if !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("open %s template %q: %w", layer.kind, name, err)
		}
	}
	return nil, fmt.Errorf("open template %q: %w", name, fs.ErrNotExist)
}

func (l *FileSystemLoader) openBuiltin(name string) (io.ReadCloser, error) {
	info, err := fs.Stat(l.builtin, name)
	if err != nil {
		return nil, fmt.Errorf("open builtin template %q: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("open builtin template %q: not a regular file", name)
	}
	file, err := l.builtin.Open(name)
	if err != nil {
		return nil, fmt.Errorf("open builtin template %q: %w", name, err)
	}
	return boundedTemplate(file, l.maxFileBytes, name)
}

func (l *FileSystemLoader) openRooted(root *os.Root, name string) (io.ReadCloser, error) {
	file, err := root.OpenFile(name, os.O_RDONLY|fileio.NonblockFlag(), 0)
	if err != nil {
		return nil, err
	}
	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}
	if !info.Mode().IsRegular() {
		file.Close()
		return nil, fmt.Errorf("%q is not a regular file", name)
	}
	return boundedTemplate(file, l.maxFileBytes, name)
}

func boundedTemplate(file io.ReadCloser, limit int, name string) (io.ReadCloser, error) {
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, int64(limit)+1))
	if err != nil {
		return nil, fmt.Errorf("read template %q: %w", name, err)
	}
	if len(data) > limit {
		return nil, fmt.Errorf("template %q exceeds %d bytes", name, limit)
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

// List returns the reachable template names of every admitted layer, sorted.
func (l *FileSystemLoader) List() []string {
	names := make(map[string]struct{})
	templateNames(names, l.builtin, true)
	if l.project != nil {
		templateNames(names, l.project.FS(), false)
	}
	if l.user != nil {
		templateNames(names, l.user.FS(), false)
	}
	sorted := make([]string, 0, len(names))
	for name := range names {
		sorted = append(sorted, name)
	}
	sort.Strings(sorted)
	return sorted
}

func templateNames(names map[string]struct{}, filesystem fs.FS, builtin bool) {
	fs.WalkDir(filesystem, ".", func(name string, entry fs.DirEntry, err error) error {
		if err != nil {
			return nil
		}
		if entry.IsDir() {
			return nil
		}
		regular := entry.Type().IsRegular()
		if entry.Type()&fs.ModeSymlink != 0 {
			info, statErr := fs.Stat(filesystem, name)
			regular = statErr == nil && info.Mode().IsRegular()
		}
		if !regular || strings.HasPrefix(name, builtinNamespace) != builtin {
			return nil
		}
		names[name] = struct{}{}
		return nil
	})
}

func validTemplateName(name string) error {
	if !fs.ValidPath(name) || name == "." || strings.ContainsAny(name, "\\\x00") || filepath.VolumeName(name) != "" {
		return fmt.Errorf("invalid template name %q", name)
	}
	return nil
}
