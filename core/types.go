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
}

// InstallRequest 描述一次引擎安装请求。
type InstallRequest struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
	Source  string `json:"source,omitempty"` // 指定来源，非空时只使用该来源
}

// SourceInfo 描述配置中的一个来源。
type SourceInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // builtin 或 custom
	Disabled bool   `json:"disabled"` // 是否被 source ban 禁用
}

// AvailableVersion 描述一个来源元数据中可安装的稳定版本。
type AvailableVersion struct {
	Version  string   `json:"version"`
	Editions []string `json:"editions"`
	Sources  []string `json:"sources"`
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

// ProgressEvent 描述安装过程中可展示给 CLI 或 GUI 的进度。
type ProgressEvent struct {
	Stage           string `json:"stage"`
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
