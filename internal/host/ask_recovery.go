package host

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/ask"
)

// pendingAsk returns the newest unresolved questionnaire on the current branch.
func pendingAsk(ctx sdk.Context) (ask.Record, bool, error) {
	leaf, err := ctx.GetLeafID()
	if err != nil {
		return ask.Record{}, false, err
	}
	branch, err := ctx.SessionManager().GetBranch(leaf)
	if err != nil {
		return ask.Record{}, false, err
	}
	entries := make([]json.RawMessage, 0, len(branch))
	for _, entry := range branch {
		raw, err := json.Marshal(entry)
		if err != nil {
			continue
		}
		entries = append(entries, raw)
	}
	return ask.LoadPending(entries)
}

// announcePendingAsk tells the user, without opening a dialog, that a form was left unanswered.
func announcePendingAsk(ctx sdk.Context) {
	record, found, err := pendingAsk(ctx)
	if err != nil || !found {
		return
	}
	form, err := ask.ParseQuestions(record.Payload)
	if err != nil {
		return
	}
	title := form.Title
	if title == "" {
		title = fmt.Sprintf("%d question(s)", len(form.Questions))
	}
	ctx.Notify(noticeText(fmt.Sprintf("An ask_user form (%s) was left unanswered. Nothing was answered for you; use `/workbench ask resume` to answer it or `/workbench ask discard` to drop it.", title), 0), "warning")
}

func resolveAsk(ctx sdk.Context, record ask.Record, state string, result any) error {
	terminal := record
	terminal.State, terminal.Payload, terminal.UpdatedAt = state, nil, time.Now().UTC()
	if result != nil {
		encoded, err := json.Marshal(result)
		if err != nil {
			return err
		}
		terminal.Result = encoded
	}
	return ctx.AppendEntry("pig-workbench.ask", terminal)
}

// askCommand implements /workbench ask [show|resume|discard].
func (a *app) askCommand(ctx sdk.Context, args string) error {
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	record, found, err := pendingAsk(ctx)
	if err != nil {
		return err
	}
	if !found {
		a.notify(ctx, emptyState("unanswered ask_user forms on this branch", ""))
		return nil
	}
	form, err := ask.ParseQuestions(record.Payload)
	if err != nil {
		return errors.New("the pending form cannot be read; discard it with `/workbench ask discard`")
	}
	switch strings.TrimSpace(args) {
	case "", "show":
		var out strings.Builder
		fmt.Fprintf(&out, "Pending ask_user form %s (%s)\n", shortID(record.AskID), form.Title)
		for _, question := range form.Questions {
			fmt.Fprintf(&out, "- %s [%s]: %s\n", question.ID, question.Type, question.Prompt)
		}
		a.notify(ctx, out.String())
		return nil
	case "discard":
		if err := resolveAsk(ctx, record, ask.StateCancelled, nil); err != nil {
			return err
		}
		ctx.Notify("Discarded the pending ask_user form; the model was not told.", "info")
		return nil
	case "resume":
		timeout := time.Duration(0)
		if ctx.Mode() == "rpc" {
			timeout = time.Duration(s.settingInt("ask.remote.timeout-ms", 300000)) * time.Millisecond
		}
		result, err := ask.Run(work, ask.Deps{SessionID: s.id, Append: func(kind string, value any) error { return ctx.AppendEntry(kind, value) }, Interactor: ask.NewDialogInteractor(nativeDialogs{host: ctx}), Store: questionSecrets{host: ctx, cwd: s.location.Current().Cwd}, Options: ask.Options{DialogTimeout: timeout}}, form)
		if err != nil {
			return err
		}
		if result.Unavailable != "" {
			ctx.Notify("Interactive dialogs are unavailable here; the form stays pending.", "warning")
			return nil
		}
		state := ask.StateCompleted
		switch {
		case result.Cancelled:
			state = ask.StateCancelled
		case result.TimedOut:
			state = ask.StateTimedOut
		}
		if err := resolveAsk(ctx, record, state, result); err != nil {
			return err
		}
		trigger := true
		return ctx.SendMessage("pig-workbench.ask", recoveredAnswers(form, result), true, sdk.SendMessageOptions{DeliverAs: "followUp", TriggerTurn: &trigger})
	}
	return errors.New("usage: /workbench ask [show|resume|discard]")
}

const recoveredAnswersIntro = "Answers to the ask_user form that was left unanswered"

// recoveredAnswers states a recovered form's outcome with one "label [id]: answer" line per question.
func recoveredAnswers(form ask.Form, result ask.Result) string {
	head := recoveredAnswersIntro
	if form.Title != "" {
		head += " (" + form.Title + ")"
	}
	switch {
	case result.Cancelled:
		return head + ": the user cancelled it. " + result.Reason
	case result.TimedOut:
		return head + ": it timed out. " + result.Reason
	}
	var out strings.Builder
	out.WriteString(head + ":")
	for _, question := range form.Questions {
		answer, ok := result.Answers[question.ID]
		if !ok {
			continue
		}
		out.WriteString("\n" + question.Label + " [" + question.ID + "]: " + answerLine(answer))
	}
	return out.String()
}

// answerLine states one answer on one line: chosen values, custom text or the secret's placeholder, then the note.
func answerLine(answer ask.Answered) string {
	parts := append([]string(nil), answer.Values...)
	for _, extra := range []string{answer.Custom, answer.Placeholder} {
		if extra != "" {
			parts = append(parts, extra)
		}
	}
	line := strings.Join(parts, ", ")
	if answer.Note != "" {
		line += " (note: " + answer.Note + ")"
	}
	return strings.ReplaceAll(line, "\n", " ")
}
