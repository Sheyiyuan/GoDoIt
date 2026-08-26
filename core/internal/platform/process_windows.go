//go:build windows

package platform

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"path/filepath"
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ManagedProcess 是平台适配层返回的受 GoDoIt 管理的子进程。
type ManagedProcess struct {
	Command  *exec.Cmd
	PID      int
	Identity string
}

// StartManagedProcess 启动一个 GUI 会话进程；调用方负责登记并调用 Wait。
func StartManagedProcess(_ context.Context, executable string, args, environment []string) (*ManagedProcess, error) {
	command := exec.Command(executable, args...)
	command.Env = environment
	if err := command.Start(); err != nil {
		return nil, err
	}
	return &ManagedProcess{Command: command, PID: command.Process.Pid, Identity: processIdentity(command.Process.Pid, executable)}, nil
}

// Wait 等待受管理进程退出。
func (p *ManagedProcess) Wait() error { return p.Command.Wait() }

// ProcessAlive 核验进程身份并报告是否仍存活。
func ProcessAlive(pid int, identity, executable string) (bool, error) {
	if pid <= 0 {
		return false, nil
	}
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false, nil
	}
	defer windows.CloseHandle(handle)
	return identity == processIdentityFromHandle(handle, pid, executable), nil
}

// RequestStop 向目标进程的顶层窗口发送 WM_CLOSE，请求其执行正常退出流程。
func RequestStop(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}
	target := struct {
		pid   uint32
		found bool
	}{pid: uint32(pid)}
	callback := windows.NewCallback(func(hwnd windows.HWND, parameter uintptr) uintptr {
		state := (*struct {
			pid   uint32
			found bool
		})(unsafe.Pointer(parameter))
		var windowPID uint32
		if _, err := windows.GetWindowThreadProcessId(hwnd, &windowPID); err == nil && windowPID == state.pid {
			state.found = true
			_, _, _ = sendMessageW.Call(uintptr(hwnd), wmClose, 0, 0)
			return 0
		}
		return 1
	})
	if err := windows.EnumWindows(callback, unsafe.Pointer(&target)); err != nil {
		return err
	}
	if !target.found {
		return errors.New("target process has no top-level window")
	}
	return nil
}

// ForceStop 强制结束进程。
func ForceStop(pid int) error {
	if pid <= 0 {
		return errors.New("invalid process id")
	}
	handle, err := windows.OpenProcess(windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(handle)
	return windows.TerminateProcess(handle, 1)
}

const wmClose = 0x0010

var sendMessageW = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageW")

func processIdentity(pid int, executable string) string {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return fmt.Sprintf("%d:%s", pid, normalizeExecutable(executable))
	}
	defer windows.CloseHandle(handle)
	return processIdentityFromHandle(handle, pid, executable)
}

func processIdentityFromHandle(handle windows.Handle, pid int, executable string) string {
	creation := windows.Filetime{}
	var exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(handle, &creation, &exit, &kernel, &user); err != nil {
		return fmt.Sprintf("%d:%s", pid, normalizeExecutable(executable))
	}
	path := make([]uint16, windows.MAX_PATH)
	size := uint32(len(path))
	if err := windows.QueryFullProcessImageName(handle, 0, &path[0], &size); err == nil {
		executable = windows.UTF16ToString(path[:size])
	}
	return fmt.Sprintf("%d:%d:%s", pid, creation.Nanoseconds(), normalizeExecutable(executable))
}

func normalizeExecutable(path string) string {
	return strings.ToLower(filepath.Clean(path))
}
