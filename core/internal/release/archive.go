package release

import (
	"archive/tar"
	"archive/zip"
	"bufio"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"
)

const (
	// PlatformLinuxAMD64 是 Linux x86_64 发布平台标识。
	PlatformLinuxAMD64 = "linux_amd64"
	// PlatformDarwinARM64 是 macOS Apple Silicon 发布平台标识。
	PlatformDarwinARM64 = "darwin_arm64"
	// PlatformWindowsAMD64 是 Windows x86_64 发布平台标识。
	PlatformWindowsAMD64 = "windows_amd64"
)

var supportedPlatforms = map[string]bool{
	PlatformLinuxAMD64:   true,
	PlatformDarwinARM64:  true,
	PlatformWindowsAMD64: true,
}

// PackageOptions 描述一个原生平台归档的输入。
type PackageOptions struct {
	Root       string
	Platform   string
	Version    string
	CLI        string
	GUI        string
	License    string
	Notices    string
	Output     string
	SourceDate time.Time
}

type archiveInput struct {
	Source      string
	Destination string
}

// ArchiveName 返回平台发布归档的固定文件名。
func ArchiveName(version, platform string) (string, error) {
	if err := ValidateReleaseVersion(version); err != nil {
		return "", err
	}
	if !supportedPlatforms[platform] {
		return "", fmt.Errorf("不支持的发布平台 %q", platform)
	}
	extension := ".zip"
	if platform == PlatformLinuxAMD64 {
		extension = ".tar.gz"
	}
	return fmt.Sprintf("GoDoIt_%s_%s%s", version, platform, extension), nil
}

// PackagePlatform 生成确定性平台归档并立即复读验证。
func PackagePlatform(options PackageOptions) error {
	expectedName, err := ArchiveName(options.Version, options.Platform)
	if err != nil {
		return err
	}
	if filepath.Base(options.Output) != expectedName {
		return fmt.Errorf("归档名必须是 %s", expectedName)
	}
	rootName := strings.TrimSuffix(expectedName, ".zip")
	rootName = strings.TrimSuffix(rootName, ".tar.gz")
	inputs, err := packageInputs(options, rootName)
	if err != nil {
		return err
	}
	timestamp := options.SourceDate.UTC()
	if timestamp.IsZero() {
		timestamp = time.Unix(0, 0).UTC()
	}
	if err := os.MkdirAll(filepath.Dir(options.Output), 0o755); err != nil {
		return err
	}
	if options.Platform == PlatformLinuxAMD64 {
		err = writeTarGzip(options.Output, inputs, timestamp)
	} else {
		err = writeZip(options.Output, inputs, timestamp)
	}
	if err != nil {
		return err
	}
	return verifyPlatformArchive(options.Output, options.Root, options.Platform, options.Version, options.License, options.Notices)
}

// SourceDateFromEnvironment 解析可复现构建使用的 SOURCE_DATE_EPOCH。
func SourceDateFromEnvironment() (time.Time, error) {
	value := strings.TrimSpace(os.Getenv("SOURCE_DATE_EPOCH"))
	if value == "" {
		return time.Time{}, nil
	}
	seconds, err := strconv.ParseInt(value, 10, 64)
	if err != nil || seconds < 0 {
		return time.Time{}, fmt.Errorf("无效 SOURCE_DATE_EPOCH %q", value)
	}
	return time.Unix(seconds, 0).UTC(), nil
}

func packageInputs(options PackageOptions, rootName string) ([]archiveInput, error) {
	for label, filename := range map[string]string{"CLI": options.CLI, "GUI": options.GUI, "LICENSE": options.License, "THIRD_PARTY_NOTICES": options.Notices} {
		if strings.TrimSpace(filename) == "" {
			return nil, fmt.Errorf("缺少 %s 输入", label)
		}
	}
	cliName, guiName := "gdit", "gdit-gui"
	if options.Platform == PlatformWindowsAMD64 {
		cliName, guiName = "gdit.exe", "gdit-gui.exe"
	}
	if options.Platform == PlatformDarwinARM64 {
		guiName = "GoDoIt.app"
	}
	inputs := []archiveInput{
		{Source: options.CLI, Destination: path.Join(rootName, cliName)},
		{Source: options.GUI, Destination: path.Join(rootName, guiName)},
		{Source: options.License, Destination: path.Join(rootName, "LICENSE")},
		{Source: options.Notices, Destination: path.Join(rootName, "THIRD_PARTY_NOTICES.txt")},
	}
	var expanded []archiveInput
	for _, input := range inputs {
		info, err := os.Lstat(input.Source)
		if err != nil {
			return nil, fmt.Errorf("读取发布输入 %s：%w", input.Source, err)
		}
		if info.Mode().IsRegular() {
			expanded = append(expanded, input)
			continue
		}
		if !info.IsDir() {
			return nil, fmt.Errorf("发布输入不是普通文件或目录：%s", input.Source)
		}
		err = filepath.WalkDir(input.Source, func(filename string, entry os.DirEntry, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			relative, err := filepath.Rel(input.Source, filename)
			if err != nil {
				return err
			}
			destination := input.Destination
			if relative != "." {
				destination = path.Join(destination, filepath.ToSlash(relative))
			}
			if entry.Type()&os.ModeSymlink != 0 {
				return fmt.Errorf("发布目录含符号链接：%s", filename)
			}
			if !entry.IsDir() {
				info, err := entry.Info()
				if err != nil {
					return err
				}
				if !info.Mode().IsRegular() {
					return fmt.Errorf("发布目录含特殊文件：%s", filename)
				}
			}
			expanded = append(expanded, archiveInput{Source: filename, Destination: destination})
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	sort.Slice(expanded, func(i, j int) bool { return expanded[i].Destination < expanded[j].Destination })
	seen := make(map[string]bool, len(expanded))
	for _, input := range expanded {
		if seen[input.Destination] {
			return nil, fmt.Errorf("归档路径重复：%s", input.Destination)
		}
		seen[input.Destination] = true
		if err := validateArchivePath(input.Destination); err != nil {
			return nil, err
		}
		if err := rejectWorkspacePath(input.Source, options.Root); err != nil {
			return nil, err
		}
	}
	return expanded, nil
}

func writeTarGzip(destination string, inputs []archiveInput, timestamp time.Time) error {
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	gzipWriter := gzip.NewWriter(output)
	gzipWriter.Header.ModTime = timestamp
	gzipWriter.Header.OS = 255
	tarWriter := tar.NewWriter(gzipWriter)
	for _, input := range inputs {
		info, err := os.Stat(input.Source)
		if err != nil {
			return closeArchiveWriters(tarWriter, gzipWriter, output, err)
		}
		header, err := tar.FileInfoHeader(info, "")
		if err != nil {
			return closeArchiveWriters(tarWriter, gzipWriter, output, err)
		}
		header.Name = input.Destination
		header.ModTime = timestamp
		header.AccessTime = time.Time{}
		header.ChangeTime = time.Time{}
		header.Uid, header.Gid = 0, 0
		header.Uname, header.Gname = "", ""
		if err := tarWriter.WriteHeader(header); err != nil {
			return closeArchiveWriters(tarWriter, gzipWriter, output, err)
		}
		if info.Mode().IsRegular() {
			if err := copyFileToWriter(tarWriter, input.Source); err != nil {
				return closeArchiveWriters(tarWriter, gzipWriter, output, err)
			}
		}
	}
	return closeArchiveWriters(tarWriter, gzipWriter, output, nil)
}

func closeArchiveWriters(tarWriter *tar.Writer, gzipWriter *gzip.Writer, output *os.File, current error) error {
	for _, close := range []func() error{tarWriter.Close, gzipWriter.Close, output.Close} {
		if err := close(); current == nil && err != nil {
			current = err
		}
	}
	return current
}

func writeZip(destination string, inputs []archiveInput, timestamp time.Time) error {
	if timestamp.Year() < 1980 {
		timestamp = time.Date(1980, time.January, 1, 0, 0, 0, 0, time.UTC)
	}
	output, err := os.OpenFile(destination, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	writer := zip.NewWriter(output)
	for _, input := range inputs {
		info, err := os.Stat(input.Source)
		if err != nil {
			return closeZip(writer, output, err)
		}
		name := input.Destination
		if info.IsDir() {
			name += "/"
		}
		header := &zip.FileHeader{Name: name, Method: zip.Deflate}
		header.SetModTime(timestamp)
		header.SetMode(info.Mode())
		entry, err := writer.CreateHeader(header)
		if err != nil {
			return closeZip(writer, output, err)
		}
		if info.Mode().IsRegular() {
			if err := copyFileToWriter(entry, input.Source); err != nil {
				return closeZip(writer, output, err)
			}
		}
	}
	return closeZip(writer, output, nil)
}

func closeZip(writer *zip.Writer, output *os.File, current error) error {
	if err := writer.Close(); current == nil && err != nil {
		current = err
	}
	if err := output.Close(); current == nil && err != nil {
		current = err
	}
	return current
}

func copyFileToWriter(destination io.Writer, source string) error {
	file, err := os.Open(source)
	if err != nil {
		return err
	}
	defer file.Close()
	_, err = io.Copy(destination, file)
	return err
}

func rejectWorkspacePath(filename, root string) error {
	info, err := os.Stat(filename)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return err
	}
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	data, err := os.ReadFile(filename)
	if err != nil {
		return err
	}
	if bytes.Contains(data, []byte(absoluteRoot)) || bytes.Contains(data, []byte(filepath.ToSlash(absoluteRoot))) {
		return fmt.Errorf("发布输入包含绝对工作区路径：%s", filename)
	}
	return nil
}

// WriteChecksums 对精确的三个平台归档生成 GNU sha256sum 兼容清单。
func WriteChecksums(directory, version string) error {
	expected, err := expectedArchiveNames(version)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	allowed := make(map[string]bool, len(expected)+1)
	for _, name := range expected {
		allowed[name] = true
	}
	allowed["SHA256SUMS"] = true
	for _, entry := range entries {
		if !allowed[entry.Name()] || entry.IsDir() {
			return fmt.Errorf("最终发布目录含意外项目：%s", entry.Name())
		}
	}
	for _, name := range expected {
		if info, err := os.Stat(filepath.Join(directory, name)); err != nil || !info.Mode().IsRegular() {
			return fmt.Errorf("缺少平台归档 %s", name)
		}
	}
	var output strings.Builder
	for _, name := range expected {
		digest, err := fileSHA256(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&output, "%s  %s\n", digest, name)
	}
	return os.WriteFile(filepath.Join(directory, "SHA256SUMS"), []byte(output.String()), 0o644)
}

// VerifyFinalRelease 校验最终目录白名单、摘要和每个平台归档内容。
func VerifyFinalRelease(root, directory, version string) error {
	expected, err := expectedArchiveNames(version)
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	if len(entries) != len(expected)+1 {
		return fmt.Errorf("最终发布目录项目数为 %d，期望 %d", len(entries), len(expected)+1)
	}
	seen := make(map[string]bool, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || seen[entry.Name()] {
			return fmt.Errorf("最终发布目录含目录或重复项目：%s", entry.Name())
		}
		seen[entry.Name()] = true
	}
	if !seen["SHA256SUMS"] {
		return fmt.Errorf("最终发布目录缺少 SHA256SUMS")
	}
	checksums, err := readChecksums(filepath.Join(directory, "SHA256SUMS"))
	if err != nil {
		return err
	}
	for _, platform := range []string{PlatformLinuxAMD64, PlatformDarwinARM64, PlatformWindowsAMD64} {
		name, _ := ArchiveName(version, platform)
		if !seen[name] {
			return fmt.Errorf("最终发布目录缺少 %s", name)
		}
		digest, err := fileSHA256(filepath.Join(directory, name))
		if err != nil {
			return err
		}
		if checksums[name] != digest {
			return fmt.Errorf("%s 的 SHA-256 不一致", name)
		}
		if err := verifyPlatformArchive(filepath.Join(directory, name), root, platform, version, filepath.Join(root, "LICENSE"), filepath.Join(root, "THIRD_PARTY_NOTICES.txt")); err != nil {
			return err
		}
	}
	if len(checksums) != len(expected) {
		return fmt.Errorf("SHA256SUMS 条目数为 %d，期望 %d", len(checksums), len(expected))
	}
	return nil
}

func expectedArchiveNames(version string) ([]string, error) {
	var result []string
	for _, platform := range []string{PlatformDarwinARM64, PlatformLinuxAMD64, PlatformWindowsAMD64} {
		name, err := ArchiveName(version, platform)
		if err != nil {
			return nil, err
		}
		result = append(result, name)
	}
	sort.Strings(result)
	return result, nil
}

func fileSHA256(filename string) (string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return "", err
	}
	defer file.Close()
	digest := sha256.New()
	if _, err := io.Copy(digest, file); err != nil {
		return "", err
	}
	return hex.EncodeToString(digest.Sum(nil)), nil
}

func readChecksums(filename string) (map[string]string, error) {
	file, err := os.Open(filename)
	if err != nil {
		return nil, err
	}
	defer file.Close()
	result := make(map[string]string)
	scanner := bufio.NewScanner(file)
	previous := ""
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 2)
		if len(parts) != 2 || len(parts[0]) != sha256.Size*2 {
			return nil, fmt.Errorf("无效 SHA256SUMS 行：%q", scanner.Text())
		}
		if _, err := hex.DecodeString(parts[0]); err != nil {
			return nil, fmt.Errorf("无效 SHA-256：%q", parts[0])
		}
		if parts[1] <= previous || result[parts[1]] != "" || filepath.Base(parts[1]) != parts[1] {
			return nil, fmt.Errorf("SHA256SUMS 文件名未排序、重复或越界：%q", parts[1])
		}
		previous = parts[1]
		result[parts[1]] = parts[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func verifyPlatformArchive(filename, root, platform, version, licensePath, noticesPath string) error {
	rootName := fmt.Sprintf("GoDoIt_%s_%s", version, platform)
	license, err := os.ReadFile(licensePath)
	if err != nil {
		return err
	}
	notices, err := os.ReadFile(noticesPath)
	if err != nil {
		return err
	}
	required := map[string][]byte{
		path.Join(rootName, "LICENSE"):                 license,
		path.Join(rootName, "THIRD_PARTY_NOTICES.txt"): notices,
	}
	switch platform {
	case PlatformLinuxAMD64:
		required[path.Join(rootName, "gdit")] = nil
		required[path.Join(rootName, "gdit-gui")] = nil
	case PlatformWindowsAMD64:
		required[path.Join(rootName, "gdit.exe")] = nil
		required[path.Join(rootName, "gdit-gui.exe")] = nil
	case PlatformDarwinARM64:
		required[path.Join(rootName, "gdit")] = nil
		required[path.Join(rootName, "GoDoIt.app", "Contents", "MacOS", "gdit-gui")] = nil
		required[path.Join(rootName, "GoDoIt.app", "Contents", "Info.plist")] = nil
		required[path.Join(rootName, "GoDoIt.app", "Contents", "Resources", "iconfile.icns")] = nil
		required[path.Join(rootName, "GoDoIt.app", "Contents", "Resources", "legal", "LICENSE")] = license
		required[path.Join(rootName, "GoDoIt.app", "Contents", "Resources", "legal", "THIRD_PARTY_NOTICES.txt")] = notices
	}
	seen := make(map[string]bool)
	consume := func(name string, mode os.FileMode, reader io.Reader) error {
		name = strings.TrimSuffix(name, "/")
		if err := validateArchivePath(name); err != nil {
			return err
		}
		if name != rootName && !strings.HasPrefix(name, rootName+"/") {
			return fmt.Errorf("归档项目越出顶层目录：%s", name)
		}
		if seen[name] {
			return fmt.Errorf("归档项目重复：%s", name)
		}
		seen[name] = true
		if forbiddenArchiveName(name) {
			return fmt.Errorf("归档含禁止项目：%s", name)
		}
		if mode.IsDir() || reader == nil {
			return nil
		}
		data, err := io.ReadAll(reader)
		if err != nil {
			return err
		}
		if absoluteRoot, err := filepath.Abs(root); err == nil && (bytes.Contains(data, []byte(absoluteRoot)) || bytes.Contains(data, []byte(filepath.ToSlash(absoluteRoot)))) {
			return fmt.Errorf("归档项目包含绝对工作区路径：%s", name)
		}
		if expected, ok := required[name]; ok && expected != nil && !bytes.Equal(data, expected) {
			return fmt.Errorf("归档法律文本与仓库不一致：%s", name)
		}
		return nil
	}
	if platform == PlatformLinuxAMD64 {
		if err := readTarGzip(filename, consume); err != nil {
			return err
		}
	} else if err := readZip(filename, consume); err != nil {
		return err
	}
	for name := range required {
		if !seen[name] {
			return fmt.Errorf("归档缺少 %s", name)
		}
	}
	if platform != PlatformDarwinARM64 {
		regularCount := 0
		for name := range seen {
			if name != rootName {
				regularCount++
			}
		}
		if regularCount != 4 {
			return fmt.Errorf("%s 归档含 %d 个文件，期望 4 个", platform, regularCount)
		}
	}
	return nil
}

func readTarGzip(filename string, consume func(string, os.FileMode, io.Reader) error) error {
	file, err := os.Open(filename)
	if err != nil {
		return err
	}
	defer file.Close()
	gzipReader, err := gzip.NewReader(file)
	if err != nil {
		return err
	}
	defer gzipReader.Close()
	reader := tar.NewReader(gzipReader)
	for {
		header, err := reader.Next()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return err
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA && header.Typeflag != tar.TypeDir {
			return fmt.Errorf("tar 含特殊项目：%s", header.Name)
		}
		if err := consume(header.Name, header.FileInfo().Mode(), reader); err != nil {
			return err
		}
	}
}

func readZip(filename string, consume func(string, os.FileMode, io.Reader) error) error {
	reader, err := zip.OpenReader(filename)
	if err != nil {
		return err
	}
	defer reader.Close()
	for _, entry := range reader.File {
		if entry.Mode()&os.ModeSymlink != 0 || !entry.Mode().IsRegular() && !entry.Mode().IsDir() {
			return fmt.Errorf("zip 含特殊项目：%s", entry.Name)
		}
		var input io.ReadCloser
		if !entry.FileInfo().IsDir() {
			input, err = entry.Open()
			if err != nil {
				return err
			}
		}
		if err := consume(entry.Name, entry.Mode(), input); err != nil {
			if input != nil {
				_ = input.Close()
			}
			return err
		}
		if input != nil {
			if err := input.Close(); err != nil {
				return err
			}
		}
	}
	return nil
}

func validateArchivePath(name string) error {
	if name == "" || strings.ContainsRune(name, '\\') || path.IsAbs(name) || path.Clean(name) != name || name == "." || strings.HasPrefix(name, "../") {
		return fmt.Errorf("不安全归档路径：%q", name)
	}
	return nil
}

func forbiddenArchiveName(name string) bool {
	lower := strings.ToLower(name)
	for _, token := range []string{"/testdata/", "/fixture", ".pem", ".key", ".p12", "/.env", "id_rsa"} {
		if strings.Contains(lower, token) {
			return true
		}
	}
	return false
}
