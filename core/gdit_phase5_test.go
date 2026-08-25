package gdit

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

type templateFixtureSource struct {
	archives map[string][]byte
	requests []SourceRequest
}

func (s *templateFixtureSource) Name() string { return "template-fixture" }

func (s *templateFixtureSource) Resolve(_ context.Context, request SourceRequest) (Artifact, error) {
	s.requests = append(s.requests, request)
	content, ok := s.archives[request.AssetName]
	if !ok {
		return Artifact{}, SourceUnavailableError{Source: s.Name(), Err: fmt.Errorf("missing fixture %s", request.AssetName)}
	}
	return Artifact{Source: s.Name(), URL: "https://fixture.invalid/" + request.AssetName, Filename: request.AssetName, ChecksumAlgorithm: "sha256", Checksum: digest(content)}, nil
}

func TestInstallTemplateUsesGenericSourceRequestAndPublishes(t *testing.T) {
	asset := "Godot_v4.5.2-stable_mono_export_templates.tpz"
	data := templateArchive(t, "4.5.2.stable.mono")
	source := &templateFixtureSource{archives: map[string][]byte{asset: data}}
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{source}, HTTPClient: fixtureHTTPClient(map[string][]byte{asset: data})})
	if err != nil {
		t.Fatal(err)
	}
	info, err := manager.InstallTemplate(context.Background(), InstallTemplateRequest{Version: "4.5.2", Edition: "dotnet"})
	if err != nil {
		t.Fatal(err)
	}
	if info.ID != "4.5.2-dotnet" || info.ArchiveName != asset || info.Path != filepath.Join(manager.root, "templates", info.ID, "payload") {
		t.Fatalf("unexpected template info: %+v", info)
	}
	if len(source.requests) != 1 {
		t.Fatalf("resolve calls = %d, want 1", len(source.requests))
	}
	request := source.requests[0]
	if request.Kind != "template" || request.AssetName != asset || request.Target.OS != "" || request.Target.Arch != "" {
		t.Fatalf("unexpected source request: %+v", request)
	}
	items, err := manager.Templates(context.Background())
	if err != nil || len(items) != 1 || items[0].ID != info.ID {
		t.Fatalf("Templates() = %+v, %v", items, err)
	}
	removed, err := manager.RemoveTemplate(context.Background(), "4.5.2", "dotnet")
	if err != nil || removed.ID != info.ID {
		t.Fatalf("RemoveTemplate() = %+v, %v", removed, err)
	}
}

func TestInstallTemplateRejectsMismatchedVersionFile(t *testing.T) {
	asset := "Godot_v4.5.2-stable_export_templates.tpz"
	data := templateArchive(t, "4.5.1.stable")
	source := &templateFixtureSource{archives: map[string][]byte{asset: data}}
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{source}, HTTPClient: fixtureHTTPClient(map[string][]byte{asset: data})})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallTemplate(context.Background(), InstallTemplateRequest{Version: "4.5.2"}); err == nil {
		t.Fatal("expected mismatched version.txt error")
	}
	items, err := manager.Templates(context.Background())
	if err != nil || len(items) != 0 {
		t.Fatalf("invalid template was published: %+v, %v", items, err)
	}
}

func TestTemplateBindingProtectsAssetAndDetachCreatesOrphan(t *testing.T) {
	requireFirstPhaseTarget(t)
	engineAsset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	templateAsset := "Godot_v4.5.2-stable_export_templates.tpz"
	archives := map[string][]byte{
		engineAsset:   godotArchive(t, "4.5.2", "standard", "engine"),
		templateAsset: templateArchive(t, "4.5.2.stable"),
	}
	source := &templateFixtureSource{archives: archives}
	unselected := &stubSource{name: "unselected", err: SourceUnavailableError{Source: "unselected", Err: errors.New("must not be called")}}
	manager := managerWithFixture(t, t.TempDir(), []Source{unselected, source}, archives)
	installed, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard", Source: source.Name(), Template: true})
	if err != nil {
		t.Fatal(err)
	}
	if unselected.calls != 0 || len(source.requests) != 2 || source.requests[0].Kind != "engine" || source.requests[1].Kind != "template" {
		t.Fatalf("entry install did not preserve selected source: unselected=%d requests=%+v", unselected.calls, source.requests)
	}
	if installed.Instance.Template != "4.5.2-standard" {
		t.Fatalf("template was not bound: %+v", installed.Instance)
	}
	if _, err := manager.RemoveTemplate(context.Background(), "4.5.2", "standard"); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("referenced template removal error = %v, want ErrAssetInUse", err)
	}
	detached, err := manager.DetachTemplate(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, orphan := range detached.Orphans {
		found = found || orphan.Kind == "template" && orphan.ID == "4.5.2-standard"
	}
	if !found {
		t.Fatalf("detached template missing from orphan snapshot: %+v", detached.Orphans)
	}
}

func TestSuggestDoesNotTouchMissingRoot(t *testing.T) {
	base := t.TempDir()
	projectDir := filepath.Join(base, "project")
	root := filepath.Join(base, "missing-gdit-root")
	if err := os.Mkdir(projectDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte("[application]\nconfig/features=PackedStringArray(\"4.5\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Options{RootDir: root, HTTPClient: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		t.Fatal("Suggest accessed the network")
		return nil, nil
	})}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Suggest(context.Background(), projectDir)
	if err != nil || !result.Installable || result.EngineSeries != "4.5" || result.Edition != "standard" {
		t.Fatalf("Suggest() = %+v, %v", result, err)
	}
	if _, err := os.Lstat(root); !os.IsNotExist(err) {
		t.Fatalf("Suggest created or read root: %v", err)
	}
}

func TestSuggestGodot3CSharpDoesNotRecommendSDK(t *testing.T) {
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte("[application]\nconfig/features=PoolStringArray(\"3.6\", \"C#\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(projectDir, "Game.csproj"), []byte("<Project><PropertyGroup><TargetFramework>net6.0</TargetFramework></PropertyGroup></Project>"), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Options{RootDir: filepath.Join(t.TempDir(), "root")})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Suggest(context.Background(), projectDir)
	if err != nil {
		t.Fatal(err)
	}
	if result.SDKStrategy != "" || result.SDKVersion != "" || result.SDKChannel != "" {
		t.Fatalf("Godot 3.x should not recommend a managed SDK: %+v", result)
	}
}

func TestInstallSuggestionPrefersHighestInstalledStableWithoutNetwork(t *testing.T) {
	requireFirstPhaseTarget(t)
	archives := map[string][]byte{}
	for _, version := range []string{"4.5.1", "4.5.3"} {
		asset, err := platform.AssetName(version, "standard", platform.Target{OS: "linux", Arch: "amd64"})
		if err != nil {
			t.Fatal(err)
		}
		archives[asset] = godotArchive(t, version, "standard", version)
	}
	source := newFixtureSource("fixture", archives)
	manager := managerWithFixture(t, t.TempDir(), []Source{source}, archives)
	for _, version := range []string{"4.5.1", "4.5.3"} {
		if _, err := manager.Install(context.Background(), InstallRequest{Version: version, Edition: "standard"}); err != nil {
			t.Fatal(err)
		}
	}
	before := source.resolve.Load()
	projectDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(projectDir, "project.godot"), []byte("[application]\nconfig/features=PackedStringArray(\"4.5\")\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	includeTemplate := false
	result, err := manager.InstallSuggestion(context.Background(), InstallSuggestionRequest{ProjectDir: projectDir, Name: "work", IncludeTemplate: &includeTemplate})
	if err != nil {
		t.Fatal(err)
	}
	if result.EngineVersion != "4.5.3" || result.Entry.Instance.Engine != "4.5.3-standard" {
		t.Fatalf("unexpected install result: %+v", result)
	}
	if source.resolve.Load() != before {
		t.Fatalf("local suggestion resolution accessed source: before=%d after=%d", before, source.resolve.Load())
	}
}

func templateArchive(t *testing.T, versionText string) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	entry, err := writer.Create("templates/version.txt")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, versionText); err != nil {
		t.Fatal(err)
	}
	payload, err := writer.Create("templates/linux_debug.x86_64")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(payload, "fixture"); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}
