package host

import (
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"os"
	"path/filepath"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/project"
)

// claimProjectOffer records that the repository at root was offered a project and reports whether
// this call was the first; the exclusive create keeps concurrent sessions from both announcing it.
func claimProjectOffer(dir, root string) (bool, error) {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return false, err
	}
	sum := sha256.Sum256([]byte(root))
	file, err := os.OpenFile(filepath.Join(dir, hex.EncodeToString(sum[:16])), os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if errors.Is(err, os.ErrExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	_, err = file.WriteString(root + "\n")
	return err == nil, errors.Join(err, file.Close())
}

// offerProject tells the user once per repository that no project recognises it. It never creates one.
func (a *app) offerProject(ctx sdk.Context, s *session) {
	if !ctx.HasUI() {
		return
	}
	cwd := s.location.Current().Cwd
	detected, err := s.projects.Detect(requestContext{ctx}, cwd)
	if err != nil || detected.How != "global" {
		return
	}
	root, err := project.RepositoryRoot(requestContext{ctx}, cwd)
	if err != nil || root == "" {
		return
	}
	base, err := stateDir()
	if err != nil {
		return
	}
	if first, err := claimProjectOffer(filepath.Join(base, "project-offers"), root); err != nil || !first {
		return
	}
	ctx.Notify("This repository belongs to no project yet (working in global). Run /workbench projects to create one or join an existing project.", "info")
}
