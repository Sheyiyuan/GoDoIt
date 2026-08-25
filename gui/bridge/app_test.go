package bridge

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

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
