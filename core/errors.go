package gdit

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidInput 表示命令输入不符合当前语法或取值约束。
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidConfig 表示配置文件或来源模板不可用。
	ErrInvalidConfig = errors.New("invalid config")
	// ErrUnsupportedPlatform 表示当前平台没有资产映射。
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	// ErrAlreadyInstalled 表示目标资产已经完整安装。
	ErrAlreadyInstalled = errors.New("version already installed")
	// ErrNoSources 表示没有可供安装的来源。
	ErrNoSources = errors.New("no sources configured")
	// ErrAllSourcesUnavailable 表示所有来源都因临时不可用而失败。
	ErrAllSourcesUnavailable = errors.New("all sources unavailable")
	// ErrInvalidArchive 表示已校验的引擎或 SDK 压缩包无法安全解压或缺少启动文件。
	ErrInvalidArchive = errors.New("invalid engine archive")
	// ErrLocalIO 表示 gdit 用户目录中的本地文件操作失败。
	ErrLocalIO = errors.New("local I/O failure")
	// ErrNotInstalled 表示目标资产尚未完整安装。
	ErrNotInstalled = errors.New("version not installed")
	// ErrEngineNotInstalled 表示启动解析时条目引用的引擎资产缺失。
	ErrEngineNotInstalled = errors.New("engine not installed")
	// ErrNoDefault 表示未设置当前条目，或 current 悬空、指向的条目不可用。
	ErrNoDefault = errors.New("no default version set")
	// ErrInstanceNotFound 表示指定条目不存在。
	ErrInstanceNotFound = errors.New("instance not found")
	// ErrCurrentInstanceInUse 表示目标条目是 current，不能删除。
	ErrCurrentInstanceInUse = errors.New("cannot remove current instance")
	// ErrNoCompatibleSDK 表示条目策略要求的 SDK 不可用。
	ErrNoCompatibleSDK = errors.New("compatible SDK not installed")
	// ErrAssetInUse 表示资产仍被一个或多个条目引用。
	ErrAssetInUse = errors.New("asset is referenced by an instance")
	// ErrInstanceRunning 表示条目仍有由 GUI 启动的运行会话。
	ErrInstanceRunning = errors.New("instance has running GUI sessions")
	// ErrSessionNotFound 表示运行会话不存在或已失效。
	ErrSessionNotFound = errors.New("session not found")
)

// SourceUnavailableError 标记来源的连接或资产暂时不可用，允许继续 fallback。
type SourceUnavailableError struct {
	Source string
	Err    error
}

func (e SourceUnavailableError) Error() string {
	if e.Source == "" {
		return fmt.Sprintf("source unavailable: %v", e.Err)
	}
	return fmt.Sprintf("source %s unavailable: %v", e.Source, e.Err)
}

func (e SourceUnavailableError) Unwrap() error     { return e.Err }
func (e SourceUnavailableError) Unavailable() bool { return true }

// IntegrityError 表示下载内容与来源声明的摘要不一致。
type IntegrityError struct {
	Source    string
	Filename  string
	Algorithm string
	Expected  string
	Actual    string
}

func (e IntegrityError) Error() string {
	algorithm := strings.ToLower(strings.TrimSpace(e.Algorithm))
	if algorithm == "" {
		algorithm = "sha256"
	}
	return fmt.Sprintf("%s mismatch for %s from %s", algorithm, e.Filename, e.Source)
}
