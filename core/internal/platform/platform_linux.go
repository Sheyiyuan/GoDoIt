//go:build linux

package platform

import (
	"os"
	"path/filepath"
	"strings"
)

// DetectFcitx 按环境变量和 Linux 进程信息检测 fcitx/fcitx5。
// 检测规则：XMODIFIERS 已含 fcitx，或系统中存在 fcitx/fcitx5 进程。
func DetectFcitx(_ Target, environment map[string]string) bool {
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
