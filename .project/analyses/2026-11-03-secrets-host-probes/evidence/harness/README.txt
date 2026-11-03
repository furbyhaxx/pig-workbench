Probe value: SECRET = "ghp_" + "S3cr3tPr0beValue0123456789abcdefABCD" (fake, assembled so the literal never sits in git).
Placeholder: red-sec-probeprobeprobep. sha256("<SECRET>\n") starts 1af7534e: its presence in output proves the tool ran with the real value.
Scenario files use {{SECRET}} and {{FILLER}} (" filler" x 40000); expand before running.
Setup: copy a PiG home with models.json (provider mock -> 127.0.0.1:18931), settings.json defaultTools ["+codemode"],
auth.json {"mock"|"probext": {"type":"api_key","key":"!printf <SECRET>"}} (0600), mcp.json server echo -> mcp_echo.py (exposure direct).
Work dir: AGENTS.md and secret.txt containing <SECRET>.
Run: ./go.sh <name> <hooks> [ENV=...]; hooks: input,tool_call,tool_result,user_bash,context,context_with_system,bpr ; PROBE_BLOCK=1|abort|panic|invalid ; PROBE_SKIP_CODEMODE=1.
Tree runs: run.py tree scenarios/s_tree.json scenarios/st_tree.json with PROBE_HOOKS=none|input.
TUI (S5): tmux, pig interactive with PROBE_HOOKS=input; typed and bracketed-paste prompts; see runs/tui.txt.
