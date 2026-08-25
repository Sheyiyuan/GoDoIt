package project

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"
)

func TestAnalyzeGodot4CSharpReadOnly(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "project.godot"), `[application]
config/name="Demo;still text" ; comment
config/features=PackedStringArray("4.5", "C#", "GL Compatibility")
`)
	writeProjectFile(t, filepath.Join(dir, "global.json"), `{"sdk":{"version":"8.0.410","rollForward":"latestPatch","allowPrerelease":false}}`)
	writeProjectFile(t, filepath.Join(dir, "Game.csproj"), `<Project><PropertyGroup><TargetFrameworks>net8.0;net9.0-linux</TargetFrameworks></PropertyGroup></Project>`)
	before := snapshot(t, dir)
	result, err := Analyze(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineSeries != "4.5" || result.Edition != "dotnet" || result.SDKVersion != "8.0.410" || result.SDKChannel != "8.0" {
		t.Fatalf("unexpected analysis: %+v", result)
	}
	if len(result.Diagnostics) != 0 {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
	after := snapshot(t, dir)
	if !reflect.DeepEqual(before, after) {
		t.Fatalf("project changed during analysis\nbefore=%+v\nafter=%+v", before, after)
	}
}

func TestAnalyzeGodot3PoolArrayAndCSharpMismatch(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "project.godot"), `[application]
config/features=PoolStringArray(
  "3.6",
  "GLES3"
)
`)
	writeProjectFile(t, filepath.Join(dir, "Game.csproj"), `<Project><PropertyGroup><TargetFramework>net6.0</TargetFramework></PropertyGroup></Project>`)
	result, err := Analyze(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineSeries != "3.6" || result.Edition != "dotnet" || result.SDKChannel != "6.0" {
		t.Fatalf("unexpected analysis: %+v", result)
	}
	if len(result.Diagnostics) != 1 || result.Diagnostics[0].Code != "csharp-project-without-feature" {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
}

func TestAnalyzeReportsContentErrors(t *testing.T) {
	dir := t.TempDir()
	writeProjectFile(t, filepath.Join(dir, "project.godot"), `[application]
config/features=PackedStringArray("4.4", "4.5")
`)
	writeProjectFile(t, filepath.Join(dir, "global.json"), `{bad`)
	writeProjectFile(t, filepath.Join(dir, "Bad.csproj"), `<Project>`)
	result, err := Analyze(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	codes := map[string]bool{}
	for _, item := range result.Diagnostics {
		codes[item.Code] = true
	}
	for _, code := range []string{"engine-series-conflict", "global-json-invalid", "csproj-invalid"} {
		if !codes[code] {
			t.Fatalf("missing diagnostic %s: %+v", code, result.Diagnostics)
		}
	}
}

func TestAnalyzeRejectsSymlinkEscape(t *testing.T) {
	dir := t.TempDir()
	outside := filepath.Join(t.TempDir(), "project.godot")
	writeProjectFile(t, outside, `[application]
config/features=PackedStringArray("4.5")
`)
	if err := os.Symlink(outside, filepath.Join(dir, "project.godot")); err != nil {
		t.Fatal(err)
	}
	if _, err := Analyze(context.Background(), dir); err == nil {
		t.Fatal("expected symlink boundary error")
	}
}

func TestAnalyzeReportsOversizedProjectFile(t *testing.T) {
	dir := t.TempDir()
	content := make([]byte, MaxProjectFileSize+1)
	if err := os.WriteFile(filepath.Join(dir, "project.godot"), content, 0o600); err != nil {
		t.Fatal(err)
	}
	result, err := Analyze(context.Background(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Diagnostics) == 0 || result.Diagnostics[0].Code != "project-godot-too-large" {
		t.Fatalf("unexpected diagnostics: %+v", result.Diagnostics)
	}
}

type fileSnapshot struct {
	Mode    os.FileMode
	ModTime time.Time
	Content string
}

func snapshot(t *testing.T, dir string) map[string]fileSnapshot {
	t.Helper()
	result := map[string]fileSnapshot{}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		path := filepath.Join(dir, entry.Name())
		info, err := os.Stat(path)
		if err != nil {
			t.Fatal(err)
		}
		content, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		result[entry.Name()] = fileSnapshot{Mode: info.Mode(), ModTime: info.ModTime(), Content: string(content)}
	}
	return result
}

func writeProjectFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.WriteFile(path, []byte(content), 0o640); err != nil {
		t.Fatal(err)
	}
}
