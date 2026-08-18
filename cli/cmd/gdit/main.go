// Command gdit 提供 GoDoIt 的命令行入口。
package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/AlecAivazis/survey/v2"
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
}

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(run(ctx, os.Args[1:], os.Stdout, os.Stderr))
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
	return runWithManager(ctx, args, stdout, stderr, manager, renderer)
}

func runWithManager(ctx context.Context, args []string, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer) int {
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
	if flags.NArg() != 1 {
		fmt.Fprintln(stderr, "usage: gdit install [--edition standard|dotnet] [--source <name>] <version>")
		return 2
	}
	result, err := manager.Install(ctx, gdit.InstallRequest{Version: flags.Arg(0), Edition: *edition, Source: *sourceName})
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

// runInteractiveInstall 在终端下依次选择 edition、version 和 source 后安装。
// 非 TTY 环境返回用法错误，避免脚本卡死。
func runInteractiveInstall(ctx context.Context, stdout, stderr io.Writer, manager managerAPI, renderer *progressRenderer, defaultEdition, defaultSource string) int {
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		fmt.Fprintln(stderr, "interactive install requires a terminal; use: gdit install [--edition standard|dotnet] [--source <name>] <version>")
		return 2
	}
	var edition string
	if err := survey.AskOne(&survey.Select{
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
		if err := survey.AskOne(&survey.Select{
			Message: "选择版本",
			Options: options,
		}, &version); err != nil {
			fmt.Fprintln(stderr, err)
			return 1
		}
	} else {
		if err := survey.AskOne(&survey.Input{
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
	if err := survey.AskOne(&survey.Select{
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

// versionValidator 校验用户手动输入的版本号格式（MAJOR.MINOR.PATCH）。
func versionValidator(answer interface{}) error {
	value := strings.TrimSpace(fmt.Sprint(answer))
	parts := strings.Split(value, ".")
	if len(parts) != 3 {
		return fmt.Errorf("version must be MAJOR.MINOR.PATCH")
	}
	for _, part := range parts {
		if part == "" {
			return fmt.Errorf("version must be MAJOR.MINOR.PATCH")
		}
		for _, character := range part {
			if character < '0' || character > '9' {
				return fmt.Errorf("version must contain only digits")
			}
		}
	}
	return nil
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
	for _, version := range versions {
		fmt.Fprintf(stdout, "%s\t%s/%s\t%s\n", version.ID, version.Target.OS, version.Target.Arch, version.Source)
	}
	return 0
}

func writeUsage(writer io.Writer) {
	fmt.Fprintln(writer, "usage: gdit <command> [options]")
	fmt.Fprintln(writer, "commands: install (i), list (l), source (s), available (a)")
	fmt.Fprintln(writer, "  install [--edition|-e standard|dotnet] [--source|-s <name>] <version>")
	fmt.Fprintln(writer, "  source [list] | use|ban|unban <name>")
	fmt.Fprintln(writer, "  available [--source|-s <name>]")
}
