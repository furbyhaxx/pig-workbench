package sessionstore

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestMain lets the test binary act as the converting process that is killed mid-step.
func TestMain(m *testing.M) {
	if bucket := os.Getenv("SESSIONSTORE_CRASH_BUCKET"); bucket != "" {
		step := os.Getenv("SESSIONSTORE_CRASH_STEP")
		_, _ = Convert(bucket, os.Getenv("SESSIONSTORE_CRASH_TARGET"), Hooks{At: func(at string) error {
			if at == step {
				os.Exit(3)
			}
			return nil
		}})
		os.Exit(0)
	}
	os.Exit(m.Run())
}

func TestARealProcessDeathAtEachStepIsRecovered(t *testing.T) {
	for _, step := range []string{"linked:a.jsonl", "journal-linked", "link-created", "swapped", "cleaned:a.jsonl"} {
		t.Run(step, func(t *testing.T) {
			bucket, target := fixture(t, map[string]string{"a.jsonl": "A\n", "b.jsonl": "B\n"})
			cmd := exec.Command(os.Args[0], "-test.run=NONE")
			cmd.Env = append(os.Environ(), "SESSIONSTORE_CRASH_BUCKET="+bucket, "SESSIONSTORE_CRASH_TARGET="+target, "SESSIONSTORE_CRASH_STEP="+step)
			err := cmd.Run()
			if exit, ok := err.(*exec.ExitError); !ok || exit.ExitCode() != 3 {
				t.Fatalf("the child must die at %s, got %v", step, err)
			}
			journals := Pending(filepath.Dir(filepath.Dir(target)))
			if len(journals) != 1 {
				t.Fatalf("journals = %v", journals)
			}
			if _, err := Resume(journals[0], Hooks{}); err != nil {
				t.Fatal(err)
			}
			if got := visible(t, bucket); len(got) != 2 || got["a.jsonl"] != "A\n" || got["b.jsonl"] != "B\n" {
				t.Fatalf("sessions after a killed conversion: %v", got)
			}
			if names, _ := os.ReadDir(target); len(names) != 2 {
				t.Fatalf("the target must hold exactly the two sessions, has %d", len(names))
			}
		})
	}
}
