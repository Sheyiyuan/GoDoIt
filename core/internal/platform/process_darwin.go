//go:build darwin

package platform

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

// ProcessAlive 核验 macOS 进程的 PID、启动时间和可执行文件 basename。
// macOS 没有 Linux /proc；kern.proc.pid 提供的启动时间可作为 PID 复用保护。
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
	info, err := unix.SysctlKinfoProc("kern.proc.pid", pid)
	if err != nil || info == nil || int(info.Proc.P_pid) != pid {
		return fmt.Sprintf("%d:%s", pid, filepath.Clean(executable))
	}
	name := strings.TrimRight(string(info.Proc.P_comm[:]), "\x00")
	return fmt.Sprintf("%d:%d:%d:%s", pid, info.Proc.P_starttime.Sec, info.Proc.P_starttime.Usec, name)
}
