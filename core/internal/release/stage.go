package release

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

const windowsVersionLanguageID = "0409"

// StageGUIProject 创建隔离的 Wails 发布工程并注入平台元数据。
func StageGUIProject(root, output, version string) error {
	if err := ValidateReleaseVersion(version); err != nil {
		return err
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return fmt.Errorf("解析仓库根目录：%w", err)
	}
	output, err = filepath.Abs(output)
	if err != nil {
		return fmt.Errorf("解析暂存目录：%w", err)
	}
	relative, err := filepath.Rel(root, output)
	if err != nil || relative == "build" || !strings.HasPrefix(relative, "build"+string(filepath.Separator)) {
		return fmt.Errorf("Wails 暂存目录必须位于仓库 build/ 下：%s", output)
	}
	if err := os.RemoveAll(output); err != nil {
		return fmt.Errorf("清理 Wails 暂存目录：%w", err)
	}
	for _, name := range []string{"go.work", "go.work.sum"} {
		if err := copyRegularFile(filepath.Join(root, name), filepath.Join(output, name)); err != nil {
			return err
		}
	}
	for _, module := range []string{"core", "cli"} {
		if err := copyTree(filepath.Join(root, module), filepath.Join(output, module), func(path string, entry os.DirEntry) bool {
			if entry.IsDir() {
				return true
			}
			return strings.HasSuffix(path, ".go") || filepath.Base(path) == "go.mod" || filepath.Base(path) == "go.sum"
		}); err != nil {
			return err
		}
	}
	guiSource := filepath.Join(root, "gui")
	guiOutput := filepath.Join(output, "gui")
	for _, name := range []string{"main.go", "go.mod", "go.sum"} {
		if err := copyRegularFile(filepath.Join(guiSource, name), filepath.Join(guiOutput, name)); err != nil {
			return err
		}
	}
	if err := copyTree(filepath.Join(guiSource, "bridge"), filepath.Join(guiOutput, "bridge"), func(path string, entry os.DirEntry) bool {
		return entry.IsDir() || strings.HasSuffix(path, ".go")
	}); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(guiSource, "frontend", "dist"), filepath.Join(guiOutput, "frontend", "dist"), nil); err != nil {
		return fmt.Errorf("复制前端产物（请先运行 frontend-build）：%w", err)
	}
	if err := copyTree(filepath.Join(guiSource, "build", "darwin"), filepath.Join(guiOutput, "build", "darwin"), nil); err != nil {
		return err
	}
	if err := copyTree(filepath.Join(guiSource, "build", "windows"), filepath.Join(guiOutput, "build", "windows"), func(path string, entry os.DirEntry) bool {
		return entry.IsDir() || filepath.Base(path) != "icon.ico"
	}); err != nil {
		return err
	}
	if err := copyRegularFile(filepath.Join(root, "assets", "icon.png"), filepath.Join(guiOutput, "build", "appicon.png")); err != nil {
		return err
	}
	if err := writeWailsConfig(filepath.Join(guiSource, "wails.json"), filepath.Join(guiOutput, "wails.json"), version); err != nil {
		return err
	}
	baseVersion, err := BaseVersion(version)
	if err != nil {
		return err
	}
	if err := writeWindowsResources(guiOutput, baseVersion, version); err != nil {
		return err
	}
	return nil
}

// InstallMacOSLegal 把离线法律文本写入应用包，供签名前验证和离线查看。
func InstallMacOSLegal(appBundle, licensePath, noticesPath string) error {
	info, err := os.Stat(appBundle)
	if err != nil || !info.IsDir() {
		return fmt.Errorf("无效 macOS 应用包 %s", appBundle)
	}
	legalDir := filepath.Join(appBundle, "Contents", "Resources", "legal")
	if err := copyRegularFile(licensePath, filepath.Join(legalDir, "LICENSE")); err != nil {
		return err
	}
	return copyRegularFile(noticesPath, filepath.Join(legalDir, "THIRD_PARTY_NOTICES.txt"))
}

func writeWailsConfig(source, destination, version string) error {
	data, err := os.ReadFile(source)
	if err != nil {
		return fmt.Errorf("读取 Wails 配置：%w", err)
	}
	var config map[string]any
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&config); err != nil {
		return fmt.Errorf("解析 Wails 配置：%w", err)
	}
	info, ok := config["info"].(map[string]any)
	if !ok {
		return fmt.Errorf("Wails 配置缺少 info 对象")
	}
	info["productVersion"] = version
	encoded, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 Wails 配置：%w", err)
	}
	encoded = append(encoded, '\n')
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	if err := os.WriteFile(destination, encoded, 0o644); err != nil {
		return fmt.Errorf("写入临时 Wails 配置：%w", err)
	}
	return nil
}

func writeWindowsResources(guiRoot, baseVersion, displayVersion string) error {
	windowsDir := filepath.Join(guiRoot, "build", "windows")
	manifest := fmt.Sprintf(`<?xml version="1.0" encoding="UTF-8" standalone="yes"?>
<assembly manifestVersion="1.0" xmlns="urn:schemas-microsoft-com:asm.v1" xmlns:asmv3="urn:schemas-microsoft-com:asm.v3">
    <assemblyIdentity type="win32" name="com.wails.GoDoIt" version="%s.0" processorArchitecture="*"/>
    <dependency>
        <dependentAssembly>
            <assemblyIdentity type="win32" name="Microsoft.Windows.Common-Controls" version="6.0.0.0" processorArchitecture="*" publicKeyToken="6595b64144ccf1df" language="*"/>
        </dependentAssembly>
    </dependency>
    <asmv3:application>
        <asmv3:windowsSettings>
            <dpiAware xmlns="http://schemas.microsoft.com/SMI/2005/WindowsSettings">true/pm</dpiAware>
            <dpiAwareness xmlns="http://schemas.microsoft.com/SMI/2016/WindowsSettings">permonitorv2,permonitor</dpiAwareness>
        </asmv3:windowsSettings>
    </asmv3:application>
</assembly>
`, baseVersion)
	if err := os.WriteFile(filepath.Join(windowsDir, "wails.exe.manifest"), []byte(manifest), 0o644); err != nil {
		return fmt.Errorf("生成 Windows manifest：%w", err)
	}
	resource := map[string]any{
		"fixed": map[string]string{
			"file_version":    baseVersion + ".0",
			"product_version": baseVersion + ".0",
		},
		"info": map[string]map[string]string{
			windowsVersionLanguageID: {
				"ProductVersion":  displayVersion,
				"CompanyName":     "Sheyiyuan",
				"FileDescription": "GoDoIt",
				"LegalCopyright":  "Copyright 2026 Sheyiyuan",
				"ProductName":     "GoDoIt",
				"Comments":        "Godot engine launcher and version manager",
			},
		},
	}
	data, err := json.MarshalIndent(resource, "", "  ")
	if err != nil {
		return fmt.Errorf("生成 Windows version resource：%w", err)
	}
	data = append(data, '\n')
	if err := os.WriteFile(filepath.Join(windowsDir, "info.json"), data, 0o644); err != nil {
		return fmt.Errorf("写入 Windows version resource：%w", err)
	}
	return nil
}

func copyTree(source, destination string, include func(string, os.DirEntry) bool) error {
	return filepath.WalkDir(source, func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, path)
		if err != nil {
			return err
		}
		if include != nil && !include(relative, entry) {
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if relative == "." {
			return os.MkdirAll(destination, 0o755)
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		return copyRegularFile(path, target)
	})
}

func copyRegularFile(source, destination string) error {
	info, err := os.Lstat(source)
	if err != nil {
		return fmt.Errorf("读取 %s：%w", source, err)
	}
	if !info.Mode().IsRegular() {
		return fmt.Errorf("发布输入不是普通文件：%s", source)
	}
	input, err := os.Open(source)
	if err != nil {
		return err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
		return err
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, info.Mode().Perm())
	if err != nil {
		return err
	}
	if _, err := io.Copy(output, input); err != nil {
		_ = output.Close()
		return err
	}
	if err := output.Close(); err != nil {
		return err
	}
	return nil
}
