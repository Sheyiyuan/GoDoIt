package release

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestPackageAndVerifyFinalRelease(t *testing.T) {
	root := t.TempDir()
	dist := filepath.Join(root, "dist")
	license := writeFixtureFile(t, root, "LICENSE", "AGPL fixture\n", 0o644)
	notices := writeFixtureFile(t, root, "THIRD_PARTY_NOTICES.txt", "notices fixture\n", 0o644)
	timestamp := time.Date(2026, time.August, 26, 8, 5, 31, 0, time.UTC)
	version := "0.2.0-dev.20260826.0123456789ab"

	linuxCLI := writeFixtureFile(t, root, "linux/gdit", "linux cli", 0o755)
	linuxGUI := writeFixtureFile(t, root, "linux/gdit-gui", "linux gui", 0o755)
	packageFixture(t, PackageOptions{Root: root, Platform: PlatformLinuxAMD64, Version: version, CLI: linuxCLI, GUI: linuxGUI, License: license, Notices: notices, Output: filepath.Join(dist, mustArchiveName(t, version, PlatformLinuxAMD64)), SourceDate: timestamp})

	windowsCLI := writeFixtureFile(t, root, "windows/gdit.exe", "windows cli", 0o755)
	windowsGUI := writeFixtureFile(t, root, "windows/gdit-gui.exe", "windows gui", 0o755)
	packageFixture(t, PackageOptions{Root: root, Platform: PlatformWindowsAMD64, Version: version, CLI: windowsCLI, GUI: windowsGUI, License: license, Notices: notices, Output: filepath.Join(dist, mustArchiveName(t, version, PlatformWindowsAMD64)), SourceDate: timestamp})

	macCLI := writeFixtureFile(t, root, "darwin/gdit", "darwin cli", 0o755)
	app := filepath.Join(root, "darwin", "GoDoIt.app")
	writeFixtureFile(t, app, "Contents/MacOS/gdit-gui", "darwin gui", 0o755)
	writeFixtureFile(t, app, "Contents/Info.plist", "plist", 0o644)
	writeFixtureFile(t, app, "Contents/Resources/iconfile.icns", "icon", 0o644)
	writeFixtureFile(t, app, "Contents/Resources/legal/LICENSE", "AGPL fixture\n", 0o644)
	writeFixtureFile(t, app, "Contents/Resources/legal/THIRD_PARTY_NOTICES.txt", "notices fixture\n", 0o644)
	packageFixture(t, PackageOptions{Root: root, Platform: PlatformDarwinARM64, Version: version, CLI: macCLI, GUI: app, License: license, Notices: notices, Output: filepath.Join(dist, mustArchiveName(t, version, PlatformDarwinARM64)), SourceDate: timestamp})
	for _, name := range []string{
		"GoDoIt_" + version + "_linux_amd64.deb",
		"GoDoIt_" + version + "_linux_amd64.rpm",
		"GoDoIt_" + version + "_windows_amd64_setup.exe",
		"GoDoIt_" + version + "_darwin_arm64.dmg",
	} {
		writeFixtureFile(t, dist, name, "installer", 0o644)
	}

	if err := WriteChecksums(dist, version); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFinalRelease(root, dist, version); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dist, "unexpected.txt"), []byte("unexpected"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := VerifyFinalRelease(root, dist, version); err == nil {
		t.Fatal("最终目录中的意外文件未被拒绝")
	}
}

func TestLinuxPackageIsDeterministic(t *testing.T) {
	root := t.TempDir()
	license := writeFixtureFile(t, root, "LICENSE", "license", 0o644)
	notices := writeFixtureFile(t, root, "THIRD_PARTY_NOTICES.txt", "notices", 0o644)
	cli := writeFixtureFile(t, root, "gdit", "cli", 0o755)
	gui := writeFixtureFile(t, root, "gdit-gui", "gui", 0o755)
	version := "0.2.0"
	timestamp := time.Date(2026, time.August, 26, 0, 0, 0, 0, time.UTC)
	first := filepath.Join(root, mustArchiveName(t, version, PlatformLinuxAMD64))
	secondDir := filepath.Join(root, "second")
	second := filepath.Join(secondDir, mustArchiveName(t, version, PlatformLinuxAMD64))
	options := PackageOptions{Root: root, Platform: PlatformLinuxAMD64, Version: version, CLI: cli, GUI: gui, License: license, Notices: notices, Output: first, SourceDate: timestamp}
	packageFixture(t, options)
	options.Output = second
	packageFixture(t, options)
	firstData, err := os.ReadFile(first)
	if err != nil {
		t.Fatal(err)
	}
	secondData, err := os.ReadFile(second)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(firstData, secondData) {
		t.Fatal("相同输入生成了不同归档")
	}
}

func packageFixture(t *testing.T, options PackageOptions) {
	t.Helper()
	if err := PackagePlatform(options); err != nil {
		t.Fatal(err)
	}
}

func mustArchiveName(t *testing.T, version, platform string) string {
	t.Helper()
	name, err := ArchiveName(version, platform)
	if err != nil {
		t.Fatal(err)
	}
	return name
}

func writeFixtureFile(t *testing.T, root, name, content string, mode os.FileMode) string {
	t.Helper()
	filename := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(filename), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filename, []byte(content), mode); err != nil {
		t.Fatal(err)
	}
	return filename
}
