//go:build windows

package platform

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows"
)

// DetectFcitx 在 Windows 上恒返回 false（不注入 Linux 专用变量）。
func DetectFcitx(_ Target, _ map[string]string) bool { return false }

// ResolveRoot 解析生效的 gdit 根目录：GDIT_ROOT 非空即用（必须是绝对路径，否则配置错误），
// 否则返回平台默认路径 %USERPROFILE%\.gdit。解析只发生在适配层，core 不读环境变量。
func ResolveRoot() (string, error) {
	if value := os.Getenv("GDIT_ROOT"); value != "" {
		if !filepath.IsAbs(value) {
			return "", errors.New("GDIT_ROOT must be an absolute path")
		}
		return filepath.Clean(value), nil
	}
	profile := os.Getenv("USERPROFILE")
	if profile == "" {
		return "", errors.New("USERPROFILE is not set")
	}
	return filepath.Join(profile, ".gdit"), nil
}

// NormalizeEnvKey 规范化 Windows 环境变量键（键名大小写不敏感）。
func NormalizeEnvKey(key string) string { return strings.ToUpper(key) }

// EqualPath 按 Windows 路径语义比较两个清理后的路径（不区分大小写）。
func EqualPath(left, right string) bool {
	return strings.EqualFold(filepath.Clean(left), filepath.Clean(right))
}

// FindGodotCommand 返回目录中可由 Windows 命令行解析的 godot 命令；不存在时返回空串。
func FindGodotCommand(directory string) string {
	base := filepath.Join(directory, "godot")
	for _, extension := range []string{".com", ".exe", ".bat", ".cmd", ""} {
		candidate := base + extension
		info, err := os.Stat(candidate)
		if err == nil && info.Mode().IsRegular() {
			return candidate
		}
	}
	return ""
}

// PrepareLauncher 在 Windows 上为 no-op（无执行位概念），仅校验路径形态。
func PrepareLauncher(payload, relativePath string) error {
	if relativePath == "" || filepath.IsAbs(relativePath) || strings.HasPrefix(relativePath, ".."+string(filepath.Separator)) {
		return errors.New("invalid launcher path")
	}
	info, err := os.Stat(filepath.Join(payload, relativePath))
	if err != nil {
		return fmt.Errorf("stat launcher: %w", err)
	}
	if !info.Mode().IsRegular() {
		return errors.New("launcher is not a regular file")
	}
	return nil
}

// ValidateLauncher 校验引擎/SDK 启动文件：存在且是常规文件（Windows 无执行位概念）。
func ValidateLauncher(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.Mode().IsRegular() {
		return errors.New("launcher is not a regular file")
	}
	return nil
}

// shimCommand 返回 godot.cmd 包装内容：调用 setup 时实际运行的 gdit.exe，
// 透传全部参数与退出码。百分号必须翻倍，避免被 cmd.exe 当成变量引用。
func shimCommand(gditExecutable string) string {
	escaped := strings.ReplaceAll(filepath.Clean(gditExecutable), "%", "%%")
	return "@echo off\r\n@\"" + escaped + "\" __shim %*\r\nexit /b %errorlevel%\r\n"
}

// ShimPath 返回 godot shim 路径（Windows：bin/godot.cmd 包装）。
func ShimPath(root string) string { return filepath.Join(root, "bin", "godot.cmd") }

// EnsureShim 幂等创建/修复 godot.cmd 包装：内容与当前 gdit.exe 路径一致时不做事，
// 缺失或内容错误时原子重建（非普通文件占用 shim 路径时报错，不删除用户数据）。
func EnsureShim(root, gditExecutable string) error {
	if !filepath.IsAbs(gditExecutable) {
		return errors.New("gdit executable path must be absolute")
	}
	if err := ValidateLauncher(gditExecutable); err != nil {
		return fmt.Errorf("gdit executable is unavailable: %w", err)
	}
	command := shimCommand(gditExecutable)
	shim := ShimPath(root)
	if err := os.MkdirAll(filepath.Dir(shim), 0o755); err != nil {
		return fmt.Errorf("create bin directory: %w", err)
	}
	info, err := os.Lstat(shim)
	switch {
	case err == nil && info.Mode().IsRegular():
		if existing, readErr := os.ReadFile(shim); readErr == nil && string(existing) == command {
			return nil
		}
	case err == nil:
		return fmt.Errorf("shim path %s is not a regular file", shim)
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("inspect shim: %w", err)
	}
	if err := WriteFileAtomic(shim, command); err != nil {
		return fmt.Errorf("create shim: %w", err)
	}
	return nil
}

// IsShimInvocation 在 Windows 上恒返回 false：godot.cmd 内的 argv[0] 是 gdit.exe，
// shim 调用由 godot.cmd 调用 __shim 子命令识别。
func IsShimInvocation(_ string) bool { return false }

// ReadCurrentPointer 读取 current 重定向文件内容（无 BOM、单行、允许结尾换行）。
// 文件不存在返回 os.ErrNotExist（由调用方转换为 ErrNoCurrent）。
func ReadCurrentPointer(root string) (string, error) {
	path := filepath.Join(root, "current")
	info, err := os.Lstat(path)
	if err != nil {
		return "", err
	}
	if !info.Mode().IsRegular() {
		return "", errors.New("current pointer must be a regular file")
	}
	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	if bytes.HasPrefix(content, []byte{0xef, 0xbb, 0xbf}) {
		return "", errors.New("current pointer file must not contain a BOM")
	}
	value := string(content)
	if strings.HasSuffix(value, "\r\n") {
		value = strings.TrimSuffix(value, "\r\n")
	} else if strings.HasSuffix(value, "\n") {
		value = strings.TrimSuffix(value, "\n")
	}
	if value == "" {
		return "", errors.New("current pointer file is empty")
	}
	if strings.ContainsAny(value, "\r\n") {
		return "", errors.New("current pointer file must contain exactly one line")
	}
	return value, nil
}

// WriteCurrentPointer 原子地把 current 重定向文件替换为目标相对路径
// （临时文件 + MoveFileEx(MOVEFILE_REPLACE_EXISTING)，零特权要求）。
func WriteCurrentPointer(root, target string) error {
	if _, err := ParseCurrentPointer(target); err != nil {
		return err
	}
	if err := WriteFileAtomic(filepath.Join(root, "current"), target+"\n"); err != nil {
		return fmt.Errorf("set current pointer: %w", err)
	}
	return nil
}

// LockFile 对已打开的文件取得排他锁（LockFileEx，非阻塞；调用方负责轮询与取消）。
func LockFile(file *os.File) error {
	overlapped := new(windows.Overlapped)
	err := windows.LockFileEx(
		windows.Handle(file.Fd()),
		windows.LOCKFILE_EXCLUSIVE_LOCK|windows.LOCKFILE_FAIL_IMMEDIATELY,
		0, 1, 0, overlapped,
	)
	if err != nil {
		if err == windows.ERROR_LOCK_VIOLATION {
			return ErrLocked
		}
		return err
	}
	return nil
}

// ReleaseLock 释放排他锁。
func ReleaseLock(file *os.File) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(file.Fd()), 0, 1, 0, overlapped)
}

// RenameAtomic 原子地把 oldPath 替换为 newPath（MoveFileEx，目标存在时直接覆盖；
// os.Rename 在 Windows 上目标存在即失败）。
func RenameAtomic(oldPath, newPath string) error {
	from, err := windows.UTF16PtrFromString(oldPath)
	if err != nil {
		return err
	}
	to, err := windows.UTF16PtrFromString(newPath)
	if err != nil {
		return err
	}
	return windows.MoveFileEx(from, to, windows.MOVEFILE_REPLACE_EXISTING)
}

// SyncDir 在 Windows 上无目录 fsync 等价物：返回 nil，目录项持久性依赖 NTFS 日志
// （原子写降级语义文档化，见架构 §4.9）。
func SyncDir(_ string) error { return nil }

// WriteFileAtomic 以临时文件 + MoveFileEx 原子写文件（无目录 fsync）。
func WriteFileAtomic(path, content string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".gdit-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close temporary file: %w", err)
	}
	if err := RenameAtomic(temporaryPath, path); err != nil {
		return fmt.Errorf("replace file: %w", err)
	}
	return nil
}

// SDKLauncherName 返回托管 SDK 的启动文件名（Windows：dotnet.exe）。
func SDKLauncherName() string { return "dotnet.exe" }

// PathHint 返回把 bin 目录加入 PATH 的提示文本（Windows 为 set 形式）。
func PathHint(directory string) string {
	return fmt.Sprintf("set PATH=%s;%%PATH%%", directory)
}

// RootAccessIssue 在 Windows 上检查根目录可访问；不读取 ACL，也不修改目录内容。
func RootAccessIssue(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return err
	}
	if !info.IsDir() {
		return errors.New("root path is not a directory")
	}
	return nil
}

// RootPermissionIssue 在 Windows 上恒返回 nil（无 POSIX 权限位，降级为目录可访问检查）。
func RootPermissionIssue(root string) error {
	_, err := os.Stat(root)
	return err
}

// CheckShim 检查 shim 平台形态是否就绪：Windows 为内容匹配当前 gdit.exe 绝对路径的
// godot.cmd，且目标可执行文件仍存在。
func CheckShim(root, gditExecutable string) (created, correct bool) {
	shim := ShimPath(root)
	info, err := os.Lstat(shim)
	if err != nil {
		return false, false
	}
	if !info.Mode().IsRegular() {
		return true, false
	}
	content, err := os.ReadFile(shim)
	if err != nil {
		return true, false
	}
	return true, string(content) == shimCommand(gditExecutable) && ValidateLauncher(gditExecutable) == nil
}
