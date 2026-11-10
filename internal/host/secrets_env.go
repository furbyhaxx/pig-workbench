package host

import (
	"bufio"
	"bytes"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strings"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/fileio"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

const maxEnvFileBytes = 1 << 20

// nameMatcher compiles case-insensitive environment-name globs minus the exclusions.
type nameMatcher struct{ include, exclude []string }

func newNameMatcher(detection config.Detection) nameMatcher {
	lower := func(globs []string) []string {
		out := make([]string, len(globs))
		for i, glob := range globs {
			out[i] = strings.ToLower(glob)
		}
		return out
	}
	return nameMatcher{include: lower(detection.EnvNames), exclude: lower(detection.ExcludedEnvNames)}
}

func (m nameMatcher) secret(name string) bool {
	name = strings.ToLower(name)
	for _, glob := range m.exclude {
		if ok, _ := path.Match(glob, name); ok {
			return false
		}
	}
	for _, glob := range m.include {
		if ok, _ := path.Match(glob, name); ok {
			return true
		}
	}
	return false
}

// parseEnvFile reads KEY=VALUE lines, with optional `export`, quotes and comments.
func parseEnvFile(data []byte) map[string]string {
	values := make(map[string]string)
	scanner := bufio.NewScanner(bytes.NewReader(data))
	scanner.Buffer(make([]byte, 0, 64<<10), maxEnvFileBytes)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimSpace(strings.TrimPrefix(line, "export "))
		name, value, ok := strings.Cut(line, "=")
		name = strings.TrimSpace(name)
		if !ok || name == "" || strings.ContainsAny(name, " \t") {
			continue
		}
		value = strings.TrimSpace(value)
		if value != "" && (value[0] == '"' || value[0] == '\'') {
			if end := strings.IndexByte(value[1:], value[0]); end >= 0 {
				value = value[1 : 1+end]
			}
		} else if cut, _, found := strings.Cut(value, " #"); found {
			value = strings.TrimSpace(cut)
		}
		values[name] = value
	}
	return values
}

// envFiles lists ~/.env and every .env* file from the repository root down to cwd; without a repository only cwd counts.
func envFiles(home, cwd string) []string {
	var files []string
	if home != "" {
		files = append(files, filepath.Join(home, ".env"))
	}
	chain := []string{cwd}
	for dir := cwd; filepath.Dir(dir) != dir; {
		if _, err := os.Stat(filepath.Join(dir, ".git")); err == nil {
			break
		}
		dir = filepath.Dir(dir)
		chain = append(chain, dir)
	}
	if _, err := os.Stat(filepath.Join(chain[len(chain)-1], ".git")); err != nil {
		chain = chain[:1]
	}
	for i := len(chain) - 1; i >= 0; i-- {
		matches, _ := filepath.Glob(filepath.Join(chain[i], ".env*"))
		sort.Strings(matches)
		files = append(files, matches...)
	}
	return files
}

const (
	maxReferencedEnvFiles = 16
	maxReferencedWords    = 256
)

// envShaped reports whether a file name is an env file: `.env`, `.env.local`, `.envrc`, `prod.env`.
func envShaped(name string) bool {
	base := filepath.Base(name)
	return strings.HasPrefix(base, ".env") || strings.HasSuffix(base, ".env")
}

// referencedEnvFiles lists the env files a tool call names: an env-shaped file itself, and the
// `.env*` files directly inside a named directory. Words are the call's string arguments and,
// for text with spaces such as a shell command, its shell words. Both counts are bounded.
func referencedEnvFiles(cwd, home string, input map[string]any) []string {
	if dir, ok := input["cwd"].(string); ok && dir != "" {
		cwd = resolveWord(cwd, home, dir)
	}
	var words []string
	var collect func(value any, depth int)
	collect = func(value any, depth int) {
		if depth > 4 || len(words) >= maxReferencedWords {
			return
		}
		switch typed := value.(type) {
		case string:
			// A path argument may contain spaces; a command is also split into its words.
			if len(typed) <= 4096 && !strings.Contains(typed, "\n") {
				words = append(words, typed)
			}
			words = append(words, shellishWords(typed)...)
		case []any:
			for _, item := range typed {
				collect(item, depth+1)
			}
		case map[string]any:
			keys := make([]string, 0, len(typed))
			for key := range typed {
				keys = append(keys, key)
			}
			sort.Strings(keys)
			for _, key := range keys {
				collect(typed[key], depth+1)
			}
		}
	}
	collect(input, 0)
	if len(words) > maxReferencedWords {
		words = words[:maxReferencedWords]
	}
	var files []string
	seen := make(map[string]bool)
	add := func(file string) {
		if !seen[file] && len(files) < maxReferencedEnvFiles {
			seen[file] = true
			files = append(files, file)
		}
	}
	for _, word := range words {
		word = strings.TrimPrefix(word, "file://")
		if strings.Contains(word, "://") {
			continue
		}
		candidates := []string{word}
		// A read selector such as `.env:1-20` follows the last colon.
		if cut := strings.LastIndexByte(word, ':'); cut > 0 {
			candidates = append(candidates, word[:cut])
		}
		for _, candidate := range candidates {
			file := resolveWord(cwd, home, candidate)
			info, err := os.Stat(file)
			if err != nil {
				continue
			}
			if info.Mode().IsRegular() && envShaped(file) {
				add(file)
			} else if info.IsDir() {
				matches, _ := filepath.Glob(filepath.Join(file, ".env*"))
				sort.Strings(matches)
				for _, match := range matches {
					add(match)
				}
			}
			break
		}
	}
	return files
}

// shellishWords splits text at spaces and shell operators and keeps quoted text together.
// It is a word finder for detection, not a shell parser.
func shellishWords(text string) []string {
	var words []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	var quote rune
	for _, r := range text {
		switch {
		case quote != 0:
			if r == quote {
				quote = 0
			} else {
				word.WriteRune(r)
			}
		case r == '\'' || r == '"':
			quote = r
		case r == ' ' || r == '\t' || r == '\n' || strings.ContainsRune(";|&<>()=`", r):
			flush()
		default:
			word.WriteRune(r)
		}
	}
	flush()
	return words
}

func resolveWord(cwd, home, word string) string {
	if word == "~" || strings.HasPrefix(word, "~/") {
		return filepath.Join(home, strings.TrimPrefix(word, "~"))
	}
	if filepath.IsAbs(word) {
		return filepath.Clean(word)
	}
	return filepath.Join(cwd, word)
}

func readEnvFile(file string) (map[string]string, bool) {
	handle, _, err := fileio.OpenRegular(file)
	if err != nil {
		return nil, false
	}
	defer handle.Close()
	data, err := io.ReadAll(io.LimitReader(handle, maxEnvFileBytes))
	if err != nil {
		return nil, false
	}
	return parseEnvFile(data), true
}

// learnEnvironment stores every secret-named value of the process environment and the
// .env files so entry-time redaction already knows them (SPEC-0013 §2).
func learnEnvironment(detector *secrets.Detector, matcher nameMatcher, environ []string, home, cwd string) error {
	for _, entry := range environ {
		name, value, ok := strings.Cut(entry, "=")
		if ok && matcher.secret(name) {
			if err := detector.Learn([]byte(value), "token", "env"); err != nil {
				return err
			}
		}
	}
	for _, file := range envFiles(home, cwd) {
		if err := learnEnvFile(detector, matcher, file); err != nil {
			return err
		}
	}
	return nil
}

// learnEnvFile stores the secret-named values of one env file; an unreadable file is skipped.
func learnEnvFile(detector *secrets.Detector, matcher nameMatcher, file string) error {
	values, ok := readEnvFile(file)
	if !ok {
		return nil
	}
	for name, value := range values {
		if matcher.secret(name) {
			if err := detector.Learn([]byte(value), "token", "env-file"); err != nil {
				return err
			}
		}
	}
	return nil
}
