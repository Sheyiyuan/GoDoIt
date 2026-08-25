package platform

import (
	"path/filepath"
	"testing"
)

func TestAssetName(t *testing.T) {
	tests := []struct {
		name    string
		version string
		edition string
		target  Target
		want    string
	}{
		{
			name:    "linux standard",
			version: "4.5.2",
			edition: "standard",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_linux.x86_64.zip",
		},
		{
			name:    "linux dotnet",
			version: "4.5.2",
			edition: "dotnet",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_mono_linux_x86_64.zip",
		},
		{
			name:    "macos standard",
			version: "4.5.2",
			edition: "standard",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v4.5.2-stable_macos.universal.zip",
		},
		{
			name:    "macos dotnet",
			version: "4.5.2",
			edition: "dotnet",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v4.5.2-stable_mono_macos.universal.zip",
		},
		{
			name:    "windows standard",
			version: "4.5.2",
			edition: "standard",
			target:  Target{OS: "windows", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_win64.exe.zip",
		},
		{
			name:    "windows dotnet",
			version: "4.5.2",
			edition: "dotnet",
			target:  Target{OS: "windows", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_mono_win64.zip",
		},
		{
			name:    "linux prerelease",
			version: "4.8-dev3",
			edition: "standard",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.8-dev3_linux.x86_64.zip",
		},
		{
			name:    "linux prerelease dotnet",
			version: "4.7.2-rc1",
			edition: "dotnet",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.7.2-rc1_mono_linux_x86_64.zip",
		},
		{
			name:    "godot3 linux standard",
			version: "3.6.2",
			edition: "standard",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v3.6.2-stable_x11.64.zip",
		},
		{
			name:    "godot3 macos standard",
			version: "3.6.2",
			edition: "standard",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v3.6.2-stable_osx.universal.zip",
		},
		{
			name:    "godot3 linux mono",
			version: "3.6.2",
			edition: "dotnet",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v3.6.2-stable_mono_x11_64.zip",
		},
		{
			name:    "godot3 macos mono",
			version: "3.6.2",
			edition: "dotnet",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v3.6.2-stable_mono_osx.universal.zip",
		},
		{
			name:    "godot3 windows standard",
			version: "3.6.2",
			edition: "standard",
			target:  Target{OS: "windows", Arch: "amd64"},
			want:    "Godot_v3.6.2-stable_win64.exe.zip",
		},
		{
			name:    "godot3 windows mono",
			version: "3.6.2",
			edition: "dotnet",
			target:  Target{OS: "windows", Arch: "amd64"},
			want:    "Godot_v3.6.2-stable_mono_win64.zip",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AssetName(test.version, test.edition, test.target)
			if err != nil {
				t.Fatal(err)
			}
			if got != test.want {
				t.Fatalf("unexpected asset: got %q want %q", got, test.want)
			}
		})
	}
}

func TestAssetNameRejectsUnknownEdition(t *testing.T) {
	if _, err := AssetName("4.5.2", "unknown", Target{OS: "linux", Arch: "amd64"}); err == nil {
		t.Fatal("expected unsupported edition error")
	}
}

func TestSDKRID(t *testing.T) {
	for _, test := range []struct {
		target Target
		want   string
	}{
		{target: Target{OS: "linux", Arch: "amd64"}, want: "linux-x64"},
		{target: Target{OS: "darwin", Arch: "arm64"}, want: "osx-arm64"},
		{target: Target{OS: "windows", Arch: "amd64"}, want: "win-x64"},
	} {
		got, err := SDKRID(test.target)
		if err != nil || got != test.want {
			t.Fatalf("SDKRID(%+v) = %q, %v", test.target, got, err)
		}
	}
}

func TestSDKArchiveFormat(t *testing.T) {
	for _, test := range []struct {
		target Target
		want   string
	}{
		{target: Target{OS: "linux", Arch: "amd64"}, want: "tar.gz"},
		{target: Target{OS: "darwin", Arch: "arm64"}, want: "tar.gz"},
		{target: Target{OS: "windows", Arch: "amd64"}, want: "zip"},
	} {
		if got := SDKArchiveFormat(test.target); got != test.want {
			t.Fatalf("SDKArchiveFormat(%+v) = %q want %q", test.target, got, test.want)
		}
	}
}

func TestValidateControlValue(t *testing.T) {
	valid := []struct {
		osName, key, value string
	}{
		{"linux", "display_driver", "auto"},
		{"linux", "display_driver", "x11"},
		{"linux", "display_driver", "wayland"},
		{"linux", "input_method", "auto"},
		{"linux", "input_method", "fcitx"},
		{"linux", "input_method", "off"},
		{"darwin", "display_driver", "auto"},
		{"darwin", "input_method", "auto"},
		{"windows", "display_driver", "auto"},
		{"windows", "input_method", "auto"},
	}
	for _, test := range valid {
		if err := ValidateControlValue(test.osName, test.key, test.value); err != nil {
			t.Fatalf("ValidateControlValue(%s, %s, %s) = %v", test.osName, test.key, test.value, err)
		}
	}
	invalid := []struct {
		osName, key, value string
	}{
		{"linux", "display_driver", "xorg"},
		{"linux", "input_method", "ibus"},
		{"darwin", "display_driver", "x11"},
		{"darwin", "input_method", "fcitx"},
		{"windows", "display_driver", "wayland"},
		{"windows", "input_method", "off"},
	}
	for _, test := range invalid {
		if err := ValidateControlValue(test.osName, test.key, test.value); err == nil {
			t.Fatalf("ValidateControlValue(%s, %s, %s) should fail", test.osName, test.key, test.value)
		}
	}
}

func TestParseCurrentPointer(t *testing.T) {
	valid := "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"
	target := filepath.Join("instances", valid+".toml")
	got, err := ParseCurrentPointer(target)
	if err != nil || got != valid {
		t.Fatalf("ParseCurrentPointer(%q) = %q, %v", target, got, err)
	}
	for _, bad := range []string{
		filepath.Join("instances", "work.toml"),
		filepath.Join("instances", "nested", valid+".toml"),
		"/abs/path.toml",
		"../instances/" + valid + ".toml",
		valid + ".toml",
	} {
		if _, err := ParseCurrentPointer(bad); err == nil {
			t.Fatalf("ParseCurrentPointer(%q) should fail", bad)
		}
	}
}

func TestResolveRootPrecedence(t *testing.T) {
	// GDIT_ROOT 非空即用（须绝对路径）。
	customRoot, err := filepath.Abs(filepath.Join("custom", "gdit-root"))
	if err != nil {
		t.Fatal(err)
	}
	t.Setenv("GDIT_ROOT", customRoot)
	root, err := ResolveRoot()
	if err != nil || root != customRoot {
		t.Fatalf("GDIT_ROOT should win: %q err=%v", root, err)
	}
	// 相对路径报配置错误。
	t.Setenv("GDIT_ROOT", "relative/path")
	if _, err := ResolveRoot(); err == nil {
		t.Fatal("relative GDIT_ROOT must be rejected")
	}
	t.Setenv("GDIT_ROOT", "   ")
	if _, err := ResolveRoot(); err == nil {
		t.Fatal("non-empty whitespace GDIT_ROOT must be rejected")
	}
	// 空 GDIT_ROOT 回退平台默认（HOME 或 USERPROFILE）。
	t.Setenv("GDIT_ROOT", "")
	root, err = ResolveRoot()
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(root) != ".gdit" {
		t.Fatalf("default root should end with .gdit: %q", root)
	}
}
