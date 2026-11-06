package host

import (
	"sort"
	"strings"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
	"github.com/furbyhaxx/pig-workbench/internal/secrets"
)

// authInventory returns the resolved auth result of every provider that holds a credential,
// as the host resolved it. Workbench never executes credential commands itself.
type authInventory func() (results map[string]map[string]any, gaps []string)

// hostAuthInventory resolves credentials through the PiG model runtime and registry.
func hostAuthInventory(ctx sdk.Context) authInventory {
	return func() (map[string]map[string]any, []string) {
		var gaps []string
		providers := map[string]struct{}{}
		off := false
		if runtime, err := sdk.ModelRuntimeCreate(ctx, sdk.ModelRuntimeCreateOptions{RefreshOnCreate: &off}); err != nil {
			gaps = append(gaps, "credential list unavailable: "+err.Error())
		} else if credentials, err := runtime.ListCredentials(); err != nil {
			gaps = append(gaps, "credential list unavailable: "+err.Error())
		} else {
			for _, credential := range credentials {
				providers[credential.ProviderID] = struct{}{}
			}
		}
		if models, err := ctx.ModelRegistry().GetAll(); err != nil {
			gaps = append(gaps, "model providers unavailable: "+err.Error())
		} else {
			for _, model := range models {
				if provider, _ := model["provider"].(string); provider != "" {
					providers[provider] = struct{}{}
				}
			}
		}
		results := make(map[string]map[string]any, len(providers))
		for provider := range providers {
			if result, err := ctx.ModelRegistry().GetProviderAuth(provider); err == nil && result != nil {
				results[provider] = result
			}
		}
		return results, gaps
	}
}

// learnAuth stores the API keys and secret-named headers of resolved provider auth.
// A value that still starts with "!" is an unresolved command and is never learned.
func learnAuth(detector *secrets.Detector, matcher nameMatcher, results map[string]map[string]any) error {
	providers := make([]string, 0, len(results))
	for provider := range results {
		providers = append(providers, provider)
	}
	sort.Strings(providers)
	learn := func(value any) error {
		text, _ := value.(string)
		if text == "" || strings.HasPrefix(text, "!") {
			return nil
		}
		return detector.Learn([]byte(text), "token", "auth")
	}
	for _, provider := range providers {
		auth, _ := results[provider]["auth"].(map[string]any)
		if err := learn(auth["apiKey"]); err != nil {
			return err
		}
		headers, _ := auth["headers"].(map[string]any)
		for name, value := range headers {
			lower := strings.ToLower(name)
			if lower == "authorization" || lower == "x-api-key" || matcher.secret(name) {
				if err := learn(strings.TrimPrefix(asText(value), "Bearer ")); err != nil {
					return err
				}
			}
		}
	}
	return nil
}

func asText(value any) string {
	text, _ := value.(string)
	return text
}
