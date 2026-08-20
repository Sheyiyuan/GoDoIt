---
title: GoDoIt 怎样启动一个条目
description: 条目如何把引擎、SDK 策略和环境配置放在一起。
section: explanation
order: 1
updated: 2026-08-19
---

## 一次启动需要哪些信息

Godot 的一次启动可能需要确定两组信息。

- 引擎版本与 edition 的组合
- dotnet 版需要的 SDK 策略，以及这次进程使用的环境

只记录当前版本，无法完整表达这次启动需要的组合。GoDoIt 因此把启动信息放进一个具名条目。

## 启动过程

条目是一个 TOML 描述文件。它引用引擎资产，并保存 SDK 策略和条目自己的环境变量。

```toml
[engine]
version = "4.5.2"
edition = "dotnet"

[dotnet]
strategy = "managed"
version = "8.0.410"

[env]
GAME_MODE = "debug"
```

运行 `gdit run` 时，GoDoIt 读取当前条目，解析引擎路径和 SDK，再合并环境并启动目标进程。整个过程只使用已经安装完整的资产。

## 换版本 = 换条目

需要使用另一个版本时，创建新条目并切换过去。

```bash
gdit install game-441 --version 4.4.1
gdit default game-441
gdit run
```

旧条目会继续保留。`gdit list` 可以列出所有条目，并标出 current。切换条目不会修改项目目录，也不会删除原有资产。

## 刻意不做的事

- 不管理项目。GoDoIt 不在项目目录写文件，也不记录项目路径与版本的关系。
- 不在启动和切换时下载或删除。`run` 与 `default` 只使用已经完整安装的资产。
- 不修改系统环境。GoDoIt 不改 shell 配置，不安装系统级 dotnet，也不改系统 PATH。

这些边界让普通的 `godot` 命令保持可预测。它始终启动同一个 current 条目，直到用户明确切换。
