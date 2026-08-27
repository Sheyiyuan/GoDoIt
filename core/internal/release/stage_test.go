package release

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestStageGUIProjectRejectsOutputOutsideBuild(t *testing.T) {
	root := t.TempDir()
	if err := StageGUIProject(root, filepath.Join(root, "stage"), "0.2.0"); err == nil {
		t.Fatal("build/ 外的暂存目录未被拒绝")
	}
}

func TestGeneratedWailsAndWindowsVersions(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "wails.json")
	destination := filepath.Join(root, "build", "wails.json")
	if err := os.WriteFile(source, []byte(`{"name":"GoDoIt","info":{"productVersion":"old"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	version := "0.2.0-dev.20260826.0123456789ab"
	if err := writeWailsConfig(source, destination, version); err != nil {
		t.Fatal(err)
	}
	var config struct {
		Info struct {
			ProductVersion string `json:"productVersion"`
		} `json:"info"`
	}
	data, err := os.ReadFile(destination)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &config); err != nil || config.Info.ProductVersion != version {
		t.Fatalf("Wails 版本未注入：%s, %v", data, err)
	}
	guiRoot := filepath.Join(root, "gui")
	if err := os.MkdirAll(filepath.Join(guiRoot, "build", "windows"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := writeWindowsResources(guiRoot, "0.2.0", version); err != nil {
		t.Fatal(err)
	}
	resource, err := os.ReadFile(filepath.Join(guiRoot, "build", "windows", "info.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !json.Valid(resource) {
		t.Fatalf("Windows resource 不是有效 JSON：%s", resource)
	}
	var versionInfo struct {
		Info map[string]map[string]string `json:"info"`
	}
	if err := json.Unmarshal(resource, &versionInfo); err != nil {
		t.Fatal(err)
	}
	if got := versionInfo.Info[windowsVersionLanguageID]["ProductVersion"]; got != version {
		t.Fatalf("Windows resource 未写入 en-US ProductVersion：%q", got)
	}
}
