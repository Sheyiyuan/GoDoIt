package gdit

import (
	"context"
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

// TestParseVersionArg 覆盖 m 前缀简写与普通版本的解析（第二阶段删除后补回）。
func TestParseVersionArg(t *testing.T) {
	for _, test := range []struct {
		input   string
		version string
		edition string
	}{
		{input: "4.5.2", version: "4.5.2", edition: "standard"},
		{input: "m4.5.2", version: "4.5.2", edition: "dotnet"},
		{input: "4.7", version: "4.7", edition: "standard"},
		{input: "4.8-dev3", version: "4.8-dev3", edition: "standard"},
		{input: "m4.7.2-rc1", version: "4.7.2-rc1", edition: "dotnet"},
	} {
		version, edition, err := ParseVersionArg(test.input)
		if err != nil || version != test.version || edition != test.edition {
			t.Fatalf("ParseVersionArg(%q) = %q/%q, %v", test.input, version, edition, err)
		}
	}
	for _, input := range []string{"", "latest", "m", "4.7-dev", "4.5.2.1", "4.7-rc", "4.8-stable"} {
		if _, _, err := ParseVersionArg(input); err == nil {
			t.Fatalf("ParseVersionArg(%q) should fail", input)
		}
	}
}

func TestVersionValidatorsKeepEngineAndSDKDomainsSeparate(t *testing.T) {
	for _, version := range []string{"4.7", "4.8-dev3", "4.7.2-rc1", "4.7.1-beta2"} {
		if err := ValidateEngineVersion(version); err != nil {
			t.Fatalf("supported Godot version %q was rejected: %v", version, err)
		}
		if err := ValidateSDKVersion(version); err == nil {
			t.Fatalf("Godot version %q must not be accepted as an SDK version", version)
		}
	}
	for _, version := range []string{"8.0.410", "11.0.100-preview.7.26381.103", "8.0.100-rc.2.23502.12"} {
		if err := ValidateSDKVersion(version); err != nil {
			t.Fatalf("supported SDK version %q was rejected: %v", version, err)
		}
	}
	if err := ValidateEngineVersion("11.0.100-preview.7.26381.103"); err == nil {
		t.Fatal("SDK preview version must not be accepted as a Godot version")
	}
}

func TestNormalizeEntryRequestAcceptsSupportedVersionForms(t *testing.T) {
	manager, err := New(Options{RootDir: t.TempDir()})
	if err != nil {
		t.Fatal(err)
	}
	for _, version := range []string{"4.7", "4.8-dev3", "4.7.2-rc1", "4.7.1-beta2"} {
		item, normalizeErr := manager.normalizeEntryRequest(context.Background(), InstallEntryRequest{
			Name:    "work",
			Version: version,
			Edition: "standard",
		})
		if normalizeErr != nil || item.Engine.Version != version {
			t.Fatalf("entry version %q was not normalized: %+v err=%v", version, item, normalizeErr)
		}
	}
	item, err := manager.normalizeEntryRequest(context.Background(), InstallEntryRequest{
		Name:        "csharp",
		Version:     "4.8-dev3",
		Edition:     "dotnet",
		SDKStrategy: "managed",
		SDKVersion:  "11.0.100-preview.7.26381.103",
	})
	if err != nil || item.Dotnet == nil || item.Dotnet.Version != "11.0.100-preview.7.26381.103" {
		t.Fatalf("preview SDK entry was not normalized: %+v err=%v", item, err)
	}
}

// TestCompareVersionsDoesNotPanicOnShortInputs 验证缺段版本串不会越界。
func TestCompareVersionsDoesNotPanicOnShortInputs(t *testing.T) {
	for _, pair := range [][2]string{
		{"8.0", "8.0.404"},
		{"8.0.404", "8.0"},
		{"8", "8.0.404"},
		{"", "8.0.404"},
		{"abc", "8.0.404"},
	} {
		if result := compareVersions(pair[0], pair[1]); result == 0 && pair[0] != "" && pair[0] != "abc" {
			t.Fatalf("compareVersions(%q, %q) = 0, want non-zero", pair[0], pair[1])
		}
	}
	_ = compareVersions("8.0", "8.0.404") // 确认不 panic
}

// TestEngineRemoveSuccessRebuildsState 覆盖引擎删除成功路径与 state 重建（第二阶段删除后补回）。
func TestEngineRemoveSuccessRebuildsState(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "engine")}
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	state, err := os.ReadFile(filepath.Join(manager.root, "state.toml"))
	if err != nil || !strings.Contains(string(state), "4.5.2-standard") {
		t.Fatalf("state missing engine: %s err=%v", state, err)
	}
	if err := manager.Remove(context.Background(), "4.5.2-standard"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "engines", "4.5.2-standard")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("engine directory still exists: %v", err)
	}
	state, err = os.ReadFile(filepath.Join(manager.root, "state.toml"))
	if err != nil || strings.Contains(string(state), "4.5.2-standard") {
		t.Fatalf("state not rebuilt after remove: %s err=%v", state, err)
	}
	// 幂等：重复删除报未安装。
	if err := manager.Remove(context.Background(), "4.5.2-standard"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected not installed, got %v", err)
	}
}

// TestRemoveInstanceWithDanglingCurrent 覆盖 current 悬空时不阻断删除非 current 条目。
func TestRemoveInstanceWithDanglingCurrent(t *testing.T) {
	manager := phase3Manager(t)
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "second", Version: "4.5.2", Edition: "standard", SetCurrent: boolPtr(false)}); err != nil {
		t.Fatal(err)
	}
	// 手工制造悬空 current：指向不存在的条目文件。
	if err := os.Remove(filepath.Join(manager.root, "current")); err != nil {
		t.Fatal(err)
	}
	if err := platform.WriteCurrentPointer(manager.root, filepath.Join("instances", "0f8b1c2d-3e4f-4a5b-9c8d-7e6f5a4b3c2d.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.RemoveInstance(context.Background(), "work"); err != nil {
		t.Fatalf("dangling current must not block removing other instances: %v", err)
	}
}

func boolPtr(value bool) *bool { return &value }

// TestInstallEntryEmitsVersionInProgress 覆盖安装进度事件携带版本 ID（第二阶段删除后补回）。
func TestInstallEntryEmitsVersionInProgress(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "standard", "engine")}
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	var stages []string
	manager.progress = func(event ProgressEvent) {
		if event.Stage == "resolve" || event.Stage == "complete" {
			stages = append(stages, event.Stage+":"+event.Version)
		}
	}
	if _, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"}); err != nil {
		t.Fatal(err)
	}
	if len(stages) != 2 || !strings.Contains(stages[0], "4.5.2-standard") || !strings.Contains(stages[1], "4.5.2-standard") {
		t.Fatalf("progress events missing version id: %+v", stages)
	}
}

// TestInstallEntryHoldsSingleGlobalLock 验证组合安装全程只持有一把全局锁：
// 下载阶段（锁内网络 I/O）探测锁被持有；完成后锁释放；嵌套加锁会死锁，由超时 ctx 兜底。
func TestInstallEntryHoldsSingleGlobalLock(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "standard", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archiveData := godotArchive(t, "4.5.2", "standard", "engine")
	downloadStarted := make(chan struct{})
	releaseDownload := make(chan struct{})
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if filepath.Base(request.URL.Path) == asset {
			close(downloadStarted)
			<-releaseDownload
		}
		return response(request, http.StatusOK, archiveData), nil
	})}
	manager, err := New(Options{RootDir: t.TempDir(), HTTPClient: client, Sources: []Source{newFixtureSource("fixture", map[string][]byte{asset: archiveData})}})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	done := make(chan error, 1)
	go func() {
		_, installErr := manager.InstallEntry(ctx, InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard"})
		done <- installErr
	}()
	select {
	case <-downloadStarted:
	case <-ctx.Done():
		t.Fatal("install never reached download")
	}
	lockPath := filepath.Join(manager.root, ".lock")
	held, err := tryFlock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("global lock must be held during the combined install")
	}
	close(releaseDownload)
	if installErr := <-done; installErr != nil {
		t.Fatal(installErr)
	}
	held, err = tryFlock(lockPath)
	if err != nil {
		t.Fatal(err)
	}
	if !held {
		t.Fatal("global lock must be released after the install")
	}
}

// tryFlock 非阻塞尝试获取平台排他锁，返回是否成功获得。
func tryFlock(path string) (bool, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return false, err
	}
	defer file.Close()
	err = platform.LockFile(file)
	if err == nil {
		if releaseErr := platform.ReleaseLock(file); releaseErr != nil {
			return false, releaseErr
		}
		return true, nil
	}
	if errors.Is(err, platform.ErrLocked) {
		return false, nil
	}
	return false, err
}

// TestLockAcquireBlocksConcurrentHolder 验证锁的互斥语义（跨进程级）。
func TestLockAcquireBlocksConcurrentHolder(t *testing.T) {
	path := filepath.Join(t.TempDir(), ".lock")
	first, err := lock.Acquire(context.Background(), path)
	if err != nil {
		t.Fatal(err)
	}
	defer first.Close()
	held, err := tryFlock(path)
	if err != nil {
		t.Fatal(err)
	}
	if held {
		t.Fatal("second locker must be blocked while the first holds the lock")
	}
}
