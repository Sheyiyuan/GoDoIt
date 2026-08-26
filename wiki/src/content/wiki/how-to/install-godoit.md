---
title: 安装 GoDoIt 发行包
description: 下载当前平台归档，校验 SHA-256，并启动 CLI 或桌面 GUI。
section: how-to
order: 0
updated: 2026-08-26
---

GoDoIt 的 GitHub Release 同时提供 CLI 和桌面 GUI。稳定版使用 `v<VERSION>` tag；
[`dev-latest`](https://github.com/Sheyiyuan/GoDoIt/releases/tag/dev-latest) 是随 `main` 更新的开发预发布。

## 选择文件

打开 [GoDoIt Releases](https://github.com/Sheyiyuan/GoDoIt/releases)，下载 `SHA256SUMS` 和当前平台
对应的归档：

| 平台 | 文件 |
|---|---|
| Linux x86_64 | `GoDoIt_<version>_linux_amd64.tar.gz` |
| macOS Apple Silicon | `GoDoIt_<version>_darwin_arm64.zip` |
| Windows x86_64 | `GoDoIt_<version>_windows_amd64.zip` |

`<version>` 是稳定版本，例如 `0.2.0`，也可能是带日期和 commit 的开发版本。归档和
`SHA256SUMS` 必须来自同一个 Release。

## Linux

下面以稳定版 `0.2.0` 为例。版本不同只需修改 `archive`。

```bash
archive=GoDoIt_0.2.0_linux_amd64.tar.gz
expected=$(awk -v name="$archive" '$2 == name { print $1 }' SHA256SUMS)
actual=$(sha256sum "$archive" | awk '{ print $1 }')
test -n "$expected" && test "$actual" = "$expected"

tar -xzf "$archive"
cd "${archive%.tar.gz}"
./gdit version
./gdit-gui
```

CLI 和 GUI 应报告同一个版本。也可以运行 `./gdit gui` 启动同目录的 GUI。需要长期从终端调用时，
把这个目录加入自己的 PATH；GoDoIt 不会自动修改 shell 配置。

## macOS Apple Silicon

```bash
archive=GoDoIt_0.2.0_darwin_arm64.zip
expected=$(awk -v name="$archive" '$2 == name { print $1 }' SHA256SUMS)
actual=$(shasum -a 256 "$archive" | awk '{ print $1 }')
test -n "$expected" && test "$actual" = "$expected"

unzip "$archive"
directory=${archive%.zip}
"$directory/gdit" version
open "$directory/GoDoIt.app"
```

当前 macOS 应用只做 ad-hoc 签名，尚未使用 Developer ID 公证。若系统安全策略拒绝运行，不要关闭
系统安全保护；可以改用源码构建，或等待后续已公证发行流程。

## Windows x86_64

在 PowerShell 中执行：

```powershell
$Archive = 'GoDoIt_0.2.0_windows_amd64.zip'
$Match = Select-String -Path SHA256SUMS -Pattern ([regex]::Escape($Archive))
if (-not $Match) { throw 'SHA256SUMS 中找不到当前归档' }
$Expected = $Match.Line.Split()[0].ToLowerInvariant()
$Actual = (Get-FileHash -Algorithm SHA256 $Archive).Hash.ToLowerInvariant()
if ($Actual -ne $Expected) { throw 'SHA-256 校验失败' }

Expand-Archive -Path $Archive -DestinationPath .
$Directory = [IO.Path]::GetFileNameWithoutExtension($Archive)
& ".\$Directory\gdit.exe" version
Start-Process ".\$Directory\gdit-gui.exe"
```

Windows 归档目前没有 Authenticode 签名，SmartScreen 可能显示未知发布者。不要绕过组织或设备的
安全策略；受管设备应等待管理员批准或使用组织认可的源码构建。

## 接着做什么

确认 `gdit version` 正常后，继续 [第一次安装并启动 Godot](/wiki/tutorials/hello-godot/)。
`gdit setup` 只会在 GoDoIt 用户目录创建 `godot` shim，不会修改 PATH；是否把 `~/.gdit/bin`
加入 PATH 仍由你决定。
