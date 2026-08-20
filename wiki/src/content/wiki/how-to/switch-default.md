---
title: 切换当前条目与 godot 命令入口
description: 查看和切换当前条目，并创建 godot 命令入口。
section: how-to
order: 2
updated: 2026-08-19
---

## 查看当前条目

```bash
gdit default
```

输出包含显示名、引擎、edition 和 SDK 策略。

## 切换当前条目

```bash
gdit default work-csharp
```

GoDoIt 会原子更新 `~/.gdit/current` 软链接。更新失败时保留原链接。这个选择对所有目录一致，GoDoIt 不会根据项目目录自动切换。

## 创建 godot 命令入口

`gdit setup` 会在 `~/.gdit/bin/` 创建或修复 `godot` shim。

```bash
gdit setup
```

然后由你把 `~/.gdit/bin` 加进 PATH。下面是一种只对当前终端生效的写法。

```bash
export PATH="$HOME/.gdit/bin:$PATH"
```

> `gdit setup` 不修改 shell 配置或系统 PATH。需要长期生效时，请把同一条配置写入自己使用的 shell 配置文件。

## 用 run 启动

不调整 PATH 时，可以直接使用 `gdit run`。

```bash
gdit run            # 启动当前条目，-d 同义
gdit run work       # 启动指定条目，不改变 current
gdit run -- -e      # -- 之后原样透传给引擎
```

需要传给 Godot 的参数放在 `--` 后面，避免 gdit 把它们当成自己的参数。引擎输出和退出码会继续传给调用方。
