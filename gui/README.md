# GoDoIt GUI

该模块是 GoDoIt 的 Wails v2 + React 桌面入口。业务规则全部来自 `../core`；bridge 只负责
生命周期、参数转换、系统选择器和结构化进度事件。

```bash
cd frontend && pnpm run check
cd frontend && pnpm run build
GOCACHE=/tmp/godoit-gui-cache wails build -clean -tags webkit2_41
```

也可以在仓库根目录运行 `make build-gui`。Linux 的 WebKit 构建标签由
`WAILS_BUILD_TAGS` 控制；macOS 或 Windows 构建时可显式传空值。

浏览器开发模式使用内存 fixture，Wails 运行时存在时会自动切换到生成的 bridge 绑定：

```bash
cd frontend && pnpm run dev
```

## 品牌资源

- `frontend/public/brand/godot.svg` 来自 Godot 官方 press icon：
  `https://godotengine.org/assets/press/icon_color.svg`
- `frontend/public/brand/csharp.svg` 使用项目确认的 C# 品牌 SVG。
- `frontend/public/logo.svg` 是项目 `assets/logo.svg` 的 GUI 副本；`frontend/public/mascot.png`
  是项目吉祥物资源。
