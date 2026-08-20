package platform

import "testing"

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
	} {
		got, err := SDKRID(test.target)
		if err != nil || got != test.want {
			t.Fatalf("SDKRID(%+v) = %q, %v", test.target, got, err)
		}
	}
}
