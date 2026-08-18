package gdit

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/source"
)

type fixtureSource struct {
	name     string
	archives map[string][]byte
	resolve  atomic.Int64
}

func (s *fixtureSource) Name() string { return s.name }

func (s *fixtureSource) Resolve(_ context.Context, request SourceRequest) (Artifact, error) {
	s.resolve.Add(1)
	asset, err := platform.AssetName(request.Version, request.Edition, platform.Target{OS: request.Target.OS, Arch: request.Target.Arch})
	if err != nil {
		return Artifact{}, err
	}
	data, ok := s.archives[asset]
	if !ok {
		return Artifact{}, fmt.Errorf("fixture asset %s not found", asset)
	}
	return Artifact{
		Source:            s.name,
		URL:               "http://localhost/" + asset,
		Filename:          asset,
		ChecksumAlgorithm: "sha256",
		Checksum:          digest(data),
	}, nil
}

func newFixtureSource(name string, archives map[string][]byte) *fixtureSource {
	return &fixtureSource{name: name, archives: archives}
}

func fixtureHTTPClient(archives map[string][]byte) *http.Client {
	return &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		data, ok := archives[filepath.Base(request.URL.Path)]
		if !ok {
			return response(request, http.StatusNotFound, nil), nil
		}
		return response(request, http.StatusOK, data), nil
	})}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func response(request *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func managerWithFixture(t *testing.T, root string, sources []Source, archives map[string][]byte) *Manager {
	t.Helper()
	manager, err := New(Options{RootDir: root, HTTPClient: fixtureHTTPClient(archives), Sources: sources})
	if err != nil {
		t.Fatal(err)
	}
	return manager
}

func TestInstallAndListRebuildState(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := godotArchive(t, "4.5.2", "standard", "standard payload")
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: archiveData}
	fixture := newFixtureSource("fixture", archives)
	manager := managerWithFixture(t, t.TempDir(), []Source{fixture}, archives)

	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.ID != "4.5.2-standard" || result.StateRebuildRequired {
		t.Fatalf("unexpected install result: %+v", result)
	}
	launcher := filepath.Join(manager.root, "versions", result.Version.ID, "payload", result.Version.Launcher)
	if info, err := os.Stat(launcher); err != nil {
		t.Fatalf("launcher missing: %v", err)
	} else if info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("launcher is not executable: %v", info.Mode().Perm())
	}

	if err := os.WriteFile(filepath.Join(manager.root, "state.toml"), []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != result.Version.ID {
		t.Fatalf("unexpected list: %+v", versions)
	}
	state, err := os.ReadFile(filepath.Join(manager.root, "state.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Contains(state, []byte("schema_version = 1")) || !bytes.Contains(state, []byte(result.Version.ID)) {
		t.Fatalf("state was not rebuilt: %s", state)
	}

	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if !errors.Is(err, ErrAlreadyInstalled) {
		t.Fatalf("expected duplicate install error, got %v", err)
	}
}

func TestInstallDotnetDoesNotOverwriteStandard(t *testing.T) {
	requireFirstPhaseTarget(t)
	standardAsset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	dotnetAsset, err := platform.AssetName("4.5.2", "dotnet", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{
		standardAsset: godotArchive(t, "4.5.2", "standard", "standard payload"),
		dotnetAsset:   godotArchive(t, "4.5.2", "dotnet", "dotnet payload"),
	}
	fixture := newFixtureSource("fixture", archives)
	manager := managerWithFixture(t, t.TempDir(), []Source{fixture}, archives)
	for _, edition := range []string{"standard", "dotnet"} {
		if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: edition}); err != nil {
			t.Fatalf("install %s: %v", edition, err)
		}
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].ID != "4.5.2-dotnet" || versions[1].ID != "4.5.2-standard" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
}

func TestInstallFallsBackOnUnavailableSource(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "fallback")}
	fixture := newFixtureSource("atomgit", archives)
	unavailable := &stubSource{name: "godothub", err: SourceUnavailableError{Source: "godothub", Err: errors.New("fixture down")}}
	manager := managerWithFixture(t, t.TempDir(), []Source{unavailable, fixture}, archives)
	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.Source != "atomgit" || unavailable.calls != 1 || fixture.resolve.Load() != 1 {
		t.Fatalf("unexpected fallback result: %+v, unavailable=%d fixture=%d", result, unavailable.calls, fixture.resolve.Load())
	}
}

func TestChecksumMismatchStopsFallback(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	data := godotArchive(t, "4.5.2", "standard", "bad checksum")
	archives := map[string][]byte{asset: data}
	bad := &stubSource{name: "godothub", artifact: Artifact{
		Filename:          asset,
		URL:               "http://localhost/" + asset,
		ChecksumAlgorithm: "sha256",
		Checksum:          strings.Repeat("0", 64),
	}}
	second := &stubSource{name: "atomgit", err: SourceUnavailableError{Source: "atomgit", Err: errors.New("must not be called")}}
	manager := managerWithFixture(t, t.TempDir(), []Source{bad, second}, archives)
	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	var integrity IntegrityError
	if !errors.As(err, &integrity) {
		t.Fatalf("expected integrity error, got %v", err)
	}
	if second.calls != 0 {
		t.Fatalf("checksum failure incorrectly fell back: %d calls", second.calls)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("failed install was published: %+v", versions)
	}
}

func TestInterruptedDownloadDoesNotPublish(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &stubSource{name: "fixture", artifact: Artifact{
		Filename:          asset,
		URL:               "http://localhost/" + asset,
		ChecksumAlgorithm: "sha256",
		Checksum:          strings.Repeat("a", 64),
	}}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		result := response(request, http.StatusOK, nil)
		result.Body = &interruptedBody{data: []byte("truncated")}
		result.ContentLength = 100
		return result, nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, Sources: []Source{fixture}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if !errors.Is(err, ErrAllSourcesUnavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("interrupted install was published: %+v", versions)
	}
}

func TestInstallCancellationDoesNotPublish(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	fixture := &stubSource{name: "fixture", artifact: Artifact{
		Filename:          asset,
		URL:               "http://localhost/" + asset,
		ChecksumAlgorithm: "sha256",
		Checksum:          strings.Repeat("a", 64),
	}}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return nil, request.Context().Err()
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, Sources: []Source{fixture}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err = manager.Install(ctx, InstallRequest{Version: "4.5.2", Edition: "standard"})
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("expected cancellation, got %v", err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("cancelled install was published: %+v", versions)
	}
}

func TestConcurrentInstallPublishesOneVersion(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "concurrent")}
	fixture := newFixtureSource("fixture", archives)
	manager := managerWithFixture(t, t.TempDir(), []Source{fixture}, archives)
	errorsFromInstall := make(chan error, 2)
	start := make(chan struct{})
	var wait sync.WaitGroup
	for range 2 {
		wait.Add(1)
		go func() {
			defer wait.Done()
			<-start
			_, installErr := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
			errorsFromInstall <- installErr
		}()
	}
	close(start)
	wait.Wait()
	close(errorsFromInstall)
	var success, duplicate int
	for installErr := range errorsFromInstall {
		switch {
		case installErr == nil:
			success++
		case errors.Is(installErr, ErrAlreadyInstalled):
			duplicate++
		default:
			t.Fatalf("unexpected install result: %v", installErr)
		}
	}
	if success != 1 || duplicate != 1 {
		t.Fatalf("unexpected concurrent results: success=%d duplicate=%d", success, duplicate)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != "4.5.2-standard" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
}

func TestInstallRejectsCorruptArchive(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	corrupt := []byte("this is not a zip archive")
	fixture := &stubSource{name: "fixture", artifact: Artifact{
		Filename:          asset,
		URL:               "http://localhost/" + asset,
		ChecksumAlgorithm: "sha256",
		Checksum:          digest(corrupt),
	}}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return response(request, http.StatusOK, corrupt), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, Sources: []Source{fixture}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if !errors.Is(err, ErrInvalidArchive) {
		t.Fatalf("expected invalid archive error, got %v", err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("corrupt archive was published: %+v", versions)
	}
}

// listingSource 是可枚举版本的 fixture 来源。
type listingSource struct {
	name     string
	versions []source.VersionInfo
	err      error
}

func (s *listingSource) Name() string { return s.name }

func (s *listingSource) Resolve(context.Context, SourceRequest) (Artifact, error) {
	return Artifact{}, errors.New("listing fixture does not serve downloads")
}

func (s *listingSource) ListVersions(context.Context) ([]source.VersionInfo, error) {
	return s.versions, s.err
}

func TestInstallWithSourceUsesOnlyThatSource(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "direct")}
	first := &stubSource{name: "first", err: SourceUnavailableError{Source: "first", Err: errors.New("must not be called")}}
	second := newFixtureSource("second", archives)
	manager := managerWithFixture(t, t.TempDir(), []Source{first, second}, archives)
	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard", Source: "second"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.Source != "second" || first.calls != 0 {
		t.Fatalf("expected only the selected source to be used, got %+v first.calls=%d", result, first.calls)
	}
}

func TestInstallWithUnknownSourceFails(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "payload")}
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard", Source: "missing"})
	if !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected config error for unknown source, got %v", err)
	}
}

func TestAvailableMergesAndSortsSources(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{
		&listingSource{name: "godothub", versions: []source.VersionInfo{
			{Version: "4.7.1", Editions: []string{"standard", "dotnet"}},
			{Version: "4.5.2", Editions: []string{"standard"}},
		}},
		&listingSource{name: "github", versions: []source.VersionInfo{
			{Version: "4.7.1", Editions: []string{"standard"}},
			{Version: "4.6.3", Editions: []string{"standard", "dotnet"}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := manager.Available(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 3 || versions[0].Version != "4.7.1" || versions[1].Version != "4.6.3" || versions[2].Version != "4.5.2" {
		t.Fatalf("unexpected order or set: %+v", versions)
	}
	if len(versions[0].Editions) != 2 || versions[0].Editions[0] != "standard" || versions[0].Editions[1] != "dotnet" {
		t.Fatalf("editions were not merged: %+v", versions[0])
	}
	if len(versions[0].Sources) != 2 || versions[0].Sources[0] != "godothub" || versions[0].Sources[1] != "github" {
		t.Fatalf("sources were not merged: %+v", versions[0])
	}
}

func TestAvailableSingleSourceFailureStillMerges(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{
		&listingSource{name: "down", err: errors.New("fixture down")},
		&listingSource{name: "up", versions: []source.VersionInfo{
			{Version: "4.5.2", Editions: []string{"standard"}},
		}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	var warnings []ProgressEvent
	manager.progress = func(event ProgressEvent) {
		if event.Stage == "warning" {
			warnings = append(warnings, event)
		}
	}
	versions, err := manager.Available(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != "4.5.2" {
		t.Fatalf("failed source must not block results: %+v", versions)
	}
	if len(warnings) != 1 || warnings[0].Source != "down" {
		t.Fatalf("expected one warning for the failed source: %+v", warnings)
	}
}

func TestAvailableWithSpecifiedSource(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{
		&listingSource{name: "one", versions: []source.VersionInfo{{Version: "4.5.2", Editions: []string{"standard"}}}},
		&listingSource{name: "two", versions: []source.VersionInfo{{Version: "4.7.1", Editions: []string{"standard"}}}},
	}})
	if err != nil {
		t.Fatal(err)
	}
	versions, err := manager.Available(context.Background(), "two")
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].Version != "4.7.1" || versions[0].Sources[0] != "two" {
		t.Fatalf("unexpected specified-source result: %+v", versions)
	}
}

func TestAvailableNoEnumerationSupportFails(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{
		newFixtureSource("plain", map[string][]byte{}),
	}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Available(context.Background(), ""); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("expected config error for sources without enumeration, got %v", err)
	}
}

func TestSetDefaultSourceWritesConfigWithLock(t *testing.T) {
	root := t.TempDir()
	content := `schema_version = 1
source_order = ["github", "godothub"]

[environment]
display_driver = "auto"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetDefaultSource(context.Background(), "godothub"); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SourceOrder) != 2 || cfg.SourceOrder[0] != "godothub" || cfg.SourceOrder[1] != "github" {
		t.Fatalf("unexpected source order: %+v", cfg.SourceOrder)
	}
	written, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "display_driver") {
		t.Fatalf("unknown field was lost after write-back: %s", written)
	}
}

func writeTwoSourceConfig(t *testing.T, root, disabled string) {
	t.Helper()
	content := `schema_version = 1
source_order = ["down", "up"]
`
	if disabled != "" {
		content += fmt.Sprintf("disabled_sources = [%q]\n", disabled)
	}
	content += `
[[custom_sources]]
name = "down"
artifact_url = "http://localhost/{tag}/{asset}"
checksum_url = "http://localhost/{tag}/SHA256SUMS.txt"

[[custom_sources]]
name = "up"
artifact_url = "http://localhost/{tag}/{asset}"
checksum_url = "http://localhost/{tag}/SHA256SUMS.txt"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestInstallDefaultSkipsDisabledSource(t *testing.T) {
	requireFirstPhaseTarget(t)
	root := t.TempDir()
	writeTwoSourceConfig(t, root, "down")
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archiveData := godotArchive(t, "4.5.2", "standard", "enabled source")
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch filepath.Base(request.URL.Path) {
		case asset:
			return response(request, http.StatusOK, archiveData), nil
		case "SHA256SUMS.txt":
			return response(request, http.StatusOK, []byte(digest(archiveData)+"  "+asset+"\n")), nil
		default:
			return response(request, http.StatusNotFound, nil), nil
		}
	})}
	manager, err := New(Options{RootDir: root, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.Source != "up" {
		t.Fatalf("disabled source must be skipped, got source %q", result.Version.Source)
	}
}

func TestInstallWithDisabledSourceFails(t *testing.T) {
	root := t.TempDir()
	writeTwoSourceConfig(t, root, "down")
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard", Source: "down"})
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled source error, got %v", err)
	}
}

func TestSetDefaultSourceRejectsDisabled(t *testing.T) {
	root := t.TempDir()
	writeTwoSourceConfig(t, root, "down")
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	err = manager.SetDefaultSource(context.Background(), "down")
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled source error, got %v", err)
	}
}

func TestAvailableWithDisabledSourceFails(t *testing.T) {
	root := t.TempDir()
	writeTwoSourceConfig(t, root, "down")
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	_, err = manager.Available(context.Background(), "down")
	if !errors.Is(err, ErrInvalidConfig) || !strings.Contains(err.Error(), "disabled") {
		t.Fatalf("expected disabled source error, got %v", err)
	}
}

func TestSetSourceDisabledWritesConfig(t *testing.T) {
	root := t.TempDir()
	content := `schema_version = 1
source_order = ["github", "godothub"]

[environment]
display_driver = "auto"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	if err := manager.SetSourceDisabled(context.Background(), "godothub", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !config.IsSourceDisabled(cfg, "godothub") {
		t.Fatalf("godothub should be disabled: %+v", cfg)
	}
	// 重复 ban 幂等。
	if err := manager.SetSourceDisabled(context.Background(), "godothub", true); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DisabledSources) != 1 {
		t.Fatalf("duplicate ban must be idempotent: %+v", cfg.DisabledSources)
	}
	if err := manager.SetSourceDisabled(context.Background(), "godothub", false); err != nil {
		t.Fatal(err)
	}
	cfg, err = config.Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if config.IsSourceDisabled(cfg, "godothub") {
		t.Fatalf("godothub should be re-enabled: %+v", cfg)
	}
	written, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(written), "display_driver") {
		t.Fatalf("unknown field was lost after write-back: %s", written)
	}
}

func TestSourcesListsConfiguredOrder(t *testing.T) {
	root := t.TempDir()
	content := `schema_version = 1
source_order = ["github", "fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	infos, err := manager.Sources(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(infos) != 2 || infos[0].Name != "github" || infos[0].Kind != "builtin" || infos[1].Name != "fixture" || infos[1].Kind != "custom" {
		t.Fatalf("unexpected sources: %+v", infos)
	}
}

func TestInstallLoadsCustomSourceFromConfig(t *testing.T) {
	requireFirstPhaseTarget(t)
	root := t.TempDir()
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archiveData := godotArchive(t, "4.5.2", "standard", "configured source")
	configFile := `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "http://localhost/{tag}/{asset}"
checksum_url = "http://localhost/{tag}/SHA256SUMS.txt"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(configFile), 0o600); err != nil {
		t.Fatal(err)
	}
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch filepath.Base(request.URL.Path) {
		case asset:
			return response(request, http.StatusOK, archiveData), nil
		case "SHA256SUMS.txt":
			return response(request, http.StatusOK, []byte(digest(archiveData)+"  "+asset+"\n")), nil
		default:
			return response(request, http.StatusNotFound, nil), nil
		}
	})}
	manager, err := New(Options{RootDir: root, HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.Source != "fixture" {
		t.Fatalf("unexpected source: %+v", result.Version)
	}
}

type interruptedBody struct {
	data []byte
}

func (b *interruptedBody) Read(destination []byte) (int, error) {
	if len(b.data) == 0 {
		return 0, io.ErrUnexpectedEOF
	}
	count := copy(destination, b.data)
	b.data = b.data[count:]
	return count, nil
}

func (*interruptedBody) Close() error { return nil }

type stubSource struct {
	name     string
	err      error
	artifact Artifact
	calls    int
}

func (s *stubSource) Name() string { return s.name }

func (s *stubSource) Resolve(context.Context, SourceRequest) (Artifact, error) {
	s.calls++
	if s.err != nil {
		return Artifact{}, s.err
	}
	result := s.artifact
	result.Source = s.name
	return result, nil
}

// godotArchive 构造与真实资产一致的 fixture zip：
// 标准版平铺放置 Godot_v{ver}-stable_linux.x86_64；mono 版为同名目录包裹，
// 内部是可执行文件 Godot_v{ver}-stable_mono_linux.x86_64 与 GodotSharp/ 目录。
func godotArchive(t *testing.T, version, edition, payload string) []byte {
	t.Helper()
	asset, err := platform.AssetName(version, edition, platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	launcher := strings.TrimSuffix(asset, ".zip")
	if edition == "dotnet" {
		launcher = launcher + "/Godot_v" + version + "-stable_mono_linux.x86_64"
	}
	var buffer bytes.Buffer
	writer := zip.NewWriter(&buffer)
	header := &zip.FileHeader{Name: launcher, Method: zip.Deflate}
	header.SetMode(0o755)
	entry, err := writer.CreateHeader(header)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(entry, payload); err != nil {
		t.Fatal(err)
	}
	if edition == "dotnet" {
		dirHeader := &zip.FileHeader{Name: strings.TrimSuffix(asset, ".zip") + "/GodotSharp/", Method: zip.Store}
		dirHeader.SetMode(0o755 | os.ModeDir)
		if _, err := writer.CreateHeader(dirHeader); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func digest512(data []byte) string {
	sum := sha512.Sum512(data)
	return hex.EncodeToString(sum[:])
}

// sha512FixtureSource 返回 SHA-512 摘要，验证 download 层按算法选择 hash。
type sha512FixtureSource struct {
	archives map[string][]byte
}

func (s *sha512FixtureSource) Name() string { return "sha512-fixture" }

func (s *sha512FixtureSource) Resolve(_ context.Context, request SourceRequest) (Artifact, error) {
	asset, err := platform.AssetName(request.Version, request.Edition, platform.Target{OS: request.Target.OS, Arch: request.Target.Arch})
	if err != nil {
		return Artifact{}, err
	}
	data, ok := s.archives[asset]
	if !ok {
		return Artifact{}, fmt.Errorf("fixture asset %s not found", asset)
	}
	return Artifact{
		Source:            s.Name(),
		URL:               "http://localhost/" + asset,
		Filename:          asset,
		ChecksumAlgorithm: "sha512",
		Checksum:          digest512(data),
	}, nil
}

func TestInstallVerifiesSHA512Checksum(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := godotArchive(t, "4.5.2", "standard", "sha512 source")
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: archiveData}
	manager := managerWithFixture(t, t.TempDir(), []Source{&sha512FixtureSource{archives: archives}}, archives)
	result, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.ChecksumAlgorithm != "sha512" {
		t.Fatalf("unexpected checksum algorithm: %+v", result.Version)
	}
	// SHA-512 安装必须对 list 可见，且 state.toml 损坏重建后仍然可见。
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != "4.5.2-standard" || versions[0].ChecksumAlgorithm != "sha512" {
		t.Fatalf("sha512 install must be visible: %+v", versions)
	}
	if err := os.WriteFile(filepath.Join(manager.root, "state.toml"), []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	versions, err = manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ChecksumAlgorithm != "sha512" {
		t.Fatalf("sha512 install lost after state rebuild: %+v", versions)
	}
}

// 摘要算法不匹配时（校验和声明 sha512 但文件是另一个摘要）必须报完整性错误。
func TestInstallRejectsMismatchedSHA512Checksum(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := godotArchive(t, "4.5.2", "standard", "sha512 mismatch")
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	source := &sha512FixtureSource{archives: map[string][]byte{asset: archiveData}}
	source.archives[asset] = archiveData
	wrongDigest := strings.Repeat("0", 128)
	wrong := &wrongChecksumSource{inner: source, checksum: wrongDigest}
	archives := map[string][]byte{asset: archiveData}
	manager := managerWithFixture(t, t.TempDir(), []Source{wrong}, archives)
	_, installErr := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"})
	var integrity IntegrityError
	if !errors.As(installErr, &integrity) {
		t.Fatalf("expected integrity error, got %v", installErr)
	}
}

type wrongChecksumSource struct {
	inner    Source
	checksum string
}

func (s *wrongChecksumSource) Name() string { return s.inner.Name() }

func (s *wrongChecksumSource) Resolve(ctx context.Context, request SourceRequest) (Artifact, error) {
	artifact, err := s.inner.Resolve(ctx, request)
	if err != nil {
		return Artifact{}, err
	}
	artifact.Checksum = s.checksum
	return artifact, nil
}

func requireFirstPhaseTarget(t *testing.T) {
	t.Helper()
	if _, err := platform.CurrentTarget(); err != nil {
		t.Skip("first phase fixture targets Linux amd64")
	}
}
