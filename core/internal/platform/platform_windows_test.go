//go:build windows

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const (
	oldInstanceID = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	newInstanceID = "7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d"
)

func TestReadCurrentPointerStrictFormat(t *testing.T) {
	root := t.TempDir()
	valid := filepath.Join("instances", oldInstanceID+".toml")
	for _, test := range []struct {
		name    string
		content []byte
	}{
		{name: "BOM", content: append([]byte{0xef, 0xbb, 0xbf}, []byte(valid)...)},
		{name: "multiple lines", content: []byte(valid + "\r\n" + valid + "\r\n")},
		{name: "blank second line", content: []byte(valid + "\n\n")},
	} {
		t.Run(test.name, func(t *testing.T) {
			if err := os.WriteFile(filepath.Join(root, "current"), test.content, 0o600); err != nil {
				t.Fatal(err)
			}
			if _, err := ReadCurrentPointer(root); err == nil {
				t.Fatal("invalid current pointer format was accepted")
			}
		})
	}
	if err := os.WriteFile(filepath.Join(root, "current"), []byte(valid+"\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if got, err := ReadCurrentPointer(root); err != nil || got != valid {
		t.Fatalf("valid CRLF pointer = %q, %v", got, err)
	}
}

func TestReadCurrentPointerRejectsNonRegularFile(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "current"), 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := ReadCurrentPointer(root); err == nil {
		t.Fatal("non-regular current pointer was accepted")
	}
}

func TestWriteCurrentPointerReplacesExistingFile(t *testing.T) {
	root := t.TempDir()
	oldTarget := filepath.Join("instances", oldInstanceID+".toml")
	newTarget := filepath.Join("instances", newInstanceID+".toml")
	if err := WriteCurrentPointer(root, oldTarget); err != nil {
		t.Fatal(err)
	}
	if err := WriteCurrentPointer(root, newTarget); err != nil {
		t.Fatal(err)
	}
	got, err := ReadCurrentPointer(root)
	if err != nil || got != newTarget {
		t.Fatalf("current replacement = %q, %v", got, err)
	}
}

func TestRenameAtomicReplacesExistingFile(t *testing.T) {
	root := t.TempDir()
	oldPath := filepath.Join(root, "old")
	newPath := filepath.Join(root, "new")
	if err := os.WriteFile(oldPath, []byte("replacement"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(newPath, []byte("original"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := RenameAtomic(oldPath, newPath); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(newPath)
	if err != nil || string(content) != "replacement" {
		t.Fatalf("replacement content = %q, %v", content, err)
	}
	if _, err := os.Stat(oldPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("source must no longer exist: %v", err)
	}
}

func TestLockFileMutualExclusion(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	second, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		t.Fatal(err)
	}
	defer second.Close()
	if err := LockFile(first); err != nil {
		t.Fatal(err)
	}
	if err := LockFile(second); !errors.Is(err, ErrLocked) {
		t.Fatalf("second lock = %v, want ErrLocked", err)
	}
	if err := ReleaseLock(first); err != nil {
		t.Fatal(err)
	}
	if err := LockFile(second); err != nil {
		t.Fatalf("lock after release = %v", err)
	}
	if err := ReleaseLock(second); err != nil {
		t.Fatal(err)
	}
}

func TestEnsureShimCreatesExpectedCommand(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "gdit.exe")
	if err := os.WriteFile(executable, []byte("fixture"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := EnsureShim(root, executable); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(ShimPath(root))
	if err != nil || string(content) != shimCommand(executable) {
		t.Fatalf("shim content = %q, %v", content, err)
	}
	if created, correct := CheckShim(root, executable); !created || !correct {
		t.Fatalf("CheckShim = %v, %v", created, correct)
	}
	if err := os.Remove(executable); err != nil {
		t.Fatal(err)
	}
	if created, correct := CheckShim(root, executable); !created || correct {
		t.Fatalf("missing executable CheckShim = %v, %v", created, correct)
	}
}

func TestShimCommandEscapesPercent(t *testing.T) {
	command := shimCommand(`C:\Users\100%\gdit.exe`)
	if !strings.Contains(command, `100%%`) {
		t.Fatalf("percent was not escaped: %q", command)
	}
}

func TestWindowsPathHelpers(t *testing.T) {
	root := t.TempDir()
	command := filepath.Join(root, "godot.cmd")
	if err := os.WriteFile(command, []byte("@echo off\r\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if !EqualPath(strings.ToUpper(root), strings.ToLower(root)) {
		t.Fatal("Windows paths must compare case-insensitively")
	}
	if got := FindGodotCommand(root); !EqualPath(got, command) {
		t.Fatalf("FindGodotCommand = %q, want %q", got, command)
	}
}
