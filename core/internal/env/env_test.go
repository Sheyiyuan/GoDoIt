package env

import (
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

func TestBuildMergesLayersAndDerivedSDK(t *testing.T) {
	result := Build(
		[]string{"PATH=/usr/bin", "LAYER=parent", "display_driver=parent", "input_method=parent"},
		map[string]string{"LAYER": "global", "display_driver": "wayland", "input_method": "fcitx"},
		map[string]string{"LAYER": "instance", "GTK_IM_MODULE": "custom"},
		platform.Target{OS: "linux", Arch: "amd64"},
		"/managed/sdk",
	)
	joined := strings.Join(result.Full, "\n")
	for _, expected := range []string{"LAYER=instance", "DOTNET_ROOT=/managed/sdk", "PATH=/managed/sdk:/usr/bin", "GTK_IM_MODULE=custom", "QT_IM_MODULE=fcitx"} {
		if !strings.Contains(joined, expected) {
			t.Fatalf("missing %q in %s", expected, joined)
		}
	}
	if strings.Join(result.Args, " ") != "--display-driver wayland" {
		t.Fatalf("unexpected args: %+v", result.Args)
	}
	if strings.Contains(joined, "display_driver=") || strings.Contains(joined, "input_method=") {
		t.Fatalf("control keys leaked into child environment: %s", joined)
	}
}

func TestBuildDoesNotInjectLinuxControlsOnMacOS(t *testing.T) {
	result := Build(nil, map[string]string{"display_driver": "x11", "input_method": "fcitx"}, nil, platform.Target{OS: "darwin", Arch: "arm64"}, "")
	if len(result.Args) != 0 || len(result.Vars) != 0 {
		t.Fatalf("macOS received Linux variables: %+v", result)
	}
}

func TestExplicitDotnetRootIgnoresParentOnlyValue(t *testing.T) {
	if ExplicitDotnetRoot(nil, nil) {
		t.Fatal("parent environment must not count as explicit takeover")
	}
	if !ExplicitDotnetRoot(map[string]string{"DOTNET_ROOT": "/custom"}, nil) {
		t.Fatal("configured DOTNET_ROOT must count as explicit takeover")
	}
}
