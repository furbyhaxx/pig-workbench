package host

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"path/filepath"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/ask"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

type nativeDialogs struct{ host sdk.Context }

func (d nativeDialogs) Available() bool {
	return d.host.HasUI() && (d.host.Mode() == "tui" || d.host.Mode() == "rpc")
}
func dialogOptions(ctx context.Context) (sdk.DialogOptions, error) {
	if err := ctx.Err(); err != nil {
		return sdk.DialogOptions{}, err
	}
	var options sdk.DialogOptions
	if deadline, ok := ctx.Deadline(); ok {
		remaining := time.Until(deadline)
		if remaining <= 0 {
			return options, context.DeadlineExceeded
		}
		options.Timeout = math.Ceil(float64(remaining) / float64(time.Millisecond))
	}
	return options, nil
}
func dialogResult(ctx context.Context, value any, ok bool, err error) (string, bool, error) {
	if ctx.Err() != nil {
		return "", false, ctx.Err()
	}
	if deadline, has := ctx.Deadline(); has && !time.Now().Before(deadline) {
		return "", false, context.DeadlineExceeded
	}
	if err != nil {
		return "", false, errors.New("native dialog failed")
	}
	if !ok {
		return "", false, nil
	}
	text, valid := value.(string)
	if !valid {
		return "", false, errors.New("native dialog returned an invalid response")
	}
	return text, true, nil
}
func (d nativeDialogs) Select(ctx context.Context, title string, options []string) (string, bool, error) {
	settings, err := dialogOptions(ctx)
	if err != nil {
		return "", false, err
	}
	value, ok, err := d.host.SelectWithOptions(noticeText(title, 0), options, settings)
	return dialogResult(ctx, value, ok, err)
}
func (d nativeDialogs) Input(ctx context.Context, title, placeholder string) (string, bool, error) {
	settings, err := dialogOptions(ctx)
	if err != nil {
		return "", false, err
	}
	value, ok, err := d.host.InputWithOptions(noticeText(title, 0), noticeText(placeholder, 0), settings)
	return dialogResult(ctx, value, ok, err)
}
func (d nativeDialogs) SecretInput(ctx context.Context, title, placeholder string) ([]byte, bool, error) {
	text, ok, err := d.Input(ctx, title, placeholder)
	return []byte(text), ok, err
}

type questionSecrets struct {
	host sdk.Context
	cwd  string
}

func (s questionSecrets) Store(name, scope, description, env string, value []byte) (string, error) {
	agent, err := sdk.AgentDir()
	if err != nil {
		return "", errors.New("secret storage directory unavailable")
	}
	master, err := secrets.LoadKey(filepath.Join(agent, "secret.key"))
	if err != nil {
		master, err = secrets.CreateKey(filepath.Join(agent, "secret.key"))
		if err != nil {
			return "", errors.New("secret key unavailable; inspect key permissions and format")
		}
	}
	defer clear(master)
	key, err := secrets.DeriveHMACKey(master)
	if err != nil {
		return "", errors.New("secret key derivation failed")
	}
	defer clear(key)
	vaultPath, err := secrets.VaultPath()
	if err != nil {
		return "", errors.New("private secret data directory unavailable")
	}
	vault, err := secrets.OpenVault(vaultPath, master)
	if err != nil {
		return "", errors.New("secret vault unavailable")
	}
	projectID := ""
	if scope == "project" {
		registry, err := project.Open(agent)
		if err != nil {
			return "", errors.New("project registry unavailable")
		}
		detection, err := registry.Detect(requestContext{s.host}, s.cwd)
		if err != nil {
			return "", errors.New("cannot determine secret project scope")
		}
		projectID = detection.ProjectID
	}
	placeholder := secrets.Placeholder(key, value)
	record := secrets.Record{Placeholder: placeholder, Kind: "token", Source: "user", Scope: scope, ProjectID: projectID, Name: name, Env: env, Description: description}
	if err := vault.Put(record, value); err != nil {
		return "", errors.New("secret vault could not acknowledge storage")
	}
	return placeholder, nil
}

func (a *app) registerAsk(ext *sdk.Extension) {
	text := map[string]any{"type": "string"}
	option := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"value", "label"}, "properties": map[string]any{"value": text, "label": text, "description": text, "preview": text, "recommended": map[string]any{"type": "boolean"}}}
	secret := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"name", "scope"}, "properties": map[string]any{"name": text, "scope": map[string]any{"type": "string", "enum": []string{"global", "project"}}, "description": text, "env": text}}
	question := map[string]any{"type": "object", "additionalProperties": false, "required": []string{"id", "label", "prompt", "type"}, "properties": map[string]any{"id": text, "label": text, "prompt": text, "type": map[string]any{"type": "string", "enum": []string{"single", "multi", "preview", "freeform", "secret"}}, "options": map[string]any{"type": "array", "maxItems": 50, "items": option}, "secret": secret, "required": map[string]any{"type": "boolean"}, "allowCustom": map[string]any{"type": "boolean"}, "note": text}}
	ext.RegisterTool(sdk.ToolDefinition{Name: "ask_user", Label: "ask_user", Description: "Ask 1–20 questions using native interactive dialogs. Supports single, multi, preview, freeform and secret questions, custom answers and notes. Blocks for the user's answers. Dismissal returns cancelled without partial answers; an elapsed dialog deadline returns timedOut. Print/JSON without UI returns interactionUnavailable. Secret values are stored encrypted and only placeholders appear in answers or session records.", PromptSnippet: "Ask the user for explicit choices or missing information.", Parameters: sdk.Schema{"type": "object", "additionalProperties": false, "required": []string{"questions"}, "properties": map[string]any{"title": text, "questions": map[string]any{"type": "array", "minItems": 1, "maxItems": 20, "items": question}}}, Execute: a.askUser})
}

func (a *app) askUser(ctx sdk.Context, params map[string]any) (any, error) {
	payload, err := json.Marshal(params)
	if err != nil {
		return nil, errors.New("invalid questionnaire arguments")
	}
	form, err := ask.ParseQuestions(payload)
	if err != nil {
		return nil, err
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return nil, err
	}
	defer release()
	timeout := time.Duration(0)
	if ctx.Mode() == "rpc" {
		configuration, err := s.settings.Current()
		if err != nil {
			return nil, err
		}
		entry, _ := configuration.Get("ask.remote.timeout-ms")
		timeout = time.Duration(entry.Value.(int64)) * time.Millisecond
	}
	result, err := ask.Run(work, ask.Deps{SessionID: s.id, Append: func(kind string, value any) error { return ctx.AppendEntry(kind, value) }, Interactor: ask.NewDialogInteractor(nativeDialogs{host: ctx}), Store: questionSecrets{host: ctx, cwd: s.location.Current().Cwd}, Options: ask.Options{DialogTimeout: timeout}}, form)
	if err != nil {
		return nil, err
	}
	output, err := json.Marshal(result)
	if err != nil {
		return nil, err
	}
	return sdk.ToolResult{Content: string(output), Details: result}, nil
}
