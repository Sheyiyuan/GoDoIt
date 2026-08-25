# GoDoIt 架构设计


> 状态为 v0.2 第六阶段 Linux 实现完成，macOS Apple Silicon 与 Windows x86_64 GUI 实机验证待完成
> 第四阶段 doctor（FR-08）验收见 §9.6；第五阶段实现见 §9.7；第六阶段实现与验收约束见 §9.8。
> 本文档是 GoDoIt 的唯一架构真理源。

第一阶段的产品边界、摘要信任模型、Go module 路径和 Linux amd64 支持范围已经确认，可以实施
`install/list` 纵向切片。2026-08-17 实测确认 GodotHub 和 Godot 官方的 URL 规则并写入内置 provider；
AtomGit 作为独立来源的规则尚未确认，确认前仍只实现 provider 契约、自定义来源和固定 fixture，
不把猜测地址写入内置 provider。

第二阶段为 `default/remove/setup/run`（见 §9.4）：`default` 取代原计划的 `use` 命名，语义为设置
全局默认版本；`run` 取代原 FR-11 的 `exec` 设计。第二阶段不包含环境注入与 .NET SDK 选择
（第三阶段），shim/run 只负责「读 current → 解析引擎 → 启动」。

第三阶段起引入 **instances 条目层**（见 §9.5），定位从「版本管理器」升级为「启动器与版本管理器」：
底层是包管理器（引擎/SDK 资产，通过 `gdit engine`/`gdit sdk` 命名空间降级访问），上层是条目（引用资产 +
SDK 策略 + 环境配置）。`current` 指向条目文件；shim/run 读条目 → 解析引擎与 SDK → 注入环境后
启动。安装与卸载的日常入口改为条目层：`gdit install` 交互式创建条目（确认条目名与各项配置），
`gdit remove <条目名>` 删除条目，资产随之变为孤儿由 `gdit autoremove` 清理；原资产层
install/remove/list 降级封装为 `gdit engine` 命名空间，`run`/`default` 只接受条目名，
不再解析版本输入。
SDK 策略为 `managed`（默认）/ `system`，推荐映射表写死由 core 维护；2026-08-18 实测确认
.NET SDK 官方下载与摘要 URL 规则；`display_driver = "auto"` 不注入参数，由 Godot 原生默认
与自动回退处理。第三阶段是破坏性布局切换，只支持全新的 `engines/`、`sdks/`、`instances/`
布局，不读取、不迁移第二阶段数据。

## 1. 边界

GoDoIt 是面向 Linux（主）、macOS（验证）与 Windows（验证）的 **Godot 引擎启动器与版本管理器**：
底层是包管理器（引擎与 .NET SDK 资产的安装、校验、卸载），上层是启动器条目（instances，
引用资产并携带 SDK 策略与环境配置），`godot` shim 读当前条目解析引擎与 SDK、注入环境后启动。
它只管理用户级 Godot 引擎、导出模板和 .NET SDK。所有持久状态都在 `~/.gdit/`。

平台矩阵（第四阶段起）：**Linux amd64 主平台**（完整验收）；**macOS Apple Silicon 与
Windows x86_64 验证级支持**（行为验收分别在 Apple Silicon 实机与 Windows 实机/CI runner
完成，交叉编译只算构建检查）。平台差异全部收敛在 platform 适配层（拆分方案见 §4.9），
业务代码不出现 `runtime.GOOS` 分支，不添加平台专属功能。

项目相关能力只有 `gdit suggest`。用户显式传入目录后，GoDoIt 只读分析 `project.godot`、
`global.json` 和 `.csproj`，给出安装建议。它不在项目目录写文件，不保存项目路径，不根据当前目录
自动切换引擎。

## 2. 用户目录

gdit 根目录默认是 `~/.gdit/`（Windows 为 `%USERPROFILE%\.gdit`），并允许用户通过环境变量
`GDIT_ROOT` 覆盖为任意绝对路径（如 Windows 的 `D:\gdit`，把数据移出系统盘）——配置、状态、
引擎、SDK、条目、临时目录全部随根目录走。解析顺序：`GDIT_ROOT`（非空即用，必须是绝对路径，
否则配置错误）→ 平台默认路径；解析只发生在 platform 适配层（`ResolveRoot`），core 仍只接收
已解析的根目录，不读环境变量。**下文文档中的 `~/.gdit/` 均指实际生效的根目录。**

```text
~/.gdit/                              # 默认根目录；可用 GDIT_ROOT 覆盖（Windows 为 %USERPROFILE%\.gdit）
├── config.toml                  # 用户配置（来源、全局环境）
├── state.toml                   # gdit 维护的已安装资产元数据（可自动重建）
├── bin/
│   ├── godot / godot.cmd        # 用户级 shim（Unix：symlink 指向 gdit；Windows：godot.cmd 包装）
│   └── gdit / gdit.exe          # gdit 自身（Windows：gdit.exe）
├── instances/                   # 条目层（第三阶段）：可切换的启动配置
│   └── <uuid>.toml              # 条目文件；文件名是 UUID v4，显示名存在文件内
├── icons/                       # 第六阶段：条目自定义图标（预设图标不复制到这里）
│   └── <uuid>.png               # 以条目 UUID 命名的规范化 PNG
├── current                      # 当前条目指针（第三阶段起；平台形态见下）
├── engines/                     # 引擎资产层（第二阶段的 versions/ 语义）
│   ├── 4.5.2-standard/
│   │   ├── install.toml         # 安装完成标记和最小可重建元数据
│   │   └── payload/             # 平台原始资产解压后的规范化内容
│   └── 4.5.2-dotnet/
├── sdks/                        # SDK 资产层（原 dotnet/，第三阶段）
│   └── 8.0.410/
├── templates/                  # 导出模板预留，本阶段不创建
├── cache/                    # P2 缓存管理预留，第一阶段不创建
└── tmp/                         # 下载和解压临时目录
```

- `config.toml` 是唯一需要用户编辑的配置文件。
- `engines/` 是判断已安装引擎的依据。一个目录只有名称合法、`install.toml` 可解析且目标可执行文件
  存在时才算完整安装。无效目录由 `doctor` 报告，`list` 不把它当作已安装版本。
- `install.toml` 由 gdit 在临时目录内生成，随整个版本目录一起原子发布。它记录版本 ID、目标平台、
  架构、edition、启动文件相对路径和资产摘要，不含用户配置。
- `state.toml` 由 gdit 维护，不一致时按 `engines/`、`sdks/` 的有效资产目录和 `install.toml` 重建，
  不要求用户编辑。
- `current` 是全局单一条目指针，所有目录使用同一个当前条目。平台形态由 platform 能力
  封装，core 层契约一致：
  - **Unix**（Linux/macOS）：symlink 指向 `instances/<uuid>.toml`，只接受规范相对链接；
    绝对路径、含 `..`、指向其他目录或指向非普通条目文件的目标一律视为无效 current。
  - **Windows**：普通文本文件，内容为规范相对路径 `instances/<uuid>.toml`（无 BOM、
    单行、允许结尾换行）；内容非规范相对路径、指向其他目录或非普通条目文件一律视为
    无效 current。选择重定向文件而非 symlink：Windows 创建文件 symlink 需要管理员或
    开发者模式，重定向文件零特权要求；读取/写入契约与 Unix 语义完全一致
    （`ReadCurrentLink`/`WriteCurrentLink` 平台能力），写入均为原子替换（Unix 临时
    symlink + rename；Windows 临时文件 + MoveFileEx）。
- `instances/` 只放条目描述文件，不复制任何二进制；条目引用资产层（引擎、SDK），不内嵌资产。
  条目文件名是存储标识符（UUID v4），用户可见的显示名（可中文）存放在文件内，二者分离：
  用户通过显示名寻址（`gdit run <显示名>`），内部一律以 UUID 为准（current、引用、GC）。
- `icons/` 只保存用户显式导入的条目图标；内置 Godot、C# 与 GoDoIt 吉祥物图标随 GUI 分发，
  不复制到用户目录。自定义图标以条目 UUID 命名，不能引用根目录外的任意路径。
- `engines/` 下只放完整安装；未完成内容只能出现在 `tmp/`。SDK 资产 `sdks/` 与引擎同规则。
- `cache/` 属于 FR-10（缓存管理，P2）预留，第一阶段不创建该目录，安装下载直接进入 `tmp/`。
- 运行时文件锁可以放在 `~/.gdit/.lock`，它不是配置或项目 lock。

第一版默认使用 `~/.gdit/`，不把同一套数据拆到多个 XDG 目录；用户可用 `GDIT_ROOT` 覆盖
为任意绝对路径（Windows 用户可把数据放在非系统盘）。三个平台保持相同根目录语义，
只有引擎资产布局、shim 形态、current 文件形态和平台命令由适配层处理。

**shim 平台形态**：Unix（Linux/macOS）为 `bin/godot` symlink 指向 gdit，以 `godot` 名称
启动时由 argv[0] 判断进入 shim 路径并 execve 引擎；Windows 为 `bin/godot.cmd` 批处理包装
（调用 `setup` 时实际运行的 `gdit.exe` 绝对路径，追加 `__shim %*` 并透传 `%errorlevel%`），由 `__shim` 入口解析 current、
spawn 引擎并透传退出码——Windows 无 execve，子进程共享控制台，Ctrl+C 天然透传。
`__shim` 与 Unix 的 argv[0] 判断共用同一份 runShim 逻辑（永远启动 current、参数直通引擎、
不经过 gdit 命令解析、无 TTY 交互）；argv[0] 判断仅 Unix 有效（Windows 下 `godot.cmd`
内的 argv[0] 是 gdit.exe），Windows 靠显式子命令识别 shim 调用。`.cmd` 不复制二进制；
原路径内升级自动生效，移动 `gdit.exe` 后重新运行 `setup` 修复包装。

core 构造时必须接收一个已解析的根目录。CLI 和 GUI 的生产入口通过 platform 适配层的
`ResolveRoot` 解析：`GDIT_ROOT` 非空时校验并采用（必须是绝对路径，否则配置错误），否则
按平台默认路径（`~/.gdit/` / `%USERPROFILE%\.gdit`）。测试直接传入临时目录。core 内部
不能再次读取真实用户主目录或环境变量，也不提供隐式的项目级根目录覆盖。

## 3. 版本标识

资产 ID 只包含精确 Godot 版本和 edition，格式如下。它是**资产层**的寻址键（`engines/<id>/`
目录名），不是用户切换的粒度。

```text
4.5.2-standard
4.5.2-dotnet
```

- `standard` 表示标准版。
- `dotnet` 表示 C#/.NET 版；CLI 可以接受 `mono` 作为输入别名，但内部统一保存为 `dotnet`。
- 版本语法：两/三段稳定版（`4.5.2`、`4.7`——官方 minor 首发不写 .0）或两/三段预发布
  （`4.8-dev3`、`4.7.2-rc1`、`4.7-rc3`、`4.7.1-beta2`）。预发布版本的资产 ID 保留后缀
  （`4.8-dev3-standard`）。
- 下载源不属于版本 ID。同一版本可以从不同来源下载，安装后仍是同一个版本。
- 平台和架构用于选择下载资产，不进入用户看到的版本 ID。

CLI 第一阶段使用下面的输入形式。

```text
gdit install --edition standard 4.5.2
gdit install --edition dotnet 4.5.2
gdit list
```

`--edition` 默认是 `standard`，输入 `mono` 时归一化为 `dotnet`。版本解析只接受三个十进制数字段，
拒绝 `latest`、版本范围、预览版和带任意后缀的输入。core 在任何路径拼接之前完成解析和规范化。

第二阶段的简写输入形式（`m` 前缀，仅小写）：`m<版本>` 等价于 `--edition mono <版本>`，
归一化后仍为 `dotnet` edition。版本号以数字开头，`m` 前缀无歧义。`m` 前缀与 `--edition`
同时出现时报用法错误；`m` 后不是合法三段版本号时走现有版本语法错误。该糖在版本解析层
实现，install/remove/default/run 四个命令统一生效。

平台支持矩阵（第四阶段起）：Linux amd64 主平台（完整验收）、macOS Apple Silicon 与
Windows x86_64 验证级支持（实机验收，交叉编译不算完成）。Linux arm64 是否列入支持范围
仍需在 review 时明确，不能仅凭上游存在对应资产就视为已支持；Windows arm64 资产（4.5.2
起有 `windows_arm64.exe.zip`）同样不承诺。

第三阶段起，用户日常寻址的单位是**条目显示名**，资产 ID 只在 `gdit engine`/`gdit sdk`
命名空间和条目引用中出现，不再直接出现在日常命令里。条目文件以 UUID v4 命名
（`instances/<uuid>.toml`），显示名（任意 UTF-8，可中文，规则见 §4.8）与存储标识分离：
显示名只承担 CLI 寻址与展示，uuid 承担文件系统与引用锚定，二者不再互相约束，因此显示名
不需要也不能匹配版本输入或引擎资产 ID 的形态。`default` 是普通显示名（交互安装默认值）。
`m` 前缀与 `--edition` 语法从条目层命令移除，`gdit engine install/remove` 保留
版本输入语法。

## 4. 核心流程

### 4.1 安装

```text
解析版本
  → 解析平台目标和上游资产名
  → 获取全局修改锁并复查是否已安装
  → 清理 tmp/ 下遗留的 operation 目录
  → 按来源顺序解析资产和预期摘要
  → 下载到 ~/.gdit/tmp/<operation-id>/
  → 校验预期摘要
  → 安全解压到同一临时目录
  → 校验启动文件并生成 install.toml
  → 原子移动到 ~/.gdit/engines/<id>/
  → 原子写入 state.toml
```

每个 source provider 把结果归为成功、不可用、配置错误、完整性错误或取消。找不到对应资产、连接超时、
限流和服务端暂时失败属于不可用，可以按顺序尝试下一来源。配置错误直接终止并指出来源名。摘要不匹配
属于完整性错误，立即终止整个安装，不能继续 fallback。context 取消也立即终止。

下载必须写入新建的 operation 目录，文件名不能来自未经清理的 URL 路径。解压时拒绝绝对路径、
越过目标目录的 `..`、设备文件和逃逸目标目录的 symlink。发布前由 platform 适配层确认启动文件布局并
设置所需执行权限。目标版本目录已经存在时返回可识别的冲突结果，不覆盖已有内容。

临时目录和 `engines/` 位于同一 gdit 根目录，以保证目录 rename 不跨文件系统。目录原子移动成功
即表示安装完成。若随后更新 `state.toml` 失败，本次安装仍返回“已安装但状态索引待重建”的结果，
下次读取时按版本目录重建。CLI 需要把这个结果明确写到 stderr，不能把版本误报为未安装。

`.lock` 覆盖 install、remove、default、setup、source use 的配置写回以及会落盘的状态重建。
等待锁时响应 context 取消。
读取操作先扫描有效版本目录；发现 `state.toml` 不一致时，在取得锁并二次扫描后再原子重写，避免用
过期快照覆盖另一个进程刚完成的安装。

获取修改锁后先删除 `tmp/` 下遗留的 `operation-*` 目录。锁内不存在其他进行中的安装，遗留目录都是
进程中断的残留，直接删除不会误伤并发任务；清理失败按本地 I/O 错误处理。

所有 TOML 状态文件都写入同目录临时文件，完成编码、flush、文件同步和 close 后再 rename，随后同步
父目录。版本目录发布和 current 切换也同步对应父目录。相关系统调用由 platform 适配层封装。

第一/二阶段 CLI 的 `install` 接受多个版本参数（如 `gdit install 4.5.2 m4.6.2`）；第三阶段该
资产层语法原样移动到 `gdit engine install`。它逐个串行执行上述完整流程：
每个版本独立解析（`m` 前缀可混用）、独立 fallback 与校验，任一失败不中断其余，退出码按是否有
失败汇总。`--edition` 显式给出时统一应用于所有参数，与任何 `m` 前缀同时出现报用法错误。该编排
只发生在 CLI 层，core 的 `Install` 接口与锁模型不变；并发下载不在本阶段范围（需先重构锁粒度与
operation 清理归属，见 4.1 锁内清理假设）。

### 4.2 current（第二阶段为默认版本，第三阶段为当前条目）

第二阶段 `gdit default <id>` 检查目标已经完整安装，然后原子替换 `~/.gdit/current` symlink（指向
`versions/<id>/`）。失败时保留旧链接。`gdit default` 无参数时显示当前默认版本；未设置或链接悬空时
报错并提示先安装或 setup。`default` 只接受已完整安装的版本，不触发隐式下载，也不做项目级自动切换。
`gdit list` 对默认版本追加 `default` 标记，且该行整行用品牌色高亮：stdout 为 TTY 且未设置
`NO_COLOR` 时生效（truecolor 不支持时回退绿色），非 TTY 保持纯文本，保证管道和重定向场景下
stdout 仍机器可读。

第三阶段起（见 §4.8 与 §9.5）：`current` 指向 `instances/<uuid>.toml`；`gdit default` 只接受
**条目显示名**——`gdit default <name>` 校验条目存在且引用完整安装后原子替换 current，失败保留旧
链接；`gdit default` 无参数显示当前条目（显示名 + 引擎 + edition + SDK 策略）。按版本号设默认的
旧形态（`gdit default <版本>`）随条目化移除；要换引擎版本就安装/创建新条目，不修改既有条目的
engine 引用。

### 4.3 启动

`gdit setup` 在用户显式执行时创建或修复 `~/.gdit/bin/godot` shim，并在该目录未加入 PATH 时给出
提示。它不修改 shell 配置或系统 PATH。以 `godot` 名称启动时，gdit 进入 shim 路径：读取
`~/.gdit/current`，解析引擎启动文件，execve 替换自身进程启动真实 Godot，透传参数、stdio 和退出码。
shim 的平台形态见 §2：Unix 靠 argv[0] 判断进入 shim 分支；Windows 由 `godot.cmd` 调用
`__shim` 显式子命令进入同一 runShim 逻辑（参数直通引擎、不过命令解析、无 TTY 交互）。

`gdit run` 第三阶段起只接受条目名：`gdit run`（无参数）启动当前条目，等价于裸 `godot`；
`gdit run <name>` 启动命名条目；`-d` 保留为「当前条目」的显式别名。无参数 + TTY 且存在
**多于一个条目**时弹出交互菜单选择要启动的条目（选项标注当前条目；单/零条目不弹，直接
走当前条目逻辑），非 TTY 保持脚本语义（直接当前条目）。原「`<版本>` 显式启动指定版本」
形态随条目化移除——启动必须经过配置好的条目（SDK 策略与条目环境才有来源），版本输入直接
报用法错误。`--` 之后的参数原样透传给引擎。

第二阶段 shim/run 只做「读 current → 解析引擎 → 启动」，不从 `config.toml` 合并环境，环境注入
属于第三阶段（FR-04）。dotnet 版启动缺 SDK 时由引擎自行报错，CLI 输出不暗示 C# 已可完整启动。
shim 不读取当前项目，也不访问网络。必须经过 gdit 而不是直接 symlink 到引擎，因为环境变量
（第三阶段起）需要只注入目标子进程。

第三阶段起（见 §9.5）：shim/run 在启动前解析当前条目——读引擎引用、按条目 SDK 策略选择 SDK
（managed 用 `~/.gdit/sdks/` 指定版本，system 用系统 dotnet）、合并全局与条目环境、注入显示
驱动参数与 fcitx 变量，dotnet 版注入 `DOTNET_ROOT`/PATH 前缀；注入只作用于目标子进程的 env，
不修改 shell、系统 PATH 或系统 dotnet。启动路径零网络、零落盘。

### 4.4 项目建议

`gdit suggest <目录>` 调用只读探测器，输出推荐的 Godot edition/版本和 .NET SDK。默认不安装，
显式 `--install` 或用户确认后才复用正常安装流程。分析结果不写入项目或 gdit 状态。

### 4.5 来源与摘要契约

source provider 接收规范化版本、edition 和 platform target，返回下载 URL、规范资产名、可选大小、
摘要算法与预期摘要。业务层只依赖这个结构和错误分类，不拼接某个镜像的 URL。内置来源和自定义来源
使用同一接口，fixture HTTP server 可以完整替代真实网络。

2026 年 8 月核对 [Godot 4.5.2 官方发布页](https://github.com/godotengine/godot-builds/releases/tag/4.5.2-stable)
时，发布资产提供 `SHA512-SUMS.txt`，官方不提供 SHA-256 清单。内置 `github` 来源使用官方资产并
按 SHA-512 校验；GodotHub 来源的 digest 字段提供 SHA-256，按项目约定使用 sha256。GoDoIt 不把
摘要计算结果当作新的信任来源，只验证下载内容是否与来源声明一致。

每个 source provider 必须明确声明摘要算法。状态模型保存 `algorithm + value`，第一阶段同时支持
`sha256`（GodotHub、自定义来源）和 `sha512`（官方 github 来源），以后扩展到上游 SHA-512 时无需
改变安装目录格式。摘要不匹配立即终止安装，不能切换到下一来源。

自定义源的校验清单按以下规则解析：每行接受「摘要 文件名」（空格分隔）或 BSD 风格「摘要 *文件名」；
也允许单列裸摘要，但裸摘要行只能出现一次，多行裸摘要视为配置错误；摘要十六进制大小写不敏感；
清单中找不到目标资产条目视为配置错误。元数据驱动来源（godothub 的 releases.json、github 的
releases 端点）中 release 或资产不存在属于来源不可用，按顺序尝试下一来源；元数据本身无法解析、
digest 字段非法才属于配置错误。摘要信任与下载信任绑定：校验值来自来源自身声明的清单或
digest 字段，GoDoIt 不把计算结果当作新的信任来源——来源被攻破即等价于镜像被攻破，这是与常见
包管理器一致的供应链信任取舍。

### 4.6 内置来源 URL 规则（2026-08 实测确认）

```text
github  资产     https://github.com/godotengine/godot-builds/releases/download/{tag}/{asset}
        tag       {version}-stable（稳定版）或 {version}（预发布），如 4.5.2-stable、4.8-dev3
        资产名     Linux：Godot_v{version}-stable_linux.x86_64.zip
                         Godot_v{version}-stable_mono_linux_x86_64.zip（dotnet 版）
                   Windows：Godot_v{version}-stable_win64.exe.zip
                         Godot_v{version}-stable_mono_win64.zip（dotnet 版）
                   macOS：Godot_v{version}-stable_macos.universal.zip（4.x）
                         Godot_v{version}-stable_mono_macos.universal.zip（4.x dotnet 版）
                   预发布版无 -stable 段：Godot_v4.8-dev3_linux.x86_64.zip
                   3.x 命名不同：Godot_v3.6.2-stable_x11.64.zip（Linux standard）、
                   Godot_v3.6.2-stable_mono_x11_64.zip（Linux mono，下划线）、
                   Godot_v3.6.2-stable_win64.exe.zip（Windows standard）、
                   Godot_v3.6.2-stable_mono_win64.zip（Windows mono）、
                   Godot_v3.6.2-stable_osx.universal.zip（macOS，3.x 用 osx 前缀）、
                   Godot_v3.6.2-stable_mono_osx.universal.zip（macOS mono）
                   3.x 的 x11 32 位、headless/server 变体与 4.x 的 arm32/arm64 变体不收录
        摘要      SHA512-SUMS.txt（同 release 目录，SHA-512）

godothub 元数据  https://legacy.godothub.com/api/releases.json
        资产      https://atomgit.com/godothub/godot/releases/download/{tag}/{asset}
                  （实测 302 到 file-cdn.gitcode.com 签名 CDN）
        摘要      releases.json 中对应资产的 digest 字段（sha256:<64 hex>）
```

2026-08 实测：官方 godot-builds releases 同时提供稳定与预发布 tag（4.7.2-stable、
4.8-dev3、4.7.2-rc1、4.7-rc3、4.7-beta5），预发布同样带完整平台资产；GodotHub 的
releases.json 只有稳定版但收录 mono 资产。`{tag}` 模板规则：版本号含预发布后缀时直接
用版本号，否则拼 `-stable`。

GodotHub 的 `releases.json` 是 GitHub Release API 结构，按 `tag_name` 匹配 release、按资产名匹配
条目后读取 `digest`。GoDoIt 不调用 godothub.com 的页面接口，只依赖上述稳定 URL。官方默认
source_order 为 `["godothub", "github"]`，保证 GodotHub 不可用时直接降级到官方源；AtomGit 独立
规则确认后加入默认顺序。用户显式在 source_order 中配置 atomgit 时，该位置返回明确的配置错误，
可用 `custom_sources` 或改写 source_order 绕过。

### 4.7 来源管理（第一阶段扩展）

> 本节的顶层 `install` 命令形态与交互流程自第三阶段起由 §9.5 接管（顶层 install 改为
> 条目层，资产层版本语法移入 `gdit engine install`）；来源语义（source_order、ban/unban、
> 指定来源、枚举）不受影响，仍以本节为准。

在第一阶段 install/list 基础上增加来源的查看、默认切换、禁用/启用、指定来源安装、可用版本
探测和交互式安装。全部围绕 source 层，不引入版本切换语义，不改变后续阶段顺序。

```text
gdit install                          # 交互式安装：依次选择 edition、version、source
gdit install [--edition standard|dotnet] [--source <name>] <version>
gdit list
gdit source                           # 按 source_order 顺序列出来源、类型与禁用状态
gdit source use <name>                # 把 <name> 移到 source_order 首位并写回 config.toml
gdit source ban <name>                # 禁用来源：不再参与自动 fallback 与默认枚举
gdit source unban <name>              # 启用来源
gdit available [--source <name>]      # 探测当前平台可安装的版本（稳定版与预发布，按系列分组输出）
```

命令简写：`install`/`i`、`list`/`l`、`source`/`s`、`available`/`a`；flag 简写
`--edition`/`-e`、`--source`/`-s`。后续阶段命令可能重新占用简写，以各阶段文档为准。

- 交互式 `install` 只在标准输入为 TTY 时可用，非 TTY 下无参数调用返回用法错误，避免脚本卡死。
  交互流程：先选 edition（standard/dotnet），再从 `available` 的枚举结果中选 version（该
  edition 的版本），最后选 source（`auto` 按顺序 fallback 或指定单个来源）。枚举失败或列表
  为空时降级为文本输入版本号。交互实现使用 `github.com/AlecAivazis/survey/v2`，只出现在 CLI
  层，core 不依赖（见 7.2）。
- `source` 无参数时列出全部来源：顺序、名称、类型（builtin/custom）、禁用状态，只读不落盘。
- `source use` 把目标来源移到 source_order 首位，其余保持相对顺序，原子写回 config.toml，
  受全局修改锁保护。写回保留全部已知与未知字段（键顺序由写回编码确定，注释不保留，
  见第 5 节）。配置文件不存在时按内置默认创建后再调整。被禁用的来源不能 use。
- `source ban`/`unban` 是强禁用语义：被禁用的来源不参与自动 fallback（install 默认流程）、
  不参与默认枚举（available），显式 `install --source <name>`、`available --source <name>` 或
  `source use <name>` 指定被禁用的来源时报配置错误（source is disabled），必须先 unban。
  禁用状态写入 config.toml 的 `disabled_sources`，受全局修改锁保护。
- `install --source <name>` 明确指定来源：只使用该来源解析资产并下载，来源不可用或完整性
  错误时直接失败，不按 source_order fallback。名字必须存在于当前 source_order 且未被禁用。
- `available` 默认合并 source_order 中所有启用且支持枚举来源的结果：版本取并集，标注每个
  版本可见于哪些来源；单个来源枚举失败只影响该来源，原因写 stderr，其余来源继续。
  指定 `--source` 时只用该来源。所有来源都失败或都不支持枚举时返回错误。
- 枚举结果 = 当前平台可安装的版本（稳定版与预发布）：稳定 tag 匹配
  `\d+\.\d+(\.\d+)?-stable`（含 4.7-stable 这类 .0 首发 tag），预发布 tag 匹配
  `\d+\.\d+(\.\d+)?-(dev|rc|beta|alpha)\d+`（如 4.8-dev3、4.7.2-rc1、4.7-rc3）；
  版本号 = 稳定 tag 去 `-stable` 后缀，预发布 tag 原样。edition 按 `platform.AssetName`
  生成的两个候选资产名在 release 资产中的实际存在判断（3.x 用 x11.64/osx.universal
  命名，其 mono 版依赖 Mono 运行时，明确不支持，不列出）。某 release 没有任何当前平台
  资产时（如 3.x 的 x11 32 位变体）不列出。交互式选择与 `available` 输出按系列分组：稳定版
  按 major（`4.x`/`3.x`），预发布统一归入 `unstable` 组（组间 major 倒序、unstable 最后，
  组内版本倒序），先选系列再选具体版本。

#### provider 契约扩展

```go
// VersionInfo 是来源可用的版本条目（稳定或预发布）。
type VersionInfo struct {
	Version  string   // 如 4.5.2、4.8-dev3
	Editions []string // 按当前平台资产判断，如 standard、dotnet
}

// VersionLister 是可枚举可用版本的来源；URL 模板型自定义源不实现该接口。
type VersionLister interface {
	ListVersions(ctx context.Context) ([]VersionInfo, error)
}
```

- `HTTPProvider` 增加可选字段 `ReleasesURL`，指向 GitHub Release API 兼容的 JSON
  （tag_name + assets[].name）。设置该字段的来源支持枚举；未设置的自定义源调用
  `ListVersions` 返回配置错误，提示该来源无法枚举版本。
- 内置 github 来源的 `ReleasesURL` 为
  `https://api.github.com/repos/godotengine/godot-builds/releases?per_page=100`。
  2026-08-17 实测返回 HTTP 200，单页 100 条，与 godothub 的 releases.json 同为
  GitHub Release API 结构；分页暂不处理，单页 100 条为当前枚举上限，
  完整历史枚举（含 4.0 及更早）后续扩展。
- godothub 来源的枚举复用已实测的 releases.json 元数据，不新增请求端点。
- 枚举只反映来源元数据声明的内容；列出的版本仍需走完整下载与摘要校验流程。

### 4.8 instances 条目层（第三阶段）

instances 是 GoDoIt 的「高级包管理器」：条目不安装任何二进制，只是**引用资产 + 携带配置**的
描述文件。底层包管理器原样保留——`gdit engine`/`gdit sdk` 命名空间做引擎与 SDK 资产的
增删查，顶层 `available`/`source` 做枚举与来源管理——条目消费它的产物，并反过来提供
「无引用资产」清单给 GC。日常命令的入口是条目层：安装 = 创建条目（顺带安装资产），
卸载 = 删除条目（资产变孤儿），启动 = 读条目。

```toml
# ~/.gdit/instances/<uuid>.toml
schema_version = 2
id = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"  # 存储标识符（UUID v4），与文件名一致
name = "default"          # 显示名：用户寻址用，唯一，可中文

[engine]
version = "4.5.2"        # 引用引擎资产 engines/4.5.2-standard
edition = "standard"

[dotnet]                  # 仅 dotnet edition 时有意义
strategy = "managed"      # managed（默认，version 必填）| system（用系统 dotnet）
version = "8.0.410"       # managed 必填，system 时忽略

[template]                # 第五阶段可选：声明导出模板资产依赖
id = "4.5.2-standard"     # 必须与 engine 的 version + edition 一致

[appearance]              # 第六阶段可选：GUI 条目图标
icon = "default"          # default | godot | csharp | mascot | custom
background = "#A179DC"   # 可选；缺失/空值为透明，也接受 #RRGGBBAA
# custom_icon = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8.png" # 仅 custom 时使用

[env]                     # 条目级环境覆盖（FR-04 演进），全局 [environment] 之下
# EXAMPLE_VARIABLE = "value"
```

- `current` 是全局单一条目指针（平台形态见 §2：Unix symlink / Windows 重定向文件），
  指向 `instances/<uuid>.toml`；切换条目 = 原子替换 current。
- `[appearance]` 是第六阶段新增的向后兼容可选字段，不提升 schema 版本。字段缺失等价于
  `icon = "default"`：standard（普通版）解析为 Godot 图标，dotnet edition 解析为 C# 图标；
  Godot 3 mono 输入在 core 内归一化为 dotnet edition，因此同样解析为 C#。用户也可显式固定
  `godot`、`csharp`、`mascot` 或导入 `custom`。`custom_icon` 只接受与条目 UUID 一致的 PNG
  文件名，并固定在 `icons/` 下解析，禁止绝对路径、`..` 和跨目录 symlink。`background`
  缺失或为空时透明，只接受 `#RRGGBB` 或 `#RRGGBBAA`。
- **显示名与存储标识分离**：文件名是 UUID v4（`crypto/rand` 生成，零新增依赖），条目文件内的
  `id` 字段与文件名一致；`name` 是用户可见的显示名，只承担寻址和展示，不参与任何文件系统
  操作（因此 macOS 文件系统的 Unicode 正规化差异不会影响中文显示名的匹配）。
- 显示名规则：由 URL 安全字符组成——ASCII 字符只允许 `[A-Za-z0-9._~-]`（RFC 3986
  unreserved），非 ASCII 文字（中文、日文等，URL 百分号编码后无歧义）全部允许；
  禁止空格、标点、符号与控制字符（`\t` 同时破坏 stdout 的 tab 分隔机器输出）。
  该规则保证显示名可以直接进入 URL 路径段或查询参数而不引入歧义，也为将来
  wiki/站点、GUI 路由等场景预留安全边界。显示名不再需要与版本输入或引擎资产
  ID 形态去歧义——版本号/资产 ID 与显示名分属两个命名空间，`gdit run 4.5.2` 只是查找一个
  恰好叫这个名字的条目，不存在歧义。
- 显示名全仓库唯一：创建条目时在全局修改锁内扫描全部条目校验，重名报输入错误；CLI 按显示名
  精确匹配寻址，永远无歧义。`default` 是普通显示名（交互安装的默认值）。
- 命名条目本阶段开放：`gdit install` 交互式安装时确认条目显示名与配置（见 §9.5），
  `gdit remove <name>` 删除条目，`gdit run <name>`/`gdit default <name>` 按显示名使用。
- 条目对引擎/SDK/模板的引用是**派生关系**：每次扫描 `instances/*.toml` 重算引用集合，
  不维护独立引用状态文件（与 state.toml 可重建同一哲学）。
- 条目创建后 `[engine]` 引用不可变（换引擎版本 = 建新条目）；`[env]` 用 `gdit env` 修改；
  `[dotnet]` 策略本阶段无专门修改命令，手写条目文件或重建条目。
- 条目校验：`id` 是合法 UUID v4 且与文件名一致；`name` 非空、字符集合法且全仓库唯一；
  `[engine]` 引用必须存在对应完整安装；standard 条目出现 `[dotnet]` 表视为配置错误；
  `managed` 且 version 缺失/非法为配置错误，`system` 时忽略 version；`managed` 声明的版本
  未安装属于启动错误（`ErrNoCompatibleSDK`），不属于配置错误。第五阶段可选 `[template].id`
  必须等于 engine 派生的模板 ID；模板资产缺失只影响导出能力，不使条目失效或阻断启动。
- 引用扫描采用失败关闭：`instances/` 中只要存在不可读、不可解析、schema 不支持、id 与文件名
  不一致、显示名重复或不是普通文件的候选条目，引用扫描整体返回配置错误。`engine remove`、
  `sdk remove` 和 `autoremove` 在错误修复前不得删除任何资产，不能跳过坏条目后继续计算孤儿。

#### 资产引用与 GC（apt 语义）

- 引用来源只有条目：`[engine]` 引用引擎资产，`[dotnet].strategy = "managed"` 引用
  `sdks/<version>/` 资产，第五阶段起可选 `[template]` 引用 `templates/<id>/` 资产。
- 引用保护：资产删除前检查条目引用，被任何条目引用的引擎/SDK/模板拒绝删除，提示由哪个
  条目引用；条目必须先删除，模板也可先从条目 detach，使资产变为孤儿。
- `gdit remove <name>` 在锁内先完整校验条目集合，并按“排除目标条目”计算孤儿及空间；全部
  计算成功后才删除条目文件。返回这份同一临界区内的孤儿结果，在删除输出后列出提示：
  `以下资产已无引用，可用 gdit autoremove 清理`，并附每个资产的占用空间。当前条目拒绝删除
  （必须先 `gdit default` 到其他条目），保证 current 不悬空。
- `gdit autoremove [-y|--yes]` 列出全部孤儿资产（名称 + 空间），TTY 下 survey 确认
  （默认否），非 TTY 必须 `-y`。删除语义与资产 remove 一致（锁内删除、state 原子重建、
  运行中的进程不受影响）。用户确认后必须在全局修改锁内重新扫描条目和孤儿集合，只删除
  复查时仍为孤儿的资产；被任何条目引用的资产永不进入实际删除结果。
- 不自动下载、不自动删除：default/run 只选择条目引用的完整安装资产，shim 启动不访问
  网络（与 FR-02/FR-05 的显式原则一致）。自动化的只是「引用关系维护」，不是资产动作本身。

### 4.9 平台适配层拆分（Linux / macOS / Windows）

平台矩阵与验证级定义见 §1 与 §3。平台差异全部收敛在 `core/internal/platform/`，
**业务代码（core 其余包、CLI、GUI bridge）不出现 `runtime.GOOS` 分支**。适配层按 OS
拆分实现文件，由文件名后缀承担 build tag：

```text
core/internal/platform/
├── platform.go           # 平台无关：Target 类型、IsLinux/IsDarwin/IsWindows 判定、共享常量
├── platform_unix.go      # linux+darwin 共用 POSIX 能力（symlink、flock、rename、fsync、权限）
├── platform_linux.go     # Linux：资产名、launcher、fcitx、display driver
├── platform_darwin.go    # macOS：资产名（app bundle）、osx 命名、无 fcitx/display
└── platform_windows.go   # Windows：.cmd shim、current 重定向文件、MoveFileEx、LockFileEx、无目录 fsync
```

上层只调用平台能力函数，不直接做 OS 分支。第四阶段同时把现有实现中散落的平台判断
（lock 的 flock、CLI 的 execve、store 的 rename/sync 封装）下沉为平台能力，业务层
行为不变。

**能力清单**（platform 导出面；测试用固定输入覆盖三平台映射）：

| 能力 | Linux | macOS | Windows |
|---|---|---|---|
| `ResolveRoot()` | `GDIT_ROOT` 非空即用（须绝对路径）→ `$HOME/.gdit` | 同 Linux | `GDIT_ROOT` 非空即用 → `%USERPROFILE%\.gdit` |
| `AssetName()` | `linux.x86_64` / `mono_linux_x86_64` | `macos.universal` / `mono_macos.universal`（3.x：`osx.universal` / `mono_osx.universal`） | `win64.exe` / `mono_win64` |
| `FindLauncher()` | 解压根目录二进制 | `.app` bundle 内 `Contents/MacOS/<bin>` | 解压根目录 `.exe` |
| `SDKRID()` | `linux-x64` | `osx-arm64` | `win-x64` |
| `SDKArchiveFormat()` | tar.gz | tar.gz | **zip**（`dotnet-sdk-<v>-win-x64.zip`，实测确认） |
| `PrepareLauncher()` | chmod +x | chmod +x | no-op（无执行位） |
| `DetectFcitx()` | XMODIFIERS / 进程检测 | 不注入 | 不注入 |
| `DisplayDriver()` | auto / x11 / wayland | 仅 auto | 仅 auto |
| `ShimPath()` / `EnsureShim()` | `bin/godot` symlink → gdit | 同 Linux | `bin/godot.cmd` 包装（记录 `setup` 时实际 `gdit.exe` 的绝对路径，调用 `__shim %*` 并透传退出码） |
| `IsShimInvocation()` | argv[0] 基名为 `godot` | 同 Linux | 恒 false（`godot.cmd` 内 argv[0] 是 gdit.exe，识别由 `__shim` 子命令承担） |
| current 读写（`ReadCurrentLink`/`WriteCurrentLink`） | symlink（规范相对链接） | 同 Linux | 普通文本文件，内容为规范相对路径 `instances/<uuid>.toml`（零特权，见 §2） |
| `AcquireLock()` | flock（x/sys/unix） | 同 Linux | LockFileEx（x/sys/windows，零新增依赖） |
| `RenameAtomic()` | rename(2) | 同 Linux | MoveFileEx(MOVEFILE_REPLACE_EXISTING)（`os.Rename` 目标存在即失败） |
| `SyncDir()` | 目录 fsync | 同 Linux | 无目录 fsync；降级为文件 FlushFileBuffers，目录项持久性依赖 NTFS 日志 |
| `PathListSeparator` | `:` | `:` | `;` |
| 引擎启动（CLI 层） | execve | execve | os/exec spawn（Windows 无 execve；子进程共享控制台，Ctrl+C 天然透传） |

**环境注入的平台化**（编译标签拆分实现）：

`core/internal/env/` 按 OS 拆分注入实现：`env.go`（合并顺序与通用逻辑）、`env_linux.go`
（display driver 参数、fcitx 变量）、`env_darwin.go` / `env_windows.go`（仅通用注入，
无 Linux 专用变量）。配置侧支持平台小节（见 §5）：全局 `[environment]` 为三平台通用，
`[environment.linux|darwin|windows]` 仅当前平台生效（覆盖全局同名键），条目 `[env]`
在两者之上。合并顺序：继承父环境 → 全局 `[environment]` → 平台小节
`[environment.<os>]` → 条目 `[env]` → 派生变量（fcitx、DOTNET_ROOT、PATH 前缀）。
`EffectiveEnv`/doctor 的 environment 检查项按此顺序展示并标注来源（global/platform/
instance/derived）。

**各层差异说明**：

- store/lock：锁、原子 rename、父目录同步、current 读写全部走适配层（架构已有「相关
  系统调用由 platform 适配层封装」的约定，第四阶段把 Windows 实现补齐）。
- env：见上「环境注入的平台化」；PATH 前缀分隔符用 `os.PathListSeparator`；
  `DOTNET_ROOT` 三平台语义一致；fcitx 与 display driver 仅 Linux（非 Linux 平台
  `display_driver`/`input_method` 只接受 `auto`，显式其他值报配置错误——沿用
  「macOS 不注入 Linux 专用变量」哲学扩展为三平台）。
- dotnet：RID 映射与资产格式按平台；Windows SDK 复用现有 zip 安全解压；launcher
  为 `dotnet.exe`。
- source：资产名按平台生成；`available` 枚举按平台资产存在判断 edition（Windows
  的 `win64.exe.zip`/`mono_win64.zip`、macOS 的 `macos.universal`/3.x `osx.universal`）。
- instance：显示名规则三平台一致（条目文件名是 UUID，不触碰文件系统大小写敏感差异）。
- shim/run：见 §2「shim 平台形态」；`run` 语义三平台一致，退出码透传。
- doctor：检查项按平台差异化（见 §9.6）。

**Windows 特有问题（设计决策，待 review）**：

1. shim 采用 `godot.cmd` 包装而非复制 `godot.exe`：包装记录 `setup` 时实际运行的
   `gdit.exe` 绝对路径，避免复制品与原二进制不同步；原路径内升级自动生效，移动后重新运行
   `setup` 修复，与 Unix symlink 的生命周期一致。包装调用
   `__shim` 入口而非 `run`：shim 语义要求参数直通引擎、不经过 gdit 命令解析且无 TTY
   交互，`run` 会解析参数并可能在 TTY 下交互选择条目（见 §2）。（建议：接受 .cmd。）
2. current 采用**普通重定向文件**（内容为规范相对路径）而非 symlink：Windows 创建文件
   symlink 需要管理员或开发者模式（Windows 10 14972+），重定向文件零特权；读写契约
   与 Unix 语义一致，原子替换用临时文件 + MoveFileEx。不再要求开启开发者模式。
3. Windows 无目录 fsync：原子写降级语义文档化，不追求与 POSIX 完全一致的崩溃保证。
4. 3.x mono 版在 Windows 同样只安装资产、不解析不注入 SDK（Mono 运行时用户自理）。
5. PATH 提示按平台输出（Unix `export PATH=...`；Windows 提示 `set PATH=...` 或
   PowerShell 形式），由适配层提供提示模板，CLI 不拼平台文本。
6. 行为验收：macOS Apple Silicon 实机；Windows x86_64 实机或 CI Windows runner
   （现有 CI 已含跨平台构建，行为测试在 windows-latest runner 上执行）。纯映射
   函数（资产名/launcher/RID/资产格式）在 Linux 上用固定输入单测覆盖三平台输出；
   涉及文件系统、进程与环境的测试必须在对应实机执行。

## 5. 配置

`~/.gdit/config.toml` 集中保存全部用户配置，内容如下。

```toml
schema_version = 1
source_order = ["godothub", "github"]
disabled_sources = []

[environment]
display_driver = "auto"
input_method = "auto"
COMMON_VARIABLE = "all platforms"     # 三平台通用变量

[environment.linux]                   # 平台小节（第四阶段）：仅当前平台生效，覆盖全局同名键
XDG_SESSION_TYPE = "x11"

[environment.windows]                 # Windows 专用变量（用户按需配置）
EXAMPLE_WINDOWS_ONLY = "value"

[[custom_sources]]
name = "company-mirror"
artifact_url = "https://mirror.example/{tag}/{asset}"
checksum_url = "https://mirror.example/{tag}/SHA256SUMS.txt"
authorization_env = "GDIT_COMPANY_MIRROR_TOKEN"
```

条目配置（第三阶段起）见 §4.8 与 §9.5：`instances/<uuid>.toml` 保存引擎引用、
`[dotnet]` 策略与 `[env]` 条目环境，不写入 config.toml。

- 环境注入合并顺序（第四阶段起）：继承父环境 → 全局 `[environment]` → 平台小节
  `[environment.linux|darwin|windows]`（仅当前平台生效，覆盖全局同名键）→ 条目
  `[env]` → 派生变量。平台小节与全局相同的键名校验规则（非空、不含 `=` 与 NUL）。
- `display_driver = "auto"` 不强制 x11；Linux 按会话检测，macOS 与 Windows 不设置
  该变量（非 Linux 平台 `display_driver`/`input_method` 只接受 `auto`，显式其他值
  报配置错误）。
- `input_method = "auto"` 只在 Linux 检测到 fcitx 时注入相关变量。
- 选择 `~/.gdit/sdks/` 中的托管 SDK 时，shim 设置 `DOTNET_ROOT` 并把对应目录放到 PATH 前部。
- 自定义源也写在此文件；认证信息不输出到日志。

自定义源模板第一版只允许 `{version}`、`{tag}` 和 `{asset}` 三个占位符。URL 必须使用 HTTPS，
localhost fixture 测试除外。认证值通过 `authorization_env` 指向的环境变量读取，配置只保存变量名。
日志和错误输出只显示来源名、主机名和脱敏后的路径，不输出完整认证 URL、header 或环境变量值。

配置文件不存在时使用内置默认值。GodotHub 和 Godot 官方的 URL 规则已经实测确认（见 4.6）；
默认 source_order 为 `["godothub", "github"]`，AtomGit 规则未确认期间，用户显式配置 atomgit 时
该位置返回明确的配置错误，可通过 `custom_sources` 配置已知镜像或改写 source_order 绕过。
`disabled_sources` 列出被 `source ban` 禁用的来源，名字必须存在于 source_order 中。
配置文件存在但无法解析、schema 版本不支持、来源重名、模板非法或禁用名单包含未知来源时返回
配置错误，不能悄悄退回默认值。第一阶段只解析安装所需字段，未进入实现阶段的已知字段仍需被
保留，避免一次写回丢失用户配置。

`gdit source use` 与 `gdit source ban/unban` 的写回保留全部已知与未知字段（键顺序由写回编码
确定，注释不保留）；配置文件不存在时按内置默认创建后再调整。写回使用同目录临时文件 + rename +
父目录 fsync，与状态文件同一套原子写规则。

`state.toml` 记录已安装资产及来源、摘要算法、摘要值、安装时间等附加元数据。按 `engines/`、`sdks/`
重建时，
无法恢复的附加元数据可标记为未知，不影响版本使用。它不记录项目，不承担用户配置职责。

## 6. .NET SDK 策略

SDK 选择由**条目的 `[dotnet]` 策略**声明，而不是运行时自动探测决策：

- `managed`（默认）：条目必须声明 `version`（如 `8.0.410`），启动时使用
  `~/.gdit/sdks/<version>/` 的托管 SDK；缺失时报错并提示 `gdit sdk install <version>`。
  策略在**条目创建**时确定（`gdit install` 交互式或非交互式）：dotnet edition 默认
  `managed`，version 缺省为映射表推荐 major 的最新可用 patch，作为显式安装动作的依赖
  一并安装（apt 依赖语义，不在 use/shim 时隐式下载）；用户可显式选择 `system` 或填写
  具体 version。`gdit engine install` 只装资产，不创建条目、不装 SDK。
- `system`：使用系统 dotnet（`dotnet --list-sdks` 检测），忽略 version 字段；系统不满足
  最低要求时仅警告，不拦截（用户可能故意用更新的 SDK）；系统无 dotnet 时启动报错提示。
- `mono`：仅 Godot 3.x dotnet（mono）版条目使用，由 core 自动设置。3.x 的 C# 版依赖
  系统 **Mono 运行时**而非 .NET SDK：GoDoIt 只负责下载安装引擎资产，启动时不解析、
  不注入任何 SDK（无 DOTNET_ROOT、无 PATH 前缀），运行时由用户自理；条目创建时传
  SDK 选项直接报错。用户显式配置 `DOTNET_ROOT` 时原样继承，不影响 mono 条目。

Godot 版本到推荐 SDK 的映射由 core **写死**维护（静态表，不做实时获取——官方无机器可读
API，且该映射年级才变化一次，跟着 gdit 版本走即可）。表粒度只到 major.minor（如
`8.0`），具体 patch 由 `gdit sdk available` 取最新可用。表缺失的版本不报错，条目的
version 由用户显式填写。映射表同时作为警告级校验依据：声明版本低于推荐 major 时警告，
不硬拦。表内容与核对记录见 §9.5。

**下载通道枚举与推荐映射不同**：`gdit sdk available` / 交互式 SDK 选择从官方
`releases-index.json` 动态枚举可下载通道（跳过 EOL，保留 preview 与 support-phase 为
active/maintenance 的通道），并始终保留 `6.0`（Godot 4.0/4.1 需要，EOL 通道官方元数据
仍长期保留可下载，标注 EOL 提示）；索引不可用时降级到 core 内置静态保底列表
`{11.0, 10.0, 9.0, 8.0, 6.0}` 并发警告。交互式 SDK 选择为**两级菜单**：先选大版本通道
（preview 通道标注 `(Preview)`），再选具体 patch；`gdit sdk available` 按通道分组输出。
SDK 版本语法接受 .NET 预发布后缀（`11.0.100-preview.7.26381.103`、
`8.0.100-rc.2.23502.12`），推荐解析（`ResolveLatestPatch`）仍只取稳定版。2026-08 核对：
官方索引中 `11.0` preview/STS、`10.0` active/LTS、`9.0` maintenance/STS、`8.0`
maintenance/LTS、`6.0` EOL；Godot 4.5 官方要求 .NET 8 或更高（导出 Android 需 9+），
映射表 `4.2+ → 8.0` 仍为最低推荐，不变更。

托管 SDK 使用与引擎相同的下载、摘要校验和临时目录安装流程。SDK 下载只由用户显式发起的
CLI 或 GUI 操作触发。普通 `godot` 启动不询问、不访问网络。GoDoIt 不修改或卸载系统 dotnet。

项目 `global.json` 和 `.csproj` 只在 `suggest` 中参与建议，不改变普通 `godot` 的 SDK 选择。

## 7. 模块结构

```text
GoDoIt/
├── go.work
├── core/
│   ├── go.mod
│   ├── gdit.go                  # CLI/GUI 共用入口
│   └── internal/
│       ├── archive/
│       ├── config/
│       ├── dotnet/
│       ├── env/
│       ├── instance/            # 条目层：实例读写、引用扫描、GC 计算
│       ├── lock/
│       ├── platform/
│       ├── project/             # 只读 suggest 探测
│       ├── source/
│       └── store/
├── cli/
│   ├── go.mod
│   └── cmd/gdit/
├── gui/
│   ├── go.mod
│   └── frontend/
└── docs/
```

- `core` 实现全部业务能力并提供公共 Go API。
- CLI 只解析参数、调用 core、输出结果。
- GUI 只通过 Wails 调用同一个 core。
- `runtime.GOOS` 判断只出现在 `core/internal/platform/`。
- `project/` 只有只读分析能力，不能写项目或维护项目列表。

仓库按阶段创建真实需要的 package。第一阶段不创建 `dotnet/`、`env/`、`project/` 或 `gui/` 空目录，
这些路径在对应能力有可运行实现时再加入。`go.work` 也只引用当前实际存在的 module。

### 7.1 core 公共 API

core 对 CLI 和 GUI 暴露一个无界面语义的 `Manager`。截至第四阶段的累计公共面如下。

```go
type Options struct {
	RootDir    string
	HTTPClient *http.Client
	Progress   func(ProgressEvent)
	Sources    []Source // 固定 fixture 或宿主显式注入的来源；为空时读取 config.toml
	SDKProbe   func(context.Context) ([]SDKInfo, error) // 为空时用默认实现（PATH 上的 dotnet --list-sdks）
}

type InstallRequest struct {
	Version string `json:"version"`
	Edition string `json:"edition"`
	Source  string `json:"source,omitempty"` // 指定来源，非空时只使用该来源
}

// InstallEntryRequest 描述一次条目层安装（顶层 install 命令）。
type InstallEntryRequest struct {
	Name        string `json:"name"`                   // 条目显示名，必填，全仓库唯一，可中文
	Version     string `json:"version"`                // 引擎版本，必填
	Edition     string `json:"edition"`                // standard（默认）或 dotnet
	Source      string `json:"source,omitempty"`       // GUI 可固定使用一个已启用来源；空值按配置 fallback
	SDKStrategy string `json:"sdk_strategy,omitempty"` // 空=按 edition 默认（dotnet 为 managed）；managed | system
	SDKVersion  string `json:"sdk_version,omitempty"`  // managed 时；空=映射表推荐 major 的最新可用 patch
	SetCurrent  *bool  `json:"set_current,omitempty"`  // nil=自动；true=设为当前；false=不改变 current
	Template    bool   `json:"template,omitempty"`     // 是否同时安装并绑定匹配导出模板
}

type SourceInfo struct {
	Name     string `json:"name"`
	Kind     string `json:"kind"`     // builtin 或 custom
	Disabled bool   `json:"disabled"` // 是否被 source ban 禁用
}

type AvailableVersion struct {
	Version  string   `json:"version"`
	Editions []string `json:"editions"`
	Sources  []string `json:"sources"`
}

func ResolveRoot() (string, error) // 第四阶段起：GDIT_ROOT 非空即用（须绝对路径），否则平台默认路径
func New(options Options) (*Manager, error)

// 资产层（gdit engine 命名空间）
func (m *Manager) Install(ctx context.Context, request InstallRequest) (InstallResult, error)
func (m *Manager) Remove(ctx context.Context, id string) error // 被条目引用的资产拒绝删除
func (m *Manager) List(ctx context.Context) ([]InstalledVersion, error)
func (m *Manager) Sources(ctx context.Context) ([]SourceInfo, error)
func (m *Manager) SetDefaultSource(ctx context.Context, name string) error
func (m *Manager) SetSourceDisabled(ctx context.Context, name string, disabled bool) error
func (m *Manager) Available(ctx context.Context, sourceName string) ([]AvailableVersion, error)
func (m *Manager) Setup(ctx context.Context) error

// 条目层（顶层命令；name/instance 参数均为显示名，core 内部解析为 UUID 再寻址）
func (m *Manager) InstallEntry(ctx context.Context, request InstallEntryRequest) (InstallEntryResult, error)
func (m *Manager) RemoveInstance(ctx context.Context, name string) (RemoveInstanceResult, error)
func (m *Manager) Instances(ctx context.Context) ([]InstanceInfo, error)
func (m *Manager) Default(ctx context.Context) (InstanceInfo, error) // 未设置或悬空报错
func (m *Manager) SetDefault(ctx context.Context, name string) error
func (m *Manager) ResolveLaunch(ctx context.Context, instance string) (LaunchTarget, error) // 空=当前条目
func (m *Manager) Orphans(ctx context.Context) ([]OrphanAsset, error)
func (m *Manager) AutoRemove(ctx context.Context) (AutoRemoveResult, error)

// SDK 与环境
func (m *Manager) SDKs(ctx context.Context) ([]SDKInfo, error)
func (m *Manager) InstallSDK(ctx context.Context, version string) (SDKInstallResult, error)
func (m *Manager) RemoveSDK(ctx context.Context, version string) error // 被条目引用的 SDK 拒绝删除
func (m *Manager) EffectiveEnv(ctx context.Context, instance string) (EnvView, error)
func (m *Manager) SetEnvVar(ctx context.Context, instance, key, value string) error // 空=全局
func (m *Manager) UnsetEnvVar(ctx context.Context, instance, key string) error

// 第四阶段（doctor，见 §9.6）：纯只读诊断，不落盘、不拿锁，network 时探测来源可达性
func (m *Manager) Doctor(ctx context.Context, network bool) (DoctorReport, error)
```

最终字段名可以在编码前做一次 Go 命名校正，行为约束保持不变。`RootDir` 必填，`HTTPClient` 为空时使用
带明确超时的默认 client。core 不接收 stdout、stderr 或 GUI 回调对象。进度通过结构化 `ProgressEvent`
报告，最终结果和错误单独返回。CLI 把结果写 stdout、把进度和警告写 stderr，GUI 把同一事件转换成
Wails 事件。

公共结果结构同时带 `json` tag，供以后 CLI 机器输出和 Wails 边界复用。错误使用可由 `errors.Is/As`
识别的类型，至少区分输入、配置、不支持平台、已安装、来源全部不可用、完整性、取消和本地 I/O。
错误文本不能成为 CLI 分支条件。

`New` 只校验依赖和路径，不访问网络。`Install`、`List` 以及之后所有可能阻塞的操作接收
`context.Context`。下载、锁等待和外部进程都必须传播取消。

`Sources` 只读列出当前配置来源，不落盘不拿锁。`SetDefaultSource` 获取全局修改锁、锁内重读配置并
调整 source_order、原子写回 config.toml，被禁用的来源不能 use。`SetSourceDisabled` 同样在锁内
读写 `disabled_sources`。`Install` 指定被禁用的来源时报配置错误。`Available` 只读网络枚举，
不落盘不拿锁；sourceName 为空时按 source_order 合并全部启用且支持枚举的来源，非空时只用指定
来源（被禁用的来源报配置错误）。

第二阶段新增：`Default` 读取 current symlink，未设置或悬空时返回可识别错误；`SetDefault` 获取
全局修改锁、锁内校验目标已完整安装后原子替换 symlink；`Remove` 获取锁、删除版本目录并原子重建
state.toml，当前默认版本返回可识别错误；`Setup` 幂等创建或修复 shim，不修改 shell 配置或系统
PATH；`ResolveLaunch` 解析启动目标（版本 ID 为空时取当前默认），校验完整安装后返回引擎可执行
文件绝对路径，不做环境合并。启动子进程本身（execve 或 spawn）在 CLI 层，core 不持有子进程。

第三阶段新增（见 §9.5）：条目层 API——`InstallEntry` 编排完整条目安装：锁内安装引擎资产
（如缺）→ 按 `[dotnet]` 策略解析并安装 SDK 依赖（managed 时）→ 原子写条目文件 →
（SetCurrent 为 true，或 nil 且尚无 current 时）原子替换 current 指针；推荐 patch 解析
失败（网络不可用）时报错终止，不写条目。`RemoveInstance` 在同一次锁内操作中校验全部条目、
按排除目标后的引用关系预先计算孤儿，全部成功后才删除目标条目（当前条目拒绝删除），把一致
的孤儿结果返回给 CLI 输出。
`Instances` 列出条目；`Default` 返回当前条目的完整 `InstanceInfo`（未设置或悬空报错）；
`SetDefault` 锁内校验条目存在且引用完整安装后原子替换 current。`ResolveLaunch(ctx,
instance)` 空参数取当前条目：读条目 → 合并全局与条目环境、按 `[dotnet]` 策略解析 SDK，
返回结构增加 `Args`（注入的引擎参数，置于用户参数之前）与 `Env`（KEY=VALUE 注入列表）；
dotnet 版 managed 策略的托管 SDK 缺失时返回可识别的 `ErrNoCompatibleSDK`，交互决策留在
CLI 层。`Orphans` 扫描条目引用后返回无引用资产清单；`AutoRemove` 在全局修改锁内重新扫描
并删除复查时仍无引用的资产，再原子重建 state。任何条目扫描错误都使上述 GC 操作失败且
不发生删除。资产层 `Remove`/`RemoveSDK` 增加同样的失败关闭引用保护：被任何条目引用的
资产拒绝删除。`SDKs` 列出系统与托管 SDK；`InstallSDK` 受全局修改锁保护并复用
tmp/operation 下载管线；`EffectiveEnv` 返回指定条目（空为当前条目）的注入增量（全局 +
条目配置合并 + 派生变量，标注来源）；`SetEnvVar`/`UnsetEnvVar` 的 instance 为空表示全局
`[environment]`，非空表示条目 `[env]`，均在全局修改锁内读写并原子写回、保留未知字段。
`Options.SDKProbe` 默认实现走 PATH 上的 `dotnet --list-sdks`，测试注入固定结果，默认
测试不执行真实 dotnet。

全局修改锁只由一次业务操作的最外层获取一次。`InstallEntry` 持锁后调用不再取锁的
`installEngineLocked`、`installSDKLocked` 等 core 内部原语；公共 `Install`、`InstallSDK`
只是“获取锁 → 调用对应 locked 原语”的包装。禁止公共修改方法在持锁状态下相互调用，避免
同一进程对 `.lock` 嵌套加锁。锁等待和锁内网络 I/O 都必须响应 context 取消。

### 7.2 依赖方向

```text
cli ────────→ core public API
gui bridge ─→ core public API
                  │
                  ├── config
                  ├── instance ─→ store
                  ├── env ──────→ platform
                  ├── dotnet ───→ archive, platform, store
                  ├── source ───→ HTTP client
                  ├── store ────→ lock
                  ├── archive
                  └── platform
```

source 负责把来源协议转换为规范化资产，store 负责 gdit 根目录内的一致性，archive 只负责受限解压，
platform 负责目标识别、资产命名、引擎布局、权限和 POSIX 原子操作。store 不依赖具体来源，source 不写
版本目录。业务编排放在 core facade，不能沉入 CLI handler。

core 的直接依赖只包括 `BurntSushi/toml` 和提供 Linux/macOS 文件锁及同步调用的 `x/sys`。CLI 子命令
解析用 Go 标准库 `flag`；交互式 install 的选择器引入 `github.com/AlecAivazis/survey/v2`（2026-08-18
review 确认），仅限 CLI 层使用，survey 类型不得进入 core，交互逻辑不影响非交互路径。

## 8. 测试策略

- 安装成功、下载中断、摘要失败、多源 fallback 和状态清单重建。
- `default` 按条目名原子切换以及失败后旧 current 保持不变；未设置、链接悬空或目标不规范时报错。
- `remove` 删除条目后 list 不再出现并返回孤儿快照；当前条目拒绝删除；资产层 remove 删除
  引擎目录后 state 原子重建，删除运行中的资产不影响已启动的引擎进程。
- `setup` 创建 shim 但不修改 shell 配置或系统 PATH；幂等，重复执行不报错不重复创建。
- shim/`run`/`run -d` 使用 current，`run <name>` 使用指定条目；均透传参数与退出码，无 current
  时报错不启动，不读取项目目录，启动路径零网络、零落盘。
- 标准版与 .NET 版不会互相覆盖。
- 系统 SDK 与托管 SDK 的选择，以及显式配置 `DOTNET_ROOT` 时的用户接管语义。
- 条目引用扫描遇到坏条目时失败关闭；`autoremove` 在确认后锁内复查，引用关系变化时不误删。
- `InstallEntry` 组合引擎与 SDK 安装只获取一次全局修改锁，不发生嵌套加锁。
- `suggest` 不修改项目目录或当前条目。
- `source use` 重排 source_order、保留未知字段、无配置文件时创建、原子写回。
- `source ban/unban` 强禁用：被禁用的来源不参与自动 fallback 与默认枚举，显式指定或 use 时报错，
  unban 后恢复；写回保留未知字段。
- `engine install --source` 只用指定源（其余源不被调用）、指定源不可用不 fallback、未知来源名报错。
- `available` 稳定与预发布均收两/三段 tag、按当前平台资产判断 edition（3.x 的
  x11.64/osx.universal 命名支持 standard、mono 版不支持）、多源合并去重、单源失败不影响
  其余、自定义源不支持枚举时报配置错误；输出与交互选择按系列分组（`4.x`/`3.x`/`unstable`）。
- CLI 命令简写 `i/l/s/a/d/rm/r/st/e` 解析正确；无参数 `install` 在非 TTY 下返回用法
  错误，不进入交互流程。
- CLI 进度渲染：TTY 下 `\r` 行内重绘破折号线（已下载段品牌色 #3A73B0，即 Go/Godot/C# 三色
  平均；终端不支持 truecolor 时回退绿色；未下载段灰色；`NO_COLOR` 时无色），100ms 节流、
  完成/警告前清行；非 TTY 下按 8 MiB 打点。进度标签为「版本ID(来源)」（如
  `4.5.1-dotnet(godothub)`），批量安装时能区分正在下载的版本；`ProgressEvent` 的
  resolve/download/complete 事件必须携带版本 ID。survey 交互提示（install 选择、
  remove/autoremove 确认）一律渲染到 stderr，stdout 只输出结果；TTY/着色判定在 CLI 层参数化，
  不依赖测试环境的真实终端。
- Linux 与 macOS 的路径、资产名称和环境差异。

所有测试显式传入 `t.TempDir()` 下的 gdit 根目录。source 测试使用内存 `http.RoundTripper` 或
`httptest.Server` 和固定资产，分别返回成功、404、5xx、截断响应、错误摘要和超时。真实网络只用于
手工 smoke test，不进入默认测试。

store 测试要在原子 rename 前后注入失败，确认半成品只留在 `tmp/`、已经发布的目录可以重建 state，
并发安装同一版本只产生一个有效结果，安装开始时清理遗留的 `operation-*` 目录且不误删其他内容。
archive 测试包含绝对路径、`..`、symlink 逃逸和异常文件类型。
CLI 集成测试捕获 stdout 和 stderr，确认结果与进度没有串流。

macOS 行为分为两层。可在 Linux 运行的纯 target 映射使用固定输入测试，涉及 app bundle、权限、symlink
和子进程环境的验收必须在 Apple Silicon 实机完成，交叉编译结果只算构建检查。

## 9. 框架搭建与第一阶段

第一阶段是 Linux amd64 上可实际使用的 `install/list` 纵向切片。它包含必要框架，同时拒绝空 package、
TODO handler 和只为编译通过的 GUI 占位。

### 9.1 交付范围

1. 建立 `go.work`、`core` module 和 `cli` module，module 路径使用 review 后确认的仓库地址。
2. 实现版本、edition、platform target、路径和错误等值对象。
3. 实现配置读取、来源顺序、自定义来源模板和敏感信息脱敏。
4. 实现 source provider、固定 fixture、下载取消、错误分类和 fallback。
5. 实现安全解压、完整安装标记、全局修改锁、原子发布、state 原子写入与重建。
6. 实现 core `Install` 与 `List`，随后接入薄 CLI。
7. 完成单元测试、core 集成测试和 CLI 集成测试，再做一次不替代 fixture 的 Linux amd64 手工安装。

第一阶段不包含 remove、default、shim、环境注入、.NET SDK 选择、doctor、suggest、模板管理和 GUI。
`dotnet` edition 的引擎资产可以安装和列出，但兼容 SDK 检测及启动要到“环境与 .NET”阶段完成；CLI
必须在输出中避免暗示这一阶段已经能完整启动 C# 项目。

### 9.2 验收标准

- `gdit install --edition standard 4.5.2` 可以从配置的来源安装，重复执行返回稳定的已安装结果。
- `gdit install --edition dotnet 4.5.2` 安装到不同版本 ID，不覆盖标准版。
- 第一来源不可用时按顺序 fallback，任一来源发生摘要不匹配时立即停止。
- 下载取消、进程中断和解压失败不会让 `list` 看见半成品。
- 删除或损坏 `state.toml` 后，`list` 能从有效版本目录重建；无效目录只由 doctor 的后续实现报告。
- 并发执行安装时，版本目录、安装标记和 state 始终一致。
- 默认测试不访问开发者真实主目录和公网。
- `go test ./core/... ./cli/...`、`go vet ./core/... ./cli/...` 和格式检查全部通过。

### 9.3 后续顺序

来源管理（4.7：source list/use、install --source、available）作为第一阶段的扩展实现，
不改变以下阶段顺序。

```text
第一阶段  install/list（含来源管理扩展）
第二阶段  remove + default/setup/shim + run
第三阶段  instances 条目层 + 资产层命名空间 + 环境注入 + .NET SDK + 资产 GC
第四阶段  doctor + 平台适配层拆分（Linux/macOS/Windows）
第五阶段  suggest + 导出模板
第六阶段  Wails GUI
```

第四阶段同时交付 doctor（FR-08，见 §9.6）与平台适配层拆分（§4.9）：doctor 的检查项
按平台差异化与适配层能力化是同一工作的两面；Windows x86_64 支持随适配层拆分一并
落地并在 Windows 实机/CI runner 验收。第五、六阶段直接建立在三平台适配层之上。

remove 放到第二阶段，与 current 的引用约束一起实现，避免第一阶段先产生一套随后要改的删除语义。

### 9.4 第二阶段：remove + default/setup/shim + run

第二阶段在 Linux amd64 上交付 remove、default、setup/shim 和 run。环境注入（FR-04）与 .NET SDK
选择（FR-05）仍属于第三阶段；本阶段 shim/run 只做「读 current → 解析引擎 → 启动」，dotnet 版
启动缺 SDK 时由引擎自行报错。

```text
gdit default                   # 显示当前默认版本；未设置或悬空时报错
gdit default <版本>            # 设为全局默认：原子替换 current symlink
gdit remove <版本>             # 卸载；当前默认版本拒绝删除
gdit setup                     # 创建/修复 ~/.gdit/bin/godot shim
gdit run -d [-- 参数]          # 启动默认版本（等价于裸 godot）
gdit run <版本> [-- 参数]      # 显式启动指定版本，不改变默认
gdit run                       # TTY 下交互选择已安装版本后启动
```

命令简写：`default`→`d`、`run`→`r`、`remove`→`rm`、`setup`→`st`，与第一阶段 `i/l/s/a` 不冲突。
版本输入统一支持 `m` 前缀（见 §3）：`gdit run m4.6.2` 等价于 `gdit run --edition dotnet 4.6.2`，
install/remove/default 同样生效。

- `default` 语义：设置全局默认 = 拿全局修改锁、锁内校验目标已完整安装、原子替换 current symlink，
  失败保留旧链接。无参数时显示当前默认，未设置或悬空时报错并提示。default 只接受已完整安装的
  版本，不触发隐式下载，对所有目录一致。
- `remove` 语义：删除不可逆，默认需要确认——TTY 下 survey 确认（默认否），非 TTY 下必须显式
  `-y`/`--yes` 跳过确认，避免脚本卡死或误删。确认后拿锁删除版本目录，随后重扫并原子重建
  state.toml（复用第一阶段重建流程）。当前默认版本不能直接删除，必须先 `default` 到其他版本。
  删除运行中版本的目录不终止已启动的引擎进程（POSIX inode 语义），gdit 只保证 current 不悬空；
  `run <版本>` 指定的是已不存在的版本时报错。
- `setup` 语义：幂等创建/修复 `~/.gdit/bin/godot`，它是指向 gdit 自身的 symlink；已存在且指向
  正确的 gdit 时什么都不做，指向错误或缺失时修复。bin 目录不在 PATH 时输出提示。setup 不修改
  shell 配置或系统 PATH。
- `run` 语义：
  - `-d`：解析 current → 启动引擎，等价于裸 `godot`。
  - `<版本>`：校验已完整安装后启动，不改变 current。
  - 无参数：仅 TTY 可用，survey 列出已安装版本（标记当前默认）选择后启动；非 TTY 返回用法错误。
  - `--` 之后的参数、stdin/stdout/stderr 和退出码原样透传。
- shim 路径：gdit 以 `godot` 名称启动（argv[0] basename 判断）时进入 shim 分支，解析后
  execve 替换自身进程，信号、stdio 和退出码天然透传。`run` 的指定版本与交互路径由 CLI 层
  spawn 启动。core 只提供 `ResolveLaunch` 解析启动目标，不启动子进程。
- 交互选择器复用 survey/v2，只出现在 CLI 层；交互逻辑不影响 `-d` 与显式版本路径。
- `run` 取代原 FR-11 的 `exec` 设计：本阶段即交付 headless 运行与参数透传能力，PRD 中
  FR-11 并入 FR-02。

### 9.5 第三阶段：instances 条目层 + 环境注入 + .NET SDK + 资产 GC

第三阶段在 Linux amd64 上交付 §4.8 的 instances 条目层、FR-04 启动环境注入、FR-05 .NET SDK
管理与 apt 式资产 GC。日常命令入口整体条目化：`gdit install` 创建条目（交互式确认条目名与
各项配置，顺带安装引擎与 SDK 依赖），`gdit remove <条目名>` 删除条目，`gdit run`/`gdit
default` 只接受条目名，版本输入直接报用法错误；原资产层 install/remove/list 降级封装为
`gdit engine` 命名空间。SDK 按条目策略（managed 默认 / system 备选）选择；资产 GC 采用
apt 语义（删除条目后提示孤儿 + `gdit autoremove`）。macOS 只实现映射层与 SDK 布局，行为
验收在 Apple Silicon 实机完成（交叉编译不算完成）。

```text
# 条目层（日常入口）
gdit install                          # 交互式条目安装：显示名、edition、版本、SDK 策略、设为当前
gdit new                              # 与 gdit install 等价的别名（无独立简写）
gdit install <name> --version <版本> [--edition standard|dotnet]
                                      # 非交互条目安装
                                      #   [--sdk managed|system] [--sdk-version <版本>]
                                      #   [--current|--no-current]
gdit remove <name>                    # 删除条目（当前条目拒绝删除）；资产变孤儿并提示
gdit list                             # 列出条目：名称、引擎、edition、SDK、current 标记
gdit default                          # 显示当前条目（名称 + 引擎 + edition + SDK 策略）
gdit default <name>                   # 切换当前条目（原子替换 current）
gdit run [<name>] [-- 参数]           # 启动当前条目（无参数）或指定条目；-d 为当前条目别名；无参数 + TTY 且多条目时交互选择
gdit env [--instance <name>]          # 显示指定条目（默认当前条目）的注入环境
gdit env set <KEY=VALUE> [--instance <name>]  # 写入全局或条目环境
gdit env unset <KEY> [--instance <name>]      # 删除全局或条目变量
gdit sdk                              # 列出系统与托管 SDK
gdit sdk available                    # 探测官方源可安装的 SDK 版本（按通道分组输出）
gdit sdk install [<版本>]             # 安装托管 SDK；无参数 + TTY 时交互式两级选择（先大版本通道再具体 patch，枚举失败降级文本输入），非 TTY 无参数报用法错误
gdit sdk remove [-y|--yes] <版本>     # 卸载托管 SDK（被条目引用的拒绝删除）
gdit autoremove [-y|--yes]            # 删除无条目引用的引擎/SDK 资产（第五阶段扩展到模板）

# engine 命名空间（资产层，原命令降级封装）
gdit engine                           # 列出已安装引擎资产（无参数 = list；与 gdit sdk 对称）
gdit engine install [--edition standard|dotnet] [--source <name>] <版本>...   # 原 install
gdit engine remove [-y|--yes] <版本>  # 原 remove；被条目引用的资产拒绝删除
gdit engine list                      # 原 list；列资产（含引用状态）
```

命令简写：顶层 `install`→`i`、`remove`→`rm`、`list`→`l`、`default`→`d`、`run`→`r`、
`env`→`e`、`source`→`s`、`available`→`a`、`setup`→`st`（沿袭 §4.7/§9.4）；`sdk`、
`autoremove` 与 engine 子命令不设简写。`source`/`available`/`setup` 留在顶层不变
（配置与查询类，与条目无关）。`--instance`、`run`、`default` 接受符合显示名
规则（见 §4.8）且已存在的条目显示名。

#### 条目层安装流程

`gdit install`（`gdit new` 为等价别名）的交互流程（仅 TTY；非 TTY 下无参数调用返回
用法错误）：

1. 条目显示名（默认 `default`；可中文；校验唯一且符合显示名规则）。
2. edition（standard/dotnet）。
3. 版本（`available` 枚举结果过滤该 edition 后**两级选择**：先选系列——稳定版按 major
   （`4.x`/`3.x`）与预发布 `unstable` 组，再选具体版本；枚举期间 TTY 显示等待动画，
   枚举失败降级为文本输入）。
4. SDK（仅 dotnet）：Godot 3.x 自动为 `mono` 策略并提示（使用系统 Mono 运行时，无
   SDK 概念）；4.x+ 时策略（managed 默认 / system），managed 时版本三选一——推荐版本
   （默认，映射表推荐 major 的最新可用 patch）/ 从 `sdk available` 枚举结果**两级选择**
   （先大版本通道再具体 patch，preview 通道标注 Preview）/ 手动输入精确版本（枚举失败时
   推荐与列表合并为文本输入）。
5. 设为当前？默认值：当前未设置时为「是」，已设置时为「否」。
6. 执行：安装引擎资产（多源 fallback，进度复用渲染器）→ managed 时解析并安装 SDK
   依赖（apt 语义）→ 原子写条目文件 → 设为当前时原子替换 current 指针。

非交互形式 `gdit install <name> --version <版本>`：`--edition` 默认 standard；
dotnet 时 `--sdk` 默认 managed；managed 且无 `--sdk-version` 时解析映射表推荐 major 的
最新 patch，**解析失败（网络不可用）时报错终止，不写条目**，提示用 `--sdk-version` 显式
指定。`--current` 明确设为当前，`--no-current` 明确不改变 current，两者互斥；均未给出时
采用自动规则（当前未设置时设为当前，否则不改变）。`gdit engine install` 只装资产，不创建
条目、不装 SDK。

#### 环境模型

- 合并顺序（第三阶段）：继承 `os.Environ()` → 应用全局 `[environment]` 用户变量 → 应用
  当前条目 `[env]` 变量（覆盖全局）→ 最后应用派生变量（fcitx 变量、dotnet 版选择托管
  SDK 时的 `DOTNET_ROOT` 与 PATH 前缀）。显示驱动以引擎参数注入，不在环境表里。
  第四阶段起在全局与条目之间插入平台小节 `[environment.<os>]`（仅当前平台生效，覆盖
  全局同名键），见 §4.9「环境注入的平台化」与 §5。
- 已知键 `display_driver`（auto/x11/wayland）与 `input_method`（auto/fcitx/off）从合并后的
  环境表中读取，因此条目 `[env]` 可以覆盖全局值；其余键按普通变量注入。
- 用户在全局 `[environment]`、平台小节或条目 `[env]` 显式配置 `DOTNET_ROOT` 时视为接管
  SDK 定位：跳过 SDK 选择与注入，不报“无兼容 SDK”错误，fcitx/display 仍正常处理。
  仅从父进程继承的 `DOTNET_ROOT` 不算显式接管，仍按条目策略选择并由派生值覆盖。
- 注入只作用于目标子进程（execve/spawn 的 env 参数），不修改 shell、系统 PATH 或系统 dotnet。
- macOS 与 Windows 不注入 Linux 专用变量（display_driver/input_method 在非 Linux 平台
  只接受 auto）。

#### 显示驱动与输入法（Linux）

- 显示驱动：Godot 只通过 `--display-driver <x11|wayland>` 参数选择显示服务器（4.5.2 与 master
  源码一致；官方 4.7 文档页误写为 `--display-server`，以源码为准），无环境变量途径。
  `display_driver = "auto"`（默认）不注入任何参数，由 Godot 原生默认处理：默认 X11，X11
  不可用时自动回退 Wayland（官方文档确认，回退逻辑还检查 `WAYLAND_DISPLAY`）。显式
  `x11`/`wayland` 时把 `--display-driver <值>` 作为引擎参数注入，置于用户参数之前；用户自带
  同名参数时用户参数生效（Godot 顺序解析，后者覆盖）。
- fcitx：`input_method = "auto"` 只在 Linux 检测到 fcitx 时注入；检测规则为环境变量
  `XMODIFIERS` 已含 `fcitx`，或系统中存在 fcitx/fcitx5 进程。注入变量
  `XMODIFIERS=@im=fcitx`、`GTK_IM_MODULE=fcitx`、`QT_IM_MODULE=fcitx`，逐个只在缺失时注入
  （不覆盖用户已有值）。`"fcitx"` 强制注入（仍不覆盖已有值），`"off"` 不注入。macOS 不注入。

#### .NET SDK 探测与策略

- 系统 SDK：PATH 上找到 `dotnet` 时执行 `dotnet --list-sdks` 解析输出（格式
  `8.0.404 [/usr/lib/dotnet/sdk]`）；dotnet 不存在时系统 SDK 为空列表，命令执行失败按空列表
  处理并走进度警告。默认实现不访问网络。
- 托管 SDK：`~/.gdit/sdks/<version>/` 下目录名合法、install.toml 可解析且 dotnet 可执行
  文件存在才计入。
- SDK 选择只按条目的 `[dotnet]` 策略（见 §6）：
  - `managed`（默认）：使用 `sdks/<version>/` 托管 SDK，缺失时返回
    `ErrNoCompatibleSDK` 并提示 `gdit sdk install <version>`；不询问、不自动下载。
  - `system`：使用系统 SDK 中版本号最高的；系统无 dotnet 时同样报错提示。
  - 选中托管 SDK 时注入 `DOTNET_ROOT=<托管目录>` 并把该目录置于 PATH 前部。
- 推荐映射表（core 静态常量，写死不实时获取；粒度 major.minor，patch 由
  `gdit sdk available` 取最新）：

  ```text
  Godot 4.0 ~ 4.1  → .NET 6.0
  Godot 4.2+       → .NET 8.0
  ```

  策略在**条目创建**时确定（见上文条目层安装流程）：dotnet edition 默认 `managed`，version
  缺省为对应 major.minor 的最新可用 patch，作为显式安装动作的依赖一并安装（apt 语义）；
  用户可显式选 `system` 或填写具体 version。表缺失的版本不报错，条目的 version 由用户显式
  填写。兼容判定 = 声明版本 major 不低于映射最低 major；低于时警告不拦截。

#### 托管 SDK 安装管线

下载源：元数据始终 .NET 官方单一来源；**资产下载内置镜像 fallback**（华为云优先，
官方兜底；2026-08 实测华为云镜像资产路径与官方一致且国内可达，USTC/阿里/腾讯/tuna
均无 dotnet 镜像）：

```text
dotnetcli 通道索引 https://dotnetcli.azureedge.net/dotnet/release-metadata/releases-index.json
                  （channel-version / support-phase / release-type / latest-sdk，机器可读）
dotnetcli 元数据  https://dotnetcli.azureedge.net/dotnet/release-metadata/{major}.{minor}/releases.json
        资产      https://builds.dotnet.microsoft.com/dotnet/Sdk/{version}/dotnet-sdk-{version}-{rid}.tar.gz
                  （即 releases.json 中 sdk.files[].url；rid 如 linux-x64、osx-arm64）
镜像资产      https://mirrors.huaweicloud.com/dotnet/Sdk/{version}/dotnet-sdk-{version}-{rid}.tar.gz
                  （由官方 URL 仅替换 host 推导，见 dotnet.MirrorURL）
        摘要      releases.json 中 files[].hash（SHA-512，128 hex，与 dotnet-install.sh 校验一致）
```

2026-08-18 实测：`builds.dotnet.microsoft.com` 与 `dotnetcli.azureedge.net` 均返回 HTTP 200；
releases.json 为稳定 JSON（`releases[].sdk.version`、`sdk.files[].name/hash/url`，hash 为
128 hex）。2026-08 实测通道索引：`11.0` preview/STS、`10.0` active/LTS、`9.0`
maintenance/STS、`8.0` maintenance/LTS、`6.0` EOL；preview 通道 SDK 版本带 `-preview`
后缀（如 `11.0.100-preview.7.26381.103`），版本校验接受该形态，推荐解析仍只取稳定版。
注意元数据中资产名不含版本号（如 `dotnet-sdk-linux-x64.tar.gz`），下载 URL 以
files[].url 为准。

流程与引擎安装同一套：解析精确版本（只接受三段精确版本号）→ 查 releases.json 定位 sdk 条目与
资产 → 下载到 `~/.gdit/tmp/<operation-id>/`（镜像优先、官方兜底；镜像同步延迟导致
404/网络失败时自动降级官方，下载成功的来源记入 install.toml 的 source 字段）→ 按
SHA-512 校验 → 安全解压（tar.gz，约束与 zip 一致：拒绝绝对路径、越界 `..`、设备文件和
逃逸目标目录的 symlink；顶层 `./` 目录条目安全跳过）→ 校验 dotnet 可执行文件并生成
install.toml（记录版本、平台、摘要、launcher=dotnet）→ 原子发布到 `~/.gdit/sdks/<version>/`
→ 原子更新 state.toml（与引擎同规则，重建范围含 `sdks/`）→ 清理 operation 目录。受全局
修改锁保护，锁内先清理遗留 operation 目录。进度事件复用 ProgressEvent
（resolve/download/complete，标签为 `<版本>(sdk)`）。摘要不匹配立即终止，不 fallback
（hash 来自官方元数据，与下载源无关，镜像只负责传输）。

`gdit sdk install` 只接受精确版本（与引擎输入一致）。SDK 下载只由显式操作触发：用户执行
`gdit sdk install <版本>`，或条目安装流程（`gdit install`，见上文）中按策略解析出的托管
SDK 作为依赖一并安装（apt 语义）。普通 `godot`（shim）启动永远不询问、不下载，只报错
并提示 `gdit sdk install <版本>`。不引入 auto_install 交互下载：条目策略已把「要哪个 SDK」
显式固化，启动路径保持零网络。

`gdit sdk remove` 删除托管 SDK 目录，确认语义与引擎 remove 一致（TTY 下 survey 确认默认否，
非 TTY 必须 `-y`/`--yes`）；被条目 `[dotnet].managed` 引用的 SDK 拒绝删除，需先改条目。
删除运行中 SDK 的目录不终止已启动的进程（POSIX inode 语义）。删除后同样参与孤儿提示
（SDK 不被任何条目引用时在 autoremove 候选列表）。

##### core API 扩展

`ResolveLaunch` 第三阶段起合并环境并解析 SDK：

- `LaunchTarget` 增加 `Args []string`（注入的引擎参数，置于用户参数之前）与
  `Env []string`（KEY=VALUE 注入列表）；dotnet 版无兼容 SDK 时返回可识别的
  `ErrNoCompatibleSDK`，交互决策留在 CLI 层。core 仍不启动子进程。

新增接口：

```go
type SDKInfo struct {
	Version string `json:"version"`
	Kind    string `json:"kind"` // system 或 managed
	Path    string `json:"path"`
}

type InstanceInfo struct {
	ID          string `json:"id"`            // 存储标识符（UUID v4），与条目文件名一致
	Name        string `json:"name"`          // 显示名，用户寻址用
	Engine      string `json:"engine"`        // 引用的引擎资产 ID，如 4.5.2-dotnet
	Edition     string `json:"edition"`       // standard 或 dotnet
	SDKStrategy string `json:"sdk_strategy"`  // managed、system；标准版为空
	SDK         string `json:"sdk"`           // managed 策略引用的 SDK 版本；其余为空
	Current     bool   `json:"current"`       // 是否是 current 指向的条目
}

type OrphanAsset struct {
	Kind string `json:"kind"` // engine 或 sdk
	ID   string `json:"id"`   // 引擎资产 ID 或 SDK 版本
	Size int64  `json:"size"`
	Path string `json:"path"`
}

// AssetChange 描述一次业务操作实际新安装的资产。
type AssetChange struct {
	Kind string `json:"kind"` // engine 或 sdk
	ID   string `json:"id"`
}

// InstallEntryResult 描述条目安装完成后的条目与资产变化。
type InstallEntryResult struct {
	Instance             InstanceInfo  `json:"instance"`
	Installed            []AssetChange `json:"installed"`
	StateRebuildRequired bool          `json:"state_rebuild_required"`
}

// RemoveInstanceResult 描述已删除条目及删除后的一致孤儿快照。
type RemoveInstanceResult struct {
	Instance InstanceInfo  `json:"instance"`
	Orphans  []OrphanAsset `json:"orphans"`
}

// SDKInstallResult 描述托管 SDK 安装结果。
type SDKInstallResult struct {
	SDK                  SDKInfo `json:"sdk"`
	StateRebuildRequired bool    `json:"state_rebuild_required"`
}

// AutoRemoveResult 描述复查后实际删除的孤儿资产。
type AutoRemoveResult struct {
	Removed              []OrphanAsset `json:"removed"`
	StateRebuildRequired bool          `json:"state_rebuild_required"`
}

type EnvVar struct {
	Key    string `json:"key"`
	Value  string `json:"value"`
	Origin string `json:"origin"` // global、platform、instance 或 derived（第四阶段起含平台小节）
}

type EnvView struct {
	Vars []EnvVar `json:"vars"` // 注入增量：用户配置合并结果 + 派生变量
	Args []string `json:"args"` // 注入的引擎参数，如 ["--display-driver", "wayland"]
}

func (m *Manager) InstallEntry(ctx context.Context, request InstallEntryRequest) (InstallEntryResult, error)
func (m *Manager) RemoveInstance(ctx context.Context, name string) (RemoveInstanceResult, error)
func (m *Manager) Instances(ctx context.Context) ([]InstanceInfo, error)
func (m *Manager) Orphans(ctx context.Context) ([]OrphanAsset, error)
func (m *Manager) AutoRemove(ctx context.Context) (AutoRemoveResult, error)
func (m *Manager) SDKs(ctx context.Context) ([]SDKInfo, error)
func (m *Manager) AvailableSDKs(ctx context.Context) ([]string, error)
func (m *Manager) InstallSDK(ctx context.Context, version string) (SDKInstallResult, error)
func (m *Manager) RemoveSDK(ctx context.Context, version string) error
func (m *Manager) EffectiveEnv(ctx context.Context, instance string) (EnvView, error)
func (m *Manager) SetEnvVar(ctx context.Context, instance, key, value string) error
func (m *Manager) UnsetEnvVar(ctx context.Context, instance, key string) error
```

- `InstallEntry` 编排完整条目安装（见上文条目层安装流程），CLI 只做参数/交互转换；
  `RemoveInstance` 锁内删除条目文件并返回同一临界区内计算的孤儿提示数据（由 CLI 输出）。
- `InstallEntry` 不承诺跨引擎目录、SDK 目录、条目文件和 current 的多路径事务。每个资产只在
  校验完成后原子发布；若后续 SDK、条目或 current 步骤失败，已经完整发布的资产和条目不回滚，
  结果按实际磁盘状态报告，未被条目引用的完整资产可由 `autoremove` 清理。任何失败都不能留下
  被识别为完整资产的半成品，CLI 不得把已发布内容误报为不存在。
- `EffectiveEnv` 的 instance 为空时取当前条目；`SetEnvVar`/`UnsetEnvVar` 的 instance 为空
  表示全局 `[environment]`，非空表示条目 `[env]`，均在全局修改锁内读写并原子写回，
  保留未知字段。
- `AutoRemove` 由 CLI 层完成 TTY 交互后调用，锁内重新扫描并逐个删除仍为孤儿的资产，再原子
  重建 state；返回实际删除结果，允许它与确认前 `Orphans` 展示的快照不同。
- 新 package：`core/internal/instance`（条目读写、引用扫描、孤儿计算）、
  `core/internal/env`（合并与派生变量）、`core/internal/dotnet`（系统/托管探测、
  推荐映射、SDK 官方源、托管布局与原子发布）；`core/internal/archive` 增加 tar.gz 安全解压；
  `core/internal/platform` 增加 fcitx 检测、显示驱动解析（Linux）与 SDK 资产名（darwin arm64）。
  core 的直接依赖仍只有 `BurntSushi/toml` 和 `x/sys`，无新增第三方依赖。

#### 配置扩展

```toml
# config.toml
[environment]                 # 全局环境；display_driver/input_method 为已知键
display_driver = "auto"       # auto | x11 | wayland
input_method = "auto"         # auto | fcitx | off
EXAMPLE_VARIABLE = "value"    # 任意用户变量
```

```toml
# instances/<uuid>.toml
schema_version = 2
id = "3f2a9c1e-8b4d-4f2a-9c1e-8b4df2a9c1e8"  # 存储标识符（UUID v4），与文件名一致
name = "default"          # 显示名，唯一，可中文

[engine]
version = "4.5.2"
edition = "dotnet"

[dotnet]
strategy = "managed"          # managed | system
version = "8.0.410"           # managed 必填（缺失报配置错误），system 时忽略

[env]
EXAMPLE_VARIABLE = "value"    # 条目环境覆盖/新增（全局之下）
```

- 校验：环境键名非空、不含 `=` 与 NUL，环境值不含 NUL；已知键取值非法时报配置错误；
  `display_driver`、`input_method` 是 gdit 控制键，解析后不作为同名环境变量传给引擎；`[dotnet]` 仅
  dotnet edition 条目有效——standard 条目出现 `[dotnet]` 表视为配置错误，
  `strategy = "managed"` 且 version 缺失或非法时报配置错误，`"system"` 时忽略 version；
  条目 `[engine]` 引用必须存在对应完整安装（managed 声明的 SDK 未安装属于启动错误，
  不属于配置错误）。条目 `id` 必须是与文件名一致的合法 UUID v4；`name`（显示名）非空、
  字符集合法且全仓库唯一。第三阶段条目文件的 schema_version 为 2（结构含 `id` 字段；
  开发期无存量用户，不实现 schema 1 迁移，schema 1 文件按非法条目失败关闭处理）。
- 写回保留全部已知与未知字段（复用现有 map 合并机制），使用同目录临时文件 + rename +
  父目录 fsync 的原子写规则。`gdit env set/unset` 与条目创建走同一套原子写；条目文件
  写回同样保留未知字段。
- 兼容边界：第三阶段不读取、不迁移 `versions/`、`dotnet/`、指向版本目录的旧 current 或
  `[versions.*.environment]`。开发与验收使用全新的 gdit 根目录；已有第二阶段测试数据需要由
  使用者自行移走后重新安装，gdit 不自动删除或改写旧数据。

##### 交付范围

1. instance 包：条目读写与校验（UUID 标识 + 显示名规则 + 唯一性）、引用扫描与孤儿计算、
   条目层 install/remove 编排（`InstallEntry`/`RemoveInstance`）、`gdit autoremove`
   （锁内删除 + state 重建）。
2. CLI 重构：原 install/remove/list 降级封装为 `gdit engine` 命名空间；顶层 install
   改为条目层（交互式 + 非交互 flags）；顶层 remove/list/default/run 条目化；
   `gdit env`（e）、`gdit sdk`、`gdit autoremove` 命令；remove 孤儿提示。
3. config 扩展与原子写回（全局环境、条目 `[env]`、`[dotnet]` 策略）及校验。
4. env 合并引擎与派生变量（fcitx、显示驱动解析、SDK 注入）。
5. platform 适配：fcitx 检测、显示驱动解析（Linux）；SDK 资产名（darwin arm64）。
6. dotnet 包：系统/托管探测、推荐映射表（写死）、SDK 官方源（releases.json + sha512）、
   tar.gz 安全解压、install.toml 与原子发布（含 state 更新）、remove（被条目引用时拒绝）。
7. shim/run 注入 `Args` + `Env`（execve/spawn），用户参数透传语义不变；run 条目化。
8. 测试：单元、core 集成、CLI 集成；macOS 行为在 Apple Silicon 实机验证。

##### 验收标准

- 全局与条目环境变量注入子进程（用打印 env 的假引擎验证），条目覆盖全局，父 shell
  环境不变。
- `display_driver = "auto"` 不注入参数；显式配置时注入 `--display-driver` 且位于用户参数前，
  用户同名参数优先。
- fcitx 仅在检测到或显式启用时注入，已有值不覆盖；macOS 不注入 Linux 变量。
- dotnet 版启动按条目策略：managed 使用 `sdks/<version>/` 托管 SDK，缺失时 shim 报错提示
  且不询问不下载；system 使用系统 SDK 最高版本。选中托管 SDK 时 `DOTNET_ROOT` 与 PATH 前缀
  只作用于子进程，不修改系统 dotnet。
- `gdit install` 交互流程完整：显示名/edition/版本/SDK 策略/设为当前逐一确认后执行；
  非 TTY 无参数 install 报用法错误。非交互形式缺省规则正确（dotnet→managed→推荐 patch）；
  `--current`、`--no-current` 与未指定时的自动规则均覆盖。
- `gdit sdk install` 无参数 + TTY 时枚举 `sdk available` 后**两级选择**（大版本通道 → 具体
  patch）再安装；枚举失败降级为文本输入；非 TTY 无参数报用法错误（不卡死脚本）。条目安装
  交互式的 managed SDK 版本支持推荐/两级列表选择/手动输入三种方式，选出的版本原样进入条目。
- dotnet 条目安装：推荐 SDK 作为依赖一并安装；`--sdk system`/显式 `--sdk-version` 尊重
  用户选择；推荐 patch 解析失败时报错终止、不写条目。
- 映射表写死：表外版本不报错，由用户显式填 version；低于推荐 major 时警告不拦截。
- 顶层命令条目化：命令只接受条目显示名（可中文，字符集见 §4.8），按显示名精确匹配；
  显示名与版本形态/引擎资产 ID 分属不同命名空间，不再互相排斥（`gdit run 4.5.2` 只是查找
  名为 `4.5.2` 的条目）；条目不存在时报 `instance not found`；engine 命名空间保留原版本
  语法与多版本批量安装。
- 条目标识与显示名分离：条目文件名是 UUID v4（内部可见，CLI 文本输出不显示）；显示名
  创建时锁内查重，重名报输入错误；中文显示名在 macOS 上创建、寻址、启动全流程可用。
- 引用保护：engine remove / sdk remove 被条目引用的资产拒绝删除；删除条目后孤儿提示只
  列出无条目引用的引擎/SDK（含空间大小）；当前条目拒绝删除；`autoremove` 列出并确认后
  在锁内复查并删除，被条目引用的资产永不删除，删除后 state 原子重建。任意坏条目使引用
  扫描失败关闭，engine remove / sdk remove / autoremove 均不删除资产。
- sdk install 固定 fixture 全流程：sha512 失败不发布、半成品不进 `sdks/`、tar.gz 拒绝
  绝对路径/越界 `..`/symlink 逃逸；安装后 state 与目录一致。
- 第三阶段测试只使用全新根目录，不读取或改写第二阶段布局；shim/run 启动路径零网络、零落盘。
- env set/unset 原子写回、保留未知字段、受全局修改锁保护，重复 set 幂等。
- `InstallEntry` 只获取一次全局修改锁，组合安装引擎与 SDK 时不发生嵌套加锁。
- 默认测试不访问真实网络、不执行真实 dotnet（注入 SDKProbe 与固定 fixture）；
  `go test ./core/... ./cli/...`、`go vet` 与格式检查全部通过。

#### 实现决策（已 review）

1. `gdit engine` 的范围：install/remove/list 进 engine；source/available/setup 留在顶层
   （配置与查询类，与条目无关）。（建议：接受。）
2. `run` 无参数 = 当前条目（去掉原 TTY 交互选择）；`-d` 保留为当前条目的显式别名。
   （建议：接受。）
3. 条目安装后设为当前：交互询问（默认当前未设置时为「是」）；非交互支持互斥的
   `--current`/`--no-current`，均未指定时使用相同自动规则。`gdit engine install` 不改变
   current。（建议：接受。）
4. 条目 `[engine]` 引用创建后不可变（换引擎版本 = 建新条目）；`[dotnet]`/`[env]` 可改。
   （建议：接受。）
5. 删除当前条目拒绝（必须先 default 到其他条目），保证 current 不悬空。（建议：接受。）
6. 交互 `install` 不含环境变量配置（用 `gdit env` 配置）。（建议：接受。）
7. 非交互 `install` 推荐 patch 解析失败（网络不可用）时报错终止、不写条目。
   （建议：接受。）
8. SDK 下载只支持 .NET 官方单源，本阶段不做 fallback 与自定义 SDK 源。（建议：接受。）
9. 用户显式配置 `DOTNET_ROOT` 时跳过 SDK 选择，视为用户接管。（建议：接受。）
10. `display_driver = "auto"` 不注入参数，依赖 Godot 原生默认与自动回退。（建议：接受。）
11. 第三阶段是破坏性布局切换，不实现第二阶段数据迁移或兼容读取；开发和验收使用全新根目录，
    gdit 不自动删除旧数据。（建议：接受。）
12. 条目标识与显示名分离：条目文件名/current/内部引用一律使用 UUID v4（`crypto/rand`
    自造格式，零新增依赖）；显示名（URL 安全字符集，可中文，全仓库唯一）只承担
    CLI 寻址与展示，CLI 不接受 UUID 输入。schema_version 升为 2，不迁移 schema 1。
    （建议：接受。）
13. `gdit sdk install` 无参数 + TTY 时交互式选择（枚举 `sdk available`，失败降级文本输入），
    非 TTY 无参数报用法错误；条目安装交互式的 managed SDK 版本三选一（推荐/列表/手动）。
    非交互形式仍只接受精确版本。（建议：接受。）

### 9.6 第四阶段：doctor（FR-08 环境诊断）

第四阶段交付 `gdit doctor`：**只读诊断命令**，收集式报告环境问题并给出建议，默认不做任何
修改、不落盘、不访问网络（FR-08「doctor 默认只报告和建议，不静默修改」）。它是
`engines/`、`sdks/` 下无效目录（§2「无效目录由 doctor 报告」）与 state 不一致的官方报告
渠道：`list`/`sdk` 只认完整安装，凡被它们忽略的内容都由 doctor 说明原因。

```text
gdit doctor [--network] [--verbose]
```

- 无参数：全量本地检查（零网络、零落盘、不获取全局修改锁）。
- `--network`：额外对启用来源做可达性探测（见「检查项」sources）。
- `--verbose`：展开细节（环境变量列表、来源探测结果等）；默认每项一行摘要。
- 简写：`doctor` 不设简写（顶层 `i/l/s/a/d/rm/r/st/e` 已占用，沿袭 sdk/autoremove 不设简写）。

#### 检查项

检查以**收集式**执行：单项失败降级为该检查项的 error 并继续，不中断后续检查；只有根目录
本身不可读等致命情况才整体返回错误（`ErrLocalIO`）。每条检查输出：状态（ok/warn/error）、
稳定 code、中文描述、可选建议。状态判定如下。

| code | 检查内容 | error | warn |
|---|---|---|---|
| `platform` | 当前 OS/arch 是否在支持矩阵（§3：linux/amd64、darwin/arm64、windows/amd64） | 不在矩阵（如 linux/arm32），提示不支持的平台组合 | — |
| `root-dir` | 实际根目录（`ResolveRoot`：`GDIT_ROOT` 非空即用，否则平台默认）存在且是目录、可读写；权限不对外可写（mode & 0o077，**仅 POSIX**；Windows 降级为目录可访问检查，不读取 ACL）。`GDIT_ROOT` 非法（非空相对路径/含 NUL）时命令在入口直接报配置错误，doctor 不进入 | 根目录缺失或不可访问（未初始化，建议 `gdit install`） | group/other 可写（建议 `chmod 700`）；`tmp/` 下遗留 `operation-*` 目录（上次安装中断残留，可安全删除） |
| `shim` | 按平台形态检查（Unix：`bin/godot` symlink 指向 gdit；Windows：`bin/godot.cmd` 存在、记录当前 `gdit.exe` 绝对路径并调用 `__shim`）；`bin/` 在 PATH 中且无其他 godot 抢先（PATH 分隔符按平台） | shim 指向错误、目标不存在或包装内容错误（建议 `gdit setup`） | 未创建 shim；`bin/` 不在 PATH（提示文本按平台模板）；PATH 中其他 `godot` 先于 `<根目录>/bin`（说明启动的是哪个，建议调整 PATH 顺序） |
| `current` | current 存在且为规范条目指针（按平台形态：Unix symlink / Windows 重定向文件，内容为 `instances/<uuid>.toml` 规范相对路径；§2 契约） | 悬空；绝对路径、含 `..`、指向其他目录或非普通条目文件；Windows 下文件内容非法或不可读 | 未设置 current（建议 `gdit install` 创建条目）；无任何条目 |
| `instances` | 全部条目可读、可解析、合法（§4.8 校验）；每个条目引擎引用完整安装；managed SDK 引用已安装 | 任意坏条目（失败关闭哲学，逐条报文件名与原因，提示修复或删除）；引擎引用缺失（提示 `gdit engine install <id>` 或删除条目）；managed SDK 引用缺失（提示 `gdit sdk install <version>`） | — |
| `engines` | `engines/` 下每个目录是完整安装（目录名合法 + install.toml 可解析 + launcher 存在；launcher 按平台校验：Unix 可执行文件 / macOS app bundle 内二进制 / Windows `.exe`） | 每个无效目录一条（原因：目录名不合法 / 缺 install.toml / install.toml 非法 / 启动文件缺失；提示手动删除或 `gdit engine remove <id>`） | state.toml 与目录不一致（下次读取会自动重建，doctor 不修改） |
| `sdks` | `sdks/` 下每个目录是完整托管 SDK（目录名合法 + install.toml + dotnet 可执行：Unix `dotnet` / Windows `dotnet.exe`）；系统 SDK 探测结果 | 每个无效目录一条（同 engines） | 系统 SDK 探测失败（PATH 无 dotnet 或 `dotnet --list-sdks` 失败；仅信息性，系统无 SDK 不是错误） |
| `templates` | `templates/` 目录状态 | — | 目录非预期存在且非空（模板支持属第五阶段 FR-07，内容暂不校验，提示不会被使用） |
| `environment` | 对当前条目计算注入增量（复用 `EffectiveEnv`）：变量（键 + 来源：global/platform/instance/derived）、显示驱动参数、fcitx 变量、DOTNET_ROOT 接管状态 | 无 current 时该项降级为 warn 并跳过预览；非 Linux 平台配置非 `auto` 的 `display_driver`/`input_method`（配置错误） | 无 current（无法预览，说明原因） |
| `sources` | config.toml 可解析；source_order 中的来源全部已知；disabled_sources 无未知来源；自定义源模板合法；authorization_env 指向的变量已设置 | 配置解析失败、schema 不支持、未知来源、模板非法 | authorization_env 未设置（不显示值）；`--network` 时来源探测失败（单个来源不可达；自定义源无元数据端点，标注跳过探测） |
| `state` | state.toml 可解析且与 `engines/`、`sdks/` 有效目录一致 | — | 不可解析或与目录不一致（下次读取自动重建） |

环境变量展示脱敏：键名匹配 `token`/`secret`/`password`/`key`（大小写不敏感）等敏感特征时
值以 `******` 掩码，`--verbose` 也不放开；路径类（`DOTNET_ROOT`、PATH 前缀、fcitx 变量）
显示完整值。`display_driver = "auto"` 时说明「不注入参数，由 Godot 原生默认与自动回退」；
显式 x11/wayland 时说明注入 `--display-driver`；用户显式配置 `DOTNET_ROOT` 时标注「用户
接管 SDK 定位」。平台差异化：fcitx 与显示驱动检查仅 Linux；macOS/Windows 跳过这两项，
`display_driver`/`input_method` 非 `auto` 报配置错误；PATH 检查用平台分隔符拆分；
PATH 提示文本由适配层提供模板（Unix `export PATH=...`，Windows `set PATH=...`）。
`root-dir` 输出实际生效的根目录并标注来源（`GDIT_ROOT` 或平台默认）；environment
检查项的每个变量标注来源层级（global/platform/instance/derived，见 §4.9 环境注入的
平台化）。

`--network` 探测：对 source_order 中每个启用且支持枚举的来源做 HEAD 请求其元数据端点
（godothub `releases.json`、github releases API），单请求超时 5 秒，串行执行，响应
context 取消；失败记为该来源 warn，全部失败汇总为 error。探测不改变配置、不写入任何
状态。默认测试不访问公网（fixture HTTP server 替代）。

#### core API

`Doctor` 是纯只读公共方法：不获取全局修改锁、不写任何文件、不重建 state、不访问网络
（除非 network=true）。结果结构带 json tag，供 CLI 渲染与第六阶段 GUI 复用。

```go
type CheckStatus string // "ok" | "warn" | "error"

type CheckResult struct {
	Code    string      `json:"code"`    // 稳定标识，对应检查项表 code
	Status  CheckStatus `json:"status"`
	Message string      `json:"message"` // 中文描述
	Suggest string      `json:"suggest"` // 可选建议，如「运行 gdit setup」
	Details []string    `json:"details"` // 可选展开细节，CLI 仅 --verbose 展示
}

type DoctorReport struct {
	Root       string        `json:"root"`
	Items      []CheckResult `json:"items"`
	OKCount    int           `json:"ok_count"`
	WarnCount  int           `json:"warn_count"`
	ErrorCount int           `json:"error_count"`
}

func (m *Manager) Doctor(ctx context.Context, network bool) (DoctorReport, error)
```

复用现有能力：store 的 `ScanValid`/`ScanSDKs`/`ReadCurrent`/`ShimPath`/`DirectorySize`、
instance 的 `Scan`/`Lookup`/`Validate`、config 的 `Load`/`validateSources`、
`EffectiveEnv`、platform 的 `DetectFcitx` 与 current 规范链接判定（与读取路径同一实现，
不复制规则）。系统 SDK 探测沿用 `Options.SDKProbe` 默认实现，测试注入固定结果。

#### CLI 输出

结果写 stdout，逐项一行，状态前缀 `[OK]`/`[WARN]`/`[ERROR]`（非 TTY 也保持，机器可读）：

```text
[OK]    根目录 ~/.gdit 存在且权限正确
[WARN]  ~/.gdit/bin 不在 PATH —— 建议：export PATH="$HOME/.gdit/bin:$PATH"
[ERROR] current 指向不存在的条目文件 instances/xxxx.toml —— 建议：gdit install 或 gdit default <name>
[OK]    引擎资产 4.5.2-standard 完整
...
3 项正常，1 项警告，1 项错误
```

- TTY 下状态着色：OK 绿 / WARN 黄 / ERROR 红；`NO_COLOR` 或非 TTY 无色（复用 CLI 层
  参数化的 TTY/着色判定，不依赖测试环境的真实终端）。
- 退出码：0 = 无 error；1 = 存在 error；warn 不影响退出码。`--network` 探测失败按 warn
  处理（网络故障与配置错误分离）。

#### 交付范围

1. core：`Doctor` 方法与 `DoctorReport` 结果类型；各检查项实现（复用 store/instance/
   config/env/platform 只读能力，不复制规则）；新增 `platform` 检查项。
2. platform 适配层拆分（§4.9）：能力化重构（ResolveRoot/AssetName/FindLauncher/SDKRID/
   SDKArchiveFormat/PrepareLauncher/DisplayDriver/DetectFcitx/ShimPath/ReadCurrentLink/
   WriteCurrentLink/RenameAtomic/AcquireLock/SyncDir/PathListSeparator 等按 OS 分文件）；
   current 规范指针判定下沉为可复用只读检查（与读取路径共用）；Windows 实现（LockFileEx、
   MoveFileEx、`godot.cmd` shim、current 重定向文件、zip SDK 资产、dotnet.exe launcher）；
   把现有散落的平台判断（lock/CLI execve/store 封装）收敛进适配层。
3. 根目录可配置：platform `ResolveRoot`（`GDIT_ROOT` 优先，须绝对路径）；CLI/GUI 生产
   入口接入；doctor 的 root-dir 检查显示实际根目录与来源。
4. 环境注入平台化：`[environment.<os>]` 平台小节配置读写与校验；`core/internal/env/`
   按编译标签拆分平台注入实现（env_linux.go / env_darwin.go / env_windows.go）；
   `EffectiveEnv` 合并平台小节并标注来源层级。
5. CLI：`gdit doctor [--network] [--verbose]` 命令与渲染（状态前缀、着色、汇总、退出码、
   平台化 PATH 提示模板）；Windows 下 `run` 走 spawn、shim 走 `godot.cmd`（调 `__shim` 入口）。
6. 测试：单元（各检查项正反例；三平台纯映射函数固定输入覆盖）、core 集成（坏 current、
   无效引擎/SDK 目录、坏条目、state 不一致、config 非法、权限 warn、环境脱敏、网络
   探测 fixture）、CLI 集成（stdout/stderr 分流、着色参数化、退出码 0/1）。
7. 实机验证：macOS Apple Silicon 实机、Windows x86_64 实机或 CI Windows runner
   （行为测试；交叉编译只算构建检查）。

#### 验收标准

- 全新根目录、正常安装 + setup 后：全部检查 ok，退出码 0；无 current 时 current 项 warn。
- 人为制造每种故障（悬空 current、非法 current 指针、坏条目、无效引擎/SDK 目录、
  state 不一致、config 非法、自定义源模板非法、authorization_env 未设置）后逐项报
  error/warn 且互不影响，退出码正确。
- doctor 执行前后 gdit 根目录内容逐字节不变（零落盘）；不获取修改锁（与并发安装
  并行执行不冲突）。
- 默认执行不访问网络；`--network` 下 fixture 服务器可达/不可达分别报 ok/warn，全部
  失败汇总 error，context 取消立即返回。
- 敏感键掩码：含 token/secret/password/key 的变量值不完整出现在任何输出（含 --verbose）。
- CLI 输出：结果在 stdout、着色符合 TTY/NO_COLOR 规则、退出码 0/1；非 TTY 纯文本可管道。
- 根目录可配置：设置 `GDIT_ROOT` 后，doctor/install/list 等全部命令读写自定义根目录
  （固定输入测试验证解析优先级与相对路径报错）；不设置时使用平台默认路径。
- 环境平台化：`[environment.linux|darwin|windows]` 平台小节仅当前平台生效并覆盖全局
  同名键，合并顺序与来源标注正确；非 Linux 平台 `display_driver`/`input_method`
  非 `auto` 报配置错误；平台注入实现按编译标签拆分，业务代码零平台分支。
- 平台行为：Linux amd64 全量验收；macOS Apple Silicon 实机验证（无 fcitx/display
  检查、app bundle launcher、路径行为）；Windows x86_64 实机或 CI runner 验证
  （`godot.cmd` shim、PATH 分号分隔、zip SDK 资产、MoveFileEx 原子写、LockFileEx
  并发互斥、dotnet.exe launcher、current 重定向文件读写与非法内容判定）。
- 业务代码零 `runtime.GOOS` 分支（`go vet` + review 检查）；纯映射函数（资产名/
  launcher/RID/资产格式）在 Linux 上用固定输入覆盖三平台输出。
- 默认测试不访问真实网络；`go test ./core/... ./cli/...`、`go vet` 与格式检查全部通过。

#### 实现决策（建议，待 review）

1. doctor 默认零网络、零落盘、不拿锁；`--network` 显式开启源可达性探测。
   （建议：接受。）
2. 退出码：0 = 无 error；1 = 存在 error；warn 与探测失败不影响退出码。
   （建议：接受。）
3. 环境变量值脱敏：敏感特征键名掩码，`--verbose` 不放开。（建议：接受。）
4. 检查全部条目（不只 current）的引用完整性，引擎/SDK 引用缺失统一 error、消息区分。
   （建议：接受。）
5. 本阶段不提供 `--fix`/自动修复，doctor 只报告和建议；修复入口沿用现有命令
   （`gdit setup`、`gdit sdk install` 等）。（建议：接受。）
6. 默认输出全部检查项（OK 行简短），`--verbose` 展开细节。（建议：接受。）
7. 第四阶段同时交付平台适配层拆分与 Windows 支持（§4.9）：doctor 检查项的平台
   差异化与适配层能力化是同一工作的两面，Windows 验收在实机/CI runner 完成。
   （建议：接受。）
8. Windows shim 用 `godot.cmd` 包装而非复制 `godot.exe`（避免二进制同步问题）。
   （建议：接受。）
9. 非 Linux 平台 `display_driver`/`input_method` 只接受 `auto`，显式其他值报配置
   错误（沿用「不注入 Linux 专用变量」哲学扩展为三平台）。（建议：接受。）
10. 映射类平台函数（资产名/launcher/RID/资产格式）在 Linux 单测用固定输入覆盖
    三平台；文件系统/进程/环境行为只在对应实机验收。（建议：接受。）
11. 根目录可配置：`GDIT_ROOT` 环境变量优先（必须是绝对路径，否则配置错误），否则
    平台默认路径（`~/.gdit/` / `%USERPROFILE%\.gdit`）；解析只发生在 platform
    适配层 `ResolveRoot`，core 不读环境变量。（建议：接受。）
12. 环境注入按平台配置：`[environment.linux|darwin|windows]` 平台小节仅当前平台
    生效、覆盖全局同名键；`core/internal/env/` 按编译标签拆分平台注入实现
    （env_linux.go / env_darwin.go / env_windows.go）。（建议：接受。）
13. Windows 的 current 用普通重定向文件（内容为规范相对路径 `instances/<uuid>.toml`）
    替代 symlink（零特权，不需要开发者模式）；Unix 保持 symlink；读写契约由
    `ReadCurrentLink`/`WriteCurrentLink` 平台能力封装，core 层一致。
    （建议：接受。）

### 9.7 第五阶段：suggest + 导出模板（FR-06 / FR-07）

第五阶段交付两项互相衔接但保持独立边界的能力：`suggest` 只读分析用户显式指定的 Godot
项目并给出条目配置建议；`template` 管理与精确 Godot 版本匹配的官方导出模板资产。
`suggest` 经用户明确授权后可编排既有条目安装和模板安装，但不会把项目路径写入条目、配置或
状态，也不会形成“项目 → 条目”的持久关联。

本阶段不引入项目自动切换、目录监听、项目清单或 GUI。GUI 仍在第六阶段，届时直接消费本节
定义的 core 结果类型。模板资产与平台无关；平台相关下载、锁、原子发布能力继续复用第四阶段
适配层，业务代码不增加 OS 分支。

```text
gdit suggest [<项目目录>]
    [--install --name <条目名>]
    [--sdk managed|system] [--sdk-version <精确版本>]
    [--current|--no-current] [--no-template]

gdit install <name> --version <版本> [既有选项] [--template]
gdit template [list]
gdit template install [--edition standard|dotnet] [--source <name>] <精确 Godot 版本>
gdit template attach [--source <name>] <条目名>
gdit template detach <条目名>
gdit template remove [-y|--yes] [--edition standard|dotnet] <精确 Godot 版本>
```

- `suggest` 的目录缺省为当前目录；路径只用于本次调用，结果返回清理后的绝对路径但不持久化。
- 默认先分析并完整报告。TTY 下随后询问是否安装（默认否），用户确认视为本次安装的明确授权，
  再询问条目名与是否设为 current；非 TTY 不询问且永不安装。
- `--install` 是跳过确认的脚本化明确授权。非 TTY 下必须同时给出 `--name`；TTY 下缺少名称时
  交互询问。
- `--sdk`、`--sdk-version`、`--current`、`--no-current` 与 `--no-template` 只允许和
  `--install` 同时使用；SDK 参数继续服从 `InstallEntryRequest` 的 edition/策略校验。
- 默认安装建议条目所需的引擎、托管 SDK（如有）和同版本导出模板；`--no-template` 仅跳过
  模板，并使新条目不包含 `[template]` 引用。普通 `gdit install` 默认不安装模板，显式
  `--template` 才安装并绑定；交互式安装询问是否需要导出模板，默认否。
- `template install`/`remove` 只接受精确 Godot 版本，不接受条目名或 `m` 前缀；edition 默认
  `standard`，C#/mono 模板显式用 `--edition dotnet`。`template` 不设简写。删除为不可逆操作，
  确认规则与 `sdk remove` 一致；被条目引用时拒绝删除。
- `template install` 是纯资产层动作，不创建条目引用；`template attach <name>` 从条目的 engine
  派生模板 ID，缺少资产时先安装，再原子写入引用；`template detach <name>` 只移除引用并报告
  新产生的孤儿，不直接删除资产。

#### 项目只读分析

新增 `core/internal/project/`，只负责固定范围内的格式解析和证据归一化，不做网络请求、安装、
条目查找或版本选择。分析范围严格限制为用户给出的目录中的以下文件：

1. 必需的 `project.godot`；它必须是普通文件或最终解析为普通文件的 symlink。
2. 可选的同目录 `global.json`。
3. 同目录按文件名排序的 `*.csproj`。本阶段不递归扫描子目录，也不向父目录查找
   `global.json`，避免越过用户显式给出的项目边界。

所有读取使用具名大小上限并响应 `context.Context` 取消；文件超限、不可读或语法损坏均返回带
文件路径的诊断，不无限读取。分析前后不创建文件、不改 mtime、不持有 gdit 修改锁，且不读取
`~/.gdit/current`。允许 symlink 时，解析后的文件仍必须位于清理后的项目目录内，不能借 symlink
越过本次分析边界。

`project.godot` 是 Godot Variant 文本格式，不按 TOML 解析，也不使用单个正则表达式猜测。
`project` 包实现最小词法解析器：识别 section、键值、字符串转义、行尾分号注释，以及
`PackedStringArray(...)`（Godot 4）和 `PoolStringArray(...)`（Godot 3）。只消费
`[application]` 下的 `config/features`：

- 唯一的 `MAJOR.MINOR` feature 是项目引擎系列；缺失或出现互相冲突的多个系列时标记为
  error，禁止 `--install` 猜测。
- feature 含 `C#` 时 edition 为 `dotnet`，否则为 `standard`。
- 同目录存在 `.csproj` 但 feature 未含 `C#` 时，edition 提升为 `dotnet` 并报告 warning，
  把“项目文件存在”作为比缺失 feature 更强的 C# 证据；feature 含 `C#` 但无 `.csproj`
  同样 warning，但仍给出 dotnet 建议。

`global.json` 使用 `encoding/json` 解析，只读取 `sdk.version`、`sdk.rollForward` 和
`sdk.allowPrerelease`；未知字段忽略。`.csproj` 使用 `encoding/xml` 解析
`TargetFramework`/`TargetFrameworks`，接受以 `netMAJOR.MINOR` 开头的目标框架（允许合法的
平台后缀），多个目标取最高 major.minor 并保留全部证据。SDK 建议优先级为：

1. `global.json` 中合法的精确 `sdk.version`；这是项目显式钉住的版本。
2. `.csproj` 目标框架对应的 SDK major.minor 通道。
3. 既有 `dotnet.RecommendedMajor`（Godot 系列 → 推荐 SDK 通道）保底映射。

`global.json` 的 `rollForward`/`allowPrerelease` 在结果中作为证据和提示展示，本阶段不实现一套
独立于 dotnet host 的完整 roll-forward 求解器：存在精确 `sdk.version` 时安装该版本；没有精确
版本时按建议通道解析最新可用 patch。损坏的可选文件不静默忽略：报告 error，仍尽可能返回
其余证据供用户查看，但 `--install` 在任何 error 存在时拒绝执行。只有 warning 不阻止安装。

#### 建议结果与安装解析

项目 feature 只声明引擎系列，不足以离线推导一个不存在的 patch。因此 `Suggest` 默认只返回
需求约束，不联网伪造精确版本：

```go
type SuggestLevel string // warning | error

type SuggestDiagnostic struct {
    Level   SuggestLevel `json:"level"`
    Code    string       `json:"code"`
    Path    string       `json:"path,omitempty"`
    Message string       `json:"message"`
}

type SuggestEvidence struct {
    Kind  string `json:"kind"` // project-feature / global-json / target-framework
    Path  string `json:"path"`
    Value string `json:"value"`
}

type ProjectSuggestion struct {
    ProjectDir   string              `json:"project_dir"`
    EngineSeries string              `json:"engine_series"` // 如 4.5，不冒充精确 patch
    Edition      string              `json:"edition"`       // standard / dotnet
    SDKStrategy  string              `json:"sdk_strategy"`  // 非 dotnet 为空；默认 managed
    SDKVersion   string              `json:"sdk_version"`   // global.json 精确版本，否则为空
    SDKChannel   string              `json:"sdk_channel"`   // 如 8.0
    Evidence     []SuggestEvidence   `json:"evidence"`
    Diagnostics  []SuggestDiagnostic `json:"diagnostics"`
    Installable  bool                `json:"installable"` // 无 error 且引擎系列明确
}

func (m *Manager) Suggest(ctx context.Context, dir string) (ProjectSuggestion, error)
```

`Suggest` 的“只读”包括 gdit 根目录：不初始化 `~/.gdit/`，不读取/重建 state，不获取锁，
不访问来源。可归因于项目内容的错误进入 `Diagnostics`，方法仍返回部分结果和 nil Go error；只有
路径解析、本地 I/O、context 取消等无法形成可信报告的失败才返回非 nil error。CLI 先完整输出
证据、建议和诊断，再决定是否进入安装动作。

安装动作由独立 core 编排承担，CLI 不复制版本选择、SDK 选择或模板安装规则：

```go
type InstallSuggestionRequest struct {
    ProjectDir      string `json:"project_dir"`
    Name            string `json:"name"`
    SDKStrategy     string `json:"sdk_strategy,omitempty"`
    SDKVersion      string `json:"sdk_version,omitempty"`
    SetCurrent      *bool  `json:"set_current,omitempty"`
    IncludeTemplate *bool  `json:"include_template,omitempty"` // nil 默认安装；false 对应 --no-template
}

type InstallSuggestionResult struct {
    Suggestion    ProjectSuggestion  `json:"suggestion"`
    EngineVersion string             `json:"engine_version"`
    Entry         InstallEntryResult `json:"entry"`
    Template      *TemplateInfo      `json:"template,omitempty"`
}

func (m *Manager) InstallSuggestion(ctx context.Context, request InstallSuggestionRequest) (InstallSuggestionResult, error)
```

`InstallSuggestion` 必须重新分析目录，不能直接信任调用方回传的 `ProjectSuggestion`。精确引擎
版本按以下确定性顺序解析：

1. 同系列、同 edition 的已安装引擎中选择最高稳定 patch；选择已安装资产时不联网。
2. 本地没有匹配资产时，调用现有 `Available`，在来源元数据中选择同一 major.minor 系列的
   最高稳定版本；不跨 minor，不自动选择预发布。
3. 所有启用来源都无法枚举或系列无可用稳定版时返回错误，提示用户用普通
   `gdit install <name> --version <精确版本>` 明确处理，不退回猜测版本。

SDK 有 `global.json` 精确版本时原样交给条目安装；只有通道时通过既有 SDK 枚举/保底列表解析
最新 patch。用户的 `--sdk`/`--sdk-version` 覆盖分析建议，但不绕过现有校验。Godot 3.x C#
继续归一化为 mono 策略，不安装 .NET SDK。

第五阶段为条目 schema 2 增加可选模板引用；这是向后兼容的可选字段，不提升 schema 版本：

```toml
[template]
id = "4.5.2-dotnet"
```

table 存在即表示该条目需要匹配的导出模板；`id` 必须严格等于
`<engine.version>-<engine.edition>`，不允许条目选择与引擎不同版本或 edition 的模板。省略 table
表示条目没有模板依赖。新版本读取既有 schema 2 条目时按“未绑定模板”处理；第五阶段以前的
旧二进制会忽略该 table，既有 `env set/unset` 的 map 合并写回会保留未知字段，且旧版 GC 不扫描
`templates/`，因此不会误删或静默丢失绑定。旧版不会展示、校验或安装模板，版本降级后的能力缺失
不属于兼容承诺。

`InstallEntryRequest` 增加 `Template bool`，`InstanceInfo` 增加 `Template string`（未绑定为空，
已绑定为模板 ID）。`instance.References` 增加 Templates 集合；引用始终从条目扫描派生，不另建
索引。模板引用非法属于条目配置错误并触发失败关闭；引用合法但资产尚未安装不影响
`default`/`run`，由 doctor 报告导出能力缺失。

条目、引擎、SDK 和模板的写操作共用一次全局修改锁；实现时把现有条目/模板安装管线拆出
`lockHeld` 内部入口，禁止递归获取同一锁。发布顺序为引擎 → SDK → 模板 → instance → current；
每项发布都保持现有原子目录/文件语义。下载资产一旦成功发布不做危险回滚：后续步骤失败时返回
已经发布的 `AssetChange`，未写 instance 就不会产生项目关联或坏条目，用户可重试或显式删除。

#### 导出模板资产模型

官方导出模板是平台无关 `.tpz`（ZIP）资产，但 standard 与 dotnet/mono 的模板不同；资产名称
和 `version.txt` 都携带 edition 语义。名称规则为：

```text
稳定 standard    Godot_v{version}-stable_export_templates.tpz
稳定 dotnet      Godot_v{version}-stable_mono_export_templates.tpz
预发布 standard  Godot_v{version}_export_templates.tpz
预发布 dotnet    Godot_v{version}_mono_export_templates.tpz
```

`version`/`edition` 使用与引擎相同的规范化规则，模板资产 ID 为
`<version>-<standard|dotnet>`。来源层扩展通用资产请求，不把模板名塞入 platform 适配层：

```go
type SourceRequest struct {
    Kind      string `json:"kind"`       // engine / template
    Version   string `json:"version"`
    Edition   string `json:"edition,omitempty"`
    AssetName string `json:"asset_name"`
    Target    Target `json:"target,omitempty"`
}
```

这是对现有公开 `SourceRequest` 的向后兼容字段扩展，不另造一套 provider 接口：引擎请求继续由
platform 生成 `AssetName` 和 `Target`，模板请求由版本层根据 edition 生成平台无关资产名，target
保持零值。`providerAdapter.Resolve` 直接把调用方已生成的 `AssetName` 传给现有内部
`source.ResolveRequest`，不再二次调用 `platform.AssetName`；注入的 fixture `Source` 因而能用同一
接口覆盖 engine/template。内置 GodotHub、GitHub 和现有 `{asset}` 自定义源复用同一元数据、认证、
fallback 与摘要契约：来源不可用可尝试下一个来源；摘要不匹配或清单损坏立即停止，不能 fallback。
`Available` 仍只枚举引擎，不把模板伪装成 edition。

```text
~/.gdit/templates/
└── <version>-<edition>/
    ├── install.toml
    └── payload/                 # .tpz 内顶层 templates/ 的内容
        ├── version.txt
        └── ...                  # 官方各目标平台的 debug/release 模板
```

```toml
# templates/<version>-<edition>/install.toml
schema_version = 1
id = "4.5.2-standard"
version = "4.5.2"
edition = "standard"
source = "github"
archive_name = "Godot_v4.5.2-stable_export_templates.tpz"
checksum_algorithm = "sha512"
checksum = "..."
installed_at = "2026-08-25T00:00:00Z"
```

安装流程复用 `tmp/operation-*`、context 取消、进度事件和 ZIP 安全解压：校验摘要后只接受单个
顶层 `templates/`，要求 `version.txt` 存在且声明版本与请求的版本/status/mono 标记一致（例如
`4.5.2.stable` 或 `4.5.2.stable.mono`），拒绝空包、越界路径、symlink 与重复目标；写完 manifest
后原子发布到 `templates/<version>-<edition>/`。目录已存在时仅在完整 manifest 与 payload 校验
通过时返回 `ErrAlreadyInstalled`；坏目录不覆盖，由 doctor 报告并提示用户处理。

```go
type TemplateInfo struct {
    ID                string   `json:"id"`
    Version           string   `json:"version"`
    Edition           string   `json:"edition"`
    Source            string   `json:"source"`
    ChecksumAlgorithm string   `json:"checksum_algorithm"`
    Checksum          string   `json:"checksum"`
    ArchiveName       string   `json:"archive_name"`
    Path              string   `json:"path"`
    Size              int64    `json:"size"`
    InstalledAt       string   `json:"installed_at"`
    References        []string `json:"references"` // 引用该模板的条目显示名，按名称排序
}

type InstallTemplateRequest struct {
    Version string `json:"version"`
    Edition string `json:"edition"`          // 空值归一化为 standard
    Source  string `json:"source,omitempty"` // 非空时禁用 fallback
}

type TemplateBindingResult struct {
    Instance  InstanceInfo  `json:"instance"`
    Template  *TemplateInfo `json:"template,omitempty"`
    Installed bool          `json:"installed"`
    Orphans   []OrphanAsset `json:"orphans,omitempty"`
}

func (m *Manager) Templates(ctx context.Context) ([]TemplateInfo, error)
func (m *Manager) InstallTemplate(ctx context.Context, request InstallTemplateRequest) (TemplateInfo, error)
func (m *Manager) RemoveTemplate(ctx context.Context, version, edition string) (TemplateInfo, error)
func (m *Manager) AttachTemplate(ctx context.Context, name, source string) (TemplateBindingResult, error)
func (m *Manager) DetachTemplate(ctx context.Context, name string) (TemplateBindingResult, error)
```

模板不加入 `state.toml`：模板列表直接按 manifest 扫描，避免扩大 state schema；这不影响引用
保护，引用集合仍由条目扫描派生。`template remove` 在锁内扫描全部条目，被引用时返回
`ErrAssetInUse`。未被任何条目引用的完整模板属于孤儿，进入 `Orphans` 和 `autoremove`；
`autoremove` 确认后必须像引擎/SDK 一样在锁内复查模板引用，只删除复查时仍为孤儿的模板。

`AttachTemplate` 在同一锁内校验条目、安装缺失资产并原子改写条目；已经绑定同一 ID 时幂等成功。
`DetachTemplate` 原子移除 `[template]`，未绑定时幂等成功；detach 不删除资产，而是在结果中返回
同一临界区计算的孤儿快照。current 指向的条目也允许 attach/detach，因为模板不参与启动解析。

本阶段只管理、校验和定位模板资产，不修改 Godot 自身的用户数据目录，也不创建
`~/.local/share/godot` 等外部 symlink。尤其不通过注入 `XDG_DATA_HOME` 强制 Godot 发现模板，
因为这会同时重定向 Godot 的其他用户数据，违反“环境注入最小化”和“所有管理数据在
`~/.gdit/`”的边界。将模板接入 Godot 导出流程必须另行确认一个上游支持的、不会接管全部用户
数据的机制；确认前 CLI/GUI 把 `TemplateInfo.Path` 作为已验证资产位置展示。

#### doctor 与 CLI 输出

第四阶段 doctor 的 `templates` 检查项在本阶段升级为正式校验：扫描每个一级目录，验证版本目录
名（版本 + edition）、manifest、摘要字段、`payload/version.txt` 和非空 payload。每个坏目录报
error，合法模板按版本报告 ok；目录不存在或为空是 ok。doctor 仍不修改、不重建模板目录，也不
访问网络。instances 检查同时验证模板 ID 与 engine 一致；绑定模板缺失记为 warn 并提示
`gdit template attach <name>`，不记为启动错误，不影响 `default`/`run`。

`suggest` 的 stdout 先输出稳定字段（项目目录、引擎系列、edition、SDK 建议、证据），warning/
error 诊断写 stderr；没有 `--install` 时即使存在分析 error 也以退出码 1 结束并保留可用报告。
安装进度继续写 stderr，最终创建的条目、实际精确引擎版本及模板 ID 写 stdout。`template list`
输出 `id/version/edition/source/size/references/installed_at`，非 TTY 为 tab 分隔纯文本。
`template detach` 与 `gdit remove` 一样在 stdout 报告解除绑定，随后列出新产生的孤儿提示。
普通 `gdit list` 增加 template 列：未绑定显示 `-`，已绑定显示模板 ID，资产缺失追加 `missing`。

#### 交付顺序

代码落地前先通过以下闸门，任一项未完成都不开始模板 store/source 实现：

1. 先同步仓库级 `AGENTS.md` 的阶段状态与平台范围：当前文件仍写“第三阶段实施中、不支持
   Windows”，与 PRD、本架构及已落地的平台代码冲突；实现与验收只能服从一套明确约束。
2. 用户完成本节 review，尤其确认“suggest 默认附带模板”与“只管理已验证模板路径、不改 Godot
   用户目录”两项产品语义。
3. 用至少一个当期官方稳定 release 核验 standard/dotnet 资产名、`.tpz` 顶层目录和
   `version.txt` 内容；把核验结果固化为小型 fixture。若上游事实与本节不同，先修订架构再编码。
4. 记录当前 `go test ./core/... ./cli/...`、race 与 vet 基线；第五阶段失败必须能区分存量问题与
   本阶段回归。

通过闸门后的实现顺序如下：

1. `core/internal/project`：Godot Variant 最小解析器、global.json/`.csproj` 结构化解析、证据与
   诊断模型；先用固定 fixture 锁定只读边界。
2. 模板资产层：版本/资产名规则、store manifest 扫描、来源解析、摘要校验、安全解压、原子
   install/list/remove；升级 doctor templates 检查。
3. 条目引用层：schema 2 可选 `[template]`、引用扫描/保护、孤儿计算、attach/detach 和
   autoremove 模板复查；保持 run/default 忽略模板缺失。
4. core 编排：`Suggest`、精确版本解析、`InstallSuggestion`，并把现有安装逻辑整理为可复用的
   锁内入口；普通 `InstallEntry` 仅新增显式 Template 选项，默认行为不变。
5. CLI：`suggest` 与 `template` 命令、普通 install 的 `--template`、TTY 确认、stdout/stderr、
   退出码和进度展示。
6. 同步 `docs/requirements.md`、`docs/commands.md`、README 与 Wiki 的已实现命令面；第六阶段
   GUI 设计只依赖本节公共 API，不回填业务规则。

#### 验收标准

- Godot 3/4 的 `PoolStringArray`/`PackedStringArray` fixture 能正确识别系列和 C#；注释、转义、
  键名相似文本不会误判。缺失/冲突 feature、损坏 JSON/XML、超限文件产生稳定诊断。
- `global.json` 精确 SDK、单/多目标 `.csproj`、Godot 推荐映射的优先级固定；Godot 3.x C#
  不建议或安装 .NET SDK。
- 默认 `suggest` 不访问网络、不读写 gdit 根目录、不拿锁；项目目录执行前后的文件集合、内容、
  mode 和 mtime 完全一致，current 也不改变。
- `--install` 只选择同 major.minor 的最高稳定版；本地命中零网络，远端枚举使用 fixture；
  无稳定匹配时失败且不跨系列、不选预发布。
- 非 TTY 未带 `--install` 永不安装；`--install` 缺 `--name` 报用法错误；TTY 确认默认否。
- standard/mono 模板资产名与 `version.txt` 匹配规则有固定 fixture；来源 fallback、指定来源、
  摘要失败、中断、坏 `.tpz`、重复安装、原子发布和删除确认均有固定 fixture 测试，默认测试
  不访问公网；注入的公共 `Source` fixture 必须同时断言收到的 `Kind`、`AssetName` 与零值/非零值
  `Target`，避免 provider adapter 重新引入平台资产名拼接。
- `InstallSuggestion` 中后续步骤失败时不留下半写 instance/current；已原子发布的资产在结果中
  明确报告，可安全重试。并发安装受同一全局锁串行化，不发生嵌套锁死。
- `[template].id` 与 engine 精确匹配；多个条目可共享一个模板。template remove 对引用资产失败
  关闭并列出引用条目，detach 后资产成为孤儿，删除条目也会正确产生模板孤儿。
- `autoremove` 把模板与引擎/SDK 一同列出并在确认后锁内复查；坏条目存在时不删除任何模板，
  并发 attach 不会误删刚获得引用的模板。
- 模板缺失不阻断 default/run，doctor 报 warn；非法或错配模板 ID 是配置 error。doctor 能区分
  合法模板与坏目录，执行前后 templates 内容逐字节不变；`state.toml` 不因模板操作改变。
- CLI 结果/诊断/进度分别进入 stdout/stderr，退出码遵循 0/1/2 约定；core 结果类型均有 json tag。
- 默认测试不访问真实网络；`go test ./core/... ./cli/...`、race、vet 与格式检查全部通过；模板
  资产名和归档布局再用至少一个当期官方 release 实测确认，live 验证不能替代 fixture。

#### 实现决策（建议，待 review）

1. `suggest` 默认严格本地只读、零网络；只在明确安装动作中为精确 patch 访问来源。
   （建议：接受。）
2. 项目分析只读指定目录的三个文件类别，不递归、不向父目录查找 `global.json`。
   （建议：接受。）
3. feature 只给出 major.minor；安装时优先最高已安装稳定 patch，否则枚举来源，永不跨 minor 或
   自动选预发布。（建议：接受。）
4. `global.json` 精确版本优先；没有精确版本时才按 target framework 或 Godot 映射解析 SDK
   通道。本阶段不重写 dotnet host 的 roll-forward 求解器。（建议：接受。）
5. `suggest --install` 默认同时安装匹配模板，`--no-template` 可关闭；项目路径不写入任何
   gdit 持久文件。（建议：接受。）
6. 模板按版本 + edition 独立存储，并作为条目的可选显式依赖；引用 ID 必须从 engine 精确派生。
   （用户已确认。）
7. 模板复用 Godot 来源及摘要信任，`.tpz` 校验后安全解压并原子发布；摘要失败不 fallback。
   （建议：接受。）
8. 不注入 `XDG_DATA_HOME`、不修改 Godot 用户目录来实现模板发现；本阶段只提供已验证模板路径，
   上游支持的无侵入接入方式确认后再扩展。（建议：需重点确认。）
9. `suggest` 分析 error 阻止安装但仍返回部分报告；warning 只提示不阻止。
   （建议：接受。）
10. 组合安装共用一次全局锁，不回滚已发布资产；失败结果必须准确列出已发布内容。
    （建议：接受。）
11. 被条目引用的模板拒绝删除；detach 或删除条目后进入孤儿集合，由 autoremove 锁内复查清理。
    模板缺失只影响导出能力，不阻断引擎启动。（用户已确认绑定方向；建议接受此生命周期。）
12. 扩展现有公开 `SourceRequest` 传入 `Kind` 与已生成的 `AssetName`，内部 provider 只负责 URL、
    元数据和摘要解析；平台资产命名与模板资产命名分别留在 platform/version 责任边界。
    （建议：接受。）

### 9.8 第六阶段：Wails GUI 实现（FR-09）

第六阶段把已有 core 能力组合为一个桌面工作台，不新增项目管理语义，也不在 GUI 层复制
版本解析、来源 fallback、SDK 求解、模板引用或平台判断。GUI 是 CLI 的另一种入口：Wails
bridge 负责生命周期、参数转换和事件转发，React 只负责视图状态与用户确认。

截至 2026-08-25，Linux 主平台实现与原生构建已经完成；macOS Apple Silicon 与 Windows
x86_64 仍需按本节验收矩阵进行 GUI 实机验证，交叉编译不计入完成。

#### 产品目标与边界

1. 首屏回答三个问题：当前会启动哪个条目、有哪些可用条目、是否存在需要处理的诊断。
2. 安装、切换、卸载和孤儿清理都必须是可见的确认流程；破坏性操作不能由单击列表项触发。
3. 项目分析只能由用户显式选择目录后触发；结果显示证据、warning/error 和拟执行动作，
   不保存目录、不监听目录、不改变 current。
4. 所有耗时操作均可取消；关闭窗口时，正在进行的操作进入“后台完成/取消”二选一，不能
   静默丢弃锁、进度或错误。
5. GUI 不修改 shell 配置、系统 PATH、系统 dotnet 或 Godot 项目目录；设置页只写
   `~/.gdit/config.toml` 允许的字段。

#### GUI 启动入口

- `gdit gui [参数]` 启动与当前 CLI 配套的 `gdit-gui`，参数原样传给 GUI 进程；GUI 进程退出
  后 CLI 返回相同的退出码。CLI 不创建 GUI 专属配置，也不通过网络查找 GUI。
- GUI 可执行文件查找顺序为 `GDIT_GUI`（显式路径）、CLI 同目录、源码树中的
  `gui/build/bin/gdit-gui`、当前目录下的同一路径，最后才查找 PATH 中的 `gdit-gui`。
  macOS 应用包和 Windows `.exe` 均由启动器按固定候选路径识别。
- `make run` 是仓库开发入口：先构建 GUI，再启动 `gui/build/bin/gdit-gui`；需要透传 CLI
  参数时使用 `make run-cli <command> [ARGS=...]`。GUI 构建失败不得回退为启动 CLI。

#### 无边框窗口与自定义顶栏

- Wails 使用 `Frameless` 窗口，GUI 不显示系统自带窗口顶栏；React 顶栏通过 Wails 的
  `--wails-draggable: drag` 区域实现拖动，最小化、最大化/还原和退出按钮调用 Wails
  runtime，不在前端模拟窗口状态。
- 顶栏风格设置写入唯一用户配置 `config.toml` 的 `[gui] titlebar_style`，值为
  `auto`、`mac` 或 `windows`。`auto` 在 Linux/macOS 默认使用左上角红黄绿交通灯风格，
  在 Windows 默认使用右上角的最小化、最大化和关闭按钮；用户可在设置页固定任一风格。
- 顶栏按钮必须标注可访问名称并设置 `--wails-draggable: no-drag`，避免点击控件触发拖动；
  无边框窗口仍保留 Wails 的可调整大小能力。
- GUI 首屏和条目图标使用仓库实际品牌资源：GoDoIt 使用 `assets/logo.svg`，Godot 和
  C# 使用各自官方仓库/品牌 SVG；应用窗口图标使用现有吉祥物透明 PNG 裁出的头像，生成
  `gui/build/appicon.png` 与 Windows `gui/build/windows/icon.ico`。
- 条目图标按圆形显示但不绘制边框，也不使用预设背景色；用户可在图标选择器设置背景色，写入条目
  `[appearance] background`。字段缺失或空字符串表示透明，只接受 `#RRGGBB` 或
  `#RRGGBBAA`，避免把任意 CSS 内容写入界面样式。

#### 信息架构与路由

桌面窗口采用“条目侧栏 + 选中条目内容区”布局，参考 XMCL 的实例工作流：条目是唯一的
一级工作对象，基础资源是条目的依赖，不与条目并列争夺导航层级。默认宽度 1180px、最小
宽度 900px；窄窗口降级为可折叠侧栏，不改变内容语义。路由和数据来源如下：

| 路由 | 内容 | core 读取/动作 |
|---|---|---|
| `/instances` | 条目浏览、设为 current、启动、卸载；`+` 打开创建向导 | `Instances`、`SetDefault`、`RemoveInstance`、`ResolveLaunch` |
| `/instances/:name` | 选中条目详情：运行时、环境、导出模板和操作记录 | `Default`、`EffectiveEnv`、`Templates`、`AttachTemplate`、`DetachTemplate` |
| `/instances/new` | 创建条目向导：版本、edition、SDK、模板、current 确认 | `Available`、`AvailableSDKs`、`InstallEntry` |
| `/tools` | 工具汇总页：项目建议、诊断和资源管理入口 | 读取 bootstrap 快照，不直接执行修改动作 |
| `/resources/engines`、`/resources/sdks`、`/resources/sources`、`/resources/cache` | 二级资源管理；查看引用、来源和孤儿清理 | `List`、`SDKs`、`Sources`、`Orphans`、`AutoRemove` |
| `/suggest` | 显式选目录、分析证据、安装建议 | `Suggest`、`InstallSuggestion` |
| `/doctor` | 本地诊断、可选网络探测、按严重级别筛选 | `Doctor(false/true)` |
| `/settings`、`/about` | 设置和关于 | `SetSourceDisabled`、`SetDefaultSource`、`SetEnvVar`；只读构建信息 |

侧栏条目列表的第一项固定为“新建条目”，头像位显示 `+`，点击进入创建向导；其后才是已有
条目。每个条目使用固定 44px 图标位，支持缺省、Godot、C#、GoDoIt 吉祥物头像和自定义图标，
当前条目用背景与文字标记高亮，不用状态点替代图标。
侧栏不直接展开项目建议、诊断或资源管理子项，只保留一个与`设置`、`关于`同级的`工具`入口；
`工具`位于`设置`之前，点击进入独立的 `/tools` 汇总页。工具页沿用设置页的全宽分区布局，
分别提供`项目建议`、`诊断`和`资源管理`入口；资源管理分区包含引擎、.NET SDK、下载来源、
缓存与孤儿四个明确命令。进入 `/suggest`、`/doctor` 或 `/resources/:kind` 后，侧栏的`工具`
入口保持选中状态，用户可直接返回汇总页。诊断 warning/error 数量显示在工具入口与工具页诊断项上，
不为工具页额外触发网络检查。当前条目变更后立即刷新侧栏和详情，不做页面级缓存。

#### 核心页面设计

**条目浏览**默认选中 current 条目。左侧条目列表负责切换上下文，右侧详情展示名称、引擎
版本、edition、SDK 策略、环境摘要和 `Launch` 主按钮；`Set current`、卸载、复制配置等
动作收进详情头部的显式按钮或溢出菜单。没有 current 时显示空状态和 `+` 创建入口。

**条目图标**在创建向导和条目详情中都可选择。`缺省` 是初始选择：普通版显示 Godot 图标，
dotnet/mono 版显示 C# 图标；Godot、C# 和 GoDoIt 吉祥物也可作为固定预设选择。自定义图标
通过系统文件选择器导入，接受 PNG/JPEG，解码后居中
裁切并规范化为 256x256 PNG，源文件不被修改。损坏、超限或透明度异常的图片拒绝导入；
自定义文件丢失时回退到 edition 默认图标并由 doctor 报 warning。

**导出资源**属于条目详情的一部分，不单独成为一级页面。详情中显示匹配的模板版本、下载
状态、大小、引用关系和 `下载模板`/`重新下载`/`解除绑定`操作；模板安装仍由 core 的模板
资产 API 完成，前端不得自行拼接模板 ID 或路径。

**安装向导**为四步状态机：`Engine`（系列、精确版本、来源）→ `Runtime`（standard 或
dotnet、managed/system、SDK patch）→ `Template`（匹配模板、大小、来源）→ `Review`。
每一步只展示 core 返回的候选项；网络枚举期间显示可取消的进度状态。Review 页明确列出
将安装的引擎/SDK/模板、预计占用、是否设为 current；只有点击 `Install` 才调用
`InstallEntry` 或 `InstallSuggestion`。

**Suggest** 先选择目录，再显示 `project.godot`、`global.json`、`.csproj` 的证据行。
error 以阻断色显示并禁用安装，warning 不阻断但必须在 Review 中再次出现；路径只在本次
React 状态中存在，Wails bridge 不写入持久配置。

**资源管理**从工具页的资源管理分区进入，分别查看 `Engines / SDKs / Sources / Cache`。
模板不在这里作为日常入口展示，只在资源诊断或条目详情中作为条目依赖出现。资源页表格显示
版本、来源、大小、引用条目和状态；`Auto remove` 先打开包含复查说明的确认对话框，成功后
用 core 返回的实际删除清单刷新，不根据前端旧列表自行推断。

**Doctor** 采用检查项列表而不是一张大卡片：每项包含状态、平台相关细节、修复建议和“打开
对应设置/命令”快捷入口。默认调用 `Doctor(false)`；“检查来源可达性”是显式开关，开启后
显示网络探测进度并把失败标为 warning，遵循 core 的错误级别。

#### Bridge 与事件契约

`gui/bridge` 只暴露下列薄方法，方法名使用面向界面的动词，参数/返回值直接映射 core 公共
类型并保留 `json` 字段：

```text
Bootstrap() -> AppSnapshot
ListInstances() / GetDefault() / ListAssets() / GetDoctor(network)
ListAvailableVersions(source) / ListAvailableSDKs()
InstallEntry(request) / InstallSuggestion(request)
SetDefault(name) / RemoveInstance(name) / AutoRemove()
SetInstanceIcon(name, request)
Suggest(projectDir) / SetEnvVar(scope, key, value)
ListSources() / SetSourceDisabled(name, disabled) / SetDefaultSource(name)
Cancel(operationID)
```

每个耗时调用返回 `operationID`，并通过 Wails 事件 `gdit:progress` 推送 `ProgressEvent`，
事件增加 bridge 侧的 `operation_id` 和 `timestamp` 包装字段但不修改 core 事件内容。
终态事件为 `complete | failed | canceled`；React 以 `operationID` 合并事件，禁止按版本
字符串猜测进度。窗口重载后调用 `Bootstrap`，不从本地缓存恢复半成品状态。

#### 状态、错误与安全

- React store 分为 `snapshot`（可重建读取状态）、`operation`（进行中任务）和 `modal`
  （确认/错误）；业务字段不复制到第二份模型。
- `context.Context` 取消映射到 `Cancel(operationID)`；Wails 窗口关闭先提示仍有任务，再
  逐项取消或等待完成。
- bridge 不接收 stdout/stderr、不记录 token、完整认证 URL 或环境变量值；敏感环境值在
  设置页默认掩码，调用 `EffectiveEnv` 时仅显示来源和键名，除非用户主动展开单项。
- 项目目录选择器把路径作为一次性输入，显示“不会写入项目或保存路径”的固定提示；GUI
  不提供“自动扫描主目录”或“按目录自动切换”入口。
- Wails bridge 只负责打开图片选择器；图片解码、大小限制、裁切、原子写入和条目字段更新由
  core 的 `SetInstanceIcon` 完成。删除条目时一并清理同 UUID 自定义图标；孤立图标由 doctor
  报 warning，不纳入引擎/SDK/模板的资产 GC。
- 安装、卸载、current 切换和模板 attach/detach 的确认文案必须来自操作结果，不以前端
  预估替代 core 校验；任何错误都保留旧视图并显示可重试动作。

#### 视觉与可用性基线

视觉基线见 [`assets/gui-design.svg`](../../assets/gui-design.svg)：浅色工作台背景、石墨文字、
青蓝主色和橙色警示色，避免大面积渐变和装饰性卡片。使用系统无衬线字体，正文 14px、标题
20px，表格行高不低于 44px；所有图标按钮带可访问名称，颜色不作为唯一状态信号。键盘焦点
可见，确认对话框支持 `Esc` 取消与 `Enter` 执行，窄窗口下表格转为纵向条目卡但不隐藏
current、edition、SDK 和引用状态。

#### 实施顺序与验收闸门

1. 先在 core 增加向后兼容的条目图标字段、`SetInstanceIcon` 和图标文件生命周期，再创建
   `gui/` Wails v2 module，建立最小 bridge、React 路由、主题 token 和 `Bootstrap`；用固定
   fixture 验证序列化与旧条目默认图标。
2. 完成条目侧栏、条目详情与 current/launch 流程，再接入 `+` 创建向导和结构化进度事件。
3. 接入条目内模板资源、二级资源管理、Suggest、Doctor、Settings；所有写操作补充成功、
   取消、错误和重载恢复测试。
4. Linux amd64 完成主验收后，再在 macOS Apple Silicon 与 Windows x86_64 验证窗口、文件
   选择器、shim 文案和路径显示；平台判断仍只在 platform/core 内。
5. 发布构建必须同时产出 CLI 与 GUI；GUI 不得改变 CLI 的退出码、网络策略或数据布局。

验收至少覆盖：首屏无 current、悬空 current、坏条目、缺 SDK/模板、安装中断、摘要失败、
取消后重载、doctor 网络开关、suggest 目录内容零变化、当前条目拒绝删除、模板引用保护、
孤儿复查不误删，以及三平台的键盘导航和高 DPI 布局。图标额外覆盖：旧条目按 edition
回退、缺省映射、五种图标策略渲染、PNG/JPEG 导入、超限/损坏图片拒绝、路径穿越拒绝、原子替换、条目
删除清理，以及自定义文件缺失时 doctor warning。

#### 待用户 review 的实现决策

1. GUI 首屏采用条目浏览器并默认选中 current；条目是一级主题，不再设置独立总览页。
   （用户已明确。）
2. 安装向导允许选择来源，但不允许在 GUI 中编辑自定义来源 URL；配置编辑继续使用
   `config.toml`，GUI 只管理来源顺序和启禁用。（建议：接受。）
3. GUI 不提供后台常驻、托盘自动切换或项目目录 watcher；后续若有需求另立阶段。（建议：接受。）
4. 第一版只做浅色主题和系统字体，先保证 Linux 主平台可读性；深色主题作为独立视觉迭代，
   不把主题偏好写入 core 配置。（建议：接受。）
5. 导出模板作为条目的资源依赖管理，不在一级或二级资源菜单中提供独立日常入口。
   （用户已明确。）
6. 条目列表第一项固定为 `+` 新建；图标选择增加 `缺省`，普通版缺省显示 Godot，
   dotnet/mono 版缺省显示 C#，并可固定选择 Godot、C#、GoDoIt 吉祥物或自定义图标。
   （用户已明确。）

## 10. Review 结论

2026-08-17 已确认以下第一阶段决策。

1. 使用来源自身提供的摘要（GodotHub 为 SHA-256，官方 github 为 SHA-512），来源同时承担下载和
   摘要信任责任。
2. Go module 使用 `github.com/Sheyiyuan/GoDoIt`。
3. 第一阶段只验收 Linux amd64。MVP 是否承诺 Linux arm64 留到平台验证时再决定。
4. 2026-08-17 实测确认 GodotHub（releases.json digest + atomgit 下载）和 Godot 官方（godot-builds
   release + SHA512-SUMS.txt）的稳定资产与校验 URL 规则，已写入内置 provider。AtomGit 独立来源的
   规则仍待确认；规则确认前不把猜测地址固化成内置来源，默认 source_order 为 `["godothub", "github"]`，
   显式配置 atomgit 时返回配置错误。

2026-08-18 追加确认来源管理扩展决策。

5. 交互式 `install` 使用 `github.com/AlecAivazis/survey/v2` 实现方向键选择（edition → version →
   source），仅限 CLI 层；非 TTY 下无参数 install 报用法错误。
6. 命令简写 `i/l/s/a`、flag 简写 `-e/-s`；后续阶段命令可能重新占用简写。
7. `source ban/unban` 为强禁用语义，状态写入 config.toml 的 `disabled_sources`。
8. `gdit source` 无参数即列出来源（保留 `source list` 写法兼容）。

2026-08-18 追加确认 instances 条目层、engine 命名空间与 SDK 策略决策（第三阶段设计）。

9. 定位升级：GoDoIt 是「Godot 引擎启动器与版本管理器」——底层包管理器（引擎/SDK 资产，
   现有 core 原样保留，通过 `gdit engine` 命名空间降级访问）+ 高级包管理器（instances
   条目：引用资产 + SDK 策略 + 环境配置），`godot` shim 读当前条目启动。
10. 日常命令入口条目化，不接受版本输入：顶层 `install` 为条目安装（交互式确认条目名与
    各项配置，顺带安装引擎与 SDK 依赖），顶层 `remove` 删除条目（更高抽象一级，不直接
    暴露删除引擎），`run`/`default` 只接受条目名；原资产层 install/remove/list 降级封装
    为 `gdit engine` 命名空间。命名条目本阶段开放。
11. SDK 策略两档：managed（默认，version 必填）/ system（备选）；不做运行时 auto 决策。
    策略在条目创建时确定，推荐 SDK 作为显式安装动作的依赖一并安装（apt 依赖语义）；
    启动路径保持零网络。
12. 推荐映射表由 core 静态常量维护（粒度 major.minor），不实时获取；同时作为警告级
    校验依据，不硬拦。
13. 资产 GC 采用 apt 语义：引用关系由条目扫描派生，不维护独立引用状态；被条目引用的
    资产拒绝删除（引用保护）；删除条目后提示孤儿，`gdit autoremove` 显式清理；
    不自动下载、不自动删除。
14. 第三阶段不兼容第二阶段数据布局，不实现迁移；只读取 `engines/`、`sdks/`、`instances/`
    和指向条目文件的 current，开发与验收从全新 gdit 根目录开始。
15. `gdit new` 作为条目安装命令 `gdit install` 的等价别名保留（无独立简写），
    `gdit engine install` 仍是资产层安装。

2026-08-19 文档 review 后补充确认以下实现约束。

16. 条目名不得匹配版本输入或引擎资产 ID，顶层 `run`/`default`/`remove` 不再承担版本输入
    的兼容解析。
17. 资产引用扫描失败关闭；坏条目存在时不执行 engine remove、sdk remove 或 autoremove，
    autoremove 在确认后锁内复查。
18. 一次业务操作只获取一次全局修改锁；`InstallEntry` 通过不取锁的内部原语组合引擎与 SDK
    安装，公共修改方法不得嵌套调用。
19. `InstanceInfo` 完整表达 edition 与 SDK 策略，`RemoveInstance` 返回同一锁内生成的孤儿快照；
    CLI 和 GUI 不重复解析条目来补业务字段。
20. 非交互条目安装用 `--current`/`--no-current` 表达显式选择，均未给出时使用“仅首个条目
    自动设为当前”的规则；core 用三态字段保留这一区别。

2026-08 平台方向变更 review 记录（第四阶段规划）：

21. 正式支持 Windows x86_64（验证级），平台矩阵改为 Linux amd64 主平台 + macOS Apple
    Silicon / Windows x86_64 验证级支持；`display_driver`/`input_method` 非 Linux 平台
    只接受 `auto`。（方向变更，AGENTS.md/PRD 已同步。）
22. 平台适配层按 OS 拆分实现文件（`platform_unix.go`/`platform_linux.go`/
    `platform_darwin.go`/`platform_windows.go`），业务代码零 `runtime.GOOS`；现有散落的
    平台判断（lock/CLI execve/store 封装）随第四阶段下沉（见 §4.9）。（建议：接受。）
23. Windows shim 用 `bin/godot.cmd` 包装（记录 `setup` 时实际 `gdit.exe` 的绝对路径并调用
    `__shim %*`），不复制 `godot.exe`；`__shim` 与 Unix argv[0] 判断共用 runShim 逻辑
    （参数直通、不过命令解析）。
    （建议：接受。）
24. Windows 原子写用 MoveFileEx(MOVEFILE_REPLACE_EXISTING)、锁用 LockFileEx
    （均走 x/sys，零新增依赖）；无目录 fsync，一致性降级文档化。（建议：接受。）
25. 2026-08 实测确认三平台资产命名：4.x 为 `win64.exe.zip`/`mono_win64.zip`（Windows）、
    `macos.universal.zip`/`mono_macos.universal.zip`（macOS）；3.x 为
    `win64.exe.zip`/`mono_win64.zip`（Windows）、`osx.universal.zip`/`mono_osx.universal.zip`
    （macOS）；.NET SDK 的 Windows 资产为 zip（`dotnet-sdk-<v>-win-x64.zip`），
    macOS 为 tar.gz。已写入 §4.6 与 §9.6。（建议：接受。）
26. 纯映射平台函数（资产名/launcher/RID/资产格式）在 Linux 单测固定输入覆盖三平台；
    文件系统/进程/环境行为在对应实机验收（macOS Apple Silicon 实机、Windows 实机或
    CI Windows runner）。（建议：接受。）
27. 数据根目录可配置：环境变量 `GDIT_ROOT` 优先（须绝对路径），否则平台默认路径
    （`~/.gdit/` / `%USERPROFILE%\.gdit`）；Windows 用户可把数据放非系统盘；解析
    只在 platform 适配层 `ResolveRoot`。（建议：接受。）
28. 环境注入按平台配置：`[environment.linux|darwin|windows]` 平台小节仅当前平台生效、
    覆盖全局；注入实现按编译标签拆分（env_linux.go / env_darwin.go / env_windows.go）。
    （建议：接受。）
29. Windows 的 current 改用普通重定向文件（内容为规范相对路径）替代 symlink，零特权；
    Unix 保持 symlink；读写由平台能力封装，core 契约一致。（建议：接受。）

框架完成后仍按项目约定先展示、review，得到用户明确的“可以”以后才能 commit。
