package host

import (
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
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
