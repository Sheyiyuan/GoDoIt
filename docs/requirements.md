# GoDoIt 需求文档（PRD）

> GoDoIt ｜ CLI/包名：gdit  
> Go! Do It! 不等戈多，自己动手。  
> 状态：v0.2 第四阶段实现完成、发布候选验证中（doctor + Linux/macOS/Windows 平台适配层）
> 第四阶段验收与实现约束见 docs/architecture/README.md §9.6 与 §4.9。
> 平台扩展：Windows x86_64 为验证级支持；发布须通过 Windows 原生验收与 macOS Apple Silicon CI。

## 1. 项目定位

GoDoIt 是面向 Linux（主）、macOS Apple Silicon（验证）与 Windows x86_64（验证）的
**Godot 引擎启动器与版本管理器**。底层是包管理器：
统一安装、校验、卸载 Godot 引擎与 .NET SDK 资产（通过 `gdit engine` 命名空间访问）；上层是
启动器条目（instances）：引用资产并携带 SDK 策略与环境配置，`godot` shim 读当前条目解析
引擎与 SDK、注入环境后启动引擎。提供 CLI 与 Wails GUI 两种入口。

GoDoIt 管理的是引擎，不管理 Godot 项目：

- 不在项目目录创建 `.gdit`、配置文件或 lock。
- 不保存“项目路径 → 引擎版本”的关联。
- `project.godot`、`global.json` 和 `.csproj` 只在用户执行 `suggest` 时只读分析。
- 普通 `godot` 命令始终启动当前条目指向的引擎，不根据工作目录自动切换。

## 2. 用户目录

GoDoIt 的配置、状态、命令入口、引擎、SDK 和缓存统一位于用户级 gdit 根目录：默认
`~/.gdit/`（Windows 为 `%USERPROFILE%\.gdit`），可用环境变量 `GDIT_ROOT` 覆盖为任意
绝对路径（Windows 用户可将数据放在非系统盘，如 `D:\gdit`）。解析顺序：`GDIT_ROOT`
（非空即用，必须是绝对路径）→ 平台默认路径；解析只发生在 platform 适配层。下文的
`~/.gdit/` 指实际生效的根目录：

```text
~/.gdit/
├── config.toml
├── state.toml
├── bin/
│   ├── godot / godot.cmd          # Unix：symlink 指向 gdit；Windows：godot.cmd 包装（见 §2 平台形态）
│   └── gdit(.exe)
├── instances/
│   └── <uuid>.toml          # 条目文件；文件名是 UUID v4 存储标识，显示名存在文件内
├── current                   # 当前条目指针：Unix 为 symlink → instances/<uuid>.toml；
│                             #   Windows 为普通文本文件，内容为规范相对路径（避免 symlink 权限问题）
├── engines/                # 引擎资产（原 versions/）
├── sdks/                   # SDK 资产（原 dotnet/）
├── templates/
├── cache/
└── tmp/
```

除用户级命令入口外，不写入项目目录、系统目录或其他应用目录。
第三阶段是破坏性布局切换，只支持上述新布局；不读取、不迁移第二阶段的 `versions/`、`dotnet/`
和旧 current。开发与验收使用全新的 gdit 根目录，gdit 不自动删除旧数据。

## 3. 功能需求

### FR-01 引擎安装（P0 · 第一/三阶段）

日常入口是条目安装：`gdit install`（`gdit new` 为等价别名；交互式，确认条目名与各项配置）
或 `gdit install <name> --version <版本>`（非交互）创建条目，顺带安装引擎资产与 SDK 依赖。
资产层安装为 `gdit engine install`：只安装引擎资产，不创建条目、不装 SDK。

- 版本支持两/三段稳定版（`4.5.2`、`4.7`——官方 minor 首发不写 .0）与两/三段预发布
  （`4.8-dev3`、`4.7.2-rc1`、`4.7-rc3`、`4.7.1-beta2`）。枚举与交互选择按系列分组：
  稳定版按 major（`4.x`/`3.x`），预发布统一归入 `unstable` 组，先选系列再选具体版本；
  枚举期间 TTY 显示等待动画。
- Godot 3.x 的 dotnet（mono）版只做**下载与安装**：资产按官方 mono 命名收录，条目
  SDK 策略自动为 `mono`（不解析、不注入 .NET SDK）；运行时由用户自理（系统 Mono），
  传 SDK 选项报错。
- 资产层版本输入支持 `m` 前缀简写：`m4.5.2` 等价于 `--edition mono 4.5.2`（内部归一化为
  dotnet），仅 `gdit engine install/remove` 接受版本号输入。
- 支持一次传入多个版本串行安装（如 `gdit engine install 4.5.2 m4.6.2`）；`--edition`
  显式给出时统一应用于所有版本，任一失败不中断其余，最终按是否有失败汇总退出码。
- 内置 GodotHub 国内镜像和 GitHub 官方源；AtomGit 独立来源规则确认后加入内置。
- 支持用户在 `config.toml` 中添加自定义源；默认按 GodotHub → GitHub 自动 fallback。
- 下载完成后按来源声明的摘要（sha256 或 sha512）校验，校验失败不得安装。
- 安装失败或中断不能留下被识别为完整版本的半成品。
- 条目安装 dotnet 版时按条目 SDK 策略解析托管 SDK 并作为依赖一并安装（apt 语义：
  在显式安装动作内下载，不在启动时隐式下载）。
- 非交互安装支持互斥的 `--current`/`--no-current`；均未指定时，只有尚无 current 才把新条目
  设为当前。

### FR-02 默认条目与启动（P0 · 第二/三阶段）

`gdit default <name>` 将指定条目设为当前条目；`gdit default` 无参数显示当前条目。
只接受条目显示名。

- 通过原子更新 `~/.gdit/current` 完成设置，失败保留旧值；current 是全局单一条目指针，
  平台形态见 §2（Unix symlink / Windows 重定向文件，契约一致）；当前条目对所有目录
  一致，不做项目级自动切换。
- `gdit setup` 显式创建或修复 `~/.gdit/bin/godot` shim，但不修改 shell 配置或系统 PATH。
- PATH 中的 `godot` shim 指向 `gdit`，由 gdit 读取 `current` 条目、解析引擎与 SDK、
  注入环境并启动真实引擎，透传参数、输出和退出码。shim 不访问网络。
- `gdit run` 启动引擎：无参数启动当前条目（`-d` 为等价别名；无参数 + TTY 且多于一个
  条目时交互选择要启动的条目）；`gdit run <name>` 启动指定条目。`--` 之后参数原样透传。
- `default` 与 `run` 只接受条目引用完整安装的资产，不触发隐式下载。
- 命名条目第三阶段开放；换引擎版本 = 安装新条目，既有条目的 engine 引用不可变。

### FR-03 资产与条目查看、卸载（P0 · 第一/二/三阶段）

- `gdit list` 列出条目：名称、引擎、edition、SDK 策略与当前标记。
- `gdit remove <name>` 删除条目；删除不可逆，TTY 下确认后执行（默认否），非 TTY 必须
  显式 `-y`/`--yes` 跳过确认；当前条目拒绝删除，必须先 `default` 到其他条目。
- 删除条目后计算无条目引用的引擎/SDK 资产（孤儿），输出提示：`以下资产已无引用，可用
  gdit autoremove 清理`，附每个资产的占用空间。
- 资产层操作：`gdit engine list` 列出已安装资产（含引用状态）；`gdit engine remove <版本>`
  卸载指定资产，被任何条目引用的资产拒绝删除。
- `gdit autoremove [-y|--yes]` 列出孤儿并确认后删除（apt 语义）；被条目引用的资产永不
  自动删除。
- 引用扫描遇到任何不可读或非法条目时失败关闭：`engine remove`、`sdk remove` 和
  `autoremove` 都不得跳过坏条目继续删除。`autoremove` 在用户确认后、实际删除前必须在锁内
  重新扫描，只删除复查时仍为孤儿的资产。

### FR-04 启动环境变量（P0 · 第三阶段）

启动 Godot 时向子进程注入环境，不修改用户全局环境。

- 全局默认写在 `~/.gdit/config.toml` 的 `[environment]`，条目覆盖写在
  `instances/<uuid>.toml` 的 `[env]`；合并顺序：继承父环境 → 全局 → 平台小节 → 条目 →
  派生变量。
- 环境注入按平台配置（第四阶段）：`[environment]` 为三平台通用变量，
  `[environment.linux|darwin|windows]` 平台小节仅当前平台生效（覆盖全局同名键）；
  平台注入规则（已知键、fcitx、显示驱动、DOTNET_ROOT/PATH 前缀格式）按编译标签拆分实现。
- 支持 `DOTNET_ROOT`、PATH 前缀、显示驱动和 fcitx 相关变量。
- Linux 下显示驱动默认自动检测，不统一强制 x11。
- fcitx 只在 Linux 检测到或用户明确启用时注入。
- macOS 不注入 Linux 专用变量。
- 用户显式配置 `DOTNET_ROOT` 时视为接管 SDK 定位，跳过 SDK 选择。
- 注入只作用于目标子进程，不修改 shell、系统 PATH 或系统 dotnet。

### FR-05 .NET SDK 管理（P0 · 第三阶段）

为 Godot .NET 版按条目的 `[dotnet]` 策略提供 SDK，不修改系统 dotnet。

- 策略两档：`managed`（默认）使用 `~/.gdit/sdks/<version>/` 的托管 SDK，条目必须声明
  version；`system` 使用系统已安装 SDK（`dotnet --list-sdks` 检测），忽略 version 字段。
- 策略在条目创建时确定：dotnet edition 默认 `managed`，version 缺省为 core 写死的推荐
  映射表（Godot 版本系列 → .NET major.minor）对应 major 的最新可用 patch，并作为依赖
  一并安装。
- 推荐映射表由 core 静态维护（粒度 major.minor，具体 patch 由 `gdit sdk available`
  取最新可用）；表缺失的版本由用户显式填写 version。声明版本低于推荐 major 时警告，
  不拦截。
- `gdit sdk available` 与交互式 SDK 选择动态枚举官方通道（releases-index.json，跳过
  EOL，保留 preview，始终保留 6.0 供 Godot 4.0/4.1；索引不可用时降级内置保底列表并发
  警告）；交互选择为**两级菜单**（先大版本通道，再选具体 patch，preview 通道标注
  Preview），枚举失败降级为手动输入版本号。SDK 版本支持 .NET 预发布后缀
  （`11.0.100-preview.7.26381.103`、`8.0.100-rc.2.23502.12`）。
- 普通 `godot` 启动只选择已有 SDK；没有时报错并给出安装提示，不交互、不下载。
- 托管 SDK 资产下载内置镜像 fallback：华为云镜像优先、官方兜底（元数据始终官方，
  摘要校验与下载源无关）；镜像同步延迟导致 404 时自动降级官方。
- 多个托管 SDK 可以共存（`~/.gdit/sdks/`）；`gdit sdk install <版本>` 显式安装（只接受
  精确版本），`gdit sdk remove [-y|--yes] <版本>` 卸载。
- 选择托管 SDK 时通过目标子进程的 `DOTNET_ROOT` 和 PATH 前缀优先使用它。
- 不卸载、不禁用、不修改系统 dotnet；被条目引用的托管 SDK 拒绝删除。

### FR-06 项目分析建议（P1 · 第五阶段）

`gdit suggest [项目目录]` 只读分析项目需要的引擎和 .NET SDK。

- 读取 `project.godot` 的 `config/features` 判断版本系列与 C# 标志。
- 可读取 `global.json` 和 `.csproj` 辅助判断 SDK。
- 输出建议清单，不在项目内写文件，也不改变当前条目。
- 用户显式确认或传入 `--install` 后，可以安装建议的引擎、SDK 和模板（走条目安装流程）。

### FR-07 导出模板（P1 · 第五阶段）

安装、列出和卸载与指定 Godot 版本匹配的导出模板，统一存放在 `~/.gdit/templates/`。

### FR-08 环境诊断（P0 · 第四阶段）

`gdit doctor` 检查：

- `~/.gdit/` 目录与权限。
- `godot` shim 和 PATH。
- 当前条目及其引擎/SDK 引用是否完整。
- 引擎、模板和 SDK 状态。
- 最终会注入的环境变量。
- 下载源可用性和配置错误。

doctor 默认只报告和建议，不静默修改。

- 默认零网络、零落盘、不获取修改锁；`--network` 显式开启来源可达性探测（探测失败按
  警告处理，不视为配置错误）。
- 检查全部条目（不只当前条目）的引用完整性；任意坏条目按失败关闭哲学报错。
- 环境变量预览对敏感键名（token/secret/password/key）掩码值，`--verbose` 也不放开。
- 退出码：0 = 无错误；1 = 存在错误；警告不影响退出码。
- 本阶段不提供自动修复（`--fix`）；doctor 只报告，修复入口沿用现有命令。
- 检查项按平台差异化：shim 形态（Unix symlink / Windows `godot.cmd`）、PATH 分隔符、
  fcitx 仅 Linux、`display_driver` 非 Linux 平台仅 `auto`、根目录权限位检查仅 POSIX
  （Windows 降级为目录可访问检查）、引擎/SDK 启动文件按平台校验（.exe / dotnet.exe /
  app bundle）。

### FR-09 GUI（P1 · 第六阶段）

Wails GUI 提供条目与版本列表、条目安装/卸载、当前条目切换、项目分析、设置、doctor 和
关于页面。

- GUI 与 CLI 调用同一个 core。
- GUI 不维护独立业务规则。
- “项目分析”由用户显式选择目录触发，不维护项目列表或扫描主目录。

### FR-10 缓存管理（P2 · 后续阶段）

查看并清理 `~/.gdit/cache/` 中的下载缓存。

### FR-11 run 启动（P0 · 第三阶段，取代原 exec 设计）

`gdit run [<name>|-d] [-- <参数>]` 使用当前或指定条目运行 Godot，透传参数、输出和退出码。

- 无参数或 `-d` 使用当前条目；`gdit run <name>` 启动指定条目。
- `--` 之后所有参数原样透传给引擎，不经过 gdit 解析。
- 不改变当前条目，不触发隐式下载。

### FR-12 来源管理（P1 · 第一阶段扩展）

- `gdit source` 查看当前来源顺序、类型与禁用状态；`gdit source use <name>` 把指定来源设为默认
  首位并写回 `config.toml`，不丢失其他配置字段。
- `gdit source ban/unban <name>` 强禁用/启用来源：被禁用的来源不参与自动 fallback 与默认探测，
  显式指定或设为默认时同样报错。
- `gdit engine install --source <name>` 用指定来源下载资产，该来源失败时不自动降级到其他来源。
- `gdit available [--source <name>]` 探测默认或指定来源上、当前平台可安装的稳定版本与 edition；
  URL 模板型自定义源无法枚举版本，探测时返回明确的配置错误。
- `gdit install`（别名 `gdit new`）无参数时进入交互式条目安装（仅终端可用），依次确认条目显示名、
  edition、版本、SDK 策略与设为当前；下载按 source_order 自动 fallback。
- 命令与 flag 支持简写：顶层 `i/l/s/a/d/rm/r/st/e`；engine 子命令与 `sdk`/`autoremove`
  不设简写。

### FR-13 instances 条目层（P0 · 第三阶段）

- 条目是「引用资产 + 携带配置」的描述文件（`instances/<uuid>.toml`）：`id` 存储标识
  （UUID v4，与文件名一致）、`name` 显示名、`[engine]` 引擎引用、`[dotnet]` SDK 策略、
  `[env]` 条目环境，不复制任何二进制。
- 显示名与存储标识分离：文件名/current/内部引用一律使用 UUID；显示名只承担 CLI 寻址与
  展示，字符集为 URL 安全字符（ASCII 只允许 `[A-Za-z0-9._~-]`，非 ASCII 文字如中文允许），
  全仓库唯一（创建时锁内查重）。显示名不再与版本输入或资产 ID 形态互相排斥。
- 条目生命周期：安装（`gdit install`，别名 `gdit new`）、删除（`gdit remove <name>`）、列出（`gdit list`）、
  切换（`gdit default <name>`）与启动（`gdit run [<name>]`）。
- 条目的 engine 引用创建后不可变；`[env]` 用 `gdit env` 修改；`[dotnet]` 策略修改手写
  或重建条目。
- 不提供第二阶段数据迁移或兼容读取；schema_version 为 2，不迁移 schema 1 条目。

### FR-14 资产 GC（P0 · 第三阶段）

- 引用关系由条目扫描派生，不维护独立引用状态文件。
- 被任何条目引用的引擎/SDK 资产拒绝删除（引用保护）。
- 删除条目后提示孤儿资产（名称 + 占用空间）；`gdit autoremove [-y|--yes]` 显式清理。
- 不自动下载、不自动删除。

## 4. 非功能需求

| 类别 | 要求 |
|---|---|
| 分发 | CLI 单二进制；GUI 独立构建 |
| 平台 | Linux 主，macOS Apple Silicon 与 Windows x86_64 验证 |
| 配置 | 手写配置只使用 TOML |
| 无侵入 | 不改系统文件、shell 配置、全局环境或系统 dotnet |
| 健壮 | 多源 fallback，安装和 current 切换不暴露半成品 |
| 安全 | 下载内容必须按来源声明的摘要校验，自定义源由用户明确配置 |
| 架构 | CLI 和 GUI 共享 core，平台差异集中在 platform 适配层 |
| 输出 | 用户结果走 stdout，调试与进度走 stderr |

## 5. MVP 验收标准

P0 包括 FR-01、FR-02、FR-03、FR-04、FR-05、FR-08、FR-11、FR-13 和 FR-14。**P0 全集是
MVP 的最终目标；第一/二阶段只交付其中资产层部分（FR-01 的资产安装、FR-03 的资产查看、
FR-12）与 default/remove/setup/run 的版本形态**，其余按架构文档 §9.3 的阶段顺序在后续
阶段落地。验收以 Linux amd64 主平台为准；macOS Apple Silicon 与 Windows x86_64 为
验证级支持，行为验收分别在 Apple Silicon 实机与 Windows 实机（或 CI Windows runner）
完成，交叉编译只算构建检查。

1. 能安装（创建条目）、列出、切换和卸载（删除条目）多个 Godot 版本条目。
2. `godot` 在任意目录都启动 `~/.gdit/current` 指向的条目解析出的引擎。
3. dotnet 版按条目策略选择 SDK（managed 托管 / system 系统），并且环境变量只影响启动的子进程。
4. 国内源不可用时可以降级到下一来源，摘要不匹配时停止安装。
5. 安装中断不会污染资产列表，current 切换失败仍保留旧 current。
6. `gdit doctor` 能诊断目录、shim、当前条目、SDK 和环境。
7. 卸载后能提示无引用资产，`gdit autoremove` 可显式清理，被条目引用的资产永不误删。
8. 非法条目存在时资产删除失败关闭；确认期间引用关系变化时，`autoremove` 复查后不误删资产。

具体实现约束见 [`docs/architecture/`](architecture/README.md)。
