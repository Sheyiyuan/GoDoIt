package dotnet

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/Sheyiyuan/GoDoIt/core/internal/platform"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestResolveLatestPatchAndArtifactFromFixture(t *testing.T) {
	metadata := `{"releases":[
        {"sdk":{"version":"8.0.410","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://localhost/sdk.tar.gz","hash":"aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}]}},
        {"sdk":{"version":"8.0.412","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://localhost/sdk.tar.gz","hash":"bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}]}}
    ]}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(metadata)), Request: request}, nil
	})}
	latest, err := ResolveLatestPatch(context.Background(), client, "8.0")
	if err != nil || latest != "8.0.412" {
		t.Fatalf("unexpected latest: %q err=%v", latest, err)
	}
	artifact, err := ResolveArtifact(context.Background(), client, "8.0.410", platform.Target{OS: "linux", Arch: "amd64"})
	if err != nil || artifact.URL != "https://localhost/sdk.tar.gz" || len(artifact.Hash) != 128 {
		t.Fatalf("unexpected artifact: %+v err=%v", artifact, err)
	}
}

func TestResolveArtifactRejectsNonHexChecksum(t *testing.T) {
	metadata := `{"releases":[
        {"sdk":{"version":"8.0.410","files":[{"name":"dotnet-sdk-linux-x64.tar.gz","rid":"linux-x64","url":"https://localhost/sdk.tar.gz","hash":"zzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzzz"}]}}
    ]}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(metadata)), Request: request}, nil
	})}
	if _, err := ResolveArtifact(context.Background(), client, "8.0.410", platform.Target{OS: "linux", Arch: "amd64"}); err == nil {
		t.Fatal("non-hex checksum must be rejected at metadata validation")
	}
}

func TestParseSystemOutputSortsNewestFirst(t *testing.T) {
	items := ParseSystemOutput("8.0.404   [/usr/lib/dotnet/sdk]\n9.0.100 [/opt/dotnet/sdk]\n7.0.100 malformed\nbroken\n")
	if len(items) != 2 || items[0].Version != "9.0.100" || items[1].Path != "/usr/lib/dotnet/sdk" {
		t.Fatalf("unexpected SDKs: %+v", items)
	}
}

func TestProbeSystemDistinguishesMissingCommandFromExecutionFailure(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-dotnet")
	items, err := ProbeSystem(context.Background(), missing)
	if err != nil || len(items) != 0 {
		t.Fatalf("missing dotnet should be an empty list: %+v err=%v", items, err)
	}
	failing, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if _, err := ProbeSystem(context.Background(), failing); err == nil {
		t.Fatal("dotnet execution failure should be reported")
	}
}

func TestRecommendedMajor(t *testing.T) {
	if RecommendedMajor("4.5.2") != "8.0" || RecommendedMajor("4.1.4") != "6.0" || RecommendedMajor("4.7") != "8.0" || RecommendedMajor("4.8-dev3") != "8.0" {
		t.Fatal("unexpected recommendation mapping")
	}
	if !BelowRecommendedMajor("7.0.100-preview.1.123", "8.0") || BelowRecommendedMajor("11.0.100-preview.7.26381.103", "8.0") {
		t.Fatal("preview SDK major comparison is incorrect")
	}
}

func TestChannelsKeepsPreviewSkipsEOL(t *testing.T) {
	index := `{"releases-index":[
		{"channel-version":"11.0","support-phase":"preview","release-type":"sts"},
		{"channel-version":"10.0","support-phase":"active","release-type":"lts"},
		{"channel-version":"9.0","support-phase":"maintenance","release-type":"sts"},
		{"channel-version":"8.0","support-phase":"maintenance","release-type":"lts"},
		{"channel-version":"7.0","support-phase":"eol","release-type":"sts"}
	]}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(index)), Request: request}, nil
	})}
	channels, err := Channels(context.Background(), client)
	if err != nil {
		t.Fatal(err)
	}
	if len(channels) != 5 {
		t.Fatalf("preview kept, eol skipped, 6.0 kept: %+v", channels)
	}
	if channels[0].MajorMinor != "11.0" || channels[1].MajorMinor != "10.0" || channels[2].MajorMinor != "9.0" || channels[3].MajorMinor != "8.0" || channels[4].MajorMinor != "6.0" {
		t.Fatalf("unexpected channel order: %+v", channels)
	}
	if channels[0].Phase != "preview" || channels[0].ReleaseType != "sts" {
		t.Fatalf("unexpected 11.0 preview channel meta: %+v", channels[0])
	}
	if channels[4].Phase != "eol" {
		t.Fatalf("kept 6.0 must be marked eol: %+v", channels[4])
	}
}

func TestAvailableFiltersPrereleaseByFlag(t *testing.T) {
	metadata := `{"releases":[
		{"sdk":{"version":"11.0.100-preview.7.26381.103","files":[]}},
		{"sdk":{"version":"11.0.100-preview.6.26359.118","files":[]}}
	]}`
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 200, Status: "200 OK", Body: io.NopCloser(bytes.NewBufferString(metadata)), Request: request}, nil
	})}
	stable, err := Available(context.Background(), client, "11.0", false)
	if err != nil || len(stable) != 0 {
		t.Fatalf("stable-only listing must exclude prereleases: %+v err=%v", stable, err)
	}
	all, err := Available(context.Background(), client, "11.0", true)
	if err != nil || len(all) != 2 || all[0] != "11.0.100-preview.7.26381.103" {
		t.Fatalf("include-prerelease listing is wrong: %+v err=%v", all, err)
	}
}

func TestChannelsReportsHTTPFailure(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: 500, Status: "500 Internal Server Error", Body: io.NopCloser(bytes.NewBufferString("")), Request: request}, nil
	})}
	if _, err := Channels(context.Background(), client); err == nil {
		t.Fatal("channel index HTTP failure must be reported")
	}
}

func TestMirrorURL(t *testing.T) {
	mirrored, err := MirrorURL("https://builds.dotnet.microsoft.com/dotnet/Sdk/10.0.400/dotnet-sdk-10.0.400-linux-x64.tar.gz")
	if err != nil || mirrored != "https://mirrors.huaweicloud.com/dotnet/Sdk/10.0.400/dotnet-sdk-10.0.400-linux-x64.tar.gz" {
		t.Fatalf("unexpected mirrored URL: %q err=%v", mirrored, err)
	}
	if _, err := MirrorURL("https://example.com/dotnet/Sdk/x.tar.gz"); err == nil {
		t.Fatal("non-official hosts must not be mirrored")
	}
}
