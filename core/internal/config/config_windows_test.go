//go:build windows

package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSetEnvironmentVariableReplacesWindowsCaseVariant(t *testing.T) {
	root := t.TempDir()
	if err := SetEnvironmentVariable(root, "Path", "first", false); err != nil {
		t.Fatal(err)
	}
	if err := SetEnvironmentVariable(root, "PATH", "second", false); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment.Global["PATH"] != "second" {
		t.Fatalf("PATH = %q", cfg.Environment.Global["PATH"])
	}
	matching := 0
	for key := range cfg.Environment.Global {
		if NormalizeEnvironmentKey(key) == "PATH" {
			matching++
		}
	}
	if matching != 1 {
		t.Fatalf("expected one Windows PATH key, got %+v", cfg.Environment.Global)
	}
	if err := SetEnvironmentVariable(root, "path", "", true); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for key := range cfg.Environment.Global {
		if NormalizeEnvironmentKey(key) == "PATH" {
			t.Fatalf("PATH variant survived unset: %+v", cfg.Environment.Global)
		}
	}
}

func TestLoadRejectsConflictingWindowsEnvironmentKeys(t *testing.T) {
	root := t.TempDir()
	content := "schema_version = 1\nsource_order = [\"github\"]\n\n[environment]\nPath = \"first\"\nPATH = \"second\"\n"
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("case-conflicting Windows environment keys must be rejected")
	}
}
