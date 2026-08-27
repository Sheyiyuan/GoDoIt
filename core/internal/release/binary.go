package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
)

type binaryBuildInfo struct {
	Version   string `json:"version"`
	Commit    string `json:"commit"`
	BuildDate string `json:"build_date"`
	GoVersion string `json:"go_version"`
}

// VerifyBinaryIdentity 执行原生 CLI 与 GUI 的只读版本入口并核对同一构建身份。
func VerifyBinaryIdentity(cli, gui, version, commit, buildDate string) error {
	if err := ValidateReleaseVersion(version); err != nil {
		return err
	}
	if err := ValidateCommit(commit); err != nil {
		return err
	}
	cliOutput, err := exec.Command(cli, "version").CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 CLI 版本检查：%w：%s", err, strings.TrimSpace(string(cliOutput)))
	}
	lines := strings.Split(strings.TrimSpace(string(cliOutput)), "\n")
	expectedLines := map[string]bool{
		"gdit " + version:    true,
		"commit " + commit:   true,
		"built " + buildDate: true,
	}
	for _, line := range lines {
		delete(expectedLines, strings.TrimSpace(line))
	}
	if len(expectedLines) != 0 {
		return fmt.Errorf("CLI 构建身份不完整：%q", strings.TrimSpace(string(cliOutput)))
	}
	guiOutput, err := exec.Command(gui, "--build-info").CombinedOutput()
	if err != nil {
		return fmt.Errorf("执行 GUI 版本检查：%w：%s", err, strings.TrimSpace(string(guiOutput)))
	}
	var info binaryBuildInfo
	decoder := json.NewDecoder(bytes.NewReader(guiOutput))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&info); err != nil {
		return fmt.Errorf("解析 GUI 构建身份：%w：%s", err, strings.TrimSpace(string(guiOutput)))
	}
	if info.Version != version || info.Commit != commit || info.BuildDate != buildDate || info.GoVersion == "" {
		return fmt.Errorf("GUI 构建身份不一致：%+v", info)
	}
	return nil
}
