package project

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

func gitOutput(ctx context.Context, dir string, args ...string) (string, error) {
	if err := ctx.Err(); err != nil {
		return "", err
	}
	command := exec.CommandContext(ctx, "git", append([]string{"-C", dir}, args...)...)
	for _, variable := range os.Environ() {
		key, _, _ := strings.Cut(variable, "=")
		key = strings.ToUpper(key)
		if strings.HasPrefix(key, "GIT_TRACE") || strings.HasPrefix(key, "GIT_CONFIG_KEY_") || strings.HasPrefix(key, "GIT_CONFIG_VALUE_") {
			continue
		}
		switch key {
		case "GIT_DIR", "GIT_WORK_TREE", "GIT_COMMON_DIR", "GIT_IMPLICIT_WORK_TREE", "GIT_INDEX_FILE",
			"GIT_OBJECT_DIRECTORY", "GIT_ALTERNATE_OBJECT_DIRECTORIES", "GIT_PREFIX", "GIT_NAMESPACE",
			"GIT_GRAFT_FILE", "GIT_SHALLOW_FILE", "GIT_REPLACE_REF_BASE", "GIT_NO_REPLACE_OBJECTS",
			"GIT_CONFIG", "GIT_CONFIG_PARAMETERS", "GIT_CONFIG_COUNT", "GIT_CEILING_DIRECTORIES":
			continue
		}
		command.Env = append(command.Env, variable)
	}
	command.Env = append(command.Env, "GIT_OPTIONAL_LOCKS=0", "GIT_TERMINAL_PROMPT=0", "GIT_TRACE2=0", "GIT_TRACE2_EVENT=0", "GIT_TRACE2_PERF=0")
	output, err := command.Output()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return "", ctxErr
	}
	var exitErr *exec.ExitError
	if errors.Is(err, exec.ErrNotFound) || errors.As(err, &exitErr) {
		return "", nil
	}
	if err != nil {
		return "", fmt.Errorf("run git: %w", err)
	}
	return string(output), nil
}

func gitRoot(ctx context.Context, dir string) (string, error) {
	output, err := gitOutput(ctx, dir, "rev-parse", "--show-toplevel")
	if err != nil || output == "" {
		return "", err
	}
	root := strings.TrimSuffix(output, "\n")
	root = strings.TrimSuffix(root, "\r")
	if !filepath.IsAbs(root) {
		return "", nil
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil || !pathWithin(root, dir) {
		return "", nil
	}
	return root, nil
}

func gitRootCommits(ctx context.Context, dir string) (map[string]bool, error) {
	output, err := gitOutput(ctx, dir, "rev-list", "--max-parents=0", "HEAD")
	if err != nil {
		return nil, err
	}
	commits := make(map[string]bool)
	for _, value := range strings.Fields(output) {
		if len(value) != 40 && len(value) != 64 {
			continue
		}
		valid := true
		for _, c := range value {
			if !strings.ContainsRune("0123456789abcdef", c) {
				valid = false
				break
			}
		}
		if valid {
			commits[value] = true
		}
	}
	return commits, nil
}

func gitRemotes(ctx context.Context, dir string) (map[string]bool, error) {
	output, err := gitOutput(ctx, dir, "config", "--local", "--null", "--get-regexp", `^remote\..*\.url$`)
	if err != nil {
		return nil, err
	}
	remotes := make(map[string]bool)
	for _, record := range strings.Split(output, "\x00") {
		_, value, found := strings.Cut(record, "\n")
		if found {
			if remote := resolveRemote(value); remote != "" {
				remotes[remote] = true
			}
		}
	}
	return remotes, nil
}

type repositoryProbe struct {
	roots, remotes             map[string]bool
	rootsLoaded, remotesLoaded bool
}

type repositoryProbes map[string]*repositoryProbe

func (probes repositoryProbes) commits(ctx context.Context, path string) (map[string]bool, error) {
	probe := probes[path]
	if probe == nil {
		probe = &repositoryProbe{}
		probes[path] = probe
	}
	if !probe.rootsLoaded {
		roots, err := gitRootCommits(ctx, path)
		if err != nil {
			return nil, err
		}
		probe.roots, probe.rootsLoaded = roots, true
	}
	return probe.roots, nil
}

func (probes repositoryProbes) remotes(ctx context.Context, path string) (map[string]bool, error) {
	probe := probes[path]
	if probe == nil {
		probe = &repositoryProbe{}
		probes[path] = probe
	}
	if !probe.remotesLoaded {
		remotes, err := gitRemotes(ctx, path)
		if err != nil {
			return nil, err
		}
		probe.remotes, probe.remotesLoaded = remotes, true
	}
	return probe.remotes, nil
}
