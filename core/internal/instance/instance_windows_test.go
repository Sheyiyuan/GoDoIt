//go:build windows

package instance

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

const windowsTestInstanceID = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"

func TestSetEnvReplacesWindowsCaseVariant(t *testing.T) {
	root := t.TempDir()
	installFixtureEngine(t, root, "4.5.2-standard", "godot.exe")
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := File{
		SchemaVersion: SchemaVersion,
		ID:            windowsTestInstanceID,
		Name:          "work",
		Engine:        Engine{Version: "4.5.2", Edition: "standard"},
		Env:           map[string]string{"Path": "first"},
	}
	if err := Write(root, item); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(root, item.ID, "PATH", "second", false); err != nil {
		t.Fatal(err)
	}
	got, err := Read(root, item.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Env) != 1 || got.Env["PATH"] != "second" {
		t.Fatalf("unexpected environment: %+v", got.Env)
	}
	if err := SetEnv(root, item.ID, "path", "", true); err != nil {
		t.Fatal(err)
	}
	got, err = Read(root, item.ID)
	if err != nil || len(got.Env) != 0 {
		t.Fatalf("environment after unset: %+v, %v", got.Env, err)
	}
}

func TestReadRejectsConflictingWindowsEnvironmentKeys(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	content := map[string]any{
		"schema_version": SchemaVersion,
		"id":             windowsTestInstanceID,
		"name":           "work",
		"engine":         map[string]any{"version": "4.5.2", "edition": "standard"},
		"env":            map[string]any{"Path": "first", "PATH": "second"},
	}
	if err := store.WriteTOMLAtomic(Path(root, windowsTestInstanceID), content); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, windowsTestInstanceID); err == nil {
		t.Fatal("case-conflicting Windows instance environment keys must be rejected")
	}
}
