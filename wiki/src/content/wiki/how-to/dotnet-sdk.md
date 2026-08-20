---
title: 管理 .NET SDK
description: 选择托管或系统 SDK，并查看、安装和删除托管版本。
section: how-to
order: 4
updated: 2026-08-19
---

dotnet 版引擎需要 .NET SDK。条目用 `managed` 或 `system` 声明 SDK 从哪里来，默认选择 `managed`。

## 两种策略的区别

| 策略 | SDK 从哪来 | 启动时 |
|---|---|---|
| `managed` | `~/.gdit/sdks/` 下 GoDoIt 安装的 SDK | 注入 `DOTNET_ROOT` 和 PATH 前缀，优先使用指定版本 |
| `system` | 系统已装的 dotnet（`dotnet --list-sdks` 探测） | 选系统里最新的，只加警告不拦截 |

`managed` 不修改系统 dotnet。`system` 不下载 SDK。托管 SDK 被条目引用时不能删除。

## 建条目时的默认行为

创建 dotnet 版条目时，如果没有给出 `--sdk-version`，GoDoIt 会按推荐映射补全版本。

```bash
gdit install work-csharp --version 4.5.2 --edition dotnet
```

推荐映射表由 core 静态维护。

| Godot 版本 | 推荐 .NET |
|---|---|
| 4.0 / 4.1 | 6.0 系列 |
| 4.2 及以上 | 8.0 系列 |

GoDoIt 会选择该系列最新可用的稳定 patch，并在这次安装中一并处理。表外版本和其他版本需要显式指定。

```bash
gdit install work-csharp --version 4.5.2 --edition dotnet --sdk-version 8.0.410
```

声明版本低于推荐 major 时，GoDoIt 给出警告，但仍允许继续。

## 查看与安装

```bash
gdit sdk list        # 托管和系统 SDK
gdit sdk available   # 官方元数据里的可安装稳定版本
gdit sdk install 8.0.410
```

安装过程从 .NET 官方元数据定位资产，SHA-512 校验通过以后才会原子发布。删除命令如下。

```bash
gdit sdk remove -y 8.0.410
```

某个版本被 managed 条目引用时，删除会失败。先删除引用它的条目，或重建一个使用其他 SDK 策略的条目。

## 用户自己接管

全局配置或条目显式设置 `DOTNET_ROOT` 时，GoDoIt 会跳过 SDK 策略选择，直接使用你给出的目录。

```bash
gdit env set DOTNET_ROOT=/opt/dotnet
```
