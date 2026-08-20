---
title: 清理孤儿资产
description: 删除条目以后，检查并清理没有引用的引擎和 SDK。
section: how-to
order: 5
updated: 2026-08-19
---

## 什么是孤儿资产

条目会引用引擎和 SDK。删除条目以后，不再被任何条目引用的资产会成为孤儿。GoDoIt 保留这些资产，直到你明确运行清理命令。

## 删条目时的提示

```bash
gdit remove -y work
```

删除完成以后，输出会列出刚刚变成孤儿的资产及其占用空间。这个列表只作提示，此时还没有删除资产。下面是输出结构示例。

```text
removed instance work
以下资产已无引用，可用 gdit autoremove 清理
engine	4.5.2-standard	<占用空间>
```

## 正式清理

```bash
gdit autoremove        # 列出孤儿并要求确认
gdit autoremove -y     # 用于脚本或非终端环境
```

确认以后，`autoremove` 会在全局修改锁内重新扫描引用。只有复查时仍然没有引用的资产才会被删除。

## 引用保护

- 被条目引用的引擎和 SDK 不会进入孤儿列表。`engine remove` 和 `sdk remove` 也会拒绝删除它们。
- `instances/` 中有无法读取或解析的条目时，所有资产删除命令都会停止。先修复条目，再重新运行清理。

## 其他删除入口

```bash
gdit engine remove -y 4.4.1      # 删除引擎资产
gdit sdk remove -y 8.0.410       # 删除托管 SDK
```

TTY 下删除会要求确认，默认选择为否。非 TTY 环境必须显式传入 `-y` 或 `--yes`。
