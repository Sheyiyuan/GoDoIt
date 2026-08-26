// Package session 负责 GUI 启动会话的最小 TOML 记录。
package session

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"github.com/BurntSushi/toml"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

// Status 是会话的持久状态。
type Status string

const (
	// Running 表示进程仍在运行。
	Running Status = "running"
	// Stopping 表示已请求正常退出。
	Stopping Status = "stopping"
	// Exited 表示进程已退出，等待记录清理。
	Exited Status = "exited"
	// Lost 表示进程身份不匹配或无法恢复。
	Lost Status = "lost"
)

// Record 是 runtime/sessions/<uuid>.toml 的结构。
type Record struct {
	SessionID       string    `toml:"session_id"`
	InstanceID      string    `toml:"instance_id"`
	InstanceName    string    `toml:"instance_name"`
	EngineID        string    `toml:"engine_id"`
	PID             int       `toml:"pid"`
	ProcessIdentity string    `toml:"process_identity"`
	StartedAt       time.Time `toml:"started_at"`
	Status          Status    `toml:"status"`
}

// EnsureDirs 创建并收紧 runtime 与 sessions 目录权限。
// 会话属于当前用户运行时状态，不应被同机其他用户读取或枚举。
func EnsureDirs(root string) error {
	runtimeDir := store.New(root).RuntimeDir()
	sessionsDir := store.New(root).SessionsDir()
	if err := os.MkdirAll(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.Chmod(runtimeDir, 0o700); err != nil {
		return err
	}
	if err := os.MkdirAll(sessionsDir, 0o700); err != nil {
		return err
	}
	return os.Chmod(sessionsDir, 0o700)
}

// Path 返回会话记录路径。
func Path(root, id string) string { return filepath.Join(store.New(root).SessionsDir(), id+".toml") }

// List 读取并按启动时间排序全部记录。
func List(root string) ([]Record, error) {
	entries, err := os.ReadDir(store.New(root).SessionsDir())
	if errors.Is(err, os.ErrNotExist) {
		return []Record{}, nil
	}
	if err != nil {
		return nil, err
	}
	result := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		var record Record
		if _, err := toml.DecodeFile(filepath.Join(store.New(root).SessionsDir(), entry.Name()), &record); err != nil {
			return nil, fmt.Errorf("decode session %s: %w", entry.Name(), err)
		}
		if record.SessionID == "" || record.InstanceID == "" || record.EngineID == "" || record.PID <= 0 || record.ProcessIdentity == "" {
			return nil, fmt.Errorf("invalid session record %s", entry.Name())
		}
		result = append(result, record)
	}
	sort.SliceStable(result, func(i, j int) bool { return result[i].StartedAt.Before(result[j].StartedAt) })
	return result, nil
}

// Write 原子写入会话记录。
func Write(root string, record Record) error {
	if record.SessionID == "" || record.InstanceID == "" || record.EngineID == "" || record.PID <= 0 || record.ProcessIdentity == "" {
		return errors.New("invalid session record")
	}
	if err := EnsureDirs(root); err != nil {
		return err
	}
	return store.WriteTOMLAtomic(Path(root, record.SessionID), record)
}

// Remove 删除指定会话记录，已不存在视为成功。
func Remove(root, id string) error {
	err := os.Remove(Path(root, id))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	return err
}
