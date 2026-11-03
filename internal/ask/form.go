// Package ask defines questionnaire arguments and branch-local recovery records.
package ask

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"reflect"
	"regexp"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"
)

const (
	maxQuestions = 20
	maxOptions   = 50
	maxPrompt    = 2000
	maxPreview   = 8 * 1024
	maxPayload   = 64 * 1024
)

var (
	questionIDPattern = regexp.MustCompile(`^[a-z0-9][a-z0-9._-]{0,63}$`)
	secretNamePattern = regexp.MustCompile(`^[A-Za-z0-9_-][A-Za-z0-9._-]{0,63}$`)
	envNamePattern    = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)
)

// Kind is the kind of response a question requests.
type Kind string

const (
	KindSingle   Kind = "single"
	KindMulti    Kind = "multi"
	KindPreview  Kind = "preview"
	KindSecret   Kind = "secret"
	KindFreeform Kind = "freeform"
)

// Option is one model-provided choice.
type Option struct {
	Value       string `json:"value"`
	Label       string `json:"label"`
	Description string `json:"description,omitempty"`
	Preview     string `json:"preview,omitempty"`
	Recommended bool   `json:"recommended,omitempty"`
}

// SecretSpec describes a secret without containing its value.
type SecretSpec struct {
	Name        string `json:"name"`
	Scope       string `json:"scope"`
	Env         string `json:"env,omitempty"`
	Description string `json:"description,omitempty"`
}

// Question describes one question and its available choices.
type Question struct {
	ID          string      `json:"id"`
	Label       string      `json:"label"`
	Prompt      string      `json:"prompt"`
	Type        Kind        `json:"type"`
	Required    bool        `json:"required,omitempty"`
	Options     []Option    `json:"options,omitempty"`
	Secret      *SecretSpec `json:"secret,omitempty"`
	AllowCustom bool        `json:"allowCustom,omitempty"`
	Note        string      `json:"note,omitempty"`
}

// Form is the argument object supplied to ask_user.
type Form struct {
	Title     string     `json:"title,omitempty"`
	Questions []Question `json:"questions"`
}

// ValidateForm validates the form and returns a normalized independent copy.
func ValidateForm(form Form) (Form, error) {
	if err := validateText("title", form.Title, false); err != nil {
		return Form{}, err
	}
	if len(form.Questions) == 0 || len(form.Questions) > maxQuestions {
		return Form{}, fmt.Errorf("questions: must contain 1 to %d questions", maxQuestions)
	}
	form = cloneForm(form)
	ids := make(map[string]struct{}, len(form.Questions))
	for i := range form.Questions {
		q := &form.Questions[i]
		if !questionIDPattern.MatchString(q.ID) {
			return Form{}, fmt.Errorf("questions[%d].id: must match [a-z0-9][a-z0-9._-]{0,63}", i)
		}
		if _, exists := ids[q.ID]; exists {
			return Form{}, fmt.Errorf("question %q: duplicate id", q.ID)
		}
		ids[q.ID] = struct{}{}
		field := fmt.Sprintf("question %q", q.ID)
		if strings.TrimSpace(q.Label) == "" {
			return Form{}, fmt.Errorf("%s.label: must not be empty", field)
		}
		for _, item := range []struct{ name, value string }{
			{field + ".label", q.Label}, {field + ".prompt", q.Prompt}, {field + ".note", q.Note},
		} {
			if err := validateText(item.name, item.value, false); err != nil {
				return Form{}, err
			}
		}
		if utf8.RuneCountInString(q.Prompt) > maxPrompt {
			return Form{}, fmt.Errorf("%s.prompt: exceeds %d characters", field, maxPrompt)
		}
		if q.Type == "" {
			q.Type = KindSingle
		}
		switch q.Type {
		case KindSingle, KindMulti, KindPreview, KindSecret, KindFreeform:
		default:
			return Form{}, fmt.Errorf("%s.type: unsupported question type", field)
		}
		if len(q.Options) > maxOptions {
			return Form{}, fmt.Errorf("%s.options: exceeds %d options", field, maxOptions)
		}
		switch q.Type {
		case KindSingle, KindMulti:
			if len(q.Options) == 0 && !q.AllowCustom {
				return Form{}, fmt.Errorf("%s.options: at least one option or allowCustom is required", field)
			}
		case KindPreview:
			if len(q.Options) == 0 {
				return Form{}, fmt.Errorf("%s.options: preview requires at least one option", field)
			}
		case KindSecret, KindFreeform:
			if len(q.Options) != 0 {
				return Form{}, fmt.Errorf("%s.options: not allowed for %s", field, q.Type)
			}
		}
		if q.Type == KindSecret {
			if q.AllowCustom {
				return Form{}, fmt.Errorf("%s.allowCustom: not allowed for secret", field)
			}
			if q.Secret == nil {
				return Form{}, fmt.Errorf("%s.secret: secret metadata is required", field)
			}
			if q.Secret.Scope == "" {
				q.Secret.Scope = "project"
			}
			if err := validateSecret(field+".secret", *q.Secret); err != nil {
				return Form{}, err
			}
		} else if q.Secret != nil {
			return Form{}, fmt.Errorf("%s.secret: only allowed for secret questions", field)
		}
		values := make(map[string]struct{}, len(q.Options))
		for j, option := range q.Options {
			optionField := fmt.Sprintf("%s.options[%d]", field, j)
			if strings.TrimSpace(option.Label) == "" {
				return Form{}, fmt.Errorf("%s.label: must not be empty", optionField)
			}
			for _, item := range []struct{ name, value string }{
				{optionField + ".value", option.Value}, {optionField + ".label", option.Label},
				{optionField + ".description", option.Description},
			} {
				if err := validateText(item.name, item.value, false); err != nil {
					return Form{}, err
				}
			}
			if _, exists := values[option.Value]; exists {
				return Form{}, fmt.Errorf("%s.value: duplicate option value", optionField)
			}
			values[option.Value] = struct{}{}
			if err := validateText(optionField+".preview", option.Preview, true); err != nil {
				return Form{}, err
			}
			if len(option.Preview) > maxPreview {
				return Form{}, fmt.Errorf("%s.preview: exceeds %d bytes", optionField, maxPreview)
			}
			if q.Type == KindPreview && strings.TrimSpace(option.Preview) == "" {
				return Form{}, fmt.Errorf("%s.preview: preview text is required", optionField)
			}
		}
	}
	encoded, err := json.Marshal(form)
	if err != nil {
		return Form{}, fmt.Errorf("encode form: %w", err)
	}
	if len(encoded) > maxPayload {
		return Form{}, fmt.Errorf("form: exceeds %d bytes", maxPayload)
	}
	return form, nil
}

func cloneForm(form Form) Form {
	form.Questions = slices.Clone(form.Questions)
	for i := range form.Questions {
		question := &form.Questions[i]
		question.Options = slices.Clone(question.Options)
		if question.Secret != nil {
			secret := *question.Secret
			question.Secret = &secret
		}
	}
	return form
}

func validateText(field, value string, multiline bool) error {
	if !utf8.ValidString(value) {
		return fmt.Errorf("%s: invalid UTF-8", field)
	}
	for _, r := range value {
		if unicode.IsControl(r) && !(multiline && (r == '\n' || r == '\t')) {
			return fmt.Errorf("%s: control characters are not allowed", field)
		}
	}
	return nil
}

func validateSecret(field string, secret SecretSpec) error {
	if !secretNamePattern.MatchString(secret.Name) {
		return fmt.Errorf("%s.name: invalid secret name", field)
	}
	if secret.Scope != "project" && secret.Scope != "global" {
		return fmt.Errorf("%s.scope: must be project or global", field)
	}
	if secret.Env != "" && !envNamePattern.MatchString(secret.Env) {
		return fmt.Errorf("%s.env: invalid environment variable name", field)
	}
	if err := validateText(field+".description", secret.Description, false); err != nil {
		return err
	}
	if utf8.RuneCountInString(secret.Description) > 200 {
		return fmt.Errorf("%s.description: exceeds 200 characters", field)
	}
	return nil
}

// Marshal returns the normalized ask_user argument object as JSON.
func (f Form) Marshal() (json.RawMessage, error) {
	form, err := ValidateForm(f)
	if err != nil {
		return nil, err
	}
	return json.Marshal(form)
}

// ParseQuestions strictly decodes and validates an ask_user argument object.
func ParseQuestions(raw json.RawMessage) (Form, error) {
	if len(raw) > maxPayload {
		return Form{}, fmt.Errorf("form: exceeds %d bytes", maxPayload)
	}
	if !utf8.Valid(raw) {
		return Form{}, fmt.Errorf("form: invalid UTF-8")
	}
	members := json.NewDecoder(bytes.NewReader(raw))
	members.UseNumber()
	if err := validateJSONValue(members, reflect.TypeOf(Form{}), "form"); err != nil {
		return Form{}, fmt.Errorf("decode form: %w", err)
	}
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var form Form
	if err := decoder.Decode(&form); err != nil {
		return Form{}, fmt.Errorf("decode form: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return Form{}, fmt.Errorf("decode form: unexpected trailing JSON")
		}
		return Form{}, fmt.Errorf("decode form: %w", err)
	}
	return ValidateForm(form)
}

func validateJSONValue(decoder *json.Decoder, typ reflect.Type, path string) error {
	for typ.Kind() == reflect.Pointer {
		typ = typ.Elem()
	}
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	if token == nil {
		return fmt.Errorf("%s: null is not allowed", path)
	}
	delim, isDelim := token.(json.Delim)
	if !isDelim {
		return nil
	}
	switch delim {
	case '{':
		if typ.Kind() != reflect.Struct {
			return fmt.Errorf("%s: unexpected object", path)
		}
		fields := make(map[string]reflect.Type, typ.NumField())
		for i := 0; i < typ.NumField(); i++ {
			field := typ.Field(i)
			name, _, _ := strings.Cut(field.Tag.Get("json"), ",")
			fields[name] = field.Type
		}
		seen := make(map[string]bool, len(fields))
		for decoder.More() {
			nameToken, err := decoder.Token()
			if err != nil {
				return err
			}
			name := nameToken.(string)
			displayName := []rune(name)
			if len(displayName) > 64 {
				displayName = append(displayName[:64], '…')
			}
			fieldPath := fmt.Sprintf("%s.%q", path, string(displayName))
			fieldType, ok := fields[name]
			if !ok {
				return fmt.Errorf("%s: unknown field", fieldPath)
			}
			if seen[name] {
				return fmt.Errorf("%s: duplicate field", fieldPath)
			}
			seen[name] = true
			if err := validateJSONValue(decoder, fieldType, fieldPath); err != nil {
				return err
			}
		}
	case '[':
		if typ.Kind() != reflect.Slice {
			return fmt.Errorf("%s: unexpected array", path)
		}
		for i := 0; decoder.More(); i++ {
			if err := validateJSONValue(decoder, typ.Elem(), fmt.Sprintf("%s[%d]", path, i)); err != nil {
				return err
			}
		}
	default:
		return fmt.Errorf("%s: unexpected delimiter", path)
	}
	_, err = decoder.Token()
	return err
}
