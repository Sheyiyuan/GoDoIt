// Command gdit 提供 GoDoIt 的命令行入口。
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"

	"github.com/AlecAivazis/survey/v2"
	"golang.org/x/term"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
	"github.com/Sheyiyuan/GoDoIt/core/buildinfo"
)

type managerAPI interface {
	Install(context.Context, gdit.InstallRequest) (gdit.InstallResult, error)
	List(context.Context) ([]gdit.InstalledVersion, error)
	Sources(context.Context) ([]gdit.SourceInfo, error)
	SetDefaultSource(context.Context, string) error
	SetSourceDisabled(context.Context, string, bool) error
	Available(context.Context, string) ([]gdit.EngineChannel, error)
	InstallEntry(context.Context, gdit.InstallEntryRequest) (gdit.InstallEntryResult, error)
	RemoveInstance(context.Context, string) (gdit.RemoveInstanceResult, error)
	Instances(context.Context) ([]gdit.InstanceInfo, error)
	Default(context.Context) (gdit.InstanceInfo, error)
	SetDefault(context.Context, string) error
	ResolveLaunch(context.Context, string) (gdit.LaunchTarget, error)
	Orphans(context.Context) ([]gdit.OrphanAsset, error)
	AutoRemove(context.Context) (gdit.AutoRemoveResult, error)
	SDKs(context.Context) ([]gdit.SDKInfo, error)
	AvailableSDKs(context.Context) ([]gdit.SDKChannel, error)
	InstallSDK(context.Context, string) (gdit.SDKInstallResult, error)
	RemoveSDK(context.Context, string) error
	EffectiveEnv(context.Context, string) (gdit.EnvView, error)
	SetEnvVar(context.Context, string, string, string) error
	UnsetEnvVar(context.Context, string, string) error
	Remove(context.Context, string) error
	Setup(context.Context) error
	Doctor(context.Context, bool) (gdit.DoctorReport, error)
}

type suggestionAPI interface {
	Suggest(context.Context, string) (gdit.ProjectSuggestion, error)
	InstallSuggestion(context.Context, gdit.InstallSuggestionRequest) (gdit.InstallSuggestionResult, error)
}

type templateAPI interface {
	Templates(context.Context) ([]gdit.TemplateInfo, error)
	InstallTemplate(context.Context, gdit.InstallTemplateRequest) (gdit.TemplateInfo, error)
	RemoveTemplate(context.Context, string, string) (gdit.TemplateInfo, error)
	AttachTemplate(context.Context, string, string) (gdit.TemplateBindingResult, error)
	DetachTemplate(context.Context, string) (gdit.TemplateBindingResult, error)
}

var stdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }
var stdoutIsTTY = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }
var resolveGUIExecutable = findGUIExecutable

var askInteractive = func(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(prompt, response, append(opts, survey.WithStdio(os.Stdin, os.Stderr, os.Stderr))...)
}

var spawnProcess = func(executable string, engineArgs, environment []string, stdout, stderr io.Writer) int {
	command := exec.Command(executable, engineArgs...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	command.Env = environment
	if command.Env == nil {
		command.Env = os.Environ()
	}
	if err := command.Start(); err != nil {
		writeErrorf(stderr, "launch %s: %v", executable, err)
		return 1
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM, syscall.SIGHUP, syscall.SIGQUIT)
	done := make(chan struct{})
	go func() {
		select {
		case received := <-signals:
			_ = command.Process.Signal(received)
		case <-done:
		}
		signal.Stop(signals)
	}()
	err := command.Wait()
	close(done)
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		if status, ok := exitErr.Sys().(syscall.WaitStatus); ok && status.Signaled() {
			return 128 + int(status.Signal())
		}
		return exitErr.ExitCode()
	}
	writeErrorf(stderr, "launch %s: %v", executable, err)
	return 1
}

// findGUIExecutable 按约定顺序查找与 CLI 配套的桌面 GUI 可执行文件。
func findGUIExecutable() (string, error) {
	if configured := strings.TrimSpace(os.Getenv("GDIT_GUI")); configured != "" {
		executable, err := resolveGUIPath(configured)
		if err != nil {
			return "", fmt.Errorf("GDIT_GUI 指向不可用的 GUI 可执行文件 %q: %w", configured, err)
		}
		return executable, nil
	}

	var candidates []string
	if executable, err := os.Executable(); err == nil {
		executableDir := filepath.Dir(executable)
		candidates = append(candidates, guiExecutableCandidates(executableDir)...)
		candidates = append(candidates, guiExecutableCandidates(filepath.Join(executableDir, "..", "gui", "build", "bin"))...)
	}
	if workingDir, err := os.Getwd(); err == nil {
		candidates = append(candidates, guiExecutableCandidates(filepath.Join(workingDir, "gui", "build", "bin"))...)
	}

	seen := make(map[string]struct{}, len(candidates))
	for _, candidate := range candidates {
		candidate = filepath.Clean(candidate)
		if _, ok := seen[candidate]; ok {
			continue
		}
		seen[candidate] = struct{}{}
		if executable, err := resolveGUIPath(candidate); err == nil {
			return executable, nil
		}
	}
	for _, name := range []string{"gdit-gui", "gdit-gui.exe"} {
		candidate, err := exec.LookPath(name)
		if err != nil {
			continue
		}
		if executable, err := resolveGUIPath(candidate); err == nil {
			return executable, nil
		}
	}
	return "", errors.New(`找不到 gdit-gui；请先运行 "make build-gui" 或设置 GDIT_GUI`)
}

func guiExecutableCandidates(directory string) []string {
	return []string{
		filepath.Join(directory, "gdit-gui"),
		filepath.Join(directory, "gdit-gui.exe"),
		filepath.Join(directory, "GoDoIt.app", "Contents", "MacOS", "gdit-gui"),
	}
}

func resolveGUIPath(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if info.Mode().IsRegular() {
		return path, nil
	}
	if info.IsDir() && strings.HasSuffix(strings.ToLower(filepath.Clean(path)), ".app") {
		appExecutable := filepath.Join(path, "Contents", "MacOS", "gdit-gui")
		appInfo, err := os.Stat(appExecutable)
		if err != nil {
			return "", err
		}
		if appInfo.Mode().IsRegular() {
			return appExecutable, nil
		}
	}
	return "", fmt.Errorf("路径不是可执行文件")
}

// runGUI 启动桌面 GUI，并把 GUI 参数和退出码原样交给调用方。
func runGUI(args []string, stdout, stderr io.Writer) int {
	executable, err := resolveGUIExecutable()
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	return spawnProcess(executable, args, nil, stdout, stderr)
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	if len(args) == 1 && (args[0] == "version" || args[0] == "--version") {
		writeVersion(stdout)
		return 0
	}
	root, err := gdit.DefaultRoot()
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	renderer := newProgressRenderer(stderr)
	manager, err := gdit.New(gdit.Options{RootDir: root, Progress: renderer.render})
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if isShimInvocation(os.Args[0]) {
		return runShim(ctx, args, stderr, manager)
	}
	return runWithManager(ctx, root, args, stdout, stderr, manager, renderer)
}

func runWithManager(ctx context.Context, root string, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	switch args[0] {
	case "install", "i", "new":
		return runInstall(ctx, args[1:], stdout, stderr, manager, renderer)
	case "__shim":
		// Windows：godot.cmd 包装调用 __shim 进入 shim 路径（参数直通引擎、不过命令解析）；
		// Unix 的 shim 靠 argv[0] 判断，此分支不经过（手动调用无害，行为一致）。
		return runShim(ctx, args[1:], stderr, manager)
	case "list", "l":
		return runList(ctx, args[1:], stdout, stderr, manager)
	case "default", "d":
		return runDefault(ctx, args[1:], stdout, stderr, manager)
	case "remove", "rm":
		return runRemoveInstance(ctx, args[1:], stdout, stderr, manager)
	case "run", "r":
		return runRun(ctx, args[1:], stdout, stderr, manager)
	case "gui":
		return runGUI(args[1:], stdout, stderr)
	case "engine":
		return runEngine(ctx, args[1:], stdout, stderr, manager, renderer)
	case "sdk":
		return runSDK(ctx, args[1:], stdout, stderr, manager, renderer)
	case "template":
		return runTemplate(ctx, args[1:], stdout, stderr, manager, renderer)
	case "suggest":
		return runSuggest(ctx, args[1:], stdout, stderr, manager, renderer)
	case "env", "e":
		return runEnv(ctx, args[1:], stdout, stderr, manager)
	case "autoremove":
		return runAutoRemove(ctx, args[1:], stdout, stderr, manager)
	case "source", "s":
		return runSource(ctx, args[1:], stdout, stderr, manager)
	case "available", "a":
		return runAvailable(ctx, args[1:], stdout, stderr, manager)
	case "setup", "st":
		return runSetup(ctx, root, args[1:], stdout, stderr, manager)
	case "doctor":
		return runDoctor(ctx, args[1:], stdout, stderr, manager)
	case "version", "--version":
		if len(args) != 1 {
			return usage(stderr, "gdit version")
		}
		writeVersion(stdout)
		return 0
	case "help", "h", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		writeErrorf(stderr, "unknown command %q", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runInstall(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if len(args) == 0 {
		return runInteractiveInstall(ctx, stdout, stderr, manager, renderer)
	}
	name := ""
	if !strings.HasPrefix(args[0], "-") {
		name, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	version := flags.String("version", "", "Godot 版本（必填）")
	edition := flags.String("edition", "standard", "standard 或 dotnet")
	flags.StringVar(edition, "e", "standard", "简写")
	sdk := flags.String("sdk", "", "managed 或 system")
	sdkVersion := flags.String("sdk-version", "", "托管 SDK 版本")
	current := flags.Bool("current", false, "设为当前条目")
	noCurrent := flags.Bool("no-current", false, "不改变 current")
	includeTemplate := flags.Bool("template", false, "安装并绑定导出模板")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if name == "" && flags.NArg() == 1 {
		name = flags.Arg(0)
	} else if flags.NArg() != 0 {
		return usage(stderr, "gdit install <name> --version <version> [options]")
	}
	if name == "" || *version == "" || *current && *noCurrent {
		return usage(stderr, "gdit install <name> --version <version> [--edition standard|dotnet] [--sdk managed|system] [--sdk-version <version>] [--current|--no-current]")
	}
	var setCurrent *bool
	if *current || *noCurrent {
		value := *current
		setCurrent = &value
	}
	result, err := manager.InstallEntry(ctx, gdit.InstallEntryRequest{Name: name, Version: *version, Edition: *edition, SDKStrategy: *sdk, SDKVersion: *sdkVersion, SetCurrent: setCurrent, Template: *includeTemplate})
	writeInstallEntryResult(stdout, stderr, result)
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	return 0
}

func runInteractiveInstall(ctx context.Context, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if !stdinIsTTY() {
		fmt.Fprintln(stderr, "interactive install requires a terminal; use: gdit install <name> --version <version>")
		return 2
	}
	var request gdit.InstallEntryRequest
	if err := askInteractive(&survey.Input{Message: "条目名", Default: "default"}, &request.Name, survey.WithValidator(func(answer interface{}) error {
		return gdit.ValidateInstanceName(strings.TrimSpace(fmt.Sprint(answer)))
	})); err != nil {
		writeError(stderr, err)
		return 1
	}
	if err := askInteractive(&survey.Select{Message: "选择 edition", Options: []string{"standard", "dotnet"}, Default: "standard"}, &request.Edition); err != nil {
		writeError(stderr, err)
		return 1
	}
	stop := startSpinner(stderr, "正在枚举可用版本…")
	channels, availableErr := manager.Available(ctx, "")
	stop()
	var choices []string
	if availableErr == nil {
		// 第一级：版本系列（4.x/3.x/unstable，只列该 edition 非空的组），第二级选具体版本。
		var groupNames []string
		groupVersions := make(map[string][]string)
		for _, channel := range channels {
			var versions []string
			for _, item := range channel.Versions {
				if contains(item.Editions, request.Edition) {
					versions = append(versions, item.Version)
				}
			}
			if len(versions) > 0 {
				groupNames = append(groupNames, channel.Name)
				groupVersions[channel.Name] = versions
			}
		}
		if len(groupNames) > 0 {
			pickedGroup := ""
			if err := askInteractive(&survey.Select{Message: "选择版本系列", Options: groupNames}, &pickedGroup); err != nil {
				writeError(stderr, err)
				return 1
			}
			choices = groupVersions[pickedGroup]
		}
	} else {
		fmt.Fprintf(stderr, "warning: 无法枚举可用版本：%v\n", availableErr)
	}
	if len(choices) > 0 {
		if err := askInteractive(&survey.Select{Message: "选择版本", Options: choices}, &request.Version); err != nil {
			writeError(stderr, err)
			return 1
		}
	} else if err := askInteractive(&survey.Input{Message: "输入版本号（如 4.5.2）"}, &request.Version, survey.WithValidator(versionValidator)); err != nil {
		writeError(stderr, err)
		return 1
	}
	if request.Edition == "dotnet" {
		if gdit.IsGodot3(request.Version) {
			// Godot 3.x mono：使用系统 Mono 运行时，无 SDK 概念，不询问策略。
			fmt.Fprintln(stderr, "info: Godot 3.x mono 使用系统 Mono 运行时，无需配置 SDK")
		} else if err := askInteractive(&survey.Select{Message: "SDK 策略", Options: []string{"managed", "system"}, Default: "managed"}, &request.SDKStrategy); err != nil {
			writeError(stderr, err)
			return 1
		}
		if request.SDKStrategy == "managed" {
			choice := ""
			if err := askInteractive(&survey.Select{
				Message: "SDK 版本",
				Options: []string{"推荐版本（默认）", "从可选列表选择", "手动输入"},
				Default: "推荐版本（默认）",
			}, &choice); err != nil {
				writeError(stderr, err)
				return 1
			}
			switch choice {
			case "从可选列表选择":
				if err := askSDKVersion(ctx, manager, stderr, &request.SDKVersion); err != nil {
					writeError(stderr, err)
					return 1
				}
			case "手动输入":
				if err := askInteractive(&survey.Input{Message: "输入 SDK 版本（如 8.0.410）"}, &request.SDKVersion, survey.WithValidator(versionValidator)); err != nil {
					writeError(stderr, err)
					return 1
				}
			default:
				// 推荐版本（默认）：留空，由 core 按映射表解析推荐 major 的最新 patch。
			}
		}
	}
	if err := askInteractive(&survey.Confirm{Message: "安装并绑定导出模板？", Default: false}, &request.Template); err != nil {
		writeError(stderr, err)
		return 1
	}
	_, currentErr := manager.Default(ctx)
	if currentErr != nil && !errors.Is(currentErr, gdit.ErrNoDefault) {
		writeError(stderr, currentErr)
		return 1
	}
	setCurrent := errors.Is(currentErr, gdit.ErrNoDefault)
	if err := askInteractive(&survey.Confirm{Message: "设为当前条目？", Default: setCurrent}, &setCurrent); err != nil {
		writeError(stderr, err)
		return 1
	}
	request.SetCurrent = &setCurrent
	result, err := manager.InstallEntry(ctx, request)
	writeInstallEntryResult(stdout, stderr, result)
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	return 0
}

func versionValidator(answer interface{}) error {
	return gdit.ValidateVersion(strings.TrimSpace(fmt.Sprint(answer)))
}

func runList(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) != 0 {
		return usage(stderr, "gdit list")
	}
	items, err := manager.Instances(ctx)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	for _, item := range items {
		line := fmt.Sprintf("%s\t%s\t%s", item.Name, item.Engine, item.Edition)
		if item.SDKStrategy != "" {
			line += "\t" + item.SDKStrategy
			if item.SDK != "" {
				line += ":" + item.SDK
			}
		}
		template := item.Template
		if template == "" {
			template = "-"
		} else if item.TemplateMissing {
			template += ":missing"
		}
		line += "\t" + template
		if item.Current {
			line = defaultLine(line + "\tcurrent")
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

func runDefault(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) > 1 {
		return usage(stderr, "gdit default [<name>]")
	}
	if len(args) == 1 {
		if err := gdit.ValidateInstanceName(args[0]); err != nil {
			writeError(stderr, err)
			return 1
		}
		if err := manager.SetDefault(ctx, args[0]); err != nil {
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "default: %s\n", args[0])
		return 0
	}
	item, err := manager.Default(ctx)
	if err != nil {
		writeDefaultError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "default: %s\t%s\t%s", item.Name, item.Engine, item.Edition)
	if item.SDKStrategy != "" {
		fmt.Fprintf(stdout, "\t%s", item.SDKStrategy)
	}
	fmt.Fprintln(stdout)
	return 0
}

func runRemoveInstance(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	name, yes, code := parseConfirmedTarget(args, "gdit remove [-y|--yes] <name>", stderr)
	if code != 0 {
		return code
	}
	if err := gdit.ValidateInstanceName(name); err != nil {
		writeError(stderr, err)
		return 1
	}
	if !yes {
		confirmed, confirmCode := confirm("remove instance "+name+"?", `remove requires confirmation; use "gdit remove -y <name>" in scripts`, stderr)
		if !confirmed {
			return confirmCode
		}
	}
	result, err := manager.RemoveInstance(ctx, name)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "removed instance %s\n", result.Instance.Name)
	writeOrphans(stdout, stderr, result.Orphans)
	return 0
}

func runEngine(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 1 {
			return usage(stderr, "gdit engine [list]")
		}
		versions, err := manager.List(ctx)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		refs, err := referencedEngines(ctx, manager)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for _, version := range versions {
			status := "orphan"
			if refs[version.ID] {
				status = "referenced"
			}
			fmt.Fprintf(stdout, "%s\t%s/%s\t%s\t%s\n", version.ID, version.Target.OS, version.Target.Arch, version.Source, status)
		}
		return 0
	}
	switch args[0] {
	case "install":
		return runEngineInstall(ctx, args[1:], stdout, stderr, manager, renderer)
	case "remove":
		versionArg, yes, code := parseConfirmedTarget(args[1:], "gdit engine remove [-y|--yes] <version>", stderr)
		if code != 0 {
			return code
		}
		// 接受资产 ID（如 4.5.2-dotnet，与 engine list 输出一致）或版本输入（4.5.2 / m4.5.2）。
		id := versionArg
		if !gdit.ValidEngineID(id) {
			version, edition, err := gdit.ParseVersionArg(versionArg)
			if err != nil {
				writeError(stderr, err)
				return 1
			}
			id = version + "-" + edition
		}
		if !yes {
			confirmed, confirmCode := confirm("remove engine "+id+"?", `engine remove requires confirmation; use -y in scripts`, stderr)
			if !confirmed {
				return confirmCode
			}
		}
		if err := manager.Remove(ctx, id); err != nil {
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "removed engine %s\n", id)
		return 0
	default:
		writeErrorf(stderr, "unknown engine command %q", args[0])
		return 2
	}
}

func runEngineInstall(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	flags := flag.NewFlagSet("engine install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	edition := flags.String("edition", "standard", "standard 或 dotnet")
	flags.StringVar(edition, "e", "standard", "简写")
	source := flags.String("source", "", "来源名")
	flags.StringVar(source, "s", "", "简写")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if flags.NArg() == 0 {
		return usage(stderr, "gdit engine install [options] <version>...")
	}
	for _, arg := range flags.Args() {
		if strings.HasPrefix(arg, "-") {
			fmt.Fprintln(stderr, `flags must be placed before versions, e.g. "gdit engine install --edition dotnet 4.5.2"`)
			return 2
		}
	}
	explicitEdition := false
	flags.Visit(func(item *flag.Flag) { explicitEdition = explicitEdition || item.Name == "edition" || item.Name == "e" })
	for _, arg := range flags.Args() {
		if strings.HasPrefix(arg, "m") && explicitEdition {
			fmt.Fprintln(stderr, `the "m" version prefix cannot be combined with --edition`)
			return 2
		}
	}
	failed := false
	for _, arg := range flags.Args() {
		version, parsedEdition, err := gdit.ParseVersionArg(arg)
		if err != nil {
			writeError(stderr, err)
			failed = true
			continue
		}
		selectedEdition := *edition
		if strings.HasPrefix(arg, "m") {
			selectedEdition = parsedEdition
		}
		result, err := manager.Install(ctx, gdit.InstallRequest{Version: version, Edition: selectedEdition, Source: *source})
		if err != nil {
			clearProgress(renderer)
			writeError(stderr, err)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "installed engine %s\n", result.Version.ID)
		writeStateWarning(stderr, result.StateRebuildRequired)
	}
	if failed {
		return 1
	}
	return 0
}

func runSDK(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 1 {
			return usage(stderr, "gdit sdk [list]")
		}
		items, err := manager.SDKs(ctx)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for _, item := range items {
			fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.Version, item.Kind, item.Path)
		}
		return 0
	}
	switch args[0] {
	case "available":
		if len(args) != 1 {
			return usage(stderr, "gdit sdk available")
		}
		channels, err := manager.AvailableSDKs(ctx)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for _, channel := range channels {
			fmt.Fprintf(stdout, "%s:\n", sdkChannelLabel(channel))
			for _, version := range channel.Versions {
				fmt.Fprintf(stdout, "  %s\n", version)
			}
		}
		return 0
	case "install":
		if len(args) == 1 {
			return runInteractiveSDKInstall(ctx, stdout, stderr, manager, renderer)
		}
		if len(args) != 2 {
			return usage(stderr, "gdit sdk install <version>")
		}
		result, err := manager.InstallSDK(ctx, args[1])
		if err != nil {
			clearProgress(renderer)
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "installed sdk %s\n", result.SDK.Version)
		writeStateWarning(stderr, result.StateRebuildRequired)
		return 0
	case "remove":
		version, yes, code := parseConfirmedTarget(args[1:], "gdit sdk remove [-y|--yes] <version>", stderr)
		if code != 0 {
			return code
		}
		if !yes {
			confirmed, confirmCode := confirm("remove SDK "+version+"?", `sdk remove requires confirmation; use -y in scripts`, stderr)
			if !confirmed {
				return confirmCode
			}
		}
		if err := manager.RemoveSDK(ctx, version); err != nil {
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "removed sdk %s\n", version)
		return 0
	default:
		writeErrorf(stderr, "unknown sdk command %q", args[0])
		return 2
	}
}

// sdkChannelLabel 生成 SDK 通道展示文本：如 "10.0 (LTS)"、"11.0 (Preview)"、"6.0 (EOL)"。
func sdkChannelLabel(channel gdit.SDKChannel) string {
	switch channel.Phase {
	case "eol":
		return channel.MajorMinor + " (EOL)"
	case "preview":
		return channel.MajorMinor + " (Preview)"
	}
	if channel.ReleaseType != "" {
		return channel.MajorMinor + " (" + strings.ToUpper(channel.ReleaseType) + ")"
	}
	return channel.MajorMinor
}

// askSDKVersion 交互式选择 SDK 版本：先选大版本通道，再选具体 patch；
// 枚举失败降级为文本输入（与条目安装的降级一致）。
func askSDKVersion(ctx context.Context, manager managerAPI, stderr io.Writer, target *string) error {
	stop := startSpinner(stderr, "正在枚举可用 SDK…")
	channels, enumErr := manager.AvailableSDKs(ctx)
	stop()
	if enumErr != nil || len(channels) == 0 {
		if enumErr != nil {
			fmt.Fprintf(stderr, "warning: 无法枚举可用 SDK：%v\n", enumErr)
		}
		return askInteractive(&survey.Input{Message: "输入 SDK 版本（如 8.0.410）"}, target, survey.WithValidator(versionValidator))
	}
	labels := make([]string, 0, len(channels))
	for _, channel := range channels {
		labels = append(labels, sdkChannelLabel(channel))
	}
	picked := ""
	if err := askInteractive(&survey.Select{Message: "选择 SDK 大版本", Options: labels}, &picked); err != nil {
		return err
	}
	var versions []string
	for index, channel := range channels {
		if labels[index] == picked {
			versions = channel.Versions
			break
		}
	}
	if len(versions) == 0 {
		return fmt.Errorf("SDK 通道 %s 无可用稳定版本", picked)
	}
	return askInteractive(&survey.Select{Message: "选择 SDK 版本", Options: versions}, target)
}

// runInteractiveSDKInstall 在 TTY 下枚举可选 SDK 版本供选择后安装；非 TTY 无参数报用法错误。
// 枚举失败降级为文本输入（与条目安装的降级一致）。
func runInteractiveSDKInstall(ctx context.Context, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if !stdinIsTTY() {
		fmt.Fprintln(stderr, `interactive sdk install requires a terminal; use: gdit sdk install <version>`)
		return 2
	}
	selected := ""
	if err := askSDKVersion(ctx, manager, stderr, &selected); err != nil {
		writeError(stderr, err)
		return 1
	}
	result, err := manager.InstallSDK(ctx, selected)
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed sdk %s\n", result.SDK.Version)
	writeStateWarning(stderr, result.StateRebuildRequired)
	return 0
}

func runTemplate(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	api, ok := manager.(templateAPI)
	if !ok {
		writeErrorf(stderr, "template capability is unavailable")
		return 1
	}
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 1 {
			return usage(stderr, "gdit template [list]")
		}
		items, err := api.Templates(ctx)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for _, item := range items {
			fmt.Fprintf(stdout, "%s\t%s\t%s\t%s\t%s\t%s\t%s\n", item.ID, item.Version, item.Edition, item.Source, formatBytes(item.Size), strings.Join(item.References, ","), item.InstalledAt)
		}
		return 0
	}
	switch args[0] {
	case "install":
		return runTemplateInstall(ctx, args[1:], stdout, stderr, api, renderer)
	case "remove":
		return runTemplateRemove(ctx, args[1:], stdout, stderr, api)
	case "attach":
		return runTemplateAttach(ctx, args[1:], stdout, stderr, api, renderer)
	case "detach":
		return runTemplateDetach(ctx, args[1:], stdout, stderr, api)
	default:
		return usage(stderr, "gdit template [list] | install|remove|attach|detach ...")
	}
}

func runTemplateInstall(ctx context.Context, args []string, stdout, stderr io.Writer, api templateAPI, renderer *progressRenderer) int {
	version := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		version, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("template install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	edition := flags.String("edition", "standard", "standard 或 dotnet")
	source := flags.String("source", "", "来源名")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if version == "" && flags.NArg() == 1 {
		version = flags.Arg(0)
	} else if flags.NArg() != 0 || version == "" {
		return usage(stderr, "gdit template install [--edition standard|dotnet] [--source <name>] <version>")
	}
	item, err := api.InstallTemplate(ctx, gdit.InstallTemplateRequest{Version: version, Edition: *edition, Source: *source})
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed template %s\t%s\n", item.ID, item.Path)
	return 0
}

func runTemplateRemove(ctx context.Context, args []string, stdout, stderr io.Writer, api templateAPI) int {
	flags := flag.NewFlagSet("template remove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	edition := flags.String("edition", "standard", "standard 或 dotnet")
	yes := flags.Bool("y", false, "跳过确认")
	flags.BoolVar(yes, "yes", false, "跳过确认")
	reordered, yesAfter := extractYesFlags(args)
	if err := flags.Parse(reordered); err != nil || flags.NArg() != 1 {
		return usage(stderr, "gdit template remove [-y|--yes] [--edition standard|dotnet] <version>")
	}
	if !(*yes || yesAfter) {
		confirmed, code := confirm("remove template "+flags.Arg(0)+"?", `template remove requires confirmation; use -y in scripts`, stderr)
		if !confirmed {
			return code
		}
	}
	item, err := api.RemoveTemplate(ctx, flags.Arg(0), *edition)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "removed template %s\n", item.ID)
	return 0
}

func runTemplateAttach(ctx context.Context, args []string, stdout, stderr io.Writer, api templateAPI, renderer *progressRenderer) int {
	flags := flag.NewFlagSet("template attach", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", "", "来源名")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if flags.NArg() != 1 {
		return usage(stderr, "gdit template attach [--source <name>] <name>")
	}
	result, err := api.AttachTemplate(ctx, flags.Arg(0), *source)
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "attached template %s to %s\n", result.Template.ID, result.Instance.Name)
	if result.Installed {
		fmt.Fprintf(stdout, "installed template %s\t%s\n", result.Template.ID, result.Template.Path)
	}
	return 0
}

func runTemplateDetach(ctx context.Context, args []string, stdout, stderr io.Writer, api templateAPI) int {
	if len(args) != 1 {
		return usage(stderr, "gdit template detach <name>")
	}
	result, err := api.DetachTemplate(ctx, args[0])
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "detached template from %s\n", result.Instance.Name)
	writeOrphans(stdout, stderr, result.Orphans)
	return 0
}

func runSuggest(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	api, ok := manager.(suggestionAPI)
	if !ok {
		writeErrorf(stderr, "suggest capability is unavailable")
		return 1
	}
	dir := ""
	if len(args) > 0 && !strings.HasPrefix(args[0], "-") {
		dir, args = args[0], args[1:]
	}
	flags := flag.NewFlagSet("suggest", flag.ContinueOnError)
	flags.SetOutput(stderr)
	install := flags.Bool("install", false, "安装建议条目")
	name := flags.String("name", "", "条目名")
	sdk := flags.String("sdk", "", "managed 或 system")
	sdkVersion := flags.String("sdk-version", "", "托管 SDK 精确版本")
	current := flags.Bool("current", false, "设为 current")
	noCurrent := flags.Bool("no-current", false, "不改变 current")
	noTemplate := flags.Bool("no-template", false, "不安装导出模板")
	handled, parsed := parseFlags(flags, args, stdout)
	if handled {
		if parsed {
			return 0
		}
		return 2
	}
	if flags.NArg() > 1 || dir != "" && flags.NArg() != 0 || *current && *noCurrent {
		return usage(stderr, "gdit suggest [<project-dir>] [--install --name <name>] [options]")
	}
	if dir == "" && flags.NArg() == 1 {
		dir = flags.Arg(0)
	}
	if dir == "" {
		dir = "."
	}
	if !*install && (*name != "" || *sdk != "" || *sdkVersion != "" || *current || *noCurrent || *noTemplate) {
		return usage(stderr, "suggest install options require --install")
	}
	suggestion, err := api.Suggest(ctx, dir)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	writeSuggestion(stdout, stderr, suggestion)
	if !suggestion.Installable {
		return 1
	}
	installNow := *install
	if !installNow && stdinIsTTY() {
		if err := askInteractive(&survey.Confirm{Message: "按建议安装条目和导出模板？", Default: false}, &installNow); err != nil {
			writeError(stderr, err)
			return 1
		}
	}
	if !installNow {
		return 0
	}
	if *name == "" {
		if !stdinIsTTY() {
			return usage(stderr, "gdit suggest <project-dir> --install --name <name>")
		}
		if err := askInteractive(&survey.Input{Message: "条目名"}, name, survey.WithValidator(func(answer interface{}) error {
			return gdit.ValidateInstanceName(strings.TrimSpace(fmt.Sprint(answer)))
		})); err != nil {
			writeError(stderr, err)
			return 1
		}
	}
	var setCurrent *bool
	if *current || *noCurrent {
		value := *current
		setCurrent = &value
	} else if stdinIsTTY() {
		value := false
		if err := askInteractive(&survey.Confirm{Message: "设为当前条目？", Default: false}, &value); err != nil {
			writeError(stderr, err)
			return 1
		}
		setCurrent = &value
	}
	includeTemplate := !*noTemplate
	result, err := api.InstallSuggestion(ctx, gdit.InstallSuggestionRequest{ProjectDir: dir, Name: *name, SDKStrategy: *sdk, SDKVersion: *sdkVersion, SetCurrent: setCurrent, IncludeTemplate: &includeTemplate})
	writeInstallEntryResult(stdout, stderr, result.Entry)
	if err != nil {
		clearProgress(renderer)
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "resolved engine %s\n", result.EngineVersion)
	if result.Template != nil {
		fmt.Fprintf(stdout, "template %s\t%s\n", result.Template.ID, result.Template.Path)
	}
	return 0
}

func writeSuggestion(stdout, stderr io.Writer, suggestion gdit.ProjectSuggestion) {
	fmt.Fprintf(stdout, "project_dir\t%s\nengine_series\t%s\nedition\t%s\nsdk_strategy\t%s\nsdk_version\t%s\nsdk_channel\t%s\n", suggestion.ProjectDir, suggestion.EngineSeries, suggestion.Edition, suggestion.SDKStrategy, suggestion.SDKVersion, suggestion.SDKChannel)
	for _, item := range suggestion.Evidence {
		fmt.Fprintf(stdout, "evidence\t%s\t%s\t%s\n", item.Kind, item.Path, item.Value)
	}
	for _, item := range suggestion.Diagnostics {
		fmt.Fprintf(stderr, "%s: %s", item.Level, item.Message)
		if item.Path != "" {
			fmt.Fprintf(stderr, " (%s)", item.Path)
		}
		fmt.Fprintln(stderr)
	}
}

func runEnv(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	instanceName, remaining, ok := extractInstanceFlag(args, stderr)
	if !ok {
		return 2
	}
	if len(remaining) == 0 {
		view, err := manager.EffectiveEnv(ctx, instanceName)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for _, variable := range view.Vars {
			fmt.Fprintf(stdout, "%s=%s\t%s\n", variable.Key, variable.Value, variable.Origin)
		}
		if len(view.Args) > 0 {
			fmt.Fprintf(stdout, "args\t%s\n", strings.Join(view.Args, " "))
		}
		return 0
	}
	switch remaining[0] {
	case "set":
		if len(remaining) != 2 {
			return usage(stderr, "gdit env set <KEY=VALUE> [--instance <name>]")
		}
		key, value, found := strings.Cut(remaining[1], "=")
		if !found {
			return usage(stderr, "gdit env set <KEY=VALUE> [--instance <name>]")
		}
		if err := manager.SetEnvVar(ctx, instanceName, key, value); err != nil {
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "set %s\n", key)
		return 0
	case "unset":
		if len(remaining) != 2 {
			return usage(stderr, "gdit env unset <KEY> [--instance <name>]")
		}
		if err := manager.UnsetEnvVar(ctx, instanceName, remaining[1]); err != nil {
			writeError(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "unset %s\n", remaining[1])
		return 0
	default:
		return usage(stderr, "gdit env [--instance <name>] | set|unset ...")
	}
}

func runAutoRemove(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	flags := flag.NewFlagSet("autoremove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	yes := flags.Bool("y", false, "跳过确认")
	flags.BoolVar(yes, "yes", false, "跳过确认")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return usage(stderr, "gdit autoremove [-y|--yes]")
	}
	orphans, err := manager.Orphans(ctx)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	if len(orphans) == 0 {
		fmt.Fprintln(stdout, "no orphan assets")
		return 0
	}
	writeOrphans(stdout, stderr, orphans)
	if !*yes {
		confirmed, confirmCode := confirm("remove these orphan assets?", `autoremove requires confirmation; use "gdit autoremove -y" in scripts`, stderr)
		if !confirmed {
			return confirmCode
		}
	}
	result, err := manager.AutoRemove(ctx)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	for _, item := range result.Removed {
		fmt.Fprintf(stdout, "removed %s %s\n", item.Kind, item.ID)
	}
	writeStateWarning(stderr, result.StateRebuildRequired)
	return 0
}

func runRun(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	var name string
	var engineArgs []string
	separator := -1
	for index, arg := range args {
		if arg == "--" {
			separator = index
			break
		}
	}
	front := args
	if separator >= 0 {
		front, engineArgs = args[:separator], args[separator+1:]
	}
	if len(front) > 1 || len(front) == 1 && strings.HasPrefix(front[0], "-") && front[0] != "-d" {
		return usage(stderr, "gdit run [<name>|-d] [-- <engine args>]")
	}
	if len(front) == 1 && front[0] != "-d" {
		name = front[0]
		if err := gdit.ValidateInstanceName(name); err != nil {
			writeError(stderr, err)
			return 1
		}
	}
	if len(front) == 0 && stdinIsTTY() {
		// 无参数 + TTY：多条目时交互选择；唯一条目直接启动；零条目给创建指引。
		instances, listErr := manager.Instances(ctx)
		if listErr != nil {
			writeError(stderr, listErr)
			return 1
		}
		switch {
		case len(instances) > 1:
			picked, pickErr := pickRunInstance(instances, stderr)
			if pickErr != nil {
				writeError(stderr, pickErr)
				return 1
			}
			name = picked
		case len(instances) == 1:
			name = instances[0].Name
		default:
			fmt.Fprintln(stderr, `no instances yet; create one with "gdit install" first`)
			return 1
		}
	}
	target, err := manager.ResolveLaunch(ctx, name)
	if err != nil {
		writeDefaultError(stderr, err)
		return 1
	}
	arguments := append(append([]string{}, target.Args...), engineArgs...)
	return spawnProcess(target.Executable, arguments, target.Env, stdout, stderr)
}

// pickRunInstance 在 TTY 下列出全部条目供选择，返回选中条目的显示名。
// 选项带引擎/edition/当前标记，当前条目标注「（当前）」。
func pickRunInstance(instances []gdit.InstanceInfo, stderr io.Writer) (string, error) {
	labels := make([]string, 0, len(instances))
	byLabel := make(map[string]string, len(instances))
	for _, item := range instances {
		label := item.Name
		if item.Current {
			label += "（当前）"
		}
		labels = append(labels, label)
		byLabel[label] = item.Name
	}
	selected := ""
	if err := askInteractive(&survey.Select{Message: "选择要启动的条目", Options: labels}, &selected); err != nil {
		return "", err
	}
	return byLabel[selected], nil
}

func runShim(ctx context.Context, args []string, stderr io.Writer, manager managerAPI) int {
	target, err := manager.ResolveLaunch(ctx, "")
	if err != nil {
		writeDefaultError(stderr, err)
		return 1
	}
	arguments := append(append([]string{}, target.Args...), args...)
	return launchEngine(target.Executable, arguments, target.Env, stderr)
}

func runSource(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) == 0 || args[0] == "list" {
		if len(args) > 1 {
			return usage(stderr, "gdit source [list]")
		}
		items, err := manager.Sources(ctx)
		if err != nil {
			writeError(stderr, err)
			return 1
		}
		for index, item := range items {
			status := ""
			if item.Disabled {
				status = "\tdisabled"
			}
			fmt.Fprintf(stdout, "%d\t%s\t%s%s\n", index+1, item.Name, item.Kind, status)
		}
		return 0
	}
	if len(args) != 2 {
		return usage(stderr, "gdit source use|ban|unban <name>")
	}
	var err error
	switch args[0] {
	case "use":
		err = manager.SetDefaultSource(ctx, args[1])
	case "ban":
		err = manager.SetSourceDisabled(ctx, args[1], true)
	case "unban":
		err = manager.SetSourceDisabled(ctx, args[1], false)
	default:
		return usage(stderr, "gdit source use|ban|unban <name>")
	}
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	switch args[0] {
	case "use":
		fmt.Fprintf(stdout, "default source is now %s\n", args[1])
	case "ban":
		fmt.Fprintf(stdout, "source %s is disabled\n", args[1])
	case "unban":
		fmt.Fprintf(stdout, "source %s is enabled\n", args[1])
	}
	return 0
}

func runAvailable(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	flags := flag.NewFlagSet("available", flag.ContinueOnError)
	flags.SetOutput(stderr)
	source := flags.String("source", "", "来源名")
	flags.StringVar(source, "s", "", "简写")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return usage(stderr, "gdit available [--source <name>]")
	}
	stop := startSpinner(stderr, "正在枚举可用版本…")
	channels, err := manager.Available(ctx, *source)
	stop()
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	for _, channel := range channels {
		fmt.Fprintf(stdout, "%s:\n", channel.Name)
		for _, item := range channel.Versions {
			fmt.Fprintf(stdout, "  %s\t%s\t%s\n", item.Version, strings.Join(item.Editions, ","), strings.Join(item.Sources, ","))
		}
	}
	return 0
}

func runSetup(ctx context.Context, root string, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) != 0 {
		return usage(stderr, "gdit setup")
	}
	if err := manager.Setup(ctx); err != nil {
		writeError(stderr, err)
		return 1
	}
	shim := filepath.Join(root, shimRelativePath())
	fmt.Fprintf(stdout, "godot shim ready at %s\n", shim)
	if !pathContainsDir(filepath.Dir(shim)) {
		fmt.Fprintf(stderr, "hint: %s\n", pathHint(filepath.Dir(shim)))
	}
	return 0
}

// runDoctor 执行环境诊断并渲染报告：结果写 stdout，逐项一行 [OK]/[WARN]/[ERROR]
// 前缀（非 TTY 也保持，机器可读）；退出码 0 = 无错误，1 = 存在错误，警告不影响。
func runDoctor(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(stderr)
	network := flags.Bool("network", false, "探测来源可达性")
	verbose := flags.Bool("verbose", false, "展开细节")
	handled, ok := parseFlags(flags, args, stdout)
	if handled {
		if ok {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		return usage(stderr, "gdit doctor [--network] [--verbose]")
	}
	report, err := manager.Doctor(ctx, *network)
	if err != nil {
		writeError(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "根目录：%s\n", report.Root)
	for _, item := range report.Items {
		fmt.Fprintf(stdout, "%s %s\n", doctorStatusPrefix(item.Status), item.Message)
		if *verbose {
			for _, detail := range item.Details {
				fmt.Fprintf(stdout, "      %s\n", detail)
			}
			if item.Suggest != "" {
				fmt.Fprintf(stdout, "      建议：%s\n", item.Suggest)
			}
		}
	}
	fmt.Fprintf(stdout, "%d 项正常，%d 项警告，%d 项错误\n", report.OKCount, report.WarnCount, report.ErrorCount)
	if report.ErrorCount > 0 {
		return 1
	}
	return 0
}

// doctorStatusPrefix 返回状态前缀（TTY 下着色；NO_COLOR 或非 TTY 纯文本）。
func doctorStatusPrefix(status gdit.CheckStatus) string {
	label := "[OK]"
	switch status {
	case gdit.StatusWarn:
		label = "[WARN]"
	case gdit.StatusError:
		label = "[ERROR]"
	}
	if os.Getenv("NO_COLOR") != "" || !stdoutIsTTY() {
		return label
	}
	switch status {
	case gdit.StatusWarn:
		return ansiYellow + label + ansiReset
	case gdit.StatusError:
		return ansiRed + label + ansiReset
	}
	return ansiGreen + label + ansiReset
}

func pathContainsDir(directory string) bool {
	for _, item := range filepath.SplitList(os.Getenv("PATH")) {
		if equalPath(item, directory) {
			return true
		}
	}
	return false
}

func parseConfirmedTarget(args []string, usageText string, stderr io.Writer) (string, bool, int) {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	yes := flags.Bool("y", false, "跳过确认")
	flags.BoolVar(yes, "yes", false, "跳过确认")
	// 允许 -y/--yes 出现在位置参数之后：先抽取确认 flag，其余交给标准解析。
	reordered, yesAfter := extractYesFlags(args)
	if err := flags.Parse(reordered); err != nil || flags.NArg() != 1 {
		return "", false, usage(stderr, usageText)
	}
	return flags.Arg(0), yesAfter || *yes, 0
}

// extractYesFlags 从参数列表中抽取 -y/--yes（允许出现在任意位置），返回剩余参数与是否出现。
func extractYesFlags(args []string) ([]string, bool) {
	result := make([]string, 0, len(args))
	yes := false
	for _, arg := range args {
		if arg == "-y" || arg == "--yes" {
			yes = true
			continue
		}
		result = append(result, arg)
	}
	return result, yes
}

func confirm(message, nonTTYHint string, stderr io.Writer) (bool, int) {
	if !stdinIsTTY() {
		fmt.Fprintln(stderr, nonTTYHint)
		return false, 1
	}
	var confirmed bool
	if err := askInteractive(&survey.Confirm{Message: message, Default: false}, &confirmed); err != nil {
		writeError(stderr, err)
		return false, 1
	}
	if !confirmed {
		fmt.Fprintln(stderr, "cancelled")
		return false, 1
	}
	return true, 0
}

func extractInstanceFlag(args []string, stderr io.Writer) (string, []string, bool) {
	remaining := make([]string, 0, len(args))
	name := ""
	for index := 0; index < len(args); index++ {
		arg := args[index]
		if arg == "--instance" {
			if index+1 >= len(args) || name != "" {
				fmt.Fprintln(stderr, "--instance requires one name")
				return "", nil, false
			}
			name = args[index+1]
			index++
			continue
		}
		if strings.HasPrefix(arg, "--instance=") {
			if name != "" {
				fmt.Fprintln(stderr, "--instance requires one name")
				return "", nil, false
			}
			name = strings.TrimPrefix(arg, "--instance=")
			if name == "" {
				fmt.Fprintln(stderr, "--instance requires one name")
				return "", nil, false
			}
			continue
		}
		remaining = append(remaining, arg)
	}
	return name, remaining, true
}

func writeOrphans(stdout, stderr io.Writer, items []gdit.OrphanAsset) {
	if len(items) == 0 {
		return
	}
	fmt.Fprintln(stderr, "以下资产已无引用，可用 gdit autoremove 清理")
	for _, item := range items {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", item.Kind, item.ID, formatBytes(item.Size))
	}
}

func referencedEngines(ctx context.Context, manager managerAPI) (map[string]bool, error) {
	result := make(map[string]bool)
	items, err := manager.Instances(ctx)
	if err != nil {
		return nil, err
	}
	for _, item := range items {
		result[item.Engine] = true
	}
	return result, nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func clearProgress(renderer *progressRenderer) {
	if renderer != nil {
		renderer.clearLine()
	}
}

func writeInstallEntryResult(stdout, stderr io.Writer, result gdit.InstallEntryResult) {
	if result.Instance.Name != "" {
		fmt.Fprintf(stdout, "installed instance %s\n", result.Instance.Name)
	}
	for _, asset := range result.Installed {
		fmt.Fprintf(stdout, "installed %s %s\n", asset.Kind, asset.ID)
	}
	writeStateWarning(stderr, result.StateRebuildRequired)
}

func writeStateWarning(stderr io.Writer, required bool) {
	if required {
		fmt.Fprintln(stderr, "warning: state index will be rebuilt on the next read")
	}
}

// writeError 统一输出错误：TTY 下红色 "error:" 前缀，非 TTY 或 NO_COLOR 时纯文本前缀。
func writeError(stderr io.Writer, err error) {
	writeErrorf(stderr, "%v", err)
}

// writeErrorf 是 writeError 的格式化变体，用于非 error 值但语义为错误的输出。
func writeErrorf(stderr io.Writer, format string, args ...any) {
	message := fmt.Sprintf(format, args...)
	if term.IsTerminal(int(os.Stderr.Fd())) && os.Getenv("NO_COLOR") == "" {
		fmt.Fprintf(stderr, "%serror:%s %s\n", ansiRed, ansiReset, message)
		return
	}
	fmt.Fprintf(stderr, "error: %s\n", message)
}

func writeDefaultError(stderr io.Writer, err error) {
	if errors.Is(err, gdit.ErrNoDefault) {
		writeErrorf(stderr, `no current instance set; run "gdit default <name>" first`)
	} else {
		writeError(stderr, err)
	}
}

func usage(stderr io.Writer, text string) int {
	fmt.Fprintln(stderr, "usage: "+text)
	return 2
}

// parseFlags 解析子命令 flags 并统一 --help/-h 行为：帮助文本写到 stdout（机器输出之外），
// 返回 (handled, ok)——handled 表示参数含帮助请求或解析失败，ok 表示解析成功。
func parseFlags(flags *flag.FlagSet, args []string, stdout io.Writer) (handled, ok bool) {
	flags.Usage = func() {
		flags.SetOutput(stdout)
		fmt.Fprintf(stdout, "usage: %s [options]\n", flags.Name())
		flags.PrintDefaults()
	}
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return true, true
		}
		return true, false
	}
	return false, true
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: gdit <command> [options]")
	fmt.Fprintln(writer, "commands: install (i), new, list (l), default (d), remove (rm), run (r), gui, engine, sdk, template, suggest, env (e), autoremove, source (s), available (a), setup (st), doctor, version")
	fmt.Fprintln(writer, "  install <name> --version <version> [--edition standard|dotnet] [--sdk managed|system] [--sdk-version <version>] [--current|--no-current] [--template]")
	fmt.Fprintln(writer, "  engine [list] | install [options] <version>... | remove [-y] <version>")
	fmt.Fprintln(writer, "  sdk [list] | available | install <version> | remove [-y] <version>")
	fmt.Fprintln(writer, "  template [list] | install|remove|attach|detach ...")
	fmt.Fprintln(writer, "  suggest [<project-dir>] [--install --name <name>] [options]")
	fmt.Fprintln(writer, "  env [--instance <name>] | set <KEY=VALUE> [--instance <name>] | unset <KEY> [--instance <name>]")
	fmt.Fprintln(writer, "  autoremove [-y|--yes]")
	fmt.Fprintln(writer, "  run [<name>|-d] [-- <engine args>]")
	fmt.Fprintln(writer, "  gui [arguments]")
	fmt.Fprintln(writer, "  doctor [--network] [--verbose]")
	fmt.Fprintln(writer, "  version")
}

func writeVersion(writer io.Writer) {
	info := buildinfo.Read()
	fmt.Fprintf(writer, "gdit %s\n", info.Version)
	if info.Commit != "" {
		fmt.Fprintf(writer, "commit %s\n", info.Commit)
	}
	if info.BuildDate != "" {
		fmt.Fprintf(writer, "built %s\n", info.BuildDate)
	}
	fmt.Fprintf(writer, "go %s\n", info.GoVersion)
}
