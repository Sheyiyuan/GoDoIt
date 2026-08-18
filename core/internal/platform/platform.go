// Package platform 集中处理当前主机目标、Godot 资产命名和启动文件布局。
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
)

// Target 是平台适配层内部使用的目标标识。
type Target struct {
	OS   string
	Arch string
}

// CurrentTarget 返回当前主机支持的目标。
func CurrentTarget() (Target, error) {
	target := Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if target.OS == "linux" && target.Arch == "amd64" {
		return target, nil
	}
	return Target{}, fmt.Errorf("%w: %s/%s", errors.New("unsupported platform"), target.OS, target.Arch)
}

// AssetName 返回目标平台的 Godot 引擎压缩包名称。
func AssetName(version, edition string, target Target) (string, error) {
	switch {
	case target.OS == "linux" && target.Arch == "amd64" && edition == "standard":
		return fmt.Sprintf("Godot_v%s-stable_linux.x86_64.zip", version), nil
	case target.OS == "linux" && target.Arch == "amd64" && edition == "dotnet":
		return fmt.Sprintf("Godot_v%s-stable_mono_linux_x86_64.zip", version), nil
	case target.OS == "darwin" && target.Arch == "arm64" && edition == "standard":
		return fmt.Sprintf("Godot_v%s-stable_macos.universal.zip", version), nil
	case target.OS == "darwin" && target.Arch == "arm64" && edition == "dotnet":
		return fmt.Sprintf("Godot_v%s-stable_mono_macos.universal.zip", version), nil
	default:
		return "", fmt.Errorf("unsupported asset target or edition %s/%s %s", target.OS, target.Arch, edition)
	}
}

// FindLauncher 找到解压目录中的引擎启动文件，并返回相对 payload 的路径。
// 真实资产结构（2026-08-18 对照 godot-ci mono.Dockerfile 核对）：标准版 zip 平铺放置
// Godot_v{ver}-stable_linux.x86_64；mono 版 zip 内是同名目录包裹，可执行文件为
// Godot_v{ver}-stable_mono_linux.x86_64（点号），另有 GodotSharp/ 目录。
func FindLauncher(payload, version, edition string, target Target) (string, error) {
	if target.OS != "linux" || target.Arch != "amd64" {
		return "", fmt.Errorf("launcher layout is not implemented for %s/%s", target.OS, target.Arch)
	}
	name := fmt.Sprintf("Godot_v%s-stable_linux.x86_64", version)
	if edition == "dotnet" {
		name = fmt.Sprintf("Godot_v%s-stable_mono_linux.x86_64", version)
	} else if edition != "standard" {
		return "", fmt.Errorf("unsupported edition %q", edition)
	}
	var launcher string
	err := filepath.WalkDir(payload, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != name {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return fmt.Errorf("launcher is not a regular file")
		}
		rel, relErr := filepath.Rel(payload, path)
		if relErr != nil {
			return relErr
		}
		launcher = rel
		return filepath.SkipDir
	})
	if err != nil {
		return "", err
	}
	if launcher == "" {
		return "", fmt.Errorf("launcher %q not found", name)
	}
	return launcher, nil
}

// PrepareLauncher 设置 Linux 引擎启动文件的执行权限。
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
