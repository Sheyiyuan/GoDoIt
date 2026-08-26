# GoDoIt 命令参考（操作 ↔ 指令）

> 状态：v0.2 第五阶段 CLI 实现完成；第六阶段 Linux 基础 GUI 实现完成，阶段 A 可用性实现与自动测试完成
> 本文档从用户操作视角列出所有指令与输出约定；设计语义以
> [`docs/architecture/README.md`](architecture/README.md) 为唯一真理源。
> 本文记录截至第五阶段的 CLI 命令面；第六阶段 GUI 不改变这些命令契约。第三阶段起不保留第二阶段命令兼容层。

## 1. 操作与指令对照

### 1.1 条目安装与管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 交互式创建条目 | `gdit install` / `gdit new` | 依次确认显示名、edition、版本、SDK 策略和是否设为当前；仅 TTY 可用。Godot 3.x dotnet 版自动 mono 策略，不询问 SDK |
| 非交互创建标准版条目 | `gdit install work --version 4.5.2` | `--edition` 默认 `standard`；显示名和精确稳定版本均必填 |
| 非交互创建 .NET 条目 | `gdit install work-csharp --version 4.5.2 --edition dotnet` | SDK 策略默认 `managed`，推荐 patch 作为依赖安装 |
| 使用系统 SDK | `gdit install work-csharp --version 4.5.2 --edition dotnet --sdk system` | 不安装托管 SDK；启动时使用系统 dotnet |
| 指定托管 SDK | `gdit install work-csharp --version 4.5.2 --edition dotnet --sdk managed --sdk-version 8.0.410` | SDK 版本只接受精确三段版本号 |
| Godot 3.x mono 条目 | `gdit install old --version 3.6.2 --edition dotnet` | 只下载安装引擎；运行时由系统 Mono 提供，传 `--sdk` 选项报错 |
| 控制 current | `--current` / `--no-current` | 两者互斥；均未给出时，仅在尚无 current 时把新条目设为当前 |
| 安装并绑定模板 | `gdit install work --version 4.5.2 --template` | 普通安装默认不装模板；交互安装会询问，默认否 |
| 查看条目 | `gdit list` | 列出名称、引擎、edition、SDK 策略、模板及 current 标记 |
| 删除条目 | `gdit remove [-y\|--yes] <name>` | 当前条目拒绝删除；删除后提示孤儿资产，不自动删除资产 |

> 命令简写：`install`→`i`、`list`→`l`、`source`→`s`、`available`→`a`、`default`→`d`、
> `run`→`r`、`remove`→`rm`、`setup`→`st`、`env`→`e`
>
> `run` 透传引擎参数请放在 `--` 之后，如 `gdit run -- -e`，避免被 gdit 自身解析

### 1.2 当前条目与启动

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看当前条目 | `gdit default` | 显示显示名、引擎、edition 和 SDK 策略；未设置或链接无效时报错 |
| 设置当前条目 | `gdit default <name>` | 原子更新 `~/.gdit/current`；条目及其引擎引用必须完整，失败保留旧链接 |
| 创建 godot 命令入口 | `gdit setup` | 创建/修复 `~/.gdit/bin/godot` shim；不改 shell 配置和系统 PATH，bin 不在 PATH 时提示 |
| 启动当前条目 | `gdit run [-- 参数]` / `gdit run -d [-- 参数]` | 等价于裸 `godot`；无参数 + TTY 且多于一个条目时先交互选择要启动的条目 |
| 启动指定条目 | `gdit run <name> [-- 参数]` | 不改变 current；显示名与版本输入/资产 ID 分属不同命名空间 |
| 启动桌面 GUI | `gdit gui [参数]` | 启动配套的 `gdit-gui`，参数和 GUI 退出码原样透传；可用 `GDIT_GUI` 指定路径 |

仓库开发时，`make run` 会先构建并启动 GUI；需要启动 CLI 可使用 `make run-cli <command>`。

### 1.3 引擎资产管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看引擎资产 | `gdit engine` / `gdit engine list` | 列出已安装引擎及引用状态 |
| 安装引擎资产 | `gdit engine install [--edition standard\|dotnet] [--source <name>] <版本>...` | 保留 `m<版本>` 和批量安装语法；不创建条目、不安装 SDK、不改变 current |
| 删除引擎资产 | `gdit engine remove [-y\|--yes] <版本>` | 接受 `<版本>`（`4.5.2`/`m4.5.2`）或资产 ID（`4.5.2-dotnet`，与 `engine list` 输出一致）；被条目引用时拒绝删除 |
| 清理孤儿资产 | `gdit autoremove [-y\|--yes]` | 确认后锁内复查并删除仍无引用的引擎/SDK/模板；存在坏条目时不删除任何资产 |

### 1.4 SDK 与环境

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看 SDK | `gdit sdk` / `gdit sdk list` | 列出系统和托管 SDK |
| 查看可安装 SDK | `gdit sdk available` | 动态枚举 .NET 官方通道（releases-index.json），按大版本分组输出稳定 SDK |
| 安装托管 SDK | `gdit sdk install [<版本>]` | 只接受精确版本，校验 SHA-512 后原子发布；无参数 + TTY 时两级选择（先大版本通道，再具体 patch） |
| 删除托管 SDK | `gdit sdk remove [-y\|--yes] <版本>` | 被 managed 条目引用时拒绝删除 |
| 查看注入环境 | `gdit env [--instance <name>]` | 未指定条目时查看 current 的最终注入增量和引擎参数 |
| 设置环境变量 | `gdit env set <KEY=VALUE> [--instance <name>]` | 无 `--instance` 写全局 `[environment]`，否则写条目 `[env]` |
| 删除环境变量 | `gdit env unset <KEY> [--instance <name>]` | 原子写回并保留未知字段 |

### 1.5 来源管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看当前来源顺序与类型 | `gdit source` | 只读，按 `source_order` 输出优先级、名称、类型（builtin/custom）和禁用状态；`gdit source list` 同义 |
| 把某来源设为默认首位 | `gdit source use <name>` | 移到 `source_order` 首位并原子写回 `config.toml`，其余保持相对顺序；被禁用的来源不能 use |
| 禁用某来源 | `gdit source ban <name>` | 强禁用：不再参与自动 fallback 和默认枚举；显式指定或 use 被禁用的来源也会报错，必须先 unban |
| 启用某来源 | `gdit source unban <name>` | 恢复自动 fallback 参与 |
| 添加自定义源 | 编辑 `~/.gdit/config.toml` | 见第 3 节示例；需同时提供资产 URL 模板与校验清单 URL |

### 1.6 可用版本探测

| 操作 | 指令 | 说明 |
|---|---|---|
| 探测默认来源的可用版本 | `gdit available` | 合并所有启用且支持枚举的来源，按系列分组输出版本、edition、来源；单来源失败不影响其余 |
| 探测指定来源 | `gdit available --source github` | 只用该来源；自定义源（URL 模板型）不支持枚举，返回配置错误 |
| 探测结果范围 | — | 只列当前平台可安装的版本：两/三段稳定版（如 `4.5.2`、`4.7`）与预发布（如 `4.8-dev3`、`4.7.2-rc1`） |

### 1.7 项目建议与导出模板

| 操作 | 指令 | 说明 |
|---|---|---|
| 只读分析项目 | `gdit suggest [<项目目录>]` | 只读取同目录 `project.godot`、`global.json` 与 `*.csproj`；默认零网络、零落盘 |
| 按建议安装 | `gdit suggest <目录> --install --name <条目名>` | 重新分析后解析同系列最高稳定版，默认安装并绑定模板；`--no-template` 跳过 |
| 查看模板 | `gdit template` / `gdit template list` | 列出版本、edition、来源、大小、引用与已验证 payload 路径 |
| 安装模板 | `gdit template install [--edition standard\|dotnet] [--source <name>] <版本>` | 校验来源摘要、归档布局和 `version.txt` 后原子发布 |
| 绑定/解绑 | `gdit template attach [--source <name>] <条目名>` / `detach <条目名>` | attach 会安装缺失资产；detach 只解除引用并报告孤儿 |
| 删除模板 | `gdit template remove [-y\|--yes] [--edition standard\|dotnet] <版本>` | 被条目引用时拒绝删除 |

### 1.8 环境诊断

| 操作 | 指令 | 说明 |
|---|---|---|
| 本地只读诊断 | `gdit doctor` | 检查平台、根目录、shim、current、条目、资产、环境、来源和 state；默认零网络、零落盘、不拿修改锁 |
| 探测来源可达性 | `gdit doctor --network` | 额外探测启用来源；单来源失败为警告，全部失败为错误 |
| 展开诊断细节 | `gdit doctor --verbose` | 展开环境变量来源、来源状态与修复建议；敏感值仍保持掩码 |

### 1.9 帮助

| 操作 | 指令 |
|---|---|
| 查看命令总览 | `gdit --help` / `gdit help` |

## 2. 输出与退出码约定

- **stdout**：只输出结果（如 `installed instance work`、`default: work`、doctor 报告、
  `removed instance work`、条目或资产列表），机器可读，tab 分隔。`list` 中当前条目的整行在
  TTY 下用品牌色（#3A73B0，truecolor 不支持时回退绿色，存在 `NO_COLOR` 时无色）高亮，
  非 TTY 保持纯文本。
- **stderr**：进度、警告、错误和交互提示。下载进度在终端（TTY）下为单行动画：
  `版本ID(来源)  破折号线  已下载/总量`（如 `4.5.1-dotnet(godothub)  58.30 MiB/88.30 MiB`），
  已下载段用品牌色（Go/Godot/C# 三色平均 #3A73B0，终端不支持 truecolor 时回退绿色）、
  未下载段灰色（存在 `NO_COLOR` 环境变量时无色，只画已下载段）；非 TTY 下按 8 MiB 打点
  （`downloaded 4.5.1-dotnet 16 MB / 30 MB from github`）。批量安装时标签携带版本 ID，
  可区分正在下载的版本。
- **退出码**：`0` 成功；`1` 输入/配置/网络/完整性/本地 I/O 错误、确认被拒绝（TTY 选否或
  非 TTY 未带 `-y`）；`2` 用法错误（未知命令、参数缺失、flag 位置错误）。

常见错误信息：

| 场景 | 信息（前缀） |
|---|---|
| 条目名非法 | `invalid input: invalid instance name` |
| 条目不存在 | `instance not found: work` |
| 版本语法错误 | `invalid input: version must be MAJOR.MINOR.PATCH` |
| edition 非法 | `invalid input: edition must be standard or dotnet` |
| 来源不存在 | `invalid config: source "missing" is not configured` |
| 来源被禁用 | `invalid config: source "godothub" is disabled` |
| 重复安装 | `version already installed: 4.5.2-standard` |
| 所有来源不可用 | `all sources unavailable: …` |
| 摘要不匹配 | `<sha256|sha512> mismatch for <资产> from <来源>` |
| 未设置当前条目 | `no current instance set; run "gdit default <name>" first` |
| 引擎资产未安装 | `engine not installed: 4.5.2-standard` |
| 当前条目不可删除 | `cannot remove current instance: work` |
| SDK 缺失 | `compatible SDK not installed: 8.0.410; run "gdit sdk install 8.0.410"` |
| 条目扫描失败 | `invalid config: cannot determine asset references` |

## 3. 配置与数据目录

所有数据默认在 `~/.gdit/`；设置绝对路径 `GDIT_ROOT` 后，以下布局整体迁移到该根目录：

```text
~/.gdit/
├── config.toml    # 唯一用户配置文件（来源、全局环境）
├── state.toml     # gdit 维护的已安装资产索引（可自动重建）
├── current        # Unix 相对 symlink；Windows 为规范相对路径重定向文件
├── instances/     # 启动器条目：<uuid>.toml，文件内为显示名、引擎引用、SDK 策略与环境
├── engines/       # 已安装引擎资产，每个资产一个目录
├── sdks/          # 托管 .NET SDK 资产
├── templates/     # 已验证导出模板资产（不进入 state.toml）
└── tmp/           # 下载/解压临时目录（中断残留自动清理）
```

自定义源示例（`config.toml`）：

```toml
schema_version = 1
source_order = ["godothub", "github"]   # 默认；可改顺序或加入自定义源
disabled_sources = []                   # source ban 写入的禁用名单

[[custom_sources]]
name = "company-mirror"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "GDIT_COMPANY_MIRROR_TOKEN"

[environment]
display_driver = "auto"
input_method = "auto"
COMMON_VALUE = "global"

[environment.windows]
PLATFORM_VALUE = "windows-only"
```

- 占位符只允许 `{version}`、`{tag}`、`{asset}`；URL 必须 HTTPS（localhost 测试除外）。
- `gdit source use` / `source ban/unban` 写回会保留全部字段，注释不保留。
- `[environment.linux|darwin|windows]` 仅在对应平台生效并覆盖全局同名键；macOS/Windows 的
  `display_driver` 与 `input_method` 只接受 `auto`。

## 4. 快速上手

```bash
gdit available
gdit install work --version 4.5.2 --current
gdit install work-csharp --version 4.5.2 --edition dotnet
gdit list
gdit default work-csharp
gdit setup
godot -e
gdit run work -- -e
gdit remove -y work
gdit autoremove
```

第三阶段只支持全新的 `engines/`、`sdks/`、`instances/` 布局，不读取、不迁移第二阶段数据。
条目文件以 UUID v4 命名（`instances/<uuid>.toml`），显示名存放在文件内：ASCII 字符只允许
`[A-Za-z0-9._~-]`（URL 安全），非 ASCII 文字（中文等）允许；显示名全仓库唯一，不与版本输入
或资产 ID 互相排斥。例如 `工作`、`work-4.5`、`4.5.2`（只是一个恰好叫这个名字的条目）都合法，
`a b`、`a/b`、`a!b` 非法。
