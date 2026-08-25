//go:build linux

package env

import "github.com/Sheyiyuan/GoDoIt/core/internal/platform"

// displayArgs 返回 Linux 的显示驱动引擎参数；auto 或空值不注入（Godot 原生默认与自动回退）。
func displayArgs(target platform.Target, driver string) []string {
	if driver == "" || driver == "auto" {
		return nil
	}
	return []string{"--display-driver", driver}
}

// applyInputMethod 在 Linux 上按 input_method 配置注入 fcitx 相关变量。
// auto：检测到 fcitx 才注入；fcitx：强制注入；off：不注入。已有值不覆盖。
func applyInputMethod(merged map[string]string, origins map[string]string, target platform.Target, mode string) {
	if mode == "off" {
		return
	}
	if mode == "auto" && !platform.DetectFcitx(target, merged) {
		return
	}
	for key, value := range map[string]string{
		"XMODIFIERS":    "@im=fcitx",
		"GTK_IM_MODULE": "fcitx",
		"QT_IM_MODULE":  "fcitx",
	} {
		if _, exists := merged[key]; exists {
			continue
		}
		merged[key] = value
		origins[key] = "derived"
	}
}
