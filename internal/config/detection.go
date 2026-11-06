package config

import (
	"fmt"
	"sort"
	"strings"

	"github.com/BurntSushi/toml"
)

// defaultEnvNames are the case-insensitive globs that mark an environment variable as secret.
var defaultEnvNames = []string{"*KEY*", "*SECRET*", "*TOKEN*", "*PASSWORD*", "*PASS", "*AUTH*", "*CREDENTIAL*", "*PRIVATE*"}

// DetectionPattern is one user token shape: a name plus exactly one of Regex, Prefix or Suffix.
type DetectionPattern struct {
	Name      string `toml:"name"`
	Regex     string `toml:"regex"`
	Prefix    string `toml:"prefix"`
	Suffix    string `toml:"suffix"`
	MinLength int    `toml:"min-length"`
}

// Detection is the merged secret detection policy of Secrets.toml (SPEC-0013 §2).
type Detection struct {
	// EnvNames are globs matched case-insensitively against environment variable names.
	EnvNames []string
	// Patterns append across layers, user first.
	Patterns []DetectionPattern
	// Excluded* lists remove false positives and append across layers.
	ExcludedValues, ExcludedHashes, ExcludedPatterns, ExcludedEnvNames []string
}

type detectionFile struct {
	Version *int64 `toml:"version"`
	Env     *struct {
		Names []string `toml:"names"`
	} `toml:"env"`
	Patterns   []DetectionPattern `toml:"patterns"`
	Exclusions struct {
		Values   []string `toml:"values"`
		Hashes   []string `toml:"hashes"`
		Patterns []string `toml:"patterns"`
		EnvNames []string `toml:"env-names"`
	} `toml:"exclusions"`
}

// ReadDetection merges the user Secrets.toml with the project one while the project is trusted.
// A project file may add patterns and exclusions but never change the environment name globs.
func ReadDetection(userPath, projectPath string, projectTrusted bool) (Detection, error) {
	result := Detection{EnvNames: append([]string(nil), defaultEnvNames...)}
	files := []struct {
		path    string
		project bool
	}{{userPath, false}}
	if projectTrusted {
		files = append(files, struct {
			path    string
			project bool
		}{projectPath, true})
	}
	for _, source := range files {
		if source.path == "" {
			continue
		}
		data, _, found, err := readTarget(source.path)
		if err != nil {
			return Detection{}, err
		}
		if !found {
			continue
		}
		if len(data) > MaxDocumentBytes {
			return Detection{}, &Error{Path: source.path, Reason: "detection policy exceeds 1 MiB; reduce its size"}
		}
		var file detectionFile
		meta, err := toml.Decode(string(data), &file)
		if err != nil {
			problem := &Error{Path: source.path, Reason: "invalid TOML in the detection policy; fix the document before reloading"}
			if parse, ok := err.(toml.ParseError); ok {
				problem.Line, problem.Column = parse.Position.Line, parse.Position.Col
			} else {
				problem.Reason = "detection policy has a value of the wrong type: " + err.Error()
			}
			return Detection{}, problem
		}
		if undecoded := meta.Undecoded(); len(undecoded) > 0 {
			keys := make([]string, len(undecoded))
			for i, key := range undecoded {
				keys[i] = key.String()
			}
			sort.Strings(keys)
			return Detection{}, &Error{Path: source.path, Key: keys[0], Reason: "unknown detection setting; Secrets.toml accepts version, [env] names, [[patterns]] and [exclusions]"}
		}
		if file.Version != nil && *file.Version != 2 {
			return Detection{}, &Error{Path: source.path, Key: "version", Reason: "expected supported integer version 2"}
		}
		if file.Env != nil {
			if source.project {
				return Detection{}, &Error{Path: source.path, Key: "env.names", Reason: "a project detection policy may only add patterns and exclusions; set env names in the user Secrets.toml"}
			}
			if len(file.Env.Names) == 0 {
				return Detection{}, &Error{Path: source.path, Key: "env.names", Reason: "env names must list at least one glob; remove the table to keep the defaults"}
			}
			result.EnvNames = append([]string(nil), file.Env.Names...)
		}
		for i, pattern := range file.Patterns {
			if pattern.Name == "" {
				return Detection{}, &Error{Path: source.path, Key: fmt.Sprintf("patterns[%d].name", i), Reason: "every pattern needs a name"}
			}
			if strings.ContainsAny(pattern.Name, " \t\n") {
				return Detection{}, &Error{Path: source.path, Key: fmt.Sprintf("patterns[%d].name", i), Reason: "pattern names cannot contain whitespace"}
			}
		}
		result.Patterns = append(result.Patterns, file.Patterns...)
		result.ExcludedValues = append(result.ExcludedValues, file.Exclusions.Values...)
		result.ExcludedHashes = append(result.ExcludedHashes, file.Exclusions.Hashes...)
		result.ExcludedPatterns = append(result.ExcludedPatterns, file.Exclusions.Patterns...)
		result.ExcludedEnvNames = append(result.ExcludedEnvNames, file.Exclusions.EnvNames...)
	}
	return result, nil
}
