package project

import (
	"net"
	"net/url"
	"strings"
	"unicode"
)

func resolveRemote(raw string) string {
	raw = strings.TrimSpace(raw)
	if raw == "" || strings.ContainsAny(raw, "\\?#%") || strings.IndexFunc(raw, unicode.IsSpace) >= 0 {
		return ""
	}
	var host, path string
	if strings.Contains(raw, "://") {
		parsed, err := url.Parse(raw)
		if err != nil || parsed.Host == "" || parsed.Opaque != "" || parsed.RawQuery != "" || parsed.Fragment != "" || parsed.RawPath != "" {
			return ""
		}
		switch strings.ToLower(parsed.Scheme) {
		case "http", "https", "ssh", "git":
		default:
			return ""
		}
		host, path = parsed.Hostname(), parsed.Path
	} else if at := strings.IndexByte(raw, '@'); at >= 0 {
		if at == 0 || strings.Contains(raw[at+1:], "@") {
			return ""
		}
		var found bool
		host, path, found = strings.Cut(raw[at+1:], ":")
		if !found {
			return ""
		}
	} else {
		var found bool
		host, path, found = strings.Cut(raw, "/")
		if !found || (!strings.Contains(host, ".") && !strings.EqualFold(host, "localhost") && net.ParseIP(host) == nil) {
			return ""
		}
	}
	host = strings.ToLower(host)
	if !validRemoteHost(host) {
		return ""
	}
	if strings.Contains(path, "//") {
		return ""
	}
	path = strings.TrimPrefix(path, "/")
	path = strings.TrimSuffix(path, "/")
	parts := strings.Split(strings.ToLower(path), "/")
	if len(parts) < 2 {
		return ""
	}
	for _, part := range parts {
		if part == "" || part == "." || part == ".." || strings.ContainsAny(part, ":@\\") || strings.IndexFunc(part, unicode.IsControl) >= 0 {
			return ""
		}
	}
	parts[len(parts)-1] = strings.TrimSuffix(parts[len(parts)-1], ".git")
	if parts[len(parts)-1] == "" || parts[len(parts)-1] == "." || parts[len(parts)-1] == ".." {
		return ""
	}
	return host + "/" + strings.Join(parts, "/")
}

func validRemoteHost(host string) bool {
	if net.ParseIP(host) != nil {
		return true
	}
	for _, label := range strings.Split(host, ".") {
		if label == "" || strings.HasPrefix(label, "-") || strings.HasSuffix(label, "-") {
			return false
		}
		for _, char := range label {
			if (char < 'a' || char > 'z') && (char < '0' || char > '9') && char != '-' {
				return false
			}
		}
	}
	return true
}
