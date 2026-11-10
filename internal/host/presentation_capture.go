package host

import (
	"maps"

	sdk "github.com/MichaelKinsy/PiG/extensions/sdk"
)

func (a *app) capturePresentationFinal(_ sdk.Context, data map[string]any) (any, error) {
	a.capturePresentation(data, data, true)
	return nil, nil
}

func (a *app) capturePresentationUpdate(ctx sdk.Context, data map[string]any) (any, error) {
	if partial, ok := data["partialResult"].(map[string]any); ok {
		s, err := a.current()
		if err != nil {
			return nil, nil
		}
		patch, cause := s.secrets.resultPatch(partial)
		if patch != nil {
			protected := maps.Clone(partial)
			for field, value := range patch.(map[string]any) {
				protected[field] = value
			}
			partial = protected
		}
		if cause != nil {
			s.secrets.withhold(ctx, cause)
		}
		a.capturePresentation(data, partial, false)
	}
	return nil, nil
}

func (a *app) capturePresentation(data, result map[string]any, final bool) {
	s, err := a.current()
	if err != nil {
		return
	}
	tool, _ := data["toolName"].(string)
	callID, _ := data["toolCallId"].(string)
	if tool == "" || callID == "" {
		return
	}
	entry, ok := s.presentations.lookup(tool)
	if !ok {
		return
	}
	if !final {
		s.viewsMu.Lock()
		name, _ := s.runningCalls.get(callID)
		running := name == tool
		s.viewsMu.Unlock()
		if !running {
			return
		}
	}
	s.presentationData.observe(presentationCallKey{Owner: entry.owner, Key: entry.key, CallID: callID}, tool, entry.descriptor, result["details"], final)
}

func (a *app) preloadPresentationHistory(_ sdk.Context, data map[string]any) (any, error) {
	s, err := a.current()
	if err != nil {
		return nil, nil
	}
	messages, _ := data["messages"].([]any)
	loaded := 0
	for i := len(messages) - 1; i >= 0 && loaded < 256; i-- {
		message, ok := messages[i].(map[string]any)
		if !ok || message["role"] != "toolResult" {
			continue
		}
		tool, _ := message["toolName"].(string)
		callID, _ := message["toolCallId"].(string)
		entry, ok := s.presentations.lookup(tool)
		if !ok || callID == "" {
			continue
		}
		key := presentationCallKey{Owner: entry.owner, Key: entry.key, CallID: callID}
		if _, exists := s.presentationData.lookup(key); exists {
			continue
		}
		s.presentationData.observe(key, tool, entry.descriptor, message["details"], true)
		loaded++
	}
	return nil, nil
}

func (s *session) presentationView(entry presentationRegistration, tool, callID string, details any, final bool) presentationRecord {
	if callID == "" {
		// Unidentified cards cannot safely share lifecycle state with another call.
		snapshot, err := decodeToolSnapshot(entry.descriptor, details)
		return presentationRecord{snapshot: snapshot, valid: err == nil && (!final || snapshot.Phase == "settled")}
	}
	key := presentationCallKey{Owner: entry.owner, Key: entry.key, CallID: callID}
	if record, ok := s.presentationData.lookup(key); ok {
		return record
	}
	s.presentationData.observe(key, tool, entry.descriptor, details, final)
	record, _ := s.presentationData.lookup(key)
	return record
}
