# GoDoIt Wiki & 官网

GoDoIt 的静态站点：官网首页 + 按 [Diátaxis](https://diataxis.fr/) 四象限组织的 Wiki。

## 技术栈

- [Astro](https://astro.build/)（静态生成，content collections + glob loader）
- pnpm

## 目录

```text
src/
├── content.config.ts      # wiki collection schema（四象限 + 元数据）
├── layouts/
│   ├── Base.astro         # 全站外壳（导航 + footer）
│   └── Wiki.astro         # wiki 布局（四象限侧边栏 + 正文）
├── pages/
│   ├── index.astro        # 官网首页
│   └── wiki/              # wiki 总览 + 文章路由 [...slug].astro
└── content/wiki/          # 文章：tutorials/ how-to/ reference/ explanation/
```

## 写作约定

- 文章放 `src/content/wiki/<象限>/<slug>.md`，frontmatter：

```yaml
---
title: 文章标题
description: 一句话摘要
section: how-to          # tutorials / how-to / reference / explanation
order: 1                 # 象限内排序
updated: 2026-08-19      # 可省略
---
```

- 一页只服务一个意图（Diátaxis 四象限不混写）
- 中文写作，命令示例用真实输出；内容从 `../docs/` 派生，`docs/` 仍是开发真理源

## 开发

```bash
pnpm install
pnpm dev        # 本地预览（搜索在 dev 模式不可用，索引只在 build 时生成）
pnpm build      # astro build + pagefind 索引，输出 dist/（含 dist/pagefind/）
pnpm preview    # 预览构建产物（含搜索）
```

内容页面由 `src/content/wiki/` 自动生成，无需手改路由。

## 搜索

构建后 Pagefind 自动索引 `dist/`，导航栏搜索按钮、`/` 或 `Ctrl+K` 可以打开搜索。UI 资源由 Pagefind 构建时输出到 `dist/pagefind/pagefind-ui.*`，页面按需注入，无需额外依赖。中文按语言包索引，不做词干匹配，正常搜索不受影响。
