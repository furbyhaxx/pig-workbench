package host

import (
	"bufio"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/location"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// artifactProvider resolves the current component's .project/ tree.
type artifactProvider struct{ location *location.Manager }

func (artifactProvider) Scheme() string { return "artifact" }

func (p artifactProvider) root(uri string) (string, error) {
	snapshot := p.location.Current()
	anchor := snapshot.ProjectRoot
	if anchor == "" {
		anchor = snapshot.Cwd
	}
	tree, _, err := findArtifactTree(anchor)
	if err != nil {
		return "", err
	}
	return artifactJoin(uri, tree)
}

func findArtifactTree(start string) (string, string, error) {
	if start == "" {
		return "", "", errors.New("artifact:// needs a working location")
	}
	dir := filepath.Clean(start)
	for {
		candidate := filepath.Join(dir, ".project")
		if info, err := os.Stat(candidate); err == nil && info.IsDir() {
			return candidate, dir, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", "", resource.ErrNotFound
		}
		dir = parent
	}
}

func artifactJoin(uri, tree string) (string, error) {
	rest, ok := strings.CutPrefix(uri, "artifact:")
	if !ok {
		return "", errors.New("artifact provider requires an artifact:// URI")
	}
	rest = strings.Trim(rest, "/")
	if rest == "" || rest == "//" {
		return tree, nil
	}
	clean := filepath.Clean(rest)
	if clean == "." || strings.HasPrefix(clean, "..") || filepath.IsAbs(clean) {
		return "", errors.New("artifact:// path must stay inside the project artifact tree")
	}
	return filepath.Join(tree, clean), nil
}

func artifactURI(uri, name string) string {
	base := strings.Trim(strings.TrimPrefix(uri, "artifact:"), "/")
	if base == "" {
		return "artifact://" + name
	}
	return "artifact://" + base + "/" + name
}

func (p artifactProvider) Stat(ctx context.Context, uri string) (resource.StatResult, error) {
	path, err := p.root(uri)
	if err != nil {
		return resource.StatResult{}, err
	}
	info, err := os.Stat(path)
	if errors.Is(err, os.ErrNotExist) {
		return resource.StatResult{}, resource.ErrNotFound
	}
	if err != nil {
		return resource.StatResult{}, err
	}
	kind := resource.EntryFile
	if info.IsDir() {
		kind = resource.EntryDir
	}
	return resource.StatResult{URI: uri, Kind: kind, Size: info.Size()}, nil
}

func (p artifactProvider) List(ctx context.Context, uri, cursor string, limit int) (resource.Listing, error) {
	path, err := p.root(uri)
	if err != nil {
		return resource.Listing{}, err
	}
	listing, err := (resource.FileProvider{}).List(ctx, resource.FileURI(path), cursor, limit)
	if err != nil {
		return resource.Listing{}, err
	}
	for index, entry := range listing.Entries {
		listing.Entries[index].URI = artifactURI(uri, entry.Name)
		listing.Entries[index].Detail = artifactColumns(filepath.Join(path, entry.Name), entry.Kind)
	}
	return listing, nil
}

func (p artifactProvider) Read(ctx context.Context, request resource.ReadRequest) (resource.ReadResult, error) {
	path, err := p.root(request.URI)
	if err != nil {
		return resource.ReadResult{}, err
	}
	page, err := (resource.FileProvider{}).Read(ctx, resource.ReadRequest{URI: resource.FileURI(path), Requested: request.Requested, Selector: request.Selector, Budget: request.Budget})
	if err != nil {
		return resource.ReadResult{}, err
	}
	page.Facts.CanonicalURI = request.URI
	return page, nil
}

// artifactDocuments are the files that carry a directory artifact's frontmatter, in lookup order.
var artifactDocuments = []string{"README.md", "plan.md", "spec.md", "report.md"}

// artifactColumns returns "id · title · status · rev N" from an artifact's document frontmatter, or "" without one.
func artifactColumns(path string, kind resource.EntryKind) string {
	candidates := []string{path}
	if kind == resource.EntryDir {
		candidates = candidates[:0]
		for _, name := range artifactDocuments {
			candidates = append(candidates, filepath.Join(path, name))
		}
	} else if !strings.HasSuffix(path, ".md") {
		return ""
	}
	for _, candidate := range candidates {
		if columns := frontmatterColumns(candidate); columns != "" {
			return columns
		}
	}
	return ""
}

func frontmatterColumns(path string) string {
	info, err := os.Lstat(path)
	if err != nil || !info.Mode().IsRegular() {
		return ""
	}
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	scanner := bufio.NewScanner(io.LimitReader(file, 8<<10))
	if !scanner.Scan() || strings.TrimRight(scanner.Text(), "\r") != "---" {
		return ""
	}
	fields := map[string]string{}
	inDocument := false
	for scanner.Scan() {
		line := strings.TrimRight(scanner.Text(), "\r")
		if line == "---" {
			break
		}
		if !strings.HasPrefix(line, " ") {
			inDocument = strings.TrimSpace(line) == "document:"
			continue
		}
		key, value, ok := strings.Cut(strings.TrimPrefix(line, "  "), ":")
		if !inDocument || !ok || strings.HasPrefix(key, " ") {
			continue
		}
		switch key {
		case "id", "title", "status", "revision":
			fields[key] = strings.Trim(strings.TrimSpace(value), `"'`)
		}
	}
	if fields["id"] == "" && fields["title"] == "" {
		return ""
	}
	parts := []string{fields["id"], fields["title"], fields["status"]}
	if fields["revision"] != "" {
		parts = append(parts, "rev "+fields["revision"])
	}
	return strings.Join(slices.DeleteFunc(parts, func(part string) bool { return part == "" }), " · ")
}
