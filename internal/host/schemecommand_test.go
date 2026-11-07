package host

import "testing"

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
	for _, command := range []string{"ls job:// | head", "cat $(echo job://x)", "rm job://a && true", "echo hi > job://a; ls", `cat "job://x"`, `cat 'job://x'`, `kill > job://abc/ctl;true`} {
		if _, _, err := parseSchemeCommand(command, schemes); err == nil {
			t.Fatalf("%q was accepted", command)
		}
	}
	if _, ok, err := parseSchemeCommand("ls /tmp", schemes); ok || err != nil {
		t.Fatalf("local command must pass through, ok=%v err=%v", ok, err)
	}
}
