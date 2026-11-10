package host

import (
	"os"
	"path/filepath"
	"sort"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/jobs"
)

// workbenchVerbs are the /workbench families in the order the usage line names them.
var workbenchVerbs = []string{"show", "settings", "tools", "cd", "jobs", "skills", "prompt", "codemode", "frames", "instructions", "ask", "projects", "sessions", "integrations", "resources", "trust"}

// verbAliases maps each accepted spelling to the one the handlers switch on.
var verbAliases = map[string]string{"list": "ls", "remove": "rm", "config": "settings"}

// subverbFamilies take a verb as their first argument; the others take a path, a name or a key there.
var subverbFamilies = map[string]bool{"jobs": true, "skills": true, "frames": true, "instructions": true, "projects": true, "sessions": true, "trust": true}

func canonicalVerb(word string) string {
	if canonical, ok := verbAliases[word]; ok {
		return canonical
	}
	return word
}

// canonicalSubverb rewrites only the first word, so a path or text argument is never touched.
func canonicalSubverb(args string) string {
	trimmed := strings.TrimLeft(args, " ")
	first, rest, found := strings.Cut(trimmed, " ")
	if canonical := canonicalVerb(first); canonical != first {
		if found {
			return canonical + " " + rest
		}
		return canonical
	}
	return args
}

// unknownVerb is the one-line answer to a verb no family has.
func unknownVerb(verb string) string {
	best, distance := "", 3
	for _, candidate := range workbenchVerbs {
		if strings.HasPrefix(candidate, verb) {
			best, distance = candidate, 0
			break
		}
		if d := editDistance(verb, candidate); d < distance {
			best, distance = candidate, d
		}
	}
	if best == "" {
		return "unknown /workbench verb " + strconvQuote(verb) + "; run /workbench for the list"
	}
	return "unknown /workbench verb " + strconvQuote(verb) + "; did you mean " + best + "?"
}

func strconvQuote(text string) string {
	return `"` + noticeText(oneLine(text, 24), 0) + `"`
}

func editDistance(a, b string) int {
	previous := make([]int, len(b)+1)
	for j := range previous {
		previous[j] = j
	}
	for i := 1; i <= len(a); i++ {
		current := make([]int, len(b)+1)
		current[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			current[j] = min(previous[j]+1, current[j-1]+1, previous[j-1]+cost)
		}
		previous = current
	}
	return previous[len(b)]
}

func completionItems(prefix string, candidates []string) []sdk.AutocompleteItem {
	var items []sdk.AutocompleteItem
	for _, candidate := range candidates {
		if strings.HasPrefix(candidate, prefix) && len(items) < 50 {
			items = append(items, sdk.AutocompleteItem{Value: candidate, Label: candidate})
		}
	}
	return items
}

// cdCompletions offers --reset and the directories below the working location that continue the prefix.
func (a *app) cdCompletions(prefix string) ([]sdk.AutocompleteItem, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	candidates := []string{"--reset"}
	dir, partial := filepath.Split(prefix)
	base := dir
	if !filepath.IsAbs(base) {
		base = filepath.Join(s.location.Current().Cwd, dir)
	}
	// One directory read; a completion must never walk a tree.
	entries, _ := os.ReadDir(base)
	for _, entry := range entries {
		if entry.IsDir() && strings.HasPrefix(entry.Name(), partial) && (partial != "" || !strings.HasPrefix(entry.Name(), ".")) {
			candidates = append(candidates, dir+entry.Name()+"/")
		}
	}
	return completionItems(prefix, candidates), nil
}

// toolsCompletions offers the tool names the last tools view or session start saw.
func (a *app) toolsCompletions(prefix string) ([]sdk.AutocompleteItem, error) {
	names, _ := a.toolNames.Load().([]string)
	return completionItems(prefix, names), nil
}

func (a *app) rememberTools(tools []sdk.ToolInfo) {
	names := make([]string, len(tools))
	for i, tool := range tools {
		names[i] = tool.Name
	}
	sort.Strings(names)
	a.toolNames.Store(names)
}

// jobsCompletions offers ls and, per retained job, its short id with each action.
func (a *app) jobsCompletions(prefix string) ([]sdk.AutocompleteItem, error) {
	candidates := []string{"ls"}
	if s, err := a.current(); err == nil {
		for _, row := range s.jobRows() {
			id := jobs.ShortID(row.id)
			candidates = append(candidates, id)
			for _, action := range []string{"status", "output", "kill", "eof"} {
				candidates = append(candidates, id+" "+action)
			}
		}
	}
	return completionItems(prefix, candidates), nil
}
