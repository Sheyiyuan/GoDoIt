package release

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

type dependencyMetadata struct {
	Name      string `json:"name"`
	Version   string `json:"version"`
	License   string `json:"license"`
	Upstream  string `json:"upstream"`
	Copyright string `json:"copyright"`
	Path      string `json:"path,omitempty"`
	Notice    string `json:"notice,omitempty"`
}

type noticeMetadata struct {
	Go     []dependencyMetadata `json:"go"`
	NPM    []dependencyMetadata `json:"npm"`
	Assets []dependencyMetadata `json:"assets"`
}

type listedGoPackage struct {
	Module *struct {
		Path    string
		Version string
		Main    bool
	}
}

type pnpmLock struct {
	Importers map[string]pnpmImporter `yaml:"importers"`
	Snapshots map[string]pnpmSnapshot `yaml:"snapshots"`
}

type pnpmImporter struct {
	Dependencies map[string]pnpmImporterDependency `yaml:"dependencies"`
}

type pnpmImporterDependency struct {
	Version string `yaml:"version"`
}

type pnpmSnapshot struct {
	Dependencies map[string]string `yaml:"dependencies"`
}

var licenseTexts = map[string]string{
	"MIT": `Permission is hereby granted, free of charge, to any person obtaining a copy
of this software and associated documentation files (the "Software"), to deal
in the Software without restriction, including without limitation the rights
to use, copy, modify, merge, publish, distribute, sublicense, and/or sell
copies of the Software, and to permit persons to whom the Software is
furnished to do so, subject to the following conditions:

The above copyright notice and this permission notice shall be included in all
copies or substantial portions of the Software.

THE SOFTWARE IS PROVIDED "AS IS", WITHOUT WARRANTY OF ANY KIND, EXPRESS OR
IMPLIED, INCLUDING BUT NOT LIMITED TO THE WARRANTIES OF MERCHANTABILITY,
FITNESS FOR A PARTICULAR PURPOSE AND NONINFRINGEMENT. IN NO EVENT SHALL THE
AUTHORS OR COPYRIGHT HOLDERS BE LIABLE FOR ANY CLAIM, DAMAGES OR OTHER
LIABILITY, WHETHER IN AN ACTION OF CONTRACT, TORT OR OTHERWISE, ARISING FROM,
OUT OF OR IN CONNECTION WITH THE SOFTWARE OR THE USE OR OTHER DEALINGS IN THE
SOFTWARE.`,
	"ISC": `Permission to use, copy, modify, and/or distribute this software for any
purpose with or without fee is hereby granted, provided that the above
copyright notice and this permission notice appear in all copies.

THE SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
REGARD TO THIS SOFTWARE INCLUDING ALL IMPLIED WARRANTIES OF MERCHANTABILITY
AND FITNESS. IN NO EVENT SHALL THE AUTHOR BE LIABLE FOR ANY SPECIAL, DIRECT,
INDIRECT, OR CONSEQUENTIAL DAMAGES OR ANY DAMAGES WHATSOEVER RESULTING FROM
LOSS OF USE, DATA OR PROFITS, WHETHER IN AN ACTION OF CONTRACT, NEGLIGENCE OR
OTHER TORTIOUS ACTION, ARISING OUT OF OR IN CONNECTION WITH THE USE OR
PERFORMANCE OF THIS SOFTWARE.`,
	"BSD-2-Clause": `Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.`,
	"BSD-3-Clause": `Redistribution and use in source and binary forms, with or without
modification, are permitted provided that the following conditions are met:

1. Redistributions of source code must retain the above copyright notice,
   this list of conditions and the following disclaimer.
2. Redistributions in binary form must reproduce the above copyright notice,
   this list of conditions and the following disclaimer in the documentation
   and/or other materials provided with the distribution.
3. Neither the name of the copyright holder nor the names of its contributors
   may be used to endorse or promote products derived from this software
   without specific prior written permission.

THIS SOFTWARE IS PROVIDED BY THE COPYRIGHT HOLDERS AND CONTRIBUTORS "AS IS"
AND ANY EXPRESS OR IMPLIED WARRANTIES, INCLUDING, BUT NOT LIMITED TO, THE
IMPLIED WARRANTIES OF MERCHANTABILITY AND FITNESS FOR A PARTICULAR PURPOSE
ARE DISCLAIMED. IN NO EVENT SHALL THE COPYRIGHT HOLDER OR CONTRIBUTORS BE
LIABLE FOR ANY DIRECT, INDIRECT, INCIDENTAL, SPECIAL, EXEMPLARY, OR
CONSEQUENTIAL DAMAGES (INCLUDING, BUT NOT LIMITED TO, PROCUREMENT OF
SUBSTITUTE GOODS OR SERVICES; LOSS OF USE, DATA, OR PROFITS; OR BUSINESS
INTERRUPTION) HOWEVER CAUSED AND ON ANY THEORY OF LIABILITY, WHETHER IN
CONTRACT, STRICT LIABILITY, OR TORT (INCLUDING NEGLIGENCE OR OTHERWISE)
ARISING IN ANY WAY OUT OF THE USE OF THIS SOFTWARE, EVEN IF ADVISED OF THE
POSSIBILITY OF SUCH DAMAGE.`,
}

// GenerateNotices 校验运行时依赖元数据，并生成确定性第三方声明。
func GenerateNotices(root, metadataPath string) ([]byte, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("解析仓库根目录：%w", err)
	}
	root = absoluteRoot
	metadata, err := readNoticeMetadata(metadataPath)
	if err != nil {
		return nil, err
	}
	goDependencies, err := runtimeGoDependencies(root)
	if err != nil {
		return nil, err
	}
	npmDependencies, err := runtimeNPMDependencies(filepath.Join(root, "gui", "frontend", "pnpm-lock.yaml"))
	if err != nil {
		return nil, err
	}
	if err := validateDependencySet("Go", goDependencies, metadata.Go); err != nil {
		return nil, err
	}
	if err := validateDependencySet("npm", npmDependencies, metadata.NPM); err != nil {
		return nil, err
	}
	for _, asset := range metadata.Assets {
		if asset.Path == "" {
			return nil, fmt.Errorf("品牌素材 %s 缺少 path", asset.Name)
		}
		if _, err := os.Stat(filepath.Join(root, filepath.FromSlash(asset.Path))); err != nil {
			return nil, fmt.Errorf("品牌素材 %s 不存在：%w", asset.Name, err)
		}
	}
	return renderNotices(metadata)
}

func readNoticeMetadata(path string) (noticeMetadata, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return noticeMetadata{}, fmt.Errorf("读取许可证元数据：%w", err)
	}
	var metadata noticeMetadata
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&metadata); err != nil {
		return noticeMetadata{}, fmt.Errorf("解析许可证元数据：%w", err)
	}
	for _, group := range [][]dependencyMetadata{metadata.Go, metadata.NPM, metadata.Assets} {
		for _, item := range group {
			if item.Name == "" || item.Version == "" || item.License == "" || item.Upstream == "" || item.Copyright == "" {
				return noticeMetadata{}, fmt.Errorf("依赖许可证元数据字段不完整：%+v", item)
			}
			if _, known := licenseTexts[item.License]; !known && item.Notice == "" {
				return noticeMetadata{}, fmt.Errorf("许可证 %s 没有固定文本或独立 notice", item.License)
			}
		}
	}
	return metadata, nil
}

func runtimeGoDependencies(root string) (map[string]string, error) {
	result := make(map[string]string)
	targets := []map[string]string{
		{"GOOS": "linux", "GOARCH": "amd64", "CGO_ENABLED": "1", "GOFLAGS": "-tags=webkit2_41"},
		{"GOOS": "darwin", "GOARCH": "arm64", "CGO_ENABLED": "1", "GOFLAGS": ""},
		{"GOOS": "windows", "GOARCH": "amd64", "CGO_ENABLED": "0", "GOFLAGS": ""},
	}
	for _, target := range targets {
		target["GOWORK"] = filepath.Join(root, "go.work")
		command := exec.Command("go", "list", "-deps", "-json", "./cli/cmd/gdit", "./gui")
		command.Dir = root
		command.Env = environmentWith(os.Environ(), target)
		output, err := commandStdout(command)
		if err != nil {
			return nil, fmt.Errorf("枚举 %s/%s Go 运行时依赖：%w", target["GOOS"], target["GOARCH"], err)
		}
		decoder := json.NewDecoder(bytes.NewReader(output))
		for {
			var item listedGoPackage
			if err := decoder.Decode(&item); err == io.EOF {
				break
			} else if err != nil {
				return nil, fmt.Errorf("解析 %s/%s Go 依赖：%w", target["GOOS"], target["GOARCH"], err)
			}
			if item.Module == nil || item.Module.Main {
				continue
			}
			if old, exists := result[item.Module.Path]; exists && old != item.Module.Version {
				return nil, fmt.Errorf("Go 运行时依赖 %s 跨平台版本不一致：%s 与 %s", item.Module.Path, old, item.Module.Version)
			}
			result[item.Module.Path] = item.Module.Version
		}
	}
	return result, nil
}

func commandStdout(command *exec.Cmd) ([]byte, error) {
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	command.Stdout = &stdout
	command.Stderr = &stderr
	if err := command.Run(); err != nil {
		diagnostic := strings.TrimSpace(stderr.String())
		if diagnostic != "" {
			return nil, fmt.Errorf("%w：%s", err, diagnostic)
		}
		return nil, err
	}
	return stdout.Bytes(), nil
}

func environmentWith(environment []string, overrides map[string]string) []string {
	result := make([]string, 0, len(environment)+len(overrides))
	for _, item := range environment {
		key, _, found := strings.Cut(item, "=")
		if found {
			if _, replaced := overrides[key]; replaced {
				continue
			}
		}
		result = append(result, item)
	}
	for key, value := range overrides {
		result = append(result, key+"="+value)
	}
	return result
}

func runtimeNPMDependencies(lockPath string) (map[string]string, error) {
	data, err := os.ReadFile(lockPath)
	if err != nil {
		return nil, fmt.Errorf("读取 pnpm lock：%w", err)
	}
	var lock pnpmLock
	if err := yaml.Unmarshal(data, &lock); err != nil {
		return nil, fmt.Errorf("解析 pnpm lock：%w", err)
	}
	root, ok := lock.Importers["."]
	if !ok {
		return nil, fmt.Errorf("pnpm lock 缺少根 importer")
	}
	queue := make([]string, 0, len(root.Dependencies))
	for name, dependency := range root.Dependencies {
		queue = append(queue, name+"@"+dependency.Version)
	}
	visited := make(map[string]bool)
	result := make(map[string]string)
	for len(queue) > 0 {
		key := queue[0]
		queue = queue[1:]
		if visited[key] {
			continue
		}
		visited[key] = true
		name, version, err := splitPNPMKey(key)
		if err != nil {
			return nil, err
		}
		if old, exists := result[name]; exists && old != version {
			return nil, fmt.Errorf("npm 运行时依赖 %s 同时使用 %s 和 %s", name, old, version)
		}
		result[name] = version
		snapshot, ok := lock.Snapshots[key]
		if !ok {
			return nil, fmt.Errorf("pnpm lock 缺少 snapshot %s", key)
		}
		for childName, childVersion := range snapshot.Dependencies {
			queue = append(queue, childName+"@"+childVersion)
		}
	}
	return result, nil
}

func splitPNPMKey(key string) (string, string, error) {
	base := key
	if index := strings.IndexByte(base, '('); index >= 0 {
		base = base[:index]
	}
	index := strings.LastIndexByte(base, '@')
	if index <= 0 || index == len(base)-1 {
		return "", "", fmt.Errorf("无法解析 pnpm package key %q", key)
	}
	return base[:index], base[index+1:], nil
}

func validateDependencySet(kind string, actual map[string]string, metadata []dependencyMetadata) error {
	expected := make(map[string]string, len(metadata))
	for _, item := range metadata {
		if _, exists := expected[item.Name]; exists {
			return fmt.Errorf("%s 许可证元数据重复：%s", kind, item.Name)
		}
		expected[item.Name] = item.Version
	}
	for name, version := range actual {
		if expected[name] == "" {
			return fmt.Errorf("%s 运行时依赖缺少许可证元数据：%s@%s", kind, name, version)
		}
		if expected[name] != version {
			return fmt.Errorf("%s 运行时依赖版本不一致：%s 实际 %s，元数据 %s", kind, name, version, expected[name])
		}
	}
	for name, version := range expected {
		if actual[name] == "" {
			return fmt.Errorf("%s 许可证元数据包含未进入产物的依赖：%s@%s", kind, name, version)
		}
	}
	return nil
}

func renderNotices(metadata noticeMetadata) ([]byte, error) {
	groups := []struct {
		title string
		items []dependencyMetadata
	}{{"Go modules", metadata.Go}, {"npm packages", metadata.NPM}, {"Brand assets", metadata.Assets}}
	var output strings.Builder
	output.WriteString("GoDoIt Third-Party Notices\n")
	output.WriteString("Generated from repository lockfiles and scripts/third_party_licenses.json.\n\n")
	usedLicenses := make(map[string]bool)
	for _, group := range groups {
		sort.Slice(group.items, func(i, j int) bool { return group.items[i].Name < group.items[j].Name })
		output.WriteString("== " + group.title + " ==\n\n")
		for _, item := range group.items {
			fmt.Fprintf(&output, "%s %s\nLicense: %s\nUpstream: %s\n%s\n", item.Name, item.Version, item.License, item.Upstream, item.Copyright)
			if item.Notice != "" {
				output.WriteString(item.Notice + "\n")
			}
			output.WriteByte('\n')
			if _, known := licenseTexts[item.License]; known {
				usedLicenses[item.License] = true
			}
		}
	}
	licenses := make([]string, 0, len(usedLicenses))
	for identifier := range usedLicenses {
		licenses = append(licenses, identifier)
	}
	sort.Strings(licenses)
	output.WriteString("== License texts ==\n")
	for _, identifier := range licenses {
		fmt.Fprintf(&output, "\n-- %s --\n%s\n", identifier, licenseTexts[identifier])
	}
	return []byte(output.String()), nil
}
