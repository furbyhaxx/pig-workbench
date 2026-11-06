package sessionstore

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func fixture(t *testing.T, files map[string]string) (bucket, target string) {
	t.Helper()
	root := t.TempDir()
	bucket = filepath.Join(root, "sessions", "--work-app--")
	target = filepath.Join(root, "projects", "prj-1", "sessions")
	if err := os.MkdirAll(bucket, 0o700); err != nil {
		t.Fatal(err)
	}
	for name, text := range files {
		if err := os.WriteFile(filepath.Join(bucket, name), []byte(text), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return bucket, target
}

func visible(t *testing.T, dir string) map[string]string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	out := map[string]string{}
	for _, entry := range entries {
		data, err := os.ReadFile(filepath.Join(dir, entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		out[entry.Name()] = string(data)
	}
	return out
}

func TestConvertSharesEveryFileThroughASymlink(t *testing.T) {
	bucket, target := fixture(t, map[string]string{"a.jsonl": "A\n", "b.jsonl": "B\n"})
	result, err := Convert(bucket, target, Hooks{})
	if err != nil || !result.Converted || result.Linked != 2 {
		t.Fatalf("result = %+v, err = %v", result, err)
	}
	if !isSymlinkTo(bucket, target) {
		t.Fatal("the bucket must be a symlink to the project storage")
	}
	if got := visible(t, bucket); len(got) != 2 || got["a.jsonl"] != "A\n" {
		t.Fatalf("sessions through the bucket: %v", got)
	}
	if _, err := os.Lstat(bucket + oldSuffix); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("the old bucket directory must be gone")
	}
	if _, err := os.Stat(JournalPath(bucket, target)); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("a finished conversion leaves no journal")
	}
	again, err := Convert(bucket, target, Hooks{})
	if err != nil || !again.Already {
		t.Fatalf("a second conversion must be a no-op: %+v %v", again, err)
	}
}

func TestAnOpenSessionFileKeepsWorkingAcrossTheConversion(t *testing.T) {
	bucket, target := fixture(t, map[string]string{"live.jsonl": "one\n"})
	open, err := os.OpenFile(filepath.Join(bucket, "live.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer open.Close()
	if _, err := Convert(bucket, target, Hooks{}); err != nil {
		t.Fatal(err)
	}
	if _, err := open.WriteString("two\n"); err != nil {
		t.Fatal(err)
	}
	appendByPath, err := os.OpenFile(filepath.Join(bucket, "live.jsonl"), os.O_APPEND|os.O_WRONLY, 0)
	if err != nil {
		t.Fatalf("the path must still resolve: %v", err)
	}
	appendByPath.WriteString("three\n")
	appendByPath.Close()
	if got := visible(t, target)["live.jsonl"]; got != "one\ntwo\nthree\n" {
		t.Fatalf("appends before and after the swap must land in one file, got %q", got)
	}
}

func TestEveryInterruptedStepResumesWithoutLosingOrDuplicatingASession(t *testing.T) {
	steps := []string{"linked:a.jsonl", "linked:b.jsonl", "journal-linked", "link-created", "swapped", "cleaned:a.jsonl", "cleaned:b.jsonl"}
	for _, step := range steps {
		t.Run(step, func(t *testing.T) {
			bucket, target := fixture(t, map[string]string{"a.jsonl": "A\n", "b.jsonl": "B\n"})
			crash := errors.New("crash")
			hooks := Hooks{At: func(at string) error {
				if at == step {
					return crash
				}
				return nil
			}}
			if _, err := Convert(bucket, target, hooks); !errors.Is(err, crash) {
				t.Fatalf("expected the simulated crash, got %v", err)
			}
			journals := Pending(filepath.Dir(filepath.Dir(target)))
			if len(journals) != 1 {
				t.Fatalf("an interrupted conversion keeps its journal, found %v", journals)
			}
			result, err := Resume(journals[0], Hooks{})
			if err != nil || !result.Converted {
				t.Fatalf("resume = %+v, %v", result, err)
			}
			if !isSymlinkTo(bucket, target) {
				t.Fatal("bucket is not the symlink after recovery")
			}
			got := visible(t, bucket)
			if len(got) != 2 || got["a.jsonl"] != "A\n" || got["b.jsonl"] != "B\n" {
				t.Fatalf("sessions after recovery: %v", got)
			}
			if _, err := os.Lstat(bucket + oldSuffix); !errors.Is(err, os.ErrNotExist) {
				t.Fatal("recovery must remove the old directory")
			}
			if len(Pending(filepath.Dir(filepath.Dir(target)))) != 0 {
				t.Fatal("recovery must clear the journal")
			}
		})
	}
}

func TestAFileCreatedDuringTheSwapIsMovedIntoTheTarget(t *testing.T) {
	bucket, target := fixture(t, map[string]string{"a.jsonl": "A\n"})
	hooks := Hooks{At: func(at string) error {
		if at == "swapped" {
			return os.WriteFile(filepath.Join(bucket+oldSuffix, "late.jsonl"), []byte("L\n"), 0o600)
		}
		return nil
	}}
	if _, err := Convert(bucket, target, hooks); err != nil {
		t.Fatal(err)
	}
	if got := visible(t, bucket); got["late.jsonl"] != "L\n" || got["a.jsonl"] != "A\n" {
		t.Fatalf("a file written between linking and the swap must survive: %v", got)
	}
}

func TestConvertRefusesInsteadOfGuessing(t *testing.T) {
	t.Run("same name, different content", func(t *testing.T) {
		bucket, target := fixture(t, map[string]string{"a.jsonl": "mine\n"})
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "a.jsonl"), []byte("theirs\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Convert(bucket, target, Hooks{}); !errors.Is(err, ErrCollision) {
			t.Fatalf("got %v", err)
		}
		if info, err := os.Lstat(bucket); err != nil || !info.IsDir() {
			t.Fatal("a refused conversion must leave the bucket a directory")
		}
		if visible(t, bucket)["a.jsonl"] != "mine\n" || visible(t, target)["a.jsonl"] != "theirs\n" {
			t.Fatal("neither file may be touched")
		}
	})
	t.Run("a subdirectory in the bucket", func(t *testing.T) {
		bucket, target := fixture(t, map[string]string{"a.jsonl": "A\n"})
		if err := os.Mkdir(filepath.Join(bucket, "nested"), 0o700); err != nil {
			t.Fatal(err)
		}
		if _, err := Convert(bucket, target, Hooks{}); !errors.Is(err, ErrUnexpectedEntry) {
			t.Fatalf("got %v", err)
		}
	})
	t.Run("a symlink to somewhere else", func(t *testing.T) {
		bucket, target := fixture(t, nil)
		if err := os.Remove(bucket); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(t.TempDir(), bucket); err != nil {
			t.Fatal(err)
		}
		if _, err := Convert(bucket, target, Hooks{}); !errors.Is(err, ErrForeignLink) {
			t.Fatalf("got %v", err)
		}
	})
}

func TestEncodeBucketMatchesPiGNaming(t *testing.T) {
	if got := EncodeBucket("/tmp/wb-symprobe/work/repo"); got != "--tmp-wb-symprobe-work-repo--" {
		t.Fatalf("got %q", got)
	}
	if got := EncodeBucket(`C:\Users\me\app`); !strings.HasPrefix(got, "--C-") {
		t.Fatalf("got %q", got)
	}
}
