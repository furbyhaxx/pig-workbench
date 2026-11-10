package host

// recent is a string-keyed map that drops its oldest entries once it holds limit keys, so a full
// map never loses the state of calls still in flight. Callers provide the locking. A nil map
// reads as empty, like a nil Go map.
type recent[V any] struct {
	limit  int
	order  []string
	values map[string]V
}

func newRecent[V any](limit int) *recent[V] {
	return &recent[V]{limit: limit, values: make(map[string]V)}
}

func (r *recent[V]) get(key string) (V, bool) {
	if r == nil {
		var zero V
		return zero, false
	}
	value, ok := r.values[key]
	return value, ok
}

// put stores value; an existing key keeps its age.
func (r *recent[V]) put(key string, value V) {
	if _, ok := r.values[key]; !ok {
		for len(r.values) >= r.limit && len(r.order) > 0 {
			oldest := r.order[0]
			r.order = r.order[1:]
			delete(r.values, oldest)
		}
		r.order = append(r.order, key)
	}
	r.values[key] = value
}

func (r *recent[V]) remove(key string) {
	if r == nil {
		return
	}
	if _, ok := r.values[key]; !ok {
		return
	}
	delete(r.values, key)
	// Removed keys leave the order list here, so it never outgrows the map by more than its limit.
	if len(r.order) >= 2*r.limit {
		kept := r.order[:0]
		for _, candidate := range r.order {
			if _, live := r.values[candidate]; live {
				kept = append(kept, candidate)
			}
		}
		r.order = kept
	}
}

func (r *recent[V]) reset() {
	clear(r.values)
	r.order = nil
}

func (r *recent[V]) len() int { return len(r.values) }
