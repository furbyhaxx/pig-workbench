package host

import (
	"bytes"
	"context"
	"errors"
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
	"os"
	"os/exec"
	"strconv"
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

// inputVerdict decides one prompt: a transform, pass-through, or a refusal that keeps the text for the editor.
func (b *secretsBoundary) inputVerdict(text string) (verdict any, cause error) {
	redacted, active, err := b.redactText(text)
	if err != nil {
		return map[string]any{"action": "handled"}, err
	}
	if !active || redacted == text {
		return nil, nil
	}
	return map[string]any{"action": "transform", "text": redacted}, nil
}

// redactInput replaces secrets in a typed, pasted or RPC prompt before it enters the session.
// A boundary failure keeps the prompt out of the model but never loses it: the text goes back to the editor.
func (a *app) redactInput(ctx sdk.Context, data map[string]any) (any, error) {
	text, _ := data["text"].(string)
	if text == "" {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	verdict, cause := s.secrets.inputVerdict(text)
	if cause != nil {
		s.secrets.withhold(ctx, cause)
		ctx.SetEditorText(text)
		ctx.Notify("Your prompt was not sent and is back in the editor; nothing was lost.", "warning")
	} else if notice := s.secrets.UnreadableNotice(); notice != "" {
		ctx.Notify(notice, "warning")
	}
	return verdict, nil
}

// resultPatch protects retained result fields atomically; no partial redaction is published.
func (b *secretsBoundary) resultPatch(data map[string]any) (patch any, cause error) {
	guard, enabled, err := b.Guard()
	if err == nil && !enabled {
		return nil, nil
	}
	if err == nil {
		out := make(map[string]any)
		if content, present := data["content"]; present {
			out["content"], err = secrets.RedactContent(guard, content)
		}
		if err == nil {
			for _, field := range []string{"details", "structuredContent"} {
				if value, present := data[field]; present {
					out[field], err = secrets.RedactData(guard, value)
					if err != nil {
						break
					}
				}
			}
		}
		if err == nil {
			return out, nil
		}
	}
	return map[string]any{"content": []any{map[string]any{"type": "text", "text": "Output withheld: the secrets boundary is unavailable. Repair the reported error, run /workbench settings reload, then retry."}}, "details": map[string]any{}, "structuredContent": map[string]any{}, "isError": true}, err
}

func (a *app) redactResult(ctx sdk.Context, data map[string]any) (any, error) {
	_, content := data["content"]
	_, details := data["details"]
	_, structured := data["structuredContent"]
	if !content && !details && !structured {
		return nil, nil
	}
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	patch, cause := s.secrets.resultPatch(data)
	if cause != nil {
		s.secrets.withhold(ctx, cause)
	}
	return patch, nil
}

const maxUserBashOutput = 4 << 20

// userBash runs a user `!` command with placeholders substituted, scheme paths applied and
// its output redacted, so the transcript only ever holds placeholders. Output is buffered
// because a secret split across two streamed chunks would escape detection.
func (a *app) userBash(ctx sdk.Context, _ map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	guard, enabled, err := s.secrets.Guard()
	routed := s.router != nil && s.settingBool("bash.route-to-tools", true)
	if err == nil && !enabled && !routed {
		return nil, nil
	}
	return map[string]any{"operations": sdk.BashOperations{Exec: func(command, cwd string, options sdk.BashExecOptions) (sdk.BashExecResult, error) {
		if err != nil {
			s.secrets.withhold(ctx, err)
			return sdk.BashExecResult{}, errors.New("secrets boundary unavailable; command not run")
		}
		redact := func(text string) (string, error) { return text, nil }
		real := command
		if enabled {
			var subErr error
			if real, subErr = guard.Substitute(command); subErr != nil {
				return sdk.BashExecResult{}, errors.New("command refused: it names an unknown or deleted secret placeholder")
			}
			redact = guard.Redact
		}
		signal := options.Signal
		if signal == nil {
			signal = context.Background()
		}
		if routed {
			work, release, admitErr := s.supervisor.Admit(signal)
			if admitErr != nil {
				return sdk.BashExecResult{}, admitErr
			}
			handled, result, rewritten, prepErr := a.prepareShellCommand(ctx, work, s, real, "user-bash-"+strconv.FormatUint(s.materialized.Add(1), 10))
			release()
			if handled || prepErr != nil {
				text, code := "", 0
				if prepErr != nil {
					text, code = prepErr.Error()+"\n", 1
				} else if tool, ok := result.(sdk.ToolResult); ok {
					text = tool.Content
				}
				redacted, redactErr := redact(text)
				if redactErr != nil {
					s.secrets.withhold(ctx, redactErr)
					return sdk.BashExecResult{}, errors.New("secrets boundary unavailable; output withheld")
				}
				if options.OnData != nil && redacted != "" {
					options.OnData([]byte(redacted))
				}
				return sdk.BashExecResult{ExitCode: &code}, nil
			}
			real = rewritten
		}
		shell := "/bin/sh"
		if _, statErr := os.Stat("/bin/bash"); statErr == nil {
			shell = "/bin/bash"
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
		redacted, redactErr := redact(output.String())
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
	if err := s.secrets.LearnReferenced(s.location.Current().Cwd, input); err != nil {
		s.secrets.withhold(ctx, err)
		return map[string]any{"block": true, "reason": "The secrets boundary is unavailable, so this call was not run."}, nil
	}
	return nil, nil
}
