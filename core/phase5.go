package gdit

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Sheyiyuan/GoDoIt/core/internal/archive"
	"github.com/Sheyiyuan/GoDoIt/core/internal/dotnet"
	"github.com/Sheyiyuan/GoDoIt/core/internal/instance"
	"github.com/Sheyiyuan/GoDoIt/core/internal/lock"
	"github.com/Sheyiyuan/GoDoIt/core/internal/project"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

// Suggest 对显式项目目录做纯只读分析，不访问 gdit 根目录、来源或网络。
func (m *Manager) Suggest(ctx context.Context, dir string) (ProjectSuggestion, error) {
	if strings.TrimSpace(dir) == "" {
		dir = "."
	}
	analysis, err := project.Analyze(ctx, dir)
	if err != nil {
		return ProjectSuggestion{}, err
	}
	result := ProjectSuggestion{
		ProjectDir:   analysis.Dir,
		EngineSeries: analysis.EngineSeries,
		Edition:      analysis.Edition,
		SDKVersion:   analysis.SDKVersion,
		SDKChannel:   analysis.SDKChannel,
		Evidence:     make([]SuggestEvidence, 0, len(analysis.Evidence)),
		Diagnostics:  make([]SuggestDiagnostic, 0, len(analysis.Diagnostics)),
	}
	if result.Edition == "dotnet" && !strings.HasPrefix(result.EngineSeries, "3.") {
		result.SDKStrategy = "managed"
		if result.SDKChannel == "" {
			result.SDKChannel = dotnet.RecommendedMajor(result.EngineSeries)
		}
	} else if strings.HasPrefix(result.EngineSeries, "3.") {
		result.SDKVersion = ""
		result.SDKChannel = ""
	}
	for _, item := range analysis.Evidence {
		result.Evidence = append(result.Evidence, SuggestEvidence{Kind: item.Kind, Path: item.Path, Value: item.Value})
	}
	hasError := false
	for _, item := range analysis.Diagnostics {
		level := SuggestLevel(item.Level)
		result.Diagnostics = append(result.Diagnostics, SuggestDiagnostic{Level: level, Code: item.Code, Path: item.Path, Message: item.Message})
		hasError = hasError || level == SuggestError
	}
	result.Installable = !hasError && result.EngineSeries != ""
	return result, nil
}

// InstallSuggestion 重新分析项目，确定同系列最高稳定 patch，并安装条目及默认模板。
func (m *Manager) InstallSuggestion(ctx context.Context, request InstallSuggestionRequest) (InstallSuggestionResult, error) {
	suggestion, err := m.Suggest(ctx, request.ProjectDir)
	result := InstallSuggestionResult{Suggestion: suggestion}
	if err != nil {
		return result, err
	}
	if !suggestion.Installable {
		return result, fmt.Errorf("%w: project analysis contains errors", ErrInvalidInput)
	}
	if err := instance.ValidateName(request.Name); err != nil {
		return result, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	version, err := m.resolveSuggestedEngine(ctx, suggestion.EngineSeries, suggestion.Edition)
	if err != nil {
		return result, err
	}
	result.EngineVersion = version
	strategy, sdkVersion := request.SDKStrategy, request.SDKVersion
	if strategy == "" {
		strategy = suggestion.SDKStrategy
	}
	if sdkVersion == "" {
		sdkVersion = suggestion.SDKVersion
	}
	if suggestion.Edition == "dotnet" && !IsGodot3(version) && strategy == "managed" && sdkVersion == "" && suggestion.SDKChannel != "" {
		sdkVersion, err = dotnet.ResolveLatestPatch(ctx, m.client, suggestion.SDKChannel)
		if err != nil {
			return result, fmt.Errorf("resolve suggested SDK patch: %w", err)
		}
	}
	includeTemplate := true
	if request.IncludeTemplate != nil {
		includeTemplate = *request.IncludeTemplate
	}
	entry, err := m.InstallEntry(ctx, InstallEntryRequest{
		Name: request.Name, Version: version, Edition: suggestion.Edition,
		SDKStrategy: strategy, SDKVersion: sdkVersion, SetCurrent: request.SetCurrent, Template: includeTemplate,
	})
	result.Entry = entry
	if err != nil {
		return result, err
	}
	if includeTemplate {
		for _, change := range entry.Installed {
			if change.Kind == "template" {
				templates, scanErr := m.Templates(ctx)
				if scanErr != nil {
					return result, scanErr
				}
				for index := range templates {
					if templates[index].ID == change.ID {
						result.Template = &templates[index]
					}
				}
			}
		}
		if result.Template == nil {
			templates, _ := m.Templates(ctx)
			for index := range templates {
				if templates[index].ID == version+"-"+suggestion.Edition {
					result.Template = &templates[index]
				}
			}
		}
	}
	return result, nil
}

func (m *Manager) resolveSuggestedEngine(ctx context.Context, series, edition string) (string, error) {
	records, err := store.New(m.root).ScanValid()
	if err != nil {
		return "", localIOError("scan engine assets", err)
	}
	best := ""
	for _, record := range records {
		manifest := record.Manifest
		if manifest.Edition == edition && stableInSeries(manifest.Version, series) && (best == "" || compareVersions(manifest.Version, best) > 0) {
			best = manifest.Version
		}
	}
	if best != "" {
		return best, nil
	}
	channels, err := m.Available(ctx, "")
	if err != nil {
		return "", fmt.Errorf("resolve stable Godot %s version: %w; use gdit install with an exact version", series, err)
	}
	for _, channel := range channels {
		for _, candidate := range channel.Versions {
			if stableInSeries(candidate.Version, series) && containsString(candidate.Editions, edition) && (best == "" || compareVersions(candidate.Version, best) > 0) {
				best = candidate.Version
			}
		}
	}
	if best == "" {
		return "", fmt.Errorf("no stable Godot %s %s version is available; use gdit install with an exact version", series, edition)
	}
	return best, nil
}

func stableInSeries(version, series string) bool {
	return !strings.Contains(version, "-") && (version == series || strings.HasPrefix(version, series+"."))
}

// Templates 返回全部完整模板并附加条目引用。
func (m *Manager) Templates(ctx context.Context) ([]TemplateInfo, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	records, err := store.New(m.root).ScanTemplates()
	if err != nil {
		return nil, localIOError("scan templates", err)
	}
	items, err := instance.Scan(m.root)
	if err != nil {
		return nil, fmt.Errorf("%w: cannot determine template references: %v", ErrInvalidConfig, err)
	}
	refs := instance.BuildReferences(items)
	result := make([]TemplateInfo, 0, len(records))
	for _, record := range records {
		info, err := templateToPublic(record, refs.Templates[record.Manifest.ID])
		if err != nil {
			return nil, localIOError("measure template asset", err)
		}
		result = append(result, info)
	}
	return result, nil
}

// InstallTemplate 下载、校验并原子发布一个精确 Godot 版本的导出模板。
func (m *Manager) InstallTemplate(ctx context.Context, request InstallTemplateRequest) (TemplateInfo, error) {
	if err := store.New(m.root).Init(); err != nil {
		return TemplateInfo{}, localIOError("initialize store", err)
	}
	guard, err := lock.Acquire(ctx, store.New(m.root).LockPath())
	if err != nil {
		return TemplateInfo{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := store.New(m.root).CleanupOperations(); err != nil {
		return TemplateInfo{}, localIOError("clean stale operations", err)
	}
	return m.installTemplateLocked(ctx, request)
}

func (m *Manager) installTemplateLocked(ctx context.Context, request InstallTemplateRequest) (TemplateInfo, error) {
	version, edition, err := normalizeTemplateRequest(request.Version, request.Edition)
	if err != nil {
		return TemplateInfo{}, err
	}
	assetName, err := managedversion.TemplateAssetName(version, edition)
	if err != nil {
		return TemplateInfo{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	id := version + "-" + edition
	storeRoot := store.New(m.root)
	records, err := storeRoot.ScanTemplates()
	if err != nil {
		return TemplateInfo{}, localIOError("scan templates", err)
	}
	if findTemplateRecord(records, id) != nil {
		return TemplateInfo{}, fmt.Errorf("%w: template %s", ErrAlreadyInstalled, id)
	}
	providers, err := m.installSources()
	if err != nil {
		return TemplateInfo{}, err
	}
	if request.Source != "" {
		if err := m.checkSourceEnabled(request.Source); err != nil {
			return TemplateInfo{}, err
		}
		providers = selectSources(providers, request.Source)
		if len(providers) == 0 {
			return TemplateInfo{}, fmt.Errorf("%w: source %q is not configured", ErrInvalidConfig, request.Source)
		}
	}
	operation, err := os.MkdirTemp(storeRoot.TmpDir(), "operation-")
	if err != nil {
		return TemplateInfo{}, localIOError("create template operation", err)
	}
	defer os.RemoveAll(operation)
	sourceRequest := SourceRequest{Kind: "template", Version: version, Edition: edition, AssetName: assetName}
	var lastUnavailable error
	for _, provider := range providers {
		m.emit(ProgressEvent{Stage: "resolve", Version: id + "(template)", Source: provider.Name(), Filename: assetName})
		artifact, resolveErr := provider.Resolve(ctx, sourceRequest)
		if resolveErr != nil {
			if errors.Is(resolveErr, context.Canceled) || errors.Is(resolveErr, context.DeadlineExceeded) {
				return TemplateInfo{}, resolveErr
			}
			if isUnavailable(resolveErr) {
				lastUnavailable = resolveErr
				continue
			}
			return TemplateInfo{}, fmt.Errorf("%w: %v", ErrInvalidConfig, resolveErr)
		}
		if err := validateArtifact(&artifact, provider.Name(), assetName); err != nil {
			return TemplateInfo{}, err
		}
		download := filepath.Join(operation, artifact.Filename)
		if err := m.download(ctx, artifact, download, id+"(template)"); err != nil {
			var integrity IntegrityError
			if errors.As(err, &integrity) || !isUnavailable(err) {
				return TemplateInfo{}, err
			}
			lastUnavailable = err
			continue
		}
		extracted := filepath.Join(operation, "extracted")
		if err := archive.ExtractZip(download, extracted); err != nil {
			return TemplateInfo{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		entries, err := os.ReadDir(extracted)
		if err != nil || len(entries) != 1 || entries[0].Name() != "templates" || !entries[0].IsDir() {
			return TemplateInfo{}, fmt.Errorf("%w: template archive must contain one top-level templates directory", ErrInvalidArchive)
		}
		staging := filepath.Join(operation, "staging")
		if err := os.Mkdir(staging, operationDirectoryMode); err != nil {
			return TemplateInfo{}, localIOError("create template staging", err)
		}
		if err := os.Rename(filepath.Join(extracted, "templates"), filepath.Join(staging, "payload")); err != nil {
			return TemplateInfo{}, localIOError("prepare template payload", err)
		}
		manifest := store.TemplateManifest{SchemaVersion: store.TemplateSchemaVersion, ID: id, Version: version, Edition: edition, Source: provider.Name(), ArchiveName: assetName, ChecksumAlgorithm: artifact.ChecksumAlgorithm, Checksum: artifact.Checksum, InstalledAt: m.now().UTC().Format("2006-01-02T15:04:05Z07:00")}
		if err := storeRoot.WriteTemplateManifest(staging, manifest); err != nil {
			return TemplateInfo{}, fmt.Errorf("%w: %v", ErrInvalidArchive, err)
		}
		published, err := storeRoot.PublishTemplate(staging, id)
		info, sizeErr := templateToPublic(store.TemplateRecord{Manifest: manifest, Dir: storeRoot.TemplateDir(id)}, nil)
		if err != nil {
			if published {
				return info, nil
			}
			return TemplateInfo{}, localIOError("publish template", err)
		}
		if sizeErr != nil {
			return TemplateInfo{}, localIOError("measure template", sizeErr)
		}
		m.emit(ProgressEvent{Stage: "complete", Version: id + "(template)", Source: provider.Name(), Filename: assetName})
		return info, nil
	}
	if lastUnavailable != nil {
		return TemplateInfo{}, lastUnavailable
	}
	return TemplateInfo{}, ErrNoSources
}

// RemoveTemplate 删除未被任何条目引用的模板资产。
func (m *Manager) RemoveTemplate(ctx context.Context, version, edition string) (TemplateInfo, error) {
	version, edition, err := normalizeTemplateRequest(version, edition)
	if err != nil {
		return TemplateInfo{}, err
	}
	id := version + "-" + edition
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return TemplateInfo{}, localIOError("create store root", err)
	}
	storeRoot := store.New(m.root)
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return TemplateInfo{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	items, err := instance.Scan(m.root)
	if err != nil {
		return TemplateInfo{}, fmt.Errorf("%w: cannot determine template references: %v", ErrInvalidConfig, err)
	}
	users := instance.BuildReferences(items).Templates[id]
	if len(users) > 0 {
		return TemplateInfo{}, fmt.Errorf("%w: template %s is referenced by %s", ErrAssetInUse, id, strings.Join(users, ", "))
	}
	records, err := storeRoot.ScanTemplates()
	if err != nil {
		return TemplateInfo{}, localIOError("scan templates", err)
	}
	record := findTemplateRecord(records, id)
	if record == nil {
		return TemplateInfo{}, fmt.Errorf("%w: template %s", ErrNotInstalled, id)
	}
	info, err := templateToPublic(*record, nil)
	if err != nil {
		return TemplateInfo{}, localIOError("measure template", err)
	}
	if err := storeRoot.RemoveTemplate(id); err != nil {
		return TemplateInfo{}, localIOError("remove template", err)
	}
	return info, nil
}

// AttachTemplate 安装缺失模板并原子绑定到指定条目。
func (m *Manager) AttachTemplate(ctx context.Context, name, source string) (TemplateBindingResult, error) {
	if err := instance.ValidateName(name); err != nil {
		return TemplateBindingResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	storeRoot := store.New(m.root)
	if err := storeRoot.Init(); err != nil {
		return TemplateBindingResult{}, localIOError("initialize store", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return TemplateBindingResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	if err := storeRoot.CleanupOperations(); err != nil {
		return TemplateBindingResult{}, localIOError("clean stale operations", err)
	}
	item, err := instance.Lookup(m.root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TemplateBindingResult{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return TemplateBindingResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	id := item.Engine.Version + "-" + item.Engine.Edition
	result := TemplateBindingResult{}
	records, err := storeRoot.ScanTemplates()
	if err != nil {
		return result, localIOError("scan templates", err)
	}
	record := findTemplateRecord(records, id)
	var info TemplateInfo
	if record == nil {
		info, err = m.installTemplateLocked(ctx, InstallTemplateRequest{Version: item.Engine.Version, Edition: item.Engine.Edition, Source: source})
		if err != nil {
			return result, err
		}
		result.Installed = true
	} else {
		info, err = templateToPublic(*record, nil)
		if err != nil {
			return result, localIOError("measure template", err)
		}
	}
	if item.Template == nil {
		if err := instance.SetTemplate(m.root, item.ID, id); err != nil {
			return result, localIOError("bind template", err)
		}
		item.Template = &instance.Template{ID: id}
	}
	items, err := instance.Scan(m.root)
	if err != nil {
		return result, fmt.Errorf("%w: cannot determine template references: %v", ErrInvalidConfig, err)
	}
	info.References = append([]string(nil), instance.BuildReferences(items).Templates[id]...)
	sort.Strings(info.References)
	result.Instance = instanceToPublic(item, false)
	result.Template = &info
	return result, nil
}

// DetachTemplate 原子解除条目模板绑定并返回新的孤儿快照。
func (m *Manager) DetachTemplate(ctx context.Context, name string) (TemplateBindingResult, error) {
	if err := instance.ValidateName(name); err != nil {
		return TemplateBindingResult{}, fmt.Errorf("%w: %v", ErrInvalidInput, err)
	}
	storeRoot := store.New(m.root)
	if err := os.MkdirAll(m.root, 0o700); err != nil {
		return TemplateBindingResult{}, localIOError("create store root", err)
	}
	guard, err := lock.Acquire(ctx, storeRoot.LockPath())
	if err != nil {
		return TemplateBindingResult{}, contextOrLocalIOError("acquire store lock", err)
	}
	defer guard.Close()
	item, err := instance.Lookup(m.root, name)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return TemplateBindingResult{}, fmt.Errorf("%w: %s", ErrInstanceNotFound, name)
		}
		return TemplateBindingResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	if item.Template != nil {
		if err := instance.SetTemplate(m.root, item.ID, ""); err != nil {
			return TemplateBindingResult{}, localIOError("detach template", err)
		}
		item.Template = nil
	}
	items, err := instance.Scan(m.root)
	if err != nil {
		return TemplateBindingResult{}, fmt.Errorf("%w: %v", ErrInvalidConfig, err)
	}
	orphans, err := m.orphansFor(items)
	if err != nil {
		return TemplateBindingResult{}, err
	}
	return TemplateBindingResult{Instance: instanceToPublic(item, false), Orphans: orphans}, nil
}

func normalizeTemplateRequest(version, edition string) (string, string, error) {
	if !managedversion.ValidEngine(version) {
		return "", "", fmt.Errorf("%w: template version must be an exact Godot release version", ErrInvalidInput)
	}
	if edition == "" {
		edition = "standard"
	}
	if edition == "mono" {
		edition = "dotnet"
	}
	if edition != "standard" && edition != "dotnet" {
		return "", "", fmt.Errorf("%w: template edition must be standard or dotnet", ErrInvalidInput)
	}
	return version, edition, nil
}

func selectSources(sources []Source, name string) []Source {
	result := make([]Source, 0, 1)
	for _, source := range sources {
		if source.Name() == name {
			result = append(result, source)
		}
	}
	return result
}

func findTemplateRecord(records []store.TemplateRecord, id string) *store.TemplateRecord {
	for index := range records {
		if records[index].Manifest.ID == id {
			return &records[index]
		}
	}
	return nil
}

func templateToPublic(record store.TemplateRecord, references []string) (TemplateInfo, error) {
	size, err := store.DirectorySize(record.Dir)
	if err != nil {
		return TemplateInfo{}, err
	}
	sort.Strings(references)
	manifest := record.Manifest
	return TemplateInfo{ID: manifest.ID, Version: manifest.Version, Edition: manifest.Edition, Source: manifest.Source, ChecksumAlgorithm: manifest.ChecksumAlgorithm, Checksum: manifest.Checksum, ArchiveName: manifest.ArchiveName, Path: filepath.Join(record.Dir, "payload"), Size: size, InstalledAt: manifest.InstalledAt, References: references}, nil
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
