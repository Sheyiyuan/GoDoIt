package bridge

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

func TestBootstrapInitializationFailureRetryCompletesLayout(t *testing.T) {
	root := filepath.Join(t.TempDir(), "gdit")
	if err := os.MkdirAll(root, 0o700); err != nil {
		t.Fatal(err)
	}
	blocker := filepath.Join(root, "templates")
	if err := os.WriteFile(blocker, []byte("not a directory"), 0o600); err != nil {
		t.Fatal(err)
	}
	app := NewApp(root)
	if snapshot, err := app.Bootstrap(); err == nil || snapshot.Root != root {
		t.Fatalf("expected retryable initialization failure with root, snapshot=%+v err=%v", snapshot, err)
	}
	for _, name := range []string{"engines", "sdks"} {
		if info, err := os.Stat(filepath.Join(root, name)); err != nil || !info.IsDir() {
			t.Fatalf("completed prefix directory %s missing after interrupted init: %v", name, err)
		}
	}
	if err := os.Remove(blocker); err != nil {
		t.Fatal(err)
	}
	snapshot, err := app.Bootstrap()
	if err != nil {
		t.Fatalf("bootstrap retry failed: %v", err)
	}
	if snapshot.Root != root {
		t.Fatalf("unexpected bootstrap root %q", snapshot.Root)
	}
	for _, name := range []string{"engines", "sdks", "templates", "instances", "tmp", "icons", "bin", "cache"} {
		if info, statErr := os.Stat(filepath.Join(root, name)); statErr != nil || !info.IsDir() {
			t.Fatalf("standard directory %s missing after retry: %v", name, statErr)
		}
	}
	for _, path := range []string{filepath.Join(root, "current"), filepath.Join(root, "bin", "godot")} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("bootstrap unexpectedly created %s: %v", path, statErr)
		}
	}
}

func TestOperationCancelEmitsMatchingTerminalEvent(t *testing.T) {
	app := NewApp(t.TempDir())
	events := make(chan ProgressEnvelope, 4)
	app.emit = func(name string, data any) {
		if name != progressEventName {
			t.Fatalf("unexpected event name %q", name)
		}
		events <- data.(ProgressEnvelope)
	}
	started := make(chan struct{})
	operation, err := app.startOperation("fixture", func(ctx context.Context, _ *gdit.Manager) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	})
	if err != nil {
		t.Fatal(err)
	}
	<-started
	if !app.Cancel(operation.OperationID) {
		t.Fatal("active operation was not canceled")
	}
	select {
	case event := <-events:
		if event.Status == "running" {
			if event.Progress == nil || event.Progress.Stage != "queued" {
				t.Fatalf("unexpected initial event: %+v", event)
			}
			event = <-events
		}
		if event.OperationID != operation.OperationID || event.Operation != "fixture" || event.Status != "canceled" {
			t.Fatalf("unexpected terminal event: %+v", event)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for terminal event")
	}
	if app.activeOperationCount() != 0 || app.Cancel(operation.OperationID) {
		t.Fatal("completed operation remained registered")
	}
}

func TestOperationForwardsMultipleAssetsFallbackAndOneTerminal(t *testing.T) {
	root := t.TempDir()
	app := NewApp(root)
	events := make(chan ProgressEnvelope, 16)
	app.emit = func(_ string, data any) { events <- data.(ProgressEnvelope) }
	var progress func(gdit.ProgressEvent)
	app.newManager = func(callback func(gdit.ProgressEvent)) (*gdit.Manager, error) {
		progress = callback
		return gdit.New(gdit.Options{RootDir: root, Progress: callback})
	}
	finished := make(chan struct{})
	operation, err := app.startOperation("install-entry", func(_ context.Context, _ *gdit.Manager) (any, error) {
		progress(gdit.ProgressEvent{Stage: "download", Version: "4.7.2-dotnet", Source: "godothub", Filename: "godot.zip", BytesDownloaded: 3, TotalBytes: 10})
		progress(gdit.ProgressEvent{Stage: "resolve", Version: "4.7.2-dotnet", Source: "github", Filename: "godot.zip", Message: "切换下载来源"})
		progress(gdit.ProgressEvent{Stage: "download", Version: "4.7.2-dotnet", Source: "github", Filename: "godot.zip", BytesDownloaded: 10, TotalBytes: 10})
		progress(gdit.ProgressEvent{Stage: "download", Version: "8.0.410(sdk)", Source: "dotnet-official", Filename: "dotnet.tar.gz", BytesDownloaded: 20, TotalBytes: 20})
		progress(gdit.ProgressEvent{Stage: "download", Version: "4.7.2-dotnet(template)", Source: "github", Filename: "templates.tpz", BytesDownloaded: 0, TotalBytes: 0})
		close(finished)
		return map[string]any{"installed": []any{map[string]any{"kind": "engine", "id": "4.7.2-dotnet"}}}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	<-finished
	terminalCount := 0
	seen := make(map[string]bool)
	deadline := time.After(2 * time.Second)
	for terminalCount == 0 {
		select {
		case event := <-events:
			if event.OperationID != operation.OperationID {
				t.Fatalf("unexpected operation id %q", event.OperationID)
			}
			if event.Status != "running" {
				terminalCount++
				continue
			}
			if event.Progress != nil && event.Progress.Filename != "" {
				seen[event.Progress.Source+"|"+event.Progress.Filename] = true
			}
		case <-deadline:
			t.Fatal("timed out waiting for operation terminal")
		}
	}
	for _, key := range []string{"godothub|godot.zip", "github|godot.zip", "dotnet-official|dotnet.tar.gz", "github|templates.tpz"} {
		if !seen[key] {
			t.Fatalf("missing forwarded subtask %q: %v", key, seen)
		}
	}
	progress(gdit.ProgressEvent{Stage: "download", Version: "late", Filename: "late.zip"})
	select {
	case event := <-events:
		t.Fatalf("event emitted after terminal: %+v", event)
	case <-time.After(50 * time.Millisecond):
	}
	if terminalCount != 1 {
		t.Fatalf("terminal count = %d", terminalCount)
	}
}

func TestBeforeCloseWaitsOrCancelsActiveOperations(t *testing.T) {
	app := NewApp(t.TempDir())
	events := make(chan ProgressEnvelope, 4)
	app.emit = func(_ string, data any) { events <- data.(ProgressEnvelope) }
	started := make(chan struct{})
	if _, err := app.startOperation("fixture", func(ctx context.Context, _ *gdit.Manager) (any, error) {
		close(started)
		<-ctx.Done()
		return nil, ctx.Err()
	}); err != nil {
		t.Fatal(err)
	}
	<-started
	app.promptClose = func(context.Context) (bool, error) { return false, nil }
	if !BeforeClose(app)(context.Background()) || app.activeOperationCount() != 1 {
		t.Fatal("continue waiting did not keep the window and operation open")
	}
	app.promptClose = func(context.Context) (bool, error) { return true, nil }
	if BeforeClose(app)(context.Background()) {
		t.Fatal("cancel and exit unexpectedly kept the window open")
	}
	deadline := time.After(2 * time.Second)
	for {
		select {
		case event := <-events:
			if event.Status == "canceled" {
				if app.activeOperationCount() != 0 {
					t.Fatal("canceled close left an active operation")
				}
				return
			}
		case <-deadline:
			t.Fatal("timed out waiting for canceled terminal")
		}
	}
}

func TestCandidateWarningCollectorKeepsOnlyNonEmptyWarnings(t *testing.T) {
	warnings, collect := candidateWarningCollector()
	collect(gdit.ProgressEvent{Stage: "resolve", Message: "not a warning"})
	collect(gdit.ProgressEvent{Stage: "warning", Source: "godothub", Message: "fixture unavailable"})
	collect(gdit.ProgressEvent{Stage: "warning", Source: "github", Message: "  "})

	if len(*warnings) != 1 || (*warnings)[0].Source != "godothub" || (*warnings)[0].Message != "fixture unavailable" {
		t.Fatalf("unexpected candidate warnings: %+v", *warnings)
	}
}

func TestEffectiveEnvironmentViewMarksSensitiveKeys(t *testing.T) {
	view := effectiveEnvironmentView(gdit.EnvView{Vars: []gdit.EnvVar{
		{Key: "PUBLIC_VALUE", Value: "visible", Origin: "global"},
		{Key: "SERVICE_TOKEN", Value: "secret", Origin: "instance"},
	}})
	if len(view.Vars) != 2 || view.Vars[0].Sensitive || !view.Vars[1].Sensitive {
		t.Fatalf("unexpected effective environment sensitivity: %+v", view.Vars)
	}
}

func TestSessionEventUsesStableNameAndPayload(t *testing.T) {
	app := NewApp(t.TempDir())
	want := gdit.SessionInfo{SessionID: "fixture", InstanceName: "studio", Status: gdit.SessionRunning}
	var eventName string
	var payload any
	app.emit = func(name string, data any) {
		eventName = name
		payload = data
	}
	app.emitSession(want)
	if eventName != sessionEventName || payload != want {
		t.Fatalf("unexpected session event: name=%q payload=%+v", eventName, payload)
	}
}

func TestIconHandlerServesOnlyCanonicalInstancePNG(t *testing.T) {
	root := t.TempDir()
	app := NewApp(root)
	id := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	if err := os.MkdirAll(filepath.Join(root, "icons"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "icons", id+".png"), []byte("png fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	request := httptest.NewRequest(http.MethodGet, "/instance-icons/"+id+".png", nil)
	response := httptest.NewRecorder()
	IconHandler(app).ServeHTTP(response, request)
	if response.Code != http.StatusOK || response.Body.String() != "png fixture" || response.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("unexpected icon response: code=%d body=%q headers=%v", response.Code, response.Body.String(), response.Header())
	}
	for _, path := range []string{
		"/instance-icons/../../config.toml",
		"/instance-icons/not-a-uuid.png",
		"/instance-icons/" + id + ".jpg",
		"/icons/" + id + ".png",
	} {
		request = httptest.NewRequest(http.MethodGet, path, nil)
		response = httptest.NewRecorder()
		IconHandler(app).ServeHTTP(response, request)
		if response.Code != http.StatusNotFound {
			t.Fatalf("unsafe icon path %q returned %d", path, response.Code)
		}
	}
}
