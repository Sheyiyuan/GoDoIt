---
title: managed 与 system 两种 SDK 策略
description: 两种 SDK 来源的行为、代价和适用情况。
section: explanation
order: 3
updated: 2026-08-19
---

## 问题

dotnet 版 Godot 需要 .NET SDK。GoDoIt 允许条目使用自己管理的 SDK，也允许它使用 PATH 中已有的系统 SDK。

SDK 来源写在条目的 `[dotnet]` 配置中。策略一旦写进条目，每次启动都会按照同一规则解析。

## managed 使用托管 SDK

`managed` 是 dotnet 条目的默认策略。SDK 安装在 `~/.gdit/sdks/`，可以被多个条目共享。启动时，GoDoIt 通过 `DOTNET_ROOT` 和 PATH 前缀指定条目声明的版本。

```
DOTNET_ROOT=/home/you/.gdit/sdks/8.0.410
PATH=/home/you/.gdit/sdks/8.0.410:...
```

- 每个 SDK 版本使用独立目录，可以同时保留多个版本。
- 安装和删除只发生在 `~/.gdit/sdks/`，不会修改系统 dotnet。
- 每个版本会额外占用磁盘空间，并需要一次显式下载。

条目必须声明精确版本。没有传入 `--sdk-version` 时，创建流程会按照推荐映射选出一个版本，并把选择写入条目。

## system 使用系统 SDK

`system` 不下载 SDK。启动时，GoDoIt 通过 `dotnet --list-sdks` 查看系统已有版本，并使用其中最高的版本。版本低于推荐 major 时会显示警告，但不会阻止启动。

- GoDoIt 不额外占用 SDK 磁盘空间，也不会发起 SDK 下载。
- 实际使用的版本由系统环境决定，系统升级以后可能变化。

系统已经安装合适的 dotnet，并且你愿意让系统维护这个版本时，可以选择 `system`。

## 为什么默认 managed

默认值考虑了两件事。

1. **版本确定**。条目声明 `8.0.410` 后，每次启动都选择这一版本。
2. **用户目录内管理**。GoDoIt 只处理 `~/.gdit/` 中的托管 SDK，不需要修改系统安装。

## 什么时候接管

全局配置或条目 `[env]` 显式设置 `DOTNET_ROOT` 时，GoDoIt 跳过上述策略，直接使用用户提供的目录。这项配置适合已经由其他工具管理 SDK 的情况。
