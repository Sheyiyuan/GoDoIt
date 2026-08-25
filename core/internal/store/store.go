// Package store 负责 ~/.gdit/ 下资产目录、安装标记、current 与状态索引的一致性。
package store

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/BurntSushi/toml"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

const stateSchemaVersion = 1

var instanceIDPattern = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

// ErrDestinationExists 表示目标资产目录已经存在，不能覆盖。
var ErrDestinationExists = errors.New("asset destination already exists")

// ErrNoCurrent 表示 current 指针未设置或已悬空。
var ErrNoCurrent = errors.New("current instance is not set")

// Manifest 是版本目录内的安装完成标记。
type Manifest struct {
	ID                string `toml:"id"`
	Version           string `toml:"version"`
	Edition           string `toml:"edition"`
	TargetOS          string `toml:"target_os"`
	TargetArch        string `toml:"target_arch"`
	Source            string `toml:"source"`
	ChecksumAlgorithm string `toml:"checksum_algorithm"`
	Checksum          string `toml:"checksum"`
	Launcher          string `toml:"launcher"`
	InstalledAt       string `toml:"installed_at"`
}

// Record 是一个完整版本目录及其安装标记。
type Record struct {
	Manifest Manifest
	Dir      string
}

// SDKManifest 是 SDK 目录内的安装完成标记。
type SDKManifest struct {
	Version           string `toml:"version"`
	TargetOS          string `toml:"target_os"`
	TargetArch        string `toml:"target_arch"`
	Source            string `toml:"source"`
	ChecksumAlgorithm string `toml:"checksum_algorithm"`
	Checksum          string `toml:"checksum"`
	Launcher          string `toml:"launcher"`
	InstalledAt       string `toml:"installed_at"`
}

// SDKRecord 是一个完整 SDK 目录及其安装标记。
type SDKRecord struct {
	Manifest SDKManifest
	Dir      string
}

// State 是 state.toml 的内容。
type State struct {
	SchemaVersion int           `toml:"schema_version"`
	Engines       []Manifest    `toml:"engines"`
	SDKs          []SDKManifest `toml:"sdks"`
}

// Store 操作一个 gdit 根目录。
type Store struct {
	Root    string
	syncDir func(string) error
}

// New 创建一个不会立即访问磁盘的 Store。
func New(root string) *Store {
	return &Store{Root: root, syncDir: platform.SyncDir}
}

// Init 创建安装操作需要的目录。
func (s *Store) Init() error {
	if err := os.MkdirAll(s.Root, 0o700); err != nil {
		return fmt.Errorf("create store root: %w", err)
	}
	for _, path := range []string{s.EnginesDir(), s.SDKsDir(), s.InstancesDir(), s.TmpDir()} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create store directory: %w", err)
		}
	}
	return nil
}

// EnginesDir 返回已发布引擎资产目录。
func (s *Store) EnginesDir() string { return filepath.Join(s.Root, "engines") }

// EngineDir 返回指定引擎版本目录。
func (s *Store) EngineDir(id string) string { return filepath.Join(s.EnginesDir(), id) }

// SDKsDir 返回已发布托管 SDK 资产目录。
func (s *Store) SDKsDir() string { return filepath.Join(s.Root, "sdks") }

// InstancesDir 返回启动器条目目录。
func (s *Store) InstancesDir() string { return filepath.Join(s.Root, "instances") }

// TmpDir 返回下载和解压临时目录。
func (s *Store) TmpDir() string { return filepath.Join(s.Root, "tmp") }

// StatePath 返回状态索引路径。
func (s *Store) StatePath() string { return filepath.Join(s.Root, "state.toml") }

// LockPath 返回全局修改锁路径。
func (s *Store) LockPath() string { return filepath.Join(s.Root, ".lock") }

// CurrentPath 返回当前条目 symlink 路径。
func (s *Store) CurrentPath() string { return filepath.Join(s.Root, "current") }

// BinDir 返回用户级命令目录。
func (s *Store) BinDir() string { return filepath.Join(s.Root, "bin") }

// ShimPath 返回 godot shim 路径（平台形态：Unix symlink / Windows godot.cmd）。
func (s *Store) ShimPath() string { return platform.ShimPath(s.Root) }

// ValidID 报告版本 ID 是否符合合法格式（三段数字版本 + standard/dotnet）。
func ValidID(id string) bool { return managedversion.ValidEngineID(id) }

// SDKDir 返回指定托管 SDK 目录。
func (s *Store) SDKDir(version string) string { return filepath.Join(s.SDKsDir(), version) }

// ReadCurrent 读取 current 指针指向的条目 UUID（平台形态见 platform：Unix symlink /
// Windows 重定向文件，契约一致）。
// 指针不存在或目标悬空时返回 ErrNoCurrent；目标位置非法时返回普通错误。
func (s *Store) ReadCurrent() (string, error) {
	target, err := platform.ReadCurrentPointer(s.Root)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoCurrent
		}
		return "", fmt.Errorf("read current pointer: %w", err)
	}
	name, err := platform.ParseCurrentPointer(target)
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(s.Root, target)
	info, err := os.Lstat(resolved)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: current pointer is dangling", ErrNoCurrent)
		}
		return "", fmt.Errorf("inspect current target: %w", err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("current target is not a regular instance file")
	}
	return name, nil
}

// SetCurrent 原子地把 current 指针替换为指向指定条目（相对路径 instances/<uuid>.toml）。
// 平台实现内部已同步父目录，失败时旧指针保持不变。
func (s *Store) SetCurrent(id string) error {
	if !instanceIDPattern.MatchString(id) {
		return fmt.Errorf("invalid instance id %q", id)
	}
	if err := platform.WriteCurrentPointer(s.Root, filepath.Join("instances", id+".toml")); err != nil {
		return fmt.Errorf("set current pointer: %w", err)
	}
	return nil
}

// EnsureShim 幂等创建或修复 godot shim（平台形态：Unix symlink 指向 gdit 可执行文件 /
// Windows godot.cmd 包装）。
func (s *Store) EnsureShim(target string) error {
	return platform.EnsureShim(s.Root, target)
}

// ScanValid 扫描有效安装目录，不读取或写入状态索引。
func (s *Store) ScanValid() ([]Record, error) {
	entries, err := os.ReadDir(s.EnginesDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read versions directory: %w", err)
	}
	records := make([]Record, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		dir := filepath.Join(s.EnginesDir(), entry.Name())
		manifest, err := ReadManifest(filepath.Join(dir, "install.toml"))
		if err != nil {
			continue
		}
		if err := validateManifest(manifest, entry.Name(), dir); err != nil {
			continue
		}
		records = append(records, Record{Manifest: manifest, Dir: dir})
	}
	sort.Slice(records, func(i, j int) bool { return records[i].Manifest.ID < records[j].Manifest.ID })
	return records, nil
}

// ScanSDKs 扫描有效托管 SDK 目录，不读取或写入状态索引。
func (s *Store) ScanSDKs() ([]SDKRecord, error) {
	entries, err := os.ReadDir(s.SDKsDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read SDK directory: %w", err)
	}
	records := make([]SDKRecord, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() || !managedversion.ValidSDK(entry.Name()) {
			continue
		}
		dir := filepath.Join(s.SDKsDir(), entry.Name())
		manifest, readErr := ReadSDKManifest(filepath.Join(dir, "install.toml"))
		if readErr != nil || validateSDKManifest(manifest, entry.Name(), dir) != nil {
			continue
		}
		records = append(records, SDKRecord{Manifest: manifest, Dir: dir})
	}
	sort.Slice(records, func(i, j int) bool {
		return compareSDKVersions(records[i].Manifest.Version, records[j].Manifest.Version) < 0
	})
	return records, nil
}

// CleanupOperations 删除 tmp/ 下遗留的 operation 目录。
// 必须在持有全局修改锁时调用：锁内不存在其他进行中的安装，遗留目录都是中断残留。
func (s *Store) CleanupOperations() error {
	entries, err := os.ReadDir(s.TmpDir())
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read tmp directory: %w", err)
	}
	for _, entry := range entries {
		if !entry.IsDir() || !strings.HasPrefix(entry.Name(), "operation-") {
			continue
		}
		if err := os.RemoveAll(filepath.Join(s.TmpDir(), entry.Name())); err != nil {
			return fmt.Errorf("remove stale operation directory: %w", err)
		}
	}
	return nil
}

// Publish 原子发布一个已经完成解压和标记写入的 staging 目录。
// 返回值 published 表示 rename 已经完成，此时即使目录同步报错也不能把安装误报为未完成。
func (s *Store) Publish(staging, id string) (published bool, err error) {
	if !ValidID(id) || filepath.Base(staging) == "." || !within(s.TmpDir(), staging) {
		return false, errors.New("invalid engine ID or staging directory")
	}
	destination := s.EngineDir(id)
	if _, err := os.Lstat(destination); err == nil {
		return false, ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check destination: %w", err)
	}
	if err := platform.RenameAtomic(staging, destination); err != nil {
		return false, fmt.Errorf("publish version: %w", err)
	}
	if err := s.syncDir(s.EnginesDir()); err != nil {
		return true, fmt.Errorf("sync versions directory: %w", err)
	}
	return true, nil
}

// PublishSDK 原子发布一个已经完成校验的 SDK staging 目录。
func (s *Store) PublishSDK(staging, version string) (published bool, err error) {
	if !managedversion.ValidSDK(version) || filepath.Base(staging) == "." || !within(s.TmpDir(), staging) {
		return false, errors.New("invalid SDK staging directory or version")
	}
	destination := s.SDKDir(version)
	if _, err := os.Lstat(destination); err == nil {
		return false, ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check SDK destination: %w", err)
	}
	if err := platform.RenameAtomic(staging, destination); err != nil {
		return false, fmt.Errorf("publish SDK: %w", err)
	}
	if err := s.syncDir(s.SDKsDir()); err != nil {
		return true, fmt.Errorf("sync SDK directory: %w", err)
	}
	return true, nil
}

// WriteManifest 将安装标记写入 staging 目录。
func (s *Store) WriteManifest(staging string, manifest Manifest) error {
	if !within(s.TmpDir(), staging) {
		return errors.New("manifest staging directory must be inside tmp")
	}
	if err := validateManifest(manifest, manifest.ID, staging); err != nil {
		return fmt.Errorf("validate install manifest: %w", err)
	}
	if err := writeTOML(filepath.Join(staging, "install.toml"), manifest); err != nil {
		return fmt.Errorf("write install manifest: %w", err)
	}
	return nil
}

// WriteSDKManifest 将 SDK 安装标记写入 staging 目录。
func (s *Store) WriteSDKManifest(staging string, manifest SDKManifest) error {
	if !within(s.TmpDir(), staging) {
		return errors.New("SDK manifest staging directory must be inside tmp")
	}
	if err := validateSDKManifest(manifest, manifest.Version, staging); err != nil {
		return fmt.Errorf("validate SDK install manifest: %w", err)
	}
	if err := writeTOML(filepath.Join(staging, "install.toml"), manifest); err != nil {
		return fmt.Errorf("write SDK install manifest: %w", err)
	}
	return nil
}

// ReconcileState 根据有效资产目录重建 state.toml，并返回是否写入过索引。
func (s *Store) ReconcileState(records []Record, sdkRecords []SDKRecord) (bool, error) {
	expected := State{SchemaVersion: stateSchemaVersion, Engines: manifests(records), SDKs: sdkManifests(sdkRecords)}
	actual, err := readState(s.StatePath())
	if err == nil && stateEqual(actual, expected) {
		return false, nil
	}
	if err := writeTOMLAtomic(s.StatePath(), expected); err != nil {
		return false, fmt.Errorf("write state: %w", err)
	}
	return true, nil
}

// StateMatches 返回 state.toml 是否与有效资产目录一致，不写盘。
// 用于读取操作的免锁快路径：一致时可以直接返回，不一致才需要拿锁重建。
func (s *Store) StateMatches(records []Record, sdkRecords []SDKRecord) (bool, error) {
	expected := State{SchemaVersion: stateSchemaVersion, Engines: manifests(records), SDKs: sdkManifests(sdkRecords)}
	actual, err := readState(s.StatePath())
	if err != nil {
		return false, err
	}
	return stateEqual(actual, expected), nil
}

// ReadManifest 读取版本目录内的安装标记。
func ReadManifest(path string) (Manifest, error) {
	var manifest Manifest
	if _, err := toml.DecodeFile(path, &manifest); err != nil {
		return Manifest{}, err
	}
	return manifest, nil
}

// ReadSDKManifest 读取 SDK 目录内的安装标记。
func ReadSDKManifest(path string) (SDKManifest, error) {
	var manifest SDKManifest
	if _, err := toml.DecodeFile(path, &manifest); err != nil {
		return SDKManifest{}, err
	}
	return manifest, nil
}

// InspectEngineDir 检查单个引擎资产目录是否为完整安装，返回原因；nil 表示完整。
// 校验规则与 ScanValid 一致（目录名合法 + install.toml 可解析 + launcher 有效），
// 供 doctor 逐条报告无效目录的原因，不复制规则。
func InspectEngineDir(dir string) error {
	name := filepath.Base(dir)
	if !ValidID(name) {
		return fmt.Errorf("invalid engine directory name %q", name)
	}
	manifest, err := ReadManifest(filepath.Join(dir, "install.toml"))
	if err != nil {
		return fmt.Errorf("read install.toml: %w", err)
	}
	if err := validateManifest(manifest, name, dir); err != nil {
		return fmt.Errorf("invalid install.toml: %w", err)
	}
	return nil
}

// InspectSDKDir 检查单个托管 SDK 目录是否为完整安装，返回原因；nil 表示完整。
func InspectSDKDir(dir string) error {
	version := filepath.Base(dir)
	if !managedversion.ValidSDK(version) {
		return fmt.Errorf("invalid SDK directory name %q", version)
	}
	manifest, err := ReadSDKManifest(filepath.Join(dir, "install.toml"))
	if err != nil {
		return fmt.Errorf("read install.toml: %w", err)
	}
	if err := validateSDKManifest(manifest, version, dir); err != nil {
		return fmt.Errorf("invalid install.toml: %w", err)
	}
	return nil
}

func validateManifest(manifest Manifest, id, dir string) error {
	if manifest.ID != id || !managedversion.ValidEngineID(manifest.ID) || manifest.ID != manifest.Version+"-"+manifest.Edition || manifest.TargetOS == "" || manifest.TargetArch == "" || manifest.Source == "" || manifest.Launcher == "" {
		return errors.New("invalid manifest")
	}
	checksumLength := 64
	switch manifest.ChecksumAlgorithm {
	case "sha512":
		checksumLength = 128
	case "sha256":
	default:
		return errors.New("invalid manifest checksum algorithm")
	}
	if len(manifest.Checksum) != checksumLength {
		return errors.New("invalid manifest checksum")
	}
	if _, err := time.Parse(time.RFC3339, manifest.InstalledAt); err != nil {
		return errors.New("invalid installation time")
	}
	if _, err := hex.DecodeString(manifest.Checksum); err != nil {
		return errors.New("invalid manifest checksum")
	}
	if filepath.IsAbs(manifest.Launcher) || manifest.Launcher == ".." || strings.HasPrefix(manifest.Launcher, ".."+string(filepath.Separator)) {
		return errors.New("invalid launcher path")
	}
	payload := filepath.Join(dir, "payload")
	launcher := filepath.Join(payload, manifest.Launcher)
	if !within(payload, launcher) {
		return errors.New("launcher escapes payload")
	}
	if err := platform.ValidateLauncher(launcher); err != nil {
		return errors.New("launcher is missing")
	}
	return nil
}

func validateSDKManifest(manifest SDKManifest, version, dir string) error {
	if manifest.Version != version || !managedversion.ValidSDK(version) || manifest.TargetOS == "" || manifest.TargetArch == "" || manifest.Source == "" || manifest.Launcher != platform.SDKLauncherName() {
		return errors.New("invalid SDK manifest")
	}
	if manifest.ChecksumAlgorithm != "sha512" || len(manifest.Checksum) != 128 {
		return errors.New("invalid SDK manifest checksum")
	}
	if _, err := hex.DecodeString(manifest.Checksum); err != nil {
		return errors.New("invalid SDK manifest checksum")
	}
	if _, err := time.Parse(time.RFC3339, manifest.InstalledAt); err != nil {
		return errors.New("invalid SDK installation time")
	}
	if err := platform.ValidateLauncher(filepath.Join(dir, manifest.Launcher)); err != nil {
		return errors.New("SDK launcher is missing")
	}
	return nil
}

func manifests(records []Record) []Manifest {
	items := make([]Manifest, 0, len(records))
	for _, record := range records {
		items = append(items, record.Manifest)
	}
	return items
}

func sdkManifests(records []SDKRecord) []SDKManifest {
	items := make([]SDKManifest, 0, len(records))
	for _, record := range records {
		items = append(items, record.Manifest)
	}
	return items
}

func readState(path string) (State, error) {
	var state State
	_, err := toml.DecodeFile(path, &state)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func stateEqual(left, right State) bool {
	if left.SchemaVersion != right.SchemaVersion || len(left.Engines) != len(right.Engines) || len(left.SDKs) != len(right.SDKs) {
		return false
	}
	for index := range left.Engines {
		if left.Engines[index] != right.Engines[index] {
			return false
		}
	}
	for index := range left.SDKs {
		if left.SDKs[index] != right.SDKs[index] {
			return false
		}
	}
	return true
}

// WriteTOMLAtomic 以临时文件、原子替换和父目录同步写入 TOML。
func WriteTOMLAtomic(path string, value any) error { return writeTOMLAtomic(path, value) }

// SyncDirectory 把目录项变更同步到磁盘（平台能力封装）。
func SyncDirectory(path string) error { return platform.SyncDir(path) }

// RemoveEngine 删除指定引擎资产目录。
func (s *Store) RemoveEngine(id string) error {
	if !ValidID(id) {
		return fmt.Errorf("invalid engine ID %q", id)
	}
	if err := os.RemoveAll(s.EngineDir(id)); err != nil {
		return err
	}
	return platform.SyncDir(s.EnginesDir())
}

// RemoveSDK 删除指定托管 SDK 资产目录。
func (s *Store) RemoveSDK(version string) error {
	if !managedversion.ValidSDK(version) {
		return fmt.Errorf("invalid SDK version %q", version)
	}
	if err := os.RemoveAll(s.SDKDir(version)); err != nil {
		return err
	}
	return platform.SyncDir(s.SDKsDir())
}

// DirectorySize 返回目录内普通文件占用的总字节数。
func DirectorySize(path string) (int64, error) {
	var size int64
	err := filepath.WalkDir(path, func(_ string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.Type().IsRegular() {
			info, infoErr := entry.Info()
			if infoErr != nil {
				return infoErr
			}
			size += info.Size()
		}
		return nil
	})
	return size, err
}

func writeTOML(path string, value any) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return err
	}
	encoder := toml.NewEncoder(file)
	if err := encoder.Encode(value); err != nil {
		_ = file.Close()
		return err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return err
	}
	return file.Close()
}

func writeTOMLAtomic(path string, value any) error {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	temporary, err := os.CreateTemp(directory, ".gdit-state-*.tmp")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)
	if err := temporary.Chmod(0o600); err != nil {
		_ = temporary.Close()
		return err
	}
	encoder := toml.NewEncoder(temporary)
	if err := encoder.Encode(value); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := platform.RenameAtomic(temporaryPath, path); err != nil {
		return err
	}
	return platform.SyncDir(directory)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}

// compareSDKVersions 按三段数字语义比较 SDK 版本，返回正数表示 left 更新。
func compareSDKVersions(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < 3; index++ {
		l, _ := strconv.Atoi(leftParts[index])
		r, _ := strconv.Atoi(rightParts[index])
		if l > r {
			return 1
		}
		if l < r {
			return -1
		}
	}
	return 0
}
