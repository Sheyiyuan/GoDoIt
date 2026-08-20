// Package gdit 提供 GoDoIt CLI 和 GUI 共用的条目、引擎、SDK 与启动环境能力。
package gdit

import (
	"context"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"errors"
	"fmt"
	"hash"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/archive"
	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/dotnet"
	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/source"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

const defaultHTTPTimeout = 30 * time.Minute
const operationDirectoryMode = 0o755
const downloadBufferSize = 32 * 1024

// Manager 负责一个 gdit 根目录中的资产、条目和启动环境管理。
type Manager struct {
	root     string
	client   *http.Client
	progress func(ProgressEvent)
	sources  []Source
	now      func() time.Time
	sdkProbe func(context.Context) ([]SDKInfo, error)
}

// DefaultRoot 返回当前用户的 ~/.gdit 路径。
func DefaultRoot() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("resolve user home: %w", err)
	}
	return filepath.Join(home, ".gdit"), nil
}

// New 创建 Manager。创建过程不访问网络，也不会创建用户目录。
func New(options Options) (*Manager, error) {
	if strings.TrimSpace(options.RootDir) == "" {
		return nil, fmt.Errorf("%w: root directory is required", ErrInvalidInput)
	}
	root, err := filepath.Abs(options.RootDir)
	if err != nil {
		return nil, fmt.Errorf("%w: resolve root directory: %v", ErrInvalidInput, err)
	}
	if filepath.Clean(root) == string(filepath.Separator) {
		return nil, fmt.Errorf("%w: root directory must not be the filesystem root", ErrInvalidInput)
	}
	client := options.HTTPClient
	if client == nil {
		client = &http.Client{Timeout: defaultHTTPTimeout}
	}
	manager := &Manager{
		root:     root,
		client:   client,
		progress: options.Progress,
		sources:  append([]Source(nil), options.Sources...),
		now:      time.Now,
	}
	manager.sdkProbe = options.SDKProbe
	if manager.sdkProbe == nil {
		manager.sdkProbe = func(ctx context.Context) ([]SDKInfo, error) {
			items, err := dotnet.ProbeSystem(ctx, "")
			result := make([]SDKInfo, 0, len(items))
			for _, item := range items {
				result = append(result, SDKInfo{Version: item.Version, Kind: item.Kind, Path: item.Path})
			}
			return result, err
		}
	}
	return manager, nil
}

// Install 下载、校验并原子发布一个 Godot 版本。
func (m *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
	return m.installEngine(ctx, request, false)
}

func (m *Manager) installEngine(ctx context.Context, request InstallRequest, lockHeld bool) (InstallResult, error) {
	version, edition, err := normalizeRequest(request)
	if err != nil {
		return InstallResult{}, err
	}
	target, err := platform.CurrentTarget()
	if err != nil {
		return InstallResult{}, fmt.Errorf("%w: %v", ErrUnsupportedPlatform, err)
	}
	assetName, err := platform.AssetName(version, edition, target)
	if err != nil {
		return InstallResult{}, fmt.Errorf("%w: %v", ErrUnsupportedPlatform, err)
	}
	storeRoot := store.New(m.root)
	if err := storeRoot.Init(); err != nil {
		return InstallResult{}, localIOError("initialize store", err)
	}
	var guard *lock.File
	if !lockHeld {
		guard, err = lock.Acquire(ctx, storeRoot.LockPath())
		if err != nil {
			return InstallResult{}, contextOrLocalIOError("acquire store lock", err)
		}
		defer guard.Close()
	}
	if err := storeRoot.CleanupOperations(); err != nil {
		return InstallResult{}, localIOError("clean up stale operation directories", err)
	}

	id := version + "-" + edition
	records, err := storeRoot.ScanValid()
	if err != nil {
		return InstallResult{}, localIOError("scan installed versions", err)
	}
	for _, record := range records {
		if record.Manifest.ID == id {
			return InstallResult{}, fmt.Errorf("%w: %s", ErrAlreadyInstalled, id)
		}
	}

	providers, err := m.installSources()
	if err != nil {
		return InstallResult{}, err
	}
	if request.Source != "" {
		if err := m.checkSourceEnabled(request.Source); err != nil {
			return InstallResult{}, err
		}
		selected := make([]Source, 0, 1)
		for _, provider := range providers {
			if provider.Name() == request.Source {
				selected = append(selected, provider)
			}
		}
		if len(selected) == 0 {
			return InstallResult{}, fmt.Errorf("%w: source %q is not configured", ErrInvalidConfig, request.Source)
		}
		providers = selected
	}
	if len(providers) == 0 {
		return InstallResult{}, ErrNoSources
	}
	operation, err := os.MkdirTemp(storeRoot.TmpDir(), "operation-")
	if err != nil {
		return InstallResult{}, localIOError("create operation directory", err)
	}
	defer os.RemoveAll(operation)

	requestForSource := SourceRequest{
		Version: version,
		Edition: edition,
		Target:  Target{OS: target.OS, Arch: target.Arch},
	}
	var lastUnavailable error
	for _, provider := range providers {
		m.emit(ProgressEvent{Stage: "resolve", Version: id, Source: provider.Name(), Filename: assetName})
		artifact, resolveErr := provider.Resolve(ctx, requestForSource)
		if resolveErr != nil {
			if errors.Is(resolveErr, context.Canceled) || errors.Is(resolveErr, context.DeadlineExceeded) {
				return InstallResult{}, resolveErr
			}
			if isUnavailable(resolveErr) {
				lastUnavailable = resolveErr
				continue
			}
			return InstallResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, resolveErr)
		}
		if err := validateArtifact(&artifact, provider.Name(), assetName); err != nil {
			return InstallResult{}, err
		}
		downloadPath := filepath.Join(operation, artifact.Filename)
		downloadErr := m.download(ctx, artifact, downloadPath, id)
		if downloadErr != nil {
			if errors.Is(downloadErr, context.Canceled) || errors.Is(downloadErr, context.DeadlineExceeded) {
				return InstallResult{}, downloadErr
			}
			var integrity IntegrityError
			if errors.As(downloadErr, &integrity) {
				return InstallResult{}, downloadErr
			}
			if isUnavailable(downloadErr) {
				lastUnavailable = downloadErr
				continue
			}
			return InstallResult{}, downloadErr
		}

		staging := filepath.Join(operation, "staging")
		if err := os.MkdirAll(filepath.Join(staging, "payload"), operationDirectoryMode); err != nil {
			return InstallResult{}, localIOError("create staging directory", err)
		}
		if err := archive.ExtractZip(downloadPath, filepath.Join(staging, "payload")); err != nil {
			return InstallResult{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		launcher, err := platform.FindLauncher(filepath.Join(staging, "payload"), version, edition, target)
		if err != nil {
			return InstallResult{}, fmt.Errorf("%w: validate engine layout: %v", ErrInvalidArchive, err)
		}
		if err := platform.PrepareLauncher(filepath.Join(staging, "payload"), launcher); err != nil {
			return InstallResult{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		manifest := store.Manifest{
			ID:                id,
			Version:           version,
			Edition:           edition,
			TargetOS:          target.OS,
			TargetArch:        target.Arch,
			Source:            provider.Name(),
			ChecksumAlgorithm: artifact.ChecksumAlgorithm,
			Checksum:          artifact.Checksum,
			Launcher:          launcher,
			InstalledAt:       m.now().UTC().Format(time.RFC3339),
		}
		if err := storeRoot.WriteManifest(staging, manifest); err != nil {
			return InstallResult{}, localIOError("write install manifest", err)
		}
		result := InstallResult{Version: manifestToPublic(manifest)}
		published, publishErr := storeRoot.Publish(staging, id)
		if publishErr != nil {
			if errors.Is(publishErr, store.ErrDestinationExists) {
				return InstallResult{}, fmt.Errorf("%w: %s", ErrAlreadyInstalled, id)
			}
			if published {
				result.StateRebuildRequired = true
				m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: publishErr.Error()})
				return result, nil
			}
			return InstallResult{}, localIOError("publish version", publishErr)
		}
		records, err = storeRoot.ScanValid()
		if err != nil {
			result.StateRebuildRequired = true
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: err.Error()})
			return result, nil
		}
		sdkRecords, sdkScanErr := storeRoot.ScanSDKs()
		if sdkScanErr != nil {
			result.StateRebuildRequired = true
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: sdkScanErr.Error()})
			return result, nil
		}
		stateChanged, stateErr := storeRoot.ReconcileState(records, sdkRecords)
		if stateErr != nil {
			result.StateRebuildRequired = true
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: stateErr.Error()})
			return result, nil
		}
		if stateChanged {
			m.emit(ProgressEvent{Stage: "state", Source: provider.Name(), Message: "state index updated"})
		}
		m.emit(ProgressEvent{Stage: "complete", Version: id, Source: provider.Name(), Filename: assetName})
		return result, nil
	}
	if lastUnavailable != nil {
		return InstallResult{}, fmt.Errorf("%w: %v", ErrAllSourcesUnavailable, lastUnavailable)
	}
	return InstallResult{}, ErrAllSourcesUnavailable
}

// List 返回所有完整安装，并在状态索引不一致时重建它。
// 先走免锁快路径：扫描有效版本目录，与 state.toml 一致时直接返回，不被并发安装阻塞；
// 不一致时才取得全局修改锁，锁内二次扫描后原子重建索引。
func (m *Manager) List(ctx context.Context) ([]InstalledVersion, error) {
	storeRoot := store.New(m.root)
	if _, err := os.Stat(m.root); errors.Is(err, os.ErrNotExist) {
		return []InstalledVersion{}, nil
	} else if err != nil {
		return nil, localIOError("inspect store root", err)
	}
	records, err := storeRoot.ScanValid()
	if err != nil {
		return nil, localIOError("scan installed versions", err)
	}
	sdkRecords, err := storeRoot.ScanSDKs()
	if err != nil {
		return nil, localIOError("scan installed SDKs", err)
	}
	if matches, stateErr := storeRoot.StateMatches(records, sdkRecords); stateErr == nil && matches {
		return manifestList(records), nil
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return nil, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	records, err = storeRoot.ScanValid()
	if err != nil {
		return nil, localIOError("scan installed versions", err)
	}
	sdkRecords, err = storeRoot.ScanSDKs()
	if err != nil {
		return nil, localIOError("scan installed SDKs", err)
	}
	if _, err := storeRoot.ReconcileState(records, sdkRecords); err != nil {
		// 读路径不因索引写失败而阻塞用户查看已安装资产：发警告并返回扫描结果，
		// 索引留待下次写操作重建（与 remove 路径的降级语义一致）。
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: " + err.Error()})
	}
	return manifestList(records), nil
}

func manifestList(records []store.Record) []InstalledVersion {
	versions := make([]InstalledVersion, 0, len(records))
	for _, record := range records {
		versions = append(versions, manifestToPublic(record.Manifest))
	}
	return versions
}

// Sources 返回当前配置的来源列表，只读，不落盘不访问网络。
func (m *Manager) Sources(ctx context.Context) ([]SourceInfo, error) {
	entries, err := config.ListSources(m.root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	result := make([]SourceInfo, 0, len(entries))
	for _, entry := range entries {
		result = append(result, SourceInfo{Name: entry.Name, Kind: entry.Kind, Disabled: entry.Disabled})
	}
	return result, nil
}

// SetDefaultSource 把指定来源移到 source_order 首位并原子写回 config.toml。
// 被禁用的来源不能设为默认。
func (m *Manager) SetDefaultSource(ctx context.Context, name string) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: source name is required", ErrInvalidInput)
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := m.checkSourceEnabled(name); err != nil {
		return err
	}
	if err := config.SetSourceOrderFirst(m.root, name); err != nil {
		if errors.Is(err, config.ErrSourceNotConfigured) {
			return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
		return localIOError("update source order", err)
	}
	return nil
}

// SetSourceDisabled 禁用或启用指定来源并原子写回 config.toml。
func (m *Manager) SetSourceDisabled(ctx context.Context, name string, disabled bool) error {
	if strings.TrimSpace(name) == "" {
		return fmt.Errorf("%w: source name is required", ErrInvalidInput)
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := config.SetSourceDisabled(m.root, name, disabled); err != nil {
		if errors.Is(err, config.ErrSourceNotConfigured) {
			return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
		return localIOError("update source state", err)
	}
	return nil
}

// checkSourceEnabled 返回被禁用来源的配置错误；未禁用或来源不存在时返回 nil（存在性由调用方校验）。
func (m *Manager) checkSourceEnabled(name string) error {
	cfg, err := config.Load(m.root)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if config.IsSourceDisabled(cfg, name) {
		return fmt.Errorf("%w: source %q is disabled", ErrInvalidConfig, name)
	}
	return nil
}

// Available 枚举默认或指定来源上当前平台可安装的版本（稳定版与预发布），按系列分组。
// 稳定版按 major 分组成 "4.x"/"3.x"，预发布（dev/rc/beta/alpha）统一归入 "unstable" 组；
// 组间按 major 倒序、unstable 组最后，组内按版本倒序。sourceName 为空时合并所有支持枚举
// 的来源，单个来源失败不影响其余。
func (m *Manager) Available(ctx context.Context, sourceName string) ([]EngineChannel, error) {
	if _, err := platform.CurrentTarget(); err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnsupportedPlatform, err)
	}
	providers, err := m.installSources()
	if err != nil {
		return nil, err
	}
	if name := strings.TrimSpace(sourceName); name != "" {
		if err := m.checkSourceEnabled(name); err != nil {
			return nil, err
		}
		selected := make([]Source, 0, 1)
		for _, provider := range providers {
			if provider.Name() == name {
				selected = append(selected, provider)
			}
		}
		if len(selected) == 0 {
			return nil, fmt.Errorf("%w: source %q is not configured", ErrInvalidConfig, name)
		}
		providers = selected
	}
	type entry struct {
		editions map[string]struct{}
		sources  []string
	}
	merged := make(map[string]*entry)
	var firstFailure error
	for _, provider := range providers {
		lister, ok := provider.(interface {
			ListVersions(context.Context) ([]source.VersionInfo, error)
		})
		if !ok {
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: "source does not support version enumeration"})
			continue
		}
		versions, listErr := lister.ListVersions(ctx)
		if listErr != nil {
			if errors.Is(listErr, context.Canceled) || errors.Is(listErr, context.DeadlineExceeded) {
				return nil, listErr
			}
			if firstFailure == nil {
				firstFailure = listErr
			}
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: listErr.Error()})
			continue
		}
		for _, version := range versions {
			item := merged[version.Version]
			if item == nil {
				item = &entry{editions: make(map[string]struct{})}
				merged[version.Version] = item
			}
			for _, edition := range version.Editions {
				item.editions[edition] = struct{}{}
			}
			item.sources = append(item.sources, provider.Name())
		}
	}
	if len(merged) == 0 {
		if firstFailure != nil {
			return nil, fmt.Errorf("%w: %v", ErrAllSourcesUnavailable, firstFailure)
		}
		return nil, fmt.Errorf("%w: no source supports version enumeration", ErrInvalidConfig)
	}
	type groupEntry struct {
		versions []AvailableVersion
	}
	groups := make(map[string]*groupEntry)
	var groupOrder []string
	for version, item := range merged {
		editions := make([]string, 0, len(item.editions))
		for _, candidate := range []string{"standard", "dotnet"} {
			if _, ok := item.editions[candidate]; ok {
				editions = append(editions, candidate)
			}
		}
		name := engineChannelName(version)
		group := groups[name]
		if group == nil {
			group = &groupEntry{}
			groups[name] = group
			groupOrder = append(groupOrder, name)
		}
		group.versions = append(group.versions, AvailableVersion{Version: version, Editions: editions, Sources: item.sources})
	}
	sort.Slice(groupOrder, func(i, j int) bool { return engineChannelBefore(groupOrder[i], groupOrder[j]) })
	result := make([]EngineChannel, 0, len(groupOrder))
	for _, name := range groupOrder {
		versions := groups[name].versions
		sort.Slice(versions, func(i, j int) bool {
			return compareVersions(versions[i].Version, versions[j].Version) > 0
		})
		result = append(result, EngineChannel{Name: name, Versions: versions})
	}
	return result, nil
}

// engineChannelName 返回版本所属的分组名：稳定版按 major 系列，预发布归 unstable。
func engineChannelName(version string) string {
	if strings.Contains(version, "-") {
		return "unstable"
	}
	return strings.Split(version, ".")[0] + ".x"
}

// engineChannelBefore 报告 left 分组是否排在 right 前：稳定系列按 major 倒序，unstable 最后。
func engineChannelBefore(left, right string) bool {
	if left == "unstable" {
		return false
	}
	if right == "unstable" {
		return true
	}
	lMajor, _ := strconv.Atoi(strings.TrimSuffix(left, ".x"))
	rMajor, _ := strconv.Atoi(strings.TrimSuffix(right, ".x"))
	return lMajor > rMajor
}

// compareVersions 按数字段 + 预发布后缀语义比较版本，返回正数表示 left 更新。
// 数字段缺段按 0 补全；同数字段时稳定版大于任何预发布，同类型预发布按序号比较。
// 输入不要求先通过版本校验：越界段按 0 处理，保证任意字符串都不会 panic。
func compareVersions(left, right string) int {
	leftParts, leftSuffix := splitVersionParts(left)
	rightParts, rightSuffix := splitVersionParts(right)
	for i := 0; i < 3; i++ {
		l, _ := strconv.Atoi(partAt(leftParts, i))
		r, _ := strconv.Atoi(partAt(rightParts, i))
		if l != r {
			if l > r {
				return 1
			}
			return -1
		}
	}
	return compareVersionSuffixes(leftSuffix, rightSuffix)
}

// versionSuffix 描述预发布后缀；Kind 为空表示稳定版。
type versionSuffix struct {
	Kind string // dev / rc / beta / alpha / preview
	Num  int
}

// splitVersionParts 把版本拆成数字段与预发布后缀：4.8-dev3 → [4 8] + dev/3；
// 11.0.100-preview.7.26381.103 → [11 0 100] + preview/7；无后缀时 Kind 为空。
func splitVersionParts(version string) ([]string, versionSuffix) {
	part, suffix, found := strings.Cut(version, "-")
	fields := strings.Split(part, ".")
	if !found {
		return fields, versionSuffix{}
	}
	kind, num := suffix, 0
	if index := strings.IndexFunc(suffix, func(r rune) bool { return r >= '0' && r <= '9' }); index > 0 {
		kind = suffix[:index]
		digits := suffix[index:]
		if dot := strings.Index(digits, "."); dot >= 0 {
			digits = digits[:dot]
		}
		num, _ = strconv.Atoi(digits)
	}
	return fields, versionSuffix{Kind: kind, Num: num}
}

// suffixRank 预发布类型优先级：稳定版最大，预览类型按成熟度递减。
func suffixRank(kind string) int {
	switch kind {
	case "":
		return 5
	case "dev":
		return 4
	case "rc":
		return 3
	case "beta":
		return 2
	case "alpha":
		return 1
	default: // preview（.NET 预览，相当于 alpha 前的预发布）
		return 0
	}
}

// compareVersionSuffixes 比较两个预发布后缀，返回正数表示 left 更新。
func compareVersionSuffixes(left, right versionSuffix) int {
	lRank, rRank := suffixRank(left.Kind), suffixRank(right.Kind)
	if lRank != rRank {
		if lRank > rRank {
			return 1
		}
		return -1
	}
	if left.Num != right.Num {
		if left.Num > right.Num {
			return 1
		}
		return -1
	}
	return 0
}

// partAt 返回版本段切片中第 index 段，越界时返回空串。
func partAt(parts []string, index int) string {
	if index < len(parts) {
		return parts[index]
	}
	return ""
}

// Default 返回 current 指向的完整条目。未设置或悬空时返回 ErrNoDefault；
// current 指向的条目损坏（含引擎引用缺失）时返回具体配置错误。
func (m *Manager) Default(ctx context.Context) (InstanceInfo, error) {
	id, err := store.New(m.root).ReadCurrent()
	if err != nil {
		if errors.Is(err, store.ErrNoCurrent) {
			return InstanceInfo{}, ErrNoDefault
		}
		return InstanceInfo{}, localIOError("read current link", err)
	}
	item, err := instance.Read(m.root, id)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return InstanceInfo{}, ErrNoDefault
		}
		return InstanceInfo{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return instanceToPublic(item, true), nil
}

// SetDefault 原子地把指定显示名条目设为 current。
func (m *Manager) SetDefault(ctx context.Context, name string) error {
	if err := instance.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	item, err := instance.Lookup(m.root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := storeRoot.SetCurrent(item.ID); err != nil {
		return localIOError("set current instance", err)
	}
	return nil
}

// Remove 删除指定引擎资产。任何坏条目或有效引用都会阻止删除。
func (m *Manager) Remove(ctx context.Context, id string) error {
	if !store.ValidID(id) {
		return fmt.Errorf("%w: invalid version id %q", ErrInvalidInput, id)
	}
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	items, err := instance.Scan(m.root)
	if err != nil {
		return fmt.Errorf("%w: cannot determine asset references: %v", ErrInvalidConfig, err)
	}
	if users := instance.BuildReferences(items).Engines[id]; len(users) > 0 {
		return fmt.Errorf("%w: engine %s is referenced by %s", ErrAssetInUse, id, strings.Join(users, ", "))
	}
	records, err := storeRoot.ScanValid()
	if err != nil {
		return localIOError("scan installed versions", err)
	}
	record := findRecord(records, id)
	if record == nil {
		return fmt.Errorf("%w: %s", ErrNotInstalled, id)
	}
	if err := storeRoot.RemoveEngine(id); err != nil {
		return localIOError("remove engine directory", err)
	}
	records, err = storeRoot.ScanValid()
	if err != nil {
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: " + err.Error()})
		return nil
	}
	sdkRecords, err := storeRoot.ScanSDKs()
	if err != nil {
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: " + err.Error()})
		return nil
	}
	if _, err := storeRoot.ReconcileState(records, sdkRecords); err != nil {
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: " + err.Error()})
		return nil
	}
	return nil
}

// Setup 幂等创建或修复 ~/.gdit/bin/godot shim：它是指向 gdit 自身的 symlink。
// 已存在且指向正确时不做任何事；指向错误或缺失时原子修复。不修改 shell 配置
// 或系统 PATH。
func (m *Manager) Setup(ctx context.Context) error {
	if err := os.MkdirAll(m.root, 0o755); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	executable, err := os.Executable()
	if err != nil {
		return localIOError("resolve gdit executable", err)
	}
	if err := storeRoot.EnsureShim(executable); err != nil {
		return localIOError("create godot shim", err)
	}
	return nil
}

// ResolveLaunch 解析条目、引擎、SDK 与最终子进程环境，不访问网络或写盘。
// name 为空取 current 条目；非空为条目显示名。
func (m *Manager) ResolveLaunch(ctx context.Context, name string) (LaunchTarget, error) {
	storeRoot := store.New(m.root)
	var item instance.File
	if name == "" {
		id, err := storeRoot.ReadCurrent()
		if err != nil {
			if errors.Is(err, store.ErrNoCurrent) {
				return LaunchTarget{}, ErrNoDefault
			}
			return LaunchTarget{}, localIOError("read current link", err)
		}
		item, err = instance.Read(m.root, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return LaunchTarget{}, ErrNoDefault
			}
			return LaunchTarget{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
	} else {
		if err := instance.ValidateName(name); err != nil {
			return LaunchTarget{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		var err error
		item, err = instance.Lookup(m.root, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return LaunchTarget{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
			}
			return LaunchTarget{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
	}
	id := item.Engine.Version + "-" + item.Engine.Edition
	records, err := storeRoot.ScanValid()
	if err != nil {
		return LaunchTarget{}, localIOError("scan installed versions", err)
	}
	record := findRecord(records, id)
	if record == nil {
		return LaunchTarget{}, fmt.Errorf("%w: %s", ErrEngineNotInstalled, id)
	}
	environment, err := m.environmentFor(ctx, item)
	if err != nil {
		return LaunchTarget{}, err
	}
	return LaunchTarget{
		ID:         record.Manifest.ID,
		Version:    record.Manifest.Version,
		Edition:    record.Manifest.Edition,
		Executable: filepath.Join(record.Dir, "payload", record.Manifest.Launcher),
		Args:       environment.Args,
		Env:        environment.Full,
	}, nil
}

// findRecord 在扫描结果中查找指定版本，未找到时返回 nil。
func findRecord(records []store.Record, id string) *store.Record {
	for index := range records {
		if records[index].Manifest.ID == id {
			return &records[index]
		}
	}
	return nil
}

func (m *Manager) installSources() ([]Source, error) {
	if len(m.sources) > 0 {
		return append([]Source(nil), m.sources...), nil
	}
	cfg, err := config.Load(m.root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	providers, err := source.ProvidersFromConfig(cfg, m.client)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	result := make([]Source, 0, len(providers))
	for _, provider := range providers {
		if config.IsSourceDisabled(cfg, provider.Name()) {
			continue
		}
		result = append(result, providerAdapter{provider: provider})
	}
	return result, nil
}

type providerAdapter struct{ provider source.Provider }

func (a providerAdapter) Name() string { return a.provider.Name() }

// ListVersions 委托底层 provider 的版本枚举；不支持的来源返回配置错误。
func (a providerAdapter) ListVersions(ctx context.Context) ([]source.VersionInfo, error) {
	lister, ok := a.provider.(source.VersionLister)
	if !ok {
		return nil, fmt.Errorf("source %s does not support version enumeration", a.provider.Name())
	}
	return lister.ListVersions(ctx)
}

func (a providerAdapter) Resolve(ctx context.Context, request SourceRequest) (Artifact, error) {
	asset, err := platform.AssetName(request.Version, request.Edition, platform.Target{OS: request.Target.OS, Arch: request.Target.Arch})
	if err != nil {
		return Artifact{}, err
	}
	result, err := a.provider.Resolve(ctx, source.ResolveRequest{Version: request.Version, Edition: request.Edition, AssetName: asset})
	if err != nil {
		return Artifact{}, err
	}
	return Artifact{
		Source:            result.Source,
		URL:               result.URL,
		Filename:          result.Filename,
		ChecksumAlgorithm: result.ChecksumAlgorithm,
		Checksum:          result.Checksum,
		AuthorizationEnv:  result.AuthorizationEnv,
	}, nil
}

func (m *Manager) download(ctx context.Context, artifact Artifact, destination, versionID string) error {
	parsed, err := validateDownloadURL(artifact.URL)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, parsed, nil)
	if err != nil {
		return fmt.Errorf("create download request: %w", err)
	}
	if artifact.AuthorizationEnv != "" {
		if token := os.Getenv(artifact.AuthorizationEnv); token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	response, err := m.client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return err
		}
		return SourceUnavailableError{Source: artifact.Source, Err: errors.New("download request failed")}
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil {
		return fmt.Errorf("%w: source returned an invalid final URL", ErrInvalidConfig)
	}
	if _, err := validateDownloadURL(response.Request.URL.String()); err != nil {
		return fmt.Errorf("%w: source redirect must use HTTPS", ErrInvalidConfig)
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return SourceUnavailableError{Source: artifact.Source, Err: fmt.Errorf("http status %d", response.StatusCode)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return fmt.Errorf("%w: source %s returned http status %d", ErrInvalidConfig, artifact.Source, response.StatusCode)
	}
	file, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o600)
	if err != nil {
		return localIOError("create download file", err)
	}
	hash, err := checksumHasher(artifact.ChecksumAlgorithm)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	reader := io.TeeReader(response.Body, hash)
	buffer := make([]byte, downloadBufferSize)
	var downloaded int64
	for {
		count, readErr := reader.Read(buffer)
		if count > 0 {
			if _, err := file.Write(buffer[:count]); err != nil {
				_ = file.Close()
				return localIOError("write download file", err)
			}
			downloaded += int64(count)
			m.emit(ProgressEvent{Stage: "download", Version: versionID, Source: artifact.Source, Filename: artifact.Filename, BytesDownloaded: downloaded, TotalBytes: response.ContentLength})
		}
		if readErr != nil {
			if !errors.Is(readErr, io.EOF) {
				_ = file.Close()
				if errors.Is(readErr, context.Canceled) || errors.Is(readErr, context.DeadlineExceeded) {
					return readErr
				}
				return SourceUnavailableError{Source: artifact.Source, Err: errors.New("read download failed")}
			}
			break
		}
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		return localIOError("sync download file", err)
	}
	if err := file.Close(); err != nil {
		return localIOError("close download file", err)
	}
	actual := hex.EncodeToString(hash.Sum(nil))
	if actual != artifact.Checksum {
		return IntegrityError{Source: artifact.Source, Filename: artifact.Filename, Algorithm: artifact.ChecksumAlgorithm, Expected: artifact.Checksum, Actual: actual}
	}
	return nil
}

func normalizeRequest(request InstallRequest) (string, string, error) {
	version := strings.TrimSpace(request.Version)
	if err := validateEngineVersion(version); err != nil {
		return "", "", err
	}
	edition := strings.ToLower(strings.TrimSpace(request.Edition))
	if edition == "" {
		edition = "standard"
	}
	if edition == "mono" {
		edition = "dotnet"
	}
	if edition != "standard" && edition != "dotnet" {
		return "", "", fmt.Errorf("%w: edition must be standard or dotnet", ErrInvalidInput)
	}
	return version, edition, nil
}

// ValidateVersion 校验 Godot 引擎版本语法。
// 保留该名称供现有调用方使用；新代码应优先使用 ValidateEngineVersion。
func ValidateVersion(version string) error {
	return ValidateEngineVersion(version)
}

// ValidateEngineVersion 校验 Godot 的两/三段稳定版或 dev/rc/beta/alpha 预发布版本。
func ValidateEngineVersion(version string) error {
	return validateEngineVersion(strings.TrimSpace(version))
}

// ValidateSDKVersion 校验 .NET SDK 的三段稳定版或 preview/rc 预发布版本。
func ValidateSDKVersion(version string) error {
	version = strings.TrimSpace(version)
	if !managedversion.ValidSDK(version) {
		return fmt.Errorf("%w: SDK version must be MAJOR.MINOR.PATCH, optionally with a preview/rc suffix", ErrInvalidInput)
	}
	return nil
}

// ValidEngineID 报告字符串是否为合法引擎资产 ID（三段数字版本（可带预发布后缀）+ standard/dotnet）。
func ValidEngineID(id string) bool { return store.ValidID(id) }

// IsGodot3 报告版本是否属于 Godot 3.x 系列。3.x 的 dotnet（mono）版依赖系统 Mono
// 运行时而非 .NET SDK，GoDoIt 只负责下载安装，不做 SDK 解析与注入。
func IsGodot3(version string) bool {
	return strings.HasPrefix(version, "3.")
}

func validateEngineVersion(version string) error {
	if !managedversion.ValidEngine(version) {
		return fmt.Errorf("%w: Godot version must be MAJOR.MINOR[.PATCH], optionally with a dev/rc/beta/alpha suffix", ErrInvalidInput)
	}
	return nil
}

// ParseVersionArg 解析 CLI 的版本参数，支持 m 前缀简写（仅小写）：
// "m4.5.2" 等价于 edition 为 dotnet 的 "4.5.2"；无前缀时 edition 为 standard。
// m 前缀与 --edition 同时出现属于用法错误，由 CLI 层检查；m 后不是合法三段
// 版本号时返回与普通版本输入相同的语法错误。
func ParseVersionArg(arg string) (version, edition string, err error) {
	trimmed := strings.TrimSpace(arg)
	if strings.HasPrefix(trimmed, "m") {
		version = trimmed[1:]
		edition = "dotnet"
	} else {
		version = trimmed
		edition = "standard"
	}
	if err := validateEngineVersion(version); err != nil {
		return "", "", err
	}
	return version, edition, nil
}

func validateArtifact(artifact *Artifact, providerName, expectedFilename string) error {
	artifact.Source = providerName
	if artifact.Filename != expectedFilename || filepath.Base(artifact.Filename) != artifact.Filename {
		return fmt.Errorf("%w: source returned unexpected asset name", ErrInvalidConfig)
	}
	if _, err := validateDownloadURL(artifact.URL); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	algorithm := strings.ToLower(strings.TrimSpace(artifact.ChecksumAlgorithm))
	if algorithm != "sha256" && algorithm != "sha512" {
		return fmt.Errorf("%w: checksum algorithm must be sha256 or sha512", ErrInvalidConfig)
	}
	checksum, err := source.ValidateChecksum(algorithm, artifact.Checksum)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	artifact.ChecksumAlgorithm = algorithm
	artifact.Checksum = checksum
	return nil
}

// checksumHasher 返回按摘要算法选择的新 hash 实例。
func checksumHasher(algorithm string) (hash.Hash, error) {
	switch strings.ToLower(strings.TrimSpace(algorithm)) {
	case "sha256":
		return sha256.New(), nil
	case "sha512":
		return sha512.New(), nil
	default:
		return nil, fmt.Errorf("unsupported checksum algorithm %q", algorithm)
	}
}

func validateDownloadURL(rawURL string) (string, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil {
		return "", errors.New("invalid download URL")
	}
	isLocal := parsed.Scheme == "http" && (parsed.Hostname() == "localhost" || parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "::1")
	if parsed.Scheme != "https" && !isLocal {
		return "", errors.New("download URL must use HTTPS")
	}
	return rawURL, nil
}

func isUnavailable(err error) bool {
	var marker interface{ Unavailable() bool }
	return errors.As(err, &marker) && marker.Unavailable()
}

func localIOError(operation string, err error) error {
	return fmt.Errorf("%w: %s: %v", ErrLocalIO, operation, err)
}

func contextOrLocalIOError(operation string, err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return localIOError(operation, err)
}

func (m *Manager) emit(event ProgressEvent) {
	if m.progress != nil {
		m.progress(event)
	}
}

func manifestToPublic(manifest store.Manifest) InstalledVersion {
	return InstalledVersion{
		ID:                manifest.ID,
		Version:           manifest.Version,
		Edition:           manifest.Edition,
		Target:            Target{OS: manifest.TargetOS, Arch: manifest.TargetArch},
		Source:            manifest.Source,
		ChecksumAlgorithm: manifest.ChecksumAlgorithm,
		Checksum:          manifest.Checksum,
		Launcher:          manifest.Launcher,
		InstalledAt:       manifest.InstalledAt,
	}
}
