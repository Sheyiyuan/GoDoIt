package main

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

type fakeManager struct {
	installResult        gdit.InstallResult
	installErr           error
	versions             []gdit.InstalledVersion
	listErr              error
	sources              []gdit.SourceInfo
	sourcesErr           error
	setSourceErr         error
	setSourceDisabledErr error
	available            []gdit.AvailableVersion
	availableErr         error
}

func (f fakeManager) Install(context.Context, gdit.InstallRequest) (gdit.InstallResult, error) {
	return f.installResult, f.installErr
}

func (f fakeManager) List(context.Context) ([]gdit.InstalledVersion, error) {
	return f.versions, f.listErr
}

func (f fakeManager) Sources(context.Context) ([]gdit.SourceInfo, error) {
	return f.sources, f.sourcesErr
}

func (f fakeManager) SetDefaultSource(context.Context, string) error { return f.setSourceErr }

func (f fakeManager) SetSourceDisabled(context.Context, string, bool) error {
	return f.setSourceDisabledErr
}

func (f fakeManager) Available(context.Context, string) ([]gdit.AvailableVersion, error) {
	return f.available, f.availableErr
}

func TestRunInstallSeparatesResultAndProgressStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}}}
	code := runWithManager(context.Background(), []string{"install", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "installed 4.5.2-standard\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if stderr.Len() != 0 {
		t.Fatalf("unexpected stderr: %q", stderr.String())
	}
}

func TestRunListWritesOnlyVersionsToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{versions: []gdit.InstalledVersion{{ID: "4.5.2-standard", Target: gdit.Target{OS: "linux", Arch: "amd64"}, Source: "fixture"}}}
	code := runWithManager(context.Background(), []string{"list"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "4.5.2-standard\tlinux/amd64\tfixture\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunInstallReportsErrorsOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installErr: errors.New("fixture failure")}
	code := runWithManager(context.Background(), []string{"install", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 || stderr.String() != "fixture failure\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestProgressWriterReportsDownloadChunks(t *testing.T) {
	var stderr bytes.Buffer
	writer := progressWriter(&stderr)
	writer(gdit.ProgressEvent{Stage: "download", Source: "fixture", Filename: "a.zip", BytesDownloaded: 8 * 1024 * 1024, TotalBytes: 16 * 1024 * 1024})
	writer(gdit.ProgressEvent{Stage: "download", Source: "fixture", Filename: "a.zip", BytesDownloaded: 16 * 1024 * 1024, TotalBytes: 16 * 1024 * 1024})
	writer(gdit.ProgressEvent{Stage: "download", Source: "fixture", Filename: "a.zip", BytesDownloaded: 17 * 1024 * 1024, TotalBytes: 16 * 1024 * 1024})
	got := stderr.String()
	if !strings.Contains(got, "downloaded 8 MB / 16 MB from fixture") {
		t.Fatalf("missing first chunk: %q", got)
	}
	if !strings.Contains(got, "downloaded 16 MB / 16 MB from fixture") {
		t.Fatalf("missing second chunk: %q", got)
	}
	if strings.Count(got, "downloaded") != 2 {
		t.Fatalf("expected exactly two progress lines: %q", got)
	}
}

func TestRunSourceListWritesOnlyStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{sources: []gdit.SourceInfo{{Name: "godothub", Kind: "builtin"}, {Name: "github", Kind: "builtin"}}}
	code := runWithManager(context.Background(), []string{"source", "list"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "1\tgodothub\tbuiltin\n2\tgithub\tbuiltin\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunSourceUseReportsConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), []string{"source", "use", "github"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "default source is now github\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunSourceUseWithoutNameReturnsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), []string{"source", "use"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunAvailableWritesVersionsToStdout(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{available: []gdit.AvailableVersion{
		{Version: "4.7.1", Editions: []string{"standard", "dotnet"}, Sources: []string{"godothub", "github"}},
		{Version: "4.5.2", Editions: []string{"standard"}, Sources: []string{"github"}},
	}}
	code := runWithManager(context.Background(), []string{"available"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "4.7.1\tstandard,dotnet\tgodothub,github\n4.5.2\tstandard\tgithub\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunSourceWithoutArgsListsSources(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{sources: []gdit.SourceInfo{
		{Name: "godothub", Kind: "builtin"},
		{Name: "github", Kind: "builtin", Disabled: true},
	}}
	code := runWithManager(context.Background(), []string{"source"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "1\tgodothub\tbuiltin\n2\tgithub\tbuiltin\tdisabled\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunSourceBanAndUnbanReportConfirmation(t *testing.T) {
	for _, test := range []struct {
		args  []string
		want  string
		call  string
		state bool
	}{
		{args: []string{"source", "ban", "godothub"}, want: "source godothub is disabled\n", call: "godothub", state: true},
		{args: []string{"source", "unban", "godothub"}, want: "source godothub is enabled\n", call: "godothub", state: false},
	} {
		var stdout, stderr bytes.Buffer
		manager := fakeManager{}
		code := runWithManager(context.Background(), test.args, &stdout, &stderr, manager, nil)
		if code != 0 {
			t.Fatalf("unexpected exit code: %d", code)
		}
		if stdout.String() != test.want || stderr.Len() != 0 {
			t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
		}
	}
}

func TestRunInstallWithoutArgsNonTTYReturnsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), []string{"install"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCommandAliases(t *testing.T) {
	for _, args := range [][]string{{"i", "4.5.2"}, {"l"}, {"s"}, {"a"}} {
		var stdout, stderr bytes.Buffer
		manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}}}
		code := runWithManager(context.Background(), args, &stdout, &stderr, manager, nil)
		if code == 2 {
			t.Fatalf("alias %v rejected: stderr=%q", args, stderr.String())
		}
	}
}

func TestFlagAliases(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-dotnet"}}}
	code := runWithManager(context.Background(), []string{"install", "-e", "dotnet", "-s", "github", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "installed 4.5.2-dotnet\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}
