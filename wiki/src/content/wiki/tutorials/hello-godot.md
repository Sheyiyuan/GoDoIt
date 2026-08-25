---
title: 第一次安装并启动 Godot
description: 查看可用版本，创建条目，然后启动编辑器。
section: tutorials
order: 1
updated: 2026-08-25
---

这篇教程带你跑通最短的一条路径。你会查看可用版本，创建一个名为 `work` 的条目，然后启动它引用的 Godot 编辑器。条目和资产的关系留到后面的解释文章再讲。

## 前提

- Linux amd64、macOS arm64 或 Windows x86_64 的受支持环境
- 已经把当前开发版 `gdit` 放进 PATH

GoDoIt 当前为 v0.2 第四阶段发布候选，仓库暂未提供正式的二进制安装流程。本页从 `gdit` 命令
可用以后开始；第五阶段的 `suggest` 和导出模板命令仍在设计中，不影响本教程。

## 1. 看看能装什么

先查看当前平台可以安装的稳定版本。

```bash
gdit available
```

输出会列出版本号、edition 和来源。下面用 `4.5.2` 标准版演示。

## 2. 建条目并安装引擎

下面的命令创建条目 `work`，安装它引用的 4.5.2 标准版引擎，并把这个条目设为当前条目。

```bash
gdit install work --version 4.5.2 --current
```

GoDoIt 默认先尝试 GodotHub，失败后再尝试 GitHub。下载完成并通过摘要校验以后，引擎目录才会发布到 `~/.gdit/engines/`。

## 3. 启动编辑器

```bash
gdit run
```

GoDoIt 会读取当前条目，找到对应的引擎，合并这次启动所需的环境，然后启动编辑器。退出编辑器以后，父进程环境没有变化。

## 4. 验证安装

```bash
gdit list
```

结果中应该有下面这一行。

```text
work	4.5.2-standard	standard	current
```

## 接着做什么

现在可以继续处理下面两个常见任务。

- 安装 dotnet 版，并选择托管或系统 SDK。参见 [.NET SDK 管理](/wiki/how-to/dotnet-sdk/)。
- 创建 `godot` 命令入口，或在多个条目之间切换。参见 [切换当前条目](/wiki/how-to/switch-default/)。

需要重来时，先运行 `gdit default <另一个条目>` 离开当前条目，再用 `gdit remove -y work` 删除条目。最后运行 `gdit autoremove -y` 清理没有引用的资产。
