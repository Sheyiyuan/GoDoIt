//go:build linux

package env

import (
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

func TestBuildInjectsLinuxDisplayAndInputMethod(t *testing.T) {
	result := Build(
		nil,
		map[string]string{"display_driver": "wayland", "input_method": "fcitx"},
		nil,
		nil,
		platform.Target{OS: "linux", Arch: "amd64"},
		"",
	)
	joined := strings.Join(result.Full, "\n")
	if strings.Join(result.Args, " ") != "--display-driver wayland" || !strings.Contains(joined, "QT_IM_MODULE=fcitx") {
		t.Fatalf("unexpected Linux environment: %+v", result)
	}
}
