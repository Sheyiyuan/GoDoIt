// Package lock 提供跨平台的进程级修改锁（平台原语委托 core/internal/platform）。
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

const pollInterval = 50 * time.Millisecond

// File 是一个持有排他锁的进程锁。
type File struct {
	file *os.File
}

// Acquire 等待并取得排他锁，等待过程响应 context 取消。
func Acquire(ctx context.Context, filename string) (*File, error) {
	file, err := os.OpenFile(filename, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open lock: %w", err)
	}
	for {
		err = platform.LockFile(file)
		if err == nil {
			return &File{file: file}, nil
		}
		if errors.Is(err, platform.ErrLocked) {
			timer := time.NewTimer(pollInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				_ = file.Close()
				return nil, ctx.Err()
			case <-timer.C:
			}
			continue
		}
		_ = file.Close()
		return nil, fmt.Errorf("acquire lock: %w", err)
	}
}

// Close 释放锁并关闭锁文件。
func (f *File) Close() error {
	if f == nil || f.file == nil {
		return nil
	}
	unlockErr := platform.ReleaseLock(f.file)
	closeErr := f.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
