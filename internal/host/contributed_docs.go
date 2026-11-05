package host

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/contribution"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

type docsSource struct {
	Repo  string   `json:"repo,omitempty"`
	Ref   string   `json:"ref,omitempty"`
	Paths []string `json:"paths,omitempty"`
	Dir   string   `json:"dir,omitempty"`
}

type docsDescriptor struct {
	Title   string      `json:"title"`
	Version string      `json:"version,omitempty"`
	Root    string      `json:"root"`
	Source  *docsSource `json:"source,omitempty"`
}

func parseDocsDescriptor(data []byte) (docsDescriptor, error) {
	var d docsDescriptor
	if err := decodeStrict(data, &d); err != nil {
		return d, err
	}
	invalid := func(message string) (docsDescriptor, error) {
		return docsDescriptor{}, fmt.Errorf("%w: docs %s", errInvalidContribution, message)
	}
	if d.Title == "" || !plainField(d.Title, 128) || !plainField(d.Version, 64) {
		return invalid("title and version must be plain text")
	}
	if !filepath.IsAbs(d.Root) {
		return invalid("root must be an absolute directory")
	}
	if info, err := os.Stat(d.Root); err != nil || !info.IsDir() {
		return invalid("root must be an existing directory")
	}
	if d.Source != nil {
		switch {
		case d.Source.Dir != "" && d.Source.Repo == "":
			if !filepath.IsAbs(d.Source.Dir) {
				return invalid("source.dir must be absolute")
			}
		case d.Source.Repo != "" && d.Source.Dir == "":
			parsed, err := url.Parse(d.Source.Repo)
			if err != nil || parsed.Scheme != "https" || parsed.Host == "" || parsed.User != nil {
				return invalid("source.repo must be an https URL without credentials")
			}
			if d.Source.Ref == "" || strings.HasPrefix(d.Source.Ref, "-") || !plainField(d.Source.Ref, 128) {
				return invalid("source.ref is required and must be a plain ref")
			}
			for _, path := range d.Source.Paths {
				if path == "" || strings.HasPrefix(path, "-") || strings.Contains(path, "..") || !plainField(path, 256) {
					return invalid("source.paths must be relative repository paths")
				}
			}
		default:
			return invalid("source needs exactly one of repo or dir")
		}
	}
	return d, nil
}

type docsSet struct {
	owner      string
	key        string
	descriptor docsDescriptor
	once       *sync.Once
	sourceDir  string
	sourceErr  error
}

// docsCatalog holds registered documentation sets for docs://.
type docsCatalog struct {
	mu    sync.RWMutex
	sets  map[string]*docsSet
	cache string
	guide func(scheme string) (string, bool)
}

func (c *docsCatalog) add(registrationID string, set *docsSet) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	for _, existing := range c.sets {
		if existing.owner == set.owner {
			return fmt.Errorf("%w: owner %q already registered docs", errConsumerConflict, set.owner)
		}
	}
	c.sets[registrationID] = set
	return nil
}

func (c *docsCatalog) remove(registrationID string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.sets, registrationID)
}

func (c *docsCatalog) byOwner(owner string) (*docsSet, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	for _, set := range c.sets {
		if set.owner == owner {
			return set, true
		}
	}
	return nil, false
}

func (c *docsCatalog) owners() []string {
	c.mu.RLock()
	defer c.mu.RUnlock()
	owners := make([]string, 0, len(c.sets))
	for _, set := range c.sets {
		owners = append(owners, set.owner)
	}
	sort.Strings(owners)
	return owners
}

func installDocs(s *session, registration contribution.Registration) (func(), error) {
	descriptor, err := parseDocsDescriptor(registration.Descriptor)
	if err != nil {
		return nil, err
	}
	if registration.Owner == "workbench" || registration.Owner == "harness" {
		return nil, fmt.Errorf("%w: docs owner %q is reserved", errConsumerConflict, registration.Owner)
	}
	set := &docsSet{owner: registration.Owner, key: registration.Key, descriptor: descriptor, once: new(sync.Once)}
	if err := s.docs.add(registration.RegistrationID, set); err != nil {
		return nil, err
	}
	return func() { s.docs.remove(registration.RegistrationID) }, nil
}

// sourceRoot materializes a registered source tree once, on first access.
func (c *docsCatalog) sourceRoot(ctx context.Context, set *docsSet) (string, error) {
	source := set.descriptor.Source
	if source == nil {
		return "", resource.ErrNotFound
	}
	if source.Dir != "" {
		return source.Dir, nil
	}
	set.once.Do(func() {
		if c.cache == "" {
			set.sourceErr = errors.New("no cache directory for source checkouts")
			return
		}
		dir := filepath.Join(c.cache, sanitizeSegment(set.owner), sanitizeSegment(source.Ref))
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			set.sourceDir = dir
			return
		}
		if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
			set.sourceErr = err
			return
		}
		temporary, err := os.MkdirTemp(filepath.Dir(dir), ".checkout-")
		if err != nil {
			set.sourceErr = err
			return
		}
		args := []string{"clone", "--quiet", "--depth", "1", "--filter=blob:none", "--branch", source.Ref}
		if len(source.Paths) > 0 {
			args = append(args, "--sparse")
		}
		args = append(args, "--", source.Repo, temporary)
		command := exec.CommandContext(ctx, "git", args...)
		command.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0")
		if output, err := command.CombinedOutput(); err != nil {
			_ = os.RemoveAll(temporary)
			set.sourceErr = fmt.Errorf("source checkout failed: %s", sanitizeProviderText(string(output)))
			return
		}
		if len(source.Paths) > 0 {
			sparse := exec.CommandContext(ctx, "git", append([]string{"-C", temporary, "sparse-checkout", "set", "--no-cone", "--"}, source.Paths...)...)
			if output, err := sparse.CombinedOutput(); err != nil {
				_ = os.RemoveAll(temporary)
				set.sourceErr = fmt.Errorf("sparse checkout failed: %s", sanitizeProviderText(string(output)))
				return
			}
		}
		if err := os.Rename(temporary, dir); err != nil {
			_ = os.RemoveAll(temporary)
			set.sourceErr = err
			return
		}
		set.sourceDir = dir
	})
	return set.sourceDir, set.sourceErr
}

// docsProvider serves docs:// read-only from registered roots and Workbench's scheme guides.
type docsProvider struct {
	catalog *docsCatalog
	schemes func() []string
}

func (docsProvider) Scheme() string { return "docs" }

type docsTarget struct {
	owner  string
	source bool
	rest   string
}

func splitDocs(uri string) (docsTarget, error) {
	rest, ok := strings.CutPrefix(uri, "docs://")
	if !ok {
		return docsTarget{}, errors.New("docs provider requires a docs:// URI")
	}
	decoded, err := url.PathUnescape(rest)
	if err != nil {
		return docsTarget{}, errors.New("invalid docs:// path encoding")
	}
	parts := strings.Split(strings.Trim(decoded, "/"), "/")
	for _, part := range parts {
		if part == "." || part == ".." {
			return docsTarget{}, errors.New("docs:// segments must not be '.' or '..'")
		}
	}
	target := docsTarget{owner: parts[0]}
	parts = parts[1:]
	if len(parts) > 0 && parts[0] == "source" {
		target.source = true
		parts = parts[1:]
	}
	target.rest = strings.Join(parts, "/")
	return target, nil
}

func (p docsProvider) resolve(ctx context.Context, target docsTarget) (string, error) {
	set, ok := p.catalog.byOwner(target.owner)
	if !ok {
		return "", resource.ErrNotFound
	}
	root := set.descriptor.Root
	if target.source {
		var err error
		if root, err = p.catalog.sourceRoot(ctx, set); err != nil {
			return "", err
		}
	}
	resolvedRoot, err := filepath.EvalSymlinks(root)
	if err != nil {
		return "", resource.ErrNotFound
	}
	full, err := filepath.EvalSymlinks(filepath.Join(resolvedRoot, filepath.FromSlash(target.rest)))
	if err != nil {
		return "", resource.ErrNotFound
	}
	if full != resolvedRoot && !strings.HasPrefix(full, resolvedRoot+string(filepath.Separator)) {
		return "", errors.New("docs:// path escapes its registered root")
	}
	return full, nil
}

func (p docsProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	target, err := splitDocs(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	if target.owner == "" || target.owner == "workbench" && target.rest == "" || target.owner == "workbench" && target.rest == "schemes" {
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	}
	if target.owner == "workbench" {
		if _, err := p.workbenchText(target.rest); err != nil {
			return resource.StatResult{}, err
		}
		return resource.StatResult{URI: uri, Kind: resource.EntryFile}, nil
	}
	path, err := p.resolve(ctx, target)
	if err != nil {
		return resource.StatResult{}, err
	}
	info, err := os.Stat(path)
	if err != nil {
		return resource.StatResult{}, resource.ErrNotFound
	}
	if info.IsDir() {
		return resource.StatResult{URI: uri, Kind: resource.EntryDir}, nil
	}
	return resource.StatResult{URI: uri, Kind: resource.EntryFile, Size: info.Size()}, nil
}

func (p docsProvider) workbenchText(rest string) (string, error) {
	if scheme, ok := strings.CutPrefix(rest, "schemes/"); ok && p.catalog.guide != nil {
		if text, found := p.catalog.guide(scheme); found {
			return text, nil
		}
	}
	return "", resource.ErrNotFound
}

func (p docsProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	target, err := splitDocs(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	var entries []resource.Entry
	switch {
	case target.owner == "":
		entries = append(entries, resource.Entry{Name: "workbench", URI: "docs://workbench/", Kind: resource.EntryDir})
		for _, owner := range p.catalog.owners() {
			entries = append(entries, resource.Entry{Name: owner, URI: "docs://" + owner + "/", Kind: resource.EntryDir})
		}
	case target.owner == "workbench" && target.rest == "":
		entries = append(entries, resource.Entry{Name: "schemes", URI: "docs://workbench/schemes/", Kind: resource.EntryDir})
	case target.owner == "workbench" && target.rest == "schemes":
		for _, scheme := range p.schemes() {
			if _, ok := p.catalog.guide(scheme); ok {
				entries = append(entries, resource.Entry{Name: scheme, URI: "docs://workbench/schemes/" + scheme, Kind: resource.EntryFile})
			}
		}
	default:
		path, err := p.resolve(ctx, target)
		if err != nil {
			return resource.Listing{}, err
		}
		dirEntries, err := os.ReadDir(path)
		if err != nil {
			return resource.Listing{}, resource.ErrUnsupported
		}
		base := "docs://" + target.owner + "/"
		if target.source {
			base += "source/"
		}
		if target.rest != "" {
			base += target.rest + "/"
		}
		if !target.source && target.rest == "" {
			if set, ok := p.catalog.byOwner(target.owner); ok && set.descriptor.Source != nil {
				entries = append(entries, resource.Entry{Name: "source", URI: base + "source/", Kind: resource.EntryDir})
			}
		}
		for _, entry := range dirEntries {
			if strings.HasPrefix(entry.Name(), ".git") {
				continue
			}
			kind := resource.EntryFile
			if entry.IsDir() {
				kind = resource.EntryDir
			} else if entry.Type()&fs.ModeSymlink != 0 {
				kind = resource.EntryLink
			}
			name := entry.Name()
			entries = append(entries, resource.Entry{Name: name, URI: base + url.PathEscape(name), Kind: kind})
		}
	}
	sort.SliceStable(entries, func(i, j int) bool {
		if (entries[i].Kind == resource.EntryDir) != (entries[j].Kind == resource.EntryDir) {
			return entries[i].Kind == resource.EntryDir
		}
		return entries[i].Name < entries[j].Name
	})
	start := 0
	if cursor != "" {
		start = len(entries)
		for i, entry := range entries {
			if entry.Name == cursor {
				start = i + 1
				break
			}
		}
	}
	listing := resource.Listing{Complete: true}
	limit = max(1, limit)
	for _, entry := range entries[start:] {
		if len(listing.Entries) >= limit {
			listing.Complete = false
			listing.NextCursor = listing.Entries[len(listing.Entries)-1].Name
			break
		}
		listing.Entries = append(listing.Entries, entry)
	}
	return listing, nil
}

const docsMaxFileBytes = 4 << 20

func (p docsProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	target, err := splitDocs(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	var data []byte
	if target.owner == "workbench" {
		text, err := p.workbenchText(target.rest)
		if err != nil {
			return resource.ReadResult{}, err
		}
		data = []byte(text)
	} else {
		path, err := p.resolve(ctx, target)
		if err != nil {
			return resource.ReadResult{}, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return resource.ReadResult{}, resource.ErrNotFound
		}
		if info.IsDir() {
			return resource.ReadResult{}, errors.New("docs:// target is a directory; use ls")
		}
		if info.Size() > docsMaxFileBytes {
			return resource.ReadResult{}, fmt.Errorf("docs file exceeds %d bytes", docsMaxFileBytes)
		}
		if data, err = os.ReadFile(path); err != nil {
			return resource.ReadResult{}, err
		}
	}
	page, err := resource.RenderText(ctx, request.URI, request.Requested, data, request.Selector, request.Budget)
	if err != nil {
		return resource.ReadResult{}, err
	}
	page.Handler = "docs"
	return resource.ReadResult{Content: page.Content, Facts: page}, nil
}
