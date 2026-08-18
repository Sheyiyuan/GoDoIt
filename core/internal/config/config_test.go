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
