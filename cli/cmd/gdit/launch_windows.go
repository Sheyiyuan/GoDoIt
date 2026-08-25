//go:build windows

package main

import (
	"errors"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
)

// equalPath 按 Windows 路径语义比较两个清理后的路径（不区分大小写）。
func equalPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

// launchEngine 在 Windows 上 spawn 引擎子进程并等待，透传退出码与信号
// （Windows 无 execve；子进程共享控制台，Ctrl+C 天然透传）。
var launchEngine = func(executable string, engineArgs, environment []string, stderr io.Writer) int {
	command := exec.Command(executable, engineArgs...)
	command.Stdin = os.Stdin
	command.Stdout = os.Stdout
	command.Stderr = stderr
	command.Env = environment
	if command.Env == nil {
		command.Env = os.Environ()
	}
	if err := command.Start(); err != nil {
		writeErrorf(stderr, "launch %s: %v", executable, err)
		return 1
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		select {
		case received := <-signals:
			_ = command.Process.Signal(received)
		case <-done:
		}
		signal.Stop(signals)
	}()
	err := command.Wait()
	close(done)
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.ExitCode()
	}
	writeErrorf(stderr, "launch %s: %v", executable, err)
	return 1
}

// isShimInvocation 在 Windows 上恒返回 false：godot.cmd 内的 argv[0] 是 gdit.exe，
// shim 调用由 godot.cmd 调用 __shim 子命令识别。
func isShimInvocation(_ string) bool { return false }

// shimRelativePath 返回根目录下 shim 的平台形态路径（Windows：bin/godot.cmd 包装）。
func shimRelativePath() string { return filepath.Join("bin", "godot.cmd") }

// pathHint 返回把 bin 目录加入 PATH 的提示文本（Windows：set 形式）。
func pathHint(directory string) string {
	return "set PATH=" + directory + ";%PATH%"
}
