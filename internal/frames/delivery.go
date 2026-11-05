package frames

import "time"

// DeliveryScope identifies the branch leaf and context epoch a delivery belongs to.
type DeliveryScope struct {
	BranchLeaf string `json:"branchLeaf"`
	Epoch      string `json:"epoch"`
	// Ancestors, when set, holds every entry ID on the current branch (and "" for the root).
	// A frame then matches when its publish leaf lies on that branch instead of equalling BranchLeaf.
	Ancestors map[string]bool `json:"-"`
}

// Matches reports whether a frame admitted at leaf and epoch belongs to this scope.
func (s DeliveryScope) Matches(leaf, epoch string) bool {
	if epoch != s.Epoch {
		return false
	}
	if s.Ancestors != nil {
		return s.Ancestors[leaf]
	}
	return leaf == s.BranchLeaf
}

// SelectPending returns the pending frames of one delivery mode in admission order.
// Frames admitted under another branch leaf or epoch are excluded, and expired frames are
// retired before the selection. Selecting does not mark frames delivered.
func (s *Session) SelectPending(delivery string, scope DeliveryScope) ([]Frame, error) {
	if !validDelivery(delivery) {
		return nil, ErrInvalidDelivery
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.clock())
	var frames []Frame
	for _, record := range s.records {
		if record.State == statePending && record.Frame.Delivery == delivery && scope.Matches(record.Frame.BranchLeaf, record.Frame.Epoch) {
			frames = append(frames, record.Frame)
		}
	}
	return frames, nil
}

// MarkDelivered retires selected frames as delivered in scope, atomically for the batch.
// A frame from another branch leaf or epoch reports ErrStaleContext.
func (s *Session) MarkDelivered(scope DeliveryScope, frameIDs []string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.expireLocked(s.clock())
	for _, frameID := range frameIDs {
		record := s.byID[frameID]
		if record == nil || record.State != statePending {
			return ErrInvalidTransition
		}
		if !scope.Matches(record.Frame.BranchLeaf, record.Frame.Epoch) {
			return ErrStaleContext
		}
	}
	for _, frameID := range frameIDs {
		if record := s.byID[frameID]; record.State == statePending {
			s.retireLocked(record, stateDelivered)
		}
	}
	return nil
}

// Cancel retires a pending frame that must not be delivered.
func (s *Session) Cancel(frameID string) error { return s.retire(frameID, stateCancelled) }

// Fail retires a pending frame the host rejected; retrying requires a new publication.
func (s *Session) Fail(frameID string) error { return s.retire(frameID, stateFailed) }

func (s *Session) retire(frameID, next string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	record := s.byID[frameID]
	if record == nil || record.State != statePending {
		return ErrInvalidTransition
	}
	s.retireLocked(record, next)
	return nil
}

// retireLocked leaves a pending frame in a terminal state and releases its pending slot.
func (s *Session) retireLocked(record *storedRecord, next string) {
	record.State = next
	s.count--
	ctx := Context{SessionID: s.sessionID, ProducerPrefix: record.ProducerPrefix, BranchLeaf: record.Frame.BranchLeaf, Epoch: record.Frame.Epoch}
	if key, eligible := coalesceFor(ctx, record.publish()); eligible && s.pending[key] == record {
		delete(s.pending, key)
	}
}

func (s *Session) expireLocked(now time.Time) {
	for _, record := range s.records {
		if record.State == statePending && record.ExpiresAt != nil && !now.Before(*record.ExpiresAt) {
			s.retireLocked(record, stateExpired)
		}
	}
}
