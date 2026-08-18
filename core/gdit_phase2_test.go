package gdit

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

// installFixture 通过 fixture 来源安装一个标准版或 dotnet 版引擎，返回版本 ID。
func installFixture(t *testing.T, manager *Manager, version, edition string) string {
	t.Helper()
	result, err := manager.Install(context.Background(), InstallRequest{Version: version, Edition: edition})
	if err != nil {
		t.Fatal(err)
	}
	if result.Version.ID != version+"-"+edition {
		t.Fatalf("unexpected installed id: %+v", result.Version)
	}
	return result.Version.ID
}

// managerWithInstalled 返回装好两个版本（standard + dotnet）的 manager。
func managerWithInstalled(t *testing.T) (*Manager, string, string) {
	t.Helper()
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
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	standard := installFixture(t, manager, "4.5.2", "standard")
	dotnet := installFixture(t, manager, "4.5.2", "dotnet")
	return manager, standard, dotnet
}

func TestParseVersionArg(t *testing.T) {
	for _, test := range []struct {
		arg     string
		version string
		edition string
		wantErr bool
	}{
		{arg: "4.5.2", version: "4.5.2", edition: "standard"},
		{arg: "m4.5.2", version: "4.5.2", edition: "dotnet"},
		{arg: "m4.6.2", version: "4.6.2", edition: "dotnet"},
		{arg: " 4.5.2 ", version: "4.5.2", edition: "standard"},
		{arg: "M4.5.2", wantErr: true}, // 前缀仅小写
		{arg: "m", wantErr: true},
		{arg: "", wantErr: true},
		{arg: "m4.x.2", wantErr: true},
		{arg: "4.5", wantErr: true},
		{arg: "latest", wantErr: true},
		{arg: "4.5.2-rc1", wantErr: true},
	} {
		version, edition, err := ParseVersionArg(test.arg)
		if test.wantErr {
			if !errors.Is(err, ErrInvalidInput) {
				t.Fatalf("ParseVersionArg(%q) expected invalid input, got %v", test.arg, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("ParseVersionArg(%q): %v", test.arg, err)
		}
		if version != test.version || edition != test.edition {
			t.Fatalf("ParseVersionArg(%q) = (%q, %q), want (%q, %q)", test.arg, version, edition, test.version, test.edition)
		}
	}
}

func TestDefaultVersionLifecycle(t *testing.T) {
	manager, standard, dotnet := managerWithInstalled(t)

	if _, err := manager.Default(context.Background()); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("expected no-default error before setup, got %v", err)
	}

	if err := manager.SetDefault(context.Background(), standard); err != nil {
		t.Fatal(err)
	}
	id, err := manager.Default(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id != standard {
		t.Fatalf("unexpected default: %q", id)
	}
	target, err := os.Readlink(filepath.Join(manager.root, "current"))
	if err != nil {
		t.Fatal(err)
	}
	if target != filepath.Join("versions", standard) {
		t.Fatalf("unexpected current link target: %q", target)
	}

	// 切换到 dotnet 版。
	if err := manager.SetDefault(context.Background(), dotnet); err != nil {
		t.Fatal(err)
	}
	id, err = manager.Default(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id != dotnet {
		t.Fatalf("unexpected default after switch: %q", id)
	}

	// 未安装的版本不能设为默认，且旧链接保持不变。
	if err := manager.SetDefault(context.Background(), "9.9.9-standard"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected not-installed error, got %v", err)
	}
	id, err = manager.Default(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if id != dotnet {
		t.Fatalf("failed set must keep the old default, got %q", id)
	}

	// 悬空链接：删除默认版本目录后 Default 报错。
	if err := os.RemoveAll(filepath.Join(manager.root, "versions", dotnet)); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Default(context.Background()); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("expected no-default error for dangling link, got %v", err)
	}
}

func TestSetDefaultRejectsInvalidID(t *testing.T) {
	manager, _, _ := managerWithInstalled(t)
	if err := manager.SetDefault(context.Background(), "not-a-version"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

func TestRemoveVersion(t *testing.T) {
	manager, standard, dotnet := managerWithInstalled(t)

	// 当前默认版本拒绝删除。
	if err := manager.SetDefault(context.Background(), standard); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), standard); !errors.Is(err, ErrDefaultInUse) {
		t.Fatalf("expected default-in-use error, got %v", err)
	}

	// 卸载非默认版本后 list 不再出现，state 原子重建。
	if err := manager.Remove(context.Background(), dotnet); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(manager.root, "versions", dotnet)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("version directory must be gone, stat err: %v", err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 1 || versions[0].ID != standard {
		t.Fatalf("unexpected versions after remove: %+v", versions)
	}
	state, err := os.ReadFile(filepath.Join(manager.root, "state.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(state), dotnet) {
		t.Fatalf("removed version still in state.toml: %s", state)
	}

	// 重复卸载与未安装版本。
	if err := manager.Remove(context.Background(), dotnet); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected not-installed error, got %v", err)
	}
	if err := manager.Remove(context.Background(), "9.9.9-standard"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected not-installed error, got %v", err)
	}
	if err := manager.Remove(context.Background(), "bad-id"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}
}

// 悬空 current（默认版本目录被外部删除）不影响删除其他版本。
func TestRemoveWithDanglingCurrent(t *testing.T) {
	manager, standard, dotnet := managerWithInstalled(t)
	if err := manager.SetDefault(context.Background(), standard); err != nil {
		t.Fatal(err)
	}
	if err := os.RemoveAll(filepath.Join(manager.root, "versions", standard)); err != nil {
		t.Fatal(err)
	}
	if err := manager.Remove(context.Background(), dotnet); err != nil {
		t.Fatalf("dangling current must not block removing other versions: %v", err)
	}
	versions, err := manager.List(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 0 {
		t.Fatalf("unexpected versions after remove: %+v", versions)
	}
}

func TestResolveLaunch(t *testing.T) {
	manager, standard, dotnet := managerWithInstalled(t)

	// 未设置默认时按空 ID 解析报错。
	if _, err := manager.ResolveLaunch(context.Background(), ""); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("expected no-default error, got %v", err)
	}

	// 指定版本解析出可执行文件绝对路径。
	target, err := manager.ResolveLaunch(context.Background(), standard)
	if err != nil {
		t.Fatal(err)
	}
	if target.ID != standard || target.Edition != "standard" {
		t.Fatalf("unexpected launch target: %+v", target)
	}
	if !filepath.IsAbs(target.Executable) {
		t.Fatalf("executable must be absolute: %q", target.Executable)
	}
	if info, err := os.Stat(target.Executable); err != nil || info.Mode().Perm()&0o111 == 0 {
		t.Fatalf("executable is not present or not executable: %v, %v", err, info)
	}
	want := filepath.Join(manager.root, "versions", standard, "payload", "Godot_v4.5.2-stable_linux.x86_64")
	if target.Executable != want {
		t.Fatalf("unexpected executable path: %q, want %q", target.Executable, want)
	}

	// 默认版本解析与空 ID 一致。
	if err := manager.SetDefault(context.Background(), dotnet); err != nil {
		t.Fatal(err)
	}
	byDefault, err := manager.ResolveLaunch(context.Background(), "")
	if err != nil {
		t.Fatal(err)
	}
	if byDefault.ID != dotnet {
		t.Fatalf("empty id must resolve the default, got %+v", byDefault)
	}

	// 未安装与非法 ID。
	if _, err := manager.ResolveLaunch(context.Background(), "9.9.9-standard"); !errors.Is(err, ErrNotInstalled) {
		t.Fatalf("expected not-installed error, got %v", err)
	}
	if _, err := manager.ResolveLaunch(context.Background(), "bad-id"); !errors.Is(err, ErrInvalidInput) {
		t.Fatalf("expected invalid input error, got %v", err)
	}

	// 默认指向不完整安装（目录存在但 install.toml 缺失）：空 ID 解析必须报
	// ErrNoDefault 而不是 ErrNotInstalled，与 Default() 的错误分类一致。
	if err := manager.SetDefault(context.Background(), standard); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(manager.root, "versions", standard, "install.toml")); err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ResolveLaunch(context.Background(), ""); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("default pointing at an incomplete install must be ErrNoDefault, got %v", err)
	}
	if _, err := manager.Default(context.Background()); !errors.Is(err, ErrNoDefault) {
		t.Fatalf("Default must agree, got %v", err)
	}
}

func TestSetupCreatesAndRepairsShim(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager, _, _ := managerWithInstalled(t)
	shimPath := filepath.Join(manager.root, "bin", "godot")
	executable, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}

	// 首次创建。
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	target, err := os.Readlink(shimPath)
	if err != nil {
		t.Fatal(err)
	}
	if target != executable {
		t.Fatalf("shim target = %q, want %q", target, executable)
	}

	// 幂等：重复执行不报错，链接不变。
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if target, err = os.Readlink(shimPath); err != nil {
		t.Fatal(err)
	}
	if target != executable {
		t.Fatalf("idempotent setup changed the shim target: %q", target)
	}

	// 指向错误目标时修复。
	if err := os.Remove(shimPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("/wrong/gdit", shimPath); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	if target, err = os.Readlink(shimPath); err != nil {
		t.Fatal(err)
	}
	if target != executable {
		t.Fatalf("wrong shim target was not repaired: %q", target)
	}

	// 普通文件占据 shim 路径时替换为 symlink。
	if err := os.Remove(shimPath); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(shimPath, []byte("stale"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err := os.Lstat(shimPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("stale file was not replaced by a symlink: %v", info.Mode())
	}

	// 空目录占用 shim 路径时替换为 symlink。
	if err := os.Remove(shimPath); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(shimPath, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	info, err = os.Lstat(shimPath)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode()&os.ModeSymlink == 0 {
		t.Fatalf("stale directory was not replaced by a symlink: %v", info.Mode())
	}
}

// 安装进度事件必须携带版本 ID，让 CLI 在批量安装时能区分正在下载的版本。
func TestInstallEmitsVersionInProgress(t *testing.T) {
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
	var events []ProgressEvent
	manager, err := New(Options{
		RootDir:    t.TempDir(),
		HTTPClient: fixtureHTTPClient(archives),
		Sources:    []Source{newFixtureSource("fixture", archives)},
		Progress:   func(event ProgressEvent) { events = append(events, event) },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "dotnet"}); err != nil {
		t.Fatal(err)
	}
	seen := make(map[string]string) // stage -> version
	for _, event := range events {
		switch event.Stage {
		case "resolve", "download", "complete":
			seen[event.Stage] = event.Version
		}
	}
	for _, stage := range []string{"resolve", "download", "complete"} {
		if seen[stage] != "4.5.2-dotnet" {
			t.Fatalf("progress event %q must carry the version id, got %q (events: %+v)", stage, seen[stage], events)
		}
	}
}
