# GoDoIt 需求文档（PRD）

> GoDoIt ｜ CLI/包名：gdit  
> Go! Do It! 不等戈多，自己动手。  
> 状态：v0.2 第一阶段实施中

## 1. 项目定位

GoDoIt 是面向 Linux（主）和 macOS（验证）的 Godot 引擎版本管理器。它统一管理 Godot 引擎、
导出模板和配套 .NET SDK，并提供 CLI 与 Wails GUI 两种入口。

GoDoIt 管理的是引擎，不管理 Godot 项目：

- 不在项目目录创建 `.gdit`、配置文件或 lock。
- 不保存“项目路径 → 引擎版本”的关联。
- `project.godot`、`global.json` 和 `.csproj` 只在用户执行 `suggest` 时只读分析。
- 普通 `godot` 命令始终启动当前全局版本，不根据工作目录自动切换。

## 2. 用户目录

GoDoIt 的配置、状态、命令入口、引擎、SDK 和缓存统一位于用户级 `~/.gdit/`：

```text
~/.gdit/
├── config.toml
├── state.toml
├── bin/
│   └── godot -> <gdit 可执行文件>
├── current -> versions/<version-id>/
├── versions/
├── dotnet/
├── templates/
├── cache/
└── tmp/
```

除用户级命令入口外，不写入项目目录、系统目录或其他应用目录。

## 3. 功能需求

### FR-01 引擎安装（P0 · 第一阶段）

`gdit install <版本>` 安装 Godot 标准版或 .NET 版。

- MVP 只支持精确的稳定版本，例如 `4.5.2`；预览版不在首版范围内。
- 内置 GodotHub 国内镜像和 GitHub 官方源；AtomGit 独立来源规则确认后加入内置。
- 支持用户在 `config.toml` 中添加自定义源。
- 默认按 GodotHub → GitHub 自动 fallback；AtomGit 规则确认后加入默认顺序。
- 下载完成后按来源声明的摘要（sha256 或 sha512）校验，校验失败不得安装。
- 安装失败或中断不能留下被识别为完整版本的半成品。

### FR-02 版本切换（P0 · 第二阶段）

`gdit use <版本>` 将已安装版本设为当前全局版本。

- 通过原子更新 `~/.gdit/current` symlink 完成切换。
- `gdit setup` 显式创建或修复 `~/.gdit/bin/godot` shim，但不修改 shell 配置或系统 PATH。
- PATH 中的 `godot` shim 指向 `gdit`，由 gdit 读取 `current`、注入环境并启动真实引擎。shim 不访问网络。
- `use` 只能选择已完整安装的版本，不触发隐式下载。
- 当前版本对所有目录一致，不做项目级自动切换。

### FR-03 版本查看与卸载（P0 · 第一/二阶段）

- `gdit list` 列出已安装版本并标记当前版本。
- `gdit remove <版本>` 卸载指定版本。
- 当前版本不能直接卸载，必须先切换或显式取消当前选择。

### FR-04 启动环境变量（P0 · 第三阶段）

启动 Godot 时向子进程注入环境，不修改用户全局环境。

- 全局默认和每版本覆盖都写在 `~/.gdit/config.toml`。
- 支持 `DOTNET_ROOT`、PATH 前缀、显示驱动和 fcitx 相关变量。
- Linux 下显示驱动默认自动检测，不统一强制 x11。
- fcitx 只在 Linux 检测到或用户明确启用时注入。
- macOS 不注入 Linux 专用变量。

### FR-05 .NET SDK 管理（P0 · 第三阶段）

为 Godot .NET 版检测并提供兼容 SDK。

- 检测系统已安装 SDK：`dotnet --list-sdks`。
- 按 Godot 版本映射最低兼容 SDK。
- 系统 SDK 不满足时，用户可通过显式的 CLI 或 GUI 操作确认下载到 `~/.gdit/dotnet/`。
- 普通 `godot` 启动只选择已有 SDK；没有兼容 SDK 时报错并给出安装提示，不交互、不下载。
- 多个托管 SDK 可以共存。
- 选择托管 SDK 时通过目标子进程的 `DOTNET_ROOT` 和 PATH 前缀优先使用它。
- 不卸载、不禁用、不修改系统 dotnet。

### FR-06 项目分析建议（P1 · 第五阶段）

`gdit suggest [项目目录]` 只读分析项目需要的引擎和 .NET SDK。

- 读取 `project.godot` 的 `config/features` 判断版本系列与 C# 标志。
- 可读取 `global.json` 和 `.csproj` 辅助判断 SDK。
- 输出建议清单，不在项目内写文件，也不改变当前全局版本。
- 用户显式确认或传入 `--install` 后，可以安装建议的引擎、SDK 和模板。

### FR-07 导出模板（P1 · 第五阶段）

安装、列出和卸载与指定 Godot 版本匹配的导出模板，统一存放在 `~/.gdit/templates/`。

### FR-08 环境诊断（P0 · 第四阶段）

`gdit doctor` 检查：

- `~/.gdit/` 目录与权限。
- `godot` shim 和 PATH。
- 当前 symlink 是否指向完整安装。
- 引擎、模板和 SDK 状态。
- 最终会注入的环境变量。
- 下载源可用性和配置错误。

doctor 默认只报告和建议，不静默修改。

### FR-09 GUI（P1 · 第六阶段）

Wails GUI 提供版本列表、安装/卸载、当前版本切换、项目分析、设置、doctor 和关于页面。

- GUI 与 CLI 调用同一个 core。
- GUI 不维护独立业务规则。
- “项目分析”由用户显式选择目录触发，不维护项目列表或扫描主目录。

### FR-10 缓存管理（P2 · 后续阶段）

查看并清理 `~/.gdit/cache/` 中的下载缓存。

### FR-11 headless 运行（P2 · 后续阶段）

`gdit exec -- <参数>` 使用当前全局版本运行 Godot，并透传参数、输出和退出码。

### FR-12 来源管理（P1 · 第一阶段扩展）

- `gdit source` 查看当前来源顺序、类型与禁用状态；`gdit source use <name>` 把指定来源设为默认
  首位并写回 `config.toml`，不丢失其他配置字段。
- `gdit source ban/unban <name>` 强禁用/启用来源：被禁用的来源不参与自动 fallback 与默认探测，
  显式指定或设为默认时同样报错。
- `gdit install --source <name>` 用指定来源下载，该来源失败时不自动降级到其他来源。
- `gdit available [--source <name>]` 探测默认或指定来源上、当前平台可安装的稳定版本与 edition；
  URL 模板型自定义源无法枚举版本，探测时返回明确的配置错误。
- `gdit install` 无参数时进入交互式安装（仅终端可用），依次选择 edition、version 和 source。
- 命令与 flag 支持简写：`i/l/s/a`、`-e`、`-s`。

## 4. 非功能需求

| 类别 | 要求 |
|---|---|
| 分发 | CLI 单二进制；GUI 独立构建 |
| 平台 | Linux 主，macOS Apple Silicon 验证，不支持 Windows |
| 配置 | 手写配置只使用 TOML |
| 无侵入 | 不改系统文件、shell 配置、全局环境或系统 dotnet |
| 健壮 | 多源 fallback，安装和 current 切换不暴露半成品 |
| 安全 | 下载内容必须按来源声明的摘要校验，自定义源由用户明确配置 |
| 架构 | CLI 和 GUI 共享 core，平台差异集中在 platform 适配层 |
| 输出 | 用户结果走 stdout，调试与进度走 stderr |

## 5. MVP 验收标准

P0 包括 FR-01、FR-02、FR-03、FR-04、FR-05 和 FR-08。**P0 全集是 MVP 的最终目标；第一阶段只
交付其中 install/list 与来源管理（FR-01 的安装部分、FR-03 的查看部分、FR-12）**，其余按架构
文档 §9.3 的阶段顺序在后续阶段落地。

1. 能安装、列出、切换和卸载多个 Godot 版本。
2. `godot` 在任意目录都启动 `~/.gdit/current` 指向的版本。
3. .NET 版能选择兼容 SDK，并且环境变量只影响启动的子进程。
4. 国内源不可用时可以降级到下一来源，摘要不匹配时停止安装。
5. 安装中断不会污染版本列表，current 切换失败仍保留旧版本。
6. `gdit doctor` 能诊断目录、shim、当前版本、SDK 和环境。

具体实现约束见 [`docs/architecture/`](architecture/README.md)。
