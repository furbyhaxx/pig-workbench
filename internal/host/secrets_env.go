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
		values, ok := readEnvFile(file)
		if !ok {
			continue
		}
		for name, value := range values {
			if matcher.secret(name) {
				if err := detector.Learn([]byte(value), "token", "env-file"); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
