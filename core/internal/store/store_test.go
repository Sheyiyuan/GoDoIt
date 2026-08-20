package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validInstalledAt = "2026-08-17T00:00:00Z"

func buildManifest(id, algorithm, checksum string) Manifest {
	version := strings.TrimSuffix(id, "-standard")
	edition := "standard"
	if strings.HasSuffix(id, "-dotnet") {
		version = strings.TrimSuffix(id, "-dotnet")
		edition = "dotnet"
	}
	return Manifest{
		ID:                id,
		Version:           version,
		Edition:           edition,
		TargetOS:          "linux",
		TargetArch:        "amd64",
		Source:            "fixture",
		ChecksumAlgorithm: algorithm,
		Checksum:          checksum,
		Launcher:          "Godot_v4.5.2-stable_linux.x86_64",
		InstalledAt:       validInstalledAt,
	}
}

func writeVersionDir(t *testing.T, s *Store, id string, manifest Manifest) {
	t.Helper()
	dir := s.EngineDir(id)
	if err := os.MkdirAll(filepath.Join(dir, "payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	launcher := filepath.Join(dir, "payload", manifest.Launcher)
	if err := os.WriteFile(launcher, []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTOML(filepath.Join(dir, "install.toml"), manifest); err != nil {
		t.Fatal(err)
	}
}

func TestScanValidAcceptsSHA256AndSHA512(t *testing.T) {
	s := New(t.TempDir())
	writeVersionDir(t, s, "4.5.2-standard", buildManifest("4.5.2-standard", "sha256", strings.Repeat("a", 64)))
	writeVersionDir(t, s, "4.5.2-dotnet", buildManifest("4.5.2-dotnet", "sha512", strings.Repeat("b", 128)))
	records, err := s.ScanValid()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 2 {
		t.Fatalf("expected 2 valid records, got %d", len(records))
	}
}

func TestScanValidSkipsInvalidManifests(t *testing.T) {
	s := New(t.TempDir())
	// manifest ID 与目录名不一致。
	mismatchDir := s.EngineDir("4.5.2-standard")
	if err := os.MkdirAll(filepath.Join(mismatchDir, "payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mismatchDir, "payload", "engine"), []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(mismatchDir, "install.toml"), []byte("id = \"wrong\"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	// 不支持的摘要算法。
	writeVersionDir(t, s, "4.5.3-standard", buildManifest("4.5.3-standard", "md5", strings.Repeat("c", 32)))
	// launcher 文件缺失。
	missingLauncher := buildManifest("4.5.4-standard", "sha256", strings.Repeat("d", 64))
	dir := s.EngineDir("4.5.4-standard")
	if err := os.MkdirAll(filepath.Join(dir, "payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeTOML(filepath.Join(dir, "install.toml"), missingLauncher); err != nil {
		t.Fatal(err)
	}
	records, err := s.ScanValid()
	if err != nil {
		t.Fatal(err)
	}
	if len(records) != 0 {
		t.Fatalf("expected no valid records, got %+v", records)
	}
}

func TestValidateManifestRejectsBadChecksumLength(t *testing.T) {
	s := New(t.TempDir())
	manifest := buildManifest("4.5.2-standard", "sha256", strings.Repeat("a", 128))
	dir := s.EngineDir("4.5.2-standard")
	if err := os.MkdirAll(filepath.Join(dir, "payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "payload", manifest.Launcher), []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := validateManifest(manifest, "4.5.2-standard", dir); err == nil {
		t.Fatal("expected wrong checksum length to be rejected")
	}
}

func TestCleanupOperationsRemovesStaleOperations(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operation-1", "operation-2", "keep"} {
		if err := os.MkdirAll(filepath.Join(s.TmpDir(), name), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(s.TmpDir(), "note.txt"), []byte("keep"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.CleanupOperations(); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"operation-1", "operation-2"} {
		if _, err := os.Stat(filepath.Join(s.TmpDir(), name)); !os.IsNotExist(err) {
			t.Fatalf("stale operation directory %s was not removed: %v", name, err)
		}
	}
	for _, name := range []string{"keep", "note.txt"} {
		if _, err := os.Stat(filepath.Join(s.TmpDir(), name)); err != nil {
			t.Fatalf("unrelated entry %s was removed: %v", name, err)
		}
	}
}

func TestCleanupOperationsMissingTmpIsNoop(t *testing.T) {
	s := New(t.TempDir())
	if err := s.CleanupOperations(); err != nil {
		t.Fatal(err)
	}
}

func TestPublishReportsDestinationExists(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.TmpDir(), "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(s.EngineDir("4.5.2-standard"), 0o755); err != nil {
		t.Fatal(err)
	}
	published, err := s.Publish(staging, "4.5.2-standard")
	if published || !errors.Is(err, ErrDestinationExists) {
		t.Fatalf("expected ErrDestinationExists, got published=%v err=%v", published, err)
	}
}

func TestPublishRejectsStagingOutsideTmp(t *testing.T) {
	s := New(t.TempDir())
	outside := filepath.Join(t.TempDir(), "staging")
	if err := os.MkdirAll(outside, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(outside, "4.5.2-standard"); err == nil {
		t.Fatal("expected staging outside tmp to be rejected")
	}
}

func TestPublishRejectsInvalidEngineID(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.TmpDir(), "staging")
	if err := os.MkdirAll(staging, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := s.Publish(staging, "11.0.100-preview.7.26381.103-standard"); err == nil {
		t.Fatal("SDK-shaped engine ID must be rejected before publish")
	}
	if _, err := os.Stat(s.EngineDir("11.0.100-preview.7.26381.103-standard")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("invalid engine directory was published: %v", err)
	}
}

// rename 成功后目录同步失败时，必须把安装视为已发布，且已发布目录可以重建状态索引。
func TestPublishReportsPublishedWhenSyncFails(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	staging := filepath.Join(s.TmpDir(), "staging")
	manifest := buildManifest("4.5.2-standard", "sha256", strings.Repeat("a", 64))
	if err := os.MkdirAll(filepath.Join(staging, "payload"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(staging, "payload", manifest.Launcher), []byte("engine"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := s.WriteManifest(staging, manifest); err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(string) error { return errors.New("injected sync failure") }
	published, err := s.Publish(staging, "4.5.2-standard")
	if !published || err == nil {
		t.Fatalf("expected published=true with sync error, got published=%v err=%v", published, err)
	}
	if _, statErr := os.Stat(s.EngineDir("4.5.2-standard")); statErr != nil {
		t.Fatalf("version directory was not published: %v", statErr)
	}
	records, scanErr := s.ScanValid()
	if scanErr != nil {
		t.Fatal(scanErr)
	}
	if len(records) != 1 {
		t.Fatalf("expected 1 record after publish, got %d", len(records))
	}
	changed, reconcileErr := s.ReconcileState(records, nil)
	if reconcileErr != nil {
		t.Fatal(reconcileErr)
	}
	if !changed {
		t.Fatal("expected state index to be written after publish")
	}
}

func TestReconcileStateRebuildsFromRecords(t *testing.T) {
	s := New(t.TempDir())
	writeVersionDir(t, s, "4.5.2-standard", buildManifest("4.5.2-standard", "sha256", strings.Repeat("a", 64)))
	records, err := s.ScanValid()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(s.StatePath(), []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	changed, err := s.ReconcileState(records, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !changed {
		t.Fatal("expected state index to be rebuilt")
	}
	records, err = s.ScanValid()
	if err != nil {
		t.Fatal(err)
	}
	changed, err = s.ReconcileState(records, nil)
	if err != nil {
		t.Fatal(err)
	}
	if changed {
		t.Fatal("expected no rewrite when state matches records")
	}
}

func TestReadCurrentRequiresDirectInstanceTarget(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	id := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	if err := os.WriteFile(filepath.Join(s.InstancesDir(), id+".toml"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := s.SetCurrent(id); err != nil {
		t.Fatal(err)
	}
	if current, err := s.ReadCurrent(); err != nil || current != id {
		t.Fatalf("unexpected current: %q err=%v", current, err)
	}
	// 非法目标：非 UUID 文件名、嵌套目录、绝对路径，一律拒绝。
	for _, target := range []string{
		filepath.Join("instances", "work.toml"),
		filepath.Join("instances", "nested", id+".toml"),
		"/abs/path.toml",
	} {
		if err := os.Remove(s.CurrentPath()); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, s.CurrentPath()); err != nil {
			t.Fatal(err)
		}
		if _, err := s.ReadCurrent(); err == nil {
			t.Fatalf("current target %q should be rejected", target)
		}
	}
}

func TestSetCurrentSyncFailureReportsErrorAndRestoresOldLink(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	oldID := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	newID := "7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d"
	for _, id := range []string{oldID, newID} {
		if err := os.WriteFile(filepath.Join(s.InstancesDir(), id+".toml"), []byte("fixture"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := s.SetCurrent(oldID); err != nil {
		t.Fatal(err)
	}
	// rename 后的父目录 fsync 失败：对调用方返回错误前必须恢复旧链接。
	s.syncDir = func(string) error { return errors.New("injected sync failure") }
	if err := s.SetCurrent(newID); err == nil {
		t.Fatal("expected sync failure to be reported")
	}
	s.syncDir = syncDirectory
	if current, err := s.ReadCurrent(); err != nil || current != oldID {
		t.Fatalf("old link must be restored after sync failure: %q err=%v", current, err)
	}
}

func TestSetCurrentInitialSyncFailureLeavesNoLink(t *testing.T) {
	s := New(t.TempDir())
	if err := s.Init(); err != nil {
		t.Fatal(err)
	}
	id := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	if err := os.WriteFile(filepath.Join(s.InstancesDir(), id+".toml"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	s.syncDir = func(string) error { return errors.New("injected sync failure") }
	if err := s.SetCurrent(id); err == nil {
		t.Fatal("expected sync failure to be reported")
	}
	if _, err := os.Lstat(s.CurrentPath()); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed initial switch must not leave current: %v", err)
	}
}
