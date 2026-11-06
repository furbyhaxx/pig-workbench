package host

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

const maxTitleRunes = 200

// customSessionDir is the directory PiG was told to use with --session-dir; nil means the default per-project buckets.
func customSessionDir(host sdk.Context) *string {
	manager := host.SessionManager()
	if usesDefault, err := manager.UsesDefaultSessionDir(); err != nil || usesDefault {
		return nil
	}
	dir, err := manager.GetSessionDir()
	if err != nil {
		return nil
	}
	return &dir
}

// entryText flattens the text of a session entry's message content.
func entryText(message map[string]any) string {
	switch content := message["content"].(type) {
	case string:
		return content
	case []any:
		var parts []string
		for _, block := range content {
			if m, ok := block.(map[string]any); ok && m["type"] == "text" {
				if text, ok := m["text"].(string); ok {
					parts = append(parts, text)
				}
			}
		}
		return strings.Join(parts, "\n")
	}
	return ""
}

// renderStored renders one file of a session nobody is running here, read through PiG's session manager.
func renderStored(ctx context.Context, host sdk.Context, info sdk.SessionInfo, name string) ([]byte, error) {
	if name == "title" {
		if info.Name == nil {
			return []byte("\n"), nil
		}
		return []byte(*info.Name + "\n"), nil
	}
	if name == "meta" {
		meta := map[string]any{"sessionId": info.ID, "name": info.Name, "cwd": info.Cwd, "messages": info.MessageCount, "created": info.Created, "modified": info.Modified, "file": info.Path}
		return json.MarshalIndent(meta, "", "  ")
	}
	manager, err := sdk.SessionManagerOpen(host, info.Path, nil, nil)
	if err != nil {
		return nil, fmt.Errorf("open session %s: %w", info.ID, err)
	}
	switch name {
	case "entries":
		entries, err := manager.GetEntries()
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, entry := range entries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			line, _ := json.Marshal(entry)
			out.Write(line)
			out.WriteByte('\n')
		}
		return []byte(out.String()), nil
	case "transcript":
		leaf, err := manager.GetLeafID()
		if err != nil {
			return nil, err
		}
		branch, err := manager.GetBranch(leaf)
		if err != nil {
			return nil, err
		}
		var out strings.Builder
		for _, entry := range branch {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			message, ok := entry["message"].(map[string]any)
			if !ok {
				continue
			}
			role, _ := message["role"].(string)
			if role == "toolResult" {
				role = "tool " + fmt.Sprint(message["toolName"])
			}
			if text := strings.TrimRight(entryText(message), "\n"); text != "" {
				fmt.Fprintf(&out, "## %s\n\n%s\n\n", role, text)
			}
		}
		return []byte(out.String()), nil
	}
	return nil, resource.ErrNotFound
}

// cleanTitle makes a title one line without control characters, at most 200 characters.
func cleanTitle(value string) string {
	var out []rune
	for _, r := range strings.TrimSpace(value) {
		if unicode.IsControl(r) {
			r = ' '
		}
		out = append(out, r)
	}
	if len(out) > maxTitleRunes {
		out = out[:maxTitleRunes]
	}
	return strings.TrimSpace(string(out))
}

// writeSessionTitle sets a title: the own session directly, a stored one after user confirmation.
func (a *app) writeSessionTitle(ctx sdk.Context, s *session, uri, content string) (string, error) {
	parts, err := splitSession(uri)
	if err != nil {
		return "", err
	}
	t, err := parseSessionTarget(parts)
	if err != nil || t.depth != 2 || t.file != "title" {
		return "", errors.New("session:// accepts writes only at <id>/title and session://self/instructions/ctl")
	}
	title := cleanTitle(content)
	provider := sessionProvider{ctx: func() (sdk.Context, bool) { return ctx, true }}
	if provider.isSelf(ctx, t.id) {
		if err := ctx.SetSessionName(title); err != nil {
			return "", err
		}
		return "title set", nil
	}
	info, err := provider.resolveStored(ctx, t.id)
	if err != nil {
		return "", err
	}
	if s.endpoint != nil {
		listed, err := s.endpoint.registry.List(context.Background())
		if err != nil {
			return "", errors.New("the live-session registry is unreadable, so the stored session was not changed")
		}
		stale := map[string]bool{}
		for _, id := range listed.Stale {
			stale[id] = true
		}
		for _, record := range listed.Records {
			if record.SessionID == info.ID && !stale[info.ID] {
				return "", fmt.Errorf("sessionLive: session %s is running; its title can only be changed by that session", shortID(info.ID))
			}
		}
	}
	if !ctx.HasUI() {
		return "", errors.New("changing a stored session's title needs user confirmation and no UI is available; nothing was changed")
	}
	answer, err := ctx.Confirm("Change session title", fmt.Sprintf("Set the title of stored session %s to %q?", shortID(info.ID), title))
	if err != nil {
		return "", err
	}
	if confirmed, _ := answer.(bool); !confirmed {
		return "", errors.New("the user declined; the title was not changed")
	}
	manager, err := sdk.SessionManagerOpen(ctx, info.Path, nil, nil)
	if err != nil {
		return "", fmt.Errorf("open session %s: %w", info.ID, err)
	}
	if _, err := manager.AppendSessionInfo(title); err != nil {
		return "", err
	}
	return "title set", nil
}
