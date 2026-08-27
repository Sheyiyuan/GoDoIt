// Package buildinfo 提供 CLI 与 GUI 共用的构建身份。
package buildinfo

import (
	"runtime"
	"runtime/debug"
	"strings"
)

var (
	version   = "dev"
	commit    string
	buildDate string
)

// Info 描述当前二进制的版本、提交和构建环境。
type Info struct {
	Version   string `json:"version"`
	Commit    string `json:"commit,omitempty"`
	BuildDate string `json:"build_date,omitempty"`
	GoVersion string `json:"go_version"`
}

// Read 返回 linker 注入的构建身份；未注入 commit 时使用 Go VCS 信息补充。
func Read() Info {
	result := Info{
		Version:   normalizedVersion(version),
		Commit:    strings.TrimSpace(commit),
		BuildDate: strings.TrimSpace(buildDate),
		GoVersion: runtime.Version(),
	}
	if result.Commit != "" {
		return result
	}
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return result
	}
	if info.GoVersion != "" {
		result.GoVersion = info.GoVersion
	}
	for _, setting := range info.Settings {
		if setting.Key == "vcs.revision" {
			result.Commit = strings.TrimSpace(setting.Value)
			break
		}
	}
	return result
}

func normalizedVersion(value string) string {
	value = strings.TrimSpace(value)
	if value == "" {
		return "dev"
	}
	return value
}
