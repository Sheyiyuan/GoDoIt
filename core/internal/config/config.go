// Package config 负责读取和原子写回用户级 TOML 配置。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

const defaultSchemaVersion = 1

// ErrSourceNotConfigured 表示来源名不在当前 source_order 中。
var ErrSourceNotConfigured = errors.New("source is not configured")

const (
	// DisplayDriverKey 是显示驱动控制键。
	DisplayDriverKey = "display_driver"
	// InputMethodKey 是输入法控制键。
	InputMethodKey = "input_method"
)

// SourceEntry 描述配置中的一个来源。
type SourceEntry struct {
	Name     string // 来源名
	Kind     string // builtin 或 custom
	Disabled bool   // 是否被 source ban 禁用
}

// ListSources 按 source_order 顺序返回配置中的来源。
func ListSources(root string) ([]SourceEntry, error) {
	cfg, err := Load(root)
	if err != nil {
		return nil, err
	}
	entries := make([]SourceEntry, 0, len(cfg.SourceOrder))
	for _, name := range cfg.SourceOrder {
		kind := "custom"
		if name == "godothub" || name == "github" || name == "atomgit" {
			kind = "builtin"
		}
		entries = append(entries, SourceEntry{Name: name, Kind: kind, Disabled: IsSourceDisabled(cfg, name)})
	}
	return entries, nil
}

// SetSourceOrderFirst 把指定来源移到 source_order 首位并原子写回 config.toml。
// 配置文件不存在时按内置默认创建后再调整。写回保留全部字段，不保留注释。
func SetSourceOrderFirst(root, name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("source name must not be empty")
	}
	cfg, err := Load(root)
	if err != nil {
		return err
	}
	if err := requireSource(cfg, name); err != nil {
		return err
	}
	cfg.SourceOrder = moveFirst(cfg.SourceOrder, name)
	return writeConfig(root, cfg)
}

// SetSourceDisabled 更新禁用名单并原子写回 config.toml。
// 配置文件不存在时按内置默认创建后再调整。写回保留全部字段，不保留注释。
func SetSourceDisabled(root, name string, disabled bool) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return errors.New("source name must not be empty")
	}
	cfg, err := Load(root)
	if err != nil {
		return err
	}
	if err := requireSource(cfg, name); err != nil {
		return err
	}
	next := make([]string, 0, len(cfg.DisabledSources))
	for _, item := range cfg.DisabledSources {
		if item != name {
			next = append(next, item)
		}
	}
	if disabled {
		// 剔除后重新加入，重复 ban 幂等。
		next = append(next, name)
	}
	cfg.DisabledSources = next
	return writeConfig(root, cfg)
}

// SetEnvironmentVariable 设置或删除全局环境变量并原子写回配置。
// remove 为 true 时忽略 value；重复设置和删除均幂等。
func SetEnvironmentVariable(root, key, value string, remove bool) error {
	var validateErr error
	if remove {
		validateErr = ValidateEnvironmentKey(key)
	} else {
		validateErr = ValidateEnvironmentVariable(key, value)
	}
	if validateErr != nil {
		return validateErr
	}
	cfg, err := Load(root)
	if err != nil {
		return err
	}
	if cfg.Environment == nil {
		cfg.Environment = defaultEnvironment()
	}
	if remove {
		delete(cfg.Environment, key)
	} else {
		cfg.Environment[key] = value
	}
	return writeConfig(root, cfg)
}

// ValidateEnvironmentKey 校验环境键名可安全传给 execve。
func ValidateEnvironmentKey(key string) error {
	if key == "" || strings.ContainsAny(key, "=\x00") {
		return errors.New("environment key must not be empty or contain '=' or NUL")
	}
	return nil
}

// ValidateEnvironmentVariable 校验环境键值可安全传给 execve。
func ValidateEnvironmentVariable(key, value string) error {
	if err := ValidateEnvironmentKey(key); err != nil {
		return err
	}
	if strings.ContainsRune(value, '\x00') {
		return errors.New("environment value must not contain NUL")
	}
	if key == DisplayDriverKey && value != "auto" && value != "x11" && value != "wayland" {
		return errors.New("display_driver must be auto, x11 or wayland")
	}
	if key == InputMethodKey && value != "auto" && value != "fcitx" && value != "off" {
		return errors.New("input_method must be auto, fcitx or off")
	}
	return nil
}

// IsSourceDisabled 返回指定来源是否在禁用名单中。
func IsSourceDisabled(cfg File, name string) bool {
	for _, item := range cfg.DisabledSources {
		if item == name {
			return true
		}
	}
	return false
}

func requireSource(cfg File, name string) error {
	for _, item := range cfg.SourceOrder {
		if item == name {
			return nil
		}
	}
	return fmt.Errorf("%w: %q", ErrSourceNotConfigured, name)
}

// writeConfig 把已知字段覆盖进已解码的配置 map（保留未知字段）后原子写回。
func writeConfig(root string, cfg File) error {
	path := filepath.Join(root, "config.toml")
	configMap := make(map[string]any)
	if _, err := os.Stat(path); err == nil {
		if _, err := toml.DecodeFile(path, &configMap); err != nil {
			return fmt.Errorf("decode config: %w", err)
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("stat config: %w", err)
	}
	configMap["schema_version"] = cfg.SchemaVersion
	configMap["source_order"] = cfg.SourceOrder
	if len(cfg.DisabledSources) > 0 {
		configMap["disabled_sources"] = cfg.DisabledSources
	} else {
		delete(configMap, "disabled_sources")
	}
	if len(cfg.CustomSources) > 0 {
		configMap["custom_sources"] = mergeCustomSources(configMap, cfg.CustomSources)
	} else {
		delete(configMap, "custom_sources")
	}
	if _, existed := configMap["environment"]; existed || !isDefaultEnvironment(cfg.Environment) {
		// 原文件已有 environment 表或用户设置了非默认值时才写回，避免任意写回
		// （如 source use/ban）把默认控制键物化进旧配置。
		configMap["environment"] = cfg.Environment
	} else {
		delete(configMap, "environment")
	}
	var builder strings.Builder
	if err := toml.NewEncoder(&builder).Encode(configMap); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return writeFileAtomic(path, builder.String())
}

// isDefaultEnvironment 报告环境 map 是否恰好等于默认控制键（display_driver/input_method 均为 auto）。
func isDefaultEnvironment(environment map[string]string) bool {
	if len(environment) != 2 {
		return false
	}
	for key, value := range environment {
		if key != DisplayDriverKey && key != InputMethodKey {
			return false
		}
		if value != "auto" {
			return false
		}
	}
	return true
}

// mergeCustomSources 逐条把 struct 已知字段覆盖进原配置 map 的 custom_sources 条目，
// 保留条目内未进入 struct 的未知字段，避免一次写回丢失用户配置。
func mergeCustomSources(configMap map[string]any, sources []CustomSource) []any {
	existing := make(map[string]map[string]any)
	// BurntSushi 解码 array of tables 到 map[string]any 时为 []map[string]any。
	if raw, ok := configMap["custom_sources"].([]map[string]any); ok {
		for _, entry := range raw {
			if name, ok := entry["name"].(string); ok {
				existing[name] = entry
			}
		}
	}
	result := make([]any, 0, len(sources))
	for _, source := range sources {
		entry, ok := existing[source.Name]
		if !ok {
			entry = make(map[string]any)
		}
		entry["name"] = source.Name
		entry["artifact_url"] = source.ArtifactURL
		entry["checksum_url"] = source.ChecksumURL
		if source.AuthorizationEnv != "" {
			entry["authorization_env"] = source.AuthorizationEnv
		} else {
			delete(entry, "authorization_env")
		}
		result = append(result, entry)
	}
	return result
}

func moveFirst(order []string, name string) []string {
	result := make([]string, 0, len(order))
	result = append(result, name)
	for _, item := range order {
		if item != name {
			result = append(result, item)
		}
	}
	return result
}

// writeFileAtomic 以临时文件 + rename + 父目录 fsync 的方式写回配置。
func writeFileAtomic(path, content string) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return fmt.Errorf("create config directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".gdit-config-*.tmp")
	if err != nil {
		return fmt.Errorf("create config temporary file: %w", err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("chmod config temporary file: %w", err)
	}
	if _, err := temporary.WriteString(content); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("write config temporary file: %w", err)
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("sync config temporary file: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close config temporary file: %w", err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := syncDirectory(directory); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
}

func syncDirectory(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}

// File 是 ~/.gdit/config.toml 的结构。
type File struct {
	SchemaVersion   int               `toml:"schema_version"`
	SourceOrder     []string          `toml:"source_order"`
	DisabledSources []string          `toml:"disabled_sources"`
	CustomSources   []CustomSource    `toml:"custom_sources"`
	Environment     map[string]string `toml:"environment"`
}

// CustomSource 描述一个与 Godot 资产 URL 兼容的自定义镜像。
type CustomSource struct {
	Name             string `toml:"name"`
	ArtifactURL      string `toml:"artifact_url"`
	ChecksumURL      string `toml:"checksum_url"`
	AuthorizationEnv string `toml:"authorization_env"`
}

// Default 返回不写入磁盘的默认配置。
// AtomGit 独立来源规则确认前不加入默认顺序，避免默认配置下镜像不可用时安装被配置错误终止。
func Default() File {
	return File{
		SchemaVersion: defaultSchemaVersion,
		SourceOrder:   []string{"godothub", "github"},
		Environment:   defaultEnvironment(),
	}
}

func defaultEnvironment() map[string]string {
	return map[string]string{DisplayDriverKey: "auto", InputMethodKey: "auto"}
}

// Load 读取配置文件。文件不存在时返回默认配置。
func Load(root string) (File, error) {
	path := filepath.Join(root, "config.toml")
	var cfg File
	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		return Default(), nil
	} else if err != nil {
		return File{}, fmt.Errorf("stat config: %w", err)
	}
	if _, err := toml.DecodeFile(path, &cfg); err != nil {
		return File{}, fmt.Errorf("decode config: %w", err)
	}
	if cfg.SchemaVersion != defaultSchemaVersion {
		return File{}, fmt.Errorf("unsupported schema version %d", cfg.SchemaVersion)
	}
	if len(cfg.SourceOrder) == 0 {
		return File{}, errors.New("source_order must not be empty")
	}
	if err := validateSources(cfg); err != nil {
		return File{}, err
	}
	if cfg.Environment == nil {
		cfg.Environment = defaultEnvironment()
	} else {
		if _, ok := cfg.Environment[DisplayDriverKey]; !ok {
			cfg.Environment[DisplayDriverKey] = "auto"
		}
		if _, ok := cfg.Environment[InputMethodKey]; !ok {
			cfg.Environment[InputMethodKey] = "auto"
		}
	}
	for key, value := range cfg.Environment {
		if err := ValidateEnvironmentVariable(key, value); err != nil {
			return File{}, fmt.Errorf("invalid environment variable %q: %w", key, err)
		}
	}
	return cfg, nil
}

func validateSources(cfg File) error {
	custom := make(map[string]struct{}, len(cfg.CustomSources))
	for _, item := range cfg.CustomSources {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			return errors.New("custom source name must not be empty")
		}
		if name != item.Name {
			return fmt.Errorf("custom source name %q contains surrounding whitespace", item.Name)
		}
		if name == "godothub" || name == "github" || name == "atomgit" {
			return fmt.Errorf("custom source name %q conflicts with a built-in source", name)
		}
		if _, ok := custom[name]; ok {
			return fmt.Errorf("duplicate custom source %q", name)
		}
		custom[name] = struct{}{}
		if strings.TrimSpace(item.ArtifactURL) == "" || strings.TrimSpace(item.ChecksumURL) == "" {
			return fmt.Errorf("custom source %q needs artifact_url and checksum_url", name)
		}
	}
	seen := make(map[string]struct{}, len(cfg.SourceOrder))
	for _, item := range cfg.SourceOrder {
		name := strings.TrimSpace(item)
		if name == "" {
			return errors.New("source_order contains an empty name")
		}
		if name != item {
			return fmt.Errorf("source_order entry %q contains surrounding whitespace", item)
		}
		if _, ok := seen[name]; ok {
			return fmt.Errorf("duplicate source_order entry %q", name)
		}
		seen[name] = struct{}{}
		if _, ok := custom[name]; !ok && name != "godothub" && name != "atomgit" && name != "github" {
			return fmt.Errorf("source %q is not built in or custom", name)
		}
	}
	disabled := make(map[string]struct{}, len(cfg.DisabledSources))
	for _, item := range cfg.DisabledSources {
		name := strings.TrimSpace(item)
		if name == "" {
			return errors.New("disabled_sources contains an empty name")
		}
		if name != item {
			return fmt.Errorf("disabled_sources entry %q contains surrounding whitespace", item)
		}
		if _, ok := disabled[name]; ok {
			return fmt.Errorf("duplicate disabled_sources entry %q", name)
		}
		disabled[name] = struct{}{}
		if _, ok := seen[name]; !ok {
			return fmt.Errorf("disabled source %q is not in source_order", name)
		}
	}
	return nil
}
