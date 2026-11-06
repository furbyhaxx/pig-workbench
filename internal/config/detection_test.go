package config

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

func writeDetection(t *testing.T, root, name, text string) string {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestDetectionAppendsProjectPoliciesAfterUserPolicies(t *testing.T) {
	root := t.TempDir()
	user := writeDetection(t, root, "user/Secrets.toml", "version = 2\n[env]\nnames = ['*ACME*']\n[[patterns]]\nname = 'acme'\nprefix = 'acme_'\nmin-length = 20\n[exclusions]\nvalues = ['fixture-value']\nhashes = ['abc']\n")
	project := writeDetection(t, root, "project/Secrets.toml", "[[patterns]]\nname = 'local'\nregex = 'loc-[a-z]{12}'\n[exclusions]\nenv-names = ['ACME_PUBLIC_ID']\npatterns = ['demo-.*']\n")
	got, err := ReadDetection(user, project, true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got.EnvNames, []string{"*ACME*"}) {
		t.Fatalf("env names = %v", got.EnvNames)
	}
	names := []string{got.Patterns[0].Name, got.Patterns[1].Name}
	if !reflect.DeepEqual(names, []string{"acme", "local"}) || got.Patterns[0].MinLength != 20 {
		t.Fatalf("patterns did not append user then project: %+v", got.Patterns)
	}
	if !reflect.DeepEqual(got.ExcludedEnvNames, []string{"ACME_PUBLIC_ID"}) || !reflect.DeepEqual(got.ExcludedPatterns, []string{"demo-.*"}) || !reflect.DeepEqual(got.ExcludedValues, []string{"fixture-value"}) {
		t.Fatalf("exclusions = %+v", got)
	}
}

func TestDetectionIgnoresAProjectPolicyUntilTheProjectIsTrusted(t *testing.T) {
	root := t.TempDir()
	project := writeDetection(t, root, "project/Secrets.toml", "[[patterns]]\nname = 'local'\nprefix = 'loc_'\n")
	got, err := ReadDetection(filepath.Join(root, "absent.toml"), project, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Patterns) != 0 || len(got.EnvNames) == 0 {
		t.Fatalf("untrusted project policy was applied or defaults were lost: %+v", got)
	}
}

func TestDetectionRejectsWhatItCannotHonour(t *testing.T) {
	cases := map[string]struct {
		project bool
		text    string
		want    string
	}{
		"project env names": {true, "[env]\nnames = ['*X*']\n", "may only add patterns and exclusions"},
		"unknown key":       {false, "version = 2\n[excusions]\nvalues = []\n", "excusions"},
		"wrong version":     {false, "version = 1\n", "version 2"},
		"unnamed pattern":   {false, "[[patterns]]\nprefix = 'x_'\n", "needs a name"},
		"wrong type":        {false, "[exclusions]\nvalues = 'one'\n", "wrong type"},
		"syntax":            {false, "[[patterns\n", "invalid TOML"},
	}
	for name, c := range cases {
		root := t.TempDir()
		path := writeDetection(t, root, "Secrets.toml", c.text)
		user, project := path, ""
		if c.project {
			user, project = "", path
		}
		_, err := ReadDetection(user, project, true)
		if err == nil || !strings.Contains(err.Error(), c.want) {
			t.Errorf("%s: error = %v, want it to mention %q", name, err, c.want)
		}
	}
}
