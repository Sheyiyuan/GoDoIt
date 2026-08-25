package store

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

// TemplateSchemaVersion 是导出模板安装清单的 schema 版本。
const TemplateSchemaVersion = 1

const templateVersionFileLimit = 1024

// TemplateManifest 是模板目录内的完整安装标记。
type TemplateManifest struct {
	SchemaVersion     int    `toml:"schema_version"`
	ID                string `toml:"id"`
	Version           string `toml:"version"`
	Edition           string `toml:"edition"`
	Source            string `toml:"source"`
	ArchiveName       string `toml:"archive_name"`
	ChecksumAlgorithm string `toml:"checksum_algorithm"`
	Checksum          string `toml:"checksum"`
	InstalledAt       string `toml:"installed_at"`
}

// TemplateRecord 是一个完整模板目录及其安装标记。
type TemplateRecord struct {
	Manifest TemplateManifest
	Dir      string
}

// ScanTemplates 扫描完整模板资产，不修改 state.toml。
func (s *Store) ScanTemplates() ([]TemplateRecord, error) {
	entries, err := os.ReadDir(s.TemplatesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read templates directory: %w", err)
	}
	result := make([]TemplateRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := s.TemplateDir(entry.Name())
		manifest, readErr := ReadTemplateManifest(filepath.Join(dir, "install.toml"))
		if readErr != nil || validateTemplateManifest(manifest, entry.Name(), dir) != nil {
			continue
		}
		result = append(result, TemplateRecord{Manifest: manifest, Dir: dir})
	}
	sort.Slice(result, func(i, j int) bool { return result[i].Manifest.ID < result[j].Manifest.ID })
	return result, nil
}

// InspectTemplateDir 检查单个模板资产目录，返回具体无效原因。
func InspectTemplateDir(dir string) error {
	id := filepath.Base(dir)
	if !managedversion.ValidEngineID(id) {
		return fmt.Errorf("invalid template directory name %q", id)
	}
	manifest, err := ReadTemplateManifest(filepath.Join(dir, "install.toml"))
	if err != nil {
		return fmt.Errorf("read install.toml: %w", err)
	}
	if err := validateTemplateManifest(manifest, id, dir); err != nil {
		return fmt.Errorf("invalid install.toml: %w", err)
	}
	return nil
}

// ReadTemplateManifest 读取模板安装标记。
func ReadTemplateManifest(path string) (TemplateManifest, error) {
	var manifest TemplateManifest
	if _, err := toml.DecodeFile(path, &manifest); err != nil {
		return TemplateManifest{}, err
	}
	return manifest, nil
}

// WriteTemplateManifest 在 tmp 下的 staging 目录写入模板安装标记。
func (s *Store) WriteTemplateManifest(staging string, manifest TemplateManifest) error {
	if !within(s.TmpDir(), staging) {
		return errors.New("template manifest staging directory must be inside tmp")
	}
	if err := validateTemplateManifest(manifest, manifest.ID, staging); err != nil {
		return fmt.Errorf("validate template install manifest: %w", err)
	}
	return writeTOML(filepath.Join(staging, "install.toml"), manifest)
}

// PublishTemplate 原子发布完成校验的模板 staging 目录。
func (s *Store) PublishTemplate(staging, id string) (published bool, err error) {
	if !managedversion.ValidEngineID(id) || filepath.Base(staging) == "." || !within(s.TmpDir(), staging) {
		return false, errors.New("invalid template ID or staging directory")
	}
	destination := s.TemplateDir(id)
	if _, err := os.Lstat(destination); err == nil {
		return false, ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check template destination: %w", err)
	}
	if err := platform.RenameAtomic(staging, destination); err != nil {
		return false, fmt.Errorf("publish template: %w", err)
	}
	if err := s.syncDir(s.TemplatesDir()); err != nil {
		return true, fmt.Errorf("sync templates directory: %w", err)
	}
	return true, nil
}

// RemoveTemplate 删除指定未引用模板目录。
func (s *Store) RemoveTemplate(id string) error {
	if !managedversion.ValidEngineID(id) {
		return fmt.Errorf("invalid template ID %q", id)
	}
	if err := os.RemoveAll(s.TemplateDir(id)); err != nil {
		return err
	}
	return platform.SyncDir(s.TemplatesDir())
}

func validateTemplateManifest(manifest TemplateManifest, id, dir string) error {
	if manifest.SchemaVersion != TemplateSchemaVersion || manifest.ID != id || !managedversion.ValidEngineID(id) || manifest.ID != manifest.Version+"-"+manifest.Edition {
		return errors.New("invalid template manifest identity")
	}
	if manifest.Source == "" || filepath.Base(manifest.ArchiveName) != manifest.ArchiveName || manifest.ArchiveName == "." {
		return errors.New("invalid template source or archive name")
	}
	length := 0
	switch manifest.ChecksumAlgorithm {
	case "sha256":
		length = 64
	case "sha512":
		length = 128
	default:
		return errors.New("invalid template checksum algorithm")
	}
	if len(manifest.Checksum) != length {
		return errors.New("invalid template checksum")
	}
	if _, err := hex.DecodeString(manifest.Checksum); err != nil {
		return errors.New("invalid template checksum")
	}
	if _, err := time.Parse(time.RFC3339, manifest.InstalledAt); err != nil {
		return errors.New("invalid template installation time")
	}
	payload := filepath.Join(dir, "payload")
	entries, err := os.ReadDir(payload)
	if err != nil || len(entries) == 0 {
		return errors.New("template payload is missing or empty")
	}
	hasPayload := false
	for _, entry := range entries {
		if entry.Name() != "version.txt" {
			hasPayload = true
			break
		}
	}
	if !hasPayload {
		return errors.New("template payload contains no export templates")
	}
	versionPath := filepath.Join(payload, "version.txt")
	info, err := os.Lstat(versionPath)
	if err != nil || !info.Mode().IsRegular() || info.Size() > templateVersionFileLimit {
		return errors.New("template payload version.txt is missing or invalid")
	}
	content, err := os.ReadFile(versionPath)
	if err != nil || !templateVersionMatches(strings.TrimSpace(string(content)), manifest.Version, manifest.Edition) {
		return errors.New("template payload version.txt does not match request")
	}
	return nil
}

func templateVersionMatches(actual, version, edition string) bool {
	expected := strings.ReplaceAll(version, "-", ".")
	if !strings.Contains(version, "-") {
		expected += ".stable"
	}
	if edition == "dotnet" {
		expected += ".mono"
	}
	return actual == expected
}
