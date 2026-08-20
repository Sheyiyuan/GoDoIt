---
title: 安装指定版本与多版本共存
description: 安装指定版本、dotnet 版和独立引擎资产。
section: how-to
order: 1
updated: 2026-08-19
---

## 装一个指定版本

用 `gdit install` 创建条目并安装它引用的引擎。版本号需要精确到三段，目前只接受稳定版。

```bash
gdit install work --version 4.5.2 --current
```

`--current` 会把新条目设为当前条目。省略这个参数时，已有的 current 保持不变。系统尚无 current 时，新条目会自动成为 current。

## 装 C#（.NET）版

添加 `--edition dotnet`。

```bash
gdit install work-csharp --version 4.5.2 --edition dotnet
```

SDK 策略默认使用 `managed`。GoDoIt 会按照推荐映射选择一个精确的 SDK 版本，并在同一次显式安装中把它装好。

需要使用系统已有的 dotnet 时，显式选择 `system`。

```bash
gdit install work-csharp --version 4.5.2 --edition dotnet --sdk system
```

也可以指定托管 SDK 的精确版本。

```bash
gdit install work-csharp --version 4.5.2 --edition dotnet --sdk managed --sdk-version 8.0.410
```

## 只装引擎资产，不建条目

`gdit engine install` 只安装引擎资产。它不会创建条目，也不会安装 SDK。

```bash
gdit engine install 4.5.2 m4.6.2
```

`m` 前缀表示 dotnet 版资产。这个前缀只用于引擎资产命令。一次给出多个版本时，各项分别安装，其中一项失败不会取消其余项目。

需要固定下载来源时使用 `--source`。指定来源失败以后不会继续回退。

```bash
gdit engine install 4.5.2 --source github
```

## 多版本共存

多个引擎可以同时安装，每个条目引用一个确定的版本。

```bash
gdit install game-452 --version 4.5.2 --current
gdit install game-441 --version 4.4.1
gdit list
```

```text
game-452	4.5.2-standard	standard	current
game-441	4.4.1-standard	standard
```

切换版本时，切到另一个条目即可。具体命令见 [切换当前条目](/wiki/how-to/switch-default/)。运行 `gdit engine list` 可以单独查看已安装的引擎资产。
