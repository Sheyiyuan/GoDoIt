package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"testing"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

type fakeManager struct {
	installResult        gdit.InstallResult
	installErr           error
	installErrs          []error                // 非空时按调用序号返回错误
	installRequests      *[]gdit.InstallRequest // 非空时记录 Install 收到的请求
	versions             []gdit.InstalledVersion
	listErr              error
	sources              []gdit.SourceInfo
	sourcesErr           error
	setSourceErr         error
	setSourceDisabledErr error
	available            []gdit.AvailableVersion
	availableErr         error
	defaultID            string
	defaultErr           error
	setDefaultErr        error
	removeErr            error
	setupErr             error
	launchTarget         gdit.LaunchTarget
	launchErr            error
	launched             *[]string // 非空时记录 ResolveLaunch 收到的版本 ID
}

func (f fakeManager) Install(_ context.Context, request gdit.InstallRequest) (gdit.InstallResult, error) {
	if f.installRequests != nil {
		*f.installRequests = append(*f.installRequests, request)
	}
	if len(f.installErrs) > 0 {
		index := 0
		if f.installRequests != nil {
			index = len(*f.installRequests) - 1
		}
		if index < len(f.installErrs) {
			if err := f.installErrs[index]; err != nil {
				return gdit.InstallResult{}, err
			}
			return f.installResult, nil
		}
		return f.installResult, f.installErr
	}
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

func (f fakeManager) Default(context.Context) (string, error) { return f.defaultID, f.defaultErr }

func (f fakeManager) SetDefault(context.Context, string) error { return f.setDefaultErr }

func (f fakeManager) Remove(context.Context, string) error { return f.removeErr }

func (f fakeManager) Setup(context.Context) error { return f.setupErr }

func (f fakeManager) ResolveLaunch(_ context.Context, id string) (gdit.LaunchTarget, error) {
	if f.launched != nil {
		*f.launched = append(*f.launched, id)
	}
	return f.launchTarget, f.launchErr
}

func TestRunInstallSeparatesResultAndProgressStreams(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}}}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"list"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "4.5.2-standard\tlinux/amd64\tfixture\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunListMarksDefaultVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{
		versions: []gdit.InstalledVersion{
			{ID: "4.5.2-standard", Target: gdit.Target{OS: "linux", Arch: "amd64"}, Source: "fixture"},
			{ID: "4.6.2-dotnet", Target: gdit.Target{OS: "linux", Arch: "amd64"}, Source: "fixture"},
		},
		defaultID: "4.6.2-dotnet",
	}
	code := runWithManager(context.Background(), "", []string{"list"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if stdout.String() != "4.5.2-standard\tlinux/amd64\tfixture\n4.6.2-dotnet\tlinux/amd64\tfixture\tdefault\n" {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunInstallReportsErrorsOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installErr: errors.New("fixture failure")}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"source", "list"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"source", "use", "github"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"source", "use"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"available"}, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"source"}, &stdout, &stderr, manager, nil)
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
		code := runWithManager(context.Background(), "", test.args, &stdout, &stderr, manager, nil)
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
	code := runWithManager(context.Background(), "", []string{"install"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestCommandAliases(t *testing.T) {
	for _, args := range [][]string{{"i", "4.5.2"}, {"l"}, {"s"}, {"a"}, {"d"}, {"st"}} {
		var stdout, stderr bytes.Buffer
		manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}}}
		code := runWithManager(context.Background(), "", args, &stdout, &stderr, manager, nil)
		if code == 2 {
			t.Fatalf("alias %v rejected: stderr=%q", args, stderr.String())
		}
	}
}

func TestFlagAliases(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-dotnet"}}}
	code := runWithManager(context.Background(), "", []string{"install", "-e", "dotnet", "-s", "github", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "installed 4.5.2-dotnet\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestInstallMPrefixImpliesDotnet(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-dotnet"}}}
	code := runWithManager(context.Background(), "", []string{"install", "m4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "installed 4.5.2-dotnet\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
}

func TestInstallMPrefixWithEditionIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"install", "-e", "dotnet", "m4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestInstallMultipleVersionsSerially(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var requests []gdit.InstallRequest
	manager := fakeManager{
		installResult:   gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}},
		installRequests: &requests,
	}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2", "m4.6.2", "4.7.1"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if len(requests) != 3 {
		t.Fatalf("expected three install requests, got %+v", requests)
	}
	if requests[0].Version != "4.5.2" || requests[0].Edition != "standard" {
		t.Fatalf("unexpected first request: %+v", requests[0])
	}
	if requests[1].Version != "4.6.2" || requests[1].Edition != "dotnet" {
		t.Fatalf("unexpected second request: %+v", requests[1])
	}
	if requests[2].Version != "4.7.1" || requests[2].Edition != "standard" {
		t.Fatalf("unexpected third request: %+v", requests[2])
	}
	want := "installed 4.5.2-standard\ninstalled 4.5.2-standard\ninstalled 4.5.2-standard\n"
	if stdout.String() != want || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestInstallMultipleVersionsWithSharedEdition(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var requests []gdit.InstallRequest
	manager := fakeManager{
		installResult:   gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-dotnet"}},
		installRequests: &requests,
	}
	code := runWithManager(context.Background(), "", []string{"install", "--edition", "dotnet", "4.5.2", "4.6.2"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if len(requests) != 2 || requests[0].Edition != "dotnet" || requests[1].Edition != "dotnet" {
		t.Fatalf("shared edition must apply to all versions: %+v", requests)
	}
}

func TestInstallMultipleVersionsMPrefixWithEditionIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"install", "-e", "dotnet", "4.5.2", "m4.6.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestInstallMultipleVersionsPartialFailureContinues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var requests []gdit.InstallRequest
	manager := fakeManager{
		installResult:   gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}},
		installErrs:     []error{nil, errors.New("fixture failure")},
		installRequests: &requests,
	}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2", "4.6.2", "4.7.1"}, &stdout, &stderr, manager, nil)
	if code != 1 {
		t.Fatalf("partial failure must exit 1, got %d", code)
	}
	if len(requests) != 3 {
		t.Fatalf("failure must not interrupt remaining versions: %+v", requests)
	}
	if stdout.String() != "installed 4.5.2-standard\ninstalled 4.5.2-standard\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "fixture failure") {
		t.Fatalf("missing failure on stderr: %q", stderr.String())
	}
}

func TestRunDefaultShowsCurrentVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{defaultID: "4.5.2-standard"}
	code := runWithManager(context.Background(), "", []string{"default"}, &stdout, &stderr, manager, nil)
	if code != 0 || stdout.String() != "default: 4.5.2-standard\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDefaultWithoutDefaultPrintsHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{defaultErr: gdit.ErrNoDefault}
	code := runWithManager(context.Background(), "", []string{"default"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no default version set") {
		t.Fatalf("missing hint on stderr: %q", stderr.String())
	}
}

func TestRunDefaultSetsVersion(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"default", "m4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 || stdout.String() != "default: 4.5.2-dotnet\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDefaultInvalidVersionFails(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"default", "4.x"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDefaultTooManyArgsIsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"default", "4.5.2", "4.6.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRemoveReportsRemoved(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"remove", "-y", "m4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 || stdout.String() != "removed 4.5.2-dotnet\n" || stderr.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRemoveLongYesFlagSkipsConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"remove", "--yes", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 || stdout.String() != "removed 4.5.2-standard\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunRemoveWithoutYesNonTTYRequiresConfirmation(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"remove", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "remove requires confirmation") {
		t.Fatalf("missing confirmation hint on stderr: %q", stderr.String())
	}
}

func TestRunRemoveErrorsOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{removeErr: gdit.ErrDefaultInUse}
	code := runWithManager(context.Background(), "", []string{"remove", "-y", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "cannot remove current default") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunSetupCreatesShimAndHintsWhenNotInPATH(t *testing.T) {
	root := t.TempDir()
	t.Setenv("PATH", root)
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), root, []string{"setup"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if stdout.String() != "godot shim ready at "+root+"/bin/godot\n" {
		t.Fatalf("unexpected stdout: %q", stdout.String())
	}
	if !strings.Contains(stderr.String(), "hint: add "+root+"/bin to PATH") {
		t.Fatalf("missing PATH hint on stderr: %q", stderr.String())
	}
}

func TestRunSetupNoHintWhenBinInPATH(t *testing.T) {
	root := t.TempDir()
	bin := root + "/bin"
	t.Setenv("PATH", bin)
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), root, []string{"setup"}, &stdout, &stderr, manager, nil)
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunSetupReportsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{setupErr: errors.New("fixture failure")}
	code := runWithManager(context.Background(), t.TempDir(), []string{"setup"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 || stderr.String() != "fixture failure\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunDefaultLaunchesWithPassthroughArgsAndExitCode(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var launches []string
	original := spawnProcess
	spawnProcess = func(executable string, engineArgs []string, stdout, stderr io.Writer) int {
		launches = append(launches, executable, strings.Join(engineArgs, " "))
		return 42
	}
	t.Cleanup(func() { spawnProcess = original })
	var launched []string
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.5.2-standard", Executable: "/fake/godot"},
		launched:     &launched,
	}
	code := runWithManager(context.Background(), "", []string{"run", "-d", "--", "-e", "--path", "."}, &stdout, &stderr, manager, nil)
	if code != 42 {
		t.Fatalf("engine exit code was not propagated: %d", code)
	}
	if len(launches) != 2 || launches[0] != "/fake/godot" || launches[1] != "-e --path ." {
		t.Fatalf("unexpected spawn call: %v", launches)
	}
	if len(launched) != 1 || launched[0] != "" {
		t.Fatalf("default launch must resolve with empty id: %v", launched)
	}
	if stdout.Len() != 0 || stderr.Len() != 0 {
		t.Fatalf("unexpected streams: stdout=%q stderr=%q", stdout.String(), stderr.String())
	}
}

func TestRunVersionLaunchesWithoutChangingDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var launches []string
	original := spawnProcess
	spawnProcess = func(executable string, engineArgs []string, stdout, stderr io.Writer) int {
		launches = append(launches, executable, strings.Join(engineArgs, " "))
		return 0
	}
	t.Cleanup(func() { spawnProcess = original })
	var launched []string
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.6.2-dotnet", Executable: "/fake/mono-godot"},
		launched:     &launched,
	}
	code := runWithManager(context.Background(), "", []string{"run", "m4.6.2", "--", "-e"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if len(launched) != 1 || launched[0] != "4.6.2-dotnet" {
		t.Fatalf("explicit version must resolve its own id: %v", launched)
	}
	if len(launches) != 2 || launches[0] != "/fake/mono-godot" || launches[1] != "-e" {
		t.Fatalf("unexpected spawn call: %v", launches)
	}
}

func TestRunWithoutVersionWithArgsLaunchesDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	original := spawnProcess
	spawnProcess = func(executable string, engineArgs []string, stdout, stderr io.Writer) int {
		return 0
	}
	t.Cleanup(func() { spawnProcess = original })
	var launched []string
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.5.2-standard", Executable: "/fake/godot"},
		launched:     &launched,
	}
	code := runWithManager(context.Background(), "", []string{"run", "--", "-e"}, &stdout, &stderr, manager, nil)
	if code != 0 || len(launched) != 1 || launched[0] != "" {
		t.Fatalf("unexpected result: code=%d launched=%v stderr=%q", code, launched, stderr.String())
	}
}

func TestRunWithoutVersionNonTTYReturnsUsage(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{versions: []gdit.InstalledVersion{{ID: "4.5.2-standard"}}}
	code := runWithManager(context.Background(), "", []string{"run"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunCombinesDashDAndVersionAsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"run", "-d", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunNoDefaultPrintsHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{launchErr: gdit.ErrNoDefault}
	code := runWithManager(context.Background(), "", []string{"run", "-d"}, &stdout, &stderr, manager, nil)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "no default version set") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunShimExecutesDefault(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var replaced []string
	original := replaceProcess
	replaceProcess = func(executable string, engineArgs []string) error {
		replaced = append(replaced, executable, strings.Join(engineArgs, " "))
		return nil
	}
	t.Cleanup(func() { replaceProcess = original })
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.5.2-standard", Executable: "/fake/godot"},
	}
	code := runShim(context.Background(), []string{"-e", "--path", "."}, &stdout, &stderr, manager)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d, stderr=%q", code, stderr.String())
	}
	if len(replaced) != 2 || replaced[0] != "/fake/godot" || replaced[1] != "-e --path ." {
		t.Fatalf("unexpected exec call: %v", replaced)
	}
}

func TestRunShimNoDefaultPrintsHint(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{launchErr: gdit.ErrNoDefault}
	code := runShim(context.Background(), []string{"-e"}, &stdout, &stderr, manager)
	if code != 1 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "no default version set") {
		t.Fatalf("missing hint on stderr: %q", stderr.String())
	}
}

func TestRunShimExecFailureReportsError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	original := replaceProcess
	replaceProcess = func(executable string, engineArgs []string) error {
		return errors.New("exec failed")
	}
	t.Cleanup(func() { replaceProcess = original })
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.5.2-standard", Executable: "/fake/godot"},
	}
	code := runShim(context.Background(), []string{"-e"}, &stdout, &stderr, manager)
	if code != 1 || stdout.Len() != 0 || !strings.Contains(stderr.String(), "exec failed") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestIsShimInvocation(t *testing.T) {
	if !isShimInvocation("/home/user/.gdit/bin/godot") || !isShimInvocation("godot") {
		t.Fatal("godot basename must enter the shim branch")
	}
	if isShimInvocation("/home/user/.gdit/bin/gdit") || isShimInvocation("") {
		t.Fatal("non-godot basename must not enter the shim branch")
	}
}

// 着色分支通过替换 stdoutIsTTY 触达：TTY 且无 NO_COLOR 时默认版本整行品牌色。
func TestRunListHighlightsDefaultLineOnTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	original := stdoutIsTTY
	stdoutIsTTY = func() bool { return true }
	t.Cleanup(func() { stdoutIsTTY = original })
	var stdout, stderr bytes.Buffer
	manager := fakeManager{
		versions:  []gdit.InstalledVersion{{ID: "4.6.2-dotnet", Target: gdit.Target{OS: "linux", Arch: "amd64"}, Source: "fixture"}},
		defaultID: "4.6.2-dotnet",
	}
	code := runWithManager(context.Background(), "", []string{"list"}, &stdout, &stderr, manager, nil)
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	want := ansiBrand + "4.6.2-dotnet\tlinux/amd64\tfixture\tdefault" + ansiReset + "\n"
	if stdout.String() != want {
		t.Fatalf("default line must be highlighted on TTY: %q", stdout.String())
	}
}

// TTY 判定可注入：非 TTY 默认值下 remove 不带 -y 报用法错误（常规测试环境的保障）。
func TestRunRemoveTTYDetectionIsInjectable(t *testing.T) {
	original := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	t.Cleanup(func() { stdinIsTTY = original })
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"remove", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 2 || !strings.Contains(stderr.String(), "remove requires confirmation") {
		t.Fatalf("unexpected result: code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunRunEngineFlagBeforeSeparatorIsUsageError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{}
	code := runWithManager(context.Background(), "", []string{"run", "-e"}, &stdout, &stderr, manager, nil)
	if code != 2 || stdout.Len() != 0 {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), `--`) {
		t.Fatalf("error must point at the -- separator: %q", stderr.String())
	}
}

// -- 之后出现 -d 必须原样透传给引擎，不能被 gdit 解析。
func TestRunPassesDashDAfterSeparatorToEngine(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var launches []string
	original := spawnProcess
	spawnProcess = func(executable string, engineArgs []string, stdout, stderr io.Writer) int {
		launches = append(launches, strings.Join(engineArgs, " "))
		return 0
	}
	t.Cleanup(func() { spawnProcess = original })
	manager := fakeManager{
		launchTarget: gdit.LaunchTarget{ID: "4.5.2-standard", Executable: "/fake/godot"},
	}
	code := runWithManager(context.Background(), "", []string{"run", "-d", "--", "-d", "--path", "."}, &stdout, &stderr, manager, nil)
	if code != 0 || len(launches) != 1 || launches[0] != "-d --path ." {
		t.Fatalf("unexpected result: code=%d launches=%v stderr=%q", code, launches, stderr.String())
	}
}

// 多版本安装中某个参数解析失败：报错后继续处理其余参数，最终退出码 1。
func TestInstallMultipleVersionsParseFailureContinues(t *testing.T) {
	var stdout, stderr bytes.Buffer
	var requests []gdit.InstallRequest
	manager := fakeManager{
		installResult:   gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}},
		installRequests: &requests,
	}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2", "bad", "4.7.1"}, &stdout, &stderr, manager, nil)
	if code != 1 {
		t.Fatalf("parse failure must exit 1, got %d", code)
	}
	if len(requests) != 2 {
		t.Fatalf("parse failure must not block remaining versions: %+v", requests)
	}
	if !strings.Contains(stderr.String(), "version must be MAJOR.MINOR.PATCH") {
		t.Fatalf("missing parse error on stderr: %q", stderr.String())
	}
}

// 状态索引待重建时 CLI 必须把警告写 stderr，不污染 stdout 的结果行。
func TestInstallStateRebuildWarningOnStderr(t *testing.T) {
	var stdout, stderr bytes.Buffer
	manager := fakeManager{
		installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "4.5.2-standard"}, StateRebuildRequired: true},
	}
	code := runWithManager(context.Background(), "", []string{"install", "4.5.2"}, &stdout, &stderr, manager, nil)
	if code != 0 || stdout.String() != "installed 4.5.2-standard\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if !strings.Contains(stderr.String(), "state index will be rebuilt") {
		t.Fatalf("missing rebuild warning on stderr: %q", stderr.String())
	}
}
