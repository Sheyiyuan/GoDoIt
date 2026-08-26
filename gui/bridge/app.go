package bridge

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"strings"
	"sync"
	"time"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const progressEventName = "gdit:progress"

type managerFactory func(progress func(gdit.ProgressEvent)) (*gdit.Manager, error)
type eventEmitter func(string, any)
type closePrompter func(context.Context) (bool, error)

// App 是 Wails 绑定对象，只保存生命周期上下文和可取消操作表。
type App struct {
	root        string
	ctx         context.Context
	mu          sync.Mutex
	operations  map[string]context.CancelFunc
	newManager  managerFactory
	emit        eventEmitter
	promptClose closePrompter
}

// ResolveRoot 委托 core 平台适配层解析 GUI 使用的 gdit 根目录。
func ResolveRoot() (string, error) { return gdit.DefaultRoot() }

// NewApp 创建 GUI bridge；不会访问网络或创建用户目录。
func NewApp(root string) *App {
	app := &App{root: root, ctx: context.Background(), operations: make(map[string]context.CancelFunc)}
	app.newManager = func(progress func(gdit.ProgressEvent)) (*gdit.Manager, error) {
		return gdit.New(gdit.Options{RootDir: root, Progress: progress})
	}
	app.promptClose = func(ctx context.Context) (bool, error) {
		choice, err := wailsruntime.MessageDialog(ctx, wailsruntime.MessageDialogOptions{
			Type:          wailsruntime.QuestionDialog,
			Title:         "仍有任务进行中",
			Message:       "继续等待任务完成，或取消全部任务并退出。",
			Buttons:       []string{"继续等待", "取消任务并退出"},
			DefaultButton: "继续等待",
			CancelButton:  "继续等待",
		})
		return choice == "取消任务并退出", err
	}
	return app
}

// Startup 返回 Wails 生命周期回调，避免把生命周期方法暴露为绑定 API。
func Startup(app *App) func(context.Context) {
	return func(ctx context.Context) {
		app.ctx = ctx
		app.emit = func(name string, data any) { wailsruntime.EventsEmit(ctx, name, data) }
	}
}

// BeforeClose 在有操作进行时要求用户选择继续等待或取消后退出。
func BeforeClose(app *App) func(context.Context) bool {
	return func(ctx context.Context) bool {
		if app.activeOperationCount() == 0 {
			return false
		}
		cancelAndExit, err := app.promptClose(ctx)
		if err != nil || !cancelAndExit {
			return true
		}
		app.cancelAll()
		return false
	}
}

// Bootstrap 读取首屏完整快照。局部读取错误进入 issues，仍返回 doctor 供用户诊断。
func (a *App) Bootstrap() (AppSnapshot, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return AppSnapshot{}, err
	}
	if err := manager.Initialize(a.ctx); err != nil {
		return AppSnapshot{Root: a.root}, err
	}
	snapshot := AppSnapshot{Root: a.root, Build: readBuildInfo(), GUI: gdit.GUISettings{TitlebarStyle: "auto"}}
	if snapshot.GUI, err = manager.GUISettings(a.ctx); err != nil {
		snapshot.Issues = append(snapshot.Issues, err.Error())
		snapshot.GUI = gdit.GUISettings{TitlebarStyle: "auto"}
	}
	if snapshot.Instances, err = manager.Instances(a.ctx); err != nil {
		snapshot.Issues = append(snapshot.Issues, err.Error())
	}
	if current, currentErr := manager.Default(a.ctx); currentErr == nil {
		snapshot.Current = &current
	} else if !errors.Is(currentErr, gdit.ErrNoDefault) {
		snapshot.Issues = append(snapshot.Issues, currentErr.Error())
	}
	if snapshot.Assets, err = listAssets(a.ctx, manager); err != nil {
		snapshot.Issues = append(snapshot.Issues, err.Error())
	}
	if snapshot.Doctor, err = manager.Doctor(a.ctx, false); err != nil {
		return snapshot, err
	}
	return snapshot, nil
}

// GetRoot 返回 GUI 当前使用的 gdit 根目录；不访问文件系统。
func (a *App) GetRoot() string { return a.root }

// GetGUISettings 返回 GUI 窗口偏好。
func (a *App) GetGUISettings() (gdit.GUISettings, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return gdit.GUISettings{}, err
	}
	return manager.GUISettings(a.ctx)
}

// SetGUISettings 更新 GUI 窗口偏好并写入 config.toml。
func (a *App) SetGUISettings(settings gdit.GUISettings) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.SetGUISettings(a.ctx, settings)
}

// ListInstances 返回条目列表。
func (a *App) ListInstances() ([]gdit.InstanceInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return nil, err
	}
	return manager.Instances(a.ctx)
}

// GetDefault 返回当前条目。
func (a *App) GetDefault() (gdit.InstanceInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return gdit.InstanceInfo{}, err
	}
	return manager.Default(a.ctx)
}

// ListAssets 返回资源管理页快照。
func (a *App) ListAssets() (AssetSnapshot, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return AssetSnapshot{}, err
	}
	return listAssets(a.ctx, manager)
}

func listAssets(ctx context.Context, manager *gdit.Manager) (AssetSnapshot, error) {
	var result AssetSnapshot
	var err error
	if result.Engines, err = manager.List(ctx); err != nil {
		return result, err
	}
	if result.SDKs, err = manager.SDKs(ctx); err != nil {
		return result, err
	}
	if result.Sources, err = manager.Sources(ctx); err != nil {
		return result, err
	}
	if result.Templates, err = manager.Templates(ctx); err != nil {
		return result, err
	}
	if result.Orphans, err = manager.Orphans(ctx); err != nil {
		return result, err
	}
	return result, nil
}

// GetInstanceDetails 返回条目、有效环境和模板资源。
func (a *App) GetInstanceDetails(name string) (InstanceDetails, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return InstanceDetails{}, err
	}
	instances, err := manager.Instances(a.ctx)
	if err != nil {
		return InstanceDetails{}, err
	}
	var selected *gdit.InstanceInfo
	for index := range instances {
		if instances[index].Name == name {
			selected = &instances[index]
			break
		}
	}
	if selected == nil {
		return InstanceDetails{}, fmt.Errorf("%w: %s", gdit.ErrInstanceNotFound, name)
	}
	environment, err := manager.EffectiveEnv(a.ctx, name)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return InstanceDetails{}, err
		}
		environment = gdit.EnvView{}
	}
	environmentError := ""
	if err != nil {
		environmentError = err.Error()
	}
	configured, configuredErr := manager.ConfiguredEnv(a.ctx, name)
	if configuredErr != nil {
		return InstanceDetails{}, configuredErr
	}
	templates, err := manager.Templates(a.ctx)
	if err != nil {
		return InstanceDetails{}, err
	}
	return InstanceDetails{Instance: *selected, Env: environment, Configured: configured, EnvError: environmentError, Templates: templates}, nil
}

// GetEnvironment 返回配置层和最终有效环境；有效环境计算失败时保留已读取的配置层。
func (a *App) GetEnvironment(name string) (EnvironmentDetails, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return EnvironmentDetails{}, err
	}
	configured, err := manager.ConfiguredEnv(a.ctx, name)
	if err != nil {
		return EnvironmentDetails{}, err
	}
	details := EnvironmentDetails{Configured: configured}
	effective, effectiveErr := manager.EffectiveEnv(a.ctx, name)
	if effectiveErr != nil {
		details.EffectiveError = effectiveErr.Error()
		return details, nil
	}
	details.Effective = effective
	return details, nil
}

// ListAvailableVersions 在后台读取引擎候选，不登记为 GUI 操作任务。
func (a *App) ListAvailableVersions(source string) ([]gdit.EngineChannel, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return nil, err
	}
	return manager.Available(a.ctx, source)
}

// ListAvailableSDKs 在后台读取 SDK 候选，不登记为 GUI 操作任务。
func (a *App) ListAvailableSDKs() ([]gdit.SDKChannel, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return nil, err
	}
	return manager.AvailableSDKs(a.ctx)
}

// GetDoctor 启动可取消的诊断；network 明确控制是否探测来源。
func (a *App) GetDoctor(network bool) (OperationStart, error) {
	return a.startOperation("doctor", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.Doctor(ctx, network)
	})
}

// Suggest 启动一次只读项目分析。
func (a *App) Suggest(projectDir string) (OperationStart, error) {
	return a.startOperation("suggest", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.Suggest(ctx, projectDir)
	})
}

// InstallEntry 启动条目安装。
func (a *App) InstallEntry(request gdit.InstallEntryRequest) (OperationStart, error) {
	return a.startOperation("install-entry", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.InstallEntry(ctx, request)
	})
}

// InstallSuggestion 启动建议安装。
func (a *App) InstallSuggestion(request gdit.InstallSuggestionRequest) (OperationStart, error) {
	return a.startOperation("install-suggestion", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.InstallSuggestion(ctx, request)
	})
}

// RemoveInstance 启动条目卸载。
func (a *App) RemoveInstance(name string) (OperationStart, error) {
	return a.startOperation("remove-instance", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.RemoveInstance(ctx, name)
	})
}

// AutoRemove 启动孤儿资产锁内复查和清理。
func (a *App) AutoRemove() (OperationStart, error) {
	return a.startOperation("autoremove", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.AutoRemove(ctx)
	})
}

// AttachTemplate 启动匹配模板的安装与绑定。
func (a *App) AttachTemplate(name, source string) (OperationStart, error) {
	return a.startOperation("attach-template", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.AttachTemplate(ctx, name, source)
	})
}

// DetachTemplate 启动模板解绑。
func (a *App) DetachTemplate(name string) (OperationStart, error) {
	return a.startOperation("detach-template", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.DetachTemplate(ctx, name)
	})
}

// SetInstanceIcon 启动图片处理与条目图标更新。
func (a *App) SetInstanceIcon(name string, request gdit.SetInstanceIconRequest) (OperationStart, error) {
	return a.startOperation("set-instance-icon", func(ctx context.Context, manager *gdit.Manager) (any, error) {
		return manager.SetInstanceIcon(ctx, name, request)
	})
}

// SetDefault 设置 current，成功后前端重新 Bootstrap。
func (a *App) SetDefault(name string) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.SetDefault(a.ctx, name)
}

// SetEnvVar 设置全局或条目环境变量；scope 为空表示全局。
func (a *App) SetEnvVar(scope, key, value string) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.SetEnvVar(a.ctx, scope, key, value)
}

// UnsetEnvVar 删除全局或条目环境变量；scope 为空表示全局。
func (a *App) UnsetEnvVar(scope, key string) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.UnsetEnvVar(a.ctx, scope, key)
}

// ListSources 返回当前来源顺序和启禁用状态。
func (a *App) ListSources() ([]gdit.SourceInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return nil, err
	}
	return manager.Sources(a.ctx)
}

// SetSourceDisabled 启用或禁用来源。
func (a *App) SetSourceDisabled(name string, disabled bool) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.SetSourceDisabled(a.ctx, name, disabled)
}

// SetDefaultSource 把来源调整到首位。
func (a *App) SetDefaultSource(name string) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	return manager.SetDefaultSource(a.ctx, name)
}

// Launch 解析条目启动目标并启动独立 Godot 子进程，不修改 current。
func (a *App) Launch(name string) error {
	manager, err := a.newManager(nil)
	if err != nil {
		return err
	}
	target, err := manager.ResolveLaunch(a.ctx, name)
	if err != nil {
		return err
	}
	command := exec.Command(target.Executable, target.Args...)
	command.Env = target.Env
	return command.Start()
}

// ListSessions 返回由 GoDoIt GUI 登记且仍可核验的会话。
func (a *App) ListSessions() (SessionSnapshot, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return SessionSnapshot{}, err
	}
	sessions, err := manager.Sessions(a.ctx)
	if err != nil {
		return SessionSnapshot{}, err
	}
	return SessionSnapshot{Sessions: sessions}, nil
}

// LaunchSession 启动并登记一个 GUI 会话。
func (a *App) LaunchSession(name string) (gdit.SessionInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return gdit.SessionInfo{}, err
	}
	return manager.LaunchSession(a.ctx, name)
}

// RequestStopSession 请求会话正常退出。
func (a *App) RequestStopSession(id string) (gdit.SessionInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return gdit.SessionInfo{}, err
	}
	return manager.RequestStopSession(a.ctx, id)
}

// ForceStopSession 强制结束会话；前端必须先完成二次确认。
func (a *App) ForceStopSession(id string) (gdit.SessionInfo, error) {
	manager, err := a.newManager(nil)
	if err != nil {
		return gdit.SessionInfo{}, err
	}
	return manager.ForceStopSession(a.ctx, id)
}

// PickProjectDirectory 打开一次性项目目录选择器，不保存返回路径。
func (a *App) PickProjectDirectory() (string, error) {
	return wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择 Godot 项目目录"})
}

// PickIconFile 打开 PNG/JPEG 图标文件选择器，实际处理仍由 core 完成。
func (a *App) PickIconFile() (string, error) {
	return wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{Title: "选择条目图标", Filters: []wailsruntime.FileFilter{{DisplayName: "图片文件", Pattern: "*.png;*.jpg;*.jpeg"}}})
}

// Cancel 取消指定操作；终态会通过 gdit:progress 事件返回。
func (a *App) Cancel(operationID string) bool {
	a.mu.Lock()
	cancel := a.operations[operationID]
	a.mu.Unlock()
	if cancel == nil {
		return false
	}
	cancel()
	return true
}

func (a *App) activeOperationCount() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return len(a.operations)
}

func (a *App) cancelAll() {
	a.mu.Lock()
	cancels := make([]context.CancelFunc, 0, len(a.operations))
	for _, cancel := range a.operations {
		cancels = append(cancels, cancel)
	}
	a.mu.Unlock()
	for _, cancel := range cancels {
		cancel()
	}
}

func (a *App) startOperation(operation string, run func(context.Context, *gdit.Manager) (any, error)) (OperationStart, error) {
	id, err := newOperationID()
	if err != nil {
		return OperationStart{}, err
	}
	ctx, cancel := context.WithCancel(a.ctx)
	var eventMu sync.Mutex
	terminal := false
	manager, err := a.newManager(func(event gdit.ProgressEvent) {
		eventMu.Lock()
		defer eventMu.Unlock()
		if terminal {
			return
		}
		a.emitEvent(ProgressEnvelope{OperationID: id, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: "running", Operation: operation, Progress: &event})
	})
	if err != nil {
		cancel()
		return OperationStart{}, err
	}
	a.mu.Lock()
	a.operations[id] = cancel
	a.mu.Unlock()
	// 先发布排队事件，确保前端在耗时任务真正开始前就能显示并提供取消入口。
	a.emitEvent(ProgressEnvelope{OperationID: id, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: "running", Operation: operation, Progress: &gdit.ProgressEvent{Stage: "queued", Message: "已排队"}})
	go func() {
		result, runErr := run(ctx, manager)
		eventMu.Lock()
		if terminal {
			eventMu.Unlock()
			return
		}
		terminal = true
		a.mu.Lock()
		delete(a.operations, id)
		a.mu.Unlock()
		status := "complete"
		errorText := ""
		if runErr != nil || errors.Is(ctx.Err(), context.Canceled) {
			status = "failed"
			if runErr != nil {
				errorText = runErr.Error()
			}
			if errors.Is(runErr, context.Canceled) || errors.Is(ctx.Err(), context.Canceled) {
				status = "canceled"
				if errorText == "" {
					errorText = context.Canceled.Error()
				}
			}
		}
		a.emitEvent(ProgressEnvelope{OperationID: id, Timestamp: time.Now().UTC().Format(time.RFC3339Nano), Status: status, Operation: operation, Result: result, Error: errorText})
		eventMu.Unlock()
		cancel()
	}()
	return OperationStart{OperationID: id}, nil
}

func (a *App) emitEvent(event ProgressEnvelope) {
	if a.emit != nil {
		a.emit(progressEventName, event)
	}
}

func newOperationID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate operation id: %w", err)
	}
	return hex.EncodeToString(bytes[:]), nil
}

func readBuildInfo() BuildInfo {
	result := BuildInfo{Version: "dev"}
	if info, ok := debug.ReadBuildInfo(); ok {
		result.GoVersion = info.GoVersion
		if info.Main.Version != "" && info.Main.Version != "(devel)" {
			result.Version = info.Main.Version
		}
		for _, setting := range info.Settings {
			if setting.Key == "vcs.revision" {
				result.Commit = setting.Value
				if len(result.Commit) > 12 {
					result.Commit = result.Commit[:12]
				}
			}
		}
	}
	return result
}

// IconHandler 只提供 /instance-icons/<uuid>.png，不暴露 gdit 根目录其他文件。
func IconHandler(app *App) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodGet || !strings.HasPrefix(request.URL.Path, "/instance-icons/") {
			http.NotFound(response, request)
			return
		}
		name := strings.TrimPrefix(request.URL.Path, "/instance-icons/")
		if strings.Contains(name, "/") || !validIconFilename(name) {
			http.NotFound(response, request)
			return
		}
		path := filepath.Join(app.root, "icons", name)
		info, err := os.Lstat(path)
		if err != nil || !info.Mode().IsRegular() {
			http.NotFound(response, request)
			return
		}
		response.Header().Set("Cache-Control", "no-store")
		response.Header().Set("Content-Type", "image/png")
		http.ServeFile(response, request, path)
	})
}

func validIconFilename(name string) bool {
	if !strings.HasSuffix(name, ".png") || len(name) != 40 {
		return false
	}
	id := strings.TrimSuffix(name, ".png")
	for index, character := range id {
		if index == 8 || index == 13 || index == 18 || index == 23 {
			if character != '-' {
				return false
			}
			continue
		}
		if !strings.ContainsRune("0123456789abcdef", character) {
			return false
		}
	}
	return id[14] == '4' && strings.ContainsRune("89ab", rune(id[19]))
}
