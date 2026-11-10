package host

import (
	"errors"
	"fmt"
	"slices"
	"strconv"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
)

const framesUsage = "usage: /workbench frames [ls [frame id|#sequence] | show|collapse|hide [source]]"

var frameVisibilityVerbs = map[string]string{"show": "shown", "collapse": "collapsed", "hide": "hidden"}

// frameSourceVisibility returns the visibility of the longest `<visibility> <source prefix>` rule covering source.
func frameSourceVisibility(rules []string, source, fallback string) string {
	best := -1
	for _, rule := range rules {
		visibility, prefix, ok := strings.Cut(rule, " ")
		if !ok || !slices.Contains([]string{"shown", "collapsed", "hidden"}, visibility) {
			continue
		}
		if (source == prefix || strings.HasPrefix(source, prefix+".")) && len(prefix) > best {
			best, fallback = len(prefix), visibility
		}
	}
	return fallback
}

// framesCommand inspects this session's frames and sets how they show in the transcript; delivery is never changed.
func (a *app) framesCommand(ctx sdk.Context, args string) error {
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	verb, target, _ := strings.Cut(strings.TrimSpace(args), " ")
	target = strings.TrimSpace(target)
	provider := sessionProvider{source: func() (*session, bool) { return s, true }}
	records, err := provider.frameRecords(ctx, "self")
	if err != nil {
		return err
	}
	switch verb {
	case "", "ls":
		if target != "" {
			id := target
			if sequence, err := strconv.ParseInt(strings.TrimPrefix(target, "#"), 10, 64); err == nil {
				for _, record := range records {
					if record.Frame.Sequence == sequence {
						id = record.Frame.FrameID
					}
				}
			}
			body, err := provider.renderFrames(ctx, sessionTarget{id: "self", file: "frames", frame: id, depth: 3})
			if err != nil {
				return fmt.Errorf("no frame %s in this session; /workbench frames ls lists them", target)
			}
			a.present(ctx, "Frame "+target, strings.TrimRight(string(body), "\n"))
			return nil
		}
		_, fallback := a.rowDisplay("")
		rules := s.settingStrings("frames.display.sources")
		lines := make([]string, 0, len(records))
		for _, record := range records {
			line := fmt.Sprintf("#%d %s %s [%s]", record.Frame.Sequence, record.Frame.Source, record.Frame.Kind, record.State)
			if record.Frame.Resource != "" {
				line += " " + record.Frame.Resource
			}
			if visibility := frameSourceVisibility(rules, record.Frame.Source, fallback); visibility != fallback {
				line += " (" + visibility + ")"
			}
			lines = append(lines, line)
		}
		if len(lines) == 0 {
			lines = append(lines, emptyState("frames in this session", ""))
		}
		a.notify(ctx, strings.Join(lines, "\n"))
		return nil
	}
	visibility, ok := frameVisibilityVerbs[verb]
	if !ok {
		return errors.New(framesUsage)
	}
	baseline, err := s.settings.Current()
	if err != nil {
		return err
	}
	edit := config.Edit{Key: "frames.display.default-visibility", Value: visibility}
	subject := "Context frames are"
	if target != "" {
		if !producerKeyPattern.MatchString(target) {
			return errors.New("a frame source is a dotted lowercase name such as session.date; /workbench frames ls lists them")
		}
		rules := slices.DeleteFunc(slices.Clone(s.settingStrings("frames.display.sources")), func(rule string) bool {
			_, prefix, _ := strings.Cut(rule, " ")
			return prefix == target
		})
		edit = config.Edit{Key: "frames.display.sources", Value: append(rules, visibility+" "+target)}
		subject = "Frames from " + target + " are"
	}
	result, saveErr := s.settings.Save(work, config.User, baseline, []config.Edit{edit}, ctx.IsProjectTrusted)
	if result.Published {
		a.settingsChanged(ctx, s, result.Revision, result.Changed)
	}
	if saveErr != nil {
		return saveErr
	}
	a.notify(ctx, fmt.Sprintf("%s now %s in the transcript (saved to user settings). The model receives every frame regardless.", subject, visibility))
	return nil
}
