package api

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"
)

func TestRequestValidation(t *testing.T) {
	request := map[string]any{"v": 1, "id": "request", "sessionId": "session", "from": map[string]any{"owner": "producer", "instance": "instance"}, "method": "discover", "params": map[string]any{"majors": []int{1}}}
	got, err := DecodeRequest(request)
	if err != nil {
		t.Fatal(err)
	}
	if got.Method != "discover" {
		t.Fatal("lost method")
	}
	request["surprise"] = true
	if _, err := DecodeRequest(request); err == nil {
		t.Fatal("unknown envelope field accepted")
	}
	delete(request, "surprise")
	request["method"] = "location.get"
	if _, err := DecodeRequest(request); err == nil {
		t.Fatal("accepted service call before discovery")
	}
	request["generation"] = "generation"
	request["timeoutMs"] = 0
	if _, err := DecodeRequest(request); err == nil {
		t.Fatal("explicit zero timeout accepted")
	}
	delete(request, "timeoutMs")
	request["params"] = map[string]any{"large": strings.Repeat("x", ControlBytes)}
	_, err = DecodeRequest(request)
	var protocol *Error
	if !errors.As(err, &protocol) || protocol.Code != "limitExceeded" {
		t.Fatalf("missing size limit: %v", err)
	}
}

func TestClosedParams(t *testing.T) {
	var request DiscoveryRequest
	if err := Decode([]byte(`{"majors":[1],"extra":true}`), &request); err == nil {
		t.Fatal("unknown params accepted")
	}
	if err := Decode([]byte(`{"majors":[1]} {}`), &request); err == nil {
		t.Fatal("trailing JSON accepted")
	}
	data, err := json.Marshal(Reply{Version: 1, ID: "req", To: Identity{"owner", "instance"}, Error: &Error{Code: "conflict", Message: "already registered"}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), `"result"`) {
		t.Fatal("error reply also has result")
	}
}
