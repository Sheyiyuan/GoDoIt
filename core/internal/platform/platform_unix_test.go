//go:build linux || darwin

package platform

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestWriteCurrentPointerSyncFailureRestoresOldLink(t *testing.T) {
	root := t.TempDir()
	oldID := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	newID := "7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d"
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{oldID, newID} {
		if err := os.WriteFile(filepath.Join(root, "instances", id+".toml"), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := WriteCurrentPointer(root, filepath.Join("instances", oldID+".toml")); err != nil {
		t.Fatal(err)
	}
	// rename 后的父目录 fsync 失败：对调用方返回错误前必须恢复旧链接。
	failSync := func(string) error { return errors.New("injected sync failure") }
	if err := writeCurrentPointerWithSync(root, filepath.Join("instances", newID+".toml"), failSync); err == nil {
		t.Fatal("expected sync failure to be reported")
	}
	target, err := ReadCurrentPointer(root)
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join("instances", oldID+".toml") {
		t.Fatalf("old link must be restored after sync failure: %q", target)
	}
}

func TestWriteCurrentPointerInitialSyncFailureLeavesNoLink(t *testing.T) {
	root := t.TempDir()
	id := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instances", id+".toml"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	failSync := func(string) error { return errors.New("injected sync failure") }
	if err := writeCurrentPointerWithSync(root, filepath.Join("instances", id+".toml"), failSync); err == nil {
		t.Fatal("expected sync failure to be reported")
	}
	if _, err := os.Lstat(filepath.Join(root, "current")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed initial switch must not leave current: %v", err)
	}
}

func TestEnsureShimIdempotent(t *testing.T) {
	root := t.TempDir()
	executable := filepath.Join(root, "bin", "gdit")
	if err := os.MkdirAll(filepath.Dir(executable), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := EnsureShim(root, executable); err != nil {
		t.Fatal(err)
	}
	if !IsShimInvocation(ShimPath(root)) {
		t.Fatal("shim path must be recognized as a shim invocation")
	}
	// 幂等：重复执行不报错，链接不变。
	if err := EnsureShim(root, executable); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(ShimPath(root))
	if err != nil || target != executable {
		t.Fatalf("shim link unexpected: %q err=%v", target, err)
	}
}

func TestRootAccessIssueRejectsReadOnlyDirectory(t *testing.T) {
	root := t.TempDir()
	if err := os.Chmod(root, 0o500); err != nil {
		t.Fatal(err)
	}
	defer os.Chmod(root, 0o700)
	if err := RootAccessIssue(root); err == nil {
		t.Fatal("read-only root directory should be rejected")
	}
}
