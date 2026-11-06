package host

import (
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

// redactText replaces secrets in text. active is false when the user disabled secrets.
func (b *secretsBoundary) redactText(text string) (out string, active bool, err error) {
	guard, enabled, err := b.Guard()
	if err != nil || !enabled {
		return "", false, err
	}
	out, err = guard.Redact(text)
	return out, true, err
}

// withhold tells the user once per cause that output was withheld.
func (b *secretsBoundary) withhold(ctx sdk.Context, err error) {
	if reason, fresh := b.Withheld(err); fresh {
		ctx.Notify(noticeText(reason, 0), "error")
	}
}

// redactInput replaces secrets in a typed, pasted or RPC prompt before it enters the session;
// a boundary failure swallows the prompt instead of letting it through.
func (a *app) redactInput(ctx sdk.Context, data map[string]any) (any, error) {
	text, _ := data["text"].(string)
	if text == "" {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	redacted, active, err := s.secrets.redactText(text)
	if err != nil {
		s.secrets.withhold(ctx, err)
		return map[string]any{"action": "handled"}, nil
	}
	if !active || redacted == text {
		return nil, nil
	}
	return map[string]any{"action": "transform", "text": redacted}, nil
}

// resultPatch is the tool_result patch for content; cause is non-nil when output was withheld.
func (b *secretsBoundary) resultPatch(content any) (patch any, cause error) {
	guard, enabled, err := b.Guard()
	if err == nil && enabled {
		var redacted any
		if redacted, err = secrets.RedactContent(guard, content); err == nil {
			return map[string]any{"content": redacted}, nil
		}
	}
	if err == nil {
		return nil, nil
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": "Output withheld: the secrets boundary is unavailable. Repair it (see /workbench secrets) and run the call again."}}, "isError": true}, err
}

// redactResult replaces secrets in every model-visible block of a tool result before it enters
// the session. A failing boundary replaces the content so raw output never reaches the transcript.
func (a *app) redactResult(ctx sdk.Context, data map[string]any) (any, error) {
	content, present := data["content"]
	if !present || content == nil {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	patch, cause := s.secrets.resultPatch(content)
	if cause != nil {
		s.secrets.withhold(ctx, cause)
	}
	return patch, nil
}
