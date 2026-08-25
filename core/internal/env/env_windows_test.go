//go:build windows

package env

import (
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

func TestBuildNormalizesWindowsPathBeforeManagedSDKPrefix(t *testing.T) {
	result := Build(
		[]string{`Path=C:\Windows\System32`, "KEEP=parent", "display_driver=parent", "input_method=parent"},
		nil,
		nil,
		nil,
		platform.Target{OS: "windows", Arch: "amd64"},
		`D:\gdit\sdks\8.0.410`,
	)
	joined := strings.Join(result.Full, "\n")
	expected := `PATH=D:\gdit\sdks\8.0.410;C:\Windows\System32`
	if !strings.Contains(joined, expected) {
		t.Fatalf("managed SDK PATH prefix missing: %s", joined)
	}
	if strings.Contains(joined, "Path=") || strings.Count(joined, "PATH=") != 1 {
		t.Fatalf("Windows environment contains duplicate PATH keys: %s", joined)
	}
	if strings.Contains(joined, "DISPLAY_DRIVER=") || strings.Contains(joined, "INPUT_METHOD=") {
		t.Fatalf("control keys leaked into Windows child environment: %s", joined)
	}
}

func TestExplicitDotnetRootIsCaseInsensitiveOnWindows(t *testing.T) {
	if !ExplicitDotnetRoot(map[string]string{"DotNet_Root": `D:\dotnet`}, nil, nil) {
		t.Fatal("mixed-case DOTNET_ROOT must count as explicit takeover")
	}
}
