package host

import (
	"context"
	"errors"
	"fmt"
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

// parseSchemeCommand accepts a standalone ls, cat, rm, mv, cp, mkdir or a single redirect.
// Anything with a pipe, substitution, compound operator or extra word is refused by name.
func parseSchemeCommand(command string, schemes []string) (schemeCommand, bool, error) {
	file, err := syntax.NewParser(syntax.Variant(syntax.LangBash)).Parse(strings.NewReader(command), "")
	if err != nil {
		if hasSchemeArg(command, schemes) {
			return schemeCommand{}, false, errors.New("invalid shell syntax involving a scheme path")
		}
		return schemeCommand{}, false, nil
	}
	containsScheme := hasSchemeArg(command, schemes)
	syntax.Walk(file, func(node syntax.Node) bool {
		if word, ok := node.(*syntax.Word); ok {
			if value, ok := staticShellWord(command, word); ok {
				containsScheme = containsScheme || hasSchemeArg(value, schemes)
			} else {
				start, end := int(word.Pos().Offset()), int(word.End().Offset())
				if start >= 0 && end <= len(command) && start <= end && strings.Contains(command[start:end], "://") {
					containsScheme = true
				}
			}
		}
		return true
	})
	if !containsScheme {
		return schemeCommand{}, false, nil
	}
	refused := errors.New("scheme paths require a standalone ls, cat, rm, mv, cp, mkdir or > redirect; expansions and shell composition are refused")
	if len(file.Stmts) != 1 {
		return schemeCommand{}, false, refused
	}
	stmt := file.Stmts[0]
	call, ok := stmt.Cmd.(*syntax.CallExpr)
	if !ok || stmt.Background || stmt.Negated || stmt.Coprocess || stmt.Disown || len(call.Assigns) != 0 {
		return schemeCommand{}, false, refused
	}
	fields := make([]string, 0, len(call.Args))
	for _, word := range call.Args {
		value, ok := staticShellWord(command, word)
		if !ok {
			return schemeCommand{}, false, refused
		}
		fields = append(fields, value)
	}
	if len(fields) == 0 {
		return schemeCommand{}, false, refused
	}
	if len(stmt.Redirs) > 0 {
		if len(stmt.Redirs) != 1 || stmt.Redirs[0].Op != syntax.RdrOut || stmt.Redirs[0].N != nil || len(fields) != 1 {
			return schemeCommand{}, false, refused
		}
		target, ok := staticShellWord(command, stmt.Redirs[0].Word)
		if !ok || !schemePath(target, schemes) {
			return schemeCommand{}, false, refused
		}
		return schemeCommand{verb: "redirect", paths: []string{target}, text: fields[0]}, true, nil
	}
	verb := fields[0]
	switch verb {
	case "ls", "cat", "rm", "mkdir":
		if len(fields) != 2 || !schemePath(fields[1], schemes) {
			return schemeCommand{}, false, refused
		}
		return schemeCommand{verb: verb, paths: fields[1:2]}, true, nil
	case "mv", "cp":
		if len(fields) != 3 || !schemePath(fields[1], schemes) && !schemePath(fields[2], schemes) {
			return schemeCommand{}, false, refused
		}
		if !schemePath(fields[1], schemes) || !schemePath(fields[2], schemes) {
			return schemeCommand{}, false, fmt.Errorf("%s between a scheme path and a local path is unsupported; both ends must be scheme paths", verb)
		}
		return schemeCommand{verb: verb, paths: fields[1:3]}, true, nil
	default:
		return schemeCommand{}, false, refused
	}
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

func hasSchemeArg(command string, schemes []string) bool {
	for _, scheme := range schemes {
		if strings.Contains(command, scheme+"://") {
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
		provider, err := s.router.Provider(target)
		if err != nil {
			return "", err
		}
		page, err := provider.List(ctx, target.URI, "", 200)
		if err != nil {
			return "", err
		}
		var b strings.Builder
		for _, entry := range page.Entries {
			_, _ = fmt.Fprintf(&b, "%s\t%s\n", entry.Kind, entry.Name)
		}
		if !page.Complete {
			b.WriteString("… listing truncated\n")
		}
		return b.String(), nil
	case "cat":
		target, err := resource.ParsePath("/", cmd.paths[0], "")
		if err != nil {
			return "", err
		}
		page, err := s.router.Read(ctx, target, resource.Budget{Bytes: 1 << 20, Lines: 100000})
		if err != nil {
			return "", err
		}
		return page.Content, nil
	default:
		return "", errors.New("unsupported scheme command")
	}
}

// dispatchSchemeCommand runs a G8.20 command in-process when it targets a registered
// non-locatable scheme, and reports whether it handled the command.
func (a *app) dispatchSchemeCommand(ctx sdk.Context, work context.Context, s *session, command string) (bool, any, error) {
	schemes := []string{}
	for _, scheme := range s.router.Schemes() {
		if !s.router.Locatable(scheme) {
			schemes = append(schemes, scheme)
		}
	}
	parsed, handled, err := parseSchemeCommand(command, schemes)
	if err != nil {
		return true, nil, err
	}
	if !handled {
		return false, nil, nil
	}
	if parsed.verb == "rm" || parsed.verb == "mv" || parsed.verb == "cp" || parsed.verb == "mkdir" {
		target, err := resource.ParsePath("/", parsed.paths[0], "")
		if err != nil {
			return true, nil, err
		}
		_, release, err := s.location.Acquire(ctx.ToolCallID(), true)
		if err != nil {
			return true, nil, err
		}
		defer release()
		operation := map[string]string{"rm": "remove", "mv": "move", "cp": "copy", "mkdir": "mkdir"}[parsed.verb]
		destination := ""
		if len(parsed.paths) == 2 {
			destination = parsed.paths[1]
		}
		result, err := a.mutateProvider(work, s, target, operation, destination, nil, "")
		return true, result, err
	}
	if parsed.verb == "redirect" {
		target, err := resource.ParsePath("/", parsed.paths[0], "")
		if err != nil {
			return true, nil, err
		}
		if target.Selection.Explicit {
			return true, nil, errors.New("redirect takes a whole path, not a line selector")
		}
		_, release, err := s.location.Acquire(ctx.ToolCallID(), true)
		if err != nil {
			return true, nil, err
		}
		defer release()
		result, err := a.writeControl(ctx, work, s, target, parsed.text)
		return true, result, err
	}
	text, err := a.runSchemeCommand(work, s, parsed)
	if err != nil {
		return true, nil, err
	}
	return true, sdk.ToolResult{Content: text, Details: map[string]any{"schemeCommand": parsed.verb, "paths": parsed.paths}}, nil
}
