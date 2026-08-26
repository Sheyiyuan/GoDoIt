package gdit

import (
	"context"
	"errors"
	"os"
	"os/exec"
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

func TestManagedSessionLifecycleEmitsEventsAndProtectsInstance(t *testing.T) {
	target, err := platform.CurrentTarget()
	if err != nil {
		t.Skipf("current platform is not supported: %v", err)
	}
	fixture, err := exec.LookPath("yes")
	if err != nil {
		t.Skip("yes fixture process is unavailable")
	}
	fixtureData, err := os.ReadFile(fixture)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	events := make(chan SessionInfo, 8)
	manager, err := New(Options{RootDir: root, Session: func(info SessionInfo) { events <- info }})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.Initialize(context.Background()); err != nil {
		t.Fatal(err)
	}
	const engineID = "4.5.2-standard"
	const launcher = "fixture-engine"
	payload := filepath.Join(root, "engines", engineID, "payload")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, launcher), fixtureData, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := "id = \"" + engineID + "\"\nversion = \"4.5.2\"\nedition = \"standard\"\ntarget_os = \"" + target.OS + "\"\ntarget_arch = \"" + target.Arch + "\"\nsource = \"fixture\"\nchecksum_algorithm = \"sha256\"\nchecksum = \"" + strings.Repeat("a", 64) + "\"\nlauncher = \"" + launcher + "\"\ninstalled_at = \"2026-08-26T00:00:00Z\"\n"
	if err := os.WriteFile(filepath.Join(root, "engines", engineID, "install.toml"), []byte(manifest), 0o600); err != nil {
		t.Fatal(err)
	}
	writeRawInstance(t, root, "session-fixture", "4.5.2", "standard", "")

	started, err := manager.LaunchSession(context.Background(), "session-fixture")
	if err != nil {
		t.Fatal(err)
	}
	if event := waitSessionEvent(t, events, SessionRunning); event.SessionID != started.SessionID {
		t.Fatalf("running event session = %q, want %q", event.SessionID, started.SessionID)
	}
	if _, err := manager.RemoveInstance(context.Background(), "session-fixture"); !errors.Is(err, ErrInstanceRunning) {
		t.Fatalf("running session did not protect instance: %v", err)
	}
	stopping, err := manager.RequestStopSession(context.Background(), started.SessionID)
	if err != nil || stopping.Status != SessionStopping {
		t.Fatalf("request stop result = %+v err=%v", stopping, err)
	}
	waitSessionEvent(t, events, SessionStopping)
	waitSessionEvent(t, events, SessionExited)
	items, err := manager.Sessions(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("exited session was not cleaned: %+v err=%v", items, err)
	}
}

func waitSessionEvent(t *testing.T, events <-chan SessionInfo, status SessionStatus) SessionInfo {
	t.Helper()
	select {
	case event := <-events:
		if event.Status != status {
			t.Fatalf("session event status = %q, want %q", event.Status, status)
		}
		return event
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for session status %q", status)
		return SessionInfo{}
	}
}
