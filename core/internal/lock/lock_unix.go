//go:build linux || darwin

// Package lock 提供 Linux 和 macOS 上的进程级修改锁。
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const pollInterval = 50 * time.Millisecond

// File 是一个持有 flock 的进程锁。
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
		err = unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return &File{file: file}, nil
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = file.Close()
			return nil, fmt.Errorf("acquire lock: %w", err)
		}
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
	}
}

// Close 释放锁并关闭锁文件。
func (f *File) Close() error {
	if f == nil || f.file == nil {
		return nil
	}
	unlockErr := unix.Flock(int(f.file.Fd()), unix.LOCK_UN)
	closeErr := f.file.Close()
	if unlockErr != nil {
		return unlockErr
	}
	return closeErr
}
