package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"mvdan.cc/sh/v3/shell"
	"mvdan.cc/sh/v3/syntax"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

// schemeCommand is one G8.20 in-process verb.
type schemeCommand struct {
	verb  string
	paths []string
	text  string
}

// schemeWord is one static shell word that names a scheme path, with its source span.
type schemeWord struct {
	start, end int
	value      string
}

// schemePlan says how a shell command uses scheme paths: one in-process verb, or
// argument words to replace with readable copies before a real shell runs it.
type schemePlan struct {
	standalone *schemeCommand
	reads      []schemeWord
}

var errSchemeWrite = errors.New("a scheme path can be written only by a standalone rm, mv, cp, mkdir or `text > path` command; run that command on its own")

// Commands that change their path arguments, so a readable copy cannot stand in for them.
var schemeWriters = map[string]bool{"rm": true, "mv": true, "mkdir": true, "rmdir": true, "touch": true, "tee": true, "chmod": true, "chown": true, "truncate": true}

// Commands whose last argument is a destination.
var schemeDestinations = map[string]bool{"cp": true, "install": true, "ln": true}

// Commands whose arguments are text, never files.
var schemeTextCommands = map[string]bool{"echo": true, "printf": true, "git": true}

// Commands whose first operand is a pattern or program, not a file.
var schemePatternCommands = map[string]bool{"grep": true, "egrep": true, "fgrep": true, "rg": true, "ag": true, "sed": true, "awk": true, "jq": true}

// planSchemeCommand finds scheme paths in a shell command. Only a static command word or
// redirect target that starts with a scheme prefix counts: text inside other words, heredoc
// bodies and words built by expansion belong to the shell. Unparseable input has no plan.
func planSchemeCommand(command string, schemes []string) (schemePlan, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		return schemePlan{}, nil
	}
	standalone, ok, err := standaloneSchemeCommand(command, file, schemes)
	if err != nil {
		return schemePlan{}, err
	}
	if ok {
		return schemePlan{standalone: &standalone}, nil
	}
	var plan schemePlan
	var refused error
	read := func(word *syntax.Word, value string) {
		plan.reads = append(plan.reads, schemeWord{start: int(word.Pos().Offset()), end: int(word.End().Offset()), value: value})
	}
	syntax.Walk(file, func(node syntax.Node) bool {
		switch part := node.(type) {
		case *syntax.Redirect:
			value, ok := staticShellWord(command, part.Word)
			if !ok || !schemePath(value, schemes) {
				break
			}
			switch part.Op {
			case syntax.RdrIn:
				read(part.Word, value)
			case syntax.Hdoc, syntax.DashHdoc, syntax.WordHdoc:
			default:
				refused = errSchemeWrite
			}
		case *syntax.CallExpr:
			verb, operands := "", 0
			for i, word := range part.Args {
				value, ok := staticShellWord(command, word)
				if i == 0 {
					if ok {
						verb = value
					}
					continue
				}
				if schemeTextCommands[verb] {
					break
				}
				if ok && strings.HasPrefix(value, "-") {
					continue
				}
				operands++
				if !ok || !schemePath(value, schemes) || schemePatternCommands[verb] && operands == 1 {
					continue
				}
				if schemeWriters[verb] || schemeDestinations[verb] && i == len(part.Args)-1 {
					refused = errSchemeWrite
					continue
				}
				read(word, value)
			}
		}
		return refused == nil
	})
	if refused != nil {
		return schemePlan{}, refused
	}
	return plan, nil
}

// standaloneSchemeCommand recognizes the in-process forms: one simple ls, cat, rm, mv, cp or
// mkdir on scheme paths, or one `text > path` redirect.
func standaloneSchemeCommand(command string, file *syntax.File, schemes []string) (schemeCommand, bool, error) {
	if len(file.Stmts) != 1 {
		return schemeCommand{}, false, nil
	}
	stmt := file.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || stmt.Background || stmt.Negated || stmt.Coprocess || stmt.Disown || len(call.Assigns) != 0 || len(call.Args) == 0 {
		return schemeCommand{}, false, nil
	}
	fields := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		value, ok := staticShellWord(command, word)
		if !ok {
			return schemeCommand{}, false, nil
		}
		fields = append(fields, value)
	}
	if len(stmt.Redirs) > 0 {
		if len(stmt.Redirs) != 1 || stmt.Redirs[0].Op != syntax.RdrOut || stmt.Redirs[0].N != nil || len(fields) != 1 {
			return schemeCommand{}, false, nil
		}
		target, ok := staticShellWord(command, stmt.Redirs[0].Word)
		if !ok || !schemePath(target, schemes) {
			return schemeCommand{}, false, nil
		}
		return schemeCommand{verb: "redirect", paths: []string{target}, text: fields[0]}, true, nil
	}
	switch verb := fields[0]; verb {
	case "ls", "cat", "rm", "mkdir":
		if len(fields) == 2 && schemePath(fields[1], schemes) {
			return schemeCommand{verb: verb, paths: fields[1:2]}, true, nil
		}
	case "mv", "cp":
		if len(fields) != 3 {
			break
		}
		from, to := schemePath(fields[1], schemes), schemePath(fields[2], schemes)
		if from && to {
			return schemeCommand{verb: verb, paths: fields[1:3]}, true, nil
		}
		if verb == "mv" && (from || to) {
			return schemeCommand{}, false, errors.New("mv between a scheme path and a local path is unsupported; both ends must be scheme paths")
		}
	}
	return schemeCommand{}, false, nil
}

func staticShellWord(command string, word *syntax.Word) (string, bool) {
	if word == nil {
		return "", false
	}
	static := true
	syntax.Walk(word, func(node syntax.Node) bool {
		switch part := node.(type) {
		case *syntax.ParamExp, *syntax.CmdSubst, *syntax.ArithmExp, *syntax.ProcSubst, *syntax.ExtGlob, *syntax.BraceExp:
			static = false
		case *syntax.SglQuoted:
			static = static && !part.Dollar
		case *syntax.DblQuoted:
			static = static && !part.Dollar
		case *syntax.Lit:
			if strings.ContainsAny(part.Value, "*?[") || strings.HasPrefix(part.Value, "~") {
				static = false
			}
		}
		return static
	})
	if !static {
		return "", false
	}
	start, end := int(word.Pos().Offset()), int(word.End().Offset())
	if start < 0 || end > len(command) || start > end {
		return "", false
	}
	words, err := shell.Fields(command[start:end], func(string) string { return "" })
	if err != nil || len(words) != 1 {
		return "", false
	}
	return words[0], true
}

func schemePath(field string, schemes []string) bool {
	for _, scheme := range schemes {
		if strings.HasPrefix(field, scheme+"://") {
			return true
		}
	}
	return false
}

// runSchemeCommand executes a parsed verb through the router and returns shell-like text.
func (a *app) runSchemeCommand(ctx context.Context, s *session, cmd schemeCommand) (string, error) {
	switch cmd.verb {
	case "ls":
		target, err := resource.ParsePath("/", cmd.paths[0], "")
		if err != nil {
			return "", err
		}
		result, err := listContainer(ctx, s, target, "", 200, true)
		return result.Content, err
	case "cat":
		target, err := resource.ParsePath("/", cmd.paths[0], "")
		if err != nil {
			return "", err
		}
		page, err := s.router.Read(ctx, target, resource.Budget{Bytes: 50 << 10, Lines: 2000})
		if err != nil {
			return "", err
		}
		return page.Content, nil
	default:
		return "", errors.New("unsupported scheme command")
	}
}

// externalSchemes are routed by Workbench for its own tools but belong to the programs a
// shell command runs.
var externalSchemes = map[string]bool{"http": true, "https": true}

// prepareShellCommand applies scheme paths to a shell command. A standalone verb runs
// in-process (handled). Otherwise the returned command has each readable scheme argument
// replaced by a read-only copy; a command without scheme paths comes back unchanged.
func (a *app) prepareShellCommand(ctx sdk.Context, work context.Context, s *session, command string) (bool, any, string, error) {
	if !s.settingBool("bash.route-to-tools", true) {
		return false, nil, command, nil
	}
	schemes := []string{}
	for _, scheme := range s.router.Schemes() {
		if !s.router.Locatable(scheme) && !externalSchemes[scheme] {
			schemes = append(schemes, scheme)
		}
	}
	plan, err := planSchemeCommand(command, schemes)
	if err != nil {
		return true, nil, "", err
	}
	if plan.standalone != nil {
		result, err := a.runStandaloneScheme(ctx, work, s, *plan.standalone)
		return true, result, "", err
	}
	if len(plan.reads) == 0 {
		return false, nil, command, nil
	}
	rewritten, err := s.materializeSchemeWords(work, command, plan.reads)
	if err != nil {
		return true, nil, "", err
	}
	return false, nil, rewritten, nil
}

const maxMaterializedBytes = 50 << 10

// materializeSchemeWords replaces each scheme word the router can read as a file with the
// path of a read-only copy. A word that names nothing readable stays as written.
func (s *session) materializeSchemeWords(ctx context.Context, command string, words []schemeWord) (string, error) {
	for i := len(words) - 1; i >= 0; i-- {
		word := words[i]
		target, err := resource.ParsePath("/", word.value, "")
		if err != nil {
			continue
		}
		provider, err := s.router.Provider(target)
		if err != nil {
			continue
		}
		stat, err := provider.Stat(ctx, target.URI)
		if err != nil {
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			continue
		}
		if stat.Kind == resource.EntryDir {
			return "", fmt.Errorf("%s is a directory and cannot be passed to a command; list it with a standalone `ls %s`", word.value, word.value)
		}
		target.Selection.Raw = true
		page, err := s.router.Read(ctx, target, resource.Budget{Bytes: maxMaterializedBytes, Lines: 2000})
		if errors.Is(err, resource.ErrNotFound) {
			continue
		}
		if err != nil {
			return "", err
		}
		if complete := page.Facts.ResourceComplete || target.Selection.Explicit && page.Facts.SelectionComplete; !complete {
			return "", fmt.Errorf("%s is too large to pass to a command (limit %d KiB); read it with the read tool or add a line selector", word.value, maxMaterializedBytes>>10)
		}
		path, err := s.writeMaterialized(target.URI, page.Content)
		if err != nil {
			return "", err
		}
		command = command[:word.start] + shellQuote(path) + command[word.end:]
	}
	return command, nil
}

// writeMaterialized stores one copy beside the session scratch; shutdown removes the tree.
func (s *session) writeMaterialized(uri, content string) (string, error) {
	if s.scratch == "" {
		return "", errors.New("scheme paths cannot be passed to commands: session scratch storage is unavailable")
	}
	directory := filepath.Join(materializedRoot(s.scratch), strconv.FormatUint(s.materialized.Add(1), 10))
	if err := os.MkdirAll(directory, 0o700); err != nil {
		return "", err
	}
	name := strings.TrimLeft(strings.Map(func(r rune) rune {
		if r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || r >= '0' && r <= '9' || r == '-' || r == '_' || r == '.' {
			return r
		}
		return '_'
	}, filepath.Base(strings.TrimRight(uri, "/"))), ".")
	if name == "" {
		name = "resource"
	}
	path := filepath.Join(directory, name)
	if err := os.WriteFile(path, []byte(content), 0o400); err != nil {
		return "", err
	}
	return path, nil
}

func materializedRoot(scratch string) string { return scratch + ".args" }

func shellQuote(value string) string { return "'" + strings.ReplaceAll(value, "'", `'\''`) + "'" }

// runStandaloneScheme runs one in-process verb through the router.
func (a *app) runStandaloneScheme(ctx sdk.Context, work context.Context, s *session, parsed schemeCommand) (any, error) {
	if parsed.verb == "rm" || parsed.verb == "mv" || parsed.verb == "cp" || parsed.verb == "mkdir" {
		target, err := resource.ParsePath("/", parsed.paths[0], "")
		if err != nil {
			return nil, err
		}
		_, release, err := s.location.Acquire(ctx.ToolCallID(), true)
		if err != nil {
			return nil, err
		}
		defer release()
		operation := map[string]string{"rm": "remove", "mv": "move", "cp": "copy", "mkdir": "mkdir"}[parsed.verb]
		destination := ""
		if len(parsed.paths) == 2 {
			destination = parsed.paths[1]
		}
		result, err := a.mutateProvider(work, s, target, operation, destination, nil, "")
		return result, err
	}
	if parsed.verb == "redirect" {
		target, err := resource.ParsePath("/", parsed.paths[0], "")
		if err != nil {
			return nil, err
		}
		if target.Selection.Explicit {
			return nil, errors.New("redirect takes a whole path, not a line selector")
		}
		_, release, err := s.location.Acquire(ctx.ToolCallID(), true)
		if err != nil {
			return nil, err
		}
		defer release()
		result, err := a.writeControl(ctx, work, s, target, parsed.text)
		return result, err
	}
	text, err := a.runSchemeCommand(work, s, parsed)
	if err != nil {
		return nil, err
	}
	return sdk.ToolResult{Content: text, Details: map[string]any{"schemeCommand": parsed.verb, "paths": parsed.paths}}, nil
}
