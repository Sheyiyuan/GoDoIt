---
title: 目录布局与配置
description: ~/.gdit 下的文件、全局配置和实例条目格式。
section: reference
order: 2
updated: 2026-08-26
---

## 目录布局

GoDoIt 的所有状态默认在用户级目录 `~/.gdit/` 中。Windows 默认目录为
`%USERPROFILE%\.gdit`；`GDIT_ROOT` 可覆盖为任意绝对路径。

```text
~/.gdit/
├── config.toml    # 唯一用户配置文件（来源、全局环境）
├── state.toml     # 已安装资产索引（可自动重建）
├── current        # Unix 相对软链接；Windows 为规范相对路径重定向文件
├── instances/     # 启动器条目：<uuid>.toml，显示名在文件内
├── icons/         # GUI 导入的条目自定义图标：<uuid>.png
├── engines/       # 已安装引擎资产，每个资产一个目录
├── sdks/          # 托管 .NET SDK 资产
├── templates/     # 已验证的导出模板资产
├── runtime/
│   ├── .lock      # 运行会话跨进程锁
│   └── sessions/  # GUI 启动的 Godot 会话登记
└── tmp/           # 下载/解压临时目录（中断残留自动清理）
```

GoDoIt 不把配置写入项目目录或系统目录，也不修改 shell 配置、系统 PATH 和系统 dotnet。
`runtime/sessions/` 只保存会话 ID、条目/引擎 ID、PID、进程身份和启动时间，不保存项目路径、
完整命令行、环境变量或可执行文件路径。关闭 GUI 不会关闭仍在运行的 Godot。

## config.toml

```toml
schema_version = 1
source_order = ["godothub", "github"]   # 默认；可改顺序或加自定义源
disabled_sources = []                   # source ban 写入的禁用名单

[environment]
display_driver = "auto"    # auto / x11 / wayland
input_method = "auto"      # auto / fcitx / off
COMMON_VALUE = "global"

[environment.linux]
LINUX_ONLY = "value"

[environment.darwin]
MACOS_ONLY = "value"

[environment.windows]
WINDOWS_ONLY = "value"

[[custom_sources]]
name = "company-mirror"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "GDIT_COMPANY_MIRROR_TOKEN"
```

自定义源占位符只允许 `{version}`、`{tag}`、`{asset}`。URL 必须使用 HTTPS，localhost fixture 测试除外。`gdit source use`、`ban` 和 `unban` 写回时会保留其他已知字段，但不会保留注释。

`[environment]` 是三平台通用变量；平台小节仅在对应平台生效，并覆盖全局同名键。
`display_driver` 与 `input_method` 只在 Linux 支持非 `auto` 值。

## instances/<uuid>.toml

条目引用资产并保存配置，它不复制二进制内容。文件名是 UUID v4 存储标识，显示名存在文件内。
下面是一份 dotnet 条目示例。

```toml
schema_version = 2
id = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"   # 存储标识，与文件名一致（gdit 自动生成）
name = "工作-csharp"                            # 显示名：CLI 寻址用，可中文，全仓库唯一

[engine]
version = "4.5.2"
edition = "dotnet"        # standard / dotnet

[dotnet]                  # 仅 dotnet 版
strategy = "managed"      # managed / system
version = "8.0.410"       # managed 必填，精确三段

[env]                     # 条目级环境变量（可选）
GAME_MODE = "debug"
```

条目文件遵循下面几条规则。

- 显示名与存储标识分离：文件名/current/内部引用一律使用 UUID；显示名只承担 CLI 寻址与展示
  （`gdit run 工作-csharp`），字符集为 URL 安全字符——ASCII 只允许 `[A-Za-z0-9._~-]`，
  非 ASCII 文字（中文等）允许，空格、标点、符号、控制字符一律禁止；显示名全仓库唯一。
- `[engine]` 引用创建后不可变。`[env]` 用 `gdit env` 修改，`[dotnet]` 策略需要手写或重建条目。
- `standard` 条目不能带 `[dotnet]`。`managed` 条目必须写精确 `version`。

## 条目扩展

第五、六阶段在现有 schema 2 中加入可选模板引用和外观配置，不改变既有条目的读取方式：

```toml
[template]
id = "4.5.2-dotnet"

[appearance]
icon = "custom"                                      # default / godot / csharp / mascot / custom
custom_icon = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8.png"
background = "#A179DC"                               # 可选；缺失时为透明
```

模板 ID 必须等于条目引擎的 `<version>-<edition>`。省略该 table 表示条目不依赖导出模板；模板
资产缺失只影响导出能力，不阻断 `default` 或 `run`。`appearance` 省略时按 edition 选择
Godot 或 C# 默认图标；自定义图标文件名必须与条目 UUID 一致，由 core 规范化为 256x256 PNG。
图标按圆形显示但不绘制边框，`background` 只接受 `#RRGGBB` 或 `#RRGGBBAA`，缺失或空值表示透明。
