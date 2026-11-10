package host

import (
	"errors"
	"fmt"
	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/location"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/config"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

// secretsBoundary owns the key, vault and detector of one session (SPEC-0013 §3, §4.4).
// A failed build is never cached, so the first call after the cause is repaired succeeds.
type secretsBoundary struct {
	settings    *config.Manager
	agentDir    string
	projectPath func() string
	trusted     func() bool
	cwd         func() string
	environ     func() []string
	home        string
	auth        authInventory

	mu          sync.Mutex
	detector    *secrets.Detector
	vault       *secrets.Vault
	names       nameMatcher
	learned     string
	noticed     string
	degraded    string
	keyRepaired string
	gaps        []string
	envSeen     map[string]string
}

func newSecretsBoundary(settings *config.Manager, agentDir string, projectPath func() string, trusted func() bool, cwd func() string) *secretsBoundary {
	home, _ := os.UserHomeDir()
	return &secretsBoundary{settings: settings, agentDir: agentDir, projectPath: projectPath, trusted: trusted, cwd: cwd, environ: os.Environ, home: home}
}

// enabled reports the live `secrets.enabled` setting; a stale or unreadable snapshot counts as enabled.
func (b *secretsBoundary) enabled() bool {
	snapshot, err := b.settings.Current()
	if err != nil {
		return true
	}
	entry, ok := snapshot.Get("secrets.enabled")
	value, isBool := entry.Value.(bool)
	return !ok || !isBool || value
}

// Guard returns the detector, building it on first use. enabled=false means the user turned the feature off.
func (b *secretsBoundary) Guard() (guard *secrets.Detector, enabled bool, err error) {
	if !b.enabled() {
		return nil, false, nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detector != nil {
		return b.detector, true, nil
	}
	detector, vault, names, err := b.build()
	if err != nil {
		return nil, true, err
	}
	b.detector, b.vault, b.names, b.learned, b.noticed, b.envSeen = detector, vault, names, "", "", nil
	if err := b.learnLocked(); err != nil {
		b.detector = nil
		return nil, true, err
	}
	return detector, true, nil
}

// Withheld returns the user-facing reason output was withheld and whether this cause is new
// since the last success, so a failing boundary notifies once instead of per request.
func (b *secretsBoundary) Withheld(err error) (reason string, fresh bool) {
	reason = "Secrets boundary unavailable, so output was withheld: " + err.Error() + ". Repair it, run /workbench settings reload, then retry."
	b.mu.Lock()
	defer b.mu.Unlock()
	fresh = b.noticed != reason
	b.noticed = reason
	return reason, fresh
}

// learnLocked stores the environment and .env secrets for the current cwd once per cwd.
func (b *secretsBoundary) learnLocked() error {
	cwd := b.cwd()
	if b.learned == cwd {
		return nil
	}
	if err := learnEnvironment(b.detector, b.names, b.environ(), b.home, cwd); err != nil {
		return err
	}
	if b.auth != nil {
		results, gaps := b.auth()
		b.gaps = gaps
		if err := learnAuth(b.detector, b.names, results); err != nil {
			return err
		}
	}
	b.learned = cwd
	return nil
}

// LearnReferenced stores the secrets of every env file a tool call names before the call runs,
// so its result is redacted on the way back (SPEC-0013 §2). A file is parsed again only after it changed.
func (b *secretsBoundary) LearnReferenced(cwd string, input map[string]any) error {
	files := referencedEnvFiles(cwd, b.home, input)
	if len(files) == 0 {
		return nil
	}
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detector == nil {
		return nil
	}
	for _, file := range files {
		info, err := os.Stat(file)
		if err != nil {
			continue
		}
		stamp := fmt.Sprintf("%d/%d", info.Size(), info.ModTime().UnixNano())
		if b.envSeen[file] == stamp {
			continue
		}
		if err := learnEnvFile(b.detector, b.names, file); err != nil {
			return err
		}
		if b.envSeen == nil || len(b.envSeen) >= 1024 {
			b.envSeen = make(map[string]string)
		}
		b.envSeen[file] = stamp
	}
	return nil
}

// Gaps lists credential sources the host could not enumerate; detection is incomplete for them.
func (b *secretsBoundary) Gaps() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.gaps...)
}

// Refresh learns the environment secrets of the current cwd after a directory change.
func (b *secretsBoundary) Refresh() error {
	b.mu.Lock()
	defer b.mu.Unlock()
	if b.detector == nil {
		return nil
	}
	return b.learnLocked()
}

// Reload drops the detector so the next use rereads Secrets.toml.
func (b *secretsBoundary) Reload() {
	b.mu.Lock()
	b.detector, b.vault = nil, nil
	b.mu.Unlock()
}

func (b *secretsBoundary) build() (*secrets.Detector, *secrets.Vault, nameMatcher, error) {
	keyPath := filepath.Join(b.agentDir, "secret.key")
	master, repaired, err := secrets.LoadOrCreateKey(keyPath)
	if err != nil {
		var permission *secrets.KeyPermissionError
		if errors.As(err, &permission) {
			return nil, nil, nameMatcher{}, err
		}
		return nil, nil, nameMatcher{}, errors.New("secret key unavailable; inspect the key file and its permissions")
	}
	if repaired {
		b.keyRepaired = keyPath
	}
	defer clear(master)
	vaultPath, err := secrets.VaultPath()
	if err != nil {
		return nil, nil, nameMatcher{}, errors.New("private secret data directory unavailable")
	}
	vault, err := secrets.OpenVault(vaultPath, master)
	if err != nil {
		return nil, nil, nameMatcher{}, errors.New("secret vault unavailable")
	}
	detection, err := config.ReadDetection(filepath.Join(b.agentDir, "Secrets.toml"), b.projectPath(), b.trusted())
	if err != nil {
		return nil, nil, nameMatcher{}, err
	}
	detector, err := secrets.NewDetector(master, vault, policyOf(detection))
	if err != nil {
		return nil, nil, nameMatcher{}, err
	}
	return detector, vault, newNameMatcher(detection), nil
}

func policyOf(detection config.Detection) secrets.DetectionPolicy {
	policy := secrets.DetectionPolicy{Exclusions: secrets.Exclusions{Values: detection.ExcludedValues, Hashes: detection.ExcludedHashes, Patterns: detection.ExcludedPatterns}}
	for _, pattern := range detection.Patterns {
		policy.Patterns = append(policy.Patterns, secrets.Pattern{Name: pattern.Name, Regex: pattern.Regex, Prefix: pattern.Prefix, Suffix: pattern.Suffix, MinLength: pattern.MinLength})
	}
	return policy
}

func newHostSecretsBoundary(ctx sdk.Context, settings *config.Manager, agentDir string, locations *location.Manager) *secretsBoundary {
	project := func() string {
		dirname, err := sdk.ConfigDirName()
		if err != nil {
			return ""
		}
		return filepath.Join(ctx.Cwd(), dirname, "Secrets.toml")
	}
	trusted := func() bool { ok, err := ctx.IsProjectTrusted(); return err == nil && ok }
	boundary := newSecretsBoundary(settings, agentDir, project, trusted, func() string { return locations.Current().Cwd })
	boundary.auth = hostAuthInventory(ctx)
	return boundary
}

// UnreadableNotice reports once what the boundary repaired or ignored on its own: a key file it
// narrowed to 0600, and vault records the current key cannot decrypt. Empty means nothing new.
func (b *secretsBoundary) UnreadableNotice() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var parts []string
	if path := b.keyRepaired; path != "" {
		b.keyRepaired = ""
		parts = append(parts, fmt.Sprintf("%s was readable by other users; its permissions were narrowed to 0600.", path))
	}
	if b.detector != nil {
		unreadable := b.detector.Unreadable()
		if key := strings.Join(unreadable, ","); key != b.degraded {
			b.degraded = key
			if len(unreadable) > 0 {
				parts = append(parts, fmt.Sprintf("%d secret vault record(s) cannot be decrypted with this key (another agent directory or a replaced secret.key wrote them) and are ignored; they are left in the vault untouched and your secrets are re-learned under the current key.", len(unreadable)))
			}
		}
	}
	return strings.Join(parts, " ")
}
