// Command gdit-release 生成并校验 GoDoIt 的三平台发布产物。
package main

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"github.com/Sheyiyuan/GoDoIt/core/internal/release"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

func run(args []string) int {
	if len(args) == 0 {
		writeUsage()
		return 2
	}
	var err error
	switch args[0] {
	case "notices":
		err = runNotices(args[1:])
	case "stage-gui":
		err = runStageGUI(args[1:])
	case "install-macos-legal":
		err = runInstallMacOSLegal(args[1:])
	case "package":
		err = runPackage(args[1:])
	case "verify-binaries":
		err = runVerifyBinaries(args[1:])
	case "verify-icons":
		err = runVerifyIcons(args[1:])
	case "checksums":
		err = runChecksums(args[1:])
	case "verify-final":
		err = runVerifyFinal(args[1:])
	case "help", "-h", "--help":
		writeUsage()
		return 0
	default:
		fmt.Fprintf(os.Stderr, "未知发布子命令 %q\n", args[0])
		writeUsage()
		return 2
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 1
	}
	return 0
}

func runNotices(args []string) error {
	flags := flag.NewFlagSet("notices", flag.ContinueOnError)
	root := flags.String("root", ".", "仓库根目录")
	metadata := flags.String("metadata", "scripts/third_party_licenses.json", "固定许可证元数据")
	output := flags.String("output", "THIRD_PARTY_NOTICES.txt", "输出文件")
	check := flags.Bool("check", false, "只校验现有输出")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("notices 不接受位置参数")
	}
	resolvedMetadata := resolveFromRoot(*root, *metadata)
	data, err := release.GenerateNotices(*root, resolvedMetadata)
	if err != nil {
		return err
	}
	resolvedOutput := resolveFromRoot(*root, *output)
	if *check {
		existing, err := os.ReadFile(resolvedOutput)
		if err != nil {
			return fmt.Errorf("读取现有第三方声明：%w", err)
		}
		if !bytes.Equal(existing, data) {
			return fmt.Errorf("%s 不是由当前锁文件和许可证元数据生成", resolvedOutput)
		}
		fmt.Fprintln(os.Stdout, resolvedOutput)
		return nil
	}
	if err := os.WriteFile(resolvedOutput, data, 0o644); err != nil {
		return fmt.Errorf("写入第三方声明：%w", err)
	}
	fmt.Fprintln(os.Stdout, resolvedOutput)
	return nil
}

func runStageGUI(args []string) error {
	flags := flag.NewFlagSet("stage-gui", flag.ContinueOnError)
	root := flags.String("root", ".", "仓库根目录")
	output := flags.String("output", "", "build/ 下的暂存目录")
	version := flags.String("version", "", "发布版本")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *output == "" || *version == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：stage-gui --root <root> --output <build/path> --version <version>")
	}
	if err := release.StageGUIProject(*root, resolveFromRoot(*root, *output), *version); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, resolveFromRoot(*root, *output))
	return nil
}

func runInstallMacOSLegal(args []string) error {
	flags := flag.NewFlagSet("install-macos-legal", flag.ContinueOnError)
	app := flags.String("app", "", "GoDoIt.app 路径")
	license := flags.String("license", "LICENSE", "AGPL 许可证路径")
	notices := flags.String("notices", "THIRD_PARTY_NOTICES.txt", "第三方声明路径")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *app == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：install-macos-legal --app <GoDoIt.app>")
	}
	return release.InstallMacOSLegal(*app, *license, *notices)
}

func runPackage(args []string) error {
	flags := flag.NewFlagSet("package", flag.ContinueOnError)
	root := flags.String("root", ".", "仓库根目录")
	platform := flags.String("platform", "", "发布平台")
	version := flags.String("version", "", "发布版本")
	cli := flags.String("cli", "", "CLI 可执行文件")
	gui := flags.String("gui", "", "GUI 可执行文件或应用包")
	license := flags.String("license", "LICENSE", "AGPL 许可证路径")
	notices := flags.String("notices", "THIRD_PARTY_NOTICES.txt", "第三方声明路径")
	output := flags.String("output", "", "归档输出路径")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *platform == "" || *version == "" || *cli == "" || *gui == "" || *output == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：package --platform <platform> --version <version> --cli <path> --gui <path> --output <path>")
	}
	timestamp, err := release.SourceDateFromEnvironment()
	if err != nil {
		return err
	}
	options := release.PackageOptions{
		Root:       *root,
		Platform:   *platform,
		Version:    *version,
		CLI:        *cli,
		GUI:        *gui,
		License:    resolveFromRoot(*root, *license),
		Notices:    resolveFromRoot(*root, *notices),
		Output:     *output,
		SourceDate: timestamp,
	}
	if err := release.PackagePlatform(options); err != nil {
		return err
	}
	fmt.Fprintln(os.Stdout, *output)
	return nil
}

func runVerifyBinaries(args []string) error {
	flags := flag.NewFlagSet("verify-binaries", flag.ContinueOnError)
	cli := flags.String("cli", "", "CLI 可执行文件")
	gui := flags.String("gui", "", "GUI 可执行文件")
	version := flags.String("version", "", "发布版本")
	commit := flags.String("commit", "", "Git commit")
	buildDate := flags.String("build-date", "", "UTC RFC3339 构建时间")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *cli == "" || *gui == "" || *version == "" || *commit == "" || *buildDate == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：verify-binaries --cli <path> --gui <path> --version <version> --commit <commit> --build-date <RFC3339>")
	}
	return release.VerifyBinaryIdentity(*cli, *gui, *version, *commit, *buildDate)
}

func runVerifyIcons(args []string) error {
	flags := flag.NewFlagSet("verify-icons", flag.ContinueOnError)
	source := flags.String("source", "assets/icon.png", "统一 PNG 图标")
	wails := flags.String("wails", "gui/build/appicon.png", "Wails PNG 图标")
	windows := flags.String("windows", "gui/build/windows/icon.ico", "Windows ICO 图标")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 {
		return fmt.Errorf("verify-icons 不接受位置参数")
	}
	return release.VerifyIcons(*source, *wails, *windows)
}

func runChecksums(args []string) error {
	flags := flag.NewFlagSet("checksums", flag.ContinueOnError)
	directory := flags.String("dir", "dist", "最终发布目录")
	version := flags.String("version", "", "发布版本")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：checksums --dir <dir> --version <version>")
	}
	return release.WriteChecksums(*directory, *version)
}

func runVerifyFinal(args []string) error {
	flags := flag.NewFlagSet("verify-final", flag.ContinueOnError)
	root := flags.String("root", ".", "仓库根目录")
	directory := flags.String("dir", "dist", "最终发布目录")
	version := flags.String("version", "", "发布版本")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *version == "" || flags.NArg() != 0 {
		return fmt.Errorf("用法：verify-final --root <root> --dir <dir> --version <version>")
	}
	return release.VerifyFinalRelease(*root, *directory, *version)
}

func resolveFromRoot(root, name string) string {
	if filepath.IsAbs(name) {
		return name
	}
	return filepath.Join(root, name)
}

func writeUsage() {
	fmt.Fprintln(os.Stderr, "usage: gdit-release <notices|stage-gui|install-macos-legal|package|verify-binaries|verify-icons|checksums|verify-final> [options]")
}
