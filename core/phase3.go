package gdit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/Sheyiyuan/GoDoIt/core/internal/archive"
	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/dotnet"
	launchenv "github.com/Sheyiyuan/GoDoIt/core/internal/env"
	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

// ValidateInstanceName 校验条目显示名（URL 安全字符集）。
func ValidateInstanceName(name string) error {
	if err := instance.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	return nil
}

// InstallEntry 安装条目依赖并原子创建条目；整个业务操作只获取一次全局修改锁。
// 条目获得新的 UUID 存储标识；显示名唯一性在锁内扫描校验。
func (m *Manager) InstallEntry(ctx context.Context, request InstallEntryRequest) (InstallEntryResult, error) {
	item, err := m.normalizeEntryRequest(ctx, request)
	if err != nil {
		return InstallEntryResult{}, err
	}
	storeRoot := store.New(m.root)
	if err := storeRoot.Init(); err != nil {
		return InstallEntryResult{}, localIOError("initialize store", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return InstallEntryResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := storeRoot.CleanupOperations(); err != nil {
		return InstallEntryResult{}, localIOError("clean stale operations", err)
	}
	// 安装前先确定 current 状态与 setCurrent 自动规则：损坏的 current 必须在任何
	// 资产发布之前干净失败，避免"资产已发布但报告失败"的误导结果。
	setCurrent := false
	if request.SetCurrent != nil {
		setCurrent = *request.SetCurrent
	} else {
		if _, currentErr := storeRoot.ReadCurrent(); errors.Is(currentErr, store.ErrNoCurrent) {
			setCurrent = true
		} else if currentErr != nil {
			return InstallEntryResult{}, localIOError("read current instance", currentErr)
		}
	}
	// 锁内扫描：显示名唯一（失败关闭），存储标识不冲突。
	items, err := instance.Scan(m.root)
	if err != nil {
		return InstallEntryResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	for _, existing := range items {
		if existing.Name == item.Name {
			return InstallEntryResult{}, fmt.Errorf("%w: instance name %q already exists", ErrInvalidInput, item.Name)
		}
	}
	if _, err := os.Lstat(instance.Path(m.root, item.ID)); err == nil {
		return InstallEntryResult{}, fmt.Errorf("%w: instance already exists: %s", ErrInvalidInput, item.ID)
	} else if !errors.Is(err, os.ErrNotExist) {
		return InstallEntryResult{}, localIOError("inspect instance", err)
	}

	result := InstallEntryResult{}
	engineID := item.Engine.Version + "-" + item.Engine.Edition
	engines, err := storeRoot.ScanValid()
	if err != nil {
		return result, localIOError("scan engine assets", err)
	}
	if findRecord(engines, engineID) == nil {
		installed, installErr := m.installEngine(ctx, InstallRequest{Version: item.Engine.Version, Edition: item.Engine.Edition, Source: request.Source}, true)
		if installErr != nil {
			return result, installErr
		}
		result.Installed = append(result.Installed, AssetChange{Kind: "engine", ID: installed.Version.ID})
		result.StateRebuildRequired = result.StateRebuildRequired || installed.StateRebuildRequired
	}
	if item.Dotnet != nil && item.Dotnet.Strategy == "managed" {
		installed, sdkScanErr := sdkInstalled(storeRoot, item.Dotnet.Version)
		if sdkScanErr != nil {
			return result, localIOError("scan managed SDKs", sdkScanErr)
		}
		if !installed {
			installedSDK, installErr := m.installSDKLocked(ctx, item.Dotnet.Version)
			if installErr != nil {
				return result, installErr
			}
			result.Installed = append(result.Installed, AssetChange{Kind: "sdk", ID: installedSDK.SDK.Version})
			result.StateRebuildRequired = result.StateRebuildRequired || installedSDK.StateRebuildRequired
		}
	}
	if request.Template {
		templateID := item.Engine.Version + "-" + item.Engine.Edition
		templates, scanErr := storeRoot.ScanTemplates()
		if scanErr != nil {
			return result, localIOError("scan templates", scanErr)
		}
		if findTemplateRecord(templates, templateID) == nil {
			installedTemplate, installErr := m.installTemplateLocked(ctx, InstallTemplateRequest{Version: item.Engine.Version, Edition: item.Engine.Edition, Source: request.Source})
			if installErr != nil {
				return result, installErr
			}
			result.Installed = append(result.Installed, AssetChange{Kind: "template", ID: installedTemplate.ID})
		}
		item.Template = &instance.Template{ID: templateID}
	}
	if err := instance.Write(m.root, item); err != nil {
		return result, localIOError("write instance", err)
	}
	result.Instance = instanceToPublic(m.root, item, false)
	if setCurrent {
		if err := storeRoot.SetCurrent(item.ID); err != nil {
			return result, localIOError("set current instance", err)
		}
		result.Instance.Current = true
	}
	return result, nil
}

func (m *Manager) normalizeEntryRequest(ctx context.Context, request InstallEntryRequest) (instance.File, error) {
	if err := instance.ValidateName(request.Name); err != nil {
		return instance.File{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if err := ValidateEngineVersion(request.Version); err != nil {
		return instance.File{}, err
	}
	edition := request.Edition
	if edition == "" {
		edition = "standard"
	}
	item := instance.File{SchemaVersion: instance.SchemaVersion, Name: request.Name, Engine: instance.Engine{Version: request.Version, Edition: edition}}
	id, err := instance.NewID()
	if err != nil {
		return instance.File{}, localIOError("generate instance id", err)
	}
	item.ID = id
	if edition == "standard" {
		if request.SDKStrategy != "" || request.SDKVersion != "" {
			return instance.File{}, fmt.Errorf("%w: SDK options require dotnet edition", ErrInvalidInput)
		}
	} else if edition == "dotnet" {
		if IsGodot3(request.Version) {
			// Godot 3.x mono：运行时由系统 Mono 提供，GoDoIt 不管理；SDK 选项一律拒绝。
			if request.SDKStrategy != "" || request.SDKVersion != "" {
				return instance.File{}, fmt.Errorf("%w: Godot 3.x mono runs on the system Mono runtime and does not accept SDK options", ErrInvalidInput)
			}
			item.Dotnet = &instance.Dotnet{Strategy: "mono"}
		} else {
			strategy := request.SDKStrategy
			if strategy == "" {
				strategy = "managed"
			}
			item.Dotnet = &instance.Dotnet{Strategy: strategy}
			if strategy == "managed" {
				item.Dotnet.Version = request.SDKVersion
			}
			if strategy == "managed" && item.Dotnet.Version == "" {
				major := dotnet.RecommendedMajor(request.Version)
				if major == "" {
					return instance.File{}, fmt.Errorf("%w: no recommended SDK mapping; provide --sdk-version", ErrInvalidInput)
				}
				version, err := dotnet.ResolveLatestPatch(ctx, m.client, major)
				if err != nil {
					return instance.File{}, fmt.Errorf("resolve recommended SDK patch: %w", err)
				}
				item.Dotnet.Version = version
			}
		}
	} else {
		return instance.File{}, fmt.Errorf("%w: edition must be standard or dotnet", ErrInvalidInput)
	}
	if err := instance.Validate(&item, item.ID+".toml"); err != nil {
		return instance.File{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	if item.Dotnet != nil && item.Dotnet.Strategy == "managed" {
		recommended := dotnet.RecommendedMajor(item.Engine.Version)
		if dotnet.BelowRecommendedMajor(item.Dotnet.Version, recommended) {
			m.emit(ProgressEvent{Stage: "warning", Message: fmt.Sprintf("SDK %s is below the recommended %s major", item.Dotnet.Version, recommended)})
		}
	}
	return item, nil
}

// Instances 返回全部条目并标记 current；任意坏条目都会使读取失败。
// current 链接损坏（目标非法）时返回普通错误，不静默降级为"无当前条目"。
func (m *Manager) Instances(ctx context.Context) ([]InstanceInfo, error) {
	items, err := instance.Scan(m.root)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	current, err := store.New(m.root).ReadCurrent()
	if err != nil && !errors.Is(err, store.ErrNoCurrent) {
		return nil, localIOError("read current instance", err)
	}
	result := make([]InstanceInfo, 0, len(items))
	templates, err := store.New(m.root).ScanTemplates()
	if err != nil {
		return nil, localIOError("scan templates", err)
	}
	installedTemplates := make(map[string]bool, len(templates))
	for _, record := range templates {
		installedTemplates[record.Manifest.ID] = true
	}
	for _, item := range items {
		info := instanceToPublic(m.root, item, item.ID == current)
		info.TemplateMissing = info.Template != "" && !installedTemplates[info.Template]
		result = append(result, info)
	}
	return result, nil
}

// RemoveInstance 删除非 current 条目，并返回同一临界区内计算的孤儿快照。
func (m *Manager) RemoveInstance(ctx context.Context, name string) (RemoveInstanceResult, error) {
	if err := instance.ValidateName(name); err != nil {
		return RemoveInstanceResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	storeRoot := store.New(m.root)
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return RemoveInstanceResult{}, localIOError("create store root", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return RemoveInstanceResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if running, sessionErr := m.hasRunningSession(ctx, name); sessionErr != nil {
		return RemoveInstanceResult{}, sessionErr
	} else if running {
		return RemoveInstanceResult{}, fmt.Errorf("%w: %s", ErrInstanceRunning, name)
	}
	items, err := instance.Scan(m.root)
	if err != nil {
		return RemoveInstanceResult{}, fmt.Errorf("%w: cannot determine asset references: %v", ErrInvalidConfig, err)
	}
	var target *instance.File
	remaining := make([]instance.File, 0, len(items))
	for index := range items {
		if items[index].Name == name {
			target = &items[index]
		} else {
			remaining = append(remaining, items[index])
		}
	}
	if target == nil {
		return RemoveInstanceResult{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
	}
	current, currentErr := storeRoot.ReadCurrent()
	if currentErr == nil && current == target.ID {
		return RemoveInstanceResult{}, fmt.Errorf("%w: %s", ErrCurrentInstanceInUse, name)
	}
	if currentErr != nil && !errors.Is(currentErr, store.ErrNoCurrent) {
		return RemoveInstanceResult{}, localIOError("read current instance", currentErr)
	}
	orphans, err := m.orphansFor(remaining)
	if err != nil {
		return RemoveInstanceResult{}, err
	}
	if err := instance.Remove(m.root, target.ID); err != nil {
		return RemoveInstanceResult{}, localIOError("remove instance", err)
	}
	return RemoveInstanceResult{Instance: instanceToPublic(m.root, *target, false), Orphans: orphans}, nil
}

// Orphans 返回当前无条目引用的引擎、托管 SDK 和导出模板资产。
func (m *Manager) Orphans(ctx context.Context) ([]OrphanAsset, error) {
	items, err := instance.Scan(m.root)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot determine asset references: %v", ErrInvalidConfig, err)
	}
	return m.orphansFor(items)
}

func (m *Manager) orphansFor(items []instance.File) ([]OrphanAsset, error) {
	refs := instance.BuildReferences(items)
	storeRoot := store.New(m.root)
	engines, err := storeRoot.ScanValid()
	if err != nil {
		return nil, localIOError("scan engine assets", err)
	}
	sdks, err := storeRoot.ScanSDKs()
	if err != nil {
		return nil, localIOError("scan SDK assets", err)
	}
	templates, err := storeRoot.ScanTemplates()
	if err != nil {
		return nil, localIOError("scan template assets", err)
	}
	result := make([]OrphanAsset, 0)
	for _, record := range engines {
		if len(refs.Engines[record.Manifest.ID]) != 0 {
			continue
		}
		size, sizeErr := store.DirectorySize(record.Dir)
		if sizeErr != nil {
			return nil, localIOError("measure engine asset", sizeErr)
		}
		result = append(result, OrphanAsset{Kind: "engine", ID: record.Manifest.ID, Size: size, Path: record.Dir})
	}
	for _, record := range sdks {
		if len(refs.SDKs[record.Manifest.Version]) != 0 {
			continue
		}
		size, sizeErr := store.DirectorySize(record.Dir)
		if sizeErr != nil {
			return nil, localIOError("measure SDK asset", sizeErr)
		}
		result = append(result, OrphanAsset{Kind: "sdk", ID: record.Manifest.Version, Size: size, Path: record.Dir})
	}
	for _, record := range templates {
		if len(refs.Templates[record.Manifest.ID]) != 0 {
			continue
		}
		size, sizeErr := store.DirectorySize(record.Dir)
		if sizeErr != nil {
			return nil, localIOError("measure template asset", sizeErr)
		}
		result = append(result, OrphanAsset{Kind: "template", ID: record.Manifest.ID, Size: size, Path: record.Dir})
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return result[i].ID < result[j].ID
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

// AutoRemove 在锁内重新扫描引用，只删除复查时仍为孤儿的资产。
func (m *Manager) AutoRemove(ctx context.Context) (AutoRemoveResult, error) {
	storeRoot := store.New(m.root)
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return AutoRemoveResult{}, localIOError("create store root", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return AutoRemoveResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	items, err := instance.Scan(m.root)
	if err != nil {
		return AutoRemoveResult{}, fmt.Errorf("%w: cannot determine asset references: %v", ErrInvalidConfig, err)
	}
	orphans, err := m.orphansFor(items)
	if err != nil {
		return AutoRemoveResult{}, err
	}
	result := AutoRemoveResult{Removed: make([]OrphanAsset, 0, len(orphans))}
	for _, orphan := range orphans {
		if ctx.Err() != nil {
			return result, ctx.Err()
		}
		var removeErr error
		if orphan.Kind == "engine" {
			removeErr = storeRoot.RemoveEngine(orphan.ID)
		} else if orphan.Kind == "sdk" {
			removeErr = storeRoot.RemoveSDK(orphan.ID)
		} else {
			removeErr = storeRoot.RemoveTemplate(orphan.ID)
		}
		if removeErr != nil {
			return result, localIOError("remove orphan asset", removeErr)
		}
		result.Removed = append(result.Removed, orphan)
	}
	engines, engineErr := storeRoot.ScanValid()
	sdks, sdkErr := storeRoot.ScanSDKs()
	if engineErr != nil || sdkErr != nil {
		result.StateRebuildRequired = true
		return result, nil
	}
	if _, err := storeRoot.ReconcileState(engines, sdks); err != nil {
		result.StateRebuildRequired = true
	}
	return result, nil
}

// SDKs 列出托管与系统 SDK，系统探测不访问网络。
func (m *Manager) SDKs(ctx context.Context) ([]SDKInfo, error) {
	managed, err := dotnet.Managed(m.root)
	if err != nil {
		return nil, localIOError("scan managed SDKs", err)
	}
	result := make([]SDKInfo, 0, len(managed))
	for _, item := range managed {
		result = append(result, SDKInfo{Version: item.Version, Kind: item.Kind, Path: item.Path})
	}
	system, err := m.sdkProbe(ctx)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		m.emit(ProgressEvent{Stage: "warning", Message: "system SDK probe failed: " + err.Error()})
	} else {
		result = append(result, system...)
	}
	sort.Slice(result, func(i, j int) bool {
		if result[i].Kind == result[j].Kind {
			return compareVersions(result[i].Version, result[j].Version) > 0
		}
		return result[i].Kind < result[j].Kind
	})
	return result, nil
}

// AvailableSDKs 枚举各 SDK 大版本通道的可用稳定版本，按通道版本倒序分组返回。
// 通道索引不可用时降级内置保底通道列表并发警告；单个通道元数据失败只发警告并跳过该通道，
// 不使整体枚举失败。
func (m *Manager) AvailableSDKs(ctx context.Context) ([]SDKChannel, error) {
	channels, err := dotnet.Channels(ctx, m.client)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		m.emit(ProgressEvent{Stage: "warning", Message: fmt.Sprintf("SDK channel index unavailable, using fallback list: %v", err)})
		channels = dotnet.FallbackChannels
	}
	result := make([]SDKChannel, 0, len(channels))
	for _, channel := range channels {
		versions, err := dotnet.Available(ctx, m.client, channel.MajorMinor, true)
		if err != nil {
			if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
				return nil, err
			}
			m.emit(ProgressEvent{Stage: "warning", Message: fmt.Sprintf("SDK channel %s unavailable: %v", channel.MajorMinor, err)})
			continue
		}
		if len(versions) == 0 {
			continue
		}
		result = append(result, SDKChannel{
			MajorMinor:  channel.MajorMinor,
			Phase:       channel.Phase,
			ReleaseType: channel.ReleaseType,
			Versions:    versions,
		})
	}
	if len(result) == 0 {
		return nil, fmt.Errorf("no SDK metadata available")
	}
	return result, nil
}

// InstallSDK 下载、校验并原子发布精确版本的托管 SDK。
func (m *Manager) InstallSDK(ctx context.Context, version string) (SDKInstallResult, error) {
	if err := ValidateSDKVersion(version); err != nil {
		return SDKInstallResult{}, err
	}
	storeRoot := store.New(m.root)
	if err := storeRoot.Init(); err != nil {
		return SDKInstallResult{}, localIOError("initialize store", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return SDKInstallResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := storeRoot.CleanupOperations(); err != nil {
		return SDKInstallResult{}, localIOError("clean stale operations", err)
	}
	return m.installSDKLocked(ctx, version)
}

func (m *Manager) installSDKLocked(ctx context.Context, version string) (SDKInstallResult, error) {
	storeRoot := store.New(m.root)
	installed, scanErr := sdkInstalled(storeRoot, version)
	if scanErr != nil {
		return SDKInstallResult{}, localIOError("scan managed SDKs", scanErr)
	}
	if installed {
		return SDKInstallResult{}, fmt.Errorf("%w: SDK %s", ErrAlreadyInstalled, version)
	}
	target, err := platform.CurrentTarget()
	if err != nil {
		return SDKInstallResult{}, fmt.Errorf("%w: %v", ErrUnsupportedPlatform, err)
	}
	resolved, err := dotnet.ResolveArtifact(ctx, m.client, version, target)
	if err != nil {
		return SDKInstallResult{}, fmt.Errorf("resolve SDK asset: %w", err)
	}
	if _, err := validateDownloadURL(resolved.URL); err != nil {
		return SDKInstallResult{}, fmt.Errorf("%w: invalid SDK asset URL", ErrInvalidConfig)
	}
	filename := filepath.Base(resolved.Name)
	if filename == "." || filename == string(filepath.Separator) || filename == "" {
		filename = "dotnet-sdk.tar.gz"
	}
	operation, err := os.MkdirTemp(storeRoot.TmpDir(), "operation-")
	if err != nil {
		return SDKInstallResult{}, localIOError("create SDK operation", err)
	}
	defer os.RemoveAll(operation)
	// 资产下载镜像优先（华为云，2026-08 实测可达且路径与官方一致），官方兜底；
	// 摘要校验在下载后统一进行（hash 来自官方元数据，与下载源无关），摘要失败不 fallback。
	mirrorURL, mirrorErr := dotnet.MirrorURL(resolved.URL)
	sources := []Artifact{{Source: "dotnet-official", URL: resolved.URL, Filename: filename, ChecksumAlgorithm: "sha512", Checksum: resolved.Hash}}
	if mirrorErr == nil && mirrorURL != resolved.URL {
		sources = append([]Artifact{{Source: "dotnet-huaweicloud", URL: mirrorURL, Filename: filename, ChecksumAlgorithm: "sha512", Checksum: resolved.Hash}}, sources...)
	}
	download := filepath.Join(operation, filename)
	sourceName := "dotnet-official"
	var downloadErr error
	for _, source := range sources {
		m.emit(ProgressEvent{Stage: "resolve", Version: version + "(sdk)", Source: source.Source, Filename: filename})
		downloadErr = m.download(ctx, source, download, version+"(sdk)")
		if downloadErr == nil {
			sourceName = source.Source
			break
		}
		if errors.Is(downloadErr, context.Canceled) || errors.Is(downloadErr, context.DeadlineExceeded) || !isUnavailable(downloadErr) {
			// 取消、超时与摘要失败等硬错误直接返回，不尝试下一来源。
			return SDKInstallResult{}, downloadErr
		}
	}
	if downloadErr != nil {
		return SDKInstallResult{}, downloadErr
	}
	staging := filepath.Join(operation, "staging")
	if platform.SDKArchiveFormat(target) == "zip" {
		if err := archive.ExtractZip(download, staging); err != nil {
			return SDKInstallResult{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
	} else {
		if err := archive.ExtractTarGz(download, staging); err != nil {
			return SDKInstallResult{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
	}
	launcherName := platform.SDKLauncherName()
	if err := platform.PrepareLauncher(staging, launcherName); err != nil {
		return SDKInstallResult{}, fmt.Errorf("%w: SDK launcher is missing", ErrInvalidArchive)
	}
	manifest := store.SDKManifest{Version: version, TargetOS: target.OS, TargetArch: target.Arch, Source: sourceName, ChecksumAlgorithm: "sha512", Checksum: resolved.Hash, Launcher: launcherName, InstalledAt: m.now().UTC().Format(time.RFC3339)}
	if err := storeRoot.WriteSDKManifest(staging, manifest); err != nil {
		return SDKInstallResult{}, localIOError("write SDK manifest", err)
	}
	result := SDKInstallResult{SDK: SDKInfo{Version: version, Kind: "managed", Path: storeRoot.SDKDir(version)}}
	published, err := storeRoot.PublishSDK(staging, version)
	if err != nil {
		if published {
			result.StateRebuildRequired = true
			return result, nil
		}
		return SDKInstallResult{}, localIOError("publish SDK", err)
	}
	engines, engineErr := storeRoot.ScanValid()
	sdks, sdkErr := storeRoot.ScanSDKs()
	if engineErr != nil || sdkErr != nil {
		result.StateRebuildRequired = true
		return result, nil
	}
	if _, err := storeRoot.ReconcileState(engines, sdks); err != nil {
		result.StateRebuildRequired = true
	}
	m.emit(ProgressEvent{Stage: "complete", Version: version, Source: sourceName, Filename: filename})
	return result, nil
}

// RemoveSDK 删除未被条目引用的托管 SDK。
func (m *Manager) RemoveSDK(ctx context.Context, version string) error {
	if err := ValidateSDKVersion(version); err != nil {
		return err
	}
	storeRoot := store.New(m.root)
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return localIOError("create store root", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	items, err := instance.Scan(m.root)
	if err != nil {
		return fmt.Errorf("%w: cannot determine asset references: %v", ErrInvalidConfig, err)
	}
	if users := instance.BuildReferences(items).SDKs[version]; len(users) > 0 {
		return fmt.Errorf("%w: SDK %s is referenced by %s", ErrAssetInUse, version, strings.Join(users, ", "))
	}
	installed, err := sdkInstalled(storeRoot, version)
	if err != nil {
		return localIOError("scan managed SDKs", err)
	}
	if !installed {
		return fmt.Errorf("%w: SDK %s", ErrNotInstalled, version)
	}
	if err := storeRoot.RemoveSDK(version); err != nil {
		return localIOError("remove SDK", err)
	}
	engines, engineErr := storeRoot.ScanValid()
	sdks, sdkErr := storeRoot.ScanSDKs()
	if engineErr != nil || sdkErr != nil {
		// 扫描失败时不能拿不完整的清单重写索引：发警告并返回，索引留待下次写操作重建。
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: scan failed"})
		return nil
	}
	if _, err := storeRoot.ReconcileState(engines, sdks); err != nil {
		m.emit(ProgressEvent{Stage: "warning", Message: "state index will be rebuilt on the next read: " + err.Error()})
	}
	return nil
}

// EffectiveEnv 返回指定条目（空为 current）的注入增量与引擎参数。
// name 为条目显示名；current 未设置返回 ErrNoDefault，current 损坏返回具体错误。
func (m *Manager) EffectiveEnv(ctx context.Context, name string) (EnvView, error) {
	var item instance.File
	if name == "" {
		id, err := store.New(m.root).ReadCurrent()
		if err != nil {
			if errors.Is(err, store.ErrNoCurrent) {
				return EnvView{}, ErrNoDefault
			}
			return EnvView{}, localIOError("read current link", err)
		}
		item, err = instance.Read(m.root, id)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return EnvView{}, ErrNoDefault
			}
			return EnvView{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
	} else {
		if err := instance.ValidateName(name); err != nil {
			return EnvView{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
		}
		var err error
		item, err = instance.Lookup(m.root, name)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				return EnvView{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
			}
			return EnvView{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
	}
	result, err := m.environmentFor(ctx, item)
	if err != nil {
		return EnvView{}, err
	}
	view := EnvView{Args: result.Args, Vars: make([]EnvVar, 0, len(result.Vars))}
	for _, variable := range result.Vars {
		view.Vars = append(view.Vars, EnvVar{Key: variable.Key, Value: variable.Value, Origin: variable.Origin})
	}
	return view, nil
}

func (m *Manager) environmentFor(ctx context.Context, item instance.File) (launchenv.Result, error) {
	cfg, err := config.Load(m.root)
	if err != nil {
		return launchenv.Result{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	target, err := platform.CurrentTarget()
	if err != nil {
		return launchenv.Result{}, fmt.Errorf("%w: %v", ErrUnsupportedPlatform, err)
	}
	global := cfg.Environment.Global
	platformSection := cfg.Environment.PlatformVars(target.OS)
	managedDir := ""
	// Godot 3.x mono 依赖系统 Mono 运行时，不做 .NET SDK 解析与注入。
	if item.Engine.Edition == "dotnet" && !launchenv.ExplicitDotnetRoot(global, platformSection, item.Env) && item.Dotnet.Strategy != "mono" {
		if item.Dotnet.Strategy == "managed" {
			installed, scanErr := sdkInstalled(store.New(m.root), item.Dotnet.Version)
			if scanErr != nil {
				return launchenv.Result{}, localIOError("scan managed SDKs", scanErr)
			}
			if !installed {
				return launchenv.Result{}, fmt.Errorf("%w: %s; run \"gdit sdk install %s\"", ErrNoCompatibleSDK, item.Dotnet.Version, item.Dotnet.Version)
			}
			managedDir = store.New(m.root).SDKDir(item.Dotnet.Version)
		} else {
			system, probeErr := m.sdkProbe(ctx)
			if probeErr != nil {
				if errors.Is(probeErr, context.Canceled) || errors.Is(probeErr, context.DeadlineExceeded) {
					return launchenv.Result{}, probeErr
				}
				m.emit(ProgressEvent{Stage: "warning", Message: "system SDK probe failed: " + probeErr.Error()})
			}
			if len(system) == 0 {
				return launchenv.Result{}, fmt.Errorf("%w: no system SDK found", ErrNoCompatibleSDK)
			}
			// 选系统 SDK 中最新且合法的版本；非法版本不参与比较，避免 compareVersions 语义失真。
			newest := ""
			for _, sdk := range system {
				if ValidateSDKVersion(sdk.Version) != nil {
					continue
				}
				if newest == "" || compareVersions(sdk.Version, newest) > 0 {
					newest = sdk.Version
				}
			}
			if newest == "" {
				return launchenv.Result{}, fmt.Errorf("%w: no valid system SDK version found", ErrNoCompatibleSDK)
			}
			recommended := dotnet.RecommendedMajor(item.Engine.Version)
			if dotnet.BelowRecommendedMajor(newest, recommended) {
				m.emit(ProgressEvent{Stage: "warning", Message: fmt.Sprintf("system SDK %s is below the recommended %s major", newest, recommended)})
			}
		}
	}
	return launchenv.Build(os.Environ(), global, platformSection, item.Env, target, managedDir), nil
}

// SetEnvVar 设置全局或条目环境变量。
func (m *Manager) SetEnvVar(ctx context.Context, name, key, value string) error {
	return m.changeEnv(ctx, name, key, value, false)
}

// UnsetEnvVar 删除全局或条目环境变量。
func (m *Manager) UnsetEnvVar(ctx context.Context, name, key string) error {
	return m.changeEnv(ctx, name, key, "", true)
}

func (m *Manager) changeEnv(ctx context.Context, name, key, value string, remove bool) error {
	var validateErr error
	if remove {
		validateErr = config.ValidateEnvironmentKey(key)
	} else {
		validateErr = config.ValidateEnvironmentVariable(key, value)
	}
	if validateErr != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, validateErr)
	}
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if name == "" {
		if err := config.SetEnvironmentVariable(m.root, key, value, remove); err != nil {
			return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
		}
		return nil
	}
	if err := instance.ValidateName(name); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	item, err := instance.Lookup(m.root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if err := instance.SetEnv(m.root, item.ID, key, value, remove); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	return nil
}

func instanceToPublic(root string, item instance.File, current bool) InstanceInfo {
	icon := instance.IconStrategy(item)
	resolvedIcon := instance.ResolvedIcon(item)
	iconMissing := false
	if icon == instance.IconCustom {
		if err := instance.InspectIcon(root, item.ID); err != nil {
			resolvedIcon = instance.ResolvedIcon(instance.File{Engine: item.Engine})
			iconMissing = true
		}
	}
	result := InstanceInfo{ID: item.ID, Name: item.Name, Engine: item.Engine.Version + "-" + item.Engine.Edition, Edition: item.Engine.Edition, Current: current, Icon: icon, ResolvedIcon: resolvedIcon, IconMissing: iconMissing, IconBackground: instance.IconBackground(item)}
	if item.Dotnet != nil {
		result.SDKStrategy = item.Dotnet.Strategy
		if item.Dotnet.Strategy == "managed" {
			result.SDK = item.Dotnet.Version
		}
	}
	if item.Template != nil {
		result.Template = item.Template.ID
	}
	return result
}

// sdkInstalled 报告指定版本的托管 SDK 是否已完整安装；扫描失败时返回错误。
func sdkInstalled(storeRoot *store.Store, version string) (bool, error) {
	records, err := storeRoot.ScanSDKs()
	if err != nil {
		return false, err
	}
	for _, record := range records {
		if record.Manifest.Version == version {
			return true, nil
		}
	}
	return false, nil
}
