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
	"golang.org/x/sys/unix"
	"golang.org/x/term"

	gdit "github.com/Sheyiyuan/GoDoIt/core"
)

type managerAPI interface {
	Install(context.Context, gdit.InstallRequest) (gdit.InstallResult, error)
	List(context.Context) ([]gdit.InstalledVersion, error)
	Sources(context.Context) ([]gdit.SourceInfo, error)
	SetDefaultSource(context.Context, string) error
	SetSourceDisabled(context.Context, string, bool) error
	Available(context.Context, string) ([]gdit.AvailableVersion, error)
	Default(context.Context) (string, error)
	SetDefault(context.Context, string) error
	Remove(context.Context, string) error
	Setup(context.Context) error
	ResolveLaunch(context.Context, string) (gdit.LaunchTarget, error)
}

// stdinIsTTY 报告标准输入是否为终端；包级变量便于测试替换以触达交互分支。
var stdinIsTTY = func() bool { return term.IsTerminal(int(os.Stdin.Fd())) }

// stdoutIsTTY 报告标准输出是否为终端；包级变量便于测试替换以触达着色分支。
var stdoutIsTTY = func() bool { return term.IsTerminal(int(os.Stdout.Fd())) }

// askInteractive 用 stderr 渲染 survey 提示并读取用户输入，保持 stdout 只输出结果。
func askInteractive(prompt survey.Prompt, response interface{}, opts ...survey.AskOpt) error {
	return survey.AskOne(prompt, response, append(opts, survey.WithStdio(os.Stdin, os.Stderr, os.Stderr))...)
}

// replaceProcess 用引擎可执行文件替换当前进程（shim 路径，execve），
// 信号、stdio 和退出码天然透传；返回非 nil 表示替换失败。
var replaceProcess = func(executable string, engineArgs []string) error {
	return unix.Exec(executable, append([]string{executable}, engineArgs...), os.Environ())
}

// spawnProcess 以子进程方式启动引擎并返回其退出码（run 命令路径）。
// 收到 SIGINT/SIGTERM 时转发给子进程：终端的 Ctrl+C 会直达同进程组的子进程，
// 但外部 kill 只发给 gdit，NotifyContext 会吞掉信号，需要显式转发。
var spawnProcess = func(executable string, engineArgs []string, stdout, stderr io.Writer) int {
	command := exec.Command(executable, engineArgs...)
	command.Stdin = os.Stdin
	command.Stdout = stdout
	command.Stderr = stderr
	if err := command.Start(); err != nil {
		fmt.Fprintf(stderr, "launch %s: %v\n", executable, err)
		return 1
	}
	signals := make(chan os.Signal, 1)
	signal.Notify(signals, os.Interrupt, syscall.SIGTERM)
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
	fmt.Fprintf(stderr, "launch %s: %v\n", executable, err)
	return 1
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}

// isShimInvocation 报告进程是否以 godot 名称启动（argv[0] basename 判断）。
func isShimInvocation(argv0 string) bool {
	return filepath.Base(argv0) == "godot"
}

func run(ctx context.Context, args []string, stdout, stderr io.Writer) int {
	root, err := gdit.DefaultRoot()
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	renderer := newProgressRenderer(stderr)
	manager, err := gdit.New(gdit.Options{
		RootDir:  root,
		Progress: renderer.render,
	})
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	if isShimInvocation(os.Args[0]) {
		return runShim(ctx, args, stdout, stderr, manager)
	}
	return runWithManager(ctx, root, args, stdout, stderr, manager, renderer)
}

func runWithManager(ctx context.Context, root string, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	if len(args) == 0 {
		writeUsage(stderr)
		return 2
	}
	switch args[0] {
	case "install", "i":
		return runInstall(ctx, args[1:], stdout, stderr, manager, renderer)
	case "list", "l":
		return runList(ctx, args[1:], stdout, stderr, manager)
	case "source", "s":
		return runSource(ctx, args[1:], stdout, stderr, manager)
	case "available", "a":
		return runAvailable(ctx, args[1:], stdout, stderr, manager)
	case "default", "d":
		return runDefault(ctx, args[1:], stdout, stderr, manager)
	case "remove", "rm":
		return runRemove(ctx, args[1:], stdout, stderr, manager)
	case "setup", "st":
		return runSetup(ctx, root, args[1:], stdout, stderr, manager)
	case "run", "r":
		return runRun(ctx, args[1:], stdout, stderr, manager)
	case "help", "h", "-h", "--help":
		writeUsage(stdout)
		return 0
	default:
		fmt.Fprintf(stderr, "unknown command %q\n", args[0])
		writeUsage(stderr)
		return 2
	}
}

func runInstall(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
	flags := flag.NewFlagSet("install", flag.ContinueOnError)
	flags.SetOutput(stderr)
	edition := flags.String("edition", "standard", "Godot edition: standard or dotnet")
	flags.StringVar(edition, "e", "standard", "shorthand for --edition")
	sourceName := flags.String("source", "", "use only this source")
	flags.StringVar(sourceName, "s", "", "shorthand for --source")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() == 0 {
		if *sourceName != "" {
			if code := checkSourceArgument(ctx, stdout, stderr, manager, *sourceName); code != 0 {
				return code
			}
		}
		return runInteractiveInstall(ctx, stdout, stderr, manager, renderer, *edition, *sourceName)
	}
	explicitEdition := false
	flags.Visit(func(visited *flag.Flag) {
		if visited.Name == "edition" || visited.Name == "e" {
			explicitEdition = true
		}
	})
	for _, versionArg := range flags.Args() {
		if strings.HasPrefix(versionArg, "m") && explicitEdition {
			fmt.Fprintln(stderr, `the "m" version prefix cannot be combined with --edition`)
			return 2
		}
	}
	// 多版本参数逐个串行安装：每个版本独立解析与执行，任一失败不中断其余，
	// 最终按是否有失败汇总退出码。
	failed := false
	for _, versionArg := range flags.Args() {
		version, parsedEdition, err := gdit.ParseVersionArg(versionArg)
		if err != nil {
			renderer.clearLine()
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		requestEdition := *edition
		if strings.HasPrefix(versionArg, "m") {
			requestEdition = parsedEdition
		}
		result, err := manager.Install(ctx, gdit.InstallRequest{Version: version, Edition: requestEdition, Source: *sourceName})
		if err != nil {
			renderer.clearLine()
			fmt.Fprintln(stderr, err)
			failed = true
			continue
		}
		fmt.Fprintf(stdout, "installed %s\n", result.Version.ID)
		if result.StateRebuildRequired {
			fmt.Fprintln(stderr, "warning: state index will be rebuilt on the next read")
		}
	}
	if failed {
		return 1
	}
	return 0
}

// runInteractiveInstall 在终端下依次选择 edition、version 和 source 后安装。
// 非 TTY 环境返回用法错误，避免脚本卡死。交互提示一律渲染到 stderr。
func runInteractiveInstall(ctx context.Context, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer, defaultEdition, defaultSource string) int {
	if !stdinIsTTY() {
		fmt.Fprintln(stderr, "interactive install requires a terminal; use: gdit install [--edition standard|dotnet] [--source <name>] <version>")
		return 2
	}
	var edition string
	if err := askInteractive(&survey.Select{
		Message: "选择 edition",
		Options: []string{"standard", "dotnet"},
		Default: defaultEdition,
	}, &edition); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	versions, err := manager.Available(ctx, "")
	if err != nil {
		fmt.Fprintf(stderr, "warning: 无法枚举可用版本：%v\n", err)
		versions = nil
	}
	var version string
	options := make([]string, 0, len(versions))
	for _, item := range versions {
		for _, candidate := range item.Editions {
			if candidate == edition {
				options = append(options, item.Version)
				break
			}
		}
	}
	if len(options) > 0 {
		if err := askInteractive(&survey.Select{
			Message: "选择版本",
			Options: options,
		}, &version); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if err := askInteractive(&survey.Input{
			Message: "输入版本号（如 4.5.2）",
		}, &version, survey.WithValidator(versionValidator)); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	}
	infos, err := manager.Sources(ctx)
	sourceOptions := []string{"auto（按顺序 fallback）"}
	if err == nil {
		for _, info := range infos {
			if !info.Disabled {
				sourceOptions = append(sourceOptions, info.Name)
			}
		}
	} else {
		fmt.Fprintf(stderr, "warning: 无法读取来源列表：%v\n", err)
	}
	var source string
	if err := askInteractive(&survey.Select{
		Message: "选择来源",
		Options: sourceOptions,
		Default: defaultSourceChoice(defaultSource, sourceOptions),
	}, &source); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	requestSource := ""
	if source != sourceOptions[0] {
		requestSource = source
	}
	result, err := manager.Install(ctx, gdit.InstallRequest{Version: version, Edition: edition, Source: requestSource})
	if err != nil {
		renderer.clearLine()
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "installed %s\n", result.Version.ID)
	if result.StateRebuildRequired {
		fmt.Fprintln(stderr, "warning: state index will be rebuilt on the next read")
	}
	return 0
}

// checkSourceArgument 校验显式 --source 在交互流程前存在且未被禁用，
// 让用户在选择阶段之前就得到明确的配置错误，而不是静默回退到 auto。
func checkSourceArgument(ctx context.Context, stdout, stderr io.Writer, manager managerAPI, name string) int {
	infos, err := manager.Sources(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, info := range infos {
		if info.Name == name {
			if info.Disabled {
				fmt.Fprintf(stderr, "invalid config: source %q is disabled\n", name)
				return 1
			}
			return 0
		}
	}
	fmt.Fprintf(stderr, "invalid config: source %q is not configured\n", name)
	return 1
}

// versionValidator 校验用户手动输入的版本号格式，复用 core 的版本语法。
// 交互流程已先选择 edition，因此拒绝 m 前缀（带 m 会命中 core 的语法错误）。
func versionValidator(answer interface{}) error {
	value := strings.TrimSpace(fmt.Sprint(answer))
	if strings.HasPrefix(value, "m") {
		return fmt.Errorf("version must be MAJOR.MINOR.PATCH")
	}
	return gdit.ValidateVersion(value)
}

func defaultSourceChoice(defaultSource string, options []string) string {
	if defaultSource == "" {
		return options[0]
	}
	for _, option := range options {
		if option == defaultSource {
			return option
		}
	}
	return options[0]
}

func runSource(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) == 0 {
		return listSources(ctx, stdout, stderr, manager)
	}
	switch args[0] {
	case "list":
		return listSources(ctx, stdout, stderr, manager)
	case "use":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: gdit source use <name>")
			return 2
		}
		if err := manager.SetDefaultSource(ctx, args[1]); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "default source is now %s\n", args[1])
		return 0
	case "ban":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: gdit source ban <name>")
			return 2
		}
		if err := manager.SetSourceDisabled(ctx, args[1], true); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "source %s is disabled\n", args[1])
		return 0
	case "unban":
		if len(args) != 2 {
			fmt.Fprintln(stderr, "usage: gdit source unban <name>")
			return 2
		}
		if err := manager.SetSourceDisabled(ctx, args[1], false); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "source %s is enabled\n", args[1])
		return 0
	default:
		fmt.Fprintf(stderr, "unknown source command %q\n", args[0])
		return 2
	}
}

func listSources(ctx context.Context, stdout, stderr io.Writer, manager managerAPI) int {
	sources, err := manager.Sources(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for index, source := range sources {
		status := ""
		if source.Disabled {
			status = "\tdisabled"
		}
		fmt.Fprintf(stdout, "%d\t%s\t%s%s\n", index+1, source.Name, source.Kind, status)
	}
	return 0
}

func runAvailable(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	flags := flag.NewFlagSet("available", flag.ContinueOnError)
	flags.SetOutput(stderr)
	sourceName := flags.String("source", "", "source name; empty uses the configured order")
	flags.StringVar(sourceName, "s", "", "shorthand for --source")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(stderr, "usage: gdit available [--source <name>]")
		return 2
	}
	versions, err := manager.Available(ctx, *sourceName)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	for _, version := range versions {
		fmt.Fprintf(stdout, "%s\t%s\t%s\n", version.Version, strings.Join(version.Editions, ","), strings.Join(version.Sources, ","))
	}
	return 0
}

func runList(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: gdit list")
		return 2
	}
	versions, err := manager.List(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	// 读取默认版本失败（未设置或悬空）时有意降级为不标记任何版本，列表仍完整输出。
	current := ""
	if id, err := manager.Default(ctx); err == nil {
		current = id
	}
	for _, version := range versions {
		line := fmt.Sprintf("%s\t%s/%s\t%s", version.ID, version.Target.OS, version.Target.Arch, version.Source)
		if version.ID == current {
			line = defaultLine(line + "\tdefault")
		}
		fmt.Fprintln(stdout, line)
	}
	return 0
}

// runShim 处理以 godot 名称启动的分支：解析默认版本后用 execve 替换自身进程，
// 参数、stdio 和退出码原样透传给引擎。
func runShim(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	target, err := manager.ResolveLaunch(ctx, "")
	if err != nil {
		if errors.Is(err, gdit.ErrNoDefault) {
			fmt.Fprintln(stderr, `no default version set; run "gdit default <version>" first`)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	if err := replaceProcess(target.Executable, args); err != nil {
		fmt.Fprintf(stderr, "launch %s: %v\n", target.Executable, err)
		return 1
	}
	return 0 // 不可达：execve 成功不返回
}

func runDefault(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) > 1 {
		fmt.Fprintln(stderr, "usage: gdit default [<version>]")
		return 2
	}
	if len(args) == 1 {
		version, edition, err := gdit.ParseVersionArg(args[0])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		id := version + "-" + edition
		if err := manager.SetDefault(ctx, id); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		fmt.Fprintf(stdout, "default: %s\n", id)
		return 0
	}
	id, err := manager.Default(ctx)
	if err != nil {
		if errors.Is(err, gdit.ErrNoDefault) {
			fmt.Fprintln(stderr, `no default version set; run "gdit default <version>" first`)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	fmt.Fprintf(stdout, "default: %s\n", id)
	return 0
}

// runRemove 卸载指定版本。TTY 下默认交互确认（默认否）；非 TTY 下必须显式
// -y/--yes 跳过确认，避免脚本卡死或误删。交互提示渲染到 stderr。
func runRemove(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	flags := flag.NewFlagSet("remove", flag.ContinueOnError)
	flags.SetOutput(stderr)
	yes := flags.Bool("y", false, "skip confirmation")
	flags.BoolVar(yes, "yes", false, "skip confirmation")
	if err := flags.Parse(args); err != nil {
		return 2
	}
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: gdit remove [-y|--yes] <version>")
		return 2
	}
	version, edition, err := gdit.ParseVersionArg(flags.Arg(0))
	if err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	id := version + "-" + edition
	if !*yes {
		if !stdinIsTTY() {
			fmt.Fprintln(stderr, `remove requires confirmation; use "gdit remove -y <version>" in scripts`)
			return 2
		}
		var confirmed bool
		if err := askInteractive(&survey.Confirm{
			Message: fmt.Sprintf("remove %s?", id),
			Default: false,
		}, &confirmed); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		if !confirmed {
			fmt.Fprintln(stderr, "remove cancelled")
			return 0
		}
	}
	if err := manager.Remove(ctx, id); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	fmt.Fprintf(stdout, "removed %s\n", id)
	return 0
}

func runSetup(ctx context.Context, root string, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	if len(args) != 0 {
		fmt.Fprintln(stderr, "usage: gdit setup")
		return 2
	}
	if err := manager.Setup(ctx); err != nil {
		fmt.Fprintln(stderr, err)
		return 1
	}
	shimPath := filepath.Join(root, "bin", "godot")
	fmt.Fprintf(stdout, "godot shim ready at %s\n", shimPath)
	if !pathContainsDir(filepath.Dir(shimPath)) {
		fmt.Fprintf(stderr, "hint: add %s to PATH to use the godot command\n", filepath.Dir(shimPath))
	}
	return 0
}

// pathContainsDir 报告目录是否已存在于 PATH 中（忽略尾斜杠等变体）。
func pathContainsDir(directory string) bool {
	clean := filepath.Clean(directory)
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if filepath.Clean(entry) == clean {
			return true
		}
	}
	return false
}

// runRun 启动引擎：-d 启动默认版本，<版本> 显式启动指定版本（不改变默认），
// 无参数且终端可用时交互选择已安装版本后启动。-- 之后的参数原样透传给引擎，
// 不经过 gdit 解析；stdin/stdout/stderr 和退出码透传。
func runRun(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI) int {
	defaultVersion := false
	var positional, engineArgs []string
	afterSeparator := false
	for _, arg := range args {
		if afterSeparator {
			engineArgs = append(engineArgs, arg)
			continue
		}
		switch arg {
		case "--":
			afterSeparator = true
		case "-d":
			defaultVersion = true
		default:
			positional = append(positional, arg)
		}
	}
	if len(positional) > 1 {
		fmt.Fprintln(stderr, "usage: gdit run [-d|<version>] [-- <engine args>]")
		return 2
	}
	if defaultVersion && len(positional) == 1 {
		fmt.Fprintln(stderr, "cannot combine -d with a version")
		return 2
	}
	if len(positional) == 1 && strings.HasPrefix(positional[0], "-") {
		fmt.Fprintf(stderr, "unexpected flag %q; engine arguments must come after \"--\"\n", positional[0])
		return 2
	}
	id := ""
	switch {
	case defaultVersion:
	case len(positional) == 1:
		version, edition, err := gdit.ParseVersionArg(positional[0])
		if err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
		id = version + "-" + edition
	default:
		if len(engineArgs) > 0 {
			// 带参数但未指定版本：等价于 -d，启动默认版本。
		} else if !stdinIsTTY() {
			fmt.Fprintln(stderr, "interactive run requires a terminal; use: gdit run -d or gdit run <version>")
			return 2
		} else {
			id = interactiveRunVersion(ctx, stderr, manager)
			if id == "" {
				return 1
			}
		}
	}
	target, err := manager.ResolveLaunch(ctx, id)
	if err != nil {
		if errors.Is(err, gdit.ErrNoDefault) {
			fmt.Fprintln(stderr, `no default version set; run "gdit default <version>" first`)
		} else {
			fmt.Fprintln(stderr, err)
		}
		return 1
	}
	return spawnProcess(target.Executable, engineArgs, stdout, stderr)
}

// interactiveRunVersion 在终端下列出已安装版本（标记当前默认）供选择，返回选中的版本 ID。
// 交互失败或没有已安装版本时输出错误并返回空字符串。交互提示渲染到 stderr。
func interactiveRunVersion(ctx context.Context, stderr io.Writer, manager managerAPI) string {
	versions, err := manager.List(ctx)
	if err != nil {
		fmt.Fprintln(stderr, err)
		return ""
	}
	if len(versions) == 0 {
		fmt.Fprintln(stderr, `no versions installed; run "gdit install" first`)
		return ""
	}
	// 读取默认版本失败时有意降级为不标记任何版本，交互仍可用。
	current, _ := manager.Default(ctx)
	options := make([]string, 0, len(versions))
	for _, version := range versions {
		label := version.ID
		if version.ID == current {
			label += "（当前默认）"
		}
		options = append(options, label)
	}
	defaultChoice := ""
	if current != "" {
		defaultChoice = current + "（当前默认）"
	}
	var choice string
	if err := askInteractive(&survey.Select{
		Message: "选择要启动的版本",
		Options: options,
		Default: defaultChoice,
	}, &choice); err != nil {
		fmt.Fprintln(stderr, err)
		return ""
	}
	return strings.TrimSuffix(choice, "（当前默认）")
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: gdit <command> [options]")
	fmt.Fprintln(writer, "commands: install (i), list (l), source (s), available (a), default (d), remove (rm), setup (st), run (r)")
	fmt.Fprintln(writer, "  install [--edition|-e standard|dotnet] [--source|-s <name>] <version>...")
	fmt.Fprintln(writer, "  source [list] | use|ban|unban <name>")
	fmt.Fprintln(writer, "  available [--source|-s <name>]")
	fmt.Fprintln(writer, "  default [<version>]")
	fmt.Fprintln(writer, "  remove [-y|--yes] <version>")
	fmt.Fprintln(writer, "  setup")
	fmt.Fprintln(writer, "  run [-d|<version>] [-- <engine args>]")
}
