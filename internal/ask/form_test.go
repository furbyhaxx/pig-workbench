package ask

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
)

func validForm() Form {
	return Form{
		Title: "Storage policy",
		Questions: []Question{{
			ID: "scope", Label: "Scope", Prompt: "Where should history be discoverable?",
			Type: KindSingle, Required: true,
			Options: []Option{
				{Value: "project", Label: "Project only", Description: "Restrict discovery"},
				{Value: "global", Label: "Project and global", Recommended: true},
			},
		}},
	}
}

func TestValidateFormChoiceAndDuplicate(t *testing.T) {
	form := validForm()
	form.Questions = append(form.Questions, form.Questions[0])
	if _, err := ValidateForm(form); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("duplicate id must name scope: %v", err)
	}

	form = validForm()
	form.Questions[0].Options = nil
	if _, err := ValidateForm(form); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("missing options must name scope: %v", err)
	}
	form.Questions[0].AllowCustom = true
	if _, err := ValidateForm(form); err != nil {
		t.Fatalf("custom-only choice must be accepted: %v", err)
	}
}

func TestValidateFormBoundaries(t *testing.T) {
	tests := []struct {
		name string
		edit func(*Form)
		want string
	}{
		{"unknown kind", func(f *Form) { f.Questions[0].Type = "invalid" }, "type"},
		{"invalid id", func(f *Form) { f.Questions[0].ID = "Bad" }, "id"},
		{"empty question label", func(f *Form) { f.Questions[0].Label = "  " }, "label"},
		{"empty option label", func(f *Form) { f.Questions[0].Options[0].Label = "" }, "label"},
		{"control in title", func(f *Form) { f.Title = "bad\x1btitle" }, "title"},
		{"control in prompt", func(f *Form) { f.Questions[0].Prompt = "bad\ntext" }, "prompt"},
		{"control in option description", func(f *Form) { f.Questions[0].Options[0].Description = "bad\x7ftext" }, "description"},
		{"long prompt", func(f *Form) { f.Questions[0].Prompt = strings.Repeat("é", 2001) }, "prompt"},
		{"too many questions", func(f *Form) {
			f.Questions = make([]Question, 21)
		}, "questions"},
		{"too many options", func(f *Form) {
			f.Questions[0].Options = make([]Option, 51)
		}, "options"},
		{"duplicate option values", func(f *Form) {
			f.Questions[0].Options[1].Value = f.Questions[0].Options[0].Value
		}, "value"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			form := validForm()
			tt.edit(&form)
			_, err := ValidateForm(form)
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("expected %s validation error, got %v", tt.want, err)
			}
		})
	}
}

func TestValidateSecretMetadata(t *testing.T) {
	form := validForm()
	q := &form.Questions[0]
	q.Type, q.Options = KindSecret, nil
	if _, err := ValidateForm(form); err == nil || !strings.Contains(err.Error(), "secret") {
		t.Fatalf("secret without metadata accepted: %v", err)
	}
	q.Secret = &SecretSpec{Name: "github-token", Env: "GITHUB_TOKEN"}
	valid, err := ValidateForm(form)
	if err != nil || valid.Questions[0].Secret.Scope != "project" {
		t.Fatalf("default project secret: %#v, %v", valid, err)
	}
	if form.Questions[0].Secret.Scope != "" {
		t.Fatal("validation mutated caller's secret specification")
	}
	for _, spec := range []SecretSpec{
		{Name: ".hidden", Scope: "project"},
		{Name: "bad name", Scope: "project"},
		{Name: "token", Scope: "user"},
		{Name: "token", Scope: "global", Env: "2BAD"},
		{Name: "token", Scope: "project", Description: strings.Repeat("x", 201)},
		{Name: "token", Scope: "project", Description: "bad\nvalue"},
	} {
		q.Secret = &spec
		if _, err := ValidateForm(form); err == nil {
			t.Fatalf("invalid secret metadata accepted: %#v", spec)
		}
	}
	q.Secret = &SecretSpec{Name: "token", Scope: "project"}
	q.AllowCustom = true
	if _, err := ValidateForm(form); err == nil {
		t.Fatal("secret must not allow a custom choice")
	}
}

func TestValidatePreview(t *testing.T) {
	form := validForm()
	form.Questions[0].Type = KindPreview
	if _, err := ValidateForm(form); err == nil || !strings.Contains(err.Error(), "scope") {
		t.Fatalf("preview without preview content must name scope: %v", err)
	}
	for i := range form.Questions[0].Options {
		form.Questions[0].Options[i].Preview = "first line\n\tsecond line"
	}
	form.Questions[0].Options[0].Preview = strings.Repeat("x", 8192)
	if _, err := ValidateForm(form); err != nil {
		t.Fatalf("valid multiline preview rejected: %v", err)
	}
	form.Questions[0].Options[0].Preview = strings.Repeat("x", 8193)
	if _, err := ValidateForm(form); err == nil {
		t.Fatal("oversized preview accepted")
	}
	form.Questions[0].Options[0].Preview = "bad\x1btext"
	if _, err := ValidateForm(form); err == nil {
		t.Fatal("terminal sequence in preview accepted")
	}
	form.Questions[0].Options = nil
	form.Questions[0].AllowCustom = true
	if _, err := ValidateForm(form); err == nil {
		t.Fatal("preview without options accepted")
	}
}

func TestParseQuestionsRejectsUnknownFields(t *testing.T) {
	tests := []string{
		`{"questions":[],"extra":true}`,
		`{"questions":[{"id":"x","label":"X","type":"freeform","extra":true}]}`,
		`{"questions":[{"id":"x","label":"X","options":[{"value":"a","label":"A","extra":true}]}]}`,
		`{"questions":[{"id":"x","label":"X","type":"secret","secret":{"name":"token","extra":true}}]}`,
	}
	for _, input := range tests {
		if _, err := ParseQuestions(json.RawMessage(input)); err == nil || !strings.Contains(err.Error(), "extra") {
			t.Fatalf("unknown field accepted: %s: %v", input, err)
		}
	}
	if _, err := ParseQuestions(json.RawMessage(`{"questions":[]} {}`)); err == nil {
		t.Fatal("trailing JSON accepted")
	}
}

func TestParseQuestionsRejectsWrongCaseAndDuplicateMembers(t *testing.T) {
	for _, input := range []string{
		`{"Title":"bad","questions":[{"id":"x","label":"X","type":"freeform"}]}`,
		`{"questions":[{"id":"x","Label":"X","type":"freeform"}]}`,
		`{"questions":[{"id":"x","id":"y","label":"X","type":"freeform"}]}`,
		`{"questions":[{"id":"x","label":"X","type":"freeform","required":null}]}`,
	} {
		if _, err := ParseQuestions(json.RawMessage(input)); err == nil {
			t.Fatalf("non-canonical or duplicate field accepted: %s", input)
		}
	}
}

func TestFormMarshalRoundTrip(t *testing.T) {
	form := validForm()
	first, err := form.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	want := `{"title":"Storage policy","questions":[{"id":"scope","label":"Scope","prompt":"Where should history be discoverable?","type":"single","required":true,"options":[{"value":"project","label":"Project only","description":"Restrict discovery"},{"value":"global","label":"Project and global","recommended":true}]}]}`
	if string(first) != want {
		t.Fatalf("unexpected wire shape: %s", first)
	}
	parsed, err := ParseQuestions(first)
	if err != nil {
		t.Fatal(err)
	}
	second, err := parsed.Marshal()
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(first, second) {
		t.Fatalf("round-trip changed bytes: %s != %s", first, second)
	}
}
