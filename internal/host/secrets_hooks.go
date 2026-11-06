package host

import (
	"bytes"
	"context"
	"errors"
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
	"os"
	"os/exec"
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

const maxUserBashOutput = 4 << 20

// userBash runs a user `!` command with placeholders substituted and its output redacted,
// so the transcript only ever holds placeholders. Output is buffered because a secret
// split across two streamed chunks would escape detection.
func (a *app) userBash(ctx sdk.Context, _ map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	guard, enabled, err := s.secrets.Guard()
	if err == nil && !enabled {
		return nil, nil
	}
	return map[string]any{"operations": sdk.BashOperations{Exec: func(command, cwd string, options sdk.BashExecOptions) (sdk.BashExecResult, error) {
		if err != nil {
			s.secrets.withhold(ctx, err)
			return sdk.BashExecResult{}, errors.New("secrets boundary unavailable; command not run")
		}
		real, subErr := guard.Substitute(command)
		if subErr != nil {
			return sdk.BashExecResult{}, errors.New("command refused: it names an unknown or deleted secret placeholder")
		}
		shell := "/bin/sh"
		if _, statErr := os.Stat("/bin/bash"); statErr == nil {
			shell = "/bin/bash"
		}
		signal := options.Signal
		if signal == nil {
			signal = context.Background()
		}
		cmd := exec.CommandContext(signal, shell, "-c", real)
		cmd.Dir = cwd
		if len(options.Env) > 0 {
			cmd.Env = os.Environ()
			for name, value := range options.Env {
				cmd.Env = append(cmd.Env, name+"="+value)
			}
		}
		var output limitedBuffer
		output.limit = maxUserBashOutput
		cmd.Stdout, cmd.Stderr = &output, &output
		runErr := cmd.Run()
		redacted, redactErr := guard.Redact(output.String())
		if redactErr != nil {
			s.secrets.withhold(ctx, redactErr)
			return sdk.BashExecResult{}, errors.New("secrets boundary unavailable; output withheld")
		}
		if options.OnData != nil && redacted != "" {
			options.OnData([]byte(redacted))
		}
		var exit *exec.ExitError
		switch {
		case runErr == nil:
			code := 0
			return sdk.BashExecResult{ExitCode: &code}, nil
		case errors.As(runErr, &exit):
			code := exit.ExitCode()
			if code < 0 {
				code = 128
			}
			return sdk.BashExecResult{ExitCode: &code}, nil
		case signal.Err() != nil:
			return sdk.BashExecResult{}, errors.New("aborted")
		default:
			return sdk.BashExecResult{}, errors.New("command could not start")
		}
	}}}, nil
}

// limitedBuffer keeps at most limit bytes and silently drops the rest.
type limitedBuffer struct {
	bytes.Buffer
	limit int
}

func (b *limitedBuffer) Write(p []byte) (int, error) {
	if room := b.limit - b.Len(); room > 0 {
		b.Buffer.Write(p[:min(len(p), room)])
	}
	return len(p), nil
}

// redactContext is the per-turn backstop for content that entered without a hook of ours: the
// system prompt, context files and other extensions' messages. It cannot withhold a request;
// a failure leaves the context as is and before_provider_request aborts the turn.
func (a *app) redactContext(ctx sdk.Context, data map[string]any) (any, error) {
	messages, ok := data["messages"].([]any)
	if !ok {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	guard, enabled, err := s.secrets.Guard()
	if err != nil {
		s.secrets.withhold(ctx, err)
		return nil, nil
	}
	if !enabled {
		return nil, nil
	}
	redacted, err := secrets.RedactMessages(guard, messages)
	if err != nil {
		s.secrets.withhold(ctx, err)
		return nil, nil
	}
	return map[string]any{"messages": redacted}, nil
}

// redactProviderRequest is the last pass over an agent turn. Returning an error alone would let
// PiG send the original payload, so a failure also aborts the request.
func (a *app) redactProviderRequest(ctx sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	payload, ok := data["payload"].(map[string]any)
	if !ok {
		if _, present := data["payload"]; present && data["payload"] != nil {
			return a.abortRequest(ctx, s, errors.New("provider payload has an unsupported shape"))
		}
		return nil, nil
	}
	guard, enabled, err := s.secrets.Guard()
	if err != nil {
		return a.abortRequest(ctx, s, err)
	}
	if !enabled {
		return nil, nil
	}
	redacted, err := secrets.RedactProviderPayload(guard, payload)
	if err != nil {
		return a.abortRequest(ctx, s, err)
	}
	return redacted, nil
}

func (a *app) abortRequest(ctx sdk.Context, s *session, cause error) (any, error) {
	s.secrets.withhold(ctx, cause)
	ctx.Abort()
	return nil, errors.New("request withheld: the secrets boundary is unavailable")
}

// substituteCall replaces placeholders in the arguments of every tool call, including other
// extensions' tools. The model and transcript keep the placeholder; only the tool receives the value.
// An unknown or deleted placeholder blocks the call, and so does an unavailable boundary.
func (a *app) substituteCall(ctx sdk.Context, data map[string]any) (any, error) {
	input, ok := data["input"].(map[string]any)
	if !ok || len(input) == 0 {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	guard, enabled, err := s.secrets.Guard()
	if err != nil {
		s.secrets.withhold(ctx, err)
		return map[string]any{"block": true, "reason": "The secrets boundary is unavailable, so this call was not run."}, nil
	}
	if !enabled {
		return nil, nil
	}
	substituted, err := secrets.SubstituteArguments(guard, input)
	if err != nil {
		return map[string]any{"block": true, "reason": "The call names a secret placeholder that does not exist or was deleted."}, nil
	}
	for key := range input {
		delete(input, key)
	}
	for key, value := range substituted {
		input[key] = value
	}
	return nil, nil
}
