package host

import "testing"

func TestSchemeCommandParsesQuotedAndEscapedStaticPaths(t *testing.T) {
	for _, command := range []string{`cat "job://x"`, `cat 'job://x'`, `cat 'jo'b://x`, `cat jo\b://x`} {
		parsed, handled, err := parseSchemeCommand(command, []string{"job"})
		if err != nil || !handled || parsed.verb != "cat" || len(parsed.paths) != 1 || parsed.paths[0] != "job://x" {
			t.Errorf("%q: parsed=%+v handled=%v err=%v", command, parsed, handled, err)
		}
	}
	for _, command := range []string{`cat 'jo'b://x | head`, `cat $(echo 'jo'b://x)`, `cat "jo$(printf b)://x"`, `echo "hi" > 'jo'b://x; touch bad`} {
		if _, _, err := parseSchemeCommand(command, []string{"job"}); err == nil {
			t.Errorf("%q was not refused", command)
		}
	}
	if _, _, err := parseSchemeCommand(`echo 'job://x'`, []string{"job"}); err == nil {
		t.Fatal("unsupported scheme command was not refused")
	}
}

func TestSchemeCommandAcceptsStandaloneVerbsOnly(t *testing.T) {
	schemes := []string{"job", "secret"}
	parsed, ok, err := parseSchemeCommand("ls job://", schemes)
	if err != nil || !ok || parsed.verb != "ls" {
		t.Fatalf("ls: %+v %v %v", parsed, ok, err)
	}
	parsed, ok, err = parseSchemeCommand("kill > job://abc/ctl", schemes)
	if err != nil || !ok || parsed.verb != "redirect" || parsed.text != "kill" {
		t.Fatalf("redirect: %+v %v %v", parsed, ok, err)
	}
	for _, command := range []string{"ls job:// | head", "cat $(echo job://x)", "rm job://a && true", "echo hi > job://a; ls", `kill > job://abc/ctl;true`} {
		if _, _, err := parseSchemeCommand(command, schemes); err == nil {
			t.Fatalf("%q was accepted", command)
		}
	}
	if _, ok, err := parseSchemeCommand("ls /tmp", schemes); ok || err != nil {
		t.Fatalf("local command must pass through, ok=%v err=%v", ok, err)
	}
}
