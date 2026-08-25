---
title: 命令参考
description: gdit 命令、参数和行为约定的速查表。
section: reference
order: 1
updated: 2026-08-25
---

命令简写为 `install`→`i`、`list`→`l`、`source`→`s`、`available`→`a`、`default`→`d`、`run`→`r`、`remove`→`rm`、`setup`→`st`、`env`→`e`。

## 条目安装与管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 交互式创建条目 | `gdit install` / `gdit new` | 依次确认显示名、edition、版本、SDK 策略和是否设为当前；仅 TTY |
| 非交互创建标准条目 | `gdit install work --version 4.5.2` | `--edition` 默认 `standard` |
| 创建 .NET 条目 | `gdit install work-cs --version 4.5.2 --edition dotnet` | SDK 默认 `managed`，推荐 patch 一并安装 |
| 用系统 SDK | `gdit install work-cs --version 4.5.2 --edition dotnet --sdk system` | 不装托管 SDK |
| 指定托管 SDK | `gdit install work-cs --version 4.5.2 --edition dotnet --sdk managed --sdk-version 8.0.410` | 精确三段版本号 |
| 控制 current | `--current` / `--no-current` | 互斥；均未给出时仅尚无 current 才设为当前 |
| 查看条目 | `gdit list` | 名称、引擎、edition、SDK 策略、current 标记 |
| 删除条目 | `gdit remove [-y\|--yes] <name>` | 当前条目拒绝删除；删除后提示孤儿资产 |

## 当前条目与启动

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看当前条目 | `gdit default` | 未设置或指针无效时报错 |
| 设置当前条目 | `gdit default <name>` | 原子更新 current 指针，失败保留旧值 |
| 创建 godot 入口 | `gdit setup` | Unix 创建 `godot` symlink，Windows 创建 `godot.cmd`；不改 shell 或系统 PATH |
| 启动当前条目 | `gdit run [-- 参数]` / `-d` | 等价裸 `godot` |
| 启动指定条目 | `gdit run <name> [-- 参数]` | 不改变 current |
| 启动桌面 GUI | `gdit gui [参数]` | 启动配套的 `gdit-gui`，参数和退出码原样透传；可用 `GDIT_GUI` 指定路径 |

仓库开发入口为 `make run`（构建并启动 GUI）；需要启动 CLI 时使用 `make run-cli <command>`。

## 引擎资产

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看引擎 | `gdit engine` / `gdit engine list` | 含引用状态 |
| 安装引擎资产 | `gdit engine install [--edition standard\|dotnet] [--source <name>] <版本>...` | 支持 `m<版本>` 简写与批量；不建条目不装 SDK |
| 删除引擎资产 | `gdit engine remove [-y\|--yes] <版本>` | 被引用时拒绝 |
| 清理孤儿 | `gdit autoremove [-y\|--yes]` | 确认后锁内复查再删；坏条目时不动任何资产 |

## SDK 与环境

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看 SDK | `gdit sdk` / `gdit sdk list` | 托管 + 系统 |
| 查看可装 SDK | `gdit sdk available` | .NET 官方元数据 |
| 安装托管 SDK | `gdit sdk install [<版本>]` | 校验 SHA-512 后原子发布；无参数 + TTY 时从可选列表选择 |
| 删除托管 SDK | `gdit sdk remove [-y\|--yes] <版本>` | 被引用时拒绝 |
| 查看注入环境 | `gdit env [--instance <name>]` | 未指定时看 current 的最终注入 |
| 设置环境变量 | `gdit env set <KEY=VALUE> [--instance <name>]` | 无 `--instance` 写全局 `[environment]` |
| 删除环境变量 | `gdit env unset <KEY> [--instance <name>]` | 原子写回并保留未知字段 |

## 来源管理

| 操作 | 指令 | 说明 |
|---|---|---|
| 查看来源 | `gdit source` | 顺序、类型、禁用状态 |
| 设为默认首位 | `gdit source use <name>` | 写回 `config.toml`，保留其他字段 |
| 强禁用/启用 | `gdit source ban/unban <name>` | 禁用来源不参与 fallback，显式指定也报错 |
| 指定来源安装 | `gdit engine install <版本> --source <name>` | 该来源失败不自动降级 |
| 探测可装版本 | `gdit available [--source <name>]` | URL 模板型自定义源无法枚举，返回配置错误 |

## 环境诊断

| 操作 | 指令 | 说明 |
|---|---|---|
| 本地诊断 | `gdit doctor` | 检查平台、根目录、shim、current、条目、资产、环境、来源和 state；默认零网络、零落盘 |
| 网络探测 | `gdit doctor --network` | 额外探测启用来源的可达性 |
| 展开细节 | `gdit doctor --verbose` | 显示环境来源、来源状态和修复建议；敏感值仍掩码 |

## 项目建议与导出模板

| 操作 | 命令 | 边界 |
|---|---|---|
| 只读分析项目 | `gdit suggest [<项目目录>]` | 默认零网络、零落盘，不改变 current |
| 按建议安装 | `gdit suggest <目录> --install --name <条目名>` | 重新分析后复用条目安装流程 |
| 管理导出模板 | `gdit template list/install/remove` | 精确版本 + edition，摘要校验后原子发布 |
| 绑定模板条目 | `gdit template attach/detach <条目名>` | 模板作为可选条目依赖，参与引用保护和 GC |

详细阶段边界见 [开发状态与阶段范围](/wiki/reference/development-status/)。

## 约定

- 版本输入只接受精确三段稳定版本，例如 `4.5.2`。条目显示名是 URL 安全字符集（ASCII 只允许 `[A-Za-z0-9._~-]`，非 ASCII 文字如中文允许），全仓库唯一，与版本号/资产 ID 分属不同命名空间。
- 传给引擎的参数放在 `--` 之后，例如 `gdit run -- -e`。
- 用户结果写入 stdout，调试、进度和错误写入 stderr。
- `GDIT_ROOT` 可把全部用户数据迁移到任意绝对路径；未设置时使用平台默认用户目录。
