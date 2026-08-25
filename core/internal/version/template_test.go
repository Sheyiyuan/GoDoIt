package version

import "testing"

func TestTemplateAssetName(t *testing.T) {
	tests := []struct{ version, edition, expected string }{
		{"4.5.2", "standard", "Godot_v4.5.2-stable_export_templates.tpz"},
		{"4.5", "standard", "Godot_v4.5-stable_export_templates.tpz"},
		{"4.5.2", "dotnet", "Godot_v4.5.2-stable_mono_export_templates.tpz"},
		{"4.6.0-rc1", "standard", "Godot_v4.6.0-rc1_export_templates.tpz"},
		{"4.6.0-rc1", "dotnet", "Godot_v4.6.0-rc1_mono_export_templates.tpz"},
	}
	for _, test := range tests {
		actual, err := TemplateAssetName(test.version, test.edition)
		if err != nil || actual != test.expected {
			t.Fatalf("TemplateAssetName(%q, %q) = %q, %v; want %q", test.version, test.edition, actual, err, test.expected)
		}
	}
}
