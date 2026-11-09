package api

// Location is the working location a Workbench session reports through `discover`
// (capability `location`), `location.get` and `location.changed` (SPEC-0001 §8).
type Location struct {
	Authority   string `json:"authority"`
	Cwd         string `json:"cwd"`
	DisplayCwd  string `json:"displayCwd"`
	ProjectRoot string `json:"projectRoot"`
	// Project is the session's project identity; nil in the builtin global project.
	Project   *Project `json:"project,omitempty"`
	Origin    string   `json:"origin"`
	Revision  uint64   `json:"revision"`
	Available bool     `json:"available"`
}

// LocationChange is the data of a `location.changed` event.
type LocationChange struct {
	SchemaVersion int      `json:"schemaVersion"`
	Previous      Location `json:"previous"`
	Current       Location `json:"current"`
	Reason        string   `json:"reason"`
}
