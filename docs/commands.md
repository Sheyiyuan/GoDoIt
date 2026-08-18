# GoDoIt 命令参考（操作 ↔ 指令）

> 状态：v0.2 第一阶段（install/list + 来源管理）
> 本文档从用户操作视角列出所有指令与输出约定；设计语义以
> [`docs/architecture/README.md`](architecture/README.md) 为唯一真理源。

## 1. 操作与指令对照

### 1.1 安装与版本管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 交互式安装 | `gdit install` | 依次上下选择 edition、version 和 source；仅在终端（TTY）可用，脚本环境请用参数形式 |
| 安装标准版 | `gdit install 4.5.2` | `--edition` 默认 `standard`，只接受精确稳定版（如 `4.5.2`），拒绝 `latest`、预览版和范围版本 |
| 安装 .NET 版 | `gdit install --edition dotnet 4.5.2` | 输入 `mono` 会被归一化为 `dotnet`；与标准版并存，不互相覆盖 |
| 指定来源安装 | `gdit install --source github 4.5.2` | 只使用该来源，失败不降级到其他来源；来源不存在或被禁用时报配置错误 |
| 查看已安装版本 | `gdit list` | 列出完整安装及其平台、来源；`state.toml` 损坏时自动按版本目录重建 |

> 命令简写：`install`→`i`、`list`→`l`、`source`→`s`、`available`→`a`，例如 `gdit i 4.5.2`
>
> flag 简写：`--edition`→`-e`、`--source`→`-s`，例如 `gdit i -e dotnet -s github 4.5.2`
>
> 后续阶段命令可能重新占用简写，以各阶段文档为准

### 1.2 来源管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看当前来源顺序与类型 | `gdit source` | 只读，按 `source_order` 输出优先级、名称、类型（builtin/custom）和禁用状态；`gdit source list` 同义 |
| 把某来源设为默认首位 | `gdit source use <name>` | 移到 `source_order` 首位并原子写回 `config.toml`，其余保持相对顺序；被禁用的来源不能 use |
| 禁用某来源 | `gdit source ban <name>` | 强禁用：不再参与自动 fallback 和默认枚举；显式指定或 use 被禁用的来源也会报错，必须先 unban |
| 启用某来源 | `gdit source unban <name>` | 恢复自动 fallback 参与 |
| 添加自定义源 | 编辑 `~/.gdit/config.toml` | 见第 3 节示例；需同时提供资产 URL 模板与校验清单 URL |

### 1.3 可用版本探测

| 操作 | 指令 | 说明 |
|---|---|---|
| 探测默认来源的可用版本 | `gdit available` | 合并所有启用且支持枚举的来源，输出版本、edition、来源；单来源失败不影响其余 |
| 探测指定来源 | `gdit available --source github` | 只用该来源；自定义源（URL 模板型）不支持枚举，返回配置错误 |
| 探测结果范围 | — | 只列当前平台可安装的稳定精确版（过滤 rc/beta/dev 和非三段 tag），如 `4.5.2`、`4.7.1` |

### 1.4 帮助

| 操作 | 指令 |
|---|---|
| 查看命令总览 | `gdit --help` / `gdit help` |

## 2. 输出与退出码约定

- **stdout**：只输出结果（如 `installed 4.5.2-standard`、版本列表），机器可读，tab 分隔。
- **stderr**：进度、警告和错误。下载进度在终端（TTY）下为单行动画：
  `来源名  破折号线  已下载/总量`，已下载段用品牌色（Go/Godot/C# 三色平均 #3A73B0，
  终端不支持 truecolor 时回退绿色）、未下载段灰色（存在 `NO_COLOR` 环境变量时无色，
  只画已下载段）；非 TTY 下按 8 MiB 打点（`downloaded 16 MB / 30 MB from github`）。
- **退出码**：`0` 成功；`1` 输入/配置/网络/完整性/本地 I/O 错误；`2` 用法错误（未知命令、参数缺失）。

常见错误信息：

| 场景 | 信息（前缀） |
|---|---|
| 版本语法错误 | `invalid input: version must be MAJOR.MINOR.PATCH` |
| edition 非法 | `invalid input: edition must be standard or dotnet` |
| 来源不存在 | `invalid config: source "missing" is not configured` |
| 来源被禁用 | `invalid config: source "godothub" is disabled` |
| 重复安装 | `version already installed: 4.5.2-standard` |
| 所有来源不可用 | `all sources unavailable: …` |
| 摘要不匹配 | `<sha256|sha512> mismatch for <资产> from <来源>` |

## 3. 配置与数据目录

所有数据在 `~/.gdit/`：

```text
~/.gdit/
├── config.toml    # 唯一用户配置文件
├── state.toml     # gdit 维护的已安装版本索引（可自动重建）
├── versions/      # 已安装引擎，每个版本一个目录
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
```

- 占位符只允许 `{version}`、`{tag}`、`{asset}`；URL 必须 HTTPS（localhost 测试除外）。
- `gdit source use` / `source ban/unban` 写回会保留全部字段，注释不保留。

## 4. 快速上手

```bash
gdit available                     # 看有哪些版本可装
gdit install 4.5.2                 # 默认顺序：godothub → github
gdit install --edition dotnet 4.5.2
gdit list                          # 确认两个版本
gdit source                        # 查看来源
gdit source use github             # 以后默认先试 github
gdit source ban godothub           # 完全禁用 godothub（可选）
gdit install                       # 交互式安装（终端下）
```

## 5. 后续阶段命令预览（未实现）

以下操作属于后续阶段（见架构文档 §9.3），当前指令无效，仅作设计预览：

| 阶段 | 操作 | 计划指令 |
|---|---|---|
| 二 | 切换当前版本 | `gdit use <版本>` |
| 二 | 卸载版本 | `gdit remove <版本>` |
| 二 | 创建 `godot` 命令入口 | `gdit setup` |
| 三 | 环境与 .NET SDK 管理 | （设计待定） |
| 四 | 环境诊断 | `gdit doctor` |
| 五 | 项目只读分析建议 | `gdit suggest [目录]` |
| 五 | 导出模板管理 | （设计待定） |
| 六 | GUI | Wails 应用 |
