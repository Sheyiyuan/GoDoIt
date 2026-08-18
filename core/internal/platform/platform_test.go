package platform

import "testing"

func TestAssetName(t *testing.T) {
	tests := []struct {
		name    string
		edition string
		target  Target
		want    string
	}{
		{
			name:    "linux standard",
			edition: "standard",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_linux.x86_64.zip",
		},
		{
			name:    "linux dotnet",
			edition: "dotnet",
			target:  Target{OS: "linux", Arch: "amd64"},
			want:    "Godot_v4.5.2-stable_mono_linux_x86_64.zip",
		},
		{
			name:    "macos standard",
			edition: "standard",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v4.5.2-stable_macos.universal.zip",
		},
		{
			name:    "macos dotnet",
			edition: "dotnet",
			target:  Target{OS: "darwin", Arch: "arm64"},
			want:    "Godot_v4.5.2-stable_mono_macos.universal.zip",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			got, err := AssetName("4.5.2", test.edition, test.target)
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
