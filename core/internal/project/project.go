// Package project 负责在显式项目目录边界内只读分析 Godot 与 .NET 项目文件。
package project

import (
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"

	managedversion "github.com/Sheyiyuan/GoDoIt/core/internal/version"
)

const (
	// MaxProjectFileSize 是 project.godot 的最大读取字节数。
	MaxProjectFileSize int64 = 4 << 20
	// MaxGlobalJSONSize 是 global.json 的最大读取字节数。
	MaxGlobalJSONSize int64 = 1 << 20
	// MaxCSharpProjectSize 是单个 .csproj 的最大读取字节数。
	MaxCSharpProjectSize int64 = 4 << 20
)

var engineFeaturePattern = regexp.MustCompile(`^[0-9]+\.[0-9]+$`)
var targetFrameworkPattern = regexp.MustCompile(`^net([0-9]+)\.([0-9]+)(?:[-.].*)?$`)

// Diagnostic 是可归因于项目内容的稳定诊断。
type Diagnostic struct {
	Level   string
	Code    string
	Path    string
	Message string
}

// Evidence 是建议结论的一项原始证据。
type Evidence struct {
	Kind  string
	Path  string
	Value string
}

// Analysis 是项目包返回的归一化只读分析结果。
type Analysis struct {
	Dir          string
	EngineSeries string
	Edition      string
	SDKVersion   string
	SDKChannel   string
	RollForward  string
	AllowPreview *bool
	Evidence     []Evidence
	Diagnostics  []Diagnostic
}

type globalJSON struct {
	SDK struct {
		Version         string `json:"version"`
		RollForward     string `json:"rollForward"`
		AllowPrerelease *bool  `json:"allowPrerelease"`
	} `json:"sdk"`
}

type projectFile struct {
	PropertyGroups []propertyGroup `xml:"PropertyGroup"`
}

type propertyGroup struct {
	TargetFramework  string `xml:"TargetFramework"`
	TargetFrameworks string `xml:"TargetFrameworks"`
}

// Analyze 分析指定目录，项目内容损坏进入诊断；路径与基础 I/O 故障直接返回 error。
func Analyze(ctx context.Context, dir string) (Analysis, error) {
	if err := ctx.Err(); err != nil {
		return Analysis{}, err
	}
	abs, err := filepath.Abs(dir)
	if err != nil {
		return Analysis{}, fmt.Errorf("resolve project directory: %w", err)
	}
	abs = filepath.Clean(abs)
	info, err := os.Stat(abs)
	if err != nil {
		return Analysis{}, fmt.Errorf("inspect project directory %q: %w", abs, err)
	}
	if !info.IsDir() {
		return Analysis{}, fmt.Errorf("project path is not a directory: %s", abs)
	}
	boundary, err := filepath.EvalSymlinks(abs)
	if err != nil {
		return Analysis{}, fmt.Errorf("resolve project directory %q: %w", abs, err)
	}
	result := Analysis{Dir: abs, Edition: "standard"}
	projectPath := filepath.Join(abs, "project.godot")
	content, err := readFile(ctx, boundary, projectPath, MaxProjectFileSize)
	if err != nil {
		if isLimitError(err) {
			result.Diagnostics = append(result.Diagnostics, diagnostic("error", "project-godot-too-large", projectPath, err.Error()))
		} else {
			return result, err
		}
	} else {
		features, parseErr := parseFeatures(content)
		if parseErr != nil {
			result.Diagnostics = append(result.Diagnostics, diagnostic("error", "project-godot-invalid", projectPath, parseErr.Error()))
		} else {
			consumeFeatures(&result, projectPath, features)
		}
	}

	globalPath := filepath.Join(abs, "global.json")
	if exists, err := regularCandidate(globalPath); err != nil {
		return result, err
	} else if exists {
		content, readErr := readFile(ctx, boundary, globalPath, MaxGlobalJSONSize)
		if readErr != nil {
			if isLimitError(readErr) {
				result.Diagnostics = append(result.Diagnostics, diagnostic("error", "global-json-too-large", globalPath, readErr.Error()))
			} else {
				return result, readErr
			}
		} else {
			consumeGlobalJSON(&result, globalPath, content)
		}
	}

	entries, err := os.ReadDir(abs)
	if err != nil {
		return result, fmt.Errorf("read project directory %q: %w", abs, err)
	}
	var csprojPaths []string
	for _, entry := range entries {
		if strings.EqualFold(filepath.Ext(entry.Name()), ".csproj") {
			csprojPaths = append(csprojPaths, filepath.Join(abs, entry.Name()))
		}
	}
	sort.Strings(csprojPaths)
	var frameworks []string
	for _, path := range csprojPaths {
		content, readErr := readFile(ctx, boundary, path, MaxCSharpProjectSize)
		if readErr != nil {
			if isLimitError(readErr) {
				result.Diagnostics = append(result.Diagnostics, diagnostic("error", "csproj-too-large", path, readErr.Error()))
				continue
			}
			return result, readErr
		}
		frameworks = append(frameworks, consumeCSharpProject(&result, path, content)...)
	}
	if len(csprojPaths) > 0 && result.Edition == "standard" {
		result.Edition = "dotnet"
		result.Diagnostics = append(result.Diagnostics, diagnostic("warning", "csharp-project-without-feature", projectPath, "发现 .csproj，但 project.godot 未声明 C# feature；按 dotnet 项目建议"))
	}
	if len(csprojPaths) == 0 && result.Edition == "dotnet" {
		result.Diagnostics = append(result.Diagnostics, diagnostic("warning", "csharp-feature-without-project", projectPath, "project.godot 声明了 C#，但同目录没有 .csproj"))
	}
	if result.SDKVersion == "" {
		result.SDKChannel = highestFrameworkChannel(frameworks)
	}
	return result, nil
}

func readFile(ctx context.Context, boundary, path string, limit int64) ([]byte, error) {
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return nil, fmt.Errorf("resolve project file %q: %w", path, err)
	}
	if !within(boundary, resolved) {
		return nil, fmt.Errorf("project file escapes project directory: %s", path)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return nil, fmt.Errorf("inspect project file %q: %w", path, err)
	}
	if !info.Mode().IsRegular() {
		return nil, fmt.Errorf("project file is not a regular file: %s", path)
	}
	if info.Size() > limit {
		return nil, fileLimitError{path: path, limit: limit}
	}
	file, err := os.Open(resolved)
	if err != nil {
		return nil, fmt.Errorf("open project file %q: %w", path, err)
	}
	defer file.Close()
	reader := &contextReader{ctx: ctx, reader: io.LimitReader(file, limit+1)}
	content, err := io.ReadAll(reader)
	if err != nil {
		return nil, fmt.Errorf("read project file %q: %w", path, err)
	}
	if int64(len(content)) > limit {
		return nil, fileLimitError{path: path, limit: limit}
	}
	return content, nil
}

type contextReader struct {
	ctx    context.Context
	reader io.Reader
}

type fileLimitError struct {
	path  string
	limit int64
}

func (e fileLimitError) Error() string {
	return fmt.Sprintf("project file exceeds %d byte limit: %s", e.limit, e.path)
}

func isLimitError(err error) bool {
	var target fileLimitError
	return errors.As(err, &target)
}

func (r *contextReader) Read(buffer []byte) (int, error) {
	if err := r.ctx.Err(); err != nil {
		return 0, err
	}
	return r.reader.Read(buffer)
}

func regularCandidate(path string) (bool, error) {
	_, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("inspect optional project file %q: %w", path, err)
	}
	return true, nil
}

func parseFeatures(content []byte) ([]string, error) {
	section := ""
	lines := strings.Split(string(content), "\n")
	for index := 0; index < len(lines); index++ {
		line := strings.TrimSpace(stripComment(lines[index]))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			continue
		}
		if section != "application" {
			continue
		}
		key, value, found := strings.Cut(line, "=")
		if !found || strings.TrimSpace(key) != "config/features" {
			continue
		}
		expression := strings.TrimSpace(value)
		for unclosedCall(expression) && index+1 < len(lines) {
			index++
			expression += "\n" + stripComment(lines[index])
		}
		return parseStringArray(expression)
	}
	return nil, nil
}

func stripComment(line string) string {
	inString, escaped := false, false
	for index, r := range line {
		if escaped {
			escaped = false
			continue
		}
		if inString && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
			continue
		}
		if r == ';' && !inString {
			return line[:index]
		}
	}
	return line
}

func unclosedCall(value string) bool {
	inString, escaped, depth := false, false, 0
	for _, r := range value {
		if escaped {
			escaped = false
			continue
		}
		if inString && r == '\\' {
			escaped = true
			continue
		}
		if r == '"' {
			inString = !inString
		} else if !inString && r == '(' {
			depth++
		} else if !inString && r == ')' {
			depth--
		}
	}
	return inString || depth > 0
}

func parseStringArray(value string) ([]string, error) {
	value = strings.TrimSpace(value)
	open := strings.IndexByte(value, '(')
	if open < 0 || !strings.HasSuffix(value, ")") {
		return nil, errors.New("config/features 必须是 PackedStringArray(...) 或 PoolStringArray(...)")
	}
	kind := strings.TrimSpace(value[:open])
	if kind != "PackedStringArray" && kind != "PoolStringArray" {
		return nil, fmt.Errorf("不支持的 config/features 类型 %q", kind)
	}
	input := strings.TrimSpace(value[open+1 : len(value)-1])
	var result []string
	for input != "" {
		if input[0] != '"' {
			return nil, errors.New("config/features 只能包含字符串")
		}
		end := quotedEnd(input)
		if end < 0 {
			return nil, errors.New("config/features 包含未闭合字符串")
		}
		decoded, err := strconv.Unquote(input[:end+1])
		if err != nil {
			return nil, fmt.Errorf("解析 config/features 字符串: %w", err)
		}
		result = append(result, decoded)
		input = strings.TrimSpace(input[end+1:])
		if input == "" {
			break
		}
		if input[0] != ',' {
			return nil, errors.New("config/features 字符串之间缺少逗号")
		}
		input = strings.TrimSpace(input[1:])
	}
	return result, nil
}

func quotedEnd(value string) int {
	escaped := false
	for index := 1; index < len(value); index++ {
		if escaped {
			escaped = false
			continue
		}
		if value[index] == '\\' {
			escaped = true
		} else if value[index] == '"' {
			return index
		}
	}
	return -1
}

func consumeFeatures(result *Analysis, path string, features []string) {
	series := make(map[string]struct{})
	for _, feature := range features {
		if engineFeaturePattern.MatchString(feature) {
			series[feature] = struct{}{}
			result.Evidence = append(result.Evidence, Evidence{Kind: "project-feature", Path: path, Value: feature})
		}
		if feature == "C#" {
			result.Edition = "dotnet"
			result.Evidence = append(result.Evidence, Evidence{Kind: "project-feature", Path: path, Value: feature})
		}
	}
	if len(series) == 0 {
		result.Diagnostics = append(result.Diagnostics, diagnostic("error", "engine-series-missing", path, "config/features 未声明 Godot MAJOR.MINOR 系列"))
		return
	}
	if len(series) > 1 {
		values := make([]string, 0, len(series))
		for value := range series {
			values = append(values, value)
		}
		sort.Strings(values)
		result.Diagnostics = append(result.Diagnostics, diagnostic("error", "engine-series-conflict", path, "config/features 包含冲突的 Godot 系列："+strings.Join(values, ", ")))
		return
	}
	for value := range series {
		result.EngineSeries = value
	}
}

func consumeGlobalJSON(result *Analysis, path string, content []byte) {
	var value globalJSON
	if err := json.Unmarshal(content, &value); err != nil {
		result.Diagnostics = append(result.Diagnostics, diagnostic("error", "global-json-invalid", path, "解析 global.json: "+err.Error()))
		return
	}
	result.RollForward = value.SDK.RollForward
	result.AllowPreview = value.SDK.AllowPrerelease
	if value.SDK.Version != "" {
		if !managedversion.ValidSDK(value.SDK.Version) {
			result.Diagnostics = append(result.Diagnostics, diagnostic("error", "sdk-version-invalid", path, "global.json 的 sdk.version 不是合法精确版本"))
		} else {
			result.SDKVersion = value.SDK.Version
			parts := strings.Split(value.SDK.Version, ".")
			result.SDKChannel = parts[0] + "." + parts[1]
			result.Evidence = append(result.Evidence, Evidence{Kind: "global-json", Path: path, Value: "sdk.version=" + value.SDK.Version})
		}
	}
	if value.SDK.RollForward != "" {
		result.Evidence = append(result.Evidence, Evidence{Kind: "global-json", Path: path, Value: "sdk.rollForward=" + value.SDK.RollForward})
	}
	if value.SDK.AllowPrerelease != nil {
		result.Evidence = append(result.Evidence, Evidence{Kind: "global-json", Path: path, Value: "sdk.allowPrerelease=" + strconv.FormatBool(*value.SDK.AllowPrerelease)})
	}
}

func consumeCSharpProject(result *Analysis, path string, content []byte) []string {
	var value projectFile
	if err := xml.Unmarshal(content, &value); err != nil {
		result.Diagnostics = append(result.Diagnostics, diagnostic("error", "csproj-invalid", path, "解析 .csproj: "+err.Error()))
		return nil
	}
	var frameworks []string
	for _, group := range value.PropertyGroups {
		for _, raw := range []string{group.TargetFramework, group.TargetFrameworks} {
			for _, framework := range strings.Split(raw, ";") {
				framework = strings.TrimSpace(framework)
				if framework == "" {
					continue
				}
				if !targetFrameworkPattern.MatchString(framework) {
					continue
				}
				frameworks = append(frameworks, framework)
				result.Evidence = append(result.Evidence, Evidence{Kind: "target-framework", Path: path, Value: framework})
			}
		}
	}
	return frameworks
}

func highestFrameworkChannel(frameworks []string) string {
	bestMajor, bestMinor := -1, -1
	for _, framework := range frameworks {
		match := targetFrameworkPattern.FindStringSubmatch(framework)
		if len(match) != 3 {
			continue
		}
		major, _ := strconv.Atoi(match[1])
		minor, _ := strconv.Atoi(match[2])
		if major > bestMajor || major == bestMajor && minor > bestMinor {
			bestMajor, bestMinor = major, minor
		}
	}
	if bestMajor < 0 {
		return ""
	}
	return fmt.Sprintf("%d.%d", bestMajor, bestMinor)
}

func diagnostic(level, code, path, message string) Diagnostic {
	return Diagnostic{Level: level, Code: code, Path: path, Message: message}
}

func within(root, path string) bool {
	relative, err := filepath.Rel(root, path)
	return err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) && !filepath.IsAbs(relative)
}
