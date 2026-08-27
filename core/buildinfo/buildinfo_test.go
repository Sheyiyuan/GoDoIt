package buildinfo

import "testing"

func TestReadPrefersInjectedIdentity(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	t.Cleanup(func() { version, commit, buildDate = oldVersion, oldCommit, oldBuildDate })
	version = "0.2.0"
	commit = "0123456789abcdef"
	buildDate = "2026-08-26T01:02:03Z"

	info := Read()
	if info.Version != version || info.Commit != commit || info.BuildDate != buildDate || info.GoVersion == "" {
		t.Fatalf("构建身份不一致：%+v", info)
	}
}

func TestReadUsesDevelopmentVersionWhenUnset(t *testing.T) {
	oldVersion, oldCommit, oldBuildDate := version, commit, buildDate
	t.Cleanup(func() { version, commit, buildDate = oldVersion, oldCommit, oldBuildDate })
	version = " "
	commit = "fixture"
	buildDate = ""

	if info := Read(); info.Version != "dev" {
		t.Fatalf("未注入版本 = %q，期望 dev", info.Version)
	}
}
