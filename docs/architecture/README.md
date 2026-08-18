# GoDoIt 架构设计

> 状态为 v0.2 第一阶段实施中  
> 本文档是 GoDoIt 的唯一架构真理源。

第一阶段的产品边界、摘要信任模型、Go module 路径和 Linux amd64 支持范围已经确认，可以实施
`install/list` 纵向切片。2026-08-17 实测确认 GodotHub 和 Godot 官方的 URL 规则并写入内置 provider；
AtomGit 作为独立来源的规则尚未确认，确认前仍只实现 provider 契约、自定义来源和固定 fixture，
不把猜测地址写入内置 provider。

## 1. 边界

GoDoIt 只管理用户级 Godot 引擎、导出模板和 .NET SDK。所有持久状态都在 `~/.gdit/`。

项目相关能力只有 `gdit suggest`。用户显式传入目录后，GoDoIt 只读分析 `project.godot`、
`global.json` 和 `.csproj`，给出安装建议。它不在项目目录写文件，不保存项目路径，不根据当前目录
自动切换引擎。

## 2. 用户目录

```text
~/.gdit/
├── config.toml                  # 用户配置
├── state.toml                   # gdit 维护的已安装版本元数据
├── bin/
│   └── godot -> <gdit 可执行文件> # 用户级 shim
├── current -> versions/<id>/    # 当前全局版本
├── versions/
│   ├── 4.5.2-standard/
│   │   ├── install.toml         # 安装完成标记和最小可重建元数据
│   │   └── payload/             # 平台原始资产解压后的规范化内容
│   └── 4.5.2-dotnet/
├── dotnet/
├── templates/
├── cache/                    # P2 缓存管理预留，第一阶段不创建
└── tmp/                         # 下载和解压临时目录
```

- `config.toml` 是唯一需要用户编辑的配置文件。
- `versions/` 是判断已安装版本的依据。一个目录只有名称合法、`install.toml` 可解析且目标可执行文件
  存在时才算完整安装。无效目录由 `doctor` 报告，`list` 不把它当作已安装版本。
- `install.toml` 由 gdit 在临时目录内生成，随整个版本目录一起原子发布。它记录版本 ID、目标平台、
  架构、edition、启动文件相对路径和资产摘要，不含用户配置。
- `state.toml` 由 gdit 维护，不一致时按有效的版本目录和 `install.toml` 重建，不要求用户编辑。
- `current` 是全局 symlink，所有目录使用同一个当前版本。
- `versions/` 下只放完整安装；未完成内容只能出现在 `tmp/`。
- `cache/` 属于 FR-10（缓存管理，P2）预留，第一阶段不创建该目录，安装下载直接进入 `tmp/`。
- 运行时文件锁可以放在 `~/.gdit/.lock`，它不是配置或项目 lock。

第一版固定使用 `~/.gdit/`，不把同一套数据拆到多个 XDG 目录。macOS 也保持相同用户目录，
只有引擎资产布局和平台命令由适配层处理。

core 构造时必须接收一个已解析的根目录。CLI 和 GUI 的生产入口通过 platform 适配层把用户主目录
解析为 `~/.gdit/`，测试直接传入临时目录。core 内部不能再次读取真实用户主目录，也不提供隐式的
项目级根目录覆盖。

## 3. 版本标识

版本 ID 只包含精确 Godot 版本和 edition，格式如下。

```text
4.5.2-standard
4.5.2-dotnet
```

- `standard` 表示标准版。
- `dotnet` 表示 C#/.NET 版；CLI 可以接受 `mono` 作为输入别名，但内部统一保存为 `dotnet`。
- MVP 只接受稳定版精确版本；预览版将在后续版本单独设计标识。
- 下载源不属于版本 ID。同一版本可以从不同来源下载，安装后仍是同一个版本。
- 平台和架构用于选择下载资产，不进入用户看到的版本 ID。

CLI 第一阶段使用下面的输入形式。

```text
gdit install --edition standard 4.5.2
gdit install --edition dotnet 4.5.2
gdit list
```

`--edition` 默认是 `standard`，输入 `mono` 时归一化为 `dotnet`。版本解析只接受三个十进制数字段，
拒绝 `latest`、版本范围、预览版和带任意后缀的输入。core 在任何路径拼接之前完成解析和规范化。

第一阶段只承诺 Linux amd64 的完整安装流程。MVP 仍需完成 Linux 的正式支持和 macOS Apple Silicon
实机验证。Linux arm64 是否列入 MVP 需要在 review 时明确，不能仅凭上游存在对应资产就视为已支持。

## 4. 核心流程

### 4.1 安装

```text
解析版本
  → 解析平台目标和上游资产名
  → 获取全局修改锁并复查是否已安装
  → 清理 tmp/ 下遗留的 operation 目录
  → 按来源顺序解析资产和预期摘要
  → 下载到 ~/.gdit/tmp/<operation-id>/
  → 校验预期摘要
  → 安全解压到同一临时目录
  → 校验启动文件并生成 install.toml
  → 原子移动到 ~/.gdit/versions/<id>/
  → 原子写入 state.toml
```

每个 source provider 把结果归为成功、不可用、配置错误、完整性错误或取消。找不到对应资产、连接超时、
限流和服务端暂时失败属于不可用，可以按顺序尝试下一来源。配置错误直接终止并指出来源名。摘要不匹配
属于完整性错误，立即终止整个安装，不能继续 fallback。context 取消也立即终止。

下载必须写入新建的 operation 目录，文件名不能来自未经清理的 URL 路径。解压时拒绝绝对路径、
越过目标目录的 `..`、设备文件和逃逸目标目录的 symlink。发布前由 platform 适配层确认启动文件布局并
设置所需执行权限。目标版本目录已经存在时返回可识别的冲突结果，不覆盖已有内容。

临时目录和 `versions/` 位于同一 gdit 根目录，以保证目录 rename 不跨文件系统。目录原子移动成功
即表示安装完成。若随后更新 `state.toml` 失败，本次安装仍返回“已安装但状态索引待重建”的结果，
下次读取时按版本目录重建。CLI 需要把这个结果明确写到 stderr，不能把版本误报为未安装。

`.lock` 覆盖 install、remove、use、setup、source use 的配置写回以及会落盘的状态重建。
等待锁时响应 context 取消。
读取操作先扫描有效版本目录；发现 `state.toml` 不一致时，在取得锁并二次扫描后再原子重写，避免用
过期快照覆盖另一个进程刚完成的安装。

获取修改锁后先删除 `tmp/` 下遗留的 `operation-*` 目录。锁内不存在其他进行中的安装，遗留目录都是
进程中断的残留，直接删除不会误伤并发任务；清理失败按本地 I/O 错误处理。

所有 TOML 状态文件都写入同目录临时文件，完成编码、flush、文件同步和 close 后再 rename，随后同步
父目录。版本目录发布和 current 切换也同步对应父目录。相关系统调用由 platform 适配层封装。

### 4.2 切换

`gdit use <id>` 检查目标已经完整安装，然后原子替换 `~/.gdit/current` symlink。失败时保留旧链接。

### 4.3 启动

`gdit setup` 在用户显式执行时创建或修复 `~/.gdit/bin/godot` shim，并在该目录未加入 PATH 时给出
提示。它不修改 shell 配置或系统 PATH。以 `godot` 名称启动时，gdit 只做下面三件事。

1. 读取 `~/.gdit/current`。
2. 从 `config.toml` 合并全局和当前版本环境。
3. 启动真实 Godot，并透传参数和退出码。

shim 不读取当前项目，也不访问网络。必须经过 gdit 而不是直接 symlink 到引擎，因为环境变量需要
只注入目标子进程。

### 4.4 项目建议

`gdit suggest <目录>` 调用只读探测器，输出推荐的 Godot edition/版本和 .NET SDK。默认不安装，
显式 `--install` 或用户确认后才复用正常安装流程。分析结果不写入项目或 gdit 状态。

### 4.5 来源与摘要契约

source provider 接收规范化版本、edition 和 platform target，返回下载 URL、规范资产名、可选大小、
摘要算法与预期摘要。业务层只依赖这个结构和错误分类，不拼接某个镜像的 URL。内置来源和自定义来源
使用同一接口，fixture HTTP server 可以完整替代真实网络。

2026 年 8 月核对 [Godot 4.5.2 官方发布页](https://github.com/godotengine/godot-builds/releases/tag/4.5.2-stable)
时，发布资产提供 `SHA512-SUMS.txt`，官方不提供 SHA-256 清单。内置 `github` 来源使用官方资产并
按 SHA-512 校验；GodotHub 来源的 digest 字段提供 SHA-256，按项目约定使用 sha256。GoDoIt 不把
摘要计算结果当作新的信任来源，只验证下载内容是否与来源声明一致。

每个 source provider 必须明确声明摘要算法。状态模型保存 `algorithm + value`，第一阶段同时支持
`sha256`（GodotHub、自定义来源）和 `sha512`（官方 github 来源），以后扩展到上游 SHA-512 时无需
改变安装目录格式。摘要不匹配立即终止安装，不能切换到下一来源。

自定义源的校验清单按以下规则解析：每行接受「摘要 文件名」（空格分隔）或 BSD 风格「摘要 *文件名」；
也允许单列裸摘要，但裸摘要行只能出现一次，多行裸摘要视为配置错误；摘要十六进制大小写不敏感；
清单中找不到目标资产条目视为配置错误。元数据驱动来源（godothub 的 releases.json、github 的
releases 端点）中 release 或资产不存在属于来源不可用，按顺序尝试下一来源；元数据本身无法解析、
digest 字段非法才属于配置错误。摘要信任与下载信任绑定：校验值来自来源自身声明的清单或
digest 字段，GoDoIt 不把计算结果当作新的信任来源——来源被攻破即等价于镜像被攻破，这是与常见
包管理器一致的供应链信任取舍。

### 4.6 内置来源 URL 规则（2026-08-17 实测确认）

```text
github  资产     https://github.com/godotengine/godot-builds/releases/download/{tag}/{asset}
        tag       {version}-stable，如 4.5.2-stable
        资产名     Godot_v{version}-stable_linux.x86_64.zip
                   Godot_v{version}-stable_mono_linux_x86_64.zip（dotnet 版）
        摘要      SHA512-SUMS.txt（同 release 目录，SHA-512）

godothub 元数据  https://legacy.godothub.com/api/releases.json
        资产      https://atomgit.com/godothub/godot/releases/download/{tag}/{asset}
                  （实测 302 到 file-cdn.gitcode.com 签名 CDN）
        摘要      releases.json 中对应资产的 digest 字段（sha256:<64 hex>）
```

GodotHub 的 `releases.json` 是 GitHub Release API 结构，按 `tag_name` 匹配 release、按资产名匹配
条目后读取 `digest`。GoDoIt 不调用 godothub.com 的页面接口，只依赖上述稳定 URL。官方默认
source_order 为 `["godothub", "github"]`，保证 GodotHub 不可用时直接降级到官方源；AtomGit 独立
规则确认后加入默认顺序。用户显式在 source_order 中配置 atomgit 时，该位置返回明确的配置错误，
可用 `custom_sources` 或改写 source_order 绕过。

### 4.7 来源管理（第一阶段扩展）

在第一阶段 install/list 基础上增加来源的查看、默认切换、禁用/启用、指定来源安装、可用版本
探测和交互式安装。全部围绕 source 层，不引入版本切换语义，不改变后续阶段顺序。

```text
gdit install                          # 交互式安装：依次选择 edition、version、source
gdit install [--edition standard|dotnet] [--source <name>] <version>
gdit list
gdit source                           # 按 source_order 顺序列出来源、类型与禁用状态
gdit source use <name>                # 把 <name> 移到 source_order 首位并写回 config.toml
gdit source ban <name>                # 禁用来源：不再参与自动 fallback 与默认枚举
gdit source unban <name>              # 启用来源
gdit available [--source <name>]      # 探测当前平台可安装的稳定版本与 edition
```

命令简写：`install`/`i`、`list`/`l`、`source`/`s`、`available`/`a`；flag 简写
`--edition`/`-e`、`--source`/`-s`。后续阶段命令可能重新占用简写，以各阶段文档为准。

- 交互式 `install` 只在标准输入为 TTY 时可用，非 TTY 下无参数调用返回用法错误，避免脚本卡死。
  交互流程：先选 edition（standard/dotnet），再从 `available` 的枚举结果中选 version（该
  edition 的版本），最后选 source（`auto` 按顺序 fallback 或指定单个来源）。枚举失败或列表
  为空时降级为文本输入版本号。交互实现使用 `github.com/AlecAivazis/survey/v2`，只出现在 CLI
  层，core 不依赖（见 7.2）。
- `source` 无参数时列出全部来源：顺序、名称、类型（builtin/custom）、禁用状态，只读不落盘。
- `source use` 把目标来源移到 source_order 首位，其余保持相对顺序，原子写回 config.toml，
  受全局修改锁保护。写回保留全部已知与未知字段（键顺序由写回编码确定，注释不保留，
  见第 5 节）。配置文件不存在时按内置默认创建后再调整。被禁用的来源不能 use。
- `source ban`/`unban` 是强禁用语义：被禁用的来源不参与自动 fallback（install 默认流程）、
  不参与默认枚举（available），显式 `install --source <name>`、`available --source <name>` 或
  `source use <name>` 指定被禁用的来源时报配置错误（source is disabled），必须先 unban。
  禁用状态写入 config.toml 的 `disabled_sources`，受全局修改锁保护。
- `install --source <name>` 明确指定来源：只使用该来源解析资产并下载，来源不可用或完整性
  错误时直接失败，不按 source_order fallback。名字必须存在于当前 source_order 且未被禁用。
- `available` 默认合并 source_order 中所有启用且支持枚举来源的结果：版本取并集，标注每个
  版本可见于哪些来源；单个来源枚举失败只影响该来源，原因写 stderr，其余来源继续。
  指定 `--source` 时只用该来源。所有来源都失败或都不支持枚举时返回错误。
- 枚举结果 = 当前平台可安装的稳定精确版本：tag 必须匹配 `\d+\.\d+\.\d+-stable`
  （过滤 rc/beta/dev 及 4.4-stable 这类非三段 tag），edition 按 `platform.AssetName`
  生成的两个候选资产名在 release 资产中的实际存在判断。某 release 没有任何当前平台资产时
  （如 Godot 3.x 的 x11 命名）不列出。预览版的枚举与安装边界一致，不在本阶段提供。

#### provider 契约扩展

```go
// VersionInfo 是来源可用的稳定版本条目。
type VersionInfo struct {
	Version  string   // 如 4.5.2
	Editions []string // 按当前平台资产判断，如 standard、dotnet
}

// VersionLister 是可枚举可用版本的来源；URL 模板型自定义源不实现该接口。
type VersionLister interface {
	ListVersions(ctx context.Context) ([]VersionInfo, error)
}
```

- `HTTPProvider` 增加可选字段 `ReleasesURL`，指向 GitHub Release API 兼容的 JSON
  （tag_name + assets[].name）。设置该字段的来源支持枚举；未设置的自定义源调用
  `ListVersions` 返回配置错误，提示该来源无法枚举版本。
- 内置 github 来源的 `ReleasesURL` 为
  `https://api.github.com/repos/godotengine/godot-builds/releases?per_page=100`。
  2026-08-17 实测返回 HTTP 200，单页 100 条，与 godothub 的 releases.json 同为
  GitHub Release API 结构；分页暂不处理，单页 100 条为当前枚举上限，
  完整历史枚举（含 4.0 及更早）后续扩展。
- godothub 来源的枚举复用已实测的 releases.json 元数据，不新增请求端点。
- 枚举只反映来源元数据声明的内容；列出的版本仍需走完整下载与摘要校验流程。

## 5. 配置

`~/.gdit/config.toml` 集中保存全部用户配置，内容如下。

```toml
schema_version = 1
source_order = ["godothub", "github"]
disabled_sources = []

[environment]
display_driver = "auto"
input_method = "auto"

[dotnet]
auto_install = "ask"

[versions."4.5.2-dotnet".environment]
EXAMPLE_VARIABLE = "value"

[[custom_sources]]
name = "company-mirror"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "GDIT_COMPANY_MIRROR_TOKEN"
```

- `display_driver = "auto"` 不强制 x11；Linux 按会话检测，macOS 不设置该变量。
- `input_method = "auto"` 只在 Linux 检测到 fcitx 时注入相关变量。
- 选择 `~/.gdit/dotnet/` 中的 SDK 时，shim 设置 `DOTNET_ROOT` 并把对应目录放到 PATH 前部。
- 自定义源也写在此文件；认证信息不输出到日志。

自定义源模板第一版只允许 `{version}`、`{tag}` 和 `{asset}` 三个占位符。URL 必须使用 HTTPS，
localhost fixture 测试除外。认证值通过 `authorization_env` 指向的环境变量读取，配置只保存变量名。
日志和错误输出只显示来源名、主机名和脱敏后的路径，不输出完整认证 URL、header 或环境变量值。

配置文件不存在时使用内置默认值。GodotHub 和 Godot 官方的 URL 规则已经实测确认（见 4.6）；
默认 source_order 为 `["godothub", "github"]`，AtomGit 规则未确认期间，用户显式配置 atomgit 时
该位置返回明确的配置错误，可通过 `custom_sources` 配置已知镜像或改写 source_order 绕过。
`disabled_sources` 列出被 `source ban` 禁用的来源，名字必须存在于 source_order 中。
配置文件存在但无法解析、schema 版本不支持、来源重名、模板非法或禁用名单包含未知来源时返回
配置错误，不能悄悄退回默认值。第一阶段只解析安装所需字段，未进入实现阶段的已知字段仍需被
保留，避免一次写回丢失用户配置。

`gdit source use` 与 `gdit source ban/unban` 的写回保留全部已知与未知字段（键顺序由写回编码
确定，注释不保留）；配置文件不存在时按内置默认创建后再调整。写回使用同目录临时文件 + rename +
父目录 fsync，与状态文件同一套原子写规则。

`state.toml` 记录已安装版本及来源、摘要算法、摘要值、安装时间等附加元数据。按 `versions/` 重建时，
无法恢复的附加元数据可标记为未知，不影响版本使用。它不记录项目，不承担用户配置职责。

## 6. .NET SDK

Godot .NET 版启动前按以下顺序选择 SDK。

1. 检查 `dotnet --list-sdks` 中是否存在兼容版本。
2. 检查 `~/.gdit/dotnet/` 中是否已有兼容版本。
3. 都没有时终止启动，报错并提示用户显式安装。

Godot 版本到最低 SDK 的映射由 core 维护。托管 SDK 使用与引擎相同的下载、摘要校验和临时目录
安装流程。SDK 下载只由用户显式发起的 CLI 或 GUI 操作触发，`auto_install = "ask"` 也只对这类操作生效。
普通 `godot` 启动不询问、不访问网络。GoDoIt 不修改或卸载系统 dotnet。

项目 `global.json` 和 `.csproj` 只在 `suggest` 中参与建议，不改变普通 `godot` 的全局 SDK 选择。

## 7. 模块结构

```text
GoDoIt/
├── go.work
├── core/
│   ├── go.mod
│   ├── gdit.go                  # CLI/GUI 共用入口
│   └── internal/
│       ├── archive/
│       ├── config/
│       ├── dotnet/
│       ├── env/
│       ├── lock/
│       ├── platform/
│       ├── project/             # 只读 suggest 探测
│       ├── source/
│       └── store/
├── cli/
│   ├── go.mod
│   └── cmd/gdit/
├── gui/
│   ├── go.mod
│   └── frontend/
└── docs/
```

- `core` 实现全部业务能力并提供公共 Go API。
- CLI 只解析参数、调用 core、输出结果。
- GUI 只通过 Wails 调用同一个 core。
- `runtime.GOOS` 判断只出现在 `core/internal/platform/`。
- `project/` 只有只读分析能力，不能写项目或维护项目列表。

仓库按阶段创建真实需要的 package。第一阶段不创建 `dotnet/`、`env/`、`project/` 或 `gui/` 空目录，
这些路径在对应能力有可运行实现时再加入。`go.work` 也只引用当前实际存在的 module。

### 7.1 core 公共 API

core 对 CLI 和 GUI 暴露一个无界面语义的 `Manager`。第一阶段最小公共面如下。

```go
type Options struct {
	RootDir    string
	HTTPClient *http.Client
	Progress   func(ProgressEvent)
	Sources    []Source // 固定 fixture 或宿主显式注入的来源；为空时读取 config.toml
}

type InstallRequest struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
	Source  string `json:"source,omitempty"` // 指定来源，非空时只使用该来源
}

type SourceInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // builtin 或 custom
	Disabled bool   `json:"disabled"` // 是否被 source ban 禁用
}

type AvailableVersion struct {
	Version  string   `json:"version"`
	Editions []string `json:"editions"`
	Sources  []string `json:"sources"`
}

func DefaultRoot() (string, error)
func New(options Options) (*Manager, error)
func (m *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error)
func (m *Manager) List(ctx context.Context) ([]InstalledVersion, error)
func (m *Manager) Sources(ctx context.Context) ([]SourceInfo, error)
func (m *Manager) SetDefaultSource(ctx context.Context, name string) error
func (m *Manager) SetSourceDisabled(ctx context.Context, name string, disabled bool) error
func (m *Manager) Available(ctx context.Context, sourceName string) ([]AvailableVersion, error)
```

最终字段名可以在编码前做一次 Go 命名校正，行为约束保持不变。`RootDir` 必填，`HTTPClient` 为空时使用
带明确超时的默认 client。core 不接收 stdout、stderr 或 GUI 回调对象。进度通过结构化 `ProgressEvent`
报告，最终结果和错误单独返回。CLI 把结果写 stdout、把进度和警告写 stderr，GUI 把同一事件转换成
Wails 事件。

公共结果结构同时带 `json` tag，供以后 CLI 机器输出和 Wails 边界复用。错误使用可由 `errors.Is/As`
识别的类型，至少区分输入、配置、不支持平台、已安装、来源全部不可用、完整性、取消和本地 I/O。
错误文本不能成为 CLI 分支条件。

`New` 只校验依赖和路径，不访问网络。`Install`、`List` 以及之后所有可能阻塞的操作接收
`context.Context`。下载、锁等待和外部进程都必须传播取消。

`Sources` 只读列出当前配置来源，不落盘不拿锁。`SetDefaultSource` 获取全局修改锁、锁内重读配置并
调整 source_order、原子写回 config.toml，被禁用的来源不能 use。`SetSourceDisabled` 同样在锁内
读写 `disabled_sources`。`Install` 指定被禁用的来源时报配置错误。`Available` 只读网络枚举，
不落盘不拿锁；sourceName 为空时按 source_order 合并全部启用且支持枚举的来源，非空时只用指定
来源（被禁用的来源报配置错误）。

### 7.2 依赖方向

```text
cli ────────→ core public API
gui bridge ─→ core public API
                  │
                  ├── config
                  ├── source ──→ HTTP client
                  ├── store ───→ lock
                  ├── archive
                  └── platform
```

source 负责把来源协议转换为规范化资产，store 负责 gdit 根目录内的一致性，archive 只负责受限解压，
platform 负责目标识别、资产命名、引擎布局、权限和 POSIX 原子操作。store 不依赖具体来源，source 不写
版本目录。业务编排放在 core facade，不能沉入 CLI handler。

core 的直接依赖只包括 `BurntSushi/toml` 和提供 Linux/macOS 文件锁及同步调用的 `x/sys`。CLI 子命令
解析用 Go 标准库 `flag`；交互式 install 的选择器引入 `github.com/AlecAivazis/survey/v2`（2026-08-18
review 确认），仅限 CLI 层使用，survey 类型不得进入 core，交互逻辑不影响非交互路径。

## 8. 测试策略

- 安装成功、下载中断、摘要失败、多源 fallback 和状态清单重建。
- `use` 原子切换以及失败后旧 current 保持不变。
- `setup` 创建 shim 但不修改 shell 配置或系统 PATH。
- shim 使用 current、注入环境并透传退出码；缺少兼容 SDK 时不访问网络。
- 标准版与 .NET 版不会互相覆盖。
- 系统 SDK 与托管 SDK 的选择。
- `suggest` 不修改项目目录或当前版本。
- `source use` 重排 source_order、保留未知字段、无配置文件时创建、原子写回。
- `source ban/unban` 强禁用：被禁用的来源不参与自动 fallback 与默认枚举，显式指定或 use 时报错，
  unban 后恢复；写回保留未知字段。
- `install --source` 只用指定源（其余源不被调用）、指定源不可用不 fallback、未知来源名报错。
- `available` 过滤 rc 和非三段 tag、按当前平台资产判断 edition（如 3.x 的 x11 命名不列出）、
  多源合并去重、单源失败不影响其余、自定义源不支持枚举时报配置错误。
- CLI 命令简写 `i/l/s/a` 与 flag 简写 `-e/-s` 解析正确；无参数 `install` 在非 TTY 下返回用法
  错误，不进入交互流程。
- CLI 进度渲染：TTY 下 `\r` 行内重绘破折号线（已下载段品牌色 #3A73B0，即 Go/Godot/C# 三色
  平均；终端不支持 truecolor 时回退绿色；未下载段灰色；`NO_COLOR` 时无色），100ms 节流、
  完成/警告前清行；非 TTY 下按 8 MiB 打点。
- Linux 与 macOS 的路径、资产名称和环境差异。

所有测试显式传入 `t.TempDir()` 下的 gdit 根目录。source 测试使用内存 `http.RoundTripper` 或
`httptest.Server` 和固定资产，分别返回成功、404、5xx、截断响应、错误摘要和超时。真实网络只用于
手工 smoke test，不进入默认测试。

store 测试要在原子 rename 前后注入失败，确认半成品只留在 `tmp/`、已经发布的目录可以重建 state，
并发安装同一版本只产生一个有效结果，安装开始时清理遗留的 `operation-*` 目录且不误删其他内容。
archive 测试包含绝对路径、`..`、symlink 逃逸和异常文件类型。
CLI 集成测试捕获 stdout 和 stderr，确认结果与进度没有串流。

macOS 行为分为两层。可在 Linux 运行的纯 target 映射使用固定输入测试，涉及 app bundle、权限、symlink
和子进程环境的验收必须在 Apple Silicon 实机完成，交叉编译结果只算构建检查。

## 9. 框架搭建与第一阶段

第一阶段是 Linux amd64 上可实际使用的 `install/list` 纵向切片。它包含必要框架，同时拒绝空 package、
TODO handler 和只为编译通过的 GUI 占位。

### 9.1 交付范围

1. 建立 `go.work`、`core` module 和 `cli` module，module 路径使用 review 后确认的仓库地址。
2. 实现版本、edition、platform target、路径和错误等值对象。
3. 实现配置读取、来源顺序、自定义来源模板和敏感信息脱敏。
4. 实现 source provider、固定 fixture、下载取消、错误分类和 fallback。
5. 实现安全解压、完整安装标记、全局修改锁、原子发布、state 原子写入与重建。
6. 实现 core `Install` 与 `List`，随后接入薄 CLI。
7. 完成单元测试、core 集成测试和 CLI 集成测试，再做一次不替代 fixture 的 Linux amd64 手工安装。

第一阶段不包含 remove、use、shim、环境注入、.NET SDK 选择、doctor、suggest、模板管理和 GUI。
`dotnet` edition 的引擎资产可以安装和列出，但兼容 SDK 检测及启动要到“环境与 .NET”阶段完成；CLI
必须在输出中避免暗示这一阶段已经能完整启动 C# 项目。

### 9.2 验收标准

- `gdit install --edition standard 4.5.2` 可以从配置的来源安装，重复执行返回稳定的已安装结果。
- `gdit install --edition dotnet 4.5.2` 安装到不同版本 ID，不覆盖标准版。
- 第一来源不可用时按顺序 fallback，任一来源发生摘要不匹配时立即停止。
- 下载取消、进程中断和解压失败不会让 `list` 看见半成品。
- 删除或损坏 `state.toml` 后，`list` 能从有效版本目录重建；无效目录只由 doctor 的后续实现报告。
- 并发执行安装时，版本目录、安装标记和 state 始终一致。
- 默认测试不访问开发者真实主目录和公网。
- `go test ./core/... ./cli/...`、`go vet ./core/... ./cli/...` 和格式检查全部通过。

### 9.3 后续顺序

来源管理（4.7：source list/use、install --source、available）作为第一阶段的扩展实现，
不改变以下阶段顺序。

```text
第一阶段  install/list
第二阶段  remove + use/setup/shim
第三阶段  环境注入 + .NET SDK
第四阶段  doctor
第五阶段  suggest + 导出模板
第六阶段  Wails GUI
```

remove 放到第二阶段，与 current 的引用约束一起实现，避免第一阶段先产生一套随后要改的删除语义。

## 10. Review 结论

2026-08-17 已确认以下第一阶段决策。

1. 使用来源自身提供的摘要（GodotHub 为 SHA-256，官方 github 为 SHA-512），来源同时承担下载和
   摘要信任责任。
2. Go module 使用 `github.com/Sheyiyuan/GoDoIt`。
3. 第一阶段只验收 Linux amd64。MVP 是否承诺 Linux arm64 留到平台验证时再决定。
4. 2026-08-17 实测确认 GodotHub（releases.json digest + atomgit 下载）和 Godot 官方（godot-builds
   release + SHA512-SUMS.txt）的稳定资产与校验 URL 规则，已写入内置 provider。AtomGit 独立来源的
   规则仍待确认；规则确认前不把猜测地址固化成内置来源，默认 source_order 为 `["godothub", "github"]`，
   显式配置 atomgit 时返回配置错误。

2026-08-18 追加确认来源管理扩展决策。

5. 交互式 `install` 使用 `github.com/AlecAivazis/survey/v2` 实现方向键选择（edition → version →
   source），仅限 CLI 层；非 TTY 下无参数 install 报用法错误。
6. 命令简写 `i/l/s/a`、flag 简写 `-e/-s`；后续阶段命令可能重新占用简写。
7. `source ban/unban` 为强禁用语义，状态写入 config.toml 的 `disabled_sources`。
8. `gdit source` 无参数即列出来源（保留 `source list` 写法兼容）。

框架完成后仍按项目约定先展示、review，得到用户明确的“可以”以后才能 commit。
