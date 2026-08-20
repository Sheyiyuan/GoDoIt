---
title: 配置输入法与显示环境
description: 设置 fcitx、显示驱动和条目自己的环境变量。
section: how-to
order: 3
updated: 2026-08-19
---

启动引擎时，GoDoIt 依次合并父进程环境、全局配置、条目配置和派生变量。合并结果只传给这次启动的引擎子进程。

## 输入法（fcitx）

Linux 下默认使用自动检测。检测到 fcitx 正在运行时，GoDoIt 会补充下面几个变量。

```text
XMODIFIERS=@im=fcitx
GTK_IM_MODULE=fcitx
QT_IM_MODULE=fcitx
```

没有检测到 fcitx 时，不会补充变量。需要强制启用或关闭时，修改 `~/.gdit/config.toml`。

```toml
[environment]
input_method = "fcitx"   # 强制启用
```

关闭自动注入时，把值改成 `off`。条目也可以单独覆盖这个控制键。

```bash
gdit env set input_method=fcitx --instance work
```

## 显示驱动

Linux 下默认值为 `auto`。此时 GoDoIt 不添加显示驱动参数，让 Godot 使用自身的默认处理。需要明确指定时，修改全局配置。

```toml
environment = { display_driver = "x11" }
```

启动时，这项配置会转换为引擎参数 `--display-driver x11`。

## 其他环境变量

普通环境变量可以写进全局配置的 `[environment]`，也可以写进条目文件的 `[env]`。使用命令修改时，不需要直接编辑 TOML。

```bash
gdit env set FOO=bar --instance work   # 设置条目变量
gdit env set FOO=bar                   # 设置全局变量
gdit env unset FOO --instance work     # 删除条目变量
```

查看最终注入结果可以运行下面的命令。

```bash
gdit env --instance work
```

输出会列出每个变量的值及其来源，包括 `global`、`instance` 和 `derived`。

## macOS 注意

macOS 会跳过 fcitx 和显示驱动等 Linux 专用处理。macOS Apple Silicon 仍需实机完成行为验收。
