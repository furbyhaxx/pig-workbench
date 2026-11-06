package host

import (
	"context"
	"errors"
	"fmt"
	"strings"

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
	if strings.ContainsAny(command, "|;&$`<>()\n") && !strings.Contains(command, " > ") && !strings.HasPrefix(strings.TrimSpace(command), ">") {
		if hasSchemeArg(command, schemes) {
			return schemeCommand{}, false, fmt.Errorf("scheme paths only work as a standalone ls, cat, rm, mv, cp, mkdir or > redirect; pipes, compounds and substitutions are refused")
		}
		return schemeCommand{}, false, nil
	}
	fields := strings.Fields(command)
	if len(fields) == 0 {
		return schemeCommand{}, false, nil
	}
	verb := fields[0]
	switch verb {
	case "ls", "cat", "rm", "mkdir":
		if len(fields) != 2 || !schemePath(fields[1], schemes) {
			return refuseOther(command, schemes)
		}
		return schemeCommand{verb: verb, paths: fields[1:2]}, true, nil
	case "mv", "cp":
		if len(fields) != 3 || !schemePath(fields[1], schemes) && !schemePath(fields[2], schemes) {
			return refuseOther(command, schemes)
		}
		if !schemePath(fields[1], schemes) || !schemePath(fields[2], schemes) {
			return schemeCommand{}, false, fmt.Errorf("%s between a scheme path and a local path is unsupported; both ends must be scheme paths", verb)
		}
		return schemeCommand{verb: verb, paths: fields[1:3]}, true, nil
	default:
		if len(fields) == 3 && fields[1] == ">" && schemePath(fields[2], schemes) {
			return schemeCommand{verb: "redirect", paths: fields[2:3], text: fields[0]}, true, nil
		}
		return refuseOther(command, schemes)
	}
}

func refuseOther(command string, schemes []string) (schemeCommand, bool, error) {
	if hasSchemeArg(command, schemes) {
		return schemeCommand{}, false, fmt.Errorf("scheme paths only work as a standalone ls, cat, rm, mv, cp, mkdir or > redirect")
	}
	return schemeCommand{}, false, nil
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
	for _, field := range strings.Fields(command) {
		if schemePath(field, schemes) {
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
	case "mkdir", "rm", "mv", "cp":
		return "", fmt.Errorf("%s is not supported by this scheme yet", cmd.verb)
	case "redirect":
		target, err := resource.ParsePath("/", cmd.paths[0], "")
		if err != nil {
			return "", err
		}
		if target.Scheme != "job" {
			return "", fmt.Errorf("redirect to %s:// is not a supported form", target.Scheme)
		}
		if err := (jobProvider{}).jobAction(s, target.URI, cmd.text); err != nil {
			return "", err
		}
		return "", nil
	default:
		return "", errors.New("unsupported scheme command")
	}
}

// dispatchSchemeCommand runs a G8.20 command in-process when it targets a registered
// non-locatable scheme, and reports whether it handled the command.
func (a *app) dispatchSchemeCommand(ctx context.Context, s *session, command string) (bool, any, error) {
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
	text, err := a.runSchemeCommand(ctx, s, parsed)
	if err != nil {
		return true, nil, err
	}
	return true, sdk.ToolResult{Content: text, Details: map[string]any{"schemeCommand": parsed.verb, "paths": parsed.paths}}, nil
}
