// Package version 统一定义 Godot 引擎、.NET SDK 与引擎资产 ID 的版本语法。
package version

import (
	"regexp"
	"strings"
)

var enginePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+(?:\.[0-9]+)?(?:-(?:dev|rc|beta|alpha)[0-9]+)?$`)
var sdkPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-(?:preview|rc)\.[0-9]+(?:\.[0-9]+)*)?$`)

// ValidEngine 报告 value 是否为受支持的 Godot 引擎版本。
func ValidEngine(value string) bool {
	return enginePattern.MatchString(value)
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
