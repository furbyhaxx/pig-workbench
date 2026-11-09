package host

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/endpoint"
	"github.com/furbyhaxx/pig-workbench/internal/project"
	"github.com/furbyhaxx/pig-workbench/internal/sessionstore"
)

// sessionBucket returns the default-directory bucket this session lives in; ok is false for --session-dir and in-memory sessions.
func sessionBucket(ctx sdk.Context) (bucket string, ok bool) {
	if customSessionDir(ctx) != nil {
		return "", false
	}
	file, err := ctx.GetSessionFile()
	if err != nil || file == nil || *file == "" {
		return "", false
	}
	return filepath.Dir(*file), true
}

// otherLiveSessionIn reports a different live session whose working directory maps to the bucket.
// ok is false when the registry cannot be read, which must defer conversion rather than allow it.
func otherLiveSessionIn(ctx context.Context, own, bucketName string) (other string, ok bool) {
	dir, err := endpointDirectory()
	if err != nil {
		return "", false
	}
	registry, err := endpoint.Open(dir)
	if err != nil {
		return "", false
	}
	listed, err := registry.List(ctx)
	if err != nil {
		return "", false
	}
	stale := map[string]bool{}
	for _, id := range listed.Stale {
		stale[id] = true
	}
	for _, record := range listed.Records {
		if record.SessionID != own && !stale[record.SessionID] && sessionstore.EncodeBucket(record.Cwd) == bucketName {
			return record.SessionID, true
		}
	}
	return "", true
}

func projectSessionTarget(agent string, detected project.Detection) string {
	return filepath.Join(agent, "projects", detected.ProjectID, "sessions")
}

// adoptSessionStorage resumes interrupted conversions and links this session's bucket to its project's storage.
func (a *app) adoptSessionStorage(ctx sdk.Context, s *session) {
	if s.settingString("projects.session-storage", "link") != "link" {
		return
	}
	agent, err := sdk.AgentDir()
	if err != nil {
		return
	}
	for _, journal := range sessionstore.Pending(filepath.Join(agent, "projects")) {
		if _, err := sessionstore.Resume(journal, sessionstore.Hooks{}); err != nil {
			ctx.Notify(noticeText("Session storage: an interrupted conversion could not be completed: "+err.Error(), 0), "warning")
		}
	}
	bucket, ok := sessionBucket(ctx)
	if !ok {
		return
	}
	work := context.Background()
	detected, err := s.projects.Detect(work, ctx.Cwd())
	if err != nil || detected.ProjectID == "" || detected.ProjectID == "global" {
		return
	}
	if info, err := os.Lstat(bucket); err != nil || info.Mode()&os.ModeSymlink != 0 {
		return
	}
	if other, known := otherLiveSessionIn(work, s.id, filepath.Base(bucket)); !known {
		ctx.Notify(noticeText("Session storage conversion deferred: the live-session registry cannot be read, so another session in this directory cannot be ruled out.", 0), "info")
		return
	} else if other != "" {
		ctx.Notify(noticeText("Session storage conversion deferred: session "+shortID(other)+" is running in this directory. It will be retried at a later start.", 0), "info")
		return
	}
	target := projectSessionTarget(agent, detected)
	result, err := sessionstore.Convert(bucket, target, sessionstore.Hooks{})
	switch {
	case errors.Is(err, sessionstore.ErrCrossFilesystem):
		ctx.Notify(noticeText("Session storage conversion deferred: the session directory and the project storage are on different filesystems.", 0), "info")
	case err != nil:
		ctx.Notify(noticeText("Session storage conversion refused: "+err.Error(), 0), "warning")
	case result.Converted && !result.Already:
		ctx.Notify(noticeText(fmt.Sprintf("Sessions of project %s now share one directory (%d session files). Undo with /workbench projects unlink-storage.", detected.Slug, result.Linked), 0), "info")
	}
}

// projectsCommand implements /workbench projects.
func (a *app) projectsCommand(ctx sdk.Context, args string) error {
	if strings.TrimSpace(args) == "" && panelsAvailable(ctx) {
		return a.projectsPanel(ctx)
	}
	s, work, release, err := a.admit(ctx)
	if err != nil {
		return err
	}
	defer release()
	verb := strings.TrimSpace(args)
	name, rest, _ := strings.Cut(verb, " ")
	if text, handled, err := s.projectsManage(work, name, rest); handled {
		if err != nil {
			return err
		}
		a.notify(ctx, text)
		return nil
	}
	detected, err := s.projects.Detect(work, s.location.Current().Cwd)
	if err != nil {
		return err
	}
	bucket, bucketOK := sessionBucket(ctx)
	switch verb {
	case "", "status":
		var out strings.Builder
		fmt.Fprintf(&out, "project: %s (%s), recognised by %s\n", detected.Slug, detected.ProjectID, detected.How)
		switch {
		case !bucketOK:
			out.WriteString("session storage: not managed (custom --session-dir or no session file)\n")
		default:
			if info, err := os.Lstat(bucket); err == nil && info.Mode()&os.ModeSymlink != 0 {
				link, _ := os.Readlink(bucket)
				fmt.Fprintf(&out, "session storage: shared, %s -> %s\n", bucket, link)
			} else {
				fmt.Fprintf(&out, "session storage: per-directory, %s\n", bucket)
			}
		}
		a.notify(ctx, out.String())
		return nil
	case "unlink-storage":
		if !bucketOK {
			return errors.New("session storage is not managed for this session (custom --session-dir or no session file); nothing to unlink")
		}
		if detected.ProjectID == "" || detected.ProjectID == "global" {
			return errors.New("this directory belongs to no project, so no shared session storage exists for it")
		}
		agent, err := sdk.AgentDir()
		if err != nil {
			return err
		}
		result, err := sessionstore.Unlink(filepath.Dir(bucket), projectSessionTarget(agent, detected))
		if err != nil {
			return err
		}
		text := fmt.Sprintf("Restored %d session directories as real directories.", len(result.Restored))
		if len(result.Kept) > 0 {
			text += fmt.Sprintf(" %d session file(s) recorded in no restored directory stay in the project storage.", len(result.Kept))
		}
		a.notify(ctx, text+" The running session keeps writing to its file; PiG lists it again after the next resume.")
		return nil
	}
	return errors.New("usage: /workbench projects [status | list | create <slug> [name] | join <id|slug> [file|marker] [component] | example | unlink-storage]")
}
