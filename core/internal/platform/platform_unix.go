//go:build linux || darwin

package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/unix"
)

// ResolveRoot 解析生效的 gdit 根目录：GDIT_ROOT 非空即用（必须是绝对路径，否则配置错误），
// 否则返回平台默认路径 ~/.gdit。解析只发生在适配层，core 不读环境变量。
func ResolveRoot() (string, error) {
	if value := os.Getenv("GDIT_ROOT"); value != "" {
		if !filepath.IsAbs(value) {
			return "", errors.New("GDIT_ROOT must be an absolute path")
		}
		return filepath.Clean(value), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".gdit"), nil
}

// NormalizeEnvKey 返回 POSIX 环境变量键原值（键名区分大小写）。
func NormalizeEnvKey(key string) string { return key }

// EqualPath 按 POSIX 路径语义比较两个清理后的路径（区分大小写）。
func EqualPath(left, right string) bool { return filepath.Clean(left) == filepath.Clean(right) }

// FindGodotCommand 返回目录中可执行的 godot 命令；不存在时返回空串。
func FindGodotCommand(directory string) string {
	candidate := filepath.Join(directory, "godot")
	info, err := os.Stat(candidate)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return ""
	}
	return candidate
}

// PrepareLauncher 设置引擎启动文件的执行权限（POSIX：chmod +x）。
func PrepareLauncher(payload, relativePath string) error {
	if relativePath == "" || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return errors.New("invalid launcher path")
	}
	path := filepath.Join(payload, relativePath)
	info, err := os.Stat(path)
	if err != nil {
		return fmt.Errorf("stat launcher: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("launcher is not a regular file")
	}
	if err := os.Chmod(path, 0o755); err != nil {
		return fmt.Errorf("chmod launcher: %w", err)
	}
	return nil
}

// ErrLocked 表示文件已被其他进程持有排他锁（由 LockFile 返回，调用方轮询重试）。
// （已在共享 platform.go 定义；此处仅保留注释避免重复定义。）

// ValidateLauncher 校验引擎/SDK 启动文件：存在、是常规文件且可执行（POSIX 检查执行位）。
func ValidateLauncher(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("launcher is not a regular file")
	}
	if info.Mode().Perm()&0o111 == 0 {
		return errors.New("launcher is not executable")
	}
	return nil
}

// ShimPath 返回 godot shim 路径（Unix：bin/godot symlink）。
func ShimPath(root string) string { return filepath.Join(root, "bin", "godot") }

// EnsureShim 幂等创建或修复指向 gdit 可执行文件的 godot shim（Unix：symlink）。
// 已存在且指向目标时不做任何事；指向错误、缺失或是普通文件/空目录时原子重建
// （非空目录占用 shim 路径时报错，不递归删除，避免误删用户数据）。
func EnsureShim(root, target string) error {
	shim := ShimPath(root)
	if err := os.MkdirAll(filepath.Dir(shim), 0o755); err != nil {
		return fmt.Errorf("create bin directory: %w", err)
	}
	info, err := os.Lstat(shim)
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		existing, readErr := os.Readlink(shim)
		if readErr != nil {
			return fmt.Errorf("read shim link: %w", readErr)
		}
		if existing == target {
			return nil
		}
	case err == nil:
		if err := os.Remove(shim); err != nil {
			return fmt.Errorf("remove stale shim: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("inspect shim: %w", err)
	}
	if err := replaceSymlink(shim, target, SyncDir); err != nil {
		return fmt.Errorf("create shim: %w", err)
	}
	return nil
}

// IsShimInvocation 报告 gdit 是否以 godot 名称启动（argv[0] 基名判断，仅 Unix 有效）。
func IsShimInvocation(argv0 string) bool { return filepath.Base(argv0) == "godot" }

// ReadCurrentPointer 读取 current 指针的原始内容：Unix 为 symlink 目标
// （如 instances/<uuid>.toml）。链接不存在返回 os.ErrNotExist（由调用方转换为 ErrNoCurrent）。
func ReadCurrentPointer(root string) (string, error) {
	return os.Readlink(filepath.Join(root, "current"))
}

// WriteCurrentPointer 原子地把 current 替换为指向指定相对目标的 symlink。
// 内部已同步父目录，失败时旧链接保持不变。
func WriteCurrentPointer(root, target string) error {
	return writeCurrentPointerWithSync(root, target, SyncDir)
}

// writeCurrentPointerWithSync 是 WriteCurrentPointer 的注入变体，供测试注入目录同步失败。
func writeCurrentPointerWithSync(root, target string, sync func(string) error) error {
	if _, err := ParseCurrentPointer(target); err != nil {
		return err
	}
	if err := replaceSymlink(filepath.Join(root, "current"), target, sync); err != nil {
		return fmt.Errorf("set current pointer: %w", err)
	}
	return nil
}

// LockFile 对已打开的文件取得排他锁（flock，非阻塞；调用方负责轮询与取消）。
func LockFile(file *os.File) error {
	for {
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
			return ErrLocked
		}
		return err
	}
}

// ReleaseLock 释放排他锁并返回是否成功。
func ReleaseLock(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}

// RenameAtomic 原子地把 oldPath 替换为 newPath（POSIX rename(2)，目标存在时直接覆盖）。
func RenameAtomic(oldPath, newPath string) error { return os.Rename(oldPath, newPath) }

// SyncDir 把目录项变更同步到磁盘（POSIX：目录 fsync）。
func SyncDir(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}

// SDKLauncherName 返回托管 SDK 的启动文件名。
func SDKLauncherName() string { return "dotnet" }

// PathHint 返回把 bin 目录加入 PATH 的提示文本（Unix 为 export 形式）。
func PathHint(directory string) string {
	return fmt.Sprintf("export PATH=%q:$PATH", directory)
}

// RootAccessIssue 检查根目录对当前用户可读、可写且可进入；只读检查不修改目录内容。
func RootAccessIssue(root string) error {
	if err := unix.Access(root, unix.R_OK|unix.W_OK|unix.X_OK); err != nil {
		return err
	}
	return nil
}

// RootPermissionIssue 检查根目录权限是否过宽（POSIX：group/other 有任何权限位时返回问题描述）。
func RootPermissionIssue(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if info.Mode().Perm()&0o077 != 0 {
		return errors.New("root directory is accessible by group or others")
	}
	return nil
}

// CheckShim 检查 shim 平台形态是否就绪：Unix 为指向 gdit 的 symlink。
// 返回 (已创建, 指向正确)。目标为 gdit 可执行文件时认为正确。
func CheckShim(root, gditExecutable string) (created, correct bool) {
	shim := ShimPath(root)
	info, err := os.Lstat(shim)
	if err != nil {
		return false, false
	}
	if info.Mode()&os.ModeSymlink == 0 {
		return true, false
	}
	target, err := os.Readlink(shim)
	if err != nil {
		return true, false
	}
	if target != gditExecutable {
		return true, false
	}
	return true, ValidateLauncher(target) == nil
}

// replaceSymlink 原子地创建或替换 linkPath 处的 symlink：先在目标目录创建临时
// symlink，rename 覆盖后同步父目录。同步失败时把链接回滚到调用前状态，再返回错误。
// sync 参数注入目录同步实现，供测试注入失败。
func replaceSymlink(linkPath, target string, sync func(string) error) error {
	directory := filepath.Dir(linkPath)
	oldTarget, oldErr := os.Readlink(linkPath)
	hadOld := oldErr == nil
	if oldErr != nil && !errors.Is(oldErr, os.ErrNotExist) {
		return fmt.Errorf("read existing link: %w", oldErr)
	}
	temporaryPath, err := createTemporarySymlink(directory, target)
	if err != nil {
		return err
	}
	defer os.Remove(temporaryPath)
	if err := os.Rename(temporaryPath, linkPath); err != nil {
		return fmt.Errorf("replace symlink: %w", err)
	}
	if err := sync(directory); err != nil {
		rollbackErr := restoreSymlink(linkPath, oldTarget, hadOld, directory, sync)
		if rollbackErr != nil {
			return fmt.Errorf("sync link directory: %v; rollback failed: %w", err, rollbackErr)
		}
		return fmt.Errorf("sync link directory: %w", err)
	}
	return nil
}

func createTemporarySymlink(directory, target string) (string, error) {
	temporary, err := os.CreateTemp(directory, ".gdit-link-*.tmp")
	if err != nil {
		return "", fmt.Errorf("create temporary link: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		return "", fmt.Errorf("close temporary link: %w", err)
	}
	if err := os.Remove(temporaryPath); err != nil {
		return "", fmt.Errorf("prepare temporary link: %w", err)
	}
	if err := os.Symlink(target, temporaryPath); err != nil {
		return "", fmt.Errorf("create temporary symlink: %w", err)
	}
	return temporaryPath, nil
}

func restoreSymlink(linkPath, oldTarget string, hadOld bool, directory string, sync func(string) error) error {
	if !hadOld {
		if err := os.Remove(linkPath); err != nil && !errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("remove replacement link: %w", err)
		}
		if err := sync(directory); err != nil {
			return fmt.Errorf("sync rollback directory: %w", err)
		}
		return nil
	}
	temporaryPath, err := createTemporarySymlink(directory, oldTarget)
	if err != nil {
		return err
	}
	defer os.Remove(temporaryPath)
	if err := os.Rename(temporaryPath, linkPath); err != nil {
		return fmt.Errorf("restore previous symlink: %w", err)
	}
	if err := sync(directory); err != nil {
		return fmt.Errorf("sync rollback directory: %w", err)
	}
	return nil
}
