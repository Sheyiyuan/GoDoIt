package instance

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/BurntSushi/toml"

	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

const validInstalledAt = "2026-08-17T00:00:00Z"

// installFixtureEngine 在临时根目录构造一个引擎资产完整安装（install.toml + payload 启动文件）。
func installFixtureEngine(t *testing.T, root, id, launcher string) {
	t.Helper()
	version := strings.TrimSuffix(id, "-standard")
	edition := "standard"
	if strings.HasSuffix(id, "-dotnet") {
		version = strings.TrimSuffix(id, "-dotnet")
		edition = "dotnet"
	}
	payload := filepath.Join(root, "engines", id, "payload")
	if err := os.MkdirAll(payload, 0o755); err != nil {
		t.Fatal(err)
	}
	manifest := store.Manifest{
		ID:                id,
		Version:           version,
		Edition:           edition,
		TargetOS:          "linux",
		TargetArch:        "amd64",
		Source:            "fixture",
		ChecksumAlgorithm: "sha256",
		Checksum:          strings.Repeat("a", 64),
		Launcher:          launcher,
		InstalledAt:       validInstalledAt,
	}
	data, err := toml.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "engines", id, "install.toml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(payload, launcher), []byte("#!/bin/sh\n"), 0o755); err != nil {
		t.Fatal(err)
	}
}

func validItem(name string) File {
	return File{
		SchemaVersion: SchemaVersion,
		ID:            "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8",
		Name:          name,
		Engine:        Engine{Version: "4.5.2", Edition: "standard"},
	}
}

func TestValidateNameAcceptsURLSafeDisplayNames(t *testing.T) {
	for _, name := range []string{"default", "work-4.5", "csharp_dev", "one.two", "工作", "GodotCSharp", "my~godot", "日本語"} {
		if err := ValidateName(name); err != nil {
			t.Fatalf("expected %q to be accepted: %v", name, err)
		}
	}
}

func TestValidateNameRejectsURLUnsafeCharacters(t *testing.T) {
	for _, name := range []string{"", "a b", "a/b", "a\\b", "a\tb", "a\nb", "a!b", "a@b", "a#b", "a%b", "a&b", "a=b", "a+b", "a;b", "a,b", "a(b)", "a[b]", "a{b}", "a:b", "a?b", "a\"b", "a'b", "😀", "，", "あ・い"} {
		if err := ValidateName(name); err == nil {
			t.Fatalf("expected %q to be rejected", name)
		}
	}
}

func TestValidateAcceptsSupportedEngineAndSDKVersions(t *testing.T) {
	for _, version := range []string{"4.7", "4.8-dev3", "4.7.2-rc1", "4.7.1-beta2"} {
		item := validItem("work")
		item.Engine.Version = version
		if err := Validate(&item, item.ID+".toml"); err != nil {
			t.Fatalf("supported engine version %q was rejected: %v", version, err)
		}
	}
	item := validItem("csharp")
	item.Engine.Edition = "dotnet"
	item.Dotnet = &Dotnet{Strategy: "managed", Version: "11.0.100-preview.7.26381.103"}
	if err := Validate(&item, item.ID+".toml"); err != nil {
		t.Fatalf("preview SDK version was rejected: %v", err)
	}
}

func TestValidateRestrictsMonoStrategyToGodot3(t *testing.T) {
	item := validItem("mono")
	item.Engine.Edition = "dotnet"
	item.Dotnet = &Dotnet{Strategy: "mono"}
	if err := Validate(&item, item.ID+".toml"); err == nil {
		t.Fatal("Godot 4.x must not accept the mono strategy")
	}
	item.Engine.Version = "3.6.2"
	if err := Validate(&item, item.ID+".toml"); err != nil {
		t.Fatalf("Godot 3.x mono strategy was rejected: %v", err)
	}
}

func TestValidateNameAllowsVersionShapedNames(t *testing.T) {
	// 显示名与版本/资产 ID 分属不同命名空间，不再互相排斥。
	for _, name := range []string{"4.5.2", "m4.5.2", "4.5.2-standard"} {
		if err := ValidateName(name); err != nil {
			t.Fatalf("expected %q to be accepted: %v", name, err)
		}
	}
}

func TestNewIDGeneratesUUIDv4(t *testing.T) {
	seen := make(map[string]struct{})
	for i := 0; i < 32; i++ {
		id, err := NewID()
		if err != nil {
			t.Fatal(err)
		}
		if !ValidID(id) {
			t.Fatalf("generated id %q is not a valid UUID v4", id)
		}
		if _, exists := seen[id]; exists {
			t.Fatalf("duplicate generated id %q", id)
		}
		seen[id] = struct{}{}
	}
}

func TestValidateRejectsSchemaIDAndNameMismatches(t *testing.T) {
	item := validItem("work")
	if err := Validate(&item, item.ID+".toml"); err != nil {
		t.Fatalf("valid item rejected: %v", err)
	}
	oldSchema := item
	oldSchema.SchemaVersion = 1
	if err := Validate(&oldSchema, oldSchema.ID+".toml"); err == nil {
		t.Fatal("schema version 1 should be rejected")
	}
	badID := item
	badID.ID = "not-a-uuid"
	if err := Validate(&badID, badID.ID+".toml"); err == nil {
		t.Fatal("non-UUID id should be rejected")
	}
	wrongFile := item
	if err := Validate(&wrongFile, "other-file.toml"); err == nil {
		t.Fatal("id not matching filename should be rejected")
	}
	dotnetOnStandard := item
	dotnetOnStandard.Dotnet = &Dotnet{Strategy: "system"}
	if err := Validate(&dotnetOnStandard, dotnetOnStandard.ID+".toml"); err == nil {
		t.Fatal("standard instance with dotnet table should be rejected")
	}
	badEngine := item
	badEngine.Engine.Version = "latest"
	if err := Validate(&badEngine, badEngine.ID+".toml"); err == nil {
		t.Fatal("non-numeric engine version should be rejected")
	}
}

func TestWriteRefusesExistingAndScanFailsClosed(t *testing.T) {
	root := t.TempDir()
	installFixtureEngine(t, root, "4.5.2-standard", "godot")
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := validItem("工作")
	if err := Write(root, item); err != nil {
		t.Fatalf("write failed: %v", err)
	}
	if err := Write(root, item); err == nil {
		t.Fatal("duplicate write must be refused")
	}
	items, err := Scan(root)
	if err != nil || len(items) != 1 || items[0].Name != "工作" {
		t.Fatalf("unexpected scan: %+v err=%v", items, err)
	}
	// 手写一个 id 与文件名不一致的坏条目文件。
	badPath := filepath.Join(root, "instances", "7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d.toml")
	content := "schema_version = 2\nid = \"3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8\"\nname = \"other\"\n[engine]\nversion = \"4.5.2\"\nedition = \"standard\"\n"
	if err := os.WriteFile(badPath, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(root); err == nil {
		t.Fatal("id/filename mismatch must fail the scan")
	}
	if err := os.Remove(badPath); err != nil {
		t.Fatal(err)
	}
	// 显示名重复使扫描失败关闭。
	duplicate := validItem("工作")
	duplicate.ID = "7c4b8d2a-1e6f-4b3a-9d5c-2f8e6a4b1c3d"
	data, err := toml.Marshal(duplicate)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instances", duplicate.ID+".toml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Scan(root); err == nil {
		t.Fatal("duplicate display name must fail the scan")
	}
}

func TestLookupFindsByNameAndMisses(t *testing.T) {
	root := t.TempDir()
	installFixtureEngine(t, root, "4.5.2-standard", "godot")
	item := validItem("工作")
	if err := Write(root, item); err != nil {
		t.Fatal(err)
	}
	found, err := Lookup(root, "工作")
	if err != nil || found.ID != item.ID {
		t.Fatalf("lookup failed: %+v err=%v", found, err)
	}
	if _, err := Lookup(root, "missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("expected not found, got %v", err)
	}
	if _, err := Lookup(root, "bad name!"); err == nil {
		t.Fatal("invalid display name should be rejected")
	}
}

func TestReadRejectsMissingEngineReference(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "instances"), 0o755); err != nil {
		t.Fatal(err)
	}
	item := validItem("work")
	data, err := toml.Marshal(item)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "instances", item.ID+".toml"), data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Read(root, item.ID); err == nil {
		t.Fatal("engine reference missing must fail the read")
	}
}

func TestSetEnvPreservesUnknownFieldsAndRemovesEmptyTable(t *testing.T) {
	root := t.TempDir()
	installFixtureEngine(t, root, "4.5.2-standard", "godot")
	item := validItem("work")
	item.Env = map[string]string{"KEEP": "v"}
	if err := Write(root, item); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(root, item.ID, "NEW", "value", false); err != nil {
		t.Fatal(err)
	}
	// 手写未知字段，验证写回保留。
	path := filepath.Join(root, "instances", item.ID+".toml")
	content := "schema_version = 2\nid = \"" + item.ID + "\"\nname = \"work\"\nfuture_field = \"kept\"\n[engine]\nversion = \"4.5.2\"\nedition = \"standard\"\n[env]\nKEEP = \"v\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(root, item.ID, "NEW", "value", false); err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		t.Fatal(err)
	}
	if raw["future_field"] != "kept" {
		t.Fatalf("unknown field was lost: %+v", raw)
	}
	environment, _ := raw["env"].(map[string]any)
	if environment == nil || environment["KEEP"] != "v" || environment["NEW"] != "value" {
		t.Fatalf("env table was not merged: %+v", raw)
	}
	// 删除最后一个变量后 env 表整体消失。
	if err := SetEnv(root, item.ID, "KEEP", "", true); err != nil {
		t.Fatal(err)
	}
	if err := SetEnv(root, item.ID, "NEW", "", true); err != nil {
		t.Fatal(err)
	}
	// 注意：BurntSushi 解码到已存在的 map 是增量合并，必须重置后再解码。
	raw = make(map[string]any)
	if _, err := toml.DecodeFile(path, &raw); err != nil {
		t.Fatal(err)
	}
	if _, exists := raw["env"]; exists {
		t.Fatalf("empty env table should be removed: %+v", raw)
	}
}

func TestBuildReferencesMapsManagedSDKOnly(t *testing.T) {
	standard := validItem("plain")
	managed := validItem("csharp")
	managed.Engine.Edition = "dotnet"
	managed.Dotnet = &Dotnet{Strategy: "managed", Version: "8.0.410"}
	managed.Template = &Template{ID: "4.5.2-dotnet"}
	system := validItem("sys")
	system.Engine.Edition = "dotnet"
	system.Dotnet = &Dotnet{Strategy: "system"}
	refs := BuildReferences([]File{standard, managed, system})
	if len(refs.Engines["4.5.2-standard"]) != 1 || refs.Engines["4.5.2-standard"][0] != "plain" {
		t.Fatalf("unexpected engine refs: %+v", refs.Engines)
	}
	if len(refs.SDKs["8.0.410"]) != 1 || refs.SDKs["8.0.410"][0] != "csharp" {
		t.Fatalf("managed SDK refs missing: %+v", refs.SDKs)
	}
	if len(refs.Engines["4.5.2-dotnet"]) != 2 {
		t.Fatalf("dotnet engine refs missing: %+v", refs.Engines)
	}
	if _, exists := refs.SDKs[""]; exists {
		t.Fatalf("system strategy must not reference an SDK: %+v", refs.SDKs)
	}
	if len(refs.Templates["4.5.2-dotnet"]) != 1 || refs.Templates["4.5.2-dotnet"][0] != "csharp" {
		t.Fatalf("template refs missing: %+v", refs.Templates)
	}
}

func TestValidateTemplateMustMatchEngine(t *testing.T) {
	item := validItem("work")
	item.Template = &Template{ID: "4.5.1-standard"}
	if err := Validate(&item, item.ID+".toml"); err == nil {
		t.Fatal("mismatched template reference should fail")
	}
	item.Template.ID = "4.5.2-standard"
	if err := Validate(&item, item.ID+".toml"); err != nil {
		t.Fatalf("matching template reference failed: %v", err)
	}
}

func TestRemoveDeletesByID(t *testing.T) {
	root := t.TempDir()
	installFixtureEngine(t, root, "4.5.2-standard", "godot")
	item := validItem("work")
	if err := Write(root, item); err != nil {
		t.Fatal(err)
	}
	if err := Remove(root, item.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "instances", item.ID+".toml")); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("instance file still exists: %v", err)
	}
	if err := Remove(root, "not-a-uuid"); err == nil {
		t.Fatal("invalid id should be rejected")
	}
}
