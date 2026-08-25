// Package config 负责读取和原子写回用户级 TOML 配置。
package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/BurntSushi/toml"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

const defaultSchemaVersion = 1

// ErrSourceNotConfigured 表示来源名不在当前 source_order 中。
var ErrSourceNotConfigured = errors.New("source is not configured")

const (
	// DisplayDriverKey 是显示驱动控制键。
	DisplayDriverKey = "display_driver"
	// InputMethodKey 是输入法控制键。
	InputMethodKey = "input_method"
	// DefaultTitlebarStyle 是跨平台顶栏的自动跟随系统值。
	DefaultTitlebarStyle = "auto"
	// TitlebarStyleMac 是左上角红黄绿交通灯风格。
	TitlebarStyleMac = "mac"
	// TitlebarStyleWindows 是右上角窗口控制按钮风格。
	TitlebarStyleWindows = "windows"
)

// GUISettings 是 config.toml 中的 GUI 偏好设置。
type GUISettings struct {
	TitlebarStyle string `toml:"titlebar_style"`
}

// ValidateTitlebarStyle 校验顶栏风格值。
func ValidateTitlebarStyle(style string) error {
	switch style {
	case DefaultTitlebarStyle, TitlebarStyleMac, TitlebarStyleWindows:
		return nil
	default:
		return fmt.Errorf("titlebar_style must be auto, mac or windows")
	}
}

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

// Environment 是 config.toml 的 [environment] 表：Global 为三平台通用变量，
// Linux/Darwin/Windows 为平台小节（仅当前平台生效，覆盖全局同名键）。
// 自定义 UnmarshalTOML 支持平台子表（BurntSushi 的 map[string]string 无法承载嵌套表）。
type Environment struct {
	Global  map[string]string
	Linux   map[string]string
	Darwin  map[string]string
	Windows map[string]string
}

// UnmarshalTOML 拆分 [environment] 表：除 linux/darwin/windows 子表外的键归入 Global。
func (e *Environment) UnmarshalTOML(data any) error {
	table, ok := data.(map[string]any)
	if !ok {
		return errors.New("environment must be a table")
	}
	result := Environment{Global: map[string]string{}}
	for key, value := range table {
		if key == "linux" || key == "darwin" || key == "windows" {
			sub, ok := value.(map[string]any)
			if !ok {
				return fmt.Errorf("environment.%s must be a table", key)
			}
			target := map[string]string{}
			for subKey, subValue := range sub {
				text, ok := subValue.(string)
				if !ok {
					return fmt.Errorf("environment.%s.%s must be a string", key, subKey)
				}
				target[subKey] = text
			}
			switch key {
			case "linux":
				result.Linux = target
			case "darwin":
				result.Darwin = target
			case "windows":
				result.Windows = target
			}
			continue
		}
		text, ok := value.(string)
		if !ok {
			return fmt.Errorf("environment.%s must be a string", key)
		}
		result.Global[key] = text
	}
	*e = result
	return nil
}

// platformSections 返回平台小节名到变量的映射（供校验与编码复用）。
func (e Environment) platformSections() map[string]map[string]string {
	return map[string]map[string]string{
		"linux":   e.Linux,
		"darwin":  e.Darwin,
		"windows": e.Windows,
	}
}

// PlatformVars 返回指定 OS 的平台小节变量；小节不存在时返回空 map。
// 供 core 环境合并使用（仅当前平台生效）。
func (e Environment) PlatformVars(osName string) map[string]string {
	switch osName {
	case "linux":
		return e.Linux
	case "darwin":
		return e.Darwin
	case "windows":
		return e.Windows
	}
	return nil
}

// toMap 把 Environment 编码为写回用的嵌套 map（空小节不输出）。
func (e Environment) toMap() map[string]any {
	result := make(map[string]any, len(e.Global)+3)
	for key, value := range e.Global {
		result[key] = value
	}
	for name, section := range e.platformSections() {
		if len(section) == 0 {
			continue
		}
		sub := make(map[string]any, len(section))
		for key, value := range section {
			sub[key] = value
		}
		result[name] = sub
	}
	return result
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
	if cfg.Environment.Global == nil {
		cfg.Environment.Global = defaultEnvironment()
	}
	key = NormalizeEnvironmentKey(key)
	for existing := range cfg.Environment.Global {
		if NormalizeEnvironmentKey(existing) == key {
			delete(cfg.Environment.Global, existing)
		}
	}
	if remove {
	} else {
		cfg.Environment.Global[key] = value
	}
	return writeConfig(root, cfg)
}

// SetTitlebarStyle 设置 GUI 顶栏风格并原子写回配置。
func SetTitlebarStyle(root, style string) error {
	if err := ValidateTitlebarStyle(style); err != nil {
		return err
	}
	cfg, err := Load(root)
	if err != nil {
		return err
	}
	cfg.GUI.TitlebarStyle = style
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
	if key == DisplayDriverKey || key == InputMethodKey {
		if err := platform.ValidateControlValue(platform.CurrentOSName(), key, value); err != nil {
			return err
		}
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
		configMap["environment"] = cfg.Environment.toMap()
	} else {
		delete(configMap, "environment")
	}
	if _, existed := configMap["gui"]; existed || cfg.GUI.TitlebarStyle != DefaultTitlebarStyle {
		configMap["gui"] = mergeGUISettings(configMap, cfg.GUI)
	} else {
		delete(configMap, "gui")
	}
	var builder strings.Builder
	if err := toml.NewEncoder(&builder).Encode(configMap); err != nil {
		return fmt.Errorf("encode config: %w", err)
	}
	return writeFileAtomic(path, builder.String())
}

// mergeGUISettings 覆盖已知 GUI 字段并保留配置文件中的未知字段。
func mergeGUISettings(configMap map[string]any, settings GUISettings) map[string]any {
	result := make(map[string]any)
	if existing, ok := configMap["gui"].(map[string]any); ok {
		for key, value := range existing {
			result[key] = value
		}
	}
	result["titlebar_style"] = settings.TitlebarStyle
	return result
}

// isDefaultEnvironment 报告环境是否恰好等于默认控制键（display_driver/input_method 均为 auto，
// 无平台小节）。
func isDefaultEnvironment(environment Environment) bool {
	if len(environment.Global) != 2 {
		return false
	}
	for key, value := range environment.Global {
		if key != DisplayDriverKey && key != InputMethodKey {
			return false
		}
		if value != "auto" {
			return false
		}
	}
	for _, section := range environment.platformSections() {
		if len(section) != 0 {
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
	if err := platform.RenameAtomic(temporaryPath, path); err != nil {
		return fmt.Errorf("replace config: %w", err)
	}
	if err := platform.SyncDir(directory); err != nil {
		return fmt.Errorf("sync config directory: %w", err)
	}
	return nil
}

// File 是 ~/.gdit/config.toml 的结构。
type File struct {
	SchemaVersion   int            `toml:"schema_version"`
	SourceOrder     []string       `toml:"source_order"`
	DisabledSources []string       `toml:"disabled_sources"`
	CustomSources   []CustomSource `toml:"custom_sources"`
	Environment     Environment    `toml:"environment"`
	GUI             GUISettings    `toml:"gui"`
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
		Environment:   Environment{Global: defaultEnvironment()},
		GUI:           GUISettings{TitlebarStyle: DefaultTitlebarStyle},
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
	if cfg.Environment.Global == nil {
		cfg.Environment.Global = defaultEnvironment()
	} else {
		if _, ok := cfg.Environment.Global[DisplayDriverKey]; !ok {
			cfg.Environment.Global[DisplayDriverKey] = "auto"
		}
		if _, ok := cfg.Environment.Global[InputMethodKey]; !ok {
			cfg.Environment.Global[InputMethodKey] = "auto"
		}
	}
	if cfg.GUI.TitlebarStyle == "" {
		cfg.GUI.TitlebarStyle = DefaultTitlebarStyle
	}
	if err := ValidateTitlebarStyle(cfg.GUI.TitlebarStyle); err != nil {
		return File{}, err
	}
	for sectionOS, section := range cfg.Environment.platformSections() {
		for key, value := range section {
			if err := validateSectionVariable(sectionOS, key, value); err != nil {
				return File{}, fmt.Errorf("invalid environment variable %q: %w", key, err)
			}
		}
	}
	for key, value := range cfg.Environment.Global {
		if err := ValidateEnvironmentVariable(key, value); err != nil {
			return File{}, fmt.Errorf("invalid environment variable %q: %w", key, err)
		}
	}
	if err := validateUniqueEnvironmentKeys(cfg.Environment.Global); err != nil {
		return File{}, err
	}
	if err := validateUniqueEnvironmentKeys(cfg.Environment.PlatformVars(platform.CurrentOSName())); err != nil {
		return File{}, err
	}
	return cfg, nil
}

func validateUniqueEnvironmentKeys(values map[string]string) error {
	seen := make(map[string]string, len(values))
	for key := range values {
		normalized := NormalizeEnvironmentKey(key)
		if previous, exists := seen[normalized]; exists && previous != key {
			return fmt.Errorf("environment keys %q and %q conflict on this platform", previous, key)
		}
		seen[normalized] = key
	}
	return nil
}

// NormalizeEnvironmentKey 返回环境变量写回与比较使用的键：控制键保持小写专用语义，
// 其他键按当前平台规范化（Windows 大小写不敏感，POSIX 保持原值）。
func NormalizeEnvironmentKey(key string) string {
	if key == DisplayDriverKey || key == InputMethodKey {
		return key
	}
	return platform.NormalizeEnvKey(key)
}

// validateSectionVariable 校验平台小节的环境变量：与全局相同的键名校验规则
// （非空、不含 = 与 NUL），控制键取值按小节所属平台的规则。
func validateSectionVariable(sectionOS, key, value string) error {
	if err := ValidateEnvironmentKey(key); err != nil {
		return err
	}
	if strings.ContainsRune(value, '\x00') {
		return errors.New("environment value must not contain NUL")
	}
	if key == DisplayDriverKey || key == InputMethodKey {
		if err := platform.ValidateControlValue(sectionOS, key, value); err != nil {
			return err
		}
	}
	return nil
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
		if item.AuthorizationEnv != "" {
			if err := ValidateEnvironmentKey(item.AuthorizationEnv); err != nil {
				return fmt.Errorf("custom source %q has invalid authorization_env: %w", name, err)
			}
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
