package resource

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"
)

// Target is a parsed ToolPath retaining the requested spelling and opaque URI parts.
type Target struct {
	Requested string    `json:"requestedUri"`
	URI       string    `json:"uri"`
	Scheme    string    `json:"scheme"`
	Path      string    `json:"path"`
	Selection Selection `json:"selector"`
}

// ParsePath uses literal-first local lookup before removing a selector.
func ParsePath(cwd, input, explicit string) (Target, error) {
	target := Target{Requested: input, Scheme: "file"}
	if input == "" {
		return target, errors.New("resource path is empty")
	}
	if strings.ContainsRune(input, 0) {
		return target, errors.New("resource path contains NUL")
	}
	if i := strings.Index(input, "://"); i > 0 {
		scheme := strings.ToLower(input[:i])
		for j, r := range scheme {
			if !(r >= 'a' && r <= 'z' || j > 0 && (r >= '0' && r <= '9' || r == '+' || r == '.' || r == '-')) {
				return target, fmt.Errorf("invalid URI scheme %q", scheme)
			}
		}
		if len(scheme) > 32 {
			return target, errors.New("URI scheme exceeds 32 bytes")
		}
		target.Scheme = scheme
		u, err := url.Parse(input)
		if err != nil {
			return target, errors.New("invalid resource URI")
		}
		if u.User != nil && scheme == "file" {
			return target, errors.New("file URI cannot contain credentials")
		}
		path := u.EscapedPath()
		// An origin without a path names its root document; a port keeps the colon, so no selector can hide there.
		if path == "" && (scheme == "http" || scheme == "https") && u.Host != "" {
			path = "/"
		}
		if scheme == "file" {
			if u.Host != "" && u.Host != "localhost" {
				return target, errors.New("file URI must be local")
			}
			if u.RawQuery != "" || u.Fragment != "" {
				return target, errors.New("file URI does not accept a query or fragment")
			}
			decoded, err := url.PathUnescape(path)
			if err != nil {
				return target, errors.New("invalid URI path encoding")
			}
			if !filepath.IsAbs(decoded) {
				return target, errors.New("file URI requires an absolute path")
			}
			if exists(decoded) {
				target.Path = decoded
				target.URI = FileURI(decoded)
				return withExplicit(target, explicit, false)
			}
		}
		base, selection, has, err := splitSelector(path)
		if err != nil {
			return target, err
		}
		decoded, err := url.PathUnescape(base)
		if err != nil {
			return target, errors.New("invalid URI path encoding")
		}
		if strings.ContainsRune(decoded, 0) {
			return target, errors.New("URI path contains NUL")
		}
		target.Selection = selection
		u.Path = decoded
		u.RawPath = base
		u.Scheme = scheme
		target.URI = u.String()
		if scheme == "file" {
			target.Path = filepath.Clean(decoded)
			target.URI = FileURI(target.Path)
		} else {
			target.Path = decoded
		}
		return withExplicit(target, explicit, has)
	}
	literal := input
	if !filepath.IsAbs(literal) {
		literal = filepath.Join(cwd, literal)
	}
	if exists(literal) {
		target.Path = filepath.Clean(literal)
		target.URI = FileURI(target.Path)
		return withExplicit(target, explicit, false)
	}
	base, selection, has, err := splitSelector(input)
	if err != nil {
		return target, err
	}
	if !filepath.IsAbs(base) {
		base = filepath.Join(cwd, base)
	}
	target.Path = filepath.Clean(base)
	target.URI = FileURI(target.Path)
	target.Selection = selection
	return withExplicit(target, explicit, has)
}

func withExplicit(t Target, explicit string, suffix bool) (Target, error) {
	if explicit == "" {
		return t, nil
	}
	if suffix {
		return t, errors.New("explicit selector and path suffix are mutually exclusive")
	}
	s, err := ParseSelector(explicit)
	t.Selection = s
	return t, err
}

func splitSelector(path string) (string, Selection, bool, error) {
	pieces := strings.Split(path, ":")
	if len(pieces) == 1 {
		return path, Selection{}, false, nil
	}
	last := pieces[len(pieces)-1]
	if last != "raw" && (last == "" || last[0] != '-' && (last[0] < '0' || last[0] > '9')) {
		return path, Selection{}, false, nil
	}
	start := len(pieces) - 1
	if start > 0 && pieces[start-1] == "raw" {
		start--
	}
	if last == "raw" && start > 0 {
		prev := pieces[start-1]
		if prev != "" && (prev[0] == '-' || prev[0] >= '0' && prev[0] <= '9') {
			start--
		}
	}
	if start == 0 {
		return path, Selection{}, false, errors.New("selector has no resource path")
	}
	s, err := ParseSelector(strings.Join(pieces[start:], ":"))
	return strings.Join(pieces[:start], ":"), s, true, err
}

func exists(path string) bool { _, err := os.Lstat(path); return err == nil }
func FileURI(path string) string {
	return (&url.URL{Scheme: "file", Path: filepath.ToSlash(path)}).String()
}
