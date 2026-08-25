// Package platform 集中处理平台目标、Godot 资产命名、启动文件布局和平台能力。
// 平台无关的纯映射（资产名、launcher、RID、资产格式、current 规范判定）在本文件，
// 依赖当前主机的能力（根目录解析、锁、原子 rename、目录同步、shim、current 读写、
// 环境控制键校验）按 OS 拆分到 platform_unix.go / platform_linux.go /
// platform_darwin.go / platform_windows.go。业务代码不出现 runtime.GOOS 分支。
package platform

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
)

// instanceIDPattern 匹配 UUID v4 形态的条目存储标识（与 store 的判定一致）。
var instanceIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// Target 是平台适配层内部使用的目标标识。
type Target struct {
	OS   string
	Arch string
}

// ErrUnsupportedPlatform 表示当前主机或目标不在支持的平台范围内，可由 errors.Is 识别。
var ErrUnsupportedPlatform = errors.New("unsupported platform")

// ErrLocked 表示文件已被其他进程持有排他锁（由 LockFile 返回，调用方轮询重试）。
var ErrLocked = errors.New("file is locked")

// CurrentTarget 返回当前主机支持的目标。
// 支持矩阵：linux/amd64（主）、darwin/arm64 与 windows/amd64（验证级）。
func CurrentTarget() (Target, error) {
	target := Target{OS: runtime.GOOS, Arch: runtime.GOARCH}
	if (target.OS == "linux" && target.Arch == "amd64") ||
		(target.OS == "darwin" && target.Arch == "arm64") ||
		(target.OS == "windows" && target.Arch == "amd64") {
		return target, nil
	}
	return Target{}, fmt.Errorf("%w: %s/%s", ErrUnsupportedPlatform, target.OS, target.Arch)
}

// IsLinux 报告目标是否为 Linux。
func IsLinux(target Target) bool { return target.OS == "linux" }

// IsDarwin 报告目标是否为 macOS。
func IsDarwin(target Target) bool { return target.OS == "darwin" }

// IsWindows 报告目标是否为 Windows。
func IsWindows(target Target) bool { return target.OS == "windows" }

// CurrentOSName 返回当前主机的 OS 名（"linux"/"darwin"/"windows"）。
func CurrentOSName() string { return runtime.GOOS }

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

// AssetName 返回目标平台的 Godot 引擎压缩包名称（纯映射，固定输入可覆盖三平台）。
// 官方命名规则（2026-08 实测）：standard 用点号分隔平台（linux.x86_64、macos.universal、
// win64.exe、3.x 的 x11.64/osx.universal）；mono 版 Linux 用下划线分隔
// （mono_linux_x86_64、3.x 的 mono_x11_64），macOS 为 mono_macos.universal，
// Windows 4.x 与 3.x 均为 mono_win64（无 .exe 段）。
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
		case target.OS == "windows" && target.Arch == "amd64" && edition == "standard":
			return fmt.Sprintf("Godot_v%s%s_win64.exe.zip", version, suffix), nil
		case target.OS == "windows" && target.Arch == "amd64" && edition == "dotnet":
			return fmt.Sprintf("Godot_v%s%s_mono_win64.zip", version, suffix), nil
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
	case target.OS == "windows" && target.Arch == "amd64" && edition == "standard":
		return fmt.Sprintf("Godot_v%s%s_win64.exe.zip", version, suffix), nil
	case target.OS == "windows" && target.Arch == "amd64" && edition == "dotnet":
		return fmt.Sprintf("Godot_v%s%s_mono_win64.zip", version, suffix), nil
	default:
		return "", fmt.Errorf("unsupported asset target or edition %s/%s %s", target.OS, target.Arch, edition)
	}
}

// majorPart 返回版本号的 major 段；无法解析时返回空串。
func majorPart(version string) string {
	part, _, _ := strings.Cut(version, ".")
	return part
}

// FindLauncher 找到解压目录中的引擎启动文件，并返回相对 payload 的路径（纯映射 +
// 目录扫描，三平台布局差异按 target 分支）。
// 真实资产结构（2026-08-18 对照 godot-ci mono.Dockerfile 核对）：Linux 标准版 zip
// 平铺放置 Godot_v{ver}{suffix}_linux.x86_64；mono 版 zip 内是同名目录包裹，可执行文件
// 为 Godot_v{ver}{suffix}_mono_linux.x86_64（点号），另有 GodotSharp/ 目录。
// macOS 为 .app bundle 内 Contents/MacOS/Godot；Windows 为解压根目录
// Godot_v{ver}{suffix}_win64.exe（mono 为 _mono_win64.exe）。
// suffix 对稳定版为 -stable，预发布版为空（见 releaseSuffix）。
func FindLauncher(payload, version, edition string, target Target) (string, error) {
	if target.OS == "darwin" && target.Arch == "arm64" {
		return findMacLauncher(payload)
	}
	if target.OS == "windows" && target.Arch == "amd64" {
		return findWindowsLauncher(payload, version, edition)
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
	return findNamedLauncher(payload, name)
}

// findNamedLauncher 在 payload 下查找指定名称的普通文件，返回相对路径。
func findNamedLauncher(payload, name string) (string, error) {
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

// findWindowsLauncher 在 Windows 解压根目录查找 .exe 启动文件。
// 4.x 与 3.x 命名一致：Godot_v{ver}{suffix}_win64.exe（standard）或
// Godot_v{ver}{suffix}_mono_win64.exe（dotnet）。
func findWindowsLauncher(payload, version, edition string) (string, error) {
	if edition != "standard" && edition != "dotnet" {
		return "", fmt.Errorf("unsupported edition %q", edition)
	}
	suffix := releaseSuffix(version)
	mono := ""
	if edition == "dotnet" {
		mono = "_mono"
	}
	return findNamedLauncher(payload, fmt.Sprintf("Godot_v%s%s%s_win64.exe", version, suffix, mono))
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

// SDKRID 返回 .NET SDK 官方元数据使用的平台 RID（纯映射）。
func SDKRID(target Target) (string, error) {
	switch {
	case target.OS == "linux" && target.Arch == "amd64":
		return "linux-x64", nil
	case target.OS == "darwin" && target.Arch == "arm64":
		return "osx-arm64", nil
	case target.OS == "windows" && target.Arch == "amd64":
		return "win-x64", nil
	default:
		return "", fmt.Errorf("unsupported SDK target %s/%s", target.OS, target.Arch)
	}
}

// SDKArchiveFormat 返回 .NET SDK 官方资产的压缩格式（纯映射）：
// Linux/macOS 为 tar.gz，Windows 为 zip（dotnet-sdk-<v>-win-x64.zip，2026-08 实测）。
func SDKArchiveFormat(target Target) string {
	if target.OS == "windows" {
		return "zip"
	}
	return "tar.gz"
}

// ParseCurrentPointer 校验 current 指针内容的规范形态（三平台契约一致，纯判定）：
// 必须是 `instances/<uuid>.toml` 规范相对路径。返回去掉扩展名的 UUID 存储标识；
// 非法内容返回可识别错误（供读取路径与 doctor 复用，不复制规则）。
func ParseCurrentPointer(target string) (string, error) {
	filename := filepath.Base(filepath.Clean(target))
	if filepath.Ext(filename) != ".toml" {
		return "", fmt.Errorf("current pointer has invalid target %q", target)
	}
	name := strings.TrimSuffix(filename, ".toml")
	if !instanceIDPattern.MatchString(name) {
		return "", fmt.Errorf("current pointer has invalid target %q", target)
	}
	if target != filepath.Join("instances", filename) {
		return "", fmt.Errorf("current pointer must be %s", filepath.Join("instances", filename))
	}
	return name, nil
}

// ValidateControlValue 校验环境控制键取值（纯映射，按目标 OS 判定）：
// Linux 接受完整取值集合（display_driver: auto/x11/wayland；input_method:
// auto/fcitx/off）；macOS 与 Windows 只接受 auto（不注入 Linux 专用变量）。
func ValidateControlValue(osName, key, value string) error {
	if osName == "linux" {
		switch key {
		case "display_driver":
			if value != "auto" && value != "x11" && value != "wayland" {
				return fmt.Errorf("display_driver must be auto, x11 or wayland on linux")
			}
		case "input_method":
			if value != "auto" && value != "fcitx" && value != "off" {
				return fmt.Errorf("input_method must be auto, fcitx or off on linux")
			}
		}
		return nil
	}
	switch key {
	case "display_driver", "input_method":
		if value != "auto" {
			return fmt.Errorf("%s only accepts auto on this platform", key)
		}
	}
	return nil
}
