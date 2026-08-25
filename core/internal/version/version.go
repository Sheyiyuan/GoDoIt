// Package version 统一定义 Godot 引擎、.NET SDK 与引擎资产 ID 的版本语法。
package version

import (
	"fmt"
	"regexp"
	"strings"
)

var enginePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-(?:dev|rc|beta|alpha)[0-9]+)?$`)
var sdkPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-(?:preview|rc)\.[0-9]+(?:\.[0-9]+)*)?$`)

// ValidEngine 报告 value 是否为受支持的 Godot 引擎版本。
func ValidEngine(value string) bool {
	return enginePattern.MatchString(value)
}

// TemplateAssetName 返回指定 Godot 版本与 edition 的官方导出模板资产名。
func TemplateAssetName(version, edition string) (string, error) {
	if !ValidEngine(version) {
		return "", fmt.Errorf("invalid Godot version %q", version)
	}
	if edition != "standard" && edition != "dotnet" {
		return "", fmt.Errorf("invalid template edition %q", edition)
	}
	prefix := "Godot_v" + version
	if !strings.Contains(version, "-") {
		prefix += "-stable"
	}
	if edition == "dotnet" {
		return prefix + "_mono_export_templates.tpz", nil
	}
	return prefix + "_export_templates.tpz", nil
}

// ValidSDK 报告 value 是否为受支持的 .NET SDK 版本。
func ValidSDK(value string) bool {
	return sdkPattern.MatchString(value)
}

// ValidEngineID 报告 value 是否为引擎版本与 edition 组成的合法资产 ID。
func ValidEngineID(value string) bool {
	for _, edition := range []string{"standard", "dotnet"} {
		suffix := "-" + edition
		if strings.HasSuffix(value, suffix) {
			return ValidEngine(strings.TrimSuffix(value, suffix))
		}
	}
	return false
}
