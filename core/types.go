package gdit

import (
	"context"
	"net/http"
)

// Options 配置 Manager 的用户目录、网络客户端和进度回调。
type Options struct {
	RootDir    string
	HTTPClient *http.Client
	Progress   func(ProgressEvent)
	Sources    []Source
	SDKProbe   func(context.Context) ([]SDKInfo, error)
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
	SDKStrategy string `json:"sdk_strategy,omitempty"`
	SDKVersion  string `json:"sdk_version,omitempty"`
	SetCurrent  *bool  `json:"set_current,omitempty"`
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
	Version string `json:"version"`
	Edition string `json:"edition"`
	Target  Target `json:"target"`
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
	ID          string `json:"id"`   // 存储标识符（UUID v4），与条目文件名一致
	Name        string `json:"name"` // 显示名，用户寻址用，可中文
	Engine      string `json:"engine"`
	Edition     string `json:"edition"`
	SDKStrategy string `json:"sdk_strategy"`
	SDK         string `json:"sdk"`
	Current     bool   `json:"current"`
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

// EnvView 描述 gdit 为目标条目增加或覆盖的环境与引擎参数。
type EnvView struct {
	Vars []EnvVar `json:"vars"`
	Args []string `json:"args"`
}
