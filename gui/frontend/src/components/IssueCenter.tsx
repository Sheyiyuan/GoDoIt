import { AlertTriangle, ChevronDown, ChevronUp, CircleAlert, Info, RefreshCw, X } from 'lucide-react'
import { useMemo, useState } from 'react'
import { NavLink } from 'react-router-dom'
import { diagnosticGroupLabels, snapshotDiagnostics, type DiagnosticGroup } from '../lib/diagnostics'
import { useAppStore } from '../store/app'

interface IssueCenterProps {
  open: boolean
  onClose: () => void
}

const groups: DiagnosticGroup[] = ['failure', 'attention', 'integration']

export function IssueCenter({ open, onClose }: IssueCenterProps) {
  const snapshot = useAppStore((state) => state.snapshot)
  const dismissed = useAppStore((state) => state.dismissedWarnings)
  const dismissWarning = useAppStore((state) => state.dismissWarning)
  const [integrationOpen, setIntegrationOpen] = useState(false)
  const entries = useMemo(() => snapshotDiagnostics(snapshot), [snapshot])

  if (!open) return null
  return (
    <div className="panel-layer" role="presentation" onMouseDown={(event) => { if (event.target === event.currentTarget) onClose() }}>
      <aside className="side-panel issue-center" role="dialog" aria-modal="true" aria-label="问题详情">
        <header className="side-panel-header"><div><strong>问题详情</strong><small>来自本次启动快照与本地 Doctor</small></div><button className="icon-button" type="button" aria-label="关闭问题详情" title="关闭" onClick={onClose}><X /></button></header>
        <div className="side-panel-actions"><button className="button" type="button" onClick={() => void useAppStore.getState().load()}><RefreshCw />重新读取</button><NavLink className="button primary" to="/doctor" onClick={onClose}>打开 Doctor</NavLink></div>
        <div className="issue-groups">
          {groups.map((group) => {
            const items = entries.filter((item) => item.group === group && !dismissed[item.id])
            const collapsed = group === 'integration' && !integrationOpen
            if (items.length === 0) return null
            return <section className={`issue-group issue-group-${group}`} key={group}>
              <button className="issue-group-heading" type="button" disabled={group !== 'integration'} onClick={() => setIntegrationOpen((value) => !value)}>
                <span>{group === 'failure' ? <CircleAlert /> : group === 'attention' ? <AlertTriangle /> : <Info />}<strong>{diagnosticGroupLabels[group]}</strong><i>{items.length}</i></span>
                {group === 'integration' && (collapsed ? <ChevronDown /> : <ChevronUp />)}
              </button>
              {!collapsed && <div className="issue-group-items">{items.map((item) => <article key={item.id}>
                <div><strong>{item.message}</strong>{item.suggest && <p>{item.suggest}</p>}{item.details?.length ? <ul>{item.details.map((detail) => <li key={detail}>{detail}</li>)}</ul> : null}</div>
                {item.status === 'warn' && group !== 'integration' && <button className="button text" type="button" onClick={() => dismissWarning(item.id)}>本次关闭</button>}
              </article>)}</div>}
            </section>
          })}
          {entries.every((item) => dismissed[item.id]) && <div className="empty-inline">本次启动没有待显示的问题</div>}
        </div>
      </aside>
    </div>
  )
}
