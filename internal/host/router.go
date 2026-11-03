package host

import (
	"os"
	"path/filepath"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/resource"
)

func buildRouter(ctx sdk.Context, settings *config.Manager) (*resource.Router, string, error) {
	router := resource.NewRouter()
	if err := router.RegisterBuiltin(resource.FileProvider{}); err != nil {
		return nil, "", err
	}
	if err := router.RegisterBuiltin(cfgProvider{source: func() (*config.Snapshot, config.Status) {
		return settings.Inspect()
	}}); err != nil {
		return nil, "", err
	}
	cache, err := os.UserCacheDir()
	if err != nil {
		ctx.Notify("Local scratch and spill storage are unavailable; local:// and out:// cannot be read this session.", "warning")
		return router, "", nil
	}
	scratch := filepath.Join(cache, "pig-workbench", "scratch", sanitizeSegment(sessionKey(ctx)))
	local, err := resource.NewLocalProvider(scratch)
	if err != nil {
		return nil, "", err
	}
	if err := router.RegisterBuiltin(local); err != nil {
		return nil, "", err
	}
	out, err := resource.NewOutStore(filepath.Join(scratch, "out"), 64<<20)
	if err != nil {
		return nil, "", err
	}
	if err := router.RegisterBuiltin(resource.OutProvider{Store: out}); err != nil {
		return nil, "", err
	}
	return router, scratch, nil
}

func sessionKey(ctx sdk.Context) string {
	if id, err := ctx.GetSessionID(); err == nil && id != "" {
		return id
	}
	return "nosession"
}

func sanitizeSegment(value string) string {
	cleaned := strings.Map(func(r rune) rune {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			return r
		default:
			return '-'
		}
	}, value)
	if cleaned == "" {
		return "session"
	}
	return cleaned
}
