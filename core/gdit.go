// Package gdit 提供 GoDoIt CLI 和 GUI 共用的引擎安装与版本查询能力。
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
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/source"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
)

const defaultHTTPTimeout = 30 * time.Minute
const operationDirectoryMode = 0o755
const downloadBufferSize = 32 * 1024

// Manager 负责一个 gdit 根目录中的安装和版本查询。
type Manager struct {
	root     string
	client   *http.Client
	progress func(ProgressEvent)
	sources  []Source
	now      func() time.Time
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
	return &Manager{
		root:     root,
		client:   client,
		progress: options.Progress,
		sources:  append([]Source(nil), options.Sources...),
		now:      time.Now,
	}, nil
}

// Install 下载、校验并原子发布一个 Godot 版本。
func (m *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error) {
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
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return InstallResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
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
		m.emit(ProgressEvent{Stage: "resolve", Source: provider.Name(), Filename: assetName})
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
		downloadErr := m.download(ctx, artifact, downloadPath)
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
		stateChanged, stateErr := storeRoot.ReconcileState(records)
		if stateErr != nil {
			result.StateRebuildRequired = true
			m.emit(ProgressEvent{Stage: "warning", Source: provider.Name(), Message: stateErr.Error()})
			return result, nil
		}
		if stateChanged {
			m.emit(ProgressEvent{Stage: "state", Source: provider.Name(), Message: "state index updated"})
		}
		m.emit(ProgressEvent{Stage: "complete", Source: provider.Name(), Filename: assetName})
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
	if matches, stateErr := storeRoot.StateMatches(records); stateErr == nil && matches {
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
	if _, err := storeRoot.ReconcileState(records); err != nil {
		return nil, localIOError("reconcile state index", err)
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

// Available 枚举默认或指定来源上当前平台可安装的稳定版本。
// sourceName 为空时合并所有支持枚举的来源，单个来源失败不影响其余。
func (m *Manager) Available(ctx context.Context, sourceName string) ([]AvailableVersion, error) {
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
	result := make([]AvailableVersion, 0, len(merged))
	for version, item := range merged {
		editions := make([]string, 0, len(item.editions))
		for _, candidate := range []string{"standard", "dotnet"} {
			if _, ok := item.editions[candidate]; ok {
				editions = append(editions, candidate)
			}
		}
		result = append(result, AvailableVersion{Version: version, Editions: editions, Sources: item.sources})
	}
	sort.Slice(result, func(i, j int) bool {
		return compareVersions(result[i].Version, result[j].Version) > 0
	})
	return result, nil
}

// compareVersions 按三段数字语义比较版本，返回正数表示 left 更新。
func compareVersions(left, right string) int {
	leftParts := strings.Split(left, ".")
	rightParts := strings.Split(right, ".")
	for i := 0; i < 3; i++ {
		l, _ := strconv.Atoi(leftParts[i])
		r, _ := strconv.Atoi(rightParts[i])
		if l != r {
			if l > r {
				return 1
			}
			return -1
		}
	}
	return 0
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

func (m *Manager) download(ctx context.Context, artifact Artifact, destination string) error {
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
			m.emit(ProgressEvent{Stage: "download", Source: artifact.Source, Filename: artifact.Filename, BytesDownloaded: downloaded, TotalBytes: response.ContentLength})
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
	parts := strings.Split(version, ".")
	if len(parts) != 3 || version == "" {
		return "", "", fmt.Errorf("%w: version must be MAJOR.MINOR.PATCH", ErrInvalidInput)
	}
	for _, part := range parts {
		if part == "" {
			return "", "", fmt.Errorf("%w: version must contain three numeric fields", ErrInvalidInput)
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return "", "", fmt.Errorf("%w: version must contain only digits", ErrInvalidInput)
			}
		}
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
