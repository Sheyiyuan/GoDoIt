# AGENTS.md — GoDoIt（够独特）项目约定

修改 GoDoIt 仓库前先读本文件。需求见 `docs/requirements.md`，架构以
`docs/architecture/README.md` 为唯一真理源。

## 项目定位

**GoDoIt（中文名：够独特，CLI/包名：gdit）** 是面向 Linux（主）和 macOS（验证）的
**Godot 引擎启动器与版本管理器**：底层是包管理器（引擎/.NET SDK/导出模板资产），上层是启动器
条目（instances，引用资产 + SDK 策略 + 环境配置）。

口号：**Go! Do It! 不等戈多，自己动手。**

- 仓库/App 名：`GoDoIt`
- CLI/包名：`gdit`
- 中文名：`够独特`
- Linux 为主平台，macOS Apple Silicon 与 Windows x64 为验证平台

GoDoIt 管理引擎，不管理项目。项目文件只供 `gdit suggest` 显式只读分析：

- 不在项目目录创建 `.gdit`、TOML 或 lock。
- 不保存项目路径与版本的关联。
- 不根据当前目录自动切换版本。
- 普通 `godot` 始终启动当前全局版本。

## 技术栈

| 层 | 选型 |
|---|---|
| core 与 CLI | Go |
| GUI | Wails v2 + React |
| 配置 | TOML，BurntSushi/toml |
| 工作区 | go work，core/CLI/GUI 分 module |
| 平台 | Linux 主，macOS Apple Silicon 与 Windows x64 验证 |

手写配置不用 YAML。JSON 只用于 Wails 或 CLI 机器输出等接口场景。

## 核心约束

1. 所有用户配置和管理数据统一放在 `~/.gdit/`。
2. `config.toml` 是唯一用户配置；`state.toml` 由 gdit 维护，已安装资产清单不一致时按 `engines/`、`sdks/` 重建。
3. 已安装引擎资产位于 `~/.gdit/engines/`（第二阶段的 `versions/` 语义）；`~/.gdit/current`
   symlink 第三阶段起指向 `instances/<uuid>.toml` 条目文件（条目显示名存放在文件内，
   CLI 按显示名寻址，存储标识为 UUID v4，二者分离）。
4. `~/.gdit/bin/godot` shim 指向 `gdit`；shim 只读取 current 条目、解析引擎与 SDK、注入
   环境并启动引擎，不读取项目、不访问网络。
5. 下载先进入 `~/.gdit/tmp/`，按来源声明的摘要（sha256 或 sha512）校验成功后才原子发布
   到 engines（SDK 同规则发布到 sdks）。
6. 下载源按 GodotHub → GitHub fallback；AtomGit 独立来源规则确认前不内置（AtomGit 只是
   GodotHub 资产的下载域名）；摘要失败不能继续安装。
7. CLI 和 GUI 共享 core，不复制安装、切换、环境、SDK 或来源逻辑。
8. 环境变量只注入目标子进程，不修改 shell、系统 PATH 或系统 dotnet。
9. Linux 显示驱动默认自动检测，不统一强制 x11；macOS 不注入 Linux 专用变量。
10. 平台判断只出现在 platform 适配层，业务代码不散落 `runtime.GOOS`。

## 仓库布局

```text
GoDoIt/
├── go.work
├── core/
│   ├── go.mod
│   ├── gdit.go
│   └── internal/
│       ├── archive/             # zip/tar.gz 安全解压
│       ├── config/
│       ├── dotnet/
│       ├── env/
│       ├── instance/             # 条目层：实例读写（UUID 标识 + 显示名）、引用扫描、GC 计算
│       ├── lock/                # 全局修改锁（flock）
│       ├── platform/
│       ├── project/             # 仅 suggest 只读分析（第五阶段创建）
│       ├── source/
│       └── store/
├── cli/
│   ├── go.mod
│   └── cmd/gdit/
├── gui/                         # 第六阶段创建（Wails v2 + React）
│   ├── go.mod
│   └── frontend/
├── docs/
│   ├── requirements.md
│   └── architecture/README.md
└── AGENTS.md
```

## 代码规范

- Go 注释一律中文；包级注释说明职责，导出函数注释说明行为与返回契约。
- 使用具名常量代替魔法数字，每个导出常量带 doc 注释。
- CLI main 和 Wails bridge 保持薄，只做参数/数据转换和 core 调用。
- 配置 struct 使用 `toml:"field_name"`，接口输出使用 `json:"field_name"`。
- 用户结果走 stdout，调试、进度和重试信息走 stderr。
- 耗时 I/O 接收 `context.Context` 并支持取消。
- 不记录 token、完整认证 URL 或敏感环境变量值。
- 不写空实现、TODO 测试骨架或只为通过编译的占位文件。

## 测试要求

- 使用临时的 gdit 根目录，不能读写开发者真实 `~/.gdit/`。
- 覆盖安装中断、sha256 失败、fallback、状态清单重建、current 原子切换和 shim 环境注入。
- 覆盖 `setup` 不修改 shell 配置或系统 PATH，以及 shim 缺少兼容 SDK 时不访问网络。
- Source 使用固定 fixture 测试，live 网络测试不能替代固定测试。
- `suggest` 测试必须确认项目目录内容没有变化（第五阶段实现 suggest 时生效）。
- macOS 平台行为需要 Apple Silicon 实机验证，交叉编译不算完成。

## 工作流

- 新功能先更新 `docs/architecture/README.md`，用户 review 后再写代码。
- 写完先展示，用户 review 后才 commit。
- `git add -A` 可以；`git commit` 必须等用户明确说“可以”。
- 使用 git flow 风格分支和约定式提交。
- 不改动与当前任务无关的文件。

当前实现状态为 `v0.2 第六阶段 Linux 实现完成`（Wails v2 + React GUI）。
前五阶段已交付：资产来源与安装、启动与默认版本、instances/SDK/环境/GC、doctor 与
Linux/macOS/Windows 平台适配层，以及 suggest + 导出模板。macOS Apple Silicon 与
Windows x86_64 的 GUI 实机验证仍待完成。
