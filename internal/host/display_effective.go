package host

import (
	"errors"
	"sync"

	"github.com/furbyhaxx/pig-workbench/internal/config"
)

type effectiveDisplay struct {
	enabled  bool
	density  string
	preview  int
	metrics  map[string]bool
	shorten  bool
	revision uint64
}

type displayCache struct {
	mu    sync.Mutex
	value effectiveDisplay
}

func (c *displayCache) get(s *session, entry presentationRegistration) (effectiveDisplay, error) {
	snapshot, err := s.settings.Current()
	if err != nil {
		return effectiveDisplay{}, err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.value.revision == snapshot.Revision() {
		return c.value, nil
	}
	value, err := displayFromSnapshot(snapshot, entry)
	if err == nil {
		c.value = value
	}
	return value, err
}

func (s *session) effectiveDisplay(entry presentationRegistration) (effectiveDisplay, error) {
	snapshot, err := s.settings.Current()
	if err != nil {
		return effectiveDisplay{}, err
	}
	return displayFromSnapshot(snapshot, entry)
}

func displayFromSnapshot(snapshot *config.Snapshot, entry presentationRegistration) (effectiveDisplay, error) {
	get := func(field string) (config.Entry, bool) {
		return snapshot.Get(displayKey(entry.owner, entry.key, field))
	}
	enabled, ok := get("enabled")
	if !ok || enabled.Application == "unavailable" {
		return effectiveDisplay{}, errors.New("presentation display schema unavailable")
	}
	density, _ := get("density")
	preview, _ := get("preview-lines")
	metrics, _ := get("metrics")
	shorten, _ := get("target-shortening")
	result := effectiveDisplay{enabled: enabled.Value.(bool), density: density.Value.(string), preview: int(preview.Value.(int64)), metrics: map[string]bool{}, shorten: shorten.Value != false, revision: snapshot.Revision()}
	for _, id := range metrics.Value.([]string) {
		result.metrics[id] = true
	}
	return result, nil
}
