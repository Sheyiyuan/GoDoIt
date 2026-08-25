package env

import (
	"path/filepath"
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

func TestBuildMergesLayersAndDerivedSDK(t *testing.T) {
	result := Build(
		[]string{"PATH=/usr/bin", "LAYER=parent", "display_driver=parent", "input_method=parent"},
		map[string]string{"LAYER": "global", "display_driver": "auto", "input_method": "auto"},
		map[string]string{"PLATFORM_ONLY": "platform"},
		map[string]string{"LAYER": "instance", "GTK_IM_MODULE": "custom"},
		platform.Target{OS: "linux", Arch: "amd64"},
		"/managed/sdk",
	)
	joined := strings.Join(result.Full, "\n")
	expectedPath := "PATH=/managed/sdk" + string(filepath.ListSeparator) + "/usr/bin"
	for _, expected := range []string{"LAYER=instance", "PLATFORM_ONLY=platform", "DOTNET_ROOT=/managed/sdk", expectedPath, "GTK_IM_MODULE=custom"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
	if strings.Contains(joined, "display_driver=") || strings.Contains(joined, "input_method=") {
		t.Fatalf("control keys leaked into child environment: %s", joined)
	}
}

func TestBuildPlatformSectionOverridesGlobal(t *testing.T) {
	result := Build(
		nil,
		map[string]string{"SHARED": "global", "GLOBAL_ONLY": "g"},
		map[string]string{"SHARED": "platform"},
		map[string]string{"SHARED": "instance"},
		platform.Target{OS: "linux", Arch: "amd64"},
		"",
	)
	joined := strings.Join(result.Full, "\n")
	for _, expected := range []string{"SHARED=instance", "GLOBAL_ONLY=g"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
	origins := make(map[string]string)
	for _, variable := range result.Vars {
		origins[variable.Key] = variable.Origin
	}
	if origins["SHARED"] != "instance" || origins["GLOBAL_ONLY"] != "global" {
		t.Fatalf("unexpected origins: %+v", result.Vars)
	}
}

// 注：Linux 专用注入（display driver 参数、fcitx 变量）按编译标签拆分实现
// （env_linux.go / env_darwin.go / env_windows.go），非当前平台的注入行为不在
// 本机构建内测试；控制键取值的平台校验为纯映射，在 platform 包用固定输入覆盖三平台。

func TestExplicitDotnetRootIgnoresParentOnlyValue(t *testing.T) {
	if ExplicitDotnetRoot(nil, nil, nil) {
		t.Fatal("parent environment must not count as explicit takeover")
	}
	if !ExplicitDotnetRoot(map[string]string{"DOTNET_ROOT": "/custom"}, nil, nil) {
		t.Fatal("configured global DOTNET_ROOT must count as explicit takeover")
	}
	if !ExplicitDotnetRoot(nil, map[string]string{"DOTNET_ROOT": "/custom"}, nil) {
		t.Fatal("configured platform-section DOTNET_ROOT must count as explicit takeover")
	}
	if !ExplicitDotnetRoot(nil, nil, map[string]string{"DOTNET_ROOT": "/custom"}) {
		t.Fatal("configured instance DOTNET_ROOT must count as explicit takeover")
	}
}
