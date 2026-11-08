package host

import (
	"encoding/json"
	"strings"
	"testing"
)

func TestStoredDisplaySchemasKeepNewestBranchRecordAndRejectInvalidIdentity(t *testing.T) {
	descriptor := buildPresentationDescriptor
	entry := func(id, parent, owner, key string) json.RawMessage {
		data, _ := json.Marshal(map[string]any{"version": 1, "owner": owner, "key": key, "descriptor": json.RawMessage(descriptor)})
		raw, _ := json.Marshal(map[string]any{"id": id, "parentId": parent, "type": "custom", "customType": displaySchemaEntry, "data": json.RawMessage(data)})
		return raw
	}
	leaf := "b"
	records, err := storedDisplaySchemas(t.Context(), []json.RawMessage{entry("a", "", "tools", "build_run"), entry("b", "a", "tools", "build_run")}, &leaf)
	if err != nil || len(records) != 1 {
		t.Fatalf("branch replay duplicated or lost a schema: %v %v", records, err)
	}
	leaf = "c"
	if _, err := storedDisplaySchemas(t.Context(), []json.RawMessage{entry("c", "", "bad\nowner", "build_run")}, &leaf); err == nil {
		t.Fatal("control character in retained identity was accepted")
	}
}

func TestRetainedDisplaySchemaRefusesProtectedValuesInsteadOfPersistingThem(t *testing.T) {
	boundary, _ := testBoundary(t)
	guard, enabled, err := boundary.Guard()
	if err != nil || !enabled {
		t.Fatal(err)
	}
	token := "ghp_" + strings.Repeat("d1S2", 6)
	descriptor := strings.Replace(buildPresentationDescriptor, "Build", token, 1)
	if _, err := retainedDisplaySchema(guard, true, storedDisplaySchema{Version: 1, Owner: "tools", Key: "build_run", Descriptor: json.RawMessage(descriptor)}); err == nil {
		t.Fatal("descriptor containing a known secret shape was retained")
	}
}
