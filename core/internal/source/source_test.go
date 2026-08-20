package source

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/config"
)

func TestHTTPProviderResolvesChecksumFromMatchingLine(t *testing.T) {
	asset := "Godot_v4.5.2-stable_linux.x86_64.zip"
	checksum := strings.Repeat("a", 64)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/4.5.2-stable/SHA256SUMS.txt":
			return fixtureResponse(request, http.StatusOK, []byte(strings.Repeat("b", 64)+" other.zip\n"+checksum+"  "+asset+"\n")), nil
		default:
			return fixtureResponse(request, http.StatusNotFound, nil), nil
		}
	})}
	provider := HTTPProvider{
		SourceName:       "fixture",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		ChecksumTemplate: "http://localhost/{tag}/SHA256SUMS.txt",
		Client:           client,
	}
	artifact, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", Edition: "standard", AssetName: asset})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ChecksumAlgorithm != "sha256" || artifact.Checksum != checksum || artifact.Filename != asset {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
}

func TestHTTPProviderResolvesSHA512Checksum(t *testing.T) {
	asset := "Godot_v4.5.2-stable_linux.x86_64.zip"
	checksum := strings.Repeat("c", 128)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/4.5.2-stable/SHA512-SUMS.txt":
			return fixtureResponse(request, http.StatusOK, []byte(strings.Repeat("d", 128)+"  other.zip\n"+checksum+"  "+asset+"\n")), nil
		default:
			return fixtureResponse(request, http.StatusNotFound, nil), nil
		}
	})}
	provider := HTTPProvider{
		SourceName:        "fixture512",
		ArtifactTemplate:  "http://localhost/{tag}/{asset}",
		ChecksumTemplate:  "http://localhost/{tag}/SHA512-SUMS.txt",
		ChecksumAlgorithm: "sha512",
		Client:            client,
	}
	artifact, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", Edition: "standard", AssetName: asset})
	if err != nil {
		t.Fatal(err)
	}
	if artifact.ChecksumAlgorithm != "sha512" || artifact.Checksum != checksum || artifact.Filename != asset {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
}

func TestGodotHubProviderResolvesDigestFromMetadata(t *testing.T) {
	asset := "Godot_v4.5.2-stable_linux.x86_64.zip"
	digest := strings.Repeat("e", 64)
	metadata := []byte(`[{"tag_name":"4.4-stable","assets":[]},{"tag_name":"4.5.2-stable","assets":[{"name":"` + asset + `","digest":"sha256:` + digest + `"}]}]`)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		switch request.URL.Path {
		case "/api/releases.json":
			return fixtureResponse(request, http.StatusOK, metadata), nil
		default:
			return fixtureResponse(request, http.StatusNotFound, nil), nil
		}
	})}
	provider := GodotHubProvider{
		SourceName:       "godothub",
		MetadataURL:      "http://localhost/api/releases.json",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		Client:           client,
	}
	artifact, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", Edition: "standard", AssetName: asset})
	if err != nil {
		t.Fatal(err)
	}
	expectedURL := "http://localhost/4.5.2-stable/" + asset
	if artifact.ChecksumAlgorithm != "sha256" || artifact.Checksum != digest || artifact.URL != expectedURL {
		t.Fatalf("unexpected artifact: %+v", artifact)
	}
}

// release 不在元数据中属于来源不可用，允许 fallback 到下一来源。
func TestGodotHubProviderMissingTagIsUnavailable(t *testing.T) {
	metadata := []byte(`[{"tag_name":"4.4-stable","assets":[]}]`)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusOK, metadata), nil
	})}
	provider := GodotHubProvider{
		SourceName:       "godothub",
		MetadataURL:      "http://localhost/api/releases.json",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		Client:           client,
	}
	_, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", AssetName: "asset.zip"})
	var unavailable UnavailableError
	if !errors.As(err, &unavailable) {
		t.Fatalf("expected unavailable error for missing tag, got %v", err)
	}
}

func TestValidateChecksumRejectsWrongLength(t *testing.T) {
	if _, err := ValidateChecksum("sha512", strings.Repeat("a", 64)); err == nil {
		t.Fatal("expected sha512 with 64 hex chars to be rejected")
	}
	if _, err := ValidateChecksum("sha256", strings.Repeat("a", 128)); err == nil {
		t.Fatal("expected sha256 with 128 hex chars to be rejected")
	}
	if _, err := ValidateChecksum("md5", strings.Repeat("a", 32)); err == nil {
		t.Fatal("expected unsupported algorithm to be rejected")
	}
}

func TestProvidersFromConfigBuildsBuiltinSources(t *testing.T) {
	cfg := config.File{
		SchemaVersion: 1,
		SourceOrder:   []string{"godothub", "github"},
	}
	providers, err := ProvidersFromConfig(cfg, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(providers) != 2 {
		t.Fatalf("expected 2 providers, got %d", len(providers))
	}
	if providers[0].Name() != "godothub" {
		t.Fatalf("expected godothub first, got %s", providers[0].Name())
	}
	if providers[1].Name() != "github" {
		t.Fatalf("expected github second, got %s", providers[1].Name())
	}
}

func TestProvidersFromConfigKeepsAtomgitUnconfigured(t *testing.T) {
	cfg := config.File{
		SchemaVersion: 1,
		SourceOrder:   []string{"godothub", "atomgit"},
	}
	if _, err := ProvidersFromConfig(cfg, nil); err == nil {
		t.Fatal("expected atomgit to remain a config error until its URL rules are confirmed")
	}
}

func TestHTTPProviderTreatsTemporaryStatusAsUnavailable(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusBadGateway, []byte("try again")), nil
	})}
	provider := HTTPProvider{
		SourceName:       "fixture",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		ChecksumTemplate: "http://localhost/{tag}/SHA256SUMS.txt",
		Client:           client,
	}
	_, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", AssetName: "asset.zip"})
	var unavailable UnavailableError
	if !asUnavailable(err, &unavailable) {
		t.Fatalf("expected unavailable error, got %v", err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) {
	return f(request)
}

func fixtureResponse(request *http.Request, status int, body []byte) *http.Response {
	return &http.Response{
		StatusCode:    status,
		Status:        fmt.Sprintf("%d %s", status, http.StatusText(status)),
		Header:        make(http.Header),
		Body:          io.NopCloser(bytes.NewReader(body)),
		ContentLength: int64(len(body)),
		Request:       request,
	}
}

func TestHTTPProviderRejectsNonHTTPSRemoteURL(t *testing.T) {
	provider := HTTPProvider{
		SourceName:       "fixture",
		ArtifactTemplate: "http://mirror.example/{tag}/{asset}",
		ChecksumTemplate: "http://mirror.example/{tag}/SHA256SUMS.txt",
	}
	_, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", AssetName: "asset.zip"})
	var configErr ConfigError
	if !asConfig(err, &configErr) {
		t.Fatalf("expected config error, got %v", err)
	}
}

func TestFindChecksumDoesNotReuseHashFromAnotherAsset(t *testing.T) {
	body := []byte(strings.Repeat("b", 64) + " other.zip\ntarget.zip\n")
	if _, err := findChecksum(body, "target.zip", "sha256"); err == nil {
		t.Fatal("expected target without a checksum to be rejected")
	}
}

// 枚举测试按 Linux amd64 平台资产名判断 edition，与第一阶段支持范围一致。
func requireLinuxAMD64(t *testing.T) {
	t.Helper()
	if runtime.GOOS != "linux" || runtime.GOARCH != "amd64" {
		t.Skip("version listing fixture targets Linux amd64")
	}
}

func TestGodotHubProviderListVersionsFiltersAndDetectsEditions(t *testing.T) {
	requireLinuxAMD64(t)
	metadata := []byte(`[
		{"tag_name":"4.8-dev3","assets":[{"name":"Godot_v4.8-dev3_linux.x86_64.zip"}]},
		{"tag_name":"4.7.2-rc1","assets":[]},
		{"tag_name":"4.7-stable","assets":[{"name":"Godot_v4.7-stable_linux.x86_64.zip"}]},
		{"tag_name":"3.6.2-stable","assets":[
			{"name":"Godot_v3.6.2-stable_x11.64.zip"},
			{"name":"Godot_v3.6.2-stable_mono_x11_64.zip"}]},
		{"tag_name":"4.5.2-stable","assets":[
			{"name":"Godot_v4.5.2-stable_linux.x86_64.zip"},
			{"name":"Godot_v4.5.2-stable_mono_linux_x86_64.zip"}]}
	]`)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return fixtureResponse(request, http.StatusOK, metadata), nil
	})}
	provider := GodotHubProvider{
		SourceName:       "godothub",
		MetadataURL:      "http://localhost/api/releases.json",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		Client:           client,
	}
	versions, err := provider.ListVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 4 {
		t.Fatalf("expected 4.8-dev3, 4.7, 3.6.2 and 4.5.2 (rc without assets filtered), got %+v", versions)
	}
	if versions[0].Version != "4.8-dev3" || len(versions[0].Editions) != 1 || versions[0].Editions[0] != "standard" {
		t.Fatalf("prerelease version must keep its tag as version and detect editions: %+v", versions[0])
	}
	if versions[1].Version != "4.7" || len(versions[1].Editions) != 1 || versions[1].Editions[0] != "standard" {
		t.Fatalf("two-part stable tag (4.7-stable) must be listed as 4.7: %+v", versions[1])
	}
	if versions[2].Version != "3.6.2" || len(versions[2].Editions) != 2 || versions[2].Editions[0] != "standard" || versions[2].Editions[1] != "dotnet" {
		t.Fatalf("Godot 3.x x11/mono asset naming must be detected: %+v", versions[2])
	}
	if versions[3].Version != "4.5.2" || len(versions[3].Editions) != 2 || versions[3].Editions[0] != "standard" || versions[3].Editions[1] != "dotnet" {
		t.Fatalf("unexpected stable version: %+v", versions[3])
	}
}

func TestHTTPProviderListVersionsUsesReleasesURL(t *testing.T) {
	requireLinuxAMD64(t)
	body := []byte(`[
		{"tag_name":"4.7.1-stable","assets":[{"name":"Godot_v4.7.1-stable_linux.x86_64.zip"}]},
		{"tag_name":"4.5.2-stable","assets":[
			{"name":"Godot_v4.5.2-stable_linux.x86_64.zip"},
			{"name":"Godot_v4.5.2-stable_mono_linux_x86_64.zip"}]}
	]`)
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if request.URL.Path != "/releases" {
			return fixtureResponse(request, http.StatusNotFound, nil), nil
		}
		return fixtureResponse(request, http.StatusOK, body), nil
	})}
	provider := HTTPProvider{
		SourceName:        "github",
		ArtifactTemplate:  "http://localhost/{tag}/{asset}",
		ChecksumTemplate:  "http://localhost/{tag}/SHA512-SUMS.txt",
		ChecksumAlgorithm: "sha512",
		ReleasesURL:       "http://localhost/releases",
		Client:            client,
	}
	versions, err := provider.ListVersions(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if len(versions) != 2 || versions[0].Version != "4.7.1" || versions[1].Version != "4.5.2" {
		t.Fatalf("unexpected versions: %+v", versions)
	}
}

func TestHTTPProviderListVersionsUnsupportedWithoutReleasesURL(t *testing.T) {
	provider := HTTPProvider{
		SourceName:       "fixture",
		ArtifactTemplate: "http://localhost/{tag}/{asset}",
		ChecksumTemplate: "http://localhost/{tag}/SHA256SUMS.txt",
	}
	_, err := provider.ListVersions(context.Background())
	var configErr ConfigError
	if !errors.As(err, &configErr) {
		t.Fatalf("expected config error for missing ReleasesURL, got %v", err)
	}
}

func TestHTTPProviderRejectsUnknownTemplatePlaceholder(t *testing.T) {
	provider := HTTPProvider{
		SourceName:       "fixture",
		ArtifactTemplate: "https://mirror.example/{unknown}/{asset}",
		ChecksumTemplate: "https://mirror.example/{tag}/SHA256SUMS.txt",
	}
	_, err := provider.Resolve(context.Background(), ResolveRequest{Version: "4.5.2", AssetName: "asset.zip"})
	var configErr ConfigError
	if !asConfig(err, &configErr) {
		t.Fatalf("expected config error, got %v", err)
	}
}

func asUnavailable(err error, target *UnavailableError) bool {
	return errors.As(err, target)
}

func asConfig(err error, target *ConfigError) bool {
	return errors.As(err, target)
}
