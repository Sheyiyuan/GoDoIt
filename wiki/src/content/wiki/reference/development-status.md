---
title: 开发状态与阶段范围
description: 当前已经实现的能力，以及后续阶段边界。
section: reference
order: 0
updated: 2026-08-26
---

GoDoIt 当前处于 `v0.2`。第六阶段 GUI 已完成 Linux 基础实现，阶段 A 可用性实现与自动测试
已经闭环；Linux 视觉验收以及 macOS Apple Silicon 与 Windows x86_64 的 GUI 实机验证仍待完成。

## 当前已经实现

- 引擎来源枚举、fallback、摘要校验和原子安装
- instances 条目、全局 current、`godot` shim 与 `gdit run`
- managed/system SDK 策略、启动环境合并和资产 GC
- `gdit doctor` 本地诊断与显式网络探测
- `gdit suggest` 项目只读分析与明确授权后的建议安装
- 导出模板安装、绑定、引用保护和孤儿清理
- Wails v2 + React 桌面工作台，与 CLI 共享同一个 core
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

## 第六阶段：Linux 已实现

Wails GUI 已提供条目浏览与安装、current/launch、模板、资源、Suggest、Doctor、来源、环境和
关于页面。GUI 直接调用同一个 core，不复制安装、环境、SDK、来源或项目分析规则。Linux
原生构建已通过；macOS Apple Silicon 与 Windows x86_64 仍需完成窗口、文件选择器、路径显示、
键盘导航与高 DPI 布局的实机验收。

### 阶段 A：GUI 可用性修补（实现与自动测试已完成）

已完成：

- GUI 首次启动通过 core 初始化标准 gdit 目录，不自动创建 shim 或 current
- Bootstrap 与 Doctor 问题按故障、需要注意、可选命令行集成分组，可查看详情并在本次会话关闭 warning
- operation 注册后立即产生 queued 事件，保证唯一终态并释放 waiter
- 顶栏提供任务入口和紧凑托盘；完整操作中心聚合多资产与 fallback 下载，展示已知或未知大小进度、
  安全结果摘要、完成时间和失败来源入口
- 顶栏设置保存成功后立即更新当前窗口，失败时保留旧状态
- 初始化失败、半成品、重试、窗口关闭等待/取消、多资产 fallback、未知大小和 waiter 释放已有自动测试

### 阶段 B：桌面工作流（部分已实现）

已完成候选后台预取与向导复用、全局和条目配置环境变量的查看/编辑/删除，以及
`runtime/sessions` 持久会话登记、GUI 重启恢复读取与 Linux 启动/关闭流程。

仍待完成：

- 顶栏“运行中 N”、跨条目全局会话面板和 `gdit:session` 实时事件
- 候选按 Godot channel 与 SDK major/minor 完整分类，以及空结果、部分来源失败和 fallback warning 独立状态
- Linux 最小窗口、高 DPI、Wayland/X11 与 macOS/Windows GUI 实机验收记录
- 跨平台 GUI CI、签名、公证、安装器和统一版本命名等发布门禁

最终需求以仓库的 `docs/requirements.md` 为准，技术设计以 `docs/architecture/README.md` 为唯一真理源。
