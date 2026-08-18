// Package store 负责 ~/.gdit/ 下版本目录、安装标记和状态索引的一致性。
package store

import (
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/BurntSushi/toml"
	"golang.org/x/sys/unix"
)

const stateSchemaVersion = 1

var versionIDPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-(standard|dotnet)$`)

// ErrDestinationExists 表示目标版本目录已经存在，不能覆盖。
var ErrDestinationExists = errors.New("version destination already exists")

// ErrNoCurrent 表示 current symlink 未设置或已悬空。
var ErrNoCurrent = errors.New("current version is not set")

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

// State 是 state.toml 的内容。
type State struct {
	SchemaVersion int        `toml:"schema_version"`
	Versions      []Manifest `toml:"versions"`
}

// Store 操作一个 gdit 根目录。
type Store struct {
	Root    string
	syncDir func(string) error
}

// New 创建一个不会立即访问磁盘的 Store。
func New(root string) *Store {
	return &Store{Root: root, syncDir: syncDirectory}
}

// Init 创建安装操作需要的目录。
func (s *Store) Init() error {
	for _, path := range []string{s.VersionsDir(), s.TmpDir()} {
		if err := os.MkdirAll(path, 0o755); err != nil {
			return fmt.Errorf("create store directory: %w", err)
		}
	}
	return nil
}

// VersionsDir 返回已发布版本目录。
func (s *Store) VersionsDir() string { return filepath.Join(s.Root, "versions") }

// TmpDir 返回下载和解压临时目录。
func (s *Store) TmpDir() string { return filepath.Join(s.Root, "tmp") }

// StatePath 返回状态索引路径。
func (s *Store) StatePath() string { return filepath.Join(s.Root, "state.toml") }

// LockPath 返回全局修改锁路径。
func (s *Store) LockPath() string { return filepath.Join(s.Root, ".lock") }

// CurrentPath 返回默认版本 symlink 路径。
func (s *Store) CurrentPath() string { return filepath.Join(s.Root, "current") }

// BinDir 返回用户级命令目录。
func (s *Store) BinDir() string { return filepath.Join(s.Root, "bin") }

// ShimPath 返回 godot shim 的 symlink 路径。
func (s *Store) ShimPath() string { return filepath.Join(s.BinDir(), "godot") }

// ValidID 报告版本 ID 是否符合合法格式（三段数字版本 + standard/dotnet）。
func ValidID(id string) bool { return versionIDPattern.MatchString(id) }

// VersionDir 返回指定版本目录。
func (s *Store) VersionDir(id string) string { return filepath.Join(s.VersionsDir(), id) }

// ReadCurrent 读取 current symlink 指向的版本 ID。
// 链接不存在或目标悬空时返回 ErrNoCurrent；目标位置非法时返回普通错误。
func (s *Store) ReadCurrent() (string, error) {
	target, err := os.Readlink(s.CurrentPath())
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", ErrNoCurrent
		}
		return "", fmt.Errorf("read current link: %w", err)
	}
	id := filepath.Base(filepath.Clean(target))
	if !versionIDPattern.MatchString(id) {
		return "", fmt.Errorf("current link has invalid target %q", target)
	}
	resolved := target
	if !filepath.IsAbs(resolved) {
		resolved = filepath.Join(s.Root, resolved)
	}
	if _, err := os.Stat(resolved); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return "", fmt.Errorf("%w: current link is dangling", ErrNoCurrent)
		}
		return "", fmt.Errorf("inspect current target: %w", err)
	}
	return id, nil
}

// SetCurrent 原子地把 current symlink 替换为指向指定版本（相对路径 versions/<id>）。
// replaceSymlink 内部已同步父目录，失败时旧链接保持不变。
func (s *Store) SetCurrent(id string) error {
	if !versionIDPattern.MatchString(id) {
		return fmt.Errorf("invalid version id %q", id)
	}
	if err := replaceSymlink(s.CurrentPath(), filepath.Join("versions", id)); err != nil {
		return fmt.Errorf("set current link: %w", err)
	}
	return nil
}

// EnsureShim 幂等创建或修复指向 gdit 可执行文件的 godot shim。
// 已存在且指向目标时不做任何事；指向错误、缺失或是普通文件/空目录时原子重建
// （非空目录占用 shim 路径时报错，不递归删除，避免误删用户数据）。
func (s *Store) EnsureShim(target string) error {
	if err := os.MkdirAll(s.BinDir(), 0o755); err != nil {
		return fmt.Errorf("create bin directory: %w", err)
	}
	info, err := os.Lstat(s.ShimPath())
	switch {
	case err == nil && info.Mode()&os.ModeSymlink != 0:
		existing, readErr := os.Readlink(s.ShimPath())
		if readErr != nil {
			return fmt.Errorf("read shim link: %w", readErr)
		}
		if existing == target {
			return nil
		}
	case err == nil:
		if err := os.Remove(s.ShimPath()); err != nil {
			return fmt.Errorf("remove stale shim: %w", err)
		}
	case errors.Is(err, os.ErrNotExist):
	default:
		return fmt.Errorf("inspect shim: %w", err)
	}
	if err := replaceSymlink(s.ShimPath(), target); err != nil {
		return fmt.Errorf("create shim: %w", err)
	}
	return nil
}

// ScanValid 扫描有效安装目录，不读取或写入状态索引。
func (s *Store) ScanValid() ([]Record, error) {
	entries, err := os.ReadDir(s.VersionsDir())
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
		dir := filepath.Join(s.VersionsDir(), entry.Name())
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
	if filepath.Base(staging) == "." || !within(s.TmpDir(), staging) {
		return false, errors.New("staging directory must be inside tmp")
	}
	destination := s.VersionDir(id)
	if _, err := os.Lstat(destination); err == nil {
		return false, ErrDestinationExists
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("check destination: %w", err)
	}
	if err := os.Rename(staging, destination); err != nil {
		return false, fmt.Errorf("publish version: %w", err)
	}
	if err := s.syncDir(s.VersionsDir()); err != nil {
		return true, fmt.Errorf("sync versions directory: %w", err)
	}
	return true, nil
}

// WriteManifest 将安装标记写入 staging 目录。
func (s *Store) WriteManifest(staging string, manifest Manifest) error {
	if !within(s.TmpDir(), staging) {
		return errors.New("manifest staging directory must be inside tmp")
	}
	if err := writeTOML(filepath.Join(staging, "install.toml"), manifest); err != nil {
		return fmt.Errorf("write install manifest: %w", err)
	}
	return nil
}

// ReconcileState 根据有效版本目录重建 state.toml，并返回是否写入过索引。
func (s *Store) ReconcileState(records []Record) (bool, error) {
	expected := State{SchemaVersion: stateSchemaVersion, Versions: manifests(records)}
	actual, err := readState(s.StatePath())
	if err == nil && stateEqual(actual, expected) {
		return false, nil
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		actual = State{}
	}
	if err := writeTOMLAtomic(s.StatePath(), expected); err != nil {
		return false, fmt.Errorf("write state: %w", err)
	}
	return true, nil
}

// StateMatches 返回 state.toml 是否与有效版本目录一致，不写盘。
// 用于读取操作的免锁快路径：一致时可以直接返回，不一致才需要拿锁重建。
func (s *Store) StateMatches(records []Record) (bool, error) {
	expected := State{SchemaVersion: stateSchemaVersion, Versions: manifests(records)}
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

func validateManifest(manifest Manifest, id, dir string) error {
	if manifest.ID != id || !versionIDPattern.MatchString(manifest.ID) || manifest.ID != manifest.Version+"-"+manifest.Edition || manifest.TargetOS == "" || manifest.TargetArch == "" || manifest.Source == "" || manifest.Launcher == "" {
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
	info, err := os.Lstat(launcher)
	if err != nil || !info.Mode().IsRegular() || info.Mode().Perm()&0o111 == 0 {
		return errors.New("launcher is missing")
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

func readState(path string) (State, error) {
	var state State
	_, err := toml.DecodeFile(path, &state)
	if err != nil {
		return State{}, err
	}
	return state, nil
}

func stateEqual(left, right State) bool {
	if left.SchemaVersion != right.SchemaVersion || len(left.Versions) != len(right.Versions) {
		return false
	}
	for index := range left.Versions {
		if left.Versions[index] != right.Versions[index] {
			return false
		}
	}
	return true
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
	if err := os.Rename(temporaryPath, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func syncDirectory(path string) error {
	fd, err := unix.Open(path, unix.O_RDONLY|unix.O_DIRECTORY, 0)
	if err != nil {
		return err
	}
	defer unix.Close(fd)
	return unix.Fsync(fd)
}

// replaceSymlink 原子地创建或替换 linkPath 处的 symlink：先在目标目录创建临时
// symlink，rename 覆盖后同步父目录。失败时清理临时链接；进程在任意一步崩溃时
// 目标目录会残留 `.gdit-link-*.tmp` 空文件或链接，无害且会被下次操作覆盖。
// CreateTemp 只能创建普通文件，因此先借用它的唯一命名，关闭并删除后再
// 以同名创建 symlink，保证并发下不会撞名。
func replaceSymlink(linkPath, target string) error {
	directory := filepath.Dir(linkPath)
	temporary, err := os.CreateTemp(directory, ".gdit-link-*.tmp")
	if err != nil {
		return fmt.Errorf("create temporary link: %w", err)
	}
	temporaryPath := temporary.Name()
	if err := temporary.Close(); err != nil {
		os.Remove(temporaryPath)
		return fmt.Errorf("close temporary link: %w", err)
	}
	defer os.Remove(temporaryPath)
	if err := os.Remove(temporaryPath); err != nil {
		return fmt.Errorf("prepare temporary link: %w", err)
	}
	if err := os.Symlink(target, temporaryPath); err != nil {
		return fmt.Errorf("create temporary symlink: %w", err)
	}
	if err := os.Rename(temporaryPath, linkPath); err != nil {
		return fmt.Errorf("replace symlink: %w", err)
	}
	return syncDirectory(directory)
}

func within(root, path string) bool {
	rel, err := filepath.Rel(root, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
