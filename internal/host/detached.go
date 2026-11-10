package host

import sdk "github.com/MichaelKinsy/PiG/extensions/sdk"

// detached runs background work that uses the retained session-start context. The host
// invalidates that context on reload or replacement and its getters then panic with the stale
// message; that one panic ends the work quietly, every other panic propagates.
func detached(host sdk.Context, work func()) (stale bool) {
	return detachedWhile(func() bool { return contextActive(host) }, work)
}

func detachedWhile(active func() bool, work func()) (stale bool) {
	defer func() {
		if failure := recover(); failure != nil {
			if active() {
				panic(failure)
			}
			stale = true
		}
	}()
	work()
	return false
}

// contextActive probes with a getter, the only staleness signal the SDK exposes.
func contextActive(host sdk.Context) (active bool) {
	if host == (sdk.Context{}) {
		return true
	}
	defer func() {
		if recover() != nil {
			active = false
		}
	}()
	host.Mode()
	return true
}
