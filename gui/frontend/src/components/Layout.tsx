import { useEffect, useState } from 'react'
import { CircleHelp, Maximize2, Menu, Minimize2, Minus, PackageOpen, Plus, RefreshCw, Settings, Wrench, X } from 'lucide-react'
import { NavLink, Outlet, useLocation } from 'react-router-dom'
import { useAppStore } from '../store/app'
import { closeWindow, isWindowMaximised, minimiseWindow, resolveTitlebarStyle, toggleMaximiseWindow } from '../lib/window'
import type { TitlebarStyle } from '../types'
import { BrandMark } from './BrandMark'
import { IconAvatar } from './IconAvatar'
import { Modal } from './Modal'
import { IssueCenter } from './IssueCenter'
import { OperationTray } from './OperationTray'
import { OperationButton, OperationCenter } from './OperationCenter'
import { snapshotDiagnostics, visibleReminderCount } from '../lib/diagnostics'

export function Layout() {
  const snapshot = useAppStore((state) => state.snapshot)
  const toast = useAppStore((state) => state.toast)
  const [sidebarOpen, setSidebarOpen] = useState(false)
  const [effectiveTitlebarStyle, setEffectiveTitlebarStyle] = useState<Exclude<TitlebarStyle, 'auto'>>('mac')
  const [maximised, setMaximised] = useState(false)
  const [issuesOpen, setIssuesOpen] = useState(false)
  const [operationsOpen, setOperationsOpen] = useState(false)
  const location = useLocation()
  const closeSidebar = () => setSidebarOpen(false)
  const toolsActive = location.pathname === '/tools' || location.pathname === '/suggest' || location.pathname === '/doctor' || location.pathname.startsWith('/resources/')
  const dismissedWarnings = useAppStore((state) => state.dismissedWarnings)
  const diagnostics = snapshotDiagnostics(snapshot)
  const reminderCount = visibleReminderCount(diagnostics, dismissedWarnings)
  const firstIssue = diagnostics.find((item) => item.group !== 'integration' && !dismissedWarnings[item.id])

  useEffect(() => {
    let active = true
    void resolveTitlebarStyle(snapshot?.gui.titlebar_style || 'auto').then((style) => { if (active) setEffectiveTitlebarStyle(style) })
    return () => { active = false }
  }, [snapshot?.gui.titlebar_style])

  useEffect(() => {
    let active = true
    void isWindowMaximised().then((value) => { if (active) setMaximised(value) })
    return () => { active = false }
  }, [])

  const toggleMaximise = () => {
    toggleMaximiseWindow()
    setMaximised((value) => !value)
  }

  const windowControls = effectiveTitlebarStyle === 'mac' ? (
    <div className="window-controls window-controls-mac" aria-label="窗口控制">
      <button className="window-control window-control-close" type="button" aria-label="关闭窗口" title="关闭窗口" onClick={closeWindow}><X /></button>
      <button className="window-control window-control-minimise" type="button" aria-label="最小化窗口" title="最小化窗口" onClick={minimiseWindow}><Minus /></button>
      <button className="window-control window-control-maximise" type="button" aria-label={maximised ? '还原窗口' : '最大化窗口'} title={maximised ? '还原窗口' : '最大化窗口'} onClick={toggleMaximise}>{maximised ? <Minimize2 /> : <Maximize2 />}</button>
    </div>
  ) : (
    <div className="window-controls window-controls-windows" aria-label="窗口控制">
      <button className="window-control" type="button" aria-label="最小化窗口" title="最小化窗口" onClick={minimiseWindow}><Minus /></button>
      <button className="window-control" type="button" aria-label={maximised ? '还原窗口' : '最大化窗口'} title={maximised ? '还原窗口' : '最大化窗口'} onClick={toggleMaximise}>{maximised ? <Minimize2 /> : <Maximize2 />}</button>
      <button className="window-control window-control-close" type="button" aria-label="关闭窗口" title="关闭窗口" onClick={closeWindow}><X /></button>
    </div>
  )

  return (
    <div className="app-shell">
      <header className={`titlebar titlebar-${effectiveTitlebarStyle}`}>
        {effectiveTitlebarStyle === 'mac' && windowControls}
        <button className="icon-button sidebar-toggle" type="button" aria-label="切换侧栏" title="切换侧栏" onClick={() => setSidebarOpen((open) => !open)}><Menu /></button>
        <BrandMark />
        <strong className="titlebar-name">GoDoIt</strong><span className="titlebar-subtitle">够独特</span>
        <div className="titlebar-status">{snapshot?.current ? <>当前 <b>{snapshot.current.name}</b></> : '尚未设置当前条目'}</div>
        <OperationButton onClick={() => setOperationsOpen(true)} />
        {effectiveTitlebarStyle === 'windows' && windowControls}
      </header>
      <aside className={`sidebar ${sidebarOpen ? 'sidebar-open' : ''}`}>
        <div className="sidebar-heading"><h1>条目</h1><p>{snapshot?.instances.length || 0} 个启动配置</p></div>
        <nav aria-label="条目">
          <NavLink to="/instances/new" className="new-instance-link" onClick={closeSidebar}><span><Plus /></span>新建条目</NavLink>
          <div className="instance-list">
            {snapshot?.instances.map((instance) => (
              <NavLink key={instance.id} to={`/instances/${encodeURIComponent(instance.name)}`} onClick={closeSidebar} className={({ isActive }) => isActive || (location.pathname === '/instances' && instance.current) ? 'instance-link active' : 'instance-link'}>
                <IconAvatar instance={instance} />
                <span className="instance-link-copy"><strong>{instance.name}</strong><small>{instance.engine.replace(`-${instance.edition}`, '')} · {instance.edition === 'dotnet' ? '.NET' : '普通版'}</small></span>
                {instance.current && <em>当前</em>}
              </NavLink>
            ))}
          </div>
        </nav>
        <nav className="bottom-nav" aria-label="应用">
          <NavLink to="/tools" className={toolsActive ? 'active' : undefined} aria-current={toolsActive ? 'page' : undefined} onClick={closeSidebar}><Wrench />工具{reminderCount > 0 && <i>{reminderCount}</i>}</NavLink>
          <NavLink to="/settings" onClick={closeSidebar}><Settings />设置</NavLink>
          <NavLink to="/about" onClick={closeSidebar}><CircleHelp />关于</NavLink>
        </nav>
      </aside>
      <main className="main-content">
        {reminderCount > 0 && firstIssue && <div className={`bootstrap-issues bootstrap-issues-${firstIssue.group}`} role="status"><strong>发现 {reminderCount} 项问题</strong><span>{firstIssue.message}</span><button className="button text" type="button" onClick={() => setIssuesOpen(true)}>查看详情</button><button className="icon-button" type="button" aria-label="重新读取" title="重新读取" onClick={() => void useAppStore.getState().load()}><RefreshCw /></button></div>}
        <Outlet />
      </main>
      {sidebarOpen && <button className="sidebar-scrim" aria-label="关闭侧栏" onClick={closeSidebar} />}
      <OperationTray onOpen={() => setOperationsOpen(true)} />
      <OperationCenter open={operationsOpen} onClose={() => setOperationsOpen(false)} />
      <IssueCenter open={issuesOpen} onClose={() => setIssuesOpen(false)} />
      <Modal />
      {toast && <div className="toast" role="status"><PackageOpen />{toast}</div>}
    </div>
  )
}
