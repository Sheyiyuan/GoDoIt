//go:build windows

package env

import "github.com/Sheyiyuan/GoDoIt/core/internal/platform"

// displayArgs 在 Windows 上恒返回 nil（不注入 Linux 专用参数）。
func displayArgs(_ platform.Target, _ string) []string { return nil }

// applyInputMethod 在 Windows 上为 no-op（不注入 Linux 专用变量）。
func applyInputMethod(_ map[string]string, _ map[string]string, _ platform.Target, _ string) {}
