// Package fileedit commits already-staged local edit effects.
//
// Commit validates every target before the first mutation, holds advisory locks
// for all canonical identities in sorted order and applies each effect with a
// same-directory atomic replacement. Symlinks are resolved to their target and
// are never replaced or removed. Multi-resource atomicity is not promised: a
// failure after the first mutation reports every committed, failed and
// unattempted effect and never rolls back.
//
// Locks coordinate Workbench callers only. An ordinary filesystem rename is not
// compare-and-swap against arbitrary external writers, so Commit re-resolves
// identity and revision immediately before each mutation.
package fileedit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/furbyhaxx/pig-workbench/internal/fileio"
)

// Errors classify preflight and commit failures for callers.
var (
	// ErrUnsupported reports a guarantee this platform or filesystem cannot
	// provide, such as a cross-filesystem move; nothing is downgraded silently.
	ErrUnsupported = errors.New("fileedit: unsupported")
	// ErrRevisionChanged reports that the source bytes moved away from the
	// revision the effect was staged against.
	ErrRevisionChanged = errors.New("fileedit: source revision changed")
	// ErrIdentityChanged reports that the source path or destination now
	// resolves to a different file than during preflight.
	ErrIdentityChanged = errors.New("fileedit: identity changed")
	// ErrDestinationExists reports that a move destination exists, including a
	// destination that appeared between preflight and commit.
	ErrDestinationExists = errors.New("fileedit: destination exists")
	// ErrAliasConflict reports several effects that resolve to one canonical
	// identity.
	ErrAliasConflict = errors.New("fileedit: effects alias one resource")
)

// Test seams for injected filesystem failures; production paths use the os
// primitives directly.
var (
	renamePath = os.Rename
	removePath = os.Remove
	linkPath   = os.Link
	syncDir    = fileio.SyncDirectory
)

// Effect is one already-staged local change.
//
// Exactly one operation per effect: a replacement (Bytes, neither Delete nor
// MoveTo), a deletion (Delete) or a move (MoveTo, with Bytes as optional new
// content). A nil Bytes on a move keeps the source bytes; a non-nil empty Bytes
// writes an empty destination. Revision is the lowercase- or uppercase-hex
// SHA256 digest of the source bytes the effect was staged against.
//
// Path and MoveTo must be absolute and may be symlinks; they are resolved to
// canonical absolute paths and the symlink itself is preserved. Relative paths
// are rejected instead of being resolved against the process working directory.
type Effect struct {
	Path     string
	Revision string
	Bytes    []byte
	Delete   bool
	MoveTo   string
}

// Outcome reports what happened to one effect. Commit returns one outcome per
// effect in effect order.
//
// Path is the canonical source target, or the effect path when preflight failed
// before the source resolved. Destination is the canonical move destination.
// Committed is true when the requested mutation landed, including a move whose
// source could not be unlinked afterwards; a non-empty Error then names the
// incomplete step, so callers re-read the affected paths. A no-op replacement
// (Bytes equal to the current bytes) reports Committed false with an empty Error
// and BeforeRevision equal to AfterRevision. Failed and unattempted outcomes
// carry Error; AfterRevision stays empty for deletions.
type Outcome struct {
	Path, Destination             string
	BeforeRevision, AfterRevision string
	Committed                     bool
	Error                         string
}

// Commit validates, locks and applies effects.
//
// A nil error means every effect is committed or reported as a no-op. Any other
// error means at least one effect did not land; the returned outcomes still
// describe every effect, including those committed before the failure, and every
// effect that was never attempted carries Error. lockDir must be absolute, holds
// private advisory lock files and is created mode 0700 when missing.
func Commit(ctx context.Context, lockDir string, effects []Effect) ([]Outcome, error) {
	outcomes := newOutcomes(effects, nil)
	if len(effects) == 0 {
		return outcomes, nil
	}
	if err := ctx.Err(); err != nil {
		markUnattempted(outcomes, 0, "not attempted: "+err.Error())
		return outcomes, fmt.Errorf("fileedit: commit not started: %w", err)
	}
	if !filepath.IsAbs(lockDir) {
		err := fmt.Errorf("fileedit: edit lock directory %q must be absolute", lockDir)
		markUnattempted(outcomes, 0, "not attempted: "+err.Error())
		return outcomes, err
	}
	targets, err := preflight(effects)
	if err != nil {
		markUnattempted(outcomes, 0, "not attempted: "+err.Error())
		return outcomes, err
	}
	outcomes = newOutcomes(effects, targets)
	release, err := lockAll(ctx, lockDir, targets)
	if err != nil {
		markUnattempted(outcomes, 0, "not attempted: "+err.Error())
		return outcomes, err
	}
	defer release()
	if err := validateLocked(ctx, targets); err != nil {
		markUnattempted(outcomes, 0, "not attempted: "+err.Error())
		return outcomes, err
	}
	committed := 0
	for i := range targets {
		if err := ctx.Err(); err != nil {
			markUnattempted(outcomes, i, "not attempted: "+err.Error())
			return outcomes, fmt.Errorf("fileedit: commit stopped after %d committed effect(s): %w", committed, err)
		}
		out := &outcomes[i]
		done, err := commitOne(ctx, &targets[i], out)
		out.Committed = done
		if done {
			committed++
		}
		if err != nil {
			out.Error = err.Error()
			markUnattempted(outcomes, i+1, "not attempted: commit stopped after an earlier failure")
			return outcomes, fmt.Errorf("fileedit: effect %d at %s failed after %d committed effect(s): %w", i, out.Path, committed, err)
		}
	}
	return outcomes, nil
}

// preflight resolves every effect and rejects aliases; it mutates nothing.
func preflight(effects []Effect) ([]target, error) {
	targets := make([]target, len(effects))
	identities := make(map[string]int, 2*len(effects))
	for i, effect := range effects {
		if err := validateEffect(effect); err != nil {
			return targets, fmt.Errorf("preflight: effect %d: %w; fix the staged effect and retry", i, err)
		}
		source, info, err := resolveSource(effect.Path)
		if err != nil {
			return targets, fmt.Errorf("preflight: effect %d: %w; re-read the source and retry", i, err)
		}
		targets[i] = target{effect: effect, source: source, info: info}
		if previous, ok := identities[source]; ok {
			return targets, fmt.Errorf("%w: effects %d and %d both resolve to %s; merge them into one effect", ErrAliasConflict, previous, i, source)
		}
		identities[source] = i
		if effect.MoveTo == "" {
			continue
		}
		destination, err := resolveDestination(effect.MoveTo)
		if err != nil {
			return targets, fmt.Errorf("preflight: effect %d: %w; re-read the destination and retry", i, err)
		}
		targets[i].dest = destination
		if previous, ok := identities[destination]; ok {
			return targets, fmt.Errorf("%w: effects %d and %d both resolve to %s; use distinct targets", ErrAliasConflict, previous, i, destination)
		}
		identities[destination] = i
	}
	return targets, nil
}

func validateEffect(e Effect) error {
	if e.Path == "" {
		return errors.New("source path is required")
	}
	if !filepath.IsAbs(e.Path) {
		return fmt.Errorf("source path %s must be absolute", e.Path)
	}
	if e.MoveTo != "" && !filepath.IsAbs(e.MoveTo) {
		return fmt.Errorf("move destination %s must be absolute", e.MoveTo)
	}
	if !validRevision(e.Revision) {
		return fmt.Errorf("revision for %s must be a 64 character SHA256 hex digest", e.Path)
	}
	switch {
	case e.Delete && e.MoveTo != "":
		return fmt.Errorf("%s cannot be deleted and moved in one effect", e.Path)
	case e.Delete && e.Bytes != nil:
		return fmt.Errorf("%s cannot be deleted and rewritten in one effect", e.Path)
	case e.Delete, e.MoveTo != "":
		return nil
	case e.Bytes == nil:
		return fmt.Errorf("%s has no replacement bytes, deletion or move destination", e.Path)
	}
	return nil
}

// validateLocked revalidates every target under the locks before any mutation.
func validateLocked(ctx context.Context, targets []target) error {
	for i := range targets {
		if err := ctx.Err(); err != nil {
			return fmt.Errorf("preflight aborted: %w", err)
		}
		if err := targets[i].checkDestination(); err != nil {
			return fmt.Errorf("preflight: effect %d: %w; nothing was changed", i, err)
		}
		observed, err := targets[i].observeSource(ctx)
		if err != nil {
			return fmt.Errorf("preflight: effect %d: %w; nothing was changed", i, err)
		}
		targets[i].before = observed.revision
	}
	return nil
}

func commitOne(ctx context.Context, t *target, out *Outcome) (bool, error) {
	if err := t.checkDestination(); err != nil {
		return false, err
	}
	observed, err := t.observeSource(ctx)
	if err != nil {
		return false, err
	}
	out.BeforeRevision = observed.revision
	switch {
	case t.effect.Delete:
		return commitDelete(t, out)
	case t.effect.MoveTo != "":
		return commitMove(t, out, observed.info)
	default:
		return commitReplace(t, out, observed)
	}
}

func commitReplace(t *target, out *Outcome, observed observation) (bool, error) {
	replacement := revisionOf(t.effect.Bytes)
	if observed.revision == replacement && observed.size == int64(len(t.effect.Bytes)) {
		out.AfterRevision = observed.revision
		return false, nil
	}
	landed, err := replaceFile(t.source, t.effect.Bytes, observed.info.Mode().Perm())
	if landed {
		out.AfterRevision = replacement
	}
	return landed, err
}

func commitDelete(t *target, out *Outcome) (bool, error) {
	if err := removePath(t.source); err != nil {
		return false, fmt.Errorf("delete %s: %w", t.source, err)
	}
	if err := syncDir(dirOf(t.source)); err != nil {
		return true, fmt.Errorf("deleted %s but directory durability is uncertain: %w", t.source, err)
	}
	return true, nil
}

func commitMove(t *target, out *Outcome, info os.FileInfo) (bool, error) {
	out.Destination = t.dest
	after := out.BeforeRevision
	if t.effect.Bytes != nil {
		if err := publishFile(t.dest, t.effect.Bytes, info.Mode().Perm()); err != nil {
			return false, err
		}
		after = revisionOf(t.effect.Bytes)
	} else if err := linkNoReplace(t.source, t.dest); err != nil {
		return false, err
	}
	out.AfterRevision = after
	if err := removePath(t.source); err != nil {
		return true, fmt.Errorf("destination %s has the content but source %s could not be removed; re-read both paths: %w", t.dest, t.source, err)
	}
	if err := syncDir(dirOf(t.dest)); err != nil {
		return true, fmt.Errorf("moved to %s but destination directory durability is uncertain: %w", t.dest, err)
	}
	if err := syncDir(dirOf(t.source)); err != nil {
		return true, fmt.Errorf("moved to %s but source directory durability is uncertain: %w", t.dest, err)
	}
	return true, nil
}

func newOutcomes(effects []Effect, targets []target) []Outcome {
	outcomes := make([]Outcome, len(effects))
	for i, effect := range effects {
		outcomes[i].Path = effect.Path
		if i < len(targets) {
			if targets[i].source != "" {
				outcomes[i].Path = targets[i].source
			}
			outcomes[i].BeforeRevision = targets[i].before
		}
	}
	return outcomes
}

func markUnattempted(outcomes []Outcome, from int, reason string) {
	for i := from; i < len(outcomes); i++ {
		if outcomes[i].Committed || outcomes[i].Error != "" {
			continue
		}
		outcomes[i].Error = reason
	}
}
