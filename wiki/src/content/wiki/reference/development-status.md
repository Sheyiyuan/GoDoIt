---
title: 开发状态与阶段范围
description: 当前已经实现的能力，以及后续阶段边界。
section: reference
order: 0
updated: 2026-08-25
---

GoDoIt 当前处于 `v0.2`。第五阶段代码已经完成，正在进行发布候选验证。

## 当前已经实现

- 引擎来源枚举、fallback、摘要校验和原子安装
- instances 条目、全局 current、`godot` shim 与 `gdit run`
- managed/system SDK 策略、启动环境合并和资产 GC
- `gdit doctor` 本地诊断与显式网络探测
- `gdit suggest` 项目只读分析与明确授权后的建议安装
- 导出模板安装、绑定、引用保护和孤儿清理
- Linux amd64 主支持，macOS arm64 与 Windows x86_64 验证级支持

完整命令见 [命令参考](/wiki/reference/commands/)。

## 第五阶段：已实现

### 项目建议

`gdit suggest [项目目录]` 只读分析用户明确指定的目录：

- 读取 `project.godot` 的 `config/features`
- 可读取同目录 `global.json` 和 `.csproj` 辅助判断 SDK
- 默认只输出建议，不联网、不获取修改锁、不写入项目或 gdit 根目录
- 只有用户确认或显式传入 `--install` 后才进入正常安装流程

GoDoIt 不会保存项目路径、建立项目与条目的持久关联，也不会根据当前目录自动切换版本。

### 导出模板

`gdit template` 管理与精确 Godot 版本和 edition 匹配的导出模板，并把资产放在`~/.gdit/templates/`。模板会复用现有来源、摘要校验、原子发布、引用保护和 `autoremove` 规则。

当前设计只管理并展示经过验证的模板资产路径，不修改 Godot 自身的用户目录，也不通过
`XDG_DATA_HOME` 接管其他 Godot 数据。模板如何无侵入接入 Godot 的自动发现流程仍需上游事实核验。

## 后续阶段

第六阶段提供 Wails GUI。GUI 将直接调用同一个 core，不复制安装、环境、SDK、来源或项目分析规则。

最终需求以仓库的 `docs/requirements.md` 为准，技术设计以 `docs/architecture/README.md` 为唯一真理源。
