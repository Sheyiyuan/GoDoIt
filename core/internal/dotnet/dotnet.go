// Package dotnet 负责系统与托管 .NET SDK 的探测、推荐映射和官方元数据解析。
package dotnet

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
	"os/exec"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
	"github.com/Sheyiyuan/GoDoIt/core/internal/store"
	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

var majorMinorPattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)

const metadataURLTemplate = "https://dotnetcli.azureedge.net/dotnet/release-metadata/%s/releases.json"
const channelsURL = "https://dotnetcli.azureedge.net/dotnet/release-metadata/releases-index.json"
const metadataLimit = 8 << 20 // 官方 release 元数据响应体上限，防止畸形/超大响应撑爆内存

// Channel 描述一个可下载的 .NET SDK 大版本通道及其支持状态。
type Channel struct {
	MajorMinor  string // "10.0"
	Phase       string // active / maintenance / eol
	ReleaseType string // lts / sts
}

// FallbackChannels 是通道索引不可用时的保底列表：
// 2026-08 实测的通道（11.0 preview、10.0 active/LTS、9.0 maintenance/STS、
// 8.0 maintenance/LTS）加上 Godot 4.0/4.1 需要的 6.0（EOL，官方元数据仍长期保留
// 可下载）。按版本倒序。
var FallbackChannels = []Channel{
	{MajorMinor: "11.0", Phase: "preview", ReleaseType: "sts"},
	{MajorMinor: "10.0", Phase: "active", ReleaseType: "lts"},
	{MajorMinor: "9.0", Phase: "maintenance", ReleaseType: "sts"},
	{MajorMinor: "8.0", Phase: "maintenance", ReleaseType: "lts"},
	{MajorMinor: "6.0", Phase: "eol", ReleaseType: "lts"},
}

type channelsIndex struct {
	ReleasesIndex []channelEntry `json:"releases-index"`
}
type channelEntry struct {
	ChannelVersion string `json:"channel-version"`
	SupportPhase   string `json:"support-phase"`
	ReleaseType    string `json:"release-type"`
}

// Channels 从官方通道索引枚举可下载的 SDK 大版本通道：跳过 eol，保留 preview 与
// 活跃通道，并始终保留 6.0（Godot 4.0/4.1 需要）。按版本倒序返回。
func Channels(ctx context.Context, client *http.Client) ([]Channel, error) {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, channelsURL, nil)
	if err != nil {
		return nil, err
	}
	response, err := client.Do(request)
	if err != nil {
		return nil, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return nil, fmt.Errorf("SDK channel index returned HTTP %s", response.Status)
	}
	var index channelsIndex
	decoder := json.NewDecoder(io.LimitReader(response.Body, metadataLimit))
	if err := decoder.Decode(&index); err != nil {
		return nil, fmt.Errorf("decode SDK channel index: %w", err)
	}
	seen := make(map[string]bool)
	result := make([]Channel, 0, len(index.ReleasesIndex)+1)
	for _, entry := range index.ReleasesIndex {
		if !majorMinorPattern.MatchString(entry.ChannelVersion) {
			continue
		}
		if entry.SupportPhase == "eol" {
			continue
		}
		seen[entry.ChannelVersion] = true
		result = append(result, Channel{MajorMinor: entry.ChannelVersion, Phase: entry.SupportPhase, ReleaseType: entry.ReleaseType})
	}
	if !seen["6.0"] {
		result = append(result, Channel{MajorMinor: "6.0", Phase: "eol", ReleaseType: "lts"})
	}
	sort.Slice(result, func(i, j int) bool { return compareMajorMinor(result[i].MajorMinor, result[j].MajorMinor) > 0 })
	return result, nil
}

// compareMajorMinor 比较两个 MAJOR.MINOR 版本号。
func compareMajorMinor(left, right string) int {
	leftParts, rightParts := strings.Split(left, "."), strings.Split(right, ".")
	for index := 0; index < 2; index++ {
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

// SDKInfo 描述系统或托管 SDK。
type SDKInfo struct {
	Version string
	Kind    string
	Path    string
}

// Artifact 描述 .NET SDK 的下载资产与 SHA-512 摘要。
type Artifact struct {
	Version string
	URL     string
	Name    string
	Hash    string
}

// RecommendedMajor 返回 Godot 版本系列对应的推荐 .NET major.minor。
// Godot 4.0/4.1 使用 .NET 6.0，4.2 及以后使用 .NET 8.0；表外版本返回空字符串供用户显式指定。
func RecommendedMajor(version string) string {
	parts := strings.Split(version, ".")
	if len(parts) < 2 || len(parts) > 3 {
		return ""
	}
	major, err := strconv.Atoi(parts[0])
	if err != nil || major < 4 {
		return ""
	}
	minorText, _, _ := strings.Cut(parts[1], "-")
	minor, err := strconv.Atoi(minorText)
	if err != nil {
		return ""
	}
	if major == 4 {
		if minor >= 2 {
			return "8.0"
		}
		if minor >= 0 {
			return "6.0"
		}
	}
	return ""
}

// BelowRecommendedMajor 报告 SDK major 是否低于推荐 major。
func BelowRecommendedMajor(version, recommendedMajorMinor string) bool {
	numericVersion, _, _ := strings.Cut(version, "-")
	versionParts := strings.Split(numericVersion, ".")
	recommendedParts := strings.Split(recommendedMajorMinor, ".")
	if len(versionParts) != 3 || len(recommendedParts) != 2 {
		return false
	}
	actual, actualErr := strconv.Atoi(versionParts[0])
	recommended, recommendedErr := strconv.Atoi(recommendedParts[0])
	return actualErr == nil && recommendedErr == nil && actual < recommended
}

// ProbeSystem 执行 dotnet --list-sdks；命令不存在时返回空列表，其他执行失败返回错误。
func ProbeSystem(ctx context.Context, command string) ([]SDKInfo, error) {
	if command == "" {
		command = "dotnet"
	}
	output, err := exec.CommandContext(ctx, command, "--list-sdks").Output()
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		if errors.Is(err, exec.ErrNotFound) || errors.Is(err, os.ErrNotExist) {
			return []SDKInfo{}, nil
		}
		return []SDKInfo{}, fmt.Errorf("run dotnet --list-sdks: %w", err)
	}
	return ParseSystemOutput(string(output)), nil
}

// ParseSystemOutput 解析 dotnet --list-sdks 输出。
func ParseSystemOutput(output string) []SDKInfo {
	var result []SDKInfo
	scanner := bufio.NewScanner(strings.NewReader(output))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		version, rest, ok := strings.Cut(line, " ")
		if !ok || !managedversion.ValidSDK(version) {
			continue
		}
		location := strings.TrimSpace(rest)
		if len(location) < 2 || location[0] != '[' || location[len(location)-1] != ']' {
			continue
		}
		path := strings.TrimSpace(location[1 : len(location)-1])
		result = append(result, SDKInfo{Version: version, Kind: "system", Path: path})
	}
	sort.Slice(result, func(i, j int) bool { return compareVersion(result[i].Version, result[j].Version) > 0 })
	return result
}

// Managed 返回有效托管 SDK 列表。
func Managed(root string) ([]SDKInfo, error) {
	records, err := store.New(root).ScanSDKs()
	if err != nil {
		return nil, err
	}
	result := make([]SDKInfo, 0, len(records))
	for _, record := range records {
		result = append(result, SDKInfo{Version: record.Manifest.Version, Kind: "managed", Path: record.Dir})
	}
	sort.Slice(result, func(i, j int) bool { return compareVersion(result[i].Version, result[j].Version) > 0 })
	return result, nil
}

// ResolveLatestPatch 从官方 release metadata 解析 major.minor 的最新稳定 patch。
// 预发布版本（带 -preview/-rc 后缀）不参与推荐解析。
func ResolveLatestPatch(ctx context.Context, client *http.Client, majorMinor string) (string, error) {
	if !majorMinorPattern.MatchString(majorMinor) {
		return "", errors.New("invalid SDK major.minor")
	}
	metadata, err := fetchMetadata(ctx, client, majorMinor)
	if err != nil {
		return "", err
	}
	versions := make([]string, 0)
	for _, release := range metadata.Releases {
		if release.SDK == nil || !managedversion.ValidSDK(release.SDK.Version) || strings.Contains(release.SDK.Version, "-") {
			continue
		}
		if strings.HasPrefix(release.SDK.Version, majorMinor+".") {
			versions = append(versions, release.SDK.Version)
		}
	}
	if len(versions) == 0 {
		return "", fmt.Errorf("no stable SDK patch available for %s", majorMinor)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersion(versions[i], versions[j]) > 0 })
	return versions[0], nil
}

// Available 返回指定 major.minor 官方元数据中的 SDK 版本，按新到旧排序。
// includePrerelease 为 false 时只返回稳定版（供推荐解析等场景使用）。
func Available(ctx context.Context, client *http.Client, majorMinor string, includePrerelease bool) ([]string, error) {
	if !majorMinorPattern.MatchString(majorMinor) {
		return nil, errors.New("invalid SDK major.minor")
	}
	metadata, err := fetchMetadata(ctx, client, majorMinor)
	if err != nil {
		return nil, err
	}
	seen := make(map[string]struct{})
	versions := make([]string, 0)
	for _, release := range metadata.Releases {
		if release.SDK == nil || !managedversion.ValidSDK(release.SDK.Version) || !strings.HasPrefix(release.SDK.Version, majorMinor+".") {
			continue
		}
		if !includePrerelease && strings.Contains(release.SDK.Version, "-") {
			continue
		}
		if _, exists := seen[release.SDK.Version]; exists {
			continue
		}
		seen[release.SDK.Version] = struct{}{}
		versions = append(versions, release.SDK.Version)
	}
	sort.Slice(versions, func(i, j int) bool { return compareVersion(versions[i], versions[j]) > 0 })
	return versions, nil
}

// ResolveArtifact 从官方元数据定位指定版本和平台的 SDK 资产。
func ResolveArtifact(ctx context.Context, client *http.Client, version string, target platform.Target) (Artifact, error) {
	if !managedversion.ValidSDK(version) {
		return Artifact{}, errors.New("SDK version must be MAJOR.MINOR.PATCH")
	}
	parts := strings.Split(version, ".")
	metadata, err := fetchMetadata(ctx, client, parts[0]+"."+parts[1])
	if err != nil {
		return Artifact{}, err
	}
	rid, err := platform.SDKRID(target)
	if err != nil {
		return Artifact{}, err
	}
	for _, release := range metadata.Releases {
		if release.SDK == nil || release.SDK.Version != version {
			continue
		}
		for _, file := range release.SDK.Files {
			if file.RID != rid || file.URL == "" {
				continue
			}
			hash := strings.ToLower(strings.TrimSpace(file.Hash))
			if len(hash) != 128 {
				continue
			}
			if _, err := hex.DecodeString(hash); err != nil {
				return Artifact{}, fmt.Errorf("SDK metadata contains an invalid checksum for %s/%s", version, rid)
			}
			return Artifact{Version: version, URL: file.URL, Name: file.Name, Hash: hash}, nil
		}
	}
	return Artifact{}, fmt.Errorf("SDK asset not found for %s/%s", version, rid)
}

type metadata struct {
	Releases []release `json:"releases"`
}
type release struct {
	SDK *sdkRelease `json:"sdk"`
}
type sdkRelease struct {
	Version string    `json:"version"`
	Files   []sdkFile `json:"files"`
}
type sdkFile struct {
	Name string `json:"name"`
	RID  string `json:"rid"`
	URL  string `json:"url"`
	Hash string `json:"hash"`
}

func fetchMetadata(ctx context.Context, client *http.Client, majorMinor string) (metadata, error) {
	if client == nil {
		client = http.DefaultClient
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(metadataURLTemplate, majorMinor), nil)
	if err != nil {
		return metadata{}, err
	}
	response, err := client.Do(request)
	if err != nil {
		return metadata{}, err
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return metadata{}, fmt.Errorf("SDK metadata returned HTTP %s", response.Status)
	}
	var result metadata
	decoder := json.NewDecoder(io.LimitReader(response.Body, metadataLimit))
	if err := decoder.Decode(&result); err != nil {
		return metadata{}, fmt.Errorf("decode SDK metadata: %w", err)
	}
	return result, nil
}

func compareVersion(left, right string) int {
	// 只比较数字前缀段；预发布后缀（-preview/-rc）不影响同数字版本的排序。
	leftParts := strings.Split(numericPrefix(left), ".")
	rightParts := strings.Split(numericPrefix(right), ".")
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

// numericPrefix 截取版本号的数字部分（到第一个 - 为止）。
func numericPrefix(version string) string {
	part, _, _ := strings.Cut(version, "-")
	return part
}

// MirrorURL 把官方 SDK 资产 URL 映射到华为云镜像（2026-08 实测镜像资产路径与官方一致，
// 仅 host 不同；元数据未被镜像，仍从官方获取）。非官方 host 返回错误，不猜测映射。
func MirrorURL(officialURL string) (string, error) {
	parsed, err := url.Parse(officialURL)
	if err != nil || parsed.Host != "builds.dotnet.microsoft.com" {
		return "", fmt.Errorf("cannot mirror SDK asset URL %q", officialURL)
	}
	parsed.Host = "mirrors.huaweicloud.com"
	return parsed.String(), nil
}
