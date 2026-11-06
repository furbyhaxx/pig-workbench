package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/frames"
)

// promptOverride replaces PiG's prompt only for an explicit force delivery.
// auto stays on PiG until the approved A/B run.
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
