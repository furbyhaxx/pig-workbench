Isolated PIG_HOME (/tmp/wb-pr/home) with models.json: provider mock -> 127.0.0.1:18931, models mock-1 (no compat) and
mock-mid (reasoning, compat.supportsMidConvoSystemMessages = true). Work dir with AGENTS.md and an empty git repo.
Extension copies: probe-prompt-a (as committed) and probe-prompt-b (sed: role "A" -> "B", module name). Env PROBE_A /
PROBE_B: comma list of force, forceopt, sections, sections-stable, steer. MODEL selects the model; PIGARGS adds flags.
s_main.json: expand {{FILLER}} to " filler" x 40000 so manual compaction has something to cut.
Run: EXTS=<ext dirs> PROBE_A=... ./go.sh NAME scenarios/<script>.json scenarios/<steps>.json ; python3 show.py NAME
Print mode and TUI runs were started by hand (pig -p ..., tmux); see runs/print.txt, runs/tui*.txt.
PR7: PROBE_A=force,toolframe with s_pr7/st_pr7. PR8: home/agent/mcp.json server "echo" (python3 mcp_echo.py, exposure
deferred, description "Echo test server."), MCP_LOG=/tmp/wb-pr/mcp.log; PROBE_A=sections | force | force,bprsub with s_two/st_two.
