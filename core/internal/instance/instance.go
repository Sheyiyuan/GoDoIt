// Package instance 负责启动器条目的读写、校验与引用扫描。
// 条目文件以 UUID v4 命名（instances/<uuid>.toml），文件内 name 是用户可见的显示名：
// 显示名只承担 CLI 寻址与展示，存储标识（id）承担文件系统与引用锚定。
package instance

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode"

	"github.com/BurntSushi/toml"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

// SchemaVersion 是当前条目文件 schema 版本（v2：id 存储标识 + name 显示名分离）。
const SchemaVersion = 2

const (
	// IconDefault 表示按 edition 解析内置图标。
	IconDefault = "default"
	// IconGodot 表示固定使用 Godot 图标。
	IconGodot = "godot"
	// IconCSharp 表示固定使用 C# 图标。
	IconCSharp = "csharp"
	// IconMascot 表示固定使用 GoDoIt 吉祥物图标。
	IconMascot = "mascot"
	// IconCustom 表示使用 icons/<uuid>.png 自定义图标。
	IconCustom = "custom"
)

// uuidPattern 是条目存储标识符（UUID v4，小写十六进制）的格式。
var uuidPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

var iconBackgroundPattern = regexp.MustCompile(`^#[0-9A-Fa-f]{6}([0-9A-Fa-f]{2})?$`)

// Engine 描述条目引用的引擎资产。
type Engine struct {
	Version string `toml:"version"`
	Edition string `toml:"edition"`
}

// Dotnet 描述 dotnet 条目的 SDK 策略。
type Dotnet struct {
	Strategy string `toml:"strategy"`
	Version  string `toml:"version,omitempty"`
}

// Template 描述条目绑定的精确导出模板资产。
type Template struct {
	ID string `toml:"id"`
}

// Appearance 描述条目的 GUI 图标策略。
type Appearance struct {
	Icon       string `toml:"icon"`
	CustomIcon string `toml:"custom_icon,omitempty"`
	Background string `toml:"background,omitempty"`
}

// File 是 instances/<uuid>.toml 的结构。
type File struct {
	SchemaVersion int               `toml:"schema_version"`
	ID            string            `toml:"id"`   // 存储标识符（UUID v4），与文件名一致
	Name          string            `toml:"name"` // 显示名：CLI 寻址用，唯一，可中文
	Engine        Engine            `toml:"engine"`
	Dotnet        *Dotnet           `toml:"dotnet,omitempty"`
	Template      *Template         `toml:"template,omitempty"`
	Appearance    *Appearance       `toml:"appearance,omitempty"`
	Env           map[string]string `toml:"env,omitempty"`
}

// References 是条目扫描派生出的资产引用关系，值为引用该资产的条目显示名。
type References struct {
	Engines   map[string][]string
	SDKs      map[string][]string
	Templates map[string][]string
}

// NewID 生成一个 UUID v4 存储标识符（crypto/rand，零第三方依赖）。
func NewID() (string, error) {
	var bytes [16]byte
	if _, err := rand.Read(bytes[:]); err != nil {
		return "", fmt.Errorf("generate instance id: %w", err)
	}
	bytes[6] = (bytes[6] & 0x0f) | 0x40 // 版本位：v4
	bytes[8] = (bytes[8] & 0x3f) | 0x80 // 变体位：RFC 4122
	return fmt.Sprintf("%x-%x-%x-%x-%x", bytes[0:4], bytes[4:6], bytes[6:8], bytes[8:10], bytes[10:16]), nil
}

// ValidID 报告字符串是否为合法条目存储标识符（UUID v4）。
func ValidID(id string) bool { return uuidPattern.MatchString(id) }

// validDisplayNameRune 报告显示名字符是否允许：
// ASCII 只允许 RFC 3986 unreserved（[A-Za-z0-9._~-]），非 ASCII 文字允许（URL 编码后无歧义），
// 空格、标点、符号与控制字符一律禁止。
func validDisplayNameRune(r rune) bool {
	if r < 0x80 {
		return r == '-' || r == '.' || r == '_' || r == '~' ||
			(r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
	}
	return unicode.IsLetter(r) || unicode.IsNumber(r) || unicode.IsMark(r)
}

// ValidateName 校验条目显示名：非空且只含 URL 安全字符（ASCII unreserved + 非 ASCII 文字）。
func ValidateName(name string) error {
	if name == "" {
		return errors.New("instance name must not be empty")
	}
	for _, r := range name {
		if !validDisplayNameRune(r) {
			return fmt.Errorf("invalid instance name %q: character %q is not URL-safe", name, r)
		}
	}
	return nil
}

// Validate 校验条目 schema、存储标识、显示名、引擎引用与 SDK 策略。
func Validate(item *File, filename string) error {
	if item.SchemaVersion != SchemaVersion {
		return fmt.Errorf("unsupported instance schema version %d", item.SchemaVersion)
	}
	if !ValidID(item.ID) {
		return fmt.Errorf("invalid instance id %q", item.ID)
	}
	if err := ValidateName(item.Name); err != nil {
		return err
	}
	if filename != "" && strings.TrimSuffix(filepath.Base(filename), ".toml") != item.ID {
		return fmt.Errorf("instance id %q does not match filename", item.ID)
	}
	if !managedversion.ValidEngine(item.Engine.Version) {
		return errors.New("engine version must be MAJOR.MINOR[.PATCH], optionally with a dev/rc/beta/alpha suffix")
	}
	if item.Engine.Edition != "standard" && item.Engine.Edition != "dotnet" {
		return errors.New("engine edition must be standard or dotnet")
	}
	if item.Engine.Edition == "standard" {
		if item.Dotnet != nil {
			return errors.New("standard instance must not contain a dotnet table")
		}
	} else {
		if item.Dotnet == nil {
			return errors.New("dotnet instance requires a dotnet table")
		}
		if item.Dotnet.Strategy == "" {
			item.Dotnet.Strategy = "managed"
		}
		switch item.Dotnet.Strategy {
		case "managed":
			if isGodot3(item.Engine.Version) {
				return errors.New("Godot 3.x dotnet instance requires mono strategy")
			}
			if !managedversion.ValidSDK(item.Dotnet.Version) {
				return errors.New("managed SDK version must be MAJOR.MINOR.PATCH, optionally with a preview/rc suffix")
			}
		case "system":
			if isGodot3(item.Engine.Version) {
				return errors.New("Godot 3.x dotnet instance requires mono strategy")
			}
		case "mono":
			// Godot 3.x mono：依赖系统 Mono 运行时，GoDoIt 不管理其运行时，版本字段无意义。
			if !isGodot3(item.Engine.Version) {
				return errors.New("mono strategy is only valid for Godot 3.x")
			}
		default:
			return errors.New("dotnet strategy must be managed, system or mono")
		}
	}
	if item.Template != nil {
		expected := item.Engine.Version + "-" + item.Engine.Edition
		if item.Template.ID != expected {
			return fmt.Errorf("template id %q must match engine %q", item.Template.ID, expected)
		}
	}
	if err := validateAppearance(item); err != nil {
		return err
	}
	for key, value := range item.Env {
		if err := config.ValidateEnvironmentVariable(key, value); err != nil {
			return fmt.Errorf("invalid environment variable %q: %w", key, err)
		}
	}
	seenEnvironmentKeys := make(map[string]string, len(item.Env))
	for key := range item.Env {
		normalized := config.NormalizeEnvironmentKey(key)
		if previous, exists := seenEnvironmentKeys[normalized]; exists && previous != key {
			return fmt.Errorf("environment keys %q and %q conflict on this platform", previous, key)
		}
		seenEnvironmentKeys[normalized] = key
	}
	return nil
}

func validateAppearance(item *File) error {
	if item.Appearance == nil {
		return nil
	}
	if item.Appearance.Icon == "" {
		item.Appearance.Icon = IconDefault
	}
	switch item.Appearance.Icon {
	case IconDefault, IconGodot, IconCSharp, IconMascot:
		if item.Appearance.CustomIcon != "" {
			return errors.New("custom_icon is only valid for custom icon strategy")
		}
	case IconCustom:
		expected := item.ID + ".png"
		if item.Appearance.CustomIcon != expected {
			return fmt.Errorf("custom_icon %q must be %q", item.Appearance.CustomIcon, expected)
		}
	default:
		return fmt.Errorf("unsupported instance icon strategy %q", item.Appearance.Icon)
	}
	if !ValidIconBackground(item.Appearance.Background) {
		return fmt.Errorf("invalid icon background %q", item.Appearance.Background)
	}
	return nil
}

// ValidIconBackground 报告背景色是否为空（透明）或合法的十六进制 CSS 颜色。
func ValidIconBackground(background string) bool {
	return background == "" || iconBackgroundPattern.MatchString(background)
}

// IconStrategy 返回条目保存的图标策略；旧条目按 default 处理。
func IconStrategy(item File) string {
	if item.Appearance == nil || item.Appearance.Icon == "" {
		return IconDefault
	}
	return item.Appearance.Icon
}

// ResolvedIcon 返回条目实际应展示的内置图标；custom 原样返回，缺失回退由调用方检查文件后处理。
func ResolvedIcon(item File) string {
	strategy := IconStrategy(item)
	if strategy != IconDefault {
		return strategy
	}
	if item.Engine.Edition == "dotnet" {
		return IconCSharp
	}
	return IconGodot
}

// IconBackground 返回条目图标背景色；旧条目和空值按透明处理。
func IconBackground(item File) string {
	if item.Appearance == nil {
		return ""
	}
	return item.Appearance.Background
}

func isGodot3(version string) bool {
	major, _, _ := strings.Cut(version, ".")
	return major == "3"
}

// Path 返回条目文件路径；调用方应先校验 id 为合法 UUID。
func Path(root, id string) string {
	return filepath.Join(root, "instances", id+".toml")
}

// Read 按存储标识符读取并完整校验指定条目及其引擎引用。
func Read(root, id string) (File, error) {
	item, err := readDefinition(root, id)
	if err != nil {
		return File{}, err
	}
	if err := validateEngineReference(root, item); err != nil {
		return File{}, err
	}
	return item, nil
}

// readDefinition 读取并校验条目文件本身，不检查其资产引用是否完整。
func readDefinition(root, id string) (File, error) {
	if !ValidID(id) {
		return File{}, fmt.Errorf("invalid instance id %q", id)
	}
	path := Path(root, id)
	info, err := os.Lstat(path)
	if err != nil {
		return File{}, err
	}
	if !info.Mode().IsRegular() {
		return File{}, errors.New("instance is not a regular file")
	}
	var item File
	if _, err := toml.DecodeFile(path, &item); err != nil {
		return File{}, fmt.Errorf("decode instance: %w", err)
	}
	if err := Validate(&item, path); err != nil {
		return File{}, err
	}
	return item, nil
}

// Lookup 按显示名在完整校验的条目集合中精确查找，未找到时返回 os.ErrNotExist。
// 任意坏条目都会使查找失败（失败关闭），与 Scan 一致。
func Lookup(root, name string) (File, error) {
	if err := ValidateName(name); err != nil {
		return File{}, err
	}
	items, err := Scan(root)
	if err != nil {
		return File{}, err
	}
	for _, item := range items {
		if item.Name == name {
			return item, nil
		}
	}
	return File{}, os.ErrNotExist
}

// ScanDefinitions 扫描并校验全部 *.toml 条目定义，但不检查资产引用是否完整。
// 该入口供 doctor 在引擎或 SDK 缺失时继续收集全部引用故障；普通业务应使用 Scan。
// 显示名重复属于坏条目，会使整个扫描失败。
func ScanDefinitions(root string) ([]File, error) {
	directory := filepath.Join(root, "instances")
	entries, err := os.ReadDir(directory)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read instances directory: %w", err)
	}
	items := make([]File, 0, len(entries))
	names := make(map[string]struct{}, len(entries))
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".toml" {
			continue
		}
		info, infoErr := entry.Info()
		if infoErr != nil || !info.Mode().IsRegular() {
			return nil, fmt.Errorf("instance candidate %q is not a readable regular file", entry.Name())
		}
		id := strings.TrimSuffix(entry.Name(), ".toml")
		item, readErr := readDefinition(root, id)
		if readErr != nil {
			return nil, fmt.Errorf("invalid instance %q: %w", id, readErr)
		}
		if _, exists := names[item.Name]; exists {
			return nil, fmt.Errorf("duplicate instance name %q", item.Name)
		}
		names[item.Name] = struct{}{}
		items = append(items, item)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].Name < items[j].Name })
	return items, nil
}

// Scan 失败关闭地扫描全部 *.toml 条目并校验其引擎引用。
func Scan(root string) ([]File, error) {
	items, err := ScanDefinitions(root)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		if err := validateEngineReference(root, item); err != nil {
			return nil, fmt.Errorf("invalid instance %q: %w", item.ID, err)
		}
	}
	return items, nil
}

func validateEngineReference(root string, item File) error {
	id := item.Engine.Version + "-" + item.Engine.Edition
	records, err := store.New(root).ScanValid()
	if err != nil {
		return fmt.Errorf("scan engine assets: %w", err)
	}
	for _, record := range records {
		if record.Manifest.ID == id {
			return nil
		}
	}
	return fmt.Errorf("engine not installed: %s", id)
}

// BuildReferences 从已经完整校验的条目集合派生资产引用关系。
func BuildReferences(items []File) References {
	refs := References{Engines: make(map[string][]string), SDKs: make(map[string][]string), Templates: make(map[string][]string)}
	for _, item := range items {
		engineID := item.Engine.Version + "-" + item.Engine.Edition
		refs.Engines[engineID] = append(refs.Engines[engineID], item.Name)
		if item.Dotnet != nil && item.Dotnet.Strategy == "managed" {
			refs.SDKs[item.Dotnet.Version] = append(refs.SDKs[item.Dotnet.Version], item.Name)
		}
		if item.Template != nil {
			refs.Templates[item.Template.ID] = append(refs.Templates[item.Template.ID], item.Name)
		}
	}
	return refs
}

// Write 原子创建新条目（item.ID 必须已含合法 UUID）；文件已存在时拒绝覆盖。
// 显示名唯一性由调用方在全局修改锁内检查。
func Write(root string, item File) error {
	if err := Validate(&item, item.ID+".toml"); err != nil {
		return err
	}
	path := Path(root, item.ID)
	if _, err := os.Lstat(path); err == nil {
		return fmt.Errorf("instance already exists: %s", item.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return store.WriteTOMLAtomic(path, item)
}

// SetEnv 按存储标识符原子设置或删除条目环境变量，并保留条目中的未知字段。
func SetEnv(root, id, key, value string, remove bool) error {
	if _, err := Read(root, id); err != nil {
		return err
	}
	var validateErr error
	if remove {
		validateErr = config.ValidateEnvironmentKey(key)
	} else {
		validateErr = config.ValidateEnvironmentVariable(key, value)
	}
	if validateErr != nil {
		return validateErr
	}
	path := Path(root, id)
	content := make(map[string]any)
	if _, err := toml.DecodeFile(path, &content); err != nil {
		return err
	}
	environment, _ := content["env"].(map[string]any)
	if environment == nil {
		environment = make(map[string]any)
	}
	key = config.NormalizeEnvironmentKey(key)
	for existing := range environment {
		if config.NormalizeEnvironmentKey(existing) == key {
			delete(environment, existing)
		}
	}
	if remove {
	} else {
		environment[key] = value
	}
	if len(environment) == 0 {
		delete(content, "env")
	} else {
		content["env"] = environment
	}
	return store.WriteTOMLAtomic(path, content)
}

// SetTemplate 原子设置或移除条目模板引用，并保留未知字段。
func SetTemplate(root, id, templateID string) error {
	item, err := Read(root, id)
	if err != nil {
		return err
	}
	if templateID != "" {
		expected := item.Engine.Version + "-" + item.Engine.Edition
		if templateID != expected {
			return fmt.Errorf("template id %q must match engine %q", templateID, expected)
		}
	}
	path := Path(root, id)
	content := make(map[string]any)
	if _, err := toml.DecodeFile(path, &content); err != nil {
		return err
	}
	if templateID == "" {
		delete(content, "template")
	} else {
		content["template"] = map[string]any{"id": templateID}
	}
	return store.WriteTOMLAtomic(path, content)
}

// SetAppearance 原子设置条目图标策略与背景色，并保留条目中的未知字段。
func SetAppearance(root, id, icon, background string) error {
	item, err := Read(root, id)
	if err != nil {
		return err
	}
	appearance := &Appearance{Icon: icon, Background: background}
	if icon == IconCustom {
		appearance.CustomIcon = id + ".png"
	}
	item.Appearance = appearance
	if err := validateAppearance(&item); err != nil {
		return err
	}
	path := Path(root, id)
	content := make(map[string]any)
	if _, err := toml.DecodeFile(path, &content); err != nil {
		return err
	}
	appearanceMap := make(map[string]any)
	if existing, ok := content["appearance"].(map[string]any); ok {
		for key, value := range existing {
			appearanceMap[key] = value
		}
	}
	appearanceMap["icon"] = appearance.Icon
	delete(appearanceMap, "custom_icon")
	if appearance.CustomIcon != "" {
		appearanceMap["custom_icon"] = appearance.CustomIcon
	}
	if appearance.Background != "" {
		appearanceMap["background"] = appearance.Background
	} else {
		delete(appearanceMap, "background")
	}
	content["appearance"] = appearanceMap
	return store.WriteTOMLAtomic(path, content)
}

// Remove 删除条目文件并同步 instances 目录。
func Remove(root, id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid instance id %q", id)
	}
	if err := os.Remove(Path(root, id)); err != nil {
		return err
	}
	if err := store.SyncDirectory(filepath.Join(root, "instances")); err != nil {
		return err
	}
	icon := filepath.Join(root, "icons", id+".png")
	if err := os.Remove(icon); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove custom icon: %w", err)
	}
	if _, err := os.Stat(filepath.Join(root, "icons")); err == nil {
		return store.SyncDirectory(filepath.Join(root, "icons"))
	}
	return nil
}
