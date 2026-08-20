---
title: 版本映射与下载源
description: Godot 与 .NET 的推荐版本对应，以及当前可用的下载来源。
section: reference
order: 3
updated: 2026-08-19
---

## Godot → .NET 推荐映射

dotnet 版条目不写 `--sdk-version` 时，按此表补推荐版本。具体 patch 会选择该系列最新可用的稳定版。

| Godot 版本 | 推荐 .NET |
|---|---|
| 4.0 / 4.1 | 6.0 系列 |
| 4.2 及以上 | 8.0 系列 |
| 表外版本 | 需要显式 `--sdk-version` |

声明版本低于推荐 major 时警告，不拦截。映射表由 core 静态维护，随 GoDoIt 版本演进。

## 下载来源

默认顺序是 `godothub` → `github`。前一个来源不可用时，安装流程会继续尝试下一个来源。

| 来源 | 类型 | 说明 |
|---|---|---|
| godothub | builtin | 使用 GodotHub 元数据和其提供的镜像地址 |
| github | builtin | GitHub 官方发布资产 |
| 自定义源 | custom | `config.toml` 的 `[[custom_sources]]`，URL 模板型 |

GodotHub 来源会根据已确认的元数据规则定位资产。多来源回退减少单个来源不可用的影响。显式使用 `--source` 时，失败不会继续尝试其他来源。AtomGit 作为独立来源的规则尚未确认，不能在配置中按内置来源直接使用。

## 下载完整性

- 下载完成后按来源声明的摘要校验，校验失败不会安装。
- 安装先进入临时目录，通过校验以后再原子发布，失败或中断不会留下完整资产。
- 中断留下的操作目录会在下一次持锁操作时清理。

## 其他版本事实

- 版本输入只接受精确三段稳定版本（`4.5.2`），预览版不在首版范围
- `m<版本>` 表示 dotnet 版引擎资产，仅 `engine install/remove` 接受
- 引擎 zip 资产名用下划线（`Godot_v4.5.2-stable_mono_linux_x86_64.zip`），解压后目录内可执行文件是点号（`Godot_v4.5.2-stable_mono_linux.x86_64`）
