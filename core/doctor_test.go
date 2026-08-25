package gdit

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

// doctorManager 创建带一个完整条目（standard 引擎已安装、current 已设置）的 manager。
func doctorManager(t *testing.T) *Manager {
	t.Helper()
	requireFirstPhaseTarget(t)
	manager := phase3Manager(t)
	result, err := manager.InstallEntry(context.Background(), InstallEntryRequest{Name: "work", Version: "4.5.2", Edition: "standard", SetCurrent: boolPtr(true)})
	if err != nil {
		t.Fatal(err)
	}
	if !result.Instance.Current {
		t.Fatal("expected current instance")
	}
	return manager
}

func doctorItem(report DoctorReport, code string) *CheckResult {
	for index := range report.Items {
		if report.Items[index].Code == code {
			return &report.Items[index]
		}
	}
	return nil
}

func TestDoctorHealthySetup(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if report.ErrorCount != 0 {
		t.Fatalf("healthy setup should have no errors: %+v", report.Items)
	}
	if report.Root != manager.root {
		t.Fatalf("report root mismatch: %q vs %q", report.Root, manager.root)
	}
	for _, code := range []string{"platform", "root-dir", "shim", "current", "instances", "engines", "sdks", "templates", "environment", "sources", "state"} {
		if item := doctorItem(report, code); item == nil {
			t.Fatalf("missing check item %s in %+v", code, report.Items)
		} else if item.Status == StatusError {
			t.Fatalf("unexpected error in %s: %+v", code, item)
		}
	}
}

func TestDoctorZeroWrites(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	// 记录执行前根目录的完整内容清单（相对路径 + 类型 + 内容哈希），执行后逐字节比对。
	before := snapshotRoot(t, manager.root)
	if _, err := manager.Doctor(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	after := snapshotRoot(t, manager.root)
	if len(before) != len(after) {
		t.Fatalf("doctor changed root contents: %d -> %d entries", len(before), len(after))
	}
	for path, content := range before {
		if after[path] != content {
			t.Fatalf("doctor changed %s", path)
		}
	}
}

func TestDoctorHonorsCanceledContext(t *testing.T) {
	manager := doctorManager(t)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := manager.Doctor(ctx, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("doctor must return context cancellation: %v", err)
	}
}

func snapshotRoot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := make(map[string]string)
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, relErr := filepath.Rel(root, path)
		if relErr != nil {
			return relErr
		}
		if entry.IsDir() {
			result[rel+"/"] = "dir"
			return nil
		}
		if entry.Type()&os.ModeSymlink != 0 {
			target, readErr := os.Readlink(path)
			if readErr != nil {
				return readErr
			}
			result[rel] = "link:" + target
			return nil
		}
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			return readErr
		}
		result[rel] = string(content)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}

func TestDoctorReportsFaults(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	// 悬空 current：替换为指向不存在的条目。
	if err := os.Remove(filepath.Join(manager.root, "current")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("instances", "deadbeef-0000-4000-8000-000000000000.toml"), filepath.Join(manager.root, "current")); err != nil {
		t.Fatal(err)
	}
	// 无效引擎目录：伪造一个缺 install.toml 的目录。
	if err := os.MkdirAll(filepath.Join(manager.root, "engines", "9.9.9-standard"), 0o755); err != nil {
		t.Fatal(err)
	}
	// state.toml 损坏。
	if err := os.WriteFile(filepath.Join(manager.root, "state.toml"), []byte("broken = ["), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "current"); item == nil || item.Status != StatusError {
		t.Fatalf("dangling current should be an error: %+v", item)
	}
	if item := doctorItem(report, "engines"); item == nil || item.Status != StatusError {
		t.Fatalf("invalid engine dir should be an error: %+v", item)
	}
	if item := doctorItem(report, "state"); item == nil || item.Status != StatusWarn {
		t.Fatalf("broken state.toml should be a warning: %+v", item)
	}
	if report.ErrorCount == 0 {
		t.Fatalf("expected errors: %+v", report.Items)
	}
}

func TestDoctorBadInstanceFailsClosed(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	// 写入一个 schema 非法的条目。
	writeRawInstance(t, manager.root, "broken", "4.5.2", "standard", "")
	if err := os.WriteFile(filepath.Join(manager.root, "instances", "not-a-uuid.toml"), []byte("schema_version = 2\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "instances"); item == nil || item.Status != StatusError {
		t.Fatalf("bad instance should fail closed: %+v", item)
	}
}

func TestDoctorNoCurrentWarns(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := phase3Manager(t)
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "current"); item == nil || item.Status != StatusWarn {
		t.Fatalf("missing current should warn: %+v", item)
	}
	if item := doctorItem(report, "environment"); item == nil || item.Status != StatusWarn {
		t.Fatalf("no current should make environment warn: %+v", item)
	}
}

func TestDoctorMasksSensitiveValues(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	if err := manager.SetEnvVar(context.Background(), "work", "GODOT_API_TOKEN", "super-secret-value"); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "environment"); item == nil {
		t.Fatal("missing environment check")
	} else if joined := strings.Join(item.Details, "\n"); strings.Contains(joined, "super-secret-value") {
		t.Fatalf("sensitive value leaked: %s", joined)
	} else if !strings.Contains(joined, "******") {
		t.Fatalf("sensitive value not masked: %s", joined)
	}
}

func TestDoctorNetworkProbe(t *testing.T) {
	requireFirstPhaseTarget(t)
	// fixture 服务器：HEAD 元数据端点返回 200。
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	// 注入带元数据端点的来源：doctor 只探测 endpoint，不下载资产，用默认 client。
	fixture := &metadataFixtureSource{name: "fixture", endpoint: server.URL + "/releases.json"}
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{fixture}})
	if err != nil {
		t.Fatal(err)
	}

	report, err := manager.Doctor(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	item := doctorItem(report, "sources")
	if item == nil || item.Status != StatusOK || !strings.Contains(item.Message, "可达") {
		t.Fatalf("reachable source should be OK: %+v", item)
	}

	// 服务器关闭：探测失败按 warn 处理，不视为错误。
	server.Close()
	report, err = manager.Doctor(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	item = doctorItem(report, "sources")
	if item == nil || item.Status != StatusWarn {
		t.Fatalf("unreachable source should warn: %+v", item)
	}
}

func TestDoctorNetworkProbeAllFailedIsError(t *testing.T) {
	requireFirstPhaseTarget(t)
	fixture := &metadataFixtureSource{name: "fixture", endpoint: "http://127.0.0.1:1/releases.json"}
	manager, err := New(Options{RootDir: t.TempDir(), Sources: []Source{fixture}})
	if err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), true)
	if err != nil {
		t.Fatal(err)
	}
	// 单个来源失败记 warn，全部失败再汇总一条 error。
	foundError := false
	for index := range report.Items {
		if report.Items[index].Code == "sources" && report.Items[index].Status == StatusError {
			foundError = true
			break
		}
	}
	if !foundError {
		t.Fatalf("all sources unreachable should be an error: %+v", report.Items)
	}
}

// metadataFixtureSource 是带元数据端点的 fixture 来源（doctor --network 探测用）。
type metadataFixtureSource struct {
	name     string
	endpoint string
	archives map[string][]byte
}

func (s *metadataFixtureSource) Name() string { return s.name }
func (s *metadataFixtureSource) MetadataEndpoint() string {
	return s.endpoint
}
func (s *metadataFixtureSource) Resolve(_ context.Context, request SourceRequest) (Artifact, error) {
	asset, err := platform.AssetName(request.Version, request.Edition, platform.Target{OS: request.Target.OS, Arch: request.Target.Arch})
	if err != nil {
		return Artifact{}, err
	}
	data, ok := s.archives[asset]
	if !ok {
		return Artifact{}, errors.New("fixture asset not found")
	}
	return Artifact{Source: s.name, URL: "http://localhost/" + asset, Filename: asset, ChecksumAlgorithm: "sha256", Checksum: digest(data)}, nil
}

func TestDoctorCheckShimWarnsWithoutSetup(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	// 未执行 setup 时 shim 不存在 → warn（不是 error）。
	if item := doctorItem(report, "shim"); item == nil || item.Status != StatusWarn {
		t.Fatalf("missing shim should warn: %+v", item)
	}
}

func TestDoctorCheckShimOKAfterSetup(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	if err := manager.Setup(context.Background()); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "shim"); item == nil || item.Status != StatusOK {
		t.Fatalf("shim after setup should be OK: %+v", item)
	}
}

func TestDoctorReportsMissingManagedSDKSeparately(t *testing.T) {
	requireFirstPhaseTarget(t)
	asset, err := platform.AssetName("4.5.2", "dotnet", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil {
		t.Fatal(err)
	}
	archives := map[string][]byte{asset: godotArchive(t, "4.5.2", "dotnet", "engine")}
	manager := managerWithFixture(t, t.TempDir(), []Source{newFixtureSource("fixture", archives)}, archives)
	if _, err := manager.Install(context.Background(), InstallRequest{Version: "4.5.2", Edition: "dotnet"}); err != nil {
		t.Fatal(err)
	}
	writeRawInstance(t, manager.root, "dotnet-work", "4.5.2", "dotnet", "[dotnet]\nstrategy = \"managed\"\nversion = \"8.0.410\"\n")
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	item := doctorItem(report, "instances")
	if item == nil || item.Status != StatusError || !strings.Contains(item.Message, "托管 SDK 8.0.410") {
		t.Fatalf("missing managed SDK should be reported separately: %+v", item)
	}
}

func TestDoctorReportsMissingEngineAndManagedSDKTogether(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := phase3Manager(t)
	writeRawInstance(t, manager.root, "dotnet-work", "4.5.2", "dotnet", "[dotnet]\nstrategy = \"managed\"\nversion = \"8.0.410\"\n")
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	messages := make([]string, 0, 2)
	for _, item := range report.Items {
		if item.Code == "instances" && item.Status == StatusError {
			messages = append(messages, item.Message)
		}
	}
	joined := strings.Join(messages, "\n")
	if !strings.Contains(joined, "引擎 4.5.2-dotnet") || !strings.Contains(joined, "托管 SDK 8.0.410") {
		t.Fatalf("doctor did not collect both reference errors: %s", joined)
	}
}

func TestDoctorMissingStateWithAssetsWarns(t *testing.T) {
	manager := doctorManager(t)
	if err := os.Remove(filepath.Join(manager.root, "state.toml")); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "state"); item == nil || item.Status != StatusWarn {
		t.Fatalf("missing state with installed assets should warn: %+v", item)
	}
}

func TestDoctorReportsInvalidSDKDirectory(t *testing.T) {
	manager := doctorManager(t)
	if err := os.MkdirAll(filepath.Join(manager.root, "sdks", "not-a-version"), 0o755); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "sdks"); item == nil || item.Status != StatusError {
		t.Fatalf("invalid SDK directory should be an error: %+v", item)
	}
}

func TestDoctorReportsInvalidCurrentPointer(t *testing.T) {
	manager := doctorManager(t)
	current := filepath.Join(manager.root, "current")
	if err := os.Remove(current); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(filepath.Join("..", "outside.toml"), current); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	if item := doctorItem(report, "current"); item == nil || item.Status != StatusError {
		t.Fatalf("invalid current pointer should be an error: %+v", item)
	}
}

func TestDoctorDoesNotAcquireGlobalLock(t *testing.T) {
	manager := doctorManager(t)
	manager.sdkProbe = func(context.Context) ([]SDKInfo, error) { return nil, nil }
	guard, err := lock.Acquire(context.Background(), store.New(manager.root).LockPath())
	if err != nil {
		t.Fatal(err)
	}
	defer guard.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	if _, err := manager.Doctor(ctx, false); err != nil {
		t.Fatalf("doctor waited for the global modification lock: %v", err)
	}
}

func TestDoctorDefaultDoesNotProbeSources(t *testing.T) {
	var requests atomic.Int64
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		requests.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()
	manager, err := New(Options{
		RootDir:  t.TempDir(),
		Sources:  []Source{&metadataFixtureSource{name: "fixture", endpoint: server.URL}},
		SDKProbe: func(context.Context) ([]SDKInfo, error) { return nil, nil },
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.Doctor(context.Background(), false); err != nil {
		t.Fatal(err)
	}
	if requests.Load() != 0 {
		t.Fatalf("doctor without --network made %d source requests", requests.Load())
	}
}

func TestDoctorReportsSourceConfigurationFaults(t *testing.T) {
	const missingAuthorizationEnv = "GDIT_DOCTOR_TEST_MISSING_TOKEN"
	previousValue, previouslySet := os.LookupEnv(missingAuthorizationEnv)
	if err := os.Unsetenv(missingAuthorizationEnv); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if previouslySet {
			_ = os.Setenv(missingAuthorizationEnv, previousValue)
			return
		}
		_ = os.Unsetenv(missingAuthorizationEnv)
	})
	tests := []struct {
		name    string
		config  string
		status  CheckStatus
		message string
	}{
		{
			name:    "invalid config",
			config:  "schema_version = [\n",
			status:  StatusError,
			message: "来源初始化失败",
		},
		{
			name: "invalid custom template",
			config: `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{branch}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`,
			status:  StatusError,
			message: "来源初始化失败",
		},
		{
			name: "missing authorization environment",
			config: `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "GDIT_DOCTOR_TEST_MISSING_TOKEN"
`,
			status:  StatusWarn,
			message: "授权变量 GDIT_DOCTOR_TEST_MISSING_TOKEN 未设置",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(test.config), 0o600); err != nil {
				t.Fatal(err)
			}
			manager, err := New(Options{RootDir: root, SDKProbe: func(context.Context) ([]SDKInfo, error) { return nil, nil }})
			if err != nil {
				t.Fatal(err)
			}
			report, err := manager.Doctor(context.Background(), false)
			if err != nil {
				t.Fatal(err)
			}
			item := doctorItem(report, "sources")
			if item == nil || item.Status != test.status || !strings.Contains(item.Message, test.message) {
				t.Fatalf("unexpected sources result: %+v", item)
			}
		})
	}
}

func TestDoctorMaskSensitiveEvenWhenVerboseNotInCore(t *testing.T) {
	requireFirstPhaseTarget(t)
	manager := doctorManager(t)
	if err := manager.SetEnvVar(context.Background(), "work", "API_KEY", "secret-12345"); err != nil {
		t.Fatal(err)
	}
	report, err := manager.Doctor(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	serialized := reportToString(report)
	if strings.Contains(serialized, "secret-12345") {
		t.Fatalf("sensitive value leaked: %s", serialized)
	}
}

func reportToString(report DoctorReport) string {
	var builder strings.Builder
	for _, item := range report.Items {
		builder.WriteString(item.Code)
		builder.WriteString(" ")
		builder.WriteString(item.Message)
		builder.WriteString("\n")
		for _, detail := range item.Details {
			builder.WriteString(detail)
			builder.WriteString("\n")
		}
	}
	return builder.String()
}

var _ = instance.SchemaVersion // 保持 instance 导入（writeRawInstance 同包，无需此引用）
