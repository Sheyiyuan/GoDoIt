//go:build linux

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"syscall"
)

// ProcessAlive 核验 Linux 进程的 PID 与 /proc 可执行文件身份。
func ProcessAlive(pid int, identity, executable string) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	process, err := os.FindProcess(pid)
	if err != nil {
		return false, nil
	}
	if err := process.Signal(syscall.Signal(0)); err != nil {
		return false, nil
	}
	return identity == processIdentity(pid, executable), nil
}

// RequestStop 请求进程正常退出。
func RequestStop(pid int) error { return signalProcess(pid, syscall.SIGTERM) }

// ForceStop 强制结束进程。
func ForceStop(pid int) error { return signalProcess(pid, syscall.SIGKILL) }

func signalProcess(pid int, signal syscall.Signal) error {
	process, err := os.FindProcess(pid)
	if err != nil {
		return err
	}
	return process.Signal(signal)
}

func processIdentity(pid int, executable string) string {
	link, err := os.Readlink(filepath.Join("/proc", strconv.Itoa(pid), "exe"))
	if err == nil {
		return fmt.Sprintf("%d:%s", pid, link)
	}
	return fmt.Sprintf("%d:%s", pid, filepath.Clean(executable))
}
