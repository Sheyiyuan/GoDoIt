package release

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReadStableVersionAndTag(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "VERSION"), []byte("0.2.0\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	version, err := ReadStableVersion(root)
	if err != nil || version != "0.2.0" {
		t.Fatalf("读取版本 = %q, %v", version, err)
	}
	if err := ValidateTag("v0.2.0", version); err != nil {
		t.Fatal(err)
	}
	if err := ValidateTag("v0.2.1", version); err == nil {
		t.Fatal("不匹配 tag 未被拒绝")
	}
}

func TestReleaseVersionForms(t *testing.T) {
	for _, version := range []string{"0.2.0", "0.2.0-dev.20260826.0123456789ab"} {
		if err := ValidateReleaseVersion(version); err != nil {
			t.Fatalf("合法版本 %q 被拒绝：%v", version, err)
		}
	}
	for _, version := range []string{"v0.2.0", "0.2", "0.2.0-beta.1", "0.2.0-dev.today.abcdef0"} {
		if err := ValidateReleaseVersion(version); err == nil {
			t.Fatalf("非法版本 %q 未被拒绝", version)
		}
	}
}
