package gdit

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

func phase3Manager(t *testing.T) *Manager {
	t.Helper()
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "engine")}
	return managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
}

// writeRawInstance 在临时根目录手写一个合法条目文件（自动生成 UUID），返回存储标识。
// dotnetText 非空时追加到 [engine] 之后。
func writeRawInstance(t *testing.T, root, name, engineVersion, edition, dotnetText string) string {
	t.Helper()
	id, err := instance.NewID()
	if err != nil {
		t.Fatal(err)
	}
	content := "schema_version = 2\nid = \"" + id + "\"\nname = \"" + name + "\"\n[engine]\nversion = \"" + engineVersion + "\"\nedition = \"" + edition + "\"\n"
	if dotnetText != "" {
		content += dotnetText
	}
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instances", id+".toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestInstanceLifecycleAndCurrentLink(t *testing.T) {
	manager := phase3Manager(t)
	result, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "工作", Version: "4.5.2", Edition: "standard"})
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.Name != "工作" || !result.Instance.Current || len(result.Installed) != 1 {
		t.Fatalf("unexpected install result: %+v", result)
	}
	current, err := os.Readlink(filepath.Join(manager.root, "current"))
	if err != nil {
		t.Fatal(err)
	}
	if current != filepath.Join("instances", result.Instance.ID+".toml") {
		t.Fatalf("unexpected current target: %q", current)
	}
	item, err := manager.Default(context.Background())
	if err != nil || item.Name != "工作" || item.Engine != "4.5.2-standard" {
		t.Fatalf("unexpected default: %+v err=%v", item, err)
	}
	if _, err := manager.RemoveInstance(context.Background(), "工作"); !errors.Is(err, ErrCurrentInstanceInUse) {
		t.Fatalf("current instance must be protected: %v", err)
	}
}

func TestInstallEntryAutoCurrentOnlyForFirstEntry(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "first", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "second", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	current, err := manager.Default(context.Background())
	if err != nil || current.Name != "first" {
		t.Fatalf("second install must not replace current: %+v err=%v", current, err)
	}
	if err := manager.SetDefault(context.Background(), "second"); err != nil {
		t.Fatal(err)
	}
	current, _ = manager.Default(context.Background())
	if current.Name != "second" {
		t.Fatalf("default switch failed: %+v", current)
	}
	if err := manager.SetDefault(context.Background(), "missing"); !errors.Is(err, ErrInstanceNotFound) {
		t.Fatalf("missing instance should not replace current: %v", err)
	}
	current, _ = manager.Default(context.Background())
	if current.Name != "second" {
		t.Fatalf("failed switch changed current: %+v", current)
	}
	removed, err := manager.RemoveInstance(context.Background(), "first")
	if err != nil {
		t.Fatal(err)
	}
	if len(removed.Orphans) != 0 {
		t.Fatalf("shared engine must remain referenced: %+v", removed.Orphans)
	}
}

func TestResolveLaunchMergesEnvironmentWithoutChangingParent(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PARENT_ONLY", "parent")
	if err := manager.SetEnvVar(context.Background(), "", "LAYER", "global"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnvVar(context.Background(), "work", "LAYER", "instance"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnvVar(context.Background(), "work", "display_driver", "wayland"); err != nil {
		t.Fatal(err)
	}
	target, err := manager.ResolveLaunch(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if strings.Join(target.Args, " ") != "--display-driver wayland" {
		t.Fatalf("unexpected injected args: %+v", target.Args)
	}
	joined := strings.Join(target.Env, "\n")
	if !strings.Contains(joined, "LAYER=instance") || !strings.Contains(joined, "PARENT_ONLY=parent") {
		t.Fatalf("unexpected launch environment: %s", joined)
	}
	if os.Getenv("LAYER") != "" {
		t.Fatal("parent environment was modified")
	}
	if err := manager.UnsetEnvVar(context.Background(), "work", "display_driver"); err != nil {
		t.Fatalf("unset display control: %v", err)
	}
	target, err = manager.ResolveLaunch(context.Background(), "work")
	if err != nil || len(target.Args) != 0 {
		t.Fatalf("unset display control did not fall back to auto: %+v err=%v", target.Args, err)
	}
}

func TestConfiguredEnvSeparatesScopesAndMasksSensitiveKeys(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnvVar(context.Background(), "", "API_TOKEN", "secret-token"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnvVar(context.Background(), "", "COMMON", "global"); err != nil {
		t.Fatal(err)
	}
	if err := manager.SetEnvVar(context.Background(), "work", "LOCAL", "instance"); err != nil {
		t.Fatal(err)
	}
	view, err := manager.ConfiguredEnv(context.Background(), "work")
	if err != nil {
		t.Fatal(err)
	}
	if len(view.Vars) < 4 {
		t.Fatalf("configured environment missing entries: %+v", view.Vars)
	}
	var token, local, display *ConfiguredEnvVar
	for index := range view.Vars {
		item := &view.Vars[index]
		switch item.Key {
		case "API_TOKEN":
			token = item
		case "LOCAL":
			local = item
		case "display_driver":
			display = item
		}
	}
	if token == nil || token.Scope != EnvScopeGlobal || !token.Editable || !token.Sensitive || token.Value != "secret-token" {
		t.Fatalf("unexpected sensitive global variable: %+v", token)
	}
	if local == nil || local.Scope != EnvScopeInstance || !local.Editable {
		t.Fatalf("unexpected instance variable: %+v", local)
	}
	if display == nil || display.Scope != EnvScopeGlobal || !display.Editable {
		t.Fatalf("default control variable should remain editable global config: %+v", display)
	}
}

func TestManagedSDKMissingDoesNotAccessNetworkDuringLaunch(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	// 用 standard fixture 的完整目录复制为 dotnet ID，仅构造启动解析所需的完整资产。
	standard := filepath.Join(manager.root, "engines", "4.5.2-standard")
	dotnetDir := filepath.Join(manager.root, "engines", "4.5.2-dotnet")
	if err := copyTestTree(standard, dotnetDir); err != nil {
		t.Fatal(err)
	}
	manifestPath := filepath.Join(dotnetDir, "install.toml")
	content, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}
	updated := strings.ReplaceAll(string(content), "4.5.2-standard", "4.5.2-dotnet")
	updated = strings.ReplaceAll(updated, `edition = "standard"`, `edition = "dotnet"`)
	if err := os.WriteFile(manifestPath, []byte(updated), 0o600); err != nil {
		t.Fatal(err)
	}
	instanceText := "[dotnet]\nstrategy = \"managed\"\nversion = \"8.0.410\"\n"
	writeRawInstance(t, manager.root, "csharp", "4.5.2", "dotnet", instanceText)
	var network atomic.Int64
	manager.client.Transport = roundTripFunc(func(request *http.Request) (*http.Response, error) {
		network.Add(1)
		return nil, errors.New("network must not be used")
	})
	if _, err := manager.ResolveLaunch(context.Background(), "csharp"); !errors.Is(err, ErrNoCompatibleSDK) {
		t.Fatalf("expected missing SDK error, got %v", err)
	}
	if network.Load() != 0 {
		t.Fatalf("launch accessed network %d times", network.Load())
	}
	if err := manager.SetEnvVar(context.Background(), "csharp", "DOTNET_ROOT", "/custom/dotnet"); err != nil {
		t.Fatal(err)
	}
	target, err := manager.ResolveLaunch(context.Background(), "csharp")
	if err != nil {
		t.Fatalf("explicit DOTNET_ROOT should take over SDK selection: %v", err)
	}
	if !strings.Contains(strings.Join(target.Env, "\n"), "DOTNET_ROOT=/custom/dotnet") {
		t.Fatalf("explicit DOTNET_ROOT missing from child environment: %+v", target.Env)
	}
	if network.Load() != 0 {
		t.Fatalf("DOTNET_ROOT takeover accessed network %d times", network.Load())
	}
}

func TestAssetReferenceProtectionAndFailClosedScan(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), "4.5.2-standard"); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("referenced engine must be protected: %v", err)
	}
	if err := os.WriteFile(filepath.Join(manager.root, "instances", "broken.toml"), []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Orphans(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("bad instance must fail closed: %v", err)
	}
	if err := manager.RemoveSDK(context.Background(), "8.0.410"); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("bad instance must block SDK removal: %v", err)
	}
	if _, err := manager.AutoRemove(context.Background()); !errors.Is(err, ErrInvalidConfig) {
		t.Fatalf("bad instance must block autoremove: %v", err)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "engines", "4.5.2-standard")); err != nil {
		t.Fatalf("asset was removed despite failed scan: %v", err)
	}
}

func TestAutoRemoveDeletesOnlyOrphans(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	orphans, err := manager.Orphans(context.Background())
	if err != nil || len(orphans) != 1 || orphans[0].ID != "4.5.2-standard" {
		t.Fatalf("unexpected orphans: %+v err=%v", orphans, err)
	}
	result, err := manager.AutoRemove(context.Background())
	if err != nil || len(result.Removed) != 1 {
		t.Fatalf("unexpected autoremove result: %+v err=%v", result, err)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "engines", "4.5.2-standard")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("orphan engine still exists: %v", err)
	}
}

func TestAutoRemoveRechecksReferencesAfterPreview(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	preview, err := manager.Orphans(context.Background())
	if err != nil || len(preview) != 1 {
		t.Fatalf("unexpected preview: %+v err=%v", preview, err)
	}
	writeRawInstance(t, manager.root, "added", "4.5.2", "standard", "")
	result, err := manager.AutoRemove(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Removed) != 0 {
		t.Fatalf("newly referenced asset was removed: %+v", result.Removed)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "engines", "4.5.2-standard")); err != nil {
		t.Fatalf("referenced engine is missing: %v", err)
	}
}

func TestExplicitOldSDKEmitsWarningWithoutBlocking(t *testing.T) {
	manager := phase3Manager(t)
	var messages []string
	manager.progress = func(event ProgressEvent) {
		if event.Stage == "warning" {
			messages = append(messages, event.Message)
		}
	}
	item, err := manager.normalizeEntryRequest(context.Background(), InstallEntryRequest{Name: "old-sdk", Version: "4.5.2", Edition: "dotnet", SDKStrategy: "managed", SDKVersion: "7.0.410"})
	if err != nil || item.Dotnet.Version != "7.0.410" {
		t.Fatalf("old SDK should be accepted with warning: %+v err=%v", item, err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0], "below the recommended") {
		t.Fatalf("missing compatibility warning: %+v", messages)
	}
}

func TestSetupIsIdempotentAndDoesNotModifyPATH(t *testing.T) {
	root := t.TempDir()
	manager, err := New(Options{RootDir: root})
	if err != nil {
		t.Fatal(err)
	}
	originalPath := filepath.Join(root, "fixture-bin")
	t.Setenv("PATH", originalPath)
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("PATH") != originalPath {
		t.Fatalf("setup modified PATH: %q", os.Getenv("PATH"))
	}
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if created, correct := platform.CheckShim(root, executable); !created || !correct {
		t.Fatalf("shim was not created correctly: created=%v correct=%v", created, correct)
	}
}

func TestInstallDotnetEntryPublishesDependenciesAndInjectsManagedSDK(t *testing.T) {
	requireFirstPhaseTarget(t)
	engineAsset, err := platform.AssetName("4.5.2", "dotnet", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	engineArchive := godotArchive(t, "4.5.2", "dotnet", "dotnet engine")
	sdkData := sdkArchive(t)
	sdkDigest := sha512.Sum512(sdkData)
	metadata := sdkMetadata(hex.EncodeToString(sdkDigest[:]))
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "releases.json"):
			return response(request, http.StatusOK, []byte(metadata)), nil
		case filepath.Base(request.URL.Path) == "sdk.tar.gz":
			return response(request, http.StatusOK, sdkData), nil
		case filepath.Base(request.URL.Path) == engineAsset:
			return response(request, http.StatusOK, engineArchive), nil
		default:
			return response(request, http.StatusNotFound, nil), nil
		}
	})}
	root := t.TempDir()
	source := newFixtureSource("fixture", map[string][]byte{engineAsset: engineArchive})
	manager, err := New(Options{RootDir: root, HTTPClient: client, Sources: []Source{source}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := manager.InstallEntry(ctx, InstallEntryRequest{
		Name:        "csharp",
		Version:     "4.5.2",
		Edition:     "dotnet",
		SDKStrategy: "managed",
		SDKVersion:  "8.0.410",
	})
	if err != nil {
		t.Fatal(err)
	}
	if result.Instance.Name != "csharp" || !result.Instance.Current || len(result.Installed) != 2 {
		t.Fatalf("unexpected combined install result: %+v", result)
	}
	if result.Installed[0] != (AssetChange{Kind: "engine", ID: "4.5.2-dotnet"}) ||
		result.Installed[1] != (AssetChange{Kind: "sdk", ID: "8.0.410"}) {
		t.Fatalf("unexpected installed dependencies: %+v", result.Installed)
	}
	for _, path := range []string{
		filepath.Join(root, "engines", "4.5.2-dotnet", "install.toml"),
		filepath.Join(root, "sdks", "8.0.410", "install.toml"),
		filepath.Join(root, "instances", result.Instance.ID+".toml"),
	} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("combined install did not publish %s: %v", path, err)
		}
	}

	target, err := manager.ResolveLaunch(context.Background(), "csharp")
	if err != nil {
		t.Fatal(err)
	}
	managedRoot := filepath.Join(root, "sdks", "8.0.410")
	joined := strings.Join(target.Env, "\n")
	if !strings.Contains(joined, "DOTNET_ROOT="+managedRoot) ||
		!strings.Contains(joined, "PATH="+managedRoot+string(filepath.ListSeparator)) {
		t.Fatalf("managed SDK was not injected into the child environment: %s", joined)
	}
	if err := manager.RemoveSDK(context.Background(), "8.0.410"); !errors.Is(err, ErrAssetInUse) {
		t.Fatalf("referenced managed SDK must be protected: %v", err)
	}
}

func TestGodot3MonoEntrySkipsSDK(t *testing.T) {
	requireFirstPhaseTarget(t)
	engineAsset, err := platform.AssetName("3.6.2", "dotnet", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	engineArchive := godotArchive(t, "3.6.2", "dotnet", "mono engine")
	root := t.TempDir()
	archives := map[string][]byte{engineAsset: engineArchive}
	source := newFixtureSource("fixture", archives)
	manager := managerWithFixture(t, root, []Source{source}, archives)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result, err := manager.InstallEntry(ctx, InstallEntryRequest{Name: "old", Version: "3.6.2", Edition: "dotnet"})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Installed) != 1 || result.Installed[0] != (AssetChange{Kind: "engine", ID: "3.6.2-dotnet"}) {
		t.Fatalf("3.x mono entry must install only the engine, no SDK: %+v", result.Installed)
	}
	item, err := instance.Read(root, result.Instance.ID)
	if err != nil {
		t.Fatal(err)
	}
	if item.Dotnet == nil || item.Dotnet.Strategy != "mono" || item.Dotnet.Version != "" {
		t.Fatalf("3.x dotnet entry must carry the mono strategy: %+v", item.Dotnet)
	}
	target, err := manager.ResolveLaunch(ctx, "old")
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(target.Env, "\n")
	// 只要求不注入托管 SDK 目录；父进程环境里既有的 DOTNET_ROOT（如系统 dotnet）原样继承。
	if strings.Contains(joined, "DOTNET_ROOT="+filepath.Join(root, "sdks")) {
		t.Fatalf("3.x mono launch must not inject a managed SDK root: %s", joined)
	}
	if _, err := manager.InstallEntry(ctx, InstallEntryRequest{Name: "bad", Version: "3.6.2", Edition: "dotnet", SDKStrategy: "managed"}); err == nil {
		t.Fatal("SDK options must be rejected for Godot 3.x entries")
	}
}

func TestSystemSDKBelowRecommendationEmitsWarning(t *testing.T) {
	requireFirstPhaseTarget(t)
	engineAsset, err := platform.AssetName("4.5.2", "dotnet", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{engineAsset: godotArchive(t, "4.5.2", "dotnet", "dotnet engine")}
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	manager.sdkProbe = func(context.Context) ([]SDKInfo, error) {
		return []SDKInfo{{Version: "6.0.428", Kind: "system", Path: "/usr/lib/dotnet/sdk"}}, nil
	}
	var messages []string
	manager.progress = func(event ProgressEvent) {
		if event.Stage == "warning" {
			messages = append(messages, event.Message)
		}
	}
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{
		Name: "system-sdk", Version: "4.5.2", Edition: "dotnet", SDKStrategy: "system",
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveLaunch(context.Background(), "system-sdk"); err != nil {
		t.Fatal(err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0], "below the recommended 8.0 major") {
		t.Fatalf("missing system SDK compatibility warning: %+v", messages)
	}
	manager.sdkProbe = func(context.Context) ([]SDKInfo, error) {
		return nil, errors.New("probe failed")
	}
	messages = nil
	if _, err := manager.ResolveLaunch(context.Background(), "system-sdk"); !errors.Is(err, ErrNoCompatibleSDK) {
		t.Fatalf("failed system probe should become a missing SDK error: %v", err)
	}
	if len(messages) != 1 || !strings.Contains(messages[0], "system SDK probe failed") {
		t.Fatalf("system probe failure was not reported: %+v", messages)
	}
	manager.sdkProbe = func(context.Context) ([]SDKInfo, error) {
		return nil, context.Canceled
	}
	messages = nil
	if _, err := manager.ResolveLaunch(context.Background(), "system-sdk"); !errors.Is(err, context.Canceled) {
		t.Fatalf("system probe cancellation was not propagated: %v", err)
	}
	if len(messages) != 0 {
		t.Fatalf("cancellation should not be downgraded to a warning: %+v", messages)
	}
}

func TestAvailableSDKsGroupsChannels(t *testing.T) {
	index := `{"releases-index":[
		{"channel-version":"11.0","support-phase":"preview","release-type":"sts"},
		{"channel-version":"10.0","support-phase":"active","release-type":"lts"},
		{"channel-version":"9.0","support-phase":"maintenance","release-type":"sts"},
		{"channel-version":"8.0","support-phase":"maintenance","release-type":"lts"},
		{"channel-version":"7.0","support-phase":"eol","release-type":"sts"}
	]}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "releases-index.json") {
			return response(request, http.StatusOK, []byte(index)), nil
		}
		var metadata string
		switch {
		case strings.Contains(request.URL.Path, "/11.0/"):
			metadata = sdkMetadataForVersion("11.0.100-preview.7.26381.103", strings.Repeat("e", 128))
		case strings.Contains(request.URL.Path, "/10.0/"):
			metadata = sdkMetadataForVersion("10.0.400", strings.Repeat("a", 128))
		case strings.Contains(request.URL.Path, "/9.0/"):
			metadata = sdkMetadataForVersion("9.0.317", strings.Repeat("b", 128))
		case strings.Contains(request.URL.Path, "/8.0/"):
			metadata = sdkMetadataForVersion("8.0.410", strings.Repeat("c", 128))
		case strings.Contains(request.URL.Path, "/6.0/"):
			metadata = sdkMetadataForVersion("6.0.428", strings.Repeat("d", 128))
		default:
			return response(request, http.StatusNotFound, nil), nil
		}
		return response(request, http.StatusOK, []byte(metadata)), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	channels, err := manager.AvailableSDKs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 5 {
		t.Fatalf("expected 5 channels (eol skipped, 6.0 and preview kept): %+v", channels)
	}
	got := []string{}
	for _, channel := range channels {
		got = append(got, channel.MajorMinor)
	}
	if strings.Join(got, ",") != "11.0,10.0,9.0,8.0,6.0" {
		t.Fatalf("unexpected channel order: %+v", got)
	}
	if channels[0].Phase != "preview" || strings.Join(channels[0].Versions, ",") != "11.0.100-preview.7.26381.103" {
		t.Fatalf("preview channel must list prerelease SDKs: %+v", channels[0])
	}
	if channels[1].Phase != "active" || channels[1].ReleaseType != "lts" || strings.Join(channels[1].Versions, ",") != "10.0.400" {
		t.Fatalf("unexpected 10.0 channel: %+v", channels[1])
	}
	if channels[4].Phase != "eol" || strings.Join(channels[4].Versions, ",") != "6.0.428" {
		t.Fatalf("6.0 fallback channel must be kept and marked eol: %+v", channels[4])
	}
}

func TestAvailableSDKsFallsBackToStaticChannels(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "releases-index.json") {
			return response(request, http.StatusNotFound, nil), nil
		}
		if strings.Contains(request.URL.Path, "/8.0/") {
			return response(request, http.StatusOK, []byte(sdkMetadataForVersion("8.0.410", strings.Repeat("c", 128)))), nil
		}
		return response(request, http.StatusNotFound, nil), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	var messages []string
	manager.progress = func(event ProgressEvent) {
		if event.Stage == "warning" {
			messages = append(messages, event.Message)
		}
	}
	channels, err := manager.AvailableSDKs(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 1 || channels[0].MajorMinor != "8.0" || strings.Join(channels[0].Versions, ",") != "8.0.410" {
		t.Fatalf("fallback channels should still enumerate reachable metadata: %+v", channels)
	}
	if len(messages) == 0 || !strings.Contains(messages[0], "channel index unavailable") {
		t.Fatalf("missing channel index fallback warning: %+v", messages)
	}
}

func TestInstallSDKFixturePublishesAndRebuildsState(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := sdkArchive(t)
	digestBytes := sha512.Sum512(archiveData)
	digest := hex.EncodeToString(digestBytes[:])
	metadata := sdkMetadata(digest)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "releases.json") {
			return response(request, http.StatusOK, []byte(metadata)), nil
		}
		return response(request, http.StatusOK, archiveData), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, SDKProbe: func(context.Context) ([]SDKInfo, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	result, err := manager.InstallSDK(context.Background(), "8.0.410")
	if err != nil {
		t.Fatal(err)
	}
	if result.SDK.Version != "8.0.410" || result.StateRebuildRequired {
		t.Fatalf("unexpected SDK result: %+v", result)
	}
	launcher := filepath.Join(manager.root, "sdks", "8.0.410", "dotnet")
	if info, err := os.Stat(launcher); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("SDK launcher was not published executable: %v %v", info, err)
	}
	state, err := os.ReadFile(filepath.Join(manager.root, "state.toml"))
	if err != nil || !strings.Contains(string(state), "8.0.410") {
		t.Fatalf("state missing SDK: %s err=%v", state, err)
	}
}

// sdkMetadataOfficialURL 与 sdkMetadataForVersion 相同，但资产 URL 指向官方 host，
// 用于验证镜像 fallback（镜像 URL 由官方 URL 推导）。
func sdkMetadataOfficialURL(digest string) string {
	return `{"releases":[{"sdk":{"version":"8.0.410","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://builds.dotnet.microsoft.com/dotnet/Sdk/8.0.410/dotnet-sdk-8.0.410-linux-x64.tar.gz","hash":"` + digest + `"}]}}]}`
}

func TestInstallSDKMirrorPreferred(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := sdkArchive(t)
	digestBytes := sha512.Sum512(archiveData)
	digest := hex.EncodeToString(digestBytes[:])
	metadata := sdkMetadataOfficialURL(digest)
	var mirrorCalls, officialCalls int
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "releases.json"):
			return response(request, http.StatusOK, []byte(metadata)), nil
		case request.URL.Host == "mirrors.huaweicloud.com":
			mirrorCalls++
			return response(request, http.StatusOK, archiveData), nil
		default:
			officialCalls++
			return response(request, http.StatusOK, archiveData), nil
		}
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, SDKProbe: func(context.Context) ([]SDKInfo, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallSDK(context.Background(), "8.0.410"); err != nil {
		t.Fatal(err)
	}
	if mirrorCalls != 1 || officialCalls != 0 {
		t.Fatalf("mirror must be tried first and succeed alone: mirror=%d official=%d", mirrorCalls, officialCalls)
	}
	manifest, err := os.ReadFile(filepath.Join(manager.root, "sdks", "8.0.410", "install.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `source = "dotnet-huaweicloud"`) {
		t.Fatalf("manifest must record the mirror source: %s", manifest)
	}
}

func TestInstallSDKFallsBackFromMirror(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := sdkArchive(t)
	digestBytes := sha512.Sum512(archiveData)
	digest := hex.EncodeToString(digestBytes[:])
	metadata := sdkMetadataOfficialURL(digest)
	var officialCalls int
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch {
		case strings.HasSuffix(request.URL.Path, "releases.json"):
			return response(request, http.StatusOK, []byte(metadata)), nil
		case request.URL.Host == "mirrors.huaweicloud.com":
			return response(request, http.StatusTeapot, nil), nil
		default:
			officialCalls++
			return response(request, http.StatusOK, archiveData), nil
		}
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, SDKProbe: func(context.Context) ([]SDKInfo, error) { return nil, nil }})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallSDK(context.Background(), "8.0.410"); err != nil {
		t.Fatal(err)
	}
	if officialCalls != 1 {
		t.Fatalf("official source must be used after the mirror returns 418: official=%d", officialCalls)
	}
	manifest, err := os.ReadFile(filepath.Join(manager.root, "sdks", "8.0.410", "install.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(manifest), `source = "dotnet-official"`) {
		t.Fatalf("manifest must record the official source after fallback: %s", manifest)
	}
}

func TestInstallSDKChecksumFailureDoesNotPublish(t *testing.T) {
	requireFirstPhaseTarget(t)
	archiveData := sdkArchive(t)
	metadata := sdkMetadata(strings.Repeat("0", 128))
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "releases.json") {
			return response(request, http.StatusOK, []byte(metadata)), nil
		}
		return response(request, http.StatusOK, archiveData), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallSDK(context.Background(), "8.0.410"); err == nil {
		t.Fatal("expected checksum failure")
	} else {
		var integrity IntegrityError
		if !errors.As(err, &integrity) {
			t.Fatalf("expected IntegrityError, got %v", err)
		}
	}
	if _, err := os.Stat(filepath.Join(manager.root, "sdks", "8.0.410")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("failed SDK was published: %v", err)
	}
}

func sdkArchive(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	gzipWriter := gzip.NewWriter(&buffer)
	tarWriter := tar.NewWriter(gzipWriter)
	// 微软 dotnet-sdk tar 包顶层带 "./" 目录条目，fixture 与真实结构一致。
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./", Mode: 0o755, Typeflag: tar.TypeDir}); err != nil {
		t.Fatal(err)
	}
	content := []byte("#!/bin/sh\nexit 0\n")
	if err := tarWriter.WriteHeader(&tar.Header{Name: "./dotnet", Mode: 0o755, Typeflag: tar.TypeReg, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tarWriter.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tarWriter.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gzipWriter.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

func sdkMetadata(digest string) string {
	return sdkMetadataForVersion("8.0.410", digest)
}

func sdkMetadataForVersion(version, digest string) string {
	return `{"releases":[{"sdk":{"version":"` + version + `","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://localhost/sdk.tar.gz","hash":"` + digest + `"}]}}]}`
}

func copyTestTree(source, destination string) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o755)
	})
}
