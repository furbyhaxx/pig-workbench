package host

import (
	"encoding/json"
	"os"
	"testing"
)

func TestReportedVersionIsThePackageVersion(t *testing.T) {
	raw, err := os.ReadFile("../../package.json")
	if err != nil {
		t.Fatal(err)
	}
	var manifest struct {
		Version string `json:"version"`
	}
	if err := json.Unmarshal(raw, &manifest); err != nil {
		t.Fatal(err)
	}
	if manifest.Version == "" || manifest.Version != workbenchVersion {
		t.Fatalf("discover reports %q, package.json says %q", workbenchVersion, manifest.Version)
	}
}
