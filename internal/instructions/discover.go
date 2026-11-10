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
	"sort"
	"strings"
)

const defaultMaxTotalBytes = 4 << 20

type scope struct {
	root  string
	input string
	trust string
	fs    *os.Root
}

type candidate struct {
	path      string
	display   string
	canonical string
}

type source struct {
	file     Selected
	class    Class
	data     []byte
	complete bool
}

type discovery struct {
	classes   []Class
	remaining int
	sources   []source
	seen      map[string]int
	reasons   []Reason
	worktrees []string
}

// Discover selects local ambient files for root within the project and user roots.
// A nil classes slice selects the built-in defaults; an empty slice selects none.
func Discover(ctx context.Context, root string, classes []Class, opts Options) (Set, error) {
	if err := ctx.Err(); err != nil {
		return Set{}, err
	}
	if root == "" || opts.MaxTotalBytes < 0 {
		return Set{}, errors.New("invalid target or total byte limit")
	}
	policies, err := validatedClasses(classes)
	if err != nil {
		return Set{}, err
	}
	if classes == nil && opts.PrivateRoot != "" {
		policies = append(policies, ProjectPrivateClass())
	}
	target, err := filepath.Abs(root)
	if err != nil {
		return Set{}, fmt.Errorf("resolve target: %w", err)
	}
	info, err := os.Stat(target)
	if err != nil {
		return Set{}, fmt.Errorf("stat target: %w", err)
	}
	targetDir := target
	if !info.IsDir() {
		targetDir = filepath.Dir(target)
	}
	boundary := opts.ProjectRoot
	if boundary == "" {
		boundary = targetDir
	}
	project, err := openScope(boundary, "project")
	if err != nil {
		return Set{}, fmt.Errorf("open project boundary: %w", err)
	}
	defer project.fs.Close()
	dirs, err := projectDirectories(project, targetDir)
	if err != nil {
		return Set{}, err
	}
	if !info.IsDir() {
		canonical, err := filepath.EvalSymlinks(target)
		if err != nil || !contains(project.root, canonical) {
			return Set{}, errors.New("file target resolves outside project boundary")
		}
		if physicalDir := filepath.Dir(canonical); physicalDir != dirs[len(dirs)-1] {
			rel, err := filepath.Rel(project.root, physicalDir)
			if err != nil {
				return Set{}, err
			}
			dirs, err = projectDirectories(project, filepath.Join(project.input, rel))
			if err != nil {
				return Set{}, err
			}
		}
	}
	limit := opts.MaxTotalBytes
	if limit == 0 {
		limit = defaultMaxTotalBytes
	}
	d := discovery{classes: policies, remaining: limit, seen: make(map[string]int)}
	if err := d.checkNested(ctx, project, dirs, opts.AllowNested); err != nil {
		return Set{}, err
	}
	for _, userRoot := range opts.UserRoots {
		if err := ctx.Err(); err != nil {
			return Set{}, err
		}
		user, err := openScope(userRoot, "user")
		if err != nil {
			for _, class := range policies {
				d.reasons = append(d.reasons, Reason{Class: class.ID, Path: userRoot, DisplayPath: ".", Status: "missing", Detail: err.Error()})
			}
			continue
		}
		err = d.scanScope(ctx, func(int) scope { return user }, []string{user.root})
		user.fs.Close()
		if err != nil {
			return Set{}, err
		}
	}
	if opts.PrivateRoot != "" {
		for _, class := range policies {
			if class.ID == "project-private" {
				if err := d.scanPrivate(ctx, opts.PrivateRoot, class); err != nil {
					return Set{}, err
				}
			}
		}
	}
	owner := func(int) scope { return project }
	if opts.OuterRoot != "" && contains(opts.OuterRoot, filepath.Dir(project.root)) && project.root != filepath.Dir(project.root) {
		outer, err := openScope(opts.OuterRoot, "user")
		if err != nil {
			return Set{}, fmt.Errorf("open outer boundary: %w", err)
		}
		defer outer.fs.Close()
		var above []string
		for dir := filepath.Dir(project.root); contains(outer.root, dir); dir = filepath.Dir(dir) {
			above = append([]string{dir}, above...)
			if dir == filepath.Dir(dir) {
				break
			}
		}
		dirs = append(above, dirs...)
		owner = func(i int) scope {
			if i < len(above) {
				return outer
			}
			return project
		}
	}
	if err := d.scanScope(ctx, owner, dirs); err != nil {
		return Set{}, err
	}
	set := Set{Reasons: d.reasons}
	for _, src := range d.sources {
		if err := ctx.Err(); err != nil {
			return Set{}, err
		}
		view, err := makeView(ctx, src.data, src.complete, src.class, src.file.DisplayPath, opts.ValidatePointers)
		if err != nil {
			return Set{}, err
		}
		if err := ctx.Err(); err != nil {
			return Set{}, err
		}
		for _, diagnostic := range view.diagnostics {
			detail := fmt.Sprintf("line %d", diagnostic.line)
			if diagnostic.status == "pointer-validation-deferred" {
				detail += ": target existence and status were not checked"
			}
			set.Reasons = append(set.Reasons, Reason{Class: src.class.ID, Path: src.file.Path, DisplayPath: src.file.DisplayPath, Status: diagnostic.status, Detail: detail})
		}
		if !view.fits {
			set.Reasons = append(set.Reasons, Reason{Class: src.file.Class, Path: src.file.Path, DisplayPath: src.file.DisplayPath, Status: "capped", Detail: "view budget cannot contain its continuation"})
			continue
		}
		src.file.Content = view.content
		src.file.Truncated = view.truncated
		src.file.OmittedItems = view.omitted
		if view.truncated {
			src.file.Continuation = src.file.DisplayPath
			set.Reasons = append(set.Reasons, Reason{Class: src.file.Class, Path: src.file.Path, DisplayPath: src.file.DisplayPath, Status: "capped", Detail: "view or source scan limit reached"})
		}
		set.Files = append(set.Files, src.file)
	}
	if err := ctx.Err(); err != nil {
		return Set{}, err
	}
	return set, nil
}

func openScope(path, trust string) (scope, error) {
	if path == "" {
		return scope{}, errors.New("empty authorization root")
	}
	input, err := filepath.Abs(path)
	if err != nil {
		return scope{}, err
	}
	canonical, err := filepath.EvalSymlinks(input)
	if err != nil {
		return scope{}, err
	}
	f, err := os.OpenRoot(canonical)
	if err != nil {
		return scope{}, err
	}
	return scope{root: canonical, input: input, trust: trust, fs: f}, nil
}

func contains(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

func projectDirectories(project scope, target string) ([]string, error) {
	if !contains(project.input, target) {
		return nil, errors.New("target is outside project boundary")
	}
	rel, err := filepath.Rel(project.input, target)
	if err != nil {
		return nil, err
	}
	current := project.input
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			info, err := os.Lstat(current)
			if err != nil {
				return nil, fmt.Errorf("inspect target directory: %w", err)
			}
			if info.Mode()&os.ModeSymlink != 0 {
				return nil, errors.New("target traverses a symlinked directory")
			}
		}
	}
	canonical, err := filepath.EvalSymlinks(target)
	if err != nil || !contains(project.root, canonical) {
		return nil, errors.New("target resolves outside project boundary")
	}
	rel, err = filepath.Rel(project.root, canonical)
	if err != nil {
		return nil, err
	}
	dirs := []string{project.root}
	current = project.root
	if rel != "." {
		for _, part := range strings.Split(rel, string(filepath.Separator)) {
			current = filepath.Join(current, part)
			dirs = append(dirs, current)
		}
	}
	return dirs, nil
}

func (d *discovery) checkNested(ctx context.Context, project scope, dirs []string, allow bool) error {
	for i := len(dirs) - 1; i > 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}
		path := filepath.Join(dirs[i], ".git")
		rel, err := filepath.Rel(project.root, path)
		if err != nil {
			return err
		}
		info, err := project.fs.Lstat(rel)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect nested repository boundary: %w", err)
		}
		if !info.IsDir() && !info.Mode().IsRegular() && info.Mode()&os.ModeSymlink == 0 {
			continue
		}
		if linkedWorktree(project, dirs[i]) {
			d.worktrees = append(d.worktrees, dirs[i])
			d.reasons = append(d.reasons, Reason{Path: path, DisplayPath: filepath.ToSlash(rel), Status: "linked-worktree", Detail: "a linked worktree of this checkout is part of the project"})
			continue
		}
		if !allow {
			return &NestedRepositoryError{Dir: dirs[i], ProjectRoot: project.root}
		}
		d.reasons = append(d.reasons, Reason{Path: path, DisplayPath: filepath.ToSlash(rel), Status: "crossed-nested-repository", Detail: "caller explicitly allowed crossing this boundary"})
	}
	return nil
}

// linkedWorktree reports whether dir is a worktree that `git worktree add` created from a checkout
// inside the project. The claim in dir/.git must be confirmed by the main checkout's back-link, so a
// foreign repository cannot pose as a worktree.
func linkedWorktree(project scope, dir string) bool {
	line, ok := firstLine(project, filepath.Join(dir, ".git"))
	admin, found := strings.CutPrefix(line, "gitdir:")
	if !ok || !found {
		return false
	}
	admin = strings.TrimSpace(admin)
	if !filepath.IsAbs(admin) {
		admin = filepath.Join(dir, admin)
	}
	admin = filepath.Clean(admin)
	gitDir := filepath.Dir(filepath.Dir(admin))
	if filepath.Base(filepath.Dir(admin)) != "worktrees" || filepath.Base(gitDir) != ".git" {
		return false
	}
	main := filepath.Dir(gitDir)
	if main == dir || !contains(project.root, main) || !contains(main, dir) {
		return false
	}
	back, ok := firstLine(project, filepath.Join(admin, "gitdir"))
	if !ok {
		return false
	}
	resolved, err := filepath.EvalSymlinks(back)
	return err == nil && resolved == filepath.Join(dir, ".git")
}

// firstLine reads the first line of a small regular file inside the project root.
func firstLine(project scope, path string) (string, bool) {
	rel, err := filepath.Rel(project.root, path)
	if err != nil || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", false
	}
	info, err := project.fs.Lstat(rel)
	if err != nil || !info.Mode().IsRegular() || info.Size() > 4096 {
		return "", false
	}
	file, err := project.fs.OpenFile(rel, os.O_RDONLY|nonblockFlag(), 0)
	if err != nil {
		return "", false
	}
	defer file.Close()
	data, err := io.ReadAll(io.LimitReader(file, 4096))
	if err != nil {
		return "", false
	}
	line, _, _ := strings.Cut(string(data), "\n")
	return strings.TrimSpace(line), true
}

func (d *discovery) inWorktree(path string) bool {
	for _, dir := range d.worktrees {
		if contains(dir, path) {
			return true
		}
	}
	return false
}

// scanScope selects over one ancestor chain; owner names the authorization scope of each directory.
func (d *discovery) scanScope(ctx context.Context, owner func(int) scope, dirs []string) error {
	candidates := make([][][]candidate, len(dirs))
	for i, dir := range dirs {
		scope := owner(i)
		candidates[i] = make([][]candidate, len(d.classes))
		for j, class := range d.classes {
			if class.ID == "project-private" {
				continue
			}
			chosen := false
			for _, name := range class.Filenames {
				if err := ctx.Err(); err != nil {
					return err
				}
				path := filepath.Join(dir, name)
				display, err := filepath.Rel(scope.root, path)
				if err != nil {
					return err
				}
				display = filepath.ToSlash(display)
				reason := Reason{Class: class.ID, Path: path, DisplayPath: display}
				rel, err := filepath.Rel(scope.root, path)
				if err != nil {
					return err
				}
				if _, err = scope.fs.Lstat(rel); err != nil {
					reason.Status, reason.Detail = "missing", err.Error()
					d.reasons = append(d.reasons, reason)
					continue
				}
				if class.Select == "first" && chosen {
					reason.Status, reason.Detail = "shadowed", "an earlier filename exists in this directory"
					d.reasons = append(d.reasons, reason)
					continue
				}
				chosen = true
				canonical, err := filepath.EvalSymlinks(path)
				if err != nil {
					reason.Status, reason.Detail = "missing", err.Error()
					d.reasons = append(d.reasons, reason)
					continue
				}
				if !contains(scope.root, canonical) {
					reason.Status, reason.Detail = "ignored", "escape from authorization root"
					d.reasons = append(d.reasons, reason)
					continue
				}
				rel, err = filepath.Rel(scope.root, canonical)
				if err != nil {
					return err
				}
				info, err := scope.fs.Stat(rel)
				if err != nil || !info.Mode().IsRegular() {
					reason.Status, reason.Detail = "missing", "candidate is unavailable or not a regular file"
					d.reasons = append(d.reasons, reason)
					continue
				}
				candidates[i][j] = append(candidates[i][j], candidate{path: path, display: display, canonical: canonical})
			}
		}
	}
	first, last := make([]int, len(d.classes)), make([]int, len(d.classes))
	for j := range d.classes {
		first[j], last[j] = -1, -1
		for i := range dirs {
			if len(candidates[i][j]) > 0 {
				if first[j] < 0 {
					first[j] = i
				}
				last[j] = i
			}
		}
	}
	for i := range dirs {
		for j, class := range d.classes {
			active := class.Placement == "ancestors" || class.Placement == "root" && i == 0 ||
				class.Placement == "nearest" && i == last[j] || class.Placement == "root+nearest" && (i == first[j] || i == last[j])
			for _, candidate := range candidates[i][j] {
				if err := ctx.Err(); err != nil {
					return err
				}
				if !active {
					d.reasons = append(d.reasons, Reason{Class: class.ID, Path: candidate.path, DisplayPath: candidate.display, Status: "ignored", Detail: "excluded by placement"})
					continue
				}
				if err := d.selectCandidate(ctx, owner(i), class, candidate); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func (d *discovery) scanPrivate(ctx context.Context, path string, class Class) error {
	before, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("inspect private notes directory: %w", err)
	}
	if !before.IsDir() {
		return errors.New("private notes root is not a directory")
	}
	private, err := openScope(path, "user")
	if err != nil {
		return fmt.Errorf("open private notes directory: %w", err)
	}
	defer private.fs.Close()
	opened, err := private.fs.Stat(".")
	if err != nil || !os.SameFile(before, opened) {
		return errors.New("private notes directory changed while opening")
	}
	dir, err := private.fs.Open(".")
	if err != nil {
		return err
	}
	defer dir.Close()
	entries, err := dir.ReadDir(4097)
	if err != nil && !errors.Is(err, io.EOF) {
		return err
	}
	if len(entries) > 4096 {
		return errors.New("private notes directory exceeds the 4096-entry scan limit")
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if !strings.HasSuffix(entry.Name(), ".md") {
			continue
		}
		name := filepath.Join(private.root, entry.Name())
		info, err := private.fs.Lstat(entry.Name())
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("inspect private note %s: %w", name, err)
		}
		if !info.Mode().IsRegular() {
			d.reasons = append(d.reasons, Reason{Class: class.ID, Path: name, DisplayPath: name, Status: "ignored", Detail: "private notes must be regular files"})
			continue
		}
		if d.remaining == 0 {
			return errors.New("private instruction source budget exhausted")
		}
		if err := d.selectCandidate(ctx, private, class, candidate{path: name, display: name, canonical: name}); err != nil {
			return err
		}
		if reason := d.reasons[len(d.reasons)-1]; reason.Status == "missing" {
			return fmt.Errorf("private note %s unavailable: %s", name, reason.Detail)
		}
	}
	return nil
}

func (d *discovery) selectCandidate(ctx context.Context, scope scope, class Class, candidate candidate) error {
	reason := Reason{Class: class.ID, Path: candidate.path, DisplayPath: candidate.display, Status: "selected"}
	if index, ok := d.seen[candidate.canonical]; ok {
		src := &d.sources[index]
		src.class = intersectClass(src.class, class)
		reason.Detail = "same canonical file as an earlier selection"
		if src.file.Role != class.Role || src.file.Trust != scope.trust {
			reason.Detail += "; conflicting role or trust uses the less privileged interpretation"
		}
		if class.Role == "context" {
			src.file.Role = "context"
		}
		if scope.trust == "project" {
			src.file.Trust = "project"
		}
		d.reasons = append(d.reasons, reason)
		return nil
	}
	data, complete, err := readSource(ctx, scope, candidate.canonical, d.remaining)
	d.remaining -= len(data)
	if err != nil {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		reason.Status, reason.Detail = "missing", err.Error()
		d.reasons = append(d.reasons, reason)
		return nil
	}
	sum := sha256.Sum256(data)
	file := Selected{Class: class.ID, Path: candidate.canonical, DisplayPath: candidate.display, Role: class.Role, Trust: scope.trust, Digest: hex.EncodeToString(sum[:])}
	if d.inWorktree(candidate.canonical) {
		for _, earlier := range d.sources {
			if earlier.file.Digest == file.Digest && earlier.file.Class == file.Class && earlier.file.Role == file.Role && earlier.file.Trust == file.Trust {
				d.remaining += len(data)
				reason.Status, reason.Detail = "duplicate", "identical bytes to "+earlier.file.Path+" delivered for the checkout"
				d.reasons = append(d.reasons, reason)
				return nil
			}
		}
	}
	d.seen[candidate.canonical] = len(d.sources)
	d.sources = append(d.sources, source{file: file, class: class, data: data, complete: complete})
	d.reasons = append(d.reasons, reason)
	return nil
}

func intersectClass(a, b Class) Class {
	if b.MaxLines < a.MaxLines {
		a.MaxLines = b.MaxLines
	}
	if b.MaxBytes < a.MaxBytes {
		a.MaxBytes = b.MaxBytes
	}
	if b.MaxItems > 0 && (a.MaxItems == 0 || b.MaxItems < a.MaxItems) {
		a.MaxItems = b.MaxItems
	}
	if b.ID == "backlog" {
		a.ID = "backlog"
	}
	return a
}

func readSource(ctx context.Context, scope scope, canonical string, limit int) ([]byte, bool, error) {
	rel, err := filepath.Rel(scope.root, canonical)
	if err != nil {
		return nil, false, err
	}
	// Rooted nonblocking opens prevent both symlink escapes and FIFO substitution.
	file, err := scope.fs.OpenFile(rel, os.O_RDONLY|nonblockFlag(), 0)
	if err != nil {
		return nil, false, err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return nil, false, err
	}
	if !info.Mode().IsRegular() {
		return nil, false, errors.New("candidate is not a regular file")
	}
	data := make([]byte, 0, min(limit, 32<<10))
	buf := make([]byte, 32<<10)
	complete := false
	for len(data) < limit {
		if err := ctx.Err(); err != nil {
			return data, false, err
		}
		n, readErr := file.Read(buf[:min(len(buf), limit-len(data))])
		data = append(data, buf[:n]...)
		if readErr == io.EOF {
			complete = true
			break
		}
		if readErr != nil {
			return data, false, readErr
		}
		if n == 0 {
			return data, false, io.ErrNoProgress
		}
	}
	if !complete {
		after, err := file.Stat()
		if err != nil {
			return data, false, err
		}
		complete = len(data) > 0 && info.Size() == int64(len(data)) && after.Size() == int64(len(data))
	}
	return data, complete, nil
}
