package gdit

import (
	"errors"
	"fmt"
	"strings"
)

var (
	// ErrInvalidInput 表示版本或 edition 输入不符合 MVP 语法。
	ErrInvalidInput = errors.New("invalid input")
	// ErrInvalidConfig 表示配置文件或来源模板不可用。
	ErrInvalidConfig = errors.New("invalid config")
	// ErrUnsupportedPlatform 表示当前平台没有资产映射。
	ErrUnsupportedPlatform = errors.New("unsupported platform")
	// ErrAlreadyInstalled 表示目标版本目录已经存在。
	ErrAlreadyInstalled = errors.New("version already installed")
	// ErrNoSources 表示没有可供安装的来源。
	ErrNoSources = errors.New("no sources configured")
	// ErrAllSourcesUnavailable 表示所有来源都因临时不可用而失败。
	ErrAllSourcesUnavailable = errors.New("all sources unavailable")
	// ErrInvalidArchive 表示已校验的压缩包无法安全解压或缺少目标启动文件。
	ErrInvalidArchive = errors.New("invalid engine archive")
	// ErrLocalIO 表示 gdit 用户目录中的本地文件操作失败。
	ErrLocalIO = errors.New("local I/O failure")
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
