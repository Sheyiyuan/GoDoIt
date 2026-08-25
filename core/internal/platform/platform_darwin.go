//go:build darwin

package platform

// DetectFcitx 在 macOS 上恒返回 false（不注入 Linux 专用变量）。
func DetectFcitx(_ Target, _ map[string]string) bool { return false }
