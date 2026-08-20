// Package env 合并启动子进程环境，并计算平台与 SDK 派生变量。
package env

import (
	"path/filepath"
	"sort"
	"strings"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

// Variable 描述一个由 gdit 配置或派生的环境变量。
type Variable struct {
	Key    string
	Value  string
	Origin string
}

// Result 是可直接交给子进程的完整环境和注入增量。
type Result struct {
	Full []string
	Vars []Variable
	Args []string
}

// Build 按父环境、全局、条目、派生变量的顺序构建启动环境。
// managedSDKDir 为空表示不注入托管 SDK；用户配置 DOTNET_ROOT 时调用方也应传空。
func Build(parent []string, global, local map[string]string, target platform.Target, managedSDKDir string) Result {
	merged := parse(parent)
	delete(merged, config.DisplayDriverKey)
	delete(merged, config.InputMethodKey)
	origins := make(map[string]string)
	controls := map[string]string{config.DisplayDriverKey: "auto", config.InputMethodKey: "auto"}
	applyConfigured(merged, origins, controls, global, "global")
	applyConfigured(merged, origins, controls, local, "instance")

	args := displayArgs(target, controls[config.DisplayDriverKey])
	if managedSDKDir != "" {
		merged["DOTNET_ROOT"] = managedSDKDir
		origins["DOTNET_ROOT"] = "derived"
		if merged["PATH"] == "" {
			merged["PATH"] = managedSDKDir
		} else {
			merged["PATH"] = managedSDKDir + string(filepath.ListSeparator) + merged["PATH"]
		}
		origins["PATH"] = "derived"
	}
	applyInputMethod(merged, origins, target, controls[config.InputMethodKey])

	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	full := make([]string, 0, len(keys))
	for _, key := range keys {
		full = append(full, key+"="+merged[key])
	}
	injectedKeys := make([]string, 0, len(origins))
	for key := range origins {
		injectedKeys = append(injectedKeys, key)
	}
	sort.Strings(injectedKeys)
	vars := make([]Variable, 0, len(injectedKeys))
	for _, key := range injectedKeys {
		vars = append(vars, Variable{Key: key, Value: merged[key], Origin: origins[key]})
	}
	return Result{Full: full, Vars: vars, Args: args}
}

// ExplicitDotnetRoot 报告全局或条目配置是否由用户显式接管 DOTNET_ROOT。
func ExplicitDotnetRoot(global, local map[string]string) bool {
	_, globalSet := global["DOTNET_ROOT"]
	_, localSet := local["DOTNET_ROOT"]
	return globalSet || localSet
}

func parse(entries []string) map[string]string {
	result := make(map[string]string, len(entries))
	for _, entry := range entries {
		key, value, ok := strings.Cut(entry, "=")
		if ok {
			result[key] = value
		}
	}
	return result
}

func applyConfigured(merged map[string]string, origins map[string]string, controls map[string]string, values map[string]string, origin string) {
	for key, value := range values {
		if key == config.DisplayDriverKey || key == config.InputMethodKey {
			controls[key] = value
			continue
		}
		merged[key] = value
		origins[key] = origin
	}
}

func displayArgs(target platform.Target, driver string) []string {
	if !platform.IsLinux(target) || driver == "" || driver == "auto" {
		return nil
	}
	return []string{"--display-driver", driver}
}

func applyInputMethod(merged map[string]string, origins map[string]string, target platform.Target, mode string) {
	if !platform.IsLinux(target) || mode == "off" {
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
