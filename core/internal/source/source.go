// Package source 负责把来源配置解析为带摘要的下载资产。
package source

import (
	"bufio"
	"context"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

const (
	maxChecksumResponseBytes = 1024 * 1024
	maxMetadataResponseBytes = 32 * 1024 * 1024
)

var (
	sha256Pattern = regexp.MustCompile(`(?i)^[0-9a-f]{64}$`)
	sha512Pattern = regexp.MustCompile(`(?i)^[0-9a-f]{128}$`)
	// stableTagPattern 只接受三段的稳定版 tag，过滤 rc/beta/dev 和非三段 tag（如 4.7-stable）。
	stableTagPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+-stable$`)
)

// VersionInfo 是来源可用的稳定版本条目。
type VersionInfo struct {
	Version  string   // 如 4.5.2
	Editions []string // 按当前平台资产判断，如 standard、dotnet
}

// VersionLister 是可枚举可用版本的来源；URL 模板型自定义源不实现该接口。
type VersionLister interface {
	ListVersions(ctx context.Context) ([]VersionInfo, error)
}

// ResolveRequest 是 provider 内部使用的资产解析请求。
type ResolveRequest struct {
	Version   string
	Edition   string
	AssetName string
}

// Artifact 是 provider 内部返回的下载资产。
type Artifact struct {
	Source            string
	URL               string
	Filename          string
	ChecksumAlgorithm string
	Checksum          string
	AuthorizationEnv  string
}

// Provider 是可参与 fallback 的内部来源。
type Provider interface {
	Name() string
	Resolve(context.Context, ResolveRequest) (Artifact, error)
}

// UnavailableError 表示来源暂时不可用，调用方可以尝试下一来源。
type UnavailableError struct {
	Err error
}

func (e UnavailableError) Error() string     { return fmt.Sprintf("source unavailable: %v", e.Err) }
func (e UnavailableError) Unwrap() error     { return e.Err }
func (e UnavailableError) Unavailable() bool { return true }

// ConfigError 表示来源配置不完整或摘要清单无法解析。
type ConfigError struct {
	Err error
}

func (e ConfigError) Error() string { return fmt.Sprintf("source config: %v", e.Err) }
func (e ConfigError) Unwrap() error { return e.Err }
func (e ConfigError) Config() bool  { return true }

// HTTPProvider 根据 URL 模板下载并解析来源提供的 SHA-256 或 SHA-512 清单。
// ReleasesURL 非空时指向 GitHub Release API 兼容的 JSON（tag_name + assets[].name），
// 该来源因此支持版本枚举；自定义源不设置该字段。
type HTTPProvider struct {
	SourceName        string
	ArtifactTemplate  string
	ChecksumTemplate  string
	ChecksumAlgorithm string
	AuthorizationEnv  string
	ReleasesURL       string
	Client            *http.Client
}

// Name 返回来源名称。
func (p HTTPProvider) Name() string { return p.SourceName }

// Resolve 解析下载 URL，并从同一来源取得预期摘要。
func (p HTTPProvider) Resolve(ctx context.Context, request ResolveRequest) (Artifact, error) {
	algorithm := strings.ToLower(strings.TrimSpace(p.ChecksumAlgorithm))
	if algorithm == "" {
		algorithm = "sha256"
	}
	if algorithm != "sha256" && algorithm != "sha512" {
		return Artifact{}, ConfigError{Err: fmt.Errorf("unsupported checksum algorithm %q", p.ChecksumAlgorithm)}
	}
	artifactURL, err := expandURL(p.ArtifactTemplate, request.Version, request.AssetName)
	if err != nil {
		return Artifact{}, ConfigError{Err: err}
	}
	checksumURL, err := expandURL(p.ChecksumTemplate, request.Version, request.AssetName)
	if err != nil {
		return Artifact{}, ConfigError{Err: err}
	}
	checksumBody, err := p.get(ctx, checksumURL)
	if err != nil {
		return Artifact{}, err
	}
	checksum, err := findChecksum(checksumBody, request.AssetName, algorithm)
	if err != nil {
		return Artifact{}, ConfigError{Err: err}
	}
	return Artifact{
		Source:            p.SourceName,
		URL:               artifactURL,
		Filename:          request.AssetName,
		ChecksumAlgorithm: algorithm,
		Checksum:          checksum,
		AuthorizationEnv:  p.AuthorizationEnv,
	}, nil
}

func (p HTTPProvider) get(ctx context.Context, rawURL string) ([]byte, error) {
	return fetchBody(ctx, p.Client, rawURL, p.AuthorizationEnv, maxChecksumResponseBytes)
}

// ListVersions 枚举来源元数据中当前平台可安装的稳定版本。
// 未配置 ReleasesURL 的来源不支持枚举，返回配置错误。
func (p HTTPProvider) ListVersions(ctx context.Context) ([]VersionInfo, error) {
	if strings.TrimSpace(p.ReleasesURL) == "" {
		return nil, ConfigError{Err: errors.New("source does not support version enumeration")}
	}
	body, err := fetchBody(ctx, p.Client, p.ReleasesURL, p.AuthorizationEnv, maxMetadataResponseBytes)
	if err != nil {
		return nil, err
	}
	return listVersionsFromReleases(body)
}

type releaseInfo struct {
	Tag    string
	Assets []string
}

// parseReleases 解析 GitHub Release API 兼容的 JSON 结构。
func parseReleases(body []byte) ([]releaseInfo, error) {
	var releases []struct {
		TagName string `json:"tag_name"`
		Assets  []struct {
			Name string `json:"name"`
		} `json:"assets"`
	}
	if err := json.Unmarshal(body, &releases); err != nil {
		return nil, errors.New("metadata is not a valid release list")
	}
	result := make([]releaseInfo, 0, len(releases))
	for _, release := range releases {
		assets := make([]string, 0, len(release.Assets))
		for _, asset := range release.Assets {
			assets = append(assets, asset.Name)
		}
		result = append(result, releaseInfo{Tag: release.TagName, Assets: assets})
	}
	return result, nil
}

// listVersionsFromReleases 过滤稳定 tag，并按当前平台资产判断每个版本的 edition。
func listVersionsFromReleases(body []byte) ([]VersionInfo, error) {
	releases, err := parseReleases(body)
	if err != nil {
		return nil, ConfigError{Err: err}
	}
	var versions []VersionInfo
	for _, release := range releases {
		if !stableTagPattern.MatchString(release.Tag) {
			continue
		}
		version := strings.TrimSuffix(release.Tag, "-stable")
		editions, editionErr := editionsForAssets(version, release.Assets)
		if editionErr != nil {
			return nil, editionErr
		}
		if len(editions) == 0 {
			continue
		}
		versions = append(versions, VersionInfo{Version: version, Editions: editions})
	}
	return versions, nil
}

// editionsForAssets 按当前平台 AssetName 生成候选资产名，判断 release 提供哪些 edition。
// 没有任何当前平台资产（如 Godot 3.x 的 x11 命名）时返回空列表。
func editionsForAssets(version string, assets []string) ([]string, error) {
	target, err := platform.CurrentTarget()
	if err != nil {
		return nil, err
	}
	var editions []string
	for _, edition := range []string{"standard", "dotnet"} {
		name, nameErr := platform.AssetName(version, edition, target)
		if nameErr != nil {
			continue
		}
		for _, asset := range assets {
			if asset == name {
				editions = append(editions, edition)
				break
			}
		}
	}
	return editions, nil
}

// fetchBody 下载并限长读取来源响应，按状态码和错误分类为不可用或配置错误。
func fetchBody(ctx context.Context, client *http.Client, rawURL, authorizationEnv string, maxBytes int64) ([]byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || (parsed.Scheme != "https" && !isLocalHTTP(parsed)) {
		return nil, ConfigError{Err: errors.New("source URL must use HTTPS")}
	}
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, ConfigError{Err: errors.New("invalid source URL")}
	}
	if authorizationEnv != "" {
		if token := os.Getenv(authorizationEnv); token != "" {
			request.Header.Set("Authorization", "Bearer "+token)
		}
	}
	response, err := client.Do(request)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return nil, err
		}
		return nil, UnavailableError{Err: errors.New("request failed")}
	}
	defer response.Body.Close()
	if response.Request == nil || response.Request.URL == nil || !isAllowedURL(response.Request.URL) {
		return nil, ConfigError{Err: errors.New("source redirect must use HTTPS")}
	}
	if response.StatusCode == http.StatusNotFound || response.StatusCode == http.StatusRequestTimeout || response.StatusCode == http.StatusTooManyRequests || response.StatusCode >= 500 {
		return nil, UnavailableError{Err: fmt.Errorf("http status %d", response.StatusCode)}
	}
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, ConfigError{Err: fmt.Errorf("http status %d", response.StatusCode)}
	}
	body, err := io.ReadAll(io.LimitReader(response.Body, maxBytes+1))
	if err != nil {
		return nil, UnavailableError{Err: errors.New("read response failed")}
	}
	if int64(len(body)) > maxBytes {
		return nil, ConfigError{Err: errors.New("response is too large")}
	}
	return body, nil
}

// 内置来源的 URL 规则（2026-08-17 实测确认，见 docs/architecture/README.md 4.6 节）。
const (
	githubArtifactTemplate = "https://github.com/godotengine/godot-builds/releases/download/{tag}/{asset}"
	githubChecksumTemplate = "https://github.com/godotengine/godot-builds/releases/download/{tag}/SHA512-SUMS.txt"
	// githubReleasesURL 是版本枚举端点，实测返回 HTTP 200，单页 100 条（分页后续扩展）。
	githubReleasesURL = "https://api.github.com/repos/godotengine/godot-builds/releases?per_page=100"

	godotHubMetadataURL      = "https://legacy.godothub.com/api/releases.json"
	godotHubArtifactTemplate = "https://atomgit.com/godothub/godot/releases/download/{tag}/{asset}"
)

// ProvidersFromConfig creates providers in the configured fallback order.
func ProvidersFromConfig(cfg config.File, client *http.Client) ([]Provider, error) {
	custom := make(map[string]config.CustomSource, len(cfg.CustomSources))
	for _, item := range cfg.CustomSources {
		custom[item.Name] = item
	}
	providers := make([]Provider, 0, len(cfg.SourceOrder))
	for _, name := range cfg.SourceOrder {
		switch name {
		case "godothub":
			providers = append(providers, GodotHubProvider{
				SourceName:       name,
				MetadataURL:      godotHubMetadataURL,
				ArtifactTemplate: godotHubArtifactTemplate,
				Client:           client,
			})
		case "github":
			providers = append(providers, HTTPProvider{
				SourceName:        name,
				ArtifactTemplate:  githubArtifactTemplate,
				ChecksumTemplate:  githubChecksumTemplate,
				ChecksumAlgorithm: "sha512",
				ReleasesURL:       githubReleasesURL,
				Client:            client,
			})
		default:
			item, ok := custom[name]
			if !ok {
				return nil, ConfigError{Err: fmt.Errorf("built-in source %q has no URL configuration yet", name)}
			}
			providers = append(providers, HTTPProvider{
				SourceName:       item.Name,
				ArtifactTemplate: item.ArtifactURL,
				ChecksumTemplate: item.ChecksumURL,
				AuthorizationEnv: item.AuthorizationEnv,
				Client:           client,
			})
		}
	}
	return providers, nil
}

func expandURL(template, version, asset string) (string, error) {
	replacements := strings.NewReplacer("{version}", version, "{tag}", version+"-stable", "{asset}", asset)
	rawURL := replacements.Replace(template)
	if strings.ContainsAny(rawURL, "{}") {
		return "", errors.New("source URL contains an unsupported template placeholder")
	}
	parsed, err := url.Parse(rawURL)
	if err != nil || parsed.Host == "" || parsed.User != nil || !isAllowedURL(parsed) {
		return "", errors.New("source URL must use HTTPS and must not contain credentials")
	}
	return rawURL, nil
}

func isLocalHTTP(parsed *url.URL) bool {
	return parsed.Scheme == "http" && (parsed.Hostname() == "127.0.0.1" || parsed.Hostname() == "localhost" || parsed.Hostname() == "::1")
}

func isAllowedURL(parsed *url.URL) bool {
	return parsed.Scheme == "https" || isLocalHTTP(parsed)
}

func checksumPattern(algorithm string) *regexp.Regexp {
	if algorithm == "sha512" {
		return sha512Pattern
	}
	return sha256Pattern
}

func findChecksum(body []byte, asset, algorithm string) (string, error) {
	pattern := checksumPattern(algorithm)
	var bareChecksum string
	scanner := bufio.NewScanner(strings.NewReader(string(body)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		fields := strings.Fields(line)
		if len(fields) == 1 && pattern.MatchString(fields[0]) {
			if bareChecksum != "" {
				return "", errors.New("checksum response contains multiple bare checksum values")
			}
			bareChecksum = strings.ToLower(fields[0])
			continue
		}
		var checksum string
		matchesAsset := false
		for _, field := range fields {
			switch {
			case pattern.MatchString(field):
				checksum = strings.ToLower(field)
			case strings.TrimPrefix(field, "*") == asset:
				matchesAsset = true
			}
		}
		if matchesAsset && checksum != "" {
			return checksum, nil
		}
	}
	if err := scanner.Err(); err != nil {
		return "", err
	}
	if bareChecksum != "" {
		return bareChecksum, nil
	}
	return "", errors.New("checksum entry not found for asset")
}

// ValidateChecksum 按算法校验并规范化来源提供的摘要值。
func ValidateChecksum(algorithm, value string) (string, error) {
	algorithm = strings.ToLower(strings.TrimSpace(algorithm))
	if algorithm != "sha256" && algorithm != "sha512" {
		return "", fmt.Errorf("unsupported checksum algorithm %q", algorithm)
	}
	value = strings.ToLower(strings.TrimSpace(value))
	if !checksumPattern(algorithm).MatchString(value) {
		return "", fmt.Errorf("%s must contain %d hexadecimal characters", algorithm, hexLength(algorithm))
	}
	if _, err := hex.DecodeString(value); err != nil {
		return "", err
	}
	return value, nil
}

func hexLength(algorithm string) int {
	if algorithm == "sha512" {
		return 128
	}
	return 64
}

// GodotHubProvider 从 GodotHub 的 releases.json 元数据解析资产摘要。
// 元数据是 GitHub Release API 结构，摘要来自对应资产的 digest 字段（sha256:<64 hex>），
// 下载资产位于 AtomGit 镜像，302 重定向到 file-cdn.gitcode.com 签名 CDN。
type GodotHubProvider struct {
	SourceName       string
	MetadataURL      string
	ArtifactTemplate string
	AuthorizationEnv string
	Client           *http.Client
}

// Name 返回来源名称。
func (p GodotHubProvider) Name() string { return p.SourceName }

// Resolve 从 releases.json 匹配 tag_name 和资产名，返回下载 URL 与预期摘要。
func (p GodotHubProvider) Resolve(ctx context.Context, request ResolveRequest) (Artifact, error) {
	artifactURL, err := expandURL(p.ArtifactTemplate, request.Version, request.AssetName)
	if err != nil {
		return Artifact{}, ConfigError{Err: err}
	}
	body, err := p.get(ctx, p.MetadataURL)
	if err != nil {
		return Artifact{}, err
	}
	digest, err := findDigest(body, request.Version, request.AssetName)
	if err != nil {
		var unavailable UnavailableError
		if errors.As(err, &unavailable) {
			// release 或资产不在元数据中：属于来源不可用，允许 fallback 到下一来源。
			return Artifact{}, err
		}
		return Artifact{}, ConfigError{Err: err}
	}
	checksum, err := ValidateChecksum("sha256", digest)
	if err != nil {
		return Artifact{}, ConfigError{Err: err}
	}
	return Artifact{
		Source:            p.SourceName,
		URL:               artifactURL,
		Filename:          request.AssetName,
		ChecksumAlgorithm: "sha256",
		Checksum:          checksum,
		AuthorizationEnv:  p.AuthorizationEnv,
	}, nil
}

func (p GodotHubProvider) get(ctx context.Context, rawURL string) ([]byte, error) {
	return fetchBody(ctx, p.Client, rawURL, p.AuthorizationEnv, maxMetadataResponseBytes)
}

// ListVersions 复用 releases.json 元数据枚举稳定版本，不新增请求端点。
func (p GodotHubProvider) ListVersions(ctx context.Context) ([]VersionInfo, error) {
	body, err := p.get(ctx, p.MetadataURL)
	if err != nil {
		return nil, err
	}
	return listVersionsFromReleases(body)
}

type godotHubRelease struct {
	TagName string          `json:"tag_name"`
	Assets  []godotHubAsset `json:"assets"`
}

type godotHubAsset struct {
	Name   string `json:"name"`
	Digest string `json:"digest"`
}

// findDigest 在 GitHub Release API 结构中找到目标 tag 下同名资产的 sha256 摘要。
func findDigest(body []byte, version, asset string) (string, error) {
	var releases []godotHubRelease
	if err := json.Unmarshal(body, &releases); err != nil {
		return "", errors.New("metadata is not a valid release list")
	}
	tag := version + "-stable"
	for _, release := range releases {
		if release.TagName != tag {
			continue
		}
		for _, item := range release.Assets {
			if item.Name != asset {
				continue
			}
			digest := strings.TrimSpace(item.Digest)
			digest = strings.TrimPrefix(digest, "sha256:")
			return digest, nil
		}
		return "", UnavailableError{Err: fmt.Errorf("asset %q not found in tag %s", asset, tag)}
	}
	return "", UnavailableError{Err: fmt.Errorf("tag %s not found in metadata", tag)}
}
