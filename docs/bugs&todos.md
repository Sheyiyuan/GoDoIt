# GUI 问题与 TODO 设计方案

> 状态：阶段 A、B 的代码与自动测试已经落地，阶段 C 发布完整性代码已经落地；三平台原生发布
> CI 首次运行和 GUI 实机验收待记录（2026-08-26）。本文不是架构
> 真理源；涉及产品语义和接口契约的内容以 `docs/architecture/README.md` §9.8 为准。
>
> 范围：完成第六阶段 GUI 的可用性、错误恢复、平台打包和验收，不新增项目管理、后台常驻、
> 自动目录切换或独立于 core 的业务规则。

## 1. 目标与原则

本轮调整解决以下问题：

1. 下载过程缺少可理解的进度、取消和失败状态。
2. GUI 首次启动把未初始化目录、可选 shim 和 PATH 集成混为故障。
3. 版本候选只在进入向导后拉取，等待感明显，分类选择也不完整。
4. 页面整体滚动导致头部、操作区和长列表缺少稳定位置。
5. `default` 的底层术语直接暴露到 GUI，不符合启动器交互习惯。
6. 关于页、版本注入、许可证和平台打包尚未形成统一发布流程。
7. 窗口顶栏设置保存后不立即生效。
8. 环境变量只能查看截断后的有效值，不能区分配置值和派生值，也不能完整编辑。
9. macOS 应用图标、CLI 启动 `.app`、GUI CI 和签名验证尚未闭环。
10. bootstrap 已收集的局部读取问题没有展示，部分操作也没有稳定的初始运行事件。
11. GUI 启动的 Godot 进程没有会话登记、退出事件和关闭入口，无法在重启 GUI 后恢复运行列表。

所有实现遵循以下原则：

- GUI 只组合 core 能力，不复制版本解析、来源 fallback、SDK 求解、引用扫描或平台判断。
- GUI 启动可初始化自己的数据目录，但不能修改 shell 配置、系统 PATH 或系统 dotnet。
- 项目路径只存在于本次 Suggest 交互，不进入任务历史、日志或持久配置。
- 环境变量值、认证 URL 和 token 不进入操作记录、诊断忽略项或日志。
- 先修复数据与状态契约，再调整页面；不能用前端猜测掩盖 bridge/core 缺口。

## 2. 优先级与实施顺序

| 阶段 | 优先级 | 内容 | 完成标志 |
|---|---|---|---|
| A | P0 | 启动初始化、bootstrap 问题展示、operation 状态契约、下载进度、顶栏即时生效 | 实现与自动测试闭环完成；GUI 实机视觉验收待补 |
| B | P1 | 候选预取与分类、滚动布局、Pin、运行会话、环境变量编辑 | 主要桌面工作流无需绕路或等待空白状态 |
| C | P1 | macOS 图标、统一版本、关于/许可证、CLI `.app` 查找、跨平台 GUI CI | 三平台都有可验证的 GUI 构建产物 |
| D | P2 | 视觉文案、无障碍、高 DPI、完整回归矩阵 | §9.8 验收项全部有自动测试或实机记录 |

阶段 A 完成前不开始发布打包。阶段 B 与 C 可以在 A 的接口稳定后并行推进。

### 当前执行状态（2026-08-26）

| 工作项 | 状态 | 已落地内容 | 剩余工作 |
|---|---|---|---|
| 启动初始化 | 已实现 | core `Initialize` 幂等创建标准目录；GUI Bootstrap 启动前调用；失败页显示根目录；不创建 current/shim；覆盖失败、半成品和重试 | GUI 实机视觉验收 |
| bootstrap 问题展示 | 已实现 | `AppSnapshot.issues` 与 Doctor 按故障/注意/可选集成分组；详情面板；warning 本次关闭；可选集成不计数 | GUI 实机视觉验收 |
| operation 契约 | 已实现 | queued 初始事件、唯一终态门、终态记录、事件先到 waiter、窗口关闭等待/取消；覆盖多资产、fallback 与敏感字段 | GUI 实机视觉验收 |
| 下载进度 | 已实现 | 按版本/文件名/来源聚合子任务；已知大小加权进度；未知大小稳定布局；顶栏入口、紧凑托盘与完整操作中心 | GUI 实机视觉验收 |
| 顶栏即时生效 | 已实现 | `updateGUISettings` 成功后原子更新 snapshot，失败回滚 | 补齐三平台窗口控件与高 DPI 实机记录 |
| 候选预取与分类 | 已实现 | 首屏后后台预取、向导复用、两级 channel 选择、局部来源 warning 与显式刷新 | 慢网和三平台实机验收 |
| 环境变量编辑 | 已实现 | 全局/条目配置层编辑、派生值只读、敏感值默认掩码、Windows 大小写规则 | 三平台实机验收 |
| 运行会话 | 已实现 | 持久登记、GUI 重启恢复、全局面板、实时事件、正常关闭与二次确认强制结束 | 多窗口和三平台实机验收 |
| 发布完整性 | 已实现 | 统一版本、离线法律文本、三平台归档、精确白名单、摘要和双通道 GitHub Release | macOS/Windows 原生 CI 首次运行 |

阶段 A、B 的代码与自动测试已经闭环，阶段 C 的发布代码也已落地。发布包只有在对应原生 CI
全部通过后才可对外使用；自动测试通过不替代 Linux 视觉验收及 macOS/Windows 实机验收。

阶段 B 当前落地：候选预取与向导复用、固定内容区滚动、Pin 文案、配置层环境查看/编辑/删除、
`runtime/sessions` 会话登记与恢复读取、Linux 会话启动/正常关闭/强制关闭、条目删除保护、会话
超时二次确认、全局运行会话面板、完整事件广播及跨平台平台层编译检查。剩余工作是 Linux 视觉
验收以及 macOS Apple Silicon、Windows x86_64 实机验收记录。

## 3. P0：启动与诊断语义

### 3.1 首次启动初始化 gdit 根目录

#### 设计

- GUI 启动时通过新增的 core 公共方法初始化 `~/.gdit/` 标准目录布局。
- 初始化只创建 gdit 根目录及 `engines/`、`sdks/`、`templates/`、`instances/`、`tmp/`、
  `icons/` 等应用目录，不访问网络，不创建空条目，不创建 current。
- 初始化必须幂等，并继续使用调用方明确传入的根目录，测试禁止接触真实 `~/.gdit/`。
- 不在 GUI 启动时自动调用 `Setup`。`Setup` 会创建 Godot shim，属于用户级命令集成，应由
  用户明确触发。
- 初始化失败时停留在可重试的启动错误页，显示目标根目录和简化错误，不进入半可用工作台。

#### Doctor 展示规则

- 根目录未初始化不再是 GUI 首次启动后的常态错误，因为 bootstrap 前已经执行初始化。
- `shim` 未创建、gdit `bin/` 不在 PATH、系统中存在更早的 Godot，对 GUI 自身均不构成故障。
  GUI 将其显示为“命令行集成”建议，不计入工作台顶部的错误/警告数量。
- shim 已创建但损坏仍是 error，因为这代表已启用的命令行集成失效。
- core Doctor 保留完整诊断语义，CLI 输出不因 GUI 展示策略而改变。

#### 验收

- 全新临时根目录第一次启动后目录完整，Doctor 不报告“根目录不存在”。
- 启动过程不创建 shim，不修改 PATH、shell 配置或项目目录。
- 重复启动不改变已有配置、条目、current 和资产。
- 初始化任一步失败后不留下被当作有效资产读取的半成品。

### 3.2 bootstrap 局部问题展示

#### 设计

- 保留 `AppSnapshot.issues`，前端增加全局问题横幅和“查看详情”面板。
- 横幅覆盖悬空 current、坏条目、资源扫描失败、GUI 配置降级等 bootstrap 局部错误。
- 问题按本次 bootstrap 结果展示，不持久化；刷新成功后自动消失。
- 每项提供“重新读取”和“打开 Doctor”，不根据错误字符串推断修复命令。
- snapshot 局部可用时继续展示可读数据；不得用空数组伪装成“没有安装任何内容”。

#### 验收

- 悬空 current、坏条目和资源扫描失败都能在首屏看到。
- 问题存在时旧视图不被破坏，用户可以进入 Doctor 或重试。
- 错误消息不包含环境变量值、token 或完整认证 URL。

### 3.3 诊断忽略机制

“允许忽略警告”不能直接对当前 `code` 实现，因为一个 `shim` code 同时包含可选建议和真实错误。

#### 第一版范围

- 先做展示分组，不做持久忽略：`故障`、`需要注意`、`可选集成`。
- `可选集成` 默认折叠，不进入侧栏提醒数量。
- 普通 warning 允许“本次关闭”，仅存在于 React 内存，重新启动后恢复。
- error 不允许忽略。

#### 后续持久忽略的前置条件

- core 为可忽略诊断提供稳定且足够细的 `issue_id`，例如
  `shim.not_installed`、`path.gdit_bin_missing`，不能依赖中文 message。
- 在 `config.toml` 增加经过架构 review 的 GUI 诊断偏好字段。
- 忽略项必须记录诊断 ID，不能记录路径、URL 或环境变量值。

## 4. P0：操作中心与下载进度

### 4.1 operation 事件契约

#### 当前问题

- bridge 只在 core 发出 `ProgressEvent` 后发送 `running`，没有中间进度的 Doctor、Suggest、
  remove 等任务不会及时出现在操作托盘，也就无法取消。
- store 每个 operation 只保留最后一个事件，组合安装中的引擎、SDK 和模板子任务会互相覆盖。
- `bytes_downloaded` 和 `total_bytes` 已存在，但 UI 没有进度条。

#### 设计

- `startOperation` 注册成功后立即发送一次 `running/queued` 事件，再启动 goroutine。
- 保持 `operation_id` 为顶层任务标识；下载子任务使用 core 提供的 `version + filename + source`
  形成展示键，不把该键当业务标识写回 core。
- bridge 仍只转发 core 进度，不计算资产选择、fallback 或安装结果。
- 终态保持 `complete | failed | canceled`，每个 operation 只能产生一次终态。
- 前端刷新后不恢复半成品任务；窗口关闭仍通过 Wails 原生对话框选择等待或取消。

### 4.2 前端操作状态

前端 store 将 operation 建模为：

```text
OperationRecord
├── id / operation / status / started_at / finished_at
├── summary / error
└── items[]
    ├── key / version / source / filename
    ├── stage
    └── bytes_downloaded / total_bytes
```

- `queued`、`resolve`、`verify`、`publish` 等没有字节总数的阶段使用不定进度。
- `download` 且 `total_bytes > 0` 时显示百分比、已下载和总大小。
- 多资产安装分别显示子任务；顶层只在全部大小已知时计算加权总进度，否则显示阶段摘要。
- 终态记录只保留在当前窗口内，支持清除；不得把请求参数写入磁盘。
- 项目路径、环境变量和来源认证配置不进入 OperationRecord。

### 4.3 组件设计

- 顶栏增加下载/任务图标和运行数量，点击打开独立“操作中心”侧面板或对话框。
- 紧凑托盘只显示当前任务摘要、总进度和取消按钮，不再承载完整历史。
- 操作中心分为“进行中”和“最近完成”：
  - 进行中：阶段、子任务进度、取消。
  - 完成：实际安装/删除结果、完成时间、清除。
  - 失败：错误摘要、返回来源页面；第一版不做通用重试。
- 重试必须由原始页面重新提交经过当前 core 校验的请求，不能从历史记录盲目重放。

### 4.4 验收与测试

- 引擎、SDK、模板下载均显示字节进度；总大小未知时布局不跳动。
- InstallEntry 同时涉及多个资产时，各子任务不会覆盖。
- Doctor、Suggest、remove 在调用返回 operation ID 后立即可见并可取消。
- 取消、摘要失败、网络失败、fallback 和成功都有稳定终态，按钮不会永久停留在 loading。
- 终态先于前端 waiter 注册时仍能正确完成，窗口内不存在未释放 waiter。
- 操作中心不泄露项目路径、token、认证 URL 或环境变量值。

## 5. P0：窗口顶栏设置即时生效

### 5.1 原因

设置页保存后只更新自己的 `titlebarStyle` 局部 state，没有更新全局 `snapshot.gui`；Layout 监听的
仍是旧值，因此顶栏样式直到重新 bootstrap 或重启才变化。

### 5.2 设计

- store 增加 `updateGUISettings` action，由它调用 bridge、成功后原子更新
  `snapshot.gui.titlebar_style`。
- Settings 不再维护与 snapshot 平行的长期配置副本，只保留提交中的临时选择和错误状态。
- 保存失败时保留旧样式并显示错误；成功后 Layout 立即重新解析 `auto/mac/windows`。
- `auto` 继续由 Wails runtime 平台信息决定，不在业务代码增加平台判断。

### 5.3 验收

- 三种选项切换后当前窗口立即改变控件位置。
- 写入失败时 UI 和配置都保持旧值。
- 重启后读取到相同设置。
- macOS/Windows 实机验证拖动、最小化、最大化、关闭和高 DPI 命中区域。

## 6. P1：版本候选预取、缓存与分类

### 6.1 启动行为

- Bootstrap 完成并渲染首屏后，前端后台并行启动引擎版本和 SDK 版本枚举。
- 预取不能阻塞首屏，失败只更新候选状态，不加入 bootstrap 故障。
- 候选只保存在当前窗口内，不新增磁盘缓存；避免引入过期策略和新的管理数据。
- 同一时刻每种候选最多一个请求。进入创建向导时复用正在进行或已完成的结果。
- 候选枚举是普通后台只读请求，不创建 operation、不进入任务队列或关闭窗口确认；应用退出时随
  GUI context 取消。
- 工具页或创建向导提供显式刷新；已有同类请求进行中时复用该请求，完成后可重新枚举。

### 6.2 分类选择

- 引擎按 core 的 `4.x`、`3.x`、`unstable` channel 显示，先选系列，再选精确版本。
- 版本项继续显示可用 edition 和来源；切换来源后保留仍合法的选择，否则选择该分类第一项。
- SDK 先选 major/minor channel，再选 patch，展示 `LTS`、`Preview`、`EOL` 等 core 字段。
- 不在前端重新解析版本字符串或判断 Godot edition 支持情况。
- 空结果、部分来源失败和全部失败必须有不同状态；部分失败保留 core warning。

### 6.3 验收

- 首屏可交互后才开始网络预取，慢网络不阻塞条目浏览和启动。
- 创建向导打开时优先复用候选，不重复请求。
- 手动刷新、切换来源、后台加载和失败均有明确反馈；应用退出时请求随 GUI context 取消。
- 分类顺序和 CLI 一致，3.x、4.x、unstable、SDK channel 均有固定 fixture 测试。
- 创建向导固定为“配置 / 确认”两步；引擎、edition、条件化 SDK、模板、图标和 current 在同一配置页。
  Standard 不展示 SDK 控件且不得提交 SDK 参数；Godot 3.x dotnet 只展示 Mono；只有 Godot 4.x+
  dotnet 才展示 Managed/System 与 SDK 版本。

## 7. P1：工作区高度与滚动模型

### 7.1 设计

- `app-shell`、主内容区和每个路由页面固定在窗口可用高度内，不让整个右侧页面无边界增长。
- 页面采用 `header / body / footer` 三段：
  - header 保持可见；
  - body 使用 `min-height: 0; overflow: auto`；
  - 仅向导或需要固定主操作的页面使用固定 footer。
- 条目详情按内容区滚动，启动按钮和条目上下文保持在 header。
- 资源表、Doctor 检查列表、版本网格和环境变量列表分别滚动，不嵌套两个同方向滚动容器。
- 窄窗口继续使用抽屉侧栏；移动断点下表格转纵向条目，字段不得隐藏。

### 7.2 验收

- 900x620 最小窗口下主操作始终可达，没有页面级和组件级双滚动冲突。
- 1180x760 默认窗口下右侧占满可用高度，短页面不会出现无意义滚动条。
- 长条目名、路径、环境值和错误文本不挤出按钮或遮挡下一行。
- Linux 100%/150%/200%、macOS Retina 和 Windows 125%/150% 完成截图验收。

## 8. P1：Pin 交互语义

### 8.1 设计边界

- 底层仍使用唯一 `current` 指针和 `SetDefault` core API，不增加收藏、排序或多个 pin。
- GUI 不展示 `default` 一词：
  - 操作名称为“设为当前”；
  - 图标使用 `Pin`；
  - 当前条目显示“已固定为当前启动条目”。
- 创建向导和 Suggest 保留“安装后设为当前条目”，避免只写“Pin”导致语义不清。
- 切换 current 前显示轻量确认，明确普通 `godot` 和顶栏启动入口之后会使用哪个条目。

### 8.2 验收

- 任意时刻最多一个 current；切换失败时旧 current 保持不变。
- 当前条目不能卸载，按钮和确认文案一致。
- GUI 文案不出现用于条目切换的 `default`，来源优先级中的“默认来源”不受影响。

## 9. P1：运行会话、多开与关闭

### 9.1 能力边界

- “运行中”只表示由 GoDoIt GUI 启动并登记的 Godot 会话，不扫描、识别或接管系统中由
  Steam、终端、文件管理器或其他启动器打开的 Godot。
- 一个条目允许同时启动多个会话。每次点击启动都创建独立 `session_id`，不复用 PID，不改变 current。
- 第一版不把 CLI `gdit run` 纳入 GUI 会话列表。CLI 在 Unix 上可能用 execve 替换自身，生命周期
  契约与 GUI 子进程不同；后续若要统一，需要单独设计跨入口 supervisor。
- 会话只关联条目和引擎，不保存用户在 Godot 内打开的项目，也不扫描进程命令行推断项目路径。
- 关闭 GoDoIt 窗口默认不关闭 Godot 会话；下载/安装任务仍沿用“等待或取消”的关闭确认。

### 9.2 会话登记与恢复

仅在 bridge 内存保存 PID 无法支持 GUI 重启恢复，因此新增 gdit 管理的运行时目录：

```text
~/.gdit/runtime/
├── sessions/
│   └── <session-uuid>.toml
└── .lock
```

每条记录只包含：

```text
session_id / instance_id / instance_name / engine_id
pid / process_identity / started_at
```

- `process_identity` 是 platform 适配层返回的不透明创建标识，用于区分 PID 复用；不能只凭 PID
  判断某个进程仍属于 GoDoIt。
- 不保存完整命令行、项目路径、环境变量、认证信息、父进程环境或可执行文件路径；恢复时由
  `engine_id` 重新扫描 manifest 推导启动文件。
- 启动成功并取得 PID 后原子写入记录；写入失败时立即报告会话登记失败，并请求关闭刚启动的进程，
  避免产生 GUI 无法管理的后台进程。
- 子进程退出后由 waiter 删除记录并发送会话事件。GUI 异常退出后，下次 bootstrap 扫描记录，
  通过 PID、进程创建标识和目标可执行文件三项核验，删除失效或不匹配的旧记录。
- 多个 GUI 窗口共享 `runtime/.lock` 和原子记录。任一窗口启动或关闭会话后，其他窗口通过事件或
  周期性轻量刷新收敛，不能维护各自冲突的会话真相。
- runtime 会话不写入 `state.toml`，不参与已安装资产清单重建。

### 9.3 Bridge 与平台适配

bridge 增加以下面向界面的契约：

```text
ListSessions() -> []SessionInfo
Launch(name) -> SessionInfo
RequestStopSession(sessionID) -> SessionInfo
ForceStopSession(sessionID) -> SessionInfo
```

- `Launch` 由 core 的会话 API 完成解析、进程创建、登记和监督；这样 CLI 删除条目与多个 GUI
  窗口都能服从同一套运行中保护，bridge 只转发 Wails 调用和会话事件。
- core 启动后必须调用 `Wait` 回收自己创建的子进程，避免僵尸进程；恢复出的非子进程由 platform
  适配层轮询身份和存活状态。成功登记后的进程生命周期不绑定 GUI context。
- 前端只提交 `session_id`，不能提交 PID 或可执行路径。bridge 每次关闭前重新核验登记记录和
  process identity，防止 PID 复用导致误杀其他进程。
- platform 适配层提供进程身份、存活检查、请求正常关闭和强制结束能力，业务代码不出现
  `runtime.GOOS`：
  - Linux/macOS：先发送适合 GUI 进程的正常终止信号并等待退出；
  - Windows：向属于目标 PID 的顶层窗口发送关闭请求；
  - 强制结束使用平台原生能力，仅作用于已重新核验的目标进程。
- 进程启动、状态变化和退出通过 `gdit:session` 事件广播，状态为
  `running | stopping | exited | lost`。

### 9.4 关闭语义

- “关闭”先请求 Godot 正常退出，给编辑器保存窗口和退出清理机会。
- 请求后进入 `stopping`。在约定超时内未退出时，UI 显示“仍在运行”，再提供独立的“强制结束”。
- “强制结束”必须二次确认并明确提示未保存内容可能丢失，不能在超时后自动执行。
- 对已经退出、身份不匹配或失联的会话，关闭操作只清理失效记录，不发送信号。
- 关闭单个会话不影响同条目的其他会话。第一版不提供无确认的“一键强制结束全部”。
- 正在运行的条目拒绝卸载，并提示先关闭其全部会话；这样能避免条目消失后留下难以解释的会话，
  也避免 Windows 对运行中资产的删除冲突。

### 9.5 UI 设计

- 顶栏增加“运行中 N”进程图标，与下载操作中心分开，避免把长期 Godot 会话当作短期安装任务。
- 点击打开“运行会话”面板，每行显示条目名、Godot 版本、启动时间、运行时长和 PID；PID 仅供诊断，
  不是前端操作参数。
- 条目详情显示该条目的运行数量：
  - 无运行会话时主按钮为“启动 Godot”；
  - 已有会话时主按钮仍可用，文案为“再启动一个”；
  - 旁边提供“查看 N 个运行会话”。
- 同一条目的多个会话必须逐行展示并可单独关闭，不能只显示一个合并状态。
- 会话自然退出后实时从运行列表移除，可短暂显示“已退出”状态后淡出；不做持久运行历史。
- 空状态明确说明这里只显示由 GoDoIt GUI 启动的 Godot，不暗示系统中没有其他 Godot 进程。

### 9.6 验收与测试

- 同一条目连续启动三个会话，得到不同 session ID 和 PID，三个会话都能独立退出。
- GUI 关闭后 Godot 继续运行；重新打开 GUI 后能恢复会话列表并继续正常关闭。
- 子进程自然退出、启动失败、登记写入失败、GUI 崩溃和遗留记录都有固定测试。
- 构造 PID 复用或 process identity 不匹配时不得向该 PID 发送任何终止请求。
- 正常关闭超时后只显示强制结束入口，不自动杀进程；强制结束必须确认。
- 多 GUI 窗口并发启动和关闭不会损坏 session TOML，也不会重复关闭同一进程。
- Linux、macOS Apple Silicon 与 Windows x86_64 分别完成正常关闭、强制结束、GUI 重启恢复实机验收。
- 测试全部使用可控的假子进程和临时 gdit 根目录，不能启动真实 Godot 或读写开发者进程。

## 10. P1：环境变量查看与编辑

### 10.1 数据契约

`EffectiveEnv` 是启动时合并结果，不能用于反推用户配置。core 需要提供结构化读取接口，分别返回：

- 全局用户变量；
- 当前平台变量；
- 条目变量；
- core 派生变量；
- 最终有效值和来源。

用户只能编辑全局和条目变量。平台默认值、托管 SDK 产生的 `DOTNET_ROOT/PATH` 和其他派生值只读。

### 10.2 UI 设计

- 条目详情只显示变量数量和来源摘要，点击一行打开独立环境变量对话框。
- 对话框使用列表和详情双区：选择变量后显示完整键、完整值、来源、作用域和是否可编辑。
- 敏感键默认掩码，用户每次主动点击后才显示；关闭对话框后恢复掩码。
- 可编辑变量支持新增、修改、删除；保存前显示作用域，不允许把派生值覆盖操作伪装成编辑。
- 长值使用可换行的只读/编辑文本区，并提供复制按钮；不在普通表格里强行展开全文。
- 设置页管理全局变量，条目详情管理条目变量；两者复用同一编辑组件和 bridge 方法。

### 10.3 验收

- 能完整查看长 PATH，且不会撑破窗口。
- 全局与条目变量能新增、修改、删除，保存后重新计算 EffectiveEnv。
- 派生变量只读，来源标识准确。
- token/secret/password/key 默认掩码，测试和错误信息中不出现值。
- Windows 环境键大小写规则继续由 core 处理。

## 11. P1：关于页、版本和法律信息

### 11.1 已落地

- 根级 `VERSION` 是稳定版本的唯一人工维护源；开发构建版本为
  `<VERSION>-dev.<UTC 日期>.<短 commit>`。
- CLI `gdit version`、GUI `--build-info`、关于页和平台元数据共用 `core/buildinfo`，发布前逐项核对。
- 发布构建使用隔离的 Wails 暂存工程和 linker flags，不临时改写已提交的 `wails.json`。
- 关于页离线内嵌项目 AGPL-3.0 全文、第三方声明、无担保提示与源代码地址，不访问网络。
- `THIRD_PARTY_NOTICES.txt` 从三平台 Go 运行时依赖、pnpm 生产依赖闭包和固定品牌素材元数据生成；
  缺少、过期、未知或未进入产物的元数据均使门禁失败。

项目未确认独立“用户协议”文本，因此当前不虚构该法律文件；许可和无担保说明以 AGPL-3.0 及
第三方声明为准。

## 12. P1：平台图标与打包

### 12.1 已落地

- `assets/icon.png` 是三平台应用图标唯一源；Wails PNG 是字节级副本，Windows ICO 必须含
  16/32/48/64/128/256 尺寸，macOS `.icns` 在原生构建中派生。
- `gdit gui` 支持配套裸二进制、Windows `.exe` 和 macOS `.app/Contents/MacOS/gdit-gui`，并保留
  `GDIT_GUI` 显式覆盖。
- Linux、macOS、Windows 分别生成 `.tar.gz`/`.zip` 归档，每个归档都携带 CLI、GUI、`LICENSE` 和
  `THIRD_PARTY_NOTICES.txt`；macOS bundle 内另有离线法律文本副本。
- macOS 原生 job 执行 arm64、plist、法律资源、ad-hoc 签名及严格签名验证；Windows job 校验
  ProductVersion。归档工具拒绝符号链接、危险路径、测试 fixture、密钥和绝对工作区路径。

Developer ID、公证、stapling、Windows Authenticode、DMG 和安装器不在阶段 C 范围。当前产物不得
描述为已公证或可信发布者签名。

## 13. P1：跨平台 GUI CI

CI 和 Release workflow 已配置：

- Go 1.25.13 最低版本与 Go 1.26 质量门禁继续保留。
- 三平台固定 Node、pnpm、Wails 版本并使用 frozen lockfile，运行前端测试、原生 Go 测试和 vet。
- Linux amd64、macOS arm64、Windows amd64 分别执行原生 Wails release build 和平台归档校验。
- 中间 artifacts 只在同一次 workflow 传递；最终 job 生成 `SHA256SUMS`，复验精确四文件白名单后
  才创建 GitHub Release。
- `main`/手动运行发布可替换的 `dev-latest`；`v<VERSION>` 只创建不可覆盖的稳定 Release。

工作流代码已经完成，三平台 GitHub-hosted runner 的首次实际运行仍待推送后验证。CI 构建通过也
不等于实机验收；文件选择器、窗口控制、缩放、Dock/任务栏图标和关闭任务提示仍需三平台记录。

## 14. 测试与验收矩阵

### 14.1 自动测试

- bridge：operation 初始事件、进度转发、唯一终态、取消、关闭窗口操作表。
- store：多子任务合并、终态先到、清除历史、敏感字段不入记录。
- 首屏：无 current、悬空 current、坏条目、bootstrap issues、初始化失败重试。
- 创建向导：后台预取复用、分类、刷新、取消、空结果、fallback warning。
- 下载：已知/未知大小、多资产、摘要失败、中断、取消和重载。
- 会话：多开、自然退出、正常关闭、强制结束、PID 复用、GUI 重启恢复和多窗口并发。
- 设置：顶栏即时生效及回滚；环境变量新增、编辑、删除、掩码和只读派生值。
- 平台：macOS `.app` 查找、Windows `.exe` 查找、图标产物尺寸和版本元数据。

### 14.2 实机验收

| 平台 | 必测内容 |
|---|---|
| Linux amd64 | WebKit 渲染、文件选择器、下载进度、会话恢复/关闭、Wayland/X11、100%/150%/200% 缩放 |
| macOS arm64 | 应用图标、Retina、窗口控制、会话恢复/关闭、`.app` 启动和 ad-hoc 签名验证 |
| Windows x86_64 | WebView2、窗口按钮、会话恢复/关闭、文件选择器、路径、125%/150% 缩放和归档 |

## 15. 明确不做

- 不在启动时自动执行 `setup` 或修改 PATH/shell 配置。
- 不把候选版本写入新的持久缓存。
- 不增加多个 pinned 条目、收藏条目或项目到条目的绑定。
- 不扫描或接管不是由 GoDoIt GUI 登记的 Godot/编辑器进程。
- 不保存进程命令行、项目路径或环境变量，也不在 GUI 退出时自动结束 Godot。
- 不持久化 Suggest 项目路径或 operation 请求参数。
- 不允许前端编辑派生环境变量。
- 不把 error 设为可忽略。
- 不在本轮增加后台常驻、系统托盘、自动更新或深色主题。

## 16. 剩余验收

阶段 A、B、C 已按架构当前契约完成代码与自动测试。后续不再变更这些产品边界，剩余工作是：

1. 补齐 Linux 最小窗口、Wayland/X11、文件选择器和 100%/150%/200% 缩放视觉记录。
2. 在 macOS Apple Silicon 与 Windows x86_64 验证候选、环境、会话、多窗口、窗口控制和高 DPI。
3. 推送后观察三平台原生 CI 与 Release workflow 首次实际运行，修复 runner/toolchain 差异。
4. Developer ID、公证、stapling、Windows Authenticode、DMG 或安装器如需进入正式分发，另开安全设计。
5. 项目方未确认独立用户协议前不创建或展示虚构条款。

继续遵守“架构文档更新并 review -> 分阶段实现 -> 展示 -> 用户 review -> commit”的仓库流程。
