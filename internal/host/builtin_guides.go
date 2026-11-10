package host

// builtinGuides are the docs://workbench/schemes/<scheme> pages of Workbench's own schemes.
// Each page states only behavior the provider implements; update it with the provider.
var builtinGuides = map[string]string{
	"file": `# file://

Local files. A plain path is the same resource: relative paths resolve against the working location, ` + "`file:///abs`" + ` is always local.

- Tools: read, write, edit, grep, find, ls.
- Selectors on read: ` + "`:N`, `:A-B`, `:A+C`, `:-N`" + `, comma ranges, ` + "`:raw`" + `. A literal filename wins over a selector.
- Reading a directory returns a paged listing.
`,
	"http": `# http:// and https://

Web content, read only.

- ` + "`read https://host/path`" + ` returns readable Markdown; ` + "`:raw`" + ` keeps the decoded document.
- A registered fetch backend serves the URL when one matches, otherwise the builtin static reader does. Scripts never run.
- Refused: credentials in the URL, non-http(s) redirects, redirect cycles, binary media. Bodies are capped at 4 MiB.
- Shell commands (curl, git) receive these URLs unchanged.
`,
	"local": `# local://

Per-session scratch directory, private to this session and removed with it.

- Tools: read, write, grep, find, ls. ` + "`write local://<name>`" + ` creates or replaces a file (4 MiB maximum, existing directory).
- Standalone shell forms: ls, cat, rm, mkdir, mv, cp and ` + "`word > local://<name>`" + `.
`,
	"artifact": `# artifact://

The ` + "`.project/`" + ` tree of the current project, the same from every subdirectory and worktree.

- ` + "`ls artifact://specs/`" + ` shows each artifact's id, title, status and revision.
- ` + "`read artifact://<type>/<name>`" + ` reads one artifact. Paths cannot leave the tree.
- A working location without a ` + "`.project/`" + ` directory above it reports not found.
`,
	"cfg": `# cfg://

Workbench settings and status as JSON, read only.

- ` + "`cfg://`" + `: effective settings with their source layer and warnings; ` + "`cfg://<section>/<key>`" + ` reads one setting.
- ` + "`cfg://workbench/status`" + `: readiness, staleness and activity counts.
- ` + "`cfg://workbench/integrations/`" + `: one entry per extension that registered with Workbench.
- Change settings with ` + "`/workbench settings`" + ` or by editing ` + "`Workbench.toml`" + `.
`,
	"session": `# session://

Sessions as trees.

- ` + "`session://self/`" + ` is this session; ` + "`session://by-id/`" + ` lists stored sessions; ` + "`session://<id>/`" + ` addresses one.
- Files: ` + "`meta`, `transcript`, `entries`, `title`" + `; this session also has ` + "`system-prompt`" + ` and ` + "`messages`" + ` (inter-session messages, both directions).
- ` + "`session://self/frames/`" + ` holds frame receipts; ` + "`session://self/instructions/`" + ` the delivered instruction files.
- Writes: ` + "`title`" + ` sets the session title; ` + "`self/instructions/ctl`" + ` accepts ` + "`pin <path> [--class <id>]`" + ` and ` + "`unpin <path>`" + `.
`,
	"job": `# job://

Shell jobs of this session, backed by the same supervisor as the jobs tool.

- ` + "`ls job://`" + ` lists jobs; ` + "`job://<id>/`" + ` has ` + "`status`, `cmdline`, `cwd`, `log`, `stdout`, `stderr`, `stdin`, `ctl`" + `.
- Write ` + "`stdin`" + ` to send bytes. Write ` + "`ctl`" + ` with ` + "`eof`, `kill`" + ` or ` + "`signal <HUP|INT|QUIT|TERM|KILL>`" + `.
- Logs are paged by byte cursor; evicted bytes are reported, never invented.
`,
	"skill": `# skill://

Skills available to this session.

- ` + "`ls skill://`" + ` lists visible skills; ` + "`ls skill://loaded/`" + ` the loaded ones.
- ` + "`read skill://<name>`" + ` loads a skill: its body and the directory to resolve its relative paths against.
- ` + "`read skill://<name>:A-B`" + ` shows a line range of the body without loading the skill.
- ` + "`write skill://<name>/ctl`" + ` with ` + "`unload`" + ` removes it from the loaded set.
`,
	"agent": `# agent://

Live sessions of this user on this machine.

- ` + "`ls agent://`" + ` lists reachable sessions with title, project, cwd, state and idle time, model and trusted/blocked; ` + "`agent://self`" + ` is this one.
- ` + "`agent://<id>/status`" + ` and ` + "`agent://<id>/messages`" + ` are readable; ` + "`agent://self/inbox`" + ` shows received messages.
- ` + "`write agent://<id>/inbox`" + ` sends a message. It can wake an idle session, so send only what the receiver should act on.
`,
	"docs": `# docs://

Documentation registered with Workbench, read only.

- ` + "`ls docs://`" + ` lists owners; ` + "`docs://<owner>/`" + ` is that extension's registered tree.
- ` + "`docs://workbench/schemes/<scheme>`" + ` is the guide of one scheme, builtin or contributed.
`,
}

// guideFor returns a scheme's guide page: Workbench's own, else the contributed provider's.
func (s *session) guideFor(scheme string) (string, bool) {
	if scheme == "https" {
		scheme = "http"
	}
	if text, ok := builtinGuides[scheme]; ok {
		return text, true
	}
	return s.schemeGuide(scheme)
}
