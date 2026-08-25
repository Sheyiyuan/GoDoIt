//go:build linux || darwin

package main

import (
	"io"
	"os"
	"path/filepath"

	"golang.org/x/sys/unix"
)

// equalPath 按 POSIX 路径语义比较两个清理后的路径。
func equalPath(left, right string) bool { return filepath.Clean(left) == filepath.Clean(right) }

// launchEngine 在 Unix 上 execve 替换自身进程启动引擎：成功不返回，
// 失败写错误并返回 1。信号、stdio 与退出码天然透传。
// 平台化启动能力：Windows 实现见 launch_windows.go（spawn 等待并透传退出码）。
var launchEngine = func(executable string, engineArgs, environment []string, stderr io.Writer) int {
	if environment == nil {
		environment = os.Environ()
	}
	if err := unix.Exec(executable, append([]string{executable}, engineArgs...), environment); err != nil {
		writeErrorf(stderr, "launch %s: %v", executable, err)
		return 1
	}
	return 0
}

// isShimInvocation 报告 gdit 是否以 godot 名称启动（argv[0] 基名判断）。
func isShimInvocation(argv0 string) bool { return filepath.Base(argv0) == "godot" }

// shimRelativePath 返回根目录下 shim 的平台形态路径（Unix：bin/godot symlink）。
func shimRelativePath() string { return filepath.Join("bin", "godot") }

// pathHint 返回把 bin 目录加入 PATH 的提示文本（Unix：export 形式）。
func pathHint(directory string) string {
	return "export PATH=" + quoteShellPath(directory) + ":$PATH"
}

// quoteShellPath 为 PATH 提示的目录做 shell 引用（含空格等特殊字符时加双引号）。
func quoteShellPath(path string) string {
	if path == "" {
		return `""`
	}
	needsQuote := false
	for _, r := range path {
		if r == ' ' || r == '\t' || r == '\n' || r == '"' || r == '\\' || r == '$' || r == '`' {
			needsQuote = true
			break
		}
	}
	if !needsQuote {
		return path
	}
	return `"` + path + `"`
}
