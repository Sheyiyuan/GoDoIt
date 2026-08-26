package gdit

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/session"
)

func TestSessionRecordRoundTripAndIdentityMismatch(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "runtime"), 0o700); err != nil {
		t.Fatal(err)
	}
	record := session.Record{
		SessionID:       "6c2d5f3a-2e7c-4f6e-8d2a-0b9c1e4f5a6b",
		InstanceID:      "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8",
		InstanceName:    "fixture",
		EngineID:        "4.5.2-standard",
		PID:             os.Getpid(),
		ProcessIdentity: "mismatched-identity",
		StartedAt:       time.Unix(1, 0).UTC(),
		Status:          session.Running,
	}
	if err := session.Write(root, record); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(session.Path(root, record.SessionID))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(content), "executable") {
		t.Fatalf("session record must not persist executable path: %s", content)
	}
	for _, path := range []string{filepath.Join(root, "runtime"), filepath.Join(root, "runtime", "sessions"), session.Path(root, record.SessionID)} {
		info, statErr := os.Stat(path)
		if statErr != nil {
			t.Fatalf("stat session path %s: %v", path, statErr)
		}
		if got := info.Mode().Perm(); got != 0o700 && path != session.Path(root, record.SessionID) {
			t.Fatalf("directory %s mode = %o, want 700", path, got)
		}
		if path == session.Path(root, record.SessionID) {
			if got := info.Mode().Perm(); got != 0o600 {
				t.Fatalf("session file mode = %o, want 600", got)
			}
		}
	}
	items, err := session.List(root)
	if err != nil || len(items) != 1 || items[0].SessionID != record.SessionID {
		t.Fatalf("unexpected session records: %+v err=%v", items, err)
	}
	alive, err := platform.ProcessAlive(record.PID, record.ProcessIdentity, os.Args[0])
	if err != nil {
		t.Fatal(err)
	}
	if alive {
		t.Fatal("identity mismatch must not be treated as a managed process")
	}
	if err := session.Remove(root, record.SessionID); err != nil {
		t.Fatal(err)
	}
	items, err = session.List(root)
	if err != nil || len(items) != 0 {
		t.Fatalf("session removal failed: %+v err=%v", items, err)
	}
}

func TestStartManagedProcessCanBeStoppedWithoutContextCoupling(t *testing.T) {
	process, err := platform.StartManagedProcess(context.Background(), "/bin/sh", []string{"-c", "sleep 30"}, os.Environ())
	if err != nil {
		t.Fatal(err)
	}
	waited := make(chan error, 1)
	go func() { waited <- process.Wait() }()
	alive, err := platform.ProcessAlive(process.PID, process.Identity, "/bin/sh")
	if err != nil || !alive {
		t.Fatalf("started fixture process is not alive: alive=%v err=%v", alive, err)
	}
	if err := platform.RequestStop(process.PID); err != nil {
		t.Fatal(err)
	}
	select {
	case <-time.After(2 * time.Second):
		t.Fatal("fixture process did not stop")
	case <-waited:
	}
}
