// Package release 提供 GoDoIt 发布产物的生成与离线校验能力。
package release

import (
	"fmt"
	"os"
	"regexp"
	"strings"
)

var (
	stableVersionPattern  = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)
	releaseVersionPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+(?:-dev\.[0-9]{8}\.[0-9a-f]{7,40})?$`)
	commitPattern         = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
)

// ReadStableVersion 读取并验证仓库根级 VERSION。
func ReadStableVersion(root string) (string, error) {
	data, err := os.ReadFile(root + string(os.PathSeparator) + "VERSION")
	if err != nil {
		return "", fmt.Errorf("读取 VERSION：%w", err)
	}
	version := strings.TrimSpace(string(data))
	if !stableVersionPattern.MatchString(version) {
		return "", fmt.Errorf("VERSION 必须是三段稳定语义版本，实际为 %q", version)
	}
	return version, nil
}

// ValidateReleaseVersion 验证稳定版或滚动开发版的发布版本格式。
func ValidateReleaseVersion(version string) error {
	if !releaseVersionPattern.MatchString(version) {
		return fmt.Errorf("无效发布版本 %q", version)
	}
	return nil
}

// ValidateCommit 验证用于发布身份的 Git commit。
func ValidateCommit(commit string) error {
	if !commitPattern.MatchString(commit) {
		return fmt.Errorf("无效 commit %q", commit)
	}
	return nil
}

// BaseVersion 返回发布版本对应的三段稳定版本。
func BaseVersion(version string) (string, error) {
	if err := ValidateReleaseVersion(version); err != nil {
		return "", err
	}
	if index := strings.IndexByte(version, '-'); index >= 0 {
		return version[:index], nil
	}
	return version, nil
}

// ValidateTag 确认稳定 tag 与 VERSION 完全一致。
func ValidateTag(tag, stableVersion string) error {
	if !stableVersionPattern.MatchString(stableVersion) {
		return fmt.Errorf("无效稳定版本 %q", stableVersion)
	}
	expected := "v" + stableVersion
	if tag != expected {
		return fmt.Errorf("tag %q 与 VERSION 不一致，期望 %q", tag, expected)
	}
	return nil
}
