package gdit

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/session"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

// Sessions 返回当前仍可核验的 GUI 运行会话；失效记录会被清理。
func (m *Manager) Sessions(ctx context.Context) ([]SessionInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	storeRoot := store.New(m.root)
	if err := session.EnsureDirs(m.root); err != nil {
		return nil, localIOError("create runtime directory", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.RuntimeLockPath())
	if err != nil {
		return nil, contextOrLocalIOError("acquire runtime lock", err)
	}
	defer guard.Close()
	records, err := session.List(m.root)
	if err != nil {
		return nil, localIOError("read sessions", err)
	}
	result := make([]SessionInfo, 0, len(records))
	for _, record := range records {
		executable, resolveErr := sessionExecutable(m.root, record.EngineID)
		alive := false
		var checkErr error
		if resolveErr == nil {
			alive, checkErr = platform.ProcessAlive(record.PID, record.ProcessIdentity, executable)
		}
		if checkErr != nil || !alive {
			if removeErr := session.Remove(m.root, record.SessionID); removeErr != nil {
				return nil, localIOError("remove stale session", removeErr)
			}
			continue
		}
		result = append(result, publicSession(record))
	}
	return result, nil
}

// LaunchSession 启动并登记一个 GUI Godot 会话；成功登记后的进程不绑定 ctx 生命周期。
func (m *Manager) LaunchSession(ctx context.Context, name string) (SessionInfo, error) {
	if err := ctx.Err(); err != nil {
		return SessionInfo{}, err
	}
	storeRoot := store.New(m.root)
	if err := storeRoot.Init(); err != nil {
		return SessionInfo{}, localIOError("initialize store", err)
	}
	if err := session.EnsureDirs(m.root); err != nil {
		return SessionInfo{}, localIOError("create runtime directory", err)
	}
	globalGuard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return SessionInfo{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer globalGuard.Close()
	runtimeGuard, err := lock.Acquire(ctx, storeRoot.RuntimeLockPath())
	if err != nil {
		return SessionInfo{}, contextOrLocalIOError("acquire runtime lock", err)
	}
	defer runtimeGuard.Close()
	item, err := lookupLaunchInstance(m.root, name)
	if err != nil {
		return SessionInfo{}, err
	}
	target, err := m.ResolveLaunch(ctx, name)
	if err != nil {
		return SessionInfo{}, err
	}
	process, err := platform.StartManagedProcess(ctx, target.Executable, target.Args, target.Env)
	if err != nil {
		return SessionInfo{}, fmt.Errorf("start Godot session: %w", err)
	}
	id, err := newSessionID()
	if err != nil {
		_ = platform.ForceStop(process.PID)
		_ = process.Wait()
		return SessionInfo{}, err
	}
	record := session.Record{SessionID: id, InstanceID: item.ID, InstanceName: item.Name, EngineID: target.ID, PID: process.PID, ProcessIdentity: process.Identity, StartedAt: m.now(), Status: session.Running}
	if err := session.Write(m.root, record); err != nil {
		_ = platform.ForceStop(process.PID)
		_ = process.Wait()
		return SessionInfo{}, fmt.Errorf("register Godot session: %w", err)
	}
	go m.waitSession(process, record)
	return publicSession(record), nil
}

// RequestStopSession 请求指定会话正常退出。
func (m *Manager) RequestStopSession(ctx context.Context, id string) (SessionInfo, error) {
	return m.stopSession(ctx, id, false)
}

// ForceStopSession 强制结束指定会话；调用方必须已完成二次确认。
func (m *Manager) ForceStopSession(ctx context.Context, id string) (SessionInfo, error) {
	return m.stopSession(ctx, id, true)
}

func (m *Manager) stopSession(ctx context.Context, id string, force bool) (SessionInfo, error) {
	if err := ctx.Err(); err != nil {
		return SessionInfo{}, err
	}
	storeRoot := store.New(m.root)
	if err := session.EnsureDirs(m.root); err != nil {
		return SessionInfo{}, localIOError("create runtime directory", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.RuntimeLockPath())
	if err != nil {
		return SessionInfo{}, contextOrLocalIOError("acquire runtime lock", err)
	}
	defer guard.Close()
	records, err := session.List(m.root)
	if err != nil {
		return SessionInfo{}, localIOError("read sessions", err)
	}
	for _, record := range records {
		if record.SessionID != id {
			continue
		}
		executable, resolveErr := sessionExecutable(m.root, record.EngineID)
		alive := false
		var checkErr error
		if resolveErr == nil {
			alive, checkErr = platform.ProcessAlive(record.PID, record.ProcessIdentity, executable)
		}
		if checkErr != nil || !alive {
			if removeErr := session.Remove(m.root, id); removeErr != nil {
				return SessionInfo{}, localIOError("remove stale session", removeErr)
			}
			return publicSession(record), nil
		}
		if force {
			if err := platform.ForceStop(record.PID); err != nil {
				return SessionInfo{}, fmt.Errorf("force stop session: %w", err)
			}
		} else if err := platform.RequestStop(record.PID); err != nil {
			return SessionInfo{}, fmt.Errorf("request session stop: %w", err)
		}
		record.Status = session.Stopping
		if err := session.Write(m.root, record); err != nil {
			return SessionInfo{}, localIOError("update session status", err)
		}
		return publicSession(record), nil
	}
	return SessionInfo{}, fmt.Errorf("%w: %s", ErrSessionNotFound, id)
}

func (m *Manager) waitSession(process *platform.ManagedProcess, record session.Record) {
	_ = process.Wait()
	guard, err := lock.Acquire(context.Background(), store.New(m.root).RuntimeLockPath())
	if err != nil {
		return
	}
	defer guard.Close()
	_ = session.Remove(m.root, record.SessionID)
}

func (m *Manager) hasRunningSession(ctx context.Context, name string) (bool, error) {
	storeRoot := store.New(m.root)
	if err := session.EnsureDirs(m.root); err != nil {
		return false, localIOError("create runtime directory", err)
	}
	runtimeGuard, err := lock.Acquire(ctx, storeRoot.RuntimeLockPath())
	if err != nil {
		return false, contextOrLocalIOError("acquire runtime lock", err)
	}
	defer runtimeGuard.Close()
	items, err := instance.Scan(m.root)
	if err != nil {
		return false, fmt.Errorf("%w: cannot determine running instance: %v", ErrInvalidConfig, err)
	}
	var id string
	for _, item := range items {
		if item.Name == name {
			id = item.ID
			break
		}
	}
	if id == "" {
		return false, nil
	}
	records, err := session.List(m.root)
	if err != nil {
		return false, localIOError("read sessions", err)
	}
	for _, record := range records {
		if record.InstanceID != id {
			continue
		}
		executable, resolveErr := sessionExecutable(m.root, record.EngineID)
		alive := false
		var checkErr error
		if resolveErr == nil {
			alive, checkErr = platform.ProcessAlive(record.PID, record.ProcessIdentity, executable)
		}
		if checkErr != nil {
			return false, localIOError("check session process", checkErr)
		}
		if alive {
			return true, nil
		}
		if err := session.Remove(m.root, record.SessionID); err != nil {
			return false, localIOError("remove stale session", err)
		}
	}
	return false, nil
}

func sessionExecutable(root, engineID string) (string, error) {
	records, err := store.New(root).ScanValid()
	if err != nil {
		return "", err
	}
	for _, record := range records {
		if record.Manifest.ID == engineID {
			return filepath.Join(record.Dir, "payload", record.Manifest.Launcher), nil
		}
	}
	return "", os.ErrNotExist
}

func lookupLaunchInstance(root, name string) (instance.File, error) {
	if name == "" {
		id, err := store.New(root).ReadCurrent()
		if err != nil {
			return instance.File{}, errors.Join(ErrNoDefault, err)
		}
		return instance.Read(root, id)
	}
	if err := instance.ValidateName(name); err != nil {
		return instance.File{}, errors.Join(ErrInvalidInput, err)
	}
	item, err := instance.Lookup(root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return instance.File{}, errors.Join(ErrInstanceNotFound, err)
		}
		return instance.File{}, errors.Join(ErrInvalidConfig, err)
	}
	return item, nil
}

func publicSession(record session.Record) SessionInfo {
	return SessionInfo{SessionID: record.SessionID, InstanceID: record.InstanceID, InstanceName: record.InstanceName, EngineID: record.EngineID, PID: record.PID, StartedAt: record.StartedAt, Status: SessionStatus(record.Status)}
}

func newSessionID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate session id: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40
	bytes[8] = (bytes[8] & 0x3f) | 0x80
	return hex.EncodeToString(bytes[:]), nil
}
