package main

import (
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/AlecAivazis/survey/v2"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

type fakeManager struct {
	installResult   gdit.InstallResult
	installRequests *[]gdit.InstallRequest
	entryResult     gdit.InstallEntryResult
	entryRequest    *gdit.InstallEntryRequest
	instances       []gdit.InstanceInfo
	defaultItem     gdit.InstanceInfo
	defaultErr      error
	setDefaultName  *string
	removeInstance  gdit.RemoveInstanceResult
	orphans         []gdit.OrphanAsset
	autoRemove      gdit.AutoRemoveResult
	sdks            []gdit.SDKInfo
	availableSDKs   []gdit.SDKChannel
	availableSDKErr error
	sdkInstall      gdit.SDKInstallResult
	sdkInstallVer   *string
	envView         gdit.EnvView
	envChange       *[]string
	launchTarget    gdit.LaunchTarget
	launchNames     *[]string
	sources         []gdit.SourceInfo
	available       []gdit.EngineChannel
	err             error
}

func (f fakeManager) Install(_ context.Context, request gdit.InstallRequest) (gdit.InstallResult, error) {
	if f.installRequests != nil {
		*f.installRequests = append(*f.installRequests, request)
	}
	return f.installResult, f.err
}
func (f fakeManager) List(context.Context) ([]gdit.InstalledVersion, error) { return nil, f.err }
func (f fakeManager) Sources(context.Context) ([]gdit.SourceInfo, error)    { return f.sources, f.err }
func (f fakeManager) SetDefaultSource(context.Context, string) error        { return f.err }
func (f fakeManager) SetSourceDisabled(context.Context, string, bool) error { return f.err }
func (f fakeManager) Available(context.Context, string) ([]gdit.EngineChannel, error) {
	return f.available, f.err
}
func (f fakeManager) InstallEntry(_ context.Context, request gdit.InstallEntryRequest) (gdit.InstallEntryResult, error) {
	if f.entryRequest != nil {
		*f.entryRequest = request
	}
	return f.entryResult, f.err
}
func (f fakeManager) RemoveInstance(context.Context, string) (gdit.RemoveInstanceResult, error) {
	return f.removeInstance, f.err
}
func (f fakeManager) Instances(context.Context) ([]gdit.InstanceInfo, error) {
	return f.instances, f.err
}
func (f fakeManager) Default(context.Context) (gdit.InstanceInfo, error) {
	return f.defaultItem, f.defaultErr
}
func (f fakeManager) SetDefault(_ context.Context, name string) error {
	if f.setDefaultName != nil {
		*f.setDefaultName = name
	}
	return f.err
}
func (f fakeManager) ResolveLaunch(_ context.Context, name string) (gdit.LaunchTarget, error) {
	if f.launchNames != nil {
		*f.launchNames = append(*f.launchNames, name)
	}
	return f.launchTarget, f.err
}
func (f fakeManager) Orphans(context.Context) ([]gdit.OrphanAsset, error) { return f.orphans, f.err }
func (f fakeManager) AutoRemove(context.Context) (gdit.AutoRemoveResult, error) {
	return f.autoRemove, f.err
}
func (f fakeManager) SDKs(context.Context) ([]gdit.SDKInfo, error) { return f.sdks, f.err }
func (f fakeManager) AvailableSDKs(context.Context) ([]gdit.SDKChannel, error) {
	if f.availableSDKErr != nil {
		return nil, f.availableSDKErr
	}
	return f.availableSDKs, f.err
}
func (f fakeManager) InstallSDK(_ context.Context, version string) (gdit.SDKInstallResult, error) {
	if f.sdkInstallVer != nil {
		*f.sdkInstallVer = version
	}
	return f.sdkInstall, f.err
}
func (f fakeManager) RemoveSDK(context.Context, string) error { return f.err }
func (f fakeManager) EffectiveEnv(context.Context, string) (gdit.EnvView, error) {
	return f.envView, f.err
}
func (f fakeManager) SetEnvVar(_ context.Context, name, key, value string) error {
	if f.envChange != nil {
		*f.envChange = append(*f.envChange, name, key, value)
	}
	return f.err
}
func (f fakeManager) UnsetEnvVar(_ context.Context, name, key string) error {
	if f.envChange != nil {
		*f.envChange = append(*f.envChange, name, key)
	}
	return f.err
}
func (f fakeManager) Remove(context.Context, string) error { return f.err }
func (f fakeManager) Setup(context.Context) error          { return f.err }

func TestInstallParsesEntryFlagsAfterName(t *testing.T) {
	var request gdit.InstallEntryRequest
	manager := fakeManager{entryRequest: &request, entryResult: gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}}}
	stdout, stderr, code := runCommand(t, manager, "install", "work", "--version", "4.5.2", "--edition", "dotnet", "--sdk", "managed", "--sdk-version", "8.0.410", "--no-current")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "installed instance work") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if request.Name != "work" || request.Version != "4.5.2" || request.Edition != "dotnet" || request.SDKVersion != "8.0.410" || request.SetCurrent == nil || *request.SetCurrent {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestInstallCurrentFlagsAreMutuallyExclusive(t *testing.T) {
	_, _, code := runCommand(t, fakeManager{}, "install", "work", "--version", "4.5.2", "--current", "--no-current")
	if code != 2 {
		t.Fatalf("expected usage exit, got %d", code)
	}
}

func TestInstallReportsAssetsPublishedBeforeLaterFailure(t *testing.T) {
	manager := fakeManager{
		entryResult: gdit.InstallEntryResult{Installed: []gdit.AssetChange{{Kind: "engine", ID: "4.5.2-dotnet"}}},
		err:         errors.New("SDK download failed"),
	}
	stdout, stderr, code := runCommand(t, manager, "install", "work", "--version", "4.5.2", "--edition", "dotnet", "--sdk-version", "8.0.410")
	if code != 1 || !strings.Contains(stdout, "installed engine 4.5.2-dotnet") || !strings.Contains(stderr, "SDK download failed") {
		t.Fatalf("partial install was not reported: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestListWritesInstancesAndCurrentMarker(t *testing.T) {
	manager := fakeManager{instances: []gdit.InstanceInfo{
		{Name: "work", Engine: "4.5.2-standard", Edition: "standard"},
		{Name: "csharp", Engine: "4.5.2-dotnet", Edition: "dotnet", SDKStrategy: "managed", SDK: "8.0.410", Current: true},
	}}
	stdout, stderr, code := runCommand(t, manager, "list")
	if code != 0 || stderr != "" || stdout != "work\t4.5.2-standard\tstandard\ncsharp\t4.5.2-dotnet\tdotnet\tmanaged:8.0.410\tcurrent\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestEngineInstallKeepsBatchAndMPrefix(t *testing.T) {
	var requests []gdit.InstallRequest
	manager := fakeManager{installRequests: &requests, installResult: gdit.InstallResult{Version: gdit.InstalledVersion{ID: "fixture"}}}
	_, stderr, code := runCommand(t, manager, "engine", "install", "4.5.2", "m4.6.2")
	if code != 0 || stderr != "" || len(requests) != 2 || requests[0].Edition != "standard" || requests[1].Edition != "dotnet" {
		t.Fatalf("unexpected requests: %+v code=%d stderr=%q", requests, code, stderr)
	}
}

func TestRunPrependsDerivedArgsAndPassesEnvironment(t *testing.T) {
	original := spawnProcess
	defer func() { spawnProcess = original }()
	var gotArgs, gotEnv []string
	spawnProcess = func(_ string, args, environment []string, _, _ io.Writer) int {
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), environment...)
		return 7
	}
	var names []string
	manager := fakeManager{launchNames: &names, launchTarget: gdit.LaunchTarget{Executable: "/engine", Args: []string{"--display-driver", "wayland"}, Env: []string{"A=B"}}}
	_, _, code := runCommand(t, manager, "run", "work", "--", "--display-driver", "x11", "-e")
	if code != 7 || !reflect.DeepEqual(names, []string{"work"}) || !reflect.DeepEqual(gotArgs, []string{"--display-driver", "wayland", "--display-driver", "x11", "-e"}) || !reflect.DeepEqual(gotEnv, []string{"A=B"}) {
		t.Fatalf("unexpected launch: code=%d names=%v args=%v env=%v", code, names, gotArgs, gotEnv)
	}
}

func TestRunWithoutArgsUsesCurrentWithoutTTYInteraction(t *testing.T) {
	original := spawnProcess
	defer func() { spawnProcess = original }()
	spawnProcess = func(string, []string, []string, io.Writer, io.Writer) int { return 0 }
	var names []string
	_, _, code := runCommand(t, fakeManager{launchNames: &names, launchTarget: gdit.LaunchTarget{Executable: "/engine"}}, "run")
	if code != 0 || !reflect.DeepEqual(names, []string{""}) {
		t.Fatalf("run must resolve current directly: code=%d names=%v", code, names)
	}
}

func TestRunWithoutArgsTTYPicksInstance(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{"选择要启动的条目": "game（当前）"},
		nil,
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	original := spawnProcess
	defer func() { spawnProcess = original }()
	spawnProcess = func(string, []string, []string, io.Writer, io.Writer) int { return 0 }
	var names []string
	manager := fakeManager{
		launchNames:  &names,
		launchTarget: gdit.LaunchTarget{Executable: "/engine"},
		instances: []gdit.InstanceInfo{
			{Name: "work", Engine: "4.7.2", Edition: "dotnet"},
			{Name: "game", Engine: "3.6.2", Edition: "standard", Current: true},
		},
	}
	_, _, code := runCommand(t, manager, "run")
	if code != 0 || !reflect.DeepEqual(names, []string{"game"}) {
		t.Fatalf("run must launch the picked instance: code=%d names=%v", code, names)
	}
}

func TestRunWithoutArgsTTYSingleInstanceSkipsMenu(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	asked := false
	askInteractive = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		asked = true
		return nil
	}
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	original := spawnProcess
	defer func() { spawnProcess = original }()
	spawnProcess = func(string, []string, []string, io.Writer, io.Writer) int { return 0 }
	var names []string
	manager := fakeManager{
		launchNames:  &names,
		launchTarget: gdit.LaunchTarget{Executable: "/engine"},
		instances:    []gdit.InstanceInfo{{Name: "work", Engine: "4.7.2", Edition: "standard"}},
	}
	_, _, code := runCommand(t, manager, "run")
	if code != 0 || asked || !reflect.DeepEqual(names, []string{"work"}) {
		t.Fatalf("the only instance must launch directly without the picker: code=%d asked=%v names=%v", code, asked, names)
	}
}

func TestRunWithoutArgsTTYNoInstancesGivesHint(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		t.Fatal("picker must not be shown with zero instances")
		return nil
	}
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	_, stderr, code := runCommand(t, fakeManager{}, "run")
	if code != 1 || !strings.Contains(stderr, `gdit install`) {
		t.Fatalf("expected an install hint for zero instances: code=%d stderr=%q", code, stderr)
	}
}

func TestSpawnProcessUsesProvidedEnvironment(t *testing.T) {
	script := filepath.Join(t.TempDir(), "print-env.sh")
	if err := os.WriteFile(script, []byte("#!/bin/sh\nprintf '%s' \"$GDIT_CHILD_ONLY\"\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	var stdout, stderr bytes.Buffer
	code := spawnProcess(script, nil, []string{"PATH=" + os.Getenv("PATH"), "GDIT_CHILD_ONLY=child"}, &stdout, &stderr)
	if code != 0 || stdout.String() != "child" || stderr.Len() != 0 {
		t.Fatalf("unexpected child result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
	if os.Getenv("GDIT_CHILD_ONLY") != "" {
		t.Fatal("spawn modified the parent environment")
	}
}

func TestEnvSetAcceptsInstanceFlagAfterAssignment(t *testing.T) {
	var change []string
	stdout, stderr, code := runCommand(t, fakeManager{envChange: &change}, "env", "set", "KEY=value", "--instance", "work")
	if code != 0 || stdout != "set KEY\n" || stderr != "" || !reflect.DeepEqual(change, []string{"work", "KEY", "value"}) {
		t.Fatalf("unexpected env change: code=%d stdout=%q stderr=%q change=%v", code, stdout, stderr, change)
	}
}

func TestAutoRemoveRequiresYesOutsideTTY(t *testing.T) {
	original := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	defer func() { stdinIsTTY = original }()
	manager := fakeManager{orphans: []gdit.OrphanAsset{{Kind: "engine", ID: "4.5.2-standard", Size: 1024}}}
	stdout, stderr, code := runCommand(t, manager, "autoremove")
	if code != 1 || !strings.Contains(stdout, "4.5.2-standard") || !strings.Contains(stderr, "requires confirmation") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	// 孤儿提示行走 stderr，stdout 只保留机器可读的数据行。
	if !strings.Contains(stderr, "以下资产已无引用") || strings.Contains(stdout, "以下资产已无引用") {
		t.Fatalf("orphan hint must be on stderr only: stdout=%q stderr=%q", stdout, stderr)
	}
}

func TestConfirmationPromptFailureReturnsError(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = func(survey.Prompt, interface{}, ...survey.AskOpt) error {
		return errors.New("prompt failed")
	}
	defer func() {
		stdinIsTTY = originalTTY
		askInteractive = originalAsk
	}()
	stdout, stderr, code := runCommand(t, fakeManager{}, "remove", "work")
	if code != 1 || stdout != "" || !strings.Contains(stderr, "prompt failed") {
		t.Fatalf("unexpected prompt failure result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestYesFlagAfterPositionalSkipsConfirmation(t *testing.T) {
	original := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	defer func() { stdinIsTTY = original }()
	manager := fakeManager{removeInstance: gdit.RemoveInstanceResult{Instance: gdit.InstanceInfo{Name: "work"}}}
	_, stderr, code := runCommand(t, manager, "remove", "work", "-y")
	if code != 0 || strings.Contains(stderr, "requires confirmation") {
		t.Fatalf("-y after positional must skip confirmation: code=%d stderr=%q", code, stderr)
	}
}

func TestAutoRemoveYesReportsActualLockedResult(t *testing.T) {
	manager := fakeManager{
		orphans:    []gdit.OrphanAsset{{Kind: "engine", ID: "old", Size: 1}},
		autoRemove: gdit.AutoRemoveResult{Removed: []gdit.OrphanAsset{{Kind: "engine", ID: "still-orphan", Size: 1}}},
	}
	stdout, stderr, code := runCommand(t, manager, "autoremove", "-y")
	if code != 0 || !strings.Contains(stdout, "removed engine still-orphan") || strings.Contains(stdout, "以下资产已无引用") || !strings.Contains(stderr, "以下资产已无引用") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestNoCurrentHintUsesInstanceLanguage(t *testing.T) {
	_, stderr, code := runCommand(t, fakeManager{defaultErr: gdit.ErrNoDefault}, "default")
	if code != 1 || !strings.Contains(stderr, "no current instance set") {
		t.Fatalf("unexpected result: code=%d stderr=%q", code, stderr)
	}
}

func TestCommandAliases(t *testing.T) {
	original := spawnProcess
	spawnProcess = func(string, []string, []string, io.Writer, io.Writer) int { return 0 }
	defer func() { spawnProcess = original }()
	for _, args := range [][]string{{"i", "work", "--version", "4.5.2"}, {"new", "work", "--version", "4.5.2"}, {"l"}, {"d"}, {"r"}, {"e"}, {"s"}, {"a"}, {"st"}} {
		manager := fakeManager{entryResult: gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}}, defaultItem: gdit.InstanceInfo{Name: "work"}, launchTarget: gdit.LaunchTarget{Executable: "/engine"}}
		_, _, code := runCommand(t, manager, args...)
		if code == 2 {
			t.Fatalf("alias %v was rejected", args)
		}
	}
}

func TestManagerErrorsGoToStderr(t *testing.T) {
	stdout, stderr, code := runCommand(t, fakeManager{err: errors.New("fixture failure")}, "list")
	if code != 1 || stdout != "" || stderr != "error: fixture failure\n" {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestRunSetupHintsWhenNotInPATH(t *testing.T) {
	t.Setenv("PATH", "/fixture/bin")
	var stdout, stderr bytes.Buffer
	code := runSetup(context.Background(), "/tmp/gdit-test", nil, &stdout, &stderr, fakeManager{})
	if code != 0 || !strings.Contains(stdout.String(), "godot shim ready at") || !strings.Contains(stderr.String(), "add /tmp/gdit-test/bin to PATH") {
		t.Fatalf("unexpected setup result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunSetupNoHintWhenBinInPATH(t *testing.T) {
	t.Setenv("PATH", "/tmp/gdit-test/bin:/fixture/bin")
	var stdout, stderr bytes.Buffer
	code := runSetup(context.Background(), "/tmp/gdit-test", nil, &stdout, &stderr, fakeManager{})
	if code != 0 || stderr.Len() != 0 {
		t.Fatalf("setup should not hint when bin is on PATH: code=%d stderr=%q", code, stderr.String())
	}
}

func TestRunSetupReportsManagerError(t *testing.T) {
	var stdout, stderr bytes.Buffer
	code := runSetup(context.Background(), "/tmp/gdit-test", nil, &stdout, &stderr, fakeManager{err: errors.New("setup failed")})
	if code != 1 || stdout.Len() != 0 || stderr.String() != "error: setup failed\n" {
		t.Fatalf("unexpected setup error result: code=%d stdout=%q stderr=%q", code, stdout.String(), stderr.String())
	}
}

func TestRunShimExecutesCurrentWithPassthroughArgs(t *testing.T) {
	original := replaceProcess
	defer func() { replaceProcess = original }()
	var gotArgs, gotEnv []string
	replaceProcess = func(_ string, args, environment []string) error {
		gotArgs = append([]string(nil), args...)
		gotEnv = append([]string(nil), environment...)
		return nil
	}
	var names []string
	manager := fakeManager{launchNames: &names, launchTarget: gdit.LaunchTarget{Executable: "/engine", Args: []string{"--display-driver", "wayland"}, Env: []string{"A=B"}}}
	var stderr bytes.Buffer
	code := runShim(context.Background(), []string{"-e", "--path", "."}, &stderr, manager)
	if code != 0 || !reflect.DeepEqual(names, []string{""}) || !reflect.DeepEqual(gotArgs, []string{"--display-driver", "wayland", "-e", "--path", "."}) || !reflect.DeepEqual(gotEnv, []string{"A=B"}) || stderr.Len() != 0 {
		t.Fatalf("unexpected shim launch: code=%d names=%v args=%v env=%v stderr=%q", code, names, gotArgs, gotEnv, stderr.String())
	}
}

func TestRunShimNoDefaultPrintsHint(t *testing.T) {
	var stderr bytes.Buffer
	code := runShim(context.Background(), nil, &stderr, fakeManager{err: gdit.ErrNoDefault})
	if code != 1 || !strings.Contains(stderr.String(), "no current instance set") {
		t.Fatalf("unexpected shim error: code=%d stderr=%q", code, stderr.String())
	}
}

func TestDefaultLineColoredOnlyOnTTY(t *testing.T) {
	t.Setenv("NO_COLOR", "")
	original := stdoutIsTTY
	stdoutIsTTY = func() bool { return false }
	plain := defaultLine("work")
	stdoutIsTTY = func() bool { return true }
	colored := defaultLine("work")
	stdoutIsTTY = original
	if plain != "work" || !strings.Contains(colored, "\x1b[") {
		t.Fatalf("unexpected default line rendering: plain=%q colored=%q", plain, colored)
	}
}

func TestEnvEqualsFlagSyntaxAccepted(t *testing.T) {
	var change []string
	stdout, stderr, code := runCommand(t, fakeManager{envChange: &change}, "env", "set", "KEY=value", "--instance=work")
	if code != 0 || stdout != "set KEY\n" || stderr != "" || !reflect.DeepEqual(change, []string{"work", "KEY", "value"}) {
		t.Fatalf("unexpected env change: code=%d stdout=%q stderr=%q change=%v", code, stdout, stderr, change)
	}
}

func TestEngineRemoveAcceptsAssetID(t *testing.T) {
	manager := fakeManager{}
	stdout, stderr, code := runCommand(t, manager, "engine", "remove", "-y", "4.5.2-dotnet")
	if code != 0 || !strings.Contains(stdout, "removed engine 4.5.2-dotnet") || stderr != "" {
		t.Fatalf("asset ID input should be accepted: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
}

func TestSDKInstallInteractiveWithoutTTYReturnsUsage(t *testing.T) {
	original := stdinIsTTY
	stdinIsTTY = func() bool { return false }
	defer func() { stdinIsTTY = original }()
	_, stderr, code := runCommand(t, fakeManager{}, "sdk", "install")
	if code != 2 || !strings.Contains(stderr, "requires a terminal") {
		t.Fatalf("unexpected result: code=%d stderr=%q", code, stderr)
	}
}

func TestSDKInstallInteractiveSelectsFromList(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{
			"选择 SDK 大版本": "8.0 (LTS)",
			"选择 SDK 版本":  "8.0.410",
		},
		nil,
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var installed string
	manager := fakeManager{
		availableSDKs: []gdit.SDKChannel{
			{MajorMinor: "10.0", Phase: "active", ReleaseType: "lts", Versions: []string{"10.0.400"}},
			{MajorMinor: "8.0", Phase: "maintenance", ReleaseType: "lts", Versions: []string{"8.0.424", "8.0.410"}},
		},
		sdkInstall:    gdit.SDKInstallResult{SDK: gdit.SDKInfo{Version: "8.0.410"}},
		sdkInstallVer: &installed,
	}
	stdout, stderr, code := runCommand(t, manager, "sdk", "install")
	if code != 0 || installed != "8.0.410" || !strings.Contains(stdout, "installed sdk 8.0.410") || stderr != "" {
		t.Fatalf("unexpected result: code=%d installed=%q stdout=%q stderr=%q", code, installed, stdout, stderr)
	}
}

func TestSDKInstallInteractiveFallsBackToTextInput(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		if inputPrompt, ok := prompt.(*survey.Input); ok && inputPrompt.Message == "输入 SDK 版本（如 8.0.410）" {
			*(response.(*string)) = "8.0.404"
		}
		return nil
	}
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var installed string
	manager := fakeManager{
		availableSDKErr: errors.New("metadata unavailable"),
		sdkInstall:      gdit.SDKInstallResult{SDK: gdit.SDKInfo{Version: "8.0.404"}},
		sdkInstallVer:   &installed,
	}
	stdout, stderr, code := runCommand(t, manager, "sdk", "install")
	if code != 0 || installed != "8.0.404" || !strings.Contains(stdout, "installed sdk 8.0.404") || !strings.Contains(stderr, "无法枚举可用 SDK") {
		t.Fatalf("unexpected result: code=%d installed=%q stdout=%q stderr=%q", code, installed, stdout, stderr)
	}
}

// answerByMessage 按 survey 提示消息分发交互回答，覆盖条目安装交互全流程。
func answerByMessage(selectAnswers map[string]string, inputAnswers map[string]string, confirmAnswer bool) func(survey.Prompt, interface{}, ...survey.AskOpt) error {
	return func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch p := prompt.(type) {
		case *survey.Select:
			if answer, ok := selectAnswers[p.Message]; ok {
				*(response.(*string)) = answer
			}
		case *survey.Input:
			if answer, ok := inputAnswers[p.Message]; ok {
				*(response.(*string)) = answer
			}
		case *survey.Confirm:
			*(response.(*bool)) = confirmAnswer
		}
		return nil
	}
}

func TestInteractiveEntryVersionSeriesSelection(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{
			"选择 edition": "standard",
			"选择版本系列":     "3.x",
			"选择版本":       "3.6.2",
		},
		map[string]string{
			"条目名": "work",
		},
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var request gdit.InstallEntryRequest
	manager := fakeManager{
		entryRequest: &request,
		entryResult:  gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}},
		available: []gdit.EngineChannel{
			{Name: "4.x", Versions: []gdit.AvailableVersion{
				{Version: "4.7.1", Editions: []string{"standard", "dotnet"}},
				{Version: "4.6.3", Editions: []string{"standard"}},
			}},
			{Name: "3.x", Versions: []gdit.AvailableVersion{
				{Version: "3.6.2", Editions: []string{"standard"}},
			}},
			{Name: "unstable", Versions: []gdit.AvailableVersion{
				{Version: "4.8-dev3", Editions: []string{"standard", "dotnet"}},
			}},
		},
	}
	_, _, code := runCommand(t, manager, "install")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if request.Version != "3.6.2" {
		t.Fatalf("picked engine version was not used: %+v", request)
	}
}

func TestInteractiveGodot3DotnetSkipsSDKStrategy(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	var seen []string
	askInteractive = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
		switch p := prompt.(type) {
		case *survey.Select:
			seen = append(seen, p.Message)
			switch p.Message {
			case "选择 edition":
				*(response.(*string)) = "dotnet"
			case "选择版本系列":
				*(response.(*string)) = "3.x"
			case "选择版本":
				*(response.(*string)) = "3.6.2"
			}
		case *survey.Input:
			*(response.(*string)) = "old"
		case *survey.Confirm:
			*(response.(*bool)) = false
		}
		return nil
	}
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var request gdit.InstallEntryRequest
	manager := fakeManager{
		entryRequest: &request,
		entryResult:  gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "old"}},
		available: []gdit.EngineChannel{
			{Name: "3.x", Versions: []gdit.AvailableVersion{
				{Version: "3.6.2", Editions: []string{"standard", "dotnet"}},
			}},
		},
	}
	stdout, stderr, code := runCommand(t, manager, "install")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	for _, message := range seen {
		if message == "SDK 策略" {
			t.Fatalf("SDK strategy must not be asked for Godot 3.x: %+v", seen)
		}
	}
	if request.Version != "3.6.2" || request.Edition != "dotnet" || request.SDKStrategy != "" {
		t.Fatalf("3.x dotnet entry must skip SDK strategy and leave it empty: %+v", request)
	}
	if !strings.Contains(stderr, "无需配置 SDK") {
		t.Fatalf("expected a hint about the system Mono runtime: %q", stderr)
	}
	_ = stdout
}

func TestInteractiveEntrySDKVersionRecommendedByDefault(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{
			"选择 edition": "dotnet",
			"SDK 策略":     "managed",
			"SDK 版本":     "推荐版本（默认）",
		},
		map[string]string{
			"条目名":            "work",
			"输入版本号（如 4.5.2）": "4.5.2",
		},
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var request gdit.InstallEntryRequest
	manager := fakeManager{entryRequest: &request, entryResult: gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}}}
	stdout, stderr, code := runCommand(t, manager, "install")
	if code != 0 || stderr != "" || !strings.Contains(stdout, "installed instance work") {
		t.Fatalf("unexpected result: code=%d stdout=%q stderr=%q", code, stdout, stderr)
	}
	if request.SDKVersion != "" {
		t.Fatalf("recommended choice must leave SDK version empty: %+v", request)
	}
}

func TestInteractiveEntrySDKVersionPickFromList(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{
			"选择 edition": "dotnet",
			"SDK 策略":     "managed",
			"SDK 版本":     "从可选列表选择",
			"选择 SDK 大版本": "8.0 (LTS)",
			"选择 SDK 版本":  "8.0.410",
		},
		map[string]string{
			"条目名":            "work",
			"输入版本号（如 4.5.2）": "4.5.2",
		},
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var request gdit.InstallEntryRequest
	manager := fakeManager{
		entryRequest: &request,
		entryResult:  gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}},
		availableSDKs: []gdit.SDKChannel{
			{MajorMinor: "10.0", Phase: "active", ReleaseType: "lts", Versions: []string{"10.0.400"}},
			{MajorMinor: "8.0", Phase: "maintenance", ReleaseType: "lts", Versions: []string{"8.0.410", "8.0.404"}},
		},
	}
	_, _, code := runCommand(t, manager, "install")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if request.SDKVersion != "8.0.410" {
		t.Fatalf("picked SDK version was not used: %+v", request)
	}
}

func TestInteractiveEntrySDKVersionManualInput(t *testing.T) {
	originalTTY := stdinIsTTY
	originalAsk := askInteractive
	stdinIsTTY = func() bool { return true }
	askInteractive = answerByMessage(
		map[string]string{
			"选择 edition": "dotnet",
			"SDK 策略":     "managed",
			"SDK 版本":     "手动输入",
		},
		map[string]string{
			"条目名":                  "work",
			"输入版本号（如 4.5.2）":       "4.5.2",
			"输入 SDK 版本（如 8.0.410）": "8.0.404",
		},
		false,
	)
	defer func() { stdinIsTTY = originalTTY; askInteractive = originalAsk }()
	var request gdit.InstallEntryRequest
	manager := fakeManager{entryRequest: &request, entryResult: gdit.InstallEntryResult{Instance: gdit.InstanceInfo{Name: "work"}}}
	_, _, code := runCommand(t, manager, "install")
	if code != 0 {
		t.Fatalf("unexpected exit code: %d", code)
	}
	if request.SDKVersion != "8.0.404" {
		t.Fatalf("manually entered SDK version was not used: %+v", request)
	}
}

func runCommand(t *testing.T, manager managerAPI, args ...string) (string, string, int) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := runWithManager(context.Background(), "/tmp/gdit-test", args, &stdout, &stderr, manager, nil)
	return stdout.String(), stderr.String(), code
}
