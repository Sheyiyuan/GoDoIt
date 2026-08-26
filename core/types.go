package gdit

import (
	"context"
	"net/http"
	"time"
)

// Options 配置 Manager 的用户目录、网络客户端和进度回调。
type Options struct {
	RootDir    string
	HTTPClient *http.Client
	Progress   func(ProgressEvent)
	Sources    []Source
	SDKProbe   func(context.Context) ([]SDKInfo, error)
}

// GUISettings 描述 GUI 在用户配置中保存的窗口偏好。
type GUISettings struct {
	TitlebarStyle string `json:"titlebar_style"`
}

// InstallRequest 描述一次引擎安装请求。
type InstallRequest struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
	Source  string `json:"source,omitempty"` // 指定来源，非空时只使用该来源
}

// InstallEntryRequest 描述一次条目层安装。
type InstallEntryRequest struct {
	Name        string `json:"name"`
	Version     string `json:"version"`
	Edition     string `json:"edition"`
	Source      string `json:"source,omitempty"`
	SDKStrategy string `json:"sdk_strategy,omitempty"`
	SDKVersion  string `json:"sdk_version,omitempty"`
	SetCurrent  *bool  `json:"set_current,omitempty"`
	Template    bool   `json:"template,omitempty"`
}

// SourceInfo 描述配置中的一个来源。
type SourceInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // builtin 或 custom
	Disabled bool   `json:"disabled"` // 是否被 source ban 禁用
}

// AvailableVersion 描述一个来源元数据中可安装的版本。
type AvailableVersion struct {
	Version  string   `json:"version"`
	Editions []string `json:"editions"`
	Sources  []string `json:"sources"`
}

// EngineChannel 描述一个可下载的引擎版本分组：稳定版按 major 系列（4.x/3.x），
// 预发布（dev/rc/beta/alpha）统一归入 unstable 组。
type EngineChannel struct {
	Name     string             `json:"name"` // "4.x" / "3.x" / "unstable"
	Versions []AvailableVersion `json:"versions"`
}

// SourceRequest 是 source provider 解析资产时收到的规范化请求。
type SourceRequest struct {
	Kind      string `json:"kind"` // engine 或 template
	Version   string `json:"version"`
	Edition   string `json:"edition,omitempty"`
	AssetName string `json:"asset_name"`
	Target    Target `json:"target,omitempty"`
}

// Target 描述当前主机的操作系统和 CPU 架构。
type Target struct {
	OS   string `json:"os"`
	Arch string `json:"arch"`
}

// Artifact 描述一个已经解析出下载地址和预期 SHA-256 的引擎资产。
type Artifact struct {
	Source            string `json:"source"`
	URL               string `json:"url"`
	Filename          string `json:"filename"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	Checksum          string `json:"checksum"`
	AuthorizationEnv  string `json:"-"`
}

// Source 是可参与安装 fallback 的下载来源。
type Source interface {
	Name() string
	Resolve(context.Context, SourceRequest) (Artifact, error)
}

// MetadataProber 是暴露元数据端点（doctor --network 可达性探测）的来源。
// URL 模板型自定义源不设置元数据端点，实现返回空串表示跳过探测。
type MetadataProber interface {
	MetadataEndpoint() string
}

// ProgressEvent 描述安装过程中可展示给 CLI 或 GUI 的进度。
type ProgressEvent struct {
	Stage           string `json:"stage"`
	Version         string `json:"version,omitempty"` // 版本 ID，如 4.5.1-dotnet；resolve/download/complete 事件必填
	Source          string `json:"source,omitempty"`
	Filename        string `json:"filename,omitempty"`
	BytesDownloaded int64  `json:"bytes_downloaded,omitempty"`
	TotalBytes      int64  `json:"total_bytes,omitempty"`
	Message         string `json:"message,omitempty"`
}

// InstalledVersion 描述一个通过完整性检查并已发布的引擎版本。
type InstalledVersion struct {
	ID                string `json:"id"`
	Version           string `json:"version"`
	Edition           string `json:"edition"`
	Target            Target `json:"target"`
	Source            string `json:"source"`
	ChecksumAlgorithm string `json:"checksum_algorithm"`
	Checksum          string `json:"checksum"`
	Launcher          string `json:"launcher"`
	InstalledAt       string `json:"installed_at"`
}

// InstallResult 描述一次安装的结果。
type InstallResult struct {
	Version              InstalledVersion `json:"version"`
	StateRebuildRequired bool             `json:"state_rebuild_required"`
}

// LaunchTarget 描述 ResolveLaunch 解析出的引擎启动目标。
type LaunchTarget struct {
	ID         string   `json:"id"`
	Version    string   `json:"version"`
	Edition    string   `json:"edition"`
	Executable string   `json:"executable"` // 引擎可执行文件的绝对路径
	Args       []string `json:"args,omitempty"`
	Env        []string `json:"env,omitempty"`
}

// SDKInfo 描述系统或托管 .NET SDK。
type SDKInfo struct {
	Version string `json:"version"`
	Kind    string `json:"kind"`
	Path    string `json:"path"`
}

// SDKChannel 描述一个可下载的 .NET SDK 大版本通道及其可用稳定版本。
type SDKChannel struct {
	MajorMinor  string   `json:"major_minor"`  // 如 "10.0"
	Phase       string   `json:"phase"`        // active / maintenance / eol
	ReleaseType string   `json:"release_type"` // lts / sts
	Versions    []string `json:"versions"`     // 该通道可用 patch，新到旧
}

// InstanceInfo 描述一个启动器条目。
type InstanceInfo struct {
	ID              string `json:"id"`   // 存储标识符（UUID v4），与条目文件名一致
	Name            string `json:"name"` // 显示名，用户寻址用，可中文
	Engine          string `json:"engine"`
	Edition         string `json:"edition"`
	SDKStrategy     string `json:"sdk_strategy"`
	SDK             string `json:"sdk"`
	Current         bool   `json:"current"`
	Template        string `json:"template"`
	TemplateMissing bool   `json:"template_missing"`
	Icon            string `json:"icon"`
	ResolvedIcon    string `json:"resolved_icon"`
	IconMissing     bool   `json:"icon_missing"`
	IconBackground  string `json:"icon_background"`
}

// SetInstanceIconRequest 描述一次条目图标策略变更。
type SetInstanceIconRequest struct {
	Icon       string `json:"icon"`
	SourcePath string `json:"source_path,omitempty"`
	Background string `json:"background,omitempty"`
}

// OrphanAsset 描述一个没有条目引用的资产。
type OrphanAsset struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
	Size int64  `json:"size"`
	Path string `json:"path"`
}

// AssetChange 描述一次业务操作新安装的资产。
type AssetChange struct {
	Kind string `json:"kind"`
	ID   string `json:"id"`
}

// InstallEntryResult 描述条目安装结果。
type InstallEntryResult struct {
	Instance             InstanceInfo  `json:"instance"`
	Installed            []AssetChange `json:"installed"`
	StateRebuildRequired bool          `json:"state_rebuild_required"`
}

// RemoveInstanceResult 描述已删除条目和删除后的孤儿快照。
type RemoveInstanceResult struct {
	Instance InstanceInfo  `json:"instance"`
	Orphans  []OrphanAsset `json:"orphans"`
}

// SDKInstallResult 描述托管 SDK 安装结果。
type SDKInstallResult struct {
	SDK                  SDKInfo `json:"sdk"`
	StateRebuildRequired bool    `json:"state_rebuild_required"`
}

// AutoRemoveResult 描述复查后实际删除的孤儿资产。
type AutoRemoveResult struct {
	Removed              []OrphanAsset `json:"removed"`
	StateRebuildRequired bool          `json:"state_rebuild_required"`
}

// EnvVar 描述注入变量及其来源。
type EnvVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Origin string `json:"origin"`
}

// EnvScope 是用户环境变量的配置作用域。
type EnvScope string

const (
	// EnvScopeGlobal 表示 config.toml 的通用环境变量。
	EnvScopeGlobal EnvScope = "global"
	// EnvScopePlatform 表示 config.toml 当前平台小节的环境变量。
	EnvScopePlatform EnvScope = "platform"
	// EnvScopeInstance 表示 instances 条目的环境变量。
	EnvScopeInstance EnvScope = "instance"
)

// ConfiguredEnvVar 描述一条未合并的用户配置环境变量。
type ConfiguredEnvVar struct {
	Key       string   `json:"key"`
	Value     string   `json:"value"`
	Scope     EnvScope `json:"scope"`
	Editable  bool     `json:"editable"`
	Sensitive bool     `json:"sensitive"`
}

// ConfiguredEnvView 描述全局、平台和条目配置层的环境变量。
type ConfiguredEnvView struct {
	Vars []ConfiguredEnvVar `json:"vars"`
}

// SessionStatus 描述 GUI 启动会话的状态。
type SessionStatus string

const (
	// SessionRunning 表示进程仍在运行。
	SessionRunning SessionStatus = "running"
	// SessionStopping 表示已请求正常退出。
	SessionStopping SessionStatus = "stopping"
	// SessionExited 表示进程已退出。
	SessionExited SessionStatus = "exited"
	// SessionLost 表示进程身份失配或无法恢复。
	SessionLost SessionStatus = "lost"
)

// SessionInfo 描述一个由 GUI 启动并登记的 Godot 会话。
type SessionInfo struct {
	SessionID    string        `json:"session_id"`
	InstanceID   string        `json:"instance_id"`
	InstanceName string        `json:"instance_name"`
	EngineID     string        `json:"engine_id"`
	PID          int           `json:"pid"`
	StartedAt    time.Time     `json:"started_at"`
	Status       SessionStatus `json:"status"`
}

// EnvView 描述 gdit 为目标条目增加或覆盖的环境与引擎参数。
type EnvView struct {
	Vars []EnvVar `json:"vars"`
	Args []string `json:"args"`
}

// SuggestLevel 是项目分析诊断的稳定级别。
type SuggestLevel string

const (
	// SuggestWarning 表示不阻止安装的项目提示。
	SuggestWarning SuggestLevel = "warning"
	// SuggestError 表示阻止建议安装的项目内容错误。
	SuggestError SuggestLevel = "error"
)

// SuggestDiagnostic 描述一条可归因于项目内容的诊断。
type SuggestDiagnostic struct {
	Level   SuggestLevel `json:"level"`
	Code    string       `json:"code"`
	Path    string       `json:"path,omitempty"`
	Message string       `json:"message"`
}

// SuggestEvidence 描述项目建议的一项原始证据。
type SuggestEvidence struct {
	Kind  string `json:"kind"`
	Path  string `json:"path"`
	Value string `json:"value"`
}

// ProjectSuggestion 是对一次显式项目目录只读分析的完整结果。
type ProjectSuggestion struct {
	ProjectDir   string              `json:"project_dir"`
	EngineSeries string              `json:"engine_series"`
	Edition      string              `json:"edition"`
	SDKStrategy  string              `json:"sdk_strategy"`
	SDKVersion   string              `json:"sdk_version"`
	SDKChannel   string              `json:"sdk_channel"`
	Evidence     []SuggestEvidence   `json:"evidence"`
	Diagnostics  []SuggestDiagnostic `json:"diagnostics"`
	Installable  bool                `json:"installable"`
}

// InstallSuggestionRequest 描述一次经用户明确授权的建议安装。
type InstallSuggestionRequest struct {
	ProjectDir      string `json:"project_dir"`
	Name            string `json:"name"`
	SDKStrategy     string `json:"sdk_strategy,omitempty"`
	SDKVersion      string `json:"sdk_version,omitempty"`
	SetCurrent      *bool  `json:"set_current,omitempty"`
	IncludeTemplate *bool  `json:"include_template,omitempty"`
}

// InstallSuggestionResult 描述重新分析并安装建议后的确定结果。
type InstallSuggestionResult struct {
	Suggestion    ProjectSuggestion  `json:"suggestion"`
	EngineVersion string             `json:"engine_version"`
	Entry         InstallEntryResult `json:"entry"`
	Template      *TemplateInfo      `json:"template,omitempty"`
}

// TemplateInfo 描述一个完整的官方导出模板资产。
type TemplateInfo struct {
	ID                string   `json:"id"`
	Version           string   `json:"version"`
	Edition           string   `json:"edition"`
	Source            string   `json:"source"`
	ChecksumAlgorithm string   `json:"checksum_algorithm"`
	Checksum          string   `json:"checksum"`
	ArchiveName       string   `json:"archive_name"`
	Path              string   `json:"path"`
	Size              int64    `json:"size"`
	InstalledAt       string   `json:"installed_at"`
	References        []string `json:"references"`
}

// InstallTemplateRequest 描述一次精确版本导出模板安装。
type InstallTemplateRequest struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
	Source  string `json:"source,omitempty"`
}

// TemplateBindingResult 描述条目模板绑定变更及其资产结果。
type TemplateBindingResult struct {
	Instance  InstanceInfo  `json:"instance"`
	Template  *TemplateInfo `json:"template,omitempty"`
	Installed bool          `json:"installed"`
	Orphans   []OrphanAsset `json:"orphans,omitempty"`
}
