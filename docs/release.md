# GoDoIt 发布维护手册

本文说明如何从仓库当前提交生成、验证并发布 GoDoIt。发布身份与安全边界以
[`architecture/README.md`](architecture/README.md) 的阶段 C 为准。

## 发布通道

| 通道 | 触发方式 | Release tag | 版本格式 | 可变性 |
|---|---|---|---|---|
| 开发预发布 | `main` push 或手动运行 `Build and Release` | `dev-latest` | `<VERSION>-dev.<UTC 日期>.<短 commit>` | 全部门禁通过后替换 |
| 稳定版 | 推送与根级 `VERSION` 完全一致的 `v<VERSION>` tag | 原 tag | `<VERSION>` | 不覆盖、不移动、不删除 |

手动运行即使选择 tag ref，也按开发预发布处理。稳定版只能由 tag push 触发。

## 发布资产

最终 GitHub Release 包含三个平台归档、四个安装包和校验清单：

```text
GoDoIt_<version>_linux_amd64.tar.gz
GoDoIt_<version>_darwin_arm64.zip
GoDoIt_<version>_windows_amd64.zip
GoDoIt_<version>_windows_amd64_setup.exe
GoDoIt_<version>_linux_amd64.deb
GoDoIt_<version>_linux_amd64.rpm
GoDoIt_<version>_darwin_arm64.dmg
SHA256SUMS
```

每个平台归档都包含 CLI、GUI、`LICENSE` 和 `THIRD_PARTY_NOTICES.txt`。macOS 归档保留完整
`GoDoIt.app`，Windows 可执行文件保留 `.exe`。发布工具会拒绝多余文件、缺失文件、危险归档路径、
符号链接、测试 fixture、签名材料和包含绝对工作区路径的输入。

## 推送前检查

1. 确认根级 `VERSION` 是不带 `v` 的三段稳定版本，例如 `0.2.0`。
2. 重新生成并检查离线法律文本和图标。
3. 运行质量门禁与当前平台原生打包。
4. 确认工作树只包含本次发布内容。

Linux 开发机使用：

```bash
make legal
make check
make package-linux
git diff --check
git status --short
```

`make package-linux` 会在 `build/release/linux_amd64/` 创建隔离的 Wails 暂存工程，并把归档写入
`dist/`。它不会改写已提交的 `gui/wails.json`，也不会读取或修改用户的 gdit 根目录。

macOS 与 Windows 必须分别在原生 arm64、amd64 环境运行 `make package-macos` 和
`make package-windows`。交叉编译不算 GUI 发布验收。

## 开发预发布

合并到 `main` 后，Release workflow 会依次执行：

1. 使用 Go 1.25.13 运行质量和发布身份门禁。
2. 在 Linux amd64、macOS arm64、Windows amd64 原生 runner 并行测试和打包。
3. 上传仅供本次 workflow 使用的不可修改中间 artifacts。
4. 汇总三个归档和四个安装包，生成并复验 `SHA256SUMS` 和最终文件白名单。
5. 所有步骤成功后，删除旧 `dev-latest` Release/tag 并创建新的 prerelease。

任一前置 job 失败时，已有 `dev-latest` 保持不变。开发版说明会记录完整来源 commit，并明确标注
macOS 未公证。

## 稳定版

稳定版发布前，先确认目标提交的 `main` CI 和开发预发布均已通过，再创建 tag：

```bash
version=$(tr -d '[:space:]' < VERSION)
git tag -a "v${version}" -m "GoDoIt ${version}"
git push origin "v${version}"
```

workflow 会验证 tag 名、`VERSION` 和当前检出 commit。若同名 GitHub Release 已存在，发布立即失败；
流程不会删除或覆盖稳定 Release。稳定版本需要修复时，更新 `VERSION` 并发布新 tag，不能复用旧 tag。

## 发布后核验

从 GitHub Release 下载全部八个文件，在 Linux 上复算摘要：

```bash
sha256sum -c SHA256SUMS
```

然后至少核对：

- 三个平台归档名使用同一版本。
- `gdit version` 与 GUI `--build-info` 的 version、commit、build date 一致。
- Linux/Windows 归档同级包含两份法律文本；macOS bundle 的 `Contents/Resources/legal/` 也包含副本。
- macOS `codesign --verify --strict`、架构和 plist 版本检查已在原生 job 通过。
- Windows ProductVersion 与发布版本一致。

## 签名边界

当前 macOS CI 只做 ad-hoc 签名，不包含 Developer ID、hardened runtime、公证或 stapling；Windows
也不做 Authenticode 签名。不得把这些归档描述为已公证或已由可信发布者签名。接入正式签名需要
单独设计凭据权限、密钥轮换、日志脱敏和失败恢复流程。
