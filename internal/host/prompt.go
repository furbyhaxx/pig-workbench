package host

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
	"github.com/furbyhaxx/pig-workbench/internal/prompt"
)

// promptOverride replaces PiG's prompt only for an explicit force delivery.
// auto stays on PiG; the live A/B was too small to change the default.
func promptOverride(enabled bool, delivery, rendered string) (string, bool) {
	if !enabled || delivery != "force" || rendered == "" {
		return "", false
	}
	return rendered, true
}

func walkReplace(value any, from, to string) (any, int) {
	switch typed := value.(type) {
	case string:
		if typed == from {
			return to, 1
		}
		return typed, 0
	case map[string]any:
		n := 0
		for key, child := range typed {
			var count int
			typed[key], count = walkReplace(child, from, to)
			n += count
		}
		return typed, n
	case []any:
		n := 0
		for i, child := range typed {
			var count int
			typed[i], count = walkReplace(child, from, to)
			n += count
		}
		return typed, n
	default:
		return value, 0
	}
}

func (a *app) spliceLatePrompt(ctx sdk.Context, data map[string]any) (any, error) {
	session, err := a.current()
	if err != nil || session.promptText == "" {
		return nil, nil
	}
	payload, ok := data["payload"]
	if !ok {
		return nil, nil
	}
	extra := lateSections(ctx, session.promptText)
	if extra == "" {
		return nil, nil
	}
	replaced, n := walkReplace(payload, session.promptText, session.promptText+"\n\n"+extra)
	if n == 0 {
		if err := session.admitLateSection(ctx, extra); err != nil {
			return nil, err
		}
		return nil, nil
	}
	return replaced, nil
}

func (s *session) admitLateSection(ctx sdk.Context, body string) error {
	if s == nil || s.contextFrames == nil || s.contextFrames.ledger == nil {
		return errors.New("late prompt section has no frame ledger")
	}
	state := s.contextFrames
	state.mu.Lock()
	defer state.mu.Unlock()
	scope, err := branchScope(ctx, s.snapshots.Epoch())
	if err != nil {
		return err
	}
	id, err := admitLate(state.ledger, scope, body)
	if err != nil {
		return err
	}
	return persistFrames(ctx, state.ledger, []string{id})
}

// admitLate queues a section the served prompt lacks for the next prompt on this branch.
func admitLate(ledger *frames.Session, scope frames.DeliveryScope, body string) (string, error) {
	context := frames.Context{ProducerPrefix: "workbench", BranchLeaf: scope.BranchLeaf, Ancestors: scope.Ancestors, Epoch: scope.Epoch, Trusted: true, Authority: "data"}
	receipt, err := ledger.Ledger(context).Admit(frames.Publish{EventKey: "prompt/late/" + promptEpoch(body), Kind: "notice", Source: "workbench", SubjectKey: "prompt.late", Body: body, Delivery: "nextPrompt", DataTrust: "trusted", CoalesceKey: "prompt.late"})
	return receipt.FrameID, err
}

func lateSections(ctx sdk.Context, served string) string {
	options, err := ctx.GetSystemPromptOptions()
	if err != nil || options.Sections == nil {
		return ""
	}
	var b strings.Builder
	for _, section := range *options.Sections {
		if section.Name == "" || strings.Contains(served, "<"+section.Name+">") {
			continue
		}
		if b.Len() > 0 {
			b.WriteString("\n\n")
		}
		fmt.Fprintf(&b, "<%s>\n%s\n</%s>", section.Name, section.Value, section.Name)
	}
	return b.String()
}

func promptEpoch(rendered string) string {
	sum := sha256.Sum256([]byte(rendered))
	return hex.EncodeToString(sum[:8])
}

func applyPromptRender(enabled bool, rendered string, renderErr error, delivery string) (string, bool, string) {
	if !enabled {
		return "", false, ""
	}
	if renderErr != nil {
		return "", false, renderErr.Error()
	}
	text, ok := promptOverride(true, delivery, rendered)
	return text, ok, ""
}

// decodePromptOptions reads the host's before_agent_start prompt inputs; a shape it cannot read yields none.
func decodePromptOptions(raw map[string]any) sdk.SystemPromptOptions {
	var options sdk.SystemPromptOptions
	if encoded, err := json.Marshal(raw); err == nil {
		_ = json.Unmarshal(encoded, &options)
	}
	return options
}

// promptData maps the host's real prompt inputs and Workbench's live state onto the template.
// Instruction files are not passed: SPEC-0007 frames deliver them, and the template would repeat them.
func promptData(s *session, options sdk.SystemPromptOptions) prompt.Data {
	hidden := make(map[string]bool, len(options.HiddenTools))
	for _, name := range options.HiddenTools {
		hidden[name] = true
	}
	declared := make([]string, 0, len(options.SelectedTools))
	for _, name := range options.SelectedTools {
		if !hidden[name] {
			declared = append(declared, name)
		}
	}
	skills := make([]prompt.HostSkill, 0, len(options.Skills))
	for _, skill := range options.Skills {
		skills = append(skills, prompt.HostSkill{Name: skill.Name, Description: skill.Description, FilePath: skill.FilePath, BaseDir: skill.BaseDir, DisableModelInvocation: skill.DisableModelInvocation})
	}
	root := options.Cwd
	if root == "" && s.location != nil {
		root = s.location.Current().Cwd
	}
	data := prompt.Data{
		ProjectRoot: root,
		Tools:       prompt.Tools{Selected: options.SelectedTools, Declared: declared, Hidden: options.HiddenTools, Snippets: options.ToolSnippets, Guidelines: options.ToolGuidelines},
		Host:        prompt.Host{CustomPrompt: options.CustomPrompt, Append: options.AppendSystemPrompt, PromptGuidelines: options.PromptGuidelines, Skills: skills},
	}
	if s.router != nil {
		for _, scheme := range s.router.Schemes() {
			data.Workbench.Schemes = append(data.Workbench.Schemes, prompt.Scheme{Name: scheme})
		}
	}
	if configuration, err := s.settings.Current(); err == nil {
		if entry, ok := configuration.Get("edit.mode"); ok {
			data.Workbench.Edit.Mode, _ = entry.Value.(string)
		}
		if entry, ok := configuration.Get("secrets.enabled"); ok {
			data.Workbench.Secrets.Enabled, _ = entry.Value.(bool)
		}
	}
	return data
}
