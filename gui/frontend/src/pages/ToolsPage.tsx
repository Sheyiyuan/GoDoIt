import { ArchiveX, Box, ChevronRight, FolderSearch2, HeartPulse, Package, Waypoints } from 'lucide-react'
import type { ReactNode } from 'react'
import { Link } from 'react-router-dom'
import { useAppStore } from '../store/app'
import { formatBytes } from '../utils'

interface ToolLinkProps {
  badge?: string
  badgeTone?: 'ok' | 'warning' | 'error'
  detail: string
  icon: ReactNode
  title: string
  to: string
  tone: 'blue' | 'green' | 'orange' | 'purple'
}

export function ToolsPage() {
  const snapshot = useAppStore((state) => state.snapshot)
  if (!snapshot) return null

  const issueCount = snapshot.doctor.error_count + snapshot.doctor.warn_count
  const issueTone = snapshot.doctor.error_count ? 'error' : issueCount ? 'warning' : 'ok'
  const enabledSources = snapshot.assets.sources.filter((source) => !source.disabled).length
  const orphanSize = snapshot.assets.orphans.reduce((total, asset) => total + asset.size, 0)

  return (
    <div className="page tools-page">
      <header className="page-header"><div><p className="eyebrow">工作台</p><h1>工具</h1></div></header>

      <section className="settings-band tools-band">
        <div className="section-heading"><div><h2>项目建议</h2><p>只读项目分析</p></div></div>
        <nav className="tool-page-list" aria-label="项目建议">
          <ToolLink to="/suggest" title="分析 Godot 项目" detail="project.godot / global.json / *.csproj" icon={<FolderSearch2 />} tone="blue" />
        </nav>
      </section>

      <section className="settings-band tools-band">
        <div className="section-heading"><div><h2>环境诊断</h2><p>本机与资产状态</p></div></div>
        <nav className="tool-page-list" aria-label="环境诊断">
          <ToolLink to="/doctor" title="Doctor" detail={`${snapshot.doctor.ok_count} 项正常 · ${snapshot.doctor.warn_count} 项警告 · ${snapshot.doctor.error_count} 项错误`} badge={issueCount ? `${issueCount} 项提醒` : '正常'} badgeTone={issueTone} icon={<HeartPulse />} tone={issueCount ? 'orange' : 'green'} />
        </nav>
      </section>

      <section className="settings-band tools-band">
        <div className="section-heading"><div><h2>资源管理</h2><p>已安装资产、来源与缓存</p></div></div>
        <nav className="tool-page-list" aria-label="资源管理">
          <ToolLink to="/resources/engines" title="引擎" detail={`${snapshot.assets.engines.length} 个已安装`} icon={<Package />} tone="blue" />
          <ToolLink to="/resources/sdks" title=".NET SDK" detail={`${snapshot.assets.sdks.length} 个可用 SDK`} icon={<Box />} tone="purple" />
          <ToolLink to="/resources/sources" title="下载来源" detail={`${enabledSources} / ${snapshot.assets.sources.length} 个已启用`} icon={<Waypoints />} tone="green" />
          <ToolLink to="/resources/cache" title="缓存与孤儿" detail={`${snapshot.assets.orphans.length} 个孤儿资产 · ${formatBytes(orphanSize)}`} icon={<ArchiveX />} tone="orange" />
        </nav>
      </section>
    </div>
  )
}

function ToolLink({ badge, badgeTone, detail, icon, title, to, tone }: ToolLinkProps) {
  return (
    <Link className="tool-page-link" to={to}>
      <span className={`tool-page-icon tool-page-icon-${tone}`}>{icon}</span>
      <span className="tool-page-copy"><strong>{title}</strong><small>{detail}</small></span>
      {badge && <span className={`tool-page-status tool-page-status-${badgeTone}`}>{badge}</span>}
      <ChevronRight className="tool-page-chevron" />
    </Link>
  )
}
