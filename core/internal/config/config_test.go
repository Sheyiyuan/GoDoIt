package config

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadReturnsDefaultsWhenConfigIsMissing(t *testing.T) {
	cfg, err := Load(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SchemaVersion != 1 || len(cfg.SourceOrder) != 2 || cfg.SourceOrder[0] != "godothub" || cfg.SourceOrder[1] != "github" {
		t.Fatalf("unexpected defaults: %+v", cfg)
	}
	if cfg.GUI.TitlebarStyle != DefaultTitlebarStyle {
		t.Fatalf("unexpected default titlebar style: %q", cfg.GUI.TitlebarStyle)
	}
}

func TestSetTitlebarStyleKeepsUnknownGUIFields(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["godothub", "github"]

[gui]
titlebar_style = "auto"
future_option = "keep-me"
`)
	if err := SetTitlebarStyle(root, TitlebarStyleWindows); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.GUI.TitlebarStyle != TitlebarStyleWindows {
		t.Fatalf("unexpected titlebar style: %q", cfg.GUI.TitlebarStyle)
	}
	content, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "future_option = \"keep-me\"") {
		t.Fatalf("unknown GUI field was lost after write-back: %s", content)
	}
}

func TestSetTitlebarStyleRejectsInvalidValue(t *testing.T) {
	if err := SetTitlebarStyle(t.TempDir(), "linux"); err == nil {
		t.Fatal("expected invalid titlebar style to be rejected")
	}
}

func TestLoadRequiresSchemaVersion(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected missing schema version error")
	}
}

func TestLoadRejectsWhitespaceInSourceNames(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = " fixture "
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected source name whitespace error")
	}
}

func writeConfigFile(t *testing.T, root, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestListSourcesFollowsOrderAndKind(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github", "fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`)
	entries, err := ListSources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || entries[0].Name != "github" || entries[0].Kind != "builtin" || entries[1].Name != "fixture" || entries[1].Kind != "custom" {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

func TestSetSourceOrderFirstMovesToFrontAndKeepsUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github", "godothub"]

[environment]
display_driver = "auto"
input_method = "auto"

[dotnet]
auto_install = "ask"
`)
	if err := SetSourceOrderFirst(root, "godothub"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SourceOrder) != 2 || cfg.SourceOrder[0] != "godothub" || cfg.SourceOrder[1] != "github" {
		t.Fatalf("unexpected source order: %+v", cfg.SourceOrder)
	}
	content, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, fragment := range []string{"[environment]", "display_driver", "[dotnet]", "auto_install"} {
		if !strings.Contains(string(content), fragment) {
			t.Fatalf("unknown field %q was lost after write-back: %s", fragment, content)
		}
	}
}

func TestSetSourceOrderFirstCreatesMissingConfig(t *testing.T) {
	root := t.TempDir()
	if err := SetSourceOrderFirst(root, "github"); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.SourceOrder) != 2 || cfg.SourceOrder[0] != "github" || cfg.SourceOrder[1] != "godothub" {
		t.Fatalf("unexpected source order: %+v", cfg.SourceOrder)
	}
}

func TestSetSourceOrderFirstRejectsUnknownSource(t *testing.T) {
	root := t.TempDir()
	if err := SetSourceOrderFirst(root, "missing"); !errors.Is(err, ErrSourceNotConfigured) {
		t.Fatalf("expected ErrSourceNotConfigured, got %v", err)
	}
}

func TestSetSourceOrderFirstRejectsEmptyName(t *testing.T) {
	root := t.TempDir()
	if err := SetSourceOrderFirst(root, "  "); err == nil {
		t.Fatal("expected empty source name to be rejected")
	}
}

func TestSetSourceDisabledBanAndUnban(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github", "godothub"]
`)
	if err := SetSourceDisabled(root, "github", true); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if !IsSourceDisabled(cfg, "github") || len(cfg.DisabledSources) != 1 {
		t.Fatalf("unexpected disabled list: %+v", cfg.DisabledSources)
	}
	if err := SetSourceDisabled(root, "github", true); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.DisabledSources) != 1 {
		t.Fatalf("duplicate ban must be idempotent: %+v", cfg.DisabledSources)
	}
	if err := SetSourceDisabled(root, "github", false); err != nil {
		t.Fatal(err)
	}
	cfg, err = Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if IsSourceDisabled(cfg, "github") || len(cfg.DisabledSources) != 0 {
		t.Fatalf("source should be re-enabled: %+v", cfg.DisabledSources)
	}
}

func TestSetSourceDisabledRejectsUnknownSource(t *testing.T) {
	root := t.TempDir()
	if err := SetSourceDisabled(root, "missing", true); !errors.Is(err, ErrSourceNotConfigured) {
		t.Fatalf("expected ErrSourceNotConfigured, got %v", err)
	}
}

func TestLoadRejectsDisabledSourceNotInOrder(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github"]
disabled_sources = ["godothub"]
`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected disabled source not in source_order to be rejected")
	}
}

func TestListSourcesMarksDisabled(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github", "godothub"]
disabled_sources = ["github"]
`)
	entries, err := ListSources(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 || !entries[0].Disabled || entries[1].Disabled {
		t.Fatalf("unexpected entries: %+v", entries)
	}
}

// custom_sources 条目内的未知字段在写回时必须保留。
func TestWriteBackKeepsUnknownFieldsInsideCustomSources(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
timeout_seconds = 120
`)
	if err := SetSourceOrderFirst(root, "fixture"); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "timeout_seconds = 120") {
		t.Fatalf("unknown field inside custom source was lost: %s", content)
	}
}

func TestLoadRejectsCustomSourceConflictingWithBuiltin(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github"]

[[custom_sources]]
name = "github"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected custom source named like a built-in to be rejected")
	}
}

func TestLoadRejectsInvalidAuthorizationEnvironmentName(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["fixture"]

[[custom_sources]]
name = "fixture"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "INVALID=NAME"
`)
	if _, err := Load(root); err == nil {
		t.Fatal("invalid authorization_env name was accepted")
	}
}

func TestEnvironmentWriteBackPreservesUnknownFields(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github"]
future_option = "keep"

[environment]
display_driver = "auto"
input_method = "auto"
EXISTING = "value"
`)
	if err := SetEnvironmentVariable(root, "NEW_VALUE", "added", false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{`future_option = "keep"`, `EXISTING = "value"`, `NEW_VALUE = "added"`} {
		if !strings.Contains(string(content), expected) {
			t.Fatalf("missing %q after write-back: %s", expected, content)
		}
	}
}

func TestLoadRejectsInvalidEnvironmentControls(t *testing.T) {
	root := t.TempDir()
	writeConfigFile(t, root, `schema_version = 1
source_order = ["github"]
[environment]
display_driver = "invalid"
`)
	if _, err := Load(root); err == nil {
		t.Fatal("expected invalid display driver to be rejected")
	}
}

func TestSetEnvironmentVariableCanUnsetControlKeys(t *testing.T) {
	root := t.TempDir()
	for _, key := range []string{DisplayDriverKey, InputMethodKey} {
		if err := SetEnvironmentVariable(root, key, "auto", false); err != nil {
			t.Fatalf("set %s: %v", key, err)
		}
		if err := SetEnvironmentVariable(root, key, "", true); err != nil {
			t.Fatalf("unset %s: %v", key, err)
		}
		if err := SetEnvironmentVariable(root, key, "", true); err != nil {
			t.Fatalf("repeat unset %s: %v", key, err)
		}
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment.Global[DisplayDriverKey] != "auto" || cfg.Environment.Global[InputMethodKey] != "auto" {
		t.Fatalf("unset controls did not fall back to defaults: %+v", cfg.Environment.Global)
	}
}

func TestSetEnvironmentVariableRejectsInvalidControlValues(t *testing.T) {
	root := t.TempDir()
	for _, test := range []struct {
		key   string
		value string
	}{
		{key: DisplayDriverKey, value: "xorg"},
		{key: InputMethodKey, value: "ibus"},
		{key: "BAD=KEY", value: "v"},
		{key: "BAD\x00KEY", value: "v"},
		{key: "KEY", value: "bad\x00value"},
	} {
		if err := SetEnvironmentVariable(root, test.key, test.value, false); err == nil {
			t.Fatalf("expected %q=%q to be rejected", test.key, test.value)
		}
	}
	// 写回不应发生：配置保持为空缺省。
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	for key, value := range cfg.Environment.Global {
		if key != DisplayDriverKey && key != InputMethodKey {
			t.Fatalf("invalid variable was persisted: %q=%q", key, value)
		}
	}
}

func TestEnvironmentPlatformSections(t *testing.T) {
	root := t.TempDir()
	doc := `schema_version = 1
source_order = ["godothub", "github"]

[environment]
display_driver = "auto"
input_method = "auto"
COMMON_VARIABLE = "all"

[environment.linux]
XDG_SESSION_TYPE = "x11"

[environment.windows]
EXAMPLE_WINDOWS_ONLY = "value"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(root)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.Environment.Global["COMMON_VARIABLE"] != "all" {
		t.Fatalf("global variables not loaded: %+v", cfg.Environment.Global)
	}
	if cfg.Environment.PlatformVars("linux")["XDG_SESSION_TYPE"] != "x11" {
		t.Fatalf("linux section not loaded: %+v", cfg.Environment.Linux)
	}
	if cfg.Environment.PlatformVars("windows")["EXAMPLE_WINDOWS_ONLY"] != "value" {
		t.Fatalf("windows section not loaded: %+v", cfg.Environment.Windows)
	}
	if len(cfg.Environment.PlatformVars("darwin")) != 0 {
		t.Fatalf("darwin section should be empty: %+v", cfg.Environment.Darwin)
	}
	// 写回保留平台小节。
	if err := SetEnvironmentVariable(root, "NEW_KEY", "value", false); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(root, "config.toml"))
	if err != nil {
		t.Fatal(err)
	}
	text := string(content)
	for _, fragment := range []string{"[environment.linux]", "XDG_SESSION_TYPE = \"x11\"", "EXAMPLE_WINDOWS_ONLY = \"value\"", "NEW_KEY = \"value\""} {
		if !strings.Contains(text, fragment) {
			t.Fatalf("platform section lost on write-back: %s\n%s", fragment, text)
		}
	}
}

func TestLoadRejectsInvalidPlatformSectionControlValues(t *testing.T) {
	root := t.TempDir()
	for _, section := range []string{"linux", "darwin", "windows"} {
		doc := `schema_version = 1
source_order = ["godothub", "github"]

[environment.` + section + `]
display_driver = "xorg"
`
		if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(doc), 0o600); err != nil {
			t.Fatal(err)
		}
		if _, err := Load(root); err == nil {
			t.Fatalf("invalid control value in %s section should be rejected", section)
		}
	}
}

func TestEnvironmentPlatformSectionMustBeTable(t *testing.T) {
	root := t.TempDir()
	doc := `schema_version = 1
source_order = ["godothub", "github"]

[environment]
linux = "not-a-table"
`
	if err := os.WriteFile(filepath.Join(root, "config.toml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(root); err == nil {
		t.Fatal("non-table platform section should be rejected")
	}
}
