package host

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strconv"
	"strings"
	"unicode"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/MichaelKinsy/PiG/extensions/sdk/tui"
	"github.com/creachadair/tomledit/parser"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

type displayFieldView struct {
	Definition config.Definition `json:"definition"`
	Effective  config.Entry      `json:"effective"`
}
type displayGroupView struct {
	Owner              string             `json:"owner"`
	Key                string             `json:"key"`
	RegistrationID     string             `json:"registrationId,omitempty"`
	Active             bool               `json:"active"`
	AppliesAfterReload bool               `json:"appliesAfterReload"`
	Fields             []displayFieldView `json:"fields"`
}

func commandIdentifier(value string) string {
	if strings.ContainsAny(value, "\"'\\") || strings.IndexFunc(value, unicode.IsSpace) >= 0 {
		return strconv.Quote(value)
	}
	return value
}

func commandFields(input string, count int) ([]string, string, error) {
	var fields []string
	for len(fields) < count {
		input = strings.TrimSpace(input)
		if input == "" {
			break
		}
		token := ""
		switch input[0] {
		case '"':
			prefix, err := strconv.QuotedPrefix(input)
			if err != nil {
				return nil, "", errors.New("invalid quoted command identifier")
			}
			token, err = strconv.Unquote(prefix)
			if err != nil {
				return nil, "", errors.New("invalid quoted command identifier")
			}
			input = input[len(prefix):]
		case '\'':
			end := strings.IndexByte(input[1:], '\'')
			if end < 0 {
				return nil, "", errors.New("unclosed command identifier")
			}
			token = input[1 : end+1]
			input = input[end+2:]
		default:
			end := strings.IndexFunc(input, unicode.IsSpace)
			if end < 0 {
				end = len(input)
			}
			token, input = input[:end], input[end:]
		}
		if input != "" && !unicode.IsSpace([]rune(input)[0]) {
			return nil, "", errors.New("command identifiers must be separated by whitespace")
		}
		fields = append(fields, token)
	}
	return fields, strings.TrimSpace(input), nil
}

func (s *session) displayGroups(snapshot *config.Snapshot, owner, key string) []displayGroupView {
	groups := map[string]*displayGroupView{}
	for _, definition := range snapshot.Catalog() {
		parts, err := parser.ParseKey(definition.Key)
		if err != nil || len(parts) != 4 || parts[0] != "tools" || owner != "" && owner != parts[1] || key != "" && key != parts[2] {
			continue
		}
		identity := parser.Key(parts[:3]).String()
		group := groups[identity]
		if group == nil {
			group = &displayGroupView{Owner: parts[1], Key: parts[2]}
			groups[identity] = group
		}
		effective, _ := snapshot.Get(definition.Key)
		group.Fields = append(group.Fields, displayFieldView{Definition: definition, Effective: effective})
	}
	for _, entry := range s.presentations.list() {
		if group := groups[parser.Key(displayNamespace(entry.owner, entry.key)).String()]; group != nil {
			group.Active = true
			group.RegistrationID = entry.registrationID
			group.AppliesAfterReload = s.presentations.appliesAfterReload(entry.registrationID)
		}
	}
	result := make([]displayGroupView, 0, len(groups))
	for _, group := range groups {
		result = append(result, *group)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Owner != result[j].Owner {
			return result[i].Owner < result[j].Owner
		}
		return result[i].Key < result[j].Key
	})
	return result
}

func displayGroupLines(groups []displayGroupView, width int) []string {
	lines := []string{"Tool display preferences"}
	if len(groups) == 0 {
		return append(lines, "No registered or dormant display schemas.")
	}
	for _, group := range groups {
		pending := false
		for _, field := range group.Fields {
			parts, _ := parser.ParseKey(field.Definition.Key)
			name := noticeText(parts[3], 0)
			encoded, _ := json.Marshal(field.Effective.Value)
			value := string(encoded)
			scope := "[" + string(field.Effective.Scope) + "]"
			if width > 0 {
				name = tui.TruncateToWidth(name, max(8, width/2), "…", false)
				value = tui.TruncateToWidth(value, max(1, width-tui.VisibleWidth(name)-tui.VisibleWidth(scope)-2), "…", false)
			}
			lines = append(lines, noticeText(name+" "+value+" "+scope, width))
			pending = pending || field.Effective.Application != "live"
		}
		if pending {
			lines = append(lines, "Some controls not applied.")
		}
		if group.AppliesAfterReload {
			lines = append(lines, "Renderer claim appliesAfterReload.")
		}
		identity := group.Owner + " / " + group.Key
		if !group.Active {
			identity += " (dormant)"
		}
		lines = append(lines, noticeText(identity, width))
	}
	return lines
}

func (a *app) displaySettingsCommand(ctx sdk.Context, work context.Context, s *session, args string) error {
	fields, tail, err := commandFields(args, 1)
	if err != nil {
		return err
	}
	verb := "show"
	if len(fields) > 0 && (fields[0] == "set" || fields[0] == "reset" || fields[0] == "show") {
		verb = fields[0]
		args = tail
	}
	if verb == "show" {
		identifiers, rest, err := commandFields(args, 2)
		if err != nil {
			return err
		}
		if rest != "" {
			return errors.New("usage: /workbench settings tools [show] [owner [key]]")
		}
		owner, key := "", ""
		if len(identifiers) > 0 {
			owner = identifiers[0]
		}
		if len(identifiers) > 1 {
			key = identifiers[1]
		}
		snapshot, status := s.settings.Inspect()
		if snapshot == nil {
			return errors.New("no valid settings snapshot is available")
		}
		groups := s.displayGroups(snapshot, owner, key)
		if len(groups) == 0 && owner != "" {
			return errors.New("no registered or dormant display schema matches that owner/key")
		}
		if ctx.Mode() == "tui" {
			notify(ctx, strings.Join(displayGroupLines(groups, ctx.Width()), "\n"))
			return nil
		}
		output := map[string]any{"toolDisplay": groups, "settings": status}
		encoded, err := json.MarshalIndent(output, "", "  ")
		if err != nil {
			return err
		}
		if len(encoded) > 48<<10 {
			return errors.New("display settings exceed the inspection budget; narrow the owner/key filter")
		}
		notify(ctx, string(encoded))
		return nil
	}
	identifiers, value, err := commandFields(args, 4)
	if err != nil {
		return err
	}
	if len(identifiers) != 4 || verb == "set" && value == "" || verb == "reset" && value != "" {
		return errors.New("usage: /workbench settings tools set <scope> <owner> <key> <field> <TOML value> | reset <scope> <owner> <key> <field>")
	}
	snapshot, err := s.settings.Current()
	if err != nil {
		return err
	}
	edit := config.Edit{Key: displayKey(identifiers[1], identifiers[2], identifiers[3]), Reset: verb == "reset"}
	if !edit.Reset {
		edit.Value, err = config.ParseValue(value)
		if err != nil {
			return err
		}
	}
	result, saveErr := s.settings.Save(work, config.Scope(identifiers[0]), snapshot, []config.Edit{edit}, ctx.IsProjectTrusted)
	if result.Published {
		a.settingsChanged(ctx, s, result.Revision, result.Changed)
	}
	if err := notifyJSON(ctx, map[string]any{"settings": "save", "result": result}); err != nil {
		return err
	}
	return saveErr
}
