package host

import (
	"errors"
	"testing"
)

func TestSchemePlanRunsStandaloneVerbsInProcess(t *testing.T) {
	schemes := []string{"job", "secret"}
	for _, command := range []string{`cat "job://x"`, `cat 'job://x'`, `cat 'jo'b://x`, `cat jo\b://x`} {
		plan, err := planSchemeCommand(command, schemes)
		if err != nil || plan.standalone == nil || plan.standalone.verb != "cat" || plan.standalone.paths[0] != "job://x" {
			t.Errorf("%q: plan=%+v err=%v", command, plan, err)
		}
	}
	plan, err := planSchemeCommand("kill > job://abc/ctl", schemes)
	if err != nil || plan.standalone == nil || plan.standalone.verb != "redirect" || plan.standalone.text != "kill" {
		t.Fatalf("redirect: %+v %v", plan, err)
	}
	plan, err = planSchemeCommand("cp secret://a secret://b", schemes)
	if err != nil || plan.standalone == nil || plan.standalone.verb != "cp" {
		t.Fatalf("cp: %+v %v", plan, err)
	}
}

func TestSchemePlanIgnoresSchemeTextThatIsNotAPathArgument(t *testing.T) {
	schemes := []string{"job", "agent", "docs"}
	for _, command := range []string{
		"ls /tmp",
		"curl -sI https://example.com/a",
		"git clone https://example.com/repo.git",
		`grep -rn "agent://" internal`,
		`git log --grep='job://'`,
		`echo "see agent://peer/inbox for details"`,
		"sed -e 's#x://#y://#' file",
		"cat <<'EOF'\nagent://peer/inbox\ndocs://workbench/tools.md\nEOF",
		"cat <<< docs://workbench/tools.md",
		"tool --from=docs://workbench/tools.md",
		`cat "jo$(printf b)://x"`,
		"cat docs://workbench/*",
		"URL=docs://workbench/tools.md env",
		"cat 'unterminated docs://x",
		"echo docs://workbench/tools.md",
		"printf '%s\\n' docs://workbench/tools.md",
		"git commit -m docs://workbench/tools.md",
		"sed docs://a file",
	} {
		plan, err := planSchemeCommand(command, schemes)
		if err != nil || plan.standalone != nil || len(plan.reads) != 0 {
			t.Errorf("%q: plan=%+v err=%v", command, plan, err)
		}
	}
}

func TestSchemePlanCollectsReadArgumentsOfComposedCommands(t *testing.T) {
	schemes := []string{"docs", "job"}
	cases := map[string][]string{
		"cat docs://workbench/tools.md | wc -l":           {"docs://workbench/tools.md"},
		"diff docs://a 'docs://b'":                        {"docs://a", "docs://b"},
		"wc -l < docs://a":                                {"docs://a"},
		"ls -la docs://workbench/":                        {"docs://workbench/"},
		"cp docs://a ./local.md":                          {"docs://a"},
		"cat $(printf x) docs://a":                        {"docs://a"},
		"grep -n docs://pattern docs://a":                 {"docs://a"},
		"rg --json 'docs://pattern' docs://a docs://b":    {"docs://a", "docs://b"},
		"if true; then head -n 3 job://1/log; fi && true": {"job://1/log"},
	}
	for command, want := range cases {
		plan, err := planSchemeCommand(command, schemes)
		if err != nil || plan.standalone != nil || len(plan.reads) != len(want) {
			t.Errorf("%q: plan=%+v err=%v", command, plan, err)
			continue
		}
		for i, word := range plan.reads {
			if word.value != want[i] || word.start >= word.end || word.end > len(command) {
				t.Errorf("%q: read %d = %+v, want %q", command, i, word, want[i])
			}
		}
	}
}

func TestSchemePlanRefusesWritesOutsideStandaloneForms(t *testing.T) {
	schemes := []string{"job", "secret", "docs"}
	for _, command := range []string{
		"rm job://a && true",
		"echo hi > job://a; ls",
		"echo hi >> job://a",
		"cat x | tee secret://a",
		"cp ./local secret://a",
		"mv secret://a ./local",
		"mkdir -p docs://x",
		"touch docs://x",
	} {
		if _, err := planSchemeCommand(command, schemes); err == nil {
			t.Errorf("%q was accepted", command)
		} else if command != "mv secret://a ./local" && !errors.Is(err, errSchemeWrite) {
			t.Errorf("%q: unexpected error %v", command, err)
		}
	}
}
