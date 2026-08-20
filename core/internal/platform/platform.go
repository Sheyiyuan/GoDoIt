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

// ErrUnsupportedPlatform 表示当前主机或目标不在支持的平台范围内，可由 errors.Is 识别。
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// CurrentTarget 返回当前主机支持的目标。
func CurrentTarget() (Target, error) {
	target := Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if (target.OS == "linux" && target.Arch == "amd64") || (target.OS == "darwin" && target.Arch == "arm64") {
		return target, nil
	}
	return Target{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, target.OS, target.Arch)
}

// releaseSuffix 返回版本号对应的官方 release 后缀：稳定版拼 -stable，预发布版
// （版本号带 -devN/-rcN/-betaN/-alphaN 后缀）不加后缀，与官方资产命名一致。
func releaseSuffix(version string) string {
	if strings.Contains(version, "-") {
		return ""
	}
	return "-stable"
}

// godot3 是 Godot 3.x 系列 major 常量，其资产命名与 4.x+ 不同（x11/osx）。
const godot3 = "3"

// AssetName 返回目标平台的 Godot 引擎压缩包名称。
// 官方命名规则（2026-08 实测）：standard 用点号分隔平台（linux.x86_64、macos.universal、
// 3.x 的 x11.64/osx.universal）；mono 版 Linux 用下划线分隔（mono_linux_x86_64、
// 3.x 的 mono_x11_64），macOS 为 mono_macos.universal。
// 3.x 的 mono 版依赖系统 Mono 运行时（非 .NET SDK），GoDoIt 只负责下载安装，不管理运行时。
func AssetName(version, edition string, target Target) (string, error) {
	suffix := releaseSuffix(version)
	if majorPart(version) == godot3 {
		switch {
		case target.OS == "linux" && target.Arch == "amd64" && edition == "standard":
			return fmt.Sprintf("Godot_v%s%s_x11.64.zip", version, suffix), nil
		case target.OS == "linux" && target.Arch == "amd64" && edition == "dotnet":
			return fmt.Sprintf("Godot_v%s%s_mono_x11_64.zip", version, suffix), nil
		case target.OS == "darwin" && target.Arch == "arm64" && edition == "standard":
			return fmt.Sprintf("Godot_v%s%s_osx.universal.zip", version, suffix), nil
		case target.OS == "darwin" && target.Arch == "arm64" && edition == "dotnet":
			return fmt.Sprintf("Godot_v%s%s_mono_osx.universal.zip", version, suffix), nil
		default:
			return "", fmt.Errorf("unsupported asset target or edition %s/%s %s", target.OS, target.Arch, edition)
		}
	}
	switch {
	case target.OS == "linux" && target.Arch == "amd64" && edition == "standard":
		return fmt.Sprintf("Godot_v%s%s_linux.x86_64.zip", version, suffix), nil
	case target.OS == "linux" && target.Arch == "amd64" && edition == "dotnet":
		return fmt.Sprintf("Godot_v%s%s_mono_linux_x86_64.zip", version, suffix), nil
	case target.OS == "darwin" && target.Arch == "arm64" && edition == "standard":
		return fmt.Sprintf("Godot_v%s%s_macos.universal.zip", version, suffix), nil
	case target.OS == "darwin" && target.Arch == "arm64" && edition == "dotnet":
		return fmt.Sprintf("Godot_v%s%s_mono_macos.universal.zip", version, suffix), nil
	default:
		return "", fmt.Errorf("unsupported asset target or edition %s/%s %s", target.OS, target.Arch, edition)
	}
}

// majorPart 返回版本号的 major 段；无法解析时返回空串。
func majorPart(version string) string {
	part, _, _ := strings.Cut(version, ".")
	return part
}

// FindLauncher 找到解压目录中的引擎启动文件，并返回相对 payload 的路径。
// 真实资产结构（2026-08-18 对照 godot-ci mono.Dockerfile 核对）：标准版 zip 平铺放置
// Godot_v{ver}{suffix}_linux.x86_64；mono 版 zip 内是同名目录包裹，可执行文件为
// Godot_v{ver}{suffix}_mono_linux_x86_64（下划线），另有 GodotSharp/ 目录。
// suffix 对稳定版为 -stable，预发布版为空（见 releaseSuffix）。
func FindLauncher(payload, version, edition string, target Target) (string, error) {
	if target.OS == "darwin" && target.Arch == "arm64" {
		return findMacLauncher(payload)
	}
	if target.OS != "linux" || target.Arch != "amd64" {
		return "", fmt.Errorf("launcher layout is not implemented for %s/%s", target.OS, target.Arch)
	}
	suffix := releaseSuffix(version)
	var name string
	if majorPart(version) == godot3 {
		// Godot 3.x：启动文件名为 Godot_v3.6.2-stable_x11.64 或
		// Godot_v3.6.2-stable_mono_x11_64（mono 用下划线）。
		name = fmt.Sprintf("Godot_v%s%s_x11.64", version, suffix)
		if edition == "dotnet" {
			name = fmt.Sprintf("Godot_v%s%s_mono_x11_64", version, suffix)
		} else if edition != "standard" {
			return "", fmt.Errorf("unsupported edition %q", edition)
		}
	} else {
		name = fmt.Sprintf("Godot_v%s%s_linux.x86_64", version, suffix)
		if edition == "dotnet" {
			// 4.x mono zip 内可执行文件名是点号（mono_linux.x86_64），与 zip 资产名的
			// 下划线（mono_linux_x86_64.zip）不同，2026-08 对照真实安装核对。
			name = fmt.Sprintf("Godot_v%s%s_mono_linux.x86_64", version, suffix)
		} else if edition != "standard" {
			return "", fmt.Errorf("unsupported edition %q", edition)
		}
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

func findMacLauncher(payload string) (string, error) {
	var launcher string
	err := filepath.WalkDir(payload, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || entry.Name() != "Godot" || filepath.Base(filepath.Dir(path)) != "MacOS" {
			return nil
		}
		info, infoErr := entry.Info()
		if infoErr != nil {
			return infoErr
		}
		if !info.Mode().IsRegular() {
			return errors.New("launcher is not a regular file")
		}
		launcher, infoErr = filepath.Rel(payload, path)
		if infoErr != nil {
			return infoErr
		}
		return filepath.SkipDir
	})
	if err != nil {
		return "", err
	}
	if launcher == "" {
		return "", errors.New("Godot.app launcher not found")
	}
	return launcher, nil
}

// SDKRID 返回 .NET SDK 官方元数据使用的平台 RID。
func SDKRID(target Target) (string, error) {
	switch {
	case target.OS == "linux" && target.Arch == "amd64":
		return "linux-x64", nil
	case target.OS == "darwin" && target.Arch == "arm64":
		return "osx-arm64", nil
	default:
		return "", fmt.Errorf("unsupported SDK target %s/%s", target.OS, target.Arch)
	}
}

// IsLinux 报告目标是否为 Linux。
func IsLinux(target Target) bool { return target.OS == "linux" }

// DetectFcitx 按环境变量和 Linux 进程信息检测 fcitx/fcitx5。
func DetectFcitx(target Target, environment map[string]string) bool {
	if !IsLinux(target) {
		return false
	}
	if strings.Contains(strings.ToLower(environment["XMODIFIERS"]), "fcitx") {
		return true
	}
	entries, err := os.ReadDir("/proc")
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.Trim(entry.Name(), "0123456789") != "" {
			continue
		}
		name, readErr := os.ReadFile(filepath.Join("/proc", entry.Name(), "comm"))
		if readErr == nil && strings.HasPrefix(strings.ToLower(strings.TrimSpace(string(name))), "fcitx") {
			return true
		}
	}
	return false
}

// PrepareLauncher 设置当前平台引擎启动文件的执行权限。
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
