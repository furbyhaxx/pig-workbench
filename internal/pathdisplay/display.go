// Package pathdisplay renders a recorded target at a terminal width without I/O.
package pathdisplay

import (
	"os"
	"path"
	"strings"

	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
)

// Target is a recorded location: the full target and the root it was resolved against.
type Target struct {
	// Value is an absolute path, a file:// or ssh:// URL, or a Workbench resource URI.
	Value string
	// Root is the root the value was resolved against; empty means none.
	Root string
}

// Display renders target at width terminal cells, applying the shortest form,
// home substitution, fish shortening and the documented fallbacks.
func Display(t Target, width int) string {
	authority, prefix, rest, isPath := split(t.Value)
	if !isPath {
		return prefix + fit(rest, width)
	}
	shown := rest
	if home, err := os.UserHomeDir(); err == nil && home != "" {
		if rest == home {
			shown = "~"
		} else if relative, ok := strings.CutPrefix(rest, home+"/"); ok {
			shown = "~/" + relative
		}
	}
	if authority == "" {
		if candidate, ok := relativeTo(t.Root, rest); ok && tui.VisibleWidth(candidate) <= tui.VisibleWidth(shown) {
			shown = candidate
		}
	} else if rootAuthority, _, rootPath, ok := split(t.Root); ok && rootAuthority == authority {
		if candidate, ok := strings.CutPrefix(rest, strings.TrimSuffix(rootPath, "/")+"/"); ok {
			shown = candidate
		}
	}
	if authority != "" {
		available := width - tui.VisibleWidth(prefix) - tui.VisibleWidth(authority) - 1
		if available < 1 {
			available = 1
		}
		return prefix + authority + ":" + fit(shown, available)
	}
	return prefix + fit(shown, width)
}

// split separates an authority-bearing URL from its path. isPath is false for
// Workbench resource URIs, which are never related to a filesystem root.
func split(value string) (authority, prefix, rest string, isPath bool) {
	if strings.HasPrefix(value, "ssh://") {
		remainder := strings.TrimPrefix(value, "ssh://")
		authority, rest, found := strings.Cut(remainder, "/")
		if !found {
			return authority, "ssh://", "", true
		}
		return authority, "ssh://", "/" + rest, true
	}
	if strings.HasPrefix(value, "file://") {
		return "", "", strings.TrimPrefix(value, "file://"), true
	}
	if scheme, remainder, found := strings.Cut(value, "://"); found && scheme != "" {
		return "", scheme + "://", remainder, false
	}
	return "", "", value, true
}

// relativeTo returns the shortest root-relative form of an absolute path.
func relativeTo(root, value string) (string, bool) {
	if root == "" || !strings.HasPrefix(root, "/") || !strings.HasPrefix(value, "/") {
		return "", false
	}
	root = strings.TrimSuffix(path.Clean(root), "/")
	clean := path.Clean(value)
	if !strings.HasPrefix(clean, "/") {
		return "", false
	}
	rootParts := strings.Split(strings.TrimPrefix(root, "/"), "/")
	valueParts := strings.Split(strings.TrimPrefix(clean, "/"), "/")
	common := 0
	for common < len(rootParts) && common < len(valueParts) && rootParts[common] == valueParts[common] {
		common++
	}
	parts := make([]string, 0, len(rootParts)-common+len(valueParts)-common)
	for range len(rootParts) - common {
		parts = append(parts, "..")
	}
	parts = append(parts, valueParts[common:]...)
	if len(parts) == 0 {
		return ".", true
	}
	return strings.Join(parts, "/"), true
}

// fit shortens shown to width: first by fish shortening non-final segments from
// the left, then by eliding middle segments, then inside the basename.
func fit(shown string, width int) string {
	if width <= 0 || tui.VisibleWidth(shown) <= width {
		return shown
	}
	original, trailing := splitSegments(shown)
	segments := append([]string(nil), original...)
	last := len(segments) - 1
	for index := firstShortenable(segments); index < last; index++ {
		if segments[index] == ".." {
			continue
		}
		segments[index] = shortenSegment(segments[index])
		candidate := joinSegments(segments, trailing)
		if tui.VisibleWidth(candidate) <= width {
			return candidate
		}
	}
	base := ""
	if last >= 0 {
		base = segments[last]
	}
	// Elide the middle from the untouched prefix; fish shortening is not applied
	// to an elided form, matching the documented goldens.
	for keep := last - 1; keep >= 1; keep-- {
		if keep < firstShortenable(original) {
			break
		}
		candidate := strings.Join(original[:keep], "/") + "/…/" + base
		if original[0] == "" {
			candidate = "/" + strings.Join(original[1:keep], "/") + "/…/" + base
		}
		if tui.VisibleWidth(candidate) <= width {
			return candidate + trailing
		}
	}
	candidate := "…/" + base
	if tui.VisibleWidth(candidate) <= width {
		return candidate + trailing
	}
	return middleEllipsis(base, width)
}

func splitSegments(shown string) ([]string, string) {
	trailing := ""
	if strings.HasSuffix(shown, "/") && shown != "/" {
		trailing = "/"
		shown = strings.TrimSuffix(shown, "/")
	}
	return strings.Split(shown, "/"), trailing
}

func joinSegments(segments []string, trailing string) string {
	return strings.Join(segments, "/") + trailing
}

// firstShortenable skips the kept prefix: leading empties from a rooted path,
// `~`, `.` and `..` together with the segment that follows each `..`.
func firstShortenable(segments []string) int {
	index := 0
	for index < len(segments) {
		switch segments[index] {
		case "", "~", ".":
			index++
		case "..":
			index += 2
		default:
			return index
		}
	}
	return index
}

// shortenSegment keeps any leading dots and the first following rune.
func shortenSegment(segment string) string {
	runes := []rune(segment)
	index := 0
	for index < len(runes) && runes[index] == '.' {
		index++
	}
	if index >= len(runes) {
		return segment
	}
	return string(runes[:index+1])
}

// middleEllipsis keeps as much of the basename as fits around a single ellipsis.
func middleEllipsis(base string, width int) string {
	if tu := tui.VisibleWidth(base); tu <= width {
		return base
	}
	runes := []rune(base)
	if width <= 1 {
		return "…"
	}
	keep := width - 1
	head := keep / 2
	tail := keep - head
	return string(runes[:head]) + "…" + string(runes[len(runes)-tail:])
}
