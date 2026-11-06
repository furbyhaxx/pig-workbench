package host

// promptOverride replaces PiG's prompt only for an explicit force delivery.
// auto stays on PiG until the approved A/B run.
func promptOverride(enabled bool, delivery, rendered string) (string, bool) {
	if !enabled || delivery != "force" || rendered == "" {
		return "", false
	}
	return rendered, true
}
