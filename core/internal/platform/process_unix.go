//go:build linux || darwin

package platform

import (
	"context"
	"os/exec"
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
// ProcessAlive、RequestStop、ForceStop 和 processIdentity 由同目录的
// process_linux.go / process_darwin.go 提供，避免把 Linux 的 /proc 假设带入 macOS。
